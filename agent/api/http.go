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

// staticDir is the path to the static assets directory, relative to the working directory.
const staticDir = "static"

// Handler wires the HTTP API to the runtime engine.
type Handler struct {
	engine *runtime.Engine
	store  *runtime.SessionStore
	mux    *http.ServeMux
}

func NewHandler(engine *runtime.Engine, store *runtime.SessionStore) *Handler {
	h := &Handler{
		engine: engine,
		store:  store,
		mux:    http.NewServeMux(),
	}
	h.mux.HandleFunc("POST /sessions", h.createSession)
	h.mux.HandleFunc("GET /sessions/{id}", h.getSession)
	h.mux.HandleFunc("DELETE /sessions/{id}", h.deleteSession)
	h.mux.HandleFunc("POST /sessions/{id}/turns", h.createTurn)
	h.mux.HandleFunc("GET /health", h.health)
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
	h.store.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

// POST /sessions/{id}/turns
// Accepts JSON body: {"message": "..."}
// Streams response as SSE if Accept: text/event-stream, otherwise returns JSON.
func (h *Handler) createTurn(w http.ResponseWriter, r *http.Request) {
	sessID := r.PathValue("id")
	sess, ok := h.store.Get(sessID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		fmt.Printf("writeJSON encode error: %v\n", err)
	}
}
