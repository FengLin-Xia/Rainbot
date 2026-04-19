package style

import (
	"context"

	"github.com/xia-rain/go_agent/internal/llm"
)

// LocalModelRewriter uses a locally-hosted model (via Ollama or llama.cpp)
// as the style backend.  It delegates to PromptRewriter since the local
// model exposes an OpenAI-compatible API.
type LocalModelRewriter struct {
	inner *PromptRewriter
}

// NewLocalModelRewriter creates a style processor backed by a local model.
// baseURL is the OpenAI-compat base URL, e.g. "http://localhost:11434".
func NewLocalModelRewriter(baseURL, model string) *LocalModelRewriter {
	client := llm.NewOllamaClient(baseURL, model)
	return &LocalModelRewriter{inner: NewPromptRewriter(client)}
}

func (l *LocalModelRewriter) Rewrite(ctx context.Context, req StyleRewriteRequest) (StyleRewriteResponse, error) {
	resp, err := l.inner.Rewrite(ctx, req)
	if err == nil && resp.Diagnostics != nil {
		resp.Diagnostics["backend"] = "local_model"
	}
	return resp, err
}
