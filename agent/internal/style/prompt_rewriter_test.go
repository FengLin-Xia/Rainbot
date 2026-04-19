package style_test

import (
	"context"
	"errors"
	"testing"

	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/style"
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

func TestPromptRewriter_MustKeepAllPresent(t *testing.T) {
	rw := style.NewPromptRewriter(&mockLLM{response: "the answer includes v1.2.3 and step 1"})
	resp, err := rw.Rewrite(context.Background(), style.StyleRewriteRequest{
		FinalAnswer:  "original",
		MustKeep:     []string{"v1.2.3", "step 1"},
		StyleProfile: style.StyleBlunt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Applied {
		t.Error("Applied = false, want true (all MustKeep items present in output)")
	}
	if !resp.ValidationPassed {
		t.Error("ValidationPassed = false, want true")
	}
}

func TestPromptRewriter_MustKeepViolation_FallsBackToOriginal(t *testing.T) {
	rw := style.NewPromptRewriter(&mockLLM{response: "rewritten text without the required literal"})
	resp, err := rw.Rewrite(context.Background(), style.StyleRewriteRequest{
		FinalAnswer:  "original with v1.2.3",
		MustKeep:     []string{"v1.2.3"},
		StyleProfile: style.StyleBlunt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Applied {
		t.Error("Applied = true, want false (MustKeep violated)")
	}
	if resp.ValidationPassed {
		t.Error("ValidationPassed = true, want false")
	}
	if resp.OutputText != "original with v1.2.3" {
		t.Errorf("OutputText = %q, want original answer as fallback", resp.OutputText)
	}
	if resp.FallbackReason == "" {
		t.Error("FallbackReason is empty, want mustkeep_violation message")
	}
}

func TestPromptRewriter_EmptyMustKeep_NoValidation(t *testing.T) {
	rw := style.NewPromptRewriter(&mockLLM{response: "rewritten text"})
	resp, err := rw.Rewrite(context.Background(), style.StyleRewriteRequest{
		FinalAnswer:  "original",
		MustKeep:     nil,
		StyleProfile: style.StyleBlunt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Applied {
		t.Error("Applied = false, want true (no MustKeep constraints to fail)")
	}
}

func TestPromptRewriter_LLMError_FallsBackToOriginal(t *testing.T) {
	rw := style.NewPromptRewriter(&mockLLM{err: errors.New("llm unavailable")})
	resp, err := rw.Rewrite(context.Background(), style.StyleRewriteRequest{
		FinalAnswer:  "original answer",
		StyleProfile: style.StyleBlunt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Applied {
		t.Error("Applied = true on LLM error, want false")
	}
	if resp.OutputText != "original answer" {
		t.Errorf("OutputText = %q, want original answer", resp.OutputText)
	}
}

func TestPromptRewriter_EmptyLLMOutput_FallsBackToOriginal(t *testing.T) {
	rw := style.NewPromptRewriter(&mockLLM{response: "   "}) // whitespace only
	resp, err := rw.Rewrite(context.Background(), style.StyleRewriteRequest{
		FinalAnswer:  "original",
		StyleProfile: style.StyleBlunt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Applied {
		t.Error("Applied = true on empty output, want false")
	}
}
