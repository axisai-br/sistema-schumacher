package chat

import (
	"regexp"
	"strings"
)

var (
	leadingAudioArtifactPattern = regexp.MustCompile(`(?i)^\s*e\s+(passagem|viagem|onibus|ônibus|quero)\b`)
)

type textReplacement struct {
	pattern     *regexp.Regexp
	replacement string
}

var customerTextReplacements = []textReplacement{
	{
		pattern:     regexp.MustCompile(`(?i)\b(?:freiburg|freiburgo|fraiburg|fraiburgo|frei burgo)\b`),
		replacement: "Fraiburgo",
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b(pra|para)\s+min\b`),
		replacement: "$1 mim",
	},
}

// NormalizeIncomingCustomerText fixes common ASR mistakes and travel-city variants
// before routing or persistence.
func NormalizeIncomingCustomerText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}

	text = leadingAudioArtifactPattern.ReplaceAllString(text, "$1")
	for _, item := range customerTextReplacements {
		text = item.pattern.ReplaceAllString(text, item.replacement)
	}

	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}
