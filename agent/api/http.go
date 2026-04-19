package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xia-rain/go_agent/internal/obs"
	"github.com/xia-rain/go_agent/internal/runtime"
)

const (
	staticDir      = "static"
	maxBodyBytes   = 64 * 1024       // 64 KB
	maxMessageLen  = 32 * 1024       // 32 KB
)

// Handler wires the HTTP API to the runtime engine.
type Handler struct {
	engine       *runtime.Engine
	store        *runtime.SessionStore
	metricsStore *obs.MetricsStore
	mux          *http.ServeMux
}

func NewHandler(engine *runtime.Engine, store *runtime.SessionStore, metricsStore *obs.MetricsStore) *Handler {
	h := &Handler{
		engine:       engine,
		store:        store,
		metricsStore: metricsStore,
		mux:          http.NewServeMux(),
	}
	h.mux.HandleFunc("POST /sessions", h.createSession)
	h.mux.HandleFunc("GET /sessions/{id}", h.getSession)
	h.mux.HandleFunc("DELETE /sessions/{id}", h.deleteSession)
	h.mux.HandleFunc("POST /sessions/{id}/turns", h.createTurn)
	h.mux.HandleFunc("GET /health", h.health)
	h.mux.HandleFunc("GET /metrics", h.metrics)
	h.mux.Handle("GET /", http.FileServer(http.Dir(staticDir)))
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// POST /sessions
func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	id := uuid.New().String()
	h.store.Create(id)
	writeJSON(w, http.StatusCreated, map[string]string{"session_id": id})
}

// GET /sessions/{id}
func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}
	sess, ok := h.store.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sess.ID,
		"created_at": sess.CreatedAt.Format(time.RFC3339),
	})
}

// DELETE /sessions/{id}
func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}
	h.store.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

// POST /sessions/{id}/turns
// Accepts JSON body: {"message": "..."}
// Streams response as SSE if Accept: text/event-stream, otherwise returns JSON.
func (h *Handler) createTurn(w http.ResponseWriter, r *http.Request) {
	sessID := r.PathValue("id")
	if _, err := uuid.Parse(sessID); err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}
	sess, ok := h.store.Get(sessID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if err.Error() == "http: request body too large" {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	body.Message = strings.TrimSpace(body.Message)
	if body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if len(body.Message) > maxMessageLen {
		http.Error(w, "message too long", http.StatusRequestEntityTooLarge)
		return
	}

	turnID := uuid.New().String()
	ctx := r.Context()

	wantsSSE := strings.Contains(r.Header.Get("Accept"), "text/event-stream")

	textCh, resultCh, err := h.engine.ProcessTurn(ctx, sess, body.Message, turnID)
	if err != nil {
		obs.Error(ctx, "process_turn_error", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if wantsSSE {
		h.streamSSE(w, r, turnID, textCh, resultCh)
	} else {
		h.collectBlocking(w, r, turnID, textCh, resultCh)
	}
}

func (h *Handler) streamSSE(
	w http.ResponseWriter,
	r *http.Request,
	turnID string,
	textCh <-chan string,
	resultCh <-chan runtime.TurnResult,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Turn-ID", turnID)
	w.WriteHeader(http.StatusOK)

	sw := runtime.NewStreamWriter(w)
	flusher, canFlush := w.(http.Flusher)

	for chunk := range textCh {
		if err := sw.WriteText(chunk); err != nil {
			return
		}
		if canFlush {
			flusher.Flush()
		}
	}

	// Drain resultCh (metrics are already logged internally).
	<-resultCh

	_ = sw.WriteDone()
	if canFlush {
		flusher.Flush()
	}
}

func (h *Handler) collectBlocking(
	w http.ResponseWriter,
	_ *http.Request,
	turnID string,
	textCh <-chan string,
	resultCh <-chan runtime.TurnResult,
) {
	var sb strings.Builder
	for chunk := range textCh {
		sb.WriteString(chunk)
	}
	result := <-resultCh

	writeJSON(w, http.StatusOK, map[string]any{
		"turn_id":       turnID,
		"output":        result.Output,
		"style_applied": result.StyleApplied,
		"metrics":       result.Metrics,
	})
}

// GET /health
func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /metrics
func (h *Handler) metrics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.metricsStore.Recent(50))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		fmt.Printf("writeJSON encode error: %v\n", err)
	}
}
