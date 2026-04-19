package style

import (
	"context"
	"fmt"
	"strings"

	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/obs"
)

const rewriteSystemPrompt = `You are a style rewriter. Your ONLY job is to rewrite the text in the requested style.

HARD RULES — never violate these:
- Do NOT add new facts, claims, or knowledge.
- Do NOT remove critical conditions, warnings, or safety notices.
- Do NOT change numbers, dates, times, locations, or version strings.
- Do NOT modify step order or code blocks.
- Do NOT weaken refusals or risk warnings.
- Do NOT introduce empty empathy or hollow reassurance.

STYLE RULES:
- plain: factual, neutral, no embellishment
- blunt: direct, no filler, no "I hope this helps", no "great question"
- sharp: opinionated, judgmental where appropriate, calls things out plainly
- mean-lite: dry humor or mild sarcasm is fine; cruelty and insults are not

Output ONLY the rewritten text. No explanation, no preamble.`

// PromptRewriter implements Processor by making a second LLM call.
// This is the default style backend for v0.1 before a local model is available.
type PromptRewriter struct {
	client llm.ModelClient
}

func NewPromptRewriter(client llm.ModelClient) *PromptRewriter {
	return &PromptRewriter{client: client}
}

func (p *PromptRewriter) Rewrite(ctx context.Context, req StyleRewriteRequest) (StyleRewriteResponse, error) {
	userPrompt := buildRewritePrompt(req)

	genReq := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: rewriteSystemPrompt},
			{Role: llm.RoleUser, Content: userPrompt},
		},
		MaxTokens:   2048,
		Temperature: 0.3,
	}

	resp, err := p.client.Generate(ctx, genReq)
	if err != nil {
		obs.Warn(ctx, "style_rewrite_error", "error", err.Error())
		return StyleRewriteResponse{
			OutputText:     req.FinalAnswer,
			Applied:        false,
			FallbackReason: fmt.Sprintf("llm error: %v", err),
		}, nil
	}

	output := strings.TrimSpace(resp.Message.Content)
	if output == "" {
		return StyleRewriteResponse{
			OutputText:     req.FinalAnswer,
			Applied:        false,
			FallbackReason: "empty rewrite output",
		}, nil
	}

	if len(req.MustKeep) > 0 {
		var missing []string
		for _, item := range req.MustKeep {
			if !strings.Contains(output, item) {
				missing = append(missing, item)
			}
		}
		if len(missing) > 0 {
			obs.Warn(ctx, "mustkeep_violation",
				"missing_count", len(missing),
				"items", strings.Join(missing, "|"),
			)
			return StyleRewriteResponse{
				OutputText:       req.FinalAnswer,
				Applied:          false,
				FallbackReason:   "mustkeep_violation: " + strings.Join(missing, ", "),
				ValidationPassed: false,
			}, nil
		}
	}

	return StyleRewriteResponse{
		OutputText:       output,
		Applied:          true,
		AppliedProfile:   req.StyleProfile,
		ValidationPassed: true,
		Diagnostics:      map[string]string{"backend": "prompt_rewriter"},
	}, nil
}

func buildRewritePrompt(req StyleRewriteRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Style: %s\n", req.StyleProfile))
	if req.RewriteMode != "" {
		sb.WriteString(fmt.Sprintf("Rewrite mode: %s\n", req.RewriteMode))
	}
	if len(req.MustKeep) > 0 {
		sb.WriteString("\nThe following items must be preserved verbatim:\n")
		for _, mk := range req.MustKeep {
			sb.WriteString("  - " + mk + "\n")
		}
	}
	if len(req.Constraints) > 0 {
		sb.WriteString("\nAdditional constraints:\n")
		for _, c := range req.Constraints {
			sb.WriteString("  - " + c + "\n")
		}
	}
	sb.WriteString("\nText to rewrite:\n")
	sb.WriteString(req.FinalAnswer)
	return sb.String()
}
