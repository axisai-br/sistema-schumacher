package chat

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
)

type PassengerMeaningV1ShadowStatus string

const (
	PassengerMeaningV1ShadowDisabled       PassengerMeaningV1ShadowStatus = "disabled"
	PassengerMeaningV1ShadowOpenAIDisabled PassengerMeaningV1ShadowStatus = "openai_disabled"
	PassengerMeaningV1ShadowInProgress     PassengerMeaningV1ShadowStatus = "in_progress"
	PassengerMeaningV1ShadowValid          PassengerMeaningV1ShadowStatus = "valid"
	PassengerMeaningV1ShadowError          PassengerMeaningV1ShadowStatus = "error"
	PassengerMeaningV1ShadowRefused        PassengerMeaningV1ShadowStatus = "refused"
)

type PassengerMeaningV1ShadowValidationStatus string

const (
	PassengerMeaningV1ShadowValidationSkipped  PassengerMeaningV1ShadowValidationStatus = "skipped"
	PassengerMeaningV1ShadowValidationAccepted PassengerMeaningV1ShadowValidationStatus = "accepted"
	PassengerMeaningV1ShadowValidationRejected PassengerMeaningV1ShadowValidationStatus = "rejected"
)

type PassengerMeaningV1ShadowSummary struct {
	OpenAI     PassengerMeaningV1ShadowOpenAISummary `json:"openai"`
	Validation PassengerMeaningV1ShadowValidation    `json:"validation"`
	Metrics    PassengerMeaningV1ShadowMetrics       `json:"metrics"`
}

type PassengerMeaningV1ShadowOpenAISummary struct {
	Status              string   `json:"status"`
	Version             int      `json:"version"`
	PassengerStatus     string   `json:"passenger_status,omitempty"`
	PassengerCount      *int     `json:"passenger_count,omitempty"`
	PassengerProvenance string   `json:"passenger_provenance,omitempty"`
	ChildUnder5Status   string   `json:"child_under_5_status,omitempty"`
	ChildUnder5Count    *int     `json:"child_under_5_count,omitempty"`
	ChildReferenceCount int      `json:"child_reference_count"`
	CorrectionPresent   bool     `json:"correction_present"`
	CorrectionTarget    string   `json:"correction_target,omitempty"`
	NeedsClarification  bool     `json:"needs_clarification"`
	MissingFields       []string `json:"missing_fields,omitempty"`
	Confidence          float64  `json:"confidence"`
	ReasonCodes         []string `json:"reason_codes,omitempty"`
	ProviderResponseID  string   `json:"provider_response_id,omitempty"`
	LatencyMs           int64    `json:"latency_ms"`
	ErrorCode           string   `json:"error_code,omitempty"`
}

type PassengerMeaningV1ShadowValidation struct {
	Status      string   `json:"status"`
	Accepted    bool     `json:"accepted"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}

type PassengerMeaningV1ShadowMetrics struct {
	SchemaParseSuccessCount      int `json:"schema_parse_success_count"`
	ValidatorAcceptanceCount     int `json:"validator_acceptance_count"`
	CriticalActionViolationCount int `json:"critical_action_violation_count"`
	StateMutationCount           int `json:"state_mutation_count"`
	ToolCallCount                int `json:"tool_call_count"`
}

type PassengerMeaningV1ShadowInput struct {
	Enabled             bool
	OpenAIInterpreter   OpenAIPassengerMeaningV1Interpreter
	CurrentTurn         string
	State               PassengerClarificationStateV1
	PromptEvent         PassengerClarificationEventV1
	SourceMessageID     string
	SourcePromptEventID string
	IdempotencyKey      string
}

func RunPassengerMeaningV1Shadow(ctx context.Context, input PassengerMeaningV1ShadowInput) PassengerMeaningV1ShadowSummary {
	summary := PassengerMeaningV1ShadowSummary{
		OpenAI:     PassengerMeaningV1ShadowOpenAISummary{Status: string(PassengerMeaningV1ShadowDisabled), Version: passengerClarificationMeaningV1Version},
		Validation: skippedPassengerMeaningV1ShadowValidation("shadow_disabled"),
	}
	if !input.Enabled {
		return summary
	}
	if input.OpenAIInterpreter == nil || !input.OpenAIInterpreter.Enabled() {
		summary.OpenAI.Status = string(PassengerMeaningV1ShadowOpenAIDisabled)
		summary.Validation = skippedPassengerMeaningV1ShadowValidation("openai_disabled")
		return summary
	}

	stateSnapshot := clonePassengerMeaningV1State(input.State)
	promptSnapshot := clonePassengerMeaningV1Prompt(input.PromptEvent)
	startedAt := time.Now()
	result, err := input.OpenAIInterpreter.InterpretPassengerMeaningV1(ctx, OpenAIPassengerMeaningV1RunInput{
		CurrentTurn:         input.CurrentTurn,
		State:               stateSnapshot,
		PromptEvent:         promptSnapshot,
		SourceMessageID:     strings.TrimSpace(input.SourceMessageID),
		SourcePromptEventID: strings.TrimSpace(input.SourcePromptEventID),
		IdempotencyKey:      strings.TrimSpace(input.IdempotencyKey),
	})
	latencyMs := time.Since(startedAt).Milliseconds()

	if result.ProposalParseable {
		summary.Metrics.SchemaParseSuccessCount = 1
		summary.OpenAI = summarizePassengerMeaningV1ShadowProposal(result.Proposal)
		summary.OpenAI.ProviderResponseID = sanitizeTravelQueryV2ShadowProviderID(result.ProviderResponseID)
		summary.OpenAI.LatencyMs = latencyMs
		summary.Validation = validatePassengerMeaningV1ShadowProposal(input, result.Proposal, result.SchemaValid && err == nil)
		if summary.Validation.Accepted {
			summary.Metrics.ValidatorAcceptanceCount = 1
		}
	}
	if err != nil {
		summary.OpenAI.Status = string(PassengerMeaningV1ShadowError)
		if errors.Is(err, ErrOpenAIPassengerMeaningV1Refusal) {
			summary.OpenAI.Status = string(PassengerMeaningV1ShadowRefused)
		}
		summary.OpenAI.ErrorCode = sanitizePassengerMeaningV1ShadowError(err)
		summary.OpenAI.LatencyMs = latencyMs
		if !result.ProposalParseable {
			summary.Validation = skippedPassengerMeaningV1ShadowValidation(passengerMeaningV1ShadowValidationSkipReason(err))
		}
		return summary
	}

	summary.OpenAI = summarizePassengerMeaningV1ShadowProposal(result.Proposal)
	summary.OpenAI.Status = string(PassengerMeaningV1ShadowValid)
	summary.OpenAI.ProviderResponseID = sanitizeTravelQueryV2ShadowProviderID(result.ProviderResponseID)
	summary.OpenAI.LatencyMs = latencyMs
	summary.Validation = validatePassengerMeaningV1ShadowProposal(input, result.Proposal, result.SchemaValid)
	if summary.Validation.Accepted {
		summary.Metrics.ValidatorAcceptanceCount = 1
		_ = MapPassengerClarificationMeaningV1(result.Proposal)
	}
	return summary
}

func validatePassengerMeaningV1ShadowProposal(input PassengerMeaningV1ShadowInput, proposal PassengerClarificationMeaningV1, schemaValid bool) PassengerMeaningV1ShadowValidation {
	result := ValidatePassengerClarificationMeaningV1(PassengerClarificationMeaningV1ValidationInput{
		Proposal:                    proposal,
		State:                       clonePassengerMeaningV1State(input.State),
		PromptEvent:                 clonePassengerMeaningV1Prompt(input.PromptEvent),
		ExpectedSourceMessageID:     strings.TrimSpace(input.SourceMessageID),
		ExpectedSourcePromptEventID: strings.TrimSpace(input.SourcePromptEventID),
	})
	if schemaValid && result.Accepted() {
		return PassengerMeaningV1ShadowValidation{
			Status:   string(PassengerMeaningV1ShadowValidationAccepted),
			Accepted: true,
		}
	}
	reasons := make([]string, 0, len(result.ReasonCodes)+1)
	if !schemaValid {
		reasons = append(reasons, "openai_schema_invalid")
	}
	reasons = append(reasons, result.ReasonCodes...)
	return PassengerMeaningV1ShadowValidation{
		Status:      string(PassengerMeaningV1ShadowValidationRejected),
		ReasonCodes: dedupePassengerMeaningV1Strings(reasons),
	}
}

func skippedPassengerMeaningV1ShadowValidation(reason string) PassengerMeaningV1ShadowValidation {
	validation := PassengerMeaningV1ShadowValidation{Status: string(PassengerMeaningV1ShadowValidationSkipped)}
	if reason = strings.TrimSpace(reason); reason != "" {
		validation.ReasonCodes = []string{reason}
	}
	return validation
}

func summarizePassengerMeaningV1ShadowProposal(proposal PassengerClarificationMeaningV1) PassengerMeaningV1ShadowOpenAISummary {
	return PassengerMeaningV1ShadowOpenAISummary{
		Version:             sanitizePassengerMeaningV1Version(proposal.Version),
		PassengerStatus:     sanitizePassengerMeaningV1Status(proposal.PassengerCount.Status),
		PassengerCount:      sanitizePassengerMeaningV1Count(proposal.PassengerCount.Value, false),
		PassengerProvenance: sanitizePassengerMeaningV1Provenance(proposal.PassengerCount.Provenance),
		ChildUnder5Status:   sanitizePassengerMeaningV1Status(proposal.ChildUnder5.Status),
		ChildUnder5Count:    sanitizePassengerMeaningV1Count(proposal.ChildUnder5.Count, true),
		ChildReferenceCount: passengerMeaningV1MinInt(len(proposal.ChildUnder5.References), 16),
		CorrectionPresent:   proposal.Correction.Present,
		CorrectionTarget:    sanitizePassengerMeaningV1Correction(proposal.Correction.Replaces),
		NeedsClarification:  proposal.NeedsClarification,
		MissingFields:       sanitizePassengerMeaningV1MissingFields(proposal.MissingFields),
		Confidence:          sanitizePassengerMeaningV1Confidence(proposal.Confidence),
		ReasonCodes:         sanitizePassengerMeaningV1ReasonCodes(proposal.ReasonCodes),
	}
}

func sanitizePassengerMeaningV1Version(version int) int {
	if version == passengerClarificationMeaningV1Version {
		return version
	}
	return 0
}

func sanitizePassengerMeaningV1Status(status PassengerMeaningStatusV1) string {
	if passengerMeaningStatusAllowedV1(status) {
		return string(status)
	}
	return "__redacted_invalid"
}

func sanitizePassengerMeaningV1Count(value *int, zeroAllowed bool) *int {
	if value == nil || *value > 99 || *value < 0 || (!zeroAllowed && *value == 0) {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func sanitizePassengerMeaningV1Provenance(value PassengerCountProvenance) string {
	switch value {
	case PassengerCountProvenanceUnknown,
		PassengerCountProvenanceSoloSpeaker,
		PassengerCountProvenanceAbsoluteTotal,
		PassengerCountProvenanceIncludesSpeakerComposition,
		PassengerCountProvenanceSubgroupOnly:
		return string(value)
	default:
		return "__redacted_invalid"
	}
}

func sanitizePassengerMeaningV1Correction(value PassengerMeaningCorrectionTargetV1) string {
	if passengerMeaningCorrectionTargetAllowedV1(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizePassengerMeaningV1MissingFields(values []PassengerMeaningMissingFieldV1) []string {
	seen := map[PassengerMeaningMissingFieldV1]struct{}{}
	out := make([]string, 0, passengerMeaningV1MinInt(len(values), 4))
	for _, value := range values {
		if !passengerMeaningMissingFieldAllowedV1(value) {
			value = "__redacted_invalid"
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, string(value))
		if len(out) == 4 {
			break
		}
	}
	return out
}

func sanitizePassengerMeaningV1ReasonCodes(values []PassengerMeaningReasonCodeV1) []string {
	seen := map[PassengerMeaningReasonCodeV1]struct{}{}
	out := make([]string, 0, passengerMeaningV1MinInt(len(values), 16))
	for _, value := range values {
		if !passengerMeaningReasonCodeAllowedV1(value) {
			value = "__redacted_invalid"
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, string(value))
		if len(out) == 16 {
			break
		}
	}
	return out
}

func sanitizePassengerMeaningV1Confidence(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return 0
	}
	return value
}

func sanitizePassengerMeaningV1ShadowError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrOpenAIPassengerMeaningV1Disabled):
		return "openai_passenger_meaning_v1_disabled"
	case errors.Is(err, ErrOpenAIPassengerMeaningV1RequestFailed):
		return "openai_passenger_meaning_v1_request_failed"
	case errors.Is(err, ErrOpenAIPassengerMeaningV1EmptyOutput):
		return "openai_passenger_meaning_v1_empty_output"
	case errors.Is(err, ErrOpenAIPassengerMeaningV1InvalidOutput):
		return "openai_passenger_meaning_v1_invalid_output"
	case errors.Is(err, ErrOpenAIPassengerMeaningV1Refusal):
		return "openai_passenger_meaning_v1_refusal"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline_exceeded"
	default:
		return "openai_passenger_meaning_v1_error"
	}
}

func passengerMeaningV1ShadowValidationSkipReason(err error) string {
	switch {
	case errors.Is(err, ErrOpenAIPassengerMeaningV1Disabled):
		return "openai_disabled"
	case errors.Is(err, ErrOpenAIPassengerMeaningV1InvalidOutput):
		return "openai_schema_invalid"
	case errors.Is(err, ErrOpenAIPassengerMeaningV1EmptyOutput):
		return "openai_missing_result"
	case errors.Is(err, ErrOpenAIPassengerMeaningV1Refusal):
		return "openai_refusal"
	default:
		return "openai_error"
	}
}

func clonePassengerMeaningV1State(state PassengerClarificationStateV1) PassengerClarificationStateV1 {
	cloned := state
	cloned.PassengerReasonCodes = append([]string(nil), state.PassengerReasonCodes...)
	cloned.ChildReasonCodes = append([]string(nil), state.ChildReasonCodes...)
	cloned.ChildReferences = append([]PassengerClarificationChildReferenceV1(nil), state.ChildReferences...)
	cloned.AppliedEventIDs = append([]string(nil), state.AppliedEventIDs...)
	cloned.AppliedMessageIDs = append([]string(nil), state.AppliedMessageIDs...)
	return cloned
}

func clonePassengerMeaningV1Prompt(prompt PassengerClarificationEventV1) PassengerClarificationEventV1 {
	cloned := prompt
	cloned.ChildReferences = append([]PassengerClarificationChildReferenceV1(nil), prompt.ChildReferences...)
	return cloned
}

func passengerMeaningV1MinInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
