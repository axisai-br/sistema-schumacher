package chat

import "strings"

const administrativeNotesSupportReply = "Para assuntos sobre notas ou financeiro, vou te encaminhar para o suporte da Schumacher Tur."

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

func buildAdministrativeNotesSupportDraftRun() RunAgentResult {
	decision := IntentDecision{
		Intent:       IntentHumanSupportInfoQuestion,
		Source:       "deterministic_administrative_notes_support",
		TemplateName: TemplateHumanSupportInfo,
		Action:       "template",
		TemplateData: map[string]interface{}{
			"topic": "administrative_notes_finance",
		},
	}
	return buildTemplateDraftRunFromDecision(decision, administrativeNotesSupportReply)
}
