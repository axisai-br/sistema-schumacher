package chat

import (
	"strings"
	"testing"
	"time"
)

func TestInterpreterCaseEvaluationLoadsAndEvaluatesCorpus(t *testing.T) {
	cases := loadInterpreterCasesForTest(t)
	report := EvaluateInterpreterCases(cases, interpreterCaseEvaluationObservedAt())
	t.Logf("interpreter case evaluation report: total=%d passed=%d failed=%d skipped=%d", report.Total, report.Passed, report.Failed, report.Skipped)
	for _, result := range report.Results {
		if !result.Passed && !result.Skipped {
			t.Logf("interpreter case evaluation failure: case=%s failures=%v", result.CaseID, result.Failures)
		}
	}

	if report.Total != len(cases) {
		t.Fatalf("expected total %d, got %+v", len(cases), report)
	}
	if len(report.Results) != len(cases) {
		t.Fatalf("expected one result per case, got %d results for %d cases", len(report.Results), len(cases))
	}
	if report.Passed+report.Failed+report.Skipped != report.Total {
		t.Fatalf("report counts do not sum to total: %+v", report)
	}
	if report.Total < 27 {
		t.Fatalf("expected at least 27 evaluated cases, got %+v", report)
	}
}

func TestInterpreterCaseEvaluationCriticalCasesPass(t *testing.T) {
	report := EvaluateInterpreterCases(loadInterpreterCasesForTest(t), interpreterCaseEvaluationObservedAt())

	for _, id := range []string{
		"reservation_help_001",
		"unsupported_package_001",
		"availability_option_003",
		"payment_preference_003",
		"availability_date_002",
		"passenger_documents_002",
	} {
		result := interpreterCaseEvaluationByID(t, report, id)
		if !result.Passed {
			t.Fatalf("expected critical case %s to pass, failures=%v result=%+v", id, result.Failures, result)
		}
		if !result.ValidatorEvaluated {
			t.Fatalf("expected critical case %s to evaluate local validator", id)
		}
	}
}

func TestInterpreterCaseEvaluationCriticalForbiddenIntents(t *testing.T) {
	cases := loadInterpreterCasesForTest(t)
	report := EvaluateInterpreterCases(cases, interpreterCaseEvaluationObservedAt())

	for _, id := range []string{
		"reservation_help_001",
		"unsupported_package_001",
		"availability_option_003",
		"payment_preference_003",
		"availability_date_002",
		"passenger_documents_002",
	} {
		item := interpreterCaseByID(t, cases, id)
		result := interpreterCaseEvaluationByID(t, report, id)
		for _, forbidden := range item.ForbiddenIntents {
			if result.EffectiveIntent == forbidden {
				t.Fatalf("case %s ended with forbidden effective intent %s: %+v", id, forbidden, result)
			}
			activePromptKind := ActivePromptKind(strings.TrimSpace(item.ActivePromptKind))
			if result.ValidatorAccepted && interpreterCaseCorpusIntent(StructuredIntent(result.ValidatorProposalIntent), activePromptKind) == forbidden {
				t.Fatalf("case %s accepted forbidden validator proposal %s: %+v", id, forbidden, result)
			}
			if result.ValidatorProbeEvaluated &&
				result.ValidatorProbeAccepted &&
				interpreterCaseCorpusIntent(StructuredIntent(result.ValidatorProbeIntent), activePromptKind) == forbidden {
				t.Fatalf("case %s accepted forbidden validator probe %s: %+v", id, forbidden, result)
			}
		}
	}
}

func TestInterpreterCaseEvaluationReportsValidatorFallbacks(t *testing.T) {
	report := EvaluateInterpreterCases(loadInterpreterCasesForTest(t), interpreterCaseEvaluationObservedAt())

	for _, tc := range []struct {
		id       string
		fallback ResponseTemplateName
	}{
		{id: "availability_option_003", fallback: TemplateContextFallbackAvailabilityOption},
		{id: "payment_preference_003", fallback: TemplateContextFallbackPaymentPreference},
		{id: "availability_date_002", fallback: TemplateContextFallbackAvailabilityDate},
		{id: "passenger_documents_002", fallback: TemplateContextFallbackPassengerDocuments},
	} {
		result := interpreterCaseEvaluationByID(t, report, tc.id)
		if result.ValidatorAccepted {
			t.Fatalf("expected case %s to be rejected by validator, got %+v", tc.id, result)
		}
		if interpreterCaseMainFallbackTemplate(result) != string(tc.fallback) {
			t.Fatalf("expected case %s fallback %s, got %+v", tc.id, tc.fallback, result)
		}
	}
}

func TestInterpreterCaseEvaluationUsesLocalStructuredLayerOnly(t *testing.T) {
	report := EvaluateInterpreterCases(loadInterpreterCasesForTest(t), interpreterCaseEvaluationObservedAt())
	for _, result := range report.Results {
		if !result.ValidatorEvaluated {
			t.Fatalf("expected validator to be evaluated for case %s", result.CaseID)
		}
		if strings.Contains(strings.ToLower(result.ValidatorProposalIntent), "openai") {
			t.Fatalf("validator proposal must not reference OpenAI: %+v", result)
		}
		if strings.Contains(strings.ToLower(result.ValidatorProbeIntent), "openai") {
			t.Fatalf("validator probe must not reference OpenAI: %+v", result)
		}
	}
}

func TestInterpreterCaseEvaluationDoesNotUseExpectedIntentAsValidatorProposal(t *testing.T) {
	report := EvaluateInterpreterCases([]InterpreterCase{{
		ID:               "local_unknown_positive_expected_intent",
		Category:         "payer_cpf",
		Phase:            string(ConversationPhaseBooked),
		ActivePromptKind: string(ActivePromptPayerCPF),
		CurrentTurn:      "texto sem cpf",
		ExpectedIntent:   string(IntentPaymentCreate),
		ExpectedAction:   "tool",
		Source:           "unit_test",
	}}, interpreterCaseEvaluationObservedAt())

	result := interpreterCaseEvaluationByID(t, report, "local_unknown_positive_expected_intent")
	if result.LocalIntent != string(StructuredIntentUnknown) {
		t.Fatalf("test case must exercise a local UNKNOWN miss, got %+v", result)
	}
	if result.ValidatorProposalIntent != result.LocalIntent {
		t.Fatalf("validator proposal must be the local interpreter output, got %+v", result)
	}
	if result.ValidatorProbeEvaluated {
		t.Fatalf("expected no validator probe without forbidden intents, got %+v", result)
	}
	if result.Passed {
		t.Fatalf("positive expected intent must fail when local interpreter returns UNKNOWN: %+v", result)
	}
	if result.EffectiveIntent == string(IntentPaymentCreate) {
		t.Fatalf("expected intent must not be fabricated into the effective intent: %+v", result)
	}
	if !strings.Contains(strings.Join(result.Failures, " "), "expected effective intent PAYMENT_CREATE") {
		t.Fatalf("expected failure to report the local intent miss, got %+v", result.Failures)
	}
}

func TestInterpreterCaseEvaluationPositiveCaseUsesLocalIntent(t *testing.T) {
	report := EvaluateInterpreterCases(loadInterpreterCasesForTest(t), interpreterCaseEvaluationObservedAt())
	result := interpreterCaseEvaluationByID(t, report, "availability_option_001")

	if !result.Passed {
		t.Fatalf("expected positive local case to pass, failures=%v result=%+v", result.Failures, result)
	}
	if result.LocalIntent != string(StructuredIntentSelectAvailabilityOption) {
		t.Fatalf("expected positive case to be produced by local interpreter, got %+v", result)
	}
	if result.ValidatorProposalIntent != result.LocalIntent {
		t.Fatalf("validator proposal must use the local positive intent, got %+v", result)
	}
}

func TestInterpreterCaseEvaluationValidatorProbeDoesNotChangeMainPassFail(t *testing.T) {
	base := InterpreterCase{
		ID:               "local_miss_without_probe",
		Category:         "payer_cpf",
		Phase:            string(ConversationPhaseBooked),
		ActivePromptKind: string(ActivePromptPayerCPF),
		CurrentTurn:      "texto sem cpf",
		ExpectedIntent:   string(IntentPaymentCreate),
		ExpectedAction:   "tool",
		Source:           "unit_test",
	}
	withProbe := base
	withProbe.ID = "local_miss_with_probe"
	withProbe.ForbiddenIntents = []string{string(IntentPaymentCreate)}

	report := EvaluateInterpreterCases([]InterpreterCase{base, withProbe}, interpreterCaseEvaluationObservedAt())
	withoutProbe := interpreterCaseEvaluationByID(t, report, "local_miss_without_probe")
	probe := interpreterCaseEvaluationByID(t, report, "local_miss_with_probe")

	if withoutProbe.ValidatorProbeEvaluated {
		t.Fatalf("did not expect probe on base case: %+v", withoutProbe)
	}
	if !probe.ValidatorProbeEvaluated {
		t.Fatalf("expected validator probe when forbidden intents are present: %+v", probe)
	}
	if probe.ValidatorProbeFallbackTemplate == "" {
		t.Fatalf("expected probe result to be reported separately, got %+v", probe)
	}
	if withoutProbe.Passed || probe.Passed {
		t.Fatalf("both local misses should fail as positive cases, without=%+v with=%+v", withoutProbe, probe)
	}
	if probe.EffectiveIntent != withoutProbe.EffectiveIntent ||
		probe.EffectiveTemplate != withoutProbe.EffectiveTemplate ||
		probe.EffectiveAction != withoutProbe.EffectiveAction {
		t.Fatalf("validator probe changed main result, without=%+v with=%+v", withoutProbe, probe)
	}
}

func interpreterCaseEvaluationObservedAt() time.Time {
	return time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
}

func interpreterCaseEvaluationByID(t *testing.T, report InterpreterCaseEvaluationReport, id string) InterpreterCaseEvaluationResult {
	t.Helper()
	for _, result := range report.Results {
		if result.CaseID == id {
			return result
		}
	}
	t.Fatalf("expected evaluation result for case %q", id)
	return InterpreterCaseEvaluationResult{}
}
