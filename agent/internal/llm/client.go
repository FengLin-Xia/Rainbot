package llm

import (
	"context"
	"encoding/json"
)

// Role constants for messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is a single entry in the conversation history.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall represents a function call requested by the model.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// GenerateRequest is the input to a model call.
type GenerateRequest struct {
	Messages    []Message
	Tools       []ToolDefinition
	MaxTokens   int
	Temperature float32
	JSONMode    bool // request JSON output
}

// GenerateResponse is the complete (non-streaming) output.
type GenerateResponse struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

// StreamChunk is one piece of a streaming response.
type StreamChunk struct {
	// Delta is incremental text content.
	Delta string
	// ToolCallDelta carries a partial tool call; accumulate by index.
	ToolCallDelta *ToolCallDelta
	FinishReason  string
	// Err is set on terminal errors.
	Err error
}

// ToolCallDelta is a streaming fragment of a tool call.
type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	ArgsDelta string
}

// Usage reports token consumption.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// ModelClient is the unified interface for all LLM providers.
// Both large reasoning models and small style models implement this.
type ModelClient interface {
	// Generate performs a blocking, non-streaming call.
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	// Stream returns a channel of incremental chunks. The channel is closed
	// when the response is complete or an error occurs.
	Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error)
	// Name returns a human-readable provider/model identifier.
	Name() string
}
