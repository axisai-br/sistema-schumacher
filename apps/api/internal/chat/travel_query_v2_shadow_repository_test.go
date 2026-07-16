package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const travelQueryV2ShadowRecoveryIndexName = "idx_chat_messages_travel_query_v2_shadow_recovery_due_at"

func TestTravelQueryV2ShadowRecoveryMigrationDefinesMarkerAndPartialIndex(t *testing.T) {
	migration := readTravelQueryV2ShadowSQLTestFile(t, "../../migrations/0021_chat_travel_query_v2_shadow_recovery_due_at.sql")
	lowerMigration := strings.ToLower(migration)
	for _, expected := range []string{
		"add column if not exists travel_query_v2_shadow_recovery_due_at timestamptz",
		"create index if not exists " + travelQueryV2ShadowRecoveryIndexName,
		"(travel_query_v2_shadow_recovery_due_at, id)",
		"where travel_query_v2_shadow_recovery_due_at is not null",
		"jsonb_each(message.normalized_payload -> 'travel_query_v2_shadow_claims')",
		"claim.value ->> 'status' = 'in_progress'",
	} {
		if !strings.Contains(lowerMigration, expected) {
			t.Fatalf("migration must contain %q", expected)
		}
	}
}

func TestTravelQueryV2ShadowRecoveryCandidateIsIndexFirstAndBounded(t *testing.T) {
	candidate := strings.ToLower(travelQueryV2ShadowRecoveryCandidateSQL)
	for _, expected := range []string{
		"travel_query_v2_shadow_recovery_due_at is not null",
		"travel_query_v2_shadow_recovery_due_at <= statement_timestamp()",
		"order by message.travel_query_v2_shadow_recovery_due_at, message.id",
		"limit $1",
		"for update of message skip locked",
	} {
		if !strings.Contains(candidate, expected) {
			t.Fatalf("candidate query must contain %q", expected)
		}
	}
	for _, forbidden := range []string{"jsonb_each", "lateral", "exists (", "created_at"} {
		if strings.Contains(candidate, forbidden) {
			t.Fatalf("candidate query must not inspect the ledger before LIMIT: found %q", forbidden)
		}
	}

	fullQuery := strings.ToLower(recoverExpiredTravelQueryV2ShadowClaimsSQL)
	limitAt := strings.Index(fullQuery, "limit $1")
	ledgerAt := strings.Index(fullQuery, "jsonb_each")
	if limitAt < 0 || ledgerAt < 0 || ledgerAt < limitAt {
		t.Fatalf("ledger must be opened only after the indexed LIMIT: limit=%d jsonb_each=%d", limitAt, ledgerAt)
	}
	if !strings.Contains(fullQuery, "travel_query_v2_shadow_recovery_due_at = transformed.next_recovery_due_at") {
		t.Fatal("recovery must atomically persist the next marker")
	}
}

func TestTravelQueryV2ShadowRecoveryExplainCheckCoversIndexAndLargeOrdinaryTable(t *testing.T) {
	verification := strings.ToLower(readTravelQueryV2ShadowSQLTestFile(t, "../../migrations/checks/0021_chat_travel_query_v2_shadow_recovery_due_at_explain.sql"))
	for _, expected := range []string{
		"explain (format json, costs off)",
		travelQueryV2ShadowRecoveryIndexName,
		"generate_series(1, 20000)",
		"generate_series(1, 120)",
		"limit 50",
		"for update of message skip locked",
		"candidate_count <> 50",
	} {
		if !strings.Contains(verification, expected) {
			t.Fatalf("EXPLAIN verification must contain %q", expected)
		}
	}
}

func TestTravelQueryV2ShadowRecoveryPostgresMarkerLifecycleAndBoundedProgress(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("CHAT_TRAVEL_V2_SHADOW_POSTGRES_TEST_URL"))
	if databaseURL == "" {
		t.Skip("set CHAT_TRAVEL_V2_SHADOW_POSTGRES_TEST_URL to run PostgreSQL repository verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL verification pool: %v", err)
	}
	defer pool.Close()
	migration := readTravelQueryV2ShadowSQLTestFile(t, "../../migrations/0021_chat_travel_query_v2_shadow_recovery_due_at.sql")
	if _, err := pool.Exec(ctx, migration); err != nil {
		t.Fatalf("apply recovery marker migration: %v", err)
	}
	repository := NewRepository(pool)

	lifecycleSessionID := "10000000-0000-0000-0000-000000000001"
	lifecycleMessageID := "20000000-0000-0000-0000-000000000001"
	batchSessionID := "10000000-0000-0000-0000-000000000002"
	cleanupFixture := seedTravelQueryV2ShadowRecoverySessions(t, ctx, pool, lifecycleSessionID, batchSessionID)
	defer cleanupFixture()
	insertTravelQueryV2ShadowRecoveryMessages(t, ctx, pool, "seed lifecycle message", `
		select
			$1::uuid as id,
			$2::uuid as session_id,
			'{}'::jsonb as normalized_payload,
			null::timestamptz as recovery_due_at,
			statement_timestamp() as created_at
	`, lifecycleMessageID, lifecycleSessionID)
	claim, err := repository.ClaimTravelQueryV2Shadow(ctx, lifecycleSessionID, lifecycleMessageID, "lifecycle-key", 2*time.Minute)
	if err != nil || claim.Status != TravelQueryV2ShadowClaimAcquired {
		t.Fatalf("acquire lifecycle claim: result=%+v err=%v", claim, err)
	}
	var markerMatchesLease bool
	if err := pool.QueryRow(ctx, `
		select floor(extract(epoch from travel_query_v2_shadow_recovery_due_at) * 1000)::bigint
			= (normalized_payload -> $2 -> $3 ->> 'lease_expires_at_unix_ms')::bigint
		from chat_messages
		where id = $1::uuid
	`, lifecycleMessageID, travelQueryV2ShadowClaimsPayloadKey, "lifecycle-key").Scan(&markerMatchesLease); err != nil || !markerMatchesLease {
		t.Fatalf("claim marker must match JSONB lease atomically: matches=%t err=%v", markerMatchesLease, err)
	}
	if err := repository.CompleteTravelQueryV2Shadow(ctx, lifecycleSessionID, lifecycleMessageID, "lifecycle-key", abandonedTravelQueryV2ShadowSummary()); err != nil {
		t.Fatalf("complete lifecycle claim: %v", err)
	}
	var markerCleared bool
	if err := pool.QueryRow(ctx, `
		select travel_query_v2_shadow_recovery_due_at is null
		from chat_messages
		where id = $1::uuid
	`, lifecycleMessageID).Scan(&markerCleared); err != nil || !markerCleared {
		t.Fatalf("completion must clear last marker: cleared=%t err=%v", markerCleared, err)
	}

	insertTravelQueryV2ShadowRecoveryMessages(t, ctx, pool, "seed ordinary messages", `
		select
			gen_random_uuid() as id,
			$1::uuid as session_id,
			'{}'::jsonb as normalized_payload,
			null::timestamptz as recovery_due_at,
			statement_timestamp() as created_at
		from generate_series(1, 10000)
	`, batchSessionID)
	insertTravelQueryV2ShadowRecoveryMessages(t, ctx, pool, "seed due messages", `
		select
			gen_random_uuid() as id,
			$1::uuid as session_id,
			jsonb_build_object(
				$2::text,
				jsonb_build_object(
					'integration-key',
					jsonb_build_object(
						'status', $3::text,
						'idempotency_key', 'integration-key',
						'lease_expires_at_unix_ms', floor(extract(epoch from (statement_timestamp() - interval '1 minute')) * 1000)
					)
				)
			) as normalized_payload,
			statement_timestamp() - interval '1 minute' as recovery_due_at,
			statement_timestamp() as created_at
		from generate_series(1, 120)
	`, batchSessionID, travelQueryV2ShadowClaimsPayloadKey, travelQueryV2ShadowClaimInProgress)
	if _, err := pool.Exec(ctx, `analyze chat_messages`); err != nil {
		t.Fatalf("analyze verification table: %v", err)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire EXPLAIN connection: %v", err)
	}
	connectionReleased := false
	defer func() {
		if !connectionReleased {
			connection.Release()
		}
	}()
	var baselineEnableSeqscan string
	var explainBackendPID int
	if err := connection.QueryRow(ctx, `show enable_seqscan`).Scan(&baselineEnableSeqscan); err != nil {
		t.Fatalf("read baseline enable_seqscan: %v", err)
	}
	if baselineEnableSeqscan != "on" {
		t.Fatalf("verification connection must start with normal enable_seqscan=on, got %q", baselineEnableSeqscan)
	}
	if err := connection.QueryRow(ctx, `select pg_backend_pid()`).Scan(&explainBackendPID); err != nil {
		t.Fatalf("read EXPLAIN backend pid: %v", err)
	}
	var plan string
	explainQuery := "explain (format json, costs off) " + strings.Replace(travelQueryV2ShadowRecoveryCandidateSQL, "$1", "50", 1)
	err = runTravelQueryV2ShadowLocalSeqscanTransaction(ctx, connection, func(tx pgx.Tx) error {
		var localEnableSeqscan string
		if err := tx.QueryRow(ctx, `show enable_seqscan`).Scan(&localEnableSeqscan); err != nil {
			return fmt.Errorf("read transaction-local enable_seqscan: %w", err)
		}
		if localEnableSeqscan != "off" {
			return fmt.Errorf("transaction-local enable_seqscan = %q, want off", localEnableSeqscan)
		}
		if err := tx.QueryRow(ctx, explainQuery).Scan(&plan); err != nil {
			return fmt.Errorf("scan indexed candidate EXPLAIN: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("EXPLAIN indexed candidate query: %v", err)
	}
	assertTravelQueryV2ShadowRecoveryPlan(t, plan)

	simulatedFailure := errors.New("simulated failure after SET LOCAL")
	err = runTravelQueryV2ShadowLocalSeqscanTransaction(ctx, connection, func(tx pgx.Tx) error {
		var localEnableSeqscan string
		if err := tx.QueryRow(ctx, `show enable_seqscan`).Scan(&localEnableSeqscan); err != nil {
			return fmt.Errorf("read transaction-local enable_seqscan before simulated failure: %w", err)
		}
		if localEnableSeqscan != "off" {
			return fmt.Errorf("transaction-local enable_seqscan before simulated failure = %q, want off", localEnableSeqscan)
		}
		return simulatedFailure
	})
	if !errors.Is(err, simulatedFailure) {
		t.Fatalf("simulated transaction failure must be returned after rollback: %v", err)
	}
	var enableSeqscanAfterRollback string
	if err := connection.QueryRow(ctx, `show enable_seqscan`).Scan(&enableSeqscanAfterRollback); err != nil {
		t.Fatalf("read enable_seqscan after rollback: %v", err)
	}
	if enableSeqscanAfterRollback != baselineEnableSeqscan {
		t.Fatalf("SET LOCAL leaked on the acquired connection: before=%q after=%q", baselineEnableSeqscan, enableSeqscanAfterRollback)
	}
	connection.Release()
	connectionReleased = true

	reacquiredConnection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("reacquire EXPLAIN connection: %v", err)
	}
	reacquiredConnectionReleased := false
	defer func() {
		if !reacquiredConnectionReleased {
			reacquiredConnection.Release()
		}
	}()
	var reacquiredBackendPID int
	var enableSeqscanAfterReacquire string
	if err := reacquiredConnection.QueryRow(ctx, `select pg_backend_pid()`).Scan(&reacquiredBackendPID); err != nil {
		t.Fatalf("read reacquired backend pid: %v", err)
	}
	if reacquiredBackendPID != explainBackendPID {
		t.Fatalf("expected to verify the same pooled connection: before=%d after=%d", explainBackendPID, reacquiredBackendPID)
	}
	if err := reacquiredConnection.QueryRow(ctx, `show enable_seqscan`).Scan(&enableSeqscanAfterReacquire); err != nil {
		t.Fatalf("read enable_seqscan after pool reacquire: %v", err)
	}
	if enableSeqscanAfterReacquire != baselineEnableSeqscan || enableSeqscanAfterReacquire == "off" {
		t.Fatalf("SET LOCAL leaked through the pool: before=%q after=%q", baselineEnableSeqscan, enableSeqscanAfterReacquire)
	}
	var subsequentQueryResult int
	if err := reacquiredConnection.QueryRow(ctx, `select 1`).Scan(&subsequentQueryResult); err != nil {
		t.Fatalf("run query after pool reacquire: %v", err)
	}
	if subsequentQueryResult != 1 {
		t.Fatalf("unexpected query result after pool reacquire: %d", subsequentQueryResult)
	}
	reacquiredConnection.Release()
	reacquiredConnectionReleased = true

	results := make([]TravelQueryV2ShadowRecoveryResult, 2)
	errorsByInstance := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	for instance := range results {
		go func(instance int) {
			defer wait.Done()
			<-start
			results[instance], errorsByInstance[instance] = repository.RecoverExpiredTravelQueryV2ShadowClaims(ctx, 50)
		}(instance)
	}
	close(start)
	wait.Wait()
	processed := results[0].MessagesProcessed + results[1].MessagesProcessed
	completed := results[0].ClaimsCompleted + results[1].ClaimsCompleted
	if errorsByInstance[0] != nil || errorsByInstance[1] != nil || processed != 100 || completed != 100 {
		t.Fatalf("two replicas must process distinct bounded batches: results=%v errors=%v", results, errorsByInstance)
	}
	third, err := repository.RecoverExpiredTravelQueryV2ShadowClaims(ctx, 50)
	if err != nil || third.MessagesProcessed != 20 || third.ClaimsCompleted != 20 {
		t.Fatalf("third sweep must continue remaining indexed rows: result=%+v err=%v", third, err)
	}
	fourth, err := repository.RecoverExpiredTravelQueryV2ShadowClaims(ctx, 50)
	if err != nil || fourth.MessagesProcessed != 0 {
		t.Fatalf("terminal rows must leave partial index: result=%+v err=%v", fourth, err)
	}
	mixedMessageID := "20000000-0000-0000-0000-000000000002"
	insertTravelQueryV2ShadowRecoveryMessages(t, ctx, pool, "seed mixed-lease message", `
		select
			$1::uuid as id,
			$2::uuid as session_id,
			jsonb_build_object(
				$3::text,
				jsonb_build_object(
					'expired-key', jsonb_build_object(
						'status', $4::text,
						'idempotency_key', 'expired-key',
						'lease_expires_at_unix_ms', floor(extract(epoch from (statement_timestamp() - interval '1 minute')) * 1000)
					),
					'future-key', jsonb_build_object(
						'status', $4::text,
						'idempotency_key', 'future-key',
						'lease_expires_at_unix_ms', floor(extract(epoch from (statement_timestamp() + interval '10 minutes')) * 1000)
					)
				)
			) as normalized_payload,
			statement_timestamp() - interval '1 minute' as recovery_due_at,
			statement_timestamp() as created_at
	`, mixedMessageID, batchSessionID, travelQueryV2ShadowClaimsPayloadKey, travelQueryV2ShadowClaimInProgress)
	mixed, err := repository.RecoverExpiredTravelQueryV2ShadowClaims(ctx, 50)
	if err != nil || mixed.MessagesProcessed != 1 || mixed.ClaimsCompleted != 1 {
		t.Fatalf("mixed message recovery: result=%+v err=%v", mixed, err)
	}
	var mixedStateValid bool
	if err := pool.QueryRow(ctx, `
		select
			normalized_payload -> $2 -> 'expired-key' ->> 'status' = $3
			and normalized_payload -> $2 -> 'future-key' ->> 'status' = $4
			and floor(extract(epoch from travel_query_v2_shadow_recovery_due_at) * 1000)::bigint
				= (normalized_payload -> $2 -> 'future-key' ->> 'lease_expires_at_unix_ms')::bigint
		from chat_messages
		where id = $1::uuid
	`, mixedMessageID, travelQueryV2ShadowClaimsPayloadKey, travelQueryV2ShadowClaimCompleted, travelQueryV2ShadowClaimInProgress).Scan(&mixedStateValid); err != nil || !mixedStateValid {
		t.Fatalf("future claim must remain indexed after mixed recovery: valid=%t err=%v", mixedStateValid, err)
	}
	if err := repository.CompleteTravelQueryV2Shadow(ctx, batchSessionID, mixedMessageID, "future-key", abandonedTravelQueryV2ShadowSummary()); err != nil {
		t.Fatalf("complete future claim: %v", err)
	}
	var terminalClaims int
	var remainingMarkers int
	if err := pool.QueryRow(ctx, `
		select
			count(*) filter (where normalized_payload -> $2 -> 'integration-key' ->> 'status' = $3),
			count(*) filter (where travel_query_v2_shadow_recovery_due_at is not null)
		from chat_messages
		where session_id = $1::uuid
	`, batchSessionID, travelQueryV2ShadowClaimsPayloadKey, travelQueryV2ShadowClaimCompleted).Scan(&terminalClaims, &remainingMarkers); err != nil {
		t.Fatalf("read terminal recovery state: %v", err)
	}
	if terminalClaims != 120 || remainingMarkers != 0 {
		t.Fatalf("unexpected terminal state: completed=%d markers=%d results=%s", terminalClaims, remainingMarkers, fmt.Sprint(results))
	}
}

func runTravelQueryV2ShadowLocalSeqscanTransaction(
	ctx context.Context,
	connection *pgxpool.Conn,
	operation func(pgx.Tx) error,
) error {
	tx, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction for local enable_seqscan: %w", err)
	}
	if _, err := tx.Exec(ctx, `set local enable_seqscan = off`); err != nil {
		return rollbackTravelQueryV2ShadowLocalSeqscanTransaction(connection, tx, fmt.Errorf("set local enable_seqscan: %w", err))
	}
	return rollbackTravelQueryV2ShadowLocalSeqscanTransaction(connection, tx, operation(tx))
}

func rollbackTravelQueryV2ShadowLocalSeqscanTransaction(
	connection *pgxpool.Conn,
	tx pgx.Tx,
	operationErr error,
) error {
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tx.Rollback(rollbackCtx); err != nil {
		rollbackErr := fmt.Errorf("rollback local enable_seqscan transaction: %w", err)
		if closeErr := connection.Conn().Close(rollbackCtx); closeErr != nil {
			return errors.Join(operationErr, rollbackErr, fmt.Errorf("close connection after rollback failure: %w", closeErr))
		}
		return errors.Join(operationErr, rollbackErr)
	}
	return operationErr
}

func assertTravelQueryV2ShadowRecoveryPlan(t *testing.T, rawPlan string) {
	t.Helper()
	type explainPlanNode struct {
		NodeType  string            `json:"Node Type"`
		IndexName string            `json:"Index Name"`
		Plans     []explainPlanNode `json:"Plans"`
	}
	var result []struct {
		Plan explainPlanNode `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(rawPlan), &result); err != nil {
		t.Fatalf("decode candidate EXPLAIN JSON: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("candidate EXPLAIN returned %d roots, want 1: %s", len(result), rawPlan)
	}
	limit := result[0].Plan
	if limit.NodeType != "Limit" || len(limit.Plans) != 1 {
		t.Fatalf("candidate plan root must be Limit with one child: %+v", limit)
	}
	lockRows := limit.Plans[0]
	if lockRows.NodeType != "LockRows" || len(lockRows.Plans) != 1 {
		t.Fatalf("candidate plan second node must be LockRows with one child: %+v", lockRows)
	}
	indexScan := lockRows.Plans[0]
	if indexScan.NodeType != "Index Scan" || indexScan.IndexName != travelQueryV2ShadowRecoveryIndexName {
		t.Fatalf("candidate plan leaf must use recovery index: %+v", indexScan)
	}
}

func seedTravelQueryV2ShadowRecoverySessions(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	lifecycleSessionID string,
	batchSessionID string,
) func() {
	t.Helper()
	const lifecycleContactKey = "test-travel-query-v2-shadow-recovery-lifecycle"
	const batchContactKey = "test-travel-query-v2-shadow-recovery-batch"

	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `
			delete from chat_messages
			where session_id in ($1::uuid, $2::uuid)
		`, lifecycleSessionID, batchSessionID); err != nil {
			return fmt.Errorf("cleanup recovery fixture messages: %w", err)
		}
		if _, err := pool.Exec(cleanupCtx, `
			delete from chat_sessions
			where id in ($1::uuid, $2::uuid)
				or contact_key in ($3::text, $4::text)
		`, lifecycleSessionID, batchSessionID, lifecycleContactKey, batchContactKey); err != nil {
			return fmt.Errorf("cleanup recovery fixture sessions: %w", err)
		}
		return nil
	}
	if err := cleanup(); err != nil {
		t.Fatalf("prepare recovery fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		insert into chat_sessions (
			id,
			channel,
			contact_key,
			status,
			handoff_status,
			metadata,
			created_at,
			updated_at
		)
		values
			($1::uuid, 'WHATSAPP', $3::text, 'ACTIVE', 'BOT', '{}'::jsonb, statement_timestamp(), statement_timestamp()),
			($2::uuid, 'WHATSAPP', $4::text, 'ACTIVE', 'BOT', '{}'::jsonb, statement_timestamp(), statement_timestamp())
	`, lifecycleSessionID, batchSessionID, lifecycleContactKey, batchContactKey); err != nil {
		t.Fatalf("seed recovery fixture sessions: %v", err)
	}
	return func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup recovery fixture: %v", err)
		}
	}
}

func insertTravelQueryV2ShadowRecoveryMessages(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	description string,
	fixtureRowsSQL string,
	args ...any,
) {
	t.Helper()
	query := `
		insert into chat_messages (
			id,
			session_id,
			direction,
			normalized_payload,
			travel_query_v2_shadow_recovery_due_at,
			created_at
		)
		select
			fixture.id,
			fixture.session_id,
			'INBOUND',
			fixture.normalized_payload,
			fixture.recovery_due_at,
			fixture.created_at
		from (
	` + fixtureRowsSQL + `
		) fixture
	`
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("%s: %v", description, err)
	}
}

func readTravelQueryV2ShadowSQLTestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
