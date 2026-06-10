package chat

import (
	"testing"
	"time"
)

func TestParsePaymentCreateInputUsesShortReplyAfterBookingCreated(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Reserva criada com sucesso. Codigo ABC12345.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameBookingCreate: buildBookingCreateResponsePayload(BookingCreateResult{
						Mode:            "created",
						BookingID:       "BK-ABC123456",
						ReservationCode: "ABC12345",
						Status:          "PENDING",
						TotalAmount:     950,
						RemainderAmount: 950,
					}),
				},
			},
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
		},
	}

	input, ok := parsePaymentCreateInput(session, history, "integral", nil, nil)
	if !ok {
		t.Fatal("expected payment create input from short payment reply")
	}
	if input.BookingID != "BK-ABC123456" || input.ReservationCode != "ABC12345" {
		t.Fatalf("unexpected payment target: %+v", input)
	}
	if input.PaymentType != "integral" {
		t.Fatalf("expected payment type integral, got %+v", input)
	}
	if input.Note != "Pagamento integral reserva ABC12345" {
		t.Fatalf("unexpected payment note: %q", input.Note)
	}
}

func TestParsePaymentCreateInputUsesBareCPFReplyAfterPayerCPFPrompt(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Reserva criada com sucesso. Codigo ABC12345.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameBookingCreate: buildBookingCreateResponsePayload(BookingCreateResult{
						Mode:            "created",
						BookingID:       "BK-ABC123456",
						ReservationCode: "ABC12345",
						Status:          "PENDING",
						TotalAmount:     950,
						RemainderAmount: 950,
					}),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "integral",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-2 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Para gerar o PIX, preciso do CPF do pagador.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
		},
	}

	input, ok := parsePaymentCreateInput(session, history, "52998224725", nil, nil)
	if !ok {
		t.Fatal("expected payment create input from bare CPF reply")
	}
	if input.CustomerDocument != "52998224725" || input.CustomerDocumentSource != "current_turn" {
		t.Fatalf("expected current-turn CPF as payer document, got %+v", input)
	}
	if input.PaymentType != "integral" {
		t.Fatalf("expected previous payment type integral, got %+v", input)
	}
}

func TestExtractBookingIdentifiersIgnoresPaymentTypeWords(t *testing.T) {
	bookingID, reservationCode := extractBookingIdentifiers("integral")
	if bookingID != "" || reservationCode != "" {
		t.Fatalf("expected no identifiers, got booking_id=%q reservation_code=%q", bookingID, reservationCode)
	}
}
