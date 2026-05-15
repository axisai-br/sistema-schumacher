package chat

import (
	"strings"
	"time"
)

type chatAgentRolloutMetadata struct {
	Mode                 string
	DecisionSource       string
	DecisionValid        *bool
	DecisionConfidence   *float64
	ValidationErrors     []string
	FallbackReason       string
	CanonicalPhaseBefore ConversationPhase
	CanonicalPhaseAfter  ConversationPhase
	ToolCallCount        int
}

func (s *Service) chatAgentMode() string {
	if s == nil {
		return chatAgentModeLegacy
	}
	return normalizeChatAgentMode(s.cfg.ChatAgentMode)
}

func (m chatAgentRolloutMetadata) toMap() map[string]interface{} {
	out := map[string]interface{}{
		"chat_agent_mode":        normalizeChatAgentMode(m.Mode),
		"canonical_phase_before": string(m.CanonicalPhaseBefore),
		"canonical_phase_after":  string(m.CanonicalPhaseAfter),
		"tool_call_count":        m.ToolCallCount,
	}
	if strings.TrimSpace(m.DecisionSource) != "" {
		out["decision_source"] = strings.TrimSpace(m.DecisionSource)
	}
	if m.DecisionValid != nil {
		out["decision_valid"] = *m.DecisionValid
	}
	if m.DecisionConfidence != nil {
		out["decision_confidence"] = *m.DecisionConfidence
	}
	if len(m.ValidationErrors) > 0 {
		out["validation_errors"] = append([]string(nil), m.ValidationErrors...)
	}
	if strings.TrimSpace(m.FallbackReason) != "" {
		out["fallback_reason"] = strings.TrimSpace(m.FallbackReason)
	}
	return out
}

func applyRolloutMetadataToRun(run *RunAgentResult, metadata chatAgentRolloutMetadata) {
	if run == nil {
		return
	}
	values := metadata.toMap()
	if run.RequestPayload == nil {
		run.RequestPayload = map[string]interface{}{}
	}
	if run.ResponsePayload == nil {
		run.ResponsePayload = map[string]interface{}{}
	}
	for key, value := range values {
		run.RequestPayload[key] = value
		run.ResponsePayload[key] = value
	}
}

func mergeToolCallRequestMetadata(context agentToolContext, metadata chatAgentRolloutMetadata) agentToolContext {
	values := metadata.toMap()
	for i := range context.Calls {
		if context.Calls[i].RequestPayload == nil {
			context.Calls[i].RequestPayload = map[string]interface{}{}
		}
		for key, value := range values {
			context.Calls[i].RequestPayload[key] = value
		}
	}
	return context
}

func buildJSONDecisionClarificationDraftRun(validation AgentDecisionValidationResult, metadata chatAgentRolloutMetadata) RunAgentResult {
	reasons := validation.Reasons
	if len(reasons) == 0 {
		reasons = []string{"json_decision_unavailable"}
	}
	payload := map[string]interface{}{
		"mode":              "TEMPLATE_FIRST_REPLY",
		"intent":            validation.Decision.Intent,
		"action":            jsonDecisionActionClarify,
		"template_name":     "json_decision_clarify",
		"validation_errors": reasons,
	}
	for key, value := range metadata.toMap() {
		payload[key] = value
	}
	return RunAgentResult{
		ReplyText:       "Nao consegui confirmar os dados com seguranca. Pode me dizer origem, destino e para quando voce quer viajar?",
		Model:           "template_realizer",
		RequestPayload:  payload,
		ResponsePayload: cloneMap(payload),
	}
}

func availabilityInputFromSchedulingPlan(plan SchedulingActionPlan) AvailabilitySearchInput {
	input := AvailabilitySearchInput{
		Origin:      strings.TrimSpace(plan.Origin),
		Destination: strings.TrimSpace(plan.Destination),
		PackageName: strings.TrimSpace(plan.PackageName),
		Qty:         plan.Qty,
		Limit:       plan.Limit,
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	if input.Limit <= 0 {
		input.Limit = 5
	}
	if dateText := strings.TrimSpace(plan.TripDate); dateText != "" {
		if parsed, err := time.Parse("2006-01-02", dateText); err == nil {
			input.TripDate = &parsed
		}
	}
	return input
}

func paymentCreateInputFromPlan(plan PaymentActionPlan) PaymentCreateInput {
	return PaymentCreateInput{
		BookingID:        strings.TrimSpace(plan.BookingID),
		ReservationCode:  strings.TrimSpace(plan.ReservationCode),
		PaymentType:      strings.TrimSpace(plan.PaymentMethod),
		CustomerDocument: strings.TrimSpace(plan.CustomerDocument),
		PaidAmount:       plan.Amount,
		Note:             "JSON decision layer payment_create",
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}
