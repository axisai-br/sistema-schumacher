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
			"cnh":"99999999999",
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
	if passenger.Confidence != 0.91 {
		t.Fatalf("expected normalized confidence 0.91, got %.2f", passenger.Confidence)
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
