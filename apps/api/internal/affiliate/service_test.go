package affiliate

import "testing"

func TestNormalizeTransferStatus(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "success", want: "SUCCESS"},
		{in: "paid", want: "SUCCESS"},
		{in: "failed", want: "FAILED"},
		{in: "refused", want: "FAILED"},
		{in: "processing", want: "PENDING"},
	}

	for _, tc := range tests {
		got := normalizeTransferStatus(tc.in)
		if got != tc.want {
			t.Fatalf("status %q: expected %q, got %q", tc.in, tc.want, got)
		}
	}
}

func TestExtractWebhookIdentifiers(t *testing.T) {
	payload := []byte(`{
		"id": "evt_123",
		"type": "transfer.updated",
		"data": {
			"transfer": {
				"id": "tr_999"
			}
		}
	}`)

	eventID, eventType, transferID := extractWebhookIdentifiers(payload)
	if eventID != "evt_123" {
		t.Fatalf("expected event id evt_123, got %s", eventID)
	}
	if eventType != "transfer.updated" {
		t.Fatalf("expected event type transfer.updated, got %s", eventType)
	}
	if transferID != "tr_999" {
		t.Fatalf("expected transfer id tr_999, got %s", transferID)
	}
}
