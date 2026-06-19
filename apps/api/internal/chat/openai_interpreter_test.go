package chat

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestOpenAIStructuredInterpretationSchemaIsStrict(t *testing.T) {
	schema := openAIStructuredInterpretationJSONSchema()
	if got, ok := schema["strict"].(bool); !ok || !got {
		t.Fatalf("expected schema strict=true, got %#v", schema["strict"])
	}

	root := mustSchemaMap(t, schema, "schema")
	assertAdditionalPropertiesFalse(t, root, "root")

	properties := mustSchemaMap(t, root, "properties")
	for _, key := range []string{"booking", "passenger_document", "payment", "safety"} {
		nested := mustSchemaMap(t, properties, key)
		assertAdditionalPropertiesFalse(t, nested, key)
	}

	intentSchema := mustSchemaMap(t, properties, "intent")
	assertSchemaEnumContainsOnly(t, intentSchema, openAIStructuredIntentEnum())
	meaningSchema := mustSchemaMap(t, properties, "turn_meaning")
	assertSchemaEnumContainsOnly(t, meaningSchema, openAITurnMeaningEnum())

	for _, forbidden := range []string{"action", "tool_name", "template_name", "tool_payload"} {
		if schemaContainsKey(schema, forbidden) {
			t.Fatalf("schema must not contain executor field %q", forbidden)
		}
	}
}

func TestBuildOpenAIStructuredInterpreterCompactInputRedactsCPF(t *testing.T) {
	local := validOpenAIGreetingCandidate()
	input := StructuredInterpreterInput{
		CurrentTurn: "Joao Vitor Messias CPF 066.456.481-03",
		History: []Message{
			{Direction: "OUTBOUND", Body: "Pode enviar nome completo e CPF ou RG?"},
			{Direction: "INBOUND", Body: "Joao Vitor Messias CPF 066.456.481-03"},
		},
	}

	got := buildOpenAIStructuredInterpreterCompactInput(input, openAIStructuredInterpretationToStructured(local))
	for _, raw := range []string{"066.456.481-03", "06645648103", "Joao Vitor Messias CPF"} {
		if strings.Contains(got, raw) {
			t.Fatalf("compact input leaked raw document fragment %q: %s", raw, got)
		}
	}
	if !strings.Contains(got, "[DOCUMENTO_REDACTED]") {
		t.Fatalf("expected redaction marker in compact input: %s", got)
	}
}

func TestBuildOpenAIStructuredInterpreterCompactInputLimitsRecentTurns(t *testing.T) {
	history := make([]Message, 0, 8)
	for i := 0; i < 8; i++ {
		history = append(history, Message{
			Direction: "INBOUND",
			Body:      "turn " + string(rune('0'+i)),
		})
	}

	raw := buildOpenAIStructuredInterpreterCompactInput(
		StructuredInterpreterInput{CurrentTurn: "oi", History: history},
		openAIStructuredInterpretationToStructured(validOpenAIGreetingCandidate()),
	)
	var compact OpenAIStructuredInterpreterCompactInput
	if err := json.Unmarshal([]byte(raw), &compact); err != nil {
		t.Fatalf("compact input must be valid json: %v", err)
	}
	if len(compact.RecentTurns) != openAIStructuredInterpreterRecentTurnsLimit {
		t.Fatalf("expected %d recent turns, got %d: %#v", openAIStructuredInterpreterRecentTurnsLimit, len(compact.RecentTurns), compact.RecentTurns)
	}
	if !strings.Contains(compact.RecentTurns[0], "turn 2") {
		t.Fatalf("expected compact input to keep the latest six turns, got %#v", compact.RecentTurns)
	}
}

func TestValidateOpenAIStructuredInterpretationAcceptsGreeting(t *testing.T) {
	result := validateOpenAIStructuredInterpretation(
		StructuredInterpreterInput{CurrentTurn: "oi"},
		validOpenAIGreetingCandidate(),
	)
	if !result.Valid {
		t.Fatalf("expected valid greeting, got reasons=%v", result.Reasons)
	}
	if result.Interpretation.Intent != StructuredIntentGreeting {
		t.Fatalf("unexpected interpretation: %+v", result.Interpretation)
	}
	if result.Interpretation.Source != "openai_structured" {
		t.Fatalf("expected openai_structured source, got %q", result.Interpretation.Source)
	}
}

func TestValidateOpenAIStructuredInterpretationPayloadEnforcesCanonicalEnums(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(map[string]interface{})
		wantReason string
	}{
		{
			name: "lowercase_intent",
			mutate: func(payload map[string]interface{}) {
				payload["intent"] = "greeting"
			},
			wantReason: "invalid_intent",
		},
		{
			name: "mixed_case_intent",
			mutate: func(payload map[string]interface{}) {
				payload["intent"] = "Greeting"
			},
			wantReason: "invalid_intent",
		},
		{
			name: "lowercase_turn_meaning",
			mutate: func(payload map[string]interface{}) {
				payload["turn_meaning"] = "confirmation"
			},
			wantReason: "invalid_turn_meaning",
		},
		{
			name: "uppercase_payment_preference",
			mutate: func(payload map[string]interface{}) {
				payment := payload["payment"].(map[string]interface{})
				payment["payment_preference"] = "SINAL"
			},
			wantReason: "invalid_payment_preference",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := validOpenAIGreetingPayloadMap()
			tc.mutate(payload)

			result := validateOpenAIStructuredInterpretationPayload(StructuredInterpreterInput{}, marshalOpenAITestPayload(t, payload))
			if result.Valid {
				t.Fatalf("expected non-canonical enum payload to be invalid")
			}
			if !containsString(result.Reasons, tc.wantReason) {
				t.Fatalf("expected reason %q, got %v", tc.wantReason, result.Reasons)
			}
		})
	}
}

func TestValidateOpenAIStructuredInterpretationPayloadAcceptsCanonicalEnums(t *testing.T) {
	greetingResult := validateOpenAIStructuredInterpretationPayload(
		StructuredInterpreterInput{CurrentTurn: "oi"},
		marshalOpenAITestPayload(t, validOpenAIGreetingPayloadMap()),
	)
	if !greetingResult.Valid {
		t.Fatalf("expected canonical greeting payload to be valid, got %v", greetingResult.Reasons)
	}

	paymentPayload := validOpenAIGreetingPayloadMap()
	paymentPayload["intent"] = string(StructuredIntentPaymentPreference)
	paymentPayload["turn_meaning"] = string(TurnMeaningAnswerToQuestion)
	payment := paymentPayload["payment"].(map[string]interface{})
	payment["payment_preference"] = "sinal"
	paymentResult := validateOpenAIStructuredInterpretationPayload(
		StructuredInterpreterInput{
			CurrentTurn: "sinal",
			History: []Message{{
				Direction: "OUTBOUND",
				Body:      "Prefere pagar o valor integral ou apenas o sinal?",
			}},
		},
		marshalOpenAITestPayload(t, paymentPayload),
	)
	if !paymentResult.Valid {
		t.Fatalf("expected canonical payment preference payload to be valid, got %v", paymentResult.Reasons)
	}
}

func TestValidateOpenAIStructuredInterpretationRejectsDocumentConfirmationWithoutContext(t *testing.T) {
	candidate := validOpenAIGreetingCandidate()
	candidate.Intent = StructuredIntentDocumentConfirmation
	candidate.TurnMeaning = TurnMeaningConfirmation

	result := validateOpenAIStructuredInterpretation(StructuredInterpreterInput{CurrentTurn: "sim"}, candidate)
	if result.Valid {
		t.Fatalf("expected document confirmation without context to be invalid")
	}
	if !containsString(result.Reasons, "document_confirmation_without_prompt") {
		t.Fatalf("expected document confirmation reason, got %v", result.Reasons)
	}
}

func TestValidateOpenAIStructuredInterpretationRejectsOutOfRangeAvailabilityOption(t *testing.T) {
	candidate := validOpenAIGreetingCandidate()
	candidate.Intent = StructuredIntentSelectAvailabilityOption
	candidate.TurnMeaning = TurnMeaningAnswerToQuestion
	candidate.Booking.SelectedOptionIndexKnown = true
	candidate.Booking.SelectedOptionIndex = 3

	result := validateOpenAIStructuredInterpretation(StructuredInterpreterInput{
		CurrentTurn: "3",
		History:     availabilitySelectionStructuredHistory(t),
	}, candidate)
	if result.Valid {
		t.Fatalf("expected out-of-range availability option to be invalid")
	}
	if !containsString(result.Reasons, "availability_option_out_of_range") {
		t.Fatalf("expected out-of-range reason, got %v", result.Reasons)
	}
}

func TestValidateOpenAIStructuredInterpretationRejectsPixPaymentPreference(t *testing.T) {
	candidate := validOpenAIGreetingCandidate()
	candidate.Intent = StructuredIntentPaymentPreference
	candidate.TurnMeaning = TurnMeaningAnswerToQuestion
	candidate.Payment.PaymentPreference = "pix"

	result := validateOpenAIStructuredInterpretation(StructuredInterpreterInput{
		CurrentTurn: "pix",
		History: []Message{{
			Direction: "OUTBOUND",
			Body:      "Prefere pagar o valor integral ou apenas o sinal?",
		}},
	}, candidate)
	if result.Valid {
		t.Fatalf("expected pix payment preference to be invalid")
	}
	if !containsString(result.Reasons, "invalid_payment_preference") {
		t.Fatalf("expected invalid payment preference reason, got %v", result.Reasons)
	}
}

func TestValidateOpenAIStructuredInterpretationPayloadRejectsMissingRequiredFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{
			name: "top_level_objects_missing",
			mutate: func(payload map[string]interface{}) {
				for _, key := range []string{"booking", "passenger_document", "payment", "safety", "reasons"} {
					delete(payload, key)
				}
			},
		},
		{
			name: "booking_null",
			mutate: func(payload map[string]interface{}) {
				payload["booking"] = nil
			},
		},
		{
			name: "payment_missing",
			mutate: func(payload map[string]interface{}) {
				delete(payload, "payment")
			},
		},
		{
			name: "safety_missing",
			mutate: func(payload map[string]interface{}) {
				delete(payload, "safety")
			},
		},
		{
			name: "nested_booking_field_missing",
			mutate: func(payload map[string]interface{}) {
				booking := payload["booking"].(map[string]interface{})
				delete(booking, "selected_option_index")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := validOpenAIGreetingPayloadMap()
			tc.mutate(payload)

			result := validateOpenAIStructuredInterpretationPayload(StructuredInterpreterInput{}, marshalOpenAITestPayload(t, payload))
			if result.Valid {
				t.Fatalf("expected missing required field payload to be invalid")
			}
			if !containsString(result.Reasons, "invalid_json_payload") {
				t.Fatalf("expected invalid json payload reason, got %v", result.Reasons)
			}
		})
	}
}

func TestValidateOpenAIStructuredInterpretationPayloadRejectsExecutorFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{
			name: "unexpected_extra_field",
			mutate: func(payload map[string]interface{}) {
				payload["unexpected"] = "extra"
			},
		},
		{
			name: "root_action",
			mutate: func(payload map[string]interface{}) {
				payload["action"] = "tool"
			},
		},
		{
			name: "root_template_name",
			mutate: func(payload map[string]interface{}) {
				payload["template_name"] = "ASK_DOCUMENT"
			},
		},
		{
			name: "nested_tool_name",
			mutate: func(payload map[string]interface{}) {
				payment := payload["payment"].(map[string]interface{})
				payment["tool_name"] = "fazer_pagamento"
			},
		},
		{
			name: "nested_tool_payload",
			mutate: func(payload map[string]interface{}) {
				booking := payload["booking"].(map[string]interface{})
				booking["tool_payload"] = map[string]interface{}{"danger": true}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := validOpenAIGreetingPayloadMap()
			tc.mutate(payload)

			result := validateOpenAIStructuredInterpretationPayload(StructuredInterpreterInput{}, marshalOpenAITestPayload(t, payload))
			if result.Valid {
				t.Fatalf("expected executor field payload to be invalid")
			}
			if !containsString(result.Reasons, "invalid_json_payload") {
				t.Fatalf("expected invalid json payload reason, got %v", result.Reasons)
			}
		})
	}
}

func TestOpenAIStructuredInterpreterFilesDoNotCallExternalProvider(t *testing.T) {
	for _, path := range []string{
		"openai_interpreter_schema.go",
		"openai_interpreter_prompt.go",
		"openai_interpreter_validator.go",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(data)
		for _, forbidden := range []string{
			`"net/http"`,
			"OpenAIJSONRunner",
			"OpenAIRunner",
			"RunIntentDecision",
			"NewOpenAI",
			"api.openai.com",
			"openai.com/v1",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not call external provider; found %q", path, forbidden)
			}
		}
	}
}

func validOpenAIGreetingCandidate() OpenAIStructuredInterpretationJSON {
	return OpenAIStructuredInterpretationJSON{
		Intent:      StructuredIntentGreeting,
		TurnMeaning: TurnMeaningGreeting,
		Confidence:  0.91,
		Booking: OpenAIStructuredBookingSlotsJSON{
			LapChildPassengerIndexes: []int{},
		},
		Payment: OpenAIStructuredPaymentSlotsJSON{
			PaymentPreference: "",
		},
		Safety:  OpenAIStructuredInterpretationSafetyJSON{},
		Reasons: []string{"greeting_only"},
	}
}

func validOpenAIGreetingPayloadMap() map[string]interface{} {
	return map[string]interface{}{
		"intent":       string(StructuredIntentGreeting),
		"turn_meaning": string(TurnMeaningGreeting),
		"confidence":   0.91,
		"booking": map[string]interface{}{
			"selected_option_index":       0,
			"selected_option_index_known": false,
			"passenger_count":             0,
			"passenger_count_known":       false,
			"child_under_5_count":         0,
			"child_under_5_count_known":   false,
			"lap_child_passenger_indexes": []interface{}{},
		},
		"passenger_document": map[string]interface{}{
			"document_like_text": false,
		},
		"payment": map[string]interface{}{
			"payment_preference": "",
		},
		"safety": map[string]interface{}{
			"executes_tool":           false,
			"mutates_state":           false,
			"sends_message":           false,
			"calls_external_provider": false,
		},
		"reasons": []interface{}{"greeting_only"},
	}
}

func marshalOpenAITestPayload(t *testing.T, payload map[string]interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}

func mustSchemaMap(t *testing.T, source map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	value, ok := source[key].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema key %q to be map, got %#v", key, source[key])
	}
	return value
}

func assertAdditionalPropertiesFalse(t *testing.T, schema map[string]interface{}, label string) {
	t.Helper()
	if got, ok := schema["additionalProperties"].(bool); !ok || got {
		t.Fatalf("expected %s additionalProperties=false, got %#v", label, schema["additionalProperties"])
	}
}

func assertSchemaEnumContainsOnly(t *testing.T, schema map[string]interface{}, want []string) {
	t.Helper()
	raw, ok := schema["enum"].([]string)
	if !ok {
		t.Fatalf("expected enum []string, got %#v", schema["enum"])
	}
	if len(raw) != len(want) {
		t.Fatalf("unexpected enum length: got %v want %v", raw, want)
	}
	seen := map[string]struct{}{}
	for _, item := range raw {
		seen[item] = struct{}{}
	}
	for _, item := range want {
		if _, ok := seen[item]; !ok {
			t.Fatalf("enum missing %q: got %v", item, raw)
		}
	}
}

func schemaContainsKey(value interface{}, key string) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		for k, v := range typed {
			if k == key || schemaContainsKey(v, key) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if schemaContainsKey(item, key) {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if item == key {
				return true
			}
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
