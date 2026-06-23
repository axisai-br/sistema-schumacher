package chat

import (
	"math"
	"testing"
)

func TestBuildStructuredInterpreterShadowReportCountsIntentAgreement(t *testing.T) {
	items := []StructuredInterpreterShadowReportItem{
		{
			Summary: structuredInterpreterShadowReportTestSummary(
				string(StructuredIntentGreeting),
				string(StructuredIntentGreeting),
				true,
			),
		},
		{
			Summary: structuredInterpreterShadowReportTestSummary(
				string(StructuredIntentAvailabilitySearch),
				string(StructuredIntentPassengerCountReply),
				false,
			),
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				Local:  StructuredInterpreterSummary{Intent: string(StructuredIntentGreeting)},
				OpenAI: OpenAIInterpreterShadowSummary{Status: string(StructuredInterpreterShadowOpenAIDisabled)},
			},
		},
	}

	report := BuildStructuredInterpreterShadowReport(items)

	if report.TotalItems != 3 {
		t.Fatalf("expected total items 3, got %d", report.TotalItems)
	}
	if report.LocalIntentCounts[string(StructuredIntentGreeting)] != 2 {
		t.Fatalf("expected greeting local count 2, got %d", report.LocalIntentCounts[string(StructuredIntentGreeting)])
	}
	if report.LocalIntentCounts[string(StructuredIntentAvailabilitySearch)] != 1 {
		t.Fatalf("expected availability local count 1, got %d", report.LocalIntentCounts[string(StructuredIntentAvailabilitySearch)])
	}
	if report.OpenAIIntentCounts[string(StructuredIntentGreeting)] != 1 {
		t.Fatalf("expected greeting OpenAI count 1, got %d", report.OpenAIIntentCounts[string(StructuredIntentGreeting)])
	}
	if report.OpenAIIntentCounts[string(StructuredIntentPassengerCountReply)] != 1 {
		t.Fatalf("expected passenger count OpenAI count 1, got %d", report.OpenAIIntentCounts[string(StructuredIntentPassengerCountReply)])
	}
	if report.IntentAgreementCount != 1 {
		t.Fatalf("expected agreement count 1, got %d", report.IntentAgreementCount)
	}
	if report.IntentDisagreementCount != 1 {
		t.Fatalf("expected disagreement count 1, got %d", report.IntentDisagreementCount)
	}
	if report.IntentAgreementRate != 0.5 {
		t.Fatalf("expected agreement rate 0.5, got %f", report.IntentAgreementRate)
	}
}

func TestBuildStructuredInterpreterShadowReportGroupsDisagreements(t *testing.T) {
	items := []StructuredInterpreterShadowReportItem{
		{
			Summary: structuredInterpreterShadowReportTestSummary(
				string(StructuredIntentAvailabilitySearch),
				string(StructuredIntentPassengerCountReply),
				false,
			),
		},
		{
			Summary: structuredInterpreterShadowReportTestSummary(
				string(StructuredIntentAvailabilitySearch),
				string(StructuredIntentPaymentPreference),
				false,
			),
		},
		{
			Summary: structuredInterpreterShadowReportTestSummary(
				string(StructuredIntentGreeting),
				string(StructuredIntentPaymentPreference),
				false,
			),
		},
	}

	report := BuildStructuredInterpreterShadowReport(items)

	if report.DisagreementsByLocalIntent[string(StructuredIntentAvailabilitySearch)] != 2 {
		t.Fatalf("expected availability disagreements 2, got %d", report.DisagreementsByLocalIntent[string(StructuredIntentAvailabilitySearch)])
	}
	if report.DisagreementsByLocalIntent[string(StructuredIntentGreeting)] != 1 {
		t.Fatalf("expected greeting disagreements 1, got %d", report.DisagreementsByLocalIntent[string(StructuredIntentGreeting)])
	}
	if report.DisagreementsByOpenAIIntent[string(StructuredIntentPaymentPreference)] != 2 {
		t.Fatalf("expected payment OpenAI disagreements 2, got %d", report.DisagreementsByOpenAIIntent[string(StructuredIntentPaymentPreference)])
	}
	if report.DisagreementsByOpenAIIntent[string(StructuredIntentPassengerCountReply)] != 1 {
		t.Fatalf("expected passenger count OpenAI disagreements 1, got %d", report.DisagreementsByOpenAIIntent[string(StructuredIntentPassengerCountReply)])
	}
}

func TestBuildStructuredInterpreterShadowReportCountsOpenAIStatusAndErrors(t *testing.T) {
	items := []StructuredInterpreterShadowReportItem{
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAI: OpenAIInterpreterShadowSummary{
					Status:    string(StructuredInterpreterShadowError),
					ErrorCode: "openai_structured_interpreter_request_failed",
					LatencyMs: 120,
				},
			},
			ValidationErrors: []string{"invalid_intent"},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAI: OpenAIInterpreterShadowSummary{
					Status:    string(StructuredInterpreterShadowValid),
					ErrorCode: "openai_structured_interpreter_invalid_output",
					LatencyMs: 280,
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAI: OpenAIInterpreterShadowSummary{
					Status: string(StructuredInterpreterShadowError),
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAI: OpenAIInterpreterShadowSummary{
					Status: string(StructuredInterpreterShadowOpenAIDisabled),
				},
			},
		},
	}

	report := BuildStructuredInterpreterShadowReport(items)

	if report.OpenAIStatusCounts[string(StructuredInterpreterShadowError)] != 2 {
		t.Fatalf("expected error status count 2, got %d", report.OpenAIStatusCounts[string(StructuredInterpreterShadowError)])
	}
	if report.OpenAIStatusCounts[string(StructuredInterpreterShadowValid)] != 1 {
		t.Fatalf("expected valid status count 1, got %d", report.OpenAIStatusCounts[string(StructuredInterpreterShadowValid)])
	}
	if report.OpenAIErrorCount != 3 {
		t.Fatalf("expected OpenAI error count 3, got %d", report.OpenAIErrorCount)
	}
	if report.ValidationErrorCount != 2 {
		t.Fatalf("expected validation error count 2, got %d", report.ValidationErrorCount)
	}
	if report.LatencyCount != 3 {
		t.Fatalf("expected latency count 3, got %d", report.LatencyCount)
	}
	if math.Abs(report.LatencyAvgMs-133.33333333333334) > 0.000001 {
		t.Fatalf("expected latency avg 133.33333333333334, got %f", report.LatencyAvgMs)
	}
	if report.LatencyMaxMs != 280 {
		t.Fatalf("expected latency max 280, got %d", report.LatencyMaxMs)
	}
}

func TestBuildStructuredInterpreterShadowReportDetectsSensitiveLeakKeys(t *testing.T) {
	items := []StructuredInterpreterShadowReportItem{
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent: string(StructuredIntentPassengerDocumentsProvided),
				},
				OpenAI: OpenAIInterpreterShadowSummary{
					Status: string(StructuredInterpreterShadowValid),
					Intent: string(StructuredIntentPassengerDocumentsProvided),
				},
				Agreement: StructuredInterpreterAgreement{Intent: true},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAI: OpenAIInterpreterShadowSummary{Status: string(StructuredInterpreterShadowValid)},
			},
			SensitiveScanPayload: map[any]any{
				"raw_output": "redacted",
			},
		},
		{
			SensitiveScanPayload: struct {
				RequestPayload string
			}{
				RequestPayload: "redacted",
			},
		},
	}

	report := BuildStructuredInterpreterShadowReport(items)

	if !report.SensitiveLeakDetected {
		t.Fatalf("expected sensitive leak to be detected")
	}
	if report.SensitiveLeakCount != 2 {
		t.Fatalf("expected sensitive leak count 2, got %d", report.SensitiveLeakCount)
	}
}

func TestBuildStructuredInterpreterShadowReportDetectsSensitiveLeakValues(t *testing.T) {
	items := []StructuredInterpreterShadowReportItem{
		{
			SensitiveScanPayload: []any{
				"safe",
				map[string]any{"media": "data:image/png;base64,AAAA"},
			},
		},
		{
			SensitiveScanPayload: "Meu CPF e 529.982.247-25",
		},
		{
			SensitiveScanPayload: map[string]any{
				"external_id": "12345678901",
			},
		},
		{
			SensitiveScanPayload: map[string]any{
				"intent": string(StructuredIntentPassengerDocumentsProvided),
			},
		},
	}

	report := BuildStructuredInterpreterShadowReport(items)

	if !report.SensitiveLeakDetected {
		t.Fatalf("expected sensitive leak to be detected")
	}
	if report.SensitiveLeakCount != 3 {
		t.Fatalf("expected sensitive leak count 3, got %d", report.SensitiveLeakCount)
	}
}

func TestBuildStructuredInterpreterShadowReportHandlesEmptyInput(t *testing.T) {
	report := BuildStructuredInterpreterShadowReport(nil)

	if report.TotalItems != 0 {
		t.Fatalf("expected total items 0, got %d", report.TotalItems)
	}
	if len(report.OpenAIStatusCounts) != 0 {
		t.Fatalf("expected empty OpenAI status counts, got %#v", report.OpenAIStatusCounts)
	}
	if len(report.LocalIntentCounts) != 0 {
		t.Fatalf("expected empty local intent counts, got %#v", report.LocalIntentCounts)
	}
	if len(report.OpenAIIntentCounts) != 0 {
		t.Fatalf("expected empty OpenAI intent counts, got %#v", report.OpenAIIntentCounts)
	}
	if len(report.DisagreementsByLocalIntent) != 0 {
		t.Fatalf("expected empty local disagreement counts, got %#v", report.DisagreementsByLocalIntent)
	}
	if len(report.DisagreementsByOpenAIIntent) != 0 {
		t.Fatalf("expected empty OpenAI disagreement counts, got %#v", report.DisagreementsByOpenAIIntent)
	}
	if report.IntentAgreementRate != 0 {
		t.Fatalf("expected agreement rate 0, got %f", report.IntentAgreementRate)
	}
	if report.LatencyCount != 0 || report.LatencyAvgMs != 0 || report.LatencyMaxMs != 0 {
		t.Fatalf("expected empty latency metrics, got count=%d avg=%f max=%d", report.LatencyCount, report.LatencyAvgMs, report.LatencyMaxMs)
	}
	if report.SensitiveLeakDetected || report.SensitiveLeakCount != 0 {
		t.Fatalf("expected no sensitive leak, got detected=%t count=%d", report.SensitiveLeakDetected, report.SensitiveLeakCount)
	}
}

func structuredInterpreterShadowReportTestSummary(localIntent string, openAIIntent string, agreement bool) StructuredInterpreterShadowSummary {
	return StructuredInterpreterShadowSummary{
		Local: StructuredInterpreterSummary{
			Intent: localIntent,
		},
		OpenAI: OpenAIInterpreterShadowSummary{
			Status: string(StructuredInterpreterShadowValid),
			Intent: openAIIntent,
		},
		Agreement: StructuredInterpreterAgreement{
			Intent: agreement,
		},
	}
}
