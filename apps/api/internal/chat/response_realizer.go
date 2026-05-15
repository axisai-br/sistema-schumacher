package chat

import (
	"fmt"
	"math"
	"strings"
)

type ResponseTemplateName string

const (
	TemplateAskPassengerCount ResponseTemplateName = "ASK_PASSENGER_COUNT"
	TemplateAskChildUnder5    ResponseTemplateName = "ASK_CHILD_UNDER_5"
	TemplateAskDocuments      ResponseTemplateName = "ASK_PASSENGER_DOCUMENTS"
	TemplateAskPaymentChoice  ResponseTemplateName = "ASK_PAYMENT_CHOICE"
	TemplateAskMAOrigin       ResponseTemplateName = "ASK_MA_ORIGIN"
	TemplateAskMADestination  ResponseTemplateName = "ASK_MA_DESTINATION"
	TemplateAskSCOrigin       ResponseTemplateName = "ASK_SC_ORIGIN"
	TemplateAskSCOriginForMA  ResponseTemplateName = "ASK_SC_ORIGIN_FOR_MA"
	TemplatePublicSCTable     ResponseTemplateName = "PUBLIC_SC_TABLE"
	TemplateAvailabilityList  ResponseTemplateName = "AVAILABILITY_LIST"
	TemplateNoAvailability    ResponseTemplateName = "NO_AVAILABILITY"
	TemplateUnsupportedCargo  ResponseTemplateName = "UNSUPPORTED_CARGO"
)

const askPassengerCountReply = "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?"
const askChildUnder5Reply = "Tem crianca de 5 anos ou menos viajando?"
const askPaymentChoiceReply = "Perfeito. Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?"
const publicSCTableReply = "Sim, temos. Segue a tabela de valores para Santa Catarina:\n\nFraiburgo: R$ 950\nMonte Carlo: R$ 950\nVideira: R$ 950\nCampos Novos: R$ 1000\nChapeco: R$ 1100\nConcordia: R$ 1100\nIpumirim: R$ 1100\nPetrolandia: R$ 1100\nItuporanga: R$ 1100\nSeara: R$ 1100\n\nSe quiser, me diga a cidade e a data para eu verificar disponibilidade.\nCaso queira verificar disponibilidade de outra cidade, entre em contato com +55 49 9886-2222."

func realizeResponseTemplate(name ResponseTemplateName) (string, bool) {
	switch name {
	case TemplateAskPassengerCount:
		return askPassengerCountReply, true
	case TemplateAskChildUnder5:
		return askChildUnder5Reply, true
	case TemplateAskMAOrigin:
		return "De qual cidade do Maranhao voce vai sair?", true
	case TemplateAskMADestination:
		return "Para qual cidade do Maranhao voce quer ir?", true
	case TemplateAskSCOrigin:
		return "De qual cidade de Santa Catarina voce vai sair?", true
	case TemplateAskSCOriginForMA:
		return "Sim, temos atendimento para o Maranhao. Para eu te passar o valor correto, me diga de qual cidade de Santa Catarina voce pretende sair.", true
	case TemplatePublicSCTable:
		return publicSCTableReply, true
	case TemplateNoAvailability:
		return "Nao encontrei disponibilidade com esses dados. Voce quer tentar outra data?", true
	case TemplateUnsupportedCargo:
		return unsupportedCargoReply, true
	default:
		return "", false
	}
}

func buildTemplateDraftRunFromDecision(decision IntentDecision, reply string) RunAgentResult {
	reply = strings.TrimSpace(reply)

	requestPayload := map[string]interface{}{
		"mode":          "TEMPLATE_FIRST_REPLY",
		"template_name": string(decision.TemplateName),
		"intent":        string(decision.Intent),
		"action":        decision.Action,
	}

	responsePayload := map[string]interface{}{
		"reply_text":    reply,
		"template_name": string(decision.TemplateName),
		"intent":        string(decision.Intent),
		"action":        decision.Action,
	}

	if decision.AvailabilityInput != nil {
		input := map[string]interface{}{
			"origin":       strings.TrimSpace(decision.AvailabilityInput.Origin),
			"destination":  strings.TrimSpace(decision.AvailabilityInput.Destination),
			"package_name": strings.TrimSpace(decision.AvailabilityInput.PackageName),
			"qtd":          decision.AvailabilityInput.Qty,
			"limit":        decision.AvailabilityInput.Limit,
		}
		requestPayload["pending_availability_input"] = input
		responsePayload["pending_availability_input"] = input
	}

	return RunAgentResult{
		ReplyText:       reply,
		Model:           "template_realizer",
		RequestPayload:  requestPayload,
		ResponsePayload: responsePayload,
	}
}

func buildAvailabilityTemplateDraftRun(decision IntentDecision, availability AvailabilitySearchResult) RunAgentResult {
	templateName := TemplateAvailabilityList
	reply := buildAvailabilityListReply(availability)
	if strings.TrimSpace(reply) == "" {
		templateName = TemplateNoAvailability
		reply, _ = realizeResponseTemplate(TemplateNoAvailability)
	}
	decision.TemplateName = templateName
	decision.Action = "tool_template"
	run := buildTemplateDraftRunFromDecision(decision, reply)
	run.ResponsePayload["result_count"] = len(availability.Results)
	return run
}

func buildAvailabilityListReply(result AvailabilitySearchResult) string {
	if len(result.Results) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Encontrei estas opcoes:\n")
	limit := len(result.Results)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		item := result.Results[i]
		builder.WriteString(formatAvailabilityOptionLine(i+1, item))
		if i < limit-1 {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n\nQual opcao voce prefere?")
	return builder.String()
}

func formatAvailabilityOptionLine(index int, item AvailabilitySearchItem) string {
	origin := strings.TrimSpace(item.OriginDisplayName)
	destination := strings.TrimSpace(item.DestinationDisplayName)
	date := strings.TrimSpace(item.TripDate)
	departureTime := strings.TrimSpace(item.OriginDepartTime)
	price := formatTemplatePrice(item.Price)
	route := strings.TrimSpace(origin + " para " + destination)
	if route == "para" {
		route = strings.TrimSpace(firstNonEmpty(origin, destination))
	}
	details := make([]string, 0, 4)
	if date != "" {
		details = append(details, date)
	}
	if departureTime != "" {
		details = append(details, "saida "+departureTime)
	}
	if price != "" {
		details = append(details, price)
	}
	if route != "" && len(details) > 0 {
		return fmt.Sprintf("%d. %s, %s", index, route, strings.Join(details, ", "))
	}
	if route != "" {
		return fmt.Sprintf("%d. %s", index, route)
	}
	return fmt.Sprintf("%d. %s", index, strings.Join(details, ", "))
}

func formatTemplatePrice(value float64) string {
	if value <= 0 {
		return ""
	}
	if math.Mod(value, 1) == 0 {
		return fmt.Sprintf("R$ %.0f", value)
	}
	return fmt.Sprintf("R$ %.2f", value)
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

func canRealizeAvailabilityToolDecisionWithoutLLM(decision IntentDecision, context agentToolContext) bool {
	if decision.Intent != IntentAvailabilitySearch || decision.Action != "tool" || context.Availability == nil {
		return false
	}
	switch strings.TrimSpace(decision.Source) {
	case "deterministic_ma_destination_followup":
		return true
	default:
		return false
	}
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
	if decision.AvailabilityInput != nil {
		state.Route.Origin = firstNonEmpty(state.Route.Origin, strings.TrimSpace(decision.AvailabilityInput.Origin))
		state.Route.Destination = firstNonEmpty(state.Route.Destination, strings.TrimSpace(decision.AvailabilityInput.Destination))
		state.Route.PackageName = firstNonEmpty(state.Route.PackageName, strings.TrimSpace(decision.AvailabilityInput.PackageName))
	}
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
	} else if hasCanonicalRoute(state) {
		state.Phase = ConversationPhaseRouteSelection
		state.AllowedNextActions = allowedNextActionsForPhase(state.Phase)
	}
	return state
}

func realizeIntentResponseTemplate(decision IntentDecision) (string, bool) {
	switch decision.TemplateName {
	case TemplateAskMAOrigin:
		return "De qual cidade do Maranhao voce vai sair?", true

	case TemplateAskSCOrigin:
		return "De qual cidade de Santa Catarina voce vai sair?", true

	default:
		return realizeResponseTemplate(decision.TemplateName)
	}
}
