package chat

import "strings"

const (
	repeatedQuestionHandoffReason = "repeated_question_handoff"
	maxHelpRequestTokens          = 6
)

// helpRequestPhrases are explicit "I need help" phrases that mean the customer
// wants a human. They are matched as whole-token sequences, so "ajuda de custo"
// or "ajudante" do not trigger.
var helpRequestPhrases = [][]string{
	{"preciso", "de", "ajuda"},
	{"quero", "ajuda"},
	{"me", "ajuda"},
	{"ajuda", "por", "favor"},
	{"ajuda", "pf"},
	{"ajuda", "pfv"},
	{"pessoa", "de", "verdade"},
}

func looksLikeExplicitHelpRequest(folded string) bool {
	tokens := strings.Fields(folded)
	if len(tokens) == 0 || len(tokens) > maxHelpRequestTokens {
		return false
	}
	if len(tokens) == 1 && tokens[0] == "ajuda" {
		return true
	}
	for _, phrase := range helpRequestPhrases {
		for i := 0; i+len(phrase) <= len(tokens); i++ {
			match := true
			for j, word := range phrase {
				if tokens[i+j] != word {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// replyAlreadySentTwiceInARow reports whether the last two OUTBOUND messages of
// the session are both equal to reply (normalized). A single re-ask is still
// allowed; sending the same text a third time is the loop we stop.
func replyAlreadySentTwiceInARow(history []Message, reply string) bool {
	want := normalizeGuardrailPhrase(reply)
	if want == "" {
		return false
	}
	seen := 0
	for i := len(history) - 1; i >= 0 && seen < 2; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		if normalizeGuardrailPhrase(messageTurnText(history[i])) != want {
			return false
		}
		seen++
	}
	return seen == 2
}

// repeatedQuestionHandoffRun replaces a safe-phase fallback draft that would
// repeat the same outbound message again (third identical send in a row) with
// the human handoff template.
func repeatedQuestionHandoffRun(history []Message, run RunAgentResult, toolCalls []ToolCall) (RunAgentResult, bool) {
	if len(toolCalls) > 0 || strings.TrimSpace(run.ReplyText) == "" {
		return run, false
	}
	if asString(run.ResponsePayload["template_name"]) != safePhaseFallbackTemplateName {
		return run, false
	}
	if !replyAlreadySentTwiceInARow(history, run.ReplyText) {
		return run, false
	}
	reply, ok := realizeResponseTemplate(TemplateHumanHandoff)
	if !ok {
		return run, false
	}
	decision := IntentDecision{Intent: IntentHumanSupport, Source: repeatedQuestionHandoffReason, TemplateName: TemplateHumanHandoff, Action: "template"}
	handoff := buildTemplateDraftRunFromDecision(decision, reply)
	for _, payload := range []map[string]interface{}{handoff.RequestPayload, handoff.ResponsePayload} {
		payload["fallback_reason"] = repeatedQuestionHandoffReason
	}
	return handoff, true
}

func draftRequestsHandoffAfterSend(draft Message) bool {
	for _, payload := range []map[string]interface{}{draft.NormalizedPayload, draft.Payload} {
		if asString(payload["fallback_reason"]) == repeatedQuestionHandoffReason {
			return true
		}
	}
	return false
}
