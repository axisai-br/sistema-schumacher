package bookings

import "testing"

const testBirthCertificateNumber = "12345678901234567890123456789012"

func syntheticValidCPFForBookingTests() string {
	// Checksum-valid CPF generated for normalization tests only.
	return "84960815086"
}

func TestNormalizePassengersUsesPluralListWhenPresent(t *testing.T) {
	passengers := normalizePassengers(
		PassengerInput{Name: "Ignorado"},
		[]PassengerInput{
			{Name: " Passageiro Teste ", Document: " 123 ", CPF: " " + syntheticValidCPFForBookingTests() + " ", RG: " 123 ", CNH: " 99999999999 ", BirthDate: " 21-05-2022 ", BirthCertificateNumber: " " + testBirthCertificateNumber + " ", BirthCity: " Santa Ines ", Phone: " 1100 ", Email: " passageiro@example.invalid "},
			{Name: "Joao"},
		},
	)

	if len(passengers) != 2 {
		t.Fatalf("expected 2 passengers, got %d", len(passengers))
	}
	if passengers[0].Name != "Passageiro Teste" || passengers[0].Document != "123" || passengers[0].DocumentType != "RG" || passengers[0].Phone != "1100" || passengers[0].Email != "passageiro@example.invalid" {
		t.Fatalf("unexpected normalization result: %+v", passengers[0])
	}
	if passengers[0].CPF != syntheticValidCPFForBookingTests() || passengers[0].RG != "123" || passengers[0].CNH != "99999999999" || passengers[0].BirthDate != "2022-05-21" || passengers[0].BirthCertificateNumber != testBirthCertificateNumber || passengers[0].BirthCity != "Santa Ines" {
		t.Fatalf("expected additional identity fields preserved, got %+v", passengers[0])
	}
	if passengers[1].Name != "Joao" {
		t.Fatalf("unexpected second passenger: %+v", passengers[1])
	}
}

func TestNormalizePassengersFallsBackToSingularPassenger(t *testing.T) {
	passengers := normalizePassengers(
		PassengerInput{Name: " Ana ", Document: " 456 "},
		nil,
	)

	if len(passengers) != 1 {
		t.Fatalf("expected 1 passenger, got %d", len(passengers))
	}
	if passengers[0].Name != "Ana" || passengers[0].Document != "456" || passengers[0].DocumentType != "RG" {
		t.Fatalf("unexpected normalization result: %+v", passengers[0])
	}
}

func TestNormalizePassengersPreservesExplicitDocumentType(t *testing.T) {
	passengers := normalizePassengers(
		PassengerInput{},
		[]PassengerInput{
			{Name: "Maria", Document: "06645648105", DocumentType: "cpf"},
			{Name: "Joao", Document: "12.345.678-X", DocumentType: "rg"},
			{Name: "Paulo", Document: "12345678901", DocumentType: "cnh"},
			{Name: "Bebe", Document: "12345678901234567890123456789012", DocumentType: "matricula"},
		},
	)

	if passengers[0].DocumentType != "CPF" {
		t.Fatalf("expected CPF document type, got %+v", passengers[0])
	}
	if passengers[1].DocumentType != "RG" {
		t.Fatalf("expected RG document type, got %+v", passengers[1])
	}
	if passengers[2].DocumentType != "CNH" {
		t.Fatalf("expected CNH document type, got %+v", passengers[2])
	}
	if passengers[3].DocumentType != "CERTIDAO_NASCIMENTO" {
		t.Fatalf("expected CERTIDAO_NASCIMENTO document type, got %+v", passengers[3])
	}
}

func TestNormalizePassengersMirrorsFormattedCPFAsDigitsOnly(t *testing.T) {
	passengers := normalizePassengers(
		PassengerInput{},
		[]PassengerInput{
			{Name: "Passageiro Teste", Document: "849.608.150-86"},
			{Name: "Outro Teste", Document: "123.456.789-01", DocumentType: "CPF"},
		},
	)

	if len(passengers) != 2 {
		t.Fatalf("expected 2 passengers, got %d", len(passengers))
	}
	if passengers[0].DocumentType != "CPF" || passengers[0].Document != syntheticValidCPFForBookingTests() || passengers[0].CPF != syntheticValidCPFForBookingTests() {
		t.Fatalf("expected formatted CPF mirrored as digits only, got %+v", passengers[0])
	}
	if passengers[1].DocumentType != "CPF" || passengers[1].CPF != "" {
		t.Fatalf("expected invalid CPF not to be persisted as cpf, got %+v", passengers[1])
	}
}

func TestCanonicalPassengerDocumentTypeAcceptsCertidaoAliases(t *testing.T) {
	tests := map[string]string{
		"cpf":                    "CPF",
		"rg":                     "RG",
		"cnh":                    "CNH",
		"certidao":               "CERTIDAO_NASCIMENTO",
		"certidao de nascimento": "CERTIDAO_NASCIMENTO",
		"matricula":              "CERTIDAO_NASCIMENTO",
	}

	for input, expected := range tests {
		if got := canonicalPassengerDocumentType(input); got != expected {
			t.Fatalf("expected %q for %q, got %q", expected, input, got)
		}
	}
}

func TestNormalizePassengersPreservesExplicitItemsForValidation(t *testing.T) {
	passengers := normalizePassengers(
		PassengerInput{},
		[]PassengerInput{
			{},
			{Name: "Carlos"},
		},
	)

	if len(passengers) != 2 {
		t.Fatalf("expected 2 passengers, got %d", len(passengers))
	}
	if passengers[0].Name != "" {
		t.Fatalf("expected first passenger to remain empty for validation, got %+v", passengers[0])
	}
	if passengers[1].Name != "Carlos" {
		t.Fatalf("unexpected second passenger result: %+v", passengers[1])
	}
}

func TestNormalizePassengersPreservesLapChildMetadata(t *testing.T) {
	passengers := normalizePassengers(
		PassengerInput{},
		[]PassengerInput{
			{Name: "Adulto", Document: "06645648105", DocumentType: "CPF"},
			{Name: "Crianca", Document: "MG1234567", DocumentType: "RG", IsLapChild: true},
		},
	)

	if !passengers[1].IsLapChild {
		t.Fatalf("expected second passenger to be lap child, got %+v", passengers[1])
	}
	if passengers[1].Notes != lapChildPassengerNote {
		t.Fatalf("expected lap child note marker, got %+v", passengers[1])
	}
}

func TestCountChargeablePassengersIgnoresLapChild(t *testing.T) {
	count := countChargeablePassengers([]PassengerInput{
		{Name: "Adulto 1"},
		{Name: "Adulto 2"},
		{Name: "Crianca", IsLapChild: true},
	})

	if count != 2 {
		t.Fatalf("expected 2 chargeable passengers, got %d", count)
	}
}

func TestBookingDetailsWithPassengersKeepsFirstPassengerAlias(t *testing.T) {
	details := bookingDetailsWithPassengers(Booking{ID: "BK-1"}, []BookingPassenger{
		{ID: "PS-1", Name: "Maria"},
		{ID: "PS-2", Name: "Joao"},
	})

	if len(details.Passengers) != 2 {
		t.Fatalf("expected 2 passengers in response, got %d", len(details.Passengers))
	}
	if details.Passenger.ID != "PS-1" {
		t.Fatalf("expected first passenger alias, got %+v", details.Passenger)
	}
}
