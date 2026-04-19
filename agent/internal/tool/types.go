package tool

import (
	"context"
	"encoding/json"
)

// Handler is the function signature for tool implementations.
type Handler func(ctx context.Context, params json.RawMessage) (string, error)

// Tool is a named, documented capability the agent can invoke.
type Tool struct {
	Name        string
	Description string
	// Parameters is a JSON Schema object describing the input.
	Parameters json.RawMessage
	Handler    Handler
}
