package chat

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
)

type PaymentCreateTool struct {
	bookings interface {
		Get(ctx context.Context, id string) (bookings.BookingDetails, error)
	}
	payments interface {
		Create(ctx context.Context, input payments.CreatePaymentInput) (payments.Payment, json.RawMessage, error)
	}
}

func NewPaymentCreateTool(
	bookingsSvc interface {
		Get(ctx context.Context, id string) (bookings.BookingDetails, error)
	},
	paymentsSvc interface {
		Create(ctx context.Context, input payments.CreatePaymentInput) (payments.Payment, json.RawMessage, error)
	},
) *PaymentCreateTool {
	return &PaymentCreateTool{
		bookings: bookingsSvc,
		payments: paymentsSvc,
	}
}

func (t *PaymentCreateTool) Enabled() bool {
	return t != nil && t.bookings != nil && t.payments != nil
}

func (t *PaymentCreateTool) Create(ctx context.Context, input PaymentCreateInput) (PaymentCreateResult, error) {
	result := PaymentCreateResult{
		Filter: input,
		Mode:   "manual_review_required_input_error",
	}
	if !t.Enabled() {
		return result, ErrAgentToolNotConfigured
	}

	input.PaymentType = normalizePaymentType(input.PaymentType)
	result.PaymentType = input.PaymentType
	result.Filter.PaymentType = input.PaymentType

	bookingID := strings.TrimSpace(input.BookingID)
	if bookingID == "" {
		result.Errors = []string{"Nao foi possivel identificar a reserva para gerar o pagamento."}
		result.MessageForAgent = "Nao gere cobranca sem booking_id confirmado. Peca o codigo da reserva ou encaminhe para revisao humana."
		return result, nil
	}

	details, err := t.bookings.Get(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			result.Mode = "manual_review_required_booking_not_found"
			result.Errors = []string{"Reserva nao encontrada."}
			result.MessageForAgent = "Nao foi possivel localizar a reserva. Peca confirmacao do codigo antes de tentar gerar o PIX."
			return result, nil
		}
		return PaymentCreateResult{}, err
	}

	result.BookingID = strings.TrimSpace(details.Booking.ID)
	result.ReservationCode = firstNonEmpty(strings.TrimSpace(details.Booking.ReservationCode), strings.TrimSpace(input.ReservationCode))
	result.BookingStatus = strings.ToUpper(strings.TrimSpace(details.Booking.Status))
	result.AmountTotal = roundMoney(details.Booking.TotalAmount)
	result.AmountPaid = roundMoney(maxFloat(details.Booking.DepositAmount, details.Booking.TotalAmount-details.Booking.RemainderAmount))

	if isPaymentBlockedBookingStatus(result.BookingStatus) {
		result.Mode = "manual_review_required_booking_ineligible"
		result.Errors = []string{"Essa reserva nao pode receber cobranca porque esta cancelada ou expirada."}
		result.MessageForAgent = "Nao gere PIX para reserva cancelada ou expirada. Oriente revisao humana."
		return result, nil
	}

	passengers := details.Passengers
	if len(passengers) == 0 && (details.Passenger.ID != "" || strings.TrimSpace(details.Passenger.Name) != "") {
		passengers = []bookings.BookingPassenger{details.Passenger}
	}
	primaryPassenger := bookings.BookingPassenger{}
	if len(passengers) > 0 {
		primaryPassenger = passengers[0]
	}

	depositPerPerson := input.DepositPerPerson
	if depositPerPerson <= 0 {
		depositPerPerson = 250
	}
	qtyGuess := countChargeableBookingPassengers(passengers)
	if qtyGuess <= 0 {
		qtyGuess = 1
	}

	stage, amountDue := calculatePaymentAmount(result.AmountTotal, result.AmountPaid, roundMoney(details.Booking.RemainderAmount), input.PaymentType, depositPerPerson, qtyGuess, input.PaidAmount)
	result.Stage = stage
	result.AmountDue = amountDue

	if input.ConfirmPaid {
		result.Mode = "awaiting_webhook"
		result.MessageForAgent = "Nao gere nova cobranca. Explique que a confirmacao oficial do pagamento depende do webhook da API."
		return result, nil
	}
	if result.AmountDue <= 0.0001 {
		result.Mode = "manual_review_required_nothing_due"
		result.Errors = []string{"Nada a cobrar: a reserva nao tem saldo pendente para esse tipo de pagamento."}
		result.MessageForAgent = "Nao gere novo PIX quando nao houver saldo pendente para o tipo de pagamento solicitado."
		return result, nil
	}

	customerDocument := resolvePaymentPayerDocument(strings.TrimSpace(input.CustomerDocument), primaryPassenger)
	if !isSupportedPaymentDocument(customerDocument) {
		result.Mode = "manual_review_required_missing_payer_document"
		result.Errors = []string{buildMissingPayerDocumentMessage(primaryPassenger.DocumentType)}
		result.MessageForAgent = "Para gerar o PIX, primeiro peca o CPF do pagador em formato numerico."
		return result, nil
	}

	customerPhone := resolvePaymentPayerPhone(strings.TrimSpace(input.CustomerPhone), primaryPassenger)
	if len(customerPhone) < 10 {
		result.Mode = "manual_review_required_missing_payer_phone"
		result.Errors = []string{"Para gerar o PIX, preciso de um telefone do pagador com DDD."}
		result.MessageForAgent = "Antes de gerar o PIX, confirme um telefone do pagador com DDD."
		return result, nil
	}

	customerName := firstNonEmpty(
		strings.TrimSpace(input.CustomerName),
		strings.TrimSpace(primaryPassenger.Name),
		"Cliente Schumacher",
	)
	customerEmail := firstNonEmpty(
		strings.TrimSpace(input.CustomerEmail),
		strings.TrimSpace(primaryPassenger.Email),
	)
	description := strings.TrimSpace(input.Note)
	if description == "" {
		description = "Pagamento " + input.PaymentType + " reserva " + firstNonEmpty(result.ReservationCode, result.BookingID)
	}

	payment, raw, err := t.payments.Create(ctx, payments.CreatePaymentInput{
		BookingID:   result.BookingID,
		Amount:      result.AmountDue,
		Method:      "PIX",
		Description: description,
		Customer: &payments.CustomerInput{
			Name:     customerName,
			Email:    customerEmail,
			Phone:    customerPhone,
			Document: customerDocument,
		},
	})
	if err != nil {
		return PaymentCreateResult{}, err
	}

	result.PaymentID = strings.TrimSpace(payment.ID)
	result.PaymentStatus = strings.TrimSpace(payment.Status)
	if payment.Provider != nil {
		result.Provider = strings.TrimSpace(*payment.Provider)
	}
	if payment.ProviderRef != nil {
		result.ProviderRef = strings.TrimSpace(*payment.ProviderRef)
	}
	result.CheckoutURL, result.PixCode = payments.ExtractCheckoutAndPix(raw)

	if strings.TrimSpace(result.PixCode) == "" {
		result.Mode = "manual_review_required_provider_error"
		result.Errors = []string{"A cobranca foi criada, mas a API nao retornou o codigo PIX copia e cola."}
		result.MessageForAgent = "Nao invente link nem codigo. Informe falha operacional e encaminhe para revisao humana."
		return result, nil
	}

	result.Mode = "pix_sent"
	result.MessageForAgent = "PIX gerado com sucesso. Envie somente o copia e cola ao cliente, sem link do provedor."
	return result, nil
}

func calculatePaymentAmount(totalAmount, amountPaid, remainderAmount float64, paymentType string, depositPerPerson float64, qtyGuess int, paidAmountFallback float64) (string, float64) {
	totalAmount = roundMoney(totalAmount)
	amountPaid = roundMoney(amountPaid)
	remainderAmount = roundMoney(maxFloat(remainderAmount, 0))
	targetDeposit := roundMoney(math.Min(totalAmount, depositPerPerson*float64(qtyGuess)))

	switch normalizePaymentType(paymentType) {
	case "integral":
		stage := "full"
		if amountPaid > 0 {
			stage = "remaining"
		}
		amountDue := remainderAmount
		if amountDue <= 0 {
			amountDue = roundMoney(math.Max(totalAmount-paidAmountFallback, 0))
		}
		return stage, amountDue
	default:
		amountPaidTowardDeposit := roundMoney(math.Min(targetDeposit, amountPaid))
		stage := "deposit"
		if amountPaidTowardDeposit > 0 {
			stage = "remaining_deposit"
		}
		return stage, roundMoney(math.Max(targetDeposit-amountPaidTowardDeposit, 0))
	}
}

func countChargeableBookingPassengers(passengers []bookings.BookingPassenger) int {
	count := 0
	for _, item := range passengers {
		if item.IsLapChild {
			continue
		}
		count++
	}
	return count
}

func resolvePaymentPayerDocument(explicit string, passenger bookings.BookingPassenger) string {
	document := normalizeDigits(explicit)
	if isSupportedPaymentDocument(document) {
		return document
	}
	if strings.EqualFold(strings.TrimSpace(passenger.DocumentType), "CPF") {
		document = normalizeDigits(passenger.Document)
		if isSupportedPaymentDocument(document) {
			return document
		}
	}
	return ""
}

func resolvePaymentPayerPhone(explicit string, passenger bookings.BookingPassenger) string {
	return normalizeDigits(firstNonEmpty(explicit, passenger.Phone))
}

func isSupportedPaymentDocument(document string) bool {
	return payments.IsSupportedDocument(document)
}

func buildMissingPayerDocumentMessage(documentType string) string {
	switch strings.ToUpper(strings.TrimSpace(documentType)) {
	case "RG":
		return "Para gerar o pagamento, preciso do CPF do pagador. O RG pode ser usado na reserva, mas o PIX exige CPF."
	case "CNH":
		return "Para gerar o pagamento, preciso do CPF do pagador. A CNH pode ser usada na reserva, mas o PIX exige CPF."
	case "CERTIDAO_NASCIMENTO":
		return "Para gerar o pagamento, preciso do CPF do pagador. A certidao pode ser usada na reserva, mas o PIX exige CPF."
	default:
		return "Para gerar o pagamento, preciso do CPF do pagador em formato numerico."
	}
}

func isPaymentBlockedBookingStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "CANCELLED", "EXPIRED":
		return true
	default:
		return false
	}
}

func normalizePaymentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "integral":
		return "integral"
	default:
		return "sinal"
	}
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

func maxFloat(values ...float64) float64 {
	max := 0.0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}
