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

type StructuredInterpreterShadowSummary struct {
	Local     StructuredInterpreterSummary   `json:"local"`
	OpenAI    OpenAIInterpreterShadowSummary `json:"openai"`
	Agreement StructuredInterpreterAgreement `json:"agreement"`
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
	LocalInterpretation StructuredInterpretation
	IdempotencyKey      string
}

func RunStructuredInterpreterShadow(ctx context.Context, input StructuredInterpreterShadowInput) StructuredInterpreterShadowSummary {
	summary := StructuredInterpreterShadowSummary{
		Local: buildStructuredInterpreterSummary(input.LocalInterpretation),
		OpenAI: OpenAIInterpreterShadowSummary{
			Status: string(StructuredInterpreterShadowDisabled),
		},
	}

	if !input.Enabled {
		return summary
	}

	if input.OpenAIInterpreter == nil || !input.OpenAIInterpreter.Enabled() {
		summary.OpenAI.Status = string(StructuredInterpreterShadowOpenAIDisabled)
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

	return summary
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
