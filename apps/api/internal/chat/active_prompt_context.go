package chat

import (
	"strings"
	"time"
)

type ActivePromptKind string

const (
	ActivePromptUnknown                  ActivePromptKind = "UNKNOWN"
	ActivePromptReservationRoute         ActivePromptKind = "RESERVATION_ROUTE"
	ActivePromptAvailabilityDateChoice   ActivePromptKind = "AVAILABILITY_DATE_CHOICE"
	ActivePromptAvailabilityOptionChoice ActivePromptKind = "AVAILABILITY_OPTION_CHOICE"
	ActivePromptPassengerCount           ActivePromptKind = "PASSENGER_COUNT"
	ActivePromptLapChildQuestion         ActivePromptKind = "LAP_CHILD_QUESTION"
	ActivePromptLapChildAssignment       ActivePromptKind = "LAP_CHILD_ASSIGNMENT"
	ActivePromptPassengerDocuments       ActivePromptKind = "PASSENGER_DOCUMENTS"
	ActivePromptDocumentConfirmation     ActivePromptKind = "DOCUMENT_CONFIRMATION"
	ActivePromptPaymentPreference        ActivePromptKind = "PAYMENT_PREFERENCE"
	ActivePromptPayerCPF                 ActivePromptKind = "PAYER_CPF"
)

type ActivePromptContext struct {
	Kind                    ActivePromptKind  `json:"kind"`
	Phase                   ConversationPhase `json:"phase,omitempty"`
	SourceMessageID         string            `json:"source_message_id,omitempty"`
	SourceMessageBody       string            `json:"source_message_body,omitempty"`
	SourceMessageReceivedAt time.Time         `json:"source_message_received_at,omitempty"`
	AvailabilityOptionCount int               `json:"availability_option_count,omitempty"`
	HasAvailabilityList     bool              `json:"has_availability_list,omitempty"`
}

func InferActivePromptContext(history []Message, state CanonicalConversationState) ActivePromptContext {
	context := ActivePromptContext{
		Kind:  ActivePromptUnknown,
		Phase: state.Phase,
	}

	message, ok := latestReliableAssistantMessage(history)
	if !ok {
		return context
	}

	body := strings.TrimSpace(messageTurnText(message))
	if messageMayCarryAvailabilityPromptV1(message) {
		authority := classifyAvailabilityPromptCandidateV1(message)
		switch authority.Class {
		case availabilityPromptAuthorityValidStructuralV1:
			if authority.Presented == nil {
				return context
			}
			event := authority.Event
			context.SourceMessageID = event.SourceMessageID
			context.SourceMessageBody = body
			context.SourceMessageReceivedAt = message.ReceivedAt
			context.Kind = event.Kind
			context.AvailabilityOptionCount = event.PresentedOptionCount
			context.HasAvailabilityList = event.PresentedOptionCount > 0
			return context
		case availabilityPromptAuthorityAbsentLegacyV1:
		default:
			return context
		}
	}
	context.SourceMessageID = strings.TrimSpace(message.ID)
	context.SourceMessageBody = body
	context.SourceMessageReceivedAt = message.ReceivedAt
	availabilityOptionCount := activePromptAvailabilityOptionCount(message, history, state)
	context.Kind = inferActivePromptKind(body, availabilityOptionCount)
	if context.Kind == ActivePromptAvailabilityOptionChoice {
		context.SourceMessageID = availabilityPromptSourceMessageIDFromMessage(message)
		context.AvailabilityOptionCount = availabilityOptionCount
		context.HasAvailabilityList = availabilityOptionCount > 0
	}

	return context
}

func latestReliableAssistantMessage(history []Message) (Message, bool) {
	message, _, ok := latestReliableAssistantMessageWithIndex(history)
	return message, ok
}

func latestReliableAssistantMessageWithIndex(history []Message) (Message, int, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		availabilityCandidate := messageMayCarryAvailabilityPromptV1(message)
		authority := classifyAvailabilityPromptCandidateV1(message)
		if availabilityCandidate {
			switch authority.Class {
			case availabilityPromptAuthorityInvalidV1:
				return Message{}, -1, false
			case availabilityPromptAuthorityUndeliveredV1:
				continue
			}
		}
		if strings.TrimSpace(messageTurnText(message)) == "" {
			continue
		}
		if !isReliableActivePromptOutbound(message) {
			continue
		}
		if isDeliveredPromptProjectionMessageV1(message) {
			if source, sourceIndex, ok := resolveDeliveredPromptSourceMessageWithIndex(history, i, message); ok {
				return source, sourceIndex, true
			}
			if messageHasAvailabilityPromptEventV1(message) {
				continue
			}
			return withoutPromptToolContext(message), i, true
		}
		if authority.Class == availabilityPromptAuthorityValidStructuralV1 &&
			authority.Presented == nil {
			continue
		}
		return message, i, true
	}
	return Message{}, -1, false
}

func resolveBotAutoReplyPromptSourceMessage(history []Message, mirrorIndex int, mirror Message) (Message, bool) {
	message, _, ok := resolveBotAutoReplyPromptSourceMessageWithIndex(history, mirrorIndex, mirror)
	return message, ok
}

func resolveBotAutoReplyPromptSourceMessageWithIndex(history []Message, mirrorIndex int, mirror Message) (Message, int, bool) {
	if !isBotAutoReplyMessage(mirror) {
		return Message{}, -1, false
	}
	return resolveDeliveredPromptSourceMessageWithIndex(history, mirrorIndex, mirror)
}

type deliveredPromptProjectionKindV1 string

const (
	deliveredPromptProjectionBotAutoReplyV1 deliveredPromptProjectionKindV1 = "BOT_AUTO_REPLY"
	deliveredPromptProjectionDraftReviewV1  deliveredPromptProjectionKindV1 = "DRAFT_REVIEW"
)

func resolveDeliveredPromptSourceMessageWithIndex(
	history []Message,
	deliveredIndex int,
	delivered Message,
) (Message, int, bool) {
	draftID, projectionKind, ok := deliveredPromptSourceReferenceV1(delivered)
	if !ok || !isTrustedDeliveredPromptOutboundV1(delivered) {
		return Message{}, -1, false
	}
	for i := deliveredIndex - 1; i >= 0; i-- {
		candidate := history[i]
		if strings.TrimSpace(candidate.ID) != draftID {
			continue
		}
		if !deliveredPromptSourceTrustworthyV1(candidate, projectionKind) {
			return Message{}, -1, false
		}

		sourceHasEvent := messageHasAvailabilityPromptEventV1(candidate)
		deliveredHasEvent := messageHasAvailabilityPromptEventV1(delivered)
		if sourceHasEvent || deliveredHasEvent {
			if !sourceHasEvent || !deliveredHasEvent {
				return Message{}, -1, false
			}
			resolved, eventOK := availabilityPromptMessageFromDeliveredSourceV1(candidate, delivered)
			if !eventOK {
				return Message{}, -1, false
			}
			return resolved, deliveredIndex, true
		}

		if !equivalentAssistantPromptBody(messageTurnText(candidate), messageTurnText(delivered)) {
			return Message{}, -1, false
		}
		return effectiveDeliveredPromptMessageV1(candidate, delivered), deliveredIndex, true
	}
	return Message{}, -1, false
}

func deliveredPromptSourceTrustworthyV1(
	source Message,
	projectionKind deliveredPromptProjectionKindV1,
) bool {
	if !strings.EqualFold(strings.TrimSpace(source.Direction), "OUTBOUND") ||
		strings.TrimSpace(messageTurnText(source)) == "" ||
		isDeliveredPromptProjectionMessageV1(source) {
		return false
	}
	mode, ok := consistentPromptMetadataStringV1(source, "mode")
	if !ok || !strings.EqualFold(mode, messageStatusAutomationDraft) {
		return false
	}

	switch projectionKind {
	case deliveredPromptProjectionBotAutoReplyV1:
		return isReliableActivePromptOutbound(source)
	case deliveredPromptProjectionDraftReviewV1:
		if !strings.EqualFold(
			strings.TrimSpace(source.ProcessingStatus),
			messageStatusAutomationReviewed,
		) {
			return false
		}
		reviewAction, actionOK := consistentPromptMetadataStringV1(source, "review_action")
		return actionOK && strings.EqualFold(reviewAction, "APPROVED_AS_IS")
	default:
		return false
	}
}

func deliveredPromptSourceReferenceV1(
	message Message,
) (string, deliveredPromptProjectionKindV1, bool) {
	mode, ok := consistentPromptMetadataStringV1(message, "mode")
	if !ok {
		return "", "", false
	}
	switch {
	case strings.EqualFold(mode, string(deliveredPromptProjectionBotAutoReplyV1)):
		draftID := botAutoReplyDraftMessageID(message)
		if draftID == "" {
			return "", "", false
		}
		return draftID, deliveredPromptProjectionBotAutoReplyV1, true
	case strings.EqualFold(mode, string(deliveredPromptProjectionDraftReviewV1)):
		draftID, draftOK := consistentPromptMetadataStringV1(message, "draft_message_id")
		if !draftOK {
			return "", "", false
		}
		reviewAction, actionOK := consistentPromptMetadataStringV1(message, "review_action")
		if !actionOK || !strings.EqualFold(reviewAction, "APPROVED_AS_IS") {
			return "", "", false
		}
		return draftID, deliveredPromptProjectionDraftReviewV1, true
	default:
		return "", "", false
	}
}

func isDeliveredPromptProjectionMessageV1(message Message) bool {
	if isBotAutoReplyMessage(message) {
		return true
	}
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if strings.EqualFold(
			strings.TrimSpace(asString(payload["mode"])),
			string(deliveredPromptProjectionDraftReviewV1),
		) {
			return true
		}
	}
	return false
}

func consistentPromptMetadataStringV1(message Message, key string) (string, bool) {
	value := ""
	found := false
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if payload == nil {
			continue
		}
		raw, exists := payload[key]
		if !exists {
			continue
		}
		candidate := strings.TrimSpace(asString(raw))
		if candidate == "" {
			return "", false
		}
		if found && !strings.EqualFold(value, candidate) {
			return "", false
		}
		value = candidate
		found = true
	}
	return value, found
}

func isTrustedDeliveredPromptOutboundV1(message Message) bool {
	switch classifyAvailabilityPromptCandidateV1(message).Class {
	case availabilityPromptAuthorityAbsentLegacyV1, availabilityPromptAuthorityValidStructuralV1:
		return true
	default:
		return false
	}
}

func confirmedOutboundDeliveryV1(message Message) bool {
	effectiveStatus := HighestDeliveryStatusV1(
		message.ProcessingStatus,
		asString(message.NormalizedPayload["provider_status"]),
	)
	if !DeliveryStatusConfirmsOutboundV1(effectiveStatus) {
		return false
	}
	return strings.TrimSpace(asString(message.NormalizedPayload["delivery_recorded_at"])) != ""
}

func effectiveDeliveredPromptMessageV1(source Message, delivered Message) Message {
	resolved := source
	resolved.ID = delivered.ID
	resolved.SessionID = delivered.SessionID
	resolved.Direction = delivered.Direction
	resolved.Kind = delivered.Kind
	resolved.ProviderMessageID = delivered.ProviderMessageID
	resolved.IdempotencyKey = delivered.IdempotencyKey
	resolved.SenderName = delivered.SenderName
	resolved.SenderPhone = delivered.SenderPhone
	resolved.Body = delivered.Body
	resolved.ProcessingStatus = delivered.ProcessingStatus
	resolved.ReceivedAt = delivered.ReceivedAt
	resolved.CreatedAt = delivered.CreatedAt
	resolved.SentAt = delivered.SentAt
	resolved.Payload = cloneMap(source.Payload)
	if resolved.Payload == nil {
		resolved.Payload = map[string]interface{}{}
	}
	resolved.NormalizedPayload = cloneMap(source.NormalizedPayload)
	if resolved.NormalizedPayload == nil {
		resolved.NormalizedPayload = map[string]interface{}{}
	}
	copyDeliveredPromptProvenanceV1(resolved.Payload, delivered.Payload)
	copyDeliveredPromptProvenanceV1(resolved.NormalizedPayload, delivered.NormalizedPayload)
	return resolved
}

func copyDeliveredPromptProvenanceV1(target map[string]interface{}, source map[string]interface{}) {
	for _, key := range []string{
		"mode",
		"draft_message_id",
		"review_mode",
		"review_action",
		"draft_reviewed",
		"draft_auto_sent",
		"sender_name",
		"auto_send_status",
		"delivery_mode",
		"delivery_recorded_at",
		"provider_status",
		passengerPromptEventV1MessageKey,
	} {
		if value, exists := source[key]; exists {
			target[key] = value
		}
	}
}

func botAutoReplyDraftMessageID(message Message) string {
	if !isBotAutoReplyMessage(message) {
		return ""
	}
	draftID, ok := consistentPromptMetadataStringV1(message, "draft_message_id")
	if !ok {
		return ""
	}
	return draftID
}

func equivalentAssistantPromptBody(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	if strings.Join(strings.Fields(left), " ") == strings.Join(strings.Fields(right), " ") {
		return true
	}
	return activePromptFolded(left) == activePromptFolded(right)
}

func withoutPromptToolContext(message Message) Message {
	if len(message.Payload) > 0 {
		payload := cloneMap(message.Payload)
		delete(payload, "tool_context")
		message.Payload = payload
	}
	if len(message.NormalizedPayload) > 0 {
		normalizedPayload := cloneMap(message.NormalizedPayload)
		delete(normalizedPayload, "tool_context")
		message.NormalizedPayload = normalizedPayload
	}
	return message
}

func isReliableActivePromptOutbound(message Message) bool {
	status := strings.ToUpper(strings.TrimSpace(message.ProcessingStatus))
	switch status {
	case messageStatusAutomationDraft, messageStatusAutomationReviewed, messageStatusAutomationPending,
		"PENDING", "MANUAL_PENDING", "SEND_FAILED", "REVIEW_REQUIRED", "PENDING_REVIEW", "BLOCKED_BY_REVIEW":
		return false
	}
	if strings.Contains(status, "REVIEW_REQUIRED") ||
		strings.Contains(status, "PENDING_REVIEW") ||
		strings.Contains(status, "BLOCKED") {
		return false
	}
	if status == messageStatusAutomationSent {
		return true
	}

	mode := strings.ToUpper(strings.TrimSpace(firstNonEmpty(
		asString(message.Payload["mode"]),
		asString(message.NormalizedPayload["mode"]),
	)))
	switch mode {
	case messageStatusAutomationDraft, messageStatusAutomationReviewed, messageStatusAutomationPending:
		return false
	}

	autoSendStatus := strings.ToUpper(strings.TrimSpace(firstNonEmpty(
		asString(message.Payload["auto_send_status"]),
		asString(message.NormalizedPayload["auto_send_status"]),
	)))
	return autoSendStatus != draftAutoSendStatusBlockedHuman &&
		autoSendStatus != draftAutoSendStatusReviewNeeded
}

func inferActivePromptKind(text string, availabilityOptionCount int) ActivePromptKind {
	folded := activePromptFolded(text)
	if folded == "" {
		return ActivePromptUnknown
	}

	if continuation := activePromptContinuationReminderFolded(folded); continuation != "" {
		if kind := inferActivePromptKindFromFolded(continuation, availabilityOptionCount); kind != ActivePromptUnknown {
			return kind
		}
	}

	return inferActivePromptKindFromFolded(folded, availabilityOptionCount)
}

func inferActivePromptKindFromFolded(folded string, availabilityOptionCount int) ActivePromptKind {
	switch {
	case activePromptLooksLikePayerCPF(folded):
		return ActivePromptPayerCPF
	case activePromptLooksLikePaymentPreference(folded):
		return ActivePromptPaymentPreference
	case activePromptLooksLikeDocumentConfirmation(folded):
		return ActivePromptDocumentConfirmation
	case activePromptLooksLikePassengerDocuments(folded):
		return ActivePromptPassengerDocuments
	case activePromptLooksLikeLapChildAssignment(folded):
		return ActivePromptLapChildAssignment
	case activePromptLooksLikePassengerCount(folded):
		return ActivePromptPassengerCount
	case activePromptLooksLikeLapChildQuestion(folded):
		return ActivePromptLapChildQuestion
	case activePromptLooksLikeAvailabilityDateChoice(folded):
		return ActivePromptAvailabilityDateChoice
	case activePromptLooksLikeAvailabilityOptionChoice(folded, availabilityOptionCount):
		return ActivePromptAvailabilityOptionChoice
	case activePromptLooksLikeReservationRoute(folded):
		return ActivePromptReservationRoute
	default:
		return ActivePromptUnknown
	}
}

func activePromptContinuationReminderFolded(folded string) string {
	const marker = "para continuar"
	index := strings.LastIndex(folded, marker)
	if index < 0 {
		return ""
	}
	return strings.TrimSpace(folded[index+len(marker):])
}

func activePromptFolded(text string) string {
	return strings.Join(strings.Fields(foldChatText(text)), " ")
}

func activePromptLooksLikePayerCPF(folded string) bool {
	return strings.Contains(folded, "cpf do pagador") ||
		(strings.Contains(folded, "preciso do cpf") &&
			(strings.Contains(folded, "pix") || strings.Contains(folded, "pagador") || strings.Contains(folded, "pagamento"))) ||
		(strings.Contains(folded, "para gerar o pix") && strings.Contains(folded, "cpf"))
}

func activePromptLooksLikePaymentPreference(folded string) bool {
	if activePromptLooksLikeInformationalPaymentReply(folded) {
		return false
	}
	return looksLikePaymentPreferencePrompt(folded) ||
		(strings.Contains(folded, "integral") && strings.Contains(folded, "sinal")) ||
		(strings.Contains(folded, "pagar tudo") && strings.Contains(folded, "entrada"))
}

func activePromptLooksLikeInformationalPaymentReply(folded string) bool {
	switch strings.Join(strings.Fields(folded), " ") {
	case activePromptFolded(paymentOptionsInfoReply),
		activePromptFolded(payingPassengerInfoReply):
		return true
	default:
		return false
	}
}

func activePromptLooksLikeDocumentConfirmation(folded string) bool {
	hasDocumentCue := strings.Contains(folded, "consegui identificar estes dados") ||
		strings.Contains(folded, "consegui ler parte do documento") ||
		strings.Contains(folded, "dados lidos") ||
		strings.Contains(folded, "documento lido") ||
		strings.Contains(folded, "dados extraidos")
	if !hasDocumentCue {
		return false
	}
	return strings.Contains(folded, "conferem") ||
		strings.Contains(folded, "confere") ||
		strings.Contains(folded, "confirmar") ||
		strings.Contains(folded, "corretos") ||
		strings.Contains(folded, "posso prosseguir") ||
		strings.Contains(folded, "criar a reserva")
}

func activePromptLooksLikePassengerDocuments(folded string) bool {
	if looksLikePassengerDocumentRequest(folded) {
		return true
	}

	hasNameCue := strings.Contains(folded, "nome completo") || strings.Contains(folded, "nomes completos")
	hasDocumentCue := strings.Contains(folded, "cpf") ||
		strings.Contains(folded, "rg") ||
		strings.Contains(folded, "cnh") ||
		strings.Contains(folded, "documento")
	hasRequestCue := strings.Contains(folded, "pode enviar") ||
		strings.Contains(folded, "envie") ||
		strings.Contains(folded, "enviar") ||
		strings.Contains(folded, "mandar") ||
		strings.Contains(folded, "digitar") ||
		strings.Contains(folded, "escrever") ||
		strings.Contains(folded, "reenviar")
	hasPhotoCue := (strings.Contains(folded, "foto") || strings.Contains(folded, "fotos")) &&
		strings.Contains(folded, "documento")

	return (hasNameCue && hasDocumentCue && hasRequestCue) || (hasPhotoCue && hasRequestCue)
}

func activePromptLooksLikeLapChildAssignment(folded string) bool {
	return strings.Contains(folded, "qual deles e a crianca") ||
		strings.Contains(folded, "qual passageiro e a crianca") ||
		strings.Contains(folded, "qual passageiro e crianca") ||
		strings.Contains(folded, "crianca de colo") && strings.Contains(folded, "qual")
}

func activePromptLooksLikePassengerCount(folded string) bool {
	return strings.Contains(folded, "a passagem e so para voce") ||
		strings.Contains(folded, "e so para voce ou") ||
		strings.Contains(folded, "so para voce ou") ||
		strings.Contains(folded, "tem mais alguem") ||
		strings.Contains(folded, "vai mais alguem") ||
		strings.Contains(folded, "quantas pessoas") ||
		strings.Contains(folded, "quantos passageiros")
}

func activePromptLooksLikeLapChildQuestion(folded string) bool {
	hasChildCue := strings.Contains(folded, "crianca de ate 5 anos") ||
		strings.Contains(folded, "crianca de 5 anos ou menos") ||
		strings.Contains(folded, "tem ate 5 anos") ||
		strings.Contains(folded, "ate 5 anos viajando") ||
		strings.Contains(folded, "crianca de colo")
	hasQuestionCue := strings.Contains(folded, "tem") ||
		strings.Contains(folded, "ha") ||
		strings.Contains(folded, "existe")
	return hasChildCue && hasQuestionCue
}

func activePromptLooksLikeAvailabilityDateChoice(folded string) bool {
	if !strings.Contains(folded, "data") && !strings.Contains(folded, "datas") && !strings.Contains(folded, "dia") {
		return false
	}
	return strings.Contains(folded, "qual") &&
		(strings.Contains(folded, "prefere") ||
			strings.Contains(folded, "escolhe") ||
			strings.Contains(folded, "escolher") ||
			strings.Contains(folded, "dessas"))
}

func activePromptLooksLikeAvailabilityOptionChoice(folded string, availabilityOptionCount int) bool {
	if strings.Contains(folded, "qual opcao voce prefere") ||
		strings.Contains(folded, "qual opcao prefere") ||
		strings.Contains(folded, "qual dessas opcoes") ||
		strings.Contains(folded, "qual horario voce prefere") ||
		strings.Contains(folded, "qual horario prefere") {
		return true
	}
	if availabilityOptionCount <= 0 {
		return false
	}
	return looksLikeAvailabilitySelectionPrompt(folded)
}

func activePromptLooksLikeReservationRoute(folded string) bool {
	if strings.Contains(folded, "trecho da viagem") ||
		strings.Contains(folded, "qual trecho") {
		return true
	}
	if strings.Contains(folded, "cidade de origem") ||
		strings.Contains(folded, "cidade de destino") ||
		strings.Contains(folded, "de qual cidade") ||
		strings.Contains(folded, "para qual cidade") {
		return true
	}
	return strings.Contains(folded, "qual cidade") &&
		(strings.Contains(folded, "sair") || strings.Contains(folded, "quer ir") || strings.Contains(folded, "destino"))
}

func activePromptAvailabilityOptionCount(message Message, history []Message, state CanonicalConversationState) int {
	if messageMayCarryAvailabilityPromptV1(message) {
		authority := classifyAvailabilityPromptCandidateV1(message)
		switch authority.Class {
		case availabilityPromptAuthorityValidStructuralV1:
			if authority.Presented != nil {
				return authority.Event.PresentedOptionCount
			}
			return 0
		case availabilityPromptAuthorityAbsentLegacyV1:
		default:
			return 0
		}
	}
	if messageHasAvailabilityPromptEventV1(message) {
		return 0
	}
	if count := availabilityOptionCountFromMessage(message); count > 0 {
		return count
	}
	if availability := asMap(state.LastToolFacts[toolNameAvailabilitySearch]); len(availability) > 0 {
		if count := len(asInterfaceSliceMaps(availability["results"])); count > 0 {
			return count
		}
	}
	if latest := findLatestAvailabilityContext(history); latest != nil && len(latest.Results) > 0 {
		return len(latest.Results)
	}
	return 0
}
