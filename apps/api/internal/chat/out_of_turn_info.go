package chat

import "strings"

type OutOfTurnInfoKind string

const (
	OutOfTurnInfoUnknown          OutOfTurnInfoKind = "UNKNOWN"
	OutOfTurnInfoPaymentOptions   OutOfTurnInfoKind = "PAYMENT_OPTIONS_INFO"
	OutOfTurnInfoPayingPassenger  OutOfTurnInfoKind = "PAYING_PASSENGER_INFO"
	OutOfTurnInfoDocuments        OutOfTurnInfoKind = "DOCUMENT_REQUIREMENTS_INFO"
	OutOfTurnInfoChildPolicy      OutOfTurnInfoKind = "CHILD_POLICY_INFO"
	OutOfTurnInfoBaggage          OutOfTurnInfoKind = "BAGGAGE_INFO"
	OutOfTurnInfoBoarding         OutOfTurnInfoKind = "BOARDING_INFO"
	OutOfTurnInfoHumanSupportInfo OutOfTurnInfoKind = "HUMAN_SUPPORT_INFO"
)

const (
	outOfTurnTemplateDataKey              = "out_of_turn_info"
	outOfTurnInfoKindTemplateDataKey      = "out_of_turn_info_kind"
	outOfTurnPendingPromptTemplateDataKey = "pending_prompt_template"
	outOfTurnActivePromptTemplateDataKey  = "active_prompt_kind"
	outOfTurnActivePromptSourceIDDataKey  = "active_prompt_source_message_id"
	outOfTurnRejectedAvailabilityDataKey  = "out_of_turn_rejected_availability_selection"
	outOfTurnRejectedOptionIndexesDataKey = "rejected_option_indexes"
	outOfTurnRejectedTripDatesDataKey     = "rejected_trip_dates"
	outOfTurnRejectedWholeContextDataKey  = "rejected_whole_context"
)

func buildOutOfTurnInfoDecision(text string, activePrompt ActivePromptContext) (IntentDecision, bool) {
	kind := detectOutOfTurnInfoQuestion(text, activePrompt)
	if kind == OutOfTurnInfoUnknown {
		return IntentDecision{}, false
	}

	templateName, ok := outOfTurnInfoTemplateName(kind)
	if !ok {
		return IntentDecision{}, false
	}
	pendingTemplate := outOfTurnPendingPromptTemplate(activePrompt.Kind)
	if pendingTemplate == "" {
		return IntentDecision{}, false
	}

	templateData := map[string]interface{}{
		outOfTurnTemplateDataKey:              true,
		outOfTurnInfoKindTemplateDataKey:      string(kind),
		outOfTurnPendingPromptTemplateDataKey: string(pendingTemplate),
		outOfTurnActivePromptTemplateDataKey:  string(activePrompt.Kind),
	}
	if sourceID := strings.TrimSpace(activePrompt.SourceMessageID); sourceID != "" {
		templateData[outOfTurnActivePromptSourceIDDataKey] = sourceID
	}
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if activePrompt.Kind == ActivePromptAvailabilityOptionChoice {
		rejection := parseAvailabilityRejectionEvidence(folded)
		if rejection.Found {
			templateData[outOfTurnRejectedAvailabilityDataKey] = true
			if len(rejection.OptionIndexes) > 0 {
				templateData[outOfTurnRejectedOptionIndexesDataKey] = append([]int(nil), rejection.OptionIndexes...)
			}
			if len(rejection.TripDates) > 0 {
				templateData[outOfTurnRejectedTripDatesDataKey] = append([]string(nil), rejection.TripDates...)
			}
			if rejection.WholeContext || !rejection.hasSpecificTarget() || activePrompt.AvailabilityOptionCount <= 1 {
				templateData[outOfTurnRejectedWholeContextDataKey] = true
			}
		}
	}

	return IntentDecision{
		Intent:       outOfTurnInfoIntent(kind),
		Source:       "deterministic_out_of_turn_info",
		TemplateName: templateName,
		Action:       "template",
		TemplateData: templateData,
	}, true
}

func canShortcutOutOfTurnInfoDecision(decision IntentDecision, state CanonicalConversationState) bool {
	return decision.Source == "deterministic_out_of_turn_info" &&
		decision.Action == "template" &&
		isInformationalTemplate(decision.TemplateName) &&
		canRealizeWithoutLLM(decision, state)
}

func detectOutOfTurnInfoQuestion(text string, activePrompt ActivePromptContext) OutOfTurnInfoKind {
	if !outOfTurnInfoActivePromptEligible(activePrompt.Kind) {
		return OutOfTurnInfoUnknown
	}

	body := NormalizeIncomingCustomerText(text)
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if folded == "" {
		return OutOfTurnInfoUnknown
	}

	switch {
	case looksLikePaymentOptionsInfoQuestion(folded):
		return OutOfTurnInfoPaymentOptions
	case looksLikePayingPassengerInfoQuestion(folded):
		return OutOfTurnInfoPayingPassenger
	case looksLikeDocumentRequirementsInfoQuestion(folded):
		return OutOfTurnInfoDocuments
	case looksLikeChildPolicyInfoQuestion(folded):
		return OutOfTurnInfoChildPolicy
	case looksLikeBaggageInfoQuestion(folded):
		return OutOfTurnInfoBaggage
	case looksLikeBoardingInfoQuestion(folded):
		return OutOfTurnInfoBoarding
	case looksLikeHumanSupportInfoQuestion(folded):
		return OutOfTurnInfoHumanSupportInfo
	default:
		return OutOfTurnInfoUnknown
	}
}

func outOfTurnInfoActivePromptEligible(kind ActivePromptKind) bool {
	switch kind {
	case ActivePromptAvailabilityOptionChoice,
		ActivePromptPassengerCount,
		ActivePromptLapChildQuestion,
		ActivePromptPassengerDocuments,
		ActivePromptDocumentConfirmation,
		ActivePromptPaymentPreference,
		ActivePromptPayerCPF:
		return true
	default:
		return false
	}
}

func outOfTurnInfoTemplateName(kind OutOfTurnInfoKind) (ResponseTemplateName, bool) {
	switch kind {
	case OutOfTurnInfoPaymentOptions:
		return TemplatePaymentOptionsInfo, true
	case OutOfTurnInfoPayingPassenger:
		return TemplatePayingPassengerInfo, true
	case OutOfTurnInfoDocuments:
		return TemplateDocumentRequirementsInfo, true
	case OutOfTurnInfoChildPolicy:
		return TemplateChildPolicyInfo, true
	case OutOfTurnInfoBaggage:
		return TemplateBaggageInfo, true
	case OutOfTurnInfoBoarding:
		return TemplateBoardingInfo, true
	case OutOfTurnInfoHumanSupportInfo:
		return TemplateHumanSupportInfo, true
	default:
		return "", false
	}
}

func outOfTurnInfoIntent(kind OutOfTurnInfoKind) Intent {
	switch kind {
	case OutOfTurnInfoPaymentOptions:
		return IntentPaymentInfoQuestion
	case OutOfTurnInfoPayingPassenger:
		return IntentPayingPassengerInfoQuestion
	case OutOfTurnInfoDocuments:
		return IntentDocumentRequirementsInfoQuestion
	case OutOfTurnInfoChildPolicy:
		return IntentChildPolicyInfoQuestion
	case OutOfTurnInfoBaggage:
		return IntentBaggageInfoQuestion
	case OutOfTurnInfoBoarding:
		return IntentBoardingInfoQuestion
	case OutOfTurnInfoHumanSupportInfo:
		return IntentHumanSupportInfoQuestion
	default:
		return IntentUnknown
	}
}

func outOfTurnPendingPromptTemplate(kind ActivePromptKind) ResponseTemplateName {
	switch kind {
	case ActivePromptAvailabilityOptionChoice:
		return TemplateContextFallbackAvailabilityOption
	case ActivePromptPassengerCount:
		return TemplateContextFallbackPassengerCount
	case ActivePromptLapChildQuestion:
		return TemplateContextFallbackChildUnder5
	case ActivePromptPassengerDocuments:
		return TemplateContextFallbackPassengerDocuments
	case ActivePromptDocumentConfirmation:
		return TemplateContextFallbackDocumentConfirmation
	case ActivePromptPaymentPreference:
		return TemplateContextFallbackPaymentPreference
	case ActivePromptPayerCPF:
		return TemplateContextFallbackPayerCPF
	default:
		return ""
	}
}

func looksLikeDocumentRequirementsInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}

	if containsFoldedAny(folded,
		"quais documentos",
		"qual documento",
		"que documento",
		"documentos precisa",
		"documento precisa",
		"documento da crianca",
		"documento do menor",
	) {
		return true
	}

	hasDocumentCue := containsFoldedAny(folded, "documento", "documentos", "cpf", "rg", "cnh", "certidao", "foto")
	if !hasDocumentCue {
		return false
	}
	return containsFoldedAny(folded,
		"precisa",
		"precisar",
		"tem que",
		"pode ser",
		"pode mandar",
		"posso mandar",
		"pode enviar",
		"posso enviar",
		"serve",
	)
}

func looksLikeChildPolicyInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	hasChildCue := containsFoldedAny(folded, "crianca", "menor", "colo", "5 anos", "cinco anos")
	if !hasChildCue {
		return false
	}
	return containsFoldedAny(folded,
		"paga",
		"pagante",
		"precisa pagar",
		"vai no colo",
		"viaja no colo",
		"como funciona",
		"tem que informar",
		"precisa informar",
	)
}

func looksLikeBaggageInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	hasBaggageCue := containsFoldedAny(folded, "bagagem", "bagagens", "mala", "malas", "mochila", "volume", "volumes")
	if !hasBaggageCue {
		return false
	}
	return containsFoldedAny(folded,
		"pode levar",
		"posso levar",
		"levar",
		"quantas",
		"quanto",
		"como funciona",
		"tem limite",
		"limite",
		"despachar",
	)
}

func looksLikeBoardingInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	if containsFoldedAny(folded, "embarque", "embarcar", "ponto de embarque", "local de embarque") {
		return true
	}
	return containsFoldedAny(folded, "onde pega", "onde pego", "onde sai", "local de saida", "horario de saida")
}

func looksLikeHumanSupportInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	hasSupportCue := containsFoldedAny(folded, "suporte", "atendimento", "atendente")
	if !hasSupportCue {
		return false
	}
	return containsFoldedAny(folded,
		"telefone",
		"numero",
		"contato",
		"whatsapp",
		"como falo",
		"como falar",
		"qual",
	)
}
