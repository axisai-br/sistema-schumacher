package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

type BookingPaymentContext struct {
	BookingID       string
	ReservationCode string
	TripID          string
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateWithProvider(ctx context.Context, paymentID, bookingID string, amount float64, method string, provider string, providerRef string, metadata json.RawMessage) (Payment, error) {
	var item Payment
	row := r.pool.QueryRow(ctx,
		`insert into payments (id, booking_id, amount, method, status, provider, provider_ref, metadata)
         values ($1, $2, $3, $4, 'PENDING', $5, $6, $7)
         returning id, booking_id, amount, method, status, provider, provider_ref, paid_at, metadata, created_at`,
		paymentID, bookingID, amount, method, provider, providerRef, metadata,
	)
	if err := row.Scan(&item.ID, &item.BookingID, &item.Amount, &item.Method, &item.Status, &item.Provider, &item.ProviderRef, &item.PaidAt, &item.Metadata, &item.CreatedAt); err != nil {
		return item, err
	}
	return item, nil
}

func (r *Repository) CreateManual(ctx context.Context, bookingID string, amount float64, method string, notes string) (Payment, error) {
	metadata, _ := json.Marshal(map[string]string{"notes": notes})
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Payment{}, err
	}
	defer tx.Rollback(ctx)

	var item Payment
	row := tx.QueryRow(ctx,
		`insert into payments (booking_id, amount, method, status, paid_at, provider, metadata)
         values ($1, $2, $3, 'PAID', now(), 'MANUAL', $4)
         returning id, booking_id, amount, method, status, provider, provider_ref, paid_at, metadata, created_at`,
		bookingID, amount, method, metadata,
	)
	if err := row.Scan(&item.ID, &item.BookingID, &item.Amount, &item.Method, &item.Status, &item.Provider, &item.ProviderRef, &item.PaidAt, &item.Metadata, &item.CreatedAt); err != nil {
		return item, err
	}
	if err := r.updateBookingStatusIfPaid(ctx, tx, bookingID, item.ID, "manual.paid", metadata); err != nil {
		return item, err
	}
	if err := tx.Commit(ctx); err != nil {
		return item, err
	}
	return item, nil
}

func (r *Repository) Get(ctx context.Context, id string) (Payment, error) {
	var item Payment
	row := r.pool.QueryRow(ctx, `select id, booking_id, amount, method, status, provider, provider_ref, paid_at, metadata, created_at from payments where id=$1`, id)
	if err := row.Scan(&item.ID, &item.BookingID, &item.Amount, &item.Method, &item.Status, &item.Provider, &item.ProviderRef, &item.PaidAt, &item.Metadata, &item.CreatedAt); err != nil {
		return item, err
	}
	return item, nil
}

func (r *Repository) GetByProviderRef(ctx context.Context, providerRef string) (Payment, error) {
	var item Payment
	row := r.pool.QueryRow(ctx, `select id, booking_id, amount, method, status, provider, provider_ref, paid_at, metadata, created_at from payments where provider_ref=$1`, providerRef)
	if err := row.Scan(&item.ID, &item.BookingID, &item.Amount, &item.Method, &item.Status, &item.Provider, &item.ProviderRef, &item.PaidAt, &item.Metadata, &item.CreatedAt); err != nil {
		return item, err
	}
	return item, nil
}

func (r *Repository) GetBookingStatus(ctx context.Context, bookingID string) (string, error) {
	var status string
	if err := r.pool.QueryRow(ctx, `select status from bookings where booking_id=$1`, bookingID).Scan(&status); err != nil {
		return "", err
	}
	return status, nil
}

func (r *Repository) GetBookingPaymentContext(ctx context.Context, bookingID string) (BookingPaymentContext, error) {
	var item BookingPaymentContext
	row := r.pool.QueryRow(ctx, `
        select booking_id, coalesce(reservation_code, ''), coalesce(trip_id, '')
        from bookings
        where booking_id=$1`, bookingID)
	if err := row.Scan(&item.BookingID, &item.ReservationCode, &item.TripID); err != nil {
		return item, err
	}
	return item, nil
}

func (r *Repository) GetBookingNotificationContext(ctx context.Context, bookingID string) (PaymentNotificationContext, error) {
	var item PaymentNotificationContext
	row := r.pool.QueryRow(ctx, `
        select
            b.booking_id,
            coalesce(b.reservation_code, ''),
            coalesce(nullif(trim(b.customer_name), ''), p.full_name, ''),
            coalesce(nullif(trim(b.customer_phone), ''), p.phone, ''),
            coalesce(pd.amount_total, 0)::numeric as amount_total,
            coalesce(pd.amount_paid, 0)::numeric as amount_paid,
            coalesce(pd.amount_due, greatest(coalesce(pd.amount_total, 0) - coalesce(pd.amount_paid, 0), 0))::numeric as amount_due,
            coalesce(pd.payment_status, '')
        from bookings b
        left join booking_payment_details pd on pd.booking_id = b.booking_id
        left join lateral (
            select coalesce(full_name, '') as full_name, coalesce(phone, '') as phone
            from passengers p
            where p.booking_id = b.booking_id
            order by p.created_at asc
            limit 1
        ) p on true
        where b.booking_id=$1`, bookingID)
	if err := row.Scan(
		&item.BookingID,
		&item.ReservationCode,
		&item.CustomerName,
		&item.CustomerPhone,
		&item.AmountTotal,
		&item.AmountPaid,
		&item.AmountDue,
		&item.PaymentStatus,
	); err != nil {
		return item, err
	}
	return item, nil
}

func (r *Repository) List(ctx context.Context, filter PaymentListFilter) ([]Payment, error) {
	query := `select id, booking_id, amount, method, status, provider, provider_ref, paid_at, metadata, created_at from payments`
	args := []interface{}{}
	clauses := []string{}

	if filter.BookingID != "" {
		args = append(args, filter.BookingID)
		clauses = append(clauses, fmt.Sprintf("booking_id=$%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("status=$%d", len(args)))
	}
	if filter.Since != nil {
		args = append(args, *filter.Since)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if filter.Until != nil {
		args = append(args, *filter.Until)
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if filter.PaidSince != nil {
		args = append(args, *filter.PaidSince)
		clauses = append(clauses, fmt.Sprintf("paid_at >= $%d", len(args)))
	}
	if filter.PaidUntil != nil {
		args = append(args, *filter.PaidUntil)
		clauses = append(clauses, fmt.Sprintf("paid_at <= $%d", len(args)))
	}
	if len(clauses) > 0 {
		query += " where " + strings.Join(clauses, " and ")
	}
	if filter.PaidSince != nil || filter.PaidUntil != nil {
		query += " order by paid_at desc nulls last"
	} else {
		query += " order by created_at desc"
	}

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(" limit $%d", len(args))
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		query += fmt.Sprintf(" offset $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Payment{}
	for rows.Next() {
		var item Payment
		if err := rows.Scan(&item.ID, &item.BookingID, &item.Amount, &item.Method, &item.Status, &item.Provider, &item.ProviderRef, &item.PaidAt, &item.Metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) AddEvent(ctx context.Context, paymentID *string, event string, payload json.RawMessage) error {
	if paymentID == nil {
		_, err := r.pool.Exec(ctx, `insert into payment_events (event, payload) values ($1,$2)`, event, payload)
		return err
	}
	_, err := r.pool.Exec(ctx, `insert into payment_events (payment_id, event, payload) values ($1,$2,$3)`, *paymentID, event, payload)
	return err
}

// MarcarCancelado marca o pagamento como CANCELLED (se ainda nao pago) e
// registra o evento.
func (r *Repository) MarcarCancelado(ctx context.Context, paymentID string, payload json.RawMessage) error {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if _, err := r.pool.Exec(ctx, `update payments set status='CANCELLED' where id=$1 and status not in ('PAID','CANCELLED')`, paymentID); err != nil {
		return err
	}
	return r.AddEvent(ctx, &paymentID, "atendimento.pix_cancelado", payload)
}

// DefinirSinalReserva grava o valor que confirma a reserva (amount_paid antes
// de qualquer pagamento e o sinal/total exigido; ver updateBookingStatusIfPaid).
// So muda enquanto o pagamento da reserva esta PENDING.
func (r *Repository) DefinirSinalReserva(ctx context.Context, bookingID string, valor float64) error {
	_, err := r.pool.Exec(ctx, `update booking_payment_details set amount_paid=$2 where booking_id=$1 and payment_status='PENDING'`, bookingID, valor)
	return err
}

func (r *Repository) MarkPaidAndConfirmBooking(ctx context.Context, providerRef string, payload json.RawMessage) (Payment, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Payment{}, false, err
	}
	defer tx.Rollback(ctx)

	var payment Payment
	row := tx.QueryRow(ctx,
		`select id, booking_id, amount, method, status, provider, provider_ref, paid_at, metadata, created_at
         from payments
         where provider_ref=$1
         for update`,
		providerRef,
	)
	if err := row.Scan(&payment.ID, &payment.BookingID, &payment.Amount, &payment.Method, &payment.Status, &payment.Provider, &payment.ProviderRef, &payment.PaidAt, &payment.Metadata, &payment.CreatedAt); err != nil {
		return Payment{}, false, err
	}

	justMarkedPaid := payment.Status != "PAID"
	if justMarkedPaid {
		row = tx.QueryRow(ctx,
			`update payments set status='PAID', paid_at=coalesce(paid_at, now()) where id=$1
             returning id, booking_id, amount, method, status, provider, provider_ref, paid_at, metadata, created_at`,
			payment.ID,
		)
		if err := row.Scan(&payment.ID, &payment.BookingID, &payment.Amount, &payment.Method, &payment.Status, &payment.Provider, &payment.ProviderRef, &payment.PaidAt, &payment.Metadata, &payment.CreatedAt); err != nil {
			return Payment{}, false, err
		}
	}
	if err := r.updateBookingStatusIfPaid(ctx, tx, payment.BookingID, payment.ID, "billing.paid", payload); err != nil {
		return Payment{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, false, err
	}
	return payment, justMarkedPaid, nil
}

func (r *Repository) updateBookingStatusIfPaid(ctx context.Context, tx pgx.Tx, bookingID string, paymentID string, event string, payload json.RawMessage) error {
	var totalAmount float64
	var depositAmount float64
	var status string
	if err := tx.QueryRow(ctx, `
        select
            coalesce(pd.amount_total, 0)::numeric as total_amount,
            coalesce(pd.amount_paid, 0)::numeric as deposit_amount,
            b.status
        from bookings b
        left join booking_payment_details pd on pd.booking_id = b.booking_id
        where b.booking_id=$1`, bookingID).Scan(&totalAmount, &depositAmount, &status); err != nil {
		return err
	}

	var paidAmount float64
	if err := tx.QueryRow(ctx, `select coalesce(sum(amount), 0)::numeric from payments where booking_id=$1 and status='PAID'`, bookingID).Scan(&paidAmount); err != nil {
		return err
	}

	paymentStatus := "PENDING"
	if paidAmount > 0 {
		paymentStatus = "PARTIAL"
	}
	if totalAmount > 0 && paidAmount >= totalAmount {
		paymentStatus = "PAID"
	}
	if _, err := tx.Exec(ctx, `
        insert into booking_payment_details (
            booking_id, payment_type, payment_status, payment_method, amount_total, amount_paid
        ) values ($1, 'PARTIAL', $2, 'PIX', $3, $4)
        on conflict (booking_id) do update
        set payment_status = excluded.payment_status,
            amount_total = excluded.amount_total,
            amount_paid = excluded.amount_paid`, bookingID, paymentStatus, totalAmount, paidAmount); err != nil {
		return err
	}

	shouldConfirm := false
	if depositAmount > 0 {
		shouldConfirm = paidAmount >= depositAmount
	} else if totalAmount > 0 {
		shouldConfirm = paidAmount >= totalAmount
	}
	if shouldConfirm && status != "CANCELLED" && status != "EXPIRED" {
		if _, err := tx.Exec(ctx, `update bookings set status='CONFIRMED', updated_at=now() where booking_id=$1`, bookingID); err != nil {
			return err
		}
		log.Printf("event=booking_confirmed booking_id=%s paid_amount=%.2f deposit_amount=%.2f total_amount=%.2f", bookingID, paidAmount, depositAmount, totalAmount)
	}

	if _, err := tx.Exec(ctx, `insert into payment_events (payment_id, event, payload) values ($1,$2,$3)`, paymentID, event, payload); err != nil {
		return err
	}
	return nil
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

func IsUniqueViolation(err error) bool {
	pgErr, ok := err.(*pgconn.PgError)
	if !ok {
		return false
	}
	return pgErr.Code == "23505"
}
