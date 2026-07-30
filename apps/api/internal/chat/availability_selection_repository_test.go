package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCreateReplyApprovedAvailabilityDraftRejectsHostileMediaMetadataPostgres(t *testing.T) {
	databaseURL := passengerStatePostgresTestURL(
		t,
		"real Repository.CreateReply approved availability draft metadata",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	requirePassengerStatePostgreSQL16(t, ctx, admin)
	schema := "availability_reply_v1_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "create schema "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create isolated reply schema: %v", err)
	}
	pool := passengerStatePostgresPoolForSchema(t, ctx, databaseURL, schema)
	defer func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "drop schema "+quotedSchema+" cascade")
		admin.Close()
	}()

	if _, err := admin.Exec(ctx, `
		create table `+quotedSchema+`.chat_sessions (
			id uuid primary key,
			channel text not null,
			contact_key text not null,
			customer_phone text,
			customer_name text,
			status text not null,
			handoff_status text not null,
			current_owner_user_id uuid,
			last_message_at timestamptz,
			last_inbound_at timestamptz,
			last_outbound_at timestamptz,
			metadata jsonb not null default '{}'::jsonb,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		);
		create table `+quotedSchema+`.chat_messages (
			id uuid primary key,
			session_id uuid not null references `+quotedSchema+`.chat_sessions(id),
			direction text not null,
			kind text not null default 'TEXT',
			provider_message_id text,
			idempotency_key text,
			sender_name text,
			sender_phone text,
			body text,
			payload jsonb not null default '{}'::jsonb,
			normalized_payload jsonb not null default '{}'::jsonb,
			processing_status text not null,
			received_at timestamptz not null,
			sent_at timestamptz,
			created_at timestamptz not null default now()
		);
		create unique index availability_reply_message_idempotency
			on `+quotedSchema+`.chat_messages(idempotency_key)
			where idempotency_key is not null;
		create table `+quotedSchema+`.outbound_messages (
			id uuid primary key default gen_random_uuid(),
			session_id uuid references `+quotedSchema+`.chat_sessions(id),
			channel text not null,
			recipient text not null,
			payload jsonb not null default '{}'::jsonb,
			provider text not null,
			provider_message_id text,
			idempotency_key text not null unique,
			status text not null,
			error_text text,
			sent_at timestamptz,
			delivered_at timestamptz,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)
	`); err != nil {
		t.Fatalf("create isolated reply tables: %v", err)
	}

	sessionID := uuid.NewString()
	ownerUserID := uuid.NewString()
	draftMessageID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		insert into chat_sessions (
			id, channel, contact_key, status, handoff_status, current_owner_user_id
		) values ($1::uuid, 'WHATSAPP', '5511999999999@s.whatsapp.net', 'ACTIVE', 'HUMAN', $2::uuid)
	`, sessionID, ownerUserID); err != nil {
		t.Fatalf("insert isolated human session: %v", err)
	}

	raw := availabilityPromptAuthorityRawResult()
	draft := availabilityPromptAuthorityMessage(
		draftMessageID,
		messageStatusAutomationDraft,
		raw,
		[]int{0},
	)
	draft.Body = buildAvailabilityListReplyForResultIndexes(raw, []int{0})
	for _, payload := range []map[string]interface{}{draft.Payload, draft.NormalizedPayload} {
		payload["mode"] = messageStatusAutomationDraft
		payload["intent"] = string(IntentAvailabilitySearch)
		payload["template_name"] = string(TemplateAvailabilityList)
	}
	draftPayload, err := json.Marshal(draft.Payload)
	if err != nil {
		t.Fatalf("encode availability draft payload: %v", err)
	}
	draftNormalized, err := json.Marshal(draft.NormalizedPayload)
	if err != nil {
		t.Fatalf("encode availability draft normalized payload: %v", err)
	}
	recordedAt := time.Date(2026, time.July, 28, 15, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		insert into chat_messages (
			id, session_id, direction, kind, idempotency_key, sender_name, body,
			payload, normalized_payload, processing_status, received_at, created_at
		) values (
			$1::uuid, $2::uuid, 'OUTBOUND', 'TEXT', $3, 'SHABAS', $4,
			$5::jsonb, $6::jsonb, $7, $8, $8
		)
	`,
		draftMessageID,
		sessionID,
		"postgres-availability-draft-"+draftMessageID,
		draft.Body,
		string(draftPayload),
		string(draftNormalized),
		messageStatusAutomationDraft,
		recordedAt,
	); err != nil {
		t.Fatalf("insert valid availability draft: %v", err)
	}

	forged := availabilityPromptAuthorityRawResult()
	forged.Results[0].Price = 1
	forged.Results[0].PackageName = "forged-package"
	var observed SendReplyInput
	sender := &fakeReplySender{
		enabled: true,
		result: SendReplyResult{
			ProviderMessageID: "postgres-approved-text-provider-id",
			ProviderStatus:    "SENT",
			SentAt:            recordedAt.Add(time.Minute),
		},
		beforeSend: func(input SendReplyInput) {
			observed = input
		},
	}
	repository := NewRepository(pool)
	service := NewService(repository, config.Config{ChatDebounceWindowMS: 250}, sender)
	result, err := service.Reply(ctx, ReplyInput{
		SessionID:      sessionID,
		OwnerUserID:    ownerUserID,
		DraftMessageID: draftMessageID,
		SenderName:     "Atendente",
		IdempotencyKey: "postgres-approved-text-reply-" + draftMessageID,
		Metadata: map[string]interface{}{
			"mode":             "ASSISTED_REPLY",
			"draft_message_id": "forged-draft",
			"review_action":    "EDITED",
			"body":             "forged body",
			"text":             "forged text",
			"kind":             "AUDIO",
			"media_kind":       "AUDIO",
			"media_base64":     "Zm9yZ2VkLWF1ZGlv",
			"media_mime_type":  "audio/ogg",
			"media_file_name":  "forged.ogg",
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(forged),
			},
			"source": "postgres-hostile-metadata-proof",
		},
	})
	if err != nil {
		t.Fatalf("approve availability draft through real Repository.CreateReply: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls=%d, want 1", sender.calls)
	}
	if observed.Message.Kind != "TEXT" || observed.Message.Body != draft.Body {
		t.Fatalf("sender did not observe canonical text draft: %+v", observed.Message)
	}
	assertApprovedAvailabilityReplyPayloadPostgres(
		t,
		"sender outbound",
		observed.Outbound.Payload,
		result.Message.ID,
		draftMessageID,
		draft.Body,
	)

	var (
		persistedKind       string
		persistedBody       string
		persistedStatus     string
		persistedPayloadRaw []byte
		persistedNormalRaw  []byte
	)
	if err := pool.QueryRow(ctx, `
		select kind, coalesce(body, ''), processing_status, payload, normalized_payload
		from chat_messages
		where id = $1::uuid
	`, result.Message.ID).Scan(
		&persistedKind,
		&persistedBody,
		&persistedStatus,
		&persistedPayloadRaw,
		&persistedNormalRaw,
	); err != nil {
		t.Fatalf("read persisted approved chat message: %v", err)
	}
	persistedPayload := decodeMap(persistedPayloadRaw)
	persistedNormalized := decodeMap(persistedNormalRaw)
	if persistedKind != "TEXT" || persistedBody != draft.Body || persistedStatus != "SENT" {
		t.Fatalf(
			"persisted approved chat message changed delivery: kind=%q body=%q status=%q",
			persistedKind,
			persistedBody,
			persistedStatus,
		)
	}
	assertApprovedAvailabilityReplyPayloadPostgres(
		t,
		"chat payload",
		persistedPayload,
		result.Message.ID,
		draftMessageID,
		draft.Body,
	)
	assertApprovedAvailabilityReplyPayloadPostgres(
		t,
		"chat normalized payload",
		persistedNormalized,
		result.Message.ID,
		draftMessageID,
		draft.Body,
	)
	persistedMessage := Message{
		ID:                result.Message.ID,
		Direction:         "OUTBOUND",
		Kind:              persistedKind,
		Body:              persistedBody,
		Payload:           persistedPayload,
		NormalizedPayload: persistedNormalized,
		ProcessingStatus:  persistedStatus,
	}
	if _, ok := availabilityPromptEventFromMessageV1(persistedMessage); !ok {
		t.Fatalf("persisted approved chat message lost matching structural event copies: %+v", persistedMessage)
	}

	var (
		outboundStatus     string
		outboundPayloadRaw []byte
	)
	if err := pool.QueryRow(ctx, `
		select status, payload
		from outbound_messages
		where id = $1::uuid
	`, result.Outbound.ID).Scan(&outboundStatus, &outboundPayloadRaw); err != nil {
		t.Fatalf("read persisted approved outbound: %v", err)
	}
	outboundPayload := decodeMap(outboundPayloadRaw)
	if outboundStatus != "SENT" {
		t.Fatalf("persisted approved outbound status=%q, want SENT", outboundStatus)
	}
	assertApprovedAvailabilityReplyPayloadPostgres(
		t,
		"persisted outbound",
		outboundPayload,
		result.Message.ID,
		draftMessageID,
		draft.Body,
	)

	history, err := repository.ListMessages(ctx, sessionID, ListMessagesFilter{Limit: 20})
	if err != nil {
		t.Fatalf("reload approved availability history: %v", err)
	}
	prompt := currentAvailabilitySelectionPromptContext(history)
	if prompt.OptionCount != 1 ||
		!prompt.HasCurrentFacts ||
		prompt.SourceMessageID != result.Message.ID {
		t.Fatalf("approved real-repository prompt lost validated draft facts: %+v", prompt)
	}
	availability := currentAvailabilitySelectionPromptAvailabilityContext(history)
	if availability == nil ||
		len(availability.Results) != 1 ||
		availability.Results[0].Price != raw.Results[0].Price ||
		availability.Results[0].PackageName != raw.Results[0].PackageName ||
		availability.Results[0].Price == forged.Results[0].Price ||
		availability.Results[0].PackageName == forged.Results[0].PackageName {
		t.Fatalf(
			"approved real-repository prompt used hostile facts: got=%+v validated=%+v forged=%+v",
			availability,
			raw.Results[0],
			forged.Results[0],
		)
	}
}

func assertApprovedAvailabilityReplyPayloadPostgres(
	t *testing.T,
	label string,
	payload map[string]interface{},
	messageID string,
	draftMessageID string,
	body string,
) {
	t.Helper()
	if got := strings.TrimSpace(asString(payload["mode"])); got != "DRAFT_REVIEW" {
		t.Fatalf("%s mode=%q, want DRAFT_REVIEW: %+v", label, got, payload)
	}
	if got := strings.TrimSpace(asString(payload["draft_message_id"])); got != draftMessageID {
		t.Fatalf("%s draft_message_id=%q, want %q: %+v", label, got, draftMessageID, payload)
	}
	if got := strings.TrimSpace(asString(payload["review_action"])); got != "APPROVED_AS_IS" {
		t.Fatalf("%s review_action=%q, want APPROVED_AS_IS: %+v", label, got, payload)
	}
	event, ok := decodeAvailabilityPromptEventV1(payload[availabilityPromptEventV1MessageKey])
	if !ok || event.SourceMessageID != messageID {
		t.Fatalf("%s availability event invalid for message %q: %+v", label, messageID, payload)
	}
	for _, key := range []string{
		"kind",
		"text",
		"media_kind",
		"media_base64",
		"media_mime_type",
		"media_file_name",
		"tool_context",
	} {
		if _, exists := payload[key]; exists {
			t.Fatalf("%s accepted hostile key %q: %+v", label, key, payload)
		}
	}
	if persistedBody, exists := payload["body"]; exists &&
		strings.TrimSpace(asString(persistedBody)) != body {
		t.Fatalf("%s body=%q, want %q: %+v", label, asString(persistedBody), body, payload)
	}
}

func TestAvailabilitySelectionStateApplyEventsSerializesSessionPostgres(t *testing.T) {
	databaseURL := passengerStatePostgresTestURL(t, "availability-selection canonical replay")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	requirePassengerStatePostgreSQL16(t, ctx, admin)
	schema := "availability_selection_v1_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "create schema "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create isolated schema: %v", err)
	}
	poolOne := passengerStatePostgresPoolForSchema(t, ctx, databaseURL, schema)
	poolTwo := passengerStatePostgresPoolForSchema(t, ctx, databaseURL, schema)
	defer func() {
		poolOne.Close()
		poolTwo.Close()
		_, _ = admin.Exec(context.Background(), "drop schema "+quotedSchema+" cascade")
		admin.Close()
	}()

	if _, err := admin.Exec(ctx, `
		create table `+quotedSchema+`.chat_sessions (
			id uuid primary key,
			channel text not null,
			contact_key text not null,
			customer_phone text,
			customer_name text,
			status text not null,
			handoff_status text not null,
			current_owner_user_id uuid,
			last_message_at timestamptz,
			last_inbound_at timestamptz,
			last_outbound_at timestamptz,
			metadata jsonb not null default '{}'::jsonb,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		);
		create table `+quotedSchema+`.chat_messages (
			id uuid primary key,
			session_id uuid not null references `+quotedSchema+`.chat_sessions(id),
			direction text not null,
			payload jsonb not null default '{}'::jsonb,
			normalized_payload jsonb not null default '{}'::jsonb,
			processing_status text not null,
			sent_at timestamptz,
			received_at timestamptz not null,
			created_at timestamptz not null default now()
		)
	`); err != nil {
		t.Fatalf("create isolated availability-selection tables: %v", err)
	}

	sessionID := uuid.NewString()
	passengerState := newPassengerClarificationStateV1()
	passengerState.BootstrapCompleted = true
	selectionState := newAvailabilitySelectionStateV1()
	selectionState.BootstrapCompleted = true
	metadata, err := json.Marshal(map[string]interface{}{"memory": map[string]interface{}{
		passengerClarificationStateV1MemoryKey: passengerState,
		availabilitySelectionStateV1MemoryKey:  selectionState,
	}})
	if err != nil {
		t.Fatalf("encode initial metadata: %v", err)
	}
	if _, err := poolOne.Exec(ctx, `
		insert into chat_sessions (id, channel, contact_key, status, handoff_status, metadata)
		values ($1::uuid, 'WHATSAPP', 'availability-selection-postgres', 'ACTIVE', 'BOT', $2::jsonb)
	`, sessionID, string(metadata)); err != nil {
		t.Fatalf("insert isolated session: %v", err)
	}

	now := availabilityTestObservedAt()
	messageIDs := []string{uuid.NewString(), uuid.NewString()}
	for index, messageID := range messageIDs {
		recordedAt := now.Add(time.Duration(index) * time.Minute)
		if _, err := poolOne.Exec(ctx, `
			insert into chat_messages (
				id, session_id, direction, processing_status, received_at, created_at
			) values ($1::uuid, $2::uuid, 'INBOUND', 'BUFFERED_PENDING', $3, $3)
		`, messageID, sessionID, recordedAt); err != nil {
			t.Fatalf("insert inbound selection event message: %v", err)
		}
	}
	availabilityA := availabilityOptionPromptFutureResultAt(now)
	availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
	availabilityB.Results[0].TripID = "postgres-trip-b"
	availabilityB.Results[0].BoardStopID = "postgres-board-b"
	availabilityB.Results[0].AlightStopID = "postgres-alight-b"
	snapshots := []AvailabilitySelectionSnapshotV1{
		mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1),
		mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1),
	}

	bootstrapSessionID := uuid.NewString()
	bootstrapBoundaryMessageID := uuid.NewString()
	bootstrapMemory := map[string]interface{}{
		passengerClarificationStateV1MemoryKey: passengerState,
	}
	bootstrapMetadata, err := json.Marshal(map[string]interface{}{
		"memory": bootstrapMemory,
		"agent": map[string]interface{}{
			canonicalAvailabilityFactsInvalidatedMetadataKey:               true,
			canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey: bootstrapBoundaryMessageID,
			canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey: now.Add(-time.Minute).UTC().Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatalf("encode invalidated bootstrap metadata: %v", err)
	}
	if _, err := poolOne.Exec(ctx, `
		insert into chat_sessions (id, channel, contact_key, status, handoff_status, metadata)
		values ($1::uuid, 'WHATSAPP', 'availability-bootstrap-postgres', 'ACTIVE', 'BOT', $2::jsonb)
	`, bootstrapSessionID, string(bootstrapMetadata)); err != nil {
		t.Fatalf("insert invalidated bootstrap session: %v", err)
	}
	bootstrapSelectionMessageID := uuid.NewString()
	bootstrapSelectionEvent := materializedAvailabilitySelectionEventForTest(
		bootstrapSelectionMessageID,
		"postgres-bootstrap-old-projection",
		"postgres-bootstrap-old-prompt",
		snapshots[0],
	)
	bootstrapSelectionPayload, err := json.Marshal(map[string]interface{}{
		availabilitySelectionEventsV1MessageKey: []AvailabilitySelectionEventV1{
			bootstrapSelectionEvent,
		},
	})
	if err != nil {
		t.Fatalf("encode pre-boundary selection event: %v", err)
	}
	if _, err := poolOne.Exec(ctx, `
		insert into chat_messages (
			id, session_id, direction, normalized_payload, processing_status, received_at, created_at
		) values ($1::uuid, $2::uuid, 'INBOUND', $3::jsonb, 'PROCESSED', $4, $4)
	`, bootstrapSelectionMessageID, bootstrapSessionID, string(bootstrapSelectionPayload), now.Add(-2*time.Minute)); err != nil {
		t.Fatalf("insert pre-boundary selection event: %v", err)
	}
	if _, err := poolOne.Exec(ctx, `
		insert into chat_messages (
			id, session_id, direction, processing_status, received_at, created_at
		) values ($1::uuid, $2::uuid, 'INBOUND', 'PROCESSED', $3, $3)
	`, bootstrapBoundaryMessageID, bootstrapSessionID, now.Add(-time.Minute)); err != nil {
		t.Fatalf("insert invalidation boundary message: %v", err)
	}
	bootstrapResult, err := NewRepository(poolOne).ApplyPassengerClarificationEventsV1(
		ctx,
		ApplyPassengerClarificationEventsV1Input{SessionID: bootstrapSessionID},
	)
	if err != nil {
		t.Fatalf("bootstrap invalidated selection state in PostgreSQL: %v", err)
	}
	if bootstrapResult.AvailabilitySelectionState.Status != AvailabilitySelectionStatusInvalidated ||
		bootstrapResult.AvailabilitySelectionState.SelectedOptionIndex != 0 ||
		bootstrapResult.AvailabilitySelectionState.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf(
			"PostgreSQL bootstrap restored pre-boundary authority: %+v",
			bootstrapResult.AvailabilitySelectionState,
		)
	}
	if _, _, ok := resolveBookingCreateSelectionFromState(
		"quero reservar opção 1",
		bootstrapResult.AvailabilitySelectionState,
	); ok {
		t.Fatal("PostgreSQL invalidation bootstrap reopened booking_create")
	}
	freshBootstrapSelectionMessageID := uuid.NewString()
	if _, err := poolOne.Exec(ctx, `
		insert into chat_messages (
			id, session_id, direction, processing_status, received_at, created_at
		) values ($1::uuid, $2::uuid, 'INBOUND', 'BUFFERED_PENDING', $3, $3)
	`, freshBootstrapSelectionMessageID, bootstrapSessionID, now); err != nil {
		t.Fatalf("insert post-boundary selection message: %v", err)
	}
	reopenedBootstrap, err := NewRepository(poolOne).ApplyPassengerClarificationEventsV1(
		ctx,
		ApplyPassengerClarificationEventsV1Input{
			SessionID: bootstrapSessionID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					freshBootstrapSelectionMessageID,
					"postgres-bootstrap-fresh-projection",
					"postgres-bootstrap-fresh-prompt",
					snapshots[1],
				),
			},
		},
	)
	if err != nil {
		t.Fatalf("apply post-boundary PostgreSQL materialization: %v", err)
	}
	if reopenedBootstrap.AvailabilitySelectionState.Status != AvailabilitySelectionStatusBookable ||
		reopenedBootstrap.AvailabilitySelectionState.SelectionEventMessageID != freshBootstrapSelectionMessageID ||
		reopenedBootstrap.AvailabilitySelectionState.Snapshot.TripID != snapshots[1].TripID {
		t.Fatalf(
			"post-boundary PostgreSQL materialization did not replace invalidation: %+v",
			reopenedBootstrap.AvailabilitySelectionState,
		)
	}

	inputs := make([]ApplyPassengerClarificationEventsV1Input, 0, 2)
	for index, messageID := range messageIDs {
		inputs = append(inputs, ApplyPassengerClarificationEventsV1Input{
			SessionID: sessionID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					messageID,
					"postgres-projection-"+messageID,
					"postgres-prompt-"+messageID,
					snapshots[index],
				),
			},
		})
	}
	inputs[0].AvailabilitySelectionEvents[0].Order = AvailabilitySelectionEventOrderV1{
		ReceivedAt:   now.Add(48 * time.Hour),
		CreatedAt:    now.Add(48 * time.Hour),
		MessageID:    "forged-caller-order-s1",
		EventOrdinal: 99,
	}
	inputs[1].AvailabilitySelectionEvents[0].Order = AvailabilitySelectionEventOrderV1{
		ReceivedAt: now.Add(-48 * time.Hour),
		CreatedAt:  now.Add(-48 * time.Hour),
		MessageID:  "forged-caller-order-s2",
	}

	repositories := []*Repository{NewRepository(poolOne), NewRepository(poolTwo)}
	applyAvailabilitySelectionEventsNewerFirstPostgres(
		t,
		ctx,
		poolOne,
		poolTwo,
		sessionID,
		inputs[1],
		inputs[0],
	)

	reloaded, err := repositories[0].GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload serialized availability selection state: %v", err)
	}
	state, ok := availabilitySelectionStateV1FromSession(reloaded)
	if !ok || state.Status != AvailabilitySelectionStatusBookable ||
		len(state.AppliedEventIDs) != 2 {
		t.Fatalf("serialized PostgreSQL apply lost an event: %+v", state)
	}
	if state.Snapshot.TripID != snapshots[1].TripID ||
		state.SelectionEventMessageID != messageIDs[1] ||
		state.LastAppliedEventOrder.MessageID != messageIDs[1] {
		t.Fatalf("lock acquisition order replaced causally newer S2: %+v", state)
	}
	var persistedEventCount int
	if err := poolOne.QueryRow(ctx, `
		select count(*)::int
		from chat_messages
		where session_id = $1::uuid
			and normalized_payload ? 'availability_selection_events_v1'
	`, sessionID).Scan(&persistedEventCount); err != nil {
		t.Fatalf("count persisted structured selection events: %v", err)
	}
	if persistedEventCount != 2 {
		t.Fatalf("persisted structured selection event count=%d, want 2", persistedEventCount)
	}
	assertAvailabilitySelectionPostgresRestartMatches(
		t,
		ctx,
		poolOne,
		sessionID,
		state,
	)

	rejectionSessionID := uuid.NewString()
	if _, err := poolOne.Exec(ctx, `
		insert into chat_sessions (id, channel, contact_key, status, handoff_status, metadata)
		values ($1::uuid, 'WHATSAPP', 'availability-rejection-postgres', 'ACTIVE', 'BOT', $2::jsonb)
	`, rejectionSessionID, string(metadata)); err != nil {
		t.Fatalf("insert rejection-order session: %v", err)
	}
	oldSelectionMessageID := uuid.NewString()
	newRejectionMessageID := uuid.NewString()
	for index, messageID := range []string{oldSelectionMessageID, newRejectionMessageID} {
		recordedAt := now.Add(time.Duration(index) * time.Minute)
		if _, err := poolOne.Exec(ctx, `
			insert into chat_messages (
				id, session_id, direction, processing_status, received_at, created_at
			) values ($1::uuid, $2::uuid, 'INBOUND', 'BUFFERED_PENDING', $3, $3)
		`, messageID, rejectionSessionID, recordedAt); err != nil {
			t.Fatalf("insert rejection-order event message: %v", err)
		}
	}
	const rejectionPromptSource = "postgres-causal-rejection-prompt-b"
	const materializationPromptSource = "postgres-causal-materialization-prompt-a"
	newerRejection := ApplyPassengerClarificationEventsV1Input{
		SessionID: rejectionSessionID,
		AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
			Type:                              AvailabilitySelectionEventRejected,
			MessageID:                         newRejectionMessageID,
			AvailabilityPromptSourceMessageID: rejectionPromptSource,
			RejectedOptionIndexes:             []int{1},
		}},
	}
	olderMaterialization := ApplyPassengerClarificationEventsV1Input{
		SessionID: rejectionSessionID,
		AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
			materializedAvailabilitySelectionEventForTest(
				oldSelectionMessageID,
				"postgres-old-selection-projection",
				materializationPromptSource,
				snapshots[0],
			),
		},
	}
	applyAvailabilitySelectionEventsNewerFirstPostgres(
		t,
		ctx,
		poolOne,
		poolTwo,
		rejectionSessionID,
		newerRejection,
		olderMaterialization,
	)
	rejectionSession, err := repositories[0].GetSession(ctx, rejectionSessionID)
	if err != nil {
		t.Fatalf("reload causal rejection state: %v", err)
	}
	rejectionState, ok := availabilitySelectionStateV1FromSession(rejectionSession)
	if !ok || rejectionState.Status != AvailabilitySelectionStatusBookable ||
		rejectionState.SelectionEventMessageID != oldSelectionMessageID ||
		rejectionState.AvailabilityPromptSourceMessageID != materializationPromptSource ||
		rejectionState.Snapshot.TripID != snapshots[0].TripID ||
		rejectionState.LastAppliedEventOrder.MessageID != newRejectionMessageID ||
		len(rejectionState.AppliedEventIDs) != 2 {
		t.Fatalf("lock order diverged from canonical PostgreSQL replay: %+v", rejectionState)
	}
	if !rejectionState.rejectsPromptOption(rejectionPromptSource, 1, snapshots[0].TripDate) {
		t.Fatalf("canonical replay lost the prompt B rejection: %+v", rejectionState.Rejections)
	}
	if _, selected, ok := resolveBookingCreateSelectionFromState(
		"quero reservar opção 1",
		rejectionState,
	); !ok || selected.TripID != snapshots[0].TripID {
		t.Fatal("PostgreSQL canonical replay did not preserve prompt A authority")
	}
	assertAvailabilitySelectionPostgresRestartMatches(
		t,
		ctx,
		poolOne,
		rejectionSessionID,
		rejectionState,
	)
}

func applyAvailabilitySelectionEventsNewerFirstPostgres(
	t *testing.T,
	ctx context.Context,
	poolOne *pgxpool.Pool,
	poolTwo *pgxpool.Pool,
	sessionID string,
	newer ApplyPassengerClarificationEventsV1Input,
	older ApplyPassengerClarificationEventsV1Input,
) {
	t.Helper()
	blocker, err := poolOne.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin causal-order blocker transaction: %v", err)
	}
	if _, err := blocker.Exec(
		ctx,
		`select id from chat_sessions where id = $1::uuid for update`,
		sessionID,
	); err != nil {
		_ = blocker.Rollback(ctx)
		t.Fatalf("lock causal-order session: %v", err)
	}

	type applyResult struct {
		order string
		err   error
	}
	results := make(chan applyResult, 2)
	go func() {
		_, applyErr := NewRepository(poolTwo).ApplyPassengerClarificationEventsV1(ctx, newer)
		results <- applyResult{order: "newer", err: applyErr}
	}()
	select {
	case result := <-results:
		_ = blocker.Rollback(ctx)
		t.Fatalf("newer causal event bypassed the session row lock: %v", result.err)
	case <-time.After(75 * time.Millisecond):
	}
	go func() {
		_, applyErr := NewRepository(poolOne).ApplyPassengerClarificationEventsV1(ctx, older)
		results <- applyResult{order: "older", err: applyErr}
	}()
	select {
	case result := <-results:
		_ = blocker.Rollback(ctx)
		t.Fatalf("older causal event bypassed the session row lock: %v", result.err)
	case <-time.After(75 * time.Millisecond):
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release causal-order blocker: %v", err)
	}
	completionOrder := make([]string, 0, 2)
	for index := 0; index < 2; index++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("apply serialized causal event: %v", result.err)
			}
			completionOrder = append(completionOrder, result.order)
		case <-ctx.Done():
			t.Fatalf("serialized causal event timed out: %v", ctx.Err())
		}
	}
	if len(completionOrder) != 2 ||
		completionOrder[0] != "newer" ||
		completionOrder[1] != "older" {
		t.Fatalf("PostgreSQL did not acquire the session lock in inverted causal order: %v", completionOrder)
	}
}

func assertAvailabilitySelectionPostgresRestartMatches(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	sessionID string,
	live AvailabilitySelectionStateV1,
) {
	t.Helper()
	liveJSON, err := json.Marshal(live)
	if err != nil {
		t.Fatalf("marshal live PostgreSQL availability state: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		update chat_sessions
		set metadata = metadata #- '{memory,availability_selection_state_v1}'
		where id = $1::uuid
	`, sessionID); err != nil {
		t.Fatalf("clear PostgreSQL availability projection for restart: %v", err)
	}
	restarted, err := NewRepository(pool).ApplyPassengerClarificationEventsV1(
		ctx,
		ApplyPassengerClarificationEventsV1Input{SessionID: sessionID},
	)
	if err != nil {
		t.Fatalf("replay PostgreSQL availability state after restart: %v", err)
	}
	restartedJSON, err := json.Marshal(restarted.AvailabilitySelectionState)
	if err != nil {
		t.Fatalf("marshal restarted PostgreSQL availability state: %v", err)
	}
	if string(restartedJSON) != string(liveJSON) {
		t.Fatalf(
			"PostgreSQL live state differs from restart replay: live=%s restart=%s",
			liveJSON,
			restartedJSON,
		)
	}
}
