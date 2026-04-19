package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/obs"
	"github.com/xia-rain/go_agent/internal/prompt"
	"github.com/xia-rain/go_agent/internal/response"
	"github.com/xia-rain/go_agent/internal/style"
	"github.com/xia-rain/go_agent/internal/tool"
)

const (
	defaultMaxTokens   = 4096
	defaultTemperature = 0.7
	maxToolRounds      = 10
)

// EngineConfig holds all dependencies for the Engine.
type EngineConfig struct {
	LLM            llm.ModelClient
	StyleProcessor style.Processor // nil = bypass style layer
	Tools          *tool.Executor
	Registry       *tool.Registry // used to build tool definitions for the LLM
	Prompt         *prompt.Builder
	MaxTokens      int
	Temperature    float32
}

// TurnResult is the complete output of a single agent turn.
type TurnResult struct {
	TurnID       string
	Output       string
	Structured   response.StructuredResponse
	StyleApplied bool
	Metrics      TurnMetrics
}

// Engine orchestrates the full agent turn loop:
//   user input → prompt → LLM → tools (loop) → structure → style → output
type Engine struct {
	cfg EngineConfig
}

func NewEngine(cfg EngineConfig) *Engine {
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = defaultMaxTokens
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = defaultTemperature
	}
	return &Engine{cfg: cfg}
}

// ProcessTurn runs a complete agent turn and streams incremental text chunks
// to the returned channel.  The channel is closed when the turn is done.
// TurnResult is sent as the final value before closing.
func (e *Engine) ProcessTurn(
	ctx context.Context,
	sess *Session,
	userInput string,
	turnID string,
) (<-chan string, <-chan TurnResult, error) {
	textCh := make(chan string, 32)
	resultCh := make(chan TurnResult, 1)

	ctx = obs.WithTurnID(ctx, turnID)
	tracer := obs.NewTracer(turnID, e.cfg.LLM.Name())

	obs.Info(ctx, "turn_start", "session", sess.ID, "input_len", len(userInput))

	go func() {
		defer close(textCh)
		defer close(resultCh)

		result, err := e.runTurn(ctx, sess, userInput, turnID, tracer, textCh)
		if err != nil {
			obs.Error(ctx, "turn_error", "error", err.Error())
			textCh <- fmt.Sprintf("[error: %v]", err)
		}
		result.Metrics = tracer.Finish(ctx)
		resultCh <- result
	}()

	return textCh, resultCh, nil
}

func (e *Engine) runTurn(
	ctx context.Context,
	sess *Session,
	userInput string,
	turnID string,
	tracer *obs.Tracer,
	textCh chan<- string,
) (TurnResult, error) {
	sess.mu.Lock()
	history := sess.history.All()
	sess.mu.Unlock()

	msgs := e.cfg.Prompt.Build(history, userInput)
	toolDefs := []llm.ToolDefinition{}
	if e.cfg.Registry != nil {
		toolDefs = e.cfg.Registry.Definitions()
	}

	// ── Agentic loop: call LLM, execute tools, repeat ──────────────────────
	var rawAnswer string
	var toolUsed bool
	var firstToken bool

	for round := 0; round < maxToolRounds; round++ {
		req := llm.GenerateRequest{
			Messages:    msgs,
			Tools:       toolDefs,
			MaxTokens:   e.cfg.MaxTokens,
			Temperature: e.cfg.Temperature,
		}

		streamStart := time.Now()
		streamCh, err := e.cfg.LLM.Stream(ctx, req)
		if err != nil {
			return TurnResult{}, fmt.Errorf("stream request: %w", err)
		}

		// Accumulate streaming chunks.
		var contentBuf strings.Builder
		toolCallMap := map[int]*pendingToolCall{}

		for chunk := range streamCh {
			if chunk.Err != nil {
				return TurnResult{}, fmt.Errorf("stream chunk: %w", chunk.Err)
			}

			if chunk.Delta != "" {
				if !firstToken {
					tracer.SetFirstTokenLatency(time.Since(streamStart))
					firstToken = true
				}
				contentBuf.WriteString(chunk.Delta)
				// Forward incremental text to the caller.
				select {
				case textCh <- chunk.Delta:
				case <-ctx.Done():
					return TurnResult{}, ctx.Err()
				}
			}

			if d := chunk.ToolCallDelta; d != nil {
				tc := toolCallMap[d.Index]
				if tc == nil {
					tc = &pendingToolCall{}
					toolCallMap[d.Index] = tc
				}
				if d.ID != "" {
					tc.id = d.ID
				}
				if d.Name != "" {
					tc.name = d.Name
				}
				tc.argsBuf.WriteString(d.ArgsDelta)
			}
		}

		assistantMsg := llm.Message{
			Role:    llm.RoleAssistant,
			Content: contentBuf.String(),
		}

		// No tool calls → we have the final answer.
		if len(toolCallMap) == 0 {
			rawAnswer = contentBuf.String()
			msgs = append(msgs, assistantMsg)
			break
		}

		// Execute tool calls concurrently.
		toolUsed = true
		var calls []tool.BatchCall
		for idx := 0; idx < len(toolCallMap); idx++ {
			tc := toolCallMap[idx]
			if tc == nil {
				continue
			}
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, llm.ToolCall{
				ID:        tc.id,
				Name:      tc.name,
				Arguments: json.RawMessage(tc.argsBuf.String()),
			})
			calls = append(calls, tool.BatchCall{
				CallID: tc.id,
				Name:   tc.name,
				Args:   json.RawMessage(tc.argsBuf.String()),
			})
		}
		msgs = append(msgs, assistantMsg)

		toolStart := time.Now()
		batchResults := e.cfg.Tools.ExecuteMany(ctx, calls)
		tracer.AddToolLatency(time.Since(toolStart))

		for _, r := range batchResults {
			tracer.AddToolCall()
			toolOutput := r.Output
			if r.Err != nil {
				toolOutput = fmt.Sprintf("error: %v", r.Err)
			}
			msgs = append(msgs, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: r.ToolCallID,
				Name:       r.ToolName,
				Content:    toolOutput,
			})
		}
		// Continue loop with tool results added to context.
	}

	// ── Structurize + Style (skipped when StyleProcessor is nil) ──────────
	output := rawAnswer
	styleApplied := false
	var structured response.StructuredResponse

	if e.cfg.StyleProcessor != nil {
		sr, err := e.structurize(ctx, rawAnswer, toolUsed)
		if err != nil {
			obs.Warn(ctx, "structurize_failed", "error", err.Error())
			sr = response.StructuredResponse{
				FinalAnswer:  rawAnswer,
				RiskLevel:    response.RiskLow,
				StyleAllowed: true,
				RewriteMode:  response.RewriteLightRewrite,
				SceneType:    response.SceneChat,
				ToolUsed:     toolUsed,
			}
		}
		structured = sr
		output = sr.FinalAnswer

		if sr.StyleAllowed && sr.RewriteMode != response.RewriteBypass {
			profile := style.ResolveProfile(sr.SceneType, sr.RiskLevel)
			tracer.SetRewrite(true, string(sr.RewriteMode))

			styleStart := time.Now()
			styleResp, err := e.cfg.StyleProcessor.Rewrite(ctx, style.StyleRewriteRequest{
				FinalAnswer:  sr.FinalAnswer,
				KeyPoints:    sr.KeyPoints,
				MustKeep:     sr.MustKeep,
				RiskLevel:    string(sr.RiskLevel),
				StyleProfile: profile,
				RewriteMode:  string(sr.RewriteMode),
			})
			tracer.SetStyleLatency(time.Since(styleStart))

			if err != nil {
				obs.Warn(ctx, "style_error", "error", err.Error())
				tracer.SetFallback("style_error: " + err.Error())
			} else if styleResp.Applied {
				output = styleResp.OutputText
				styleApplied = true
			} else {
				tracer.SetFallback(styleResp.FallbackReason)
			}
		}
	}

	// Persist turn to session history.
	sess.mu.Lock()
	sess.history.Append(llm.Message{Role: llm.RoleUser, Content: userInput})
	sess.history.Append(llm.Message{Role: llm.RoleAssistant, Content: rawAnswer})
	sess.mu.Unlock()

	if output == "" {
		output = rawAnswer
		obs.Warn(ctx, "output_empty_fallback")
	}

	obs.Info(ctx, "turn_done",
		"style_applied", styleApplied,
		"output_len", len(output),
	)

	return TurnResult{
		TurnID:       turnID,
		Output:       output,
		Structured:   structured,
		StyleApplied: styleApplied,
	}, nil
}

// structurize asks the LLM to convert the raw answer into a StructuredResponse.
func (e *Engine) structurize(ctx context.Context, rawAnswer string, toolUsed bool) (response.StructuredResponse, error) {
	if rawAnswer == "" {
		return response.StructuredResponse{}, fmt.Errorf("empty answer")
	}

	req := llm.GenerateRequest{
		Messages:  e.cfg.Prompt.BuildStructureRequest(rawAnswer),
		MaxTokens: 1024,
		JSONMode:  true,
	}

	resp, err := e.cfg.LLM.Generate(ctx, req)
	if err != nil {
		return response.StructuredResponse{}, fmt.Errorf("structurize llm call: %w", err)
	}

	var sr response.StructuredResponse
	if err := json.Unmarshal([]byte(resp.Message.Content), &sr); err != nil {
		return response.StructuredResponse{}, fmt.Errorf("structurize parse: %w", err)
	}
	if sr.FinalAnswer == "" {
		sr.FinalAnswer = rawAnswer
	}
	sr.ToolUsed = sr.ToolUsed || toolUsed
	return sr, nil
}

// pendingToolCall accumulates streaming tool call fragments.
type pendingToolCall struct {
	id      string
	name    string
	argsBuf strings.Builder
}
