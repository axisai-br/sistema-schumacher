package chat

import "strings"

const paymentsSpecialistPrompt = "You plan payment-related actions. Return only valid JSON. Never confirm payment or generate charge instructions without backend tool facts. Use booking and payment state only from canonical state or tool results."

func buildPaymentsSpecialistSystemPrompt() string {
	return paymentsSpecialistPrompt
}

func paymentsSpecialistActionPlanSchema() map[string]interface{} {
	return specialistActionPlanSchema("payment_action_plan", []string{
		toolNameBookingLookup,
		toolNamePaymentStatus,
		toolNamePaymentCreate,
	})
}

func validatePaymentActionPlan(plan PaymentActionPlan, state CanonicalConversationState) SpecialistPlanValidationResult {
	requests := append([]SpecialistToolRequest(nil), plan.ToolRequests...)
	if len(requests) == 0 && strings.TrimSpace(plan.ToolName) != "" {
		requests = append(requests, SpecialistToolRequest{
			ToolName:         strings.TrimSpace(plan.ToolName),
			ReasonCode:       "legacy_payment_plan",
			BookingID:        strings.TrimSpace(plan.BookingID),
			ReservationCode:  strings.TrimSpace(plan.ReservationCode),
			PaymentMethod:    strings.TrimSpace(plan.PaymentMethod),
			CustomerDocument: strings.TrimSpace(plan.CustomerDocument),
			Amount:           plan.Amount,
		})
	}
	return validateSpecialistPlan(specialistPlanValidationInput{
		Domain:             jsonDecisionDomainPayments,
		Action:             plan.Action,
		MissingFields:      plan.MissingFields,
		ToolRequests:       requests,
		CustomerFacingText: plan.CustomerFacingText,
		ReplyPlan:          plan.ReplyPlan,
		State:              state,
	})
}
