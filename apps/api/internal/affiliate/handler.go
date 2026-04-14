package affiliate

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"schumacher-tur/api/internal/auth"
	httpx "schumacher-tur/api/internal/shared/http"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/affiliate", func(r chi.Router) {
		r.Get("/balance", h.getBalance)
		r.Post("/withdraw", h.withdraw)
		r.Get("/withdrawals-history", h.listWithdrawalsHistory)
	})
}

func (h *Handler) RegisterWebhooks(r chi.Router) {
	r.Post("/webhooks/pagarme", h.handlePagarmeWebhook)
}

func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authenticated user", nil)
		return
	}

	out, err := h.svc.GetBalance(r.Context(), userID)
	if err != nil {
		switch err {
		case ErrRoleRequired:
			httpx.WriteError(w, http.StatusForbidden, "ROLE_REQUIRED", "financeiro role required", nil)
			return
		case ErrRecipientNotLinked:
			httpx.WriteError(w, http.StatusForbidden, "RECIPIENT_NOT_LINKED", "recipient not linked for user", nil)
			return
		case ErrPagarmeNotConfigured:
			httpx.WriteError(w, http.StatusServiceUnavailable, "PAGARME_NOT_CONFIGURED", "pagarme integration not configured", nil)
			return
		default:
			httpx.WriteError(w, http.StatusBadGateway, "BALANCE_PROVIDER_ERROR", "could not load recipient balance", err.Error())
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authenticated user", nil)
		return
	}

	var input WithdrawRequest
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	out, err := h.svc.Withdraw(r.Context(), userID, input.Amount)
	if err != nil {
		switch err {
		case ErrInvalidAmount:
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be greater than zero", nil)
			return
		case ErrInsufficientBalance:
			httpx.WriteError(w, http.StatusConflict, "INSUFFICIENT_BALANCE", "insufficient available balance", nil)
			return
		case ErrRoleRequired:
			httpx.WriteError(w, http.StatusForbidden, "ROLE_REQUIRED", "financeiro role required", nil)
			return
		case ErrRecipientNotLinked:
			httpx.WriteError(w, http.StatusForbidden, "RECIPIENT_NOT_LINKED", "recipient not linked for user", nil)
			return
		case ErrPagarmeNotConfigured:
			httpx.WriteError(w, http.StatusServiceUnavailable, "PAGARME_NOT_CONFIGURED", "pagarme integration not configured", nil)
			return
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "WITHDRAW_ERROR", "could not request withdrawal", err.Error())
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) listWithdrawalsHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authenticated user", nil)
		return
	}

	filter := ListFilter{
		Limit:  parseIntOrDefault(r.URL.Query().Get("limit"), 20),
		Offset: parseIntOrDefault(r.URL.Query().Get("offset"), 0),
	}
	out, err := h.svc.ListWithdrawalsHistory(r.Context(), userID, filter)
	if err != nil {
		switch err {
		case ErrRoleRequired:
			httpx.WriteError(w, http.StatusForbidden, "ROLE_REQUIRED", "financeiro role required", nil)
			return
		case ErrRecipientNotLinked:
			httpx.WriteError(w, http.StatusForbidden, "RECIPIENT_NOT_LINKED", "recipient not linked for user", nil)
			return
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "WITHDRAWALS_HISTORY_ERROR", "could not list withdrawals history", err.Error())
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) handlePagarmeWebhook(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	signature := strings.TrimSpace(r.Header.Get("X-Pagarme-Signature"))
	basicUser, basicPass, ok := r.BasicAuth()
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "WEBHOOK_BASIC_REQUIRED", "missing basic auth credentials", nil)
		return
	}

	err := h.svc.HandleWebhook(r.Context(), body, signature, basicUser, basicPass)
	if err != nil {
		if err == ErrWebhookUnauthorized {
			httpx.WriteError(w, http.StatusUnauthorized, "INVALID_WEBHOOK_CREDENTIALS", "invalid webhook credentials", nil)
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "WEBHOOK_PROCESS_ERROR", "could not process webhook", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func parseIntOrDefault(raw string, fallback int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
