package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type DocumentExtractPassenger struct {
	Name         string  `json:"name"`
	Document     string  `json:"document"`
	DocumentType string  `json:"document_type"`
	CPF          string  `json:"cpf,omitempty"`
	CNH          string  `json:"cnh,omitempty"`
	RG           string  `json:"rg,omitempty"`
	BirthDate    string  `json:"birth_date,omitempty"`
	Confidence   float64 `json:"confidence"`
}

type DocumentExtractResult struct {
	Mode                   string                     `json:"mode"`
	Passengers             []DocumentExtractPassenger `json:"passengers"`
	ExpectedPassengerCount int                        `json:"expected_passenger_count"`
	MediaCount             int                        `json:"media_count"`
	FailureReason          string                     `json:"failure_reason,omitempty"`
	RawText                string                     `json:"raw_text,omitempty"`
	Model                  string                     `json:"model,omitempty"`
	ProviderResponseID     string                     `json:"provider_response_id,omitempty"`
	RequestPayload         map[string]interface{}     `json:"request_payload,omitempty"`
	ResponsePayload        map[string]interface{}     `json:"response_payload,omitempty"`
}

func (s *Service) resolveDocumentExtractContext(ctx context.Context, session Session, candidates []Message, memory map[string]interface{}, draftID string) (agentToolContext, bool, error) {
	media := collectCandidateMedia(candidates)
	recent := normalizeRecentMemoryMessages(memory["recent_messages"])
	if len(media) == 0 || (!isWaitingForPassengerDocuments(recent) && !hasRecentDocumentRequest(recent)) {
		return agentToolContext{}, false, nil
	}
	extractableMedia := documentExtractImageMedia(media)

	expected := inferExpectedPassengerCountFromMemory(
		strings.TrimSpace(asString(memory["current_turn_body"])),
		recent,
	)

	/* Log for init of extraction */
	s.logReprocess(
		"document_extract event=start session_id=%s message_id=%s media_count=%d expected=%d should_run=%t",
		session.ID,
		latestCandidateMessageID(candidates),
		len(media),
		expected,
		shouldRunDocumentExtract(memory),
	)

	startedAt := time.Now().UTC()
	requestPayload := map[string]interface{}{
		"mode":                     "DOCUMENT_EXTRACT",
		"current_turn_message_ids": candidateMessageIDs(candidates),
		"media_count":              len(media),
		"expected_passenger_count": expected,
	}

	/* Log of medias receives */
	for index, item := range media {
		s.logReprocess(
			"document_extract event=media session_id=%s index=%d kind=%s mime=%s url_present=%t",
			session.ID,
			index,
			strings.TrimSpace(item.Kind),
			strings.TrimSpace(item.MimeType),
			strings.TrimSpace(item.URL) != "",
		)
	}

	if len(extractableMedia) == 0 && hasDocumentExtractPDFMedia(media) {
		result := DocumentExtractResult{
			Mode:                   "LOW_CONFIDENCE",
			ExpectedPassengerCount: expected,
			MediaCount:             len(media),
			FailureReason:          "unsupported_pdf",
			Model:                  "document_extract_guardrail",
		}
		return agentToolContext{DocumentExtract: &result}, true, nil
	}

	run, result, runErr := s.runDocumentExtract(ctx, session, candidates, extractableMedia, expected, draftID)
	finishedAt := time.Now().UTC()
	finishedAtPtr := &finishedAt

	if runErr != nil {

		/* Log when runDocumentExtract fails */
		s.logReprocess(
			"document_extract event=run_failed session_id=%s message_id=%s media_count=%d expected=%d error=%v",
			session.ID,
			latestCandidateMessageID(candidates),
			len(media),
			expected,
			runErr,
		)

		call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:      session.ID,
			MessageID:      latestCandidateMessageID(candidates),
			ToolName:       toolNameDocumentExtract,
			RequestPayload: requestPayload,
			Status:         "FAILED",
			ErrorCode:      "DOCUMENT_EXTRACT_ERROR",
			ErrorMessage:   strings.TrimSpace(runErr.Error()),
			StartedAt:      startedAt,
			FinishedAt:     finishedAtPtr,
		})
		if err != nil {
			return agentToolContext{}, false, err
		}
		return agentToolContext{Calls: []ToolCall{call}}, false, nil
	}

	responsePayload := buildDocumentExtractResponsePayload(result)

	/* Log when the extraction work */
	s.logReprocess(
		"document_extract event=run_done session_id=%s message_id=%s mode=%s passenger_count=%d expected=%d media_count=%d failure_reason=%s model=%s",
		session.ID,
		latestCandidateMessageID(candidates),
		strings.TrimSpace(result.Mode),
		len(result.Passengers),
		result.ExpectedPassengerCount,
		result.MediaCount,
		strings.TrimSpace(result.FailureReason),
		strings.TrimSpace(result.Model),
	)

	call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
		SessionID:       session.ID,
		MessageID:       latestCandidateMessageID(candidates),
		ToolName:        toolNameDocumentExtract,
		RequestPayload:  requestPayload,
		ResponsePayload: responsePayload,
		Status:          "COMPLETED",
		StartedAt:       startedAt,
		FinishedAt:      finishedAtPtr,
	})
	if err != nil {
		return agentToolContext{}, false, err
	}

	result.RequestPayload = run.RequestPayload
	result.ResponsePayload = run.ResponsePayload
	return agentToolContext{
		Calls:           []ToolCall{call},
		DocumentExtract: &result,
	}, true, nil
}

func shouldRunDocumentExtract(memory map[string]interface{}) bool {
	recent := normalizeRecentMemoryMessages(memory["recent_messages"])
	media := normalizeMediaMemoryItems(memory["current_turn_media"])
	return len(media) > 0 && (isWaitingForPassengerDocuments(recent) || hasRecentDocumentRequest(recent))
}

func documentExtractImageMedia(media []AgentMediaInput) []AgentMediaInput {
	items := make([]AgentMediaInput, 0, len(media))
	for _, item := range media {
		if strings.EqualFold(strings.TrimSpace(item.Kind), "IMAGE") ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.MimeType)), "image/") {
			items = append(items, item)
		}
	}
	return items
}

func hasDocumentExtractPDFMedia(media []AgentMediaInput) bool {
	for _, item := range media {
		if strings.EqualFold(strings.TrimSpace(item.Kind), "PDF") ||
			strings.EqualFold(strings.TrimSpace(item.MimeType), "application/pdf") ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.URL)), "data:application/pdf") {
			return true
		}
	}
	return false
}

func hasRecentDocumentRequest(recent []map[string]interface{}) bool {
	for i := len(recent) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(asString(recent[i]["direction"])), "OUTBOUND") {
			continue
		}
		body := strings.Join(strings.Fields(foldChatText(asString(recent[i]["body"]))), " ")
		if body == "" {
			continue
		}
		if strings.Contains(body, "documento") || strings.Contains(body, "documentos") {
			if strings.Contains(body, "foto") || strings.Contains(body, "enviar") || strings.Contains(body, "digitar") || strings.Contains(body, "nome completo") {
				return true
			}
		}
	}
	return false
}

func (s *Service) runDocumentExtract(ctx context.Context, session Session, candidates []Message, media []AgentMediaInput, expected int, draftID string) (RunAgentResult, DocumentExtractResult, error) {
	/* Log of extract document */
	s.logReprocess(
		"document_extract event=runner_start session_id=%s message_count=%d media_count=%d expected=%d draft_id=%s",
		session.ID,
		len(candidates),
		len(media),
		expected,
		strings.TrimSpace(draftID),
	)

	run, err := s.runner.Run(ctx, RunAgentInput{
		Session:          session,
		CurrentTurnIDs:   candidateMessageIDs(candidates),
		CurrentTurnMedia: media,
		SystemPrompt:     buildDocumentExtractSystemPrompt(),
		UserPrompt:       buildDocumentExtractUserPrompt(expected),
		IdempotencyKey:   strings.TrimSpace(draftID) + "-document-extract",
		TextFormat:       buildDocumentExtractTextFormat(),
	})
	if err != nil {
		/* Log fails */
		s.logReprocess(
			"document_extract event=runner_failed session_id=%s media_count=%d expected=%d error=%v",
			session.ID,
			len(media),
			expected,
			err,
		)
		return RunAgentResult{}, DocumentExtractResult{}, err
	}

	result := parseDocumentExtractResult(run.ReplyText)

	/* Log parser */
	s.logReprocess(
		"document_extract event=parse_done session_id=%s mode=%s passenger_count=%d failure_reason=%s raw_len=%d model=%s provider_response_id=%s",
		session.ID,
		strings.TrimSpace(result.Mode),
		len(result.Passengers),
		strings.TrimSpace(result.FailureReason),
		len(strings.TrimSpace(run.ReplyText)),
		strings.TrimSpace(run.Model),
		strings.TrimSpace(run.ProviderResponseID),
	)

	result.ExpectedPassengerCount = expected
	result.MediaCount = len(media)
	result.RawText = strings.TrimSpace(run.ReplyText)
	result.Model = strings.TrimSpace(run.Model)
	result.ProviderResponseID = strings.TrimSpace(run.ProviderResponseID)
	return run, result, nil
}

func buildDocumentExtractSystemPrompt() string {
	return strings.TrimSpace(`Voce extrai dados de documentos brasileiros enviados por foto nitida para uma reserva de passagem.
Responda exclusivamente em JSON valido, sem markdown.
Priorize documentos nesta ordem quando houver mais de um numero: CPF, RG, CNH, CERTIDAO_NASCIMENTO.
CPF brasileiro tem 11 digitos e dois digitos verificadores; nao trate como CPF um numero apenas por ter 11 digitos.

Extraia apenas:
- nome completo
- tipo do documento
- numero do documento
- CPF, CNH e RG em campos separados quando estiverem visiveis no mesmo arquivo
- data de nascimento quando estiver visivel
- confianca da leitura

Quando CNH ou CNH-e contiver CPF visivel, use o CPF como documento principal:
- document_type deve ser "CPF"
- document deve ser o CPF sem pontuacao
- cpf deve repetir o CPF sem pontuacao
- cnh deve preservar o numero da CNH quando visivel

Para RG, document deve conter apenas o numero do RG.
Nao inclua orgao emissor/UF no numero do RG.
Nao junte SSP, SSP/SC, SSPSC, SDS, SESP, IFP ou PC ao numero do RG.
Exemplo: 2817314 SSP SC deve virar document "2817314", nunca "2817314SSPSC".

Em CNH/CNH-e, nao confunda numero de registro da CNH, numero lateral, espelho, QR Code, RENACH, MRZ, codigo de seguranca ou protocolo com CPF.
Em CNH-e, se CPF estiver visivel, CPF deve ser documento principal.
Se houver duvida entre CPF, RG e CNH, retorne mode "PARTIAL".
Se o CPF nao estiver visivel e a CNH estiver legivel, use document_type "CNH" e preserve o numero de CNH em cnh/document.

Quando houver frente e verso do documento, combine as informacoes com cuidado.
Nunca invente dados ausentes.

Formato:
{
  "mode": "EXTRACTED" | "PARTIAL" | "LOW_CONFIDENCE",
  "passengers": [
    {"name": "Nome completo", "document_type": "CPF|RG|CNH|CERTIDAO_NASCIMENTO", "document": "numero principal sem pontuacao desnecessaria", "cpf": "CPF quando visivel", "cnh": "CNH quando visivel", "rg": "RG quando visivel", "birth_date": "YYYY-MM-DD quando disponivel", "confidence": 0.0}
  ],
  "failure_reason": ""
}
Use LOW_CONFIDENCE apenas quando o arquivo estiver ilegivel ou sem nome/documento suficiente.`)
}

func buildDocumentExtractUserPrompt(expected int) string {
	if expected > 1 {
		return fmt.Sprintf("Extraia nome completo e documento da foto recebida. A conversa espera %d passageiros; retorne somente os passageiros que conseguir ler com seguranca.", expected)
	}
	return "Extraia o maximo de informacao legivel. Nao classifique como LOW_CONFIDENCE se pelo menos nome ou algum documento puder ser lido parcialmente. Use PARTIAL quando algum campo faltar ou estiver incerto. Retorne LOW_CONFIDENCE somente se nenhum dado util puder ser lido. Se houver CPF visivel, sempre priorize CPF mesmo que tambem exista RG/CNH. Se o nome estiver parcialmente visivel, retorne o trecho lido e marque confidence menor. Nunca invente numeros ausentes."
}

func buildDocumentExtractTextFormat() map[string]interface{} {
	return map[string]interface{}{
		"type":   "json_schema",
		"name":   "document_extract_result",
		"strict": true,
		"schema": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{
					"type": "string",
					"enum": []string{"EXTRACTED", "PARTIAL", "LOW_CONFIDENCE"},
				},
				"passengers": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"name": map[string]interface{}{
								"type": "string",
							},
							"document_type": map[string]interface{}{
								"type": "string",
								"enum": []string{"CPF", "RG", "CNH", "CERTIDAO_NASCIMENTO", "UNKNOWN"},
							},
							"document": map[string]interface{}{
								"type": "string",
							},
							"cpf": map[string]interface{}{
								"type": "string",
							},
							"cnh": map[string]interface{}{
								"type": "string",
							},
							"rg": map[string]interface{}{
								"type": "string",
							},
							"birth_date": map[string]interface{}{
								"type": "string",
							},
							"confidence": map[string]interface{}{
								"type":    "number",
								"minimum": 0,
								"maximum": 1,
							},
						},
						"required": []string{"name", "document_type", "document", "cpf", "cnh", "rg", "birth_date", "confidence"},
					},
				},
				"failure_reason": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"mode", "passengers", "failure_reason"},
		},
	}
}

func hasIncompletePassenger(passengers []DocumentExtractPassenger) bool {
	/* Validate the labels of extract data that comes from image */

	for _, passenger := range passengers {
		if strings.TrimSpace(passenger.Name) == "" || strings.TrimSpace(passenger.Document) == "" || strings.TrimSpace(passenger.DocumentType) == "" || strings.EqualFold(strings.TrimSpace(passenger.DocumentType), "UNKNOWN") {
			return true
		}
	}
	return false
}

func parseDocumentExtractResult(text string) DocumentExtractResult {
	result := DocumentExtractResult{Mode: "LOW_CONFIDENCE"}
	payload := extractJSONObject(text)
	if len(payload) == 0 {
		result.FailureReason = "json_parse_failed"
		return result
	}

	mode := strings.ToUpper(strings.TrimSpace(asString(payload["mode"])))
	if mode == "" {
		mode = strings.ToUpper(strings.TrimSpace(asString(payload["status"])))
	}
	if mode == "" {
		mode = "LOW_CONFIDENCE"
	}
	result.Mode = mode
	result.FailureReason = strings.TrimSpace(firstNonEmpty(asString(payload["failure_reason"]), asString(payload["reason"])))

	rawPassengers := asInterfaceSliceMaps(payload["passengers"])
	if len(rawPassengers) == 0 {
		if passenger := parseDocumentExtractPassenger(payload); passenger.Document != "" || passenger.Name != "" {
			rawPassengers = []map[string]interface{}{payload}
		}
	}
	needsReview := false
	for _, raw := range rawPassengers {
		passenger := parseDocumentExtractPassenger(raw)
		if passenger.Name == "" && passenger.Document == "" {
			continue
		}
		if passenger.Document != "" && passenger.DocumentType == "" {
			passenger.DocumentType = "UNKNOWN"
		}
		if documentExtractPassengerNeedsReview(raw, passenger) {
			needsReview = true
		}
		result.Passengers = append(result.Passengers, passenger)
	}
	if len(result.Passengers) > 0 {
		if needsReview || hasIncompletePassenger(result.Passengers) || hasUnreliablePassengerDocument(result.Passengers) {
			result.Mode = "PARTIAL"
		} else {
			result.Mode = "EXTRACTED"
		}
		result.FailureReason = ""
	} else if result.FailureReason == "" {
		result.FailureReason = "no_complete_document_found"
	}
	return result
}

func parseDocumentExtractPassenger(raw map[string]interface{}) DocumentExtractPassenger {
	name := normalizePassengerName(firstNonEmpty(
		asString(raw["name"]),
		asString(raw["full_name"]),
		asString(raw["nome"]),
		asString(raw["nome_completo"]),
	))

	documents := extractDocumentFields(raw)
	documentType, document := selectDocumentByPriority(raw, documents)
	if documentType != "" && document != "" && documents[documentType] == "" {
		documents[documentType] = document
	}
	if cpf := documents["CPF"]; isValidCPF(cpf) {
		documentType = "CPF"
		document = cpf
	}
	return normalizeDocumentExtractPassenger(DocumentExtractPassenger{
		Name:         name,
		Document:     document,
		DocumentType: documentType,
		CPF:          documents["CPF"],
		CNH:          documents["CNH"],
		RG:           documents["RG"],
		BirthDate:    normalizeDocumentBirthDate(raw),
		Confidence:   normalizeDocumentConfidence(asFloat64(raw["confidence"])),
	})
}

func normalizeDocumentExtractPassenger(passenger DocumentExtractPassenger) DocumentExtractPassenger {
	passenger.Name = normalizePassengerName(passenger.Name)
	documentType := normalizePassengerDocumentType(passenger.DocumentType)
	rawDocument := strings.TrimSpace(passenger.Document)
	passenger.DocumentType = documentType
	passenger.CPF = normalizePassengerDocumentValue(passenger.CPF, "CPF")
	passenger.CNH = normalizePassengerDocumentValue(passenger.CNH, "CNH")
	passenger.RG = normalizePassengerDocumentValue(passenger.RG, "RG")

	switch documentType {
	case "CPF":
		passenger.CPF = firstNonEmpty(passenger.CPF, normalizePassengerDocumentValue(rawDocument, "CPF"))
	case "CNH":
		passenger.CNH = firstNonEmpty(passenger.CNH, normalizePassengerDocumentValue(rawDocument, "CNH"))
	case "RG":
		passenger.RG = firstNonEmpty(passenger.RG, normalizePassengerDocumentValue(rawDocument, "RG"))
	case "CERTIDAO_NASCIMENTO":
		passenger.Document = normalizePassengerDocumentValue(rawDocument, "CERTIDAO_NASCIMENTO")
	default:
		passenger.CPF = firstNonEmpty(passenger.CPF, normalizePassengerDocumentValue(rawDocument, "CPF"))
		if passenger.CPF == "" {
			passenger.RG = firstNonEmpty(passenger.RG, normalizePassengerDocumentValue(rawDocument, "RG"))
		}
	}

	if passenger.CPF != "" {
		passenger.DocumentType = "CPF"
		passenger.Document = passenger.CPF
		return passenger
	}

	switch passenger.DocumentType {
	case "CNH":
		passenger.Document = firstNonEmpty(passenger.CNH, normalizePassengerDocumentValue(passenger.Document, "CNH"))
		passenger.CNH = firstNonEmpty(passenger.CNH, passenger.Document)
	case "RG":
		passenger.Document = firstNonEmpty(passenger.RG, normalizePassengerDocumentValue(passenger.Document, "RG"))
		passenger.RG = firstNonEmpty(passenger.RG, passenger.Document)
	case "CERTIDAO_NASCIMENTO":
		passenger.Document = normalizePassengerDocumentValue(passenger.Document, "CERTIDAO_NASCIMENTO")
	default:
		if looksLikeCNHEExtract(passenger) && passenger.CNH != "" {
			passenger.DocumentType = "CNH"
			passenger.Document = passenger.CNH
		} else if passenger.RG != "" {
			passenger.DocumentType = "RG"
			passenger.Document = passenger.RG
		} else if passenger.CNH != "" {
			passenger.DocumentType = "CNH"
			passenger.Document = passenger.CNH
		} else {
			passenger.Document = ""
		}
	}

	if looksLikeCNHEExtract(passenger) && passenger.CPF == "" && passenger.CNH != "" {
		passenger.DocumentType = "CNH"
		passenger.Document = passenger.CNH
	}
	return passenger
}

func looksLikeCNHEExtract(passenger DocumentExtractPassenger) bool {
	if strings.TrimSpace(passenger.CNH) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(passenger.DocumentType), "CNH") {
		return true
	}
	return strings.TrimSpace(passenger.CPF) != "" &&
		strings.TrimSpace(passenger.CNH) != "" &&
		strings.TrimSpace(passenger.RG) != ""
}

func hasUnreliablePassengerDocument(passengers []DocumentExtractPassenger) bool {
	for _, passenger := range passengers {
		if passenger.Confidence > 0 && passenger.Confidence < 0.75 {
			return true
		}
		documentType := normalizePassengerDocumentType(passenger.DocumentType)
		switch documentType {
		case "CPF":
			if !isValidCPF(passenger.Document) {
				return true
			}
		case "RG":
			if normalizePassengerDocumentValue(passenger.Document, "RG") == "" || rgDocumentContainsIssuer(passenger.Document) {
				return true
			}
			if looksLikeCNHEExtract(passenger) && passenger.CPF == "" && passenger.CNH == "" && passenger.Confidence < 0.9 {
				return true
			}
		case "CNH", "CERTIDAO_NASCIMENTO":
			if normalizePassengerDocumentValue(passenger.Document, documentType) == "" {
				return true
			}
		default:
			return true
		}
		if looksLikeCNHEExtract(passenger) && passenger.CPF != "" && (documentType != "CPF" || passenger.Document != passenger.CPF) {
			return true
		}
	}
	return false
}

func rgDocumentContainsIssuer(value string) bool {
	normalized := normalizeAlphaNumeric(value)
	for _, suffix := range rgIssuerSuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	for _, issuer := range rgIssuerTokens {
		if strings.Contains(normalized, issuer) {
			return true
		}
	}
	return false
}

func documentExtractPassengerNeedsReview(raw map[string]interface{}, passenger DocumentExtractPassenger) bool {
	if passenger.Confidence > 0 && passenger.Confidence < 0.75 {
		return true
	}
	if passenger.DocumentType == "CPF" && !isValidCPF(passenger.Document) {
		return true
	}
	if rawCPF := strings.TrimSpace(asString(raw["cpf"])); rawCPF != "" && normalizePassengerDocumentValue(rawCPF, "CPF") == "" {
		return true
	}
	explicitType := normalizePassengerDocumentType(firstNonEmpty(asString(raw["document_type"]), asString(raw["tipo_documento"]), asString(raw["type"])))
	if explicitType != "CPF" {
		return false
	}
	if passenger.CPF != "" {
		return false
	}
	explicitDocument := firstNonEmpty(asString(raw["document"]), asString(raw["numero"]), asString(raw["number"]), asString(raw["document_number"]))
	if explicitDocument == "" {
		return true
	}
	return normalizePassengerDocumentValue(explicitDocument, "CPF") == ""
}

func normalizeDocumentBirthDate(raw map[string]interface{}) string {
	value := strings.TrimSpace(firstNonEmpty(
		asString(raw["birth_date"]),
		asString(raw["date_of_birth"]),
		asString(raw["data_nascimento"]),
		asString(raw["nascimento"]),
	))
	if value == "" {
		return ""
	}
	if parsed, ok := parseFlexibleDate(value); ok {
		return parsed.Format("2006-01-02")
	}
	return value
}

func parseDocumentExtractContextPayload(payload map[string]interface{}) DocumentExtractResult {
	result := DocumentExtractResult{
		Mode:                   strings.ToUpper(strings.TrimSpace(asString(payload["mode"]))),
		ExpectedPassengerCount: readInt(payload["expected_passenger_count"]),
		MediaCount:             readInt(payload["media_count"]),
		FailureReason:          strings.TrimSpace(asString(payload["failure_reason"])),
		Model:                  strings.TrimSpace(asString(payload["model"])),
		ProviderResponseID:     strings.TrimSpace(asString(payload["provider_response_id"])),
	}
	for _, raw := range asInterfaceSliceMaps(payload["passengers"]) {
		passenger := parseDocumentExtractPassenger(raw)
		if passenger.Name == "" && passenger.Document == "" {
			continue
		}
		result.Passengers = append(result.Passengers, passenger)
	}
	return result
}

func extractDocumentFields(raw map[string]interface{}) map[string]string {
	documents := map[string]string{}
	explicitType := normalizePassengerDocumentType(firstNonEmpty(asString(raw["document_type"]), asString(raw["tipo_documento"]), asString(raw["type"])))
	explicitDocument := firstNonEmpty(asString(raw["document"]), asString(raw["numero"]), asString(raw["number"]), asString(raw["document_number"]))
	if explicitType != "" {
		if document := normalizePassengerDocumentValue(explicitDocument, explicitType); document != "" {
			documents[explicitType] = document
		}
	}
	for _, candidate := range []struct {
		Type string
		Keys []string
	}{
		{"CPF", []string{"cpf"}},
		{"RG", []string{"rg"}},
		{"CNH", []string{"cnh"}},
		{"CERTIDAO_NASCIMENTO", []string{"certidao", "certidao_nascimento", "birth_certificate", "matricula"}},
	} {
		for _, key := range candidate.Keys {
			if document := normalizePassengerDocumentValue(asString(raw[key]), candidate.Type); document != "" {
				documents[candidate.Type] = document
				break
			}
		}
	}
	return documents
}

func selectDocumentByPriority(raw map[string]interface{}, documents map[string]string) (string, string) {
	candidates := []struct {
		Type string
	}{
		{"CPF"},
		{"RG"},
		{"CNH"},
		{"CERTIDAO_NASCIMENTO"},
	}
	explicitType := normalizePassengerDocumentType(firstNonEmpty(asString(raw["document_type"]), asString(raw["tipo_documento"]), asString(raw["type"])))
	explicitDocument := firstNonEmpty(asString(raw["document"]), asString(raw["numero"]), asString(raw["number"]), asString(raw["document_number"]))
	for _, candidate := range candidates {
		if document := documents[candidate.Type]; document != "" {
			return candidate.Type, document
		}
		if explicitType == "" {
			if document := normalizePassengerDocumentValue(explicitDocument, candidate.Type); document != "" {
				return candidate.Type, document
			}
		}
	}
	if explicitType != "" {
		if document := normalizePassengerDocumentValue(explicitDocument, explicitType); document != "" {
			return explicitType, document
		}
	}
	if explicitDocument != "" && explicitType == "" {
		for _, candidate := range candidates {
			if document := normalizePassengerDocumentValue(explicitDocument, candidate.Type); document != "" {
				return candidate.Type, document
			}
		}
	}
	return "", ""
}

func normalizeDocumentConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		if value <= 100 {
			return value / 100
		}
		return 1
	}
	return value
}

func extractJSONObject(text string) map[string]interface{} {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end >= start {
			trimmed = trimmed[start : end+1]
		}
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil
	}
	return payload
}

func buildDocumentExtractDraftRun(result DocumentExtractResult) RunAgentResult {
	return RunAgentResult{
		ReplyText:          buildDocumentExtractReply(result),
		Model:              firstNonEmpty(strings.TrimSpace(result.Model), "document_extract"),
		ProviderResponseID: strings.TrimSpace(result.ProviderResponseID),
		RequestPayload: map[string]interface{}{
			"mode":    "DOCUMENT_EXTRACT_DETERMINISTIC_REPLY",
			"extract": buildDocumentExtractResponsePayload(result),
		},
		ResponsePayload: map[string]interface{}{
			"reply_text": buildDocumentExtractReply(result),
		},
	}
}

func buildDocumentExtractReply(result DocumentExtractResult) string {
	return buildConfirmExtractedDocumentReply(result)
}

func buildDocumentExtractResponsePayload(result DocumentExtractResult) map[string]interface{} {
	passengers := make([]map[string]interface{}, 0, len(result.Passengers))
	for _, passenger := range result.Passengers {
		passengers = append(passengers, map[string]interface{}{
			"name":          passenger.Name,
			"document":      passenger.Document,
			"document_type": passenger.DocumentType,
			"cpf":           passenger.CPF,
			"cnh":           passenger.CNH,
			"rg":            passenger.RG,
			"birth_date":    passenger.BirthDate,
			"confidence":    passenger.Confidence,
		})
	}
	payload := map[string]interface{}{
		"mode":                     result.Mode,
		"passenger_count":          len(result.Passengers),
		"expected_passenger_count": result.ExpectedPassengerCount,
		"media_count":              result.MediaCount,
		"passengers":               passengers,
	}
	if result.FailureReason != "" {
		payload["failure_reason"] = result.FailureReason
	}
	if result.ProviderResponseID != "" {
		payload["provider_response_id"] = result.ProviderResponseID
	}
	if result.Model != "" {
		payload["model"] = result.Model
	}
	return payload
}

func mergeAgentToolContexts(base agentToolContext, extra agentToolContext) agentToolContext {
	base.Calls = append(base.Calls, extra.Calls...)
	if extra.Availability != nil {
		base.Availability = extra.Availability
	}
	if extra.Pricing != nil {
		base.Pricing = extra.Pricing
	}
	if extra.Booking != nil {
		base.Booking = extra.Booking
	}
	if extra.BookingCreate != nil {
		base.BookingCreate = extra.BookingCreate
	}
	if extra.BookingCancel != nil {
		base.BookingCancel = extra.BookingCancel
	}
	if extra.Reschedule != nil {
		base.Reschedule = extra.Reschedule
	}
	if extra.Payments != nil {
		base.Payments = extra.Payments
	}
	if extra.PaymentCreate != nil {
		base.PaymentCreate = extra.PaymentCreate
	}
	if extra.DocumentExtract != nil {
		base.DocumentExtract = extra.DocumentExtract
	}
	return base
}

func latestCandidateMessageID(candidates []Message) string {
	if len(candidates) == 0 {
		return ""
	}
	return strings.TrimSpace(candidates[len(candidates)-1].ID)
}
