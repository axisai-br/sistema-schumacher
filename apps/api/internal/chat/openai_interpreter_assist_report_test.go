package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildOpenAIInterpreterAssistReportAggregatesStatusesAndDimensions(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReport([]OpenAIInterpreterAssistReportItem{
		{
			Status:              string(OpenAIInterpreterAssistAccepted),
			ValidationStatus:    string(OpenAIInterpreterAssistValidationAccepted),
			Considered:          true,
			HasConsidered:       true,
			Accepted:            true,
			LocalIntent:         string(StructuredIntentUnknown),
			OpenAIIntent:        string(StructuredIntentPassengerCountReply),
			OpenAIConfidence:    0.72,
			HasOpenAIConfidence: true,
			TemplateName:        string(TemplateAskPassengerCount),
			Action:              "booking_continuation",
			Source:              "openai_interpreter_assist",
		},
		{
			Status:              string(OpenAIInterpreterAssistRejected),
			ValidationStatus:    string(OpenAIInterpreterAssistValidationRejected),
			Considered:          true,
			HasConsidered:       true,
			LocalIntent:         string(StructuredIntentAvailabilitySearch),
			LocalConfidence:     0.52,
			HasLocalConfidence:  true,
			OpenAIIntent:        string(StructuredIntentPaymentPreference),
			OpenAIConfidence:    0.62,
			HasOpenAIConfidence: true,
			RejectReason:        "intent_not_allowed_by_active_prompt",
			ReasonCodes:         []string{"intent_not_allowed_by_active_prompt"},
			FallbackTemplate:    string(TemplateContextFallbackPaymentPreference),
			Source:              "openai_interpreter_assist",
		},
		{
			Status:        string(OpenAIInterpreterAssistRejected),
			Considered:    true,
			HasConsidered: true,
			OpenAIIntent:  string(StructuredIntentAvailabilitySearch),
			RejectReason:  "openai_assist_tool_action_not_allowed",
			ReasonCodes:   []string{"openai_assist_tool_action_not_allowed"},
		},
		{
			Status:        string(OpenAIInterpreterAssistRejected),
			Considered:    true,
			HasConsidered: true,
			OpenAIIntent:  string(StructuredIntentBookingCancelRequest),
			RejectReason:  "booking_cancel_not_allowed_in_openai_assist",
			ReasonCodes:   []string{"booking_cancel_not_allowed_in_openai_assist"},
		},
		{
			Status:        string(OpenAIInterpreterAssistSkipped),
			HasConsidered: true,
			RejectReason:  "assist_disabled",
			SkipReason:    "assist_disabled",
			ReasonCodes:   []string{"assist_disabled"},
			Source:        "local",
		},
		{
			Status: "unexpected_status",
		},
		{
			Malformed: true,
		},
	})

	if report.Total != 7 ||
		report.Considered != 4 ||
		report.NotConsidered != 1 ||
		report.Accepted != 1 ||
		report.Rejected != 3 ||
		report.Skipped != 1 ||
		report.UnknownStatusCount != 1 ||
		report.MalformedCount != 1 {
		t.Fatalf("unexpected assist totals: %+v", report)
	}
	if report.ByStatus[string(OpenAIInterpreterAssistAccepted)] != 1 ||
		report.ByStatus[string(OpenAIInterpreterAssistRejected)] != 3 ||
		report.ByStatus[string(OpenAIInterpreterAssistSkipped)] != 1 ||
		report.ByStatus[structuredInterpreterShadowRedactedMetricKey] != 1 {
		t.Fatalf("unexpected status groups: %+v", report.ByStatus)
	}
	if report.ByIntent[string(StructuredIntentPassengerCountReply)].Accepted != 1 ||
		report.ByIntent[string(StructuredIntentPaymentPreference)].Rejected != 1 ||
		report.ByTemplate[string(TemplateAskPassengerCount)].Accepted != 1 ||
		report.ByAction["booking_continuation"].Accepted != 1 ||
		report.BySource["openai_interpreter_assist"].Accepted != 1 ||
		report.BySource["openai_interpreter_assist"].Rejected != 1 ||
		report.ByConfidenceBucket["0.70-0.84"].Accepted != 1 ||
		report.ByConfidenceBucket["0.50-0.69"].Rejected != 1 {
		t.Fatalf("unexpected assist dimensions: %+v", report)
	}
	if report.ByReason["intent_not_allowed_by_active_prompt"] != 1 ||
		report.ByReasonStatus["intent_not_allowed_by_active_prompt"].Rejected != 1 ||
		report.ByRejectReason["intent_not_allowed_by_active_prompt"] != 1 ||
		report.BySkipReason["assist_disabled"] != 1 ||
		report.ByFallbackTemplate[string(TemplateContextFallbackPaymentPreference)] != 1 {
		t.Fatalf("unexpected reason metrics: %+v", report)
	}
	if report.BlockedToolActionCount != 1 || report.BlockedCriticalIntentCount != 1 {
		t.Fatalf("unexpected safety block counters: %+v", report)
	}
	if report.LocalUnknownCount != 1 || report.LocalLowConfidenceCount != 1 {
		t.Fatalf("unexpected local weakness counters: %+v", report)
	}
}

func TestBuildOpenAIInterpreterAssistReportDoesNotCountMissingConsideredAsFalse(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReport([]OpenAIInterpreterAssistReportItem{
		{
			Status:       string(OpenAIInterpreterAssistRejected),
			RejectReason: "openai_confidence_below_threshold",
			ReasonCodes:  []string{"openai_confidence_below_threshold"},
		},
	})

	if report.Total != 1 || report.Rejected != 1 || report.Considered != 0 || report.NotConsidered != 0 {
		t.Fatalf("expected missing considered to be excluded from true/false counters, got %+v", report)
	}
	if report.ByReason["openai_confidence_below_threshold"] != 1 {
		t.Fatalf("expected older payload reason to remain counted, got %+v", report.ByReason)
	}
}

func TestBuildOpenAIInterpreterAssistReportRedactsSensitiveMetricKeys(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReport([]OpenAIInterpreterAssistReportItem{
		{
			Status:              string(OpenAIInterpreterAssistRejected),
			ValidationStatus:    "already paid the pix",
			OpenAIIntent:        "Joao Vitor Messias",
			OpenAIConfidence:    0.8,
			HasOpenAIConfidence: true,
			RejectReason:        "telefone_(48)99999-9999",
			SkipReason:          "12.345.678-9",
			ReasonCodes: []string{
				"telefone_(48)99999-9999",
				"booking_550e8400-e29b-41d4-a716-446655440000",
			},
			FallbackTemplate: "booking_550e8400-e29b-41d4-a716-446655440000",
			TemplateName:     "data:image/png;base64,abc",
			Action:           "run payment_create now",
			Source:           "Joao Vitor Messias",
		},
	})

	if report.ByValidationStatus[structuredInterpreterShadowRedactedMetricKey].Rejected != 1 ||
		report.ByIntent[structuredInterpreterShadowRedactedMetricKey].Rejected != 1 ||
		report.ByReason[structuredInterpreterShadowRedactedMetricKey] != 3 ||
		report.ByRejectReason[structuredInterpreterShadowRedactedMetricKey] != 1 ||
		report.BySkipReason[structuredInterpreterShadowRedactedMetricKey] != 1 ||
		report.ByFallbackTemplate[structuredInterpreterShadowRedactedMetricKey] != 1 ||
		report.ByTemplate[structuredInterpreterShadowRedactedMetricKey].Rejected != 1 ||
		report.ByAction[structuredInterpreterShadowRedactedMetricKey].Rejected != 1 ||
		report.BySource[structuredInterpreterShadowRedactedMetricKey].Rejected != 1 {
		t.Fatalf("expected sensitive dimensions to be redacted, got %+v", report)
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{
		"telefone_(48)99999-9999",
		"12.345.678-9",
		"550e8400-e29b-41d4-a716-446655440000",
		"Joao Vitor Messias",
		"data:image",
		"run payment_create now",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("report leaked forbidden value %q in %s", forbidden, body)
		}
	}
}

func TestBuildOpenAIInterpreterAssistReportKeepsKnownValidatorRejectReasons(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReport([]OpenAIInterpreterAssistReportItem{
		{
			Status:       string(OpenAIInterpreterAssistRejected),
			RejectReason: "unsafe_side_effect_claim",
			ReasonCodes:  []string{"unsafe_side_effect_claim"},
		},
	})

	if report.ByReason["unsafe_side_effect_claim"] != 1 ||
		report.ByReasonStatus["unsafe_side_effect_claim"].Rejected != 1 ||
		report.ByRejectReason["unsafe_side_effect_claim"] != 1 {
		t.Fatalf("expected known validator reason to stay granular, got %+v", report)
	}
}

func TestOpenAIInterpreterAssistReportDoesNotAlterRuntimeDecision(t *testing.T) {
	input := OpenAIInterpreterAssistInput{
		Enabled: false,
		LocalInterpretation: StructuredInterpretation{
			Intent:     StructuredIntentUnknown,
			Confidence: 0.4,
		},
	}

	before := RunOpenAIInterpreterRuntimeAssist(nil, input)
	_ = BuildOpenAIInterpreterAssistReportFromMessages([]Message{
		{
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: before.Metadata(),
			},
		},
	})
	after := RunOpenAIInterpreterRuntimeAssist(nil, input)

	if before.Status != after.Status ||
		before.Considered != after.Considered ||
		before.RejectReason != after.RejectReason ||
		before.DecisionSource != after.DecisionSource {
		t.Fatalf("reporting must not alter runtime decision: before=%+v after=%+v", before, after)
	}
}
