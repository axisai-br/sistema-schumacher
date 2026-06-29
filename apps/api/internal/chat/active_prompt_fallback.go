package chat

import (
	"strconv"
	"strings"
)

func buildActivePromptLapChildAssignmentFallbackDecision(source string) IntentDecision {
	return buildActivePromptContextualFallbackTemplateDecision(source, TemplateContextFallbackLapChildAssignment)
}

func buildActivePromptContextualFallbackDecision(ctx ActivePromptContext, body string, folded string, state CanonicalConversationState) (IntentDecision, bool) {
	_ = folded

	switch ctx.Kind {
	case ActivePromptAvailabilityOptionChoice:
		optionCount := ctx.AvailabilityOptionCount
		if optionCount > 0 {
			if index := extractSelectedOptionIndex(body); index > 0 && index <= optionCount {
				return IntentDecision{}, false
			}
		}
		if activePromptFallbackLooksLikeDateSelection(body) {
			return IntentDecision{}, false
		}
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_availability_option",
			TemplateContextFallbackAvailabilityOption,
		), true
	case ActivePromptAvailabilityDateChoice:
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_availability_date",
			TemplateContextFallbackAvailabilityDate,
		), true
	case ActivePromptPassengerCount:
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_passenger_count",
			TemplateContextFallbackPassengerCount,
		), true
	case ActivePromptLapChildQuestion:
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_child_under_5",
			TemplateContextFallbackChildUnder5,
		), true
	case ActivePromptLapChildAssignment:
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_lap_child_assignment",
			TemplateContextFallbackLapChildAssignment,
		), true
	case ActivePromptPassengerDocuments:
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_passenger_documents",
			TemplateContextFallbackPassengerDocuments,
		), true
	case ActivePromptDocumentConfirmation:
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_document_confirmation",
			TemplateContextFallbackDocumentConfirmation,
		), true
	case ActivePromptPaymentPreference:
		if looksLikeUnsupportedPaymentMethodQuestion(body) {
			return IntentDecision{}, false
		}
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_payment_preference",
			TemplateContextFallbackPaymentPreference,
		), true
	case ActivePromptPayerCPF:
		if state.Phase != "" && !isPaymentDocumentReplyPhase(state.Phase) {
			return IntentDecision{}, false
		}
		return buildActivePromptContextualFallbackTemplateDecision(
			"deterministic_active_prompt_fallback_payer_cpf",
			TemplateContextFallbackPayerCPF,
		), true
	default:
		return IntentDecision{}, false
	}
}

func buildActivePromptContextualFallbackTemplateDecision(source string, templateName ResponseTemplateName) IntentDecision {
	return IntentDecision{
		Intent:       IntentUnknown,
		Source:       strings.TrimSpace(source),
		Action:       "template",
		TemplateName: templateName,
	}
}

func activePromptFallbackLooksLikeDateSelection(text string) bool {
	if match := isoDatePattern.FindStringSubmatch(text); len(match) == 4 {
		return true
	}
	if match := brDatePattern.FindStringSubmatch(text); len(match) >= 3 {
		day, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		return day >= 1 && day <= 31 && month >= 1 && month <= 12
	}
	return false
}

func activePromptLapChildAssignmentOptionCount(ctx ActivePromptContext) int {
	maxIndex := 0
	for _, line := range strings.Split(ctx.SourceMessageBody, "\n") {
		index, ok := parseLeadingNumberedListIndex(line)
		if !ok {
			continue
		}
		if index > maxIndex {
			maxIndex = index
		}
	}
	return maxIndex
}

func activePromptLapChildAssignmentAnswerIndex(text string) int {
	if index := extractSelectedOptionIndex(text); index > 0 {
		return index
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	for _, char := range trimmed {
		if char < '0' || char > '9' {
			return 0
		}
	}
	index, err := strconv.Atoi(trimmed)
	if err != nil || index <= 0 {
		return 0
	}
	return index
}

func parseLeadingNumberedListIndex(line string) (int, bool) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] < '0' || line[0] > '9' {
		return 0, false
	}

	end := 0
	for end < len(line) && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	rest := strings.TrimLeft(line[end:], " \t")
	if len(rest) < 2 {
		return 0, false
	}
	switch rest[0] {
	case '.', ')', '-':
	default:
		return 0, false
	}
	if rest[1] != ' ' && rest[1] != '\t' {
		return 0, false
	}

	index, err := strconv.Atoi(line[:end])
	if err != nil || index <= 0 {
		return 0, false
	}
	return index, true
}
