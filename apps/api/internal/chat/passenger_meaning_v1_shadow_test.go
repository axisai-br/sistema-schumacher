package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

type passengerMeaningV1InterpreterFunc func(context.Context, OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error)

func (f passengerMeaningV1InterpreterFunc) Enabled() bool { return f != nil }

func (f passengerMeaningV1InterpreterFunc) InterpretPassengerMeaningV1(ctx context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
	return f(ctx, input)
}

type memoryPassengerMeaningV1ShadowStore struct {
	*fakeStore
	mu             sync.Mutex
	claims         map[string]PassengerMeaningV1ShadowClaimResult
	identities     map[string]PassengerMeaningV1ShadowIdentity
	leaseExpiresAt map[string]time.Time
	claimCalls     int
	completions    int
	completed      chan struct{}
}

func newMemoryPassengerMeaningV1ShadowStore() *memoryPassengerMeaningV1ShadowStore {
	return &memoryPassengerMeaningV1ShadowStore{
		fakeStore:      newFakeStore(),
		claims:         map[string]PassengerMeaningV1ShadowClaimResult{},
		identities:     map[string]PassengerMeaningV1ShadowIdentity{},
		leaseExpiresAt: map[string]time.Time{},
		completed:      make(chan struct{}, 64),
	}
}

func (s *memoryPassengerMeaningV1ShadowStore) ClaimPassengerMeaningV1Shadow(_ context.Context, identity PassengerMeaningV1ShadowIdentity, leaseDuration time.Duration) (PassengerMeaningV1ShadowClaimResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimCalls++
	if existing, ok := s.claims[identity.IdempotencyKey]; ok {
		return existing, nil
	}
	s.claims[identity.IdempotencyKey] = PassengerMeaningV1ShadowClaimResult{Status: PassengerMeaningV1ShadowClaimInProgress}
	s.identities[identity.IdempotencyKey] = identity
	s.leaseExpiresAt[identity.IdempotencyKey] = time.Now().Add(leaseDuration)
	return PassengerMeaningV1ShadowClaimResult{Status: PassengerMeaningV1ShadowClaimAcquired}, nil
}

func (s *memoryPassengerMeaningV1ShadowStore) CompletePassengerMeaningV1Shadow(_ context.Context, identity PassengerMeaningV1ShadowIdentity, summary PassengerMeaningV1ShadowSummary) error {
	s.mu.Lock()
	s.claims[identity.IdempotencyKey] = PassengerMeaningV1ShadowClaimResult{Status: PassengerMeaningV1ShadowClaimCompleted, Summary: summary}
	s.completions++
	s.mu.Unlock()
	select {
	case s.completed <- struct{}{}:
	default:
	}
	return nil
}

func (s *memoryPassengerMeaningV1ShadowStore) RecoverExpiredPassengerMeaningV1ShadowClaims(_ context.Context, sessionID string, batchSize int) (PassengerMeaningV1ShadowRecoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if batchSize <= 0 || batchSize > passengerMeaningV1ShadowRecoveryBatchSize {
		batchSize = passengerMeaningV1ShadowRecoveryBatchSize
	}
	keys := make([]string, 0, len(s.claims))
	for key, claim := range s.claims {
		identity := s.identities[key]
		if identity.SessionID == sessionID && claim.Status == PassengerMeaningV1ShadowClaimInProgress && !s.leaseExpiresAt[key].After(time.Now()) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > batchSize {
		keys = keys[:batchSize]
	}
	for _, key := range keys {
		s.claims[key] = PassengerMeaningV1ShadowClaimResult{
			Status:  PassengerMeaningV1ShadowClaimCompleted,
			Summary: abandonedPassengerMeaningV1ShadowSummary(),
		}
	}
	return PassengerMeaningV1ShadowRecoveryResult{
		MessagesProcessed: len(keys),
		ClaimsCompleted:   len(keys),
	}, nil
}

func (s *memoryPassengerMeaningV1ShadowStore) snapshotClaim(key string) (PassengerMeaningV1ShadowClaimResult, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claims[key], s.claimCalls, s.completions
}

func TestPassengerMeaningV1ShadowHasZeroRuntimeInfluence(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	stateBefore := clonePassengerMeaningV1State(state)
	turn := "João CPF 52998224725 telefone 48999998888; eu e mais 2 crianças"
	provider := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{passengerMeaningV1UnknownChildReference("joao")}
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true, ProviderResponseID: "resp_safe_1"}, nil
	})

	summary := RunPassengerMeaningV1Shadow(context.Background(), PassengerMeaningV1ShadowInput{
		Enabled:             true,
		OpenAIInterpreter:   provider,
		CurrentTurn:         turn,
		State:               state,
		PromptEvent:         prompt,
		SourceMessageID:     "message-sensitive",
		SourcePromptEventID: prompt.EventID,
		IdempotencyKey:      "shadow-zero-runtime",
	})
	if !reflect.DeepEqual(state, stateBefore) {
		t.Fatalf("shadow must not mutate passenger state:\nbefore=%+v\nafter=%+v", stateBefore, state)
	}
	if !summary.Validation.Accepted {
		t.Fatalf("expected structured proposal to validate: %+v", summary)
	}
	if summary.Metrics.SchemaParseSuccessCount != 1 || summary.Metrics.ValidatorAcceptanceCount != 1 ||
		summary.Metrics.CriticalActionViolationCount != 0 || summary.Metrics.StateMutationCount != 0 || summary.Metrics.ToolCallCount != 0 {
		t.Fatalf("unexpected shadow safety metrics: %+v", summary.Metrics)
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, forbidden := range []string{turn, "João", "52998224725", "48999998888", "joao", "message-sensitive", prompt.EventID} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("persistable summary leaked raw body, PII, or source IDs (%q): %s", forbidden, raw)
		}
	}
}

func TestPassengerMeaningV1ShadowProviderErrorsAndTimeoutsFailOpen(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	providerErr := passengerMeaningV1InterpreterFunc(func(_ context.Context, _ OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		return OpenAIPassengerMeaningV1RunResult{}, ErrOpenAIPassengerMeaningV1RequestFailed
	})
	summary := RunPassengerMeaningV1Shadow(context.Background(), PassengerMeaningV1ShadowInput{
		Enabled: true, OpenAIInterpreter: providerErr, CurrentTurn: "somos 3", State: state, PromptEvent: prompt,
		SourceMessageID: "message-error", SourcePromptEventID: prompt.EventID,
	})
	if summary.OpenAI.Status != string(PassengerMeaningV1ShadowError) || summary.OpenAI.ErrorCode != "openai_passenger_meaning_v1_request_failed" {
		t.Fatalf("provider error must become a closed fail-open summary: %+v", summary)
	}

	store := newMemoryPassengerMeaningV1ShadowStore()
	var calls atomic.Int32
	timeoutProvider := passengerMeaningV1InterpreterFunc(func(ctx context.Context, _ OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		calls.Add(1)
		<-ctx.Done()
		return OpenAIPassengerMeaningV1RunResult{}, ctx.Err()
	})
	job := passengerMeaningV1ShadowTestJob("timeout", state, prompt, timeoutProvider)
	job.Timeout = 15 * time.Millisecond
	service := NewService(store, config.Config{ChatOpenAIPassengerV1ShadowEnabled: true}, timeoutProvider)
	startedAt := time.Now()
	result := service.executePassengerMeaningV1ShadowJob(job)
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("provider timeout was not bounded: %s", elapsed)
	}
	if calls.Load() != 1 || result.Summary.OpenAI.ErrorCode != "context_deadline_exceeded" || result.CompletionError != nil {
		t.Fatalf("timeout must be persisted and fail open: calls=%d result=%+v", calls.Load(), result)
	}
}

func TestPassengerMeaningV1IdempotencyOneCallPerMessageEpoch(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	store := newMemoryPassengerMeaningV1ShadowStore()
	var calls atomic.Int32
	provider := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		calls.Add(1)
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
	})
	service := NewService(store, config.Config{ChatOpenAIPassengerV1ShadowEnabled: true}, provider)
	job := passengerMeaningV1ShadowTestJob("same-identity", state, prompt, provider)
	first := service.executePassengerMeaningV1ShadowJob(job)
	second := service.executePassengerMeaningV1ShadowJob(job)
	if first.ClaimStatus != PassengerMeaningV1ShadowClaimAcquired || second.ClaimStatus != PassengerMeaningV1ShadowClaimCompleted {
		t.Fatalf("expected acquire then durable reuse: first=%+v second=%+v", first, second)
	}
	if calls.Load() != 1 {
		t.Fatalf("same message/epoch/version identity must call provider once, got %d", calls.Load())
	}

	differentEpoch := job
	differentPrompt, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, "prompt-passenger-different-epoch")
	if !ok {
		t.Fatal("expected a second canonical passenger prompt epoch")
	}
	differentState := clonePassengerMeaningV1State(state)
	differentState.PassengerPromptMessageID = differentPrompt.MessageID
	differentEpoch.Identity.PromptEventID = differentPrompt.EventID
	differentEpoch.Identity.IdempotencyKey = "passenger-shadow-different-epoch"
	differentEpoch.Input.SourcePromptEventID = differentEpoch.Identity.PromptEventID
	differentEpoch.Input.PromptEvent = differentPrompt
	differentEpoch.Input.State = differentState
	service.executePassengerMeaningV1ShadowJob(differentEpoch)
	if calls.Load() != 2 {
		t.Fatalf("different prompt epoch must have a different provider identity, got %d calls", calls.Load())
	}
}

func TestPassengerMeaningV1ShadowExpiredClaimRecoveredByNewIdentity(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	store := newMemoryPassengerMeaningV1ShadowStore()
	var providerACalls atomic.Int32
	providerA := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		providerACalls.Add(1)
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
	})
	jobA := passengerMeaningV1ShadowTestJob("expired-a", state, prompt, providerA)
	claimA, err := store.ClaimPassengerMeaningV1Shadow(context.Background(), jobA.Identity, time.Millisecond)
	if err != nil || claimA.Status != PassengerMeaningV1ShadowClaimAcquired {
		t.Fatalf("acquire claim A: claim=%+v err=%v", claimA, err)
	}
	if _, err := providerA.InterpretPassengerMeaningV1(context.Background(), OpenAIPassengerMeaningV1RunInput{
		SourceMessageID: jobA.Identity.MessageID, SourcePromptEventID: jobA.Identity.PromptEventID,
	}); err != nil {
		t.Fatalf("simulate provider A before crash: %v", err)
	}
	store.mu.Lock()
	store.leaseExpiresAt[jobA.Identity.IdempotencyKey] = time.Now().Add(-time.Second)
	store.mu.Unlock()

	var providerBCalls atomic.Int32
	providerB := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		providerBCalls.Add(1)
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
	})
	jobB := passengerMeaningV1ShadowTestJob("new-b", state, prompt, providerB)
	jobB.Identity.SessionID = jobA.Identity.SessionID
	service := NewService(store, config.Config{ChatOpenAIPassengerV1ShadowEnabled: true}, providerB)
	resultB := service.executePassengerMeaningV1ShadowJob(jobB)
	claimAAfter, _, _ := store.snapshotClaim(jobA.Identity.IdempotencyKey)
	if claimAAfter.Status != PassengerMeaningV1ShadowClaimCompleted ||
		claimAAfter.Summary.OpenAI.ErrorCode != "passenger_meaning_v1_shadow_execution_abandoned" {
		t.Fatalf("new identity B must terminalize expired A without replay: A=%+v B=%+v", claimAAfter, resultB)
	}
	if providerACalls.Load() != 1 || providerBCalls.Load() != 1 {
		t.Fatalf("recovery must not recall provider A or interfere with B: A=%d B=%d", providerACalls.Load(), providerBCalls.Load())
	}
	replayB := service.executePassengerMeaningV1ShadowJob(jobB)
	claimAAfterReplay, _, _ := store.snapshotClaim(jobA.Identity.IdempotencyKey)
	if replayB.ClaimStatus != PassengerMeaningV1ShadowClaimCompleted ||
		claimAAfterReplay.Status != PassengerMeaningV1ShadowClaimCompleted ||
		providerACalls.Load() != 1 || providerBCalls.Load() != 1 {
		t.Fatalf("recovery and new claim must be idempotent: A=%+v replayB=%+v callsA=%d callsB=%d", claimAAfterReplay, replayB, providerACalls.Load(), providerBCalls.Load())
	}
}

func TestPassengerMeaningV1ShadowExpiredClaimRecoveredByNormalNewInbound(t *testing.T) {
	store := newMemoryPassengerMeaningV1ShadowStore()
	session := seedPassengerCollectionPhase(t, store.fakeStore)
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	var providerACalls atomic.Int32
	providerA := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		providerACalls.Add(1)
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
	})
	jobA := passengerMeaningV1ShadowTestJob("normal-inbound-expired-a", state, prompt, providerA)
	jobA.Identity.SessionID = session.ID
	if claim, err := store.ClaimPassengerMeaningV1Shadow(context.Background(), jobA.Identity, time.Millisecond); err != nil || claim.Status != PassengerMeaningV1ShadowClaimAcquired {
		t.Fatalf("acquire old claim A: claim=%+v err=%v", claim, err)
	}
	if _, err := providerA.InterpretPassengerMeaningV1(context.Background(), OpenAIPassengerMeaningV1RunInput{
		SourceMessageID: jobA.Identity.MessageID, SourcePromptEventID: jobA.Identity.PromptEventID,
	}); err != nil {
		t.Fatalf("simulate provider A before crash: %v", err)
	}
	store.mu.Lock()
	store.leaseExpiresAt[jobA.Identity.IdempotencyKey] = time.Now().Add(-time.Second)
	store.mu.Unlock()

	var providerBCalls atomic.Int32
	providerB := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		providerBCalls.Add(1)
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
	})
	service := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIPassengerV1ShadowEnabled: true,
	}, providerB)
	if _, err := service.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "provider-passenger-meaning-recovery-b",
			IdempotencyKey:    "ingest-passenger-meaning-recovery-b",
			Body:              "eu e mais 2 crianças",
		},
	}); err != nil {
		t.Fatalf("ingest new inbound B: %v", err)
	}
	resultB, err := service.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess new inbound B: %v", err)
	}
	select {
	case <-store.completed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for new shadow B")
	}
	claimAAfter, claimCalls, _ := store.snapshotClaim(jobA.Identity.IdempotencyKey)
	store.mu.Lock()
	durableIdentities := len(store.identities)
	store.mu.Unlock()
	if claimAAfter.Status != PassengerMeaningV1ShadowClaimCompleted ||
		claimAAfter.Summary.OpenAI.ErrorCode != "passenger_meaning_v1_shadow_execution_abandoned" ||
		claimCalls != 2 || durableIdentities != 2 {
		t.Fatalf("normal inbound B must create its own identity and terminalize A: A=%+v claims=%d identities=%d", claimAAfter, claimCalls, durableIdentities)
	}
	stateAfter, ok := passengerClarificationStateV1FromSession(resultB.Session)
	if !ok || stateAfter.PassengerCountKnown || stateAfter.ChildUnder5CountKnown {
		t.Fatalf("shadow recovery/result must not promote meaning into B1 state: %+v", stateAfter)
	}
	if providerACalls.Load() != 1 || providerBCalls.Load() != 1 {
		t.Fatalf("normal recovery must not recall A or duplicate B: A=%d B=%d", providerACalls.Load(), providerBCalls.Load())
	}
}

func TestPassengerMeaningV1ShadowRecoverySQLIsSessionBoundedAndIdentityIndependent(t *testing.T) {
	query := strings.ToLower(recoverExpiredPassengerMeaningV1ShadowClaimsSQL)
	for _, expected := range []string{
		"message.session_id = $1::uuid",
		"jsonb_each(message.normalized_payload -> $2)",
		"order by message.created_at, message.id",
		"limit $4",
		"for update of message skip locked",
		"completed_at",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("passenger recovery query must contain %q", expected)
		}
	}
	for _, forbidden := range []string{"message.id = $", "claim.key = $"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("recovery must not require replaying the exact old identity: found %q", forbidden)
		}
	}
}

func TestPassengerMeaningV1ShadowRecoverySQLLimitsClaimsWithinSingleMessage(t *testing.T) {
	claims := make(map[string]passengerMeaningV1ShadowClaimRecord, passengerMeaningV1ShadowRecoveryBatchSize+1)
	for index := 0; index < passengerMeaningV1ShadowRecoveryBatchSize+1; index++ {
		key := "claim-" + string(rune('a'+index))
		claims[key] = passengerMeaningV1ShadowClaimRecord{
			Status:              passengerMeaningV1ShadowClaimInProgress,
			IdempotencyKey:      key,
			ContractVersion:     passengerClarificationMeaningV1Version,
			SourceMessageID:     "one-message-with-26-expired-claims",
			SourcePromptEventID: "prompt-with-26-expired-claims",
		}
	}
	messagePayload := map[string]interface{}{passengerMeaningV1ShadowClaimsPayloadKey: claims}
	raw, err := json.Marshal(messagePayload)
	if err != nil {
		t.Fatalf("marshal one-message recovery fixture: %v", err)
	}
	var decoded map[string]map[string]passengerMeaningV1ShadowClaimRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode one-message recovery fixture: %v", err)
	}
	if got := len(decoded[passengerMeaningV1ShadowClaimsPayloadKey]); got != passengerMeaningV1ShadowRecoveryBatchSize+1 {
		t.Fatalf("fixture must contain 26 claims in one message, got %d", got)
	}

	query := strings.ToLower(recoverExpiredPassengerMeaningV1ShadowClaimsSQL)
	claimCandidatesStart := strings.Index(query, "candidate_claims as materialized")
	lockedMessagesStart := strings.Index(query, "locked_messages as materialized")
	if claimCandidatesStart < 0 || lockedMessagesStart <= claimCandidatesStart {
		t.Fatal("recovery must select a claim-level candidate batch before locking its messages")
	}
	claimCandidatesSQL := query[claimCandidatesStart:lockedMessagesStart]
	jsonExpansion := strings.Index(claimCandidatesSQL, "jsonb_each")
	claimLimit := strings.Index(claimCandidatesSQL, "limit $4")
	if jsonExpansion < 0 || claimLimit <= jsonExpansion {
		t.Fatal("recovery LIMIT $4 must apply after jsonb_each so one message cannot complete more than 25 claims")
	}
	transformedStart := strings.Index(query, "transformed as (")
	updatedStart := strings.Index(query, "updated as (")
	if transformedStart < 0 || updatedStart <= transformedStart {
		t.Fatal("recovery must transform the selected claim batch before updating messages")
	}
	transformedSQL := query[transformedStart:updatedStart]
	if strings.Contains(transformedSQL, "jsonb_each") || !strings.Contains(transformedSQL, "jsonb_object_agg(") ||
		!strings.Contains(transformedSQL, "selected.key") {
		t.Fatal("recovery transformation must aggregate only selected claim keys, not reopen every claim in the message")
	}
}

func TestPassengerMeaningV1ShadowConcurrencyIsBounded(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	store := newMemoryPassengerMeaningV1ShadowStore()
	release := make(chan struct{})
	started := make(chan struct{}, 16)
	var active atomic.Int32
	var maxActive atomic.Int32
	provider := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		current := active.Add(1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
	})
	service := NewService(store, config.Config{ChatOpenAIPassengerV1ShadowEnabled: true}, provider)
	accepted := 0
	for index := 0; index < passengerMeaningV1ShadowMaxConcurrent+8; index++ {
		job := passengerMeaningV1ShadowTestJob("bounded-"+string(rune('a'+index)), state, prompt, provider)
		if service.schedulePassengerMeaningV1Shadow(job) {
			accepted++
		}
	}
	if accepted != passengerMeaningV1ShadowMaxConcurrent {
		close(release)
		t.Fatalf("nonblocking scheduler must accept exactly capacity=%d, got %d", passengerMeaningV1ShadowMaxConcurrent, accepted)
	}
	for index := 0; index < accepted; index++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("timed out waiting for bounded providers to start")
		}
	}
	if maxActive.Load() > passengerMeaningV1ShadowMaxConcurrent {
		close(release)
		t.Fatalf("provider concurrency exceeded bound: %d", maxActive.Load())
	}
	close(release)
	for index := 0; index < accepted; index++ {
		select {
		case <-store.completed:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for bounded jobs to complete")
		}
	}
	deadline := time.Now().Add(time.Second)
	for len(passengerMeaningV1ShadowBackgroundSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(passengerMeaningV1ShadowBackgroundSlots) != 0 {
		t.Fatal("scheduler slots were not released after completion")
	}
}

func TestPassengerMeaningV1ShadowRejectsIneligibleStructuralStateBeforeProvider(t *testing.T) {
	state, _ := passengerMeaningV1PassengerPromptFixture()
	state.PassengerSlotStatus = PassengerClarificationSlotConflicting
	state.PassengerPromptMessageID = ""
	_, ok := buildPassengerMeaningV1ShadowBackgroundJob(
		"session-1",
		[]Message{{ID: "message-1"}},
		"somos 3",
		CanonicalConversationState{Phase: ConversationPhasePassengerCollection},
		state,
		nil,
		passengerMeaningV1ShadowProviderTimeout,
	)
	if ok {
		t.Fatal("conflicting state must not create a provider job")
	}
}

type passengerMeaningV1RuntimeProjection struct {
	Status              string
	Reason              string
	DraftBody           string
	TemplateName        string
	Action              string
	AutoSendStatus      string
	PassengerKnown      bool
	PassengerCount      int
	PassengerProvenance PassengerCountProvenance
	PassengerSlot       PassengerClarificationSlotStatusV1
	ChildKnown          bool
	ChildCount          int
	ChildSlot           PassengerClarificationSlotStatusV1
}

func TestPassengerMeaningV1ShadowFlagOffAndOnPreserveB1Runtime(t *testing.T) {
	baseline, _, baselineStore := runPassengerMeaningV1RuntimeFixture(t, false)
	withShadow, providerCalls, shadowStore := runPassengerMeaningV1RuntimeFixture(t, true)
	if !reflect.DeepEqual(baseline, withShadow) {
		t.Fatalf("shadow must not change response/template/autosend/state:\nbaseline=%+v\nshadow=%+v", baseline, withShadow)
	}
	if providerCalls != 1 {
		t.Fatalf("eligible flag-on turn must call provider once, got %d", providerCalls)
	}
	if _, claims, completions := shadowStore.snapshotClaim(firstPassengerMeaningV1ClaimKey(shadowStore)); claims != 1 || completions != 1 {
		t.Fatalf("eligible shadow must durably claim and complete once: claims=%d completions=%d", claims, completions)
	}
	if _, claims, completions := baselineStore.snapshotClaim(firstPassengerMeaningV1ClaimKey(baselineStore)); claims != 0 || completions != 0 {
		t.Fatalf("flag off must preserve exact B1 path without shadow storage: claims=%d completions=%d", claims, completions)
	}
}

func TestPassengerMeaningV1ShadowSkipsDeterministicStrongDecisions(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantIntent Intent
	}{
		{name: "explicit_cancellation", body: "quero cancelar minha reserva", wantIntent: IntentBookingCancel},
		{name: "explicit_human_request", body: "quero falar com atendente", wantIntent: IntentHumanSupport},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryPassengerMeaningV1ShadowStore()
			session := seedPassengerCollectionPhase(t, store.fakeStore)
			providerStarted := make(chan struct{}, 1)
			var providerCalls atomic.Int32
			provider := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
				providerCalls.Add(1)
				providerStarted <- struct{}{}
				proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 1)
				return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
			})
			service := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIPassengerV1ShadowEnabled: true,
			}, provider)
			var routedDecision IntentDecision
			var routerCalls int
			service.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
				routerCalls++
				routedDecision = routeDeterministicIntent(history, currentTurn, state, observedAt)
				return routedDecision
			}

			if _, err := service.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "provider-passenger-meaning-v1-strong-" + test.name,
					IdempotencyKey:    "ingest-passenger-meaning-v1-strong-" + test.name,
					Body:              test.body,
				},
			}); err != nil {
				t.Fatalf("ingest STRONG passenger shadow fixture: %v", err)
			}
			if _, err := service.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID}); err != nil {
				t.Fatalf("reprocess STRONG passenger shadow fixture: %v", err)
			}
			if routerCalls != 1 || routedDecision.Intent != test.wantIntent ||
				travelQueryV2ShadowDecisionStrength(routedDecision, false) != DecisionStrengthStrong {
				t.Fatalf("fixture must route exactly one STRONG %s decision: calls=%d decision=%+v", test.wantIntent, routerCalls, routedDecision)
			}

			select {
			case <-providerStarted:
				select {
				case <-store.completed:
				case <-time.After(2 * time.Second):
					t.Fatal("timed out waiting for incorrectly scheduled STRONG shadow to complete")
				}
				t.Fatalf("STRONG %s decision must not call PassengerMeaningV1 provider, got %d call(s)", test.wantIntent, providerCalls.Load())
			case <-time.After(100 * time.Millisecond):
			}
			if _, claims, completions := store.snapshotClaim(""); providerCalls.Load() != 0 || claims != 0 || completions != 0 {
				t.Fatalf("STRONG %s decision must not schedule or persist shadow: provider=%d claims=%d completions=%d",
					test.wantIntent, providerCalls.Load(), claims, completions)
			}
		})
	}
}

func runPassengerMeaningV1RuntimeFixture(t *testing.T, enabled bool) (passengerMeaningV1RuntimeProjection, int32, *memoryPassengerMeaningV1ShadowStore) {
	t.Helper()
	store := newMemoryPassengerMeaningV1ShadowStore()
	session := seedPassengerCollectionPhase(t, store.fakeStore)
	var calls atomic.Int32
	provider := passengerMeaningV1InterpreterFunc(func(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
		calls.Add(1)
		proposal := passengerMeaningV1KnownPassengerProposal(input.SourceMessageID, input.SourcePromptEventID, 3)
		return OpenAIPassengerMeaningV1RunResult{Proposal: proposal, ProposalParseable: true, SchemaValid: true}, nil
	})
	service := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIPassengerV1ShadowEnabled: enabled,
	}, provider)
	var routedDecision IntentDecision
	service.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
		routedDecision = routeDeterministicIntent(history, currentTurn, state, observedAt)
		return routedDecision
	}
	if _, err := service.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "provider-passenger-meaning-v1",
			IdempotencyKey:    "ingest-passenger-meaning-v1",
			Body:              "eu e mais 2 crianças",
		},
	}); err != nil {
		t.Fatalf("ingest passenger meaning fixture: %v", err)
	}
	result, err := service.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess passenger meaning fixture: %v", err)
	}
	if travelQueryV2ShadowDecisionStrength(routedDecision, false) == DecisionStrengthStrong {
		t.Fatalf("flag comparison fixture must remain eligible through a non-STRONG deterministic decision: %+v", routedDecision)
	}
	if enabled {
		select {
		case <-store.completed:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for passenger shadow completion")
		}
	}
	if result.Draft == nil {
		t.Fatal("expected deterministic B1 draft")
	}
	state, ok := passengerClarificationStateV1FromSession(result.Session)
	if !ok {
		t.Fatal("expected passenger state in reprocess result")
	}
	projection := passengerMeaningV1RuntimeProjection{
		Status:              result.Status,
		Reason:              result.Reason,
		DraftBody:           result.Draft.Body,
		TemplateName:        firstNonEmpty(asString(result.Draft.Payload["template_name"]), asString(result.Draft.NormalizedPayload["template_name"])),
		Action:              firstNonEmpty(asString(result.Draft.Payload["action"]), asString(result.Draft.NormalizedPayload["action"])),
		AutoSendStatus:      firstNonEmpty(asString(result.Draft.Payload["auto_send_status"]), asString(result.Draft.NormalizedPayload["auto_send_status"])),
		PassengerKnown:      state.PassengerCountKnown,
		PassengerCount:      state.PassengerCount,
		PassengerProvenance: state.PassengerCountProvenance,
		PassengerSlot:       state.PassengerSlotStatus,
		ChildKnown:          state.ChildUnder5CountKnown,
		ChildCount:          state.ChildUnder5Count,
		ChildSlot:           state.ChildSlotStatus,
	}
	return projection, calls.Load(), store
}

func firstPassengerMeaningV1ClaimKey(store *memoryPassengerMeaningV1ShadowStore) string {
	store.mu.Lock()
	defer store.mu.Unlock()
	for key := range store.claims {
		return key
	}
	return ""
}

func passengerMeaningV1ShadowTestJob(id string, state PassengerClarificationStateV1, prompt PassengerClarificationEventV1, provider OpenAIPassengerMeaningV1Interpreter) passengerMeaningV1ShadowBackgroundJob {
	identity := PassengerMeaningV1ShadowIdentity{
		SessionID:       "session-" + id,
		MessageID:       "message-" + id,
		PromptEventID:   prompt.EventID,
		IdempotencyKey:  "passenger-shadow-" + id,
		ContractVersion: passengerClarificationMeaningV1Version,
	}
	return passengerMeaningV1ShadowBackgroundJob{
		Identity: identity,
		Input: PassengerMeaningV1ShadowInput{
			Enabled:             true,
			OpenAIInterpreter:   provider,
			CurrentTurn:         "eu e mais 2 crianças",
			State:               clonePassengerMeaningV1State(state),
			PromptEvent:         clonePassengerMeaningV1Prompt(prompt),
			SourceMessageID:     identity.MessageID,
			SourcePromptEventID: identity.PromptEventID,
			IdempotencyKey:      identity.IdempotencyKey,
		},
	}
}

func TestPassengerMeaningV1ShadowSanitizedErrorNeverPersistsRawProviderText(t *testing.T) {
	rawProviderError := errors.New("provider leaked CPF 52998224725")
	if got := sanitizePassengerMeaningV1ShadowError(rawProviderError); got != "openai_passenger_meaning_v1_error" {
		t.Fatalf("unexpected closed error code: %q", got)
	}
}
