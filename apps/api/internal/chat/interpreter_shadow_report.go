package chat

import (
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

var sensitiveShadowReportFormattedCPFPattern = regexp.MustCompile(`\b[0-9]{3}\.?[0-9]{3}\.?[0-9]{3}-?[0-9]{2}\b`)

const structuredInterpreterShadowRedactedMetricKey = "__redacted_sensitive"

var structuredInterpreterShadowValidationStatusMetricKeys = map[string]struct{}{
	string(StructuredInterpreterShadowValidationAccepted): {},
	string(StructuredInterpreterShadowValidationRejected): {},
	string(StructuredInterpreterShadowValidationSkipped):  {},
	"missing": {},
}

var structuredInterpreterShadowIntentMetricKeys = map[string]struct{}{
	"UNKNOWN":                             {},
	"GREETING":                            {},
	"AVAILABILITY_SEARCH":                 {},
	"SELECT_AVAILABILITY_OPTION":          {},
	"PASSENGER_COUNT_REPLY":               {},
	"LAP_CHILD_ASSIGNMENT_ANSWER":         {},
	"PASSENGER_DOCUMENTS_PROVIDED":        {},
	"DOCUMENT_CONFIRMATION":               {},
	"BOOKING_CREATE_CONFIRMATION":         {},
	"PAYMENT_PREFERENCE":                  {},
	"PAYMENT_INFO_QUESTION":               {},
	"PAYMENT_METHOD_QUESTION":             {},
	"PAYING_PASSENGER_INFO_QUESTION":      {},
	"DOCUMENT_REQUIREMENTS_INFO_QUESTION": {},
	"CHILD_POLICY_INFO_QUESTION":          {},
	"BAGGAGE_INFO_QUESTION":               {},
	"BOARDING_INFO_QUESTION":              {},
	"HUMAN_SUPPORT_INFO_QUESTION":         {},
	"PAYMENT_STATUS_QUERY":                {},
	"PAYMENT_CREATE":                      {},
	"BOOKING_CANCEL":                      {},
	"BOOKING_CANCEL_REQUEST":              {},
	"RESCHEDULE":                          {},
	"UNSUPPORTED_CARGO":                   {},
	"UNSUPPORTED_PACKAGE":                 {},
	"HUMAN_SUPPORT":                       {},
}

var structuredInterpreterShadowSourceMetricKeys = map[string]struct{}{
	"deterministic": {},
	"deterministic_active_prompt_availability_date":     {},
	"deterministic_active_prompt_availability_option":   {},
	"deterministic_active_prompt_document_confirmation": {},
	"deterministic_active_prompt_lap_child_assignment":  {},
	"deterministic_active_prompt_passenger_count":       {},
	"deterministic_active_prompt_passenger_documents":   {},
	"deterministic_active_prompt_payment_preference":    {},
	"deterministic_active_prompt_reservation_route":     {},
	"deterministic_availability_selection":              {},
	"deterministic_booking_cancel_request":              {},
	"deterministic_document_confirmation":               {},
	"deterministic_greeting":                            {},
	"deterministic_lap_child_assignment":                {},
	"deterministic_passenger_count_reply":               {},
	"deterministic_passenger_documents":                 {},
	"deterministic_payment_preference":                  {},
	"deterministic_reservation_start":                   {},
	"deterministic_test":                                {},
	"deterministic_out_of_turn_info":                    {},
	"interpreter_case_eval":                             {},
	"latest_assistant_asked_document_confirmation":      {},
	"latest_assistant_asked_lap_child_assignment":       {},
	"local":                              {},
	"local_structured":                   {},
	"no_deterministic_match":             {},
	"openai_interpreter_assist_rejected": {},
	"openai_structured":                  {},
	"test":                               {},
	"test_low_confidence":                {},
	"unit_test":                          {},
}

var structuredInterpreterShadowTurnMeaningMetricKeys = map[string]struct{}{
	string(TurnMeaningUnknown):          {},
	string(TurnMeaningGreeting):         {},
	string(TurnMeaningNewRequest):       {},
	string(TurnMeaningAnswerToQuestion): {},
	string(TurnMeaningConfirmation):     {},
}

var structuredInterpreterShadowRejectReasonMetricKeys = map[string]struct{}{
	"active_prompt_required":                       {},
	"ambiguous_availability_option_reply":          {},
	"ambiguous_document_confirmation":              {},
	"availability_option_index_out_of_range":       {},
	"availability_options_unavailable":             {},
	"availability_route_required":                  {},
	"availability_search_not_allowed_in_phase":     {},
	"availability_selection_missing_current_facts": {},
	"booking_cancel_not_allowed_in_openai_assist":  {},
	"child_under_5_count_mismatch":                 {},
	"child_under_5_reply_required":                 {},
	"context_canceled":                             {},
	"context_deadline_exceeded":                    {},
	"document_confirmation_required":               {},
	"empty_current_turn":                           {},
	"intent_not_allowed_by_active_prompt":          {},
	"invalid_availability_date":                    {},
	"invalid_passenger_document":                   {},
	"lap_child_assignment_index_mismatch":          {},
	"lap_child_assignment_index_out_of_range":      {},
	"lap_child_assignment_index_required":          {},
	"local_result_not_eligible":                    {},
	"openai_assist_tool_action_not_allowed":        {},
	"openai_confidence_below_threshold":            {},
	"openai_disabled":                              {},
	"openai_error":                                 {},
	"openai_missing_result":                        {},
	"openai_schema_invalid":                        {},
	"openai_structured_interpreter_disabled":       {},
	"openai_structured_interpreter_empty_output":   {},
	"openai_structured_interpreter_error":          {},
	"openai_structured_interpreter_invalid_output": {},
	"openai_structured_interpreter_request_failed": {},
	"passenger_count_mismatch":                     {},
	"passenger_count_required":                     {},
	"passenger_document_slot_required":             {},
	"passenger_documents_required":                 {},
	"payer_cpf_not_allowed_in_phase":               {},
	"payer_cpf_required":                           {},
	"payer_cpf_slot_required":                      {},
	"payment_preference_mismatch":                  {},
	"payment_preference_required":                  {},
	"pix_is_method_not_payment_preference":         {},
	"reservation_help_turn_meaning_mismatch":       {},
	"selected_option_index_mismatch":               {},
	"selected_option_index_required":               {},
	"shadow_disabled":                              {},
	"unsupported_destination":                      {},
	"unsafe_side_effect_claim":                     {},
	"unknown_intent":                               {},
}

var structuredInterpreterShadowFallbackTemplateMetricKeys = map[string]struct{}{
	string(TemplateAskPassengerCount):                   {},
	string(TemplateAskChildUnder5):                      {},
	string(TemplateAskDocuments):                        {},
	string(TemplateAskLapChildAssignment):               {},
	string(TemplateAskPaymentChoice):                    {},
	string(TemplateAskMAOrigin):                         {},
	string(TemplateAskMADestination):                    {},
	string(TemplateAskSCOrigin):                         {},
	string(TemplateAskSCOriginForMA):                    {},
	string(TemplateAskReservationRouteSC):               {},
	string(TemplatePublicSCTable):                       {},
	string(TemplateAvailabilityList):                    {},
	string(TemplateNoAvailability):                      {},
	string(TemplateUnsupportedCargo):                    {},
	string(TemplateUnsupportedPackage):                  {},
	string(TemplateHumanHandoff):                        {},
	string(TemplateBookingCreated):                      {},
	string(TemplateConfirmDocument):                     {},
	string(TemplatePaymentCreate):                       {},
	string(TemplatePaymentMethods):                      {},
	string(TemplatePaymentOptionsInfo):                  {},
	string(TemplatePayingPassengerInfo):                 {},
	string(TemplateDocumentRequirementsInfo):            {},
	string(TemplateChildPolicyInfo):                     {},
	string(TemplateBaggageInfo):                         {},
	string(TemplateBoardingInfo):                        {},
	string(TemplateHumanSupportInfo):                    {},
	string(TemplateContextFallbackAvailabilityOption):   {},
	string(TemplateContextFallbackAvailabilityDate):     {},
	string(TemplateContextFallbackPassengerCount):       {},
	string(TemplateContextFallbackChildUnder5):          {},
	string(TemplateContextFallbackLapChildAssignment):   {},
	string(TemplateContextFallbackPassengerDocuments):   {},
	string(TemplateContextFallbackDocumentConfirmation): {},
	string(TemplateContextFallbackPaymentPreference):    {},
	string(TemplateContextFallbackPayerCPF):             {},
}

var structuredInterpreterShadowConfidenceBucketMetricKeys = map[string]struct{}{
	"0.00-0.49": {},
	"0.50-0.69": {},
	"0.70-0.84": {},
	"0.85-1.00": {},
	"invalid":   {},
}

type StructuredInterpreterShadowReportItem struct {
	Summary              StructuredInterpreterShadowSummary
	ValidationErrors     []string
	SensitiveScanPayload any
}

type StructuredInterpreterShadowReport struct {
	TotalItems                  int                                               `json:"total_items"`
	OpenAIStatusCounts          map[string]int                                    `json:"openai_status_counts"`
	LocalIntentCounts           map[string]int                                    `json:"local_intent_counts"`
	OpenAIIntentCounts          map[string]int                                    `json:"openai_intent_counts"`
	IntentAgreementCount        int                                               `json:"intent_agreement_count"`
	IntentDisagreementCount     int                                               `json:"intent_disagreement_count"`
	IntentAgreementRate         float64                                           `json:"intent_agreement_rate"`
	DisagreementsByLocalIntent  map[string]int                                    `json:"disagreements_by_local_intent"`
	DisagreementsByOpenAIIntent map[string]int                                    `json:"disagreements_by_openai_intent"`
	OpenAIErrorCount            int                                               `json:"openai_error_count"`
	ValidationErrorCount        int                                               `json:"validation_error_count"`
	LatencyCount                int                                               `json:"latency_count"`
	LatencyAvgMs                float64                                           `json:"latency_avg_ms"`
	LatencyMaxMs                int64                                             `json:"latency_max_ms"`
	SensitiveLeakDetected       bool                                              `json:"sensitive_leak_detected"`
	SensitiveLeakCount          int                                               `json:"sensitive_leak_count"`
	OpenAIValidation            StructuredInterpreterShadowOpenAIValidationReport `json:"openai_validation"`
}

type StructuredInterpreterShadowOpenAIValidationStatusCounts struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
	Skipped  int `json:"skipped"`
}

type StructuredInterpreterShadowOpenAIValidationReport struct {
	Total                              int                                                                `json:"total"`
	Accepted                           int                                                                `json:"accepted"`
	Rejected                           int                                                                `json:"rejected"`
	Skipped                            int                                                                `json:"skipped"`
	UnknownStatusCount                 int                                                                `json:"unknown_status_count"`
	ByStatus                           map[string]int                                                     `json:"by_status"`
	ByLocalIntent                      map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts `json:"by_local_intent"`
	ByOpenAIIntent                     map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts `json:"by_openai_intent"`
	ByLocalSource                      map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts `json:"by_local_source"`
	ByOpenAISource                     map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts `json:"by_openai_source"`
	ByLocalTurnMeaning                 map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts `json:"by_local_turn_meaning"`
	ByOpenAITurnMeaning                map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts `json:"by_openai_turn_meaning"`
	ByRejectReason                     map[string]int                                                     `json:"by_reject_reason"`
	ByFallbackTemplate                 map[string]int                                                     `json:"by_fallback_template"`
	ByConfidenceBucket                 map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts `json:"by_confidence_bucket"`
	LocalUnknownOpenAIAccepted         int                                                                `json:"local_unknown_openai_accepted"`
	LocalUnknownOpenAIRejected         int                                                                `json:"local_unknown_openai_rejected"`
	LocalUnknownAcceptedByOpenAIIntent map[string]int                                                     `json:"local_unknown_accepted_by_openai_intent"`
	LocalUnknownRejectedByOpenAIIntent map[string]int                                                     `json:"local_unknown_rejected_by_openai_intent"`
	IntentMismatchOpenAIAccepted       int                                                                `json:"intent_mismatch_openai_accepted"`
	IntentMismatchOpenAIRejected       int                                                                `json:"intent_mismatch_openai_rejected"`
	RejectedByActivePromptCount        int                                                                `json:"rejected_by_active_prompt_count"`
	RejectedByMissingCurrentFactsCount int                                                                `json:"rejected_by_missing_current_facts_count"`
}

func BuildStructuredInterpreterShadowReport(items []StructuredInterpreterShadowReportItem) StructuredInterpreterShadowReport {
	report := StructuredInterpreterShadowReport{
		TotalItems:                  len(items),
		OpenAIStatusCounts:          map[string]int{},
		LocalIntentCounts:           map[string]int{},
		OpenAIIntentCounts:          map[string]int{},
		DisagreementsByLocalIntent:  map[string]int{},
		DisagreementsByOpenAIIntent: map[string]int{},
		OpenAIValidation:            newStructuredInterpreterShadowOpenAIValidationReport(),
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

		addStructuredInterpreterShadowOpenAIValidationMetrics(&report.OpenAIValidation, summary)
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

func newStructuredInterpreterShadowOpenAIValidationReport() StructuredInterpreterShadowOpenAIValidationReport {
	return StructuredInterpreterShadowOpenAIValidationReport{
		ByStatus:                           map[string]int{},
		ByLocalIntent:                      map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts{},
		ByOpenAIIntent:                     map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts{},
		ByLocalSource:                      map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts{},
		ByOpenAISource:                     map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts{},
		ByLocalTurnMeaning:                 map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts{},
		ByOpenAITurnMeaning:                map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts{},
		ByRejectReason:                     map[string]int{},
		ByFallbackTemplate:                 map[string]int{},
		ByConfidenceBucket:                 map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts{},
		LocalUnknownAcceptedByOpenAIIntent: map[string]int{},
		LocalUnknownRejectedByOpenAIIntent: map[string]int{},
	}
}

func addStructuredInterpreterShadowOpenAIValidationMetrics(report *StructuredInterpreterShadowOpenAIValidationReport, summary StructuredInterpreterShadowSummary) {
	validation := summary.OpenAIValidation
	if !structuredInterpreterShadowOpenAIValidationHasData(validation) {
		return
	}

	status := structuredInterpreterShadowOpenAIValidationStatus(validation)
	statusKey := structuredInterpreterShadowValidationStatusMetricKey(status)
	if statusKey == "" {
		statusKey = "missing"
	}

	report.Total++
	report.ByStatus[statusKey]++

	switch status {
	case string(StructuredInterpreterShadowValidationAccepted):
		report.Accepted++
	case string(StructuredInterpreterShadowValidationRejected):
		report.Rejected++
	case string(StructuredInterpreterShadowValidationSkipped):
		report.Skipped++
	default:
		report.UnknownStatusCount++
	}

	addStructuredInterpreterShadowValidationDimension(report.ByLocalIntent, summary.Local.Intent, status, structuredInterpreterShadowIntentMetricKey)
	addStructuredInterpreterShadowValidationDimension(report.ByOpenAIIntent, summary.OpenAI.Intent, status, structuredInterpreterShadowIntentMetricKey)
	addStructuredInterpreterShadowValidationDimension(report.ByLocalSource, summary.Local.Source, status, structuredInterpreterShadowSourceMetricKey)
	addStructuredInterpreterShadowValidationDimension(report.ByOpenAISource, summary.OpenAI.Source, status, structuredInterpreterShadowSourceMetricKey)
	addStructuredInterpreterShadowValidationDimension(report.ByLocalTurnMeaning, summary.Local.TurnMeaning, status, structuredInterpreterShadowTurnMeaningMetricKey)
	addStructuredInterpreterShadowValidationDimension(report.ByOpenAITurnMeaning, summary.OpenAI.TurnMeaning, status, structuredInterpreterShadowTurnMeaningMetricKey)

	rejectReason := structuredInterpreterShadowRejectReasonMetricKey(validation.RejectReason)
	if rejectReason != "" {
		report.ByRejectReason[rejectReason]++
	}
	fallbackTemplate := structuredInterpreterShadowFallbackTemplateMetricKey(string(validation.FallbackTemplate))
	if fallbackTemplate != "" {
		report.ByFallbackTemplate[fallbackTemplate]++
	}

	if structuredInterpreterShadowOpenAIValidationHasProposal(summary.OpenAI) {
		addStructuredInterpreterShadowValidationDimension(report.ByConfidenceBucket, structuredInterpreterShadowConfidenceBucket(summary.OpenAI.Confidence), status, structuredInterpreterShadowConfidenceBucketMetricKey)
	}

	localIntent := strings.TrimSpace(summary.Local.Intent)
	openAIIntent := strings.TrimSpace(summary.OpenAI.Intent)
	if localIntent == string(StructuredIntentUnknown) && openAIIntent != "" {
		switch status {
		case string(StructuredInterpreterShadowValidationAccepted):
			report.LocalUnknownOpenAIAccepted++
			if key := structuredInterpreterShadowIntentMetricKey(openAIIntent); key != "" {
				report.LocalUnknownAcceptedByOpenAIIntent[key]++
			}
		case string(StructuredInterpreterShadowValidationRejected):
			report.LocalUnknownOpenAIRejected++
			if key := structuredInterpreterShadowIntentMetricKey(openAIIntent); key != "" {
				report.LocalUnknownRejectedByOpenAIIntent[key]++
			}
		}
	}

	if localIntent != "" && openAIIntent != "" && localIntent != openAIIntent {
		switch status {
		case string(StructuredInterpreterShadowValidationAccepted):
			report.IntentMismatchOpenAIAccepted++
		case string(StructuredInterpreterShadowValidationRejected):
			report.IntentMismatchOpenAIRejected++
		}
	}

	if status == string(StructuredInterpreterShadowValidationRejected) {
		if structuredInterpreterShadowRejectReasonIsActivePrompt(rejectReason) {
			report.RejectedByActivePromptCount++
		}
		if structuredInterpreterShadowRejectReasonIsMissingCurrentFacts(rejectReason) {
			report.RejectedByMissingCurrentFactsCount++
		}
	}
}

func structuredInterpreterShadowOpenAIValidationHasData(validation OpenAIInterpreterShadowValidation) bool {
	return strings.TrimSpace(validation.Status) != "" ||
		validation.Accepted ||
		strings.TrimSpace(validation.RejectReason) != "" ||
		strings.TrimSpace(string(validation.FallbackTemplate)) != ""
}

func structuredInterpreterShadowOpenAIValidationStatus(validation OpenAIInterpreterShadowValidation) string {
	status := strings.TrimSpace(validation.Status)
	if status != "" {
		return status
	}
	if validation.Accepted {
		return string(StructuredInterpreterShadowValidationAccepted)
	}
	return ""
}

func addStructuredInterpreterShadowValidationDimension(
	counts map[string]StructuredInterpreterShadowOpenAIValidationStatusCounts,
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
	case string(StructuredInterpreterShadowValidationAccepted):
		value.Accepted++
	case string(StructuredInterpreterShadowValidationRejected):
		value.Rejected++
	case string(StructuredInterpreterShadowValidationSkipped):
		value.Skipped++
	default:
		return
	}
	counts[key] = value
}

func structuredInterpreterShadowOpenAIValidationHasProposal(summary OpenAIInterpreterShadowSummary) bool {
	return strings.TrimSpace(summary.Intent) != "" ||
		strings.TrimSpace(summary.TurnMeaning) != "" ||
		strings.TrimSpace(summary.Source) != "" ||
		summary.Confidence != 0
}

func structuredInterpreterShadowConfidenceBucket(confidence float64) string {
	switch {
	case confidence < 0:
		return "invalid"
	case confidence < 0.50:
		return "0.00-0.49"
	case confidence < 0.70:
		return "0.50-0.69"
	case confidence < 0.85:
		return "0.70-0.84"
	case confidence <= 1:
		return "0.85-1.00"
	default:
		return "invalid"
	}
}

func structuredInterpreterShadowValidationStatusMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, structuredInterpreterShadowValidationStatusMetricKeys)
}

func structuredInterpreterShadowIntentMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, structuredInterpreterShadowIntentMetricKeys)
}

func structuredInterpreterShadowSourceMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, structuredInterpreterShadowSourceMetricKeys)
}

func structuredInterpreterShadowTurnMeaningMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, structuredInterpreterShadowTurnMeaningMetricKeys)
}

func structuredInterpreterShadowRejectReasonMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, structuredInterpreterShadowRejectReasonMetricKeys)
}

func structuredInterpreterShadowFallbackTemplateMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, structuredInterpreterShadowFallbackTemplateMetricKeys)
}

func structuredInterpreterShadowConfidenceBucketMetricKey(value string) string {
	return structuredInterpreterShadowKnownMetricKey(value, structuredInterpreterShadowConfidenceBucketMetricKeys)
}

func structuredInterpreterShadowRejectReasonIsActivePrompt(reason string) bool {
	switch reason {
	case "active_prompt_required", "intent_not_allowed_by_active_prompt":
		return true
	default:
		return false
	}
}

func structuredInterpreterShadowRejectReasonIsMissingCurrentFacts(reason string) bool {
	return reason == "availability_selection_missing_current_facts"
}

func structuredInterpreterShadowKnownMetricKey(value string, known map[string]struct{}) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if _, ok := known[trimmed]; ok && !structuredInterpreterShadowReportMetricKeyLooksSensitive(trimmed) {
		return trimmed
	}
	return structuredInterpreterShadowRedactedMetricKey
}

func structuredInterpreterShadowReportMetricKeyLooksSensitive(value string) bool {
	if len([]rune(value)) > 96 {
		return true
	}

	lower := strings.ToLower(value)
	for _, marker := range []string{"data:image", "data:application", "data:audio"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if structuredInterpreterShadowContainsPossibleCPF(value) {
		return true
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return true
	}
	if strings.Contains(value, " ") {
		return true
	}

	digitRunLength := 0
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digitRunLength++
			if digitRunLength >= 8 {
				return true
			}
			continue
		}
		digitRunLength = 0
	}

	return false
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
