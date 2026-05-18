package chat

import "strings"

const schedulingSpecialistPrompt = "You plan booking, cancellation, and rescheduling actions. Return only valid JSON. Respect the current booking phase and missing fields. Never confirm booking creation without backend tool success."

func buildSchedulingSpecialistSystemPrompt() string {
	return schedulingSpecialistPrompt
}

func schedulingSpecialistActionPlanSchema() map[string]interface{} {
	return specialistActionPlanSchema("scheduling_action_plan", []string{
		toolNameBookingLookup,
		toolNameBookingCreate,
		toolNameBookingCancel,
		toolNameRescheduleLookup,
	})
}

func validateSchedulingActionPlan(plan SchedulingActionPlan, state CanonicalConversationState) SpecialistPlanValidationResult {
	requests := append([]SpecialistToolRequest(nil), plan.ToolRequests...)
	if len(requests) == 0 && strings.TrimSpace(plan.ToolName) != "" {
		requests = append(requests, SpecialistToolRequest{
			ToolName:    strings.TrimSpace(plan.ToolName),
			ReasonCode:  "legacy_scheduling_plan",
			Origin:      strings.TrimSpace(plan.Origin),
			Destination: strings.TrimSpace(plan.Destination),
			PackageName: strings.TrimSpace(plan.PackageName),
			TripDate:    strings.TrimSpace(plan.TripDate),
			Qty:         plan.Qty,
			Limit:       plan.Limit,
		})
	}
	return validateSpecialistPlan(specialistPlanValidationInput{
		Domain:             jsonDecisionDomainScheduling,
		Action:             plan.Action,
		MissingFields:      plan.MissingFields,
		ToolRequests:       requests,
		CustomerFacingText: plan.CustomerFacingText,
		ReplyPlan:          plan.ReplyPlan,
		State:              state,
	})
}
