package style

import "github.com/xia-rain/go_agent/internal/response"

// ResolveProfile selects the appropriate StyleProfile for a turn based on
// scene type and risk level.  High-risk content always falls back to plain.
func ResolveProfile(scene response.SceneType, risk response.RiskLevel) StyleProfile {
	if risk == response.RiskHigh {
		return StylePlain
	}
	switch scene {
	case response.SceneChat, response.SceneReflection, response.SceneDiscussion:
		return StyleSharp
	case response.SceneTutorial, response.SceneCoding:
		return StyleBlunt
	case response.SceneEmotional, response.SceneRisky:
		return StylePlain
	default:
		return StyleBlunt
	}
}
