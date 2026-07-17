package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"schumacher-tur/api/internal/shared/config"
)

type fakeOpenAITravelQueryV2Interpreter struct {
	enabled   bool
	calls     int
	result    OpenAITravelQueryV2RunResult
	err       error
	lastInput OpenAITravelQueryV2RunInput
}

type fakeTravelQueryV2ShadowClaimStore struct {
	*fakeStore
	mu                          sync.Mutex
	claims                      map[string]travelQueryV2ShadowClaimRecord
	recoveryDueAt               map[string]time.Time
	claimAttempts               chan struct{}
	completed                   chan struct{}
	now                         time.Time
	completionFailuresRemaining int
	completionAlwaysFails       bool
	completionAttempts          int
	completionSummaries         []TravelQueryV2ShadowSummary
	completionDeadlines         []time.Time
	claimFailuresRemaining      int
	claimPanicsRemaining        int
	claimError                  error
	claimDeadlines              []time.Time
	recoverySweeps              chan struct{}
	recoveryFailuresRemaining   int
	recoveryPanicsRemaining     int
	recoveryCalls               int
	recoveryTransitions         int
	recoveryLeaseRepairs        int
	recoveryDeadlines           []time.Time
	recoveryCandidateRowsRead   []int
}

func newFakeTravelQueryV2ShadowClaimStore() *fakeTravelQueryV2ShadowClaimStore {
	return &fakeTravelQueryV2ShadowClaimStore{
		fakeStore:      newFakeStore(),
		claims:         map[string]travelQueryV2ShadowClaimRecord{},
		recoveryDueAt:  map[string]time.Time{},
		claimAttempts:  make(chan struct{}, 32),
		completed:      make(chan struct{}, 32),
		recoverySweeps: make(chan struct{}, 64),
		now:            time.Date(2030, 7, 15, 12, 0, 0, 0, time.UTC),
	}
}

func (store *fakeTravelQueryV2ShadowClaimStore) ClaimTravelQueryV2Shadow(ctx context.Context, sessionID string, messageID string, idempotencyKey string, leaseDuration time.Duration) (TravelQueryV2ShadowClaimResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if deadline, ok := ctx.Deadline(); ok {
		store.claimDeadlines = append(store.claimDeadlines, deadline)
	}
	select {
	case store.claimAttempts <- struct{}{}:
	default:
	}
	if store.claimPanicsRemaining > 0 {
		store.claimPanicsRemaining--
		panic("sensitive claim panic detail")
	}
	if store.claimFailuresRemaining > 0 {
		store.claimFailuresRemaining--
		if store.claimError != nil {
			return TravelQueryV2ShadowClaimResult{}, store.claimError
		}
		return TravelQueryV2ShadowClaimResult{}, errors.New("sensitive claim storage detail")
	}
	key := sessionID + "|" + messageID + "|" + idempotencyKey
	if record, exists := store.claims[key]; exists {
		switch record.Status {
		case travelQueryV2ShadowClaimInProgress:
			if record.LeaseExpiresAtUnixMilli > 0 && store.now.UnixMilli() >= record.LeaseExpiresAtUnixMilli {
				summary := abandonedTravelQueryV2ShadowSummary()
				record.Status = travelQueryV2ShadowClaimCompleted
				record.CompletedAt = store.now.Format(time.RFC3339Nano)
				record.Summary = &summary
				store.persistTravelQueryV2ShadowClaimLocked(sessionID, messageID, idempotencyKey, record)
				select {
				case store.completed <- struct{}{}:
				default:
				}
				return TravelQueryV2ShadowClaimResult{Status: TravelQueryV2ShadowClaimCompleted, Summary: summary}, nil
			}
			return TravelQueryV2ShadowClaimResult{Status: TravelQueryV2ShadowClaimInProgress}, nil
		case travelQueryV2ShadowClaimCompleted:
			result := TravelQueryV2ShadowClaimResult{Status: TravelQueryV2ShadowClaimCompleted}
			if record.Summary != nil {
				result.Summary = *record.Summary
			}
			return result, nil
		}
	}
	if leaseDuration <= 0 {
		leaseDuration = travelQueryV2ShadowLeaseDuration(travelQueryV2ShadowProviderTimeout)
	}
	leaseExpiresAt := store.now.Add(leaseDuration)
	record := travelQueryV2ShadowClaimRecord{
		Status:                  travelQueryV2ShadowClaimInProgress,
		IdempotencyKey:          idempotencyKey,
		ClaimedAt:               store.now.Format(time.RFC3339Nano),
		LeaseExpiresAt:          leaseExpiresAt.Format(time.RFC3339Nano),
		LeaseExpiresAtUnixMilli: leaseExpiresAt.UnixMilli(),
	}
	store.persistTravelQueryV2ShadowClaimLocked(sessionID, messageID, idempotencyKey, record)
	return TravelQueryV2ShadowClaimResult{Status: TravelQueryV2ShadowClaimAcquired}, nil
}

func (store *fakeTravelQueryV2ShadowClaimStore) CompleteTravelQueryV2Shadow(ctx context.Context, sessionID string, messageID string, idempotencyKey string, summary TravelQueryV2ShadowSummary) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.completionAttempts++
	store.completionSummaries = append(store.completionSummaries, summary)
	if deadline, ok := ctx.Deadline(); ok {
		store.completionDeadlines = append(store.completionDeadlines, deadline)
	}
	if store.completionFailuresRemaining > 0 {
		store.completionFailuresRemaining--
		return errors.New("forced terminal persistence failure")
	}
	if store.completionAlwaysFails {
		return errors.New("forced terminal persistence failure")
	}
	key := sessionID + "|" + messageID + "|" + idempotencyKey
	record, exists := store.claims[key]
	if !exists {
		return ErrTravelQueryV2ShadowClaimNotFound
	}
	if record.Status == travelQueryV2ShadowClaimCompleted {
		return nil
	}
	record.Status = travelQueryV2ShadowClaimCompleted
	record.CompletedAt = store.now.Format(time.RFC3339Nano)
	record.Summary = &summary
	store.persistTravelQueryV2ShadowClaimLocked(sessionID, messageID, idempotencyKey, record)
	select {
	case store.completed <- struct{}{}:
	default:
	}
	return nil
}

func (store *fakeTravelQueryV2ShadowClaimStore) RecoverExpiredTravelQueryV2ShadowClaims(ctx context.Context, batchSize int) (TravelQueryV2ShadowRecoveryResult, error) {
	store.mu.Lock()
	store.recoveryCalls++
	if deadline, ok := ctx.Deadline(); ok {
		store.recoveryDeadlines = append(store.recoveryDeadlines, deadline)
	}
	select {
	case store.recoverySweeps <- struct{}{}:
	default:
	}
	if store.recoveryPanicsRemaining > 0 {
		store.recoveryPanicsRemaining--
		store.mu.Unlock()
		panic("sensitive recovery panic detail")
	}
	if store.recoveryFailuresRemaining > 0 {
		store.recoveryFailuresRemaining--
		store.mu.Unlock()
		return TravelQueryV2ShadowRecoveryResult{}, errors.New("sensitive recovery storage detail")
	}
	if err := ctx.Err(); err != nil {
		store.mu.Unlock()
		return TravelQueryV2ShadowRecoveryResult{}, err
	}
	if batchSize <= 0 || batchSize > travelQueryV2ShadowRecoveryBatchSize {
		batchSize = travelQueryV2ShadowRecoveryBatchSize
	}

	type fakeRecoveryMessage struct {
		sessionID string
		messageID string
		dueAt     time.Time
	}
	dueMessages := make([]fakeRecoveryMessage, 0, len(store.recoveryDueAt))
	for messageKey, dueAt := range store.recoveryDueAt {
		if dueAt.After(store.now) {
			continue
		}
		parts := strings.SplitN(messageKey, "|", 2)
		if len(parts) != 2 {
			continue
		}
		dueMessages = append(dueMessages, fakeRecoveryMessage{sessionID: parts[0], messageID: parts[1], dueAt: dueAt})
	}
	sort.Slice(dueMessages, func(left int, right int) bool {
		if dueMessages[left].dueAt.Equal(dueMessages[right].dueAt) {
			leftKey := dueMessages[left].sessionID + "|" + dueMessages[left].messageID
			rightKey := dueMessages[right].sessionID + "|" + dueMessages[right].messageID
			return leftKey < rightKey
		}
		return dueMessages[left].dueAt.Before(dueMessages[right].dueAt)
	})
	if len(dueMessages) > batchSize {
		dueMessages = dueMessages[:batchSize]
	}
	store.recoveryCandidateRowsRead = append(store.recoveryCandidateRowsRead, len(dueMessages))
	candidates := dueMessages

	result := TravelQueryV2ShadowRecoveryResult{MessagesProcessed: len(candidates)}
	for _, candidate := range candidates {
		prefix := candidate.sessionID + "|" + candidate.messageID + "|"
		claimKeys := make([]string, 0)
		for key := range store.claims {
			if strings.HasPrefix(key, prefix) {
				claimKeys = append(claimKeys, key)
			}
		}
		sort.Strings(claimKeys)
		for _, key := range claimKeys {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			record := store.claims[key]
			if record.Status != travelQueryV2ShadowClaimInProgress {
				continue
			}
			idempotencyKey := strings.TrimPrefix(key, prefix)
			if record.LeaseExpiresAtUnixMilli <= 0 {
				leaseExpiresAt := store.now.Add(travelQueryV2ShadowLegacyLeaseRepairGrace)
				record.ClaimedAt = store.now.Format(time.RFC3339Nano)
				record.LeaseExpiresAt = leaseExpiresAt.Format(time.RFC3339Nano)
				record.LeaseExpiresAtUnixMilli = leaseExpiresAt.UnixMilli()
				if record.IdempotencyKey == "" {
					record.IdempotencyKey = idempotencyKey
				}
				store.persistTravelQueryV2ShadowClaimLocked(candidate.sessionID, candidate.messageID, idempotencyKey, record)
				result.ClaimsLeaseRepaired++
				store.recoveryLeaseRepairs++
				continue
			}
			if store.now.UnixMilli() < record.LeaseExpiresAtUnixMilli {
				continue
			}
			summary := abandonedTravelQueryV2ShadowSummary()
			record.Status = travelQueryV2ShadowClaimCompleted
			record.CompletedAt = store.now.Format(time.RFC3339Nano)
			record.Summary = &summary
			if record.IdempotencyKey == "" {
				record.IdempotencyKey = idempotencyKey
			}
			store.persistTravelQueryV2ShadowClaimLocked(candidate.sessionID, candidate.messageID, idempotencyKey, record)
			result.ClaimsCompleted++
			store.recoveryTransitions++
			select {
			case store.completed <- struct{}{}:
			default:
			}
		}
	}
	store.mu.Unlock()
	return result, nil
}

func (store *fakeTravelQueryV2ShadowClaimStore) persistTravelQueryV2ShadowClaimLocked(sessionID string, messageID string, idempotencyKey string, record travelQueryV2ShadowClaimRecord) {
	store.claims[sessionID+"|"+messageID+"|"+idempotencyKey] = record

	message := store.messages[messageID]
	if message.NormalizedPayload == nil {
		message.NormalizedPayload = map[string]interface{}{}
	}
	claims := asMap(message.NormalizedPayload[travelQueryV2ShadowClaimsPayloadKey])
	if claims == nil {
		claims = map[string]interface{}{}
	}
	persisted := map[string]interface{}{
		"status":          record.Status,
		"idempotency_key": record.IdempotencyKey,
	}
	if record.ClaimedAt != "" {
		persisted["claimed_at"] = record.ClaimedAt
	}
	if record.LeaseExpiresAt != "" {
		persisted["lease_expires_at"] = record.LeaseExpiresAt
		persisted["lease_expires_at_unix_ms"] = record.LeaseExpiresAtUnixMilli
	}
	if record.CompletedAt != "" {
		persisted["completed_at"] = record.CompletedAt
	}
	if record.Summary != nil {
		persisted["summary"] = *record.Summary
	}
	claims[idempotencyKey] = persisted
	message.NormalizedPayload[travelQueryV2ShadowClaimsPayloadKey] = claims
	store.messages[messageID] = message
	store.recalculateTravelQueryV2ShadowRecoveryDueAtLocked(sessionID, messageID)
}

func (store *fakeTravelQueryV2ShadowClaimStore) recalculateTravelQueryV2ShadowRecoveryDueAtLocked(sessionID string, messageID string) {
	messageKey := sessionID + "|" + messageID
	prefix := messageKey + "|"
	var earliest time.Time
	for key, record := range store.claims {
		if !strings.HasPrefix(key, prefix) || record.Status != travelQueryV2ShadowClaimInProgress {
			continue
		}
		dueAt := store.now
		if record.LeaseExpiresAtUnixMilli > 0 {
			dueAt = time.UnixMilli(record.LeaseExpiresAtUnixMilli).UTC()
		}
		if earliest.IsZero() || dueAt.Before(earliest) {
			earliest = dueAt
		}
	}
	if earliest.IsZero() {
		delete(store.recoveryDueAt, messageKey)
		return
	}
	store.recoveryDueAt[messageKey] = earliest
}

func (store *fakeTravelQueryV2ShadowClaimStore) completedClaim(sessionID string, messageID string, idempotencyKey string) (travelQueryV2ShadowClaimRecord, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.claims[sessionID+"|"+messageID+"|"+idempotencyKey]
	return record, ok
}

func (store *fakeTravelQueryV2ShadowClaimStore) persistedClaimStatus(messageID string, idempotencyKey string) string {
	store.mu.Lock()
	defer store.mu.Unlock()
	message := store.messages[messageID]
	claims := asMap(message.NormalizedPayload[travelQueryV2ShadowClaimsPayloadKey])
	return strings.TrimSpace(asString(asMap(claims[idempotencyKey])["status"]))
}

func (store *fakeTravelQueryV2ShadowClaimStore) recoveryMarker(sessionID string, messageID string) (time.Time, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	dueAt, ok := store.recoveryDueAt[sessionID+"|"+messageID]
	return dueAt, ok
}

func seedFakeTravelQueryV2ShadowClaim(store *fakeTravelQueryV2ShadowClaimStore, sessionID string, messageID string, idempotencyKey string, record travelQueryV2ShadowClaimRecord) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if record.LeaseExpiresAtUnixMilli > 0 {
		leaseExpiresAt := time.UnixMilli(record.LeaseExpiresAtUnixMilli).UTC()
		if record.LeaseExpiresAt == "" {
			record.LeaseExpiresAt = leaseExpiresAt.Format(time.RFC3339Nano)
		}
		if record.ClaimedAt == "" {
			record.ClaimedAt = leaseExpiresAt.Add(-travelQueryV2ShadowLeaseDuration(travelQueryV2ShadowProviderTimeout)).Format(time.RFC3339Nano)
		}
	}
	store.persistTravelQueryV2ShadowClaimLocked(sessionID, messageID, idempotencyKey, record)
}

type threadSafeTravelQueryV2ShadowLogger struct {
	mu      sync.Mutex
	entries []string
}

func (logger *threadSafeTravelQueryV2ShadowLogger) Printf(format string, args ...interface{}) {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	logger.entries = append(logger.entries, fmt.Sprintf(format, args...))
}

func (logger *threadSafeTravelQueryV2ShadowLogger) contains(fragment string) bool {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	for _, entry := range logger.entries {
		if strings.Contains(entry, fragment) {
			return true
		}
	}
	return false
}

func (logger *threadSafeTravelQueryV2ShadowLogger) count(fragment string) int {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	count := 0
	for _, entry := range logger.entries {
		if strings.Contains(entry, fragment) {
			count++
		}
	}
	return count
}

func (logger *threadSafeTravelQueryV2ShadowLogger) snapshot() []string {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	return append([]string(nil), logger.entries...)
}

func waitTravelQueryV2ShadowBackgroundSlotsEmpty(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(travelQueryV2ShadowBackgroundSlots) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("travel V2 shadow background slots were not released: used=%d capacity=%d", len(travelQueryV2ShadowBackgroundSlots), cap(travelQueryV2ShadowBackgroundSlots))
}

func travelQueryV2ShadowTestBackgroundJob(provider OpenAITravelQueryV2Interpreter) travelQueryV2ShadowBackgroundJob {
	return travelQueryV2ShadowBackgroundJob{
		SessionID:      "scheduler-session",
		MessageID:      "scheduler-message",
		IdempotencyKey: "scheduler-key",
		Input: TravelQueryV2ShadowInput{
			Enabled:                  true,
			OpenAIInterpreter:        provider,
			StructuredInput:          StructuredInterpreterInput{CurrentTurn: "quero escolher a poltrona", ObservedAt: time.Now().UTC()},
			LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
			ExistingDecisionStrength: DecisionStrengthFallback,
			IdempotencyKey:           "scheduler-key",
		},
	}
}

func TestTravelQueryV2ShadowSchedulerLogsEveryClosedReason(t *testing.T) {
	waitTravelQueryV2ShadowBackgroundSlotsEmpty(t)
	logger := &threadSafeTravelQueryV2ShadowLogger{}
	provider := &fakeOpenAITravelQueryV2Interpreter{enabled: true, result: OpenAITravelQueryV2RunResult{
		Proposal: travelQueryV2BaseProposal(TravelQueryIntentSeatRequest), ProposalParseable: true, SchemaValid: true,
	}}
	job := travelQueryV2ShadowTestBackgroundJob(provider)

	disabled := NewService(newFakeTravelQueryV2ShadowClaimStore(), config.Config{}, logger, provider)
	if disabled.scheduleTravelQueryV2Shadow(job) {
		t.Fatal("disabled scheduler must reject the job")
	}

	emptyKey := NewService(newFakeTravelQueryV2ShadowClaimStore(), config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger, provider)
	emptyJob := job
	emptyJob.IdempotencyKey = ""
	if emptyKey.scheduleTravelQueryV2Shadow(emptyJob) {
		t.Fatal("scheduler must reject an empty idempotency key")
	}

	incompatible := NewService(newFakeStore(), config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger, provider)
	if incompatible.scheduleTravelQueryV2Shadow(job) {
		t.Fatal("scheduler must reject an incompatible store")
	}

	heldSlots := 0
	defer func() {
		for heldSlots > 0 {
			<-travelQueryV2ShadowBackgroundSlots
			heldSlots--
		}
	}()
	for heldSlots < cap(travelQueryV2ShadowBackgroundSlots) {
		select {
		case travelQueryV2ShadowBackgroundSlots <- struct{}{}:
			heldSlots++
		default:
			t.Fatalf("could not reserve scheduler capacity for the capacity_full regression: held=%d", heldSlots)
		}
	}
	capacityStore := newFakeTravelQueryV2ShadowClaimStore()
	capacityService := NewService(capacityStore, config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger, provider)
	if capacityService.scheduleTravelQueryV2Shadow(job) {
		t.Fatal("scheduler must reject a job when all slots are occupied")
	}
	for heldSlots > 0 {
		<-travelQueryV2ShadowBackgroundSlots
		heldSlots--
	}

	scheduledStore := newFakeTravelQueryV2ShadowClaimStore()
	scheduled := NewService(scheduledStore, config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger, provider)
	if !scheduled.scheduleTravelQueryV2Shadow(job) {
		t.Fatal("compatible enabled scheduler must accept the job")
	}
	waitTravelQueryV2ShadowSignal(t, scheduledStore.completed, "scheduled terminal claim")
	waitTravelQueryV2ShadowBackgroundSlotsEmpty(t)

	for _, reason := range []string{"disabled", "empty_idempotency_key", "incompatible_store", "capacity_full", "scheduled"} {
		if !logger.contains("travel_v2_shadow_scheduler event=" + reason + " reason=" + reason) {
			t.Fatalf("missing scheduler reason %q in logs: %v", reason, logger.snapshot())
		}
	}
}

func TestTravelQueryV2ShadowSchedulerReleasesSlotOnClaimErrorAndPanic(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeTravelQueryV2ShadowClaimStore)
		wantClass string
	}{
		{name: "claim error", configure: func(store *fakeTravelQueryV2ShadowClaimStore) { store.claimFailuresRemaining = 1 }, wantClass: "storage"},
		{name: "claim timeout", configure: func(store *fakeTravelQueryV2ShadowClaimStore) {
			store.claimFailuresRemaining = 1
			store.claimError = context.DeadlineExceeded
		}, wantClass: "timeout"},
		{name: "claim panic", configure: func(store *fakeTravelQueryV2ShadowClaimStore) { store.claimPanicsRemaining = 1 }, wantClass: "panic"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			waitTravelQueryV2ShadowBackgroundSlotsEmpty(t)
			store := newFakeTravelQueryV2ShadowClaimStore()
			test.configure(store)
			logger := &threadSafeTravelQueryV2ShadowLogger{}
			provider := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
			service := NewService(store, config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger, provider)
			if !service.scheduleTravelQueryV2Shadow(travelQueryV2ShadowTestBackgroundJob(provider)) {
				t.Fatal("scheduler unexpectedly rejected claim failure regression job")
			}
			waitTravelQueryV2ShadowSignal(t, store.claimAttempts, "claim attempt before "+test.name)
			waitTravelQueryV2ShadowBackgroundSlotsEmpty(t)
			if provider.calls != 0 {
				t.Fatalf("provider must not run after %s, calls=%d", test.name, provider.calls)
			}
			store.mu.Lock()
			claimDeadlines := append([]time.Time(nil), store.claimDeadlines...)
			store.mu.Unlock()
			if len(claimDeadlines) != 1 {
				t.Fatalf("claim must receive one bounded context, deadlines=%v", claimDeadlines)
			}
			claimBudget := time.Until(claimDeadlines[0])
			if claimBudget <= 0 || claimBudget > travelQueryV2ShadowStoreTimeout {
				t.Fatalf("claim context budget=%s, want within (0,%s]", claimBudget, travelQueryV2ShadowStoreTimeout)
			}
			if !logger.contains("reason=claim_failed operation=claim error_class="+test.wantClass) || logger.contains("sensitive") {
				t.Fatalf("claim failure must be sanitized and classified, logs=%v", logger.snapshot())
			}
		})
	}
}

func TestTravelQueryV2ShadowStorageFailureLogIsSanitized(t *testing.T) {
	logger := &threadSafeTravelQueryV2ShadowLogger{}
	service := NewService(newFakeTravelQueryV2ShadowClaimStore(), config.Config{}, logger)
	postgresError := &pgconn.PgError{Code: "57014", Message: "sensitive SQL and payload detail"}
	service.logTravelQueryV2ShadowStorageFailure("job", "claim_failed", travelQueryV2ShadowOperationClaim, fmt.Errorf("wrapped: %w", postgresError))
	entries := logger.snapshot()
	if len(entries) != 1 || !strings.Contains(entries[0], "operation=claim error_class=postgres sqlstate=57014 timeout=false canceled=false") {
		t.Fatalf("unexpected PostgreSQL metadata log: %v", entries)
	}
	if strings.Contains(strings.Join(entries, "\n"), "sensitive") {
		t.Fatalf("storage log leaked error detail: %v", entries)
	}
}

func TestTravelQueryV2ShadowRecoveryZeroCandidatesIsSuccessfulSweep(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	logger := &threadSafeTravelQueryV2ShadowLogger{}
	service := NewService(store, config.Config{}, logger)
	service.runTravelQueryV2ShadowRecoverySweep(context.Background(), store, travelQueryV2ShadowRecoveryLoopConfig{
		BatchSize:    10,
		SweepTimeout: time.Second,
	})
	entries := logger.snapshot()
	if !logger.contains("travel_v2_shadow_recovery event=sweep_started reason=sweep_started") ||
		!logger.contains("travel_v2_shadow_recovery event=sweep_done reason=sweep_done messages=0 claims_completed=0 claims_lease_repaired=0") ||
		logger.contains("reason=sweep_failed") {
		t.Fatalf("zero candidates must be logged as a successful sweep: %v", entries)
	}
}

type blockingOpenAITravelQueryV2Interpreter struct {
	mu             sync.Mutex
	calls          int
	lastInput      OpenAITravelQueryV2RunInput
	result         OpenAITravelQueryV2RunResult
	err            error
	started        chan struct{}
	release        chan struct{}
	waitForContext bool
	panicOnCall    bool
	startOnce      sync.Once
}

func (fake *blockingOpenAITravelQueryV2Interpreter) Enabled() bool { return true }

func (fake *blockingOpenAITravelQueryV2Interpreter) InterpretTravelQueryV2(ctx context.Context, input OpenAITravelQueryV2RunInput) (OpenAITravelQueryV2RunResult, error) {
	fake.mu.Lock()
	fake.calls++
	fake.lastInput = input
	fake.mu.Unlock()
	if fake.started != nil {
		fake.startOnce.Do(func() { close(fake.started) })
	}
	if fake.panicOnCall {
		panic("provider panic with sensitive detail")
	}
	if fake.waitForContext {
		<-ctx.Done()
		return fake.result, ctx.Err()
	}
	if fake.release != nil {
		select {
		case <-fake.release:
		case <-ctx.Done():
			return fake.result, ctx.Err()
		}
	}
	return fake.result, fake.err
}

func (fake *blockingOpenAITravelQueryV2Interpreter) callCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.calls
}

func (fake *blockingOpenAITravelQueryV2Interpreter) input() OpenAITravelQueryV2RunInput {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.lastInput
}

func (fake *fakeOpenAITravelQueryV2Interpreter) Enabled() bool {
	return fake.enabled
}

func (fake *fakeOpenAITravelQueryV2Interpreter) InterpretTravelQueryV2(_ context.Context, input OpenAITravelQueryV2RunInput) (OpenAITravelQueryV2RunResult, error) {
	fake.calls++
	fake.lastInput = input
	return fake.result, fake.err
}

func TestOpenAITravelQueryV2SchemaIsStrictAndContainsOnlySemanticContract(t *testing.T) {
	format := openAITravelQueryMeaningV2JSONSchema()
	if format["type"] != "json_schema" || format["name"] != "travel_query_meaning_v2" || format["strict"] != true {
		t.Fatalf("unexpected V2 response format: %+v", format)
	}
	schema, ok := format["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema object, got %T", format["schema"])
	}
	assertOpenAITravelQueryV2StrictObjects(t, schema, "root")
	assertOpenAITravelQueryV2SchemaHasNoOperationalKeys(t, schema, "root")

	properties := schema["properties"].(map[string]interface{})
	for _, field := range []string{
		"intent", "turn_meaning", "origin", "destination", "mentioned_locations",
		"date_preference", "option_reference", "route_coverage", "seat_request",
		"institutional_topic", "needs_clarification", "missing_fields", "confidence", "reasons",
	} {
		if _, exists := properties[field]; !exists {
			t.Fatalf("schema missing TravelQueryMeaningV2 field %q", field)
		}
	}
	missingFields := properties["missing_fields"].(map[string]interface{})
	items := missingFields["items"].(map[string]interface{})
	gotMissingFieldEnum, _ := items["enum"].([]string)
	wantMissingFieldEnum := openAITravelQueryMissingFieldV2Enum()
	if strings.Join(gotMissingFieldEnum, ",") != strings.Join(wantMissingFieldEnum, ",") {
		t.Fatalf("unexpected missing_fields enum: got=%v want=%v", gotMissingFieldEnum, wantMissingFieldEnum)
	}
}

func assertOpenAITravelQueryV2StrictObjects(t *testing.T, node interface{}, path string) {
	t.Helper()
	switch typed := node.(type) {
	case map[string]interface{}:
		if typed["type"] == "object" {
			if typed["additionalProperties"] != false {
				t.Fatalf("object %s must set additionalProperties=false: %+v", path, typed)
			}
			properties, _ := typed["properties"].(map[string]interface{})
			required, _ := typed["required"].([]string)
			if len(required) != len(properties) {
				t.Fatalf("object %s must require every property: required=%v properties=%v", path, required, properties)
			}
		}
		for key, value := range typed {
			assertOpenAITravelQueryV2StrictObjects(t, value, path+"."+key)
		}
	case []interface{}:
		for index, value := range typed {
			assertOpenAITravelQueryV2StrictObjects(t, value, fmt.Sprintf("%s[%d]", path, index))
		}
	}
}

func assertOpenAITravelQueryV2SchemaHasNoOperationalKeys(t *testing.T, node interface{}, path string) {
	t.Helper()
	forbidden := map[string]struct{}{
		"action": {}, "tool": {}, "tools": {}, "template": {}, "template_name": {},
		"trip_id": {}, "route_id": {}, "stop_id": {}, "booking_id": {}, "payment_id": {},
	}
	switch typed := node.(type) {
	case map[string]interface{}:
		for key, value := range typed {
			if _, blocked := forbidden[key]; blocked {
				t.Fatalf("schema contains operational key %s.%s", path, key)
			}
			assertOpenAITravelQueryV2SchemaHasNoOperationalKeys(t, value, path+"."+key)
		}
	case []interface{}:
		for index, value := range typed {
			assertOpenAITravelQueryV2SchemaHasNoOperationalKeys(t, value, fmt.Sprintf("%s[%d]", path, index))
		}
	}
}

func TestOpenAITravelQueryV2PromptUsesMinimalStructuredContextAndRedactsSensitiveData(t *testing.T) {
	observedAt := time.Date(2030, 7, 14, 12, 0, 0, 0, time.UTC)
	input := OpenAITravelQueryV2RunInput{
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "opção 2 no dia 15",
			ObservedAt:  observedAt,
			State: CanonicalConversationState{
				Phase: ConversationPhaseTripSelection,
				Route: CanonicalRouteState{Origin: "Seara/SC", Destination: "Videira/SC"},
			},
		},
		ActivePrompt: ActivePromptContext{
			Kind:                    ActivePromptAvailabilityOptionChoice,
			Phase:                   ConversationPhaseTripSelection,
			SourceMessageID:         "must-not-be-sent",
			SourceMessageBody:       "must-not-be-sent either",
			AvailabilityOptionCount: 2,
			HasAvailabilityList:     true,
		},
		AvailabilityFacts:        travelQueryV2TestAvailabilityFacts(observedAt, 2),
		LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
		ExistingDecisionStrength: DecisionStrengthFallback,
	}

	raw := buildOpenAITravelQueryV2CompactInput(input)
	var compact OpenAITravelQueryV2CompactInput
	if err := json.Unmarshal([]byte(raw), &compact); err != nil {
		t.Fatalf("decode compact input: %v", err)
	}
	if compact.CurrentTurn != input.StructuredInput.CurrentTurn || compact.ObservedDate != "2030-07-14" {
		t.Fatalf("unexpected current turn/date: %+v", compact)
	}
	if len(compact.AvailabilityFacts) != 2 || compact.AvailabilityFacts[1].Index != 2 {
		t.Fatalf("expected two indexed current facts, got %+v", compact.AvailabilityFacts)
	}
	if len(compact.LocationCatalog) != 0 {
		t.Fatalf("catalog must be omitted when current availability facts suffice: %+v", compact.LocationCatalog)
	}
	if strings.Contains(raw, "must-not-be-sent") || strings.Contains(raw, "source_message") || strings.Contains(raw, "trip_id") || strings.Contains(raw, "stop_id") {
		t.Fatalf("compact input leaked prompt body or operational IDs: %s", raw)
	}

	sensitive := input
	sensitive.StructuredInput.CurrentTurn = "Meu CPF é 529.982.247-25 e meu telefone é (48) 99999-9999"
	sensitive.AvailabilityFacts = TravelQueryAvailabilityFactsV2{}
	rawSensitive := buildOpenAITravelQueryV2CompactInput(sensitive)
	if strings.Contains(rawSensitive, "529.982.247-25") || strings.Contains(rawSensitive, "99999-9999") {
		t.Fatalf("compact input leaked sensitive data: %s", rawSensitive)
	}
	if !strings.Contains(rawSensitive, "DADO_SENSIVEL_REDACTED") {
		t.Fatalf("expected redaction marker, got %s", rawSensitive)
	}
	if !strings.Contains(rawSensitive, "Fraiburgo/SC") || !strings.Contains(rawSensitive, "Santa Ines/MA") {
		t.Fatalf("expected minimal canonical catalog when facts are absent: %s", rawSensitive)
	}

	prompt := buildOpenAITravelQueryV2SystemPrompt()
	for _, required := range []string{
		"Opcao 1 ou 2", "Opcao 3 permanece INDEX=3", "daqui a 2 dias", "Santa Cecilia/SE",
		"DEICTIC", "EARLIEST_AVAILABLE", "poltrona especifica", "ACKNOWLEDGEMENT",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing semantic instruction %q: %s", required, prompt)
		}
	}
}

func TestTravelQueryV2ShadowFactsUseOnlyCurrentReliableVisibleList(t *testing.T) {
	observedAt := time.Date(2030, 7, 14, 12, 0, 0, 0, time.UTC)
	visibleResult := AvailabilitySearchResult{Results: travelQueryV2TestAvailabilityFacts(observedAt, 2).VisibleOptions}
	invisibleResult := AvailabilitySearchResult{Results: travelQueryV2TestAvailabilityFacts(observedAt, 3).VisibleOptions}
	visibleBody := buildAvailabilityListReply(visibleResult)
	history := []Message{
		{
			ID:               "visible-sent-list",
			Direction:        "OUTBOUND",
			Body:             visibleBody,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       observedAt.Add(-2 * time.Minute),
			Payload: map[string]interface{}{"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(visibleResult),
			}},
		},
		{
			ID:               "newer-invisible-draft",
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(invisibleResult),
			ProcessingStatus: messageStatusAutomationDraft,
			ReceivedAt:       observedAt.Add(-time.Minute),
			Payload: map[string]interface{}{"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(invisibleResult),
			}},
		},
	}
	state := CanonicalConversationState{Phase: ConversationPhaseTripSelection}
	activePrompt := InferActivePromptContext(history, state)
	facts := buildTravelQueryV2ShadowAvailabilityFacts(StructuredInterpreterInput{
		ObservedAt: observedAt,
		History:    history,
		State:      state,
	}, activePrompt)

	if facts.SourceMessageID != "visible-sent-list" || len(facts.VisibleOptions) != 2 {
		t.Fatalf("expected only current reliable visible facts, active=%+v facts=%+v", activePrompt, facts)
	}
	if facts.VisibleOptions[1].TripID != "trip-v2-2" {
		t.Fatalf("invisible draft shifted visible option facts: %+v", facts.VisibleOptions)
	}
}

func TestTravelQueryV2ShadowCatalogPreservesCanonicalIdentityByKnownStopID(t *testing.T) {
	facts := TravelQueryAvailabilityFactsV2{VisibleOptions: []AvailabilitySearchItem{
		{OriginDisplayName: "Seara", OriginStopID: "SC_SEARA", DestinationDisplayName: "Videira", DestinationStopID: "SC_VIDEIRA"},
		{OriginDisplayName: "Nome inventado", OriginStopID: "STOP_DESCONHECIDO"},
	}}
	catalog := buildTravelQueryV2ShadowLocationCatalog(facts)
	searaCount := 0
	unknownCount := 0
	for _, evidence := range catalog {
		if evidence.StopID == "SC_SEARA" {
			searaCount++
			if evidence.CanonicalName != "Seara/SC" {
				t.Fatalf("known StopID must preserve catalog identity, got %+v", evidence)
			}
		}
		if evidence.StopID == "STOP_DESCONHECIDO" || evidence.CanonicalName == "Nome inventado" {
			unknownCount++
		}
	}
	if searaCount != 1 {
		t.Fatalf("display without UF must not duplicate SC_SEARA, catalog=%+v", catalog)
	}
	if unknownCount != 0 {
		t.Fatalf("unknown StopID/display must not create catalog identity, catalog=%+v", catalog)
	}
}

func TestOpenAITravelQueryV2RunnerUsesSingleResponsesCallWithoutToolsOrStorage(t *testing.T) {
	requestCount := 0
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(openAITravelQueryV2TestResponse("resp_travel_v2", validOpenAITravelQueryV2OutputText(t)))
	}))
	defer server.Close()

	runner := NewOpenAITravelQueryV2Runner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL + "/v1",
	})
	result, err := runner.InterpretTravelQueryV2(context.Background(), OpenAITravelQueryV2RunInput{
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "de Fraiburgo para Santa Inês",
			ObservedAt:  time.Date(2030, 7, 14, 0, 0, 0, 0, time.UTC),
		},
		LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
		ExistingDecisionStrength: DecisionStrengthFallback,
		IdempotencyKey:           "travel-v2-idempotency",
	})
	if err != nil {
		t.Fatalf("run Travel V2: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected exactly one V2 call, got %d", requestCount)
	}
	if result.Proposal.Origin == nil || result.Proposal.Origin.Name != "Fraiburgo/SC" ||
		result.Proposal.Destination == nil || result.Proposal.Destination.Name != "Santa Ines/MA" {
		t.Fatalf("unexpected mapped proposal: %+v", result.Proposal)
	}
	if result.ProviderResponseID != "resp_travel_v2" || !result.SchemaValid || !result.ProposalParseable {
		t.Fatalf("unexpected runner metadata: %+v", result)
	}
	if captured["store"] != false || captured["tool_choice"] != "none" {
		t.Fatalf("request must disable storage and tool choice: %+v", captured)
	}
	tools, ok := captured["tools"].([]interface{})
	if !ok || len(tools) != 0 {
		t.Fatalf("request must use tools=[]: %#v", captured["tools"])
	}
	textConfig := captured["text"].(map[string]interface{})
	format := textConfig["format"].(map[string]interface{})
	if format["type"] != "json_schema" || format["strict"] != true {
		t.Fatalf("request must use strict structured output: %+v", format)
	}
}

func TestOpenAITravelQueryV2RunnerReturnsSafeErrors(t *testing.T) {
	tests := []struct {
		name          string
		response      func(*testing.T) map[string]interface{}
		wantError     error
		wantParseable bool
	}{
		{
			name: "schema invalid but parseable",
			response: func(t *testing.T) map[string]interface{} {
				payload := validOpenAITravelQueryV2PayloadMap()
				delete(payload, "reasons")
				return openAITravelQueryV2TestResponse("resp_invalid", string(mustMarshalOpenAITravelQueryV2Test(t, payload)))
			},
			wantError:     ErrOpenAITravelQueryV2InvalidOutput,
			wantParseable: true,
		},
		{
			name: "missing fields enum is case sensitive",
			response: func(t *testing.T) map[string]interface{} {
				payload := validOpenAITravelQueryV2PayloadMap()
				payload["missing_fields"] = []interface{}{"ORIGIN"}
				payload["needs_clarification"] = true
				return openAITravelQueryV2TestResponse("resp_invalid_enum_case", string(mustMarshalOpenAITravelQueryV2Test(t, payload)))
			},
			wantError:     ErrOpenAITravelQueryV2InvalidOutput,
			wantParseable: true,
		},
		{
			name: "refusal",
			response: func(_ *testing.T) map[string]interface{} {
				return map[string]interface{}{
					"id": "resp_refusal",
					"output": []interface{}{map[string]interface{}{"content": []interface{}{map[string]interface{}{
						"type": "refusal", "refusal": "sensitive provider text must not escape",
					}}}},
				}
			},
			wantError: ErrOpenAITravelQueryV2Refusal,
		},
		{
			name: "missing result",
			response: func(_ *testing.T) map[string]interface{} {
				return map[string]interface{}{"id": "resp_empty", "output": []interface{}{}}
			},
			wantError: ErrOpenAITravelQueryV2EmptyOutput,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(writer).Encode(test.response(t))
			}))
			defer server.Close()
			runner := NewOpenAITravelQueryV2Runner(config.Config{OpenAIAPIKey: "sk-test", OpenAIModel: "gpt-test", OpenAIBaseURL: server.URL})
			result, err := runner.InterpretTravelQueryV2(context.Background(), OpenAITravelQueryV2RunInput{})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("expected %v, got %v", test.wantError, err)
			}
			if result.ProposalParseable != test.wantParseable {
				t.Fatalf("expected parseable=%t, got %+v", test.wantParseable, result)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"error":{"message":"provider internal detail"}}`))
	}))
	defer server.Close()
	runner := NewOpenAITravelQueryV2Runner(config.Config{OpenAIAPIKey: "sk-test", OpenAIModel: "gpt-test", OpenAIBaseURL: server.URL})
	if _, err := runner.InterpretTravelQueryV2(context.Background(), OpenAITravelQueryV2RunInput{}); !errors.Is(err, ErrOpenAITravelQueryV2RequestFailed) {
		t.Fatalf("expected request failure, got %v", err)
	}
	if got := sanitizeTravelQueryV2ShadowMissingFields([]string{"ORIGIN"}); len(got) != 1 || got[0] != "__redacted_invalid" {
		t.Fatalf("shadow summary must not lowercase invalid missing field, got %v", got)
	}
}

func TestOpenAITravelQueryV2MissingFieldsValidationIsExactAndCaseSensitive(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "origin exact", value: "origin", valid: true},
		{name: "origin exact after trim", value: "  origin  ", valid: true},
		{name: "origin uppercase", value: "ORIGIN"},
		{name: "origin title case", value: "Origin"},
		{name: "unknown value", value: "booking_id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := validOpenAITravelQueryV2PayloadMap()
			payload["origin"] = nil
			payload["needs_clarification"] = true
			payload["missing_fields"] = []interface{}{test.value}

			validation := validateOpenAITravelQueryMeaningV2Payload(mustMarshalOpenAITravelQueryV2Test(t, payload))
			if validation.Valid != test.valid || !validation.ProposalParseable {
				t.Fatalf("unexpected exact missing_fields validation: value=%q validation=%+v", test.value, validation)
			}
		})
	}
}

func TestTravelQueryV2ShadowSemanticCasesPreserveProviderMeaning(t *testing.T) {
	observedAt := time.Date(2030, 7, 14, 12, 0, 0, 0, time.UTC)
	base := func(currentTurn string, proposal TravelQueryMeaningV2) (TravelQueryV2ShadowInput, *fakeOpenAITravelQueryV2Interpreter) {
		fake := &fakeOpenAITravelQueryV2Interpreter{
			enabled: true,
			result: OpenAITravelQueryV2RunResult{
				Proposal:          proposal,
				ProposalParseable: true,
				SchemaValid:       true,
			},
		}
		return TravelQueryV2ShadowInput{
			Enabled:           true,
			OpenAIInterpreter: fake,
			StructuredInput: StructuredInterpreterInput{
				CurrentTurn: currentTurn,
				ObservedAt:  observedAt,
				State:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
			},
			LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
			ExistingDecisionStrength: DecisionStrengthFallback,
		}, fake
	}
	assertSingleCall := func(t *testing.T, fake *fakeOpenAITravelQueryV2Interpreter, summary TravelQueryV2ShadowSummary) {
		t.Helper()
		if fake.calls != 1 || summary.OpenAI.Status != string(TravelQueryV2ShadowValid) {
			t.Fatalf("expected one valid shadow call, calls=%d summary=%+v", fake.calls, summary)
		}
	}

	t.Run("de Fraiburgo para Santa Ines", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
		proposal.Origin = &LocationMeaning{Name: "Fraiburgo/SC", Role: LocationRoleOrigin}
		proposal.Destination = &LocationMeaning{Name: "Santa Ines/MA", Role: LocationRoleDestination}
		input, fake := base("de Fraiburgo para Santa Inês", proposal)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.Origin == nil || summary.OpenAI.Origin.Role != string(LocationRoleOrigin) ||
			summary.OpenAI.Destination == nil || summary.OpenAI.Destination.Role != string(LocationRoleDestination) {
			t.Fatalf("expected preserved route roles, got %+v", summary.OpenAI)
		}
	})

	t.Run("opcao 1 ou 2 clarifies without selection", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentSelectAvailabilityOption)
		proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
		proposal.NeedsClarification = true
		proposal.MissingFields = []string{"option_reference"}
		input, fake := base("opção 1 ou 2", proposal)
		travelQueryV2AttachSelectionContext(&input, observedAt, 2)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if !summary.OpenAI.NeedsClarification || summary.OpenAI.OptionReference.Kind != string(OptionReferenceNone) || summary.OpenAI.OptionReference.Index != 0 {
			t.Fatalf("expected clarification without selection, got %+v", summary.OpenAI)
		}
	})

	t.Run("opcao 3 remains index before range rejection", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentSelectAvailabilityOption)
		proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 3}
		input, fake := base("opção 3", proposal)
		travelQueryV2AttachSelectionContext(&input, observedAt, 2)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.OptionReference.Index != 3 || summary.Validation.Accepted || !containsString(summary.Validation.ReasonCodes, string(TravelQueryValidationReasonOptionIndexOutOfRange)) {
			t.Fatalf("expected INDEX=3 plus factual range rejection, got %+v", summary)
		}
	})

	t.Run("relative date is temporal not index", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
		proposal.DatePreference = DatePreference{Mode: DateModeExact, ExactDate: "2030-07-16"}
		input, fake := base("daqui a 2 dias", proposal)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.OptionReference.Kind != string(OptionReferenceNone) || summary.OpenAI.DatePreference.ExactDate != "2030-07-16" {
			t.Fatalf("expected temporal meaning without INDEX, got %+v", summary.OpenAI)
		}
	})

	t.Run("option and date stay separate", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentSelectAvailabilityOption)
		proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 2}
		proposal.DatePreference = DatePreference{Mode: DateModeExact, ExactDate: "2030-07-15"}
		input, fake := base("opção 2 no dia 15", proposal)
		travelQueryV2AttachSelectionContext(&input, observedAt, 2)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.OptionReference.Index != 2 || summary.OpenAI.DatePreference.ExactDate != "2030-07-15" {
			t.Fatalf("expected separate index and date, got %+v", summary.OpenAI)
		}
	})

	t.Run("conjunction se does not invent UF", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
		proposal.Destination = &LocationMeaning{Name: "Santa Cecilia", Role: LocationRoleDestination}
		input, fake := base("Santa Cecilia se tiver vaga", proposal)
		input.LocationCatalog = append(input.LocationCatalog,
			TravelQueryLocationEvidenceV2{CanonicalName: "Santa Cecilia/SC", StopID: "SC_SANTA_CECILIA"},
			TravelQueryLocationEvidenceV2{CanonicalName: "Santa Cecilia/SE", StopID: "SE_SANTA_CECILIA"},
		)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.Destination == nil || summary.OpenAI.Destination.Name != travelQueryV2ShadowUnallowlistedLocationMarker {
			t.Fatalf("ambiguous city without canonical identity must be redacted, got %+v", summary.OpenAI.Destination)
		}
	})

	t.Run("explicit SE is preserved", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
		proposal.Destination = &LocationMeaning{Name: "Santa Cecilia/SE", Role: LocationRoleDestination}
		input, fake := base("Santa Cecilia/SE", proposal)
		input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{CanonicalName: "Santa Cecilia/SE", StopID: "SE_SANTA_CECILIA"})
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.Destination == nil || summary.OpenAI.Destination.Name != "Santa Cecilia/SE" {
			t.Fatalf("expected explicit UF SE, got %+v", summary.OpenAI.Destination)
		}
	})

	for _, optionCount := range []int{1, 3} {
		t.Run(fmt.Sprintf("deictic essa with %d options", optionCount), func(t *testing.T) {
			proposal := travelQueryV2BaseProposal(TravelQueryIntentSelectAvailabilityOption)
			proposal.OptionReference = OptionReference{Kind: OptionReferenceDeictic}
			if optionCount > 1 {
				proposal.NeedsClarification = true
				proposal.MissingFields = []string{"option_reference"}
			}
			input, fake := base("essa", proposal)
			travelQueryV2AttachSelectionContext(&input, observedAt, optionCount)
			summary := RunTravelQueryV2Shadow(context.Background(), input)
			assertSingleCall(t, fake, summary)
			if summary.OpenAI.OptionReference.Kind != string(OptionReferenceDeictic) || summary.OpenAI.NeedsClarification != (optionCount > 1) {
				t.Fatalf("unexpected deictic meaning: %+v", summary.OpenAI)
			}
		})
	}

	t.Run("earliest available", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
		proposal.Origin = &LocationMeaning{Name: "Fraiburgo/SC", Role: LocationRoleOrigin}
		proposal.Destination = &LocationMeaning{Name: "Santa Ines/MA", Role: LocationRoleDestination}
		proposal.DatePreference = DatePreference{Mode: DateModeEarliestAvailable}
		input, fake := base("a primeira que tiver vaga", proposal)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.DatePreference.Mode != string(DateModeEarliestAvailable) {
			t.Fatalf("expected EARLIEST_AVAILABLE, got %+v", summary.OpenAI.DatePreference)
		}
	})

	t.Run("route coverage", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentRouteCoverage)
		proposal.MentionedLocations = []LocationMeaning{{Name: "Seara/SC", Role: LocationRoleVia}}
		proposal.RouteCoverage = RouteCoverageMeaning{QueryLocation: "Seara/SC", Mode: RouteCoverageModeExactStop}
		input, fake := base("passa em Seara?", proposal)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.RouteCoverage.Mode != string(RouteCoverageModeExactStop) {
			t.Fatalf("expected coverage meaning, got %+v", summary.OpenAI.RouteCoverage)
		}
	})

	t.Run("specific seat", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentSeatRequest)
		proposal.SeatRequest = SeatRequestChooseSpecificSeat
		input, fake := base("quero a poltrona 12", proposal)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.SeatRequest != string(SeatRequestChooseSpecificSeat) {
			t.Fatalf("expected specific seat request, got %+v", summary.OpenAI)
		}
	})

	t.Run("institutional", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentInstitutionalQuestion)
		proposal.InstitutionalTopic = InstitutionalTopicCompanyInfo
		input, fake := base("de que cidade é a empresa?", proposal)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.InstitutionalTopic != string(InstitutionalTopicCompanyInfo) {
			t.Fatalf("expected institutional topic, got %+v", summary.OpenAI)
		}
	})

	t.Run("acknowledgement", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentUnknown)
		proposal.TurnMeaning = TurnMeaningAcknowledgement
		input, fake := base("entendi, obrigado", proposal)
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.OpenAI.TurnMeaning != string(TurnMeaningAcknowledgement) {
			t.Fatalf("expected acknowledgement, got %+v", summary.OpenAI)
		}
	})

	t.Run("strong real decision stays protected", func(t *testing.T) {
		proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
		proposal.Origin = &LocationMeaning{Name: "Fraiburgo/SC", Role: LocationRoleOrigin}
		proposal.Destination = &LocationMeaning{Name: "Santa Ines/MA", Role: LocationRoleDestination}
		input, fake := base("de Fraiburgo para Santa Inês", proposal)
		input.ExistingDecisionStrength = DecisionStrengthStrong
		summary := RunTravelQueryV2Shadow(context.Background(), input)
		assertSingleCall(t, fake, summary)
		if summary.Validation.Accepted || !containsString(summary.Validation.ReasonCodes, string(TravelQueryValidationReasonStrongDecisionProtected)) {
			t.Fatalf("strong runtime decision must remain protected, got %+v", summary.Validation)
		}
	})
}

func TestTravelQueryV2ShadowSafeStatusesAndSanitizedPersistence(t *testing.T) {
	baseInput := TravelQueryV2ShadowInput{
		Enabled:                  true,
		StructuredInput:          StructuredInterpreterInput{CurrentTurn: "oi", ObservedAt: time.Now().UTC()},
		ExistingDecisionStrength: DecisionStrengthFallback,
	}
	proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
	proposal.Destination = &LocationMeaning{Name: "telefone (48) 99999-9999", Role: LocationRoleDestination}
	proposal.SeatRequest = SeatRequestMode("CPF 529.982.247-25")
	proposal.Reasons = []string{"CPF 529.982.247-25"}

	tests := []struct {
		name           string
		result         OpenAITravelQueryV2RunResult
		err            error
		wantStatus     TravelQueryV2ShadowStatus
		wantErrorCode  string
		wantValidation TravelQueryV2ShadowValidationStatus
	}{
		{
			name:           "schema invalid parseable still runs factual validator",
			result:         OpenAITravelQueryV2RunResult{Proposal: proposal, ProposalParseable: true},
			err:            ErrOpenAITravelQueryV2InvalidOutput,
			wantStatus:     TravelQueryV2ShadowError,
			wantErrorCode:  "openai_travel_query_v2_invalid_output",
			wantValidation: TravelQueryV2ShadowValidationRejected,
		},
		{name: "refusal", err: ErrOpenAITravelQueryV2Refusal, wantStatus: TravelQueryV2ShadowRefused, wantErrorCode: "openai_travel_query_v2_refusal", wantValidation: TravelQueryV2ShadowValidationSkipped},
		{name: "provider error", err: ErrOpenAITravelQueryV2RequestFailed, wantStatus: TravelQueryV2ShadowError, wantErrorCode: "openai_travel_query_v2_request_failed", wantValidation: TravelQueryV2ShadowValidationSkipped},
		{name: "timeout", err: context.DeadlineExceeded, wantStatus: TravelQueryV2ShadowError, wantErrorCode: "context_deadline_exceeded", wantValidation: TravelQueryV2ShadowValidationSkipped},
		{name: "missing result", err: ErrOpenAITravelQueryV2EmptyOutput, wantStatus: TravelQueryV2ShadowError, wantErrorCode: "openai_travel_query_v2_empty_output", wantValidation: TravelQueryV2ShadowValidationSkipped},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeOpenAITravelQueryV2Interpreter{enabled: true, result: test.result, err: test.err}
			input := baseInput
			input.OpenAIInterpreter = fake
			summary := RunTravelQueryV2Shadow(context.Background(), input)
			if fake.calls != 1 || summary.OpenAI.Status != string(test.wantStatus) || summary.OpenAI.ErrorCode != test.wantErrorCode || summary.Validation.Status != string(test.wantValidation) {
				t.Fatalf("unexpected safe status: calls=%d summary=%+v", fake.calls, summary)
			}
			serialized, err := json.Marshal(summary)
			if err != nil {
				t.Fatalf("marshal summary: %v", err)
			}
			body := string(serialized)
			for _, forbidden := range []string{"99999-9999", "529.982.247-25", "sensitive provider text", "provider internal detail"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("sanitized summary leaked %q: %s", forbidden, body)
				}
			}
		})
	}
}

func TestTravelQueryV2ShadowSchemaInvalidParseableProposalIsNeverEffectivelyAccepted(t *testing.T) {
	proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
	proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
	proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
	fake := &fakeOpenAITravelQueryV2Interpreter{
		enabled: true,
		result: OpenAITravelQueryV2RunResult{
			Proposal:          proposal,
			ProposalParseable: true,
			SchemaValid:       false,
		},
		err: ErrOpenAITravelQueryV2InvalidOutput,
	}

	summary := RunTravelQueryV2Shadow(context.Background(), TravelQueryV2ShadowInput{
		Enabled:                  true,
		OpenAIInterpreter:        fake,
		StructuredInput:          StructuredInterpreterInput{CurrentTurn: "de Seara para Videira", ObservedAt: time.Now().UTC()},
		LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
		ExistingDecisionStrength: DecisionStrengthFallback,
	})

	if summary.OpenAI.Status != string(TravelQueryV2ShadowError) || summary.OpenAI.ErrorCode != "openai_travel_query_v2_invalid_output" {
		t.Fatalf("expected schema-invalid OpenAI status, got %+v", summary.OpenAI)
	}
	if summary.Validation.Accepted || summary.Validation.Status != string(TravelQueryV2ShadowValidationRejected) ||
		!containsString(summary.Validation.ReasonCodes, travelQueryV2ShadowSchemaInvalidReason) {
		t.Fatalf("schema-invalid proposal must never be effectively accepted, got %+v", summary.Validation)
	}
}

func TestTravelQueryV2ShadowPersistsOnlyExactCanonicalLocationIdentities(t *testing.T) {
	proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
	proposal.Origin = &LocationMeaning{Name: "Fraiburgo/SC", Role: LocationRoleOrigin}
	proposal.Destination = &LocationMeaning{Name: "João da Silva", Role: LocationRoleDestination}
	proposal.MentionedLocations = []LocationMeaning{
		{Name: "joao@example.com", Role: LocationRoleVia},
		{Name: "Rua das Flores, 123", Role: LocationRoleNearbyReference},
		{Name: "texto arbitrário do provider", Role: LocationRoleUnknown},
	}
	proposal.RouteCoverage = RouteCoverageMeaning{QueryLocation: "endereço residencial completo", Mode: RouteCoverageModeExactStop}
	proposal.Reasons = []string{"João da Silva mora na Rua das Flores, 123"}
	fake := &fakeOpenAITravelQueryV2Interpreter{enabled: true, result: OpenAITravelQueryV2RunResult{
		Proposal: proposal, ProposalParseable: true, SchemaValid: true,
	}}
	summary := RunTravelQueryV2Shadow(context.Background(), TravelQueryV2ShadowInput{
		Enabled:                  true,
		OpenAIInterpreter:        fake,
		StructuredInput:          StructuredInterpreterInput{CurrentTurn: "consulta", ObservedAt: time.Now().UTC()},
		LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
		ExistingDecisionStrength: DecisionStrengthStrong,
	})
	if summary.Validation.Accepted || !containsString(summary.Validation.ReasonCodes, string(TravelQueryValidationReasonStrongDecisionProtected)) {
		t.Fatalf("expected rejected proposal under strong guardrail, got %+v", summary.Validation)
	}
	if summary.OpenAI.Origin == nil || summary.OpenAI.Origin.Name != "Fraiburgo/SC" {
		t.Fatalf("exact allowlisted identity must be preserved, got %+v", summary.OpenAI.Origin)
	}
	if summary.OpenAI.Destination == nil || summary.OpenAI.Destination.Name != travelQueryV2ShadowUnallowlistedLocationMarker {
		t.Fatalf("free provider destination must be redacted, got %+v", summary.OpenAI.Destination)
	}
	if summary.OpenAI.RouteCoverage.QueryLocation != travelQueryV2ShadowUnallowlistedLocationMarker {
		t.Fatalf("free provider query location must be redacted, got %+v", summary.OpenAI.RouteCoverage)
	}
	for _, location := range summary.OpenAI.MentionedLocations {
		if location.Name != travelQueryV2ShadowUnallowlistedLocationMarker {
			t.Fatalf("free provider location must be redacted, got %+v", summary.OpenAI.MentionedLocations)
		}
	}
	serialized, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, forbidden := range []string{"João da Silva", "joao@example.com", "Rua das Flores", "texto arbitrário", "endereço residencial"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("shadow summary leaked provider text %q: %s", forbidden, serialized)
		}
	}
}

func TestTravelQueryV2ShadowDurableClaimAllowsOnlyOneProviderCall(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	logger := &threadSafeTravelQueryV2ShadowLogger{}
	proposal := travelQueryV2BaseProposal(TravelQueryIntentSeatRequest)
	proposal.SeatRequest = SeatRequestChooseSpecificSeat
	provider := &blockingOpenAITravelQueryV2Interpreter{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result:  OpenAITravelQueryV2RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true},
	}
	svc := NewService(store, config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger)
	job := travelQueryV2ShadowBackgroundJob{
		SessionID:      "session-one-call",
		MessageID:      "message-one-call",
		IdempotencyKey: "travel-v2-one-call",
		Input: TravelQueryV2ShadowInput{
			Enabled:                  true,
			OpenAIInterpreter:        provider,
			StructuredInput:          StructuredInterpreterInput{CurrentTurn: "quero a poltrona 12", ObservedAt: time.Now().UTC()},
			LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
			ExistingDecisionStrength: DecisionStrengthFallback,
			IdempotencyKey:           "travel-v2-one-call",
		},
	}
	firstDone := make(chan struct{})
	go func() {
		svc.executeTravelQueryV2ShadowJob(job)
		close(firstDone)
	}()
	waitTravelQueryV2ShadowSignal(t, provider.started, "first provider call")

	// A concurrent execution observes the durable IN_PROGRESS claim and exits.
	inProgress := svc.executeTravelQueryV2ShadowJob(job)
	if provider.callCount() != 1 {
		t.Fatalf("concurrent execution must not duplicate provider call, got %d", provider.callCount())
	}
	if inProgress.ClaimStatus != TravelQueryV2ShadowClaimInProgress || inProgress.Summary.OpenAI.Status != string(TravelQueryV2ShadowInProgress) {
		t.Fatalf("IN_PROGRESS must be ignored with safe status, got %+v", inProgress)
	}
	if got := store.persistedClaimStatus(job.MessageID, job.IdempotencyKey); got != travelQueryV2ShadowClaimInProgress {
		t.Fatalf("expected durable IN_PROGRESS ledger status, got %q", got)
	}
	close(provider.release)
	waitTravelQueryV2ShadowSignal(t, firstDone, "first background execution")
	waitTravelQueryV2ShadowSignal(t, store.completed, "terminal completed claim")

	// A later execution reuses COMPLETED and also does not call the provider.
	reused := svc.executeTravelQueryV2ShadowJob(job)
	if provider.callCount() != 1 {
		t.Fatalf("COMPLETED claim must be reused without provider call, got %d", provider.callCount())
	}
	claim, ok := store.completedClaim(job.SessionID, job.MessageID, job.IdempotencyKey)
	if !ok || claim.Status != travelQueryV2ShadowClaimCompleted || claim.Summary == nil {
		t.Fatalf("expected durable terminal summary, got %+v exists=%t", claim, ok)
	}
	if got := store.persistedClaimStatus(job.MessageID, job.IdempotencyKey); got != travelQueryV2ShadowClaimCompleted {
		t.Fatalf("expected durable COMPLETED ledger status, got %q", got)
	}
	if reused.ClaimStatus != TravelQueryV2ShadowClaimCompleted || reused.Summary.OpenAI.Status != claim.Summary.OpenAI.Status {
		t.Fatalf("COMPLETED result must be reused, reused=%+v claim=%+v", reused, claim)
	}
	for _, reason := range []string{"started", "claim_acquired", "claim_in_progress", "claim_completed_reused", "provider_started", "provider_completed", "completion_completed"} {
		if !logger.contains("travel_v2_shadow_job event=" + reason + " reason=" + reason) {
			t.Fatalf("missing job reason %q in logs: %v", reason, logger.snapshot())
		}
	}
}

func TestTravelQueryV2ShadowFailuresBecomeTerminalClaims(t *testing.T) {
	tests := []struct {
		name          string
		provider      func() *blockingOpenAITravelQueryV2Interpreter
		timeout       time.Duration
		wantStatus    TravelQueryV2ShadowStatus
		wantErrorCode string
	}{
		{
			name: "refusal",
			provider: func() *blockingOpenAITravelQueryV2Interpreter {
				return &blockingOpenAITravelQueryV2Interpreter{err: ErrOpenAITravelQueryV2Refusal}
			},
			wantStatus:    TravelQueryV2ShadowRefused,
			wantErrorCode: "openai_travel_query_v2_refusal",
		},
		{
			name: "provider error",
			provider: func() *blockingOpenAITravelQueryV2Interpreter {
				return &blockingOpenAITravelQueryV2Interpreter{err: ErrOpenAITravelQueryV2RequestFailed}
			},
			wantStatus:    TravelQueryV2ShadowError,
			wantErrorCode: "openai_travel_query_v2_request_failed",
		},
		{
			name: "timeout",
			provider: func() *blockingOpenAITravelQueryV2Interpreter {
				return &blockingOpenAITravelQueryV2Interpreter{waitForContext: true}
			},
			timeout:       20 * time.Millisecond,
			wantStatus:    TravelQueryV2ShadowError,
			wantErrorCode: "context_deadline_exceeded",
		},
		{
			name: "panic",
			provider: func() *blockingOpenAITravelQueryV2Interpreter {
				return &blockingOpenAITravelQueryV2Interpreter{panicOnCall: true}
			},
			wantStatus:    TravelQueryV2ShadowError,
			wantErrorCode: "openai_travel_query_v2_panic",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeTravelQueryV2ShadowClaimStore()
			store.completionFailuresRemaining = 2
			provider := test.provider()
			svc := NewService(store, config.Config{ChatOpenAITravelV2ShadowEnabled: true})
			job := travelQueryV2ShadowBackgroundJob{
				SessionID:      "session-terminal-" + test.name,
				MessageID:      "message-terminal-" + test.name,
				IdempotencyKey: "travel-v2-terminal-" + test.name,
				Timeout:        test.timeout,
				Input: TravelQueryV2ShadowInput{
					Enabled:                  true,
					OpenAIInterpreter:        provider,
					StructuredInput:          StructuredInterpreterInput{CurrentTurn: "oi", ObservedAt: time.Now().UTC()},
					LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
					ExistingDecisionStrength: DecisionStrengthFallback,
				},
			}
			execution := svc.executeTravelQueryV2ShadowJob(job)
			if execution.CompletionError != nil {
				t.Fatalf("third terminal persistence attempt must succeed: %v", execution.CompletionError)
			}
			claim, ok := store.completedClaim(job.SessionID, job.MessageID, job.IdempotencyKey)
			if !ok || claim.Status != travelQueryV2ShadowClaimCompleted || claim.Summary == nil {
				t.Fatalf("failure must persist terminal COMPLETED claim, got %+v exists=%t", claim, ok)
			}
			if claim.Summary.OpenAI.Status != string(test.wantStatus) || claim.Summary.OpenAI.ErrorCode != test.wantErrorCode {
				t.Fatalf("unexpected terminal summary: %+v", claim.Summary)
			}
			store.mu.Lock()
			completionAttempts := store.completionAttempts
			completionSummaries := append([]TravelQueryV2ShadowSummary(nil), store.completionSummaries...)
			completionDeadlines := append([]time.Time(nil), store.completionDeadlines...)
			store.mu.Unlock()
			if completionAttempts != travelQueryV2ShadowCompletionMaxAttempts ||
				len(completionSummaries) != travelQueryV2ShadowCompletionMaxAttempts ||
				len(completionDeadlines) != travelQueryV2ShadowCompletionMaxAttempts {
				t.Fatalf("expected three independent completion attempts, attempts=%d summaries=%d deadlines=%d", completionAttempts, len(completionSummaries), len(completionDeadlines))
			}
			for attempt := 1; attempt < len(completionDeadlines); attempt++ {
				if !completionDeadlines[attempt].After(completionDeadlines[attempt-1]) {
					t.Fatalf("completion attempt %d did not receive a fresh context deadline: %v", attempt+1, completionDeadlines)
				}
			}
			wantSummary, err := json.Marshal(*claim.Summary)
			if err != nil {
				t.Fatalf("marshal terminal summary: %v", err)
			}
			for attempt, completionSummary := range completionSummaries {
				gotSummary, marshalErr := json.Marshal(completionSummary)
				if marshalErr != nil || string(gotSummary) != string(wantSummary) {
					t.Fatalf("completion attempt %d changed terminal summary: got=%s want=%s err=%v", attempt+1, gotSummary, wantSummary, marshalErr)
				}
			}
			svc.executeTravelQueryV2ShadowJob(job)
			if provider.callCount() != 1 {
				t.Fatalf("terminal failure must not retry provider, calls=%d", provider.callCount())
			}
		})
	}
}

func TestTravelQueryV2ShadowExhaustedCompletionRetriesAreRecoveredBySweeperWithoutReprocess(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	store.completionAlwaysFails = true
	logger := &threadSafeTravelQueryV2ShadowLogger{}
	provider := &blockingOpenAITravelQueryV2Interpreter{err: ErrOpenAITravelQueryV2RequestFailed}
	svc := NewService(store, config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger)
	job := travelQueryV2ShadowBackgroundJob{
		SessionID:      "session-stale-claim",
		MessageID:      "message-stale-claim",
		IdempotencyKey: "travel-v2-stale-claim",
		Timeout:        40 * time.Millisecond,
		Input: TravelQueryV2ShadowInput{
			Enabled:                  true,
			OpenAIInterpreter:        provider,
			StructuredInput:          StructuredInterpreterInput{CurrentTurn: "oi", ObservedAt: time.Now().UTC()},
			LocationCatalog:          buildTravelQueryV2ShadowLocationCatalog(TravelQueryAvailabilityFactsV2{}),
			ExistingDecisionStrength: DecisionStrengthFallback,
		},
	}

	first := svc.executeTravelQueryV2ShadowJob(job)
	if first.CompletionError == nil {
		t.Fatal("expected all bounded terminal persistence attempts to fail")
	}
	if !logger.contains("travel_v2_shadow_job event=completion_failed reason=completion_failed operation=completion") || logger.contains("sensitive") {
		t.Fatalf("completion failure must be sanitized and classified, logs=%v", logger.snapshot())
	}
	if provider.callCount() != 1 {
		t.Fatalf("expected one provider call before completion failure, got %d", provider.callCount())
	}
	record, ok := store.completedClaim(job.SessionID, job.MessageID, job.IdempotencyKey)
	if !ok || record.Status != travelQueryV2ShadowClaimInProgress || record.Summary != nil {
		t.Fatalf("exhausted completion retries must leave a temporary IN_PROGRESS lease, record=%+v exists=%t", record, ok)
	}
	claimedAt, claimedErr := time.Parse(time.RFC3339Nano, record.ClaimedAt)
	leaseExpiresAt, leaseErr := time.Parse(time.RFC3339Nano, record.LeaseExpiresAt)
	if claimedErr != nil || leaseErr != nil || record.LeaseExpiresAtUnixMilli == 0 || !leaseExpiresAt.After(claimedAt) {
		t.Fatalf("IN_PROGRESS claim must persist a valid timestamp and lease: record=%+v claimed_err=%v lease_err=%v", record, claimedErr, leaseErr)
	}
	minimumLease := job.Timeout + time.Duration(travelQueryV2ShadowCompletionMaxAttempts)*travelQueryV2ShadowStoreTimeout
	if leaseExpiresAt.Sub(claimedAt) <= minimumLease {
		t.Fatalf("lease must exceed provider timeout and completion retry contexts: lease=%s minimum=%s", leaseExpiresAt.Sub(claimedAt), minimumLease)
	}
	store.mu.Lock()
	if store.completionAttempts != travelQueryV2ShadowCompletionMaxAttempts || len(store.completionSummaries) != travelQueryV2ShadowCompletionMaxAttempts {
		store.mu.Unlock()
		t.Fatalf("unexpected bounded completion attempts")
	}
	firstSummary, err := json.Marshal(store.completionSummaries[0])
	if err != nil {
		store.mu.Unlock()
		t.Fatalf("marshal first completion summary: %v", err)
	}
	for attempt, summary := range store.completionSummaries[1:] {
		gotSummary, marshalErr := json.Marshal(summary)
		if marshalErr != nil || string(gotSummary) != string(firstSummary) {
			store.mu.Unlock()
			t.Fatalf("retry %d did not preserve the same terminal summary: got=%s want=%s err=%v", attempt+2, gotSummary, firstSummary, marshalErr)
		}
	}
	store.mu.Unlock()

	recent := svc.executeTravelQueryV2ShadowJob(job)
	if recent.ClaimStatus != TravelQueryV2ShadowClaimInProgress || provider.callCount() != 1 {
		t.Fatalf("recent IN_PROGRESS claim must be ignored without provider recall: result=%+v calls=%d", recent, provider.callCount())
	}

	store.mu.Lock()
	store.now = time.UnixMilli(record.LeaseExpiresAtUnixMilli + 1).UTC()
	store.mu.Unlock()

	svc.runTravelQueryV2ShadowRecoverySweep(context.Background(), store, travelQueryV2ShadowRecoveryLoopConfig{
		BatchSize:    10,
		SweepTimeout: time.Second,
	})
	if provider.callCount() != 1 {
		t.Fatalf("independent sweeper must never recall provider, calls=%d", provider.callCount())
	}
	recovered, ok := store.completedClaim(job.SessionID, job.MessageID, job.IdempotencyKey)
	if !ok || recovered.Status != travelQueryV2ShadowClaimCompleted || recovered.Summary == nil ||
		recovered.Summary.OpenAI.ErrorCode != "travel_query_v2_shadow_execution_abandoned" {
		t.Fatalf("expected durable abandoned COMPLETED recovery, record=%+v exists=%t", recovered, ok)
	}

	reused := svc.executeTravelQueryV2ShadowJob(job)
	if reused.ClaimStatus != TravelQueryV2ShadowClaimCompleted || reused.Summary.OpenAI.ErrorCode != recovered.Summary.OpenAI.ErrorCode || provider.callCount() != 1 {
		t.Fatalf("COMPLETED recovery summary must be reused without provider recall: result=%+v calls=%d", reused, provider.callCount())
	}
}

func TestTravelQueryV2ShadowRecoverySweepIsBoundedAndRecoversAllExpiredClaimsFromMessage(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	provider := &blockingOpenAITravelQueryV2Interpreter{}
	expiredAt := store.now.Add(-time.Minute)
	recentAt := store.now.Add(time.Minute)
	completedSummary := TravelQueryV2ShadowSummary{
		OpenAI:     TravelQueryV2ShadowOpenAISummary{Status: string(TravelQueryV2ShadowValid)},
		Validation: TravelQueryV2ShadowValidation{Status: string(TravelQueryV2ShadowValidationAccepted), Accepted: true},
	}
	seedFakeTravelQueryV2ShadowClaim(store, "a-session", "a-message", "expired-one", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: "expired-one", LeaseExpiresAtUnixMilli: expiredAt.UnixMilli(),
	})
	seedFakeTravelQueryV2ShadowClaim(store, "a-session", "a-message", "expired-two", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: "expired-two", LeaseExpiresAtUnixMilli: expiredAt.UnixMilli(),
	})
	seedFakeTravelQueryV2ShadowClaim(store, "a-session", "a-message", "recent", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: "recent", LeaseExpiresAtUnixMilli: recentAt.UnixMilli(),
	})
	seedFakeTravelQueryV2ShadowClaim(store, "a-session", "a-message", "completed", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimCompleted, IdempotencyKey: "completed", Summary: &completedSummary,
	})
	seedFakeTravelQueryV2ShadowClaim(store, "z-session", "z-message", "expired-later-batch", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: "expired-later-batch", LeaseExpiresAtUnixMilli: expiredAt.UnixMilli(),
	})

	result, err := store.RecoverExpiredTravelQueryV2ShadowClaims(context.Background(), 1)
	if err != nil {
		t.Fatalf("recover expired claims: %v", err)
	}
	if result.MessagesProcessed != 1 || result.ClaimsCompleted != 2 || result.ClaimsLeaseRepaired != 0 {
		t.Fatalf("bounded sweep must recover all expired claims from selected message, got %+v", result)
	}
	for _, idempotencyKey := range []string{"expired-one", "expired-two"} {
		record, ok := store.completedClaim("a-session", "a-message", idempotencyKey)
		if !ok || record.Status != travelQueryV2ShadowClaimCompleted || record.Summary == nil ||
			record.Summary.OpenAI.ErrorCode != "travel_query_v2_shadow_execution_abandoned" || record.IdempotencyKey != idempotencyKey {
			t.Fatalf("expired claim %q was not terminalized safely: record=%+v exists=%t", idempotencyKey, record, ok)
		}
	}
	recent, _ := store.completedClaim("a-session", "a-message", "recent")
	if recent.Status != travelQueryV2ShadowClaimInProgress {
		t.Fatalf("recent claim must remain IN_PROGRESS, got %+v", recent)
	}
	completed, _ := store.completedClaim("a-session", "a-message", "completed")
	if completed.Status != travelQueryV2ShadowClaimCompleted || completed.Summary == nil || completed.Summary.OpenAI.Status != completedSummary.OpenAI.Status {
		t.Fatalf("completed claim must remain unchanged, got %+v", completed)
	}
	deferred, _ := store.completedClaim("z-session", "z-message", "expired-later-batch")
	if deferred.Status != travelQueryV2ShadowClaimInProgress {
		t.Fatalf("batch limit must defer another message, got %+v", deferred)
	}
	nextDueAt, hasMarker := store.recoveryMarker("a-session", "a-message")
	if !hasMarker || !nextDueAt.Equal(recentAt) {
		t.Fatalf("future claim must keep marker at its lease: due_at=%s exists=%t want=%s", nextDueAt, hasMarker, recentAt)
	}
	if provider.callCount() != 0 {
		t.Fatalf("recovery must not call provider, calls=%d", provider.callCount())
	}
}

func TestTravelQueryV2ShadowRecoveryMarkerTracksClaimAndCompletionAtomically(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	leaseDuration := 2 * time.Minute
	claim, err := store.ClaimTravelQueryV2Shadow(context.Background(), "marker-session", "marker-message", "marker-key", leaseDuration)
	if err != nil || claim.Status != TravelQueryV2ShadowClaimAcquired {
		t.Fatalf("acquire marked claim: result=%+v err=%v", claim, err)
	}
	dueAt, hasMarker := store.recoveryMarker("marker-session", "marker-message")
	wantDueAt := store.now.Add(leaseDuration)
	if !hasMarker || !dueAt.Equal(wantDueAt) {
		t.Fatalf("claim must atomically set marker to lease: due_at=%s exists=%t want=%s", dueAt, hasMarker, wantDueAt)
	}

	if err := store.CompleteTravelQueryV2Shadow(context.Background(), "marker-session", "marker-message", "marker-key", abandonedTravelQueryV2ShadowSummary()); err != nil {
		t.Fatalf("complete marked claim: %v", err)
	}
	if dueAt, hasMarker := store.recoveryMarker("marker-session", "marker-message"); hasMarker {
		t.Fatalf("last completion must remove message from partial index, due_at=%s", dueAt)
	}
}

func TestTravelQueryV2ShadowRecoveryIndexedBatchesProgressPastOrdinaryMessages(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	provider := &blockingOpenAITravelQueryV2Interpreter{}
	store.mu.Lock()
	for index := 0; index < 10000; index++ {
		messageID := fmt.Sprintf("ordinary-%05d", index)
		store.messages[messageID] = Message{ID: messageID, NormalizedPayload: map[string]interface{}{}}
	}
	store.mu.Unlock()
	expiredAt := store.now.Add(-time.Minute)
	for index := 0; index < 120; index++ {
		messageID := fmt.Sprintf("due-%03d", index)
		idempotencyKey := fmt.Sprintf("claim-%03d", index)
		seedFakeTravelQueryV2ShadowClaim(store, "bounded-session", messageID, idempotencyKey, travelQueryV2ShadowClaimRecord{
			Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: idempotencyKey, LeaseExpiresAtUnixMilli: expiredAt.UnixMilli(),
		})
	}

	wantProcessed := []int{50, 50, 20, 0}
	for sweep, want := range wantProcessed {
		result, err := store.RecoverExpiredTravelQueryV2ShadowClaims(context.Background(), 50)
		if err != nil || result.MessagesProcessed != want || result.ClaimsCompleted != want {
			t.Fatalf("sweep %d did not make bounded progress: result=%+v err=%v want=%d", sweep+1, result, err, want)
		}
	}
	store.mu.Lock()
	rowsRead := append([]int(nil), store.recoveryCandidateRowsRead...)
	remainingMarkers := len(store.recoveryDueAt)
	store.mu.Unlock()
	if fmt.Sprint(rowsRead) != fmt.Sprint(wantProcessed) {
		t.Fatalf("candidate selection must read only bounded indexed rows, got=%v want=%v", rowsRead, wantProcessed)
	}
	if remainingMarkers != 0 {
		t.Fatalf("all terminal messages must leave partial index, markers=%d", remainingMarkers)
	}
	if provider.callCount() != 0 {
		t.Fatalf("indexed recovery must not call provider, calls=%d", provider.callCount())
	}
}

func TestTravelQueryV2ShadowRecoveryConcurrentInstancesTerminalizeOnce(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	provider := &blockingOpenAITravelQueryV2Interpreter{}
	seedFakeTravelQueryV2ShadowClaim(store, "shared-session", "shared-message", "shared-key", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: "shared-key", LeaseExpiresAtUnixMilli: store.now.Add(-time.Minute).UnixMilli(),
	})
	firstService := NewService(store, config.Config{}, provider)
	secondService := NewService(store, config.Config{}, provider)
	loopConfig := travelQueryV2ShadowRecoveryLoopConfig{BatchSize: 10, SweepTimeout: time.Second}
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	for _, service := range []*Service{firstService, secondService} {
		go func(service *Service) {
			defer wait.Done()
			<-start
			service.runTravelQueryV2ShadowRecoverySweep(context.Background(), store, loopConfig)
		}(service)
	}
	close(start)
	wait.Wait()

	store.mu.Lock()
	transitions := store.recoveryTransitions
	store.mu.Unlock()
	if transitions != 1 {
		t.Fatalf("concurrent recovery instances must terminalize once, transitions=%d", transitions)
	}
	record, ok := store.completedClaim("shared-session", "shared-message", "shared-key")
	if !ok || record.Status != travelQueryV2ShadowClaimCompleted || record.Summary == nil {
		t.Fatalf("expected one durable terminal claim, record=%+v exists=%t", record, ok)
	}
	if dueAt, hasMarker := store.recoveryMarker("shared-session", "shared-message"); hasMarker {
		t.Fatalf("terminalized message must leave partial index, due_at=%s", dueAt)
	}
	if provider.callCount() != 0 {
		t.Fatalf("concurrent recovery must not call provider, calls=%d", provider.callCount())
	}
}

func TestTravelQueryV2ShadowRecoveryLoopStartsImmediatelyWithFlagDisabledAndStopsOnCancel(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	provider := &blockingOpenAITravelQueryV2Interpreter{}
	seedFakeTravelQueryV2ShadowClaim(store, "startup-session", "startup-message", "startup-key", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: "startup-key", LeaseExpiresAtUnixMilli: store.now.Add(-time.Minute).UnixMilli(),
	})
	svc := NewService(store, config.Config{ChatOpenAITravelV2ShadowEnabled: false}, provider)
	ctx, cancel := context.WithCancel(context.Background())
	done := svc.startTravelQueryV2ShadowRecoveryLoop(ctx, travelQueryV2ShadowRecoveryLoopConfig{
		BatchSize:    10,
		Interval:     time.Hour,
		SweepTimeout: time.Second,
	})
	waitTravelQueryV2ShadowSignal(t, store.completed, "immediate startup recovery with flag disabled")
	if provider.callCount() != 0 {
		t.Fatalf("startup recovery must not call provider, calls=%d", provider.callCount())
	}
	cancel()
	waitTravelQueryV2ShadowSignal(t, done, "recovery loop cancellation")
	store.mu.Lock()
	recoveryCalls := store.recoveryCalls
	deadlines := len(store.recoveryDeadlines)
	store.mu.Unlock()
	if recoveryCalls != 1 || deadlines != 1 {
		t.Fatalf("expected one immediate bounded sweep before cancellation, calls=%d deadlines=%d", recoveryCalls, deadlines)
	}
}

func TestTravelQueryV2ShadowRecoveryLoopRetriesAfterStorageFailureOrPanic(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeTravelQueryV2ShadowClaimStore)
	}{
		{name: "storage unavailable", configure: func(store *fakeTravelQueryV2ShadowClaimStore) { store.recoveryFailuresRemaining = 1 }},
		{name: "panic", configure: func(store *fakeTravelQueryV2ShadowClaimStore) { store.recoveryPanicsRemaining = 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeTravelQueryV2ShadowClaimStore()
			test.configure(store)
			provider := &blockingOpenAITravelQueryV2Interpreter{}
			logger := &threadSafeTravelQueryV2ShadowLogger{}
			seedFakeTravelQueryV2ShadowClaim(store, "retry-session", "retry-message", "retry-key", travelQueryV2ShadowClaimRecord{
				Status: travelQueryV2ShadowClaimInProgress, IdempotencyKey: "retry-key", LeaseExpiresAtUnixMilli: store.now.Add(-time.Minute).UnixMilli(),
			})
			svc := NewService(store, config.Config{ChatOpenAITravelV2ShadowEnabled: false}, logger, provider)
			ctx, cancel := context.WithCancel(context.Background())
			done := svc.startTravelQueryV2ShadowRecoveryLoop(ctx, travelQueryV2ShadowRecoveryLoopConfig{
				BatchSize:    10,
				Interval:     5 * time.Millisecond,
				SweepTimeout: time.Second,
			})
			waitTravelQueryV2ShadowSignal(t, store.completed, "recovery retry after "+test.name)
			cancel()
			waitTravelQueryV2ShadowSignal(t, done, "recovery loop stop after "+test.name)
			store.mu.Lock()
			recoveryCalls := store.recoveryCalls
			store.mu.Unlock()
			if recoveryCalls < 2 {
				t.Fatalf("failed sweep must retry next cycle, calls=%d", recoveryCalls)
			}
			if !logger.contains("reason=sweep_started") || !logger.contains("reason=sweep_failed") || logger.contains("sensitive") {
				t.Fatalf("recovery errors must be logged with sanitized fixed metadata, entries=%v", logger.snapshot())
			}
			if provider.callCount() != 0 {
				t.Fatalf("recovery retry must not call provider, calls=%d", provider.callCount())
			}
		})
	}
}

func TestTravelQueryV2ShadowRecoveryRepairsLegacyLeaseBeforeTerminalizing(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	seedFakeTravelQueryV2ShadowClaim(store, "legacy-session", "legacy-message", "legacy-key", travelQueryV2ShadowClaimRecord{
		Status: travelQueryV2ShadowClaimInProgress,
	})

	first, err := store.RecoverExpiredTravelQueryV2ShadowClaims(context.Background(), 10)
	if err != nil {
		t.Fatalf("repair legacy claim lease: %v", err)
	}
	if first.ClaimsLeaseRepaired != 1 || first.ClaimsCompleted != 0 {
		t.Fatalf("legacy claim must receive conservative durable grace first, got %+v", first)
	}
	repaired, ok := store.completedClaim("legacy-session", "legacy-message", "legacy-key")
	if !ok || repaired.Status != travelQueryV2ShadowClaimInProgress || repaired.IdempotencyKey != "legacy-key" ||
		repaired.LeaseExpiresAtUnixMilli <= store.now.UnixMilli() {
		t.Fatalf("legacy claim lease was not repaired durably: record=%+v exists=%t", repaired, ok)
	}
	repairedDueAt, hasMarker := store.recoveryMarker("legacy-session", "legacy-message")
	if !hasMarker || repairedDueAt.UnixMilli() != repaired.LeaseExpiresAtUnixMilli {
		t.Fatalf("legacy repair must move marker to grace lease: due_at=%s exists=%t record=%+v", repairedDueAt, hasMarker, repaired)
	}
	second, err := store.RecoverExpiredTravelQueryV2ShadowClaims(context.Background(), 10)
	if err != nil || second.MessagesProcessed != 0 {
		t.Fatalf("repaired recent lease must not be terminalized early: result=%+v err=%v", second, err)
	}
	store.mu.Lock()
	store.now = time.UnixMilli(repaired.LeaseExpiresAtUnixMilli + 1).UTC()
	store.mu.Unlock()
	third, err := store.RecoverExpiredTravelQueryV2ShadowClaims(context.Background(), 10)
	if err != nil || third.ClaimsCompleted != 1 {
		t.Fatalf("legacy claim must terminalize after repaired lease: result=%+v err=%v", third, err)
	}
	terminal, _ := store.completedClaim("legacy-session", "legacy-message", "legacy-key")
	if terminal.Status != travelQueryV2ShadowClaimCompleted || terminal.Summary == nil ||
		terminal.Summary.OpenAI.ErrorCode != "travel_query_v2_shadow_execution_abandoned" {
		t.Fatalf("unexpected legacy terminal summary: %+v", terminal)
	}
	if dueAt, hasMarker := store.recoveryMarker("legacy-session", "legacy-message"); hasMarker {
		t.Fatalf("terminal legacy claim must clear marker, due_at=%s", dueAt)
	}
}

func TestReprocessSystemBufferFlushSchedulesTravelQueryV2ShadowExactlyOnce(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	logger := &threadSafeTravelQueryV2ShadowLogger{}
	agentRunner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resposta real do buffer", Model: "gpt-test"}}
	proposal := travelQueryV2BaseProposal(TravelQueryIntentSeatRequest)
	proposal.SeatRequest = SeatRequestChooseSpecificSeat
	provider := &fakeOpenAITravelQueryV2Interpreter{enabled: true, result: OpenAITravelQueryV2RunResult{
		Proposal: proposal, ProposalParseable: true, SchemaValid: true,
	}}
	service := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatOpenAITravelV2ShadowEnabled: true}, logger, agentRunner, provider)
	ingested, err := service.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511900000310",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-system-buffer-shadow",
			IdempotencyKey:    "idem-system-buffer-shadow",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest system buffer turn: %v", err)
	}
	out, err := service.Reprocess(context.Background(), ReprocessInput{
		SessionID: ingested.Session.ID,
		Trigger:   "SYSTEM_BUFFER_FLUSH",
		Metadata:  map[string]interface{}{"job_run_id": "job-system-buffer-shadow"},
	})
	if err != nil {
		t.Fatalf("reprocess system buffer turn: %v", err)
	}
	waitTravelQueryV2ShadowSignal(t, store.completed, "system buffer terminal shadow claim")
	waitTravelQueryV2ShadowBackgroundSlotsEmpty(t)
	if logger.count("travel_v2_shadow_scheduler event=scheduled reason=scheduled") != 1 {
		t.Fatalf("successful SYSTEM_BUFFER_FLUSH must schedule exactly once: %v", logger.snapshot())
	}
	if provider.calls != 1 {
		t.Fatalf("successful SYSTEM_BUFFER_FLUSH must call provider once, calls=%d", provider.calls)
	}
	claim, ok := store.completedClaim(out.Session.ID, latestCandidateMessageID(out.Messages), provider.lastInput.IdempotencyKey)
	if !ok || claim.Status != travelQueryV2ShadowClaimCompleted || claim.Summary == nil {
		t.Fatalf("system buffer turn must persist a COMPLETED claim: claim=%+v exists=%t", claim, ok)
	}
	if out.Draft == nil || out.Draft.Body != agentRunner.result.ReplyText {
		t.Fatalf("shadow must preserve the real draft: %+v", out.Draft)
	}
	messages, err := store.ListMessages(context.Background(), out.Session.ID, ListMessagesFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list system buffer messages: %v", err)
	}
	outboundCount := 0
	for _, message := range messages {
		if message.Direction == "OUTBOUND" {
			outboundCount++
		}
	}
	if outboundCount != 1 {
		t.Fatalf("shadow must not create a second response, outbound_count=%d messages=%+v", outboundCount, messages)
	}
}

func TestReprocessTravelQueryV2SchedulerObservesDisabledAndEarlyIdempotentReturns(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		store := newFakeTravelQueryV2ShadowClaimStore()
		logger := &threadSafeTravelQueryV2ShadowLogger{}
		agentRunner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resposta sem shadow", Model: "gpt-test"}}
		provider := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
		service := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, logger, agentRunner, provider)
		ingested, err := service.Ingest(context.Background(), IngestMessageInput{
			ContactKey: "5511900000311",
			Message:    IngestMessagePayload{Direction: "INBOUND", ProviderMessageID: "msg-shadow-disabled", IdempotencyKey: "idem-shadow-disabled", Body: "oi"},
		})
		if err != nil {
			t.Fatalf("ingest disabled shadow turn: %v", err)
		}
		if _, err := service.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID}); err != nil {
			t.Fatalf("reprocess disabled shadow turn: %v", err)
		}
		if logger.count("travel_v2_shadow_scheduler event=disabled reason=disabled") != 1 || provider.calls != 0 {
			t.Fatalf("disabled successful Reprocess must log once without provider call: logs=%v calls=%d", logger.snapshot(), provider.calls)
		}
	})

	t.Run("early idempotent draft", func(t *testing.T) {
		store := newFakeTravelQueryV2ShadowClaimStore()
		logger := &threadSafeTravelQueryV2ShadowLogger{}
		provider := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
		service := NewService(store, config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger, provider)
		queued, err := service.QueueAutomationDraft(context.Background(), QueueAutomationDraftInput{
			ContactKey:     "5511900000312",
			Body:           "draft existente",
			IdempotencyKey: "existing-draft-before-shadow-job",
		})
		if err != nil {
			t.Fatalf("queue existing draft: %v", err)
		}
		out, err := service.Reprocess(context.Background(), ReprocessInput{SessionID: queued.Session.ID})
		if err != nil {
			t.Fatalf("reprocess early idempotent draft: %v", err)
		}
		if !out.Idempotent || out.Draft == nil || out.Draft.ID != queued.Message.ID {
			t.Fatalf("expected existing draft idempotent return, got %+v", out)
		}
		if logger.count("travel_v2_shadow_scheduler event=empty_idempotency_key reason=empty_idempotency_key") != 1 || provider.calls != 0 {
			t.Fatalf("early idempotent return must be observable without provider call: logs=%v calls=%d", logger.snapshot(), provider.calls)
		}
		select {
		case <-store.claimAttempts:
			t.Fatal("early idempotent return without a current turn must not attempt a claim")
		default:
		}
	})
}

func TestReprocessTravelQueryV2ErrorDoesNotSchedule(t *testing.T) {
	logger := &threadSafeTravelQueryV2ShadowLogger{}
	provider := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	service := NewService(newFakeTravelQueryV2ShadowClaimStore(), config.Config{ChatOpenAITravelV2ShadowEnabled: true}, logger, provider)
	if _, err := service.Reprocess(context.Background(), ReprocessInput{SessionID: "missing-session"}); err == nil {
		t.Fatal("expected missing session Reprocess to fail")
	}
	if logger.count("travel_v2_shadow_scheduler") != 0 || provider.calls != 0 {
		t.Fatalf("failed Reprocess must not schedule shadow: logs=%v calls=%d", logger.snapshot(), provider.calls)
	}
}

func TestReprocessTravelQueryV2SlowShadowIsOutsideCriticalPathAndMainContext(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	agentRunner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resposta real sem atraso do shadow", Model: "gpt-test"}}
	proposal := travelQueryV2BaseProposal(TravelQueryIntentSeatRequest)
	proposal.SeatRequest = SeatRequestChooseSpecificSeat
	provider := &blockingOpenAITravelQueryV2Interpreter{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result:  OpenAITravelQueryV2RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatOpenAITravelV2ShadowEnabled: true}, agentRunner, provider)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511900000302",
		Message:    IngestMessagePayload{Direction: "INBOUND", ProviderMessageID: "msg-shadow-slow", IdempotencyKey: "idem-shadow-slow", Body: "oi"},
	})
	if err != nil {
		t.Fatalf("ingest slow shadow turn: %v", err)
	}
	mainContext, cancelMain := context.WithCancel(context.Background())
	type reprocessOutcome struct {
		result ReprocessResult
		err    error
	}
	outcome := make(chan reprocessOutcome, 1)
	go func() {
		result, runErr := svc.Reprocess(mainContext, ReprocessInput{SessionID: ingested.Session.ID})
		outcome <- reprocessOutcome{result: result, err: runErr}
	}()

	var out ReprocessResult
	select {
	case got := <-outcome:
		if got.err != nil {
			t.Fatalf("reprocess with slow shadow: %v", got.err)
		}
		out = got.result
	case <-time.After(time.Second):
		t.Fatal("slow shadow blocked the real response path")
	}
	if out.Draft == nil || out.Draft.Body != agentRunner.result.ReplyText {
		t.Fatalf("slow shadow changed real draft: %+v", out.Draft)
	}
	waitTravelQueryV2ShadowSignal(t, provider.started, "background slow provider")
	cancelMain()
	close(provider.release)
	waitTravelQueryV2ShadowSignal(t, store.completed, "slow provider terminal result")
	if provider.callCount() != 1 {
		t.Fatalf("expected exactly one slow provider call, got %d", provider.callCount())
	}
}

func TestReprocessTravelQueryV2TimeoutOrPanicDoesNotChangeRealDraft(t *testing.T) {
	tests := []struct {
		name          string
		provider      *blockingOpenAITravelQueryV2Interpreter
		timeout       time.Duration
		wantErrorCode string
	}{
		{name: "timeout", provider: &blockingOpenAITravelQueryV2Interpreter{waitForContext: true}, timeout: 20 * time.Millisecond, wantErrorCode: "context_deadline_exceeded"},
		{name: "panic", provider: &blockingOpenAITravelQueryV2Interpreter{panicOnCall: true}, wantErrorCode: "openai_travel_query_v2_panic"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeTravelQueryV2ShadowClaimStore()
			agentRunner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resposta real preservada", Model: "gpt-test"}}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatOpenAITravelV2ShadowEnabled: true}, agentRunner, test.provider)
			svc.travelV2Timeout = test.timeout
			contactKey := fmt.Sprintf("55119000004%02d", index)
			out := ingestAndReprocessShadowDraft(t, svc, contactKey, "oi")
			if out.Draft == nil || out.Draft.Body != agentRunner.result.ReplyText {
				t.Fatalf("shadow %s changed real draft: %+v", test.name, out.Draft)
			}
			waitTravelQueryV2ShadowSignal(t, store.completed, test.name+" terminal result")
			input := test.provider.input()
			claim, ok := store.completedClaim(out.Session.ID, latestCandidateMessageID(out.Messages), input.IdempotencyKey)
			if !ok || claim.Summary == nil || claim.Summary.OpenAI.ErrorCode != test.wantErrorCode {
				t.Fatalf("unexpected %s terminal claim: %+v exists=%t", test.name, claim, ok)
			}
		})
	}
}

func TestReprocessTravelQueryV2DocumentMediaUsesStrongGuardrail(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	agentRunner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Pessoa Teste","document_type":"CPF","document":"529.982.247-25","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	proposal := travelQueryV2BaseProposal(TravelQueryIntentAvailabilitySearch)
	proposal.Origin = &LocationMeaning{Name: "Fraiburgo/SC", Role: LocationRoleOrigin}
	proposal.Destination = &LocationMeaning{Name: "Santa Ines/MA", Role: LocationRoleDestination}
	provider := &fakeOpenAITravelQueryV2Interpreter{enabled: true, result: OpenAITravelQueryV2RunResult{
		Proposal: proposal, ProposalParseable: true, SchemaValid: true,
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatOpenAITravelV2ShadowEnabled: true}, agentRunner, provider)
	session := seedDocumentCollectionBookingHistory(t, store.fakeStore, "5549988709092")
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-v2-document-strong",
			IdempotencyKey:    "idem-v2-document-strong",
			Body:              "segue",
			NormalizedPayload: map[string]interface{}{"image_data_url": "data:image/jpeg;base64,/9j/2Q==", "image_mime_type": "image/jpeg"},
		},
	})
	if err != nil {
		t.Fatalf("ingest document media: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess document media: %v", err)
	}
	waitTravelQueryV2ShadowSignal(t, store.completed, "document guardrail shadow")
	if provider.lastInput.ExistingDecisionStrength != DecisionStrengthStrong {
		t.Fatalf("document media must force STRONG before shadow, input=%+v", provider.lastInput)
	}
	claim, ok := store.completedClaim(out.Session.ID, latestCandidateMessageID(out.Messages), provider.lastInput.IdempotencyKey)
	if !ok || claim.Summary == nil || claim.Summary.Validation.Accepted ||
		!containsString(claim.Summary.Validation.ReasonCodes, string(TravelQueryValidationReasonStrongDecisionProtected)) {
		t.Fatalf("document media proposal must be factually protected, claim=%+v exists=%t", claim, ok)
	}
}

func TestReprocessTravelQueryV2ShadowRunsBesideV1WithZeroRuntimeEffect(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	agentRunner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resposta real intacta", Model: "gpt-test"}}
	v1 := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{Interpretation: StructuredInterpretation{
			Intent: StructuredIntentGreeting, TurnMeaning: TurnMeaningGreeting, Confidence: 0.9, Source: "openai_structured",
		}},
	}
	proposal := travelQueryV2BaseProposal(TravelQueryIntentSeatRequest)
	proposal.SeatRequest = SeatRequestChooseSpecificSeat
	v2 := &fakeOpenAITravelQueryV2Interpreter{
		enabled: true,
		result:  OpenAITravelQueryV2RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true, ProviderResponseID: "resp_v2_shadow"},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAITravelV2ShadowEnabled:    true,
	}, agentRunner, v1, v2)

	out := ingestAndReprocessShadowDraft(t, svc, "5511900000301", "oi")
	if v1.calls != 1 {
		t.Fatalf("expected V1 to remain intact, calls=%d", v1.calls)
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != agentRunner.result.ReplyText {
		t.Fatalf("V2 changed the real draft: %+v", out.Draft)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("V2 must not execute tools: %+v", out.ToolCalls)
	}
	if _, exists := out.Memory[structuredInterpreterShadowKey]; !exists {
		t.Fatalf("expected V1 shadow summary to remain intact")
	}
	if _, exists := out.Memory[travelQueryV2ShadowKey]; exists {
		t.Fatalf("background V2 result must not enter the synchronous response memory")
	}
	waitTravelQueryV2ShadowSignal(t, store.completed, "first terminal shadow result")
	waitTravelQueryV2ShadowSignal(t, store.claimAttempts, "first durable claim")
	if v2.calls != 1 {
		t.Fatalf("expected one background V2 call, got %d", v2.calls)
	}
	if v2.lastInput.IdempotencyKey == "" || v2.lastInput.IdempotencyKey == v1.lastInput.IdempotencyKey {
		t.Fatalf("expected distinct non-empty V2 idempotency key")
	}
	messageID := latestCandidateMessageID(out.Messages)
	claim, ok := store.completedClaim(out.Session.ID, messageID, v2.lastInput.IdempotencyKey)
	if !ok || claim.Status != travelQueryV2ShadowClaimCompleted || claim.Summary == nil {
		t.Fatalf("expected durable COMPLETED shadow claim, got %+v exists=%t", claim, ok)
	}

	if _, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: out.Session.ID}); err != nil {
		t.Fatalf("repeat reprocess: %v", err)
	}
	waitTravelQueryV2ShadowSignal(t, store.claimAttempts, "reused completed claim")
	if v2.calls != 1 {
		t.Fatalf("repeated Reprocess must reuse COMPLETED without another provider call, got %d", v2.calls)
	}
}

func travelQueryV2BaseProposal(intent TravelQueryIntent) TravelQueryMeaningV2 {
	return TravelQueryMeaningV2{
		Intent:             intent,
		TurnMeaning:        TurnMeaningNewRequest,
		MentionedLocations: []LocationMeaning{},
		DatePreference:     DatePreference{Mode: DateModeUnspecified},
		OptionReference:    OptionReference{Kind: OptionReferenceNone},
		RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
		SeatRequest:        SeatRequestNone,
		InstitutionalTopic: InstitutionalTopicNone,
		MissingFields:      []string{},
		Confidence:         0.95,
		Reasons:            []string{},
	}
}

func travelQueryV2TestAvailabilityFacts(observedAt time.Time, optionCount int) TravelQueryAvailabilityFactsV2 {
	options := make([]AvailabilitySearchItem, 0, optionCount)
	for index := 0; index < optionCount; index++ {
		options = append(options, AvailabilitySearchItem{
			TripID:                 fmt.Sprintf("trip-v2-%d", index+1),
			BoardStopID:            fmt.Sprintf("board-v2-%d", index+1),
			AlightStopID:           fmt.Sprintf("alight-v2-%d", index+1),
			OriginStopID:           "SC_SEARA",
			DestinationStopID:      "SC_VIDEIRA",
			OriginDisplayName:      "Seara/SC",
			DestinationDisplayName: "Videira/SC",
			TripDate:               observedAt.AddDate(0, 0, index+1).Format("2006-01-02"),
		})
	}
	return TravelQueryAvailabilityFactsV2{VisibleOptions: options}
}

func travelQueryV2AttachSelectionContext(input *TravelQueryV2ShadowInput, observedAt time.Time, optionCount int) {
	body := "Encontrei estas opções:\n"
	for index := 1; index <= optionCount; index++ {
		body += fmt.Sprintf("%d. Opção %d\n", index, index)
	}
	body += "Qual opção você prefere?"
	receivedAt := observedAt.Add(-time.Minute)
	input.StructuredInput.State.Phase = ConversationPhaseTripSelection
	input.StructuredInput.History = []Message{{
		ID:               "prompt-travel-v2",
		Direction:        "OUTBOUND",
		Body:             body,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       receivedAt,
	}}
	input.ActivePrompt = ActivePromptContext{
		Kind:                    ActivePromptAvailabilityOptionChoice,
		Phase:                   ConversationPhaseTripSelection,
		SourceMessageID:         "prompt-travel-v2",
		SourceMessageBody:       body,
		SourceMessageReceivedAt: receivedAt,
		AvailabilityOptionCount: optionCount,
		HasAvailabilityList:     true,
	}
	facts := travelQueryV2TestAvailabilityFacts(observedAt, optionCount)
	facts.SourceMessageID = "prompt-travel-v2"
	facts.SourceMessageReceivedAt = receivedAt
	input.AvailabilityFacts = facts
	input.LocationCatalog = buildTravelQueryV2ShadowLocationCatalog(facts)
}

func validOpenAITravelQueryV2PayloadMap() map[string]interface{} {
	return map[string]interface{}{
		"intent":       string(TravelQueryIntentAvailabilitySearch),
		"turn_meaning": string(TurnMeaningNewRequest),
		"origin": map[string]interface{}{
			"name": "Fraiburgo/SC", "role": string(LocationRoleOrigin),
		},
		"destination": map[string]interface{}{
			"name": "Santa Ines/MA", "role": string(LocationRoleDestination),
		},
		"mentioned_locations": []interface{}{},
		"date_preference": map[string]interface{}{
			"mode": string(DateModeUnspecified), "exact_date": "",
		},
		"option_reference": map[string]interface{}{
			"kind": string(OptionReferenceNone), "index": 0, "date": "",
		},
		"route_coverage": map[string]interface{}{
			"query_location": "", "mode": string(RouteCoverageModeUnspecified),
		},
		"seat_request":        string(SeatRequestNone),
		"institutional_topic": string(InstitutionalTopicNone),
		"needs_clarification": false,
		"missing_fields":      []interface{}{},
		"confidence":          0.95,
		"reasons":             []interface{}{"explicit_route"},
	}
}

func validOpenAITravelQueryV2OutputText(t *testing.T) string {
	t.Helper()
	return string(mustMarshalOpenAITravelQueryV2Test(t, validOpenAITravelQueryV2PayloadMap()))
}

func mustMarshalOpenAITravelQueryV2Test(t *testing.T, value interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	return data
}

func openAITravelQueryV2TestResponse(id string, outputText string) map[string]interface{} {
	return map[string]interface{}{
		"id": id,
		"output": []interface{}{map[string]interface{}{
			"type": "message",
			"content": []interface{}{map[string]interface{}{
				"type": "output_text", "text": outputText,
			}},
		}},
	}
}

func waitTravelQueryV2ShadowSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}
