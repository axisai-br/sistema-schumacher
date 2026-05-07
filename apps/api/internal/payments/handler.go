package payments

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	httpx "schumacher-tur/api/internal/shared/http"
)

type Handler struct {
	svc       *Service
	secretKey string
}

func NewHandler(svc *Service, secretKey string) *Handler {
	return &Handler{svc: svc, secretKey: strings.TrimSpace(secretKey)}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/payments", func(r chi.Router) {
		r.Get("/", h.list)
		r.Get("/{paymentId}/status", h.getStatus)
		r.Post("/{paymentId}/sync", h.sync)
		r.Post("/", h.create)
		r.Post("/manual", h.createManual)
	})
}

func (h *Handler) RegisterWebhooks(r chi.Router) {
	r.Post("/webhooks/pagarme", h.handleWebhook)
	r.Post("/webhooks/abacatepay", h.handleWebhook)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input CreatePaymentInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	if input.BookingID == "" || input.Amount <= 0 || input.Method == "" {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "booking_id, amount and method are required", nil)
		return
	}
	if !isProviderMethod(input.Method) {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid provider method", nil)
		return
	}

	payment, raw, err := h.svc.Create(r.Context(), input)
	if err != nil {
		if strings.Contains(err.Error(), "PAGARME_SECRET_KEY is required") || strings.Contains(err.Error(), "PAGARME_PAYMENT_RECIPIENT_ID is required") {
			httpx.WriteError(w, http.StatusServiceUnavailable, "CHECKOUT_NOT_CONFIGURED", err.Error(), nil)
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "PAYMENT_CREATE_ERROR", "could not create payment", err.Error())
		return
	}

	providerRaw, checkoutURL, pixCode := parseProviderData(raw)
	httpx.WriteJSON(w, http.StatusCreated, CreatePaymentResponse{
		Payment:     payment,
		ProviderRaw: providerRaw,
		CheckoutURL: checkoutURL,
		PixCode:     pixCode,
	})
}

func (h *Handler) createManual(w http.ResponseWriter, r *http.Request) {
	var input ManualPaymentInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	if input.BookingID == "" || input.Amount <= 0 || input.Method == "" {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "booking_id, amount and method are required", nil)
		return
	}
	if !isManualMethod(input.Method) {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid manual method", nil)
		return
	}

	payment, err := h.svc.CreateManual(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "PAYMENT_MANUAL_ERROR", "could not create manual payment", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"payment": payment})
}

func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "paymentId")
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid payment id", nil)
		return
	}
	status, err := h.svc.GetStatus(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "paymentId")
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid payment id", nil)
		return
	}
	result, err := h.svc.Sync(r.Context(), id)
	if err != nil {
		if IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "PAYMENT_SYNC_ERROR", "could not sync payment", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter := PaymentListFilter{}
	q := r.URL.Query()

	if v := q.Get("booking_id"); v != "" {
		filter.BookingID = v
	}
	if v := q.Get("status"); v != "" {
		if !isValidPaymentStatus(v) {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_STATUS", "invalid status parameter", nil)
			return
		}
		filter.Status = v
	}
	if v := q.Get("since"); v != "" {
		parsed, err := parseTimeParam(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_DATE", "invalid since parameter", err.Error())
			return
		}
		filter.Since = parsed
	}
	if v := q.Get("until"); v != "" {
		parsed, err := parseTimeParam(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_DATE", "invalid until parameter", err.Error())
			return
		}
		filter.Until = parsed
	}
	if v := q.Get("paid_since"); v != "" {
		parsed, err := parseTimeParam(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_DATE", "invalid paid_since parameter", err.Error())
			return
		}
		filter.PaidSince = parsed
	}
	if v := q.Get("paid_until"); v != "" {
		parsed, err := parseTimeParam(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_DATE", "invalid paid_until parameter", err.Error())
			return
		}
		filter.PaidUntil = parsed
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_LIMIT", "invalid limit parameter", nil)
			return
		}
		filter.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_OFFSET", "invalid offset parameter", nil)
			return
		}
		filter.Offset = n
	}

	items, err := h.svc.List(r.Context(), filter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "PAYMENT_LIST_ERROR", "could not list payments", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if h.secretKey == "" {
		httpx.WriteError(w, http.StatusServiceUnavailable, "WEBHOOK_NOT_CONFIGURED", "webhook security not configured", nil)
		return
	}

	body, _ := io.ReadAll(r.Body)
	if !verifyWebhookSignature(r, body, h.secretKey) {
		log.Printf("event=webhook_rejected reason=invalid_signature path=%s remote=%s", r.URL.Path, r.RemoteAddr)
		httpx.WriteError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", "invalid webhook signature", nil)
		return
	}

	evt, err := ParseWebhook(body)
	if err != nil {
		log.Printf("event=webhook_rejected reason=invalid_body path=%s remote=%s", r.URL.Path, r.RemoteAddr)
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid webhook body", err.Error())
		return
	}
	if err := h.svc.HandleWebhook(r.Context(), evt); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "WEBHOOK_ERROR", "could not handle webhook", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func verifyWebhookSignature(r *http.Request, body []byte, secret string) bool {
	if strings.TrimSpace(secret) == "" {
		return false
	}
	incoming := webhookSignaturesFromHeaders(r)
	if len(incoming) == 0 {
		return false
	}
	return VerifyWebhookSignature(secret, body, incoming)
}

func webhookSignaturesFromHeaders(r *http.Request) []string {
	values := []string{}
	for _, name := range []string{"X-Hub-Signature", "X-Hub-Signature-256"} {
		raw := strings.TrimSpace(r.Header.Get(name))
		if raw == "" {
			continue
		}
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			values = append(values, part)
		}
	}
	return values
}

func parseTimeParam(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return &t, nil
	}
	return nil, fmt.Errorf("invalid time format")
}

func isProviderMethod(method string) bool {
	switch method {
	case "PIX":
		return true
	default:
		return false
	}
}

func isManualMethod(method string) bool {
	switch method {
	case "CASH", "TRANSFER", "OTHER":
		return true
	default:
		return false
	}
}

func isValidPaymentStatus(status string) bool {
	switch status {
	case "PENDING", "PAID", "FAILED", "REFUNDED", "CANCELLED":
		return true
	default:
		return false
	}
}
