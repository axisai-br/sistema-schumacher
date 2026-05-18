package chat

import (
	"strings"
)

type openAIContinuityMetadata struct {
	ProviderResponseID     string
	ProviderConversationID string
	ProviderModel          string
}

func readOpenAIContinuityMetadata(metadata map[string]interface{}) openAIContinuityMetadata {
	if metadata == nil {
		return openAIContinuityMetadata{}
	}

	agent := asMap(metadata["agent"])
	openai := asMap(metadata["openai"])

	return openAIContinuityMetadata{
		ProviderResponseID: firstNonEmpty(
			asString(metadata["provider_response_id"]),
			asString(metadata["openai_provider_response_id"]),
			asString(metadata["latest_provider_response_id"]),
			asString(agent["provider_response_id"]),
			asString(agent["draft_provider_response_id"]),
			asString(openai["provider_response_id"]),
		),
		ProviderConversationID: firstNonEmpty(
			asString(metadata["provider_conversation_id"]),
			asString(metadata["openai_conversation_id"]),
			asString(metadata["conversation_id"]),
			asString(agent["provider_conversation_id"]),
			asString(agent["conversation_id"]),
			asString(openai["conversation_id"]),
		),
		ProviderModel: firstNonEmpty(
			asString(metadata["provider_model"]),
			asString(metadata["openai_model"]),
			asString(metadata["model"]),
			asString(agent["provider_model"]),
			asString(agent["draft_model"]),
			asString(agent["model"]),
			asString(openai["model"]),
		),
	}
}

func openAIModelFamily(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return ""
	}
	for _, suffix := range []string{"-mini", "-nano", "-preview", "-latest"} {
		if strings.HasSuffix(normalized, suffix) {
			return strings.TrimSuffix(normalized, suffix)
		}
	}
	return normalized
}

func openAIModelsCompatible(currentModel string, previousModel string) bool {
	if strings.TrimSpace(previousModel) == "" {
		return true
	}
	currentFamily := openAIModelFamily(currentModel)
	if currentFamily == "" {
		return false
	}
	return currentFamily == openAIModelFamily(previousModel)
}

func shouldRetryOpenAIWithoutContinuity(err error) bool {
	if err == nil {
		return false
	}

	text := strings.ToLower(err.Error())
	if strings.Contains(text, "previous_response_id") {
		return true
	}
	if strings.Contains(text, "conversation") && strings.Contains(text, "cannot be used in conjunction") {
		return true
	}
	if strings.Contains(text, "conversation") && strings.Contains(text, "not found") {
		return true
	}
	if strings.Contains(text, "previous response") && strings.Contains(text, "not found") {
		return true
	}
	return false
}

func extractOpenAIConversationID(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	switch typed := payload["conversation"].(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		return firstNonEmpty(asString(typed["id"]), asString(typed["conversation_id"]))
	}

	return firstNonEmpty(
		asString(payload["conversation_id"]),
		asString(payload["provider_conversation_id"]),
	)
}

func openAIRequestUsesContinuity(requestPayload map[string]interface{}) bool {
	if requestPayload == nil {
		return false
	}
	if _, ok := requestPayload["previous_response_id"]; ok {
		return true
	}
	if _, ok := requestPayload["conversation"]; ok {
		return true
	}
	return false
}

func openAIRequestUsesPreviousResponseID(requestPayload map[string]interface{}) bool {
	if requestPayload == nil {
		return false
	}
	_, ok := requestPayload["previous_response_id"]
	return ok
}

func openAIRetryIdempotencyKey(idempotencyKey string) string {
	trimmed := strings.TrimSpace(idempotencyKey)
	if trimmed == "" {
		return ""
	}
	return trimmed + ":retry_without_continuity"
}
