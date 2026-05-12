package chat

import "strings"

type ResponseTemplateName string

const (
	TemplateAskPassengerCount ResponseTemplateName = "ASK_PASSENGER_COUNT"
	TemplateAskMAOrigin       ResponseTemplateName = "ASK_MA_ORIGIN"
	TemplateAskSCOrigin       ResponseTemplateName = "ASK_SC_ORIGIN"
	TemplateNoAvailability    ResponseTemplateName = "NO_AVAILABILITY"
	TemplateUnsupportedCargo  ResponseTemplateName = "UNSUPPORTED_CARGO"
)

const askPassengerCountReply = "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?"

func realizeResponseTemplate(name ResponseTemplateName) (string, bool) {
	switch name {
	case TemplateAskPassengerCount:
		return askPassengerCountReply, true
	case TemplateAskMAOrigin:
		return "De qual cidade do Maranhao voce vai sair?", true
	case TemplateAskSCOrigin:
		return "De qual cidade de Santa Catarina voce vai sair?", true
	case TemplateNoAvailability:
		return "Nao encontrei disponibilidade com esses dados. Voce quer tentar outra data?", true
	case TemplateUnsupportedCargo:
		return "No atendimento automatico, consigo ajudar apenas com passagens e bagagens comuns do passageiro. Para enviar moto ou qualquer item que nao seja bagagem, fale com o suporte: +55 49 9886-2222.", true
	default:
		return "", false
	}
}

func buildTemplateDraftRun(templateName ResponseTemplateName, reply string) RunAgentResult {
	reply = strings.TrimSpace(reply)
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":          "TEMPLATE_FIRST_REPLY",
			"template_name": string(templateName),
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"template_name": string(templateName),
		},
	}
}

func canRealizeWithoutLLM(decision IntentDecision, state CanonicalConversationState) bool {
	if decision.TemplateName == "" {
		return false
	}
	if decision.Intent == IntentSelectAvailabilityOption {
		return decision.SelectedOptionIndex > 0 && hasCanonicalSelectedTrip(state, decision.SelectedOptionIndex)
	}
	return true
}

func hasCanonicalSelectedTrip(state CanonicalConversationState, selectedIndex int) bool {
	if selectedIndex <= 0 {
		return false
	}
	availability := asMap(state.LastToolFacts[toolNameAvailabilitySearch])
	results := asInterfaceSliceMaps(availability["results"])
	return selectedIndex <= len(results)
}

func applyIntentDecisionToCanonicalState(state CanonicalConversationState, decision IntentDecision) CanonicalConversationState {
	if decision.SelectedOptionIndex > 0 {
		state.Route.SelectedOptionIndex = decision.SelectedOptionIndex
		availability := asMap(state.LastToolFacts[toolNameAvailabilitySearch])
		results := asInterfaceSliceMaps(availability["results"])
		index := decision.SelectedOptionIndex - 1
		if index >= 0 && index < len(results) {
			selected := results[index]
			state.Route.Origin = firstNonEmpty(state.Route.Origin, strings.TrimSpace(asString(selected["origin_display_name"])))
			state.Route.Destination = firstNonEmpty(state.Route.Destination, strings.TrimSpace(asString(selected["destination_display_name"])))
			state.Route.TripDate = firstNonEmpty(state.Route.TripDate, strings.TrimSpace(asString(selected["trip_date"])))
			state.Route.DepartureTime = firstNonEmpty(state.Route.DepartureTime, strings.TrimSpace(asString(selected["origin_depart_time"])))
			state.Route.TripID = firstNonEmpty(state.Route.TripID, strings.TrimSpace(asString(selected["trip_id"])))
			state.Route.BoardStopID = firstNonEmpty(state.Route.BoardStopID, strings.TrimSpace(asString(selected["board_stop_id"])))
			state.Route.AlightStopID = firstNonEmpty(state.Route.AlightStopID, strings.TrimSpace(asString(selected["alight_stop_id"])))
			state.Route.PackageName = firstNonEmpty(state.Route.PackageName, strings.TrimSpace(asString(selected["package_name"])))
		}
		state.Phase = ConversationPhasePassengerCollection
		state.AllowedNextActions = allowedNextActionsForPhase(state.Phase)
	}
	return state
}
