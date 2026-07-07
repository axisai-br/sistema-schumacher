package chat

import "strings"

var openAIInterpreterAssistStatusMetricKeys = map[string]struct{}{
	string(OpenAIInterpreterAssistAccepted): {},
	string(OpenAIInterpreterAssistRejected): {},
	string(OpenAIInterpreterAssistSkipped):  {},
	"missing":                               {},
}

var openAIInterpreterAssistValidationStatusMetricKeys = map[string]struct{}{
	string(OpenAIInterpreterAssistValidationAccepted): {},
	string(OpenAIInterpreterAssistValidationRejected): {},
	string(OpenAIInterpreterAssistValidationSkipped):  {},
	"missing": {},
}

var openAIInterpreterAssistReasonMetricKeys = map[string]struct{}{
	"active_prompt_required":                        {},
	"assist_disabled":                               {},
	"availability_input_not_safely_parseable":       {},
	"availability_selection_missing_current_facts":  {},
	"booking_cancel_not_allowed_in_openai_assist":   {},
	"booking_create_not_allowed_in_openai_assist":   {},
	"context_canceled":                              {},
	"context_deadline_exceeded":                     {},
	"deterministic_booking_handled":                 {},
	"deterministic_tool_handled":                    {},
	"document_collection_media_turn":                {},
	"document_extract_not_allowed_in_openai_assist": {},
	"intent_not_allowed_by_active_prompt":           {},
	"intent_not_allowed_in_openai_assist":           {},
	"local_result_not_eligible":                     {},
	"no_safe_greeting_template":                     {},
	"openai_assist_tool_action_not_allowed":         {},
	"openai_confidence_below_threshold":             {},
	"openai_disabled":                               {},
	"openai_error":                                  {},
	"openai_missing_result":                         {},
	"openai_schema_invalid":                         {},
	"payment_create_not_allowed_in_openai_assist":   {},
	"selected_option_index_required":                {},
}

var openAIInterpreterAssistActionMetricKeys = map[string]struct{}{
	"booking_continuation": {},
	"clarify":              {},
	"safe_fallback":        {},
	"specialist":           {},
	"template":             {},
	"tool":                 {},
	"tool_template":        {},
}

var openAIInterpreterAssistSourceMetricKeys = map[string]struct{}{
	"local":                     {},
	"openai_interpreter_assist": {},
}

type OpenAIInterpreterAssistReportItem struct {
	Status              string
	ValidationStatus    string
	Considered          bool
	HasConsidered       bool
	Accepted            bool
	LocalIntent         string
	LocalConfidence     float64
	HasLocalConfidence  bool
	OpenAIIntent        string
	OpenAIConfidence    float64
	HasOpenAIConfidence bool
	RejectReason        string
	SkipReason          string
	ReasonCodes         []string
	FallbackTemplate    string
	TemplateName        string
	Action              string
	Source              string
	Malformed           bool
}

type OpenAIInterpreterAssistStatusCounts struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
	Skipped  int `json:"skipped"`
}

type OpenAIInterpreterAssistReport struct {
	Total                      int                                            `json:"total"`
	Considered                 int                                            `json:"considered"`
	NotConsidered              int                                            `json:"not_considered"`
	Accepted                   int                                            `json:"accepted"`
	Rejected                   int                                            `json:"rejected"`
	Skipped                    int                                            `json:"skipped"`
	UnknownStatusCount         int                                            `json:"unknown_status_count"`
	MalformedCount             int                                            `json:"malformed_count"`
	ByStatus                   map[string]int                                 `json:"by_status"`
	ByValidationStatus         map[string]OpenAIInterpreterAssistStatusCounts `json:"by_validation_status"`
	ByReason                   map[string]int                                 `json:"by_reason"`
	ByReasonStatus             map[string]OpenAIInterpreterAssistStatusCounts `json:"by_reason_status"`
	ByRejectReason             map[string]int                                 `json:"by_reject_reason"`
	BySkipReason               map[string]int                                 `json:"by_skip_reason"`
	ByFallbackTemplate         map[string]int                                 `json:"by_fallback_template"`
	ByIntent                   map[string]OpenAIInterpreterAssistStatusCounts `json:"by_intent"`
	ByTemplate                 map[string]OpenAIInterpreterAssistStatusCounts `json:"by_template"`
	ByAction                   map[string]OpenAIInterpreterAssistStatusCounts `json:"by_action"`
	BySource                   map[string]OpenAIInterpreterAssistStatusCounts `json:"by_source"`
	ByConfidenceBucket         map[string]OpenAIInterpreterAssistStatusCounts `json:"by_confidence_bucket"`
	BlockedToolActionCount     int                                            `json:"blocked_tool_action_count"`
	BlockedCriticalIntentCount int                                            `json:"blocked_critical_intent_count"`
	LocalUnknownCount          int                                            `json:"local_unknown_count"`
	LocalLowConfidenceCount    int                                            `json:"local_low_confidence_count"`
}

func BuildOpenAIInterpreterAssistReport(items []OpenAIInterpreterAssistReportItem) OpenAIInterpreterAssistReport {
	report := newOpenAIInterpreterAssistReport()

	for _, item := range items {
		report.Total++
		if item.Malformed {
			report.MalformedCount++
			continue
		}

		if item.HasConsidered {
			if item.Considered {
				report.Considered++
			} else {
				report.NotConsidered++
			}
		}

		status := openAIInterpreterAssistStatus(item)
		statusKey := openAIInterpreterAssistStatusMetricKey(status)
		if statusKey == "" {
			statusKey = "missing"
		}
		report.ByStatus[statusKey]++

		switch status {
		case string(OpenAIInterpreterAssistAccepted):
			report.Accepted++
		case string(OpenAIInterpreterAssistRejected):
			report.Rejected++
		case string(OpenAIInterpreterAssistSkipped):
			report.Skipped++
		default:
			report.UnknownStatusCount++
		}

		addOpenAIInterpreterAssistStatusDimension(report.ByValidationStatus, item.ValidationStatus, status, openAIInterpreterAssistValidationStatusMetricKey)
		addOpenAIInterpreterAssistStatusDimension(report.ByIntent, item.OpenAIIntent, status, structuredInterpreterShadowIntentMetricKey)
		addOpenAIInterpreterAssistStatusDimension(report.ByTemplate, item.TemplateName, status, structuredInterpreterShadowFallbackTemplateMetricKey)
		addOpenAIInterpreterAssistStatusDimension(report.ByAction, item.Action, status, openAIInterpreterAssistActionMetricKey)
		addOpenAIInterpreterAssistStatusDimension(report.BySource, item.Source, status, openAIInterpreterAssistSourceMetricKey)
		if item.HasOpenAIConfidence {
			addOpenAIInterpreterAssistStatusDimension(report.ByConfidenceBucket, structuredInterpreterShadowConfidenceBucket(item.OpenAIConfidence), status, structuredInterpreterShadowConfidenceBucketMetricKey)
		}

		if item.LocalIntent == string(StructuredIntentUnknown) {
			report.LocalUnknownCount++
		}
		if item.HasLocalConfidence && item.LocalConfidence > 0 && item.LocalConfidence < openAIInterpreterAssistConfidenceThreshold {
			report.LocalLowConfidenceCount++
		}

		for _, reason := range openAIInterpreterAssistItemReasons(item) {
			key := openAIInterpreterAssistReasonMetricKey(reason)
			if key == "" {
				continue
			}
			report.ByReason[key]++
			addOpenAIInterpreterAssistStatusDimension(report.ByReasonStatus, key, status, openAIInterpreterAssistKnownReasonMetricKey)
		}

		rejectReason := openAIInterpreterAssistReasonMetricKey(item.RejectReason)
		if rejectReason != "" {
			report.ByRejectReason[rejectReason]++
		}
		skipReason := openAIInterpreterAssistReasonMetricKey(item.SkipReason)
		if skipReason != "" {
			report.BySkipReason[skipReason]++
		}
		fallbackTemplate := structuredInterpreterShadowFallbackTemplateMetricKey(item.FallbackTemplate)
		if fallbackTemplate != "" {
			report.ByFallbackTemplate[fallbackTemplate]++
		}

		if openAIInterpreterAssistReasonIsBlockedToolAction(rejectReason) {
			report.BlockedToolActionCount++
		}
		if openAIInterpreterAssistReasonIsBlockedCriticalIntent(rejectReason) {
			report.BlockedCriticalIntentCount++
		}
	}

	return report
}

func newOpenAIInterpreterAssistReport() OpenAIInterpreterAssistReport {
	return OpenAIInterpreterAssistReport{
		ByStatus:           map[string]int{},
		ByValidationStatus: map[string]OpenAIInterpreterAssistStatusCounts{},
		ByReason:           map[string]int{},
		ByReasonStatus:     map[string]OpenAIInterpreterAssistStatusCounts{},
		ByRejectReason:     map[string]int{},
		BySkipReason:       map[string]int{},
		ByFallbackTemplate: map[string]int{},
		ByIntent:           map[string]OpenAIInterpreterAssistStatusCounts{},
		ByTemplate:         map[string]OpenAIInterpreterAssistStatusCounts{},
		ByAction:           map[string]OpenAIInterpreterAssistStatusCounts{},
		BySource:           map[string]OpenAIInterpreterAssistStatusCounts{},
		ByConfidenceBucket: map[string]OpenAIInterpreterAssistStatusCounts{},
	}
}

func openAIInterpreterAssistStatus(item OpenAIInterpreterAssistReportItem) string {
	status := strings.TrimSpace(item.Status)
	if status != "" {
		return status
	}
	if item.Accepted {
		return string(OpenAIInterpreterAssistAccepted)
	}
	return ""
}

func addOpenAIInterpreterAssistStatusDimension(
	counts map[string]OpenAIInterpreterAssistStatusCounts,
	rawKey string,
	status string,
	metricKey func(string) string,
) {
	key := metricKey(rawKey)
	if key == "" {
		return
	}

	value := counts[key]
	switch status {
	case string(OpenAIInterpreterAssistAccepted):
		value.Accepted++
	case string(OpenAIInterpreterAssistRejected):
		value.Rejected++
	case string(OpenAIInterpreterAssistSkipped):
		value.Skipped++
	default:
		return
	}
	counts[key] = value
}

func openAIInterpreterAssistItemReasons(item OpenAIInterpreterAssistReportItem) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, reason := range append(append([]string{}, item.ReasonCodes...), item.RejectReason, item.SkipReason) {
		trimmed := strings.TrimSpace(reason)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func openAIInterpreterAssistStatusMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, openAIInterpreterAssistStatusMetricKeys)
}

func openAIInterpreterAssistValidationStatusMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, openAIInterpreterAssistValidationStatusMetricKeys)
}

func openAIInterpreterAssistReasonMetricKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if _, ok := openAIInterpreterAssistReasonMetricKeys[trimmed]; ok && !structuredInterpreterShadowReportMetricKeyLooksSensitive(trimmed) {
		return trimmed
	}
	return structuredInterpreterShadowRejectReasonMetricKey(trimmed)
}

func openAIInterpreterAssistKnownReasonMetricKey(value string) string {
	if strings.TrimSpace(value) == structuredInterpreterShadowRedactedMetricKey {
		return structuredInterpreterShadowRedactedMetricKey
	}
	return openAIInterpreterAssistReasonMetricKey(value)
}

func openAIInterpreterAssistActionMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, openAIInterpreterAssistActionMetricKeys)
}

func openAIInterpreterAssistSourceMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, openAIInterpreterAssistSourceMetricKeys)
}

func openAIInterpreterAssistReasonIsBlockedToolAction(reason string) bool {
	return reason == "openai_assist_tool_action_not_allowed"
}

func openAIInterpreterAssistReasonIsBlockedCriticalIntent(reason string) bool {
	switch reason {
	case "document_extract_not_allowed_in_openai_assist",
		"booking_create_not_allowed_in_openai_assist",
		"payment_create_not_allowed_in_openai_assist",
		"booking_cancel_not_allowed_in_openai_assist":
		return true
	default:
		return false
	}
}
