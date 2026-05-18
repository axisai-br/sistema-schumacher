package chat

import "strings"

type BookingNextAction string

const (
	BookingNextCallCreate                  BookingNextAction = "call_create"
	BookingNextAskPassengerClarification   BookingNextAction = "ask_passenger_clarification"
	BookingNextAskPassengerDocuments       BookingNextAction = "ask_passenger_documents"
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
	HasPassengerDetails         bool
	PassengerDetailsCount       int
	PassengerDetailsText        string
	HasAvailabilityShown        bool
	AskedPassengerQuestion      bool
	PassengerCountContextActive bool
	RequestedPassengerDocuments bool
	BookingCreated              bool
	AskedPaymentPreference      bool
}

func (c BookingDraftContext) IsAdvancedBookingFlow() bool {
	return c.HasAvailabilityShown ||
		c.AskedPassengerQuestion ||
		c.RequestedPassengerDocuments ||
		c.BookingCreated ||
		strings.TrimSpace(c.TripID) != ""
}

func collectBookingDraftContext(session Session, history []Message, currentTurn string) BookingDraftContext {
	context := BookingDraftContext{
		SelectedOptionIndex:         findLatestSelectedOptionIndex(history),
		PassengerCountContextActive: lastBotAskedPassengerCount(history),
	}

	context = mergePassengerClarificationSlotsIntoBookingDraft(context, parsePassengerClarificationSlots(currentTurn))

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
			context = mergePassengerClarificationSlotsIntoBookingDraft(context, parsePassengerClarificationSlots(body))
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
	if len(passengers) > 0 {
		context.HasPassengerDetails = true
		context.PassengerDetailsCount = len(passengers)
		context.PassengerDetailsText = passengerDetailsText
	}

	if context.PassengerCount == 0 && context.RequestedPassengerDocuments {
		context.PassengerCount = inferExpectedPassengerCount(history, currentTurn, passengerDetailsText)
		context.PassengerCountKnown = context.PassengerCount > 0
	}

	return context
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
		return BookingNextCallCreate
	}
	if !context.HasPassengerDetails {
		return BookingNextAskPassengerDocuments
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
	case BookingNextAskPassengerDocuments:
		return buildAskDocumentsReply(context.PassengerCount, context.PassengerDetailsCount)
	case BookingNextAskBookingPaymentPreference:
		return "Perfeito. Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?"
	default:
		return ""
	}
}

func buildBookingContinuationDraftRun(reply string, action BookingNextAction, context BookingDraftContext) RunAgentResult {
	reply = strings.TrimSpace(reply)
	templateName := bookingContinuationTemplateName(action, context)
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":                  "TEMPLATE_FIRST_REPLY",
			"intent":                string(IntentPassengerCountReply),
			"action":                string(action),
			"template_name":         string(templateName),
			"passenger_count":       context.PassengerCount,
			"child_under_5_count":   context.ChildUnder5Count,
			"passenger_count_known": context.PassengerCountKnown,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"template_name": string(templateName),
			"intent":        string(IntentPassengerCountReply),
			"action":        string(action),
		},
	}
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
	case BookingNextAskBookingPaymentPreference:
		return TemplateAskPaymentChoice
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
