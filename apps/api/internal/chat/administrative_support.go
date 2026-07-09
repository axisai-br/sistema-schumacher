package chat

import "strings"

const (
	administrativeNotesSupportDecisionSource = "deterministic_administrative_notes_support"
	administrativeNotesSupportReply          = "Para assuntos sobre notas ou financeiro, fale com o suporte da Schumacher Tur: +55 49 9886-2222."
)

func looksLikeAdministrativeNotesSupportQuestion(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	return containsFoldedPhrase(folded, []string{
		"baixa das notas",
		"baixa de notas",
		"baixa da nota",
		"baixa de nota",
		"notas",
		"nota fiscal",
		"notas fiscais",
		"faturamento",
		"financeiro",
		"emissao de nota",
		"emissao da nota",
		"emitir nota",
		"comprovante fiscal",
	})
}

func shouldRouteAdministrativeNotesSupportTurn(phase ConversationPhase, history []Message, currentTurn string, session Session, documentCollectionMediaTurn bool) bool {
	if documentCollectionMediaTurn || !looksLikeAdministrativeNotesSupportQuestion(currentTurn) {
		return false
	}
	if looksLikePassengerDocumentText(currentTurn, session) {
		return false
	}
	if phase == ConversationPhaseBookingPending || phase == ConversationPhasePassengerCollection {
		return true
	}
	if lastAssistantAskedPassengerDocumentRequest(history) ||
		lastAssistantAskedDocumentConfirmation(history) ||
		lastAssistantAskedBookingProceedConfirmation(history) {
		return true
	}
	activePrompt := InferActivePromptContext(history, CanonicalConversationState{Phase: phase})
	return activePrompt.Kind == ActivePromptPassengerDocuments
}

func buildAdministrativeNotesSupportIntentDecision() IntentDecision {
	return IntentDecision{
		Intent:       IntentHumanSupportInfoQuestion,
		Source:       administrativeNotesSupportDecisionSource,
		TemplateName: TemplateHumanSupportInfo,
		Action:       "template",
		TemplateData: map[string]interface{}{
			"topic": "administrative_notes_finance",
		},
	}
}

func buildAdministrativeNotesSupportDraftRun() RunAgentResult {
	decision := buildAdministrativeNotesSupportIntentDecision()
	return buildTemplateDraftRunFromDecision(decision, administrativeNotesSupportReply)
}
