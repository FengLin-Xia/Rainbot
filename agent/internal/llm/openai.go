package llm

import (
	"context"
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIClient implements ModelClient for any OpenAI-compatible API.
type OpenAIClient struct {
	client *openai.Client
	model  string
	name   string
}

// NewOpenAIClient creates a client for the official OpenAI API.
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		client: openai.NewClient(apiKey),
		model:  model,
		name:   "openai/" + model,
	}
}

// NewOpenAICompatClient creates a client for any OpenAI-compatible endpoint.
func NewOpenAICompatClient(baseURL, apiKey, model string) *OpenAIClient {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	return &OpenAIClient{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
		name:   "openai-compat/" + model,
	}
}

func (c *OpenAIClient) Name() string { return c.name }

func (c *OpenAIClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	oaiReq := c.buildRequest(req)
	oaiReq.Stream = false

	resp, err := c.client.CreateChatCompletion(ctx, oaiReq)
	if err != nil {
		return nil, fmt.Errorf("openai generate: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai generate: no choices returned")
	}

	choice := resp.Choices[0]
	msg := Message{
		Role:    RoleAssistant,
		Content: choice.Message.Content,
	}
	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}

	return &GenerateResponse{
		Message:      msg,
		FinishReason: string(choice.FinishReason),
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func (c *OpenAIClient) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	oaiReq := c.buildRequest(req)
	oaiReq.Stream = true
	oaiReq.StreamOptions = &openai.StreamOptions{IncludeUsage: true}

	stream, err := c.client.CreateChatCompletionStream(ctx, oaiReq)
	if err != nil {
		return nil, fmt.Errorf("openai stream: %w", err)
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer stream.Close()

		for {
			resp, err := stream.Recv()
			if err != nil {
				if err.Error() != "EOF" {
					ch <- StreamChunk{Err: fmt.Errorf("openai stream recv: %w", err)}
				}
				return
			}
			if len(resp.Choices) == 0 {
				continue
			}

			choice := resp.Choices[0]
			chunk := StreamChunk{
				Delta:        choice.Delta.Content,
				FinishReason: string(choice.FinishReason),
			}

			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				chunk.ToolCallDelta = &ToolCallDelta{
					Index:     idx,
					ID:        tc.ID,
					Name:      tc.Function.Name,
					ArgsDelta: tc.Function.Arguments,
				}
			}

			ch <- chunk
		}
	}()

	return ch, nil
}

func (c *OpenAIClient) buildRequest(req GenerateRequest) openai.ChatCompletionRequest {
	msgs := make([]openai.ChatCompletionMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		oaiMsg := openai.ChatCompletionMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Name,
					Arguments: string(tc.Arguments),
				},
			})
		}
		msgs = append(msgs, oaiMsg)
	}

	oaiReq := openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	for _, td := range req.Tools {
		oaiReq.Tools = append(oaiReq.Tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Parameters, // json.RawMessage satisfies `any`
			},
		})
	}

	if req.JSONMode {
		oaiReq.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	return oaiReq
}
