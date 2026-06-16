package chat

import "strings"

const safePhaseFallbackTemplateName = "SAFE_PHASE_FALLBACK"

func shouldUseSafePhaseFallback(state CanonicalConversationState, draft BookingDraftContext) bool {
	if draft.IsAdvancedBookingFlow() {
		return true
	}
	switch state.Phase {
	case ConversationPhaseRouteSelection,
		ConversationPhaseTripSelection,
		ConversationPhasePassengerCollection,
		ConversationPhaseBookingPending,
		ConversationPhaseBooked,
		ConversationPhasePaymentPending:
		return true
	default:
		return false
	}
}

func shouldUseSafePhaseFallbackForCurrentTurn(state CanonicalConversationState, draft BookingDraftContext, history []Message) bool {
	return shouldUseSafePhaseFallback(state, draft) || lastAssistantAskedTravelData(history)
}

func shouldApplySafeFallbackAfterDecision(decision IntentDecision) bool {
	switch decision.Intent {
	case IntentUnknown:
		return true
	case IntentBookingCancel, IntentPaymentStatusQuery, IntentPaymentCreate, IntentReschedule, IntentHumanSupport:
		return false
	default:
		return strings.TrimSpace(decision.Action) == "" ||
			strings.EqualFold(strings.TrimSpace(decision.Action), "legacy_tool") ||
			strings.EqualFold(strings.TrimSpace(decision.Action), "safe_fallback")
	}
}

func lastAssistantAskedTravelData(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		folded := strings.Join(strings.Fields(foldChatText(messageTurnText(history[i]))), " ")
		if folded == "" {
			return false
		}
		asksRoute := strings.Contains(folded, "origem") ||
			strings.Contains(folded, "destino") ||
			strings.Contains(folded, "cidade") ||
			strings.Contains(folded, "data")
		return asksRoute && (strings.Contains(folded, "viagem") ||
			strings.Contains(folded, "viajar") ||
			strings.Contains(folded, "consultar") ||
			strings.Contains(folded, "verificar") ||
			strings.Contains(folded, "passagem") ||
			strings.Contains(folded, "maranhao") ||
			strings.Contains(folded, "santa catarina"))
	}
	return false
}

func buildSafeFallbackReplyForPhase(state CanonicalConversationState, draft BookingDraftContext, currentTurn string) (string, bool) {
	phase := state.Phase
	if phase == "" {
		phase = ConversationPhaseDiscovery
	}
	if phase == ConversationPhaseDiscovery && draft.IsAdvancedBookingFlow() {
		if !hasCompleteBookingTrip(draft) {
			phase = ConversationPhaseRouteSelection
		} else if !draft.PassengerCountKnown || !draft.ChildUnder5CountKnown || !draft.HasPassengerDetails {
			phase = ConversationPhasePassengerCollection
		}
	}

	switch phase {
	case ConversationPhaseDiscovery:
		return "Para consultar, preciso da cidade de origem, destino e data da viagem.", true
	case ConversationPhaseRouteSelection:
		return buildSafeRouteSelectionFallback(draft), true
	case ConversationPhaseTripSelection:
		return "Antes de reservar, preciso que voce escolha uma das opcoes que enviei.", true
	case ConversationPhasePassengerCollection:
		return buildSafePassengerCollectionFallback(draft), true
	case ConversationPhaseBookingPending:
		return buildSafeBookingPendingFallback(draft), true
	case ConversationPhaseBooked:
		return askPaymentChoiceReply, true
	case ConversationPhasePaymentPending:
		return "O pagamento ainda esta pendente. Se voce ja pagou, aguarde a confirmacao; se precisar, posso verificar o status da reserva.", true
	case ConversationPhaseHandoffHuman:
		return "", false
	default:
		return "Para continuar, preciso confirmar os dados da viagem.", true
	}
}

func buildSafeRouteSelectionFallback(draft BookingDraftContext) string {
	if strings.TrimSpace(draft.Origin) == "" && strings.TrimSpace(draft.Destination) == "" {
		return "Para consultar, preciso da cidade de origem, destino e data da viagem."
	}
	if strings.TrimSpace(draft.Origin) == "" {
		return "De qual cidade voce vai sair?"
	}
	if strings.TrimSpace(draft.Destination) == "" {
		return "Para qual cidade voce quer ir?"
	}
	if strings.TrimSpace(draft.TripDate) == "" && !draft.HasAvailabilityShown {
		return "Para qual data voce quer viajar?"
	}
	return "Para verificar as opcoes, preciso confirmar origem, destino e data."
}

func buildSafePassengerCollectionFallback(draft BookingDraftContext) string {
	if !hasCompleteBookingTrip(draft) {
		return "Antes de criar a reserva, preciso que voce escolha uma opcao de viagem disponivel."
	}
	action := decideNextBookingStep(draft)
	switch action {
	case BookingNextAskPassengerClarification:
		return buildBookingContinuationReply(draft, action)
	case BookingNextAskPassengerDocuments:
		return buildAskDocumentsReply(draft.PassengerCount, draft.PassengerDetailsCount)
	case BookingNextAskLapChildAssignment:
		return buildAskLapChildAssignmentReply(draft)
	case BookingNextCallCreate:
		return "Recebi os dados. Eles conferem para criar a reserva?"
	default:
		return "Para continuar a reserva, preciso confirmar os dados dos passageiros."
	}
}

func buildSafeBookingPendingFallback(draft BookingDraftContext) string {
	if !hasCompleteBookingTrip(draft) {
		return "Antes de criar a reserva, preciso que voce escolha uma opcao de viagem disponivel."
	}
	if !draft.HasPassengerDetails {
		return buildAskDocumentsReply(firstPositiveInt(draft.PassengerCount, 1), draft.PassengerDetailsCount)
	}
	return "Recebi os dados. Eles conferem para criar a reserva?"
}

func hasCompleteBookingTrip(draft BookingDraftContext) bool {
	return draft.HasAvailabilityShown &&
		strings.TrimSpace(draft.TripID) != "" &&
		strings.TrimSpace(draft.BoardStopID) != "" &&
		strings.TrimSpace(draft.AlightStopID) != "" &&
		strings.TrimSpace(draft.Origin) != "" &&
		strings.TrimSpace(draft.Destination) != "" &&
		strings.TrimSpace(draft.TripDate) != ""
}

func buildSafeFallbackDraftRun(reply string, state CanonicalConversationState, reason string) RunAgentResult {
	reply = strings.TrimSpace(reply)
	requestPayload := map[string]interface{}{
		"mode":          "SAFE_PHASE_FALLBACK",
		"template_name": safePhaseFallbackTemplateName,
		"intent":        string(IntentUnknown),
		"action":        "template",
		"phase":         string(state.Phase),
	}
	if strings.TrimSpace(reason) != "" {
		requestPayload["fallback_reason"] = strings.TrimSpace(reason)
	}
	responsePayload := map[string]interface{}{
		"reply_text":    reply,
		"template_name": safePhaseFallbackTemplateName,
		"intent":        string(IntentUnknown),
		"action":        "template",
		"phase":         string(state.Phase),
	}
	if strings.TrimSpace(reason) != "" {
		responsePayload["fallback_reason"] = strings.TrimSpace(reason)
	}
	return RunAgentResult{
		ReplyText:       reply,
		Model:           "template_realizer",
		RequestPayload:  requestPayload,
		ResponsePayload: responsePayload,
	}
}

func containsOutOfDomainSchedulingVocabulary(text string) bool {
	folded := normalizeGuardrailPhrase(text)
	if folded == "" {
		return false
	}
	for _, phrase := range []string{
		"assento, classe ou horario",
		"por assento, por classe ou por horario",
		"qual passeio/servico",
		"qual passeio",
		"qual servico",
		"agendar horario",
	} {
		if strings.Contains(folded, normalizeGuardrailPhrase(phrase)) {
			return true
		}
	}
	return false
}

func normalizeGuardrailPhrase(text string) string {
	return strings.Join(strings.Fields(foldChatText(text)), " ")
}

func shouldReplaceLoopingDraftWithSafeFallback(history []Message, replyText string, toolCalls []ToolCall, before ConversationPhase, after ConversationPhase) bool {
	if strings.TrimSpace(replyText) == "" || len(toolCalls) > 0 || before != after {
		return false
	}
	latest := latestOutboundMessageText(history)
	if latest == "" {
		return false
	}
	return normalizedTextVerySimilar(latest, replyText)
}

func latestOutboundMessageText(history []Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return strings.Join(strings.Fields(foldChatText(messageTurnText(history[i]))), " ")
	}
	return ""
}

func normalizedTextVerySimilar(left string, right string) bool {
	left = strings.Join(strings.Fields(foldChatText(left)), " ")
	right = strings.Join(strings.Fields(foldChatText(right)), " ")
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	shorter, longer := left, right
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	return len(shorter) >= 24 && strings.Contains(longer, shorter)
}
