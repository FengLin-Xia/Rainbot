package response

// SceneType classifies the nature of a turn for style routing.
type SceneType string

const (
	SceneChat       SceneType = "chat"
	SceneReflection SceneType = "reflection"
	SceneDiscussion SceneType = "discussion"
	SceneTutorial   SceneType = "tutorial"
	SceneCoding     SceneType = "coding"
	SceneEmotional  SceneType = "emotional"
	SceneRisky      SceneType = "risky"
)

// RiskLevel signals how sensitive the content is.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// RewriteMode controls how aggressively the style layer may rewrite.
type RewriteMode string

const (
	RewriteBypass      RewriteMode = "bypass"
	RewriteLightRewrite RewriteMode = "light_rewrite"
	RewriteStrongRewrite RewriteMode = "strong_rewrite"
)

// StructuredResponse is the intermediate object produced by the reasoning
// layer before the style layer processes it.  It separates "what to say"
// from "how to say it".
type StructuredResponse struct {
	// FinalAnswer is the core answer produced by the large model.
	FinalAnswer string `json:"final_answer"`
	// KeyPoints are the bullet-level facts that must survive rewriting.
	KeyPoints []string `json:"key_points,omitempty"`
	// MustKeep contains literal strings that may not be altered (numbers,
	// steps, warnings, refusal boundaries, code blocks, etc.).
	MustKeep []string `json:"must_keep,omitempty"`
	// RiskLevel drives style routing.
	RiskLevel RiskLevel `json:"risk_level"`
	// StyleAllowed is false when the style layer must be bypassed entirely.
	StyleAllowed bool `json:"style_allowed"`
	// RewriteMode constrains how deeply the style layer may rewrite.
	RewriteMode RewriteMode `json:"rewrite_mode"`
	// SceneType informs which style profile to select.
	SceneType SceneType `json:"scene_type"`
	// ToolUsed records whether tool results influenced this answer.
	ToolUsed bool `json:"tool_used"`
}
