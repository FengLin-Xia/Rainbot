package llm

import (
	"context"
	"fmt"
)

// AnthropicClient implements ModelClient for the Anthropic API.
// Uses an OpenAI-compatible proxy layer or direct implementation.
// For v0.1, we delegate to an OpenAI-compat wrapper around the Anthropic API.
type AnthropicClient struct {
	inner *OpenAIClient
}

// NewAnthropicClient creates a client for the Anthropic API via its
// OpenAI-compatible endpoint (https://api.anthropic.com/v1).
func NewAnthropicClient(apiKey, model string) (*AnthropicClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: api key is required")
	}
	inner := NewOpenAICompatClient("https://api.anthropic.com/v1", apiKey, model)
	inner.name = "anthropic/" + model
	return &AnthropicClient{inner: inner}, nil
}

func (c *AnthropicClient) Name() string { return c.inner.Name() }

func (c *AnthropicClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return c.inner.Generate(ctx, req)
}

func (c *AnthropicClient) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	return c.inner.Stream(ctx, req)
}
