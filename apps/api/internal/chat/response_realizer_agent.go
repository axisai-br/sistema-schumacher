package chat

import "strings"

const responseRealizerAgentPrompt = "Write one short Portuguese message for the customer. Use only the validated reply plan and tool facts. Ask at most one question. Do not mention internal systems."

func buildResponseRealizerAgentSystemPrompt() string {
	return responseRealizerAgentPrompt
}

type ResponseRealizerValidationResult struct {
	Valid   bool
	Reasons []string
}

func validateResponseRealizerOutput(reply string, plan CustomerReplyPlan, toolFacts map[string]interface{}) ResponseRealizerValidationResult {
	reasons := []string{}
	trimmed := strings.TrimSpace(reply)
	if trimmed == "" {
		reasons = append(reasons, "empty_reply")
	}
	if strings.Count(trimmed, "?") > 1 {
		reasons = append(reasons, "too_many_questions")
	}
	if mentionsInternalSystem(trimmed) {
		reasons = append(reasons, "mentions_internal_system")
	}
	if containsOperationalAutoSendClaimWithoutTool(trimmed) && !hasValidatedOperationalFacts(toolFacts) {
		reasons = append(reasons, "operational_claim_without_tool_facts")
	}
	if strings.TrimSpace(plan.Message) != "" && containsOperationalAutoSendClaimWithoutTool(trimmed) && !hasValidatedOperationalFacts(toolFacts) {
		reasons = append(reasons, "reply_plan_operational_fact_unvalidated")
	}
	return ResponseRealizerValidationResult{Valid: len(reasons) == 0, Reasons: reasons}
}

func mentionsInternalSystem(text string) bool {
	folded := foldChatText(text)
	blocked := []string{"sistema interno", "backend", "ferramenta", "tool", "json", "agente", "llm"}
	for _, item := range blocked {
		if strings.Contains(folded, " "+item+" ") {
			return true
		}
	}
	return false
}

func hasValidatedOperationalFacts(toolFacts map[string]interface{}) bool {
	if len(toolFacts) == 0 {
		return false
	}
	for _, name := range []string{toolNameAvailabilitySearch, toolNamePricingQuote, toolNameBookingLookup, toolNameBookingCreate, toolNameBookingCancel, toolNamePaymentStatus, toolNamePaymentCreate, toolNameRescheduleLookup} {
		if len(asMap(toolFacts[name])) > 0 {
			return true
		}
	}
	return false
}
