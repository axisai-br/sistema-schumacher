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

func TestBuildStructuredInterpreterShadowReportCountsOpenAIValidationMetrics(t *testing.T) {
	items := []StructuredInterpreterShadowReportItem{
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent:      string(StructuredIntentUnknown),
					TurnMeaning: string(TurnMeaningUnknown),
					Source:      "deterministic",
				},
				OpenAI: OpenAIInterpreterShadowSummary{
					Status:      string(StructuredInterpreterShadowValid),
					Intent:      string(StructuredIntentPaymentPreference),
					TurnMeaning: string(TurnMeaningAnswerToQuestion),
					Confidence:  0.72,
					Source:      "openai_structured",
				},
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:   string(StructuredInterpreterShadowValidationAccepted),
					Accepted: true,
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent:      string(StructuredIntentAvailabilitySearch),
					TurnMeaning: string(TurnMeaningNewRequest),
					Source:      "deterministic",
				},
				OpenAI: OpenAIInterpreterShadowSummary{
					Status:      string(StructuredInterpreterShadowValid),
					Intent:      string(StructuredIntentSelectAvailabilityOption),
					TurnMeaning: string(TurnMeaningAnswerToQuestion),
					Confidence:  0.91,
					Source:      "openai_structured",
				},
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:           string(StructuredInterpreterShadowValidationRejected),
					RejectReason:     "availability_selection_missing_current_facts",
					FallbackTemplate: TemplateContextFallbackAvailabilityOption,
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent:      string(StructuredIntentPassengerCountReply),
					TurnMeaning: string(TurnMeaningAnswerToQuestion),
					Source:      "deterministic",
				},
				OpenAI: OpenAIInterpreterShadowSummary{
					Status:      string(StructuredInterpreterShadowValid),
					Intent:      string(StructuredIntentPaymentPreference),
					TurnMeaning: string(TurnMeaningAnswerToQuestion),
					Confidence:  0.62,
					Source:      "openai_structured",
				},
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:           string(StructuredInterpreterShadowValidationRejected),
					RejectReason:     "intent_not_allowed_by_active_prompt",
					FallbackTemplate: TemplateContextFallbackPaymentPreference,
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent:      string(StructuredIntentGreeting),
					TurnMeaning: string(TurnMeaningGreeting),
					Source:      "deterministic",
				},
				OpenAI: OpenAIInterpreterShadowSummary{
					Status: string(StructuredInterpreterShadowOpenAIDisabled),
				},
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationSkipped),
					RejectReason: "openai_disabled",
				},
			},
		},
	}

	report := BuildStructuredInterpreterShadowReport(items)
	validation := report.OpenAIValidation

	if validation.Total != 4 || validation.Accepted != 1 || validation.Rejected != 2 || validation.Skipped != 1 {
		t.Fatalf("unexpected validation status totals: %+v", validation)
	}
	if validation.ByStatus[string(StructuredInterpreterShadowValidationAccepted)] != 1 ||
		validation.ByStatus[string(StructuredInterpreterShadowValidationRejected)] != 2 ||
		validation.ByStatus[string(StructuredInterpreterShadowValidationSkipped)] != 1 {
		t.Fatalf("unexpected validation status counts: %+v", validation.ByStatus)
	}
	if validation.ByLocalIntent[string(StructuredIntentUnknown)].Accepted != 1 ||
		validation.ByLocalIntent[string(StructuredIntentAvailabilitySearch)].Rejected != 1 ||
		validation.ByOpenAIIntent[string(StructuredIntentPaymentPreference)].Accepted != 1 ||
		validation.ByOpenAIIntent[string(StructuredIntentPaymentPreference)].Rejected != 1 {
		t.Fatalf("unexpected validation intent groups: local=%+v openai=%+v", validation.ByLocalIntent, validation.ByOpenAIIntent)
	}
	if validation.ByLocalSource["deterministic"].Accepted != 1 ||
		validation.ByLocalSource["deterministic"].Rejected != 2 ||
		validation.ByLocalSource["deterministic"].Skipped != 1 ||
		validation.ByOpenAISource["openai_structured"].Accepted != 1 ||
		validation.ByOpenAISource["openai_structured"].Rejected != 2 {
		t.Fatalf("unexpected validation source groups: local=%+v openai=%+v", validation.ByLocalSource, validation.ByOpenAISource)
	}
	if validation.ByLocalTurnMeaning[string(TurnMeaningUnknown)].Accepted != 1 ||
		validation.ByOpenAITurnMeaning[string(TurnMeaningAnswerToQuestion)].Accepted != 1 ||
		validation.ByOpenAITurnMeaning[string(TurnMeaningAnswerToQuestion)].Rejected != 2 {
		t.Fatalf("unexpected validation turn meaning groups: local=%+v openai=%+v", validation.ByLocalTurnMeaning, validation.ByOpenAITurnMeaning)
	}
	if validation.ByRejectReason["availability_selection_missing_current_facts"] != 1 ||
		validation.ByRejectReason["intent_not_allowed_by_active_prompt"] != 1 ||
		validation.ByRejectReason["openai_disabled"] != 1 {
		t.Fatalf("unexpected reject reason groups: %+v", validation.ByRejectReason)
	}
	if validation.ByFallbackTemplate[string(TemplateContextFallbackAvailabilityOption)] != 1 ||
		validation.ByFallbackTemplate[string(TemplateContextFallbackPaymentPreference)] != 1 {
		t.Fatalf("unexpected fallback template groups: %+v", validation.ByFallbackTemplate)
	}
	if validation.ByConfidenceBucket["0.70-0.84"].Accepted != 1 ||
		validation.ByConfidenceBucket["0.85-1.00"].Rejected != 1 ||
		validation.ByConfidenceBucket["0.50-0.69"].Rejected != 1 {
		t.Fatalf("unexpected confidence buckets: %+v", validation.ByConfidenceBucket)
	}
	if validation.LocalUnknownOpenAIAccepted != 1 ||
		validation.LocalUnknownAcceptedByOpenAIIntent[string(StructuredIntentPaymentPreference)] != 1 {
		t.Fatalf("expected local UNKNOWN accepted metric, got %+v", validation)
	}
	if validation.IntentMismatchOpenAIAccepted != 1 || validation.IntentMismatchOpenAIRejected != 2 {
		t.Fatalf("unexpected intent mismatch metrics: %+v", validation)
	}
	if validation.RejectedByMissingCurrentFactsCount != 1 || validation.RejectedByActivePromptCount != 1 {
		t.Fatalf("unexpected rejected category metrics: %+v", validation)
	}
}

func TestBuildStructuredInterpreterShadowReportHandlesUnknownOpenAIValidationStatus(t *testing.T) {
	report := BuildStructuredInterpreterShadowReport([]StructuredInterpreterShadowReportItem{
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent: string(StructuredIntentGreeting),
				},
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status: "unexpected_status",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent: string(StructuredIntentGreeting),
				},
			},
		},
	})

	if report.OpenAIValidation.Total != 1 {
		t.Fatalf("expected one validation item, got %+v", report.OpenAIValidation)
	}
	if report.OpenAIValidation.UnknownStatusCount != 1 {
		t.Fatalf("expected unknown status count 1, got %+v", report.OpenAIValidation)
	}
	if report.OpenAIValidation.ByStatus["__redacted_sensitive"] != 1 {
		t.Fatalf("expected unknown status to be redacted, got %+v", report.OpenAIValidation.ByStatus)
	}
}

func TestBuildStructuredInterpreterShadowReportRedactsSensitiveOpenAIValidationMetricKeys(t *testing.T) {
	report := BuildStructuredInterpreterShadowReport([]StructuredInterpreterShadowReportItem{
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Intent: string(StructuredIntentGreeting),
					Source: "Meu CPF e 529.982.247-25",
				},
				OpenAI: OpenAIInterpreterShadowSummary{
					Intent: string(StructuredIntentPaymentPreference),
					Source: "source with raw spaces",
				},
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "telefone_48999999999",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "(48)99999-9999",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "12.345.678-9",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				Local: StructuredInterpreterSummary{
					Source: "550e8400-e29b-41d4-a716-446655440000",
				},
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:           string(StructuredInterpreterShadowValidationRejected),
					FallbackTemplate: ResponseTemplateName("booking_550e8400-e29b-41d4-a716-446655440000"),
				},
			},
		},
	})

	validation := report.OpenAIValidation
	if validation.ByLocalSource["__redacted_sensitive"].Rejected != 2 ||
		validation.ByOpenAISource["__redacted_sensitive"].Rejected != 1 ||
		validation.ByRejectReason["__redacted_sensitive"] != 3 ||
		validation.ByFallbackTemplate["__redacted_sensitive"] != 1 {
		t.Fatalf("expected sensitive metric keys to be redacted, got %+v", validation)
	}
}

func TestBuildStructuredInterpreterShadowReportOpenAIValidationCategoryCountersUseAllowedRejectReasons(t *testing.T) {
	report := BuildStructuredInterpreterShadowReport([]StructuredInterpreterShadowReportItem{
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "malformed_active_prompt_customer_text",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "malformed_missing_current_facts_customer_text",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "intent_not_allowed_by_active_prompt",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "active_prompt_required",
				},
			},
		},
		{
			Summary: StructuredInterpreterShadowSummary{
				OpenAIValidation: OpenAIInterpreterShadowValidation{
					Status:       string(StructuredInterpreterShadowValidationRejected),
					RejectReason: "availability_selection_missing_current_facts",
				},
			},
		},
	})

	validation := report.OpenAIValidation
	if validation.ByRejectReason["__redacted_sensitive"] != 2 {
		t.Fatalf("expected malformed reject reasons to be redacted, got %+v", validation.ByRejectReason)
	}
	if validation.ByRejectReason["intent_not_allowed_by_active_prompt"] != 1 ||
		validation.ByRejectReason["active_prompt_required"] != 1 ||
		validation.ByRejectReason["availability_selection_missing_current_facts"] != 1 {
		t.Fatalf("expected known reject reasons to stay granular, got %+v", validation.ByRejectReason)
	}
	if validation.RejectedByActivePromptCount != 2 {
		t.Fatalf("expected only known active prompt reasons to count, got %+v", validation)
	}
	if validation.RejectedByMissingCurrentFactsCount != 1 {
		t.Fatalf("expected only known missing current facts reason to count, got %+v", validation)
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
