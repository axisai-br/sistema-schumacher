package affiliate

import "testing"

func TestParseBalanceResponse(t *testing.T) {
	raw := []byte(`{
		"available": {"amount": 15000, "currency": "BRL"},
		"waiting_funds": {"amount": 2400, "currency": "BRL"},
		"transferred": {"amount": 9500, "currency": "BRL"}
	}`)

	out, err := parseBalanceResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.AvailableAmount != 15000 {
		t.Fatalf("expected available 15000, got %d", out.AvailableAmount)
	}
	if out.WaitingFundsAmount != 2400 {
		t.Fatalf("expected waiting funds 2400, got %d", out.WaitingFundsAmount)
	}
	if out.TransferredAmount != 9500 {
		t.Fatalf("expected transferred 9500, got %d", out.TransferredAmount)
	}
	if out.Currency != "BRL" {
		t.Fatalf("expected BRL, got %s", out.Currency)
	}
}
