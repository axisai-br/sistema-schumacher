package chat

import "testing"

func TestStructuredInterpreterShadowReportItemsFromMessagesIgnoresMessagesWithoutShadow(t *testing.T) {
	items := StructuredInterpreterShadowReportItemsFromMessages([]Message{
		{ID: "no-payload"},
		{ID: "other-normalized", NormalizedPayload: map[string]interface{}{"other": "value"}},
		{ID: "other-payload", Payload: map[string]interface{}{"other": "value"}},
	})

	if len(items) != 0 {
		t.Fatalf("expected no report items, got %+v", items)
	}
}

func TestStructuredInterpreterShadowReportItemFromMessageMapsValidShadow(t *testing.T) {
	item, ok := StructuredInterpreterShadowReportItemFromMessage(Message{
		ID: "shadow-valid",
		NormalizedPayload: map[string]interface{}{
			structuredInterpreterShadowKey: map[string]interface{}{
				"local": map[string]interface{}{
					"intent":       string(StructuredIntentAvailabilitySearch),
					"turn_meaning": string(TurnMeaningNewRequest),
					"confidence":   0.82,
					"source":       "local_structured",
				},
				"openai": map[string]interface{}{
					"status":               string(StructuredInterpreterShadowValid),
					"intent":               string(StructuredIntentAvailabilitySearch),
					"turn_meaning":         string(TurnMeaningNewRequest),
					"confidence":           0.91,
					"source":               "openai_structured",
					"provider_response_id": "resp_shadow_1",
					"latency_ms":           float64(237),
				},
				"agreement": map[string]interface{}{
					"intent":                true,
					"turn_meaning":          true,
					"selected_option_index": true,
					"passenger_count":       false,
					"child_under_5_count":   true,
					"document_like_text":    true,
					"payment_preference":    false,
				},
			},
		},
	})

	if !ok {
		t.Fatal("expected report item")
	}
	if item.Summary.Local.Intent != string(StructuredIntentAvailabilitySearch) ||
		item.Summary.Local.TurnMeaning != string(TurnMeaningNewRequest) ||
		item.Summary.Local.Confidence != 0.82 ||
		item.Summary.Local.Source != "local_structured" {
		t.Fatalf("unexpected local summary: %+v", item.Summary.Local)
	}
	if item.Summary.OpenAI.Status != string(StructuredInterpreterShadowValid) ||
		item.Summary.OpenAI.Intent != string(StructuredIntentAvailabilitySearch) ||
		item.Summary.OpenAI.TurnMeaning != string(TurnMeaningNewRequest) ||
		item.Summary.OpenAI.Confidence != 0.91 ||
		item.Summary.OpenAI.Source != "openai_structured" ||
		item.Summary.OpenAI.ProviderResponseID != "resp_shadow_1" ||
		item.Summary.OpenAI.LatencyMs != 237 {
		t.Fatalf("unexpected OpenAI summary: %+v", item.Summary.OpenAI)
	}
	if !item.Summary.Agreement.Intent ||
		!item.Summary.Agreement.TurnMeaning ||
		!item.Summary.Agreement.SelectedOptionIndex ||
		item.Summary.Agreement.PassengerCount ||
		!item.Summary.Agreement.ChildUnder5Count ||
		!item.Summary.Agreement.DocumentLikeText ||
		item.Summary.Agreement.PaymentPreference {
		t.Fatalf("unexpected agreement: %+v", item.Summary.Agreement)
	}
	if item.SensitiveScanPayload != nil {
		t.Fatalf("loader must not expose sensitive scan payload by default, got %+v", item.SensitiveScanPayload)
	}
}

func TestStructuredInterpreterShadowReportItemFromMessagePreservesMessageValidationErrors(t *testing.T) {
	message := Message{
		ID: "shadow-message-validation-errors",
		NormalizedPayload: map[string]interface{}{
			"validation_errors": []interface{}{"json_decision_invalid", " "},
			structuredInterpreterShadowKey: map[string]interface{}{
				"local": map[string]interface{}{
					"intent": string(StructuredIntentGreeting),
				},
				"openai": map[string]interface{}{
					"status": string(StructuredInterpreterShadowValid),
					"intent": string(StructuredIntentGreeting),
				},
				"agreement": map[string]interface{}{
					"intent": true,
				},
			},
		},
	}

	item, ok := StructuredInterpreterShadowReportItemFromMessage(message)
	if !ok {
		t.Fatal("expected report item")
	}
	if len(item.ValidationErrors) != 1 || item.ValidationErrors[0] != "json_decision_invalid" {
		t.Fatalf("expected message-level validation error to be preserved, got %+v", item.ValidationErrors)
	}

	report := BuildStructuredInterpreterShadowReportFromMessages([]Message{message})
	if report.ValidationErrorCount != 1 {
		t.Fatalf("expected validation error count 1, got %+v", report)
	}
}

func TestStructuredInterpreterShadowReportItemFromMessageFallsBackToPayload(t *testing.T) {
	item, ok := StructuredInterpreterShadowReportItemFromMessage(Message{
		ID: "shadow-payload",
		NormalizedPayload: map[string]interface{}{
			structuredInterpreterShadowKey: "malformed",
		},
		Payload: map[string]interface{}{
			structuredInterpreterShadowKey: map[string]interface{}{
				"openai": map[string]interface{}{
					"status": string(StructuredInterpreterShadowOpenAIDisabled),
				},
			},
		},
	})

	if !ok {
		t.Fatal("expected fallback report item")
	}
	if item.Summary.OpenAI.Status != string(StructuredInterpreterShadowOpenAIDisabled) {
		t.Fatalf("expected fallback payload status, got %+v", item.Summary.OpenAI)
	}
}

func TestStructuredInterpreterShadowReportItemFromMessageKeepsValidationErrorsWhenFallingBackToPayload(t *testing.T) {
	message := Message{
		ID: "shadow-payload-with-normalized-validation-errors",
		NormalizedPayload: map[string]interface{}{
			"validation_errors":            []string{"json_decision_invalid"},
			structuredInterpreterShadowKey: "malformed",
		},
		Payload: map[string]interface{}{
			structuredInterpreterShadowKey: map[string]interface{}{
				"openai": map[string]interface{}{
					"status": string(StructuredInterpreterShadowOpenAIDisabled),
				},
			},
		},
	}

	item, ok := StructuredInterpreterShadowReportItemFromMessage(message)
	if !ok {
		t.Fatal("expected fallback report item")
	}
	if len(item.ValidationErrors) != 1 || item.ValidationErrors[0] != "json_decision_invalid" {
		t.Fatalf("expected normalized payload validation error to be preserved, got %+v", item.ValidationErrors)
	}

	report := BuildStructuredInterpreterShadowReportFromMessages([]Message{message})
	if report.ValidationErrorCount != 1 {
		t.Fatalf("expected validation error count 1, got %+v", report)
	}
}

func TestStructuredInterpreterShadowReportItemFromMessageHandlesMalformedPayload(t *testing.T) {
	for _, message := range []Message{
		{ID: "shadow-string", NormalizedPayload: map[string]interface{}{structuredInterpreterShadowKey: "malformed"}},
		{ID: "shadow-empty-map", NormalizedPayload: map[string]interface{}{structuredInterpreterShadowKey: map[string]interface{}{}}},
		{
			ID: "shadow-malformed-nested",
			NormalizedPayload: map[string]interface{}{
				structuredInterpreterShadowKey: map[string]interface{}{
					"local":     "not-a-map",
					"openai":    "not-a-map",
					"agreement": "not-a-map",
				},
			},
		},
	} {
		t.Run(message.ID, func(t *testing.T) {
			if item, ok := StructuredInterpreterShadowReportItemFromMessage(message); ok {
				t.Fatalf("expected malformed shadow to be ignored, got %+v", item)
			}
		})
	}
}

func TestStructuredInterpreterShadowReportItemFromMessagePreservesErrorAndLatencyVariants(t *testing.T) {
	cases := []struct {
		name  string
		raw   interface{}
		want  int64
		error string
	}{
		{name: "int", raw: 120, want: 120, error: "openai_structured_interpreter_request_failed"},
		{name: "float", raw: float64(121.9), want: 121, error: "openai_structured_interpreter_invalid_output"},
		{name: "string", raw: "122", want: 122, error: "context_deadline_exceeded"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, ok := StructuredInterpreterShadowReportItemFromMessage(Message{
				ID: "shadow-error-" + tc.name,
				NormalizedPayload: map[string]interface{}{
					structuredInterpreterShadowKey: map[string]interface{}{
						"openai": map[string]interface{}{
							"status":     string(StructuredInterpreterShadowError),
							"latency_ms": tc.raw,
							"error_code": tc.error,
						},
					},
				},
			})

			if !ok {
				t.Fatal("expected report item")
			}
			if item.Summary.OpenAI.Status != string(StructuredInterpreterShadowError) {
				t.Fatalf("expected error status, got %+v", item.Summary.OpenAI)
			}
			if item.Summary.OpenAI.ErrorCode != tc.error {
				t.Fatalf("expected error code %q, got %+v", tc.error, item.Summary.OpenAI)
			}
			if item.Summary.OpenAI.LatencyMs != tc.want {
				t.Fatalf("expected latency %d, got %+v", tc.want, item.Summary.OpenAI)
			}
		})
	}
}

func TestBuildStructuredInterpreterShadowReportFromMessagesUsesLoadedItems(t *testing.T) {
	report := BuildStructuredInterpreterShadowReportFromMessages([]Message{
		{ID: "no-shadow"},
		{
			ID: "agreement",
			NormalizedPayload: map[string]interface{}{
				structuredInterpreterShadowKey: map[string]interface{}{
					"local":  map[string]interface{}{"intent": string(StructuredIntentGreeting)},
					"openai": map[string]interface{}{"status": string(StructuredInterpreterShadowValid), "intent": string(StructuredIntentGreeting), "latency_ms": "100"},
					"agreement": map[string]interface{}{
						"intent": true,
					},
				},
			},
		},
		{
			ID: "disagreement",
			NormalizedPayload: map[string]interface{}{
				structuredInterpreterShadowKey: map[string]interface{}{
					"local":  map[string]interface{}{"intent": string(StructuredIntentAvailabilitySearch)},
					"openai": map[string]interface{}{"status": string(StructuredInterpreterShadowValid), "intent": string(StructuredIntentPaymentPreference), "latency_ms": 200},
					"agreement": map[string]interface{}{
						"intent": false,
					},
				},
			},
		},
	})

	if report.TotalItems != 2 {
		t.Fatalf("expected 2 loaded items, got %+v", report)
	}
	if report.IntentAgreementCount != 1 || report.IntentDisagreementCount != 1 || report.IntentAgreementRate != 0.5 {
		t.Fatalf("expected one agreement and one disagreement, got %+v", report)
	}
	if report.LatencyCount != 2 || report.LatencyMaxMs != 200 || report.LatencyAvgMs != 150 {
		t.Fatalf("expected latency metrics from loaded items, got %+v", report)
	}
}

func TestStructuredInterpreterShadowReportLoaderDoesNotUseMessageBodyAsSensitivePayload(t *testing.T) {
	report := BuildStructuredInterpreterShadowReportFromMessages([]Message{
		{
			ID:   "sensitive-body",
			Body: "Meu CPF e 529.982.247-25 e RG 123456",
			NormalizedPayload: map[string]interface{}{
				"current_turn_body": "Meu CPF e 529.982.247-25",
				structuredInterpreterShadowKey: map[string]interface{}{
					"local":  map[string]interface{}{"intent": string(StructuredIntentPassengerDocumentsProvided)},
					"openai": map[string]interface{}{"status": string(StructuredInterpreterShadowOpenAIDisabled)},
				},
			},
		},
	})

	if report.TotalItems != 1 {
		t.Fatalf("expected report item, got %+v", report)
	}
	if report.SensitiveLeakDetected || report.SensitiveLeakCount != 0 {
		t.Fatalf("loader must not scan body/current_turn as sensitive payload by default, got %+v", report)
	}
}
