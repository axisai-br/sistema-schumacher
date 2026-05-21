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

	run, result, runErr := s.runDocumentExtract(ctx, session, candidates, media, expected, draftID)
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
	return strings.TrimSpace(`Voce extrai dados de documentos brasileiros enviados por foto para uma reserva de passagem.
Responda exclusivamente em JSON valido, sem markdown.
Priorize documentos nesta ordem quando houver mais de um numero: CPF, RG, CNH, CERTIDAO_NASCIMENTO.
		Estrutura de cada documento:
		- CPF: "xxx.xxx.xxx-xx" 
		- RG: "x.xxx.xxx"
		- Matricula certidao_nascimento: "xxxxxx xx xx xxxx x xxxxx xxx xxxxxxx-xx"
		- CNH: "xxxxxxxxxxx"
		Atualmente existem 2 versoes de Identidade que podem ser enviadas:
		- A mais recente possui um campo do nome completo da pessoa e o CPF ja na parte da frente do documento. Alem disso esse novo formato nao contem mais o numero do RG
		- A antiga as informacoes ficam atras: com o nome completo da pessoa + CPF + RG, nesse caso a prioridade de leitura para extracao continua sendo nome + CPF.
		A intencao eh apenas extrair essas informacoes: Nome completo + numero do documento + data de nascimento quando estiver visivel
Formato:
{
  "mode": "EXTRACTED" | "PARTIAL" | "LOW_CONFIDENCE",
  "passengers": [
    {"name": "Nome completo", "document_type": "CPF|RG|CNH|CERTIDAO_NASCIMENTO", "document": "numero sem pontuacao desnecessaria", "birth_date": "YYYY-MM-DD quando disponivel", "confidence": 0.0}
  ],
  "failure_reason": ""
}
Use LOW_CONFIDENCE apenas quando a imagem estiver ilegivel ou sem nome/documento suficiente.`)
}

func buildDocumentExtractUserPrompt(expected int) string {
	if expected > 1 {
		return fmt.Sprintf("Extraia nome completo e documento da foto recebida. A conversa espera %d passageiros; retorne somente os passageiros que conseguir ler com seguranca.", expected)
	}
	return "Extraia o maximo de informacao legivel. Nao classifique como LOW_CONFIDENCE se pelo menos nome ou algum documento puder ser lido parcialmente. Use PARTIAL quando algum campo faltar ou estiver incerto. Retorne LOW_CONFIDENCE somente se nenhum dado util puder ser lido. Se houver CPF visivel, sempre priorize CPF mesmo que tambem exista RG/CNH. Se o nome estiver parcialmente visivel, retorne o trecho lido e marque confidence menor. Nunca invente numeros ausentes."
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
	for _, raw := range rawPassengers {
		passenger := parseDocumentExtractPassenger(raw)
		if passenger.Name == "" && passenger.Document == "" {
			continue
		}
		if passenger.Document != "" && passenger.DocumentType == "" {
			passenger.DocumentType = "UNKNOWN"
		}
		result.Passengers = append(result.Passengers, passenger)
	}
	if len(result.Passengers) > 0 {
		if hasIncompletePassenger(result.Passengers) {
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

	documentType, document := selectDocumentByPriority(raw)
	return DocumentExtractPassenger{
		Name:         name,
		Document:     document,
		DocumentType: documentType,
		BirthDate:    normalizeDocumentBirthDate(raw),
		Confidence:   normalizeDocumentConfidence(asFloat64(raw["confidence"])),
	}
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

func selectDocumentByPriority(raw map[string]interface{}) (string, string) {
	candidates := []struct {
		Type string
		Keys []string
	}{
		{"CPF", []string{"cpf"}},
		{"RG", []string{"rg"}},
		{"CNH", []string{"cnh"}},
		{"CERTIDAO_NASCIMENTO", []string{"certidao", "certidao_nascimento", "birth_certificate", "matricula"}},
	}
	explicitType := normalizePassengerDocumentType(firstNonEmpty(asString(raw["document_type"]), asString(raw["tipo_documento"]), asString(raw["type"])))
	explicitDocument := firstNonEmpty(asString(raw["document"]), asString(raw["numero"]), asString(raw["number"]), asString(raw["document_number"]))
	for _, candidate := range candidates {
		for _, key := range candidate.Keys {
			if document := normalizePassengerDocumentValue(asString(raw[key]), candidate.Type); document != "" {
				return candidate.Type, document
			}
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
	if explicitDocument != "" {
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
