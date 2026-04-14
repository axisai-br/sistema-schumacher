package affiliate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrWebhookDuplicate = errors.New("webhook duplicate")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetAuthorizationContext(ctx context.Context, userID string) (AuthorizationContext, error) {
	var out AuthorizationContext
	out.UserID = userID

	err := r.pool.QueryRow(ctx, `
		select exists (
			select 1
			from user_roles ur
			join roles rl on rl.id = ur.role_id
			where ur.user_id = $1::uuid
			  and rl.name = 'financeiro'
		)`,
		userID,
	).Scan(&out.HasRole)
	if err != nil {
		return out, err
	}

	var recipientID string
	err = r.pool.QueryRow(ctx, `
		select recipient_id
		from affiliate_recipients
		where user_id = $1::uuid and is_active = true`,
		userID,
	).Scan(&recipientID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return out, nil
		}
		return out, err
	}

	out.RecipientID = recipientID
	out.HasRecipient = true
	return out, nil
}

func (r *Repository) InsertWithdrawal(ctx context.Context, userID, recipientID string, amount int64, currency, idempotencyKey string, providerRequest []byte) (WithdrawalRecord, error) {
	var item WithdrawalRecord
	row := r.pool.QueryRow(ctx, `
		insert into affiliate_withdrawals
			(user_id, recipient_id, amount_cents, currency, status, idempotency_key, provider_request_payload, requested_at, created_at, updated_at)
		values
			($1::uuid, $2, $3, $4, 'PENDING', $5, $6, now(), now(), now())
		returning id, user_id::text, recipient_id, amount_cents, currency, status, transfer_id, idempotency_key,
		          provider_request_payload, provider_response_payload, provider_error, requested_at, processed_at, created_at, updated_at`,
		userID, recipientID, amount, currency, idempotencyKey, providerRequest,
	)
	err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.RecipientID,
		&item.AmountCents,
		&item.Currency,
		&item.Status,
		&item.TransferID,
		&item.IdempotencyKey,
		&item.ProviderRequestRaw,
		&item.ProviderResponseRaw,
		&item.ProviderError,
		&item.RequestedAt,
		&item.ProcessedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) MarkWithdrawalFailed(ctx context.Context, withdrawalID string, providerResponse []byte, providerError string) error {
	_, err := r.pool.Exec(ctx, `
		update affiliate_withdrawals
		set status = 'FAILED',
		    provider_response_payload = $2,
		    provider_error = $3,
		    processed_at = now(),
		    updated_at = now()
		where id = $1::uuid`,
		withdrawalID,
		providerResponse,
		providerError,
	)
	return err
}

func (r *Repository) MarkWithdrawalFromTransfer(ctx context.Context, withdrawalID string, transfer PagarmeTransferResult) error {
	normalized := normalizeTransferStatus(transfer.Status)
	_, err := r.pool.Exec(ctx, `
		update affiliate_withdrawals
		set status = $2,
		    transfer_id = nullif($3, ''),
		    provider_response_payload = $4,
		    processed_at = case when $2 in ('FAILED', 'SUCCESS') then now() else processed_at end,
		    updated_at = now()
		where id = $1::uuid`,
		withdrawalID,
		normalized,
		transfer.ID,
		transfer.Raw,
	)
	return err
}

func (r *Repository) SyncWithdrawalByTransferID(ctx context.Context, transferID string, transfer PagarmeTransferResult) error {
	normalized := normalizeTransferStatus(transfer.Status)
	_, err := r.pool.Exec(ctx, `
		update affiliate_withdrawals
		set status = $2,
		    provider_response_payload = $3,
		    processed_at = case when $2 in ('FAILED', 'SUCCESS') then now() else processed_at end,
		    updated_at = now()
		where transfer_id = $1`,
		transferID,
		normalized,
		transfer.Raw,
	)
	return err
}

func (r *Repository) ListWithdrawals(ctx context.Context, userID string, filter ListFilter) ([]WithdrawalHistoryItem, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.pool.QueryRow(ctx, `select count(*) from affiliate_withdrawals where user_id = $1::uuid`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, `
		select id::text, amount_cents, currency, status, transfer_id, requested_at, processed_at
		from affiliate_withdrawals
		where user_id = $1::uuid
		order by requested_at desc
		limit $2 offset $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]WithdrawalHistoryItem, 0, limit)
	for rows.Next() {
		var it WithdrawalHistoryItem
		if err := rows.Scan(&it.ID, &it.Amount, &it.Currency, &it.Status, &it.TransferID, &it.RequestedAt, &it.ProcessedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

func (r *Repository) FindPendingTransferIDs(ctx context.Context, userID string, filter ListFilter) ([]string, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.pool.Query(ctx, `
		select transfer_id
		from affiliate_withdrawals
		where user_id = $1::uuid
		  and status = 'PENDING'
		  and transfer_id is not null
		order by requested_at desc
		limit $2 offset $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) SaveWebhookEvent(ctx context.Context, eventID, eventType, transferID, signature *string, payload []byte) (WebhookEventRecord, error) {
	hash := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(hash[:])

	var item WebhookEventRecord
	row := r.pool.QueryRow(ctx, `
		insert into pagarme_webhook_events
			(provider, event_id, event_type, transfer_id, signature, payload_hash, payload, process_status, received_at)
		values
			('PAGARME', $1, $2, $3, $4, $5, $6, 'RECEIVED', now())
		returning id::text, event_id, event_type, transfer_id, signature, payload_hash, process_status, received_at`,
		eventID, eventType, transferID, signature, payloadHash, payload,
	)
	err := row.Scan(
		&item.ID,
		&item.EventID,
		&item.EventType,
		&item.TransferID,
		&item.Signature,
		&item.PayloadHash,
		&item.ProcessStatus,
		&item.ReceivedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(err.Error(), "23505") {
			return WebhookEventRecord{}, ErrWebhookDuplicate
		}
		return WebhookEventRecord{}, err
	}
	return item, nil
}

func (r *Repository) MarkWebhookProcessed(ctx context.Context, webhookID string, processingErr error) error {
	status := "PROCESSED"
	var errText *string
	if processingErr != nil {
		status = "FAILED"
		msg := processingErr.Error()
		errText = &msg
	}
	_, err := r.pool.Exec(ctx, `
		update pagarme_webhook_events
		set process_status = $2,
		    processing_error = $3,
		    processed_at = now()
		where id = $1::uuid`,
		webhookID, status, errText,
	)
	return err
}

func normalizeTransferStatus(status string) string {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	switch normalized {
	case "SUCCESS", "SUCCEEDED", "PAID":
		return "SUCCESS"
	case "FAILED", "ERROR", "REFUSED", "CANCELED", "CANCELLED":
		return "FAILED"
	default:
		return "PENDING"
	}
}

func maskProviderError(msg string) string {
	if strings.TrimSpace(msg) == "" {
		return "provider error"
	}
	if len(msg) > 240 {
		return msg[:240]
	}
	return msg
}

func newIdempotencyKey(userID string, amount int64, now time.Time) string {
	return fmt.Sprintf("wdr_%s_%d_%d", strings.ReplaceAll(userID, "-", ""), amount, now.UnixNano())
}
