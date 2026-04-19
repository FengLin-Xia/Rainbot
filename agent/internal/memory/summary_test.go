package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/memory"
)

type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Name() string { return "mock" }
func (m *mockLLM) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.GenerateResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: m.response},
	}, nil
}
func (m *mockLLM) Stream(_ context.Context, _ llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func TestSummaryMemory_CompressStoresSummary(t *testing.T) {
	s := memory.NewSummaryMemory()
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "what is 2+2"},
		{Role: llm.RoleAssistant, Content: "4"},
	}
	if err := s.Compress(context.Background(), msgs, &mockLLM{response: "User asked math, answer was 4."}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.Get(); got != "User asked math, answer was 4." {
		t.Errorf("Get() = %q, want summary text", got)
	}
}

func TestSummaryMemory_CompressEmptyMsgs_NoOp(t *testing.T) {
	s := memory.NewSummaryMemory()
	if err := s.Compress(context.Background(), nil, &mockLLM{response: "should not be called"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.Get(); got != "" {
		t.Errorf("Get() = %q, want empty (no messages to compress)", got)
	}
}

func TestSummaryMemory_CompressLLMError_ReturnsError(t *testing.T) {
	s := memory.NewSummaryMemory()
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "hello"}}
	err := s.Compress(context.Background(), msgs, &mockLLM{err: errors.New("llm down")})
	if err == nil {
		t.Error("expected error on LLM failure, got nil")
	}
	if got := s.Get(); got != "" {
		t.Errorf("summary should remain empty on error, got %q", got)
	}
}

func TestSummaryMemory_CompressFiltersToolAndSystemMessages(t *testing.T) {
	s := memory.NewSummaryMemory()
	var captured llm.GenerateRequest
	client := &captureClient{}
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "system prompt"},
		{Role: llm.RoleUser, Content: "user message"},
		{Role: llm.RoleTool, Content: "tool output"},
		{Role: llm.RoleAssistant, Content: "assistant reply"},
	}
	client.fn = func(req llm.GenerateRequest) {
		captured = req
	}
	_ = s.Compress(context.Background(), msgs, client)

	userMsg := captured.Messages[1].Content
	if contains(userMsg, "system prompt") {
		t.Error("system message should be filtered from summary input")
	}
	if contains(userMsg, "tool output") {
		t.Error("tool message should be filtered from summary input")
	}
	if !contains(userMsg, "user message") || !contains(userMsg, "assistant reply") {
		t.Error("user and assistant messages should be included")
	}
}

// ── ShortTermMemory drain tests ────────────────────────────────────────────

func TestShortTerm_DrainOldestIfAbove_BelowThreshold(t *testing.T) {
	m := memory.NewShortTerm(10)
	appendPairs(m, 3) // 6 messages, threshold 8
	drained := m.DrainOldestIfAbove(8, 4)
	if drained != nil {
		t.Errorf("expected nil (below threshold), got %d messages", len(drained))
	}
}

func TestShortTerm_DrainOldestIfAbove_AboveThreshold(t *testing.T) {
	m := memory.NewShortTerm(10)
	appendPairs(m, 5) // 10 messages, threshold 8
	drained := m.DrainOldestIfAbove(8, 4)
	if len(drained) != 4 {
		t.Errorf("len(drained) = %d, want 4", len(drained))
	}
	if m.Len() != 6 {
		t.Errorf("remaining Len() = %d, want 6", m.Len())
	}
}

func TestShortTerm_DrainOldestIfAbove_RoundsToPairs(t *testing.T) {
	m := memory.NewShortTerm(10)
	appendPairs(m, 5)
	drained := m.DrainOldestIfAbove(8, 3) // 3 rounded down to 2
	if len(drained) != 2 {
		t.Errorf("len(drained) = %d, want 2 (rounded to pair)", len(drained))
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func appendPairs(m *memory.ShortTermMemory, pairs int) {
	for i := 0; i < pairs; i++ {
		m.Append(llm.Message{Role: llm.RoleUser, Content: "q"})
		m.Append(llm.Message{Role: llm.RoleAssistant, Content: "a"})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

type captureClient struct {
	fn func(llm.GenerateRequest)
}

func (c *captureClient) Name() string { return "capture" }
func (c *captureClient) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if c.fn != nil {
		c.fn(req)
	}
	return &llm.GenerateResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "summary"}}, nil
}
func (c *captureClient) Stream(_ context.Context, _ llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}
