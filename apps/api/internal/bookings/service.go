package bookings

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/pricing"
)

var (
	ErrInvalidAmounts              = errors.New("deposit + remainder must equal total_amount")
	ErrNegativeAmounts             = errors.New("amounts cannot be negative")
	ErrMissingStops                = errors.New("board_stop_id and alight_stop_id are required")
	ErrMissingFields               = errors.New("trip_id and at least one passenger.name are required")
	ErrPassengerDocumentType       = errors.New("passenger document_type must be CPF, RG, CNH or CERTIDAO_NASCIMENTO")
	ErrPassengerNameRequired       = errors.New("every passenger must include name")
	ErrSeatRequiresSinglePassenger = errors.New("seat_id can be used only with a single passenger")
	ErrInitialPaymentRequired      = errors.New("initial_payment is required")
	ErrInvalidInitialPayment       = errors.New("initial payment method or amount is invalid")
	ErrInitialPaymentBelowMinimum  = errors.New("initial_payment.amount must be at least 30% of total_amount")
)

const lapChildPassengerNote = "CRIANCA_DE_COLO_ATE_5_ANOS"

type Service struct {
	repo     *Repository
	pricing  *pricing.Service
	payments *payments.Service
}

func NewService(repo *Repository, pricingSvc *pricing.Service, paymentsSvc *payments.Service) *Service {
	return &Service{repo: repo, pricing: pricingSvc, payments: paymentsSvc}
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]BookingListItem, error) {
	return s.repo.List(ctx, filter)
}

func (s *Service) Create(ctx context.Context, input CreateBookingInput) (BookingDetails, error) {
	passengers := normalizePassengers(input.Passenger, input.Passengers)
	if input.TripID == "" || len(passengers) == 0 {
		return BookingDetails{}, ErrMissingFields
	}
	for _, passenger := range passengers {
		if strings.TrimSpace(passenger.Name) == "" {
			return BookingDetails{}, ErrPassengerNameRequired
		}
		if passenger.DocumentType != "" && !isSupportedPassengerDocumentType(passenger.DocumentType) {
			return BookingDetails{}, ErrPassengerDocumentType
		}
	}
	if strings.TrimSpace(input.SeatID) != "" && len(passengers) > 1 {
		return BookingDetails{}, ErrSeatRequiresSinglePassenger
	}
	if input.BoardStopID == "" || input.AlightStopID == "" {
		return BookingDetails{}, ErrMissingStops
	}
	if input.TotalAmount < 0 || input.DepositAmount < 0 || input.RemainderAmount < 0 {
		return BookingDetails{}, ErrNegativeAmounts
	}

	fareMode := "AUTO"
	if input.FareMode != nil && *input.FareMode != "" {
		fareMode = *input.FareMode
	}

	quote, err := s.pricing.Quote(ctx, pricing.QuoteInput{
		TripID:          input.TripID,
		BoardStopID:     input.BoardStopID,
		AlightStopID:    input.AlightStopID,
		FareMode:        fareMode,
		FareAmountFinal: input.FareAmountFinal,
	})
	if err != nil {
		return BookingDetails{}, err
	}

	total := roundTo2(quote.FinalAmount * float64(countChargeablePassengers(passengers)))
	deposit := input.DepositAmount
	remainder := input.RemainderAmount
	if deposit == 0 && remainder == 0 {
		remainder = total
	}
	if total > 0 {
		sum := deposit + remainder
		if math.Abs(sum-total) > 0.01 {
			return BookingDetails{}, ErrInvalidAmounts
		}
	}

	data := CreateBookingData{
		TripID:            input.TripID,
		SeatID:            input.SeatID,
		BoardStopID:       input.BoardStopID,
		AlightStopID:      input.AlightStopID,
		OriginStopID:      quote.OriginStopID,
		DestinationStopID: quote.DestinationStopID,
		BoardStopOrder:    quote.BoardStopOrder,
		AlightStopOrder:   quote.AlightStopOrder,
		FareMode:          quote.FareMode,
		FareAmountCalc:    quote.CalcAmount,
		FareAmountFinal:   quote.FinalAmount,
		FareSnapshot:      quote.Snapshot,
		Passengers:        passengers,
		IdempotencyKey:    strings.TrimSpace(input.IdempotencyKey),
		Source:            input.Source,
		TotalAmount:       total,
		DepositAmount:     deposit,
		RemainderAmount:   remainder,
	}

	return s.repo.Create(ctx, data)
}

func (s *Service) Get(ctx context.Context, id string) (BookingDetails, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) UpdateStatus(ctx context.Context, id string, status string) (BookingDetails, error) {
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *Service) Checkout(ctx context.Context, input CheckoutBookingInput) (CheckoutResponse, error) {
	if strings.TrimSpace(input.InitialPayment.Method) == "" || input.InitialPayment.Amount <= 0 {
		return CheckoutResponse{}, ErrInvalidInitialPayment
	}
	if !isCheckoutPaymentMethod(input.InitialPayment.Method) {
		return CheckoutResponse{}, ErrInvalidInitialPayment
	}

	minRequired := roundTo2(math.Max(input.TotalAmount*0.30, 0.01))
	if roundTo2(input.InitialPayment.Amount) < minRequired {
		return CheckoutResponse{}, ErrInitialPaymentBelowMinimum
	}

	deposit := roundTo2(input.InitialPayment.Amount)
	remainder := roundTo2(math.Max(input.TotalAmount-deposit, 0))
	booking, err := s.Create(ctx, CreateBookingInput{
		TripID:          input.TripID,
		SeatID:          input.SeatID,
		BoardStopID:     input.BoardStopID,
		AlightStopID:    input.AlightStopID,
		FareMode:        input.FareMode,
		FareAmountFinal: input.FareAmountFinal,
		Passenger:       input.Passenger,
		Passengers:      input.Passengers,
		IdempotencyKey:  input.IdempotencyKey,
		Source:          input.Source,
		TotalAmount:     input.TotalAmount,
		DepositAmount:   deposit,
		RemainderAmount: remainder,
	})
	if err != nil {
		return CheckoutResponse{}, err
	}
	method := strings.ToUpper(strings.TrimSpace(input.InitialPayment.Method))
	payment := CheckoutPayment{
		ID:        "",
		BookingID: booking.Booking.ID,
		Amount:    deposit,
		Method:    method,
		Status:    "PENDING",
		CreatedAt: booking.Booking.CreatedAt,
	}

	return CheckoutResponse{
		Booking:     booking,
		Payment:     payment,
		ProviderRaw: nil,
		CheckoutURL: nil,
		PixCode:     nil,
	}, nil
}

func mapCustomer(input *CheckoutCustomerInput) *payments.CustomerInput {
	if input == nil {
		return nil
	}
	return &payments.CustomerInput{
		Name:     input.Name,
		Email:    input.Email,
		Phone:    input.Phone,
		Document: input.Document,
	}
}

func mapPayment(payment payments.Payment) CheckoutPayment {
	return CheckoutPayment{
		ID:          payment.ID,
		BookingID:   payment.BookingID,
		Amount:      payment.Amount,
		Method:      payment.Method,
		Status:      payment.Status,
		Provider:    payment.Provider,
		ProviderRef: payment.ProviderRef,
		PaidAt:      payment.PaidAt,
		Metadata:    payment.Metadata,
		CreatedAt:   payment.CreatedAt,
	}
}

func parseProviderData(raw []byte) (interface{}, *string, *string) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	var parsed interface{}
	_ = json.Unmarshal(raw, &parsed)

	obj, ok := parsed.(map[string]interface{})
	if !ok {
		return parsed, nil, nil
	}

	checkout := readStringFromMap(obj, "url")
	if checkout == nil {
		if dataObj, ok := obj["data"].(map[string]interface{}); ok {
			checkout = readStringFromMap(dataObj, "url")
		}
	}

	var pixCode *string
	if dataObj, ok := obj["data"].(map[string]interface{}); ok {
		pixCode = readStringFromMap(dataObj, "pixQrCode")
	}
	if pixCode == nil {
		pixCode = readStringFromMap(obj, "pixQrCode")
	}

	return parsed, checkout, pixCode
}

func readStringFromMap(obj map[string]interface{}, key string) *string {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func isAutomaticCheckoutMethod(method string) bool {
	switch method {
	case "PIX", "CARD":
		return true
	default:
		return false
	}
}

func isCheckoutPaymentMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "PIX", "CARD", "CASH", "TRANSFER", "OTHER":
		return true
	default:
		return false
	}
}

func roundTo2(value float64) float64 {
	return math.Round(value*100) / 100
}

func normalizePassengers(primary PassengerInput, list []PassengerInput) []PassengerInput {
	if len(list) > 0 {
		passengers := make([]PassengerInput, 0, len(list))
		for _, passenger := range list {
			passengers = append(passengers, normalizePassengerIdentityFields(PassengerInput{
				Name:                   strings.TrimSpace(passenger.Name),
				Document:               strings.TrimSpace(passenger.Document),
				DocumentType:           normalizePassengerDocumentType(passenger.DocumentType, passenger.Document),
				CPF:                    strings.TrimSpace(passenger.CPF),
				RG:                     strings.TrimSpace(passenger.RG),
				CNH:                    strings.TrimSpace(passenger.CNH),
				BirthDate:              normalizePassengerBirthDate(passenger.BirthDate),
				BirthCertificateNumber: strings.TrimSpace(passenger.BirthCertificateNumber),
				BirthCity:              strings.TrimSpace(passenger.BirthCity),
				Phone:                  strings.TrimSpace(passenger.Phone),
				Email:                  strings.TrimSpace(passenger.Email),
				Notes:                  mergePassengerNotes(passenger.Notes, passenger.IsLapChild),
				IsLapChild:             passenger.IsLapChild,
			}))
		}
		return passengers
	}
	if !hasPassengerPayload(primary) {
		return nil
	}
	return []PassengerInput{normalizePassengerIdentityFields(PassengerInput{
		Name:                   strings.TrimSpace(primary.Name),
		Document:               strings.TrimSpace(primary.Document),
		DocumentType:           normalizePassengerDocumentType(primary.DocumentType, primary.Document),
		CPF:                    strings.TrimSpace(primary.CPF),
		RG:                     strings.TrimSpace(primary.RG),
		CNH:                    strings.TrimSpace(primary.CNH),
		BirthDate:              normalizePassengerBirthDate(primary.BirthDate),
		BirthCertificateNumber: strings.TrimSpace(primary.BirthCertificateNumber),
		BirthCity:              strings.TrimSpace(primary.BirthCity),
		Phone:                  strings.TrimSpace(primary.Phone),
		Email:                  strings.TrimSpace(primary.Email),
		Notes:                  mergePassengerNotes(primary.Notes, primary.IsLapChild),
		IsLapChild:             primary.IsLapChild,
	})}
}

func normalizePassengerIdentityFields(passenger PassengerInput) PassengerInput {
	switch passenger.DocumentType {
	case "CPF":
		if cpf := normalizePassengerCPF(passenger.CPF); cpf != "" {
			passenger.CPF = cpf
		} else {
			passenger.CPF = normalizePassengerCPF(passenger.Document)
		}
		if passenger.CPF != "" {
			passenger.Document = passenger.CPF
		}
	case "RG":
		if passenger.RG == "" {
			passenger.RG = passenger.Document
		}
	case "CNH":
		if passenger.CNH == "" {
			passenger.CNH = passenger.Document
		}
	case "CERTIDAO_NASCIMENTO":
		if passenger.BirthCertificateNumber == "" {
			passenger.BirthCertificateNumber = passenger.Document
		}
	}
	return passenger
}

func normalizePassengerCPF(value string) string {
	digits := passengerDocumentDigits(value)
	if len(digits) != 11 || !payments.IsSupportedDocument(digits) {
		return ""
	}
	return digits
}

func passengerDocumentDigits(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func normalizePassengerBirthDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "02-01-2006", time.RFC3339, time.RFC3339Nano} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}

func hasPassengerPayload(passenger PassengerInput) bool {
	return strings.TrimSpace(passenger.Name) != "" ||
		strings.TrimSpace(passenger.Document) != "" ||
		strings.TrimSpace(passenger.DocumentType) != "" ||
		strings.TrimSpace(passenger.CPF) != "" ||
		strings.TrimSpace(passenger.RG) != "" ||
		strings.TrimSpace(passenger.CNH) != "" ||
		strings.TrimSpace(passenger.BirthDate) != "" ||
		strings.TrimSpace(passenger.BirthCertificateNumber) != "" ||
		strings.TrimSpace(passenger.BirthCity) != "" ||
		strings.TrimSpace(passenger.Phone) != "" ||
		strings.TrimSpace(passenger.Email) != "" ||
		passenger.IsLapChild
}

func countChargeablePassengers(passengers []PassengerInput) int {
	count := 0
	for _, passenger := range passengers {
		if passenger.IsLapChild {
			continue
		}
		count++
	}
	return count
}

func mergePassengerNotes(notes string, isLapChild bool) string {
	notes = strings.TrimSpace(notes)
	if !isLapChild {
		return notes
	}
	if notes == "" {
		return lapChildPassengerNote
	}
	if strings.Contains(strings.ToUpper(notes), lapChildPassengerNote) {
		return notes
	}
	return notes + " | " + lapChildPassengerNote
}

func isLapChildNotes(notes string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(notes)), lapChildPassengerNote)
}

func normalizePassengerDocumentType(documentType, document string) string {
	normalizedType := canonicalPassengerDocumentType(documentType)
	if normalizedType != "" {
		return normalizedType
	}

	trimmedDocument := strings.TrimSpace(document)
	if trimmedDocument == "" {
		return ""
	}
	if isCPFDocument(trimmedDocument) {
		return "CPF"
	}
	return "RG"
}

func isSupportedPassengerDocumentType(documentType string) bool {
	return canonicalPassengerDocumentType(documentType) != ""
}

func canonicalPassengerDocumentType(documentType string) string {
	normalized := strings.ToUpper(strings.TrimSpace(documentType))
	normalized = strings.NewReplacer(" ", "_", "-", "_").Replace(normalized)

	switch normalized {
	case "CPF", "RG", "CNH":
		return normalized
	case "CERTIDAO", "CERTIDAO_NASCIMENTO", "CERTIDAO_DE_NASCIMENTO", "MATRICULA", "MATRICULA_CERTIDAO":
		return "CERTIDAO_NASCIMENTO"
	default:
		return ""
	}
}

func isCPFDocument(document string) bool {
	digits := 0
	for _, r := range strings.TrimSpace(document) {
		if r < '0' || r > '9' {
			if r == '.' || r == '-' || r == '/' || r == ' ' {
				continue
			}
			return false
		}
		digits++
	}
	return digits == 11
}
