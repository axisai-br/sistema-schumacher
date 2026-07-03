package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"testing"
)

type fakeOpenAIInterpreter struct {
	enabled   bool
	calls     int
	result    OpenAIStructuredInterpreterRunResult
	err       error
	lastInput OpenAIStructuredInterpreterRunInput
}

func (f *fakeOpenAIInterpreter) Enabled() bool {
	return f.enabled
}

func (f *fakeOpenAIInterpreter) InterpretStructuredTurn(ctx context.Context, input OpenAIStructuredInterpreterRunInput) (OpenAIStructuredInterpreterRunResult, error) {
	f.calls++
	f.lastInput = input
	return f.result, f.err
}

func TestRunStructuredInterpreterShadowDisabledDoesNotCallOpenAI(t *testing.T) {
	fake := &fakeOpenAIInterpreter{enabled: true}

	local := StructuredInterpretation{
		Intent:      StructuredIntentGreeting,
		TurnMeaning: TurnMeaningGreeting,
		Confidence:  1,
		Source:      "deterministic_test",
	}

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:             false,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "oi"},
		LocalInterpretation: local,
		IdempotencyKey:      "test-key",
	})

	if fake.calls != 0 {
		t.Fatalf("expected OpenAI not to be called when shadow is disabled, got calls=%d", fake.calls)
	}

	if summary.Local.Intent != string(StructuredIntentGreeting) {
		t.Fatalf("expected local intent %q, got %q", StructuredIntentGreeting, summary.Local.Intent)
	}

	if summary.OpenAI.Status != string(StructuredInterpreterShadowDisabled) {
		t.Fatalf("expected OpenAI status %q, got %q", StructuredInterpreterShadowDisabled, summary.OpenAI.Status)
	}
}

func TestRunStructuredInterpreterShadowSkipReasonDoesNotCallOpenAI(t *testing.T) {
	fake := &fakeOpenAIInterpreter{enabled: true}

	local := StructuredInterpretation{
		Intent:      StructuredIntentUnknown,
		TurnMeaning: TurnMeaningUnknown,
		Confidence:  0,
		Source:      "deterministic_test",
	}

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:             true,
		SkipReason:          "test_skip_reason",
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "paga agora?"},
		LocalInterpretation: local,
		IdempotencyKey:      "test-key",
	})

	if fake.calls != 0 {
		t.Fatalf("expected OpenAI not to be called when shadow has skip reason, got calls=%d", fake.calls)
	}
	if summary.OpenAI.Status != string(StructuredInterpreterShadowSkipped) {
		t.Fatalf("expected OpenAI status %q, got %q", StructuredInterpreterShadowSkipped, summary.OpenAI.Status)
	}
	if summary.OpenAIValidation.Status != string(StructuredInterpreterShadowValidationSkipped) ||
		summary.OpenAIValidation.RejectReason != "test_skip_reason" {
		t.Fatalf("expected skip validation reason, got %+v", summary.OpenAIValidation)
	}
}

func TestRunStructuredInterpreterShadowEnabledButRunnerDisabled(t *testing.T) {
	fake := &fakeOpenAIInterpreter{enabled: false}

	local := StructuredInterpretation{
		Intent:      StructuredIntentGreeting,
		TurnMeaning: TurnMeaningGreeting,
		Confidence:  1,
		Source:      "deterministic_test",
	}

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "oi"},
		LocalInterpretation: local,
		IdempotencyKey:      "test-key",
	})

	if fake.calls != 0 {
		t.Fatalf("expected OpenAI not to be called when runner is disabled, got calls=%d", fake.calls)
	}

	if summary.OpenAI.Status != string(StructuredInterpreterShadowOpenAIDisabled) {
		t.Fatalf("expected OpenAI status %q, got %q", StructuredInterpreterShadowOpenAIDisabled, summary.OpenAI.Status)
	}
}

func TestRunStructuredInterpreterShadowEnabledValidResult(t *testing.T) {
	local := StructuredInterpretation{
		Intent:      StructuredIntentGreeting,
		TurnMeaning: TurnMeaningGreeting,
		Confidence:  1,
		Source:      "deterministic_test",
	}

	openAIInterpretation := StructuredInterpretation{
		Intent:      StructuredIntentGreeting,
		TurnMeaning: TurnMeaningGreeting,
		Confidence:  0.91,
		Source:      "openai_structured",
		Booking: StructuredBookingSlots{
			SelectedOptionIndex:      2,
			SelectedOptionIndexKnown: true,
			PassengerCount:           3,
			PassengerCountKnown:      true,
			ChildUnder5Count:         1,
			ChildUnder5CountKnown:    true,
		},
		PassengerDocument: StructuredPassengerDocumentSlots{DocumentLikeText: true},
		Payment:           StructuredPaymentSlots{PaymentPreference: "pix"},
	}
	local.Booking.SelectedOptionIndex = 2
	local.Booking.SelectedOptionIndexKnown = true
	local.Booking.PassengerCount = 3
	local.Booking.PassengerCountKnown = true
	local.Booking.ChildUnder5Count = 1
	local.Booking.ChildUnder5CountKnown = true
	local.PassengerDocument.DocumentLikeText = true
	local.Payment.PaymentPreference = "PIX"

	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     openAIInterpretation,
			ProviderResponseID: "resp_test_123",
		},
	}

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "oi"},
		LocalInterpretation: local,
		IdempotencyKey:      "test-key",
	})

	if fake.calls != 1 {
		t.Fatalf("expected OpenAI to be called once, got calls=%d", fake.calls)
	}

	if summary.OpenAI.Status != string(StructuredInterpreterShadowValid) {
		t.Fatalf("expected OpenAI status %q, got %q", StructuredInterpreterShadowValid, summary.OpenAI.Status)
	}

	if summary.OpenAI.Intent != string(StructuredIntentGreeting) {
		t.Fatalf("expected OpenAI intent %q, got %q", StructuredIntentGreeting, summary.OpenAI.Intent)
	}

	if summary.OpenAI.ProviderResponseID != "resp_test_123" {
		t.Fatalf("expected provider response id, got %q", summary.OpenAI.ProviderResponseID)
	}

	if !summary.Agreement.Intent {
		t.Fatalf("expected intent agreement")
	}

	if !summary.Agreement.TurnMeaning {
		t.Fatalf("expected turn meaning agreement")
	}
	if !summary.Agreement.SelectedOptionIndex {
		t.Fatalf("expected selected option agreement")
	}
	if !summary.Agreement.PassengerCount {
		t.Fatalf("expected passenger count agreement")
	}
	if !summary.Agreement.ChildUnder5Count {
		t.Fatalf("expected child under 5 count agreement")
	}
	if !summary.Agreement.DocumentLikeText {
		t.Fatalf("expected document-like text agreement")
	}
	if !summary.Agreement.PaymentPreference {
		t.Fatalf("expected payment preference agreement")
	}
	assertOpenAIShadowValidation(t, summary.OpenAIValidation, string(StructuredInterpreterShadowValidationAccepted), true, "", "")
}

func TestRunStructuredInterpreterShadowValidationAcceptsOpenAIProposal(t *testing.T) {
	tests := []struct {
		name         string
		currentTurn  string
		state        CanonicalConversationState
		activePrompt ActivePromptContext
		proposal     StructuredInterpretation
	}{
		{
			name:        "greeting",
			currentTurn: "oi",
			proposal:    validationProposal(StructuredIntentGreeting, TurnMeaningGreeting),
		},
		{
			name:        "reservation help",
			currentTurn: "como faço pra fazer uma reserva?",
			state:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
			proposal:    validationProposal(StructuredIntentAvailabilitySearch, TurnMeaningNewRequest),
		},
		{
			name:         "payment preference sinal",
			currentTurn:  "sinal",
			state:        CanonicalConversationState{Phase: ConversationPhaseBooked},
			activePrompt: ActivePromptContext{Kind: ActivePromptPaymentPreference},
			proposal: func() StructuredInterpretation {
				proposal := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
				proposal.Payment.PaymentPreference = "sinal"
				return proposal
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeOpenAIInterpreter{
				enabled: true,
				result: OpenAIStructuredInterpreterRunResult{
					Interpretation:     tc.proposal,
					ProviderResponseID: "resp_shadow_validation_accept",
				},
			}

			summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
				Enabled:           true,
				OpenAIInterpreter: fake,
				StructuredInput: StructuredInterpreterInput{
					CurrentTurn: tc.currentTurn,
					State:       tc.state,
				},
				ActivePrompt:        tc.activePrompt,
				LocalInterpretation: validationProposal(StructuredIntentUnknown, TurnMeaningUnknown),
				IdempotencyKey:      "test-key",
			})

			if summary.OpenAI.Status != string(StructuredInterpreterShadowValid) {
				t.Fatalf("expected valid OpenAI shadow status, got %+v", summary.OpenAI)
			}
			assertOpenAIShadowValidation(t, summary.OpenAIValidation, string(StructuredInterpreterShadowValidationAccepted), true, "", "")
		})
	}
}

func TestRunStructuredInterpreterShadowValidationRejectsOpenAIProposal(t *testing.T) {
	tests := []struct {
		name         string
		currentTurn  string
		state        CanonicalConversationState
		activePrompt ActivePromptContext
		proposal     StructuredInterpretation
		reason       string
		fallback     ResponseTemplateName
	}{
		{
			name:         "pix is not payment preference",
			currentTurn:  "pix",
			state:        CanonicalConversationState{Phase: ConversationPhaseBooked},
			activePrompt: ActivePromptContext{Kind: ActivePromptPaymentPreference},
			proposal: func() StructuredInterpretation {
				proposal := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
				proposal.Payment.PaymentPreference = "pix"
				return proposal
			}(),
			reason:   "pix_is_method_not_payment_preference",
			fallback: TemplateContextFallbackPaymentPreference,
		},
		{
			name:        "option answer without active prompt",
			currentTurn: "ok",
			state:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
			proposal:    validationAvailabilityOptionProposal(1),
			reason:      "active_prompt_required",
			fallback:    TemplateContextFallbackAvailabilityOption,
		},
		{
			name:        "unsupported destination",
			currentTurn: "como faço pra reservar passagem para Bahia",
			state:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
			proposal:    validationProposal(StructuredIntentAvailabilitySearch, TurnMeaningNewRequest),
			reason:      "unsupported_destination",
			fallback:    TemplateUnsupportedPackage,
		},
		{
			name:         "invalid passenger document cpf",
			currentTurn:  "12345678900",
			activePrompt: ActivePromptContext{Kind: ActivePromptPassengerDocuments},
			proposal: func() StructuredInterpretation {
				proposal := validationProposal(StructuredIntentPassengerDocumentsProvided, TurnMeaningAnswerToQuestion)
				proposal.PassengerDocument.DocumentLikeText = true
				return proposal
			}(),
			reason:   "invalid_passenger_document",
			fallback: TemplateContextFallbackPassengerDocuments,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeOpenAIInterpreter{
				enabled: true,
				result: OpenAIStructuredInterpreterRunResult{
					Interpretation:     tc.proposal,
					ProviderResponseID: "resp_shadow_validation_reject",
				},
			}

			summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
				Enabled:           true,
				OpenAIInterpreter: fake,
				StructuredInput: StructuredInterpreterInput{
					CurrentTurn: tc.currentTurn,
					State:       tc.state,
				},
				ActivePrompt:        tc.activePrompt,
				LocalInterpretation: validationProposal(StructuredIntentUnknown, TurnMeaningUnknown),
				IdempotencyKey:      "test-key",
			})

			if summary.OpenAI.Status != string(StructuredInterpreterShadowValid) {
				t.Fatalf("expected valid OpenAI shadow status, got %+v", summary.OpenAI)
			}
			assertOpenAIShadowValidation(t, summary.OpenAIValidation, string(StructuredInterpreterShadowValidationRejected), false, tc.reason, tc.fallback)
		})
	}
}

func TestRunStructuredInterpreterShadowValidationRejectsRealRunnerSemanticInvalidOutput(t *testing.T) {
	tests := []struct {
		name         string
		currentTurn  string
		state        CanonicalConversationState
		activePrompt ActivePromptContext
		mutate       func(map[string]interface{})
		reason       string
		fallback     ResponseTemplateName
	}{
		{
			name:        "unsafe side effect claim",
			currentTurn: "oi",
			mutate: func(payload map[string]interface{}) {
				safety := payload["safety"].(map[string]interface{})
				safety["executes_tool"] = true
			},
			reason: "unsafe_side_effect_claim",
		},
		{
			name:         "payment pix after preference prompt",
			currentTurn:  "pix",
			state:        CanonicalConversationState{Phase: ConversationPhaseBooked},
			activePrompt: ActivePromptContext{Kind: ActivePromptPaymentPreference},
			mutate: func(payload map[string]interface{}) {
				payload["intent"] = string(StructuredIntentPaymentPreference)
				payload["turn_meaning"] = string(TurnMeaningAnswerToQuestion)
				payment := payload["payment"].(map[string]interface{})
				payment["payment_preference"] = "pix"
			},
			reason:   "pix_is_method_not_payment_preference",
			fallback: TemplateContextFallbackPaymentPreference,
		},
		{
			name:        "availability option without list context",
			currentTurn: "ok",
			state:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
			mutate: func(payload map[string]interface{}) {
				payload["intent"] = string(StructuredIntentSelectAvailabilityOption)
				payload["turn_meaning"] = string(TurnMeaningAnswerToQuestion)
				booking := payload["booking"].(map[string]interface{})
				booking["selected_option_index"] = 1
				booking["selected_option_index_known"] = true
			},
			reason:   "active_prompt_required",
			fallback: TemplateContextFallbackAvailabilityOption,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := validOpenAIGreetingPayloadMap()
			tc.mutate(payload)
			runner := openAIStructuredInterpreterShadowTestRunner(t, payload)

			summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
				Enabled:           true,
				OpenAIInterpreter: runner,
				StructuredInput: StructuredInterpreterInput{
					CurrentTurn: tc.currentTurn,
					State:       tc.state,
				},
				ActivePrompt:        tc.activePrompt,
				LocalInterpretation: validationProposal(StructuredIntentGreeting, TurnMeaningGreeting),
				IdempotencyKey:      "test-key",
			})

			if summary.OpenAI.Status != string(StructuredInterpreterShadowError) {
				t.Fatalf("expected OpenAI error status for runner-invalid output, got %+v", summary.OpenAI)
			}
			if summary.OpenAI.ErrorCode != "openai_structured_interpreter_invalid_output" {
				t.Fatalf("expected invalid output error code, got %+v", summary.OpenAI)
			}
			assertOpenAIShadowValidation(t, summary.OpenAIValidation, string(StructuredInterpreterShadowValidationRejected), false, tc.reason, tc.fallback)
		})
	}
}

func TestRunStructuredInterpreterShadowValidationSkipsRealRunnerSchemaInvalidOutput(t *testing.T) {
	payload := validOpenAIGreetingPayloadMap()
	delete(payload, "payment")
	runner := openAIStructuredInterpreterShadowTestRunner(t, payload)

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:             true,
		OpenAIInterpreter:   runner,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "oi"},
		LocalInterpretation: validationProposal(StructuredIntentGreeting, TurnMeaningGreeting),
		IdempotencyKey:      "test-key",
	})

	if summary.OpenAI.Status != string(StructuredInterpreterShadowError) {
		t.Fatalf("expected OpenAI error status for runner-invalid output, got %+v", summary.OpenAI)
	}
	assertOpenAIShadowValidation(t, summary.OpenAIValidation, string(StructuredInterpreterShadowValidationSkipped), false, "openai_schema_invalid", "")
}

func TestRunStructuredInterpreterShadowValidationSkipped(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		interpreter *fakeOpenAIInterpreter
		reason      string
		wantCalls   int
	}{
		{
			name:        "shadow disabled",
			enabled:     false,
			interpreter: &fakeOpenAIInterpreter{enabled: true},
			reason:      "shadow_disabled",
		},
		{
			name:        "openai disabled",
			enabled:     true,
			interpreter: &fakeOpenAIInterpreter{enabled: false},
			reason:      "openai_disabled",
		},
		{
			name:        "schema invalid",
			enabled:     true,
			interpreter: &fakeOpenAIInterpreter{enabled: true, err: ErrOpenAIStructuredInterpreterInvalidOutput},
			reason:      "openai_schema_invalid",
			wantCalls:   1,
		},
		{
			name:        "openai error",
			enabled:     true,
			interpreter: &fakeOpenAIInterpreter{enabled: true, err: errors.New("provider failed")},
			reason:      "openai_error",
			wantCalls:   1,
		},
		{
			name:    "missing result",
			enabled: true,
			interpreter: &fakeOpenAIInterpreter{
				enabled: true,
				result: OpenAIStructuredInterpreterRunResult{
					ProviderResponseID: "resp_missing_interpretation",
				},
			},
			reason:    "openai_missing_result",
			wantCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
				Enabled:             tc.enabled,
				OpenAIInterpreter:   tc.interpreter,
				StructuredInput:     StructuredInterpreterInput{CurrentTurn: "oi"},
				LocalInterpretation: validationProposal(StructuredIntentGreeting, TurnMeaningGreeting),
				IdempotencyKey:      "test-key",
			})

			if tc.interpreter.calls != tc.wantCalls {
				t.Fatalf("expected %d OpenAI calls, got %d", tc.wantCalls, tc.interpreter.calls)
			}
			assertOpenAIShadowValidation(t, summary.OpenAIValidation, string(StructuredInterpreterShadowValidationSkipped), false, tc.reason, "")
		})
	}
}

func TestRunStructuredInterpreterShadowValidationPayloadContainsFields(t *testing.T) {
	proposal := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
	proposal.Payment.PaymentPreference = "pix"
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_shadow_validation_payload",
		},
	}

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:           true,
		OpenAIInterpreter: fake,
		StructuredInput:   StructuredInterpreterInput{CurrentTurn: "pix"},
		ActivePrompt:      ActivePromptContext{Kind: ActivePromptPaymentPreference},
		LocalInterpretation: func() StructuredInterpretation {
			local := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
			local.Payment.PaymentPreference = "pix"
			return local
		}(),
		IdempotencyKey: "test-key",
	})

	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	validation := mustNestedMap(t, payload, "openai_validation")
	for _, key := range []string{"status", "accepted", "reject_reason", "fallback_template"} {
		if _, ok := validation[key]; !ok {
			t.Fatalf("expected openai_validation.%s in payload: %#v", key, validation)
		}
	}
	if got := asString(validation["status"]); got != string(StructuredInterpreterShadowValidationRejected) {
		t.Fatalf("expected rejected validation status, got %q", got)
	}
	if got := asString(validation["fallback_template"]); got != string(TemplateContextFallbackPaymentPreference) {
		t.Fatalf("expected fallback template, got %q", got)
	}
}

func openAIStructuredInterpreterShadowTestRunner(t *testing.T, payload map[string]interface{}) *OpenAIStructuredInterpreterRunner {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openAIStructuredInterpreterTestResponse("resp_shadow_validation_runner", string(marshalOpenAITestPayload(t, payload))))
	}))
	t.Cleanup(server.Close)

	return NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL,
	})
}

func TestRunStructuredInterpreterShadowEnabledErrorDoesNotPanic(t *testing.T) {
	local := StructuredInterpretation{
		Intent:      StructuredIntentGreeting,
		TurnMeaning: TurnMeaningGreeting,
		Confidence:  1,
		Source:      "deterministic_test",
	}

	fake := &fakeOpenAIInterpreter{
		enabled: true,
		err:     errors.New("openai exploded because markets are fake"),
	}

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "oi"},
		LocalInterpretation: local,
		IdempotencyKey:      "test-key",
	})

	if fake.calls != 1 {
		t.Fatalf("expected OpenAI to be called once, got calls=%d", fake.calls)
	}

	if summary.OpenAI.Status != string(StructuredInterpreterShadowError) {
		t.Fatalf("expected OpenAI status %q, got %q", StructuredInterpreterShadowError, summary.OpenAI.Status)
	}

	if strings.TrimSpace(summary.OpenAI.ErrorCode) == "" {
		t.Fatalf("expected sanitized error code")
	}
	assertOpenAIShadowValidation(t, summary.OpenAIValidation, string(StructuredInterpreterShadowValidationSkipped), false, "openai_error", "")
}

func TestRunStructuredInterpreterShadowSummaryDoesNotLeakSensitiveData(t *testing.T) {
	local := StructuredInterpretation{
		Intent:      StructuredIntentPassengerDocumentsProvided,
		TurnMeaning: TurnMeaningAnswerToQuestion,
		Confidence:  1,
		Source:      "deterministic_test",
	}

	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: StructuredInterpretation{
				Intent:      StructuredIntentPassengerDocumentsProvided,
				TurnMeaning: TurnMeaningAnswerToQuestion,
				Confidence:  0.88,
				Source:      "openai_structured",
			},
			ProviderResponseID: "resp_test_sensitive",
		},
	}

	summary := RunStructuredInterpreterShadow(context.Background(), StructuredInterpreterShadowInput{
		Enabled:           true,
		OpenAIInterpreter: fake,
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "Joao Vitor 06645648103",
		},
		LocalInterpretation: local,
		IdempotencyKey:      "test-key",
	})

	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}

	text := string(raw)

	for _, forbidden := range []string{
		"06645648103",
		"Joao Vitor",
		"João Vitor",
		"RawOutput",
		"RequestPayload",
		"ResponsePayload",
		"CurrentTurn",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("shadow summary leaked forbidden value %q in %s", forbidden, text)
		}
	}
}

func assertOpenAIShadowValidation(t *testing.T, got OpenAIInterpreterShadowValidation, status string, accepted bool, reason string, fallback ResponseTemplateName) {
	t.Helper()
	if got.Status != status {
		t.Fatalf("expected validation status %q, got %+v", status, got)
	}
	if got.Accepted != accepted {
		t.Fatalf("expected validation accepted=%t, got %+v", accepted, got)
	}
	if got.RejectReason != reason {
		t.Fatalf("expected validation reason %q, got %+v", reason, got)
	}
	if got.FallbackTemplate != fallback {
		t.Fatalf("expected validation fallback %q, got %+v", fallback, got)
	}
}
