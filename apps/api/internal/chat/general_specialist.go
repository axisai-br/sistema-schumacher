package chat

const generalSpecialistPrompt = "You plan travel-information actions from canonical state and the current turn. Return only valid JSON. Use tools only through the backend plan. Do not invent route, date, price, seat, document, or availability facts."

func buildGeneralSpecialistSystemPrompt() string {
	return generalSpecialistPrompt
}

func generalSpecialistActionPlanSchema() map[string]interface{} {
	return specialistActionPlanSchema("general_action_plan", []string{
		toolNameAvailabilitySearch,
		toolNamePricingQuote,
		toolNameDocumentExtract,
	})
}

func validateGeneralActionPlan(plan GeneralActionPlan, state CanonicalConversationState) SpecialistPlanValidationResult {
	requests := append([]SpecialistToolRequest(nil), plan.ToolRequests...)
	return validateSpecialistPlan(specialistPlanValidationInput{
		Domain:             jsonDecisionDomainGeneral,
		Action:             plan.Action,
		MissingFields:      plan.MissingFields,
		ToolRequests:       requests,
		CustomerFacingText: plan.CustomerFacingText,
		ReplyPlan:          plan.ReplyPlan,
		State:              state,
	})
}
