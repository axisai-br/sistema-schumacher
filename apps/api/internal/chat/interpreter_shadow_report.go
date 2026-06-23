package chat

import (
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

var sensitiveShadowReportFormattedCPFPattern = regexp.MustCompile(`\b[0-9]{3}\.?[0-9]{3}\.?[0-9]{3}-?[0-9]{2}\b`)

type StructuredInterpreterShadowReportItem struct {
	Summary              StructuredInterpreterShadowSummary
	ValidationErrors     []string
	SensitiveScanPayload any
}

type StructuredInterpreterShadowReport struct {
	TotalItems                  int            `json:"total_items"`
	OpenAIStatusCounts          map[string]int `json:"openai_status_counts"`
	LocalIntentCounts           map[string]int `json:"local_intent_counts"`
	OpenAIIntentCounts          map[string]int `json:"openai_intent_counts"`
	IntentAgreementCount        int            `json:"intent_agreement_count"`
	IntentDisagreementCount     int            `json:"intent_disagreement_count"`
	IntentAgreementRate         float64        `json:"intent_agreement_rate"`
	DisagreementsByLocalIntent  map[string]int `json:"disagreements_by_local_intent"`
	DisagreementsByOpenAIIntent map[string]int `json:"disagreements_by_openai_intent"`
	OpenAIErrorCount            int            `json:"openai_error_count"`
	ValidationErrorCount        int            `json:"validation_error_count"`
	LatencyCount                int            `json:"latency_count"`
	LatencyAvgMs                float64        `json:"latency_avg_ms"`
	LatencyMaxMs                int64          `json:"latency_max_ms"`
	SensitiveLeakDetected       bool           `json:"sensitive_leak_detected"`
	SensitiveLeakCount          int            `json:"sensitive_leak_count"`
}

func BuildStructuredInterpreterShadowReport(items []StructuredInterpreterShadowReportItem) StructuredInterpreterShadowReport {
	report := StructuredInterpreterShadowReport{
		TotalItems:                  len(items),
		OpenAIStatusCounts:          map[string]int{},
		LocalIntentCounts:           map[string]int{},
		OpenAIIntentCounts:          map[string]int{},
		DisagreementsByLocalIntent:  map[string]int{},
		DisagreementsByOpenAIIntent: map[string]int{},
	}

	var intentComparisonCount int
	var latencyTotalMs int64
	for _, item := range items {
		summary := item.Summary
		localIntent := strings.TrimSpace(summary.Local.Intent)
		openAIStatus := strings.TrimSpace(summary.OpenAI.Status)
		openAIIntent := strings.TrimSpace(summary.OpenAI.Intent)
		errorCode := strings.TrimSpace(summary.OpenAI.ErrorCode)

		if openAIStatus != "" {
			report.OpenAIStatusCounts[openAIStatus]++
		}
		if localIntent != "" {
			report.LocalIntentCounts[localIntent]++
		}
		if openAIIntent != "" {
			report.OpenAIIntentCounts[openAIIntent]++
		}

		if openAIIntent != "" {
			intentComparisonCount++
			if summary.Agreement.Intent {
				report.IntentAgreementCount++
			} else {
				report.IntentDisagreementCount++
				if localIntent != "" {
					report.DisagreementsByLocalIntent[localIntent]++
				}
				report.DisagreementsByOpenAIIntent[openAIIntent]++
			}
		}

		if openAIStatus == string(StructuredInterpreterShadowError) || errorCode != "" {
			report.OpenAIErrorCount++
		}
		if len(item.ValidationErrors) > 0 || errorCode == "openai_structured_interpreter_invalid_output" {
			report.ValidationErrorCount++
		}

		if shouldCountStructuredInterpreterShadowLatency(summary.OpenAI) {
			report.LatencyCount++
			latencyTotalMs += summary.OpenAI.LatencyMs
			if summary.OpenAI.LatencyMs > report.LatencyMaxMs {
				report.LatencyMaxMs = summary.OpenAI.LatencyMs
			}
		}

		if structuredInterpreterShadowPayloadHasSensitiveLeak(item.SensitiveScanPayload) {
			report.SensitiveLeakCount++
		}
	}

	if intentComparisonCount > 0 {
		report.IntentAgreementRate = float64(report.IntentAgreementCount) / float64(intentComparisonCount)
	}
	if report.LatencyCount > 0 {
		report.LatencyAvgMs = float64(latencyTotalMs) / float64(report.LatencyCount)
	}
	report.SensitiveLeakDetected = report.SensitiveLeakCount > 0

	return report
}

func shouldCountStructuredInterpreterShadowLatency(summary OpenAIInterpreterShadowSummary) bool {
	status := strings.TrimSpace(summary.Status)
	return summary.LatencyMs > 0 ||
		status == string(StructuredInterpreterShadowValid) ||
		status == string(StructuredInterpreterShadowError)
}

func structuredInterpreterShadowPayloadHasSensitiveLeak(payload any) bool {
	if payload == nil {
		return false
	}
	return structuredInterpreterShadowValueHasSensitiveLeak(reflect.ValueOf(payload), 0)
}

func structuredInterpreterShadowValueHasSensitiveLeak(value reflect.Value, depth int) bool {
	if depth > 32 || !value.IsValid() {
		return false
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.String:
		return structuredInterpreterShadowStringHasSensitiveLeak(value.String())
	case reflect.Map:
		for _, key := range value.MapKeys() {
			if structuredInterpreterShadowMapKeyHasSensitiveLeak(key) {
				return true
			}
			if structuredInterpreterShadowValueHasSensitiveLeak(value.MapIndex(key), depth+1) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if structuredInterpreterShadowValueHasSensitiveLeak(value.Index(i), depth+1) {
				return true
			}
		}
	case reflect.Struct:
		valueType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := valueType.Field(i)
			if field.PkgPath != "" {
				continue
			}
			if structuredInterpreterShadowKeyHasSensitiveLeak(field.Name) {
				return true
			}
			if structuredInterpreterShadowValueHasSensitiveLeak(value.Field(i), depth+1) {
				return true
			}
		}
	}

	return false
}

func structuredInterpreterShadowMapKeyHasSensitiveLeak(key reflect.Value) bool {
	if !key.IsValid() {
		return false
	}
	for key.Kind() == reflect.Interface || key.Kind() == reflect.Pointer {
		if key.IsNil() {
			return false
		}
		key = key.Elem()
	}
	if key.Kind() != reflect.String {
		return false
	}
	return structuredInterpreterShadowKeyHasSensitiveLeak(key.String())
}

func structuredInterpreterShadowKeyHasSensitiveLeak(key string) bool {
	identifier := normalizeStructuredInterpreterShadowSensitiveIdentifier(key)
	if identifier == "" {
		return false
	}
	if structuredInterpreterShadowSuspiciousWholeIdentifier(identifier) {
		return true
	}
	for _, token := range strings.Split(identifier, "_") {
		if structuredInterpreterShadowSuspiciousToken(token) {
			return true
		}
	}
	return false
}

func structuredInterpreterShadowStringHasSensitiveLeak(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}

	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"data:image", "data:application", "data:audio"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	if structuredInterpreterShadowContainsPossibleCPF(trimmed) {
		return true
	}

	identifier := normalizeStructuredInterpreterShadowSensitiveIdentifier(trimmed)
	if structuredInterpreterShadowSuspiciousWholeIdentifier(identifier) {
		return true
	}
	for _, token := range strings.Split(identifier, "_") {
		if structuredInterpreterShadowSuspiciousToken(token) {
			return true
		}
	}

	return false
}

func structuredInterpreterShadowSuspiciousWholeIdentifier(identifier string) bool {
	switch identifier {
	case "raw_output", "compact_input", "request_payload", "response_payload":
		return true
	default:
		return false
	}
}

func structuredInterpreterShadowSuspiciousToken(token string) bool {
	switch token {
	case "cpf", "rg", "document", "documento", "raw", "prompt", "input":
		return true
	default:
		return false
	}
}

func normalizeStructuredInterpreterShadowSensitiveIdentifier(value string) string {
	var builder strings.Builder
	lastWasSeparator := true
	lastWasLowerOrDigit := false

	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			if lastWasLowerOrDigit && !lastWasSeparator {
				builder.WriteByte('_')
			}
			builder.WriteRune(unicode.ToLower(char))
			lastWasSeparator = false
			lastWasLowerOrDigit = false
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
			lastWasSeparator = false
			lastWasLowerOrDigit = true
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastWasSeparator = false
			lastWasLowerOrDigit = true
		default:
			if !lastWasSeparator && builder.Len() > 0 {
				builder.WriteByte('_')
			}
			lastWasSeparator = true
			lastWasLowerOrDigit = false
		}
	}

	return strings.Trim(builder.String(), "_")
}

func structuredInterpreterShadowContainsPossibleCPF(value string) bool {
	if sensitiveShadowReportFormattedCPFPattern.MatchString(value) {
		return true
	}

	digitRunLength := 0
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digitRunLength++
			continue
		}
		if digitRunLength == 11 {
			return true
		}
		digitRunLength = 0
	}
	return digitRunLength == 11
}
