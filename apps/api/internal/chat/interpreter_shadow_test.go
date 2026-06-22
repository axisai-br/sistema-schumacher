package chat

import (
	"context"
	"encoding/json"
	"errors"
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
