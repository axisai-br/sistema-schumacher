package chat

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	openAIInterpreterAssistMetadataKey         = "openai_interpreter_assist"
	openAIInterpreterAssistConfidenceThreshold = 0.70
)

type OpenAIInterpreterAssistStatus string

const (
	OpenAIInterpreterAssistAccepted OpenAIInterpreterAssistStatus = "accepted"
	OpenAIInterpreterAssistRejected OpenAIInterpreterAssistStatus = "rejected"
	OpenAIInterpreterAssistSkipped  OpenAIInterpreterAssistStatus = "skipped"
)

type OpenAIInterpreterAssistValidationStatus string

const (
	OpenAIInterpreterAssistValidationAccepted OpenAIInterpreterAssistValidationStatus = "accepted"
	OpenAIInterpreterAssistValidationRejected OpenAIInterpreterAssistValidationStatus = "rejected"
	OpenAIInterpreterAssistValidationSkipped  OpenAIInterpreterAssistValidationStatus = "skipped"
)

type OpenAIInterpreterAssistInput struct {
	Enabled                     bool
	OpenAIInterpreter           OpenAIStructuredInterpreter
	ReusableOpenAIResult        *OpenAIInterpreterReusableResult
	StructuredInput             StructuredInterpreterInput
	ActivePrompt                ActivePromptContext
	LocalInterpretation         StructuredInterpretation
	DeterministicDecision       IntentDecision
	DeterministicToolHandled    bool
	DeterministicBookingHandled bool
	DocumentCollectionMediaTurn bool
	IdempotencyKey              string
}

type OpenAIInterpreterAssistDecision struct {
	Enabled              bool
	Considered           bool
	Status               OpenAIInterpreterAssistStatus
	LocalIntent          StructuredIntent
	LocalConfidence      float64
	OpenAIIntent         StructuredIntent
	OpenAIConfidence     float64
	OpenAIInterpretation StructuredInterpretation
	ValidationStatus     OpenAIInterpreterAssistValidationStatus
	ValidationResult     InterpretationValidationResult
	Accepted             bool
	RejectReason         string
	FallbackTemplate     ResponseTemplateName
	DecisionSource       string
	IntentDecision       IntentDecision
	ProviderResponseID   string
	ReasonCodes          []string
}

func shouldConsiderOpenAIInterpreterAssist(local StructuredInterpretation, deterministicDecision IntentDecision, state CanonicalConversationState, activePrompt ActivePromptContext) bool {
	_ = state
	_ = activePrompt
	if deterministicDecision.Intent != "" && deterministicDecision.Intent != IntentUnknown {
		return false
	}
	if local.Intent == StructuredIntentUnknown {
		return true
	}
	return local.Confidence > 0 && local.Confidence < openAIInterpreterAssistConfidenceThreshold
}

func RunOpenAIInterpreterRuntimeAssist(ctx context.Context, input OpenAIInterpreterAssistInput) OpenAIInterpreterAssistDecision {
	decision := OpenAIInterpreterAssistDecision{
		Enabled:          input.Enabled,
		Status:           OpenAIInterpreterAssistSkipped,
		LocalIntent:      input.LocalInterpretation.Intent,
		LocalConfidence:  input.LocalInterpretation.Confidence,
		ValidationStatus: OpenAIInterpreterAssistValidationSkipped,
		DecisionSource:   "local",
	}

	if !input.Enabled {
		decision.ReasonCodes = []string{"assist_disabled"}
		decision.RejectReason = "assist_disabled"
		return decision
	}
	if input.DeterministicToolHandled {
		decision.ReasonCodes = []string{"deterministic_tool_handled"}
		decision.RejectReason = "deterministic_tool_handled"
		return decision
	}
	if input.DeterministicBookingHandled {
		decision.ReasonCodes = []string{"deterministic_booking_handled"}
		decision.RejectReason = "deterministic_booking_handled"
		return decision
	}
	if input.DocumentCollectionMediaTurn {
		decision.ReasonCodes = []string{"document_collection_media_turn"}
		decision.RejectReason = "document_collection_media_turn"
		return decision
	}
	if !shouldConsiderOpenAIInterpreterAssist(input.LocalInterpretation, input.DeterministicDecision, input.StructuredInput.State, input.ActivePrompt) {
		decision.ReasonCodes = []string{"local_result_not_eligible"}
		decision.RejectReason = "local_result_not_eligible"
		return decision
	}

	decision.Considered = true

	var result OpenAIStructuredInterpreterRunResult
	var err error
	if input.ReusableOpenAIResult != nil {
		result = input.ReusableOpenAIResult.Result
		err = input.ReusableOpenAIResult.Err
	} else {
		if input.OpenAIInterpreter == nil || !input.OpenAIInterpreter.Enabled() {
			decision.ReasonCodes = []string{"openai_disabled"}
			decision.RejectReason = "openai_disabled"
			return decision
		}
		result, err = input.OpenAIInterpreter.InterpretStructuredTurn(ctx, OpenAIStructuredInterpreterRunInput{
			StructuredInput:     input.StructuredInput,
			LocalInterpretation: input.LocalInterpretation,
			IdempotencyKey:      input.IdempotencyKey,
		})
	}
	if err != nil {
		if proposal, ok := parseableOpenAIInterpreterShadowInvalidProposal(result, err); ok {
			return evaluateOpenAIInterpreterAssistProposal(input, decision, proposal, result.ProviderResponseID)
		}
		decision.ReasonCodes = []string{openAIInterpreterAssistSkipReason(err)}
		decision.RejectReason = openAIInterpreterAssistSkipReason(err)
		return decision
	}
	if result.Interpretation.Intent == "" || result.Interpretation.TurnMeaning == "" {
		decision.ReasonCodes = []string{"openai_missing_result"}
		decision.RejectReason = "openai_missing_result"
		return decision
	}

	return evaluateOpenAIInterpreterAssistProposal(input, decision, result.Interpretation, result.ProviderResponseID)
}

func evaluateOpenAIInterpreterAssistProposal(input OpenAIInterpreterAssistInput, base OpenAIInterpreterAssistDecision, proposal StructuredInterpretation, providerResponseID string) OpenAIInterpreterAssistDecision {
	base.OpenAIInterpretation = proposal
	base.OpenAIIntent = proposal.Intent
	base.OpenAIConfidence = proposal.Confidence
	base.ProviderResponseID = strings.TrimSpace(providerResponseID)
	base.DecisionSource = "openai_interpreter_assist"

	if proposal.Confidence < openAIInterpreterAssistConfidenceThreshold {
		base.Status = OpenAIInterpreterAssistRejected
		base.ValidationStatus = OpenAIInterpreterAssistValidationSkipped
		base.RejectReason = "openai_confidence_below_threshold"
		base.FallbackTemplate = ""
		base.ReasonCodes = []string{base.RejectReason}
		return base
	}

	activePrompt := input.ActivePrompt
	if activePrompt.Kind == "" {
		activePrompt = InferActivePromptContext(input.StructuredInput.History, input.StructuredInput.State)
	}
	validation := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  input.StructuredInput.CurrentTurn,
		History:      input.StructuredInput.History,
		State:        input.StructuredInput.State,
		ActivePrompt: activePrompt,
		ObservedAt:   input.StructuredInput.ObservedAt,
	})
	base.ValidationResult = validation
	if !validation.Accepted {
		base.Status = OpenAIInterpreterAssistRejected
		base.ValidationStatus = OpenAIInterpreterAssistValidationRejected
		base.RejectReason = validation.RejectReason
		base.FallbackTemplate = validation.FallbackTemplate
		base.ReasonCodes = appendOpenAIInterpreterAssistReasonCode(nil, validation.RejectReason)
		return base
	}

	intentDecision, rejectReason, fallback := convertAcceptedStructuredInterpretationToIntentDecision(
		proposal,
		input.StructuredInput.CurrentTurn,
		input.StructuredInput.History,
		input.StructuredInput.State,
		activePrompt,
		input.StructuredInput.ObservedAt,
	)
	if rejectReason != "" {
		base.Status = OpenAIInterpreterAssistRejected
		base.ValidationStatus = OpenAIInterpreterAssistValidationAccepted
		base.RejectReason = rejectReason
		base.FallbackTemplate = fallback
		base.ReasonCodes = appendOpenAIInterpreterAssistReasonCode(nil, rejectReason)
		return base
	}

	base.Status = OpenAIInterpreterAssistAccepted
	base.ValidationStatus = OpenAIInterpreterAssistValidationAccepted
	base.Accepted = true
	base.IntentDecision = intentDecision
	return base
}

func convertAcceptedStructuredInterpretationToIntentDecision(proposal StructuredInterpretation, currentTurn string, history []Message, state CanonicalConversationState, activePrompt ActivePromptContext, observedAt time.Time) (IntentDecision, string, ResponseTemplateName) {
	source := "openai_interpreter_assist"
	switch proposal.Intent {
	case StructuredIntentAvailabilitySearch:
		if looksLikeReservationHowToProceedIntent(currentTurn) && shouldUseReservationStartTemplate(state, history) {
			return IntentDecision{
				Intent:       IntentAvailabilitySearch,
				Source:       source,
				TemplateName: TemplateAskReservationRouteSC,
				Action:       "template",
			}, "", ""
		}
		if input, ok := parseAvailabilitySearchInput(history, currentTurn, observedAt); ok {
			_ = input
			return IntentDecision{}, "openai_assist_tool_action_not_allowed", ""
		}
		return IntentDecision{}, "availability_input_not_safely_parseable", ""
	case StructuredIntentSelectAvailabilityOption:
		if activePrompt.Kind != ActivePromptAvailabilityOptionChoice {
			return IntentDecision{}, "active_prompt_required", TemplateContextFallbackAvailabilityOption
		}
		if !proposal.Booking.SelectedOptionIndexKnown || proposal.Booking.SelectedOptionIndex <= 0 {
			return IntentDecision{}, "selected_option_index_required", TemplateContextFallbackAvailabilityOption
		}
		return IntentDecision{
			Intent:              IntentSelectAvailabilityOption,
			Source:              source,
			SelectedOptionIndex: proposal.Booking.SelectedOptionIndex,
			TemplateName:        TemplateAskPassengerCount,
			Action:              "template",
		}, "", ""
	case StructuredIntentPassengerCountReply:
		if activePrompt.Kind != ActivePromptPassengerCount && activePrompt.Kind != ActivePromptLapChildQuestion {
			return IntentDecision{}, "active_prompt_required", TemplateContextFallbackPassengerCount
		}
		return IntentDecision{
			Intent: IntentPassengerCountReply,
			Source: source,
			Action: "booking_continuation",
		}, "", ""
	case StructuredIntentLapChildAssignmentAnswer:
		if activePrompt.Kind != ActivePromptLapChildAssignment {
			return IntentDecision{}, "active_prompt_required", TemplateContextFallbackLapChildAssignment
		}
		return IntentDecision{
			Intent:              IntentLapChildAssignmentAnswer,
			Source:              source,
			SelectedOptionIndex: firstLapChildPassengerIndex(proposal),
			Action:              "booking_continuation",
		}, "", ""
	case StructuredIntentPassengerDocumentsProvided:
		return IntentDecision{}, "document_extract_not_allowed_in_openai_assist", TemplateContextFallbackPassengerDocuments
	case StructuredIntentDocumentConfirmation:
		return IntentDecision{}, "booking_create_not_allowed_in_openai_assist", TemplateContextFallbackDocumentConfirmation
	case StructuredIntentPaymentPreference:
		return IntentDecision{}, "payment_create_not_allowed_in_openai_assist", TemplateContextFallbackPaymentPreference
	case StructuredIntentBookingCancelRequest:
		return IntentDecision{}, "booking_cancel_not_allowed_in_openai_assist", ""
	case StructuredIntentGreeting:
		return IntentDecision{}, "no_safe_greeting_template", ""
	default:
		return IntentDecision{}, "intent_not_allowed_in_openai_assist", structuredIntentFallbackTemplate(proposal.Intent)
	}
}

func firstLapChildPassengerIndex(proposal StructuredInterpretation) int {
	for _, index := range proposal.Booking.LapChildPassengerIndexes {
		if index > 0 {
			return index
		}
	}
	return 0
}

func (d OpenAIInterpreterAssistDecision) Metadata() map[string]interface{} {
	out := map[string]interface{}{
		"enabled":              d.Enabled,
		"considered":           d.Considered,
		"status":               string(d.Status),
		"openai_assist_status": string(d.Status),
		"validation_status":    string(d.ValidationStatus),
		"accepted":             d.Accepted,
		"decision_source":      d.DecisionSource,
		"local_intent":         string(d.LocalIntent),
		"local_confidence":     d.LocalConfidence,
	}
	if d.OpenAIIntent != "" {
		out["openai_intent"] = string(d.OpenAIIntent)
		out["openai_confidence"] = d.OpenAIConfidence
	}
	if strings.TrimSpace(d.RejectReason) != "" {
		out["reject_reason"] = strings.TrimSpace(d.RejectReason)
		if d.Status == OpenAIInterpreterAssistSkipped {
			out["skip_reason"] = strings.TrimSpace(d.RejectReason)
		}
	}
	if d.FallbackTemplate != "" {
		out["fallback_template"] = string(d.FallbackTemplate)
	}
	if len(d.ReasonCodes) > 0 {
		out["controlled_reason_codes"] = append([]string(nil), d.ReasonCodes...)
		out["reasons"] = append([]string(nil), d.ReasonCodes...)
	}
	return out
}

func openAIInterpreterAssistSkipReason(err error) string {
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

func appendOpenAIInterpreterAssistReasonCode(base []string, extra string) []string {
	out := append([]string(nil), base...)
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return out
	}
	for _, item := range out {
		if item == extra {
			return out
		}
	}
	return append(out, extra)
}
