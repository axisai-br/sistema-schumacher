package chat

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	passengerMeaningV1ShadowClaimInProgress  = "IN_PROGRESS"
	passengerMeaningV1ShadowClaimCompleted   = "COMPLETED"
	passengerMeaningV1ShadowClaimsPayloadKey = "passenger_meaning_v1_shadow_claims"

	passengerMeaningV1ShadowProviderTimeout = 10 * time.Second
	passengerMeaningV1ShadowStoreTimeout    = 2 * time.Second
	passengerMeaningV1ShadowMaxConcurrent   = 4

	passengerMeaningV1ShadowCompletionMaxAttempts = 3
	passengerMeaningV1ShadowCompletionBackoff     = 10 * time.Millisecond
	passengerMeaningV1ShadowLeaseSafetyMargin     = 5 * time.Second
	passengerMeaningV1ShadowRecoveryBatchSize     = 25
)

var (
	ErrPassengerMeaningV1ShadowClaimNotFound = errors.New("passenger meaning v1 shadow claim not found")
	errPassengerMeaningV1ShadowClaimPanic    = errors.New("passenger meaning v1 shadow claim panic")
	errPassengerMeaningV1ShadowCompletePanic = errors.New("passenger meaning v1 shadow completion panic")
	errPassengerMeaningV1ShadowRecoveryPanic = errors.New("passenger meaning v1 shadow recovery panic")
)

type PassengerMeaningV1ShadowClaimStatus string

const (
	PassengerMeaningV1ShadowClaimAcquired   PassengerMeaningV1ShadowClaimStatus = "ACQUIRED"
	PassengerMeaningV1ShadowClaimInProgress PassengerMeaningV1ShadowClaimStatus = PassengerMeaningV1ShadowClaimStatus(passengerMeaningV1ShadowClaimInProgress)
	PassengerMeaningV1ShadowClaimCompleted  PassengerMeaningV1ShadowClaimStatus = PassengerMeaningV1ShadowClaimStatus(passengerMeaningV1ShadowClaimCompleted)
)

type PassengerMeaningV1ShadowIdentity struct {
	SessionID       string
	MessageID       string
	PromptEventID   string
	IdempotencyKey  string
	ContractVersion int
}

type PassengerMeaningV1ShadowClaimResult struct {
	Status  PassengerMeaningV1ShadowClaimStatus
	Summary PassengerMeaningV1ShadowSummary
}

type passengerMeaningV1ShadowClaimRecord struct {
	Status                  string                           `json:"status"`
	IdempotencyKey          string                           `json:"idempotency_key"`
	ContractVersion         int                              `json:"contract_version"`
	SourceMessageID         string                           `json:"source_message_id"`
	SourcePromptEventID     string                           `json:"source_prompt_event_id"`
	ClaimedAt               string                           `json:"claimed_at,omitempty"`
	LeaseExpiresAt          string                           `json:"lease_expires_at,omitempty"`
	LeaseExpiresAtUnixMilli int64                            `json:"lease_expires_at_unix_ms,omitempty"`
	CompletedAt             string                           `json:"completed_at,omitempty"`
	Summary                 *PassengerMeaningV1ShadowSummary `json:"summary,omitempty"`
}

type PassengerMeaningV1ShadowClaimStore interface {
	ClaimPassengerMeaningV1Shadow(ctx context.Context, identity PassengerMeaningV1ShadowIdentity, leaseDuration time.Duration) (PassengerMeaningV1ShadowClaimResult, error)
	CompletePassengerMeaningV1Shadow(ctx context.Context, identity PassengerMeaningV1ShadowIdentity, summary PassengerMeaningV1ShadowSummary) error
	RecoverExpiredPassengerMeaningV1ShadowClaims(ctx context.Context, sessionID string, batchSize int) (PassengerMeaningV1ShadowRecoveryResult, error)
}

type PassengerMeaningV1ShadowRecoveryResult struct {
	MessagesProcessed int
	ClaimsCompleted   int
}

type passengerMeaningV1ShadowBackgroundJob struct {
	Identity PassengerMeaningV1ShadowIdentity
	Input    PassengerMeaningV1ShadowInput
	Timeout  time.Duration
}

type passengerMeaningV1ShadowExecutionResult struct {
	ClaimStatus     PassengerMeaningV1ShadowClaimStatus
	Summary         PassengerMeaningV1ShadowSummary
	CompletionError error
}

var passengerMeaningV1ShadowBackgroundSlots = make(chan struct{}, passengerMeaningV1ShadowMaxConcurrent)

func (s *Service) schedulePassengerMeaningV1Shadow(job passengerMeaningV1ShadowBackgroundJob) bool {
	if s == nil || !s.cfg.ChatOpenAIPassengerV1ShadowEnabled {
		return false
	}
	if !passengerMeaningV1ShadowIdentityValid(job.Identity) {
		s.logPassengerMeaningV1ShadowEvent("scheduler", "invalid_identity")
		return false
	}
	if _, ok := s.store.(PassengerMeaningV1ShadowClaimStore); !ok {
		s.logPassengerMeaningV1ShadowEvent("scheduler", "incompatible_store")
		return false
	}
	select {
	case passengerMeaningV1ShadowBackgroundSlots <- struct{}{}:
	default:
		s.logPassengerMeaningV1ShadowEvent("scheduler", "capacity_full")
		return false
	}
	s.logPassengerMeaningV1ShadowEvent("scheduler", "scheduled")
	go func() {
		defer func() {
			<-passengerMeaningV1ShadowBackgroundSlots
			_ = recover()
		}()
		s.executePassengerMeaningV1ShadowJob(job)
	}()
	return true
}

func (s *Service) executePassengerMeaningV1ShadowJob(job passengerMeaningV1ShadowBackgroundJob) passengerMeaningV1ShadowExecutionResult {
	store, ok := s.store.(PassengerMeaningV1ShadowClaimStore)
	if !ok {
		return passengerMeaningV1ShadowExecutionResult{}
	}
	recoveryContext, cancelRecovery := context.WithTimeout(context.Background(), passengerMeaningV1ShadowStoreTimeout)
	recovery, recoveryErr := recoverExpiredPassengerMeaningV1ShadowClaimsSafely(
		recoveryContext,
		store,
		job.Identity.SessionID,
		passengerMeaningV1ShadowRecoveryBatchSize,
	)
	cancelRecovery()
	if recoveryErr != nil {
		s.logPassengerMeaningV1ShadowStorageFailure("recovery", "sweep_failed", "recovery", recoveryErr)
	} else if recovery.ClaimsCompleted > 0 {
		s.logPassengerMeaningV1ShadowRecovery(recovery)
	}
	timeout := job.Timeout
	if timeout <= 0 {
		timeout = passengerMeaningV1ShadowProviderTimeout
	}
	claimContext, cancelClaim := context.WithTimeout(context.Background(), passengerMeaningV1ShadowStoreTimeout)
	claim, err := claimPassengerMeaningV1ShadowSafely(claimContext, store, job.Identity, passengerMeaningV1ShadowLeaseDuration(timeout))
	cancelClaim()
	if err != nil {
		s.logPassengerMeaningV1ShadowStorageFailure("job", "claim_failed", "claim", err)
		return passengerMeaningV1ShadowExecutionResult{}
	}
	switch claim.Status {
	case PassengerMeaningV1ShadowClaimInProgress:
		return passengerMeaningV1ShadowExecutionResult{ClaimStatus: claim.Status, Summary: inProgressPassengerMeaningV1ShadowSummary()}
	case PassengerMeaningV1ShadowClaimCompleted:
		return passengerMeaningV1ShadowExecutionResult{ClaimStatus: claim.Status, Summary: claim.Summary}
	case PassengerMeaningV1ShadowClaimAcquired:
	default:
		s.logPassengerMeaningV1ShadowStorageFailure("job", "claim_failed", "claim", ErrPassengerMeaningV1ShadowClaimNotFound)
		return passengerMeaningV1ShadowExecutionResult{}
	}

	providerContext, cancelProvider := context.WithTimeout(context.Background(), timeout)
	summary := runPassengerMeaningV1ShadowSafely(providerContext, job.Input)
	cancelProvider()
	completionErr := completePassengerMeaningV1ShadowWithRetry(store, job.Identity, summary)
	if completionErr != nil {
		s.logPassengerMeaningV1ShadowStorageFailure("job", "completion_failed", "completion", completionErr)
	}
	return passengerMeaningV1ShadowExecutionResult{
		ClaimStatus:     PassengerMeaningV1ShadowClaimAcquired,
		Summary:         summary,
		CompletionError: completionErr,
	}
}

func recoverExpiredPassengerMeaningV1ShadowClaimsSafely(ctx context.Context, store PassengerMeaningV1ShadowClaimStore, sessionID string, batchSize int) (result PassengerMeaningV1ShadowRecoveryResult, err error) {
	defer func() {
		if recover() != nil {
			result = PassengerMeaningV1ShadowRecoveryResult{}
			err = errPassengerMeaningV1ShadowRecoveryPanic
		}
	}()
	return store.RecoverExpiredPassengerMeaningV1ShadowClaims(ctx, sessionID, batchSize)
}

func completePassengerMeaningV1ShadowWithRetry(store PassengerMeaningV1ShadowClaimStore, identity PassengerMeaningV1ShadowIdentity, summary PassengerMeaningV1ShadowSummary) error {
	var lastErr error
	for attempt := 1; attempt <= passengerMeaningV1ShadowCompletionMaxAttempts; attempt++ {
		completeContext, cancelComplete := context.WithTimeout(context.Background(), passengerMeaningV1ShadowStoreTimeout)
		lastErr = completePassengerMeaningV1ShadowSafely(completeContext, store, identity, summary)
		cancelComplete()
		if lastErr == nil {
			return nil
		}
		if attempt < passengerMeaningV1ShadowCompletionMaxAttempts {
			timer := time.NewTimer(time.Duration(attempt) * passengerMeaningV1ShadowCompletionBackoff)
			<-timer.C
		}
	}
	return lastErr
}

func claimPassengerMeaningV1ShadowSafely(ctx context.Context, store PassengerMeaningV1ShadowClaimStore, identity PassengerMeaningV1ShadowIdentity, leaseDuration time.Duration) (result PassengerMeaningV1ShadowClaimResult, err error) {
	defer func() {
		if recover() != nil {
			result = PassengerMeaningV1ShadowClaimResult{}
			err = errPassengerMeaningV1ShadowClaimPanic
		}
	}()
	return store.ClaimPassengerMeaningV1Shadow(ctx, identity, leaseDuration)
}

func completePassengerMeaningV1ShadowSafely(ctx context.Context, store PassengerMeaningV1ShadowClaimStore, identity PassengerMeaningV1ShadowIdentity, summary PassengerMeaningV1ShadowSummary) (err error) {
	defer func() {
		if recover() != nil {
			err = errPassengerMeaningV1ShadowCompletePanic
		}
	}()
	return store.CompletePassengerMeaningV1Shadow(ctx, identity, summary)
}

func passengerMeaningV1ShadowLeaseDuration(providerTimeout time.Duration) time.Duration {
	if providerTimeout <= 0 {
		providerTimeout = passengerMeaningV1ShadowProviderTimeout
	}
	retryBudget := time.Duration(passengerMeaningV1ShadowCompletionMaxAttempts) * passengerMeaningV1ShadowStoreTimeout
	for attempt := 1; attempt < passengerMeaningV1ShadowCompletionMaxAttempts; attempt++ {
		retryBudget += time.Duration(attempt) * passengerMeaningV1ShadowCompletionBackoff
	}
	return providerTimeout + retryBudget + passengerMeaningV1ShadowLeaseSafetyMargin
}

func passengerMeaningV1ShadowIdentityValid(identity PassengerMeaningV1ShadowIdentity) bool {
	return strings.TrimSpace(identity.SessionID) != "" &&
		strings.TrimSpace(identity.MessageID) != "" &&
		strings.TrimSpace(identity.PromptEventID) != "" &&
		strings.TrimSpace(identity.IdempotencyKey) != "" &&
		identity.ContractVersion == passengerClarificationMeaningV1Version
}

func abandonedPassengerMeaningV1ShadowSummary() PassengerMeaningV1ShadowSummary {
	return PassengerMeaningV1ShadowSummary{
		OpenAI: PassengerMeaningV1ShadowOpenAISummary{
			Status:    string(PassengerMeaningV1ShadowError),
			Version:   passengerClarificationMeaningV1Version,
			ErrorCode: "passenger_meaning_v1_shadow_execution_abandoned",
		},
		Validation: skippedPassengerMeaningV1ShadowValidation("shadow_execution_abandoned"),
	}
}

func inProgressPassengerMeaningV1ShadowSummary() PassengerMeaningV1ShadowSummary {
	return PassengerMeaningV1ShadowSummary{
		OpenAI: PassengerMeaningV1ShadowOpenAISummary{
			Status:  string(PassengerMeaningV1ShadowInProgress),
			Version: passengerClarificationMeaningV1Version,
		},
		Validation: skippedPassengerMeaningV1ShadowValidation("shadow_in_progress"),
	}
}

func runPassengerMeaningV1ShadowSafely(ctx context.Context, input PassengerMeaningV1ShadowInput) (summary PassengerMeaningV1ShadowSummary) {
	defer func() {
		if recover() != nil {
			summary = PassengerMeaningV1ShadowSummary{
				OpenAI: PassengerMeaningV1ShadowOpenAISummary{
					Status:    string(PassengerMeaningV1ShadowError),
					Version:   passengerClarificationMeaningV1Version,
					ErrorCode: "openai_passenger_meaning_v1_panic",
				},
				Validation: skippedPassengerMeaningV1ShadowValidation("openai_error"),
			}
		}
	}()
	return RunPassengerMeaningV1Shadow(ctx, input)
}

func (s *Service) logPassengerMeaningV1ShadowEvent(component string, reason string) {
	if s == nil {
		return
	}
	defer func() { _ = recover() }()
	s.logReprocess(
		"chat passenger_meaning_v1_shadow_%s event=%s reason=%s",
		strings.TrimSpace(component),
		strings.TrimSpace(reason),
		strings.TrimSpace(reason),
	)
}

func (s *Service) logPassengerMeaningV1ShadowStorageFailure(component string, reason string, operation string, err error) {
	errorClass, sqlState, timeout, canceled := travelQueryV2ShadowStorageErrorMetadata(err)
	defer func() { _ = recover() }()
	s.logReprocess(
		"chat passenger_meaning_v1_shadow_%s event=%s reason=%s operation=%s error_class=%s sqlstate=%s timeout=%t canceled=%t",
		strings.TrimSpace(component),
		strings.TrimSpace(reason),
		strings.TrimSpace(reason),
		strings.TrimSpace(operation),
		errorClass,
		sqlState,
		timeout,
		canceled,
	)
}

func (s *Service) logPassengerMeaningV1ShadowRecovery(result PassengerMeaningV1ShadowRecoveryResult) {
	if s == nil {
		return
	}
	defer func() { _ = recover() }()
	s.logReprocess(
		"chat passenger_meaning_v1_shadow_recovery event=sweep_done reason=sweep_done messages=%d claims_completed=%d",
		result.MessagesProcessed,
		result.ClaimsCompleted,
	)
}
