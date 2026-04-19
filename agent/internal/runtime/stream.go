package runtime

import (
	"fmt"
	"io"
)

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
