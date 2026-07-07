package chat

import "strings"

func OpenAIInterpreterAssistReportItemFromMessage(message Message) (OpenAIInterpreterAssistReportItem, bool) {
	malformed := false
	for _, payload := range []map[string]interface{}{message.NormalizedPayload, message.Payload} {
		if len(payload) == 0 {
			continue
		}
		value, ok := payload[openAIInterpreterAssistMetadataKey]
		if !ok {
			continue
		}
		item, valid := openAIInterpreterAssistReportItemFromValue(value)
		if valid {
			return item, true
		}
		malformed = true
	}
	if malformed {
		return OpenAIInterpreterAssistReportItem{Malformed: true}, true
	}
	return OpenAIInterpreterAssistReportItem{}, false
}

func OpenAIInterpreterAssistReportItemsFromMessages(messages []Message) []OpenAIInterpreterAssistReportItem {
	items := make([]OpenAIInterpreterAssistReportItem, 0, len(messages))
	for _, message := range messages {
		item, ok := OpenAIInterpreterAssistReportItemFromMessage(message)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items
}

func BuildOpenAIInterpreterAssistReportFromMessages(messages []Message) OpenAIInterpreterAssistReport {
	return BuildOpenAIInterpreterAssistReport(OpenAIInterpreterAssistReportItemsFromMessages(messages))
}

func openAIInterpreterAssistReportItemFromValue(value interface{}) (OpenAIInterpreterAssistReportItem, bool) {
	switch typed := value.(type) {
	case OpenAIInterpreterAssistDecision:
		return openAIInterpreterAssistReportItemFromMap(typed.Metadata())
	case *OpenAIInterpreterAssistDecision:
		if typed == nil {
			return OpenAIInterpreterAssistReportItem{}, false
		}
		return openAIInterpreterAssistReportItemFromMap(typed.Metadata())
	case map[string]interface{}:
		return openAIInterpreterAssistReportItemFromMap(typed)
	default:
		return OpenAIInterpreterAssistReportItem{}, false
	}
}

func openAIInterpreterAssistReportItemFromMap(payload map[string]interface{}) (OpenAIInterpreterAssistReportItem, bool) {
	if len(payload) == 0 {
		return OpenAIInterpreterAssistReportItem{}, false
	}

	item := OpenAIInterpreterAssistReportItem{
		Status:              openAIInterpreterAssistFirstNonEmptyString(payload["openai_assist_status"], payload["status"]),
		ValidationStatus:    strings.TrimSpace(asString(payload["validation_status"])),
		Considered:          structuredInterpreterShadowBool(payload["considered"]),
		HasConsidered:       openAIInterpreterAssistMapHasKey(payload, "considered"),
		Accepted:            structuredInterpreterShadowBool(payload["accepted"]),
		LocalIntent:         strings.TrimSpace(asString(payload["local_intent"])),
		LocalConfidence:     structuredInterpreterShadowFloat(payload["local_confidence"]),
		HasLocalConfidence:  openAIInterpreterAssistMapHasKey(payload, "local_confidence"),
		OpenAIIntent:        strings.TrimSpace(asString(payload["openai_intent"])),
		OpenAIConfidence:    structuredInterpreterShadowFloat(payload["openai_confidence"]),
		HasOpenAIConfidence: openAIInterpreterAssistMapHasKey(payload, "openai_confidence"),
		RejectReason:        strings.TrimSpace(asString(payload["reject_reason"])),
		SkipReason:          strings.TrimSpace(asString(payload["skip_reason"])),
		ReasonCodes: appendStructuredInterpreterShadowValidationErrors(
			asStringSlice(payload["controlled_reason_codes"]),
			asStringSlice(payload["reasons"]),
			[]string{strings.TrimSpace(asString(payload["reason"]))},
		),
		FallbackTemplate: strings.TrimSpace(asString(payload["fallback_template"])),
		TemplateName:     strings.TrimSpace(openAIInterpreterAssistFirstNonEmptyString(payload["template_name"], payload["template"])),
		Action:           strings.TrimSpace(asString(payload["action"])),
		Source:           strings.TrimSpace(openAIInterpreterAssistFirstNonEmptyString(payload["decision_source"], payload["source"])),
	}

	return item, openAIInterpreterAssistReportItemHasData(item)
}

func openAIInterpreterAssistReportItemHasData(item OpenAIInterpreterAssistReportItem) bool {
	return strings.TrimSpace(item.Status) != "" ||
		strings.TrimSpace(item.ValidationStatus) != "" ||
		item.HasConsidered ||
		item.Considered ||
		item.Accepted ||
		strings.TrimSpace(item.LocalIntent) != "" ||
		item.HasLocalConfidence ||
		strings.TrimSpace(item.OpenAIIntent) != "" ||
		item.HasOpenAIConfidence ||
		strings.TrimSpace(item.RejectReason) != "" ||
		strings.TrimSpace(item.SkipReason) != "" ||
		len(item.ReasonCodes) > 0 ||
		strings.TrimSpace(item.FallbackTemplate) != "" ||
		strings.TrimSpace(item.TemplateName) != "" ||
		strings.TrimSpace(item.Action) != "" ||
		strings.TrimSpace(item.Source) != ""
}

func openAIInterpreterAssistFirstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(asString(value)); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func openAIInterpreterAssistMapHasKey(payload map[string]interface{}, key string) bool {
	_, ok := payload[key]
	return ok
}
