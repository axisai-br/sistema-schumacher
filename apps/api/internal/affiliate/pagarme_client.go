package affiliate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type PagarmeClient struct {
	baseURL   string
	secretKey string
	http      *http.Client
}

type PagarmeTransferResult struct {
	ID     string
	Status string
	Raw    []byte
}

type RecipientDetailsResult struct {
	AnticipationInfo AnticipationInfo
}

func NewPagarmeClient(baseURL, secretKey string) *PagarmeClient {
	return &PagarmeClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		secretKey: secretKey,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *PagarmeClient) GetRecipientBalance(ctx context.Context, recipientID string) (BalanceResponse, []byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/recipients/%s/balance", c.baseURL, recipientID), nil)
	if err != nil {
		return BalanceResponse{}, nil, err
	}

	resp, raw, err := c.do(req)
	if err != nil {
		return BalanceResponse{}, raw, err
	}
	if resp.StatusCode >= 300 {
		return BalanceResponse{}, raw, fmt.Errorf("pagarme balance error: status=%d", resp.StatusCode)
	}

	balance, err := parseBalanceResponse(raw)
	if err != nil {
		return BalanceResponse{}, raw, err
	}
	return balance, raw, nil
}

func (c *PagarmeClient) CreateTransfer(ctx context.Context, amount int64, recipientID, idempotencyKey string) (PagarmeTransferResult, []byte, []byte, error) {
	payload := map[string]interface{}{
		"amount":       amount,
		"recipient_id": recipientID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return PagarmeTransferResult{}, nil, nil, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("%s/transfers", c.baseURL), body)
	if err != nil {
		return PagarmeTransferResult{}, body, nil, err
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, raw, err := c.do(req)
	if err != nil {
		return PagarmeTransferResult{}, body, raw, err
	}
	if resp.StatusCode >= 300 {
		return PagarmeTransferResult{}, body, raw, fmt.Errorf("pagarme transfer error: status=%d", resp.StatusCode)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return PagarmeTransferResult{}, body, raw, err
	}
	id, _ := parsed["id"].(string)
	status, _ := parsed["status"].(string)
	return PagarmeTransferResult{ID: id, Status: status, Raw: raw}, body, raw, nil
}

func (c *PagarmeClient) GetRecipientDetails(ctx context.Context, recipientID string) (RecipientDetailsResult, []byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/recipients/%s", c.baseURL, recipientID), nil)
	if err != nil {
		return RecipientDetailsResult{}, nil, err
	}
	resp, raw, err := c.do(req)
	if err != nil {
		return RecipientDetailsResult{}, raw, err
	}
	if resp.StatusCode >= 300 {
		return RecipientDetailsResult{}, raw, fmt.Errorf("pagarme recipient details error: status=%d", resp.StatusCode)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return RecipientDetailsResult{}, raw, err
	}

	result := RecipientDetailsResult{}
	if settings, ok := parsed["automatic_anticipation_settings"].(map[string]interface{}); ok {
		enabled, _ := settings["enabled"].(bool)
		atype, _ := settings["type"].(string)
		result.AnticipationInfo = AnticipationInfo{
			Enabled:          enabled,
			Type:             atype,
			Delay:            extractInt64(settings["delay"]),
			VolumePercentage: extractInt64(settings["volume_percentage"]),
		}
	}
	return result, raw, nil
}

func (c *PagarmeClient) GetTransfer(ctx context.Context, transferID string) (PagarmeTransferResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/transfers/%s", c.baseURL, transferID), nil)
	if err != nil {
		return PagarmeTransferResult{}, err
	}
	resp, raw, err := c.do(req)
	if err != nil {
		return PagarmeTransferResult{}, err
	}
	if resp.StatusCode >= 300 {
		return PagarmeTransferResult{}, fmt.Errorf("pagarme get transfer error: status=%d", resp.StatusCode)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return PagarmeTransferResult{}, err
	}
	id, _ := parsed["id"].(string)
	status, _ := parsed["status"].(string)
	return PagarmeTransferResult{ID: id, Status: status, Raw: raw}, nil
}

func (c *PagarmeClient) newRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	reader := bytes.NewReader(body)
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+basicToken(c.secretKey))
	return req, nil
}

func (c *PagarmeClient) do(req *http.Request) (*http.Response, []byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw, nil
}

func basicToken(secret string) string {
	return base64.StdEncoding.EncodeToString([]byte(secret + ":"))
}

func parseBalanceResponse(raw []byte) (BalanceResponse, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return BalanceResponse{}, err
	}

	availableAmount, availableCurrency := extractAmountAndCurrency(payload["available"])
	waitingAmount, waitingCurrency := extractAmountAndCurrency(payload["waiting_funds"])
	transferredAmount, transferredCurrency := extractAmountAndCurrency(payload["transferred"])

	currency := firstNonEmpty(availableCurrency, waitingCurrency, transferredCurrency, "BRL")
	return BalanceResponse{
		AvailableAmount:    availableAmount,
		WaitingFundsAmount: waitingAmount,
		TransferredAmount:  transferredAmount,
		Currency:           currency,
	}, nil
}

func extractAmountAndCurrency(raw interface{}) (int64, string) {
	switch typed := raw.(type) {
	case map[string]interface{}:
		amount := extractInt64(typed["amount"])
		currency, _ := typed["currency"].(string)
		return amount, currency
	case []interface{}:
		var amount int64
		currency := ""
		for _, item := range typed {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			amount += extractInt64(m["amount"])
			if currency == "" {
				currency, _ = m["currency"].(string)
			}
		}
		return amount, currency
	default:
		return 0, ""
	}
}

func extractInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
