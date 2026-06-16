package chat

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	messageStatusAutomationPending                 = "AUTOMATION_PENDING"
	agentStatusReadyForAutomation                  = "READY_FOR_AUTOMATION"
	messageStatusAutomationDraft                   = "AUTOMATION_DRAFT"
	messageStatusAutomationReviewed                = "AUTOMATION_REVIEWED"
	messageStatusAutomationSent                    = "AUTOMATION_SENT"
	agentStatusDraftGenerated                      = "DRAFT_GENERATED"
	agentStatusDraftReviewed                       = "DRAFT_REVIEWED"
	agentStatusDraftAutoSent                       = "DRAFT_AUTO_SENT"
	draftAutoSendStatusEligible                    = "AUTO_SEND_ELIGIBLE"
	draftAutoSendStatusRetryPending                = "AUTO_SEND_RETRY_PENDING"
	draftAutoSendStatusBlockedHuman                = "AUTO_SEND_BLOCKED_HUMAN"
	draftAutoSendStatusReviewNeeded                = "REVIEW_REQUIRED"
	draftAutoSendReasonToolCall                    = "tool_call_present"
	draftAutoSendReasonNonTextTurn                 = "non_text_turn"
	draftAutoSendReasonDeliveryFail                = "delivery_failed"
	draftAutoSendReasonHumanHandoff                = "human_handoff_active"
	draftAutoSendReasonOperationalClaimWithoutTool = "operational_claim_without_tool"
)

const (
	draftAutoSendReasonBookingFlowRegression   = "booking_flow_regression"
	draftAutoSendReasonOutOfScopeDuringBooking = "out_of_scope_during_booking_flow"
	draftAutoSendReasonOperationalNoTool       = "operational_claim_without_tool"
)

var (
	autoSendDepartureTimePattern = regexp.MustCompile(`(?i)\b(?:sa[ií]da\s+(?:as|às)|saida\s+as)\s+\d{1,2}[:h]\d{2}\b`)
	autoSendRouteDatePattern     = regexp.MustCompile(`\b\d{1,2}/\d{1,2}(?:/\d{2,4})?\b`)
	autoSendPricePattern         = regexp.MustCompile(`(?i)\br\$\s*\d+`)
)

type draftAutoSendPolicy struct {
	Status  string
	Reasons []string
}

func selectReprocessCandidateMessages(history []Message) []Message {
	candidates := make([]Message, 0, 4)
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if message.Direction == "OUTBOUND" {
			if strings.EqualFold(strings.TrimSpace(message.ProcessingStatus), messageStatusAutomationDraft) {
				continue
			}
			if len(candidates) > 0 {
				break
			}
			return nil
		}
		if message.Direction != "INBOUND" {
			continue
		}
		if isAudioMessageKind(message.Kind) && hasFailedAudioTranscription(message) {
			continue
		}
		if isAudioMessageKind(message.Kind) && !hasCompletedAudioTranscription(message) {
			if len(candidates) == 0 {
				candidates = append([]Message{message}, candidates...)
			}
			break
		}
		if !isReprocessableInboundStatus(message.ProcessingStatus) {
			if len(candidates) > 0 {
				break
			}
			continue
		}
		candidates = append([]Message{message}, candidates...)
	}
	return candidates
}

func isReprocessableInboundStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "", "RECEIVED", "BUFFERED_PENDING", messageStatusAutomationPending, agentStatusReadyForAutomation:
		return true
	default:
		return false
	}
}

func isAutomationDraftStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case messageStatusAutomationDraft, messageStatusAutomationReviewed, messageStatusAutomationSent:
		return true
	default:
		return false
	}
}

func buildReprocessMemory(session Session, history []Message, candidates []Message, observedAt time.Time) map[string]interface{} {
	recent := history
	if len(recent) > 8 {
		recent = recent[len(recent)-8:]
	}

	items := make([]map[string]interface{}, 0, len(recent))
	lastCustomerMessage := ""
	lastAssistantMessage := ""
	for _, message := range recent {
		body := messageTurnText(message)
		items = append(items, map[string]interface{}{
			"id":                message.ID,
			"direction":         message.Direction,
			"kind":              strings.TrimSpace(message.Kind),
			"body":              truncateBufferBody(body),
			"processing_status": message.ProcessingStatus,
			"created_at":        message.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
		if message.Direction == "INBOUND" && body != "" {
			lastCustomerMessage = body
		}
		if message.Direction == "OUTBOUND" && body != "" {
			lastAssistantMessage = body
		}
	}

	return map[string]interface{}{
		"generated_at":             observedAt.UTC().Format(time.RFC3339Nano),
		"customer_phone":           session.CustomerPhone,
		"customer_name":            session.CustomerName,
		"current_turn_body":        joinCandidateBodies(candidates),
		"current_turn_kinds":       candidateKinds(candidates),
		"current_turn_media":       candidateMediaMemory(candidates),
		"current_turn_message_ids": candidateMessageIDs(candidates),
		"last_customer_message":    lastCustomerMessage,
		"last_assistant_message":   lastAssistantMessage,
		"recent_messages":          items,
	}
}

func buildReprocessAgentState(session Session, candidates []Message, trigger string, observedAt time.Time, metadata map[string]interface{}) map[string]interface{} {
	state := map[string]interface{}{
		"status":                   agentStatusReadyForAutomation,
		"trigger":                  trigger,
		"requested_at":             observedAt.UTC().Format(time.RFC3339Nano),
		"handoff_status":           session.HandoffStatus,
		"current_turn_body":        joinCandidateBodies(candidates),
		"current_turn_message_ids": candidateMessageIDs(candidates),
	}
	for key, value := range metadata {
		state[key] = value
	}
	return state
}

func buildReprocessBufferState(metadata map[string]interface{}, candidates []Message, trigger string, observedAt time.Time) map[string]interface{} {
	buffer := cloneBuffer(metadata)
	buffer["status"] = bufferStatusIdle
	buffer["message_count"] = 0
	buffer["window_started_at"] = nil
	buffer["pending_until"] = nil
	buffer["agent_status"] = agentStatusReadyForAutomation
	buffer["reprocess_trigger"] = trigger
	buffer["reprocess_requested_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	buffer["current_turn_message_ids"] = candidateMessageIDs(candidates)
	buffer["current_turn_body"] = truncateBufferBody(joinCandidateBodies(candidates))
	if len(candidates) == 0 {
		return buffer
	}

	last := candidates[len(candidates)-1]
	buffer["last_message_id"] = last.ID
	buffer["last_message_at"] = last.ReceivedAt.UTC().Format(time.RFC3339Nano)
	buffer["last_message_body"] = truncateBufferBody(last.Body)
	buffer["last_direction"] = last.Direction
	buffer["processing_status"] = messageStatusAutomationPending
	buffer["provider_message_id"] = last.ProviderMessageID

	return buffer
}

func candidateMessageIDs(messages []Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func candidateKinds(messages []Message) []string {
	kinds := make([]string, 0, len(messages))
	for _, message := range messages {
		kinds = append(kinds, normalizedCandidateKind(message))
	}
	return kinds
}

func normalizedCandidateKind(message Message) string {
	kind := strings.ToUpper(strings.TrimSpace(message.Kind))
	if kind == "" {
		kind = "TEXT"
	}
	if isAudioMessageKind(kind) && hasCompletedAudioTranscription(message) {
		return "TEXT"
	}
	return kind
}

func isAudioMessageKind(kind string) bool {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "AUDIO", "VOICE", "PTT":
		return true
	default:
		return false
	}
}

func hasCompletedAudioTranscription(message Message) bool {
	status := strings.ToUpper(strings.TrimSpace(asString(message.NormalizedPayload["transcription_status"])))
	if status == "FAILED" {
		return false
	}
	return strings.TrimSpace(asString(message.NormalizedPayload["transcription_text"])) != ""
}

func currentTurnHasCompletedAudioTranscript(candidates []Message) bool {
	if len(candidates) == 0 {
		return false
	}
	return hasCompletedAudioTranscription(candidates[len(candidates)-1])
}

func hasFailedAudioTranscription(message Message) bool {
	return strings.EqualFold(strings.TrimSpace(asString(message.NormalizedPayload["transcription_status"])), "FAILED")
}

func candidateMediaMemory(messages []Message) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(messages))
	for _, item := range collectCandidateMedia(messages) {
		items = append(items, map[string]interface{}{
			"kind":        item.Kind,
			"url_present": strings.TrimSpace(item.URL) != "",
			"mime_type":   item.MimeType,
			"message_id":  item.MessageID,
			"file_name":   item.FileName,
		})
	}
	return items
}

func joinCandidateBodies(messages []Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		body := messageTurnText(message)
		if body == "" {
			continue
		}
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

func collectCandidateMedia(messages []Message) []AgentMediaInput {
	items := make([]AgentMediaInput, 0, len(messages))
	for _, message := range messages {
		kind := strings.ToUpper(strings.TrimSpace(message.Kind))
		normalized := message.NormalizedPayload
		switch kind {
		case "IMAGE":
			mimeType := strings.TrimSpace(asString(normalized["image_mime_type"]))
			dataURL := strings.TrimSpace(asString(normalized["image_data_url"]))
			imageURL := strings.TrimSpace(asString(normalized["image_url"]))

			if strings.HasPrefix(strings.ToLower(dataURL), "data:image/") {
				items = append(items, AgentMediaInput{
					Kind:      "IMAGE",
					URL:       dataURL,
					MimeType:  mimeType,
					MessageID: message.ID,
				})
				continue
			}

			if isHTTPURL(imageURL) {
				items = append(items, AgentMediaInput{
					Kind:      "IMAGE",
					URL:       imageURL,
					MimeType:  mimeType,
					MessageID: message.ID,
				})
			}

		case "DOCUMENT":
			mimeType := strings.TrimSpace(asString(normalized["document_mime_type"]))
			fileName := strings.TrimSpace(asString(normalized["document_file_name"]))

			if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
				if url := strings.TrimSpace(asString(normalized["document_url"])); isHTTPURL(url) {
					items = append(items, AgentMediaInput{
						Kind:      "IMAGE",
						URL:       url,
						MimeType:  mimeType,
						MessageID: message.ID,
						FileName:  fileName,
					})
				}
				continue
			}

			if strings.EqualFold(mimeType, "application/pdf") {
				dataURL := strings.TrimSpace(asString(normalized["document_data_url"]))
				documentURL := strings.TrimSpace(asString(normalized["document_url"]))

				if strings.HasPrefix(strings.ToLower(dataURL), "data:application/pdf") {
					items = append(items, AgentMediaInput{
						Kind:      "PDF",
						URL:       dataURL,
						MimeType:  mimeType,
						MessageID: message.ID,
						FileName:  fileName,
					})
					continue
				}

				if isHTTPURL(documentURL) {
					items = append(items, AgentMediaInput{
						Kind:      "PDF",
						URL:       documentURL,
						MimeType:  mimeType,
						MessageID: message.ID,
						FileName:  fileName,
					})
				}
			}
		}
	}
	return items
}

func isHTTPURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}

func buildAgentDraftIdempotencyKey(sessionID string, messageIDs []string) string {
	return "chat-agent-draft-" + uuid.NewSHA1(uuid.Nil, []byte(sessionID+"|"+strings.Join(messageIDs, ","))).String()
}

func buildAutoSendReplyIdempotencyKey(draftID string) string {
	return "chat-auto-reply-" + strings.TrimSpace(draftID)
}

func evaluateDraftAutoSendPolicy(candidates []Message, toolCalls []ToolCall, replyText string) draftAutoSendPolicy {
	reasons := make([]string, 0, 2)
	if len(toolCalls) > 0 && !hasOnlyAutoSendSafeToolCalls(toolCalls) {
		reasons = append(reasons, draftAutoSendReasonToolCall)
	}
	allowHandledDocumentMedia := hasCompletedDocumentExtractToolCall(toolCalls) ||
		strings.TrimSpace(replyText) == unsupportedPDFDocumentReply
	if hasBlockingNonTextCandidate(candidates, allowHandledDocumentMedia) {
		reasons = append(reasons, draftAutoSendReasonNonTextTurn)
	}
	if len(toolCalls) == 0 && containsOperationalAutoSendClaimWithoutTool(replyText) {
		reasons = append(reasons, draftAutoSendReasonOperationalNoTool)
	}
	if containsOutOfDomainSchedulingVocabulary(replyText) {
		reasons = append(reasons, draftAutoSendReasonBookingFlowRegression)
	}

	policy := draftAutoSendPolicy{Status: draftAutoSendStatusEligible}
	if len(reasons) > 0 {
		policy.Status = draftAutoSendStatusReviewNeeded
		policy.Reasons = reasons
	}
	return policy
}

func containsOperationalAutoSendClaimWithoutTool(text string) bool {
	folded := foldChatText(text)
	if folded == "" {
		return false
	}

	if looksLikePublicSCTableReply(folded) {
		return false
	}

	blockedPhrases := []string{
		"qual horario prefere",
		"tem vaga",
		"tem disponibilidade",
		"disponivel",
		"disponibilidade",
	}
	for _, phrase := range blockedPhrases {
		if strings.Contains(folded, " "+phrase+" ") {
			return true
		}
	}

	hasRoute := (strings.Contains(text, "→") || strings.Contains(text, "->") || strings.Contains(folded, " para ")) &&
		mentionsSupportedCityForAutoSend(folded, scPackageDestinations) &&
		mentionsSupportedCityForAutoSend(folded, maPackageDestinations)
	hasDate := autoSendRouteDatePattern.MatchString(text)
	hasTimePreference := strings.Contains(folded, " manha ") || strings.Contains(folded, " tarde ") || strings.Contains(folded, " noite ")
	if hasTimePreference && (hasRoute || hasDate || strings.Contains(folded, " horario ")) {
		return true
	}
	if autoSendDepartureTimePattern.MatchString(text) {
		return true
	}
	if hasRoute && hasDate {
		return true
	}
	if autoSendPricePattern.MatchString(text) && !looksLikePublicSCTableReply(folded) {
		return true
	}
	return false
}

func mentionsSupportedCityForAutoSend(folded string, candidates map[string]string) bool {
	for key := range candidates {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			return true
		}
	}
	return false
}

func looksLikePublicSCTableReply(folded string) bool {
	if !strings.Contains(folded, " santa catarina ") && !strings.Contains(folded, " sc ") {
		return false
	}
	matchCount := 0
	for key := range scPackageDestinations {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			matchCount++
		}
	}
	return matchCount >= 3
}

func hasOnlyAutoSendSafeToolCalls(toolCalls []ToolCall) bool {
	if len(toolCalls) == 0 {
		return false
	}
	for _, call := range toolCalls {
		if !isAutoSendSafeToolCall(call) {
			return false
		}
	}
	return true
}

func hasCompletedDocumentExtractToolCall(toolCalls []ToolCall) bool {
	for _, call := range toolCalls {
		if strings.TrimSpace(call.ToolName) == toolNameDocumentExtract &&
			strings.EqualFold(strings.TrimSpace(call.Status), "COMPLETED") {
			return true
		}
	}
	return false
}

// Until there is an operator UI for approving payment/reservation drafts, these
// completed tool calls are allowed to proceed through auto-send.
func isAutoSendSafeToolCall(call ToolCall) bool {
	if !strings.EqualFold(strings.TrimSpace(call.Status), "COMPLETED") {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case toolNameAvailabilitySearch,
		toolNameDocumentExtract,
		toolNamePricingQuote,
		toolNameBookingLookup,
		toolNameBookingCreate,
		toolNamePaymentCreate:
		return true
	default:
		return false
	}
}

func hasBlockingNonTextCandidate(candidates []Message, allowExtractedDocumentMedia bool) bool {
	for _, message := range candidates {
		kind := normalizedCandidateKind(message)
		normalized := message.NormalizedPayload
		switch kind {
		case "", "TEXT":
			if strings.TrimSpace(asString(normalized["document_file_name"])) != "" ||
				strings.TrimSpace(asString(normalized["document_url"])) != "" ||
				strings.TrimSpace(asString(normalized["document_mime_type"])) != "" {
				return true
			}
		case "IMAGE":
			continue
		case "DOCUMENT":
			if allowExtractedDocumentMedia && isExtractableDocumentCandidate(message) {
				continue
			}
			return true
		default:
			return true
		}
	}
	return false
}

func isExtractableDocumentCandidate(message Message) bool {
	for _, item := range collectCandidateMedia([]Message{message}) {
		switch strings.ToUpper(strings.TrimSpace(item.Kind)) {
		case "IMAGE", "PDF":
			return true
		}
	}
	return false
}

func hasBlockingUntranscribedAudioCandidate(candidates []Message) bool {
	message, ok := currentTurnUntranscribedAudioCandidate(candidates)
	return ok && message.ID != ""
}

func currentTurnUntranscribedAudioCandidate(candidates []Message) (Message, bool) {
	if len(candidates) == 0 {
		return Message{}, false
	}
	message := candidates[len(candidates)-1]
	if !isAudioMessageKind(message.Kind) {
		return Message{}, false
	}
	if strings.TrimSpace(message.Body) != "" {
		return Message{}, false
	}
	if hasCompletedAudioTranscription(message) {
		return Message{}, false
	}
	return message, true
}

func messageTurnText(message Message) string {
	body := strings.TrimSpace(message.Body)
	if body != "" {
		return body
	}
	if hasCompletedAudioTranscription(message) {
		return strings.TrimSpace(asString(message.NormalizedPayload["transcription_text"]))
	}
	return ""
}

func buildDraftGeneratedAgentState(metadata map[string]interface{}, candidates []Message, draftID string, run RunAgentResult, toolCalls []ToolCall, autoSend draftAutoSendPolicy, observedAt time.Time) map[string]interface{} {
	state := cloneNestedMetadataMap(metadata, "agent")
	runMode := agentRunMode(run)
	state["status"] = agentStatusDraftGenerated
	state["draft_idempotency_key"] = draftID
	state["draft_generated_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["draft_message_ids"] = candidateMessageIDs(candidates)
	state["draft_model"] = strings.TrimSpace(run.Model)
	if runMode != "" {
		state["run_mode"] = runMode
	}
	copyRunMetadata(state, run)
	state["provider_response_id"] = strings.TrimSpace(run.ProviderResponseID)
	state["provider_conversation_id"] = strings.TrimSpace(run.ProviderConversationID)
	state["tool_calls_count"] = len(toolCalls)
	state["auto_send_status"] = autoSend.Status
	if len(toolCalls) > 0 {
		state["tool_names"] = toolCallNames(toolCalls)
		state["tool_call_ids"] = toolCallIDs(toolCalls)
	}
	if len(autoSend.Reasons) > 0 {
		state["auto_send_reasons"] = autoSend.Reasons
	}
	return state
}

func buildDraftGeneratedBufferState(metadata map[string]interface{}, candidates []Message, draftID string, observedAt time.Time) map[string]interface{} {
	buffer := cloneBuffer(metadata)
	buffer["status"] = bufferStatusIdle
	buffer["agent_status"] = agentStatusDraftGenerated
	buffer["draft_idempotency_key"] = draftID
	buffer["draft_generated_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	buffer["current_turn_message_ids"] = candidateMessageIDs(candidates)
	return buffer
}

func buildDraftReviewedAgentState(metadata map[string]interface{}, draft Message, ownerUserID string, action string, observedAt time.Time) map[string]interface{} {
	state := cloneNestedMetadataMap(metadata, "agent")
	state["status"] = agentStatusDraftReviewed
	state["reviewed_draft_message_id"] = draft.ID
	state["reviewed_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["reviewed_by_user_id"] = strings.TrimSpace(ownerUserID)
	state["review_action"] = strings.TrimSpace(action)
	return state
}

func buildDraftAutoSentAgentState(metadata map[string]interface{}, draft Message, reply Message, outbound ReplyOutbound, observedAt time.Time) map[string]interface{} {
	state := cloneNestedMetadataMap(metadata, "agent")
	state["status"] = agentStatusDraftAutoSent
	state["auto_sent_draft_message_id"] = draft.ID
	state["auto_sent_message_id"] = reply.ID
	state["auto_sent_outbound_id"] = outbound.ID
	state["auto_sent_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["auto_send_status"] = draftAutoSendStatusEligible
	if strings.TrimSpace(outbound.ProviderMessageID) != "" {
		state["auto_sent_provider_message_id"] = strings.TrimSpace(outbound.ProviderMessageID)
	}
	return state
}

func buildDraftAutoSendRetryPendingAgentState(metadata map[string]interface{}, draft Message, reply Message, outbound ReplyOutbound, errorText string, observedAt time.Time) map[string]interface{} {
	state := cloneNestedMetadataMap(metadata, "agent")
	state["status"] = agentStatusDraftGenerated
	state["draft_idempotency_key"] = firstNonEmpty(
		asString(draft.NormalizedPayload["draft_idempotency_key"]),
		asString(draft.Payload["draft_idempotency_key"]),
		strings.TrimSpace(draft.IdempotencyKey),
		asString(state["draft_idempotency_key"]),
	)
	state["draft_generated_at"] = firstNonEmpty(
		asString(draft.NormalizedPayload["generated_at"]),
		asString(draft.Payload["generated_at"]),
		asString(state["draft_generated_at"]),
	)
	state["auto_send_status"] = draftAutoSendStatusRetryPending
	state["auto_send_reasons"] = mergeDistinctStrings(
		firstNonEmptyStringSlice(
			asStringSlice(draft.NormalizedPayload["auto_send_reasons"]),
			asStringSlice(draft.Payload["auto_send_reasons"]),
			asStringSlice(state["auto_send_reasons"]),
		),
		draftAutoSendReasonDeliveryFail,
	)
	state["auto_send_last_attempt_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["auto_send_last_error_text"] = strings.TrimSpace(errorText)
	state["auto_send_last_reply_message_id"] = reply.ID
	state["auto_send_last_outbound_id"] = outbound.ID
	return state
}

func buildDraftAutoSendRetryRequestedPayload(draft Message, input RetryDraftAutoSendInput, observedAt time.Time) map[string]interface{} {
	payload := map[string]interface{}{
		"auto_send_status":             draftAutoSendStatusRetryPending,
		"auto_send_retry_requested_at": observedAt.UTC().Format(time.RFC3339Nano),
		"auto_send_retry_request_count": maxInts(
			asInt(draft.NormalizedPayload["auto_send_retry_request_count"]),
			asInt(draft.Payload["auto_send_retry_request_count"]),
		) + 1,
	}
	if requestedBy := strings.TrimSpace(input.RequestedBy); requestedBy != "" {
		payload["auto_send_retry_requested_by"] = requestedBy
	}
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		payload["auto_send_retry_request_reason"] = reason
	}
	if len(input.Metadata) > 0 {
		payload["auto_send_retry_request_metadata"] = cloneMap(input.Metadata)
	}
	return payload
}

func buildDraftAutoSendRetryRequestedAgentState(metadata map[string]interface{}, draft Message, input RetryDraftAutoSendInput, observedAt time.Time) map[string]interface{} {
	state := cloneNestedMetadataMap(metadata, "agent")
	state["status"] = agentStatusDraftGenerated
	state["draft_idempotency_key"] = firstNonEmpty(
		asString(draft.NormalizedPayload["draft_idempotency_key"]),
		asString(draft.Payload["draft_idempotency_key"]),
		strings.TrimSpace(draft.IdempotencyKey),
		asString(state["draft_idempotency_key"]),
	)
	state["draft_generated_at"] = firstNonEmpty(
		asString(draft.NormalizedPayload["generated_at"]),
		asString(draft.Payload["generated_at"]),
		asString(state["draft_generated_at"]),
	)
	state["auto_send_status"] = draftAutoSendStatusRetryPending
	state["auto_send_reasons"] = firstNonEmptyStringSlice(
		asStringSlice(draft.NormalizedPayload["auto_send_reasons"]),
		asStringSlice(draft.Payload["auto_send_reasons"]),
		asStringSlice(state["auto_send_reasons"]),
	)
	state["auto_send_retry_requested_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["auto_send_retry_request_count"] = maxInts(
		asInt(draft.NormalizedPayload["auto_send_retry_request_count"]),
		asInt(draft.Payload["auto_send_retry_request_count"]),
		asInt(state["auto_send_retry_request_count"]),
	) + 1
	if requestedBy := strings.TrimSpace(input.RequestedBy); requestedBy != "" {
		state["auto_send_retry_requested_by"] = requestedBy
	}
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		state["auto_send_retry_request_reason"] = reason
	}
	if len(input.Metadata) > 0 {
		state["auto_send_retry_request_metadata"] = cloneMap(input.Metadata)
	}
	return state
}

func buildDraftAutoSendBlockedAgentState(metadata map[string]interface{}, draft Message, session Session, observedAt time.Time) map[string]interface{} {
	state := cloneNestedMetadataMap(metadata, "agent")
	state["status"] = agentStatusDraftGenerated
	state["draft_idempotency_key"] = firstNonEmpty(
		asString(draft.NormalizedPayload["draft_idempotency_key"]),
		asString(draft.Payload["draft_idempotency_key"]),
		strings.TrimSpace(draft.IdempotencyKey),
		asString(state["draft_idempotency_key"]),
	)
	state["draft_generated_at"] = firstNonEmpty(
		asString(draft.NormalizedPayload["generated_at"]),
		asString(draft.Payload["generated_at"]),
		asString(state["draft_generated_at"]),
	)
	state["auto_send_status"] = draftAutoSendStatusBlockedHuman
	state["auto_send_reasons"] = mergeDistinctStrings(
		firstNonEmptyStringSlice(
			asStringSlice(draft.NormalizedPayload["auto_send_reasons"]),
			asStringSlice(draft.Payload["auto_send_reasons"]),
			asStringSlice(state["auto_send_reasons"]),
		),
		draftAutoSendReasonHumanHandoff,
	)
	state["auto_send_blocked_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["auto_send_block_reason"] = draftAutoSendReasonHumanHandoff
	state["handoff_status"] = session.HandoffStatus
	if strings.TrimSpace(session.CurrentOwnerUserID) != "" {
		state["current_owner_user_id"] = strings.TrimSpace(session.CurrentOwnerUserID)
	}
	return state
}

func buildAgentDraftPayload(session Session, candidates []Message, draftID string, systemPrompt string, userPrompt string, run RunAgentResult, tools agentToolContext, autoSend draftAutoSendPolicy, observedAt time.Time) (map[string]interface{}, map[string]interface{}) {
	runMode := agentRunMode(run)
	payload := map[string]interface{}{
		"mode":                     "AUTOMATION_DRAFT",
		"draft_idempotency_key":    draftID,
		"generated_at":             observedAt.UTC().Format(time.RFC3339Nano),
		"current_turn_message_ids": candidateMessageIDs(candidates),
		"model":                    strings.TrimSpace(run.Model),
		"provider_response_id":     strings.TrimSpace(run.ProviderResponseID),
		"provider_conversation_id": strings.TrimSpace(run.ProviderConversationID),
		"auto_send_status":         autoSend.Status,
		"request_payload":          run.RequestPayload,
		"response_payload":         run.ResponsePayload,
		"system_prompt":            systemPrompt,
		"user_prompt":              userPrompt,
		"session_channel":          session.Channel,
		"contact_key":              session.ContactKey,
	}
	if runMode != "" {
		payload["run_mode"] = runMode
	}
	copyRunMetadata(payload, run)
	if len(tools.Calls) > 0 {
		payload["tool_calls"] = serializeToolCalls(tools.Calls)
	}
	if len(autoSend.Reasons) > 0 {
		payload["auto_send_reasons"] = autoSend.Reasons
	}
	if tools.Availability != nil || tools.Pricing != nil || tools.Booking != nil || tools.BookingCreate != nil || tools.Payments != nil || tools.PaymentCreate != nil || tools.BookingCancel != nil || tools.DocumentExtract != nil {
		toolContext := map[string]interface{}{}
		if tools.Availability != nil {
			toolContext[toolNameAvailabilitySearch] = buildAvailabilityToolResponsePayload(*tools.Availability)
		}
		if tools.Pricing != nil {
			toolContext[toolNamePricingQuote] = buildPricingQuoteResponsePayload(*tools.Pricing)
		}
		if tools.Booking != nil {
			toolContext[toolNameBookingLookup] = buildBookingLookupResponsePayload(*tools.Booking)
		}
		if tools.BookingCreate != nil {
			toolContext[toolNameBookingCreate] = buildBookingCreateResponsePayload(*tools.BookingCreate)
		}
		if tools.Payments != nil {
			toolContext[toolNamePaymentStatus] = buildPaymentStatusResponsePayload(*tools.Payments)
		}
		if tools.PaymentCreate != nil {
			toolContext[toolNamePaymentCreate] = buildPaymentCreateResponsePayload(*tools.PaymentCreate)
		}
		if tools.BookingCancel != nil {
			toolContext[toolNameBookingCancel] = buildBookingCancelResponsePayload(*tools.BookingCancel)
		}
		if tools.DocumentExtract != nil {
			toolContext[toolNameDocumentExtract] = buildDocumentExtractResponsePayload(*tools.DocumentExtract)
		}
		payload["tool_context"] = toolContext
	}
	normalized := map[string]interface{}{
		"mode":                     "AUTOMATION_DRAFT",
		"draft_idempotency_key":    draftID,
		"generated_at":             observedAt.UTC().Format(time.RFC3339Nano),
		"agent_status":             agentStatusDraftGenerated,
		"current_turn_message_ids": candidateMessageIDs(candidates),
		"model":                    strings.TrimSpace(run.Model),
		"provider_response_id":     strings.TrimSpace(run.ProviderResponseID),
		"provider_conversation_id": strings.TrimSpace(run.ProviderConversationID),
		"auto_send_status":         autoSend.Status,
		"tool_call_count":          len(tools.Calls),
	}
	if runMode != "" {
		normalized["run_mode"] = runMode
	}
	copyRunMetadata(normalized, run)
	if len(tools.Calls) > 0 {
		normalized["tool_names"] = toolCallNames(tools.Calls)
	}
	if len(autoSend.Reasons) > 0 {
		normalized["auto_send_reasons"] = autoSend.Reasons
	}
	return payload, normalized
}

func agentRunMode(run RunAgentResult) string {
	return firstNonEmpty(
		strings.TrimSpace(asString(run.RequestPayload["mode"])),
		strings.TrimSpace(asString(run.ResponsePayload["mode"])),
	)
}

func copyRunMetadata(target map[string]interface{}, run RunAgentResult) {
	for _, key := range []string{
		"intent",
		"template_name",
		"action",
		"chat_agent_mode",
		"decision_source",
		"decision_valid",
		"decision_confidence",
		"validation_errors",
		"fallback_reason",
		"canonical_phase_before",
		"canonical_phase_after",
		"tool_call_count",
	} {
		value := firstNonEmpty(
			strings.TrimSpace(asString(run.RequestPayload[key])),
			strings.TrimSpace(asString(run.ResponsePayload[key])),
		)
		if value != "" {
			target[key] = value
			continue
		}
		if value, ok := run.RequestPayload[key]; ok {
			target[key] = value
			continue
		}
		if value, ok := run.ResponsePayload[key]; ok {
			target[key] = value
		}
	}
}

func cloneNestedMetadataMap(metadata map[string]interface{}, key string) map[string]interface{} {
	raw, ok := metadata[key].(map[string]interface{})
	if !ok || len(raw) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(raw))
	for nestedKey, value := range raw {
		cloned[nestedKey] = value
	}
	return cloned
}

func cloneMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func mergeDistinctStrings(base []string, extras ...string) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(base)+len(extras))
	for _, item := range base {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	for _, item := range extras {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func maxInts(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	best := values[0]
	for _, value := range values[1:] {
		if value > best {
			best = value
		}
	}
	return best
}

func toolCallNames(toolCalls []ToolCall) []string {
	names := make([]string, 0, len(toolCalls))
	for _, item := range toolCalls {
		if name := strings.TrimSpace(item.ToolName); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func toolCallIDs(toolCalls []ToolCall) []string {
	ids := make([]string, 0, len(toolCalls))
	for _, item := range toolCalls {
		if id := strings.TrimSpace(item.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func serializeToolCalls(toolCalls []ToolCall) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(toolCalls))
	for _, item := range toolCalls {
		payload := map[string]interface{}{
			"id":               item.ID,
			"tool_name":        item.ToolName,
			"status":           item.Status,
			"request_payload":  item.RequestPayload,
			"response_payload": item.ResponsePayload,
			"started_at":       item.StartedAt.UTC().Format(time.RFC3339Nano),
		}
		if item.ErrorCode != "" {
			payload["error_code"] = item.ErrorCode
		}
		if item.ErrorMessage != "" {
			payload["error_message"] = item.ErrorMessage
		}
		if item.FinishedAt != nil {
			payload["finished_at"] = item.FinishedAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, payload)
	}
	return items
}
