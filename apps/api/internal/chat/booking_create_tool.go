package chat

import (
	"context"
	"errors"
	"strings"

	"schumacher-tur/api/internal/bookings"
)

type BookingCreateTool struct {
	svc interface {
		Create(ctx context.Context, input bookings.CreateBookingInput) (bookings.BookingDetails, error)
	}
}

func NewBookingCreateTool(svc interface {
	Create(ctx context.Context, input bookings.CreateBookingInput) (bookings.BookingDetails, error)
}) *BookingCreateTool {
	return &BookingCreateTool{svc: svc}
}

func (t *BookingCreateTool) Enabled() bool {
	return t != nil && t.svc != nil
}

func (t *BookingCreateTool) Create(ctx context.Context, input BookingCreateInput) (BookingCreateResult, error) {
	result := BookingCreateResult{
		Filter:     input,
		Mode:       "manual_review_required_input_error",
		Passengers: []BookingCreatePassengerResult{},
	}
	if !t.Enabled() {
		return result, ErrAgentToolNotConfigured
	}

	if strings.TrimSpace(input.TripID) == "" || strings.TrimSpace(input.BoardStopID) == "" || strings.TrimSpace(input.AlightStopID) == "" {
		result.Errors = []string{"Nao foi possivel resolver a opcao de viagem para criar a reserva."}
		result.MessageForAgent = "Nao confirme reserva sem trip_id, board_stop_id e alight_stop_id. Peca ao cliente para escolher novamente a opcao correta."
		return result, nil
	}
	if len(input.Passengers) == 0 {
		result.Errors = []string{"Nao foi possivel identificar os dados do passageiro."}
		result.MessageForAgent = "Antes de criar a reserva, confirme nome completo e documento do passageiro."
		return result, nil
	}

	passengers := make([]bookings.PassengerInput, 0, len(input.Passengers))
	for _, item := range input.Passengers {
		passengers = append(passengers, bookings.PassengerInput{
			Name:                   strings.TrimSpace(item.Name),
			Document:               strings.TrimSpace(item.Document),
			DocumentType:           strings.TrimSpace(item.DocumentType),
			CPF:                    strings.TrimSpace(item.CPF),
			RG:                     strings.TrimSpace(item.RG),
			CNH:                    strings.TrimSpace(item.CNH),
			BirthDate:              strings.TrimSpace(item.BirthDate),
			BirthCertificateNumber: strings.TrimSpace(item.BirthCertificateNumber),
			BirthCity:              strings.TrimSpace(item.BirthCity),
			Phone:                  strings.TrimSpace(item.Phone),
			Email:                  strings.TrimSpace(item.Email),
			Notes:                  strings.TrimSpace(item.Notes),
			IsLapChild:             item.IsLapChild,
		})
	}

	source := "WHATSAPP"
	created, err := t.svc.Create(ctx, bookings.CreateBookingInput{
		TripID:         strings.TrimSpace(input.TripID),
		BoardStopID:    strings.TrimSpace(input.BoardStopID),
		AlightStopID:   strings.TrimSpace(input.AlightStopID),
		Passengers:     passengers,
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		Source:         &source,
	})
	if err != nil {
		result.Mode = "manual_review_required_validation_error"
		result.Errors = []string{strings.TrimSpace(err.Error())}
		switch {
		case errors.Is(err, bookings.ErrMissingFields),
			errors.Is(err, bookings.ErrMissingStops),
			errors.Is(err, bookings.ErrPassengerNameRequired),
			errors.Is(err, bookings.ErrPassengerDocumentType),
			errors.Is(err, bookings.ErrSeatRequiresSinglePassenger),
			errors.Is(err, bookings.ErrNegativeAmounts),
			errors.Is(err, bookings.ErrInvalidAmounts):
			result.MessageForAgent = "A tentativa de reserva falhou por validacao. Explique o dado faltante ou invalido e nao tente criar novamente sem corrigir."
			return result, nil
		case errors.Is(err, bookings.ErrNoSeatsAvailable):
			result.Mode = "manual_review_required_no_seats"
			result.MessageForAgent = "Nao ha assentos disponiveis para essa opcao. Informe indisponibilidade e ofereca nova busca."
			return result, nil
		case errors.Is(err, bookings.ErrSeatNotInTrip):
			result.MessageForAgent = "A opcao selecionada ficou inconsistente. Nao confirme reserva; peca nova escolha de opcao."
			return result, nil
		default:
			return BookingCreateResult{}, err
		}
	}

	result.Mode = "created"
	result.BookingID = strings.TrimSpace(created.Booking.ID)
	result.ReservationCode = strings.TrimSpace(created.Booking.ReservationCode)
	result.Status = strings.TrimSpace(created.Booking.Status)
	result.ReservedUntil = created.Booking.ExpiresAt
	result.TotalAmount = created.Booking.TotalAmount
	result.DepositAmount = created.Booking.DepositAmount
	result.RemainderAmount = created.Booking.RemainderAmount
	for _, item := range created.Passengers {
		result.Passengers = append(result.Passengers, BookingCreatePassengerResult{
			Name:                   strings.TrimSpace(item.Name),
			Document:               strings.TrimSpace(item.Document),
			DocumentType:           strings.TrimSpace(item.DocumentType),
			CPF:                    strings.TrimSpace(item.CPF),
			RG:                     strings.TrimSpace(item.RG),
			CNH:                    strings.TrimSpace(item.CNH),
			BirthDate:              strings.TrimSpace(item.BirthDate),
			BirthCertificateNumber: strings.TrimSpace(item.BirthCertificateNumber),
			BirthCity:              strings.TrimSpace(item.BirthCity),
			Phone:                  strings.TrimSpace(item.Phone),
			SeatID:                 strings.TrimSpace(item.SeatID),
			IsLapChild:             item.IsLapChild,
		})
	}
	result.MessageForAgent = "Reserva criada com sucesso. Informe o reservation_code ao cliente e siga para o pagamento."
	return result, nil
}
