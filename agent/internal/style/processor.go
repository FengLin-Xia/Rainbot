package style

import "context"

// StyleProfile defines the target personality and expression constraints.
type StyleProfile string

const (
	StylePlain    StyleProfile = "plain"     // neutral, factual
	StyleBlunt    StyleProfile = "blunt"     // direct, no fluff, no empty sympathy
	StyleSharp    StyleProfile = "sharp"     // opinionated, pointed, clearly judging
	StyleMeanLite StyleProfile = "mean-lite" // allows light sarcasm / dry humor, no cruelty
)

// StyleRewriteRequest is the full input to the style layer.
type StyleRewriteRequest struct {
	TurnID       string
	FinalAnswer  string
	KeyPoints    []string
	MustKeep     []string
	RiskLevel    string
	StyleProfile StyleProfile
	RewriteMode  string
	Constraints  []string
	Metadata     map[string]string
}

// StyleRewriteResponse is what the style layer returns.
type StyleRewriteResponse struct {
	OutputText       string
	Applied          bool
	AppliedProfile   StyleProfile
	ValidationPassed bool
	FallbackReason   string
	Diagnostics      map[string]string
}

// Processor is the interface every style backend must implement.
type Processor interface {
	Rewrite(ctx context.Context, req StyleRewriteRequest) (StyleRewriteResponse, error)
}
