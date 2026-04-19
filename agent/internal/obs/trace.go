package obs

import (
	"context"
	"encoding/json"
	"time"
)

// TurnMetrics records all timing and diagnostic data for a single agent turn.
type TurnMetrics struct {
	TurnID              string `json:"turn_id"`
	ProviderName        string `json:"provider_name"`
	FirstTokenLatencyMs int64  `json:"first_token_latency_ms"`
	ToolLatencyMs       int64  `json:"tool_latency_ms"`
	StyleLatencyMs      int64  `json:"style_latency_ms"`
	TotalTurnLatencyMs  int64  `json:"total_turn_latency_ms"`
	FallbackReason      string `json:"fallback_reason,omitempty"`
	TimeoutStage        string `json:"timeout_stage,omitempty"`
	RewriteEnabled      bool   `json:"rewrite_enabled"`
	RewriteMode         string `json:"rewrite_mode,omitempty"`
	ToolCallCount       int    `json:"tool_call_count"`
	InputTokens         int    `json:"input_tokens"`
	OutputTokens        int    `json:"output_tokens"`
}

// Tracer collects timing checkpoints during a turn and emits the final metrics.
type Tracer struct {
	metrics   TurnMetrics
	turnStart time.Time
}

func NewTracer(turnID, providerName string) *Tracer {
	return &Tracer{
		metrics: TurnMetrics{
			TurnID:       turnID,
			ProviderName: providerName,
		},
		turnStart: time.Now(),
	}
}

func (t *Tracer) SetFirstTokenLatency(d time.Duration) {
	t.metrics.FirstTokenLatencyMs = d.Milliseconds()
}

func (t *Tracer) AddToolLatency(d time.Duration) {
	t.metrics.ToolLatencyMs += d.Milliseconds()
}

func (t *Tracer) SetStyleLatency(d time.Duration) {
	t.metrics.StyleLatencyMs = d.Milliseconds()
}

func (t *Tracer) SetFallback(reason string) {
	t.metrics.FallbackReason = reason
}

func (t *Tracer) SetTimeoutStage(stage string) {
	t.metrics.TimeoutStage = stage
}

func (t *Tracer) SetRewrite(enabled bool, mode string) {
	t.metrics.RewriteEnabled = enabled
	t.metrics.RewriteMode = mode
}

func (t *Tracer) AddToolCall() {
	t.metrics.ToolCallCount++
}

func (t *Tracer) SetTokens(input, output int) {
	t.metrics.InputTokens = input
	t.metrics.OutputTokens = output
}

func (t *Tracer) Finish(ctx context.Context) TurnMetrics {
	t.metrics.TotalTurnLatencyMs = time.Since(t.turnStart).Milliseconds()
	b, _ := json.Marshal(t.metrics)
	Info(ctx, "turn_metrics", "metrics", string(b))
	defaultStore.Push(t.metrics)
	return t.metrics
}
