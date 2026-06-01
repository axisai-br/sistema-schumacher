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

	if !containsAll(reply, "Joao Vitor Messias | CPF | 06645648103", "passageiro faltante", "CPF ou RG") {
		t.Fatalf("unexpected reply: %q", reply)
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
