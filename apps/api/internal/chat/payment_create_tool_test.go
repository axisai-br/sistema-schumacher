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
}

func (f *fakePaymentCreatePaymentsService) Create(_ context.Context, input payments.CreatePaymentInput) (payments.Payment, json.RawMessage, error) {
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
