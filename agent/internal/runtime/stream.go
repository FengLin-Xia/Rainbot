package runtime

import (
	"encoding/json"
	"fmt"
	"io"
)

// EventType identifies the kind of StreamEvent.
type EventType string

const (
	EventText      EventType = "text"
	EventToolStart EventType = "tool_start"
	EventToolDone  EventType = "tool_done"
	EventDone      EventType = "done"
	EventError     EventType = "error"
)

// ToolEvent carries tool call metadata for EventToolStart and EventToolDone.
type ToolEvent struct {
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// StreamEvent is the unified event type sent through the ProcessTurn channel.
type StreamEvent struct {
	Type   EventType   `json:"type"`
	Text   string      `json:"text,omitempty"`
	Tool   *ToolEvent  `json:"tool,omitempty"`
	Result *TurnResult `json:"result,omitempty"`
	ErrMsg string      `json:"error,omitempty"`
}

// StreamWriter writes SSE-formatted events to an http.ResponseWriter or
// any io.Writer.
type StreamWriter struct {
	w io.Writer
}

func NewStreamWriter(w io.Writer) *StreamWriter {
	return &StreamWriter{w: w}
}

// WriteText sends an incremental text chunk.
func (s *StreamWriter) WriteText(chunk string) error {
	_, err := fmt.Fprintf(s.w, "data: %s\n\n", escapeSSE(chunk))
	return err
}

// WriteDone sends the terminal event.
func (s *StreamWriter) WriteDone() error {
	_, err := fmt.Fprintf(s.w, "data: [DONE]\n\n")
	return err
}

// WriteError sends an error event.
func (s *StreamWriter) WriteError(msg string) error {
	_, err := fmt.Fprintf(s.w, "event: error\ndata: %s\n\n", escapeSSE(msg))
	return err
}

// WriteEvent serializes a StreamEvent as a named SSE event.
func (s *StreamWriter) WriteEvent(e StreamEvent) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", e.Type, data)
	return err
}

func escapeSSE(s string) string {
	// SSE data lines must not contain raw newlines; replace with spaces.
	// For structured JSON use a single-line JSON payload instead.
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, ' ')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}
