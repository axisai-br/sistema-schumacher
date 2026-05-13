package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

var (
	optionIndexPattern           = regexp.MustCompile(`(?i)\bop[cç][aã]o\s*0?([1-5])\b`)
	passengerNamePattern         = regexp.MustCompile(`(?i)\bnome(?:\s+completo)?\s*(?:é|e|:|-)?\s*([A-ZÀ-ÿ][A-Za-zÀ-ÿ' ]{3,100}?)(?:\s+(?:cpf|rg|cnh|certid[aã]o|matr[ií]cula)\b|$)`)
	passengerAltNamePattern      = regexp.MustCompile(`(?i)\b(?:meu nome|sou)\s*(?:é|e)?\s*([A-ZÀ-ÿ][A-Za-zÀ-ÿ' ]{3,100}?)(?:\s+(?:cpf|rg|cnh|certid[aã]o|matr[ií]cula)\b|$)`)
	passengerCPFPattern          = regexp.MustCompile(`(?i)\bcpf\b[^0-9]*([0-9.\-]{11,14})`)
	passengerRGPattern           = regexp.MustCompile(`(?i)\brg\b[^A-Z0-9]*([A-Z0-9.\-]{4,20})`)
	passengerCNHPattern          = regexp.MustCompile(`(?i)\bcnh\b[^A-Z0-9]*([A-Z0-9.\-]{4,20})`)
	passengerBirthRecordPattern  = regexp.MustCompile(`(?i)\b(?:certid[aã]o(?: de nascimento)?|matr[ií]cula)\b[^A-Z0-9]*([A-Z0-9.\-]{8,40})`)
	passengerLooseCPFLinePattern = regexp.MustCompile(`(?i)^\s*([A-Za-zÀ-ÿ][A-Za-zÀ-ÿ' ]{3,100}?)\s+([0-9.\-]{11,14})\s*$`)
	passengerWordQtyPattern      = regexp.MustCompile(`\b(um|uma|dois|duas|tres|quatro|cinco)\s+(?:pessoas?|passageiros?|passagens?|assentos?|lugares?)\b`)
	passengerDigitQtyPattern     = regexp.MustCompile(`\b([1-9])\s+(?:pessoas?|passageiros?|passagens?|assentos?|lugares?)\b`)
	childUnder5AgePattern        = regexp.MustCompile(`\b(?:meu|minha|filho|filha|crianca|menino|menina|bebe)\s+(?:filho|filha|crianca|menino|menina|bebe)?\s*(?:tem|de)\s+([0-5])(?:\s+anos?)?\b`)
)

type PassengerClarificationSlots struct {
	PassengerCount        int
	PassengerCountKnown   bool
	ChildUnder5Count      int
	ChildUnder5CountKnown bool
}

func parseBookingCreateInput(session Session, history []Message, text string, currentAvailability *AvailabilitySearchResult) (BookingCreateInput, bool) {
	body := strings.TrimSpace(text)
	if body == "" {
		return BookingCreateInput{}, false
	}
	if shouldBlockBookingCreateBecausePaymentFlow(history, body) {
		return BookingCreateInput{}, false
	}
	confirmationOnly := looksLikeBookingCreateConfirmation(body)
	if !looksLikeCreateBookingIntent(body) && !confirmationOnly {
		return BookingCreateInput{}, false
	}

	selectedOptionIndex, selected, ok := resolveBookingCreateSelection(body, history, currentAvailability)
	if !ok {
		return BookingCreateInput{}, false
	}

	passengerSource := body
	passengers := extractBookingCreatePassengers(passengerSource, session)
	if len(passengers) == 0 && confirmationOnly {
		passengerSource = findLatestPassengerDetailsText(history, session)
		passengers = extractBookingCreatePassengers(passengerSource, session)
	}
	if len(passengers) == 0 {
		return BookingCreateInput{}, false
	}

	qty := inferExpectedPassengerCount(history, body, passengerSource)
	if qty <= 0 {
		qty = len(passengers)
	}
	if qty != len(passengers) {
		return BookingCreateInput{}, false
	}
	applyLapChildFlags(passengers, inferLapChildCount(history))

	input := BookingCreateInput{
		SelectedOptionIndex:    selectedOptionIndex,
		TripID:                 strings.TrimSpace(selected.TripID),
		BoardStopID:            strings.TrimSpace(selected.BoardStopID),
		AlightStopID:           strings.TrimSpace(selected.AlightStopID),
		OriginDisplayName:      strings.TrimSpace(selected.OriginDisplayName),
		DestinationDisplayName: strings.TrimSpace(selected.DestinationDisplayName),
		TripDate:               strings.TrimSpace(selected.TripDate),
		DepartureTime:          strings.TrimSpace(selected.OriginDepartTime),
		Qty:                    qty,
		CustomerName:           firstNonEmpty(strings.TrimSpace(session.CustomerName), strings.TrimSpace(passengers[0].Name)),
		CustomerPhone:          strings.TrimSpace(session.CustomerPhone),
		Passengers:             passengers,
	}
	input.IdempotencyKey = buildBookingCreateIdempotencyKey(session, input)
	return input, true
}

func parseBookingCreateFromDocumentConfirmation(session Session, history []Message, currentTurn string) (BookingCreateInput, bool) {
	if (!lastAssistantAskedDocumentConfirmation(history) && !lastAssistantAskedBookingProceedConfirmation(history)) ||
		!looksLikeDocumentConfirmation(currentTurn) {
		return BookingCreateInput{}, false
	}

	context := collectBookingDraftContext(session, history, currentTurn)
	if (!context.PassengerCountKnown || context.PassengerCount <= 0) && context.PassengerDetailsCount > 0 {
		context.PassengerCount = context.PassengerDetailsCount
		context.PassengerCountKnown = true
	}

	if !context.PassengerCountKnown || context.PassengerCount <= 0 {
		return BookingCreateInput{}, false
	}

	if strings.TrimSpace(context.TripID) == "" ||
		strings.TrimSpace(context.BoardStopID) == "" ||
		strings.TrimSpace(context.AlightStopID) == "" ||
		strings.TrimSpace(context.Origin) == "" ||
		strings.TrimSpace(context.Destination) == "" ||
		strings.TrimSpace(context.TripDate) == "" {
		return BookingCreateInput{}, false
	}

	expected := context.PassengerCount
	var passengers []BookingCreatePassengerInput

	if extract := findLatestDocumentExtractContext(history); extract != nil {
		if strings.ToUpper(strings.TrimSpace(extract.Mode)) != "EXTRACTED" {
			return BookingCreateInput{}, false
		}
		if expected <= 0 {
			expected = extract.ExpectedPassengerCount
		}
		if expected <= 0 {
			expected = len(extract.Passengers)
		}
		if expected <= 0 || len(extract.Passengers) != expected {
			return BookingCreateInput{}, false
		}

		passengers = make([]BookingCreatePassengerInput, 0, len(extract.Passengers))
		for _, extracted := range extract.Passengers {
			passenger := bookingPassengerFromDocumentExtract(extracted, session)
			if strings.TrimSpace(passenger.Name) == "" ||
				strings.TrimSpace(passenger.DocumentType) == "" ||
				strings.TrimSpace(passenger.Document) == "" {
				return BookingCreateInput{}, false
			}
			passengers = append(passengers, passenger)
		}
	} else {
		passengerSource := context.PassengerDetailsText
		if passengerSource == "" {
			passengerSource = findLatestPassengerDetailsText(history, session)
		}
		passengers = extractBookingCreatePassengers(passengerSource, session)
		if len(passengers) == 0 {
			return BookingCreateInput{}, false
		}
		if expected <= 0 {
			expected = inferExpectedPassengerCount(history, currentTurn, passengerSource)
		}
		if expected <= 0 {
			expected = len(passengers)
		}
	}

	if expected <= 0 || len(passengers) != expected {
		return BookingCreateInput{}, false
	}
	applyLapChildFlags(passengers, context.ChildUnder5Count)

	input := BookingCreateInput{
		SelectedOptionIndex:    context.SelectedOptionIndex,
		TripID:                 strings.TrimSpace(context.TripID),
		BoardStopID:            strings.TrimSpace(context.BoardStopID),
		AlightStopID:           strings.TrimSpace(context.AlightStopID),
		OriginDisplayName:      strings.TrimSpace(context.Origin),
		DestinationDisplayName: strings.TrimSpace(context.Destination),
		TripDate:               strings.TrimSpace(context.TripDate),
		DepartureTime:          strings.TrimSpace(context.DepartureTime),
		Qty:                    expected,
		CustomerName:           firstNonEmpty(strings.TrimSpace(session.CustomerName), strings.TrimSpace(passengers[0].Name)),
		CustomerPhone:          strings.TrimSpace(session.CustomerPhone),
		Passengers:             passengers,
	}
	input.IdempotencyKey = buildBookingCreateIdempotencyKey(session, input)
	return input, true
}

func bookingPassengerFromDocumentExtract(extracted DocumentExtractPassenger, session Session) BookingCreatePassengerInput {
	documentType := normalizePassengerDocumentType(extracted.DocumentType)
	document := normalizePassengerDocumentValue(extracted.Document, documentType)
	return BookingCreatePassengerInput{
		Name:         strings.TrimSpace(extracted.Name),
		DocumentType: documentType,
		Document:     document,
		Phone:        strings.TrimSpace(session.CustomerPhone),
	}
}

func looksLikeDocumentConfirmation(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	switch folded {
	case "conferem",
		"confere",
		"sim",
		"sim sim",
		"isso",
		"isso mesmo",
		"correto",
		"certo",
		"sim esta correto",
		"esta correto",
		"sim correto",
		"esta certo",
		"ta certo",
		"tá certo",
		"ta tudo certo",
		"tá tudo certo",
		"tudo certo",
		"sim ta tudo certo",
		"sim tá tudo certo",
		"sim sim ta tudo certo",
		"sim sim tá tudo certo",
		"sim sim ta tudo certo tudo certo",
		"sim sim tá tudo certo tudo certo",
		"pode seguir",
		"pode prosseguir",
		"pode criar",
		"pode fazer a reserva",
		"confirmado",
		"confirmo":
		return true
	default:
		return strings.Contains(folded, "tudo certo") ||
			strings.Contains(folded, "pode criar") ||
			strings.Contains(folded, "pode seguir") ||
			strings.Contains(folded, "pode prosseguir")
	}
}

func lastAssistantAskedDocumentConfirmation(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		body := strings.Join(strings.Fields(foldChatText(message.Body)), " ")
		if body == "" {
			continue
		}
		return strings.Contains(body, "consegui identificar estes dados") ||
			strings.Contains(body, "eles conferem") ||
			strings.Contains(body, "dados conferem") ||
			strings.Contains(body, "confere")
	}
	return false
}

func lastAssistantAskedBookingProceedConfirmation(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		body := strings.Join(strings.Fields(foldChatText(message.Body)), " ")
		if body == "" {
			continue
		}
		return strings.Contains(body, "posso prosseguir") ||
			strings.Contains(body, "posso seguir") ||
			strings.Contains(body, "seguir com a reserva") ||
			strings.Contains(body, "criar a reserva") ||
			strings.Contains(body, "fazer a reserva") ||
			strings.Contains(body, "prosseguir e criar")
	}
	return false
}

func findLatestDocumentExtractContext(history []Message) *DocumentExtractResult {
	for i := len(history) - 1; i >= 0; i-- {
		for _, toolContext := range messageToolContexts(history[i]) {
			payload := asMap(toolContext[toolNameDocumentExtract])
			if len(payload) == 0 {
				continue
			}
			result := parseDocumentExtractContextPayload(payload)
			if strings.TrimSpace(result.Mode) != "" {
				return &result
			}
		}
	}
	return nil
}

func shouldBlockBookingCreateBecausePaymentFlow(history []Message, currentTurn string) bool {
	folded := strings.Join(strings.Fields(foldChatText(currentTurn)), " ")
	if folded == "" || !looksLikePaymentFlowShortReply(folded) {
		return false
	}
	if !historyMentionsCreatedBooking(history) {
		return false
	}
	if historyMentionsPaymentChoice(history) || historyMentionsPixConfirmation(history) {
		return true
	}

	switch folded {
	case "pix", "integral", "sinal", "entrada", "deposito":
		return true
	default:
		return false
	}
}

func historyMentionsCreatedBooking(history []Message) bool {
	previous := findLatestBookingCreateContext(history)
	return previous != nil && strings.TrimSpace(previous.BookingID) != ""
}

func historyMentionsPaymentChoice(history []Message) bool {
	start := len(history) - 20
	if start < 0 {
		start = 0
	}

	for i := len(history) - 1; i >= start; i-- {
		message := history[i]
		body := strings.Join(strings.Fields(foldChatText(message.Body)), " ")
		if body == "" {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
			if detectRequestedPaymentType(body) != "" {
				return true
			}
			if body == "pix" || body == "via pix" || body == "pelo pix" {
				return true
			}
		}

		if strings.Contains(body, "valor integral") ||
			strings.Contains(body, "pagar o valor integral") ||
			strings.Contains(body, "apenas o sinal") ||
			strings.Contains(body, "sinal de r") ||
			strings.Contains(body, "integral ou sinal") ||
			strings.Contains(body, "prefere pagar") {
			return true
		}
	}

	return false
}

func historyMentionsPixConfirmation(history []Message) bool {
	start := len(history) - 20
	if start < 0 {
		start = 0
	}

	for i := len(history) - 1; i >= start; i-- {
		body := strings.Join(strings.Fields(foldChatText(history[i].Body)), " ")
		if body == "" {
			continue
		}

		if strings.Contains(body, "chave pix") ||
			strings.Contains(body, "codigo pix") ||
			strings.Contains(body, "codigo do pix") ||
			strings.Contains(body, "copia e cola") ||
			strings.Contains(body, "pix copia") {
			return true
		}

		if strings.Contains(body, "pix") && (strings.Contains(body, "quer que eu envie") ||
			strings.Contains(body, "posso enviar") ||
			strings.Contains(body, "vou gerar") ||
			strings.Contains(body, "gerar") ||
			strings.Contains(body, "enviar") ||
			strings.Contains(body, "manda")) {
			return true
		}
	}

	return false
}

func looksLikePaymentFlowShortReply(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	switch folded {
	case "sim",
		"ok",
		"okay",
		"certo",
		"isso",
		"isso mesmo",
		"pode",
		"pode sim",
		"pode mandar",
		"pode enviar",
		"manda",
		"mande",
		"envia",
		"envie",
		"pix",
		"via pix",
		"pelo pix",
		"integral",
		"sinal",
		"entrada",
		"deposito":
		return true
	}

	words := strings.Fields(folded)
	if len(words) <= 4 {
		if strings.Contains(folded, "pix") {
			return true
		}
		if detectRequestedPaymentType(folded) != "" {
			return true
		}
		if strings.Contains(folded, "pode") &&
			(strings.Contains(folded, "mandar") || strings.Contains(folded, "enviar") || len(words) <= 2) {
			return true
		}
	}

	return false
}

func looksLikeBookingCreateConfirmation(text string) bool {
	folded := strings.TrimSpace(foldChatText(text))
	switch folded {
	case "sim", "isso", "isso mesmo", "pode seguir", "pode reservar", "confirmo", "confirmado", "ok", "certo",
		"sim esta correto", "esta correto", "sim correto", "pode prosseguir", "pode criar", "pode fazer a reserva":
		return true
	default:
		return false
	}
}

func looksLikeCreateBookingIntent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	keywords := []string{
		"quero reservar",
		"quero fazer a reserva",
		"pode reservar",
		"faz a reserva",
		"fazer a reserva",
		"seguir com a reserva",
		"vou querer",
		"quero a opcao",
		"quero a opção",
		"fechar essa",
		"fechar a passagem",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	if extractSelectedOptionIndex(text) > 0 {
		actionWords := []string{"quero", "vou", "pode", "faz", "fechar", "reserv"}
		for _, keyword := range actionWords {
			if strings.Contains(lower, keyword) {
				return true
			}
		}
	}
	return false
}

func resolveBookingCreateSelection(text string, history []Message, currentAvailability *AvailabilitySearchResult) (int, AvailabilitySearchItem, bool) {
	options := []AvailabilitySearchItem{}
	if currentAvailability != nil && len(currentAvailability.Results) > 0 {
		options = append(options, currentAvailability.Results...)
	} else if previous := findLatestAvailabilityContext(history); previous != nil {
		options = append(options, previous.Results...)
	}
	if len(options) == 0 {
		return 0, AvailabilitySearchItem{}, false
	}

	index := extractSelectedOptionIndex(text)
	if index <= 0 {
		index = findLatestSelectedOptionIndex(history)
	}
	if index > 0 {
		if index > len(options) {
			return 0, AvailabilitySearchItem{}, false
		}
		return index, options[index-1], true
	}
	if len(options) == 1 {
		return 1, options[0], true
	}
	return 0, AvailabilitySearchItem{}, false
}

func extractSelectedOptionIndex(text string) int {
	lower := strings.Join(strings.Fields(foldChatText(text)), " ")
	if match := optionIndexPattern.FindStringSubmatch(lower); len(match) == 2 {
		return asInt(float64(match[1][0] - '0'))
	}
	if len(lower) == 1 && lower[0] >= '1' && lower[0] <= '5' {
		return int(lower[0] - '0')
	}
	if len(lower) == 2 && lower[0] == '0' && lower[1] >= '1' && lower[1] <= '5' {
		return int(lower[1] - '0')
	}
	switch {
	case lower == "primeira", lower == "primeiro", strings.Contains(lower, "primeira opcao"), strings.Contains(lower, "primeiro opcao"), strings.Contains(lower, "a primeira"), strings.Contains(lower, "o primeiro"):
		return 1
	case lower == "segunda", lower == "segundo", strings.Contains(lower, "segunda opcao"), strings.Contains(lower, "segundo opcao"), strings.Contains(lower, "a segunda"), strings.Contains(lower, "o segundo"):
		return 2
	case lower == "terceira", lower == "terceiro", strings.Contains(lower, "terceira opcao"), strings.Contains(lower, "terceiro opcao"), strings.Contains(lower, "a terceira"), strings.Contains(lower, "o terceiro"):
		return 3
	case lower == "quarta", lower == "quarto", strings.Contains(lower, "quarta opcao"), strings.Contains(lower, "quarto opcao"), strings.Contains(lower, "a quarta"), strings.Contains(lower, "o quarto"):
		return 4
	case lower == "quinta", lower == "quinto", strings.Contains(lower, "quinta opcao"), strings.Contains(lower, "quinto opcao"), strings.Contains(lower, "a quinta"), strings.Contains(lower, "o quinto"):
		return 5
	}
	return 0
}

func extractBookingCreatePassengers(text string, session Session) []BookingCreatePassengerInput {
	if passengers := extractBookingCreatePassengersByLines(text, session); len(passengers) > 0 {
		return passengers
	}
	if match := passengerLooseCPFLinePattern.FindStringSubmatch(text); len(match) == 3 {
		name := normalizePassengerName(match[1])
		document := normalizeDigits(match[2])
		if name != "" && len(document) == 11 {
			return []BookingCreatePassengerInput{
				{
					Name:         name,
					Document:     document,
					DocumentType: "CPF",
					Phone:        strings.TrimSpace(session.CustomerPhone),
				},
			}
		}
	}
	document, documentType := extractBookingPassengerDocument(text)
	if document == "" || documentType == "" {
		return nil
	}
	name := extractBookingPassengerName(text, session.CustomerName)
	if name == "" {
		return nil
	}
	return []BookingCreatePassengerInput{
		{
			Name:         name,
			Document:     document,
			DocumentType: documentType,
			Phone:        strings.TrimSpace(session.CustomerPhone),
		},
	}
}

func extractBookingCreatePassengersByLines(text string, session Session) []BookingCreatePassengerInput {
	segments := splitPassengerSegments(text)
	if len(segments) == 0 {
		return nil
	}

	passengers := make([]BookingCreatePassengerInput, 0, len(segments))
	for _, segment := range segments {
		if match := passengerLooseCPFLinePattern.FindStringSubmatch(segment); len(match) == 3 {
			name := normalizePassengerName(match[1])
			document := normalizeDigits(match[2])
			if name == "" || len(document) != 11 {
				return nil
			}
			passengers = append(passengers, BookingCreatePassengerInput{
				Name:         name,
				Document:     document,
				DocumentType: "CPF",
				Phone:        strings.TrimSpace(session.CustomerPhone),
			})
			continue
		}

		if passenger, ok := parseStructuredPassengerLine(segment, session); ok {
			passengers = append(passengers, passenger)
			continue
		}
	}
	if len(passengers) == 0 {
		return nil
	}
	return passengers
}

func parseStructuredPassengerLine(segment string, session Session) (BookingCreatePassengerInput, bool) {
	if !strings.Contains(segment, "|") {
		return BookingCreatePassengerInput{}, false
	}

	parts := strings.Split(segment, "|")
	if len(parts) != 3 {
		return BookingCreatePassengerInput{}, false
	}

	namePart := strings.TrimSpace(parts[0])
	namePart = strings.TrimLeft(namePart, "-* ")
	if folded := strings.TrimSpace(foldChatText(namePart)); strings.HasPrefix(folded, "passageiro") {
		if idx := strings.Index(namePart, ":"); idx >= 0 {
			namePart = strings.TrimSpace(namePart[idx+1:])
		}
	}

	name := normalizePassengerName(namePart)
	documentType := normalizePassengerDocumentType(parts[1])
	document := normalizePassengerDocumentValue(parts[2], documentType)
	if name == "" || documentType == "" || document == "" {
		return BookingCreatePassengerInput{}, false
	}

	return BookingCreatePassengerInput{
		Name:         name,
		Document:     document,
		DocumentType: documentType,
		Phone:        strings.TrimSpace(session.CustomerPhone),
	}, true
}

func splitPassengerSegments(text string) []string {
	candidates := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == ';'
	})
	segments := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		segments = append(segments, candidate)
	}
	if len(segments) > 1 {
		return segments
	}
	return nil
}

func extractBookingPassengerName(text string, fallback string) string {
	if match := passengerNamePattern.FindStringSubmatch(text); len(match) == 2 {
		return normalizePassengerName(match[1])
	}
	if match := passengerAltNamePattern.FindStringSubmatch(text); len(match) == 2 {
		return normalizePassengerName(match[1])
	}
	return normalizePassengerName(fallback)
}

func normalizePassengerName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	value = strings.Trim(value, " ,.;:-")
	if len(strings.Fields(value)) < 2 {
		return ""
	}
	return value
}

func extractBookingPassengerDocument(text string) (string, string) {
	if match := passengerCPFPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		document := normalizeDigits(match[1])
		if len(document) == 11 {
			return document, "CPF"
		}
	}
	if match := passengerRGPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		document := normalizeAlphaNumeric(match[1])
		if document != "" {
			return document, "RG"
		}
	}
	if match := passengerBirthRecordPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		document := normalizeAlphaNumeric(match[1])
		if document != "" {
			return document, "CERTIDAO_NASCIMENTO"
		}
	}
	if match := passengerCNHPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		document := normalizeAlphaNumeric(match[1])
		if document != "" {
			return document, "CNH"
		}
	}
	return "", ""
}

func inferExpectedPassengerCount(history []Message, texts ...string) int {
	for _, text := range texts {
		if qty := inferPassengerQuantityFromFreeText(text); qty > 0 {
			return qty
		}
	}
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Direction != "INBOUND" {
			continue
		}
		if qty := inferPassengerQuantityFromFreeText(history[i].Body); qty > 0 {
			return qty
		}
	}
	for i := len(history) - 1; i >= 0; i-- {
		if qty := inferPassengerQuantityFromFreeText(history[i].Body); qty > 0 {
			return qty
		}
	}
	return 0
}

func inferPassengerQuantityFromFreeText(text string) int {
	if passengerCount, _, ok := parsePassengerCountReply(text); ok && passengerCount > 0 {
		return passengerCount
	}
	if qty := extractPassengerQuantity(text); qty > 0 {
		return qty
	}

	folded := foldChatText(text)
	if folded == "" {
		return 0
	}

	for _, pattern := range []string{
		"so eu",
		"so pra mim",
		"so para mim",
		"apenas eu",
		"apenas pra mim",
		"apenas para mim",
		"somente eu",
		"somente pra mim",
		"somente para mim",
		"sou eu",
	} {
		if strings.Contains(folded, pattern) {
			return 1
		}
	}

	for _, pattern := range []string{
		"eu e minha",
		"eu e meu",
		"pra mim e minha",
		"pra mim e meu",
		"para mim e minha",
		"para mim e meu",
		"eu e mais uma pessoa",
		"eu e mais um passageiro",
		"eu e mais um acompanhante",
		"os dois",
		"as duas",
		"dos dois",
		"das duas",
		"nos dois",
		"nos duas",
	} {
		if strings.Contains(folded, pattern) {
			return 2
		}
	}

	if match := passengerWordQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
		switch match[1] {
		case "um", "uma":
			return 1
		case "dois", "duas":
			return 2
		case "tres":
			return 3
		case "quatro":
			return 4
		case "cinco":
			return 5
		}
	}

	return 0
}

func lastBotAskedPassengerCount(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		body := strings.TrimSpace(message.Body)
		if body == "" {
			continue
		}
		return looksLikePassengerCountQuestion(body)
	}
	return false
}

func looksLikePassengerCountQuestion(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}

	patterns := []string{
		"a passagem e so para voce",
		"a passagem e so para voce ou tem mais alguem",
		"e so para voce",
		"tem mais alguem",
		"quantas pessoas",
		"quantos passageiros",
		"passageiros",
		"crianca de ate 5 anos",
		"crianca de 5 anos ou menos",
		"ha crianca",
		"tem crianca",
	}
	for _, pattern := range patterns {
		if strings.Contains(folded, pattern) {
			return true
		}
	}
	return false
}

func parsePassengerCountReply(currentTurn string) (int, int, bool) {
	slots := parsePassengerClarificationSlots(currentTurn)
	if slots.PassengerCountKnown || slots.ChildUnder5CountKnown {
		return slots.PassengerCount, slots.ChildUnder5Count, true
	}
	return 0, 0, false
}

func parsePassengerClarificationSlots(currentTurn string) PassengerClarificationSlots {
	folded := strings.Join(strings.Fields(foldChatText(currentTurn)), " ")
	if folded == "" {
		return PassengerClarificationSlots{}
	}

	slots := PassengerClarificationSlots{}

	switch {
	case containsAnyFolded(folded, "eu e mais uma pessoa", "eu e mais uma", "eu e mais um passageiro", "eu e mais um acompanhante", "eu e outra pessoa", "eu e outra", "eu e minha", "eu e meu", "eu e minha esposa", "eu e meu esposo", "eu e minha filha", "eu e meu filho", "eu e minha mulher", "eu e meu marido"):
		slots.PassengerCount = 2
		slots.PassengerCountKnown = true
	case containsAnyFolded(folded, "pra mim", "para mim", "so pra mim", "so para mim", "e so pra mim", "eh so pra mim", "e so para mim", "eh so para mim", "passagem so para mim", "so eu", "somente eu", "e so eu", "eh so eu", "sou eu", "sou so eu", "sozinho", "vou sozinho", "vou so", "uma pessoa", "1 pessoa", "um passageiro"):
		slots.PassengerCount = 1
		slots.PassengerCountKnown = true
	case containsAnyFolded(folded, "duas pessoas", "dois passageiros", "2 pessoas", "2 passageiros", "as duas", "os dois", "nos duas", "nos dois", "dos dois", "das duas"):
		slots.PassengerCount = 2
		slots.PassengerCountKnown = true
	case containsAnyFolded(folded, "tres pessoas", "3 pessoas", "tres passageiros", "3 passageiros"):
		slots.PassengerCount = 3
		slots.PassengerCountKnown = true
	case containsAnyFolded(folded, "quatro pessoas", "4 pessoas", "quatro passageiros", "4 passageiros"):
		slots.PassengerCount = 4
		slots.PassengerCountKnown = true
	case containsAnyFolded(folded, "cinco pessoas", "5 pessoas", "cinco passageiros", "5 passageiros"):
		slots.PassengerCount = 5
		slots.PassengerCountKnown = true
	}

	if !slots.PassengerCountKnown {
		if match := passengerDigitQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
			if qty := readInt(match[1]); qty > 0 {
				slots.PassengerCount = qty
				slots.PassengerCountKnown = true
			}
		}
	}

	if !slots.PassengerCountKnown {
		if match := passengerWordQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
			switch match[1] {
			case "um", "uma":
				slots.PassengerCount = 1
			case "dois", "duas":
				slots.PassengerCount = 2
			case "tres":
				slots.PassengerCount = 3
			case "quatro":
				slots.PassengerCount = 4
			case "cinco":
				slots.PassengerCount = 5
			}
			slots.PassengerCountKnown = slots.PassengerCount > 0
		}
	}

	if isShortNoReply(folded) ||
		containsAnyFolded(
			folded,
			"nao tem crianca",
			"não tem criança",
			"sem crianca",
			"sem criança",
			"nenhuma crianca",
			"nenhuma criança",
			"tem nenhuma crianca nao",
			"tem nenhuma criança não",
			"tem nenhuma crianca não",
			"tem crianca nao",
			"tem criança não",
			"crianca nao",
			"criança não",
			"nao vai crianca",
			"não vai criança",
			"nao leva crianca",
			"não leva criança",
			"nao leva crianca de 5 anos",
			"sem crianca de 5 anos",
			"nao tem filhos",
			"sem filhos",
		) {
		slots.ChildUnder5Count = 0
		slots.ChildUnder5CountKnown = true
	}
	if !slots.ChildUnder5CountKnown {
		if match := childUnder5AgePattern.FindStringSubmatch(folded); len(match) == 2 {
			slots.ChildUnder5Count = 1
			slots.ChildUnder5CountKnown = true
		}
	}

	if !slots.ChildUnder5CountKnown &&
		containsAnyFolded(folded, "tem crianca", "uma crianca", "1 crianca", "meu filho tem", "minha filha tem", "meu filho de", "minha filha de") {
		slots.ChildUnder5Count = 1
		slots.ChildUnder5CountKnown = true
	}

	return slots
}

func isShortNoReply(folded string) bool {
	switch strings.TrimSpace(folded) {
	case "nao", "n":
		return true
	default:
		return false
	}
}

func containsAnyFolded(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		foldedPattern := strings.Join(strings.Fields(foldChatText(pattern)), " ")
		if foldedPattern != "" && strings.Contains(value, foldedPattern) {
			return true
		}
	}
	return false
}

func normalizeDigits(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func normalizeAlphaNumeric(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(value)) {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func findLatestSelectedOptionIndex(history []Message) int {
	for i := len(history) - 1; i >= 0; i-- {
		body := strings.TrimSpace(history[i].Body)
		if body == "" {
			continue
		}
		if index := extractSelectedOptionIndex(body); index > 0 {
			return index
		}
	}
	return 0
}

func findLatestPassengerDetailsText(history []Message, session Session) string {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		body := strings.TrimSpace(message.Body)
		if body == "" || looksLikeBookingCreateConfirmation(body) {
			continue
		}
		if len(extractBookingCreatePassengers(body, session)) > 0 {
			return body
		}
	}
	return ""
}

func inferLapChildCount(history []Message) int {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if message.Direction != "INBOUND" || !looksLikeBookingCreateConfirmation(message.Body) {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			previous := history[j]
			if previous.Direction != "OUTBOUND" {
				continue
			}
			folded := foldChatText(previous.Body)
			if strings.Contains(folded, "ate 5 anos") || strings.Contains(folded, "crianca de 5 anos ou menos") {
				return 1
			}
			break
		}
	}
	return 0
}

func applyLapChildFlags(passengers []BookingCreatePassengerInput, lapChildCount int) {
	if lapChildCount <= 0 || len(passengers) == 0 {
		return
	}
	if lapChildCount > len(passengers)-1 {
		lapChildCount = len(passengers) - 1
	}
	for i := len(passengers) - lapChildCount; i < len(passengers); i++ {
		if i >= 0 && i < len(passengers) {
			passengers[i].IsLapChild = true
		}
	}
}

func normalizePassengerDocumentType(value string) string {
	folded := strings.TrimSpace(foldChatText(value))
	switch folded {
	case "cpf":
		return "CPF"
	case "rg":
		return "RG"
	case "cnh":
		return "CNH"
	case "certidao_nascimento", "certidao de nascimento", "certidao", "matricula":
		return "CERTIDAO_NASCIMENTO"
	default:
		return ""
	}
}

func normalizePassengerDocumentValue(value string, documentType string) string {
	switch documentType {
	case "CPF":
		document := normalizeDigits(value)
		if len(document) == 11 {
			return document
		}
		return ""
	case "RG", "CNH", "CERTIDAO_NASCIMENTO":
		return normalizeAlphaNumeric(value)
	default:
		return ""
	}
}

func buildBookingCreateIdempotencyKey(session Session, input BookingCreateInput) string {
	parts := []string{
		strings.TrimSpace(session.ContactKey),
		strings.TrimSpace(input.TripID),
		strings.TrimSpace(input.BoardStopID),
		strings.TrimSpace(input.AlightStopID),
	}
	for _, item := range input.Passengers {
		parts = append(parts, strings.TrimSpace(item.DocumentType)+"="+strings.TrimSpace(item.Document))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "chat-booking-create-" + hex.EncodeToString(hash[:16])
}

func findLatestAvailabilityContext(history []Message) *AvailabilitySearchResult {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if message.Direction != "OUTBOUND" || !isAutomationDraftStatus(message.ProcessingStatus) {
			continue
		}
		toolContext := asMap(message.Payload["tool_context"])
		if len(toolContext) == 0 {
			continue
		}
		payload := asMap(toolContext[toolNameAvailabilitySearch])
		if len(payload) == 0 {
			continue
		}
		result := parseAvailabilityContextPayload(payload)
		if len(result.Results) > 0 {
			return &result
		}
	}
	return nil
}

func parseAvailabilityContextPayload(payload map[string]interface{}) AvailabilitySearchResult {
	result := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      strings.TrimSpace(asString(payload["origin"])),
			Destination: strings.TrimSpace(asString(payload["destination"])),
			PackageName: strings.TrimSpace(asString(payload["package_name"])),
			Qty:         asInt(payload["qtd"]),
			Limit:       len(asInterfaceSliceMaps(payload["results"])),
		},
		Results: []AvailabilitySearchItem{},
	}
	if rawDate := strings.TrimSpace(asString(payload["trip_date"])); rawDate != "" {
		if parsed, err := time.Parse("2006-01-02", rawDate); err == nil {
			parsed = parsed.UTC()
			result.Filter.TripDate = &parsed
		}
	}
	for _, item := range asInterfaceSliceMaps(payload["results"]) {
		result.Results = append(result.Results, AvailabilitySearchItem{
			SegmentID:              strings.TrimSpace(asString(item["segment_id"])),
			TripID:                 strings.TrimSpace(asString(item["trip_id"])),
			RouteID:                strings.TrimSpace(asString(item["route_id"])),
			BoardStopID:            strings.TrimSpace(asString(item["board_stop_id"])),
			AlightStopID:           strings.TrimSpace(asString(item["alight_stop_id"])),
			OriginStopID:           strings.TrimSpace(asString(item["origin_stop_id"])),
			DestinationStopID:      strings.TrimSpace(asString(item["destination_stop_id"])),
			OriginDisplayName:      strings.TrimSpace(asString(item["origin_display_name"])),
			DestinationDisplayName: strings.TrimSpace(asString(item["destination_display_name"])),
			OriginDepartTime:       strings.TrimSpace(asString(item["origin_depart_time"])),
			TripDate:               strings.TrimSpace(asString(item["trip_date"])),
			SeatsAvailable:         asInt(item["seats_available"]),
			Price:                  asFloat64(item["price"]),
			Currency:               strings.TrimSpace(asString(item["currency"])),
			Status:                 strings.TrimSpace(asString(item["status"])),
			TripStatus:             strings.TrimSpace(asString(item["trip_status"])),
			PackageName:            strings.TrimSpace(asString(item["package_name"])),
		})
	}
	return result
}

func buildBookingCreateRequestPayload(input BookingCreateInput) map[string]interface{} {
	passengers := make([]map[string]interface{}, 0, len(input.Passengers))
	for _, item := range input.Passengers {
		passengers = append(passengers, map[string]interface{}{
			"name":          item.Name,
			"document":      item.Document,
			"document_type": item.DocumentType,
			"phone":         item.Phone,
			"is_lap_child":  item.IsLapChild,
		})
	}
	return map[string]interface{}{
		"selected_option_index":    input.SelectedOptionIndex,
		"trip_id":                  input.TripID,
		"board_stop_id":            input.BoardStopID,
		"alight_stop_id":           input.AlightStopID,
		"origin_display_name":      input.OriginDisplayName,
		"destination_display_name": input.DestinationDisplayName,
		"trip_date":                input.TripDate,
		"departure_time":           input.DepartureTime,
		"qtd":                      input.Qty,
		"customer_name":            input.CustomerName,
		"customer_phone":           input.CustomerPhone,
		"idempotency_key":          input.IdempotencyKey,
		"passengers":               passengers,
	}
}

func buildBookingCreateResponsePayload(result BookingCreateResult) map[string]interface{} {
	passengers := make([]map[string]interface{}, 0, len(result.Passengers))
	for _, item := range result.Passengers {
		passengers = append(passengers, map[string]interface{}{
			"name":          item.Name,
			"document":      item.Document,
			"document_type": item.DocumentType,
			"phone":         item.Phone,
			"seat_id":       item.SeatID,
			"is_lap_child":  item.IsLapChild,
		})
	}
	payload := map[string]interface{}{
		"mode":              result.Mode,
		"booking_id":        result.BookingID,
		"reservation_code":  result.ReservationCode,
		"status":            result.Status,
		"total_amount":      result.TotalAmount,
		"deposit_amount":    result.DepositAmount,
		"remainder_amount":  result.RemainderAmount,
		"passenger_count":   len(result.Passengers),
		"passengers":        passengers,
		"errors":            result.Errors,
		"message_for_agent": result.MessageForAgent,
	}
	if result.ReservedUntil != nil {
		payload["reserved_until"] = result.ReservedUntil.UTC().Format(time.RFC3339Nano)
	}
	return payload
}

func asFloat64(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}
