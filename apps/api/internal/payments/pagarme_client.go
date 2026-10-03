package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.pagar.me/core/v5"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

type MobilePhone struct {
	CountryCode string `json:"country_code,omitempty"`
	AreaCode    string `json:"area_code,omitempty"`
	Number      string `json:"number,omitempty"`
}

type CustomerPhones struct {
	MobilePhone *MobilePhone `json:"mobile_phone,omitempty"`
}

type OrderCustomer struct {
	Name         string          `json:"name"`
	Email        string          `json:"email,omitempty"`
	Type         string          `json:"type,omitempty"`
	Document     string          `json:"document"`
	DocumentType string          `json:"document_type,omitempty"`
	Phones       *CustomerPhones `json:"phones,omitempty"`
}

type OrderItem struct {
	Code        string `json:"code,omitempty"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
}

type PixPayment struct {
	ExpiresIn int `json:"expires_in,omitempty"`
}

type SplitOptions struct {
	Liable              bool `json:"liable"`
	ChargeProcessingFee bool `json:"charge_processing_fee"`
	ChargeRemainderFee  bool `json:"charge_remainder_fee"`
}

type SplitRule struct {
	Amount      int          `json:"amount"`
	Type        string       `json:"type"`
	RecipientID string       `json:"recipient_id"`
	Options     SplitOptions `json:"options"`
}

type OrderPayment struct {
	PaymentMethod string      `json:"payment_method"`
	Pix           *PixPayment `json:"pix,omitempty"`
	Split         []SplitRule `json:"split,omitempty"`
}

type OrderRequest struct {
	Code     string            `json:"code,omitempty"`
	Customer *OrderCustomer    `json:"customer,omitempty"`
	Items    []OrderItem       `json:"items"`
	Payments []OrderPayment    `json:"payments"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type OrderLastTransaction struct {
	ID        string `json:"id"`
	QRCode    string `json:"qr_code"`
	QRCodeURL string `json:"qr_code_url"`
	Status    string `json:"status"`
}

type OrderCharge struct {
	ID              string               `json:"id"`
	OrderID         string               `json:"order_id"`
	Status          string               `json:"status"`
	PaymentMethod   string               `json:"payment_method"`
	PaidAmount      int64                `json:"paid_amount"`
	Amount          int64                `json:"amount"`
	LastTransaction OrderLastTransaction `json:"last_transaction"`
}

type OrderResponse struct {
	ID       string            `json:"id"`
	Code     string            `json:"code"`
	Status   string            `json:"status"`
	Charges  []OrderCharge     `json:"charges"`
	Metadata map[string]string `json:"metadata"`
}

func (o OrderResponse) PrimaryChargeID() string {
	for _, charge := range o.Charges {
		id := strings.TrimSpace(charge.ID)
		if id != "" {
			return id
		}
	}
	return ""
}

func (c *Client) CreateOrder(ctx context.Context, req OrderRequest) (OrderResponse, json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return OrderResponse{}, nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return OrderResponse{}, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.SetBasicAuth(c.apiKey, "")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return OrderResponse{}, nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return OrderResponse{}, raw, fmt.Errorf("pagarme error: %s", strings.TrimSpace(string(raw)))
	}

	var order OrderResponse
	if err := json.Unmarshal(raw, &order); err != nil {
		return OrderResponse{}, raw, err
	}
	return order, raw, nil
}

// CancelCharge cancela uma cobranca (DELETE /charges/{id}). Use so em cobranca
// pendente: em cobranca paga a Pagar.me faz estorno.
func (c *Client) CancelCharge(ctx context.Context, chargeID string) (json.RawMessage, error) {
	chargeID = strings.TrimSpace(chargeID)
	if chargeID == "" {
		return nil, errors.New("charge id is required")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/charges/"+chargeID, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.SetBasicAuth(c.apiKey, "")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return raw, fmt.Errorf("pagarme error: %s", strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func (c *Client) GetOrderByID(ctx context.Context, orderID string) (OrderResponse, json.RawMessage, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return OrderResponse{}, nil, errors.New("order id is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/orders/"+orderID, nil)
	if err != nil {
		return OrderResponse{}, nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.SetBasicAuth(c.apiKey, "")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return OrderResponse{}, nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return OrderResponse{}, raw, fmt.Errorf("pagarme error: %s", strings.TrimSpace(string(raw)))
	}

	var order OrderResponse
	if err := json.Unmarshal(raw, &order); err != nil {
		return OrderResponse{}, raw, err
	}
	return order, raw, nil
}

func BuildCustomer(input *CustomerInput, bookingID string) *OrderCustomer {
	if input == nil {
		return nil
	}

	document := digitsOnly(input.Document)
	if !IsSupportedDocument(document) {
		return nil
	}

	customer := &OrderCustomer{
		Name:     strings.TrimSpace(input.Name),
		Email:    resolveCustomerEmail(input.Email, bookingID),
		Document: document,
	}
	switch len(document) {
	case 14:
		customer.DocumentType = "CNPJ"
		customer.Type = "company"
	default:
		customer.DocumentType = "CPF"
		customer.Type = "individual"
	}

	if phone := buildMobilePhone(input.Phone); phone != nil {
		customer.Phones = &CustomerPhones{MobilePhone: phone}
	}
	return customer
}

func IsSupportedDocument(document string) bool {
	digits := digitsOnly(document)
	switch len(digits) {
	case 11:
		return isValidCPF(digits)
	case 14:
		return isValidCNPJ(digits)
	default:
		return false
	}
}

func isValidCPF(value string) bool {
	digits := digitsOnly(value)
	if len(digits) != 11 || allDigitsEqual(digits) {
		return false
	}
	return cpfCheckDigit(digits[:9], 10) == int(digits[9]-'0') &&
		cpfCheckDigit(digits[:10], 11) == int(digits[10]-'0')
}

func cpfCheckDigit(digits string, weight int) int {
	sum := 0
	for _, char := range digits {
		sum += int(char-'0') * weight
		weight--
	}
	remainder := (sum * 10) % 11
	if remainder == 10 {
		return 0
	}
	return remainder
}

func isValidCNPJ(value string) bool {
	digits := digitsOnly(value)
	if len(digits) != 14 || allDigitsEqual(digits) {
		return false
	}
	return cnpjCheckDigit(digits[:12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == int(digits[12]-'0') &&
		cnpjCheckDigit(digits[:13], []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == int(digits[13]-'0')
}

func cnpjCheckDigit(digits string, weights []int) int {
	sum := 0
	for i, char := range digits {
		sum += int(char-'0') * weights[i]
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

func allDigitsEqual(digits string) bool {
	if digits == "" {
		return false
	}
	for i := 1; i < len(digits); i++ {
		if digits[i] != digits[0] {
			return false
		}
	}
	return true
}

func BuildSingleRecipientSplit(recipientID string) []SplitRule {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return nil
	}
	return []SplitRule{{
		Amount:      100,
		Type:        "percentage",
		RecipientID: recipientID,
		Options: SplitOptions{
			Liable:              true,
			ChargeProcessingFee: true,
			ChargeRemainderFee:  true,
		},
	}}
}

func resolveCustomerEmail(explicitEmail, bookingID string) string {
	email := strings.TrimSpace(explicitEmail)
	if email != "" {
		return email
	}
	token := digitsAndLettersOnly(strings.ToLower(strings.TrimSpace(bookingID)))
	if token == "" {
		return ""
	}
	return "reserva." + token + "@schumachertur.com"
}

func buildMobilePhone(raw string) *MobilePhone {
	digits := digitsOnly(raw)
	if digits == "" {
		return nil
	}

	country := "55"
	switch {
	case strings.HasPrefix(digits, "55") && len(digits) >= 12:
		digits = digits[2:]
	case len(digits) < 10:
		return &MobilePhone{CountryCode: country, Number: digits}
	}

	if len(digits) < 10 {
		return &MobilePhone{CountryCode: country, Number: digits}
	}
	return &MobilePhone{
		CountryCode: country,
		AreaCode:    digits[:2],
		Number:      digits[2:],
	}
}

func digitsOnly(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func digitsAndLettersOnly(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		}
	}
	return b.String()
}

type WebhookEvent struct {
	Type        string          `json:"type"`
	Data        json.RawMessage `json:"data"`
	Raw         json.RawMessage `json:"-"`
	OrderID     string          `json:"-"`
	ProviderRef string          `json:"-"`
}

func (e WebhookEvent) IsPaidEvent() bool {
	switch strings.ToLower(strings.TrimSpace(e.Type)) {
	case "charge.paid", "order.paid":
		return true
	default:
		return false
	}
}

func ParseWebhook(body []byte) (WebhookEvent, error) {
	var evt WebhookEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return evt, err
	}
	evt.Raw = body
	evt.ProviderRef, evt.OrderID = extractWebhookRefs(evt.Type, evt.Data)
	return evt, nil
}

func VerifyWebhookSignature(secret string, body []byte, incoming []string) bool {
	if strings.TrimSpace(secret) == "" || len(incoming) == 0 {
		return false
	}

	sha1Mac := hmac.New(sha1.New, []byte(secret))
	sha1Mac.Write(body)
	sha256Mac := hmac.New(sha256.New, []byte(secret))
	sha256Mac.Write(body)

	candidates := map[string]struct{}{
		hex.EncodeToString(sha1Mac.Sum(nil)):               {},
		"sha1=" + hex.EncodeToString(sha1Mac.Sum(nil)):     {},
		hex.EncodeToString(sha256Mac.Sum(nil)):             {},
		"sha256=" + hex.EncodeToString(sha256Mac.Sum(nil)): {},
	}

	for _, item := range incoming {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := candidates[item]; ok {
			return true
		}
	}
	return false
}

func extractWebhookRefs(eventType string, raw json.RawMessage) (string, string) {
	type chargePayload struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		OrderID string `json:"order_id"`
	}
	type orderPayload struct {
		ID      string          `json:"id"`
		Charges []chargePayload `json:"charges"`
	}

	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "charge.paid":
		var charge chargePayload
		if err := json.Unmarshal(raw, &charge); err != nil {
			return "", ""
		}
		return strings.TrimSpace(charge.ID), strings.TrimSpace(charge.OrderID)
	case "order.paid":
		var order orderPayload
		if err := json.Unmarshal(raw, &order); err != nil {
			return "", ""
		}
		for _, charge := range order.Charges {
			if strings.EqualFold(strings.TrimSpace(charge.Status), "paid") && strings.TrimSpace(charge.ID) != "" {
				return strings.TrimSpace(charge.ID), strings.TrimSpace(order.ID)
			}
		}
		for _, charge := range order.Charges {
			if strings.TrimSpace(charge.ID) != "" {
				return strings.TrimSpace(charge.ID), strings.TrimSpace(order.ID)
			}
		}
		return "", strings.TrimSpace(order.ID)
	default:
		return "", ""
	}
}
