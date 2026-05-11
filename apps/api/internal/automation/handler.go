package automation

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/realtime"
	httpx "schumacher-tur/api/internal/shared/http"
)

type Handler struct {
	svc    *Service
	broker *realtime.Broker
}

func NewHandler(svc *Service, brokers ...*realtime.Broker) *Handler {
	var broker *realtime.Broker
	if len(brokers) > 0 {
		broker = brokers[0]
	}
	return &Handler{svc: svc, broker: broker}
}

func (h *Handler) RegisterWebhooks(r chi.Router) {
	r.Route("/webhooks/evolution", func(r chi.Router) {
		r.Post("/", h.handleEvolutionEvent)
		r.Post("/messages", h.handleEvolutionEvent)
		r.Post("/messages-upsert", h.handleEvolutionEvent)
		r.Post("/status", h.handleEvolutionEvent)
		r.Post("/messages-update", h.handleEvolutionEvent)
		r.Post("/presence", h.handleEvolutionEvent)
		r.Post("/presence-update", h.handleEvolutionEvent)
	})
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/automation/jobs", func(r chi.Router) {
		r.Post("/chat-review-alerts/run", h.runChatReviewAlerts)
		r.Post("/chat-buffer-flush/run", h.runChatBufferFlush)
		r.Post("/chat-auto-send-retry/run", h.runChatAutoSendRetry)
		r.Post("/bookings-expire/run", h.runBookingsExpire)
		r.Post("/payment-notifications/run", h.runPaymentNotifications)
		r.Post("/sheet-sync/run", h.runSheetSync)
		r.Get("/{jobName}/runs", h.listJobRuns)
	})
	r.Get("/automation/cutover/readiness", h.getCutoverReadiness)
}

func (h *Handler) handleEvolutionMessages(w http.ResponseWriter, r *http.Request) {
	h.handleEvolutionEvent(w, r)
}

func (h *Handler) handleEvolutionStatus(w http.ResponseWriter, r *http.Request) {
	h.handleEvolutionEvent(w, r)
}

func (h *Handler) handleEvolutionPresence(w http.ResponseWriter, r *http.Request) {
	h.handleEvolutionEvent(w, r)
}

func (h *Handler) handleEvolutionEvent(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeEvolutionWebhook(w, r) {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "could not read request body", err.Error())
		return
	}

	event, err := resolveEvolutionWebhookEvent(body)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	if h.dispatchEvolutionWebhook(w, r, event, body) {
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "skipped",
		"event":  event,
		"reason": "unsupported_event",
	})
}

func (h *Handler) runBookingsExpire(w http.ResponseWriter, r *http.Request) {
	var input RunBookingsExpireInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, httpx.ErrEmptyBody) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	result, err := h.svc.RunBookingsExpire(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidBookingsExpireLimit), errors.Is(err, ErrInvalidBookingsExpireHold):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "BOOKINGS_EXPIRE_ERROR", "could not expire pending bookings", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) runChatReviewAlerts(w http.ResponseWriter, r *http.Request) {
	var input RunChatReviewAlertsInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, httpx.ErrEmptyBody) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	result, err := h.svc.RunChatReviewAlerts(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "CHAT_REVIEW_ALERT_ERROR", "could not send chat review alert", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) runChatBufferFlush(w http.ResponseWriter, r *http.Request) {
	var input RunChatBufferFlushInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, httpx.ErrEmptyBody) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	result, err := h.svc.RunChatBufferFlush(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidChatBufferFlushLimit):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_BUFFER_FLUSH_ERROR", "could not flush due chat buffers", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) runChatAutoSendRetry(w http.ResponseWriter, r *http.Request) {
	var input RunChatAutoSendRetryInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, httpx.ErrEmptyBody) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	result, err := h.svc.RunChatAutoSendRetry(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidChatAutoSendRetryLimit), errors.Is(err, ErrInvalidChatAutoSendRetryCooldown):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_AUTO_SEND_RETRY_ERROR", "could not retry pending draft auto-sends", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) runPaymentNotifications(w http.ResponseWriter, r *http.Request) {
	var input RunPaymentNotificationsInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, httpx.ErrEmptyBody) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	result, err := h.svc.RunPaymentNotifications(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrPaymentNotificationTarget):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		case payments.IsNotFound(err):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "PAYMENT_NOTIFICATION_ERROR", "could not queue payment notification draft", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) runSheetSync(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented(w, r)
}

func (h *Handler) listJobRuns(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_QUERY", "invalid limit", err.Error())
			return
		}
		limit = parsed
	}

	result, err := h.svc.ListJobRuns(r.Context(), ListJobRunsInput{
		JobName:       chi.URLParam(r, "jobName"),
		Status:        r.URL.Query().Get("status"),
		TriggerSource: r.URL.Query().Get("trigger_source"),
		Limit:         limit,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrMissingJobName), errors.Is(err, ErrInvalidJobRunLimit):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "AUTOMATION_JOB_RUNS_ERROR", "could not list automation job runs", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) getCutoverReadiness(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GetCutoverReadiness(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "CUTOVER_READINESS_ERROR", "could not load cutover readiness", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) authorizeEvolutionWebhook(w http.ResponseWriter, r *http.Request) bool {
	expected := strings.TrimSpace(h.svc.cfg.EvolutionWebhookSecret)
	if expected == "" {
		return true
	}

	candidates := []string{
		r.Header.Get("X-Evolution-Webhook-Secret"),
		r.Header.Get("X-Webhook-Secret"),
		r.URL.Query().Get("webhookSecret"),
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); auth != "" {
		if token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")); token != "" && token != auth {
			candidates = append(candidates, token)
		}
	}

	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == expected {
			return true
		}
	}

	httpx.WriteError(w, http.StatusUnauthorized, "EVOLUTION_WEBHOOK_UNAUTHORIZED", "invalid evolution webhook secret", nil)
	return false
}

func (h *Handler) dispatchEvolutionWebhook(w http.ResponseWriter, r *http.Request, event string, body []byte) bool {
	switch strings.TrimSpace(event) {
	case "messages.upsert":
		result, err := h.svc.HandleEvolutionMessages(r.Context(), body)
		if err != nil {
			switch {
			case errors.Is(err, ErrInvalidEvolutionPayload), errors.Is(err, ErrMissingEvolutionChatKey):
				httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "EVOLUTION_WEBHOOK_ERROR", "could not process evolution webhook", err.Error())
			}
			return true
		}
		h.publishChatEvent("chat.message.upsert", result.SessionID)
		httpx.WriteJSON(w, http.StatusAccepted, result)
		return true
	case "messages.update":
		result, err := h.svc.HandleEvolutionStatus(r.Context(), body)
		if err != nil {
			switch {
			case errors.Is(err, ErrInvalidEvolutionPayload), errors.Is(err, ErrMissingEvolutionMessageID), errors.Is(err, ErrMissingEvolutionStatus):
				httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "EVOLUTION_STATUS_WEBHOOK_ERROR", "could not process evolution status webhook", err.Error())
			}
			return true
		}
		h.publishChatEvent("chat.message.status", "")
		httpx.WriteJSON(w, http.StatusAccepted, result)
		return true
	case "presence.update", "presence-update":
		result, err := h.svc.HandleEvolutionPresence(r.Context(), body)
		if err != nil {
			switch {
			case errors.Is(err, ErrInvalidEvolutionPayload), errors.Is(err, ErrMissingEvolutionChatKey), errors.Is(err, ErrMissingEvolutionPresence):
				httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "EVOLUTION_PRESENCE_WEBHOOK_ERROR", "could not process evolution presence webhook", err.Error())
			}
			return true
		}
		h.publishChatEvent("chat.presence.update", result.SessionID)
		httpx.WriteJSON(w, http.StatusAccepted, result)
		return true
	default:
		return false
	}
}

func (h *Handler) publishChatEvent(eventType string, sessionID string) {
	if h.broker == nil {
		return
	}
	h.broker.Publish(realtime.ChatEvent{
		Type:      eventType,
		Channel:   "WHATSAPP",
		SessionID: strings.TrimSpace(sessionID),
		At:        time.Now().UTC(),
	})
}


func resolveEvolutionWebhookEvent(body []byte) (string, error) {
	payload, _, err := decodeEvolutionPayload(body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Event), nil
}
