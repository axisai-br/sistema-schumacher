package chat

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type ResponseTemplateName string

const (
	TemplateAskPassengerCount        ResponseTemplateName = "ASK_PASSENGER_COUNT"
	TemplateAskChildUnder5           ResponseTemplateName = "ASK_CHILD_UNDER_5"
	TemplateAskDocuments             ResponseTemplateName = "ASK_PASSENGER_DOCUMENTS"
	TemplateAskLapChildAssignment    ResponseTemplateName = "ASK_LAP_CHILD_ASSIGNMENT"
	TemplateAskPaymentChoice         ResponseTemplateName = "ASK_PAYMENT_CHOICE"
	TemplateAskMAOrigin              ResponseTemplateName = "ASK_MA_ORIGIN"
	TemplateAskMADestination         ResponseTemplateName = "ASK_MA_DESTINATION"
	TemplateAskSCOrigin              ResponseTemplateName = "ASK_SC_ORIGIN"
	TemplateAskSCOriginForMA         ResponseTemplateName = "ASK_SC_ORIGIN_FOR_MA"
	TemplateAskReservationRouteSC    ResponseTemplateName = "ASK_RESERVATION_ROUTE_SC"
	TemplatePublicSCTable            ResponseTemplateName = "PUBLIC_SC_TABLE"
	TemplateAvailabilityList         ResponseTemplateName = "AVAILABILITY_LIST"
	TemplateNoAvailability           ResponseTemplateName = "NO_AVAILABILITY"
	TemplateUnsupportedCargo         ResponseTemplateName = "UNSUPPORTED_CARGO"
	TemplateUnsupportedPackage       ResponseTemplateName = "UNSUPPORTED_PACKAGE"
	TemplateHumanHandoff             ResponseTemplateName = "HUMAN_HANDOFF"
	TemplateBookingCreated           ResponseTemplateName = "BOOKING_CREATED"
	TemplateConfirmDocument          ResponseTemplateName = "CONFIRM_EXTRACTED_DOCUMENT"
	TemplatePaymentCreate            ResponseTemplateName = "PAYMENT_CREATE"
	TemplatePaymentMethods           ResponseTemplateName = "PAYMENT_METHODS"
	TemplatePaymentOptionsInfo       ResponseTemplateName = "PAYMENT_OPTIONS_INFO"
	TemplatePayingPassengerInfo      ResponseTemplateName = "PAYING_PASSENGER_INFO"
	TemplateDocumentRequirementsInfo ResponseTemplateName = "DOCUMENT_REQUIREMENTS_INFO"
	TemplateChildPolicyInfo          ResponseTemplateName = "CHILD_POLICY_INFO"
	TemplateBaggageInfo              ResponseTemplateName = "BAGGAGE_INFO"
	TemplateBoardingInfo             ResponseTemplateName = "BOARDING_INFO"
	TemplateHumanSupportInfo         ResponseTemplateName = "HUMAN_SUPPORT_INFO"

	TemplateContextFallbackAvailabilityOption   ResponseTemplateName = "CONTEXT_FALLBACK_AVAILABILITY_OPTION"
	TemplateContextFallbackAvailabilityDate     ResponseTemplateName = "CONTEXT_FALLBACK_AVAILABILITY_DATE"
	TemplateContextFallbackPassengerCount       ResponseTemplateName = "CONTEXT_FALLBACK_PASSENGER_COUNT"
	TemplateContextFallbackChildUnder5          ResponseTemplateName = "CONTEXT_FALLBACK_CHILD_UNDER_5"
	TemplateContextFallbackLapChildAssignment   ResponseTemplateName = "CONTEXT_FALLBACK_LAP_CHILD_ASSIGNMENT"
	TemplateContextFallbackPassengerDocuments   ResponseTemplateName = "CONTEXT_FALLBACK_PASSENGER_DOCUMENTS"
	TemplateContextFallbackDocumentConfirmation ResponseTemplateName = "CONTEXT_FALLBACK_DOCUMENT_CONFIRMATION"
	TemplateContextFallbackPaymentPreference    ResponseTemplateName = "CONTEXT_FALLBACK_PAYMENT_PREFERENCE"
	TemplateContextFallbackPayerCPF             ResponseTemplateName = "CONTEXT_FALLBACK_PAYER_CPF"
)

const (
	askPassengerCountReply        = "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?"
	askChildUnder5Reply           = "Tem crianca de 5 anos ou menos viajando?"
	askReservationRouteSCReply    = "Para fazer a reserva, primeiro preciso saber o trecho da viagem. Me diga de qual cidade você vai sair e para qual cidade de Santa Catarina quer ir."
	askPaymentChoiceReply         = "Perfeito. Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?"
	paymentOptionsInfoReply       = "O pagamento pode ser realizado de 2 formas: você pode pagar agora o valor integral, ou pagar agora apenas o sinal de R$ 250 por passageiro pagante e pagar o restante no embarque."
	payingPassengerInfoReply      = "Passageiro pagante é o passageiro maior de 5 anos."
	documentRequirementsInfoReply = "Para seguir com a reserva, preciso do nome completo e de CPF, RG ou CNH do passageiro. Se preferir, pode enviar uma foto legivel do documento."
	childPolicyInfoReply          = "Crianca de 5 anos ou menos nao entra como passageiro pagante, mas preciso saber se vai alguma crianca nessa idade para registrar corretamente."
	baggageInfoReply              = "No atendimento automatico consigo orientar sobre bagagens comuns do passageiro. Para itens especiais ou algo que nao seja bagagem comum, fale com o suporte: +55 49 9886-2222."
	boardingInfoReply             = "O local e o horario de embarque dependem da opcao de viagem escolhida. Depois que a reserva estiver com a opcao correta, o atendimento confirma esses detalhes."
	humanSupportInfoReply         = "Se precisar falar com o suporte, o contato e +55 49 9886-2222."
	paymentMethodsSupportReply    = "Por aqui consigo seguir apenas com PIX. Para verificar outras formas de pagamento, fale com o suporte: 55 49 99986-2222."
	publicSCTableReply            = "Sim, temos. Segue a tabela de valores para Santa Catarina:\n\nFraiburgo: R$ 950\nMonte Carlo: R$ 950\nVideira: R$ 950\nCampos Novos: R$ 1000\nChapeco: R$ 1100\nConcordia: R$ 1100\nIpumirim: R$ 1100\nPetrolandia: R$ 1100\nItuporanga: R$ 1100\nSeara: R$ 1100\n\nSe quiser, me diga a cidade e a data para eu verificar.\nCaso queira consultar outra cidade, entre em contato com +55 49 9886-2222."
	unsupportedPDFDocumentReply   = "Não consigo ler PDF com segurança por aqui. Por favor, envie uma foto nítida do documento ou escreva o nome completo e CPF/RG do passageiro."
)

func realizeResponseTemplate(name ResponseTemplateName) (string, bool) {
	switch name {
	case TemplateAskPassengerCount:
		return askPassengerCountReply, true
	case TemplateAskChildUnder5:
		return askChildUnder5Reply, true
	case TemplateAskDocuments:
		return buildAskDocumentsReply(1, 0), true
	case TemplateAskLapChildAssignment:
		return "", false
	case TemplateAskPaymentChoice:
		return askPaymentChoiceReply, true
	case TemplateAskMAOrigin:
		return "De qual cidade do Maranhao voce vai sair?", true
	case TemplateAskMADestination:
		return "Para qual cidade do Maranhao voce quer ir?", true
	case TemplateAskSCOrigin:
		return "De qual cidade de Santa Catarina voce vai sair?", true
	case TemplateAskSCOriginForMA:
		return "De qual cidade de Santa Catarina voce vai sair para o Maranhao?", true
	case TemplateAskReservationRouteSC:
		return askReservationRouteSCReply, true
	case TemplatePublicSCTable:
		return publicSCTableReply, true
	case TemplateUnsupportedCargo:
		return unsupportedCargoReply, true
	case TemplateUnsupportedPackage:
		return buildUnsupportedPackageReply(), true
	case TemplateHumanHandoff:
		return "Vou te encaminhar para um atendente continuar por aqui.", true
	case TemplatePaymentMethods:
		return paymentMethodsSupportReply, true
	case TemplatePaymentOptionsInfo:
		return paymentOptionsInfoReply, true
	case TemplatePayingPassengerInfo:
		return payingPassengerInfoReply, true
	case TemplateDocumentRequirementsInfo:
		return documentRequirementsInfoReply, true
	case TemplateChildPolicyInfo:
		return childPolicyInfoReply, true
	case TemplateBaggageInfo:
		return baggageInfoReply, true
	case TemplateBoardingInfo:
		return boardingInfoReply, true
	case TemplateHumanSupportInfo:
		return humanSupportInfoReply, true
	case TemplateContextFallbackAvailabilityOption:
		return "Não consegui identificar qual opção você escolheu. Responda com o número da opção, por exemplo: 1.", true
	case TemplateContextFallbackAvailabilityDate:
		return "Não consegui identificar a data. Me envie a data no formato dia/mês, por exemplo: 06/07.", true
	case TemplateContextFallbackPassengerCount:
		return "A passagem é só para você ou vai mais alguém junto? Também preciso saber se tem criança de 5 anos ou menos.", true
	case TemplateContextFallbackChildUnder5:
		return "Tem criança de 5 anos ou menos viajando? Responda sim ou não.", true
	case TemplateContextFallbackLapChildAssignment:
		return "Me diga qual passageiro é a criança de 5 anos ou menos usando o número da lista.", true
	case TemplateContextFallbackPassengerDocuments:
		return "Ainda preciso do nome completo e CPF, RG ou CNH do passageiro. Se preferir, envie uma foto legível do documento.", true
	case TemplateContextFallbackDocumentConfirmation:
		return "Esses dados conferem? Responda sim para prosseguir ou envie a correção.", true
	case TemplateContextFallbackPaymentPreference:
		return "O pagamento é por PIX. Você prefere pagar o valor integral ou apenas o sinal?", true
	case TemplateContextFallbackPayerCPF:
		return "Para gerar o PIX, preciso do CPF do pagador. Envie somente os 11 números do CPF.", true
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

	if len(decision.TemplateData) > 0 {
		requestPayload["template_data"] = cloneMap(decision.TemplateData)
		responsePayload["template_data"] = cloneMap(decision.TemplateData)
	}

	if decision.AvailabilityInput != nil {
		input := map[string]interface{}{
			"origin":       strings.TrimSpace(decision.AvailabilityInput.Origin),
			"destination":  strings.TrimSpace(decision.AvailabilityInput.Destination),
			"package_name": strings.TrimSpace(decision.AvailabilityInput.PackageName),
			"qtd":          decision.AvailabilityInput.Qty,
			"limit":        decision.AvailabilityInput.Limit,
		}
		if decision.AvailabilityInput.TripDate != nil {
			input["trip_date"] = decision.AvailabilityInput.TripDate.UTC().Format("2006-01-02")
		}
		if decision.AvailabilityInput.DateFrom != nil {
			input["date_from"] = decision.AvailabilityInput.DateFrom.UTC().Format("2006-01-02")
		}
		if decision.AvailabilityInput.DateTo != nil {
			input["date_to"] = decision.AvailabilityInput.DateTo.UTC().Format("2006-01-02")
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
		reply = buildNoAvailabilityReply(availability)
	}
	decision.TemplateName = templateName
	decision.Action = "tool_template"
	run := buildTemplateDraftRunFromDecision(decision, reply)
	run.ResponsePayload["result_count"] = len(availability.Results)
	return run
}

func buildAvailabilityListReply(result AvailabilitySearchResult) string {
	options := futureAvailabilityOptions(result.Results, time.Now())
	if len(options) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Encontrei estas opcoes:\n")
	limit := len(options)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		item := options[i]
		builder.WriteString(formatAvailabilityOptionLine(i+1, item))
		if i < limit-1 {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n\nQual opcao voce prefere?")
	return builder.String()
}

func futureAvailabilityOptions(items []AvailabilitySearchItem, now time.Time) []AvailabilitySearchItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]AvailabilitySearchItem, 0, len(items))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, item := range items {
		dateText := strings.TrimSpace(item.TripDate)
		if dateText == "" {
			out = append(out, item)
			continue
		}
		parsed, err := time.Parse("2006-01-02", dateText)
		if err != nil || !parsed.Before(today) {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return items
	}
	return out
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

func buildNoAvailabilityReply(result AvailabilitySearchResult) string {
	parts := []string{"Nao encontrei disponibilidade"}
	route := formatRouteText(result.Filter.Origin, result.Filter.Destination)
	if route != "" {
		parts = append(parts, "para "+route)
	}
	if packageName := strings.TrimSpace(result.Filter.PackageName); packageName != "" {
		parts = append(parts, "no "+packageName)
	}
	if result.Filter.TripDate != nil {
		parts = append(parts, "em "+result.Filter.TripDate.Format("02/01/2006"))
	} else if result.Filter.DateFrom != nil && result.Filter.DateTo != nil {
		parts = append(parts, "entre "+result.Filter.DateFrom.Format("02/01/2006")+" e "+result.Filter.DateTo.Format("02/01/2006"))
	}
	return strings.Join(parts, " ") + "."
}

func formatRouteText(origin string, destination string) string {
	origin = strings.TrimSpace(origin)
	destination = strings.TrimSpace(destination)
	switch {
	case origin != "" && destination != "":
		return origin + " para " + destination
	case origin != "":
		return "saindo de " + origin
	case destination != "":
		return destination
	default:
		return ""
	}
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
	if decision.TemplateName == TemplateAvailabilityList || decision.TemplateName == TemplateNoAvailability || decision.TemplateName == TemplateBookingCreated {
		return false
	}
	if decision.Intent == IntentSelectAvailabilityOption {
		return decision.SelectedOptionIndex > 0 && hasCanonicalSelectedTrip(state, decision.SelectedOptionIndex)
	}
	if decision.TemplateName == TemplateAskPaymentChoice {
		return hasBookingCreatedFacts(state)
	}
	return true
}

func isContextualFallbackTemplate(name ResponseTemplateName) bool {
	switch name {
	case TemplateContextFallbackAvailabilityOption,
		TemplateContextFallbackAvailabilityDate,
		TemplateContextFallbackPassengerCount,
		TemplateContextFallbackChildUnder5,
		TemplateContextFallbackLapChildAssignment,
		TemplateContextFallbackPassengerDocuments,
		TemplateContextFallbackDocumentConfirmation,
		TemplateContextFallbackPaymentPreference,
		TemplateContextFallbackPayerCPF:
		return true
	default:
		return false
	}
}

func isInformationalTemplate(name ResponseTemplateName) bool {
	switch name {
	case TemplatePaymentOptionsInfo,
		TemplatePayingPassengerInfo,
		TemplateDocumentRequirementsInfo,
		TemplateChildPolicyInfo,
		TemplateBaggageInfo,
		TemplateBoardingInfo,
		TemplateHumanSupportInfo:
		return true
	default:
		return false
	}
}

func canRealizeAvailabilityToolDecisionWithoutLLM(decision IntentDecision, context agentToolContext) bool {
	if decision.Intent != IntentAvailabilitySearch || decision.Action != "tool" || context.Availability == nil {
		return false
	}
	switch strings.TrimSpace(decision.Source) {
	case "deterministic_availability_date_selection", "deterministic_ma_destination_followup", "deterministic_origin_followup", "deterministic_verify_all_options":
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
	if isContextualFallbackTemplate(decision.TemplateName) || isInformationalTemplate(decision.TemplateName) {
		return state
	}
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
		if decision.AvailabilityInput != nil {
			if destination := cleanStateCityName(decision.AvailabilityInput.Destination); destination != "" {
				return fmt.Sprintf("Perfeito — %s/SC. De qual cidade do Maranhao voce vai sair?", destination), true
			}
		}
		return "De qual cidade do Maranhao voce vai sair?", true

	case TemplateAskSCOrigin:
		return "De qual cidade de Santa Catarina voce vai sair?", true
	case TemplateAskMADestination:
		if decision.AvailabilityInput != nil {
			if origin := cleanStateCityName(decision.AvailabilityInput.Origin); origin != "" {
				return fmt.Sprintf("Perfeito — %s/SC. Para qual cidade do Maranhao voce quer ir?", origin), true
			}
		}
		return realizeResponseTemplate(decision.TemplateName)

	default:
		reply, ok := realizeResponseTemplate(decision.TemplateName)
		if !ok {
			return "", false
		}
		return appendOutOfTurnPendingPromptReminder(reply, decision), true
	}
}

func appendOutOfTurnPendingPromptReminder(reply string, decision IntentDecision) string {
	reply = strings.TrimSpace(reply)
	if reply == "" || !templateDataBool(decision.TemplateData, outOfTurnTemplateDataKey) {
		return reply
	}

	pendingTemplate := ResponseTemplateName(strings.TrimSpace(asString(decision.TemplateData[outOfTurnPendingPromptTemplateDataKey])))
	if pendingTemplate == "" {
		return reply
	}
	pendingReply, ok := realizeResponseTemplate(pendingTemplate)
	if !ok || strings.TrimSpace(pendingReply) == "" {
		return reply
	}
	return reply + "\n\nPara continuar: " + trimTrailingSentencePunctuation(pendingReply)
}

func templateDataBool(data map[string]interface{}, key string) bool {
	value, ok := data[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func trimTrailingSentencePunctuation(text string) string {
	text = strings.TrimSpace(text)
	for strings.HasSuffix(text, ".") || strings.HasSuffix(text, "?") || strings.HasSuffix(text, "!") {
		text = strings.TrimSpace(text[:len(text)-1])
	}
	return text
}

func cleanStateCityName(value string) string {
	value = strings.TrimSpace(value)
	for _, suffix := range []string{"/SC", "/MA", "- SC", "- MA", " SC", " MA"} {
		value = strings.TrimSuffix(value, suffix)
	}
	return strings.TrimSpace(value)
}

func buildAskDocumentsReply(expectedPassengerCount int, collectedDocumentCount int) string {
	missing := expectedPassengerCount - collectedDocumentCount
	if missing <= 0 {
		missing = expectedPassengerCount
	}
	if missing <= 1 {
		return "Ainda falta o documento de 1 passageiro. Pode enviar o nome completo e CPF, RG ou CNH completo do passageiro faltante. Se preferir, pode mandar foto legivel do documento."
	}
	return fmt.Sprintf("Perfeito. Agora pode enviar os nomes completos e os documentos dos %d passageiros faltantes (CPF, RG ou CNH completos). Se preferir, pode mandar fotos legiveis dos documentos.", missing)
}

func buildConfirmExtractedDocumentReply(result DocumentExtractResult) string {
	if len(result.Passengers) == 0 {
		if strings.EqualFold(strings.TrimSpace(result.FailureReason), "unsupported_pdf") {
			return unsupportedPDFDocumentReply
		}
		return "Nao consegui ler o documento com seguranca. Pode reenviar uma foto mais perto e com boa luz? Se preferir, pode digitar nome completo e CPF, RG ou CNH completo."
	}
	if strings.EqualFold(strings.TrimSpace(result.Mode), "PARTIAL") {
		lines := []string{"Consegui ler parte do documento, mas preciso confirmar antes de seguir:"}
		confirmable := documentExtractPartialHasConfirmableResultData(result, result.ExpectedPassengerCount)
		for index, passenger := range result.Passengers {
			displayPassenger := passenger
			if confirmable {
				displayPassenger = normalizeDocumentExtractPassengerForBookingConfirmation(passenger)
			}
			name := strings.TrimSpace(displayPassenger.Name)
			docType := strings.TrimSpace(displayPassenger.DocumentType)
			document := strings.TrimSpace(displayPassenger.Document)
			if name == "" {
				name = "nao identificado"
			}
			if docType == "" {
				docType = "documento"
			}
			if document == "" {
				document = "numero nao identificado"
			}
			lines = append(lines, fmt.Sprintf("%d. Nome: %s\n   Documento lido: %s %s%s", index+1, name, docType, maskDocumentForDisplay(document, docType), formatPassengerAdditionalIdentityForConfirmation(displayPassenger)))
		}
		if !confirmable {
			lines = append(lines, "Envie o CPF, RG ou CNH completo do passageiro para eu seguir com a reserva.")
			return strings.Join(lines, "\n")
		}
		lines = append(lines, "Confirme se os dados lidos estao corretos ou envie o documento correto para eu seguir com a reserva.")
		return strings.Join(lines, "\n")
	}
	lines := []string{"Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?"}
	for index, passenger := range result.Passengers {
		name := strings.TrimSpace(passenger.Name)
		docType := strings.TrimSpace(passenger.DocumentType)
		document := strings.TrimSpace(passenger.Document)
		if name == "" {
			name = "nome nao identificado"
		}
		if docType == "" {
			docType = "documento"
		}
		if document == "" {
			document = "numero nao identificado"
		}
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s%s%s", index+1, name, docType, maskDocumentForDisplay(document, docType), formatPassengerAdditionalIdentityForConfirmation(passenger), formatPassengerLapChildForConfirmation(passenger)))
	}
	if missing := result.ExpectedPassengerCount - len(result.Passengers); missing > 0 {
		lines = append(lines, buildAskDocumentsReply(missing, 0))
	}
	return strings.Join(lines, "\n")
}

func formatPassengerLapChildForConfirmation(passenger DocumentExtractPassenger) string {
	if !passenger.IsLapChild {
		return ""
	}
	return " | crianca de ate 5 anos"
}

func formatPassengerAdditionalIdentityForConfirmation(passenger DocumentExtractPassenger) string {
	primaryType := normalizePassengerDocumentType(passenger.DocumentType)
	primaryDocument := normalizePassengerDocumentValue(passenger.Document, primaryType)
	details := make([]string, 0, 6)
	for _, item := range []struct {
		label        string
		documentType string
		document     string
	}{
		{label: "CPF", documentType: "CPF", document: passenger.CPF},
		{label: "RG", documentType: "RG", document: passenger.RG},
		{label: "CNH", documentType: "CNH", document: passenger.CNH},
	} {
		if primaryType == "CPF" && item.documentType != "CPF" {
			continue
		}
		document := normalizePassengerDocumentValue(item.document, item.documentType)
		if document == "" || (primaryType == item.documentType && primaryDocument == document) {
			continue
		}
		details = append(details, item.label+" "+maskDocumentForDisplay(document, item.documentType))
	}
	if birthDate := strings.TrimSpace(passenger.BirthDate); birthDate != "" {
		details = append(details, "nascimento "+formatBirthDateForDisplay(birthDate))
	}
	if birthCertificate := normalizePassengerDocumentValue(passenger.BirthCertificateNumber, "CERTIDAO_NASCIMENTO"); birthCertificate != "" {
		if primaryType != "CERTIDAO_NASCIMENTO" || primaryDocument != birthCertificate {
			details = append(details, "certidao "+maskDocumentForDisplay(birthCertificate, "CERTIDAO_NASCIMENTO"))
		}
	}
	if birthCity := normalizePassengerBirthCity(passenger.BirthCity); birthCity != "" {
		details = append(details, "naturalidade "+birthCity)
	}
	if len(details) == 0 {
		return ""
	}
	return " | " + strings.Join(details, " | ")
}

func formatBirthDateForDisplay(value string) string {
	parsed, ok := parseFlexibleDate(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return parsed.Format("02-01-2006")
}

func buildBookingCreatedReply(result BookingCreateResult) string {
	if strings.TrimSpace(result.BookingID) == "" && strings.TrimSpace(result.ReservationCode) == "" && strings.TrimSpace(result.Status) == "" {
		return ""
	}
	return askPaymentChoiceReply
}

func buildBookingCreatedDraftRun(result BookingCreateResult) RunAgentResult {
	reply := buildBookingCreatedReply(result)
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":          "TEMPLATE_FIRST_REPLY",
			"template_name": string(TemplateBookingCreated),
			"tool_name":     toolNameBookingCreate,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"template_name": string(TemplateBookingCreated),
			"tool_name":     toolNameBookingCreate,
		},
	}
}

func hasBookingCreatedFacts(state CanonicalConversationState) bool {
	if strings.TrimSpace(state.Booking.BookingID) != "" || strings.TrimSpace(state.Booking.ReservationCode) != "" {
		return true
	}
	return len(asMap(state.LastToolFacts[toolNameBookingCreate])) > 0
}

func buildPaymentCreateDraftRun(result PaymentCreateResult) RunAgentResult {
	reply := buildPaymentCreateReply(result)

	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":          "TEMPLATE_FIRST_REPLY",
			"template_name": string(TemplatePaymentCreate),
			"tool_name":     toolNamePaymentCreate,
			"payment_type":  strings.TrimSpace(result.PaymentType),
			"amount_due":    result.AmountDue,
			"mode_result":   strings.TrimSpace(result.Mode),
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"template_name": string(TemplatePaymentCreate),
			"tool_name":     toolNamePaymentCreate,
			"payment_type":  strings.TrimSpace(result.PaymentType),
			"amount_due":    result.AmountDue,
			"mode_result":   strings.TrimSpace(result.Mode),
		},
	}
}

func buildPaymentCreateReply(result PaymentCreateResult) string {
	mode := strings.TrimSpace(result.Mode)

	if mode == "pix_sent" && strings.TrimSpace(result.PixCode) != "" {
		return strings.TrimSpace(result.PixCode)
	}

	switch mode {
	case "manual_review_required_missing_payer_document":
		return "Para gerar o PIX, preciso do CPF do pagador."

	case "manual_review_required_missing_payer_phone":
		return "Para gerar o PIX, preciso confirmar um telefone do pagador com DDD."

	case "manual_review_required_booking_not_found":
		return "Nao consegui localizar a reserva para gerar o PIX. Vou deixar para o atendimento verificar."

	case "manual_review_required_booking_ineligible":
		return "Essa reserva nao pode receber cobranca automaticamente. Vou deixar para o atendimento verificar."

	case "manual_review_required_nothing_due":
		return "Nao encontrei valor pendente para gerar um novo PIX dessa reserva."

	case "manual_review_required_provider_error":
		return "A cobranca foi criada, mas o codigo PIX nao voltou corretamente. Vou deixar para o atendimento verificar."
	}

	if len(result.Errors) > 0 {
		return strings.Join(result.Errors, " ")
	}

	return "Nao consegui gerar o PIX com seguranca. Vou deixar para o atendimento verificar."
}
