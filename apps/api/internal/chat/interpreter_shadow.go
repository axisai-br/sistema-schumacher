package chat

import (
	"context"
	"errors"
	"strings"
	"time"
)

const structuredInterpreterShadowKey = "structured_interpreter_shadow"

type StructuredInterpreterShadowStatus string

const (
	StructuredInterpreterShadowDisabled       StructuredInterpreterShadowStatus = "disabled"
	StructuredInterpreterShadowSkipped        StructuredInterpreterShadowStatus = "skipped"
	StructuredInterpreterShadowOpenAIDisabled StructuredInterpreterShadowStatus = "openai_disabled"
	StructuredInterpreterShadowValid          StructuredInterpreterShadowStatus = "valid"
	StructuredInterpreterShadowError          StructuredInterpreterShadowStatus = "error"
)

type StructuredInterpreterShadowValidationStatus string

const (
	StructuredInterpreterShadowValidationSkipped  StructuredInterpreterShadowValidationStatus = "skipped"
	StructuredInterpreterShadowValidationAccepted StructuredInterpreterShadowValidationStatus = "accepted"
	StructuredInterpreterShadowValidationRejected StructuredInterpreterShadowValidationStatus = "rejected"
)

type StructuredInterpreterShadowSummary struct {
	Local            StructuredInterpreterSummary      `json:"local"`
	OpenAI           OpenAIInterpreterShadowSummary    `json:"openai"`
	OpenAIValidation OpenAIInterpreterShadowValidation `json:"openai_validation"`
	Agreement        StructuredInterpreterAgreement    `json:"agreement"`
}

type StructuredInterpreterSummary struct {
	Intent      string  `json:"intent"`
	TurnMeaning string  `json:"turn_meaning"`
	Confidence  float64 `json:"confidence"`
	Source      string  `json:"source"`
}

type OpenAIInterpreterShadowSummary struct {
	Status             string  `json:"status"`
	Intent             string  `json:"intent,omitempty"`
	TurnMeaning        string  `json:"turn_meaning,omitempty"`
	Confidence         float64 `json:"confidence,omitempty"`
	Source             string  `json:"source,omitempty"`
	ProviderResponseID string  `json:"provider_response_id,omitempty"`
	LatencyMs          int64   `json:"latency_ms"`
	ErrorCode          string  `json:"error_code,omitempty"`
}

type OpenAIInterpreterShadowValidation struct {
	Status           string               `json:"status"`
	Accepted         bool                 `json:"accepted"`
	RejectReason     string               `json:"reject_reason"`
	FallbackTemplate ResponseTemplateName `json:"fallback_template"`
}

type StructuredInterpreterAgreement struct {
	Intent              bool `json:"intent"`
	TurnMeaning         bool `json:"turn_meaning"`
	SelectedOptionIndex bool `json:"selected_option_index"`
	PassengerCount      bool `json:"passenger_count"`
	ChildUnder5Count    bool `json:"child_under_5_count"`
	DocumentLikeText    bool `json:"document_like_text"`
	PaymentPreference   bool `json:"payment_preference"`
}

func buildStructuredInterpreterSummary(value StructuredInterpretation) StructuredInterpreterSummary {
	return StructuredInterpreterSummary{
		Intent:      string(value.Intent),
		TurnMeaning: string(value.TurnMeaning),
		Confidence:  value.Confidence,
		Source:      value.Source,
	}
}

func compareStructuredInterpretations(local StructuredInterpretation, remote StructuredInterpretation) StructuredInterpreterAgreement {
	return StructuredInterpreterAgreement{
		Intent:              local.Intent == remote.Intent,
		TurnMeaning:         local.TurnMeaning == remote.TurnMeaning,
		SelectedOptionIndex: compareKnownInt(local.Booking.SelectedOptionIndexKnown, local.Booking.SelectedOptionIndex, remote.Booking.SelectedOptionIndexKnown, remote.Booking.SelectedOptionIndex),
		PassengerCount:      compareKnownInt(local.Booking.PassengerCountKnown, local.Booking.PassengerCount, remote.Booking.PassengerCountKnown, remote.Booking.PassengerCount),
		ChildUnder5Count:    compareKnownInt(local.Booking.ChildUnder5CountKnown, local.Booking.ChildUnder5Count, remote.Booking.ChildUnder5CountKnown, remote.Booking.ChildUnder5Count),
		DocumentLikeText:    local.PassengerDocument.DocumentLikeText == remote.PassengerDocument.DocumentLikeText,
		PaymentPreference:   strings.EqualFold(strings.TrimSpace(local.Payment.PaymentPreference), strings.TrimSpace(remote.Payment.PaymentPreference)),
	}
}

func compareKnownInt(aKnown bool, a int, bKnown bool, b int) bool {
	if aKnown != bKnown {
		return false
	}
	if !aKnown {
		return true
	}
	return a == b
}

type StructuredInterpreterShadowInput struct {
	Enabled             bool
	OpenAIInterpreter   OpenAIStructuredInterpreter
	StructuredInput     StructuredInterpreterInput
	ActivePrompt        ActivePromptContext
	LocalInterpretation StructuredInterpretation
	IdempotencyKey      string
}

func RunStructuredInterpreterShadow(ctx context.Context, input StructuredInterpreterShadowInput) StructuredInterpreterShadowSummary {
	summary := StructuredInterpreterShadowSummary{
		Local: buildStructuredInterpreterSummary(input.LocalInterpretation),
		OpenAI: OpenAIInterpreterShadowSummary{
			Status: string(StructuredInterpreterShadowDisabled),
		},
		OpenAIValidation: skippedOpenAIInterpreterShadowValidation("shadow_disabled"),
	}

	if !input.Enabled {
		return summary
	}

	if input.OpenAIInterpreter == nil || !input.OpenAIInterpreter.Enabled() {
		summary.OpenAI.Status = string(StructuredInterpreterShadowOpenAIDisabled)
		summary.OpenAIValidation = skippedOpenAIInterpreterShadowValidation("openai_disabled")
		return summary
	}

	startedAt := time.Now()
	result, err := input.OpenAIInterpreter.InterpretStructuredTurn(ctx, OpenAIStructuredInterpreterRunInput{
		StructuredInput:     input.StructuredInput,
		LocalInterpretation: input.LocalInterpretation,
		IdempotencyKey:      input.IdempotencyKey,
	})
	latencyMs := time.Since(startedAt).Milliseconds()

	if err != nil {
		summary.OpenAI.Status = string(StructuredInterpreterShadowError)
		summary.OpenAI.ErrorCode = sanitizeOpenAIInterpreterShadowError(err)
		summary.OpenAI.LatencyMs = latencyMs
		if proposal, ok := parseableOpenAIInterpreterShadowInvalidProposal(result, err); ok {
			summary.OpenAI.Intent = string(proposal.Intent)
			summary.OpenAI.TurnMeaning = string(proposal.TurnMeaning)
			summary.OpenAI.Confidence = proposal.Confidence
			summary.OpenAI.Source = proposal.Source
			summary.OpenAI.ProviderResponseID = result.ProviderResponseID
			summary.Agreement = compareStructuredInterpretations(input.LocalInterpretation, proposal)
			summary.OpenAIValidation = validateOpenAIInterpreterShadowProposal(input, proposal)
			return summary
		}
		summary.OpenAIValidation = skippedOpenAIInterpreterShadowValidation(openAIInterpreterShadowValidationSkipReason(err))
		return summary
	}

	summary.OpenAI = OpenAIInterpreterShadowSummary{
		Status:             string(StructuredInterpreterShadowValid),
		Intent:             string(result.Interpretation.Intent),
		TurnMeaning:        string(result.Interpretation.TurnMeaning),
		Confidence:         result.Interpretation.Confidence,
		Source:             result.Interpretation.Source,
		ProviderResponseID: result.ProviderResponseID,
		LatencyMs:          latencyMs,
	}
	summary.Agreement = compareStructuredInterpretations(input.LocalInterpretation, result.Interpretation)
	summary.OpenAIValidation = validateOpenAIInterpreterShadowProposal(input, result.Interpretation)

	return summary
}

func parseableOpenAIInterpreterShadowInvalidProposal(result OpenAIStructuredInterpreterRunResult, err error) (StructuredInterpretation, bool) {
	if !errors.Is(err, ErrOpenAIStructuredInterpreterInvalidOutput) || result.Validation.Valid {
		return StructuredInterpretation{}, false
	}
	proposal := result.Validation.Interpretation
	if !openAIInterpreterShadowProposalLooksParseable(proposal) {
		return StructuredInterpretation{}, false
	}
	return proposal, true
}

func openAIInterpreterShadowProposalLooksParseable(proposal StructuredInterpretation) bool {
	if proposal.Intent == "" || proposal.TurnMeaning == "" {
		return false
	}
	if !openAIStructuredIntentAllowed(proposal.Intent) || !openAITurnMeaningAllowed(proposal.TurnMeaning) {
		return false
	}
	if proposal.Confidence < 0 || proposal.Confidence > 1 {
		return false
	}
	return true
}

func validateOpenAIInterpreterShadowProposal(input StructuredInterpreterShadowInput, proposal StructuredInterpretation) OpenAIInterpreterShadowValidation {
	if proposal.Intent == "" || proposal.TurnMeaning == "" {
		return skippedOpenAIInterpreterShadowValidation("openai_missing_result")
	}

	activePrompt := input.ActivePrompt
	if activePrompt.Kind == "" {
		activePrompt = InferActivePromptContext(input.StructuredInput.History, input.StructuredInput.State)
	}

	result := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  input.StructuredInput.CurrentTurn,
		History:      input.StructuredInput.History,
		State:        input.StructuredInput.State,
		ActivePrompt: activePrompt,
		ObservedAt:   input.StructuredInput.ObservedAt,
	})
	if result.Accepted {
		return OpenAIInterpreterShadowValidation{
			Status:   string(StructuredInterpreterShadowValidationAccepted),
			Accepted: true,
		}
	}
	return OpenAIInterpreterShadowValidation{
		Status:           string(StructuredInterpreterShadowValidationRejected),
		Accepted:         false,
		RejectReason:     result.RejectReason,
		FallbackTemplate: result.FallbackTemplate,
	}
}

func skippedOpenAIInterpreterShadowValidation(reason string) OpenAIInterpreterShadowValidation {
	return OpenAIInterpreterShadowValidation{
		Status:       string(StructuredInterpreterShadowValidationSkipped),
		Accepted:     false,
		RejectReason: strings.TrimSpace(reason),
	}
}

func openAIInterpreterShadowValidationSkipReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrOpenAIStructuredInterpreterDisabled):
		return "openai_disabled"
	case errors.Is(err, ErrOpenAIStructuredInterpreterInvalidOutput):
		return "openai_schema_invalid"
	case errors.Is(err, ErrOpenAIStructuredInterpreterEmptyOutput):
		return "openai_missing_result"
	default:
		return "openai_error"
	}
}

func sanitizeOpenAIInterpreterShadowError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrOpenAIStructuredInterpreterDisabled):
		return "openai_structured_interpreter_disabled"
	case errors.Is(err, ErrOpenAIStructuredInterpreterRequestFailed):
		return "openai_structured_interpreter_request_failed"
	case errors.Is(err, ErrOpenAIStructuredInterpreterEmptyOutput):
		return "openai_structured_interpreter_empty_output"
	case errors.Is(err, ErrOpenAIStructuredInterpreterInvalidOutput):
		return "openai_structured_interpreter_invalid_output"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline_exceeded"
	default:
		return "openai_structured_interpreter_error"
	}
}
