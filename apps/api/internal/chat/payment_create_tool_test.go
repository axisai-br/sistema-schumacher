package chat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
)

type fakePaymentCreateBookingsService struct {
	result bookings.BookingDetails
	err    error
}

func (f *fakePaymentCreateBookingsService) Get(_ context.Context, _ string) (bookings.BookingDetails, error) {
	if f.err != nil {
		return bookings.BookingDetails{}, f.err
	}
	return f.result, nil
}

type fakePaymentCreatePaymentsService struct {
	payment   payments.Payment
	raw       json.RawMessage
	err       error
	lastInput payments.CreatePaymentInput
	calls     int
}

func syntheticValidCPFForTests() string {
	// Checksum-valid CPF generated for validator tests only. Do not pair with real contact data.
	return "84960815086"
}

func (f *fakePaymentCreatePaymentsService) Create(_ context.Context, input payments.CreatePaymentInput) (payments.Payment, json.RawMessage, error) {
	f.calls++
	f.lastInput = input
	if f.err != nil {
		return payments.Payment{}, nil, f.err
	}
	return f.payment, f.raw, nil
}

func TestPaymentCreateToolCreatesIntegralPixFromRemainder(t *testing.T) {
	paymentSvc := &fakePaymentCreatePaymentsService{
		payment: payments.Payment{
			ID:        "pay-1",
			BookingID: "BK-ABC123456",
			Status:    "PENDING",
			CreatedAt: time.Now().UTC(),
		},
		raw: json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201PIXCODE","qr_code_url":"https://provider/pix"}}]}`),
	}
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-ABC123456",
				Status:          "PENDING",
				ReservationCode: "ABC12345",
				TotalAmount:     950,
				DepositAmount:   300,
				RemainderAmount: 650,
			},
			Passenger: bookings.BookingPassenger{
				Name:         "Maria Silva",
				Document:     "52998224725",
				DocumentType: "CPF",
				Phone:        "48999999999",
			},
		},
	}, paymentSvc)

	result, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:       "BK-ABC123456",
		ReservationCode: "ABC12345",
		PaymentType:     "integral",
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if result.Mode != "pix_sent" {
		t.Fatalf("expected pix_sent mode, got %s", result.Mode)
	}
	if result.AmountDue != 650 {
		t.Fatalf("expected amount_due 650, got %.2f", result.AmountDue)
	}
	if result.PixCode != "000201PIXCODE" {
		t.Fatalf("expected pix code, got %s", result.PixCode)
	}
	if paymentSvc.lastInput.Amount != 650 {
		t.Fatalf("expected payment amount 650, got %.2f", paymentSvc.lastInput.Amount)
	}
	if paymentSvc.lastInput.Customer == nil || paymentSvc.lastInput.Customer.Document != "52998224725" {
		t.Fatalf("expected customer document propagated, got %+v", paymentSvc.lastInput.Customer)
	}
}

func TestPaymentCreateToolChargesDepositOnlyForPayingPassengerWhenLapChildExists(t *testing.T) {
	paymentSvc := &fakePaymentCreatePaymentsService{
		payment: payments.Payment{
			ID:        "pay-lap-child",
			BookingID: "BK-LAPCHILD",
			Status:    "PENDING",
			CreatedAt: time.Now().UTC(),
		},
		raw: json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201LAPCHILD","qr_code_url":"https://provider/pix"}}]}`),
	}
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-LAPCHILD",
				Status:          "PENDING",
				ReservationCode: "LAP12345",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passengers: []bookings.BookingPassenger{
				{
					Name:         "Joao Vitor Messias",
					Document:     "84960815086",
					DocumentType: "CPF",
					CPF:          "84960815086",
					Phone:        "48999999999",
				},
				{
					Name:         "Crianca Messias",
					Document:     testBirthCertificateNumber,
					DocumentType: "CERTIDAO_NASCIMENTO",
					IsLapChild:   true,
				},
			},
		},
	}, paymentSvc)

	result, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:       "BK-LAPCHILD",
		ReservationCode: "LAP12345",
		PaymentType:     "sinal",
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if result.Mode != "pix_sent" {
		t.Fatalf("expected pix_sent mode, got %s", result.Mode)
	}
	if result.AmountDue != 250 {
		t.Fatalf("expected deposit for one paying passenger, got %.2f", result.AmountDue)
	}
	if paymentSvc.lastInput.Amount != 250 {
		t.Fatalf("expected payment amount for one paying passenger, got %.2f", paymentSvc.lastInput.Amount)
	}
}

func TestPaymentCreateToolRejectsInvalidCPFWithElevenDigits(t *testing.T) {
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-ABC123456",
				Status:          "PENDING",
				ReservationCode: "ABC12345",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passenger: bookings.BookingPassenger{
				Name:         "Maria Silva",
				Document:     "12345678901",
				DocumentType: "CPF",
				Phone:        "48999999999",
			},
		},
	}, &fakePaymentCreatePaymentsService{})

	result, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:       "BK-ABC123456",
		ReservationCode: "ABC12345",
		PaymentType:     "sinal",
	})
	if err != nil {
		t.Fatalf("expected operational result, got %v", err)
	}
	if result.Mode != "manual_review_required_missing_payer_document" {
		t.Fatalf("expected missing payer document mode, got %s", result.Mode)
	}
}

func TestPaymentCreateToolRequiresPayerCPFWhenBookingUsesRG(t *testing.T) {
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-ABC123456",
				Status:          "PENDING",
				ReservationCode: "ABC12345",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passenger: bookings.BookingPassenger{
				Name:         "Maria Silva",
				Document:     "RG123456",
				DocumentType: "RG",
				Phone:        "48999999999",
			},
		},
	}, &fakePaymentCreatePaymentsService{})

	result, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:       "BK-ABC123456",
		ReservationCode: "ABC12345",
		PaymentType:     "sinal",
	})
	if err != nil {
		t.Fatalf("expected operational result, got %v", err)
	}
	if result.Mode != "manual_review_required_missing_payer_document" {
		t.Fatalf("expected missing payer document mode, got %s", result.Mode)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected one operational error, got %+v", result.Errors)
	}
}

func TestPaymentCreateToolUsesAdditionalPassengerCPFWhenPrimaryDocumentIsNotCPF(t *testing.T) {
	cases := []struct {
		documentType string
		document     string
	}{
		{documentType: "RG", document: "2817314"},
		{documentType: "CNH", document: "12345678900"},
		{documentType: "CERTIDAO_NASCIMENTO", document: "12345678901234567890123456789012"},
	}

	for _, tc := range cases {
		t.Run(tc.documentType, func(t *testing.T) {
			paymentSvc := &fakePaymentCreatePaymentsService{
				payment: payments.Payment{
					ID:        "pay-additional-cpf",
					BookingID: "BK-ABC123456",
					Status:    "PENDING",
					CreatedAt: time.Now().UTC(),
				},
				raw: json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201ADDITIONALCPF","qr_code_url":"https://provider/pix"}}]}`),
			}
			tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
				result: bookings.BookingDetails{
					Booking: bookings.Booking{
						ID:              "BK-ABC123456",
						Status:          "PENDING",
						ReservationCode: "ABC12345",
						TotalAmount:     950,
						RemainderAmount: 950,
					},
					Passenger: bookings.BookingPassenger{
						Name:         "Passageiro Teste",
						Document:     tc.document,
						DocumentType: tc.documentType,
						CPF:          syntheticValidCPFForTests(),
						Phone:        "11000000000",
					},
				},
			}, paymentSvc)

			result, err := tool.Create(context.Background(), PaymentCreateInput{
				BookingID:       "BK-ABC123456",
				ReservationCode: "ABC12345",
				PaymentType:     "sinal",
			})
			if err != nil {
				t.Fatalf("create payment: %v", err)
			}
			if result.Mode != "pix_sent" || result.PixCode != "000201ADDITIONALCPF" {
				t.Fatalf("expected PIX from saved passenger CPF, got %+v", result)
			}
			if paymentSvc.lastInput.Customer == nil || paymentSvc.lastInput.Customer.Document != syntheticValidCPFForTests() {
				t.Fatalf("expected additional passenger CPF as customer document, got %+v", paymentSvc.lastInput.Customer)
			}
		})
	}
}

func TestPaymentCreateToolUsesCurrentTurnCPFWhenBookingUsesRG(t *testing.T) {
	paymentSvc := &fakePaymentCreatePaymentsService{
		payment: payments.Payment{
			ID:        "pay-current",
			BookingID: "BK-ABC123456",
			Status:    "PENDING",
			CreatedAt: time.Now().UTC(),
		},
		raw: json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201CURRENT","qr_code_url":"https://provider/pix"}}]}`),
	}
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-ABC123456",
				Status:          "PENDING",
				ReservationCode: "ABC12345",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passenger: bookings.BookingPassenger{
				Name:         "Maria Silva",
				Document:     "2817314",
				DocumentType: "RG",
				Phone:        "48999999999",
			},
		},
	}, paymentSvc)

	result, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:              "BK-ABC123456",
		ReservationCode:        "ABC12345",
		PaymentType:            "integral",
		CustomerDocument:       "52998224725",
		CustomerDocumentSource: "current_turn",
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if result.Mode != "pix_sent" {
		t.Fatalf("expected pix_sent mode, got %s", result.Mode)
	}
	if paymentSvc.lastInput.Customer == nil || paymentSvc.lastInput.Customer.Document != "52998224725" {
		t.Fatalf("expected current-turn CPF as customer document, got %+v", paymentSvc.lastInput.Customer)
	}
}

func TestPaymentCreateToolUsesHistoryCPFWhenBookingUsesRG(t *testing.T) {
	paymentSvc := &fakePaymentCreatePaymentsService{
		payment: payments.Payment{
			ID:        "pay-history",
			BookingID: "BK-ABC123456",
			Status:    "PENDING",
			CreatedAt: time.Now().UTC(),
		},
		raw: json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201HISTORY","qr_code_url":"https://provider/pix"}}]}`),
	}
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-ABC123456",
				Status:          "PENDING",
				ReservationCode: "ABC12345",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passenger: bookings.BookingPassenger{
				Name:         "Maria Silva",
				Document:     "2817314",
				DocumentType: "RG",
				Phone:        "48999999999",
			},
		},
	}, paymentSvc)

	result, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:              "BK-ABC123456",
		ReservationCode:        "ABC12345",
		PaymentType:            "sinal",
		CustomerDocument:       "529.982.247-25",
		CustomerDocumentSource: "history",
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if result.Mode != "pix_sent" {
		t.Fatalf("expected pix_sent mode, got %s", result.Mode)
	}
	if paymentSvc.lastInput.Customer == nil || paymentSvc.lastInput.Customer.Document != "52998224725" {
		t.Fatalf("expected history CPF as customer document, got %+v", paymentSvc.lastInput.Customer)
	}
}

func TestPaymentCreateToolRejectsInvalidCurrentTurnCPFWithoutCallingPayments(t *testing.T) {
	paymentSvc := &fakePaymentCreatePaymentsService{}
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-ABC123456",
				Status:          "PENDING",
				ReservationCode: "ABC12345",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passenger: bookings.BookingPassenger{
				Name:         "Maria Silva",
				Document:     "2817314",
				DocumentType: "RG",
				Phone:        "48999999999",
			},
		},
	}, paymentSvc)

	result, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:              "BK-ABC123456",
		ReservationCode:        "ABC12345",
		PaymentType:            "sinal",
		CustomerDocument:       "12345678901",
		CustomerDocumentSource: "current_turn",
	})
	if err != nil {
		t.Fatalf("expected operational result, got %v", err)
	}
	if result.Mode != "manual_review_required_missing_payer_document" {
		t.Fatalf("expected missing payer document mode, got %s", result.Mode)
	}
	if paymentSvc.calls != 0 {
		t.Fatalf("expected invalid CPF not to call payment service, got %d calls", paymentSvc.calls)
	}
}

func TestTranscriptBareCPFAfterPayerPromptGeneratesPIX(t *testing.T) {
	now := time.Now().UTC()
	session := Session{CustomerName: "Maria Silva", CustomerPhone: "48999999999"}
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
						Passengers: []BookingCreatePassengerResult{
							{Name: "Maria Silva", DocumentType: "RG", Document: "2817314"},
						},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "integral", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Para gerar o PIX, preciso do CPF do pagador.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-1 * time.Minute)},
	}

	input, ok := parsePaymentCreateInput(session, history, "52998224725", nil, nil)
	if !ok {
		t.Fatalf("expected payment input from bare CPF")
	}
	if input.CustomerDocument != "52998224725" || input.CustomerDocumentSource != "current_turn" {
		t.Fatalf("expected current-turn CPF on payment input, got %+v", input)
	}

	paymentSvc := &fakePaymentCreatePaymentsService{
		payment: payments.Payment{ID: "pay-bare-cpf", BookingID: "BK-ABC123456", Status: "PENDING", CreatedAt: now},
		raw:     json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201BARECPF","qr_code_url":"https://provider/pix"}}]}`),
	}
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-ABC123456",
				Status:          "PENDING",
				ReservationCode: "ABC12345",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passenger: bookings.BookingPassenger{
				Name:         "Maria Silva",
				Document:     "2817314",
				DocumentType: "RG",
				Phone:        "48999999999",
			},
		},
	}, paymentSvc)

	result, err := tool.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if result.Mode != "pix_sent" || result.PixCode != "000201BARECPF" {
		t.Fatalf("expected PIX generated after bare CPF, got %+v", result)
	}
}
