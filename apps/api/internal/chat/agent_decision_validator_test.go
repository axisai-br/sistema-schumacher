package chat

import "testing"

func TestAgentDecisionValidatorLowConfidenceBecomesClarify(t *testing.T) {
	decision := validAvailabilityJSONDecision()
	decision.Confidence = 0.40

	result := validateAgentIntentDecision(decision, discoveryState())

	if result.Valid {
		t.Fatal("expected low confidence decision to be invalid")
	}
	if result.Decision.Action != jsonDecisionActionClarify {
		t.Fatalf("expected clarify action, got %s", result.Decision.Action)
	}
	if !hasValidationReason(result.Reasons, "low_confidence") {
		t.Fatalf("expected low_confidence reason, got %#v", result.Reasons)
	}
}

func TestAgentDecisionValidatorRejectsActionOutsideCanonicalPhase(t *testing.T) {
	decision := validAvailabilityJSONDecision()

	result := validateAgentIntentDecision(decision, CanonicalConversationState{
		Phase:              ConversationPhaseBooked,
		AllowedNextActions: allowedNextActionsForPhase(ConversationPhaseBooked),
	})

	if result.Valid {
		t.Fatal("expected availability search to be rejected in booked phase")
	}
	if result.Decision.Action != jsonDecisionActionClarify {
		t.Fatalf("expected clarify action, got %s", result.Decision.Action)
	}
	if !hasValidationReason(result.Reasons, "intent_not_allowed_for_phase") {
		t.Fatalf("expected intent_not_allowed_for_phase reason, got %#v", result.Reasons)
	}
}

func TestAgentDecisionValidatorRejectsToolActionMissingRequiredFields(t *testing.T) {
	decision := validAvailabilityJSONDecision()
	decision.AvailabilityInput = nil

	result := validateAgentIntentDecision(decision, discoveryState())

	if result.Valid {
		t.Fatal("expected tool decision without input to be invalid")
	}
	if !hasValidationReason(result.Reasons, "availability_input_required") {
		t.Fatalf("expected availability_input_required reason, got %#v", result.Reasons)
	}
}

func TestAgentDecisionValidatorValidAvailabilitySearchPasses(t *testing.T) {
	result := validateAgentIntentDecision(validAvailabilityJSONDecision(), discoveryState())

	if !result.Valid {
		t.Fatalf("expected valid availability decision, got reasons %#v", result.Reasons)
	}
	if result.Decision.Action != jsonDecisionActionTool {
		t.Fatalf("expected tool action, got %s", result.Decision.Action)
	}
}

func TestAgentDecisionValidatorPaymentCreateRequiresBookingContext(t *testing.T) {
	decision := IntentDecisionJSON{
		Domain:        jsonDecisionDomainPayments,
		Intent:        string(IntentPaymentCreate),
		Action:        jsonDecisionActionTool,
		Confidence:    0.95,
		MissingFields: []string{},
		SafeNextStep:  "call payment_create",
		PaymentInput: &PaymentActionPlan{
			ToolName:        toolNamePaymentCreate,
			BookingID:       "booking-1",
			ReservationCode: "SCH-1",
			PaymentMethod:   "PIX_INTEGRAL",
		},
	}

	result := validateAgentIntentDecision(decision, CanonicalConversationState{
		Phase:              ConversationPhaseBooked,
		AllowedNextActions: allowedNextActionsForPhase(ConversationPhaseBooked),
	})

	if result.Valid {
		t.Fatal("expected payment decision without booking context to be invalid")
	}
	if !hasValidationReason(result.Reasons, "payment_requires_booking_context") {
		t.Fatalf("expected payment_requires_booking_context reason, got %#v", result.Reasons)
	}

	state := CanonicalConversationState{
		Phase:              ConversationPhaseBooked,
		AllowedNextActions: allowedNextActionsForPhase(ConversationPhaseBooked),
		Booking:            CanonicalBookingState{BookingID: "booking-1", ReservationCode: "SCH-1"},
	}
	result = validateAgentIntentDecision(decision, state)
	if !result.Valid {
		t.Fatalf("expected payment decision with booking context to pass, got %#v", result.Reasons)
	}
}

func validAvailabilityJSONDecision() IntentDecisionJSON {
	return IntentDecisionJSON{
		Domain:        jsonDecisionDomainScheduling,
		Intent:        string(IntentAvailabilitySearch),
		Action:        jsonDecisionActionTool,
		Confidence:    0.92,
		MissingFields: []string{},
		SafeNextStep:  "call availability_search",
		AvailabilityInput: &SchedulingActionPlan{
			ToolName:    toolNameAvailabilitySearch,
			Origin:      "Chapeco/SC",
			Destination: "Santa Ines/MA",
			PackageName: "Pacote p/ Maranhão",
			Qty:         1,
			Limit:       5,
		},
	}
}

func discoveryState() CanonicalConversationState {
	return CanonicalConversationState{
		Phase:              ConversationPhaseDiscovery,
		AllowedNextActions: allowedNextActionsForPhase(ConversationPhaseDiscovery),
		LastToolFacts:      map[string]interface{}{},
	}
}

func hasValidationReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
