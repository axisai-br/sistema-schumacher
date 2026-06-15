package bookings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSeatNotInTrip    = errors.New("seat does not belong to trip bus")
	ErrNoSeatsAvailable = errors.New("no seats available for segment")
)

// Repository handles booking persistence.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context, filter ListFilter) ([]BookingListItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := `
    select
      b.booking_id as id,
      b.trip_id,
      b.status,
      coalesce(b.reservation_code, '') as reservation_code,
      coalesce(pd.amount_total, 0)::numeric as total_amount,
      coalesce(pd.amount_paid, 0)::numeric as deposit_amount,
      coalesce(pd.amount_due, greatest(coalesce(pd.amount_total, 0) - coalesce(pd.amount_paid, 0), 0))::numeric as remainder_amount,
      coalesce(p.full_name, b.customer_name) as passenger_name,
      coalesce(p.phone, b.customer_phone) as passenger_phone,
      ''::text as passenger_email,
      case when coalesce(p.seat_number, '') ~ '^[0-9]+$' then p.seat_number::int else 0 end as seat_number,
      b.reserved_until,
      b.created_at
    from bookings b
    left join booking_payment_details pd on pd.booking_id = b.booking_id
    left join lateral (
      select full_name, phone, seat_number
      from passengers p
      where p.booking_id = b.booking_id
      order by p.created_at asc
      limit 1
    ) p on true`

	args := []interface{}{}
	clauses := []string{}

	if filter.BookingID != "" && filter.ReservationCode != "" {
		args = append(args, filter.BookingID, filter.ReservationCode)
		clauses = append(clauses, fmt.Sprintf("(b.booking_id = $%d or b.reservation_code = $%d)", len(args)-1, len(args)))
	} else if filter.BookingID != "" {
		args = append(args, filter.BookingID)
		clauses = append(clauses, fmt.Sprintf("b.booking_id = $%d", len(args)))
	} else if filter.ReservationCode != "" {
		args = append(args, filter.ReservationCode)
		clauses = append(clauses, fmt.Sprintf("b.reservation_code = $%d", len(args)))
	}
	if filter.TripID != "" {
		args = append(args, filter.TripID)
		clauses = append(clauses, fmt.Sprintf("b.trip_id = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("b.status = $%d", len(args)))
	}
	if len(clauses) > 0 {
		query += "\n    where " + strings.Join(clauses, " and ")
	}

	args = append(args, limit)
	query += fmt.Sprintf("\n    order by b.created_at desc\n    limit $%d", len(args))
	args = append(args, offset)
	query += fmt.Sprintf(" offset $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []BookingListItem{}
	for rows.Next() {
		var item BookingListItem
		if err := rows.Scan(
			&item.ID,
			&item.TripID,
			&item.Status,
			&item.ReservationCode,
			&item.TotalAmount,
			&item.DepositAmount,
			&item.RemainderAmount,
			&item.PassengerName,
			&item.PassengerPhone,
			&item.PassengerEmail,
			&item.SeatNumber,
			&item.ExpiresAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Create(ctx context.Context, input CreateBookingData) (BookingDetails, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BookingDetails{}, err
	}
	defer tx.Rollback(ctx)

	var tripExists bool
	var seatsTotal int
	if err := tx.QueryRow(ctx, `select true, greatest(coalesce(seats_total, 0), 0) from trips where trip_id = $1`, input.TripID).Scan(&tripExists, &seatsTotal); err != nil {
		return BookingDetails{}, err
	}
	if !tripExists {
		return BookingDetails{}, fmt.Errorf("trip not found")
	}

	seatID := strings.TrimSpace(input.SeatID)
	if seatID != "" {
		if seatsTotal > 0 {
			var seatNum int
			if _, err := fmt.Sscanf(seatID, "%d", &seatNum); err != nil || seatNum <= 0 || seatNum > seatsTotal {
				return BookingDetails{}, ErrSeatNotInTrip
			}
		}
	}

	var booking Booking
	row := tx.QueryRow(ctx, `
    with generated as (
      select 'BK-' || upper(replace(gen_random_uuid()::text, '-', '')) as booking_id
    )
    insert into bookings (
      booking_id, trip_id, origin_stop_id, destination_stop_id,
      passenger_qty, customer_name, customer_phone, status, reservation_code, reserved_until, row_type, idempotency_key,
      created_at, updated_at
    )
    select
      g.booking_id,
      $1, $2, $3,
      $4, $5, $6, 'PENDING',
      right(g.booking_id, 8), now() + interval '50 minutes', 'booking', nullif($7, ''),
      now(), now()
    from generated g
    returning booking_id, trip_id, status, reservation_code, reserved_until, created_at
	  `, input.TripID, input.OriginStopID, input.DestinationStopID, len(input.Passengers), firstPassengerName(input.Passengers), firstPassengerPhone(input.Passengers), input.IdempotencyKey)
	if err := row.Scan(&booking.ID, &booking.TripID, &booking.Status, &booking.ReservationCode, &booking.ExpiresAt, &booking.CreatedAt); err != nil {
		return BookingDetails{}, err
	}
	booking.Source = "WHATSAPP"
	booking.TotalAmount = input.TotalAmount
	booking.DepositAmount = input.DepositAmount
	booking.RemainderAmount = input.RemainderAmount

	passengers := make([]BookingPassenger, 0, len(input.Passengers))
	for _, inputPassenger := range input.Passengers {
		passengerSeatID := seatID
		if passengerSeatID == "" {
			passengerSeatID, err = r.findAvailableSeat(ctx, tx, input.TripID, seatsTotal)
			if err != nil {
				return BookingDetails{}, err
			}
		}

		var passenger BookingPassenger
		row = tx.QueryRow(ctx, `
    with generated as (
      select 'PS-' || upper(replace(gen_random_uuid()::text, '-', '')) as passenger_id
    )
    insert into passengers (
      passenger_id, booking_id, trip_id, full_name, document, document_type, cpf, rg, cnh,
      birth_date, birth_certificate_number, birth_city, seat_number,
      origin_stop_id, destination_stop_id, phone, notes, status, row_type, created_at, updated_at
    )
    select
      g.passenger_id,
      $1, $2, $3, nullif($4, ''), nullif($5, ''), nullif($6, ''), nullif($7, ''), nullif($8, ''),
      nullif($9, '')::date, nullif($10, ''), nullif($11, ''), $12,
      $13, $14, nullif($15, ''), nullif($16, ''), 'RESERVED', 'passenger', now(), now()
    from generated g
    returning passenger_id, booking_id, trip_id, full_name, coalesce(document, ''), coalesce(document_type, ''),
      coalesce(cpf, ''), coalesce(rg, ''), coalesce(cnh, ''),
      coalesce(to_char(birth_date, 'YYYY-MM-DD'), ''), coalesce(birth_certificate_number, ''), coalesce(birth_city, ''),
      coalesce(phone, ''), coalesce(notes, ''), coalesce(seat_number, ''), status, created_at
	  `, booking.ID, input.TripID, inputPassenger.Name, inputPassenger.Document, inputPassenger.DocumentType, inputPassenger.CPF, inputPassenger.RG, inputPassenger.CNH, inputPassenger.BirthDate, inputPassenger.BirthCertificateNumber, inputPassenger.BirthCity, passengerSeatID, input.OriginStopID, input.DestinationStopID, inputPassenger.Phone, inputPassenger.Notes)
		var seatNumber string
		if err := row.Scan(&passenger.ID, &passenger.BookingID, &passenger.TripID, &passenger.Name, &passenger.Document, &passenger.DocumentType, &passenger.CPF, &passenger.RG, &passenger.CNH, &passenger.BirthDate, &passenger.BirthCertificateNumber, &passenger.BirthCity, &passenger.Phone, &passenger.Notes, &seatNumber, &passenger.Status, &passenger.CreatedAt); err != nil {
			return BookingDetails{}, err
		}
		passenger.IsLapChild = isLapChildNotes(passenger.Notes)
		passenger.SeatID = seatNumber
		passenger.Email = inputPassenger.Email
		passenger.BoardStopID = input.BoardStopID
		passenger.AlightStopID = input.AlightStopID
		passenger.FareMode = input.FareMode
		if passenger.IsLapChild {
			passenger.FareAmountCalc = 0
			passenger.FareAmountFinal = 0
		} else {
			passenger.FareAmountCalc = input.FareAmountCalc
			passenger.FareAmountFinal = input.FareAmountFinal
		}

		passengers = append(passengers, passenger)
	}

	if _, err := tx.Exec(ctx, `
    insert into booking_payment_details (
      booking_id, payment_type, payment_status, payment_method,
      amount_total, amount_paid
    ) values ($1, 'PARTIAL', 'PENDING', 'PIX', $2, $3)
    on conflict (booking_id) do update
      set amount_total = excluded.amount_total,
          amount_paid = excluded.amount_paid
  `, booking.ID, input.TotalAmount, input.DepositAmount); err != nil {
		return BookingDetails{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return BookingDetails{}, err
	}

	return bookingDetailsWithPassengers(booking, passengers), nil
}

func (r *Repository) findAvailableSeat(ctx context.Context, tx pgx.Tx, tripID string, seatsTotal int) (string, error) {
	limit := seatsTotal
	if limit <= 0 {
		limit = 50
	}

	var seatID string
	row := tx.QueryRow(ctx, `
    select gs::text as seat_number
    from generate_series(1, $2) as gs
    where not exists (
      select 1
      from passengers p
      join bookings b on b.booking_id = p.booking_id
      where p.trip_id = $1
        and p.seat_number = gs::text
        and upper(coalesce(b.status, '')) not in ('CANCELLED', 'EXPIRED')
    )
    order by gs asc
    limit 1
  `, tripID, limit)
	if err := row.Scan(&seatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNoSeatsAvailable
		}
		return "", err
	}

	return seatID, nil
}

func (r *Repository) Get(ctx context.Context, id string) (BookingDetails, error) {
	var booking Booking
	row := r.pool.QueryRow(ctx, `
    select
      b.booking_id, b.trip_id, b.status, coalesce(b.reservation_code, ''),
      coalesce(pd.amount_total, 0)::numeric as total_amount,
      coalesce(pd.amount_paid, 0)::numeric as deposit_amount,
      coalesce(pd.amount_due, greatest(coalesce(pd.amount_total, 0) - coalesce(pd.amount_paid, 0), 0))::numeric as remainder_amount,
      b.reserved_until,
      b.created_at
    from bookings b
    left join booking_payment_details pd on pd.booking_id = b.booking_id
    where b.booking_id=$1
  `, id)
	if err := row.Scan(&booking.ID, &booking.TripID, &booking.Status, &booking.ReservationCode, &booking.TotalAmount, &booking.DepositAmount, &booking.RemainderAmount, &booking.ExpiresAt, &booking.CreatedAt); err != nil {
		return BookingDetails{}, err
	}
	booking.Source = "WHATSAPP"

	rows, err := r.pool.Query(ctx, `
    select
      p.passenger_id, p.booking_id, p.trip_id, p.full_name, coalesce(p.document, ''), coalesce(p.document_type, ''),
      coalesce(p.cpf, ''), coalesce(p.rg, ''), coalesce(p.cnh, ''),
      coalesce(to_char(p.birth_date, 'YYYY-MM-DD'), ''), coalesce(p.birth_certificate_number, ''), coalesce(p.birth_city, ''),
      coalesce(p.phone, ''),
      ''::text as email,
      coalesce(p.notes, '') as notes,
      case when position('CRIANCA_DE_COLO_ATE_5_ANOS' in upper(coalesce(p.notes, ''))) > 0 then true else false end as is_lap_child,
      coalesce(p.seat_number, '') as seat_id,
      coalesce(p.origin_stop_id, ''),
      coalesce(p.destination_stop_id, ''),
      0::int as board_stop_order,
      0::int as alight_stop_order,
      'AUTO'::text as fare_mode,
      case
        when position('CRIANCA_DE_COLO_ATE_5_ANOS' in upper(coalesce(p.notes, ''))) > 0 then 0::numeric
        else (coalesce(pd.amount_total, 0) / greatest(coalesce(pc.chargeable_count, 0), 1))::numeric
      end as fare_amount_calc,
      case
        when position('CRIANCA_DE_COLO_ATE_5_ANOS' in upper(coalesce(p.notes, ''))) > 0 then 0::numeric
        else (coalesce(pd.amount_total, 0) / greatest(coalesce(pc.chargeable_count, 0), 1))::numeric
      end as fare_amount_final,
      p.status,
      p.created_at
    from passengers p
    join bookings b on b.booking_id = p.booking_id
    left join booking_payment_details pd on pd.booking_id = p.booking_id
    left join lateral (
      select count(*)::numeric as chargeable_count
      from passengers px
      where px.booking_id = p.booking_id
        and position('CRIANCA_DE_COLO_ATE_5_ANOS' in upper(coalesce(px.notes, ''))) = 0
    ) pc on true
    where p.booking_id=$1
    order by p.created_at asc`, id)
	if err != nil {
		return BookingDetails{}, err
	}
	defer rows.Close()

	passengers := []BookingPassenger{}
	for rows.Next() {
		var passenger BookingPassenger
		if err := rows.Scan(
			&passenger.ID, &passenger.BookingID, &passenger.TripID, &passenger.Name, &passenger.Document, &passenger.DocumentType,
			&passenger.CPF, &passenger.RG, &passenger.CNH, &passenger.BirthDate, &passenger.BirthCertificateNumber, &passenger.BirthCity,
			&passenger.Phone, &passenger.Email, &passenger.Notes, &passenger.IsLapChild, &passenger.SeatID,
			&passenger.BoardStopID, &passenger.AlightStopID, &passenger.BoardStopOrder, &passenger.AlightStopOrder,
			&passenger.FareMode, &passenger.FareAmountCalc, &passenger.FareAmountFinal, &passenger.Status, &passenger.CreatedAt,
		); err != nil {
			return BookingDetails{}, err
		}
		passengers = append(passengers, passenger)
	}
	if err := rows.Err(); err != nil {
		return BookingDetails{}, err
	}

	return bookingDetailsWithPassengers(booking, passengers), nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status string) (BookingDetails, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BookingDetails{}, err
	}
	defer tx.Rollback(ctx)

	var booking Booking
	row := tx.QueryRow(ctx, `
    update bookings
    set status=$1, updated_at=now()
    where booking_id=$2
    returning booking_id, trip_id, status, created_at
  `, status, id)
	if err := row.Scan(&booking.ID, &booking.TripID, &booking.Status, &booking.CreatedAt); err != nil {
		return BookingDetails{}, err
	}
	booking.Source = "WHATSAPP"

	if status == "CANCELLED" || status == "EXPIRED" {
		if _, err := tx.Exec(ctx, `update passengers set status='CANCELLED', updated_at=now() where booking_id=$1`, id); err != nil {
			return BookingDetails{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return BookingDetails{}, err
	}

	return r.Get(ctx, id)
}

func IsUniqueViolation(err error) bool {
	pgErr, ok := err.(*pgconn.PgError)
	if !ok {
		return false
	}
	return pgErr.Code == "23505" || pgErr.Code == "23P01"
}

func bookingDetailsWithPassengers(booking Booking, passengers []BookingPassenger) BookingDetails {
	details := BookingDetails{
		Booking:    booking,
		Passengers: passengers,
	}
	if len(passengers) > 0 {
		details.Passenger = passengers[0]
	}
	return details
}

func firstPassengerName(passengers []PassengerInput) string {
	if len(passengers) == 0 {
		return ""
	}
	return strings.TrimSpace(passengers[0].Name)
}

func firstPassengerPhone(passengers []PassengerInput) string {
	if len(passengers) == 0 {
		return ""
	}
	return strings.TrimSpace(passengers[0].Phone)
}
