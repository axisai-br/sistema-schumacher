package payments

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"testing"
)

func TestParseTimeParam(t *testing.T) {
	if _, err := parseTimeParam("2026-02-03T10:00:00Z"); err != nil {
		t.Fatalf("expected valid RFC3339, got %v", err)
	}
	if _, err := parseTimeParam("2026-02-03"); err != nil {
		t.Fatalf("expected valid date, got %v", err)
	}
	if _, err := parseTimeParam("invalid"); err == nil {
		t.Fatalf("expected error for invalid date")
	}
}

func TestIsValidPaymentStatus(t *testing.T) {
	if !isValidPaymentStatus("PAID") {
		t.Fatalf("expected PAID to be valid")
	}
	if isValidPaymentStatus("UNKNOWN") {
		t.Fatalf("expected UNKNOWN to be invalid")
	}
}

func TestParseWebhookExtractProviderRef(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "charge paid uses charge id",
			payload: `{"type":"charge.paid","data":{"id":"ch_123","order_id":"or_999"}}`,
			want:    "ch_123",
		},
		{
			name:    "order paid uses paid charge id",
			payload: `{"type":"order.paid","data":{"id":"or_456","charges":[{"id":"ch_456","status":"paid"}]}}`,
			want:    "ch_456",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evt, err := ParseWebhook([]byte(tc.payload))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if evt.ProviderRef != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, evt.ProviderRef)
			}
		})
	}
}

func TestWebhookSignaturesFromHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "/webhooks/pagarme", nil)
	req.Header.Set("X-Hub-Signature", "sha1=abc")
	req.Header.Set("X-Hub-Signature-256", "sha256=def")

	got := webhookSignaturesFromHeaders(req)
	if len(got) != 2 {
		t.Fatalf("expected 2 signatures, got %d", len(got))
	}
	if got[0] != "sha1=abc" || got[1] != "sha256=def" {
		t.Fatalf("unexpected signatures: %#v", got)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "super-secret"
	body := []byte(`{"type":"charge.paid"}`)
	signatures := signPayloads(secret, body)

	if !VerifyWebhookSignature(secret, body, signatures) {
		t.Fatalf("expected valid signature")
	}
	if VerifyWebhookSignature(secret, body, []string{"wrong-signature"}) {
		t.Fatalf("expected invalid signature to fail")
	}
}

func TestVerifyWebhookAuthenticationAcceptsValidBasicAuth(t *testing.T) {
	handler := NewHandler(nil, "ignored-hmac-secret", "pagarme-user", "pagarme-pass")
	req := httptest.NewRequest("POST", "/webhooks/pagarme", nil)
	req.SetBasicAuth("pagarme-user", "pagarme-pass")

	if !handler.verifyWebhookAuthentication(req, []byte(`{"type":"charge.paid"}`)) {
		t.Fatalf("expected valid basic auth to be accepted")
	}
}

func TestVerifyWebhookAuthenticationRejectsInvalidBasicAuth(t *testing.T) {
	secret := "super-secret"
	body := []byte(`{"type":"charge.paid"}`)
	handler := NewHandler(nil, secret, "pagarme-user", "pagarme-pass")
	req := httptest.NewRequest("POST", "/webhooks/pagarme", nil)
	req.SetBasicAuth("pagarme-user", "wrong-pass")
	req.Header.Set("X-Hub-Signature", signPayloads(secret, body)[0])

	if handler.verifyWebhookAuthentication(req, body) {
		t.Fatalf("expected invalid basic auth to be rejected when basic auth is configured")
	}
}

func TestVerifyWebhookAuthenticationRejectsMissingBasicAuth(t *testing.T) {
	handler := NewHandler(nil, "", "pagarme-user", "pagarme-pass")
	req := httptest.NewRequest("POST", "/webhooks/pagarme", nil)

	if handler.verifyWebhookAuthentication(req, []byte(`{"type":"charge.paid"}`)) {
		t.Fatalf("expected missing basic auth to be rejected")
	}
}

func TestVerifyWebhookAuthenticationFallsBackToHMAC(t *testing.T) {
	secret := "super-secret"
	body := []byte(`{"type":"charge.paid"}`)
	handler := NewHandler(nil, secret, "", "")
	req := httptest.NewRequest("POST", "/webhooks/pagarme", nil)
	req.Header.Set("X-Hub-Signature-256", signPayloads(secret, body)[1])

	if !handler.verifyWebhookAuthentication(req, body) {
		t.Fatalf("expected valid hmac signature to be accepted when basic auth is not configured")
	}
}

func TestWebhookSecurityConfigured(t *testing.T) {
	if NewHandler(nil, "", "", "").webhookSecurityConfigured() {
		t.Fatalf("expected empty webhook security config to be disabled")
	}
	if !NewHandler(nil, "", "pagarme-user", "pagarme-pass").webhookSecurityConfigured() {
		t.Fatalf("expected basic auth config to enable webhook security")
	}
	if !NewHandler(nil, "super-secret", "", "").webhookSecurityConfigured() {
		t.Fatalf("expected hmac secret to enable webhook security")
	}
}

func signPayloads(key string, body []byte) []string {
	sha1Mac := hmac.New(sha1.New, []byte(key))
	sha1Mac.Write(body)
	sha256Mac := hmac.New(sha256.New, []byte(key))
	sha256Mac.Write(body)
	return []string{
		"sha1=" + hex.EncodeToString(sha1Mac.Sum(nil)),
		"sha256=" + hex.EncodeToString(sha256Mac.Sum(nil)),
	}
}
