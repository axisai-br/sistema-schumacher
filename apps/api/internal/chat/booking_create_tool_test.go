package chat

import (
	"context"
	"errors"
	"testing"

	"schumacher-tur/api/internal/bookings"
)

type fakeBookingCreateService struct {
	result    bookings.BookingDetails
	err       error
	lastInput bookings.CreateBookingInput
}

func (f *fakeBookingCreateService) Create(_ context.Context, input bookings.CreateBookingInput) (bookings.BookingDetails, error) {
	f.lastInput = input
	if f.err != nil {
		return bookings.BookingDetails{}, f.err
	}
	return f.result, nil
}

func TestBookingCreateToolMapsValidationErrorsToOperationalResult(t *testing.T) {
	tool := NewBookingCreateTool(&fakeBookingCreateService{err: bookings.ErrPassengerDocumentType})

	result, err := tool.Create(context.Background(), BookingCreateInput{
		TripID:       "trip-1",
		BoardStopID:  "board-1",
		AlightStopID: "alight-1",
		Passengers: []BookingCreatePassengerInput{
			{Name: "Joao", Document: "123", DocumentType: "INVALID"},
		},
	})
	if err != nil {
		t.Fatalf("expected no fatal error, got %v", err)
	}
	if result.Mode != "manual_review_required_validation_error" {
		t.Fatalf("expected validation mode, got %s", result.Mode)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected one validation error, got %+v", result.Errors)
	}
}

func TestBookingCreateToolReturnsUnexpectedErrors(t *testing.T) {
	tool := NewBookingCreateTool(&fakeBookingCreateService{err: errors.New("db down")})

	_, err := tool.Create(context.Background(), BookingCreateInput{
		TripID:       "trip-1",
		BoardStopID:  "board-1",
		AlightStopID: "alight-1",
		Passengers: []BookingCreatePassengerInput{
			{Name: "Joao", Document: "06645648105", DocumentType: "CPF"},
		},
	})
	if err == nil {
		t.Fatalf("expected fatal error for unexpected failure")
	}
}

func TestBookingCreateToolMapsAdditionalPassengerIdentityFields(t *testing.T) {
	fake := &fakeBookingCreateService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{ID: "BK-1", ReservationCode: "ABC12345", Status: "PENDING"},
			Passengers: []bookings.BookingPassenger{
				{
					Name:                   "Crianca Silva",
					Document:               testBirthCertificateNumber,
					DocumentType:           "CERTIDAO_NASCIMENTO",
					CPF:                    "84960815086",
					RG:                     "1234567",
					CNH:                    "99999999999",
					BirthDate:              "2022-05-21",
					BirthCertificateNumber: testBirthCertificateNumber,
					BirthCity:              "Santa Ines",
				},
			},
		},
	}
	tool := NewBookingCreateTool(fake)

	result, err := tool.Create(context.Background(), BookingCreateInput{
		TripID:       "trip-1",
		BoardStopID:  "board-1",
		AlightStopID: "alight-1",
		Passengers: []BookingCreatePassengerInput{
			{
				Name:                   "Crianca Silva",
				Document:               testBirthCertificateNumber,
				DocumentType:           "CERTIDAO_NASCIMENTO",
				CPF:                    "84960815086",
				RG:                     "1234567",
				CNH:                    "99999999999",
				BirthDate:              "2022-05-21",
				BirthCertificateNumber: testBirthCertificateNumber,
				BirthCity:              "Santa Ines",
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fake.lastInput.Passengers) != 1 {
		t.Fatalf("expected passenger sent to booking service, got %+v", fake.lastInput.Passengers)
	}
	passenger := fake.lastInput.Passengers[0]
	if passenger.CPF != "84960815086" || passenger.RG != "1234567" || passenger.CNH != "99999999999" || passenger.BirthDate != "2022-05-21" || passenger.BirthCertificateNumber != testBirthCertificateNumber || passenger.BirthCity != "Santa Ines" {
		t.Fatalf("expected additional identity fields mapped to booking service, got %+v", passenger)
	}
	if len(result.Passengers) != 1 || result.Passengers[0].CPF != "84960815086" || result.Passengers[0].RG != "1234567" || result.Passengers[0].CNH != "99999999999" || result.Passengers[0].BirthCity != "Santa Ines" {
		t.Fatalf("expected additional identity fields in result, got %+v", result.Passengers)
	}
}
