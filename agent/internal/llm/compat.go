package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// CompatClient is a raw-HTTP OpenAI-compatible client that supports
// vendor extensions such as DeepSeek's reasoning_content field.
type CompatClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	name       string
}

// NewCompatClient creates a raw-HTTP OpenAI-compat client.
func NewCompatClient(baseURL, apiKey, model string) *CompatClient {
	return &CompatClient{
		httpClient: &http.Client{},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		name:       "openai-compat/" + model,
	}
}

func (c *CompatClient) Name() string { return c.name }

func (c *CompatClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return withRetry(ctx, func() (*GenerateResponse, error) {
		return c.generate(ctx, req)
	})
}

func (c *CompatClient) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	return withRetry(ctx, func() (<-chan StreamChunk, error) {
		return c.stream(ctx, req)
	})
}

// ── request / response wire types ────────────────────────────────────────────

type cMsg struct {
	Role             string  `json:"role"`
	Content          string  `json:"content"` // must always be present per DeepSeek spec
	ReasoningContent string  `json:"reasoning_content,omitempty"`
	ToolCalls        []cTC   `json:"tool_calls,omitempty"`
	ToolCallID       string  `json:"tool_call_id,omitempty"`
	Name             string  `json:"name,omitempty"`
}

type cTC struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type cTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type cReq struct {
	Model          string       `json:"model"`
	Messages       []cMsg       `json:"messages"`
	Tools          []cTool      `json:"tools,omitempty"`
	MaxTokens      int          `json:"max_tokens,omitempty"`
	Temperature    float32      `json:"temperature"`
	Stream         bool         `json:"stream"`
	StreamOptions  *cStreamOpts `json:"stream_options,omitempty"`
	ResponseFormat *cRespFmt    `json:"response_format,omitempty"`
}

type cStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type cRespFmt struct {
	Type string `json:"type"`
}

type cResp struct {
	Choices []struct {
		Message struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []cTC  `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *cAPIError `json:"error,omitempty"`
}

type cChunk struct {
	Choices []struct {
		Delta struct {
			Content          string     `json:"content"`
			ReasoningContent string     `json:"reasoning_content"`
			ToolCalls        []cTCDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *cAPIError `json:"error,omitempty"`
}

type cTCDelta struct {
	Index    *int   `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type cAPIError struct {
	Message string `json:"message"`
	Code    any    `json:"code"`
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (c *CompatClient) buildMsgs(msgs []Message) []cMsg {
	out := make([]cMsg, 0, len(msgs))
	for _, m := range msgs {
		cm := cMsg{
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
			ToolCallID:       m.ToolCallID,
			Name:             m.Name,
		}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, cTC{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: tc.Name, Arguments: string(tc.Arguments)},
			})
		}
		out = append(out, cm)
	}
	return out
}

func (c *CompatClient) buildTools(defs []ToolDefinition) []cTool {
	out := make([]cTool, 0, len(defs))
	for _, d := range defs {
		var t cTool
		t.Type = "function"
		t.Function.Name = d.Name
		t.Function.Description = d.Description
		t.Function.Parameters = d.Parameters
		out = append(out, t)
	}
	return out
}

func (c *CompatClient) do(ctx context.Context, body cReq) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("compat marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("compat new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	return c.httpClient.Do(req)
}

// ── generate (non-streaming) ──────────────────────────────────────────────────

func (c *CompatClient) generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	body := cReq{
		Model:       c.model,
		Messages:    c.buildMsgs(req.Messages),
		Tools:       c.buildTools(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      false,
	}
	if req.JSONMode {
		body.ResponseFormat = &cRespFmt{Type: "json_object"}
	}

	resp, err := c.do(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("compat generate: %w", err)
	}
	defer resp.Body.Close()

	var cr cResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("compat generate decode: %w", err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("compat generate api error: %s", cr.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("compat generate status %d", resp.StatusCode)
	}
	if len(cr.Choices) == 0 {
		return nil, fmt.Errorf("compat generate: no choices")
	}

	ch := cr.Choices[0]
	msg := Message{
		Role:             RoleAssistant,
		Content:          ch.Message.Content,
		ReasoningContent: ch.Message.ReasoningContent,
	}
	for _, tc := range ch.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	return &GenerateResponse{
		Message:      msg,
		FinishReason: ch.FinishReason,
		Usage: Usage{
			InputTokens:  cr.Usage.PromptTokens,
			OutputTokens: cr.Usage.CompletionTokens,
		},
	}, nil
}

// ── stream ────────────────────────────────────────────────────────────────────

func (c *CompatClient) stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	body := cReq{
		Model:         c.model,
		Messages:      c.buildMsgs(req.Messages),
		Tools:         c.buildTools(req.Tools),
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		Stream:        true,
		StreamOptions: &cStreamOpts{IncludeUsage: true},
	}

	resp, err := c.do(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("compat stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var cr cResp
		_ = json.NewDecoder(resp.Body).Decode(&cr)
		resp.Body.Close()
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		if cr.Error != nil {
			msg = cr.Error.Message
		}
		return nil, fmt.Errorf("compat stream: error, status code: %d, message: %s", resp.StatusCode, msg)
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(line[5:])
			if payload == "[DONE]" {
				return
			}

			var chunk cChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				ch <- StreamChunk{Err: fmt.Errorf("compat stream parse: %w", err)}
				return
			}
			if chunk.Error != nil {
				ch <- StreamChunk{Err: fmt.Errorf("compat stream api error: %s", chunk.Error.Message)}
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta
			sc := StreamChunk{
				Delta:          delta.Content,
				ReasoningDelta: delta.ReasoningContent,
				FinishReason:   chunk.Choices[0].FinishReason,
			}
			for _, tc := range delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				sc.ToolCallDelta = &ToolCallDelta{
					Index:     idx,
					ID:        tc.ID,
					Name:      tc.Function.Name,
					ArgsDelta: tc.Function.Arguments,
				}
			}
			ch <- sc
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Err: fmt.Errorf("compat stream read: %w", err)}
		}
	}()

	return ch, nil
}
