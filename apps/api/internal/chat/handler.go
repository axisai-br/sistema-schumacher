package chat

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"schumacher-tur/api/internal/auth"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpx "schumacher-tur/api/internal/shared/http"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/chat", func(r chi.Router) {
		r.Post("/messages/ingest", h.ingestMessage)
		r.Get("/reports/structured-interpreter-shadow", h.getStructuredInterpreterShadowReport)
		r.Get("/sessions", h.listSessions)
		r.Get("/sessions/summary", h.getSessionsSummary)
		r.Route("/sessions/{sessionId}", func(r chi.Router) {
			r.Get("/", h.getSession)
			r.Get("/draft", h.getCurrentDraft)
			r.Post("/draft/retry-auto-send", h.retryDraftAutoSend)
			r.Get("/messages", h.listMessages)
			r.Post("/handoff", h.requestHandoff)
			r.Post("/resume", h.resumeSession)
			r.Post("/resolve", h.resolveSession)
			r.Post("/reply", h.reply)
			r.Post("/reply/media", h.replyMedia)
			r.Post("/reprocess", h.reprocess)
		})
	})
}

func (h *Handler) ingestMessage(w http.ResponseWriter, r *http.Request) {
	var input IngestMessageInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	result, err := h.svc.Ingest(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrContactKeyRequired), errors.Is(err, ErrDirectionRequired), errors.Is(err, ErrInvalidDirection):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_INGEST_ERROR", "could not ingest chat message", err.Error())
		}
		return
	}

	status := http.StatusCreated
	if result.Idempotent {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, result)
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListSessionsFilter(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_PAGINATION", "invalid query parameters", nil)
		return
	}

	items, err := h.svc.ListSessions(r.Context(), filter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "CHAT_LIST_ERROR", "could not list chat sessions", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) getSessionsSummary(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListSessionsFilter(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_PAGINATION", "invalid query parameters", nil)
		return
	}

	item, err := h.svc.GetSessionsSummary(r.Context(), filter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "CHAT_SUMMARY_ERROR", "could not summarize chat sessions", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	item, err := h.svc.GetSession(r.Context(), sessionID.String())
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "CHAT_GET_ERROR", "could not load chat session", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) getCurrentDraft(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	item, err := h.svc.GetCurrentDraft(r.Context(), sessionID.String())
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrDraftNotFound):
			httpx.WriteError(w, http.StatusNotFound, "DRAFT_NOT_FOUND", "chat session has no automation draft", nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_DRAFT_GET_ERROR", "could not load current draft", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) retryDraftAutoSend(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	var input RetryDraftAutoSendInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	input.SessionID = sessionID.String()

	result, err := h.svc.RetryDraftAutoSend(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrDraftNotFound):
			httpx.WriteError(w, http.StatusNotFound, "DRAFT_NOT_FOUND", "chat session has no automation draft", nil)
		case errors.Is(err, ErrDraftAutoSendRetryNotAllowed):
			httpx.WriteError(w, http.StatusConflict, "DRAFT_AUTO_SEND_RETRY_NOT_ALLOWED", err.Error(), nil)
		case errors.Is(err, ErrReplyDeliveryFailed):
			httpx.WriteError(w, http.StatusBadGateway, "CHAT_AUTO_SEND_RETRY_ERROR", "could not retry auto-send for current draft", err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_DRAFT_AUTO_SEND_RETRY_ERROR", "could not retry auto-send for current draft", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	filter, err := parseListMessagesFilter(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_PAGINATION", "invalid query parameters", nil)
		return
	}

	items, err := h.svc.ListMessages(r.Context(), sessionID.String(), filter)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "CHAT_MESSAGES_LIST_ERROR", "could not list chat messages", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) getStructuredInterpreterShadowReport(w http.ResponseWriter, r *http.Request) {
	filter, err := parseStructuredInterpreterShadowReportFilter(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_QUERY", "invalid query parameters", nil)
		return
	}

	result, err := h.svc.GetStructuredInterpreterShadowReport(r.Context(), filter)
	if err != nil {
		if errors.Is(err, ErrShadowReportSessionRequired) {
			httpx.WriteError(w, http.StatusBadRequest, "INVALID_QUERY", err.Error(), nil)
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "CHAT_STRUCTURED_INTERPRETER_SHADOW_REPORT_ERROR", "could not load structured interpreter shadow report", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) requestHandoff(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	var input RequestHandoffInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	authUser, ok := auth.UserFromContext(r.Context())
	if !ok || authUser.ID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authenticated user", nil)
		return
	}
	input.SessionID = sessionID.String()
	input.AssignedUserID = authUser.ID
	input.RequestedBy = "OPERATOR"
	if input.Reason == "" {
		input.Reason = "manual_assume"
	}

	result, err := h.svc.RequestHandoff(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrAuthenticatedUserRequired):
			httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authenticated user", nil)
		case errors.Is(err, ErrUserProfileNotConfigured):
			httpx.WriteError(w, http.StatusForbidden, "USER_PROFILE_NOT_CONFIGURED", "authenticated user profile is not configured", nil)
		case errors.Is(err, ErrInvalidAssignedUser):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		case errors.Is(err, ErrHandoffAlreadyActive):
			httpx.WriteError(w, http.StatusConflict, "HANDOFF_ALREADY_ACTIVE", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_HANDOFF_ERROR", "could not request handoff", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) resumeSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	var input ResumeSessionInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	input.SessionID = sessionID.String()

	result, err := h.svc.ResumeSession(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrNoActiveHandoff):
			httpx.WriteError(w, http.StatusConflict, "NO_ACTIVE_HANDOFF", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_RESUME_ERROR", "could not resume chat session", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) resolveSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	var input ResolveSessionInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	input.SessionID = sessionID.String()

	result, err := h.svc.ResolveSession(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrResolveByRequired), errors.Is(err, ErrInvalidResolveBy):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		case errors.Is(err, ErrSessionAlreadyResolved), errors.Is(err, ErrResolveRequiresHuman), errors.Is(err, ErrResolveOwnerMismatch):
			httpx.WriteError(w, http.StatusConflict, "RESOLVE_NOT_ALLOWED", err.Error(), nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_RESOLVE_ERROR", "could not resolve chat session", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	var input ReplyInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	input.SessionID = sessionID.String()

	result, err := h.svc.Reply(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrReplyBodyRequired), errors.Is(err, ErrReplyOwnerRequired), errors.Is(err, ErrInvalidReplyOwner), errors.Is(err, ErrInvalidReplyDraft):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		case errors.Is(err, ErrReplyRequiresHuman), errors.Is(err, ErrReplyOwnerMismatch), errors.Is(err, ErrReplyDraftNotAllowed):
			httpx.WriteError(w, http.StatusConflict, "REPLY_NOT_ALLOWED", err.Error(), nil)
		case errors.Is(err, ErrReplyDeliveryFailed):
			httpx.WriteError(w, http.StatusBadGateway, "CHAT_REPLY_DELIVERY_ERROR", "could not deliver assisted reply", err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_REPLY_ERROR", "could not create assisted reply", err.Error())
		}
		return
	}

	status := http.StatusCreated
	if result.Idempotent {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, result)
}

func (h *Handler) replyMedia(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	if err := r.ParseMultipartForm(25 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid multipart body", err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "file is required", nil)
		return
	}
	defer file.Close()

	fileContent, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "could not read uploaded file", err.Error())
		return
	}

	metadata := map[string]interface{}{}
	if rawMetadata := r.FormValue("metadata"); rawMetadata != "" {
		if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "metadata must be a valid json object", nil)
			return
		}
	}

	input := ReplyMediaInput{
		SessionID:      sessionID.String(),
		OwnerUserID:    r.FormValue("owner_user_id"),
		SenderName:     r.FormValue("sender_name"),
		IdempotencyKey: r.FormValue("idempotency_key"),
		Caption:        r.FormValue("caption"),
		MediaType:      r.FormValue("media_type"),
		FileName:       header.Filename,
		MimeType:       header.Header.Get("Content-Type"),
		FileContent:    fileContent,
		Metadata:       metadata,
	}

	result, err := h.svc.ReplyMedia(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrReplyBodyRequired), errors.Is(err, ErrReplyOwnerRequired), errors.Is(err, ErrInvalidReplyOwner),
			errors.Is(err, ErrReplyMediaFileRequired), errors.Is(err, ErrReplyMediaTypeInvalid):
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		case errors.Is(err, ErrReplyRequiresHuman), errors.Is(err, ErrReplyOwnerMismatch):
			httpx.WriteError(w, http.StatusConflict, "REPLY_NOT_ALLOWED", err.Error(), nil)
		case errors.Is(err, ErrReplyDeliveryFailed):
			httpx.WriteError(w, http.StatusBadGateway, "CHAT_REPLY_DELIVERY_ERROR", "could not deliver assisted media reply", err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_REPLY_MEDIA_ERROR", "could not create assisted media reply", err.Error())
		}
		return
	}

	status := http.StatusCreated
	if result.Idempotent {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, result)
}

func (h *Handler) reprocess(w http.ResponseWriter, r *http.Request) {
	sessionID, err := httpx.ParseUUIDParam(r, "sessionId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_ID", "invalid session id", nil)
		return
	}

	var input ReprocessInput
	if err := httpx.DecodeJSON(r, &input); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	input.SessionID = sessionID.String()

	result, err := h.svc.Reprocess(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "chat session not found", nil)
		case errors.Is(err, ErrReprocessRequiresBot), errors.Is(err, ErrReprocessNoMessages):
			httpx.WriteError(w, http.StatusConflict, "REPROCESS_NOT_ALLOWED", err.Error(), nil)
		case errors.Is(err, ErrAgentToolFailed):
			httpx.WriteError(w, http.StatusBadGateway, "CHAT_AGENT_TOOL_ERROR", "could not resolve agent tool context", err.Error())
		case errors.Is(err, ErrAgentRunFailed):
			httpx.WriteError(w, http.StatusBadGateway, "CHAT_AGENT_RUN_ERROR", "could not generate agent draft", err.Error())
		case errors.Is(err, ErrReplyDeliveryFailed):
			httpx.WriteError(w, http.StatusBadGateway, "CHAT_AUTO_SEND_ERROR", "could not auto-send agent draft", err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "CHAT_REPROCESS_ERROR", "could not reprocess chat session", err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func parseListSessionsFilter(r *http.Request) (ListSessionsFilter, error) {
	filter := ListSessionsFilter{
		Channel:             r.URL.Query().Get("channel"),
		Status:              r.URL.Query().Get("status"),
		HandoffStatus:       r.URL.Query().Get("handoff_status"),
		ContactKey:          r.URL.Query().Get("contact_key"),
		AgentStatus:         r.URL.Query().Get("agent_status"),
		DraftReviewStatus:   r.URL.Query().Get("draft_review_status"),
		DraftAutoSendStatus: r.URL.Query().Get("draft_auto_send_status"),
		OrderBy:             r.URL.Query().Get("order_by"),
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil {
			return ListSessionsFilter{}, err
		}
		filter.Limit = value
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		value, err := strconv.Atoi(offset)
		if err != nil {
			return ListSessionsFilter{}, err
		}
		filter.Offset = value
	}

	return filter, nil
}

func parseStructuredInterpreterShadowReportFilter(r *http.Request) (StructuredInterpreterShadowReportFilter, error) {
	filter := StructuredInterpreterShadowReportFilter{}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil || value <= 0 || value > 1000 {
			return StructuredInterpreterShadowReportFilter{}, errors.New("invalid limit")
		}
		filter.Limit = value
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		value, err := strconv.Atoi(offset)
		if err != nil || value < 0 {
			return StructuredInterpreterShadowReportFilter{}, errors.New("invalid offset")
		}
		filter.Offset = value
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		return StructuredInterpreterShadowReportFilter{}, ErrShadowReportSessionRequired
	}
	parsed, err := uuid.Parse(sessionID)
	if err != nil {
		return StructuredInterpreterShadowReportFilter{}, errors.New("invalid session_id")
	}
	filter.SessionID = parsed.String()

	return filter, nil
}

func parseListMessagesFilter(r *http.Request) (ListMessagesFilter, error) {
	filter := ListMessagesFilter{}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil {
			return ListMessagesFilter{}, err
		}
		filter.Limit = value
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		value, err := strconv.Atoi(offset)
		if err != nil {
			return ListMessagesFilter{}, err
		}
		filter.Offset = value
	}

	return filter, nil
}
