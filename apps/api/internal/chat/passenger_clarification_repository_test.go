package chat

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPassengerStateApplyEventsSerializesSessionPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("CHAT_PASSENGER_STATE_POSTGRES_TEST_URL"))
	if databaseURL == "" {
		t.Skip("set CHAT_PASSENGER_STATE_POSTGRES_TEST_URL to execute the required PostgreSQL concurrency proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	schema := "passenger_b1_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
		)
	`); err != nil {
		t.Fatalf("create isolated chat_sessions: %v", err)
	}

	sessionID := uuid.NewString()
	initial := newPassengerClarificationStateV1()
	initial.BootstrapCompleted = true
	metadata, err := json.Marshal(map[string]interface{}{"memory": map[string]interface{}{
		passengerClarificationStateV1MemoryKey: initial,
	}})
	if err != nil {
		t.Fatalf("encode initial state: %v", err)
	}
	if _, err := poolOne.Exec(ctx, `
		insert into chat_sessions (id, channel, contact_key, status, handoff_status, metadata)
		values ($1::uuid, 'WHATSAPP', 'postgres-concurrency', 'ACTIVE', 'BOT', $2::jsonb)
	`, sessionID, string(metadata)); err != nil {
		t.Fatalf("insert isolated session: %v", err)
	}

	blocker, err := poolOne.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin blocker transaction: %v", err)
	}
	if _, err := blocker.Exec(ctx, `select id from chat_sessions where id = $1::uuid for update`, sessionID); err != nil {
		_ = blocker.Rollback(ctx)
		t.Fatalf("lock isolated session: %v", err)
	}

	repositories := []*Repository{NewRepository(poolOne), NewRepository(poolTwo)}
	inputs := []ApplyPassengerClarificationEventsV1Input{
		{
			SessionID: sessionID,
			Events: []PassengerClarificationEventV1{{
				Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
				MessageID: "postgres-passenger-event", Value: 2, ValueKnown: true,
				PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
			}},
		},
		{
			SessionID: sessionID,
			Events: []PassengerClarificationEventV1{{
				Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
				MessageID: "postgres-child-event", Value: 0, ValueKnown: true,
			}},
		},
	}
	type applyResult struct{ err error }
	results := make(chan applyResult, 2)
	for i := range repositories {
		repository := repositories[i]
		input := inputs[i]
		go func() {
			_, applyErr := repository.ApplyPassengerClarificationEventsV1(ctx, input)
			results <- applyResult{err: applyErr}
		}()
	}
	select {
	case result := <-results:
		_ = blocker.Rollback(ctx)
		t.Fatalf("ApplyPassengerClarificationEventsV1 bypassed the held session row lock: %v", result.err)
	case <-time.After(75 * time.Millisecond):
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release blocker transaction: %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("serialized passenger apply failed: %v", result.err)
			}
		case <-ctx.Done():
			t.Fatalf("serialized passenger apply timed out: %v", ctx.Err())
		}
	}

	finalSession, err := repositories[0].GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload serialized passenger state: %v", err)
	}
	state, ok := passengerClarificationStateV1FromSession(finalSession)
	if !ok || !state.PassengerCountKnown || state.PassengerCount != 2 ||
		!state.ChildUnder5CountKnown || state.ChildUnder5Count != 0 ||
		!containsString(state.AppliedMessageIDs, "postgres-passenger-event") ||
		!containsString(state.AppliedMessageIDs, "postgres-child-event") {
		t.Fatalf("serialized PostgreSQL apply lost an event: %+v", state)
	}
}

func TestPassengerPostBookingAuthorityIgnoresInactivePassengersPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("CHAT_PASSENGER_STATE_POSTGRES_TEST_URL"))
	if databaseURL == "" {
		t.Skip("set CHAT_PASSENGER_STATE_POSTGRES_TEST_URL to execute the required PostgreSQL post-booking authority proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	schema := "passenger_b1_booking_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "create schema "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create isolated booking schema: %v", err)
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
		create table `+quotedSchema+`.bookings (
			id uuid primary key
		);
		create table `+quotedSchema+`.booking_passengers (
			id uuid primary key,
			booking_id uuid not null references `+quotedSchema+`.bookings(id),
			notes text,
			is_active boolean not null default true
		)
	`); err != nil {
		t.Fatalf("create isolated post-booking tables: %v", err)
	}

	insertSession := func(contactKey string, bookingID string) string {
		t.Helper()
		sessionID := uuid.NewString()
		initial := newPassengerClarificationStateV1()
		initial.BootstrapCompleted = true
		metadata, marshalErr := json.Marshal(map[string]interface{}{
			"memory": map[string]interface{}{
				"canonical_state": map[string]interface{}{
					"booking": map[string]interface{}{"booking_id": bookingID},
				},
				passengerClarificationStateV1MemoryKey: initial,
			},
		})
		if marshalErr != nil {
			t.Fatalf("encode %s session metadata: %v", contactKey, marshalErr)
		}
		if _, insertErr := pool.Exec(ctx, `
			insert into chat_sessions (id, channel, contact_key, status, handoff_status, metadata)
			values ($1::uuid, 'WHATSAPP', $2, 'ACTIVE', 'BOT', $3::jsonb)
		`, sessionID, contactKey, string(metadata)); insertErr != nil {
			t.Fatalf("insert %s session: %v", contactKey, insertErr)
		}
		return sessionID
	}

	repository := NewRepository(pool)
	bookingID := uuid.NewString()
	if _, err := pool.Exec(ctx, `insert into bookings (id) values ($1::uuid)`, bookingID); err != nil {
		t.Fatalf("insert refresh booking: %v", err)
	}
	passengerIDs := make([]string, 0, 4)
	for i, notes := range []string{
		"ADULTO",
		"CRIANCA_DE_COLO_ATE_5_ANOS",
		"ADULTO",
		"CRIANCA_DE_COLO_ATE_5_ANOS",
	} {
		passengerID := uuid.NewString()
		passengerIDs = append(passengerIDs, passengerID)
		if _, err := pool.Exec(ctx, `
			insert into booking_passengers (id, booking_id, notes, is_active)
			values ($1::uuid, $2::uuid, $3, true)
		`, passengerID, bookingID, notes); err != nil {
			t.Fatalf("insert active passenger %d: %v", i, err)
		}
	}
	sessionID := insertSession("postgres-post-booking-refresh", bookingID)
	initial, err := repository.ApplyPassengerClarificationEventsV1(ctx, ApplyPassengerClarificationEventsV1Input{SessionID: sessionID})
	if err != nil {
		t.Fatalf("bootstrap four-active post-booking authority: %v", err)
	}
	if initial.State.Authority != PassengerClarificationAuthorityPostBooking ||
		!initial.State.PassengerCountKnown || initial.State.PassengerCount != 4 ||
		!initial.State.ChildUnder5CountKnown || initial.State.ChildUnder5Count != 2 {
		t.Fatalf("initial post-booking authority must contain four active passengers: %+v", initial.State)
	}
	initialProjection := collectBookingDraftContextFromState(initial.Session, nil, "")
	if !initialProjection.BookingCreated || initialProjection.PassengerCount != 4 ||
		initialProjection.ChildUnder5Count != 2 || initialProjection.ExpectedDocumentCount != 4 {
		t.Fatalf("initial post-booking projection must contain four active passengers: %+v", initialProjection)
	}

	if _, err := pool.Exec(ctx, `
		update booking_passengers
		set is_active = false
		where id in ($1::uuid, $2::uuid)
	`, passengerIDs[2], passengerIDs[3]); err != nil {
		t.Fatalf("deactivate two passengers: %v", err)
	}
	refreshed, err := repository.ApplyPassengerClarificationEventsV1(ctx, ApplyPassengerClarificationEventsV1Input{SessionID: sessionID})
	if err != nil {
		t.Fatalf("refresh two-active post-booking authority: %v", err)
	}
	if refreshed.State.Authority != PassengerClarificationAuthorityPostBooking ||
		!refreshed.State.PassengerCountKnown || refreshed.State.PassengerCount != 2 ||
		!refreshed.State.ChildUnder5CountKnown || refreshed.State.ChildUnder5Count != 1 {
		t.Fatalf("post-booking authority did not refresh to two active passengers: %+v", refreshed.State)
	}
	refreshedProjection := collectBookingDraftContextFromState(refreshed.Session, nil, "")
	if !refreshedProjection.BookingCreated || refreshedProjection.PassengerCount != 2 ||
		refreshedProjection.ChildUnder5Count != 1 || refreshedProjection.ExpectedDocumentCount != 2 {
		t.Fatalf("post-booking projection did not refresh to two active passengers: %+v", refreshedProjection)
	}
	persistedTwo, err := repository.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload persisted two-active authority: %v", err)
	}
	persistedTwoState, ok := passengerClarificationStateV1FromSession(persistedTwo)
	if !ok || persistedTwoState.PassengerCount != 2 || persistedTwoState.ChildUnder5Count != 1 {
		t.Fatalf("refreshed two-active authority was not persisted: %+v", persistedTwoState)
	}

	if _, err := pool.Exec(ctx, `alter table booking_passengers rename to booking_passengers_unavailable`); err != nil {
		t.Fatalf("make active-passenger query unavailable: %v", err)
	}
	_, refreshErr := repository.ApplyPassengerClarificationEventsV1(ctx, ApplyPassengerClarificationEventsV1Input{SessionID: sessionID})
	if _, err := pool.Exec(ctx, `alter table booking_passengers_unavailable rename to booking_passengers`); err != nil {
		t.Fatalf("restore active-passenger query table: %v", err)
	}
	if refreshErr == nil {
		t.Fatal("expected active-passenger query error instead of stale post-booking fallback")
	}
	persistedAfterQueryError, err := repository.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload authority after query error: %v", err)
	}
	persistedAfterQueryErrorState, ok := passengerClarificationStateV1FromSession(persistedAfterQueryError)
	if !ok || persistedAfterQueryErrorState.PassengerCount != 2 || persistedAfterQueryErrorState.ChildUnder5Count != 1 {
		t.Fatalf("query error changed persisted post-booking authority: %+v", persistedAfterQueryErrorState)
	}

	if _, err := pool.Exec(ctx, `
		update booking_passengers
		set is_active = false
		where booking_id = $1::uuid
	`, bookingID); err != nil {
		t.Fatalf("deactivate all passengers: %v", err)
	}
	unknown, err := repository.ApplyPassengerClarificationEventsV1(ctx, ApplyPassengerClarificationEventsV1Input{SessionID: sessionID})
	if err != nil {
		t.Fatalf("refresh zero-active post-booking authority: %v", err)
	}
	if unknown.State.Authority == PassengerClarificationAuthorityPostBooking ||
		unknown.State.PassengerCountKnown || unknown.State.ChildUnder5CountKnown || unknown.State.HasEvidence {
		t.Fatalf("zero active passengers must clear stale post-booking authority: %+v", unknown.State)
	}
	unknownProjection := collectBookingDraftContextFromState(unknown.Session, nil, "")
	if unknownProjection.BookingCreated || unknownProjection.ExpectedDocumentCount != 0 {
		t.Fatalf("zero active passengers fabricated post-booking projection: %+v", unknownProjection)
	}
	persistedUnknown, err := repository.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload persisted zero-active authority: %v", err)
	}
	persistedUnknownState, ok := passengerClarificationStateV1FromSession(persistedUnknown)
	if !ok || persistedUnknownState.Authority == PassengerClarificationAuthorityPostBooking ||
		persistedUnknownState.PassengerCountKnown || persistedUnknownState.ChildUnder5CountKnown {
		t.Fatalf("zero-active UNKNOWN state was not persisted: %+v", persistedUnknownState)
	}
}

func passengerStatePostgresPoolForSchema(t *testing.T, ctx context.Context, databaseURL, schema string) *pgxpool.Pool {
	t.Helper()
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL URL: %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("open PostgreSQL schema pool: %v", err)
	}
	return pool
}
