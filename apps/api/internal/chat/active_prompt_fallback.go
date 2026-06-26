package chat

import (
	"strconv"
	"strings"
)

func buildActivePromptLapChildAssignmentFallbackDecision(source string) IntentDecision {
	return IntentDecision{
		Intent: IntentUnknown,
		Source: strings.TrimSpace(source),
		Action: "safe_fallback",
	}
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
