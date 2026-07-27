package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
