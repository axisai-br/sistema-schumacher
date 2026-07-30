package chat

import (
	"strings"
	"time"
)

const (
	canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey = "canonical_availability_facts_invalidated_after_message_id"
	canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey = "canonical_availability_facts_invalidated_after_created_at"
)

type canonicalAvailabilityFactsInvalidationBoundary struct {
	AfterMessageID string
	AfterCreatedAt time.Time
}

func (b canonicalAvailabilityFactsInvalidationBoundary) known() bool {
	return strings.TrimSpace(b.AfterMessageID) != "" || !b.AfterCreatedAt.IsZero()
}

func canonicalAvailabilityFactsInvalidationBoundaryForCandidates(
	candidates []Message,
	fallback time.Time,
) canonicalAvailabilityFactsInvalidationBoundary {
	boundary := canonicalAvailabilityFactsInvalidationBoundary{AfterCreatedAt: fallback.UTC()}
	if len(candidates) == 0 {
		return boundary
	}
	message := candidates[len(candidates)-1]
	boundary.AfterMessageID = strings.TrimSpace(message.ID)
	if recordedAt := canonicalAvailabilityHistoryMessageTime(message); !recordedAt.IsZero() {
		boundary.AfterCreatedAt = recordedAt
	}
	return boundary
}

func canonicalAvailabilityFactsInvalidationBoundaryInMetadata(metadata map[string]interface{}) canonicalAvailabilityFactsInvalidationBoundary {
	for _, root := range []map[string]interface{}{
		asMap(metadata["agent"]),
		asMap(metadata["memory"]),
	} {
		if len(root) == 0 {
			continue
		}
		boundary := canonicalAvailabilityFactsInvalidationBoundary{
			AfterMessageID: strings.TrimSpace(asString(root[canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey])),
		}
		if raw := strings.TrimSpace(asString(root[canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey])); raw != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				boundary.AfterCreatedAt = parsed.UTC()
			}
		}
		if boundary.known() {
			return boundary
		}
	}
	return canonicalAvailabilityFactsInvalidationBoundary{}
}

func writeCanonicalAvailabilityFactsInvalidationMetadata(
	root map[string]interface{},
	invalidated bool,
	boundary canonicalAvailabilityFactsInvalidationBoundary,
) {
	if root == nil {
		return
	}
	if !invalidated {
		delete(root, canonicalAvailabilityFactsInvalidatedMetadataKey)
		delete(root, canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey)
		delete(root, canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey)
		return
	}
	root[canonicalAvailabilityFactsInvalidatedMetadataKey] = true
	if messageID := strings.TrimSpace(boundary.AfterMessageID); messageID != "" {
		root[canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey] = messageID
	} else {
		delete(root, canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey)
	}
	if !boundary.AfterCreatedAt.IsZero() {
		root[canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey] = boundary.AfterCreatedAt.UTC().Format(time.RFC3339Nano)
	} else {
		delete(root, canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey)
	}
}

// availabilityInferenceHistory returns a read-only overlay. It never changes
// the persisted transcript: availability artifacts at/before the durable
// boundary are hidden, while later prompts are admitted only after reliable
// delivery and with complete trip/stop facts. A legacy marker without a usable
// boundary conservatively hides every availability artifact already present.
func availabilityInferenceHistory(
	history []Message,
	boundary canonicalAvailabilityFactsInvalidationBoundary,
) []Message {
	boundaryIndex := -1
	if boundaryMessageID := strings.TrimSpace(boundary.AfterMessageID); boundaryMessageID != "" {
		for i := range history {
			if strings.TrimSpace(history[i].ID) == boundaryMessageID {
				boundaryIndex = i
				break
			}
		}
	}

	overlay := make([]Message, len(history))
	for i, message := range history {
		overlay[i] = message
		if !messageHasAvailabilityInferenceArtifacts(message) {
			continue
		}
		atOrBeforeBoundary := canonicalAvailabilityHistoryMessageAtOrBeforeBoundary(
			message,
			i,
			boundaryIndex,
			boundary,
		)
		if atOrBeforeBoundary || !isCompleteReliableAvailabilityPromptMessage(message) {
			overlay[i] = withoutAvailabilityInferenceArtifacts(message)
		}
	}
	return overlay
}

func availabilityInferenceHistoryForSession(session Session, history []Message) []Message {
	history = availabilityPromptBarrierInferenceHistoryV1(history)
	if !canonicalAvailabilityFactsInvalidatedInMetadata(session.Metadata) {
		return history
	}
	return availabilityInferenceHistory(
		history,
		canonicalAvailabilityFactsInvalidationBoundaryInMetadata(session.Metadata),
	)
}

func availabilityPromptBarrierInferenceHistoryV1(history []Message) []Message {
	barrierIndex := latestDeliveredInvalidAvailabilityPromptIndexV1(history, len(history))
	if barrierIndex < 0 {
		return history
	}
	overlay := append([]Message(nil), history...)
	for index := 0; index <= barrierIndex; index++ {
		message := history[index]
		if !messageHasAvailabilityInferenceArtifacts(message) &&
			!messageMayCarryAvailabilityPromptV1(message) {
			continue
		}
		overlay[index] = withoutAvailabilityInferenceArtifacts(message)
	}
	return overlay
}

func canonicalAvailabilityHistoryMessageAtOrBeforeBoundary(
	message Message,
	historyIndex int,
	boundaryIndex int,
	boundary canonicalAvailabilityFactsInvalidationBoundary,
) bool {
	if boundaryIndex >= 0 {
		return historyIndex <= boundaryIndex
	}
	if !boundary.AfterCreatedAt.IsZero() {
		messageTime := canonicalAvailabilityHistoryMessageTime(message)
		return messageTime.IsZero() || !messageTime.After(boundary.AfterCreatedAt)
	}
	return true
}

func canonicalAvailabilityHistoryMessageTime(message Message) time.Time {
	if !message.CreatedAt.IsZero() {
		return message.CreatedAt.UTC()
	}
	if !message.ReceivedAt.IsZero() {
		return message.ReceivedAt.UTC()
	}
	if message.SentAt != nil && !message.SentAt.IsZero() {
		return message.SentAt.UTC()
	}
	return time.Time{}
}

func messageHasAvailabilityInferenceArtifacts(message Message) bool {
	if messageLooksLikeAvailabilitySelectionPrompt(message) {
		return true
	}
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if payloadHasAvailabilityInferenceArtifacts(payload) {
			return true
		}
	}
	return false
}

func payloadHasAvailabilityInferenceArtifacts(payload map[string]interface{}) bool {
	if len(payload) == 0 {
		return false
	}
	if _, exists := payload[availabilityPromptEventV1MessageKey]; exists {
		return true
	}
	if _, exists := payload[availabilitySelectionEventsV1MessageKey]; exists {
		return true
	}
	if asInt(payload["selected_option_index"]) > 0 || len(asMap(payload[selectedAvailabilityResultPayloadKey])) > 0 {
		return true
	}
	if strings.EqualFold(
		payloadMetadataString(payload, "intent"),
		string(IntentAvailabilitySearch),
	) || strings.EqualFold(
		payloadMetadataString(payload, "intent"),
		string(IntentSelectAvailabilityOption),
	) || strings.EqualFold(
		payloadMetadataString(payload, "template_name"),
		string(TemplateAvailabilityList),
	) {
		return true
	}
	if availability := asMap(asMap(payload["tool_context"])[toolNameAvailabilitySearch]); len(availability) > 0 {
		return true
	}
	for _, key := range []string{"request_payload", "response_payload"} {
		if payloadHasAvailabilityInferenceArtifacts(asMap(payload[key])) {
			return true
		}
	}
	return false
}

func messageLooksLikeAvailabilitySelectionPrompt(message Message) bool {
	body := strings.TrimSpace(messageTurnText(message))
	if body == "" {
		return false
	}
	if availabilityOptionCountFromRenderedPrompt(body) > 0 {
		return true
	}
	return looksLikeAvailabilitySelectionPrompt(strings.Join(strings.Fields(foldChatText(body)), " "))
}

func isCompleteReliableAvailabilityPromptMessage(message Message) bool {
	if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") ||
		isBotAutoReplyMessage(message) ||
		!isReliableActivePromptOutbound(message) ||
		!messageLooksLikeAvailabilitySelectionPrompt(message) {
		return false
	}
	observedAt := canonicalAvailabilityHistoryMessageTime(message)
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	availability := visibleAvailabilityContextFromPromptMessageAt(message, observedAt)
	if availability == nil || len(availability.Results) == 0 {
		return false
	}
	for _, item := range availability.Results {
		if !hasCompleteAvailabilitySearchItemFacts(item) {
			return false
		}
	}
	return true
}

func withoutAvailabilityInferenceArtifacts(message Message) Message {
	if messageLooksLikeAvailabilitySelectionPrompt(message) {
		message.Body = ""
	}
	message.Payload = withoutAvailabilityInferencePayloadArtifacts(message.Payload)
	message.NormalizedPayload = withoutAvailabilityInferencePayloadArtifacts(message.NormalizedPayload)
	return message
}

func withoutAvailabilityInferencePayloadArtifacts(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return payload
	}
	filtered := cloneMap(payload)
	delete(filtered, "selected_option_index")
	delete(filtered, selectedAvailabilityResultPayloadKey)
	delete(filtered, availabilitySelectionEventsV1MessageKey)
	delete(filtered, availabilityPromptEventV1MessageKey)
	if strings.EqualFold(
		payloadMetadataString(filtered, "intent"),
		string(IntentAvailabilitySearch),
	) || strings.EqualFold(
		payloadMetadataString(filtered, "intent"),
		string(IntentSelectAvailabilityOption),
	) {
		delete(filtered, "intent")
	}
	if strings.EqualFold(
		payloadMetadataString(filtered, "template_name"),
		string(TemplateAvailabilityList),
	) {
		delete(filtered, "template_name")
	}

	if toolContext := asMap(filtered["tool_context"]); len(toolContext) > 0 {
		filteredToolContext := cloneMap(toolContext)
		delete(filteredToolContext, toolNameAvailabilitySearch)
		if len(filteredToolContext) == 0 {
			delete(filtered, "tool_context")
		} else {
			filtered["tool_context"] = filteredToolContext
		}
	}
	for _, key := range []string{"request_payload", "response_payload"} {
		if nested := asMap(filtered[key]); len(nested) > 0 {
			filtered[key] = withoutAvailabilityInferencePayloadArtifacts(nested)
		}
	}
	return filtered
}
