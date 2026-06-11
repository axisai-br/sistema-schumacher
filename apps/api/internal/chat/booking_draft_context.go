package chat

import (
	"fmt"
	"strings"
)

type BookingNextAction string

const (
	BookingNextCallCreate                  BookingNextAction = "call_create"
	BookingNextAwaitTripSelection          BookingNextAction = "await_trip_selection"
	BookingNextAskPassengerClarification   BookingNextAction = "ask_passenger_clarification"
	BookingNextAskPassengerDocuments       BookingNextAction = "ask_passenger_documents"
	BookingNextAskLapChildAssignment       BookingNextAction = "ask_lap_child_assignment"
	BookingNextAskBookingPaymentPreference BookingNextAction = "ask_booking_payment_preference"
)

type BookingDraftContext struct {
	Origin                      string
	Destination                 string
	SelectedOptionIndex         int
	TripID                      string
	BoardStopID                 string
	AlightStopID                string
	TripDate                    string
	DepartureTime               string
	Price                       float64
	Currency                    string
	PassengerCount              int
	ChildUnder5Count            int
	PassengerCountKnown         bool
	ChildUnder5CountKnown       bool
	LapChildAssignmentKnown     bool
	LapChildPassengerIndexes    []int
	NeedsLapChildAssignment     bool
	HasPassengerDetails         bool
	PassengerDetailsCount       int
	PassengerDetailsText        string
	PassengerDetails            []BookingCreatePassengerInput
	HasAvailabilityShown        bool
	AskedPassengerQuestion      bool
	PassengerCountContextActive bool
	RequestedPassengerDocuments bool
	BookingCreated              bool
	AskedPaymentPreference      bool
}

func (c BookingDraftContext) IsAdvancedBookingFlow() bool {
	return c.HasAvailabilityShown ||
		c.RequestedPassengerDocuments ||
		c.BookingCreated ||
		c.HasPassengerDetails ||
		strings.TrimSpace(c.TripID) != ""
}

func collectBookingDraftContext(session Session, history []Message, currentTurn string) BookingDraftContext {
	context := BookingDraftContext{
		SelectedOptionIndex:         findLatestSelectedOptionIndex(history),
		PassengerCountContextActive: lastBotAskedPassengerCount(history),
	}

	currentSlots := parsePassengerClarificationSlots(currentTurn)
	if !currentSlots.ChildUnder5CountKnown && isShortYesReply(currentTurn) && lastAssistantAskedChildUnder5(history) {
		currentSlots.ChildUnder5Count = 1
		currentSlots.ChildUnder5CountKnown = true
	}
	context = mergePassengerClarificationSlotsIntoBookingDraft(context, currentSlots)

	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		body := messageTurnText(message)
		folded := strings.Join(strings.Fields(foldChatText(body)), " ")

		if strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			if !context.AskedPassengerQuestion && looksLikePassengerCountQuestion(body) {
				context.AskedPassengerQuestion = true
			}
			if !context.RequestedPassengerDocuments && looksLikePassengerDocumentRequest(folded) {
				context.RequestedPassengerDocuments = true
			}
			if !context.AskedPaymentPreference && looksLikePaymentPreferencePrompt(folded) {
				context.AskedPaymentPreference = true
			}
		}

		if strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") &&
			(!context.PassengerCountKnown || !context.ChildUnder5CountKnown) {
			slots := parsePassengerClarificationSlots(body)
			if !slots.ChildUnder5CountKnown && isShortYesReply(body) && previousAssistantAskedChildUnder5(history, i) {
				slots.ChildUnder5Count = 1
				slots.ChildUnder5CountKnown = true
			}
			context = mergePassengerClarificationSlotsIntoBookingDraft(context, slots)
		}

		for _, toolContext := range messageToolContexts(message) {
			if availability := asMap(toolContext[toolNameAvailabilitySearch]); availability != nil {
				context.HasAvailabilityShown = true
				mergeAvailabilityPayloadIntoBookingDraft(&context, availability)
			}
			if booking := asMap(toolContext[toolNameBookingCreate]); booking != nil {
				context.BookingCreated = true
				if context.PassengerCount == 0 {
					context.PassengerCount = readInt(booking["passenger_count"])
					context.PassengerCountKnown = context.PassengerCount > 0
				}
			}
		}
	}

	passengerDetailsText := findLatestPassengerDetailsText(history, session)
	passengers := extractBookingCreatePassengers(passengerDetailsText, session)
	correction, hasCorrection := findLatestPassengerDocumentCorrection(history, currentTurn)
	if len(passengers) == 0 {
		if extract := findLatestDocumentExtractContext(history); extract != nil && (strings.EqualFold(strings.TrimSpace(extract.Mode), "EXTRACTED") || hasCorrection) {
			passengers = bookingPassengersFromDocumentExtract(*extract, session, context.TripDate)
		}
	}
	if hasCorrection {
		passengers = applyPassengerDocumentCorrection(passengers, correction)
	}
	if len(passengers) > 0 {
		context.HasPassengerDetails = true
		context.PassengerDetailsCount = len(passengers)
		context.PassengerDetailsText = passengerDetailsText
		context.PassengerDetails = passengers
		context.LapChildPassengerIndexes = lapChildIndexesFromPassengers(passengers)
		if len(context.LapChildPassengerIndexes) > 0 {
			context.LapChildAssignmentKnown = true
			if !context.ChildUnder5CountKnown {
				context.ChildUnder5Count = len(context.LapChildPassengerIndexes)
				context.ChildUnder5CountKnown = true
			}
		}
	}
	if context.PassengerDetailsCount > 0 &&
		context.PassengerDetailsCount > context.PassengerCount &&
		context.ChildUnder5Count > 0 {
		context.PassengerCount = context.PassengerDetailsCount
		context.PassengerCountKnown = true
	}
	if context.PassengerCount == 0 && context.RequestedPassengerDocuments {
		context.PassengerCount = inferExpectedPassengerCount(history, currentTurn, passengerDetailsText)
		context.PassengerCountKnown = context.PassengerCount > 0
	}
	if context.ChildUnder5Count > 0 && len(passengers) > 0 && !context.LapChildAssignmentKnown {
		if indexes, ok := inferLapChildAssignmentIndexes(history, currentTurn, passengers, context.ChildUnder5Count); ok {
			context.LapChildPassengerIndexes = indexes
			context.LapChildAssignmentKnown = true
		}
	}
	context.NeedsLapChildAssignment = context.ChildUnder5Count > 0 &&
		context.HasPassengerDetails &&
		!context.LapChildAssignmentKnown

	return context
}

func isShortYesReply(text string) bool {
	switch strings.Join(strings.Fields(foldChatText(text)), " ") {
	case "sim", "s", "ss", "positivo", "posi", "tem", "tem sim", "sim tem",
		"sim ela", "sim ele",
		"ela", "ele",
		"ela sim", "ele sim",
		"isso ela", "isso ele",
		"e ela", "eh ela", "é ela",
		"e ele", "eh ele", "é ele":
		return true
	default:
		return false
	}
}

func lastAssistantAskedChildUnder5(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return looksLikeChildUnder5Question(history[i].Body)
	}
	return false
}

func previousAssistantAskedChildUnder5(history []Message, beforeIndex int) bool {
	for i := beforeIndex - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return looksLikeChildUnder5Question(history[i].Body)
	}
	return false
}

func looksLikeChildUnder5Question(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	return strings.Contains(folded, "crianca de ate 5 anos") ||
		strings.Contains(folded, "crianca de 5 anos ou menos") ||
		strings.Contains(folded, "tem ate 5 anos") ||
		strings.Contains(folded, "ate 5 anos viajando")
}

func mergePassengerReplyIntoBookingDraft(context BookingDraftContext, passengerCount int, childUnder5Count int) BookingDraftContext {
	return mergePassengerClarificationSlotsIntoBookingDraft(context, PassengerClarificationSlots{
		PassengerCount:        passengerCount,
		PassengerCountKnown:   passengerCount > 0,
		ChildUnder5Count:      childUnder5Count,
		ChildUnder5CountKnown: childUnder5Count >= 0,
	})
}

func mergePassengerClarificationSlotsIntoBookingDraft(context BookingDraftContext, slots PassengerClarificationSlots) BookingDraftContext {
	if slots.PassengerCountKnown && slots.PassengerCount > 0 {
		context.PassengerCount = slots.PassengerCount
		context.PassengerCountKnown = true
	}
	if slots.ChildUnder5CountKnown {
		context.ChildUnder5Count = slots.ChildUnder5Count
		context.ChildUnder5CountKnown = true
	}
	if slots.PassengerCountKnown || slots.ChildUnder5CountKnown {
		context.AskedPassengerQuestion = true
		context.PassengerCountContextActive = true
	}
	return context
}

func decideNextBookingStep(context BookingDraftContext) BookingNextAction {
	if context.BookingCreated {
		return BookingNextAskBookingPaymentPreference
	}
	if !context.PassengerCountKnown || context.PassengerCount <= 0 {
		return BookingNextAskPassengerClarification
	}
	if !context.ChildUnder5CountKnown {
		return BookingNextAskPassengerClarification
	}
	if !context.HasAvailabilityShown ||
		strings.TrimSpace(context.TripID) == "" ||
		strings.TrimSpace(context.BoardStopID) == "" ||
		strings.TrimSpace(context.AlightStopID) == "" ||
		strings.TrimSpace(context.Origin) == "" ||
		strings.TrimSpace(context.Destination) == "" ||
		strings.TrimSpace(context.TripDate) == "" {
		return BookingNextAwaitTripSelection
	}
	if !context.HasPassengerDetails {
		return BookingNextAskPassengerDocuments
	}
	if context.NeedsLapChildAssignment {
		return BookingNextAskLapChildAssignment
	}
	return BookingNextCallCreate
}

func buildBookingContinuationReply(context BookingDraftContext, action BookingNextAction) string {
	switch action {
	case BookingNextAskPassengerClarification:
		if context.PassengerCountKnown && context.PassengerCount > 0 && !context.ChildUnder5CountKnown {
			return "Tem crianca de 5 anos ou menos viajando?"
		}
		return "Entendi. A passagem e so para voce ou vai mais alguem junto?"
	case BookingNextAwaitTripSelection:
		return "Antes de criar a reserva, preciso que voce escolha uma opcao de viagem disponivel."
	case BookingNextAskPassengerDocuments:
		return buildAskDocumentsReply(context.PassengerCount, context.PassengerDetailsCount)
	case BookingNextAskLapChildAssignment:
		return buildAskLapChildAssignmentReply(context)
	case BookingNextAskBookingPaymentPreference:
		return "Perfeito. Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?"
	default:
		return ""
	}
}

func buildAskLapChildAssignmentReply(context BookingDraftContext) string {
	passengers := context.PassengerDetails
	if len(passengers) == 0 {
		passengers = extractBookingCreatePassengers(context.PassengerDetailsText, Session{})
	}
	if len(passengers) == 0 {
		return "Recebi os dados dos passageiros. Qual deles e a crianca de ate 5 anos?"
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Recebi os dados dos %d passageiros. Qual deles e a crianca de ate 5 anos?", len(passengers)))
	for i, passenger := range passengers {
		name := strings.TrimSpace(passenger.Name)
		if name == "" {
			name = fmt.Sprintf("Passageiro %d", i+1)
		}
		builder.WriteString(fmt.Sprintf("\n%d. %s", i+1, name))
	}
	return builder.String()
}

func buildBookingContinuationDraftRun(reply string, action BookingNextAction, context BookingDraftContext) RunAgentResult {
	reply = strings.TrimSpace(reply)
	templateName := bookingContinuationTemplateName(action, context)
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":                        "TEMPLATE_FIRST_REPLY",
			"intent":                      string(IntentPassengerCountReply),
			"action":                      string(action),
			"template_name":               string(templateName),
			"passenger_count":             context.PassengerCount,
			"child_under_5_count":         context.ChildUnder5Count,
			"passenger_count_known":       context.PassengerCountKnown,
			"lap_child_assignment_known":  context.LapChildAssignmentKnown,
			"lap_child_passenger_indexes": context.LapChildPassengerIndexes,
			"needs_lap_child_assignment":  context.NeedsLapChildAssignment,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"template_name": string(templateName),
			"intent":        string(IntentPassengerCountReply),
			"action":        string(action),
		},
	}
}

func shouldDraftPassengerDocumentConfirmation(session Session, history []Message, currentTurn string, context BookingDraftContext) bool {
	if !context.HasPassengerDetails || len(context.PassengerDetails) == 0 {
		return false
	}
	if looksLikeDocumentConfirmation(currentTurn) {
		return false
	}
	if _, ok := parsePassengerDocumentCorrection(currentTurn); ok {
		return true
	}
	if !context.RequestedPassengerDocuments &&
		!lastAssistantAskedDocumentConfirmation(history) &&
		!lastAssistantAskedBookingProceedConfirmation(history) {
		return false
	}
	return len(extractBookingCreatePassengers(currentTurn, session)) > 0
}

func shouldAskPassengerNameAfterCPF(session Session, currentTurn string, context BookingDraftContext) bool {
	if context.HasPassengerDetails || looksLikeDocumentConfirmation(currentTurn) {
		return false
	}
	if extractValidCPF(currentTurn) == "" {
		return false
	}
	return len(extractBookingCreatePassengers(currentTurn, session)) == 0
}

func buildInvalidPassengerCPFDraftRun(context BookingDraftContext) RunAgentResult {
	reply := "O CPF informado parece invalido. Pode reenviar o CPF correto com 11 digitos ou mandar uma foto legivel do documento?"
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":            "TEMPLATE_FIRST_REPLY",
			"intent":          string(IntentPassengerDocumentsProvided),
			"action":          "ask_valid_passenger_cpf",
			"template_name":   string(TemplateAskDocuments),
			"passenger_count": context.PassengerCount,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":     reply,
			"intent":         string(IntentPassengerDocumentsProvided),
			"action":         "ask_valid_passenger_cpf",
			"template_name":  string(TemplateAskDocuments),
			"needs_document": true,
		},
	}
}

func buildAskPassengerNameAfterCPFDraftRun(context BookingDraftContext) RunAgentResult {
	reply := "Recebi o CPF. Pode enviar tambem o nome completo do passageiro para eu conferir antes de criar a reserva?"
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":            "TEMPLATE_FIRST_REPLY",
			"intent":          string(IntentPassengerDocumentsProvided),
			"action":          "ask_passenger_name_for_cpf",
			"template_name":   string(TemplateAskDocuments),
			"passenger_count": context.PassengerCount,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"intent":        string(IntentPassengerDocumentsProvided),
			"action":        "ask_passenger_name_for_cpf",
			"template_name": string(TemplateAskDocuments),
		},
	}
}

func buildUnsupportedPDFDocumentDraftRun(context BookingDraftContext) RunAgentResult {
	reply := unsupportedPDFDocumentReply
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":            "TEMPLATE_FIRST_REPLY",
			"intent":          string(IntentPassengerDocumentsProvided),
			"action":          "ask_photo_or_typed_document_after_pdf",
			"template_name":   string(TemplateAskDocuments),
			"passenger_count": context.PassengerCount,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"intent":        string(IntentPassengerDocumentsProvided),
			"action":        "ask_photo_or_typed_document_after_pdf",
			"template_name": string(TemplateAskDocuments),
		},
	}
}

func buildPassengerDocumentConfirmationDraftRun(context BookingDraftContext) RunAgentResult {
	result := documentExtractResultFromBookingDraft(context)
	reply := buildConfirmExtractedDocumentReply(result)
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":                     "TEMPLATE_FIRST_REPLY",
			"template_name":            string(TemplateConfirmDocument),
			"intent":                   string(IntentPassengerDocumentsProvided),
			"action":                   "confirm_passenger_documents",
			"passenger_count":          context.PassengerCount,
			"expected_passenger_count": result.ExpectedPassengerCount,
			"passenger_details_count":  len(result.Passengers),
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":               reply,
			"template_name":            string(TemplateConfirmDocument),
			"intent":                   string(IntentPassengerDocumentsProvided),
			"action":                   "confirm_passenger_documents",
			"expected_passenger_count": result.ExpectedPassengerCount,
			"passenger_details_count":  len(result.Passengers),
		},
	}
}

func documentExtractResultFromBookingDraft(context BookingDraftContext) DocumentExtractResult {
	expected := context.PassengerCount
	if expected <= 0 {
		expected = len(context.PassengerDetails)
	}
	passengers := make([]DocumentExtractPassenger, 0, len(context.PassengerDetails))
	for _, passenger := range context.PassengerDetails {
		docType := normalizePassengerDocumentType(passenger.DocumentType)
		document := normalizePassengerDocumentValue(passenger.Document, docType)
		item := DocumentExtractPassenger{
			Name:         strings.TrimSpace(passenger.Name),
			DocumentType: docType,
			Document:     document,
			Confidence:   1,
		}
		switch docType {
		case "CPF":
			item.CPF = document
		case "RG":
			item.RG = document
		case "CNH":
			item.CNH = document
		}
		passengers = append(passengers, item)
	}
	return DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: expected,
		Passengers:             passengers,
	}
}

func shouldTreatAsPassengerDocumentFlow(phase ConversationPhase, history []Message, currentTurn string, session Session) bool {
	if phase == ConversationPhaseBookingPending || phase == ConversationPhasePassengerCollection {
		return true
	}
	if lastAssistantAskedDocumentConfirmation(history) || lastAssistantAskedBookingProceedConfirmation(history) {
		return true
	}
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		folded := strings.Join(strings.Fields(foldChatText(history[i].Body)), " ")
		if looksLikePassengerDocumentRequest(folded) {
			return true
		}
		break
	}
	return looksLikePassengerDocumentText(currentTurn, session)
}

func bookingContinuationTemplateName(action BookingNextAction, context BookingDraftContext) ResponseTemplateName {
	switch action {
	case BookingNextAskPassengerClarification:
		if context.PassengerCountKnown && context.PassengerCount > 0 && !context.ChildUnder5CountKnown {
			return TemplateAskChildUnder5
		}
		return TemplateAskPassengerCount
	case BookingNextAskPassengerDocuments:
		return TemplateAskDocuments
	case BookingNextAskLapChildAssignment:
		return TemplateAskLapChildAssignment
	case BookingNextAskBookingPaymentPreference:
		return TemplateAskPaymentChoice
	case BookingNextAwaitTripSelection:
		return TemplateAvailabilityList
	default:
		return ""
	}
}

func messageToolContexts(message Message) []map[string]interface{} {
	contexts := make([]map[string]interface{}, 0, 2)
	for _, raw := range []interface{}{message.Payload["tool_context"], message.NormalizedPayload["tool_context"]} {
		context := asMap(raw)
		if len(context) == 0 {
			continue
		}
		contexts = append(contexts, context)
	}
	return contexts
}

func mergeAvailabilityPayloadIntoBookingDraft(context *BookingDraftContext, payload map[string]interface{}) {
	if context == nil {
		return
	}

	if origin := strings.TrimSpace(asString(payload["origin"])); origin != "" && context.Origin == "" {
		context.Origin = origin
	}
	if destination := strings.TrimSpace(asString(payload["destination"])); destination != "" && context.Destination == "" {
		context.Destination = destination
	}
	if tripDate := strings.TrimSpace(asString(payload["trip_date"])); tripDate != "" && context.TripDate == "" {
		context.TripDate = tripDate
	}

	results := asInterfaceSliceMaps(payload["results"])
	if len(results) == 0 {
		return
	}

	selectedIndex := context.SelectedOptionIndex - 1
	if selectedIndex < 0 || selectedIndex >= len(results) {
		if len(results) != 1 {
			return
		}
		selectedIndex = 0
	}

	item := results[selectedIndex]
	if context.Origin == "" {
		context.Origin = strings.TrimSpace(asString(item["origin_display_name"]))
	}
	if context.Destination == "" {
		context.Destination = strings.TrimSpace(asString(item["destination_display_name"]))
	}
	if context.TripID == "" {
		context.TripID = strings.TrimSpace(asString(item["trip_id"]))
	}
	if context.BoardStopID == "" {
		context.BoardStopID = strings.TrimSpace(asString(item["board_stop_id"]))
	}
	if context.AlightStopID == "" {
		context.AlightStopID = strings.TrimSpace(asString(item["alight_stop_id"]))
	}
	if context.TripDate == "" {
		context.TripDate = strings.TrimSpace(asString(item["trip_date"]))
	}
	if context.DepartureTime == "" {
		context.DepartureTime = strings.TrimSpace(asString(item["origin_depart_time"]))
	}
	if context.Price == 0 {
		context.Price = asFloat64(item["price"])
	}
	if context.Currency == "" {
		context.Currency = strings.TrimSpace(asString(item["currency"]))
	}
}

func looksLikePassengerDocumentRequest(folded string) bool {
	return strings.Contains(folded, "foto legivel do documento") ||
		(strings.Contains(folded, "nome completo") && strings.Contains(folded, "documento")) ||
		(strings.Contains(folded, "nomes completos") && strings.Contains(folded, "documentos")) ||
		(strings.Contains(folded, "envie") && strings.Contains(folded, "documento")) ||
		(strings.Contains(folded, "pode enviar") && strings.Contains(folded, "documento"))
}

func looksLikePaymentPreferencePrompt(folded string) bool {
	return strings.Contains(folded, "valor integral") ||
		strings.Contains(folded, "apenas o sinal") ||
		strings.Contains(folded, "sinal de r$ 250") ||
		strings.Contains(folded, "prefere pagar")
}
