package payments

import "testing"

func TestParseProviderData(t *testing.T) {
	raw := []byte(`{"data":{"url":"https://checkout.example/pix","pixQrCode":"000201abc"}}`)
	parsed, checkoutURL, pixCode := parseProviderData(raw)

	if parsed == nil {
		t.Fatalf("expected parsed provider payload")
	}
	if checkoutURL == nil || *checkoutURL != "https://checkout.example/pix" {
		t.Fatalf("unexpected checkout url: %#v", checkoutURL)
	}
	if pixCode == nil || *pixCode != "000201abc" {
		t.Fatalf("unexpected pix code: %#v", pixCode)
	}
}

func TestExtractCheckoutAndPix(t *testing.T) {
	checkoutURL, pixCode := ExtractCheckoutAndPix([]byte(`{"charges":[{"last_transaction":{"qr_code_url":"https://provider/pix","qr_code":"000201abc"}}]}`))

	if checkoutURL != "https://provider/pix" {
		t.Fatalf("unexpected checkout url: %q", checkoutURL)
	}
	if pixCode != "000201abc" {
		t.Fatalf("unexpected pix code: %q", pixCode)
	}
}

func TestBuildCustomerSynthesizesEmailWhenMissing(t *testing.T) {
	customer := BuildCustomer(&CustomerInput{
		Name:     "Joao Vitor Messias",
		Email:    "",
		Phone:    "554988709047",
		Document: "06645648103",
	}, "BK-C9A55BC8C67846F9B09EF5EFFC576A50")

	if customer == nil {
		t.Fatalf("expected customer payload")
	}
	if customer.Email != "reserva.bkc9a55bc8c67846f9b09ef5effc576a50@schumachertur.com" {
		t.Fatalf("unexpected fallback email: %q", customer.Email)
	}
}

func TestBuildCustomerRejectsInvalidDocument(t *testing.T) {
	customer := BuildCustomer(&CustomerInput{
		Name:     "Joao Vitor Messias",
		Document: "12345678901",
	}, "BK-C9A55BC8C67846F9B09EF5EFFC576A50")

	if customer != nil {
		t.Fatalf("expected invalid customer document to be rejected, got %#v", customer)
	}
}

func TestIsSupportedDocumentValidatesCPFAndCNPJ(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     bool
	}{
		{name: "valid cpf", document: "529.982.247-25", want: true},
		{name: "invalid cpf check digit", document: "12345678901", want: false},
		{name: "repeated cpf", document: "11111111111", want: false},
		{name: "valid cnpj", document: "11.222.333/0001-81", want: true},
		{name: "invalid cnpj check digit", document: "11222333000182", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSupportedDocument(tc.document); got != tc.want {
				t.Fatalf("IsSupportedDocument(%q) = %v, want %v", tc.document, got, tc.want)
			}
		})
	}
}

func TestBuildSingleRecipientSplit(t *testing.T) {
	split := BuildSingleRecipientSplit(" rp_123 ")

	if len(split) != 1 {
		t.Fatalf("expected one split rule, got %d", len(split))
	}
	rule := split[0]
	if rule.RecipientID != "rp_123" {
		t.Fatalf("unexpected recipient id: %q", rule.RecipientID)
	}
	if rule.Type != "percentage" || rule.Amount != 100 {
		t.Fatalf("unexpected split amount/type: %#v", rule)
	}
	if !rule.Options.Liable || !rule.Options.ChargeProcessingFee || !rule.Options.ChargeRemainderFee {
		t.Fatalf("expected recipient to be liable and receive remainder after fees: %#v", rule.Options)
	}
}
