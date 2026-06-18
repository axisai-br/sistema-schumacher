package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	optionIndexPattern             = regexp.MustCompile(`(?i)\bop[cç][aã]o\s*0?([1-5])\b`)
	passengerNamePattern           = regexp.MustCompile(`(?i)\bnome(?:\s+completo)?\s*(?:é|e|:|-)?\s*([A-ZÀ-ÿ][A-Za-zÀ-ÿ' ]{3,100}?)(?:\s+(?:cpf|rg|cnh|certid[aã]o|matr[ií]cula)\b|$)`)
	passengerAltNamePattern        = regexp.MustCompile(`(?i)\b(?:meu nome|sou)\s*(?:é|e)?\s*([A-ZÀ-ÿ][A-Za-zÀ-ÿ' ]{3,100}?)(?:\s+(?:cpf|rg|cnh|certid[aã]o|matr[ií]cula)\b|$)`)
	passengerCPFPattern            = regexp.MustCompile(`(?i)\bcpf\b[^0-9]*([0-9.\-]{11,14})`)
	passengerRGPattern             = regexp.MustCompile(`(?i)\brg\b[^A-Z0-9]*([A-Z0-9.\-]{4,20})`)
	passengerCNHPattern            = regexp.MustCompile(`(?i)\bcnh\b[^A-Z0-9]*([A-Z0-9.\-]{4,20})`)
	passengerBirthRecordPattern    = regexp.MustCompile(`(?i)\b(?:certid[aã]o(?: de nascimento)?|matr[ií]cula)\b[^A-Z0-9]*([A-Z0-9][A-Z0-9.\- ]{6,80})`)
	passengerBirthDatePattern      = regexp.MustCompile(`(?i)\b(?:data\s+de\s+nascimento|nascimento|nasc\.?)\b[^0-9]*([0-9]{2}/[0-9]{2}/[0-9]{4}|[0-9]{4}-[0-9]{2}-[0-9]{2})`)
	passengerBirthCityPattern      = regexp.MustCompile(`(?i)\b(?:naturalidade|natural\s+de|cidade\s+de\s+nascimento)\b\s*(?:é|e|:|-|de)?\s*([A-ZÀ-ÿ][A-Za-zÀ-ÿ' ]{1,80})`)
	passengerLooseCPFLinePattern   = regexp.MustCompile(`(?i)^\s*([A-Za-zÀ-ÿ][A-Za-zÀ-ÿ' ]{3,100}?)\s+([0-9.\-]{11,14})\s*$`)
	passengerLooseTypedLinePattern = regexp.MustCompile(`(?i)^\s*([A-Za-zÀ-ÿ][A-Za-zÀ-ÿ' ]{3,100}?)\s+(cpf|rg|cnh|certid[aã]o(?: de nascimento)?|matr[ií]cula)\s+([A-Z0-9.\- ]{4,80}?)(?:\s+(?:cpf|rg|cnh|certid[aã]o|matr[ií]cula|data\s+de\s+nascimento|nascimento|nasc\.?|naturalidade|natural\s+de|cidade\s+de\s+nascimento)\b.*)?\s*$`)
	passengerInlineCPFPattern      = regexp.MustCompile(`[0-9]{3}\.?[0-9]{3}\.?[0-9]{3}-?[0-9]{2}`)
	passengerInlineNamePrefix      = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?(?:passageir[oa]\s*)?[0-9]{1,2}\s*[\).:-]\s*`)
	passengerInlineNameSuffix      = regexp.MustCompile(`(?i)(?:\b(?:cpf|documento|doc)\b|[-–—/|])+\s*$`)
	passengerWordQtyPattern        = regexp.MustCompile(`\b(um|uma|dois|duas|tres|quatro|cinco)\s+(?:pessoas?|passageiros?|passagens?|assentos?|lugares?)\b`)
	passengerDigitQtyPattern       = regexp.MustCompile(`\b([1-9])\s+(?:pessoas?|passageiros?|passagens?|assentos?|lugares?)\b`)
	passengerSomosEmQtyPattern     = regexp.MustCompile(`\bsomos\s+em\s+([1-9])\b`)
	passengerMePlusDigitQtyPattern = regexp.MustCompile(`\b(?:eu|pra mim|para mim)\s+e\s+mais\s+([1-9])\s+(?:pessoas?|passageiros?|passagens?|assentos?|lugares?)\b`)
	passengerMePlusWordQtyPattern  = regexp.MustCompile(`\b(?:eu|pra mim|para mim)\s+e\s+mais\s+(um|uma|dois|duas|tres|quatro|cinco)\s+(?:pessoas?|passageiros?|passagens?|assentos?|lugares?)\b`)
	soloPassengerReplyPattern      = regexp.MustCompile(`\b(?:(?:e|eh)\s+)?(?:so|somente|apenas)\s+(?:eu|pra mim|para mim|mim|pra ele|para ele|ele|pra ela|para ela|ela|pra voce|para voce|voce)\b`)
	childUnder5AgePattern          = regexp.MustCompile(`\b(?:meu|minha|filho|filha|crianca|menino|menina|bebe)\s+(?:filho|filha|crianca|menino|menina|bebe)?\s*(?:tem|de)\s+([0-5])(?:\s+anos?)?\b`)
)

var (
	brazilianUFs       = []string{"AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG", "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO"}
	rgIssuerTokens     = []string{"SSP", "SDS", "SESP", "IFP", "PC"}
	rgIssuerSuffixes   = buildRGIssuerSuffixes()
	cpfCandidateRegexp = regexp.MustCompile(`[0-9][0-9.\-\s]{9,20}[0-9]`)
)

type PassengerDocumentCorrection struct {
	NameConfirmed          bool
	Name                   string
	DocumentType           string
	Document               string
	CPF                    string
	RG                     string
	CNH                    string
	BirthDate              string
	BirthCertificateNumber string
	BirthCity              string
}

type PassengerClarificationSlots struct {
	PassengerCount        int
	PassengerCountKnown   bool
	ChildUnder5Count      int
	ChildUnder5CountKnown bool
}

type BookingPassengerDocumentPartial struct {
	NameFragment string
	DocumentType string
	Document     string
}

type bookingPassengerDocumentProgress struct {
	Passengers []BookingCreatePassengerInput
	Partials   []BookingPassengerDocumentPartial
	SourceText string
}

type inlinePassengerCPFMatch struct {
	Start    int
	End      int
	Raw      string
	Document string
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
	if !validateLapChildStateForBooking(history, body, passengers) {
		return BookingCreateInput{}, false
	}

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

func validateLapChildStateForBooking(history []Message, currentTurn string, passengers []BookingCreatePassengerInput) bool {
	context := collectBookingDraftContext(Session{}, history, currentTurn)
	if context.ChildUnder5Count <= 0 {
		return true
	}
	if len(context.LapChildPassengerIndexes) > 0 {
		applyLapChildPassengerIndexes(passengers, context.LapChildPassengerIndexes)
	}
	return hasExpectedLapChildCount(passengers, context.ChildUnder5Count)
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
	correction, hasCorrection := findLatestPassengerDocumentCorrection(history, currentTurn)

	if extract := findLatestDocumentExtractContext(history); extract != nil {
		if strings.ToUpper(strings.TrimSpace(extract.Mode)) != "EXTRACTED" && !hasCorrection {
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

		passengers = bookingPassengersFromDocumentExtract(*extract, session, context.TripDate)
		if hasCorrection {
			passengers = applyPassengerDocumentCorrection(passengers, correction)
		}
		for _, passenger := range passengers {
			if strings.TrimSpace(passenger.Name) == "" ||
				strings.TrimSpace(passenger.DocumentType) == "" ||
				strings.TrimSpace(passenger.Document) == "" {
				return BookingCreateInput{}, false
			}
		}
	} else {
		passengerSource := context.PassengerDetailsText
		if passengerSource == "" {
			passengerSource = findLatestPassengerDetailsText(history, session)
		}
		passengers = append([]BookingCreatePassengerInput(nil), context.PassengerDetails...)
		if len(passengers) == 0 {
			passengers = extractBookingCreatePassengers(passengerSource, session)
			if len(passengers) == 0 {
				return BookingCreateInput{}, false
			}
		}
		if expected <= 0 {
			expected = inferExpectedPassengerCount(history, currentTurn, passengerSource)
		}
		if expected <= 0 {
			expected = len(passengers)
		}
		if hasCorrection {
			passengers = applyPassengerDocumentCorrection(passengers, correction)
		}
	}

	if expected <= 0 || len(passengers) != expected {
		return BookingCreateInput{}, false
	}
	if len(context.LapChildPassengerIndexes) > 0 {
		applyLapChildPassengerIndexes(passengers, context.LapChildPassengerIndexes)
	}
	if count := countLapChildPassengers(passengers); count > 0 && !context.ChildUnder5CountKnown {
		context.ChildUnder5Count = count
		context.ChildUnder5CountKnown = true
	}
	if context.ChildUnder5Count > 0 && !hasExpectedLapChildCount(passengers, context.ChildUnder5Count) {
		return BookingCreateInput{}, false
	}

	input := buildBookingCreateInputFromDraftContext(session, context, passengers, expected)
	input.IdempotencyKey = buildBookingCreateIdempotencyKey(session, input)
	return input, true
}

func bookingPassengerFromDocumentExtract(extracted DocumentExtractPassenger, session Session) BookingCreatePassengerInput {
	extracted = normalizeDocumentExtractPassenger(extracted)
	documentType := normalizePassengerDocumentType(extracted.DocumentType)
	document := normalizePassengerDocumentValue(extracted.Document, documentType)
	return BookingCreatePassengerInput{
		Name:                   strings.TrimSpace(extracted.Name),
		DocumentType:           documentType,
		Document:               document,
		CPF:                    normalizePassengerDocumentValue(extracted.CPF, "CPF"),
		RG:                     firstNonEmpty(normalizePassengerDocumentValue(extracted.RG, "RG"), documentValueForType(document, documentType, "RG")),
		CNH:                    firstNonEmpty(normalizePassengerDocumentValue(extracted.CNH, "CNH"), documentValueForType(document, documentType, "CNH")),
		BirthDate:              strings.TrimSpace(extracted.BirthDate),
		BirthCertificateNumber: firstNonEmpty(normalizePassengerDocumentValue(extracted.BirthCertificateNumber, "CERTIDAO_NASCIMENTO"), documentValueForType(document, documentType, "CERTIDAO_NASCIMENTO")),
		BirthCity:              normalizePassengerBirthCity(extracted.BirthCity),
		Phone:                  strings.TrimSpace(session.CustomerPhone),
		Notes:                  buildPassengerIdentityNotes(extracted, documentType, document),
	}
}

func documentValueForType(document string, documentType string, expectedType string) string {
	if !strings.EqualFold(strings.TrimSpace(documentType), expectedType) {
		return ""
	}
	return normalizePassengerDocumentValue(document, expectedType)
}

func buildPassengerIdentityNotes(extracted DocumentExtractPassenger, primaryType string, primaryDocument string) string {
	secondary := make([]string, 0, 6)
	for _, item := range []struct {
		Type     string
		Document string
	}{
		{"CPF", normalizePassengerDocumentValue(extracted.CPF, "CPF")},
		{"CNH", normalizePassengerDocumentValue(extracted.CNH, "CNH")},
		{"RG", normalizePassengerDocumentValue(extracted.RG, "RG")},
		{"CERTIDAO_NASCIMENTO", normalizePassengerDocumentValue(extracted.BirthCertificateNumber, "CERTIDAO_NASCIMENTO")},
	} {
		if item.Document == "" {
			continue
		}
		if item.Type == primaryType && item.Document == primaryDocument {
			continue
		}
		secondary = append(secondary, item.Type+": "+item.Document)
	}
	if birthDate := strings.TrimSpace(extracted.BirthDate); birthDate != "" {
		secondary = append(secondary, "DATA_NASCIMENTO: "+birthDate)
	}
	if birthCity := normalizePassengerBirthCity(extracted.BirthCity); birthCity != "" {
		secondary = append(secondary, "NATURALIDADE: "+birthCity)
	}
	if len(secondary) == 0 {
		return ""
	}
	return "Dados adicionais extraidos: " + strings.Join(secondary, " | ")
}

func bookingPassengersFromDocumentExtract(result DocumentExtractResult, session Session, tripDate string) []BookingCreatePassengerInput {
	passengers := make([]BookingCreatePassengerInput, 0, len(result.Passengers))
	for _, extracted := range result.Passengers {
		passenger := bookingPassengerFromDocumentExtract(extracted, session)
		if extracted.IsLapChild || isLapChildFromBirthDate(extracted.BirthDate, tripDate) {
			passenger.IsLapChild = true
		}
		passengers = append(passengers, passenger)
	}
	return passengers
}

func parsePassengerDocumentCorrection(text string) (PassengerDocumentCorrection, bool) {
	body := strings.TrimSpace(text)
	if body == "" {
		return PassengerDocumentCorrection{}, false
	}

	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	looseNameCPF := passengerLooseCPFLinePattern.FindStringSubmatch(body)
	document, documentType := extractBookingPassengerDocument(body)
	cpf := extractValidCPF(body)
	if document == "" && cpf != "" {
		document = cpf
		documentType = "CPF"
	}
	hasCorrectionCue := looksLikeBareCPF(body) ||
		len(looseNameCPF) == 3 ||
		strings.Contains(folded, "cpf") ||
		strings.Contains(folded, "rg") ||
		strings.Contains(folded, "cnh") ||
		strings.Contains(folded, "certidao") ||
		strings.Contains(folded, "matricula") ||
		strings.Contains(folded, "nascimento") ||
		strings.Contains(folded, "naturalidade") ||
		strings.Contains(folded, "documento correto") ||
		strings.Contains(folded, "documento certo") ||
		strings.Contains(folded, "corrige") ||
		strings.Contains(folded, "corrigir") ||
		strings.Contains(folded, "correto") ||
		strings.Contains(folded, "usar esse documento") ||
		strings.Contains(folded, "use esse documento")
	if !hasCorrectionCue {
		return PassengerDocumentCorrection{}, false
	}

	correction := PassengerDocumentCorrection{
		NameConfirmed: strings.Contains(folded, "nome esta certo") ||
			strings.Contains(folded, "nome ta certo") ||
			strings.Contains(folded, "nome correto") ||
			strings.Contains(folded, "nome esta correto"),
		DocumentType: documentType,
		Document:     document,
		CPF:          cpf,
		RG:           extractPassengerRG(body),
		CNH:          extractPassengerCNH(body),
		BirthDate:    extractPassengerBirthDate(body),
		BirthCity:    extractPassengerBirthCity(body),
	}
	correction.BirthCertificateNumber = extractPassengerBirthCertificateNumber(body)

	if len(looseNameCPF) == 3 {
		correction.Name = normalizePassengerName(looseNameCPF[1])
	}
	if correction.Name == "" && !correction.NameConfirmed {
		correction.Name = extractBookingPassengerName(body, "")
	}

	if !correction.hasChanges() {
		return PassengerDocumentCorrection{}, false
	}
	return correction, true
}

func (c PassengerDocumentCorrection) hasChanges() bool {
	return strings.TrimSpace(c.Name) != "" ||
		strings.TrimSpace(c.Document) != "" ||
		strings.TrimSpace(c.CPF) != "" ||
		strings.TrimSpace(c.RG) != "" ||
		strings.TrimSpace(c.CNH) != "" ||
		strings.TrimSpace(c.BirthDate) != "" ||
		strings.TrimSpace(c.BirthCertificateNumber) != "" ||
		strings.TrimSpace(c.BirthCity) != "" ||
		c.NameConfirmed
}

func extractValidCPF(text string) string {
	if match := passengerCPFPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		if document := normalizePassengerDocumentValue(match[1], "CPF"); document != "" {
			return document
		}
	}
	for _, candidate := range cpfCandidateRegexp.FindAllString(text, -1) {
		if document := normalizePassengerDocumentValue(candidate, "CPF"); document != "" {
			return document
		}
	}
	digits := normalizeDigits(text)
	if isValidCPF(digits) {
		return digits
	}
	return ""
}

func findLatestPassengerDocumentCorrection(history []Message, currentTurn string) (PassengerDocumentCorrection, bool) {
	if correction, ok := parsePassengerDocumentCorrection(currentTurn); ok {
		return correction, true
	}

	inCorrectionWindow := false
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		body := strings.TrimSpace(messageTurnText(message))
		if body == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			folded := strings.Join(strings.Fields(foldChatText(body)), " ")
			if strings.Contains(folded, "vou usar") ||
				strings.Contains(folded, "usando o cpf") ||
				strings.Contains(folded, "posso prosseguir") ||
				strings.Contains(folded, "eles conferem") ||
				strings.Contains(folded, "preciso confirmar antes de seguir") {
				inCorrectionWindow = true
				continue
			}
			if looksLikePassengerDocumentRequest(folded) {
				break
			}
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
			continue
		}
		if !inCorrectionWindow {
			continue
		}
		if correction, ok := parsePassengerDocumentCorrection(body); ok {
			return correction, true
		}
	}
	return PassengerDocumentCorrection{}, false
}

func applyPassengerDocumentCorrection(passengers []BookingCreatePassengerInput, correction PassengerDocumentCorrection) []BookingCreatePassengerInput {
	if len(passengers) == 0 {
		return passengers
	}

	out := append([]BookingCreatePassengerInput(nil), passengers...)
	target := -1
	if correction.Name != "" {
		for i, passenger := range out {
			if strings.EqualFold(strings.Join(strings.Fields(foldChatText(passenger.Name)), " "), strings.Join(strings.Fields(foldChatText(correction.Name)), " ")) {
				target = i
				break
			}
		}
	}
	if target < 0 && len(out) == 1 {
		target = 0
	}
	if target < 0 {
		return out
	}

	if correction.Name != "" {
		out[target].Name = correction.Name
	}
	if strings.TrimSpace(correction.DocumentType) != "" && strings.TrimSpace(correction.Document) != "" {
		out[target].DocumentType = normalizePassengerDocumentType(correction.DocumentType)
		out[target].Document = normalizePassengerDocumentValue(correction.Document, out[target].DocumentType)
	}
	if correction.CPF != "" {
		out[target].CPF = normalizePassengerDocumentValue(correction.CPF, "CPF")
	}
	if correction.RG != "" {
		out[target].RG = normalizePassengerDocumentValue(correction.RG, "RG")
	}
	if correction.CNH != "" {
		out[target].CNH = normalizePassengerDocumentValue(correction.CNH, "CNH")
	}
	if correction.BirthDate != "" {
		out[target].BirthDate = correction.BirthDate
	}
	if correction.BirthCertificateNumber != "" {
		out[target].BirthCertificateNumber = normalizePassengerDocumentValue(correction.BirthCertificateNumber, "CERTIDAO_NASCIMENTO")
	}
	if correction.BirthCity != "" {
		out[target].BirthCity = normalizePassengerBirthCity(correction.BirthCity)
	}
	switch out[target].DocumentType {
	case "CPF":
		out[target].CPF = normalizePassengerDocumentValue(out[target].Document, "CPF")
	case "RG":
		out[target].RG = normalizePassengerDocumentValue(out[target].Document, "RG")
	case "CNH":
		out[target].CNH = normalizePassengerDocumentValue(out[target].Document, "CNH")
	case "CERTIDAO_NASCIMENTO":
		out[target].BirthCertificateNumber = normalizePassengerDocumentValue(out[target].Document, "CERTIDAO_NASCIMENTO")
	}
	return out
}

func buildBookingCreateInputFromDraftContext(session Session, context BookingDraftContext, passengers []BookingCreatePassengerInput, expected int) BookingCreateInput {
	return BookingCreateInput{
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
}

func looksLikeDocumentConfirmation(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}

	if looksLikeDocumentCorrectionOrRejection(text) {
		return false
	}

	exactConfirmations := map[string]struct{}{
		"conferem":                         {},
		"confere":                          {},
		"sim":                              {},
		"sim sim":                          {},
		"s":                                {},
		"ss":                               {},
		"isso":                             {},
		"isso mesmo":                       {},
		"correto":                          {},
		"certo":                            {},
		"sim correto":                      {},
		"sim esta correto":                 {},
		"esta correto":                     {},
		"sim esta certo":                   {},
		"sim ta certo":                     {},
		"esta certo":                       {},
		"ta certo":                         {},
		"tudo certo":                       {},
		"ta tudo certo":                    {},
		"esta tudo certo":                  {},
		"sim tudo certo":                   {},
		"sim ta tudo certo":                {},
		"sim esta tudo certo":              {},
		"sim sim ta tudo certo":            {},
		"sim sim esta tudo certo":          {},
		"sim sim ta tudo certo tudo certo": {},
		"confirmo":                         {},
		"confirmado":                       {},
		"positivo":                         {},
		"posi":                             {},
		"aham":                             {},
		"uhum":                             {},
		"ok":                               {},
		"okay":                             {},
		"beleza":                           {},
		"blz":                              {},
		"pode sim":                         {},
		"pode seguir":                      {},
		"pode continuar":                   {},
		"pode prosseguir":                  {},
		"pode criar":                       {},
		"pode reservar":                    {},
	}

	if _, ok := exactConfirmations[folded]; ok {
		return true
	}

	confirmationPhrases := []string{
		"esta correto",
		"ta correto",
		"esta certo",
		"ta certo",
		"tudo certo",
		"esta tudo certo",
		"ta tudo certo",
		"os dados estao certos",
		"os dados tao certos",
		"as informacoes estao certas",
		"as informacoes tao certas",
		"as informacoes conferem",
		"os dados conferem",
		"pode seguir",
		"pode continuar",
		"pode prosseguir",
		"pode criar",
		"pode criar a reserva",
		"pode fazer a reserva",
		"pode reservar",
		"pode fazer",
		"pode sim",
	}

	return containsFoldedPhrase(folded, confirmationPhrases)
}

func looksLikeDocumentConfirmationContextReply(text string) bool {
	return looksLikeDocumentConfirmation(text) || looksLikeDocumentCorrectionReply(text)
}

func looksLikeDocumentCorrectionReply(text string) bool {
	_, ok := parsePassengerDocumentCorrection(text)
	return ok
}

func looksLikeDocumentCorrectionOrRejection(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	if _, ok := parsePassengerDocumentCorrection(text); ok {
		return true
	}

	rejectionPhrases := []string{
		"nao",
		"nao esta certo",
		"nao ta certo",
		"nao confere",
		"esta errado",
		"ta errado",
		"errado",
		"incorreto",
		"incorreta",
		"tem erro",
		"tem um erro",
	}

	if containsFoldedPhrase(folded, rejectionPhrases) {
		return true
	}

	correctionPhrases := []string{
		"corrigir",
		"corrige",
		"corrija",
		"alterar",
		"altera",
		"altere",
		"mudar",
		"muda",
		"mude",
		"trocar",
		"troca",
		"troque",
		"arrumar",
		"arruma",
		"arrume",
		"ajustar",
		"ajusta",
		"ajuste",
		"corrigir o cpf",
		"corrigir cpf",
		"alterar o cpf",
		"alterar cpf",
		"trocar o cpf",
		"trocar cpf",
		"corrigir o documento",
		"alterar o documento",
		"trocar o documento",
		"corrigir o nome",
		"alterar o nome",
		"trocar o nome",
	}

	return containsFoldedPhrase(folded, correctionPhrases)
}

func containsFoldedPhrase(text string, phrases []string) bool {
	paddedText := " " + text + " "
	for _, phrase := range phrases {
		normalizedPhrase := strings.Join(strings.Fields(foldChatText(phrase)), " ")
		if normalizedPhrase == "" {
			continue
		}
		if paddedText == " "+normalizedPhrase+" " || strings.Contains(paddedText, " "+normalizedPhrase+" ") {
			return true
		}
	}
	return false
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
			strings.Contains(body, "consegui ler parte do documento") ||
			strings.Contains(body, "preciso confirmar antes de seguir") ||
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
			strings.Contains(body, "esta tudo correto para eu confirmar") ||
			strings.Contains(body, "seguir com o proximo passo") ||
			strings.Contains(body, "seguir com a reserva") ||
			strings.Contains(body, "criar a reserva") ||
			strings.Contains(body, "fazer a reserva") ||
			strings.Contains(body, "prosseguir e criar")
	}
	return false
}

func lastAssistantAskedLapChildAssignment(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		return assistantAskedLapChildAssignment(message.Body)
	}
	return false
}

func previousAssistantAskedLapChildAssignment(history []Message, beforeIndex int) bool {
	for i := beforeIndex - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		return assistantAskedLapChildAssignment(message.Body)
	}
	return false
}

func assistantAskedLapChildAssignment(text string) bool {
	body := strings.Join(strings.Fields(foldChatText(text)), " ")
	if body == "" {
		return false
	}
	return strings.Contains(body, "qual deles e a crianca") ||
		strings.Contains(body, "qual passageiro e a crianca") ||
		strings.Contains(body, "crianca de ate 5 anos e qual passageiro") ||
		strings.Contains(body, "crianca de ate 5 anos e qual deles")
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
	case "pix", "integral", "sinal", "entrada":
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
		"ss",
		"s",
		"positivo",
		"posi",
		"joia",
		"claro",
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
		"entrada":
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
	case "sim", "ss", "s", "posi", "exato", "positivo", "isso", "isso mesmo", "pode seguir", "pode reservar", "confirmo", "confirmado", "ok", "certo",
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
	if progress := extractInlineBookingPassengerDocuments(text, session); len(progress.Passengers) > 0 && passengerDocumentProgressCount(progress) > 1 {
		return progress.Passengers
	}
	if match := passengerLooseCPFLinePattern.FindStringSubmatch(text); len(match) == 3 {
		name := normalizePassengerName(match[1])
		document := normalizeDigits(match[2])
		if name != "" && isValidCPF(document) {
			passenger := enrichPassengerAdditionalIdentityFromText(BookingCreatePassengerInput{
				Name:         name,
				Document:     document,
				DocumentType: "CPF",
				CPF:          document,
				Phone:        strings.TrimSpace(session.CustomerPhone),
			}, text)
			return []BookingCreatePassengerInput{
				passenger,
			}
		}
	}
	if passenger, ok := parseLooseTypedPassengerDocument(text, session); ok {
		return []BookingCreatePassengerInput{enrichPassengerAdditionalIdentityFromText(passenger, text)}
	}
	if passenger, ok := parseStructuredPassengerLine(text, session); ok {
		return []BookingCreatePassengerInput{passenger}
	}
	document, documentType := extractBookingPassengerDocument(text)
	if document == "" || documentType == "" {
		return nil
	}
	name := extractBookingPassengerName(text, session.CustomerName)
	if name == "" {
		return nil
	}
	passenger := BookingCreatePassengerInput{
		Name:         name,
		Document:     document,
		DocumentType: documentType,
		Phone:        strings.TrimSpace(session.CustomerPhone),
	}
	return []BookingCreatePassengerInput{enrichPassengerAdditionalIdentityFromText(passenger, text)}
}

func extractBookingPassengerDocumentProgress(text string, session Session) bookingPassengerDocumentProgress {
	text = strings.TrimSpace(text)
	if text == "" {
		return bookingPassengerDocumentProgress{}
	}
	progress := extractInlineBookingPassengerDocuments(text, session)
	if len(progress.Partials) > 0 {
		progress.SourceText = text
		return progress
	}
	if passengerDocumentProgressCount(progress) > 1 {
		if passengers := extractBookingCreatePassengersByLines(text, session); len(passengers) > 0 {
			return bookingPassengerDocumentProgress{
				Passengers: passengers,
				SourceText: text,
			}
		}
		progress.SourceText = text
		return progress
	}
	passengers := extractBookingCreatePassengers(text, session)
	if len(passengers) == 0 {
		return bookingPassengerDocumentProgress{}
	}
	return bookingPassengerDocumentProgress{
		Passengers: passengers,
		SourceText: text,
	}
}

func passengerDocumentProgressCount(progress bookingPassengerDocumentProgress) int {
	return len(progress.Passengers) + len(progress.Partials)
}

func extractInlineBookingPassengerDocuments(text string, session Session) bookingPassengerDocumentProgress {
	text = strings.TrimSpace(text)
	if text == "" {
		return bookingPassengerDocumentProgress{}
	}

	matches := inlinePassengerCPFMatches(text)
	if len(matches) == 0 {
		return bookingPassengerDocumentProgress{}
	}

	progress := bookingPassengerDocumentProgress{
		Passengers: make([]BookingCreatePassengerInput, 0, len(matches)),
		Partials:   make([]BookingPassengerDocumentPartial, 0),
		SourceText: text,
	}
	nameStart := 0
	for _, match := range matches {
		namePart := cleanInlinePassengerName(text[nameStart:match.Start])
		name := normalizePassengerName(namePart)
		segment := strings.TrimSpace(text[nameStart:match.End])
		if name != "" {
			passenger := BookingCreatePassengerInput{
				Name:         name,
				Document:     match.Document,
				DocumentType: "CPF",
				CPF:          match.Document,
				Phone:        strings.TrimSpace(session.CustomerPhone),
			}
			progress.Passengers = append(progress.Passengers, enrichPassengerAdditionalIdentityFromText(passenger, segment))
		} else if fragment := normalizePassengerNameFragment(namePart); fragment != "" {
			progress.Partials = append(progress.Partials, BookingPassengerDocumentPartial{
				NameFragment: fragment,
				DocumentType: "CPF",
				Document:     match.Document,
			})
		}
		nameStart = match.End
	}

	if len(progress.Passengers) == 0 && len(progress.Partials) == 0 {
		return bookingPassengerDocumentProgress{}
	}
	return progress
}

func inlinePassengerCPFMatches(text string) []inlinePassengerCPFMatch {
	indices := passengerInlineCPFPattern.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return nil
	}
	matches := make([]inlinePassengerCPFMatch, 0, len(indices))
	for _, index := range indices {
		start, end := index[0], index[1]
		if hasAdjacentDigit(text, start, end) {
			continue
		}
		raw := text[start:end]
		document := normalizePassengerDocumentValue(raw, "CPF")
		if document == "" {
			continue
		}
		matches = append(matches, inlinePassengerCPFMatch{
			Start:    start,
			End:      end,
			Raw:      raw,
			Document: document,
		})
	}
	return matches
}

func hasAdjacentDigit(text string, start int, end int) bool {
	if start > 0 && isASCIIDigit(text[start-1]) {
		return true
	}
	if end < len(text) && isASCIIDigit(text[end]) {
		return true
	}
	return false
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func cleanInlinePassengerName(value string) string {
	value = normalizeInlinePassengerNameStart(value)
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		label := strings.Join(strings.Fields(foldChatText(value[:idx])), " ")
		switch label {
		case "nome", "nome completo", "passageiro", "passageira", "adulto", "adulta", "crianca", "filho", "filha":
			value = value[idx+1:]
		}
	}
	value = normalizeInlinePassengerNameStart(value)
	for {
		cleaned := passengerInlineNameSuffix.ReplaceAllString(strings.TrimSpace(value), "")
		cleaned = strings.Trim(cleaned, " \t\r\n,.;:-/|*")
		cleaned = normalizeInlinePassengerNameStart(cleaned)
		if cleaned == value {
			break
		}
		value = cleaned
	}
	return strings.Join(strings.Fields(strings.Trim(value, " \t\r\n,.;:-/|*")), " ")
}

func normalizeInlinePassengerNameStart(value string) string {
	for {
		previous := value
		value = strings.TrimSpace(value)
		value = strings.TrimLeft(value, " \t\r\n,.;:-/|*")
		value = trimLeadingInlinePassengerConjunction(value)
		value = passengerInlineNamePrefix.ReplaceAllString(value, "")
		if value == previous {
			return value
		}
	}
}

func trimLeadingInlinePassengerConjunction(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n,;:/|*")
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	first := fields[0]
	foldedFirst := strings.Join(strings.Fields(foldChatText(strings.Trim(first, " \t\r\n,;:/|*"))), " ")
	if foldedFirst != "e" {
		return trimmed
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, first))
	return strings.TrimLeft(rest, " \t\r\n,.;:-/|*")
}

func normalizePassengerNameFragment(value string) string {
	value = cleanInlinePassengerName(value)
	if value == "" {
		return ""
	}
	tokens := strings.Fields(value)
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.Trim(token, " \t\r\n,.;:-/|*")
		if token == "" || !containsPassengerNameLetter(token) {
			continue
		}
		kept = append(kept, token)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, " ")
}

func containsPassengerNameLetter(value string) bool {
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= 'À' && char <= 'ÿ') {
			return true
		}
	}
	return false
}

func extractBookingCreatePassengersByLines(text string, session Session) []BookingCreatePassengerInput {
	segments := splitPassengerSegments(text)
	if len(segments) == 0 {
		return nil
	}

	passengers := make([]BookingCreatePassengerInput, 0, len(segments))
	for _, segment := range segments {
		cleanSegment, lapChildLabelKnown, isLapChild := stripPassengerLapChildLabel(segment)
		if match := passengerLooseCPFLinePattern.FindStringSubmatch(segment); len(match) == 3 {
			name := normalizePassengerName(match[1])
			document := normalizeDigits(match[2])
			if name == "" || !isValidCPF(document) {
				return nil
			}
			passenger := BookingCreatePassengerInput{
				Name:         name,
				Document:     document,
				DocumentType: "CPF",
				CPF:          document,
				Phone:        strings.TrimSpace(session.CustomerPhone),
				IsLapChild:   lapChildLabelKnown && isLapChild,
			}
			passengers = append(passengers, enrichPassengerAdditionalIdentityFromText(passenger, segment))
			continue
		}
		if match := passengerLooseCPFLinePattern.FindStringSubmatch(cleanSegment); len(match) == 3 {
			name := normalizePassengerName(match[1])
			document := normalizeDigits(match[2])
			if name == "" || !isValidCPF(document) {
				return nil
			}
			passenger := BookingCreatePassengerInput{
				Name:         name,
				Document:     document,
				DocumentType: "CPF",
				CPF:          document,
				Phone:        strings.TrimSpace(session.CustomerPhone),
				IsLapChild:   lapChildLabelKnown && isLapChild,
			}
			passengers = append(passengers, enrichPassengerAdditionalIdentityFromText(passenger, cleanSegment))
			continue
		}

		if passenger, ok := parseLooseTypedPassengerDocument(cleanSegment, session); ok {
			if lapChildLabelKnown {
				passenger.IsLapChild = isLapChild
			}
			passengers = append(passengers, enrichPassengerAdditionalIdentityFromText(passenger, cleanSegment))
			continue
		}

		if passenger, ok := parseStructuredPassengerLine(cleanSegment, session); ok {
			if lapChildLabelKnown {
				passenger.IsLapChild = isLapChild
			}
			passengers = append(passengers, enrichPassengerAdditionalIdentityFromText(passenger, cleanSegment))
			continue
		}
	}
	if len(passengers) == 0 {
		return nil
	}
	return passengers
}

func parseLooseTypedPassengerDocument(segment string, session Session) (BookingCreatePassengerInput, bool) {
	match := passengerLooseTypedLinePattern.FindStringSubmatch(segment)
	if len(match) != 4 {
		return BookingCreatePassengerInput{}, false
	}
	name := normalizePassengerName(match[1])
	documentType := normalizePassengerDocumentType(match[2])
	document := normalizePassengerDocumentValue(match[3], documentType)
	if name == "" || documentType == "" || document == "" {
		return BookingCreatePassengerInput{}, false
	}
	passenger := BookingCreatePassengerInput{
		Name:         name,
		Document:     document,
		DocumentType: documentType,
		Phone:        strings.TrimSpace(session.CustomerPhone),
	}
	return enrichPassengerAdditionalIdentityFromText(passenger, segment), true
}

func stripPassengerLapChildLabel(segment string) (string, bool, bool) {
	trimmed := strings.TrimSpace(segment)
	trimmed = strings.TrimLeft(trimmed, "-* ")
	if idx := strings.Index(trimmed, "."); idx > 0 && idx <= 3 {
		if readInt(strings.TrimSpace(trimmed[:idx])) > 0 {
			trimmed = strings.TrimSpace(trimmed[idx+1:])
		}
	}
	idx := strings.Index(trimmed, ":")
	if idx < 0 {
		return segment, false, false
	}
	label := strings.Join(strings.Fields(foldChatText(trimmed[:idx])), " ")
	value := strings.TrimSpace(trimmed[idx+1:])
	switch label {
	case "crianca", "filho", "filha", "menino", "menina", "bebe":
		return value, true, true
	case "adulto", "adulta", "mae", "pai", "responsavel":
		return value, true, false
	default:
		return segment, false, false
	}
}

func parseStructuredPassengerLine(segment string, session Session) (BookingCreatePassengerInput, bool) {
	if !strings.Contains(segment, "|") {
		return BookingCreatePassengerInput{}, false
	}

	parts := strings.Split(segment, "|")
	if len(parts) < 3 {
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

	return enrichPassengerAdditionalIdentityFromText(BookingCreatePassengerInput{
		Name:         name,
		Document:     document,
		DocumentType: documentType,
		Phone:        strings.TrimSpace(session.CustomerPhone),
	}, segment), true
}

func enrichPassengerAdditionalIdentityFromText(passenger BookingCreatePassengerInput, text string) BookingCreatePassengerInput {
	passenger.CPF = firstNonEmpty(
		normalizePassengerDocumentValue(passenger.CPF, "CPF"),
		passengerCPFForPrimaryDocument(passenger),
		extractValidCPF(text),
	)
	passenger.RG = firstNonEmpty(
		normalizePassengerDocumentValue(passenger.RG, "RG"),
		passengerDocumentForPrimaryType(passenger, "RG"),
		extractPassengerRG(text),
	)
	passenger.CNH = firstNonEmpty(
		normalizePassengerDocumentValue(passenger.CNH, "CNH"),
		passengerDocumentForPrimaryType(passenger, "CNH"),
		extractPassengerCNH(text),
	)
	if birthDate := extractPassengerBirthDate(text); birthDate != "" {
		passenger.BirthDate = birthDate
	}
	if birthRecord := normalizePassengerDocumentValue(firstNonEmpty(passenger.BirthCertificateNumber, extractPassengerBirthCertificateNumber(text)), "CERTIDAO_NASCIMENTO"); birthRecord != "" {
		passenger.BirthCertificateNumber = birthRecord
	}
	if strings.EqualFold(passenger.DocumentType, "CERTIDAO_NASCIMENTO") && passenger.BirthCertificateNumber == "" {
		passenger.BirthCertificateNumber = normalizePassengerDocumentValue(passenger.Document, "CERTIDAO_NASCIMENTO")
	}
	if city := normalizePassengerBirthCity(firstNonEmpty(passenger.BirthCity, extractPassengerBirthCity(text))); city != "" {
		passenger.BirthCity = city
	}
	return passenger
}

func passengerCPFForPrimaryDocument(passenger BookingCreatePassengerInput) string {
	if !strings.EqualFold(strings.TrimSpace(passenger.DocumentType), "CPF") {
		return ""
	}
	return normalizePassengerDocumentValue(passenger.Document, "CPF")
}

func passengerDocumentForPrimaryType(passenger BookingCreatePassengerInput, documentType string) string {
	if !strings.EqualFold(strings.TrimSpace(passenger.DocumentType), documentType) {
		return ""
	}
	return normalizePassengerDocumentValue(passenger.Document, documentType)
}

func extractPassengerRG(text string) string {
	if match := passengerRGPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		return normalizePassengerDocumentValue(match[1], "RG")
	}
	return ""
}

func extractPassengerCNH(text string) string {
	if match := passengerCNHPattern.FindStringSubmatch(strings.ToUpper(text)); len(match) == 2 {
		return normalizePassengerDocumentValue(match[1], "CNH")
	}
	return ""
}

func extractPassengerBirthDate(text string) string {
	match := passengerBirthDatePattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return ""
	}
	if parsed, ok := parseFlexibleDate(match[1]); ok {
		return parsed.Format("2006-01-02")
	}
	return strings.TrimSpace(match[1])
}

func extractPassengerBirthCertificateNumber(text string) string {
	match := passengerBirthRecordPattern.FindStringSubmatch(strings.ToUpper(text))
	if len(match) != 2 {
		return ""
	}
	return normalizeBirthCertificateNumberFromText(match[1])
}

func extractPassengerBirthCity(text string) string {
	match := passengerBirthCityPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return ""
	}
	return normalizePassengerBirthCity(match[1])
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
	candidates := typedPassengerDocumentCandidates(text)
	if len(candidates) == 0 {
		return "", ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Start < candidates[j].Start
	})
	return candidates[0].Document, candidates[0].DocumentType
}

type typedPassengerDocumentCandidate struct {
	Start        int
	Document     string
	DocumentType string
}

func typedPassengerDocumentCandidates(text string) []typedPassengerDocumentCandidate {
	body := strings.ToUpper(text)
	candidates := make([]typedPassengerDocumentCandidate, 0, 4)
	for _, item := range []struct {
		DocumentType string
		Pattern      *regexp.Regexp
	}{
		{"CPF", passengerCPFPattern},
		{"RG", passengerRGPattern},
		{"CNH", passengerCNHPattern},
		{"CERTIDAO_NASCIMENTO", passengerBirthRecordPattern},
	} {
		match := item.Pattern.FindStringSubmatchIndex(body)
		if len(match) < 4 || match[2] < 0 || match[3] < 0 {
			continue
		}
		document := normalizePassengerDocumentValue(body[match[2]:match[3]], item.DocumentType)
		if document == "" {
			continue
		}
		candidates = append(candidates, typedPassengerDocumentCandidate{
			Start:        match[0],
			Document:     document,
			DocumentType: item.DocumentType,
		})
	}
	return candidates
}

func looksLikePassengerDocumentText(text string, session Session) bool {
	if len(extractBookingCreatePassengers(text, session)) > 0 {
		return true
	}
	if _, ok := parsePassengerDocumentCorrection(text); ok {
		return true
	}
	if extractValidCPF(text) != "" {
		return true
	}
	return looksLikeInvalidPassengerCPF(text)
}

func looksLikeInvalidPassengerCPF(text string) bool {
	body := strings.TrimSpace(text)
	if body == "" || extractValidCPF(body) != "" {
		return false
	}
	if match := passengerCPFPattern.FindStringSubmatch(strings.ToUpper(body)); len(match) == 2 && len(normalizeDigits(match[1])) == 11 {
		return true
	}
	if match := passengerLooseCPFLinePattern.FindStringSubmatch(body); len(match) == 3 && len(normalizeDigits(match[2])) == 11 {
		return true
	}
	if isBareDocumentDigits(body) && len(normalizeDigits(body)) == 11 {
		return true
	}
	return false
}

func isBareDocumentDigits(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	for _, char := range trimmed {
		if (char >= '0' && char <= '9') || char == '.' || char == '-' || char == ' ' || char == '\t' || char == '\n' {
			continue
		}
		return false
	}
	return true
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
		if qty := inferPassengerQuantityFromFreeText(messageTurnText(history[i])); qty > 0 {
			return qty
		}
	}
	for i := len(history) - 1; i >= 0; i-- {
		if qty := inferPassengerQuantityFromFreeText(messageTurnText(history[i])); qty > 0 {
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

func lastBotAskedRouteAndPassengerCollection(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		body := strings.TrimSpace(message.Body)
		if body == "" {
			continue
		}
		return looksLikePassengerCountQuestion(body) && looksLikeRouteCollectionPrompt(body)
	}
	return false
}

func looksLikeRouteCollectionPrompt(text string) bool {
	if isBookingRegressionDraftText(text) {
		return true
	}

	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	hasDateCue := strings.Contains(folded, " data") ||
		strings.Contains(folded, " quando") ||
		strings.Contains(folded, " dia ")
	hasRouteCue := strings.Contains(folded, " origem") ||
		strings.Contains(folded, " destino") ||
		strings.Contains(folded, " de qual cidade") ||
		strings.Contains(folded, " para qual cidade") ||
		strings.Contains(folded, " cidade de saida") ||
		strings.Contains(folded, " cidade voce sai")
	return hasDateCue && hasRouteCue
}

func looksLikePassengerCountQuestion(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	if looksLikePassengerDocumentRequest(folded) {
		return false
	}
	if strings.Contains(folded, "consegui identificar estes dados") ||
		strings.Contains(folded, "eles conferem") ||
		strings.Contains(folded, "posso prosseguir e criar a reserva") {
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
	folded := strings.Join(strings.Fields(foldChatText(NormalizeIncomingCustomerText(currentTurn))), " ")
	if folded == "" {
		return PassengerClarificationSlots{}
	}

	slots := PassengerClarificationSlots{}

	if match := passengerMePlusDigitQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
		if extra := passengerDigitNumber(match[1]); extra > 0 {
			slots.PassengerCount = extra + 1
			slots.PassengerCountKnown = true
		}
	}

	if !slots.PassengerCountKnown {
		if match := passengerMePlusWordQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
			if extra := passengerWordNumber(match[1]); extra > 0 {
				slots.PassengerCount = extra + 1
				slots.PassengerCountKnown = true
			}
		}
	}

	if !slots.PassengerCountKnown {
		switch {
		case containsAnyFolded(
			folded,
			"eu e mais uma pessoa",
			"eu e mais uma",
			"eu e mais um passageiro",
			"eu e mais um acompanhante",
			"eu e outra pessoa",
			"eu e outra",
			"eu e minha",
			"eu e meu",
			"eu e minha esposa",
			"eu e meu esposo",
			"eu e minha filha",
			"eu e meu filho",
			"eu e minha mulher",
			"eu e meu marido",

			// novos casos
			"pra mim e pra minha",
			"pra mim e pro meu",
			"pra mim e para minha",
			"pra mim e para meu",
			"pra mim e minha",
			"pra mim e meu",
			"pra mim e pra minha filha",
			"pra mim e pro meu filho",
			"pra mim e minha filha",
			"pra mim e meu filho",
			"para mim e para minha",
			"para mim e para meu",
			"para mim e minha",
			"para mim e meu",
			"para mim e minha filha",
			"para mim e meu filho",
		):
			slots.PassengerCount = 2
			slots.PassengerCountKnown = true
		case isSoloPassengerReply(folded):
			slots.PassengerCount = 1
			slots.PassengerCountKnown = true
		case containsAnyFolded(folded, "uma pessoa", "1 pessoa", "um passageiro"):
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
	}

	if !slots.PassengerCountKnown {
		if match := passengerDigitQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
			if qty := passengerDigitNumber(match[1]); qty > 0 {
				slots.PassengerCount = qty
				slots.PassengerCountKnown = true
			}
		}
	}

	if !slots.PassengerCountKnown {
		if match := passengerWordQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
			slots.PassengerCount = passengerWordNumber(match[1])
			slots.PassengerCountKnown = slots.PassengerCount > 0
		}
	}

	if !slots.PassengerCountKnown {
		if match := passengerSomosEmQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
			if qty := passengerDigitNumber(match[1]); qty > 0 {
				slots.PassengerCount = qty
				slots.PassengerCountKnown = true
			}
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
			"nao tem crianca de 5 anos",
			"não tem criança de 5 anos",
			"nao tem crianca nao",
			"não tem criança não",
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

func isSoloPassengerReply(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	switch strings.TrimSpace(folded) {
	case "eu",
		"pra mim",
		"para mim",
		"so eu",
		"somente eu",
		"sou eu",
		"sou so eu",
		"sozinho",
		"eu sozinho",
		"eu vou sozinho",
		"vou sozinho",
		"vou so",
		"so pra mim",
		"so para mim",
		"so eu mesmo",
		"so pra mim mesmo",
		"so para mim mesmo",
		"pra mim mesmo",
		"para mim mesmo",
		"e so pra mim",
		"eh so pra mim",
		"e so para mim",
		"eh so para mim",
		"passagem so pra mim",
		"passagem so para mim",
		"e so eu",
		"eh so eu":
		return true
	}

	if hasAdditionalPassengerCue(folded) {
		return false
	}

	return soloPassengerReplyPattern.MatchString(folded)
}

func hasAdditionalPassengerCue(folded string) bool {
	return containsAnyFolded(
		folded,
		"e mais",
		"mais uma pessoa",
		"mais um passageiro",
		"outra pessoa",
		"outro passageiro",
		"acompanhante",
		"duas pessoas",
		"dois passageiros",
		"2 pessoas",
		"2 passageiros",
		"minha esposa",
		"meu esposo",
		"minha filha",
		"meu filho",
		"minha mulher",
		"meu marido",
	)
}

func passengerWordNumber(value string) int {
	switch strings.TrimSpace(value) {
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
	default:
		return 0
	}
}

func passengerDigitNumber(value string) int {
	value = strings.TrimSpace(value)
	if len(value) != 1 || value[0] < '1' || value[0] > '9' {
		return 0
	}
	return int(value[0] - '0')
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

func buildRGIssuerSuffixes() []string {
	suffixes := make([]string, 0, len(rgIssuerTokens)*(len(brazilianUFs)+1))
	for _, issuer := range rgIssuerTokens {
		for _, uf := range brazilianUFs {
			suffixes = append(suffixes, issuer+uf)
		}
	}
	suffixes = append(suffixes, rgIssuerTokens...)
	return suffixes
}

func normalizeRGDocument(value string) string {
	document := normalizeAlphaNumeric(value)
	for {
		changed := false
		for _, suffix := range rgIssuerSuffixes {
			if strings.HasSuffix(document, suffix) {
				document = strings.TrimSuffix(document, suffix)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	if len(document) < 4 {
		return ""
	}
	return document
}

func maskDocumentForLog(doc string) string {
	digits := normalizeDigits(doc)
	if len(digits) < 5 {
		return "[documento]"
	}
	prefix := digits
	if len(prefix) > 3 {
		prefix = prefix[:3]
	}
	return prefix + "*******" + digits[len(digits)-2:]
}

func maskDocumentForDisplay(document string, documentType string) string {
	if !strings.EqualFold(strings.TrimSpace(documentType), "CPF") {
		return strings.TrimSpace(document)
	}
	digits := normalizeDigits(document)
	if len(digits) != 11 {
		return strings.TrimSpace(document)
	}
	return digits[:3] + ".***.***-" + digits[9:]
}

func looksLikeBareCPF(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	for _, char := range trimmed {
		if (char >= '0' && char <= '9') || char == '.' || char == '-' || char == ' ' || char == '\t' || char == '\n' {
			continue
		}
		return false
	}
	return isValidCPF(normalizeDigits(trimmed))
}

func findLatestSelectedOptionIndex(history []Message) int {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") &&
			previousAssistantAskedLapChildAssignment(history, i) {
			continue
		}
		body := strings.TrimSpace(messageTurnText(history[i]))
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
	return findLatestPassengerDocumentProgress(history, session).SourceText
}

func findLatestPassengerDocumentProgress(history []Message, session Session) bookingPassengerDocumentProgress {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		body := strings.TrimSpace(messageTurnText(message))
		if body == "" || looksLikeBookingCreateConfirmation(body) {
			continue
		}
		if looksLikePassengerDocumentCorrectionInstruction(body) {
			continue
		}
		progress := extractBookingPassengerDocumentProgress(body, session)
		if len(progress.Passengers) > 0 || len(progress.Partials) > 0 {
			return mergePassengerDocumentProgressWithPriorPartial(history[:i], session, progress)
		}
	}
	return bookingPassengerDocumentProgress{}
}

func mergePassengerDocumentProgressWithPriorPartial(history []Message, session Session, latest bookingPassengerDocumentProgress) bookingPassengerDocumentProgress {
	if len(latest.Passengers) == 0 {
		return latest
	}
	for i := len(history) - 1; i >= 0; i-- {
		if messageHasBookingCreateContext(history[i]) {
			break
		}
		body := strings.TrimSpace(messageTurnText(history[i]))
		if body == "" || looksLikeBookingCreateConfirmation(body) || looksLikePassengerDocumentCorrectionInstruction(body) {
			continue
		}
		prior := extractBookingPassengerDocumentProgress(body, session)
		if len(prior.Partials) == 0 {
			continue
		}
		if !passengerProgressCompletesPartial(latest, prior.Partials) {
			continue
		}

		merged := bookingPassengerDocumentProgress{
			Passengers: mergePassengerDocumentsInPriorOrder(prior, latest),
			Partials:   unresolvedPassengerDocumentPartials(prior.Partials, latest),
			SourceText: strings.TrimSpace(strings.TrimSpace(prior.SourceText) + "\n" + strings.TrimSpace(latest.SourceText)),
		}
		merged.Partials = append(merged.Partials, latest.Partials...)
		return merged
	}
	return latest
}

func messageHasBookingCreateContext(message Message) bool {
	for _, toolContext := range messageToolContexts(message) {
		if booking := asMap(toolContext[toolNameBookingCreate]); len(booking) > 0 {
			return true
		}
	}
	return false
}

func passengerProgressCompletesPartial(progress bookingPassengerDocumentProgress, partials []BookingPassengerDocumentPartial) bool {
	for _, partial := range partials {
		for _, passenger := range progress.Passengers {
			if passengerMatchesDocumentPartial(passenger, partial) {
				return true
			}
		}
	}
	return false
}

func unresolvedPassengerDocumentPartials(partials []BookingPassengerDocumentPartial, progress bookingPassengerDocumentProgress) []BookingPassengerDocumentPartial {
	unresolved := make([]BookingPassengerDocumentPartial, 0, len(partials))
	for _, partial := range partials {
		resolved := false
		for _, passenger := range progress.Passengers {
			if passengerMatchesDocumentPartial(passenger, partial) {
				resolved = true
				break
			}
		}
		if !resolved {
			unresolved = append(unresolved, partial)
		}
	}
	return unresolved
}

func mergePassengerDocumentsInPriorOrder(prior bookingPassengerDocumentProgress, latest bookingPassengerDocumentProgress) []BookingCreatePassengerInput {
	candidates := append([]BookingCreatePassengerInput{}, prior.Passengers...)
	candidates = append(candidates, latest.Passengers...)
	byDocument := map[string]BookingCreatePassengerInput{}
	for _, passenger := range candidates {
		if key := bookingPassengerDocumentKey(passenger); key != "" {
			byDocument[key] = passenger
		}
	}

	merged := make([]BookingCreatePassengerInput, 0, len(candidates))
	seen := map[string]bool{}
	appendPassenger := func(passenger BookingCreatePassengerInput) {
		key := bookingPassengerDocumentKey(passenger)
		if key == "" {
			key = strings.Join(strings.Fields(foldChatText(passenger.Name)), " ")
		}
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		merged = append(merged, passenger)
	}

	for _, match := range inlinePassengerCPFMatches(prior.SourceText) {
		if passenger, ok := byDocument["CPF:"+match.Document]; ok {
			appendPassenger(passenger)
		}
	}
	for _, passenger := range prior.Passengers {
		appendPassenger(passenger)
	}
	for _, passenger := range latest.Passengers {
		appendPassenger(passenger)
	}
	return merged
}

func passengerMatchesDocumentPartial(passenger BookingCreatePassengerInput, partial BookingPassengerDocumentPartial) bool {
	partialKey := partialPassengerDocumentKey(partial)
	if partialKey == "" || bookingPassengerDocumentKey(passenger) != partialKey {
		return false
	}
	fragment := strings.Join(strings.Fields(foldChatText(partial.NameFragment)), " ")
	if fragment == "" {
		return true
	}
	name := strings.Join(strings.Fields(foldChatText(passenger.Name)), " ")
	return strings.Contains(name, fragment)
}

func partialPassengerDocumentKey(partial BookingPassengerDocumentPartial) string {
	docType := normalizePassengerDocumentType(partial.DocumentType)
	document := normalizePassengerDocumentValue(partial.Document, docType)
	if docType == "" || document == "" {
		return ""
	}
	return docType + ":" + document
}

func bookingPassengerDocumentKey(passenger BookingCreatePassengerInput) string {
	for _, candidate := range []struct {
		Type  string
		Value string
	}{
		{normalizePassengerDocumentType(passenger.DocumentType), passenger.Document},
		{"CPF", passenger.CPF},
		{"RG", passenger.RG},
		{"CNH", passenger.CNH},
		{"CERTIDAO_NASCIMENTO", passenger.BirthCertificateNumber},
	} {
		docType := normalizePassengerDocumentType(candidate.Type)
		document := normalizePassengerDocumentValue(candidate.Value, docType)
		if docType != "" && document != "" {
			return docType + ":" + document
		}
	}
	return ""
}

func looksLikePassengerDocumentCorrectionInstruction(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	phrases := []string{
		"nome esta certo",
		"nome ta certo",
		"nome correto",
		"nome esta correto",
		"quero que use",
		"usar esse documento",
		"use esse documento",
		"documento correto",
		"documento certo",
		"cpf correto",
		"corrigir cpf",
		"corrigir o cpf",
		"corrige cpf",
		"corrige o cpf",
		"corrigir documento",
		"corrigir o documento",
	}
	return containsFoldedPhrase(folded, phrases)
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

func lapChildIndexesFromPassengers(passengers []BookingCreatePassengerInput) []int {
	indexes := []int{}
	for i, passenger := range passengers {
		if passenger.IsLapChild {
			indexes = append(indexes, i+1)
		}
	}
	return indexes
}

func applyLapChildPassengerIndexes(passengers []BookingCreatePassengerInput, indexes []int) {
	for i := range passengers {
		passengers[i].IsLapChild = false
	}
	for _, index := range indexes {
		zeroBased := index - 1
		if zeroBased >= 0 && zeroBased < len(passengers) {
			passengers[zeroBased].IsLapChild = true
		}
	}
}

func countLapChildPassengers(passengers []BookingCreatePassengerInput) int {
	count := 0
	for _, passenger := range passengers {
		if passenger.IsLapChild {
			count++
		}
	}
	return count
}

func hasExpectedLapChildCount(passengers []BookingCreatePassengerInput, expected int) bool {
	if expected <= 0 {
		return true
	}
	return countLapChildPassengers(passengers) == expected
}

func inferLapChildAssignmentIndexes(history []Message, currentTurn string, passengers []BookingCreatePassengerInput, expected int) ([]int, bool) {
	if expected <= 0 || len(passengers) == 0 {
		return nil, false
	}
	if indexes := lapChildIndexesFromPassengers(passengers); len(indexes) == expected {
		return indexes, true
	}

	if lastAssistantAskedLapChildAssignment(history) {
		if indexes, ok := parseLapChildAssignmentAnswer(currentTurn, passengers, expected); ok {
			return indexes, true
		}
	}

	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		body := strings.Join(strings.Fields(foldChatText(history[i].Body)), " ")
		if !strings.Contains(body, "qual deles e a crianca") && !strings.Contains(body, "qual passageiro e a crianca") {
			continue
		}
		for j := i + 1; j < len(history); j++ {
			if strings.EqualFold(strings.TrimSpace(history[j].Direction), "INBOUND") {
				if indexes, ok := parseLapChildAssignmentAnswer(history[j].Body, passengers, expected); ok {
					return indexes, true
				}
			}
		}
		break
	}
	return nil, false
}

func parseLapChildAssignmentAnswer(text string, passengers []BookingCreatePassengerInput, expected int) ([]int, bool) {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" || expected <= 0 {
		return nil, false
	}

	indexes := []int{}
	seen := map[int]bool{}
	add := func(index int) {
		if index <= 0 || index > len(passengers) || seen[index] {
			return
		}
		seen[index] = true
		indexes = append(indexes, index)
	}

	for _, field := range strings.Fields(folded) {
		switch field {
		case "1", "01", "primeiro", "primeira":
			add(1)
		case "2", "02", "segundo", "segunda":
			add(2)
		case "3", "03", "terceiro", "terceira":
			add(3)
		case "4", "04", "quarto", "quarta":
			add(4)
		case "5", "05", "quinto", "quinta":
			add(5)
		}
	}
	if len(indexes) == expected {
		return indexes, true
	}

	for i, passenger := range passengers {
		name := strings.Join(strings.Fields(foldChatText(passenger.Name)), " ")
		if name == "" {
			continue
		}
		first := strings.Fields(name)[0]
		if folded == name || folded == first || strings.Contains(folded, name) || strings.Contains(folded, "o "+first) || strings.Contains(folded, "a "+first) {
			add(i + 1)
		}
	}
	if len(indexes) == expected {
		return indexes, true
	}
	return nil, false
}

func isLapChildFromBirthDate(birthDate string, tripDate string) bool {
	birthDate = strings.TrimSpace(birthDate)
	tripDate = strings.TrimSpace(tripDate)
	if birthDate == "" || tripDate == "" {
		return false
	}
	birth, ok := parseFlexibleDate(birthDate)
	if !ok {
		return false
	}
	trip, ok := parseFlexibleDate(tripDate)
	if !ok {
		return false
	}
	age := trip.Year() - birth.Year()
	if trip.Month() < birth.Month() || (trip.Month() == birth.Month() && trip.Day() < birth.Day()) {
		age--
	}
	return age >= 0 && age <= 5
}

func parseFlexibleDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", "02/01/2006", "02-01-2006", time.RFC3339, time.RFC3339Nano} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
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
		if isValidCPF(document) {
			return document
		}
		return ""
	case "RG":
		return normalizeRGDocument(value)
	case "CNH":
		return normalizeAlphaNumeric(value)
	case "CERTIDAO_NASCIMENTO":
		document := normalizeDigits(value)
		if len(document) == 32 {
			return document
		}
		return ""
	default:
		return ""
	}
}

func normalizePassengerBirthCity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	tokens := strings.Fields(value)
	city := make([]string, 0, len(tokens))
	for _, token := range tokens {
		folded := strings.Trim(strings.Join(strings.Fields(foldChatText(token)), " "), ".,;:-")
		switch folded {
		case "cpf", "rg", "cnh", "certidao", "matricula", "documento", "data", "nascimento", "nasc":
			return strings.TrimSpace(strings.Join(city, " "))
		}
		city = append(city, strings.Trim(token, ".,;:"))
	}
	return strings.TrimSpace(strings.Join(city, " "))
}

func normalizeBirthCertificateNumberFromText(value string) string {
	tokens := strings.Fields(strings.TrimSpace(value))
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		folded := strings.Trim(strings.Join(strings.Fields(foldChatText(token)), " "), ".,;:-")
		switch folded {
		case "cpf", "rg", "cnh", "certidao", "matricula", "documento", "data", "nascimento", "nasc", "naturalidade", "natural":
			return normalizePassengerDocumentValue(strings.Join(kept, ""), "CERTIDAO_NASCIMENTO")
		}
		kept = append(kept, token)
	}
	return normalizePassengerDocumentValue(strings.Join(kept, ""), "CERTIDAO_NASCIMENTO")
}

func isValidCPF(value string) bool {
	digits := normalizeDigits(value)
	if len(digits) != 11 {
		return false
	}
	allEqual := true
	for i := 1; i < len(digits); i++ {
		if digits[i] != digits[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return false
	}

	return cpfCheckDigit(digits[:9], 10) == int(digits[9]-'0') &&
		cpfCheckDigit(digits[:10], 11) == int(digits[10]-'0')
}

func cpfCheckDigit(digits string, weight int) int {
	sum := 0
	for _, char := range digits {
		sum += int(char-'0') * weight
		weight--
	}
	remainder := (sum * 10) % 11
	if remainder == 10 {
		return 0
	}
	return remainder
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
			"name":                     item.Name,
			"document":                 item.Document,
			"document_type":            item.DocumentType,
			"cpf":                      item.CPF,
			"rg":                       item.RG,
			"cnh":                      item.CNH,
			"birth_date":               item.BirthDate,
			"birth_certificate_number": item.BirthCertificateNumber,
			"birth_city":               item.BirthCity,
			"phone":                    item.Phone,
			"is_lap_child":             item.IsLapChild,
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
			"name":                     item.Name,
			"document":                 item.Document,
			"document_type":            item.DocumentType,
			"cpf":                      item.CPF,
			"rg":                       item.RG,
			"cnh":                      item.CNH,
			"birth_date":               item.BirthDate,
			"birth_certificate_number": item.BirthCertificateNumber,
			"birth_city":               item.BirthCity,
			"phone":                    item.Phone,
			"seat_id":                  item.SeatID,
			"is_lap_child":             item.IsLapChild,
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
