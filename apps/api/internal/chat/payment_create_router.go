package chat

import (
	"strings"
)

func parsePaymentCreateInput(session Session, history []Message, text string, currentBooking *BookingLookupResult, currentBookingCreate *BookingCreateResult) (PaymentCreateInput, bool) {
	body := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if body == "" {
		return PaymentCreateInput{}, false
	}
	if !looksLikePaymentCreateIntent(body) && !looksLikeContextualPaymentCreateIntent(history, body) {
		return PaymentCreateInput{}, false
	}

	bookingID, reservationCode := resolvePaymentCreateTarget(body, history, currentBooking, currentBookingCreate)
	if bookingID == "" {
		return PaymentCreateInput{}, false
	}

	paymentType := resolveRequestedPaymentType(history, body)
	customerDocument, customerDocumentSource := resolvePaymentCreateCustomerDocument(history, body)

	input := PaymentCreateInput{
		BookingID:              bookingID,
		ReservationCode:        reservationCode,
		PaymentType:            paymentType,
		DepositPerPerson:       250,
		CustomerName:           strings.TrimSpace(session.CustomerName),
		CustomerPhone:          strings.TrimSpace(session.CustomerPhone),
		CustomerDocument:       customerDocument,
		CustomerDocumentSource: customerDocumentSource,
		Note:                   buildPaymentCreateNoteForType(bookingID, reservationCode, paymentType),
	}
	return input, true
}

func looksLikePaymentCreateIntent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	keywords := []string{
		"gerar pix",
		"gerar o pix",
		"gera o pix",
		"gera pix",
		"criar pix",
		"manda o pix",
		"manda pix",
		"envia o pix",
		"enviar pix",
		"quero pagar",
		"pagar via pix",
		"fazer o pagamento",
		"fazer pagamento",
		"gerar pagamento",
		"criar cobranca",
		"criar cobrança",
		"copia e cola do pix",
		"copia e cola pix",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func looksLikeContextualPaymentCreateIntent(history []Message, text string) bool {
	if !historyMentionsCreatedBooking(history) {
		return false
	}

	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}

	if lastAssistantAskedPayerCPF(history) && looksLikeBareCPF(text) {
		return true
	}

	if looksLikePaymentCreateConfirmationReply(folded) && historyMentionsPixConfirmation(history) {
		return true
	}

	if looksLikePixOnlyPaymentReply(folded) && historyMentionsPaymentChoice(history) {
		return true
	}

	if detectRequestedPaymentType(folded) != "" && (historyMentionsPaymentChoice(history) || historyMentionsPixConfirmation(history)) {
		return true
	}

	return false
}

func looksLikePaymentCreateConfirmationReply(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	switch folded {
	case "sim",
		"ok",
		"okay",
		"certo",
		"isso",
		"isso mesmo",
		"pode",
		"pode sim",
		"pode mandar",
		"pode enviar",
		"manda",
		"mande",
		"envia",
		"envie",
		"confirmo",
		"confirmado":
		return true
	}

	words := strings.Fields(folded)
	if len(words) <= 4 &&
		strings.Contains(folded, "pode") &&
		(strings.Contains(folded, "mandar") || strings.Contains(folded, "enviar")) {
		return true
	}

	return false
}

func looksLikePixOnlyPaymentReply(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	switch folded {
	case "pix",
		"no pix",
		"por pix",
		"via pix",
		"pelo pix",
		"pagar no pix",
		"pagar via pix",
		"manda pix",
		"mande pix",
		"envia pix",
		"envie pix":
		return true
	}

	return len(strings.Fields(folded)) <= 4 && strings.Contains(folded, "pix")
}

func lastAssistantAskedPayerCPF(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		body := strings.Join(strings.Fields(foldChatText(messageTurnText(message))), " ")
		if body == "" {
			continue
		}
		return strings.Contains(body, "cpf do pagador") ||
			(strings.Contains(body, "preciso do cpf") && (strings.Contains(body, "pix") || strings.Contains(body, "pagador") || strings.Contains(body, "pagamento"))) ||
			(strings.Contains(body, "para gerar o pix") && strings.Contains(body, "cpf"))
	}
	return false
}

func resolveRequestedPaymentType(history []Message, text string) string {
	if explicit := detectRequestedPaymentType(text); explicit != "" {
		return explicit
	}
	if previous := findLatestRequestedPaymentType(history); previous != "" {
		return previous
	}
	return extractRequestedPaymentType(text)
}

func findLatestRequestedPaymentType(history []Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
			continue
		}

		body := strings.TrimSpace(message.Body)
		if body == "" {
			continue
		}

		if detected := detectRequestedPaymentType(body); detected != "" {
			return detected
		}
	}

	return ""
}

func detectRequestedPaymentType(text string) string {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return ""
	}

	integralKeywords := []string{
		"integral",
		"valor total",
		"pagar tudo",
		"quitar",
		"quitacao",
		"restante",
		"saldo",
	}
	for _, keyword := range integralKeywords {
		if strings.Contains(folded, keyword) {
			return "integral"
		}
	}

	signalKeywords := []string{
		"sinal",
		"entrada",
		"deposito",
	}
	for _, keyword := range signalKeywords {
		if strings.Contains(folded, keyword) {
			return "sinal"
		}
	}

	return ""
}

func extractRequestedPaymentType(text string) string {
	if detected := detectRequestedPaymentType(text); detected != "" {
		return detected
	}
	return "sinal"
}

func extractExplicitPaymentDocument(text string) string {
	if match := passengerCPFPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		return normalizePassengerDocumentValue(match[1], "CPF")
	}
	if looksLikeBareCPF(text) {
		return normalizeDigits(text)
	}
	return ""
}

func resolvePaymentCreateCustomerDocument(history []Message, text string) (string, string) {
	if document := extractExplicitPaymentDocument(text); document != "" {
		return document, "current_turn"
	}
	if document := findLatestPaymentCPFInHistory(history); document != "" {
		return document, "history"
	}
	return "", ""
}

func findLatestPaymentCPFInHistory(history []Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		body := strings.TrimSpace(messageTurnText(message))
		if body == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
			if correction, ok := parsePassengerDocumentCorrection(body); ok && correction.DocumentType == "CPF" {
				return correction.Document
			}
			if previousAssistantAskedPayerCPF(history, i) {
				if document := extractExplicitPaymentDocument(body); document != "" {
					return document
				}
			}
		}
	}
	return ""
}

func previousAssistantAskedPayerCPF(history []Message, beforeIndex int) bool {
	for i := beforeIndex - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return lastAssistantAskedPayerCPF(history[:i+1])
	}
	return false
}

func resolvePaymentCreateTarget(text string, history []Message, currentBooking *BookingLookupResult, currentBookingCreate *BookingCreateResult) (string, string) {
	bookingID, reservationCode := extractBookingIdentifiers(text)
	if bookingID != "" {
		return bookingID, reservationCode
	}

	if currentBooking != nil && len(currentBooking.Results) > 0 {
		item := currentBooking.Results[0]
		return strings.TrimSpace(item.ID), firstNonEmpty(reservationCode, strings.TrimSpace(item.ReservationCode))
	}
	if currentBookingCreate != nil && strings.TrimSpace(currentBookingCreate.BookingID) != "" {
		return strings.TrimSpace(currentBookingCreate.BookingID), firstNonEmpty(reservationCode, strings.TrimSpace(currentBookingCreate.ReservationCode))
	}
	if previous := findLatestBookingCreateContext(history); previous != nil && strings.TrimSpace(previous.BookingID) != "" {
		return strings.TrimSpace(previous.BookingID), firstNonEmpty(reservationCode, strings.TrimSpace(previous.ReservationCode))
	}
	if previous := findLatestBookingLookupContext(history); previous != nil && len(previous.Results) > 0 {
		item := previous.Results[0]
		return strings.TrimSpace(item.ID), firstNonEmpty(reservationCode, strings.TrimSpace(item.ReservationCode))
	}
	return "", reservationCode
}

func buildPaymentCreateNote(bookingID string, reservationCode string, text string) string {
	return buildPaymentCreateNoteForType(bookingID, reservationCode, extractRequestedPaymentType(text))
}

func buildPaymentCreateNoteForType(bookingID string, reservationCode string, paymentType string) string {
	target := firstNonEmpty(strings.TrimSpace(reservationCode), strings.TrimSpace(bookingID))
	return "Pagamento " + normalizePaymentType(paymentType) + " reserva " + target
}

func findLatestBookingCreateContext(history []Message) *BookingCreateResult {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if message.Direction != "OUTBOUND" || !isAutomationDraftStatus(message.ProcessingStatus) {
			continue
		}
		toolContext := asMap(message.Payload["tool_context"])
		payload := asMap(toolContext[toolNameBookingCreate])
		if len(payload) == 0 {
			continue
		}
		result := parseBookingCreateContextPayload(payload)
		if strings.TrimSpace(result.BookingID) != "" {
			return &result
		}
	}
	return nil
}

func findLatestBookingLookupContext(history []Message) *BookingLookupResult {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if message.Direction != "OUTBOUND" || !isAutomationDraftStatus(message.ProcessingStatus) {
			continue
		}
		toolContext := asMap(message.Payload["tool_context"])
		payload := asMap(toolContext[toolNameBookingLookup])
		if len(payload) == 0 {
			continue
		}
		result := parseBookingLookupContextPayload(payload)
		if len(result.Results) > 0 {
			return &result
		}
	}
	return nil
}

func parseBookingCreateContextPayload(payload map[string]interface{}) BookingCreateResult {
	result := BookingCreateResult{
		Mode:            strings.TrimSpace(asString(payload["mode"])),
		BookingID:       strings.TrimSpace(asString(payload["booking_id"])),
		ReservationCode: strings.TrimSpace(asString(payload["reservation_code"])),
		Status:          strings.TrimSpace(asString(payload["status"])),
		TotalAmount:     asFloat64(payload["total_amount"]),
		DepositAmount:   asFloat64(payload["deposit_amount"]),
		RemainderAmount: asFloat64(payload["remainder_amount"]),
		Errors:          asStringSlice(payload["errors"]),
		MessageForAgent: strings.TrimSpace(asString(payload["message_for_agent"])),
	}
	for _, item := range asInterfaceSliceMaps(payload["passengers"]) {
		result.Passengers = append(result.Passengers, BookingCreatePassengerResult{
			Name:         strings.TrimSpace(asString(item["name"])),
			Document:     strings.TrimSpace(asString(item["document"])),
			DocumentType: strings.TrimSpace(asString(item["document_type"])),
			Phone:        strings.TrimSpace(asString(item["phone"])),
			SeatID:       strings.TrimSpace(asString(item["seat_id"])),
			IsLapChild:   strings.EqualFold(strings.TrimSpace(asString(item["is_lap_child"])), "true"),
		})
	}
	return result
}

func parseBookingLookupContextPayload(payload map[string]interface{}) BookingLookupResult {
	result := BookingLookupResult{
		Filter: BookingLookupInput{
			BookingID:       strings.TrimSpace(asString(payload["booking_id"])),
			ReservationCode: strings.TrimSpace(asString(payload["reservation_code"])),
			Limit:           asInt(payload["result_count"]),
		},
	}
	for _, item := range asInterfaceSliceMaps(payload["results"]) {
		result.Results = append(result.Results, BookingLookupItem{
			ID:              strings.TrimSpace(asString(item["booking_id"])),
			ReservationCode: strings.TrimSpace(asString(item["reservation_code"])),
			Status:          strings.TrimSpace(asString(item["status"])),
			TotalAmount:     asFloat64(item["total_amount"]),
			DepositAmount:   asFloat64(item["deposit_amount"]),
			RemainderAmount: asFloat64(item["remainder_amount"]),
			PassengerName:   strings.TrimSpace(asString(item["passenger_name"])),
			PassengerPhone:  strings.TrimSpace(asString(item["passenger_phone"])),
			SeatNumber:      asInt(item["seat_number"]),
		})
	}
	return result
}

func buildPaymentCreateRequestPayload(input PaymentCreateInput) map[string]interface{} {
	payload := map[string]interface{}{
		"booking_id":               strings.TrimSpace(input.BookingID),
		"reservation_code":         strings.TrimSpace(input.ReservationCode),
		"payment_type":             normalizePaymentType(input.PaymentType),
		"confirm_paid":             input.ConfirmPaid,
		"paid_amount":              input.PaidAmount,
		"deposit_per_person":       input.DepositPerPerson,
		"customer_name":            strings.TrimSpace(input.CustomerName),
		"customer_phone":           strings.TrimSpace(input.CustomerPhone),
		"customer_document":        strings.TrimSpace(input.CustomerDocument),
		"customer_document_source": strings.TrimSpace(input.CustomerDocumentSource),
		"customer_email":           strings.TrimSpace(input.CustomerEmail),
		"note":                     strings.TrimSpace(input.Note),
	}
	return payload
}

func buildPaymentCreateResponsePayload(result PaymentCreateResult) map[string]interface{} {
	payload := map[string]interface{}{
		"mode":              result.Mode,
		"booking_id":        result.BookingID,
		"reservation_code":  result.ReservationCode,
		"booking_status":    result.BookingStatus,
		"payment_type":      result.PaymentType,
		"stage":             result.Stage,
		"amount_total":      result.AmountTotal,
		"amount_paid":       result.AmountPaid,
		"amount_due":        result.AmountDue,
		"payment_id":        result.PaymentID,
		"payment_status":    result.PaymentStatus,
		"provider":          result.Provider,
		"provider_ref":      result.ProviderRef,
		"pix_code":          result.PixCode,
		"checkout_url":      result.CheckoutURL,
		"errors":            result.Errors,
		"message_for_agent": result.MessageForAgent,
	}
	return payload
}
