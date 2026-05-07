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
