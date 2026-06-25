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

	message, ok := latestReliableActivePromptMessage(history)
	if !ok {
		return context
	}

	body := strings.TrimSpace(messageTurnText(message))
	context.SourceMessageID = strings.TrimSpace(message.ID)
	context.SourceMessageBody = body
	context.SourceMessageReceivedAt = message.ReceivedAt
	availabilityOptionCount := activePromptAvailabilityOptionCount(message, history, state)
	context.Kind = inferActivePromptKind(body, availabilityOptionCount)
	if context.Kind == ActivePromptAvailabilityOptionChoice {
		context.AvailabilityOptionCount = availabilityOptionCount
		context.HasAvailabilityList = availabilityOptionCount > 0
	}

	return context
}

func latestReliableActivePromptMessage(history []Message) (Message, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		if strings.TrimSpace(messageTurnText(message)) == "" {
			continue
		}
		if !isReliableActivePromptOutbound(message) {
			continue
		}
		return message, true
	}
	return Message{}, false
}

func isReliableActivePromptOutbound(message Message) bool {
	status := strings.ToUpper(strings.TrimSpace(message.ProcessingStatus))
	switch status {
	case messageStatusAutomationDraft, messageStatusAutomationReviewed, messageStatusAutomationPending,
		"REVIEW_REQUIRED", "PENDING_REVIEW", "BLOCKED_BY_REVIEW":
		return false
	}
	if strings.Contains(status, "REVIEW_REQUIRED") ||
		strings.Contains(status, "PENDING_REVIEW") ||
		strings.Contains(status, "BLOCKED") {
		return false
	}

	mode := strings.ToUpper(strings.TrimSpace(firstNonEmpty(
		asString(message.Payload["mode"]),
		asString(message.NormalizedPayload["mode"]),
	)))
	if mode == messageStatusAutomationDraft || mode == "AUTOMATION_DRAFT" {
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
	return looksLikePaymentPreferencePrompt(folded) ||
		(strings.Contains(folded, "integral") && strings.Contains(folded, "sinal")) ||
		(strings.Contains(folded, "pagar tudo") && strings.Contains(folded, "entrada"))
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
