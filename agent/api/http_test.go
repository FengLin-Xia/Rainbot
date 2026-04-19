package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/xia-rain/go_agent/api"
	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/obs"
	"github.com/xia-rain/go_agent/internal/prompt"
	"github.com/xia-rain/go_agent/internal/runtime"
	"github.com/xia-rain/go_agent/internal/tool"
)

// stubLLM satisfies llm.ModelClient; never actually called in validation tests.
type stubLLM struct{}

func (s *stubLLM) Name() string { return "stub" }
func (s *stubLLM) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}, nil
}
func (s *stubLLM) Stream(_ context.Context, _ llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func newTestHandler(t *testing.T) (http.Handler, *runtime.SessionStore) {
	t.Helper()
	registry := tool.NewRegistry()
	executor := tool.NewExecutor(registry, 0)
	engine := runtime.NewEngine(runtime.EngineConfig{
		LLM:      &stubLLM{},
		Tools:    executor,
		Registry: registry,
		Prompt:   prompt.New("", 0),
	})
	store := runtime.NewSessionStore()
	h := api.NewHandler(engine, store, obs.DefaultMetricsStore())
	return h, store
}

func createSession(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("createSession: got %d, want 201", w.Code)
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	return body.SessionID
}

// ── Session ID validation ──────────────────────────────────────────────────

func TestCreateTurn_InvalidSessionID_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sessions/not-a-uuid/turns",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetSession_InvalidSessionID_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/sessions/bad-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteSession_InvalidSessionID_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/sessions/bad-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ── Message validation ─────────────────────────────────────────────────────

func TestCreateTurn_EmptyMessage_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	sessID := createSession(t, h)

	body := `{"message":"   "}`
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+sessID+"/turns",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateTurn_MessageTooLong_Returns413(t *testing.T) {
	h, _ := newTestHandler(t)
	sessID := createSession(t, h)

	longMsg := strings.Repeat("a", 33*1024) // 33KB > 32KB limit
	payload, _ := json.Marshal(map[string]string{"message": longMsg})
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+sessID+"/turns",
		bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
}

func TestCreateTurn_BodyTooLarge_Returns413(t *testing.T) {
	h, _ := newTestHandler(t)
	sessID := createSession(t, h)

	// Craft a body > 64KB but whose JSON message field itself is within limits
	// by padding with a large unknown field.
	bigBody := `{"message":"hello","padding":"` + strings.Repeat("x", 65*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+sessID+"/turns",
		strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
}

// ── Health & session lifecycle ─────────────────────────────────────────────

func TestHealth_Returns200(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestCreateSession_ReturnsUUID(t *testing.T) {
	h, _ := newTestHandler(t)
	sessID := createSession(t, h)
	if _, err := uuid.Parse(sessID); err != nil {
		t.Errorf("session_id %q is not a valid UUID: %v", sessID, err)
	}
}

func TestGetSession_ExistingSession_Returns200(t *testing.T) {
	h, _ := newTestHandler(t)
	sessID := createSession(t, h)
	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetSession_UnknownSession_Returns404(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/sessions/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
