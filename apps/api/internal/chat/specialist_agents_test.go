package chat

import "testing"

func TestIntentAgentRoutesAvailabilityToGeneralSpecialist(t *testing.T) {
	decision := IntentDecisionJSON{
		Domain:        jsonDecisionDomainGeneral,
		Intent:        string(IntentAvailabilitySearch),
		Action:        jsonDecisionActionSpecialist,
		Confidence:    0.95,
		MissingFields: []string{},
		SafeNextStep:  "plan with general specialist",
	}

	result := validateAgentIntentDecision(decision, discoveryState())

	if !result.Valid {
		t.Fatalf("expected valid general specialist route, got %#v", result.Reasons)
	}
	if !intentAgentRoutesToSpecialist(result.Decision, jsonDecisionDomainGeneral) {
		t.Fatalf("expected availability intent to route to general specialist")
	}
}

func TestIntentAgentRoutesBookingCreationToSchedulingSpecialist(t *testing.T) {
	decision := IntentDecisionJSON{
		Domain:        jsonDecisionDomainScheduling,
		Intent:        string(IntentBookingCreateConfirmation),
		Action:        jsonDecisionActionSpecialist,
		Confidence:    0.95,
		MissingFields: []string{},
		SafeNextStep:  "plan with scheduling specialist",
	}
	state := CanonicalConversationState{
		Phase:              ConversationPhaseBookingPending,
		AllowedNextActions: allowedNextActionsForPhase(ConversationPhaseBookingPending),
		LastToolFacts:      map[string]interface{}{},
	}

	result := validateAgentIntentDecision(decision, state)

	if !result.Valid {
		t.Fatalf("expected valid scheduling specialist route, got %#v", result.Reasons)
	}
	if !intentAgentRoutesToSpecialist(result.Decision, jsonDecisionDomainScheduling) {
		t.Fatalf("expected booking intent to route to scheduling specialist")
	}
}

func TestIntentAgentRoutesPaymentRequestToPaymentsSpecialist(t *testing.T) {
	decision := IntentDecisionJSON{
		Domain:        jsonDecisionDomainPayments,
		Intent:        string(IntentPaymentCreate),
		Action:        jsonDecisionActionSpecialist,
		Confidence:    0.95,
		MissingFields: []string{},
		SafeNextStep:  "plan with payments specialist",
	}
	state := CanonicalConversationState{
		Phase:              ConversationPhaseBooked,
		AllowedNextActions: allowedNextActionsForPhase(ConversationPhaseBooked),
		Booking:            CanonicalBookingState{BookingID: "booking-1", ReservationCode: "SCH-1"},
		LastToolFacts:      map[string]interface{}{},
	}

	result := validateAgentIntentDecision(decision, state)

	if !result.Valid {
		t.Fatalf("expected valid payments specialist route, got %#v", result.Reasons)
	}
	if !intentAgentRoutesToSpecialist(result.Decision, jsonDecisionDomainPayments) {
		t.Fatalf("expected payment intent to route to payments specialist")
	}
}

func TestSpecialistCannotBypassBackendValidation(t *testing.T) {
	plan := GeneralActionPlan{
		Action: specialistPlanActionTool,
		ToolRequests: []SpecialistToolRequest{{
			ToolName:   toolNameBookingCreate,
			ReasonCode: "try_cross_domain_tool",
		}},
	}

	result := validateGeneralActionPlan(plan, discoveryState())

	if result.Valid {
		t.Fatal("expected cross-domain tool request to be rejected")
	}
	if !hasValidationReason(result.Reasons, "tool_not_allowed_for_specialist") {
		t.Fatalf("expected tool_not_allowed_for_specialist, got %#v", result.Reasons)
	}
	if len(result.ApprovedToolRequests) != 0 {
		t.Fatalf("expected no approved tool requests, got %#v", result.ApprovedToolRequests)
	}
}

func TestSpecialistCannotReturnCustomerFacingProse(t *testing.T) {
	plan := GeneralActionPlan{
		Action:             specialistPlanActionClarify,
		MissingFields:      []string{"origin"},
		CustomerFacingText: "Me diga sua cidade de origem.",
	}

	result := validateGeneralActionPlan(plan, discoveryState())

	if result.Valid {
		t.Fatal("expected customer-facing prose to be rejected")
	}
	if !hasValidationReason(result.Reasons, "customer_facing_text_not_allowed") {
		t.Fatalf("expected customer_facing_text_not_allowed, got %#v", result.Reasons)
	}
}

func TestInvalidSpecialistPlanIsRejected(t *testing.T) {
	plan := GeneralActionPlan{
		Action: specialistPlanActionTool,
		ToolRequests: []SpecialistToolRequest{{
			ToolName:   toolNameAvailabilitySearch,
			ReasonCode: "missing_route",
		}},
	}

	result := validateGeneralActionPlan(plan, discoveryState())

	if result.Valid {
		t.Fatal("expected invalid availability plan to be rejected")
	}
	if !hasValidationReason(result.Reasons, "availability_route_required") {
		t.Fatalf("expected availability_route_required, got %#v", result.Reasons)
	}
}

func TestResponseRealizerCannotMentionUnvalidatedOperationalFacts(t *testing.T) {
	result := validateResponseRealizerOutput(
		"Tem disponibilidade para Chapeco no dia 20/05.",
		CustomerReplyPlan{Message: "Tem disponibilidade para Chapeco no dia 20/05."},
		nil,
	)

	if result.Valid {
		t.Fatal("expected operational fact without tool facts to be rejected")
	}
	if !hasValidationReason(result.Reasons, "operational_claim_without_tool_facts") {
		t.Fatalf("expected operational_claim_without_tool_facts, got %#v", result.Reasons)
	}
}
