package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	FindMessageByKeys(ctx context.Context, providerMessageID string, idempotencyKey string) (*Message, error)
	UpsertSession(ctx context.Context, input UpsertSessionInput) (Session, error)
	CreateMessage(ctx context.Context, input CreateMessageInput) (Message, error)
	UpdateMessage(ctx context.Context, input UpdateMessageInput) (Message, error)
	CreateToolCall(ctx context.Context, input CreateToolCallInput) (ToolCall, error)
	UpdateSessionBufferState(ctx context.Context, input UpdateSessionBufferStateInput) (Session, error)
	RequestHandoff(ctx context.Context, input RequestHandoffInput) (RequestHandoffResult, error)
	ResumeSession(ctx context.Context, input ResumeSessionInput) (ResumeSessionResult, error)
	FindReplyByIdempotency(ctx context.Context, sessionID string, idempotencyKey string) (*ReplyResult, error)
	CreateReply(ctx context.Context, input ReplyInput, debounceWindow time.Duration) (ReplyResult, error)
	CreateAutomationReply(ctx context.Context, input CreateAutomationReplyInput, debounceWindow time.Duration) (ReplyResult, error)
	MarkReplyDeliverySent(ctx context.Context, input MarkReplyDeliverySentInput) (ReplyResult, error)
	MarkReplyDeliveryFailure(ctx context.Context, input MarkReplyDeliveryFailureInput) (ReplyResult, error)
	UpdateDraftAutoSendState(ctx context.Context, input UpdateDraftAutoSendStateInput) (SaveAgentDraftResult, error)
	SaveReprocessSnapshot(ctx context.Context, input SaveReprocessSnapshotInput) (SaveReprocessSnapshotResult, error)
	SaveAgentDraft(ctx context.Context, input SaveAgentDraftInput) (SaveAgentDraftResult, error)
	ListSessions(ctx context.Context, filter ListSessionsFilter) ([]Session, error)
	CountSessionsSummary(ctx context.Context, filter ListSessionsFilter, reviewSLASeconds int) (SessionsSummary, error)
	GetSession(ctx context.Context, id string) (Session, error)
	ListMessages(ctx context.Context, sessionID string, filter ListMessagesFilter) ([]Message, error)
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) FindMessageByKeys(ctx context.Context, providerMessageID string, idempotencyKey string) (*Message, error) {
	clauses := make([]string, 0, 2)
	args := make([]interface{}, 0, 2)

	if trimmed := strings.TrimSpace(providerMessageID); trimmed != "" {
		args = append(args, trimmed)
		clauses = append(clauses, fmt.Sprintf("provider_message_id = $%d", len(args)))
	}
	if trimmed := strings.TrimSpace(idempotencyKey); trimmed != "" {
		args = append(args, trimmed)
		clauses = append(clauses, fmt.Sprintf("idempotency_key = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return nil, nil
	}

	query := `
		select
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
		from chat_messages
		where ` + strings.Join(clauses, " or ") + `
		order by created_at asc
		limit 1`

	row := r.pool.QueryRow(ctx, query, args...)
	item, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &item, nil
}

func (r *Repository) UpsertSession(ctx context.Context, input UpsertSessionInput) (Session, error) {
	metadata, err := encodeMap(input.Metadata)
	if err != nil {
		return Session{}, err
	}

	row := r.pool.QueryRow(ctx, `
		insert into chat_sessions (
			channel,
			contact_key,
			customer_phone,
			customer_name,
			status,
			handoff_status,
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			updated_at
		) values (
			$1,
			$2,
			nullif($3, ''),
			nullif($4, ''),
			'ACTIVE',
			'BOT',
			$5,
			$6,
			$7,
			$8::jsonb,
			now()
		)
		on conflict (channel, contact_key) do update
		set customer_phone = coalesce(nullif(excluded.customer_phone, ''), chat_sessions.customer_phone),
				customer_name = coalesce(nullif(excluded.customer_name, ''), chat_sessions.customer_name),
				last_message_at = case
					when chat_sessions.last_message_at is null then excluded.last_message_at
					when excluded.last_message_at is null then chat_sessions.last_message_at
					else greatest(chat_sessions.last_message_at, excluded.last_message_at)
				end,
				last_inbound_at = case
					when chat_sessions.last_inbound_at is null then excluded.last_inbound_at
					when excluded.last_inbound_at is null then chat_sessions.last_inbound_at
					else greatest(chat_sessions.last_inbound_at, excluded.last_inbound_at)
				end,
				last_outbound_at = case
					when chat_sessions.last_outbound_at is null then excluded.last_outbound_at
					when excluded.last_outbound_at is null then chat_sessions.last_outbound_at
					else greatest(chat_sessions.last_outbound_at, excluded.last_outbound_at)
				end,
				metadata = chat_sessions.metadata || excluded.metadata,
				updated_at = now()
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.Channel, input.ContactKey, input.CustomerPhone, input.CustomerName, input.LastMessageAt, input.LastInboundAt, input.LastOutboundAt, metadata)

	return scanSession(row)
}

func (r *Repository) CreateMessage(ctx context.Context, input CreateMessageInput) (Message, error) {
	payload, err := encodeMap(input.Payload)
	if err != nil {
		return Message{}, err
	}
	normalized, err := encodeMap(input.NormalizedPayload)
	if err != nil {
		return Message{}, err
	}

	row := r.pool.QueryRow(ctx, `
		insert into chat_messages (
			session_id,
			direction,
			kind,
			provider_message_id,
			idempotency_key,
			sender_name,
			sender_phone,
			body,
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at
		) values (
			$1::uuid,
			$2,
			$3,
			nullif($4, ''),
			nullif($5, ''),
			nullif($6, ''),
			nullif($7, ''),
			nullif($8, ''),
			$9::jsonb,
			$10::jsonb,
			$11,
			$12,
			$13
		)
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.SessionID, input.Direction, input.Kind, input.ProviderMessageID, input.IdempotencyKey, input.SenderName, input.SenderPhone, input.Body, payload, normalized, input.ProcessingStatus, input.ReceivedAt, input.SentAt)

	return scanMessage(row)
}

func (r *Repository) UpdateMessage(ctx context.Context, input UpdateMessageInput) (Message, error) {
	normalized, err := encodeMap(input.NormalizedPayload)
	if err != nil {
		return Message{}, err
	}

	row := r.pool.QueryRow(ctx, `
		update chat_messages
		set body = nullif($2, ''),
				normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $3::jsonb,
				processing_status = $4
		where id = $1::uuid
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.MessageID, input.Body, normalized, input.ProcessingStatus)

	return scanMessage(row)
}

func (r *Repository) CreateToolCall(ctx context.Context, input CreateToolCallInput) (ToolCall, error) {
	requestPayload, err := encodeMap(input.RequestPayload)
	if err != nil {
		return ToolCall{}, err
	}
	responsePayload, err := encodeMap(input.ResponsePayload)
	if err != nil {
		return ToolCall{}, err
	}

	row := r.pool.QueryRow(ctx, `
		insert into chat_tool_calls (
			session_id,
			message_id,
			tool_name,
			request_payload,
			response_payload,
			status,
			error_code,
			error_message,
			started_at,
			finished_at
		) values (
			$1::uuid,
			nullif($2, '')::uuid,
			$3,
			$4::jsonb,
			$5::jsonb,
			$6,
			nullif($7, ''),
			nullif($8, ''),
			$9,
			$10
		)
		returning
			id::text,
			session_id::text,
			coalesce(message_id::text, ''),
			tool_name,
			request_payload,
			response_payload,
			status,
			coalesce(error_code, ''),
			coalesce(error_message, ''),
			started_at,
			finished_at,
			created_at
	`, input.SessionID, input.MessageID, input.ToolName, requestPayload, responsePayload, input.Status, input.ErrorCode, input.ErrorMessage, input.StartedAt.UTC(), normalizeTimePointer(input.FinishedAt))

	return scanToolCall(row)
}

func (r *Repository) UpdateSessionBufferState(ctx context.Context, input UpdateSessionBufferStateInput) (Session, error) {
	bufferPayload, err := encodeMap(input.Buffer)
	if err != nil {
		return Session{}, err
	}

	row := r.pool.QueryRow(ctx, `
		update chat_sessions
		set metadata = chat_sessions.metadata || jsonb_build_object('buffer', $2::jsonb),
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID, bufferPayload)

	return scanSession(row)
}

func (r *Repository) RequestHandoff(ctx context.Context, input RequestHandoffInput) (RequestHandoffResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RequestHandoffResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metadata, err := encodeMap(input.Metadata)
	if err != nil {
		return RequestHandoffResult{}, err
	}

	handoffRow := tx.QueryRow(ctx, `
		insert into chat_handoffs (
			session_id,
			requested_by,
			reason,
			status,
			assigned_user_id,
			metadata,
			requested_at,
			updated_at
		) values (
			$1::uuid,
			$2,
			nullif($3, ''),
			'REQUESTED',
			nullif($4, '')::uuid,
			$5::jsonb,
			now(),
			now()
		)
		returning
			id::text,
			session_id::text,
			requested_by,
			coalesce(reason, ''),
			status,
			coalesce(assigned_user_id::text, ''),
			requested_at,
			resolved_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID, input.RequestedBy, input.Reason, input.AssignedUserID, metadata)

	handoff, err := scanHandoff(handoffRow)
	if err != nil {
		return RequestHandoffResult{}, err
	}

	sessionRow := tx.QueryRow(ctx, `
		update chat_sessions
		set handoff_status = case
				when nullif($2, '') is null then 'HUMAN_REQUESTED'
				else 'HUMAN'
			end,
				current_owner_user_id = nullif($2, '')::uuid,
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID, input.AssignedUserID)

	session, err := scanSession(sessionRow)
	if err != nil {
		return RequestHandoffResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RequestHandoffResult{}, err
	}

	return RequestHandoffResult{
		Session: session,
		Handoff: handoff,
	}, nil
}

func (r *Repository) ResumeSession(ctx context.Context, input ResumeSessionInput) (ResumeSessionResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ResumeSessionResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	row := tx.QueryRow(ctx, `
		select
			id::text,
			session_id::text,
			requested_by,
			coalesce(reason, ''),
			status,
			coalesce(assigned_user_id::text, ''),
			requested_at,
			resolved_at,
			metadata,
			created_at,
			updated_at
		from chat_handoffs
		where session_id = $1::uuid
			and status = 'REQUESTED'
			and resolved_at is null
		order by requested_at desc, created_at desc
		limit 1
		for update
	`, input.SessionID)

	handoff, err := scanHandoff(row)
	if err != nil {
		return ResumeSessionResult{}, err
	}

	metadata := handoff.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	for key, value := range input.Metadata {
		metadata[key] = value
	}
	if resumedBy := strings.TrimSpace(input.ResumedBy); resumedBy != "" {
		metadata["resumed_by"] = resumedBy
	}
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		metadata["resume_reason"] = reason
	}

	metadataPayload, err := encodeMap(metadata)
	if err != nil {
		return ResumeSessionResult{}, err
	}

	handoffRow := tx.QueryRow(ctx, `
		update chat_handoffs
		set status = 'RESOLVED',
				resolved_at = now(),
				metadata = $2::jsonb,
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			session_id::text,
			requested_by,
			coalesce(reason, ''),
			status,
			coalesce(assigned_user_id::text, ''),
			requested_at,
			resolved_at,
			metadata,
			created_at,
			updated_at
	`, handoff.ID, metadataPayload)

	handoff, err = scanHandoff(handoffRow)
	if err != nil {
		return ResumeSessionResult{}, err
	}

	sessionRow := tx.QueryRow(ctx, `
		update chat_sessions
		set handoff_status = 'BOT',
				current_owner_user_id = null,
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID)

	session, err := scanSession(sessionRow)
	if err != nil {
		return ResumeSessionResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ResumeSessionResult{}, err
	}

	return ResumeSessionResult{
		Session: session,
		Handoff: handoff,
	}, nil
}

func (r *Repository) FindReplyByIdempotency(ctx context.Context, sessionID string, idempotencyKey string) (*ReplyResult, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return nil, nil
	}

	messageRow := r.pool.QueryRow(ctx, `
		select
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
		from chat_messages
		where session_id = $1::uuid
			and direction = 'OUTBOUND'
			and idempotency_key = $2
		order by created_at asc
		limit 1
	`, sessionID, key)

	message, err := scanMessage(messageRow)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	session, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	outboundRow := r.pool.QueryRow(ctx, `
		select
			id::text,
			coalesce(session_id::text, ''),
			channel,
			recipient,
			payload,
			provider,
			coalesce(provider_message_id, ''),
			idempotency_key,
			status,
			sent_at,
			delivered_at,
			created_at,
			updated_at
		from outbound_messages
		where session_id = $1::uuid
			and idempotency_key = $2
		order by created_at asc
		limit 1
	`, sessionID, key)

	outbound, err := scanReplyOutbound(outboundRow)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
	}, nil
}

func (r *Repository) CreateReply(ctx context.Context, input ReplyInput, debounceWindow time.Duration) (ReplyResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReplyResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	sessionRow := tx.QueryRow(ctx, `
		select
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
		from chat_sessions
		where id = $1::uuid
		for update
	`, input.SessionID)

	session, err := scanSession(sessionRow)
	if err != nil {
		return ReplyResult{}, err
	}
	if session.HandoffStatus != "BOT" || strings.TrimSpace(session.CurrentOwnerUserID) != "" {
		return ReplyResult{}, ErrReprocessRequiresBot
	}

	recordedAt := time.Now().UTC()
	replyBody := strings.TrimSpace(input.Body)
	replyMode := "ASSISTED_REPLY"
	reviewAction := ""
	var reviewedDraft *Message

	if strings.TrimSpace(input.DraftMessageID) != "" {
		draftRow := tx.QueryRow(ctx, `
			select
				id::text,
				session_id::text,
				direction,
				kind,
				coalesce(provider_message_id, ''),
				coalesce(idempotency_key, ''),
				coalesce(sender_name, ''),
				coalesce(sender_phone, ''),
				coalesce(body, ''),
				payload,
				normalized_payload,
				processing_status,
				received_at,
				sent_at,
				created_at
			from chat_messages
			where id = $1::uuid
			for update
		`, input.DraftMessageID)

		draft, err := scanMessage(draftRow)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ReplyResult{}, ErrReplyDraftNotAllowed
			}
			return ReplyResult{}, err
		}
		if draft.SessionID != input.SessionID || draft.Direction != "OUTBOUND" || !strings.EqualFold(strings.TrimSpace(draft.ProcessingStatus), messageStatusAutomationDraft) {
			return ReplyResult{}, ErrReplyDraftNotAllowed
		}

		if replyBody == "" {
			replyBody = strings.TrimSpace(draft.Body)
		}
		if replyBody == "" {
			return ReplyResult{}, ErrReplyBodyRequired
		}

		reviewAction = "APPROVED_AS_IS"
		if strings.TrimSpace(replyBody) != strings.TrimSpace(draft.Body) {
			reviewAction = "EDITED"
		}
		reviewModePayload, err := encodeMap(map[string]interface{}{
			"review_mode":         "CONTROLLED",
			"review_status":       "APPROVED",
			"review_action":       reviewAction,
			"reviewed_at":         recordedAt.Format(time.RFC3339Nano),
			"reviewed_by_user_id": input.OwnerUserID,
		})
		if err != nil {
			return ReplyResult{}, err
		}

		draftRow = tx.QueryRow(ctx, `
			update chat_messages
			set payload = coalesce(payload, '{}'::jsonb) || $2::jsonb,
					normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $2::jsonb,
					processing_status = $3
			where id = $1::uuid
			returning
				id::text,
				session_id::text,
				direction,
				kind,
				coalesce(provider_message_id, ''),
				coalesce(idempotency_key, ''),
				coalesce(sender_name, ''),
				coalesce(sender_phone, ''),
				coalesce(body, ''),
				payload,
				normalized_payload,
				processing_status,
				received_at,
				sent_at,
				created_at
		`, input.DraftMessageID, reviewModePayload, messageStatusAutomationReviewed)

		updatedDraft, err := scanMessage(draftRow)
		if err != nil {
			return ReplyResult{}, err
		}
		reviewedDraft = &updatedDraft
		replyMode = "DRAFT_REVIEW"
	}

	replyPayload := map[string]interface{}{
		"mode":            "ASSISTED_REPLY",
		"shadow_mode":     true,
		"owner_user_id":   input.OwnerUserID,
		"sender_name":     input.SenderName,
		"session_channel": session.Channel,
		"contact_key":     session.ContactKey,
	}
	replyPayload["mode"] = replyMode
	if reviewedDraft != nil {
		replyPayload["draft_message_id"] = reviewedDraft.ID
		replyPayload["review_mode"] = "CONTROLLED"
		replyPayload["review_action"] = reviewAction
		replyPayload["draft_reviewed"] = true
	}
	for key, value := range input.Metadata {
		replyPayload[key] = value
	}

	messagePayload, err := encodeMap(replyPayload)
	if err != nil {
		return ReplyResult{}, err
	}

	messageRow := tx.QueryRow(ctx, `
		insert into chat_messages (
			session_id,
			direction,
			kind,
			idempotency_key,
			sender_name,
			body,
			payload,
			normalized_payload,
			processing_status,
			received_at
		) values (
			$1::uuid,
			'OUTBOUND',
			'TEXT',
			$2,
			nullif($3, ''),
			nullif($4, ''),
			$5::jsonb,
			$5::jsonb,
			'MANUAL_PENDING',
			$6
		)
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.SessionID, input.IdempotencyKey, input.SenderName, replyBody, messagePayload, recordedAt)

	message, err := scanMessage(messageRow)
	if err != nil {
		return ReplyResult{}, err
	}

	sessionMetadata := session.Metadata
	if sessionMetadata == nil {
		sessionMetadata = map[string]interface{}{}
	}
	updatedMetadata := make(map[string]interface{}, len(sessionMetadata)+1)
	for key, value := range sessionMetadata {
		updatedMetadata[key] = value
	}
	updatedMetadata["buffer"] = buildBufferState(session.Metadata, message, debounceWindow)
	if reviewedDraft != nil {
		updatedMetadata["agent"] = buildDraftReviewedAgentState(session.Metadata, *reviewedDraft, input.OwnerUserID, reviewAction, recordedAt)
	}

	metadataPayload, err := encodeMap(updatedMetadata)
	if err != nil {
		return ReplyResult{}, err
	}

	sessionRow = tx.QueryRow(ctx, `
		update chat_sessions
		set last_message_at = $2,
				last_outbound_at = $2,
				metadata = $3::jsonb,
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID, recordedAt, metadataPayload)

	session, err = scanSession(sessionRow)
	if err != nil {
		return ReplyResult{}, err
	}

	outboundPayload := map[string]interface{}{
		"mode":          "ASSISTED_REPLY",
		"shadow_mode":   true,
		"body":          replyBody,
		"owner_user_id": input.OwnerUserID,
		"sender_name":   input.SenderName,
		"message_id":    message.ID,
	}
	outboundPayload["mode"] = replyMode
	if reviewedDraft != nil {
		outboundPayload["draft_message_id"] = reviewedDraft.ID
		outboundPayload["review_mode"] = "CONTROLLED"
		outboundPayload["review_action"] = reviewAction
		outboundPayload["draft_reviewed"] = true
	}
	for key, value := range input.Metadata {
		outboundPayload[key] = value
	}

	outboundPayloadBytes, err := encodeMap(outboundPayload)
	if err != nil {
		return ReplyResult{}, err
	}

	outboundRow := tx.QueryRow(ctx, `
		insert into outbound_messages (
			session_id,
			channel,
			recipient,
			payload,
			provider,
			idempotency_key,
			status,
			created_at,
			updated_at
		) values (
			$1::uuid,
			$2,
			$3,
			$4::jsonb,
			'EVOLUTION',
			$5,
			'MANUAL_PENDING',
			now(),
			now()
		)
		returning
			id::text,
			coalesce(session_id::text, ''),
			channel,
			recipient,
			payload,
			provider,
			coalesce(provider_message_id, ''),
			idempotency_key,
			status,
			sent_at,
			delivered_at,
			created_at,
			updated_at
	`, input.SessionID, session.Channel, session.ContactKey, outboundPayloadBytes, input.IdempotencyKey)

	outbound, err := scanReplyOutbound(outboundRow)
	if err != nil {
		return ReplyResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ReplyResult{}, err
	}

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    reviewedDraft,
	}, nil
}

func (r *Repository) CreateAutomationReply(ctx context.Context, input CreateAutomationReplyInput, debounceWindow time.Duration) (ReplyResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReplyResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	sessionRow := tx.QueryRow(ctx, `
		select
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
		from chat_sessions
		where id = $1::uuid
		for update
	`, input.SessionID)

	session, err := scanSession(sessionRow)
	if err != nil {
		return ReplyResult{}, err
	}

	draftRow := tx.QueryRow(ctx, `
		select
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
		from chat_messages
		where id = $1::uuid
		for update
	`, input.DraftMessageID)

	draft, err := scanMessage(draftRow)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReplyResult{}, ErrReplyDraftNotAllowed
		}
		return ReplyResult{}, err
	}
	if draft.SessionID != input.SessionID || draft.Direction != "OUTBOUND" || !strings.EqualFold(strings.TrimSpace(draft.ProcessingStatus), messageStatusAutomationDraft) {
		return ReplyResult{}, ErrReplyDraftNotAllowed
	}

	replyBody := strings.TrimSpace(draft.Body)
	if replyBody == "" {
		return ReplyResult{}, ErrReplyBodyRequired
	}
	senderName := strings.TrimSpace(input.SenderName)
	if senderName == "" {
		senderName = firstNonEmpty(strings.TrimSpace(draft.SenderName), "SHABAS")
	}
	recordedAt := time.Now().UTC()

	replyPayload := map[string]interface{}{
		"mode":             "BOT_AUTO_REPLY",
		"draft_message_id": draft.ID,
		"draft_auto_sent":  true,
		"sender_name":      senderName,
		"session_channel":  session.Channel,
		"contact_key":      session.ContactKey,
		"auto_send_status": firstNonEmpty(asString(draft.NormalizedPayload["auto_send_status"]), asString(draft.Payload["auto_send_status"]), draftAutoSendStatusEligible),
	}
	if reasons := firstNonEmptyStringSlice(asStringSlice(draft.NormalizedPayload["auto_send_reasons"]), asStringSlice(draft.Payload["auto_send_reasons"])); len(reasons) > 0 {
		replyPayload["auto_send_reasons"] = reasons
	}
	for key, value := range input.Metadata {
		replyPayload[key] = value
	}

	messagePayload, err := encodeMap(replyPayload)
	if err != nil {
		return ReplyResult{}, err
	}

	messageRow := tx.QueryRow(ctx, `
		insert into chat_messages (
			session_id,
			direction,
			kind,
			idempotency_key,
			sender_name,
			body,
			payload,
			normalized_payload,
			processing_status,
			received_at
		) values (
			$1::uuid,
			'OUTBOUND',
			'TEXT',
			$2,
			nullif($3, ''),
			nullif($4, ''),
			$5::jsonb,
			$5::jsonb,
			'AUTOMATION_PENDING',
			$6
		)
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.SessionID, input.IdempotencyKey, senderName, replyBody, messagePayload, recordedAt)

	message, err := scanMessage(messageRow)
	if err != nil {
		return ReplyResult{}, err
	}

	sessionMetadata := session.Metadata
	if sessionMetadata == nil {
		sessionMetadata = map[string]interface{}{}
	}
	updatedMetadata := make(map[string]interface{}, len(sessionMetadata))
	for key, value := range sessionMetadata {
		updatedMetadata[key] = value
	}
	updatedMetadata["buffer"] = buildBufferState(session.Metadata, message, debounceWindow)

	metadataPayload, err := encodeMap(updatedMetadata)
	if err != nil {
		return ReplyResult{}, err
	}

	sessionRow = tx.QueryRow(ctx, `
		update chat_sessions
		set last_message_at = $2,
				last_outbound_at = $2,
				metadata = $3::jsonb,
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID, recordedAt, metadataPayload)

	session, err = scanSession(sessionRow)
	if err != nil {
		return ReplyResult{}, err
	}

	outboundPayload := map[string]interface{}{
		"mode":             "BOT_AUTO_REPLY",
		"body":             replyBody,
		"draft_message_id": draft.ID,
		"draft_auto_sent":  true,
		"sender_name":      senderName,
		"message_id":       message.ID,
		"auto_send_status": replyPayload["auto_send_status"],
	}
	if reasons, ok := replyPayload["auto_send_reasons"]; ok {
		outboundPayload["auto_send_reasons"] = reasons
	}
	for key, value := range input.Metadata {
		outboundPayload[key] = value
	}

	outboundPayloadBytes, err := encodeMap(outboundPayload)
	if err != nil {
		return ReplyResult{}, err
	}

	outboundRow := tx.QueryRow(ctx, `
		insert into outbound_messages (
			session_id,
			channel,
			recipient,
			payload,
			provider,
			idempotency_key,
			status,
			created_at,
			updated_at
		) values (
			$1::uuid,
			$2,
			$3,
			$4::jsonb,
			'EVOLUTION',
			$5,
			'AUTOMATION_PENDING',
			now(),
			now()
		)
		returning
			id::text,
			coalesce(session_id::text, ''),
			channel,
			recipient,
			payload,
			provider,
			coalesce(provider_message_id, ''),
			idempotency_key,
			status,
			sent_at,
			delivered_at,
			created_at,
			updated_at
	`, input.SessionID, session.Channel, session.ContactKey, outboundPayloadBytes, input.IdempotencyKey)

	outbound, err := scanReplyOutbound(outboundRow)
	if err != nil {
		return ReplyResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ReplyResult{}, err
	}

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    &draft,
	}, nil
}

func (r *Repository) UpdateDraftAutoSendState(ctx context.Context, input UpdateDraftAutoSendStateInput) (SaveAgentDraftResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SaveAgentDraftResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	session, err := r.scanSessionByID(ctx, tx, input.SessionID)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}

	draft, err := r.scanMessageByID(ctx, tx, input.DraftMessageID)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}
	if draft.SessionID != input.SessionID || draft.Direction != "OUTBOUND" || !isAutomationDraftStatus(draft.ProcessingStatus) {
		return SaveAgentDraftResult{}, ErrReplyDraftNotAllowed
	}

	payload := map[string]interface{}{}
	for key, value := range input.Payload {
		payload[key] = value
	}
	payload["auto_send_status"] = strings.TrimSpace(input.AutoSendStatus)
	if reasons := mergeDistinctStrings(readDraftAutoSendReasons(draft), input.AutoSendReasons...); len(reasons) > 0 {
		payload["auto_send_reasons"] = reasons
	}
	payloadBytes, err := encodeMap(payload)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}

	draftRow := tx.QueryRow(ctx, `
		update chat_messages
		set payload = coalesce(payload, '{}'::jsonb) || $2::jsonb,
				normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $2::jsonb
		where id = $1::uuid
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.DraftMessageID, payloadBytes)

	updatedDraft, err := scanMessage(draftRow)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}

	if len(input.Agent) > 0 {
		sessionMetadata := session.Metadata
		if sessionMetadata == nil {
			sessionMetadata = map[string]interface{}{}
		}
		updatedMetadata := make(map[string]interface{}, len(sessionMetadata)+1)
		for key, value := range sessionMetadata {
			updatedMetadata[key] = value
		}
		updatedMetadata["agent"] = input.Agent
		metadataPayload, encodeErr := encodeMap(updatedMetadata)
		if encodeErr != nil {
			return SaveAgentDraftResult{}, encodeErr
		}
		sessionRow := tx.QueryRow(ctx, `
			update chat_sessions
			set metadata = $2::jsonb,
					updated_at = now()
			where id = $1::uuid
			returning
				id::text,
				channel,
				contact_key,
				coalesce(customer_phone, ''),
				coalesce(customer_name, ''),
				status,
				handoff_status,
				coalesce(current_owner_user_id::text, ''),
				last_message_at,
				last_inbound_at,
				last_outbound_at,
				metadata,
				created_at,
				updated_at
		`, input.SessionID, metadataPayload)
		session, err = scanSession(sessionRow)
		if err != nil {
			return SaveAgentDraftResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return SaveAgentDraftResult{}, err
	}

	return SaveAgentDraftResult{
		Session: session,
		Message: updatedDraft,
	}, nil
}

func (r *Repository) MarkReplyDeliverySent(ctx context.Context, input MarkReplyDeliverySentInput) (ReplyResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReplyResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	messageBefore, err := r.scanMessageByID(ctx, tx, input.MessageID)
	if err != nil {
		return ReplyResult{}, err
	}

	deliveryMode := deliveryModeForPayload(messageBefore.Payload)
	payload := map[string]interface{}{
		"delivery_mode":        deliveryMode,
		"delivery_recorded_at": input.SentAt.UTC().Format(time.RFC3339Nano),
		"provider_status":      input.ProviderStatus,
	}
	for key, value := range input.Payload {
		payload[key] = value
	}
	payloadBytes, err := encodeMap(payload)
	if err != nil {
		return ReplyResult{}, err
	}

	messageRow := tx.QueryRow(ctx, `
		update chat_messages
		set provider_message_id = nullif($2, ''),
				normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $4::jsonb,
				processing_status = $3,
				sent_at = coalesce(sent_at, $5),
				created_at = created_at
		where id = $1::uuid
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.MessageID, input.ProviderMessageID, input.ProviderStatus, payloadBytes, input.SentAt.UTC())

	message, err := scanMessage(messageRow)
	if err != nil {
		return ReplyResult{}, err
	}

	outboundRow := tx.QueryRow(ctx, `
		update outbound_messages
		set provider_message_id = nullif($2, ''),
				status = $3,
				payload = coalesce(payload, '{}'::jsonb) || $4::jsonb,
				error_text = null,
				sent_at = coalesce(sent_at, $5),
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			coalesce(session_id::text, ''),
			channel,
			recipient,
			payload,
			provider,
			coalesce(provider_message_id, ''),
			idempotency_key,
			status,
			sent_at,
			delivered_at,
			created_at,
			updated_at
	`, input.OutboundID, input.ProviderMessageID, input.ProviderStatus, payloadBytes, input.SentAt.UTC())

	outbound, err := scanReplyOutbound(outboundRow)
	if err != nil {
		return ReplyResult{}, err
	}

	var updatedDraft *Message
	if strings.EqualFold(asString(message.Payload["mode"]), "BOT_AUTO_REPLY") {
		draftID := strings.TrimSpace(firstNonEmpty(asString(message.Payload["draft_message_id"]), asString(message.NormalizedPayload["draft_message_id"])))
		if draftID != "" {
			draftPayload := map[string]interface{}{
				"auto_send_status":              draftAutoSendStatusEligible,
				"auto_sent":                     true,
				"auto_sent_at":                  input.SentAt.UTC().Format(time.RFC3339Nano),
				"auto_sent_message_id":          message.ID,
				"auto_sent_outbound_id":         outbound.ID,
				"auto_sent_provider_message_id": strings.TrimSpace(input.ProviderMessageID),
				"auto_send_last_error_text":     nil,
				"auto_send_retry_pending_at":    nil,
			}
			draftPayloadBytes, encodeErr := encodeMap(draftPayload)
			if encodeErr != nil {
				return ReplyResult{}, encodeErr
			}
			draftRow := tx.QueryRow(ctx, `
				update chat_messages
				set payload = coalesce(payload, '{}'::jsonb) || $2::jsonb,
						normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $2::jsonb,
						processing_status = $3
				where id = $1::uuid
				returning
					id::text,
					session_id::text,
					direction,
					kind,
					coalesce(provider_message_id, ''),
					coalesce(idempotency_key, ''),
					coalesce(sender_name, ''),
					coalesce(sender_phone, ''),
					coalesce(body, ''),
					payload,
					normalized_payload,
					processing_status,
					received_at,
					sent_at,
					created_at
			`, draftID, draftPayloadBytes, messageStatusAutomationSent)
			draft, scanErr := scanMessage(draftRow)
			if scanErr != nil {
				return ReplyResult{}, scanErr
			}
			updatedDraft = &draft

			session, sessionErr := r.scanSessionByID(ctx, tx, input.SessionID)
			if sessionErr != nil {
				return ReplyResult{}, sessionErr
			}
			sessionMetadata := session.Metadata
			if sessionMetadata == nil {
				sessionMetadata = map[string]interface{}{}
			}
			newMetadata := make(map[string]interface{}, len(sessionMetadata))
			for key, value := range sessionMetadata {
				newMetadata[key] = value
			}
			newMetadata["agent"] = buildDraftAutoSentAgentState(session.Metadata, draft, message, outbound, input.SentAt.UTC())
			metadataPayload, encodeErr := encodeMap(newMetadata)
			if encodeErr != nil {
				return ReplyResult{}, encodeErr
			}
			if _, execErr := tx.Exec(ctx, `
				update chat_sessions
				set metadata = $2::jsonb,
						updated_at = now()
				where id = $1::uuid
			`, input.SessionID, metadataPayload); execErr != nil {
				return ReplyResult{}, execErr
			}
		}
	}

	session, err := r.scanSessionByID(ctx, tx, input.SessionID)
	if err != nil {
		return ReplyResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ReplyResult{}, err
	}

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    updatedDraft,
	}, nil
}

func (r *Repository) MarkReplyDeliveryFailure(ctx context.Context, input MarkReplyDeliveryFailureInput) (ReplyResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReplyResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	messageBefore, err := r.scanMessageByID(ctx, tx, input.MessageID)
	if err != nil {
		return ReplyResult{}, err
	}

	payloadBytes, err := encodeMap(map[string]interface{}{
		"delivery_mode":       deliveryModeForPayload(messageBefore.Payload),
		"delivery_failed":     true,
		"delivery_failed_at":  time.Now().UTC().Format(time.RFC3339Nano),
		"delivery_error_text": strings.TrimSpace(input.ErrorText),
	})
	if err != nil {
		return ReplyResult{}, err
	}

	messageRow := tx.QueryRow(ctx, `
		update chat_messages
		set normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $2::jsonb,
				processing_status = 'SEND_FAILED'
		where id = $1::uuid
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.MessageID, payloadBytes)

	message, err := scanMessage(messageRow)
	if err != nil {
		return ReplyResult{}, err
	}

	outboundRow := tx.QueryRow(ctx, `
		update outbound_messages
		set status = 'SEND_FAILED',
				error_text = nullif($2, ''),
				payload = coalesce(payload, '{}'::jsonb) || $3::jsonb,
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			coalesce(session_id::text, ''),
			channel,
			recipient,
			payload,
			provider,
			coalesce(provider_message_id, ''),
			idempotency_key,
			status,
			sent_at,
			delivered_at,
			created_at,
			updated_at
	`, input.OutboundID, input.ErrorText, payloadBytes)

	outbound, err := scanReplyOutbound(outboundRow)
	if err != nil {
		return ReplyResult{}, err
	}

	session, err := r.scanSessionByID(ctx, tx, input.SessionID)
	if err != nil {
		return ReplyResult{}, err
	}

	var updatedDraft *Message
	if strings.EqualFold(asString(message.Payload["mode"]), "BOT_AUTO_REPLY") {
		draftID := strings.TrimSpace(firstNonEmpty(asString(message.Payload["draft_message_id"]), asString(message.NormalizedPayload["draft_message_id"])))
		if draftID != "" {
			draft, draftErr := r.scanMessageByID(ctx, tx, draftID)
			if draftErr != nil {
				return ReplyResult{}, draftErr
			}
			observedAt := time.Now().UTC()
			draftPayload := map[string]interface{}{
				"auto_send_status":                draftAutoSendStatusRetryPending,
				"auto_send_reasons":               mergeDistinctStrings(readDraftAutoSendReasons(draft), draftAutoSendReasonDeliveryFail),
				"auto_send_last_attempt_at":       observedAt.Format(time.RFC3339Nano),
				"auto_send_retry_pending_at":      observedAt.Format(time.RFC3339Nano),
				"auto_send_last_error_text":       strings.TrimSpace(input.ErrorText),
				"auto_send_last_reply_message_id": message.ID,
				"auto_send_last_outbound_id":      outbound.ID,
			}
			draftPayloadBytes, encodeErr := encodeMap(draftPayload)
			if encodeErr != nil {
				return ReplyResult{}, encodeErr
			}
			draftRow := tx.QueryRow(ctx, `
				update chat_messages
				set payload = coalesce(payload, '{}'::jsonb) || $2::jsonb,
						normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $2::jsonb
				where id = $1::uuid
				returning
					id::text,
					session_id::text,
					direction,
					kind,
					coalesce(provider_message_id, ''),
					coalesce(idempotency_key, ''),
					coalesce(sender_name, ''),
					coalesce(sender_phone, ''),
					coalesce(body, ''),
					payload,
					normalized_payload,
					processing_status,
					received_at,
					sent_at,
					created_at
			`, draftID, draftPayloadBytes)
			updated, scanErr := scanMessage(draftRow)
			if scanErr != nil {
				return ReplyResult{}, scanErr
			}
			updatedDraft = &updated

			sessionMetadata := session.Metadata
			if sessionMetadata == nil {
				sessionMetadata = map[string]interface{}{}
			}
			updatedMetadata := make(map[string]interface{}, len(sessionMetadata)+1)
			for key, value := range sessionMetadata {
				updatedMetadata[key] = value
			}
			updatedMetadata["agent"] = buildDraftAutoSendRetryPendingAgentState(session.Metadata, updated, message, outbound, input.ErrorText, observedAt)
			metadataPayload, encodeErr := encodeMap(updatedMetadata)
			if encodeErr != nil {
				return ReplyResult{}, encodeErr
			}
			sessionRow := tx.QueryRow(ctx, `
				update chat_sessions
				set metadata = $2::jsonb,
						updated_at = now()
				where id = $1::uuid
				returning
					id::text,
					channel,
					contact_key,
					coalesce(customer_phone, ''),
					coalesce(customer_name, ''),
					status,
					handoff_status,
					coalesce(current_owner_user_id::text, ''),
					last_message_at,
					last_inbound_at,
					last_outbound_at,
					metadata,
					created_at,
					updated_at
			`, input.SessionID, metadataPayload)
			session, err = scanSession(sessionRow)
			if err != nil {
				return ReplyResult{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return ReplyResult{}, err
	}

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    updatedDraft,
	}, nil
}

func (r *Repository) SaveReprocessSnapshot(ctx context.Context, input SaveReprocessSnapshotInput) (SaveReprocessSnapshotResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SaveReprocessSnapshotResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	messagePayload, err := encodeMap(input.MessageMetadata)
	if err != nil {
		return SaveReprocessSnapshotResult{}, err
	}
	rows, err := tx.Query(ctx, `
		update chat_messages
		set normalized_payload = coalesce(normalized_payload, '{}'::jsonb) || $3::jsonb,
				processing_status = $4
		where session_id = $1::uuid
			and id = any($2::uuid[])
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.SessionID, input.MessageIDs, messagePayload, input.MessageStatus)
	if err != nil {
		return SaveReprocessSnapshotResult{}, err
	}
	defer rows.Close()

	messages := make([]Message, 0, len(input.MessageIDs))
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return SaveReprocessSnapshotResult{}, err
		}
		messages = append(messages, item)
	}
	if err := rows.Err(); err != nil {
		return SaveReprocessSnapshotResult{}, err
	}

	memoryPayload, err := encodeMap(input.Memory)
	if err != nil {
		return SaveReprocessSnapshotResult{}, err
	}
	agentPayload, err := encodeMap(input.Agent)
	if err != nil {
		return SaveReprocessSnapshotResult{}, err
	}
	bufferPayload, err := encodeMap(input.Buffer)
	if err != nil {
		return SaveReprocessSnapshotResult{}, err
	}

	sessionRow := tx.QueryRow(ctx, `
		update chat_sessions
		set metadata = chat_sessions.metadata
				|| jsonb_build_object('memory', $2::jsonb)
				|| jsonb_build_object('agent', $3::jsonb)
				|| jsonb_build_object('buffer', $4::jsonb),
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID, memoryPayload, agentPayload, bufferPayload)

	session, err := scanSession(sessionRow)
	if err != nil {
		return SaveReprocessSnapshotResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return SaveReprocessSnapshotResult{}, err
	}

	return SaveReprocessSnapshotResult{
		Session:  session,
		Messages: messages,
	}, nil
}

func (r *Repository) SaveAgentDraft(ctx context.Context, input SaveAgentDraftInput) (SaveAgentDraftResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SaveAgentDraftResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	payload, err := encodeMap(input.Payload)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}
	normalized, err := encodeMap(input.NormalizedPayload)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}
	messageRow := tx.QueryRow(ctx, `
		insert into chat_messages (
			session_id,
			direction,
			kind,
			idempotency_key,
			sender_name,
			body,
			payload,
			normalized_payload,
			processing_status,
			received_at
		) values (
			$1::uuid,
			'OUTBOUND',
			'TEXT',
			$2,
			nullif($3, ''),
			nullif($4, ''),
			$5::jsonb,
			$6::jsonb,
			$7,
			$8
		)
		returning
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
	`, input.SessionID, input.IdempotencyKey, input.SenderName, input.Body, payload, normalized, input.ProcessingStatus, input.RecordedAt.UTC())

	message, err := scanMessage(messageRow)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}

	agentPayload, err := encodeMap(input.Agent)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}
	bufferPayload, err := encodeMap(input.Buffer)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}
	sessionRow := tx.QueryRow(ctx, `
		update chat_sessions
		set metadata = chat_sessions.metadata
				|| jsonb_build_object('agent', $2::jsonb)
				|| jsonb_build_object('buffer', $3::jsonb),
				updated_at = now()
		where id = $1::uuid
		returning
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
	`, input.SessionID, agentPayload, bufferPayload)

	session, err := scanSession(sessionRow)
	if err != nil {
		return SaveAgentDraftResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return SaveAgentDraftResult{}, err
	}

	return SaveAgentDraftResult{
		Session: session,
		Message: message,
	}, nil
}

func (r *Repository) ListSessions(ctx context.Context, filter ListSessionsFilter) ([]Session, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	reviewSLASeconds := filter.ReviewSLASeconds
	if reviewSLASeconds <= 0 {
		reviewSLASeconds = 15 * 60
	}

	query := `
		select
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
		from chat_sessions`

	args := make([]interface{}, 0, 9)
	clauses := make([]string, 0, 6)

	if filter.Channel != "" {
		args = append(args, filter.Channel)
		clauses = append(clauses, fmt.Sprintf("channel = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.HandoffStatus != "" {
		args = append(args, filter.HandoffStatus)
		clauses = append(clauses, fmt.Sprintf("handoff_status = $%d", len(args)))
	}
	if filter.ContactKey != "" {
		args = append(args, filter.ContactKey)
		clauses = append(clauses, fmt.Sprintf("contact_key = $%d", len(args)))
	}
	if filter.AgentStatus != "" {
		args = append(args, filter.AgentStatus)
		clauses = append(clauses, fmt.Sprintf("upper(coalesce(metadata->'agent'->>'status', '')) = $%d", len(args)))
	}
	if filter.DraftAutoSendStatus != "" {
		args = append(args, filter.DraftAutoSendStatus)
		clauses = append(clauses, fmt.Sprintf("upper(coalesce(metadata->'agent'->>'auto_send_status', '')) = $%d", len(args)))
	}
	switch filter.DraftReviewStatus {
	case "PENDING_REVIEW":
		clauses = append(clauses, "upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'")
	case "REVIEWED":
		clauses = append(clauses, "upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_REVIEWED'")
	}
	if len(clauses) > 0 {
		query += "\nwhere " + strings.Join(clauses, " and ")
	}

	if filter.OrderBy == "REVIEW_PRIORITY" {
		args = append(args, reviewSLASeconds)
		slaIdx := len(args)
		args = append(args, limit)
		limitIdx := len(args)
		query += fmt.Sprintf(`
order by
	case
		when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
		 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
		 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) >= $%d::int then 0
		when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
		 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
		 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) >= greatest($%d::int / 2, 1) then 1
		when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED' then 2
		when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_REVIEWED' then 3
		else 4
	end asc,
	case
		when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
		 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
		then extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz)))::int
		else 0
	end desc,
	last_message_at desc nulls last,
	created_at desc
limit $%d`, slaIdx, slaIdx, limitIdx)
	} else {
		args = append(args, limit)
		query += fmt.Sprintf("\norder by last_message_at desc nulls last, created_at desc\nlimit $%d", len(args))
	}
	args = append(args, offset)
	query += fmt.Sprintf(" offset $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Session{}
	for rows.Next() {
		item, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) CountSessionsSummary(ctx context.Context, filter ListSessionsFilter, reviewSLASeconds int) (SessionsSummary, error) {
	query := `
		select
			count(*)::int as total_count,
			coalesce(sum(case when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED' then 1 else 0 end), 0)::int as pending_review_count,
			coalesce(sum(case when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_REVIEWED' then 1 else 0 end), 0)::int as reviewed_count,
			coalesce(sum(case when upper(coalesce(metadata->'agent'->>'status', '')) not in ('DRAFT_GENERATED', 'DRAFT_REVIEWED') then 1 else 0 end), 0)::int as no_draft_count,
			coalesce(sum(case when handoff_status = 'HUMAN' and current_owner_user_id is not null then 1 else 0 end), 0)::int as human_owned_count,
			coalesce(sum(case when handoff_status = 'BOT' and current_owner_user_id is null then 1 else 0 end), 0)::int as bot_owned_count,
			coalesce(sum(case
				when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
				 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
				 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) >= greatest($1::int / 2, 1)
				 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) < $1::int
				then 1 else 0 end), 0)::int as due_soon_review_count,
			coalesce(sum(case
				when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
				 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
				 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) >= $1::int
				then 1 else 0 end), 0)::int as overdue_review_count,
			coalesce(sum(case
				when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
				 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
				 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) >= $1::int
				then 1 else 0 end), 0)::int as high_priority_review_count,
			coalesce(sum(case
				when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
				 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
				 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) >= greatest($1::int / 2, 1)
				 and extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) < $1::int
				then 1 else 0 end), 0)::int as medium_priority_review_count,
			coalesce(sum(case
				when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
				 and (
					coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') = ''
					or extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz))) < greatest($1::int / 2, 1)
				 )
				then 1 else 0 end), 0)::int as low_priority_review_count,
			coalesce(max(case
				when upper(coalesce(metadata->'agent'->>'status', '')) = 'DRAFT_GENERATED'
				 and coalesce(metadata->'agent'->>'draft_generated_at', metadata->'agent'->>'requested_at', '') != ''
				then extract(epoch from (now() - coalesce(nullif(metadata->'agent'->>'draft_generated_at', '')::timestamptz, nullif(metadata->'agent'->>'requested_at', '')::timestamptz)))::int
				else 0 end), 0)::int as oldest_pending_age_seconds,
			coalesce(sum(case when upper(coalesce(metadata->'agent'->>'auto_send_status', '')) = 'AUTO_SEND_RETRY_PENDING' then 1 else 0 end), 0)::int as auto_send_retry_pending_count,
			coalesce(sum(case when upper(coalesce(metadata->'agent'->>'auto_send_status', '')) = 'AUTO_SEND_BLOCKED_HUMAN' then 1 else 0 end), 0)::int as auto_send_blocked_human_count,
			coalesce(sum(case when upper(coalesce(metadata->'agent'->>'auto_send_status', '')) in ('AUTO_SEND_RETRY_PENDING', 'AUTO_SEND_BLOCKED_HUMAN') then 1 else 0 end), 0)::int as auto_send_issue_count
		from chat_sessions`

	args := make([]interface{}, 0, 5)
	args = append(args, reviewSLASeconds)
	clauses := make([]string, 0, 4)
	if filter.Channel != "" {
		args = append(args, filter.Channel)
		clauses = append(clauses, fmt.Sprintf("channel = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.HandoffStatus != "" {
		args = append(args, filter.HandoffStatus)
		clauses = append(clauses, fmt.Sprintf("handoff_status = $%d", len(args)))
	}
	if filter.ContactKey != "" {
		args = append(args, filter.ContactKey)
		clauses = append(clauses, fmt.Sprintf("contact_key = $%d", len(args)))
	}
	if len(clauses) > 0 {
		query += "\nwhere " + strings.Join(clauses, " and ")
	}

	var summary SessionsSummary
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&summary.TotalCount,
		&summary.PendingReviewCount,
		&summary.ReviewedCount,
		&summary.NoDraftCount,
		&summary.HumanOwnedCount,
		&summary.BotOwnedCount,
		&summary.DueSoonReviewCount,
		&summary.OverdueReviewCount,
		&summary.HighPriorityReviewCount,
		&summary.MediumPriorityReviewCount,
		&summary.LowPriorityReviewCount,
		&summary.OldestPendingAgeSeconds,
		&summary.AutoSendRetryPendingCount,
		&summary.AutoSendBlockedHumanCount,
		&summary.AutoSendIssueCount,
	)
	return summary, err
}

func (r *Repository) GetSession(ctx context.Context, id string) (Session, error) {
	row := r.pool.QueryRow(ctx, `
		select
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
		from chat_sessions
		where id = $1::uuid
	`, id)

	return scanSession(row)
}

func (r *Repository) scanSessionByID(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}, id string) (Session, error) {
	row := querier.QueryRow(ctx, `
		select
			id::text,
			channel,
			contact_key,
			coalesce(customer_phone, ''),
			coalesce(customer_name, ''),
			status,
			handoff_status,
			coalesce(current_owner_user_id::text, ''),
			last_message_at,
			last_inbound_at,
			last_outbound_at,
			metadata,
			created_at,
			updated_at
		from chat_sessions
		where id = $1::uuid
	`, id)

	return scanSession(row)
}

func (r *Repository) scanMessageByID(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}, id string) (Message, error) {
	row := querier.QueryRow(ctx, `
		select
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
		from chat_messages
		where id = $1::uuid
	`, id)
	return scanMessage(row)
}

func deliveryModeForPayload(payload map[string]interface{}) string {
	if strings.EqualFold(strings.TrimSpace(asString(payload["mode"])), "BOT_AUTO_REPLY") {
		return "BOT_AUTO_SEND"
	}
	return "MANUAL_CONTROLLED"
}

func (r *Repository) ListMessages(ctx context.Context, sessionID string, filter ListMessagesFilter) ([]Message, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.pool.Query(ctx, `
		select
			id::text,
			session_id::text,
			direction,
			kind,
			coalesce(provider_message_id, ''),
			coalesce(idempotency_key, ''),
			coalesce(sender_name, ''),
			coalesce(sender_phone, ''),
			coalesce(body, ''),
			payload,
			normalized_payload,
			processing_status,
			received_at,
			sent_at,
			created_at
		from chat_messages
		where session_id = $1::uuid
		order by created_at asc
		limit $2 offset $3
	`, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Message{}
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func scanSession(scanner interface {
	Scan(dest ...interface{}) error
}) (Session, error) {
	var item Session
	var ownerID string
	var metadataBytes []byte

	err := scanner.Scan(
		&item.ID,
		&item.Channel,
		&item.ContactKey,
		&item.CustomerPhone,
		&item.CustomerName,
		&item.Status,
		&item.HandoffStatus,
		&ownerID,
		&item.LastMessageAt,
		&item.LastInboundAt,
		&item.LastOutboundAt,
		&metadataBytes,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Session{}, err
	}

	item.CurrentOwnerUserID = ownerID
	item.Metadata = decodeMap(metadataBytes)

	return item, nil
}

func scanMessage(scanner interface {
	Scan(dest ...interface{}) error
}) (Message, error) {
	var item Message
	var payloadBytes []byte
	var normalizedBytes []byte

	err := scanner.Scan(
		&item.ID,
		&item.SessionID,
		&item.Direction,
		&item.Kind,
		&item.ProviderMessageID,
		&item.IdempotencyKey,
		&item.SenderName,
		&item.SenderPhone,
		&item.Body,
		&payloadBytes,
		&normalizedBytes,
		&item.ProcessingStatus,
		&item.ReceivedAt,
		&item.SentAt,
		&item.CreatedAt,
	)
	if err != nil {
		return Message{}, err
	}

	item.Payload = decodeMap(payloadBytes)
	item.NormalizedPayload = decodeMap(normalizedBytes)

	return item, nil
}

func scanToolCall(scanner interface {
	Scan(dest ...interface{}) error
}) (ToolCall, error) {
	var (
		item          ToolCall
		requestBytes  []byte
		responseBytes []byte
		messageID     string
		errorCode     string
		errorMessage  string
		finishedAt    *time.Time
	)

	if err := scanner.Scan(
		&item.ID,
		&item.SessionID,
		&messageID,
		&item.ToolName,
		&requestBytes,
		&responseBytes,
		&item.Status,
		&errorCode,
		&errorMessage,
		&item.StartedAt,
		&finishedAt,
		&item.CreatedAt,
	); err != nil {
		return ToolCall{}, err
	}

	item.MessageID = messageID
	item.RequestPayload = decodeMap(requestBytes)
	item.ResponsePayload = decodeMap(responseBytes)
	item.ErrorCode = errorCode
	item.ErrorMessage = errorMessage
	item.FinishedAt = finishedAt
	return item, nil
}

func scanReplyOutbound(scanner interface {
	Scan(dest ...interface{}) error
}) (ReplyOutbound, error) {
	var item ReplyOutbound
	var payloadBytes []byte

	err := scanner.Scan(
		&item.ID,
		&item.SessionID,
		&item.Channel,
		&item.Recipient,
		&payloadBytes,
		&item.Provider,
		&item.ProviderMessageID,
		&item.IdempotencyKey,
		&item.Status,
		&item.SentAt,
		&item.DeliveredAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return ReplyOutbound{}, err
	}

	item.Payload = decodeMap(payloadBytes)
	return item, nil
}

func scanHandoff(scanner interface {
	Scan(dest ...interface{}) error
}) (Handoff, error) {
	var item Handoff
	var assignedUserID string
	var metadataBytes []byte

	err := scanner.Scan(
		&item.ID,
		&item.SessionID,
		&item.RequestedBy,
		&item.Reason,
		&item.Status,
		&assignedUserID,
		&item.RequestedAt,
		&item.ResolvedAt,
		&metadataBytes,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Handoff{}, err
	}

	item.AssignedUserID = assignedUserID
	item.Metadata = decodeMap(metadataBytes)

	return item, nil
}

func encodeMap(input map[string]interface{}) (string, error) {
	if len(input) == 0 {
		return "{}", nil
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeMap(raw []byte) map[string]interface{} {
	if len(raw) == 0 {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func IsUniqueViolation(err error) bool {
	pgErr, ok := err.(*pgconn.PgError)
	if !ok {
		return false
	}
	return pgErr.Code == "23505"
}
