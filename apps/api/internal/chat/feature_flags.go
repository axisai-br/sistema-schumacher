package chat

import (
	"os"
	"strings"
)

const (
	chatAgentModeLegacy     = "legacy"
	chatAgentModeHybridJSON = "hybrid_json"
	chatAgentModeJSONOnly   = "json_only"
)

func chatFeatureEnabled(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func canonicalStateEnabled() bool {
	return chatFeatureEnabled("CHAT_CANONICAL_STATE_ENABLED", true)
}

func intentRouterEnabled() bool {
	return chatFeatureEnabled("CHAT_INTENT_ROUTER_ENABLED", true)
}

func templateRealizerEnabled() bool {
	return chatFeatureEnabled("CHAT_TEMPLATE_REALIZER_ENABLED", true)
}

func llmIntentFallbackEnabled() bool {
	return chatFeatureEnabled("CHAT_LLM_INTENT_FALLBACK_ENABLED", false)
}

func legacyPromptFallbackEnabled() bool {
	return chatFeatureEnabled("CHAT_LEGACY_PROMPT_FALLBACK_ENABLED", true)
}

func specialistAgentsEnabled() bool {
	return chatFeatureEnabled("CHAT_SPECIALIST_AGENTS_ENABLED", false)
}

func normalizeChatAgentMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case chatAgentModeHybridJSON:
		return chatAgentModeHybridJSON
	case chatAgentModeJSONOnly:
		return chatAgentModeJSONOnly
	default:
		return chatAgentModeLegacy
	}
}

func jsonDecisionLayerEnabledForMode(mode string) bool {
	switch normalizeChatAgentMode(mode) {
	case chatAgentModeHybridJSON, chatAgentModeJSONOnly:
		return true
	default:
		return false
	}
}

func freeFormLLMFallbackEnabledForMode(mode string) bool {
	return normalizeChatAgentMode(mode) != chatAgentModeJSONOnly && legacyPromptFallbackEnabled()
}
