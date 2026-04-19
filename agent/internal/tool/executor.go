package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/xia-rain/go_agent/internal/obs"
)

// Executor runs tool calls with timeout enforcement and logging.
type Executor struct {
	registry *Registry
	timeout  time.Duration
}

func NewExecutor(registry *Registry, timeout time.Duration) *Executor {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Executor{registry: registry, timeout: timeout}
}

// ExecuteResult is the output of a single tool execution.
type ExecuteResult struct {
	ToolCallID string
	ToolName   string
	Output     string
	Err        error
	LatencyMs  int64
}

// BatchCall describes a single tool invocation within a concurrent batch.
type BatchCall struct {
	CallID string
	Name   string
	Args   json.RawMessage
}

// ExecuteMany runs all calls concurrently and returns results in the same order.
func (e *Executor) ExecuteMany(ctx context.Context, calls []BatchCall) []ExecuteResult {
	results := make([]ExecuteResult, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		wg.Add(1)
		go func(idx int, bc BatchCall) {
			defer wg.Done()
			results[idx] = e.Execute(ctx, bc.CallID, bc.Name, bc.Args)
		}(i, c)
	}
	wg.Wait()
	return results
}

// Execute runs a single tool call identified by name and arguments.
func (e *Executor) Execute(ctx context.Context, callID, name string, args json.RawMessage) ExecuteResult {
	t, ok := e.registry.Get(name)
	if !ok {
		return ExecuteResult{
			ToolCallID: callID,
			ToolName:   name,
			Err:        fmt.Errorf("tool %q not found", name),
		}
	}

	tCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	start := time.Now()
	obs.Debug(ctx, "tool_start", "tool", name, "call_id", callID)

	output, err := t.Handler(tCtx, args)
	latency := time.Since(start)

	if err != nil {
		obs.Warn(ctx, "tool_error", "tool", name, "error", err.Error(), "latency_ms", latency.Milliseconds())
	} else {
		obs.Debug(ctx, "tool_done", "tool", name, "latency_ms", latency.Milliseconds())
	}

	return ExecuteResult{
		ToolCallID: callID,
		ToolName:   name,
		Output:     output,
		Err:        err,
		LatencyMs:  latency.Milliseconds(),
	}
}
