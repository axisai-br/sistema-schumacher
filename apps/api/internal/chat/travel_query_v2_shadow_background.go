package chat

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	travelQueryV2ShadowClaimInProgress  = "IN_PROGRESS"
	travelQueryV2ShadowClaimCompleted   = "COMPLETED"
	travelQueryV2ShadowClaimsPayloadKey = "travel_query_v2_shadow_claims"

	travelQueryV2ShadowProviderTimeout = 10 * time.Second
	travelQueryV2ShadowStoreTimeout    = 2 * time.Second
	travelQueryV2ShadowMaxConcurrent   = 4

	travelQueryV2ShadowCompletionMaxAttempts = 3
	travelQueryV2ShadowCompletionBackoff     = 10 * time.Millisecond
	travelQueryV2ShadowLeaseSafetyMargin     = 5 * time.Second

	travelQueryV2ShadowRecoveryBatchSize      = 50
	travelQueryV2ShadowRecoveryInterval       = 30 * time.Second
	travelQueryV2ShadowRecoverySweepTimeout   = 5 * time.Second
	travelQueryV2ShadowLegacyLeaseRepairGrace = time.Minute
)

var (
	ErrTravelQueryV2ShadowClaimNotFound = errors.New("travel query v2 shadow claim not found")
	errTravelQueryV2ShadowRecoveryPanic = errors.New("travel query v2 shadow recovery panic")
)

type TravelQueryV2ShadowClaimStatus string

const (
	TravelQueryV2ShadowClaimAcquired   TravelQueryV2ShadowClaimStatus = "ACQUIRED"
	TravelQueryV2ShadowClaimInProgress TravelQueryV2ShadowClaimStatus = TravelQueryV2ShadowClaimStatus(travelQueryV2ShadowClaimInProgress)
	TravelQueryV2ShadowClaimCompleted  TravelQueryV2ShadowClaimStatus = TravelQueryV2ShadowClaimStatus(travelQueryV2ShadowClaimCompleted)
)

type TravelQueryV2ShadowClaimResult struct {
	Status  TravelQueryV2ShadowClaimStatus
	Summary TravelQueryV2ShadowSummary
}

type travelQueryV2ShadowExecutionResult struct {
	ClaimStatus     TravelQueryV2ShadowClaimStatus
	Summary         TravelQueryV2ShadowSummary
	CompletionError error
}

type travelQueryV2ShadowClaimRecord struct {
	Status                  string                      `json:"status"`
	IdempotencyKey          string                      `json:"idempotency_key"`
	ClaimedAt               string                      `json:"claimed_at,omitempty"`
	LeaseExpiresAt          string                      `json:"lease_expires_at,omitempty"`
	LeaseExpiresAtUnixMilli int64                       `json:"lease_expires_at_unix_ms,omitempty"`
	CompletedAt             string                      `json:"completed_at,omitempty"`
	Summary                 *TravelQueryV2ShadowSummary `json:"summary,omitempty"`
}

type TravelQueryV2ShadowClaimStore interface {
	ClaimTravelQueryV2Shadow(ctx context.Context, sessionID string, messageID string, idempotencyKey string, leaseDuration time.Duration) (TravelQueryV2ShadowClaimResult, error)
	CompleteTravelQueryV2Shadow(ctx context.Context, sessionID string, messageID string, idempotencyKey string, summary TravelQueryV2ShadowSummary) error
}

type TravelQueryV2ShadowRecoveryResult struct {
	MessagesProcessed   int
	ClaimsCompleted     int
	ClaimsLeaseRepaired int
}

type TravelQueryV2ShadowRecoveryStore interface {
	RecoverExpiredTravelQueryV2ShadowClaims(ctx context.Context, batchSize int) (TravelQueryV2ShadowRecoveryResult, error)
}

type travelQueryV2ShadowRecoveryLoopConfig struct {
	BatchSize    int
	Interval     time.Duration
	SweepTimeout time.Duration
}

type travelQueryV2ShadowBackgroundJob struct {
	SessionID      string
	MessageID      string
	IdempotencyKey string
	Input          TravelQueryV2ShadowInput
	Timeout        time.Duration
}

var travelQueryV2ShadowBackgroundSlots = make(chan struct{}, travelQueryV2ShadowMaxConcurrent)

func (s *Service) StartTravelQueryV2ShadowRecoveryLoop(ctx context.Context) <-chan struct{} {
	return s.startTravelQueryV2ShadowRecoveryLoop(ctx, travelQueryV2ShadowRecoveryLoopConfig{
		BatchSize:    travelQueryV2ShadowRecoveryBatchSize,
		Interval:     travelQueryV2ShadowRecoveryInterval,
		SweepTimeout: travelQueryV2ShadowRecoverySweepTimeout,
	})
}

func (s *Service) startTravelQueryV2ShadowRecoveryLoop(ctx context.Context, loopConfig travelQueryV2ShadowRecoveryLoopConfig) <-chan struct{} {
	done := make(chan struct{})
	if s == nil || ctx == nil {
		close(done)
		return done
	}
	store, ok := s.store.(TravelQueryV2ShadowRecoveryStore)
	if !ok {
		close(done)
		return done
	}
	if loopConfig.BatchSize <= 0 {
		loopConfig.BatchSize = travelQueryV2ShadowRecoveryBatchSize
	}
	if loopConfig.Interval <= 0 {
		loopConfig.Interval = travelQueryV2ShadowRecoveryInterval
	}
	if loopConfig.SweepTimeout <= 0 {
		loopConfig.SweepTimeout = travelQueryV2ShadowRecoverySweepTimeout
	}
	go func() {
		defer close(done)
		s.runTravelQueryV2ShadowRecoveryLoop(ctx, store, loopConfig)
	}()
	return done
}

func (s *Service) runTravelQueryV2ShadowRecoveryLoop(ctx context.Context, store TravelQueryV2ShadowRecoveryStore, loopConfig travelQueryV2ShadowRecoveryLoopConfig) {
	if ctx.Err() != nil {
		return
	}
	s.runTravelQueryV2ShadowRecoverySweep(ctx, store, loopConfig)
	ticker := time.NewTicker(loopConfig.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTravelQueryV2ShadowRecoverySweep(ctx, store, loopConfig)
		}
	}
}

func (s *Service) runTravelQueryV2ShadowRecoverySweep(ctx context.Context, store TravelQueryV2ShadowRecoveryStore, loopConfig travelQueryV2ShadowRecoveryLoopConfig) {
	defer func() {
		if recover() != nil {
			s.logTravelQueryV2ShadowRecovery("chat travel_v2_shadow_recovery event=sweep_failed")
		}
	}()
	sweepContext, cancelSweep := context.WithTimeout(ctx, loopConfig.SweepTimeout)
	result, err := recoverExpiredTravelQueryV2ShadowClaimsSafely(sweepContext, store, loopConfig.BatchSize)
	cancelSweep()
	if err != nil {
		s.logTravelQueryV2ShadowRecovery("chat travel_v2_shadow_recovery event=sweep_failed")
		return
	}
	if result.MessagesProcessed > 0 {
		s.logTravelQueryV2ShadowRecovery(
			"chat travel_v2_shadow_recovery event=sweep_done messages=%d claims_completed=%d claims_lease_repaired=%d",
			result.MessagesProcessed,
			result.ClaimsCompleted,
			result.ClaimsLeaseRepaired,
		)
	}
}

func (s *Service) logTravelQueryV2ShadowRecovery(format string, values ...interface{}) {
	defer func() {
		_ = recover()
	}()
	s.logReprocess(format, values...)
}

func recoverExpiredTravelQueryV2ShadowClaimsSafely(ctx context.Context, store TravelQueryV2ShadowRecoveryStore, batchSize int) (result TravelQueryV2ShadowRecoveryResult, err error) {
	defer func() {
		if recover() != nil {
			result = TravelQueryV2ShadowRecoveryResult{}
			err = errTravelQueryV2ShadowRecoveryPanic
		}
	}()
	return store.RecoverExpiredTravelQueryV2ShadowClaims(ctx, batchSize)
}

func (s *Service) scheduleTravelQueryV2Shadow(job travelQueryV2ShadowBackgroundJob) bool {
	if s == nil || !s.cfg.ChatOpenAITravelV2ShadowEnabled || strings.TrimSpace(job.IdempotencyKey) == "" {
		return false
	}
	if _, ok := s.store.(TravelQueryV2ShadowClaimStore); !ok {
		return false
	}
	select {
	case travelQueryV2ShadowBackgroundSlots <- struct{}{}:
	default:
		return false
	}

	go func() {
		defer func() {
			<-travelQueryV2ShadowBackgroundSlots
			if recover() != nil {
				// The shadow is fail-open and never propagates a background panic
				// into the response path.
			}
		}()
		s.executeTravelQueryV2ShadowJob(job)
	}()
	return true
}

func (s *Service) executeTravelQueryV2ShadowJob(job travelQueryV2ShadowBackgroundJob) travelQueryV2ShadowExecutionResult {
	store, ok := s.store.(TravelQueryV2ShadowClaimStore)
	if !ok {
		return travelQueryV2ShadowExecutionResult{}
	}
	timeout := job.Timeout
	if timeout <= 0 {
		timeout = travelQueryV2ShadowProviderTimeout
	}
	claimContext, cancelClaim := context.WithTimeout(context.Background(), travelQueryV2ShadowStoreTimeout)
	claim, err := store.ClaimTravelQueryV2Shadow(
		claimContext,
		strings.TrimSpace(job.SessionID),
		strings.TrimSpace(job.MessageID),
		strings.TrimSpace(job.IdempotencyKey),
		travelQueryV2ShadowLeaseDuration(timeout),
	)
	cancelClaim()
	if err != nil {
		return travelQueryV2ShadowExecutionResult{}
	}
	if claim.Status == TravelQueryV2ShadowClaimInProgress {
		return travelQueryV2ShadowExecutionResult{
			ClaimStatus: claim.Status,
			Summary: TravelQueryV2ShadowSummary{
				OpenAI:     TravelQueryV2ShadowOpenAISummary{Status: string(TravelQueryV2ShadowInProgress)},
				Validation: skippedTravelQueryV2ShadowValidation("shadow_in_progress"),
			},
		}
	}
	if claim.Status == TravelQueryV2ShadowClaimCompleted {
		return travelQueryV2ShadowExecutionResult{ClaimStatus: claim.Status, Summary: claim.Summary}
	}
	if claim.Status != TravelQueryV2ShadowClaimAcquired {
		return travelQueryV2ShadowExecutionResult{}
	}

	providerContext, cancelProvider := context.WithTimeout(context.Background(), timeout)
	summary := runTravelQueryV2ShadowSafely(providerContext, job.Input)
	cancelProvider()

	completionErr := completeTravelQueryV2ShadowWithRetry(store, job, summary)
	return travelQueryV2ShadowExecutionResult{
		ClaimStatus:     TravelQueryV2ShadowClaimAcquired,
		Summary:         summary,
		CompletionError: completionErr,
	}
}

func completeTravelQueryV2ShadowWithRetry(store TravelQueryV2ShadowClaimStore, job travelQueryV2ShadowBackgroundJob, summary TravelQueryV2ShadowSummary) error {
	var lastErr error
	for attempt := 1; attempt <= travelQueryV2ShadowCompletionMaxAttempts; attempt++ {
		completeContext, cancelComplete := context.WithTimeout(context.Background(), travelQueryV2ShadowStoreTimeout)
		lastErr = store.CompleteTravelQueryV2Shadow(
			completeContext,
			strings.TrimSpace(job.SessionID),
			strings.TrimSpace(job.MessageID),
			strings.TrimSpace(job.IdempotencyKey),
			summary,
		)
		cancelComplete()
		if lastErr == nil {
			return nil
		}
		if attempt == travelQueryV2ShadowCompletionMaxAttempts {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * travelQueryV2ShadowCompletionBackoff)
		<-timer.C
	}
	return lastErr
}

func travelQueryV2ShadowLeaseDuration(providerTimeout time.Duration) time.Duration {
	if providerTimeout <= 0 {
		providerTimeout = travelQueryV2ShadowProviderTimeout
	}
	retryBudget := time.Duration(travelQueryV2ShadowCompletionMaxAttempts) * travelQueryV2ShadowStoreTimeout
	for attempt := 1; attempt < travelQueryV2ShadowCompletionMaxAttempts; attempt++ {
		retryBudget += time.Duration(attempt) * travelQueryV2ShadowCompletionBackoff
	}
	return providerTimeout + retryBudget + travelQueryV2ShadowLeaseSafetyMargin
}

func abandonedTravelQueryV2ShadowSummary() TravelQueryV2ShadowSummary {
	return TravelQueryV2ShadowSummary{
		OpenAI: TravelQueryV2ShadowOpenAISummary{
			Status:    string(TravelQueryV2ShadowError),
			ErrorCode: "travel_query_v2_shadow_execution_abandoned",
		},
		Validation: skippedTravelQueryV2ShadowValidation("shadow_execution_abandoned"),
	}
}

func runTravelQueryV2ShadowSafely(ctx context.Context, input TravelQueryV2ShadowInput) (summary TravelQueryV2ShadowSummary) {
	defer func() {
		if recover() == nil {
			return
		}
		summary = TravelQueryV2ShadowSummary{
			OpenAI: TravelQueryV2ShadowOpenAISummary{
				Status:    string(TravelQueryV2ShadowError),
				ErrorCode: "openai_travel_query_v2_panic",
			},
			Validation: skippedTravelQueryV2ShadowValidation("openai_error"),
		}
	}()
	return RunTravelQueryV2Shadow(ctx, input)
}
