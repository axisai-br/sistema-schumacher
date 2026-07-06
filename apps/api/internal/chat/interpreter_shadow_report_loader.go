package chat

import (
	"encoding/json"
	"strconv"
	"strings"
)

func StructuredInterpreterShadowReportItemFromMessage(message Message) (StructuredInterpreterShadowReportItem, bool) {
	messageValidationErrors := asStringSlice(message.NormalizedPayload["validation_errors"])
	for _, payload := range []map[string]interface{}{message.NormalizedPayload, message.Payload} {
		if len(payload) == 0 {
			continue
		}
		item, ok := structuredInterpreterShadowReportItemFromPayload(payload)
		if ok {
			item.ValidationErrors = appendStructuredInterpreterShadowValidationErrors(
				messageValidationErrors,
				item.ValidationErrors,
			)
			return item, true
		}
	}
	return StructuredInterpreterShadowReportItem{}, false
}

func StructuredInterpreterShadowReportItemsFromMessages(messages []Message) []StructuredInterpreterShadowReportItem {
	items := make([]StructuredInterpreterShadowReportItem, 0, len(messages))
	for _, message := range messages {
		item, ok := StructuredInterpreterShadowReportItemFromMessage(message)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items
}

func BuildStructuredInterpreterShadowReportFromMessages(messages []Message) StructuredInterpreterShadowReport {
	return BuildStructuredInterpreterShadowReport(StructuredInterpreterShadowReportItemsFromMessages(messages))
}

func structuredInterpreterShadowReportItemFromPayload(payload map[string]interface{}) (StructuredInterpreterShadowReportItem, bool) {
	value, ok := payload[structuredInterpreterShadowKey]
	if !ok {
		return StructuredInterpreterShadowReportItem{}, false
	}
	item, ok := structuredInterpreterShadowReportItemFromValue(value)
	if !ok {
		return StructuredInterpreterShadowReportItem{}, false
	}
	item.ValidationErrors = appendStructuredInterpreterShadowValidationErrors(
		asStringSlice(payload["validation_errors"]),
		item.ValidationErrors,
	)
	return item, true
}

func structuredInterpreterShadowReportItemFromValue(value interface{}) (StructuredInterpreterShadowReportItem, bool) {
	switch typed := value.(type) {
	case StructuredInterpreterShadowSummary:
		return StructuredInterpreterShadowReportItem{Summary: typed}, true
	case *StructuredInterpreterShadowSummary:
		if typed == nil {
			return StructuredInterpreterShadowReportItem{}, false
		}
		return StructuredInterpreterShadowReportItem{Summary: *typed}, true
	case map[string]interface{}:
		return structuredInterpreterShadowReportItemFromMap(typed)
	default:
		return StructuredInterpreterShadowReportItem{}, false
	}
}

func structuredInterpreterShadowReportItemFromMap(payload map[string]interface{}) (StructuredInterpreterShadowReportItem, bool) {
	if len(payload) == 0 {
		return StructuredInterpreterShadowReportItem{}, false
	}

	item := StructuredInterpreterShadowReportItem{
		Summary: StructuredInterpreterShadowSummary{
			Local:            structuredInterpreterSummaryFromMap(asMap(payload["local"])),
			OpenAI:           openAIInterpreterShadowSummaryFromMap(asMap(payload["openai"])),
			OpenAIValidation: openAIInterpreterShadowValidationFromMap(asMap(payload["openai_validation"])),
			Agreement:        structuredInterpreterAgreementFromMap(asMap(payload["agreement"])),
		},
		ValidationErrors: asStringSlice(payload["validation_errors"]),
	}
	return item, structuredInterpreterShadowReportItemHasData(item)
}

func structuredInterpreterShadowReportItemHasData(item StructuredInterpreterShadowReportItem) bool {
	return strings.TrimSpace(item.Summary.Local.Intent) != "" ||
		strings.TrimSpace(item.Summary.Local.TurnMeaning) != "" ||
		item.Summary.Local.Confidence != 0 ||
		strings.TrimSpace(item.Summary.Local.Source) != "" ||
		strings.TrimSpace(item.Summary.OpenAI.Status) != "" ||
		strings.TrimSpace(item.Summary.OpenAI.Intent) != "" ||
		strings.TrimSpace(item.Summary.OpenAI.TurnMeaning) != "" ||
		item.Summary.OpenAI.Confidence != 0 ||
		strings.TrimSpace(item.Summary.OpenAI.Source) != "" ||
		strings.TrimSpace(item.Summary.OpenAI.ProviderResponseID) != "" ||
		item.Summary.OpenAI.LatencyMs != 0 ||
		strings.TrimSpace(item.Summary.OpenAI.ErrorCode) != "" ||
		strings.TrimSpace(item.Summary.OpenAIValidation.Status) != "" ||
		item.Summary.OpenAIValidation.Accepted ||
		strings.TrimSpace(item.Summary.OpenAIValidation.RejectReason) != "" ||
		strings.TrimSpace(string(item.Summary.OpenAIValidation.FallbackTemplate)) != "" ||
		item.Summary.Agreement.Intent ||
		item.Summary.Agreement.TurnMeaning ||
		item.Summary.Agreement.SelectedOptionIndex ||
		item.Summary.Agreement.PassengerCount ||
		item.Summary.Agreement.ChildUnder5Count ||
		item.Summary.Agreement.DocumentLikeText ||
		item.Summary.Agreement.PaymentPreference ||
		len(item.ValidationErrors) > 0
}

func appendStructuredInterpreterShadowValidationErrors(values ...[]string) []string {
	var errors []string
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, item := range value {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			errors = append(errors, trimmed)
		}
	}
	return errors
}

func structuredInterpreterSummaryFromMap(payload map[string]interface{}) StructuredInterpreterSummary {
	if len(payload) == 0 {
		return StructuredInterpreterSummary{}
	}
	return StructuredInterpreterSummary{
		Intent:      strings.TrimSpace(asString(payload["intent"])),
		TurnMeaning: strings.TrimSpace(asString(payload["turn_meaning"])),
		Confidence:  structuredInterpreterShadowFloat(payload["confidence"]),
		Source:      strings.TrimSpace(asString(payload["source"])),
	}
}

func openAIInterpreterShadowSummaryFromMap(payload map[string]interface{}) OpenAIInterpreterShadowSummary {
	if len(payload) == 0 {
		return OpenAIInterpreterShadowSummary{}
	}
	return OpenAIInterpreterShadowSummary{
		Status:             strings.TrimSpace(asString(payload["status"])),
		Intent:             strings.TrimSpace(asString(payload["intent"])),
		TurnMeaning:        strings.TrimSpace(asString(payload["turn_meaning"])),
		Confidence:         structuredInterpreterShadowFloat(payload["confidence"]),
		Source:             strings.TrimSpace(asString(payload["source"])),
		ProviderResponseID: strings.TrimSpace(asString(payload["provider_response_id"])),
		LatencyMs:          structuredInterpreterShadowLatencyMs(payload["latency_ms"]),
		ErrorCode:          strings.TrimSpace(asString(payload["error_code"])),
	}
}

func openAIInterpreterShadowValidationFromMap(payload map[string]interface{}) OpenAIInterpreterShadowValidation {
	if len(payload) == 0 {
		return OpenAIInterpreterShadowValidation{}
	}
	return OpenAIInterpreterShadowValidation{
		Status:           strings.TrimSpace(asString(payload["status"])),
		Accepted:         structuredInterpreterShadowBool(payload["accepted"]),
		RejectReason:     strings.TrimSpace(asString(payload["reject_reason"])),
		FallbackTemplate: ResponseTemplateName(strings.TrimSpace(asString(payload["fallback_template"]))),
	}
}

func structuredInterpreterAgreementFromMap(payload map[string]interface{}) StructuredInterpreterAgreement {
	if len(payload) == 0 {
		return StructuredInterpreterAgreement{}
	}
	return StructuredInterpreterAgreement{
		Intent:              structuredInterpreterShadowBool(payload["intent"]),
		TurnMeaning:         structuredInterpreterShadowBool(payload["turn_meaning"]),
		SelectedOptionIndex: structuredInterpreterShadowBool(payload["selected_option_index"]),
		PassengerCount:      structuredInterpreterShadowBool(payload["passenger_count"]),
		ChildUnder5Count:    structuredInterpreterShadowBool(payload["child_under_5_count"]),
		DocumentLikeText:    structuredInterpreterShadowBool(payload["document_like_text"]),
		PaymentPreference:   structuredInterpreterShadowBool(payload["payment_preference"]),
	}
}

func structuredInterpreterShadowFloat(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func structuredInterpreterShadowLatencyMs(value interface{}) int64 {
	return int64(structuredInterpreterShadowFloat(value))
}

func structuredInterpreterShadowBool(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}
