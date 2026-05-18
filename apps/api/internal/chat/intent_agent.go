package chat

import "strings"

const intentAgentPrompt = "You classify the current customer turn using the provided canonical state. Return only valid JSON matching the schema. Do not write customer-facing text. If required information is missing or confidence is low, choose clarify. Never claim operational facts."

func buildIntentAgentSystemPrompt() string {
	return intentAgentPrompt
}

func buildJSONDecisionSystemPrompt() string {
	return buildIntentAgentSystemPrompt()
}

func intentAgentTargetSpecialist(decision IntentDecisionJSON) string {
	switch strings.ToLower(strings.TrimSpace(decision.Domain)) {
	case jsonDecisionDomainGeneral:
		return jsonDecisionDomainGeneral
	case jsonDecisionDomainScheduling:
		return jsonDecisionDomainScheduling
	case jsonDecisionDomainPayments:
		return jsonDecisionDomainPayments
	default:
		return ""
	}
}

func intentAgentRoutesToSpecialist(decision IntentDecisionJSON, specialist string) bool {
	return strings.EqualFold(strings.TrimSpace(decision.Action), jsonDecisionActionSpecialist) &&
		strings.EqualFold(intentAgentTargetSpecialist(decision), specialist)
}
