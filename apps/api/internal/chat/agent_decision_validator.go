package chat

import "strings"

const minJSONDecisionConfidence = 0.70

type AgentDecisionValidationResult struct {
	Decision IntentDecisionJSON
	Valid    bool
	Reasons  []string
}

func validateAgentIntentDecision(decision IntentDecisionJSON, state CanonicalConversationState) AgentDecisionValidationResult {
	normalized := normalizeAgentIntentDecision(decision)
	reasons := validateAgentIntentDecisionReasons(normalized, state)
	if len(reasons) == 0 {
		return AgentDecisionValidationResult{Decision: normalized, Valid: true}
	}
	normalized.Action = jsonDecisionActionClarify
	if len(normalized.MissingFields) == 0 {
		normalized.MissingFields = append([]string(nil), reasons...)
	}
	normalized.AvailabilityInput = nil
	normalized.PaymentInput = nil
	normalized.BookingInput = nil
	normalized.SelectedOptionIndex = nil
	return AgentDecisionValidationResult{Decision: normalized, Valid: false, Reasons: reasons}
}

func normalizeAgentIntentDecision(decision IntentDecisionJSON) IntentDecisionJSON {
	decision.Domain = strings.ToLower(strings.TrimSpace(decision.Domain))
	decision.Intent = strings.ToUpper(strings.TrimSpace(decision.Intent))
	decision.Action = strings.ToLower(strings.TrimSpace(decision.Action))
	decision.SafeNextStep = strings.TrimSpace(decision.SafeNextStep)
	if decision.MissingFields == nil {
		decision.MissingFields = []string{}
	}
	return decision
}

func validateAgentIntentDecisionReasons(decision IntentDecisionJSON, state CanonicalConversationState) []string {
	reasons := []string{}
	if decision.Confidence < minJSONDecisionConfidence {
		reasons = append(reasons, "low_confidence")
	}
	if !jsonDecisionDomainAllowed(decision.Domain) {
		reasons = append(reasons, "invalid_domain")
	}
	if !jsonDecisionActionAllowed(decision.Action) {
		reasons = append(reasons, "invalid_action")
	}
	if decision.Intent == "" {
		reasons = append(reasons, "missing_intent")
	}
	if !agentDecisionIntentAllowedForPhase(decision.Intent, state) {
		reasons = append(reasons, "intent_not_allowed_for_phase")
	}
	if containsOperationalAutoSendClaimWithoutTool(decision.SafeNextStep) && !agentDecisionHasToolFacts(decision, state) {
		reasons = append(reasons, "operational_claim_without_tool")
	}
	switch decision.Action {
	case jsonDecisionActionTool:
		reasons = append(reasons, validateAgentToolDecisionReasons(decision, state)...)
	case jsonDecisionActionTemplate:
		if decision.SafeNextStep == "" && decision.BookingInput == nil {
			reasons = append(reasons, "template_missing_safe_next_step")
		}
	case jsonDecisionActionClarify:
		if len(decision.MissingFields) == 0 && decision.SafeNextStep == "" {
			reasons = append(reasons, "clarify_missing_fields")
		}
	}
	return reasons
}

func jsonDecisionDomainAllowed(domain string) bool {
	switch domain {
	case jsonDecisionDomainGeneral, jsonDecisionDomainScheduling, jsonDecisionDomainPayments, jsonDecisionDomainHandoff:
		return true
	default:
		return false
	}
}

func jsonDecisionActionAllowed(action string) bool {
	switch action {
	case jsonDecisionActionTemplate, jsonDecisionActionTool, jsonDecisionActionSpecialist, jsonDecisionActionClarify, jsonDecisionActionHandoff, jsonDecisionActionNoop:
		return true
	default:
		return false
	}
}

func agentDecisionIntentAllowedForPhase(intent string, state CanonicalConversationState) bool {
	if intent == "" || intent == string(IntentUnknown) {
		return false
	}
	if len(state.AllowedNextActions) == 0 {
		return true
	}
	for _, allowed := range state.AllowedNextActions {
		if strings.EqualFold(strings.TrimSpace(allowed), intent) {
			return true
		}
	}
	return false
}

func validateAgentToolDecisionReasons(decision IntentDecisionJSON, state CanonicalConversationState) []string {
	reasons := []string{}
	switch decision.Intent {
	case string(IntentAvailabilitySearch):
		input := decision.AvailabilityInput
		if input == nil {
			return append(reasons, "availability_input_required")
		}
		if strings.TrimSpace(input.ToolName) != "" && !strings.EqualFold(strings.TrimSpace(input.ToolName), toolNameAvailabilitySearch) {
			reasons = append(reasons, "availability_tool_name_invalid")
		}
		if strings.TrimSpace(input.Origin) == "" && strings.TrimSpace(input.Destination) == "" && strings.TrimSpace(input.PackageName) == "" {
			reasons = append(reasons, "availability_route_required")
		}
	case string(IntentPaymentCreate):
		input := decision.PaymentInput
		if input == nil {
			return append(reasons, "payment_input_required")
		}
		if strings.TrimSpace(input.ToolName) != "" && !strings.EqualFold(strings.TrimSpace(input.ToolName), toolNamePaymentCreate) {
			reasons = append(reasons, "payment_tool_name_invalid")
		}
		if !agentDecisionHasBookingContext(state) {
			reasons = append(reasons, "payment_requires_booking_context")
		}
		if strings.TrimSpace(input.BookingID) == "" && strings.TrimSpace(input.ReservationCode) == "" {
			reasons = append(reasons, "payment_booking_reference_required")
		}
	default:
		reasons = append(reasons, "unsupported_tool_intent")
	}
	return reasons
}

func agentDecisionHasBookingContext(state CanonicalConversationState) bool {
	if strings.TrimSpace(state.Booking.BookingID) != "" || strings.TrimSpace(state.Booking.ReservationCode) != "" {
		return true
	}
	return len(asMap(state.LastToolFacts[toolNameBookingCreate])) > 0
}

func agentDecisionHasToolFacts(decision IntentDecisionJSON, state CanonicalConversationState) bool {
	switch decision.Intent {
	case string(IntentAvailabilitySearch):
		return len(asMap(state.LastToolFacts[toolNameAvailabilitySearch])) > 0
	case string(IntentPaymentStatusQuery):
		return len(asMap(state.LastToolFacts[toolNamePaymentStatus])) > 0
	case string(IntentPaymentCreate):
		return len(asMap(state.LastToolFacts[toolNamePaymentCreate])) > 0
	case string(IntentBookingCreateConfirmation):
		return len(asMap(state.LastToolFacts[toolNameBookingCreate])) > 0
	default:
		return false
	}
}
