package chat

import (
	"strings"
	"testing"
)

func TestParseDocumentExtractResultPrioritizesCPFOverOtherDocuments(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Joao Vitor Messias",
			"document_type":"RG",
			"document":"1234567",
			"cpf":"066.456.481-03",
			"confidence":91
		}]
	}`)

	if result.Mode != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.Name != "Joao Vitor Messias" {
		t.Fatalf("unexpected passenger name: %+v", passenger)
	}
	if passenger.DocumentType != "CPF" || passenger.Document != "06645648103" {
		t.Fatalf("expected CPF priority, got %+v", passenger)
	}
	if passenger.CPF != "06645648103" || passenger.RG != "1234567" {
		t.Fatalf("expected secondary documents preserved, got %+v", passenger)
	}
	if passenger.Confidence != 0.91 {
		t.Fatalf("expected normalized confidence 0.91, got %.2f", passenger.Confidence)
	}
}

func TestParseDocumentExtractResultCNHePrioritizesVisibleCPFAndCleansRG(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"CLAUDECIR SCHUMACHER",
			"document_type":"RG",
			"document":"2817314 SSP SC",
			"cpf":"845.617.189-15",
			"cnh":"01235234139",
			"rg":"2817314 SSP SC",
			"birth_date":"",
			"confidence":0.9
		}],
		"failure_reason":""
	}`)

	if result.Mode != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "CPF" || passenger.Document != "84561718915" || passenger.CPF != "84561718915" {
		t.Fatalf("expected visible CPF as primary document, got %+v", passenger)
	}
	if passenger.CNH != "01235234139" {
		t.Fatalf("expected CNH preserved as secondary document, got %+v", passenger)
	}
	if passenger.RG != "2817314" {
		t.Fatalf("expected RG without issuer suffix, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultPrioritizesCPFWhenModelKeepsRGAsDocument(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Nome Sobrenome",
			"document_type":"RG",
			"document":"2873144",
			"cpf":"066.456.481-03",
			"rg":"2873144",
			"cnh":"",
			"birth_date":"",
			"confidence":0.93
		}],
		"failure_reason":""
	}`)

	if result.Mode != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "CPF" || passenger.Document != "06645648103" || passenger.CPF != "06645648103" {
		t.Fatalf("expected CPF to win over model RG document, got %+v", passenger)
	}
	if passenger.RG != "2873144" {
		t.Fatalf("expected RG preserved as secondary document, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultCNHeWithoutValidCPFUsesLegibleCNH(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Nome Sobrenome",
			"document_type":"CNH",
			"document":"01235234139",
			"cpf":"",
			"cnh":"01235234139",
			"rg":"",
			"birth_date":"",
			"confidence":0.91
		}],
		"failure_reason":""
	}`)

	if result.Mode != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED mode for legible CNH without CPF, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "CNH" || passenger.Document != "01235234139" || passenger.CPF != "" {
		t.Fatalf("expected CNH without invented CPF, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultInvalidCPFNeedsReview(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Nome Sobrenome",
			"document_type":"CPF",
			"document":"12345678901",
			"cpf":"12345678901",
			"cnh":"",
			"rg":"",
			"birth_date":"",
			"confidence":0.94
		}],
		"failure_reason":""
	}`)

	if result.Mode != "PARTIAL" {
		t.Fatalf("expected PARTIAL for invalid CPF, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected passenger retained for correction, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType == "CPF" && passenger.Document != "" {
		t.Fatalf("invalid CPF must not be accepted as primary document, got %+v", passenger)
	}
	if passenger.CPF != "" {
		t.Fatalf("invalid CPF must not be normalized into cpf field, got %+v", passenger)
	}
}

func TestDocumentExtractResponsePayloadPreservesSecondaryDocuments(t *testing.T) {
	payload := buildDocumentExtractResponsePayload(DocumentExtractResult{
		Mode: "EXTRACTED",
		Passengers: []DocumentExtractPassenger{
			{
				Name:         "Claudecir Schumacher",
				DocumentType: "CPF",
				Document:     "06645648103",
				CPF:          "06645648103",
				CNH:          "99999999999",
				BirthDate:    "1970-01-02",
				Confidence:   0.92,
			},
		},
	})

	passengers := asInterfaceSliceMaps(payload["passengers"])
	if len(passengers) != 1 {
		t.Fatalf("expected one passenger in payload, got %+v", payload)
	}
	if passengers[0]["document_type"] != "CPF" || passengers[0]["document"] != "06645648103" {
		t.Fatalf("expected CPF as primary document in payload, got %+v", passengers[0])
	}
	if passengers[0]["cpf"] != "06645648103" || passengers[0]["cnh"] != "99999999999" {
		t.Fatalf("expected secondary documents in payload, got %+v", passengers[0])
	}

	parsed := parseDocumentExtractContextPayload(payload)
	if len(parsed.Passengers) != 1 {
		t.Fatalf("expected one parsed passenger, got %+v", parsed.Passengers)
	}
	if parsed.Passengers[0].DocumentType != "CPF" || parsed.Passengers[0].Document != "06645648103" || parsed.Passengers[0].CNH != "99999999999" {
		t.Fatalf("expected context parser to preserve CPF primary and CNH secondary, got %+v", parsed.Passengers[0])
	}
}

func TestParseDocumentExtractResultUsesCPFAndPreservesCNHFromGenericDocument(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Claudecir Schumacher",
			"document_type":"CNH",
			"document":"99999999999",
			"cpf":"066.456.481-03",
			"birth_date":"1970-01-02",
			"confidence":0.92
		}]
	}`)

	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "CPF" || passenger.Document != "06645648103" || passenger.CPF != "06645648103" {
		t.Fatalf("expected visible CPF as primary document, got %+v", passenger)
	}
	if passenger.CNH != "99999999999" {
		t.Fatalf("expected CNH from generic document preserved, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultUsesCPFWhenDocumentTypeIsImplicit(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Joao Vitor Messias",
			"document":"066.456.481-03",
			"rg":"1234567",
			"confidence":0.9
		}]
	}`)

	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "CPF" || passenger.Document != "06645648103" {
		t.Fatalf("expected CPF priority for implicit document, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultCleansRGIssuerSuffix(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Claudecir Schumacher",
			"document_type":"RG",
			"document":"2817314SSPSC",
			"confidence":0.9
		}]
	}`)

	if result.Mode != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "RG" || passenger.Document != "2817314" {
		t.Fatalf("expected cleaned RG as primary document, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultMarksPartialForIssuerOnlyRG(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Claudecir Schumacher",
			"document_type":"RG",
			"document":"SSP/SC",
			"confidence":0.9
		}]
	}`)

	if result.Mode != "PARTIAL" {
		t.Fatalf("expected PARTIAL mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger for review, got %+v", result.Passengers)
	}
	if result.Passengers[0].Document != "" {
		t.Fatalf("expected issuer-only RG to be blank after normalization, got %+v", result.Passengers[0])
	}
}

func TestParseDocumentExtractResultAllowsCNHWithoutCPF(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Claudecir Schumacher",
			"document_type":"CNH",
			"document":"01235234139",
			"cnh":"01235234139",
			"confidence":0.9
		}]
	}`)

	if result.Mode != "EXTRACTED" {
		t.Fatalf("expected EXTRACTED mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "CNH" || passenger.Document != "01235234139" || passenger.CNH != "01235234139" {
		t.Fatalf("expected legible CNH as primary document, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultMarksPartialForLowConfidence(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Claudecir Schumacher",
			"document_type":"RG",
			"document":"2817314",
			"confidence":0.7
		}]
	}`)

	if result.Mode != "PARTIAL" {
		t.Fatalf("expected PARTIAL mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 || result.Passengers[0].Document != "2817314" {
		t.Fatalf("expected passenger data preserved for review, got %+v", result.Passengers)
	}
}

func TestParseDocumentExtractResultMarksPartialForInvalidCPF(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Claudecir Schumacher",
			"document_type":"CPF",
			"document":"123.456.789-01",
			"confidence":0.9
		}]
	}`)

	if result.Mode != "PARTIAL" {
		t.Fatalf("expected PARTIAL mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger for review, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "" || passenger.Document != "" || passenger.CPF != "" {
		t.Fatalf("expected invalid CPF not to become a trusted document, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultMarksPartialForInvalidCPFField(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Claudecir Schumacher",
			"document_type":"RG",
			"document":"2817314 SSP/SC",
			"cpf":"123.456.789-01",
			"confidence":0.9
		}]
	}`)

	if result.Mode != "PARTIAL" {
		t.Fatalf("expected PARTIAL mode, got %s", result.Mode)
	}
	passenger := result.Passengers[0]
	if passenger.DocumentType != "RG" || passenger.Document != "2817314" || passenger.CPF != "" {
		t.Fatalf("expected invalid CPF not to be promoted and RG to be cleaned, got %+v", passenger)
	}
}

func TestParseDocumentExtractResultKeepsBirthDate(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"Joao Vitor Messias",
			"document":"066.456.481-03",
			"birth_date":"21/05/2022",
			"confidence":0.9
		}]
	}`)

	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	if result.Passengers[0].BirthDate != "2022-05-21" {
		t.Fatalf("expected normalized birth date, got %+v", result.Passengers[0])
	}
	if !isLapChildFromBirthDate(result.Passengers[0].BirthDate, "2026-05-25") {
		t.Fatalf("expected birth date to identify lap child at trip date")
	}
}

func TestParseDocumentExtractResultMarksPartialWhenPassengerIsIncomplete(t *testing.T) {
	result := parseDocumentExtractResult(`{
		"mode":"LOW_CONFIDENCE",
		"passengers":[{
			"name":"",
			"document":"066.456.481-03",
			"confidence":0.7
		}]
	}`)

	if result.Mode != "PARTIAL" {
		t.Fatalf("expected PARTIAL mode, got %s", result.Mode)
	}
	if len(result.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", result.Passengers)
	}
	passenger := result.Passengers[0]
	if passenger.Name != "" {
		t.Fatalf("expected missing name for incomplete passenger, got %+v", passenger)
	}
	if passenger.DocumentType != "CPF" || passenger.Document != "06645648103" {
		t.Fatalf("expected normalized document, got %+v", passenger)
	}
}

func TestBuildDocumentExtractReplyAsksOnlyMissingPassengerDocuments(t *testing.T) {
	reply := buildDocumentExtractReply(DocumentExtractResult{
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "06645648103", Confidence: 0.9},
		},
	})

	if !containsAll(reply, "Joao Vitor Messias | CPF | 066.***.***-03", "passageiro faltante", "CPF ou RG") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestBuildDocumentExtractReplyForPartialDoesNotConfirmAsCertain(t *testing.T) {
	reply := buildDocumentExtractReply(DocumentExtractResult{
		Mode: "PARTIAL",
		Passengers: []DocumentExtractPassenger{
			{Name: "CLAUDECIR SCHUMACHER", DocumentType: "RG", Document: "2817314", CNH: "01235234139", Confidence: 0.7},
		},
	})

	if !containsAll(reply, "Consegui ler parte do documento", "Documento lido: RG 2817314", "envie o CPF do passageiro") {
		t.Fatalf("unexpected partial reply: %q", reply)
	}
	if strings.Contains(reply, "Eles conferem? Posso prosseguir") {
		t.Fatalf("partial reply must not confirm as certain: %q", reply)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
