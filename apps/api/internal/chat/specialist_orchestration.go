package chat

import (
	"context"
	"fmt"
)

type agentJSONGenericRunner interface {
	Run(ctx context.Context, input RunJSONDecisionInput, output interface{}) (OpenAIJSONRunResult, error)
}

type specialistPlannerRunResult struct {
	Validation SpecialistPlanValidationResult
	JSONRun    OpenAIJSONRunResult
	Domain     string
}

func (s *Service) canRunSpecialistPlanner() bool {
	if s == nil || s.jsonRunner == nil || !s.jsonRunner.Enabled() {
		return false
	}
	_, ok := s.jsonRunner.(agentJSONGenericRunner)
	return ok
}

func (s *Service) runSpecialistPlanner(ctx context.Context, session Session, decision IntentDecisionJSON, currentTurn string, state CanonicalConversationState, history []Message, idempotencyKey string) (specialistPlannerRunResult, error) {
	runner, ok := s.jsonRunner.(agentJSONGenericRunner)
	if !ok {
		return specialistPlannerRunResult{}, fmt.Errorf("%w: specialist planner runner unavailable", ErrOpenAIJSONRunnerNotConfigured)
	}
	compactInput := buildJSONDecisionCompactInput(currentTurn, state, history)
	switch intentAgentTargetSpecialist(decision) {
	case jsonDecisionDomainGeneral:
		var plan GeneralActionPlan
		jsonRun, err := runner.Run(ctx, RunJSONDecisionInput{
			SystemPrompt:   buildGeneralSpecialistSystemPrompt(),
			CompactInput:   compactInput,
			SchemaName:     "general_action_plan",
			Schema:         generalSpecialistActionPlanSchema(),
			IdempotencyKey: idempotencyKey + ":general_specialist",
			Session:        session,
		}, &plan)
		if err != nil {
			return specialistPlannerRunResult{}, err
		}
		return specialistPlannerRunResult{Validation: validateGeneralActionPlan(plan, state), JSONRun: jsonRun, Domain: jsonDecisionDomainGeneral}, nil
	case jsonDecisionDomainScheduling:
		var plan SchedulingActionPlan
		jsonRun, err := runner.Run(ctx, RunJSONDecisionInput{
			SystemPrompt:   buildSchedulingSpecialistSystemPrompt(),
			CompactInput:   compactInput,
			SchemaName:     "scheduling_action_plan",
			Schema:         schedulingSpecialistActionPlanSchema(),
			IdempotencyKey: idempotencyKey + ":scheduling_specialist",
			Session:        session,
		}, &plan)
		if err != nil {
			return specialistPlannerRunResult{}, err
		}
		return specialistPlannerRunResult{Validation: validateSchedulingActionPlan(plan, state), JSONRun: jsonRun, Domain: jsonDecisionDomainScheduling}, nil
	case jsonDecisionDomainPayments:
		var plan PaymentActionPlan
		jsonRun, err := runner.Run(ctx, RunJSONDecisionInput{
			SystemPrompt:   buildPaymentsSpecialistSystemPrompt(),
			CompactInput:   compactInput,
			SchemaName:     "payment_action_plan",
			Schema:         paymentsSpecialistActionPlanSchema(),
			IdempotencyKey: idempotencyKey + ":payments_specialist",
			Session:        session,
		}, &plan)
		if err != nil {
			return specialistPlannerRunResult{}, err
		}
		return specialistPlannerRunResult{Validation: validatePaymentActionPlan(plan, state), JSONRun: jsonRun, Domain: jsonDecisionDomainPayments}, nil
	default:
		return specialistPlannerRunResult{}, fmt.Errorf("%w: unsupported specialist domain", ErrOpenAIJSONInvalidOutput)
	}
}
