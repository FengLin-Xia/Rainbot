package tool_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xia-rain/go_agent/internal/tool"
)

func newTestRegistry(t *testing.T) *tool.Registry {
	t.Helper()
	r := tool.NewRegistry()
	r.MustRegister(tool.Tool{
		Name:        "echo",
		Description: "echoes input text",
		Parameters:  []byte(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		Handler: func(_ context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}
			return p.Text, nil
		},
	})
	r.MustRegister(tool.Tool{
		Name:        "slow",
		Description: "sleeps 50ms then returns done",
		Parameters:  []byte(`{"type":"object"}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			select {
			case <-time.After(50 * time.Millisecond):
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	})
	return r
}

func TestExecuteMany_ResultsOrderMatchesInput(t *testing.T) {
	exec := tool.NewExecutor(newTestRegistry(t), 5*time.Second)

	calls := []tool.BatchCall{
		{CallID: "c1", Name: "echo", Args: json.RawMessage(`{"text":"alpha"}`)},
		{CallID: "c2", Name: "echo", Args: json.RawMessage(`{"text":"beta"}`)},
		{CallID: "c3", Name: "echo", Args: json.RawMessage(`{"text":"gamma"}`)},
	}

	results := exec.ExecuteMany(context.Background(), calls)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	want := []string{"alpha", "beta", "gamma"}
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("results[%d].Err = %v", i, r.Err)
		}
		if r.Output != want[i] {
			t.Errorf("results[%d].Output = %q, want %q", i, r.Output, want[i])
		}
		if r.ToolCallID != calls[i].CallID {
			t.Errorf("results[%d].ToolCallID = %q, want %q", i, r.ToolCallID, calls[i].CallID)
		}
	}
}

func TestExecuteMany_RunsConcurrently(t *testing.T) {
	exec := tool.NewExecutor(newTestRegistry(t), 5*time.Second)

	calls := []tool.BatchCall{
		{CallID: "s1", Name: "slow"},
		{CallID: "s2", Name: "slow"},
		{CallID: "s3", Name: "slow"},
	}

	start := time.Now()
	results := exec.ExecuteMany(context.Background(), calls)
	elapsed := time.Since(start)

	for i, r := range results {
		if r.Err != nil {
			t.Errorf("results[%d].Err = %v", i, r.Err)
		}
	}
	// 3 serial 50ms tools = ~150ms; concurrent should be well under that
	if elapsed > 120*time.Millisecond {
		t.Errorf("elapsed %v suggests sequential execution (3 × 50ms concurrent should finish < 120ms)", elapsed)
	}
}

func TestExecuteMany_UnknownToolReturnsError(t *testing.T) {
	exec := tool.NewExecutor(newTestRegistry(t), 5*time.Second)

	results := exec.ExecuteMany(context.Background(), []tool.BatchCall{
		{CallID: "x", Name: "does_not_exist"},
	})

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected error for unknown tool, got nil")
	}
}

func TestExecuteMany_EmptyBatch(t *testing.T) {
	exec := tool.NewExecutor(newTestRegistry(t), 5*time.Second)
	results := exec.ExecuteMany(context.Background(), nil)
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}
