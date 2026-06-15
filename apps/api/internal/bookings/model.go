package bookings

import (
	"encoding/json"
	"time"
)

type Booking struct {
	ID              string     `json:"id"`
	TripID          string     `json:"trip_id"`
	Status          string     `json:"status"`
	ReservationCode string     `json:"reservation_code"`
	Source          string     `json:"source"`
	TotalAmount     float64    `json:"total_amount"`
	DepositAmount   float64    `json:"deposit_amount"`
	RemainderAmount float64    `json:"remainder_amount"`
	ExpiresAt       *time.Time `json:"expires_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type BookingListItem struct {
	ID              string     `json:"id"`
	TripID          string     `json:"trip_id"`
	Status          string     `json:"status"`
	ReservationCode string     `json:"reservation_code"`
	TotalAmount     float64    `json:"total_amount"`
	DepositAmount   float64    `json:"deposit_amount"`
	RemainderAmount float64    `json:"remainder_amount"`
	PassengerName   string     `json:"passenger_name"`
	PassengerPhone  string     `json:"passenger_phone"`
	PassengerEmail  string     `json:"passenger_email"`
	SeatNumber      int        `json:"seat_number"`
	ExpiresAt       *time.Time `json:"expires_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type PassengerInput struct {
	Name                   string `json:"name"`
	Document               string `json:"document"`
	DocumentType           string `json:"document_type"`
	CPF                    string `json:"cpf"`
	RG                     string `json:"rg"`
	CNH                    string `json:"cnh"`
	BirthDate              string `json:"birth_date"`
	BirthCertificateNumber string `json:"birth_certificate_number"`
	BirthCity              string `json:"birth_city"`
	Phone                  string `json:"phone"`
	Email                  string `json:"email"`
	Notes                  string `json:"notes"`
	IsLapChild             bool   `json:"is_lap_child"`
}

type CreateBookingInput struct {
	TripID          string           `json:"trip_id"`
	SeatID          string           `json:"seat_id"`
	BoardStopID     string           `json:"board_stop_id"`
	AlightStopID    string           `json:"alight_stop_id"`
	FareMode        *string          `json:"fare_mode"`
	FareAmountFinal *float64         `json:"fare_amount_final"`
	Passenger       PassengerInput   `json:"passenger"`
	Passengers      []PassengerInput `json:"passengers"`
	IdempotencyKey  string           `json:"idempotency_key"`
	Source          *string          `json:"source"`
	TotalAmount     float64          `json:"total_amount"`
	DepositAmount   float64          `json:"deposit_amount"`
	RemainderAmount float64          `json:"remainder_amount"`
}

type CheckoutCustomerInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Document string `json:"document"`
}

type CheckoutPaymentInput struct {
	Method      string                 `json:"method"`
	Amount      float64                `json:"amount"`
	Description string                 `json:"description"`
	Notes       string                 `json:"notes"`
	Customer    *CheckoutCustomerInput `json:"customer"`
}

type CheckoutBookingInput struct {
	TripID          string               `json:"trip_id"`
	SeatID          string               `json:"seat_id"`
	BoardStopID     string               `json:"board_stop_id"`
	AlightStopID    string               `json:"alight_stop_id"`
	FareMode        *string              `json:"fare_mode"`
	FareAmountFinal *float64             `json:"fare_amount_final"`
	Passenger       PassengerInput       `json:"passenger"`
	Passengers      []PassengerInput     `json:"passengers"`
	IdempotencyKey  string               `json:"idempotency_key"`
	Source          *string              `json:"source"`
	TotalAmount     float64              `json:"total_amount"`
	DepositAmount   float64              `json:"deposit_amount"`
	RemainderAmount float64              `json:"remainder_amount"`
	InitialPayment  CheckoutPaymentInput `json:"initial_payment"`
}

type UpdateBookingInput struct {
	Status *string `json:"status"`
}

type ListFilter struct {
	Limit           int
	Offset          int
	BookingID       string
	ReservationCode string
	TripID          string
	Status          string
}

type BookingPassenger struct {
	ID                     string    `json:"id"`
	BookingID              string    `json:"booking_id"`
	TripID                 string    `json:"trip_id"`
	Name                   string    `json:"name"`
	Document               string    `json:"document"`
	DocumentType           string    `json:"document_type"`
	CPF                    string    `json:"cpf"`
	RG                     string    `json:"rg"`
	CNH                    string    `json:"cnh"`
	BirthDate              string    `json:"birth_date"`
	BirthCertificateNumber string    `json:"birth_certificate_number"`
	BirthCity              string    `json:"birth_city"`
	Phone                  string    `json:"phone"`
	Email                  string    `json:"email"`
	Notes                  string    `json:"notes"`
	IsLapChild             bool      `json:"is_lap_child"`
	SeatID                 string    `json:"seat_id"`
	BoardStopID            string    `json:"board_stop_id"`
	AlightStopID           string    `json:"alight_stop_id"`
	BoardStopOrder         int       `json:"board_stop_order"`
	AlightStopOrder        int       `json:"alight_stop_order"`
	FareMode               string    `json:"fare_mode"`
	FareAmountCalc         float64   `json:"fare_amount_calc"`
	FareAmountFinal        float64   `json:"fare_amount_final"`
	Status                 string    `json:"status"`
	CreatedAt              time.Time `json:"created_at"`
}

type BookingDetails struct {
	Booking    Booking            `json:"booking"`
	Passenger  BookingPassenger   `json:"passenger"`
	Passengers []BookingPassenger `json:"passengers"`
}

type CheckoutPayment struct {
	ID          string          `json:"id"`
	BookingID   string          `json:"booking_id"`
	Amount      float64         `json:"amount"`
	Method      string          `json:"method"`
	Status      string          `json:"status"`
	Provider    *string         `json:"provider"`
	ProviderRef *string         `json:"provider_ref"`
	PaidAt      *time.Time      `json:"paid_at"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
}

type CheckoutResponse struct {
	Booking     BookingDetails  `json:"booking"`
	Payment     CheckoutPayment `json:"payment"`
	ProviderRaw interface{}     `json:"provider_raw,omitempty"`
	CheckoutURL *string         `json:"checkout_url"`
	PixCode     *string         `json:"pix_code"`
}

type CreateBookingData struct {
	TripID            string
	SeatID            string
	BoardStopID       string
	AlightStopID      string
	OriginStopID      string
	DestinationStopID string
	BoardStopOrder    int
	AlightStopOrder   int
	FareMode          string
	FareAmountCalc    float64
	FareAmountFinal   float64
	FareSnapshot      []byte
	Passengers        []PassengerInput
	IdempotencyKey    string
	Source            *string
	TotalAmount       float64
	DepositAmount     float64
	RemainderAmount   float64
}
