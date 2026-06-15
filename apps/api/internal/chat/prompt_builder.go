package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const defaultAgentSystemPrompt = `
	Voce e o atendente Shabas da Schumacher Tur.
	Responda sempre em Portugues do Brasil, com uma mensagem curta, direta e cordial.
	Faca no maximo uma pergunta.
	Nunca exponha sistemas internos, IDs internos, classificacoes ou raciocinio.
	Nunca invente rota, data, horario, preco, disponibilidade, reserva ou pagamento.
	Use somente o snapshot validado recebido no prompt do usuario.
`

func buildAgentSystemPrompt() string {
	return defaultAgentSystemPrompt
}

func buildAgentUserPrompt(session Session, memory map[string]interface{}, tools agentToolContext) string {
	snapshot := buildLegacyPromptSnapshot(session, memory, tools)
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return `{"task":"write_short_portuguese_reply","error":"prompt_snapshot_unavailable"}`
	}
	return "SNAPSHOT_VALIDADO_JSON\n" + string(payload)
}

func buildLegacyPromptSnapshot(session Session, memory map[string]interface{}, tools agentToolContext) map[string]interface{} {
	// Business routing rules removed from this legacy prompt are owned by:
	// intent_router.go for deterministic intents and package/cargo routing;
	// conversation_state_machine.go for phase transitions and allowed actions;
	// tool_router.go for tool input normalization and validation;
	// response_realizer.go for safe customer-facing templates;
	// agent_contracts.go for strict JSON planner schemas.
	snapshot := map[string]interface{}{
		"task":              "write_one_short_customer_reply_if_no_template_applies",
		"now_utc":           time.Now().UTC().Format(time.RFC3339),
		"current_user_turn": strings.TrimSpace(asString(memory["current_turn_body"])),
		"session": map[string]interface{}{
			"channel":        strings.TrimSpace(session.Channel),
			"customer_name":  strings.TrimSpace(session.CustomerName),
			"customer_phone": strings.TrimSpace(session.CustomerPhone),
		},
	}
	if kinds := asStringSlice(memory["current_turn_kinds"]); len(kinds) > 0 {
		snapshot["current_turn_kinds"] = kinds
	}
	if media := normalizeMediaMemoryItems(memory["current_turn_media"]); len(media) > 0 {
		snapshot["current_turn_media_count"] = len(media)
	}
	if state := compactPromptCanonicalState(memory["canonical_state"]); len(state) > 0 {
		snapshot["canonical_state"] = state
	}
	if routeContext := compactPromptRouteContext(memory); len(routeContext) > 0 {
		snapshot["route_context"] = routeContext
	}
	if toolFacts := compactPromptToolFacts(tools); len(toolFacts) > 0 {
		snapshot["last_validated_tool_facts"] = toolFacts
	}
	if recent := compactPromptRecentMessages(memory["recent_messages"], 4); len(recent) > 0 {
		snapshot["recent_turns"] = recent
	}
	if last := strings.TrimSpace(asString(memory["last_assistant_message"])); last != "" {
		snapshot["last_customer_facing_message"] = last
	}
	if bookingDraft := asMap(memory["booking_draft_context"]); len(bookingDraft) > 0 {
		snapshot["booking_draft_context"] = compactPromptBookingDraftContext(bookingDraft)
	}
	return snapshot
}

func compactPromptRouteContext(memory map[string]interface{}) map[string]interface{} {
	currentTurn := strings.TrimSpace(asString(memory["current_turn_body"]))
	recentMessages := normalizeRecentMemoryMessages(memory["recent_messages"])
	currentTurnMedia := normalizeMediaMemoryItems(memory["current_turn_media"])
	context := derivePromptConversationContext(currentTurn, recentMessages, currentTurnMedia)
	return compactInterfaceFields(map[string]interface{}{
		"origin":                    context.Origin,
		"destination":               context.Destination,
		"package_name":              context.PackageName,
		"route_direction":           context.RouteDirection,
		"short_date_follow_up":      context.ShortDateFollowUp,
		"destination_chosen_now":    context.DestinationChosenNow,
		"date_chosen_for_route":     context.DateChosenForDestination,
		"travel_option_chosen_now":  context.TravelOptionChosenNow,
		"passenger_count_reply":     context.PassengerCountReplyContext,
		"passenger_count":           context.PassengerCount,
		"child_under_5_count":       context.ChildUnder5Count,
		"expected_passenger_count":  context.ExpectedPassengerCount,
		"waiting_for_documents":     context.WaitingForPassengerDocuments,
		"current_turn_media_count":  context.CurrentTurnMediaCount,
		"current_turn_has_document": context.CurrentTurnHasImage,
	})
}

func compactPromptCanonicalState(value interface{}) map[string]interface{} {
	state, ok := value.(CanonicalConversationState)
	if !ok {
		return nil
	}
	snapshot := map[string]interface{}{
		"phase":                string(state.Phase),
		"allowed_next_actions": state.AllowedNextActions,
	}
	if route := compactStringFields(map[string]string{
		"origin":         state.Route.Origin,
		"destination":    state.Route.Destination,
		"package_name":   state.Route.PackageName,
		"trip_date":      state.Route.TripDate,
		"departure_time": state.Route.DepartureTime,
	}); len(route) > 0 {
		if state.Route.SelectedOptionIndex > 0 {
			route["selected_option_index"] = state.Route.SelectedOptionIndex
		}
		snapshot["route"] = route
	}
	passengers := map[string]interface{}{}
	if state.Passengers.ExpectedCount > 0 {
		passengers["expected_count"] = state.Passengers.ExpectedCount
	}
	if state.Passengers.ChildUnder5Count > 0 {
		passengers["child_under_5_count"] = state.Passengers.ChildUnder5Count
	}
	if state.Passengers.DocumentsCollected {
		passengers["documents_collected"] = true
	}
	if len(passengers) > 0 {
		snapshot["passengers"] = passengers
	}
	if booking := compactStringFields(map[string]string{
		"reservation_code": state.Booking.ReservationCode,
		"status":           state.Booking.Status,
	}); len(booking) > 0 {
		snapshot["booking"] = booking
	}
	if payment := compactStringFields(map[string]string{
		"status":     state.Payment.Status,
		"preference": state.Payment.Preference,
	}); len(payment) > 0 {
		snapshot["payment"] = payment
	}
	if len(state.LastToolFacts) > 0 {
		snapshot["has_tool_facts"] = true
	}
	return snapshot
}

func compactPromptRecentMessages(value interface{}, limit int) []map[string]interface{} {
	recentMessages := normalizeRecentMemoryMessages(value)
	if len(recentMessages) == 0 || limit <= 0 {
		return nil
	}
	start := len(recentMessages) - limit
	if start < 0 {
		start = 0
	}
	out := make([]map[string]interface{}, 0, len(recentMessages)-start)
	for _, message := range recentMessages[start:] {
		direction := strings.TrimSpace(asString(message["direction"]))
		kind := strings.ToUpper(strings.TrimSpace(firstNonEmpty(asString(message["kind"]), "TEXT")))
		body := strings.TrimSpace(asString(message["body"]))
		if body == "" && kind == "TEXT" {
			continue
		}
		item := map[string]interface{}{
			"direction": direction,
			"kind":      kind,
		}
		if body != "" {
			item["body"] = body
		}
		out = append(out, item)
	}
	return out
}

func compactPromptBookingDraftContext(value map[string]interface{}) map[string]interface{} {
	return compactInterfaceFields(map[string]interface{}{
		"origin":                    strings.TrimSpace(asString(value["origin"])),
		"destination":               strings.TrimSpace(asString(value["destination"])),
		"trip_date":                 strings.TrimSpace(asString(value["trip_date"])),
		"departure_time":            strings.TrimSpace(asString(value["departure_time"])),
		"passenger_count":           asInt(value["passenger_count"]),
		"passenger_count_known":     value["passenger_count_known"],
		"child_under_5_count":       asInt(value["child_under_5_count"]),
		"child_under_5_count_known": value["child_under_5_count_known"],
		"has_availability_shown":    value["has_availability_shown"],
	})
}

func compactPromptToolFacts(tools agentToolContext) map[string]interface{} {
	facts := map[string]interface{}{}
	if tools.DocumentExtract != nil {
		passengers := make([]map[string]interface{}, 0, len(tools.DocumentExtract.Passengers))
		for _, item := range tools.DocumentExtract.Passengers {
			passengers = append(passengers, compactInterfaceFields(map[string]interface{}{
				"name":                     strings.TrimSpace(item.Name),
				"document_type":            strings.TrimSpace(item.DocumentType),
				"document":                 strings.TrimSpace(item.Document),
				"cpf":                      strings.TrimSpace(item.CPF),
				"cnh":                      strings.TrimSpace(item.CNH),
				"rg":                       strings.TrimSpace(item.RG),
				"birth_date":               strings.TrimSpace(item.BirthDate),
				"birth_certificate_number": strings.TrimSpace(item.BirthCertificateNumber),
				"birth_city":               strings.TrimSpace(item.BirthCity),
				"confidence":               item.Confidence,
			}))
		}
		facts[toolNameDocumentExtract] = compactInterfaceFields(map[string]interface{}{
			"mode":                     strings.TrimSpace(tools.DocumentExtract.Mode),
			"expected_passenger_count": tools.DocumentExtract.ExpectedPassengerCount,
			"extracted_passengers":     passengers,
			"failure_reason":           strings.TrimSpace(tools.DocumentExtract.FailureReason),
		})
	}
	if tools.Availability != nil {
		facts[toolNameAvailabilitySearch] = compactAvailabilityFact(*tools.Availability)
	}
	if tools.Pricing != nil {
		facts[toolNamePricingQuote] = compactPricingFact(*tools.Pricing)
	}
	if tools.Booking != nil {
		facts[toolNameBookingLookup] = compactBookingLookupFact(*tools.Booking)
	}
	if tools.BookingCreate != nil {
		facts[toolNameBookingCreate] = compactBookingCreateFact(*tools.BookingCreate)
	}
	if tools.Reschedule != nil {
		facts[toolNameRescheduleLookup] = compactRescheduleFact(*tools.Reschedule)
	}
	if tools.Payments != nil {
		facts[toolNamePaymentStatus] = compactPaymentStatusFact(*tools.Payments)
	}
	if tools.PaymentCreate != nil {
		facts[toolNamePaymentCreate] = compactPaymentCreateFact(*tools.PaymentCreate)
	}
	if tools.BookingCancel != nil {
		facts[toolNameBookingCancel] = compactBookingCancelFact(*tools.BookingCancel)
	}
	return facts
}

func compactAvailabilityFact(result AvailabilitySearchResult) map[string]interface{} {
	filter := result.Filter
	options := make([]map[string]interface{}, 0, minInt(len(result.Results), 5))
	for i, item := range result.Results {
		if i >= 5 {
			break
		}
		options = append(options, compactInterfaceFields(map[string]interface{}{
			"origin":       strings.TrimSpace(item.OriginDisplayName),
			"destination":  strings.TrimSpace(item.DestinationDisplayName),
			"trip_date":    strings.TrimSpace(item.TripDate),
			"departure":    strings.TrimSpace(item.OriginDepartTime),
			"price":        formatPromptMoney(item.Price, item.Currency),
			"package_name": strings.TrimSpace(item.PackageName),
		}))
	}
	return compactInterfaceFields(map[string]interface{}{
		"filter": compactInterfaceFields(map[string]interface{}{
			"origin":       strings.TrimSpace(filter.Origin),
			"destination":  strings.TrimSpace(filter.Destination),
			"package_name": strings.TrimSpace(filter.PackageName),
			"trip_date":    formatPromptDate(filter.TripDate),
			"qty":          filter.Qty,
		}),
		"result_count": len(result.Results),
		"options":      options,
	})
}

func compactPricingFact(result PricingQuoteResult) map[string]interface{} {
	quotes := make([]map[string]interface{}, 0, minInt(len(result.Results), 5))
	for i, item := range result.Results {
		if i >= 5 {
			break
		}
		quotes = append(quotes, compactInterfaceFields(map[string]interface{}{
			"origin":       strings.TrimSpace(item.OriginDisplayName),
			"destination":  strings.TrimSpace(item.DestinationDisplayName),
			"trip_date":    strings.TrimSpace(item.TripDate),
			"departure":    strings.TrimSpace(item.OriginDepartTime),
			"final_amount": formatPromptMoney(item.FinalAmount, item.Currency),
		}))
	}
	return compactInterfaceFields(map[string]interface{}{
		"fare_mode": strings.TrimSpace(result.Filter.FareMode),
		"quotes":    quotes,
	})
}

func compactBookingLookupFact(result BookingLookupResult) map[string]interface{} {
	reservations := make([]map[string]interface{}, 0, minInt(len(result.Results), 5))
	for i, item := range result.Results {
		if i >= 5 {
			break
		}
		reservations = append(reservations, compactInterfaceFields(map[string]interface{}{
			"reservation_code": strings.TrimSpace(item.ReservationCode),
			"status":           strings.TrimSpace(item.Status),
			"passenger_name":   strings.TrimSpace(item.PassengerName),
			"total":            formatPromptMoney(item.TotalAmount, "BRL"),
			"deposit":          formatPromptMoney(item.DepositAmount, "BRL"),
			"remainder":        formatPromptMoney(item.RemainderAmount, "BRL"),
			"expires_at":       formatPromptTime(item.ExpiresAt),
		}))
	}
	return compactInterfaceFields(map[string]interface{}{
		"reservation_code_filter": strings.TrimSpace(result.Filter.ReservationCode),
		"result_count":            len(result.Results),
		"reservations":            reservations,
	})
}

func compactBookingCreateFact(result BookingCreateResult) map[string]interface{} {
	passengers := make([]map[string]interface{}, 0, len(result.Passengers))
	for _, item := range result.Passengers {
		passengers = append(passengers, compactInterfaceFields(map[string]interface{}{
			"name":                     strings.TrimSpace(item.Name),
			"document_type":            strings.TrimSpace(item.DocumentType),
			"document":                 strings.TrimSpace(item.Document),
			"cpf":                      strings.TrimSpace(item.CPF),
			"rg":                       strings.TrimSpace(item.RG),
			"cnh":                      strings.TrimSpace(item.CNH),
			"birth_date":               strings.TrimSpace(item.BirthDate),
			"birth_certificate_number": strings.TrimSpace(item.BirthCertificateNumber),
			"birth_city":               strings.TrimSpace(item.BirthCity),
		}))
	}
	return compactInterfaceFields(map[string]interface{}{
		"mode":             strings.TrimSpace(result.Mode),
		"reservation_code": strings.TrimSpace(result.ReservationCode),
		"status":           strings.TrimSpace(result.Status),
		"total":            formatPromptMoney(result.TotalAmount, "BRL"),
		"deposit":          formatPromptMoney(result.DepositAmount, "BRL"),
		"remainder":        formatPromptMoney(result.RemainderAmount, "BRL"),
		"reserved_until":   formatPromptTime(result.ReservedUntil),
		"route": compactInterfaceFields(map[string]interface{}{
			"origin":      strings.TrimSpace(result.Filter.OriginDisplayName),
			"destination": strings.TrimSpace(result.Filter.DestinationDisplayName),
			"trip_date":   strings.TrimSpace(result.Filter.TripDate),
			"departure":   strings.TrimSpace(result.Filter.DepartureTime),
			"qty":         result.Filter.Qty,
		}),
		"passengers": passengers,
		"errors":     compactStringSlice(result.Errors),
		"message":    strings.TrimSpace(result.MessageForAgent),
	})
}

func compactRescheduleFact(result RescheduleAssistResult) map[string]interface{} {
	options := make([]map[string]interface{}, 0, minInt(len(result.Options), 5))
	for i, item := range result.Options {
		if i >= 5 {
			break
		}
		options = append(options, compactInterfaceFields(map[string]interface{}{
			"origin":       strings.TrimSpace(item.Origin),
			"destination":  strings.TrimSpace(item.Destination),
			"trip_date":    strings.TrimSpace(item.TripDate),
			"departure":    strings.TrimSpace(item.DepartureTime),
			"price":        formatPromptMoney(item.Price, item.Currency),
			"package_name": strings.TrimSpace(item.PackageName),
		}))
	}
	return compactInterfaceFields(map[string]interface{}{
		"mode": strings.TrimSpace(result.Mode),
		"current": compactInterfaceFields(map[string]interface{}{
			"origin":          strings.TrimSpace(result.Current.Origin),
			"destination":     strings.TrimSpace(result.Current.Destination),
			"trip_date":       strings.TrimSpace(result.Current.TripDate),
			"passenger_count": result.Current.PassengerCount,
		}),
		"requested": compactInterfaceFields(map[string]interface{}{
			"origin":      strings.TrimSpace(result.Requested.Origin),
			"destination": strings.TrimSpace(result.Requested.Destination),
			"trip_date":   strings.TrimSpace(result.Requested.TripDate),
			"qty":         result.Requested.Qty,
		}),
		"options":        options,
		"errors":         compactStringSlice(result.Errors),
		"manual_fields":  compactStringSlice(result.FieldsRequiredForManualCompletion),
		"message":        strings.TrimSpace(result.MessageForAgent),
		"booking_status": bookingStatusFromRescheduleFact(result),
	})
}

func compactPaymentStatusFact(result PaymentStatusResult) map[string]interface{} {
	payments := make([]map[string]interface{}, 0, minInt(len(result.Results), 5))
	total := 0.0
	paid := 0.0
	for i, item := range result.Results {
		total += item.Amount
		if strings.EqualFold(item.Status, "PAID") {
			paid += item.Amount
		}
		if i >= 5 {
			continue
		}
		payments = append(payments, compactInterfaceFields(map[string]interface{}{
			"status":     strings.TrimSpace(item.Status),
			"method":     strings.TrimSpace(item.Method),
			"amount":     formatPromptMoney(item.Amount, "BRL"),
			"created_at": item.CreatedAt.UTC().Format(time.RFC3339),
			"paid_at":    formatPromptTime(item.PaidAt),
		}))
	}
	return compactInterfaceFields(map[string]interface{}{
		"reservation_code_filter": strings.TrimSpace(result.Filter.ReservationCode),
		"result_count":            len(result.Results),
		"payments":                payments,
		"total_found":             formatPromptMoney(total, "BRL"),
		"total_paid":              formatPromptMoney(paid, "BRL"),
	})
}

func compactPaymentCreateFact(result PaymentCreateResult) map[string]interface{} {
	return compactInterfaceFields(map[string]interface{}{
		"mode":             strings.TrimSpace(result.Mode),
		"reservation_code": strings.TrimSpace(result.ReservationCode),
		"booking_status":   strings.TrimSpace(result.BookingStatus),
		"payment_type":     strings.TrimSpace(result.PaymentType),
		"stage":            strings.TrimSpace(result.Stage),
		"amount_total":     formatPromptMoney(result.AmountTotal, "BRL"),
		"amount_paid":      formatPromptMoney(result.AmountPaid, "BRL"),
		"amount_due":       formatPromptMoney(result.AmountDue, "BRL"),
		"payment_status":   strings.TrimSpace(result.PaymentStatus),
		"pix_code":         strings.TrimSpace(result.PixCode),
		"errors":           compactStringSlice(result.Errors),
		"message":          strings.TrimSpace(result.MessageForAgent),
	})
}

func compactBookingCancelFact(result BookingCancelResult) map[string]interface{} {
	return compactInterfaceFields(map[string]interface{}{
		"mode":             strings.TrimSpace(result.Mode),
		"reservation_code": strings.TrimSpace(result.ReservationCode),
		"previous_status":  strings.TrimSpace(result.PreviousStatus),
		"booking_status":   strings.TrimSpace(result.BookingStatus),
		"passenger_count":  result.PassengerCount,
		"reason":           strings.TrimSpace(result.Reason),
		"errors":           compactStringSlice(result.Errors),
		"message":          strings.TrimSpace(result.MessageForAgent),
	})
}

func compactStringFields(fields map[string]string) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range fields {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out[key] = trimmed
		}
	}
	return out
}

func compactInterfaceFields(fields map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range fields {
		if isEmptyPromptValue(value) {
			continue
		}
		out[key] = value
	}
	return out
}

func isEmptyPromptValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []string:
		return len(typed) == 0
	case []map[string]interface{}:
		return len(typed) == 0
	case map[string]interface{}:
		return len(typed) == 0
	case bool:
		return !typed
	case int:
		return typed == 0
	case float64:
		return typed == 0
	default:
		return false
	}
}

func compactStringSlice(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func formatPromptMoney(value float64, currency string) string {
	if value <= 0 {
		return ""
	}
	if strings.TrimSpace(currency) == "" {
		currency = "BRL"
	}
	if strings.EqualFold(currency, "BRL") {
		return fmt.Sprintf("R$ %.2f", value)
	}
	return fmt.Sprintf("%.2f %s", value, strings.TrimSpace(currency))
}

func formatPromptDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}

func formatPromptTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func bookingStatusFromRescheduleFact(result RescheduleAssistResult) string {
	if result.Booking == nil {
		return ""
	}
	return strings.TrimSpace(result.Booking.Status)
}

type promptConversationContext struct {
	Origin                       string
	Destination                  string
	PackageName                  string
	RouteDirection               string
	ShortDateFollowUp            bool
	DestinationChosenNow         bool
	DateChosenForDestination     bool
	TravelOptionChosenNow        bool
	ShouldRespondWithSCTable     bool
	ShouldAskSCOriginForMA       bool
	PassengerCountReplyContext   bool
	PassengerCountReplyParsed    bool
	PassengerCount               int
	ChildUnder5Count             int
	ExpectedPassengerCount       int
	CapturedPassengerCount       int
	OutstandingPassengerCount    int
	WaitingForPassengerDocuments bool
	CurrentTurnHasImage          bool
	CurrentTurnMediaCount        int
}

func derivePromptConversationContext(currentTurn string, recentMessages []map[string]interface{}, currentTurnMedia []map[string]interface{}) promptConversationContext {
	texts := make([]string, 0, len(recentMessages))
	for _, message := range recentMessages {
		body := strings.TrimSpace(asString(message["body"]))
		if body == "" {
			continue
		}
		texts = append(texts, body)
	}
	currentContext := inferRouteContextFromText(currentTurn)
	historyContext := inferLatestRouteContextFromTexts(texts)
	currentContext = inferConversationTurnRouteContext(currentTurn, historyContext)
	merged := mergeInferredRouteContext(currentContext, historyContext)
	folded := foldChatText(currentTurn)
	tripDate := extractTripDate(currentTurn, time.Now().UTC())
	passengerCountReplyContext, passengerCountReplyParsed, passengerCount, childUnder5Count := detectPassengerCountReplyContext(currentTurn, recentMessages)
	destinationChosenNow := currentContext.Destination != "" && !strings.EqualFold(currentContext.Destination, historyContext.Destination)
	dateChosenForDestination := tripDate != nil && merged.Destination != "" && merged.PackageName != "" && merged.Origin == "" && (historyContext.Destination != "" || destinationChosenNow)
	travelOptionChosenNow := extractSelectedOptionIndex(currentTurn) > 0 || strings.Contains(folded, "essa opcao") || strings.Contains(folded, "essa viagem")
	expectedPassengerCount := inferExpectedPassengerCountFromMemory(currentTurn, recentMessages)
	capturedPassengerCount := inferKnownPassengerCountFromMemory(currentTurn, recentMessages)
	outstandingPassengerCount := 0
	if expectedPassengerCount > capturedPassengerCount {
		outstandingPassengerCount = expectedPassengerCount - capturedPassengerCount
	}
	waitingForPassengerDocuments := isWaitingForPassengerDocuments(recentMessages)

	return promptConversationContext{
		Origin:                       merged.Origin,
		Destination:                  merged.Destination,
		PackageName:                  merged.PackageName,
		RouteDirection:               merged.RouteDirection,
		ShortDateFollowUp:            looksLikeShortDateFollowUp(currentTurn),
		DestinationChosenNow:         destinationChosenNow && tripDate == nil,
		DateChosenForDestination:     dateChosenForDestination,
		TravelOptionChosenNow:        travelOptionChosenNow,
		ShouldRespondWithSCTable:     detectBroadTravelState(folded) == "SC" && !looksLikeBroadStateScheduleLookup(currentTurn) && currentContext.Origin == "" && currentContext.Destination == "",
		ShouldAskSCOriginForMA:       detectBroadTravelState(folded) == "MA" && !looksLikeBroadStateScheduleLookup(currentTurn) && currentContext.Origin == "" && currentContext.Destination == "",
		PassengerCountReplyContext:   passengerCountReplyContext,
		PassengerCountReplyParsed:    passengerCountReplyParsed,
		PassengerCount:               passengerCount,
		ChildUnder5Count:             childUnder5Count,
		ExpectedPassengerCount:       expectedPassengerCount,
		CapturedPassengerCount:       capturedPassengerCount,
		OutstandingPassengerCount:    outstandingPassengerCount,
		WaitingForPassengerDocuments: waitingForPassengerDocuments,
		CurrentTurnHasImage:          len(currentTurnMedia) > 0,
		CurrentTurnMediaCount:        len(currentTurnMedia),
	}
}

func detectPassengerCountReplyContext(currentTurn string, recentMessages []map[string]interface{}) (bool, bool, int, int) {
	if !lastBotAskedPassengerCountFromTexts(recentMessages) {
		return false, false, 0, 0
	}
	passengerCount, childUnder5Count, ok := parsePassengerCountReply(currentTurn)
	if ok {
		return true, true, passengerCount, childUnder5Count
	}
	return true, false, 0, 0
}

func lastBotAskedPassengerCountFromTexts(recentMessages []map[string]interface{}) bool {
	for i := len(recentMessages) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(asString(recentMessages[i]["direction"])), "OUTBOUND") {
			continue
		}
		if looksLikePassengerCountQuestion(strings.TrimSpace(asString(recentMessages[i]["body"]))) {
			return true
		}
	}
	return false
}

func inferExpectedPassengerCountFromMemory(currentTurn string, recentMessages []map[string]interface{}) int {
	if qty := inferPassengerQuantityFromFreeText(currentTurn); qty > 0 {
		return qty
	}
	for i := len(recentMessages) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(asString(recentMessages[i]["direction"])), "INBOUND") {
			continue
		}
		if qty := inferPassengerQuantityFromFreeText(strings.TrimSpace(asString(recentMessages[i]["body"]))); qty > 0 {
			return qty
		}
	}
	for i := len(recentMessages) - 1; i >= 0; i-- {
		if qty := inferPassengerQuantityFromFreeText(strings.TrimSpace(asString(recentMessages[i]["body"]))); qty > 0 {
			return qty
		}
	}
	return 0
}

func inferKnownPassengerCountFromMemory(currentTurn string, recentMessages []map[string]interface{}) int {
	if count := len(extractBookingCreatePassengers(currentTurn, Session{})); count > 0 {
		return count
	}
	for i := len(recentMessages) - 1; i >= 0; i-- {
		body := strings.TrimSpace(asString(recentMessages[i]["body"]))
		if body == "" {
			continue
		}
		if count := len(extractBookingCreatePassengers(body, Session{})); count > 0 {
			return count
		}
	}
	return 0
}

func isWaitingForPassengerDocuments(recentMessages []map[string]interface{}) bool {
	for i := len(recentMessages) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(asString(recentMessages[i]["direction"])), "OUTBOUND") {
			continue
		}
		body := foldChatText(asString(recentMessages[i]["body"]))
		if body == "" {
			continue
		}
		if strings.Contains(body, "foto legivel do documento") ||
			(strings.Contains(body, "envie") && strings.Contains(body, "documento")) ||
			(strings.Contains(body, "nomes completos") && strings.Contains(body, "documentos")) ||
			(strings.Contains(body, "frente e verso") && strings.Contains(body, "documento")) {
			return true
		}
	}
	return false
}

func normalizeRecentMemoryMessages(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(typed))
		for _, raw := range typed {
			message, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			items = append(items, message)
		}
		return items
	default:
		return nil
	}
}

func normalizeMediaMemoryItems(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(typed))
		for _, raw := range typed {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			items = append(items, item)
		}
		return items
	default:
		return nil
	}
}

func inferLatestRouteContextFromTexts(texts []string) inferredRouteContext {
	context := inferredRouteContext{}
	for _, text := range texts {
		turnContext := inferConversationTurnRouteContext(text, context)
		context = mergeInferredRouteContext(turnContext, context)
	}
	return context
}
