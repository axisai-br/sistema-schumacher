package chat

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"schumacher-tur/api/internal/auth"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrContactKeyRequired           = errors.New("contact_key is required")
	ErrDirectionRequired            = errors.New("message.direction is required")
	ErrInvalidDirection             = errors.New("message.direction must be INBOUND or OUTBOUND")
	ErrDraftBodyRequired            = errors.New("automation draft body is required")
	ErrPresenceRequired             = errors.New("presence_status is required")
	ErrSessionNotFound              = errors.New("chat session not found")
	ErrHandoffAlreadyActive         = errors.New("chat session already waiting for human handoff")
	ErrNoActiveHandoff              = errors.New("chat session has no active human handoff")
	ErrAuthenticatedUserRequired    = errors.New("authenticated user is required")
	ErrUserProfileNotConfigured     = errors.New("authenticated user profile is not configured")
	ErrInvalidAssignedUser          = errors.New("assigned_user_id must be a valid uuid")
	ErrResolveByRequired            = errors.New("resolved_by_user_id is required")
	ErrInvalidResolveBy             = errors.New("resolved_by_user_id must be a valid uuid")
	ErrResolveRequiresHuman         = errors.New("chat session resolve requires active human ownership")
	ErrResolveOwnerMismatch         = errors.New("resolved_by_user_id does not match current session owner")
	ErrSessionAlreadyResolved       = errors.New("chat session is already resolved")
	ErrReplyBodyRequired            = errors.New("reply.body is required")
	ErrReplyOwnerRequired           = errors.New("owner_user_id is required")
	ErrInvalidReplyOwner            = errors.New("owner_user_id must be a valid uuid")
	ErrInvalidReplyDraft            = errors.New("draft_message_id must be a valid uuid")
	ErrReplyRequiresHuman           = errors.New("chat session reply requires active human ownership")
	ErrReplyOwnerMismatch           = errors.New("owner_user_id does not match current session owner")
	ErrReplyDraftNotAllowed         = errors.New("draft_message_id is not an active automation draft in this session")
	ErrReplyMediaFileRequired       = errors.New("reply.media file is required")
	ErrReplyMediaTypeInvalid        = errors.New("reply.media_type must be IMAGE, AUDIO or DOCUMENT")
	ErrDraftNotFound                = errors.New("chat session has no automation draft")
	ErrDraftAutoSendRetryNotAllowed = errors.New("current draft is not waiting for auto-send retry")
	ErrReplyDeliveryFailed          = errors.New("chat reply delivery failed")
	ErrReprocessRequiresBot         = errors.New("chat session reprocess requires bot ownership")
	ErrReprocessNoMessages          = errors.New("chat session has no pending messages to reprocess")
	ErrAgentRunFailed               = errors.New("chat agent run failed")
	ErrShadowReportSessionRequired  = errors.New("structured interpreter shadow report session_id is required")
)

type Service struct {
	store             Store
	cfg               config.Config
	logger            chatLogger
	sender            ReplySender
	runner            AgentRunner
	jsonRunner        AgentJSONDecisionRunner
	availability      AvailabilitySearcher
	pricing           PricingQuoteSearcher
	bookings          BookingLookupSearcher
	bookingCreate     BookingCreator
	reschedules       RescheduleAssistSearcher
	payments          PaymentStatusSearcher
	paymentCreate     PaymentCreator
	bookingCancel     BookingCanceler
	profiles          AuthUserProfileEnsurer
	openaiInterpreter OpenAIStructuredInterpreter
}

type chatLogger interface {
	Printf(format string, v ...interface{})
}

type AuthUserProfileEnsurer interface {
	EnsureUserProfileIDFromAuth(ctx context.Context, user auth.AuthUser) (string, error)
}

func NewService(store Store, cfg config.Config, deps ...interface{}) *Service {
	var logger chatLogger
	var sender ReplySender
	var runner AgentRunner
	var jsonRunner AgentJSONDecisionRunner
	var availability AvailabilitySearcher
	var pricing PricingQuoteSearcher
	var bookings BookingLookupSearcher
	var bookingCreate BookingCreator
	var reschedules RescheduleAssistSearcher
	var payments PaymentStatusSearcher
	var paymentCreate PaymentCreator
	var bookingCancel BookingCanceler
	var profiles AuthUserProfileEnsurer
	var openaiInterpreter OpenAIStructuredInterpreter
	for _, dep := range deps {
		switch typed := dep.(type) {
		case chatLogger:
			if logger == nil {
				logger = typed
			}
		case ReplySender:
			if sender == nil {
				sender = typed
			}
		case AgentRunner:
			if runner == nil {
				runner = typed
			}
		case AgentJSONDecisionRunner:
			if jsonRunner == nil {
				jsonRunner = typed
			}
		case AvailabilitySearcher:
			if availability == nil {
				availability = typed
			}
		case PricingQuoteSearcher:
			if pricing == nil {
				pricing = typed
			}
		case BookingLookupSearcher:
			if bookings == nil {
				bookings = typed
			}
		case BookingCreator:
			if bookingCreate == nil {
				bookingCreate = typed
			}
		case RescheduleAssistSearcher:
			if reschedules == nil {
				reschedules = typed
			}
		case PaymentStatusSearcher:
			if payments == nil {
				payments = typed
			}
		case PaymentCreator:
			if paymentCreate == nil {
				paymentCreate = typed
			}
		case BookingCanceler:
			if bookingCancel == nil {
				bookingCancel = typed
			}
		case AuthUserProfileEnsurer:
			if profiles == nil {
				profiles = typed
			}

		case OpenAIStructuredInterpreter:
			if openaiInterpreter == nil {
				openaiInterpreter = typed
			}
		}
	}
	return &Service{
		store:             store,
		cfg:               cfg,
		logger:            logger,
		sender:            sender,
		runner:            runner,
		jsonRunner:        jsonRunner,
		availability:      availability,
		pricing:           pricing,
		bookings:          bookings,
		bookingCreate:     bookingCreate,
		reschedules:       reschedules,
		payments:          payments,
		paymentCreate:     paymentCreate,
		bookingCancel:     bookingCancel,
		profiles:          profiles,
		openaiInterpreter: openaiInterpreter,
	}
}

func (s *Service) logReprocess(format string, v ...interface{}) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Printf(format, v...)
}

func (s *Service) Ingest(ctx context.Context, input IngestMessageInput) (IngestMessageResult, error) {
	channel := strings.ToUpper(strings.TrimSpace(input.Channel))
	if channel == "" {
		channel = "WHATSAPP"
	}

	contactKey := strings.TrimSpace(input.ContactKey)
	if contactKey == "" {
		return IngestMessageResult{}, ErrContactKeyRequired
	}

	direction := strings.ToUpper(strings.TrimSpace(input.Message.Direction))
	if direction == "" {
		return IngestMessageResult{}, ErrDirectionRequired
	}
	if direction != "INBOUND" && direction != "OUTBOUND" {
		return IngestMessageResult{}, ErrInvalidDirection
	}

	kind := strings.ToUpper(strings.TrimSpace(input.Message.Kind))
	if kind == "" {
		kind = "TEXT"
	}

	if existing, err := s.store.FindMessageByKeys(ctx, strings.TrimSpace(input.Message.ProviderMessageID), strings.TrimSpace(input.Message.IdempotencyKey)); err != nil {
		return IngestMessageResult{}, err
	} else if existing != nil {
		session, err := s.store.GetSession(ctx, existing.SessionID)
		if err != nil {
			return IngestMessageResult{}, err
		}
		return IngestMessageResult{Session: session, Message: *existing, Idempotent: true}, nil
	}

	now := time.Now().UTC()
	receivedAt := now
	if input.Message.ReceivedAt != nil {
		receivedAt = input.Message.ReceivedAt.UTC()
	}

	var lastMessageAt *time.Time
	var lastInboundAt *time.Time
	var lastOutboundAt *time.Time
	switch direction {
	case "INBOUND":
		lastMessageAt = &receivedAt
		lastInboundAt = &receivedAt
	case "OUTBOUND":
		moment := receivedAt
		if input.Message.SentAt != nil {
			moment = input.Message.SentAt.UTC()
		}
		lastMessageAt = &moment
		lastOutboundAt = &moment
	}

	session, err := s.store.UpsertSession(ctx, UpsertSessionInput{
		Channel:        channel,
		ContactKey:     contactKey,
		CustomerPhone:  strings.TrimSpace(input.CustomerPhone),
		CustomerName:   strings.TrimSpace(input.CustomerName),
		LastMessageAt:  lastMessageAt,
		LastInboundAt:  lastInboundAt,
		LastOutboundAt: lastOutboundAt,
		Metadata:       input.Metadata,
	})
	if err != nil {
		return IngestMessageResult{}, err
	}

	processingStatus := resolveInboundProcessingStatus(
		strings.ToUpper(strings.TrimSpace(input.Message.ProcessingStatus)),
		direction,
		session,
		s.cfg.ChatDebounceWindowMS > 0,
	)
	normalizedPayload := annotateOwnershipBlock(input.Message.NormalizedPayload, session, direction, processingStatus)

	message, err := s.store.CreateMessage(ctx, CreateMessageInput{
		SessionID:         session.ID,
		Direction:         direction,
		Kind:              kind,
		ProviderMessageID: strings.TrimSpace(input.Message.ProviderMessageID),
		IdempotencyKey:    strings.TrimSpace(input.Message.IdempotencyKey),
		SenderName:        strings.TrimSpace(input.Message.SenderName),
		SenderPhone:       strings.TrimSpace(input.Message.SenderPhone),
		Body:              strings.TrimSpace(input.Message.Body),
		Payload:           input.Message.Payload,
		NormalizedPayload: normalizedPayload,
		ProcessingStatus:  processingStatus,
		ReceivedAt:        receivedAt,
		SentAt:            normalizeTimePointer(input.Message.SentAt),
	})
	if err != nil {
		if IsUniqueViolation(err) {
			existing, findErr := s.store.FindMessageByKeys(ctx, strings.TrimSpace(input.Message.ProviderMessageID), strings.TrimSpace(input.Message.IdempotencyKey))
			if findErr != nil {
				return IngestMessageResult{}, findErr
			}
			if existing != nil {
				session, getErr := s.store.GetSession(ctx, existing.SessionID)
				if getErr != nil {
					return IngestMessageResult{}, getErr
				}
				return IngestMessageResult{Session: session, Message: *existing, Idempotent: true}, nil
			}
		}
		return IngestMessageResult{}, err
	}

	bufferState := buildBufferState(
		session.Metadata,
		message,
		time.Duration(s.cfg.ChatDebounceWindowMS)*time.Millisecond,
	)
	if hasHumanOwnership(session) && direction == "INBOUND" {
		bufferState = buildHumanBlockedBufferState(session.Metadata, message, session.CurrentOwnerUserID)
	}

	if session, err = s.store.UpdateSessionBufferState(ctx, UpdateSessionBufferStateInput{
		SessionID: session.ID,
		Buffer:    bufferState,
	}); err != nil {
		return IngestMessageResult{}, err
	}

	return IngestMessageResult{
		Session:    session,
		Message:    message,
		Idempotent: false,
	}, nil
}

func (s *Service) UpdateMessage(ctx context.Context, input UpdateMessageInput) (Message, error) {
	return s.store.UpdateMessage(ctx, input)
}

func (s *Service) QueueAutomationDraft(ctx context.Context, input QueueAutomationDraftInput) (QueueAutomationDraftResult, error) {
	channel := strings.ToUpper(strings.TrimSpace(input.Channel))
	if channel == "" {
		channel = "WHATSAPP"
	}

	contactKey := strings.TrimSpace(input.ContactKey)
	if contactKey == "" {
		return QueueAutomationDraftResult{}, ErrContactKeyRequired
	}

	body := strings.TrimSpace(input.Body)
	if body == "" {
		return QueueAutomationDraftResult{}, ErrDraftBodyRequired
	}

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = "chat-automation-draft-" + uuid.NewString()
	}

	if existing, err := s.store.FindMessageByKeys(ctx, "", idempotencyKey); err != nil {
		return QueueAutomationDraftResult{}, err
	} else if existing != nil {
		session, err := s.store.GetSession(ctx, existing.SessionID)
		if err != nil {
			return QueueAutomationDraftResult{}, err
		}
		return QueueAutomationDraftResult{
			Session:    session,
			Message:    *existing,
			Idempotent: true,
		}, nil
	}

	session, err := s.store.UpsertSession(ctx, UpsertSessionInput{
		Channel:       channel,
		ContactKey:    contactKey,
		CustomerPhone: strings.TrimSpace(input.CustomerPhone),
		CustomerName:  strings.TrimSpace(input.CustomerName),
	})
	if err != nil {
		return QueueAutomationDraftResult{}, err
	}

	senderName := strings.TrimSpace(input.SenderName)
	if senderName == "" {
		senderName = "AUTOMATION"
	}

	observedAt := time.Now().UTC()
	payload, normalizedPayload := buildQueuedAutomationDraftPayload(session, idempotencyKey, input.Metadata, observedAt)
	agentState := buildQueuedAutomationDraftAgentState(session.Metadata, idempotencyKey, input.Metadata, observedAt)
	bufferState := buildDraftGeneratedBufferState(session.Metadata, nil, idempotencyKey, observedAt)

	saved, err := s.store.SaveAgentDraft(ctx, SaveAgentDraftInput{
		SessionID:         session.ID,
		IdempotencyKey:    idempotencyKey,
		Body:              body,
		SenderName:        senderName,
		ProcessingStatus:  messageStatusAutomationDraft,
		Payload:           payload,
		NormalizedPayload: normalizedPayload,
		Agent:             agentState,
		Buffer:            bufferState,
		RecordedAt:        observedAt,
	})
	if err != nil {
		if IsUniqueViolation(err) {
			existing, findErr := s.store.FindMessageByKeys(ctx, "", idempotencyKey)
			if findErr != nil {
				return QueueAutomationDraftResult{}, findErr
			}
			if existing != nil {
				currentSession, getErr := s.store.GetSession(ctx, existing.SessionID)
				if getErr != nil {
					return QueueAutomationDraftResult{}, getErr
				}
				return QueueAutomationDraftResult{
					Session:    currentSession,
					Message:    *existing,
					Idempotent: true,
				}, nil
			}
		}
		return QueueAutomationDraftResult{}, err
	}

	return QueueAutomationDraftResult{
		Session: saved.Session,
		Message: saved.Message,
	}, nil
}

func (s *Service) ListSessions(ctx context.Context, filter ListSessionsFilter) ([]Session, error) {
	filter = normalizeListSessionsFilter(filter)
	filter.ReviewSLASeconds = s.chatReviewSLASeconds()
	items, err := s.store.ListSessions(ctx, filter)
	if err != nil {
		return nil, err
	}
	reviewSLASeconds := s.chatReviewSLASeconds()
	now := time.Now().UTC()
	for i := range items {
		items[i] = decorateSessionDraftSummary(items[i], reviewSLASeconds, now)
	}
	return items, nil
}

func (s *Service) GetSessionsSummary(ctx context.Context, filter ListSessionsFilter) (SessionsSummary, error) {
	filter = normalizeListSessionsFilter(filter)
	filter.Limit = 0
	filter.Offset = 0
	filter.AgentStatus = ""
	filter.DraftReviewStatus = ""
	filter.DraftAutoSendStatus = ""
	filter.OrderBy = ""
	summary, err := s.store.CountSessionsSummary(ctx, filter, s.chatReviewSLASeconds())
	if err != nil {
		return SessionsSummary{}, err
	}
	summary.ReviewSLASeconds = s.chatReviewSLASeconds()
	summary = decorateSessionsSummaryAlert(summary)
	return summary, nil
}

func (s *Service) RequestHandoff(ctx context.Context, input RequestHandoffInput) (RequestHandoffResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return RequestHandoffResult{}, err
	}
	if session.HandoffStatus == "HUMAN_REQUESTED" || session.HandoffStatus == "HUMAN" {
		return RequestHandoffResult{}, ErrHandoffAlreadyActive
	}

	requestedBy := strings.ToUpper(strings.TrimSpace(input.RequestedBy))
	if requestedBy == "" {
		requestedBy = "MANUAL"
	}
	assignedUserID := strings.TrimSpace(input.AssignedUserID)
	if assignedUserID != "" {
		if s.profiles != nil {
			authUser, ok := auth.UserFromContext(ctx)
			if !ok || strings.TrimSpace(authUser.ID) == "" {
				return RequestHandoffResult{}, ErrAuthenticatedUserRequired
			}
			profileID, err := s.profiles.EnsureUserProfileIDFromAuth(ctx, authUser)
			if err != nil {
				return RequestHandoffResult{}, fmt.Errorf("%w: %v", ErrUserProfileNotConfigured, err)
			}
			assignedUserID = profileID
		}
		parsed, err := uuid.Parse(assignedUserID)
		if err != nil {
			return RequestHandoffResult{}, ErrInvalidAssignedUser
		}
		assignedUserID = parsed.String()
	}

	result, err := s.store.RequestHandoff(ctx, RequestHandoffInput{
		SessionID:      sessionID,
		RequestedBy:    requestedBy,
		Reason:         strings.TrimSpace(input.Reason),
		AssignedUserID: assignedUserID,
		Metadata:       input.Metadata,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return RequestHandoffResult{}, ErrUserProfileNotConfigured
		}
		return RequestHandoffResult{}, err
	}
	return result, nil
}

func (s *Service) ResumeSession(ctx context.Context, input ResumeSessionInput) (ResumeSessionResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return ResumeSessionResult{}, err
	}
	if session.HandoffStatus != "HUMAN_REQUESTED" && session.HandoffStatus != "HUMAN" {
		return ResumeSessionResult{}, ErrNoActiveHandoff
	}

	resumedBy := strings.ToUpper(strings.TrimSpace(input.ResumedBy))
	if resumedBy == "" {
		resumedBy = "MANUAL"
	}

	return s.store.ResumeSession(ctx, ResumeSessionInput{
		SessionID: sessionID,
		ResumedBy: resumedBy,
		Reason:    strings.TrimSpace(input.Reason),
		Metadata:  input.Metadata,
	})
}

func (s *Service) ResolveSession(ctx context.Context, input ResolveSessionInput) (ResolveSessionResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return ResolveSessionResult{}, err
	}
	if strings.EqualFold(strings.TrimSpace(session.Status), "RESOLVED") {
		return ResolveSessionResult{}, ErrSessionAlreadyResolved
	}
	if session.HandoffStatus != "HUMAN" || strings.TrimSpace(session.CurrentOwnerUserID) == "" {
		return ResolveSessionResult{}, ErrResolveRequiresHuman
	}

	resolvedBy := strings.TrimSpace(input.ResolvedByUserID)
	if resolvedBy == "" {
		return ResolveSessionResult{}, ErrResolveByRequired
	}
	parsedResolvedBy, err := uuid.Parse(resolvedBy)
	if err != nil {
		return ResolveSessionResult{}, ErrInvalidResolveBy
	}
	resolvedBy = parsedResolvedBy.String()
	if resolvedBy != session.CurrentOwnerUserID {
		return ResolveSessionResult{}, ErrResolveOwnerMismatch
	}

	reason := strings.TrimSpace(input.ResolveReason)
	result, err := s.store.ResolveSession(ctx, ResolveSessionInput{
		SessionID:        sessionID,
		ResolvedByUserID: resolvedBy,
		ResolveReason:    reason,
		Metadata:         input.Metadata,
	})
	if err != nil {
		return ResolveSessionResult{}, err
	}
	if strings.TrimSpace(result.Status) == "" {
		result.Status = "resolved"
	}
	if strings.TrimSpace(result.Reason) == "" {
		result.Reason = "manual_resolve"
	}
	return result, nil
}

func (s *Service) Reply(ctx context.Context, input ReplyInput) (ReplyResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return ReplyResult{}, err
	}
	if session.HandoffStatus != "HUMAN" || strings.TrimSpace(session.CurrentOwnerUserID) == "" {
		return ReplyResult{}, ErrReplyRequiresHuman
	}

	ownerUserID := strings.TrimSpace(input.OwnerUserID)
	if ownerUserID == "" {
		return ReplyResult{}, ErrReplyOwnerRequired
	}
	parsedOwnerID, err := uuid.Parse(ownerUserID)
	if err != nil {
		return ReplyResult{}, ErrInvalidReplyOwner
	}
	ownerUserID = parsedOwnerID.String()
	if ownerUserID != session.CurrentOwnerUserID {
		return ReplyResult{}, ErrReplyOwnerMismatch
	}

	draftMessageID := strings.TrimSpace(input.DraftMessageID)
	if draftMessageID != "" {
		parsedDraftID, err := uuid.Parse(draftMessageID)
		if err != nil {
			return ReplyResult{}, ErrInvalidReplyDraft
		}
		draftMessageID = parsedDraftID.String()
	}

	body := strings.TrimSpace(input.Body)
	if body == "" && draftMessageID == "" {
		return ReplyResult{}, ErrReplyBodyRequired
	}
	input.Body = body
	input.DraftMessageID = draftMessageID

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = "chat-reply-" + uuid.NewString()
	}

	if existing, err := s.store.FindReplyByIdempotency(ctx, sessionID, idempotencyKey); err != nil {
		return ReplyResult{}, err
	} else if existing != nil {
		if s.canDeliverReply(existing.Outbound) {
			delivered, err := s.deliverReply(ctx, *existing)
			if err != nil {
				return ReplyResult{}, err
			}
			delivered.Idempotent = true
			return delivered, nil
		}
		existing.Idempotent = true
		return *existing, nil
	}

	senderName := strings.TrimSpace(input.SenderName)
	if senderName == "" {
		senderName = "HUMAN"
	}

	result, err := s.store.CreateReply(ctx, ReplyInput{
		SessionID:      sessionID,
		OwnerUserID:    ownerUserID,
		DraftMessageID: draftMessageID,
		Body:           body,
		SenderName:     senderName,
		IdempotencyKey: idempotencyKey,
		Metadata:       input.Metadata,
	}, time.Duration(s.cfg.ChatDebounceWindowMS)*time.Millisecond)
	if err != nil {
		if IsUniqueViolation(err) {
			existing, findErr := s.store.FindReplyByIdempotency(ctx, sessionID, idempotencyKey)
			if findErr != nil {
				return ReplyResult{}, findErr
			}
			if existing != nil {
				if s.canDeliverReply(existing.Outbound) {
					delivered, deliverErr := s.deliverReply(ctx, *existing)
					if deliverErr != nil {
						return ReplyResult{}, deliverErr
					}
					delivered.Idempotent = true
					return delivered, nil
				}
				existing.Idempotent = true
				return *existing, nil
			}
		}
		return ReplyResult{}, err
	}

	if s.canDeliverReply(result.Outbound) {
		return s.deliverReply(ctx, result)
	}

	return result, nil
}

func (s *Service) ReplyMedia(ctx context.Context, input ReplyMediaInput) (ReplyMediaResult, error) {
	fileContent := input.FileContent
	if len(fileContent) == 0 {
		return ReplyMediaResult{}, ErrReplyMediaFileRequired
	}

	mediaType, ok := normalizeReplyMediaType(input.MediaType, input.MimeType)
	if !ok {
		return ReplyMediaResult{}, ErrReplyMediaTypeInvalid
	}

	fileName := strings.TrimSpace(input.FileName)
	if fileName == "" {
		fileName = defaultReplyMediaFileName(mediaType, input.MimeType)
	}

	caption := strings.TrimSpace(input.Caption)
	body := caption
	if body == "" {
		body = defaultReplyMediaBody(mediaType)
	}

	metadata := map[string]interface{}{}
	for key, value := range input.Metadata {
		metadata[key] = value
	}
	mimeType := strings.TrimSpace(input.MimeType)
	if mimeType == "" {
		mimeType = defaultReplyMediaMIMEType(mediaType)
	}
	metadata["media_kind"] = strings.ToLower(mediaType)
	metadata["media_mime_type"] = mimeType
	metadata["media_file_name"] = fileName
	metadata["media_base64"] = base64.StdEncoding.EncodeToString(fileContent)
	if caption != "" {
		metadata["media_caption"] = caption
	}

	return s.Reply(ctx, ReplyInput{
		SessionID:      input.SessionID,
		OwnerUserID:    input.OwnerUserID,
		Body:           body,
		SenderName:     input.SenderName,
		IdempotencyKey: input.IdempotencyKey,
		Metadata:       metadata,
	})
}

func (s *Service) Reprocess(ctx context.Context, input ReprocessInput) (ReprocessResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return ReprocessResult{}, err
	}
	trigger := strings.ToUpper(strings.TrimSpace(input.Trigger))
	if trigger == "" {
		trigger = "MANUAL"
	}
	jobRunID := strings.TrimSpace(asString(input.Metadata["job_run_id"]))
	s.logReprocess(
		"chat reprocess event=start session_id=%s contact_key=%s trigger=%s job_run_id=%s handoff_status=%s current_owner_user_id=%s",
		session.ID,
		strings.TrimSpace(session.ContactKey),
		trigger,
		jobRunID,
		strings.TrimSpace(session.HandoffStatus),
		strings.TrimSpace(session.CurrentOwnerUserID),
	)
	if session.HandoffStatus != "BOT" || strings.TrimSpace(session.CurrentOwnerUserID) != "" {
		return ReprocessResult{}, ErrReprocessRequiresBot
	}

	history, err := s.store.ListMessages(ctx, sessionID, normalizeListMessagesFilter(ListMessagesFilter{Limit: 50}))
	if err != nil {
		return ReprocessResult{}, err
	}

	candidates := selectReprocessCandidateMessages(history)
	s.logReprocess(
		"chat reprocess event=candidates_selected session_id=%s trigger=%s job_run_id=%s candidate_count=%d history_count=%d",
		session.ID,
		trigger,
		jobRunID,
		len(candidates),
		len(history),
	)
	if len(candidates) == 0 {
		if existingDraft := findLatestDraftMessage(history); existingDraft != nil {
			result := ReprocessResult{
				Session:    session,
				Status:     "accepted",
				Reason:     "draft_already_generated",
				Draft:      existingDraft,
				Idempotent: true,
			}
			return s.finishReprocessWithAutoSend(ctx, result, trigger, jobRunID)
		}
		return ReprocessResult{}, ErrReprocessNoMessages
	}

	observedAt := time.Now().UTC()
	memory := buildReprocessMemory(session, history, candidates, observedAt)
	agentState := buildReprocessAgentState(session, candidates, trigger, observedAt, input.Metadata)
	buffer := buildReprocessBufferState(session.Metadata, candidates, trigger, observedAt)
	untranscribedAudioMessage, untranscribedAudio := currentTurnUntranscribedAudioCandidate(candidates)
	currentTurn := NormalizeIncomingCustomerText(strings.TrimSpace(asString(memory["current_turn_body"])))
	structuredCanonicalState := deriveCanonicalConversationState(session, history, currentTurn)

	canonicalState := CanonicalConversationState{}
	if canonicalStateEnabled() {
		canonicalState = structuredCanonicalState
		agentState["canonical_state"] = canonicalState
	}

	structuredInput := StructuredInterpreterInput{
		CurrentTurn: currentTurn,
		State:       structuredCanonicalState,
		History:     history,
		ObservedAt:  observedAt,
	}

	localInterpretation := InterpretStructuredTurn(structuredInput)

	shadow := RunStructuredInterpreterShadow(ctx, StructuredInterpreterShadowInput{
		Enabled:             s.cfg.ChatOpenAIInterpreterShadowEnabled,
		OpenAIInterpreter:   s.openaiInterpreter,
		StructuredInput:     structuredInput,
		LocalInterpretation: localInterpretation,
		IdempotencyKey:      buildStructuredInterpreterShadowIdempotencyKey(session.ID, candidates),
	})

	memory[structuredInterpreterShadowKey] = shadow
	agentState[structuredInterpreterShadowKey] = shadow
	if looksLikeBareCPF(currentTurn) {
		s.logReprocess(
			"chat reprocess event=cpf_reply_detected session_id=%s trigger=%s job_run_id=%s phase=%s last_bot_asked_payer_cpf=%t document=%s",
			session.ID,
			trigger,
			jobRunID,
			canonicalState.Phase,
			lastAssistantAskedPayerCPF(history),
			maskDocumentForLog(currentTurn),
		)
	}

	messageMetadata := map[string]interface{}{
		"agent_ready_for_automation": true,
		"agent_status":               agentStatusReadyForAutomation,
		"automation_trigger":         trigger,
		"automation_requested_at":    observedAt.Format(time.RFC3339Nano),
		"current_turn_message_ids":   candidateMessageIDs(candidates),
		"current_turn_body":          joinCandidateBodies(candidates),
	}
	if interpretation, ok := availabilityDraftTurnInterpretation(session, currentTurn, observedAt); ok {
		messageMetadata["interpretation"] = interpretation
	}
	if untranscribedAudio {
		messageMetadata["auto_send_status"] = draftAutoSendStatusReviewNeeded
		messageMetadata["auto_send_reasons"] = []string{draftAutoSendReasonNonTextTurn}
	}

	s.logReprocess(
		"chat reprocess event=save_reprocess_snapshot_start session_id=%s trigger=%s job_run_id=%s message_count=%d",
		session.ID,
		trigger,
		jobRunID,
		len(candidates),
	)
	persisted, err := s.store.SaveReprocessSnapshot(ctx, SaveReprocessSnapshotInput{
		SessionID:       sessionID,
		MessageIDs:      candidateMessageIDs(candidates),
		MessageStatus:   messageStatusAutomationPending,
		MessageMetadata: messageMetadata,
		Memory:          memory,
		Agent:           agentState,
		Buffer:          buffer,
	})
	if err != nil {
		s.logReprocess(
			"chat reprocess event=save_reprocess_snapshot_failed session_id=%s trigger=%s job_run_id=%s error=%v",
			session.ID,
			trigger,
			jobRunID,
			err,
		)
		return ReprocessResult{}, err
	}
	s.logReprocess(
		"chat reprocess event=save_reprocess_snapshot_done session_id=%s trigger=%s job_run_id=%s message_count=%d",
		persisted.Session.ID,
		trigger,
		jobRunID,
		len(persisted.Messages),
	)

	result := ReprocessResult{
		Session:  persisted.Session,
		Status:   "accepted",
		Reason:   "automation_pending",
		Memory:   memory,
		Messages: persisted.Messages,
	}
	if untranscribedAudio {
		bodyPresent := strings.TrimSpace(untranscribedAudioMessage.Body) != ""
		transcriptionStatus := strings.ToUpper(strings.TrimSpace(asString(untranscribedAudioMessage.NormalizedPayload["transcription_status"])))
		s.logReprocess(
			"chat reprocess event=runner_skipped session_id=%s trigger=%s job_run_id=%s reason=untranscribed_audio message_id=%s transcription_status=%s body_present=%t current_turn_count=%d",
			persisted.Session.ID,
			trigger,
			jobRunID,
			untranscribedAudioMessage.ID,
			transcriptionStatus,
			bodyPresent,
			len(candidates),
		)
		result.Reason = "review_required"
		return result, nil
	}
	draftID := buildAgentDraftIdempotencyKey(sessionID, candidateMessageIDs(candidates))
	if existing, err := s.store.FindMessageByKeys(ctx, "", draftID); err != nil {
		return ReprocessResult{}, err
	} else if existing != nil && existing.SessionID == sessionID && existing.Direction == "OUTBOUND" && isAutomationDraftStatus(existing.ProcessingStatus) {
		currentSession, getErr := s.GetSession(ctx, sessionID)
		if getErr != nil {
			return ReprocessResult{}, getErr
		}
		result.Session = currentSession
		result.Draft = existing
		result.Idempotent = true
		result.Reason = "draft_already_generated"
		return s.finishReprocessWithAutoSend(ctx, result, trigger, jobRunID)
	}

	systemPrompt := buildAgentSystemPrompt()
	agentMode := s.chatAgentMode()
	phaseBefore := canonicalState.Phase
	rolloutMetadata := chatAgentRolloutMetadata{
		Mode:                 agentMode,
		CanonicalPhaseBefore: phaseBefore,
		CanonicalPhaseAfter:  canonicalState.Phase,
	}
	unsupportedCargo, unsupportedCargoHandled := inferUnsupportedCargoQuery(currentTurn)
	if !unsupportedCargoHandled && !s.canRunAgent() && !jsonDecisionLayerEnabledForMode(agentMode) {
		s.logReprocess(
			"chat reprocess event=runner_disabled session_id=%s trigger=%s job_run_id=%s reason=agent_runner_unavailable",
			persisted.Session.ID,
			trigger,
			jobRunID,
		)
		return result, nil
	}
	passengerCountContext := lastBotAskedPassengerCount(history)
	if passengerCountContext {
		passengerCountContext = !lastBotAskedRouteAndPassengerCollection(history)
	}
	if passengerCountContext && looksLikeHumanSupportIntent(strings.Join(strings.Fields(foldChatText(currentTurn)), " ")) {
		passengerCountContext = false
	}

	toolContext := agentToolContext{}

	var deterministicBookingRun *RunAgentResult
	var deterministicBookingHandled bool
	var deterministicToolHandled bool
	var deterministicBookingAction BookingNextAction
	documentCollectionMediaTurn := shouldRunDocumentExtract(memory) && s.canRunAgent()
	documentAttempted := false
	documentHandled := false
	if documentCollectionMediaTurn && !unsupportedCargoHandled {
		documentAttempted = true
		documentContext, handled, err := s.resolveDocumentExtractContext(ctx, persisted.Session, candidates, memory, draftID)
		if err != nil {
			return ReprocessResult{}, err
		}
		documentHandled = handled
		toolContext = mergeAgentToolContexts(toolContext, documentContext)
		result.ToolCalls = toolContext.Calls
	}
	if !documentHandled {
		if draftSession, draftToolContext, draftRun, handled, err := s.resolveAvailabilityDraftTurn(ctx, persisted.Session, history, currentTurn, observedAt); err != nil {
			return ReprocessResult{}, err
		} else if handled {
			persisted.Session = draftSession
			toolContext = mergeAgentToolContexts(toolContext, draftToolContext)
			result.ToolCalls = toolContext.Calls
			deterministicToolHandled = len(draftToolContext.Calls) > 0
			if toolContext.Availability != nil {
				toolFacts := map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(*toolContext.Availability),
				}
				mergeToolFactsIntoCanonicalState(&canonicalState, toolFacts)
				agentState["canonical_state"] = canonicalState
				memory["canonical_state"] = canonicalState
			}
			if draftRun != nil {
				deterministicBookingRun = draftRun
				deterministicBookingHandled = true
			}
		}
	}
	if passengerCountContext && !deterministicBookingHandled && !documentHandled {
		s.logReprocess(
			"chat reprocess event=passenger_count_context_detected session_id=%s trigger=%s job_run_id=%s",
			persisted.Session.ID,
			trigger,
			jobRunID,
		)
		currentTurnSlots := parsePassengerClarificationSlots(currentTurn)
		currentTurnAudioTranscript := currentTurnHasCompletedAudioTranscript(candidates)
		currentTurnSlotsDetected := currentTurnSlots.PassengerCountKnown || currentTurnSlots.ChildUnder5CountKnown
		bookingDraft := collectBookingDraftContext(persisted.Session, history, currentTurn)
		memory["passenger_count_reply_context"] = "true"
		memory["passenger_count_reply_parsed"] = bookingDraft.PassengerCountKnown || bookingDraft.ChildUnder5CountKnown
		memory["passenger_count"] = bookingDraft.PassengerCount
		memory["passenger_count_known"] = bookingDraft.PassengerCountKnown
		memory["child_under_5_count"] = bookingDraft.ChildUnder5Count
		memory["child_under_5_count_known"] = bookingDraft.ChildUnder5CountKnown

		memory["booking_draft_context"] = map[string]interface{}{
			"origin":                         bookingDraft.Origin,
			"destination":                    bookingDraft.Destination,
			"selected_option_index":          bookingDraft.SelectedOptionIndex,
			"trip_id":                        bookingDraft.TripID,
			"board_stop_id":                  bookingDraft.BoardStopID,
			"alight_stop_id":                 bookingDraft.AlightStopID,
			"trip_date":                      bookingDraft.TripDate,
			"departure_time":                 bookingDraft.DepartureTime,
			"price":                          bookingDraft.Price,
			"currency":                       bookingDraft.Currency,
			"passenger_count":                bookingDraft.PassengerCount,
			"passenger_count_known":          bookingDraft.PassengerCountKnown,
			"child_under_5_count":            bookingDraft.ChildUnder5Count,
			"child_under_5_count_known":      bookingDraft.ChildUnder5CountKnown,
			"lap_child_assignment_known":     bookingDraft.LapChildAssignmentKnown,
			"lap_child_passenger_indexes":    bookingDraft.LapChildPassengerIndexes,
			"needs_lap_child_assignment":     bookingDraft.NeedsLapChildAssignment,
			"has_availability_shown":         bookingDraft.HasAvailabilityShown,
			"asked_passenger_question":       bookingDraft.AskedPassengerQuestion,
			"passenger_count_context_active": bookingDraft.PassengerCountContextActive,
		}

		deterministicBookingAction = decideNextBookingStep(bookingDraft)

		s.logReprocess(
			"chat reprocess event=booking_draft_context_collected session_id=%s trigger=%s job_run_id=%s action=%s origin=%q destination=%q trip_date=%q trip_id_present=%t passenger_count=%d passenger_count_known=%t child_under_5_count=%d child_under_5_count_known=%t",
			persisted.Session.ID,
			trigger,
			jobRunID,
			deterministicBookingAction,
			bookingDraft.Origin,
			bookingDraft.Destination,
			bookingDraft.TripDate,
			strings.TrimSpace(bookingDraft.TripID) != "",
			bookingDraft.PassengerCount,
			bookingDraft.PassengerCountKnown,
			bookingDraft.ChildUnder5Count,
			bookingDraft.ChildUnder5CountKnown,
		)

		if deterministicBookingAction == BookingNextAskPassengerClarification &&
			currentTurnAudioTranscript &&
			!currentTurnSlotsDetected {
			reply := "Nao consegui entender o audio com seguranca. Pode escrever se a passagem e so para voce ou se vai mais alguem junto? Exemplo: so eu ou eu e mais uma pessoa."
			run := buildBookingContinuationDraftRun(reply, deterministicBookingAction, bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
			memory["audio_passenger_slots_detected"] = false
			memory["passenger_count_reply_parsed"] = false
		}

		if !deterministicBookingHandled && shouldReplyPassengerDocumentsAlreadySentNotRecognized(currentTurn, bookingDraft) {
			reply := buildPassengerDocumentsAlreadySentNotRecognizedReply(bookingDraft)
			run := buildBookingContinuationDraftRun(reply, BookingNextAskPassengerDocuments, bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		}

		if !deterministicBookingHandled &&
			canDraftPassengerDocumentConfirmation(bookingDraft, deterministicBookingAction) &&
			shouldDraftPassengerDocumentConfirmation(persisted.Session, history, currentTurn, bookingDraft) {
			run := buildPassengerDocumentConfirmationDraftRun(bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		}

		if !deterministicBookingHandled && deterministicBookingAction == BookingNextCallCreate {
			updatedContext, used, err := s.resolveContextualActionTools(
				ctx,
				persisted.Session,
				history,
				currentTurn,
				toolContext,
			)
			if err != nil {
				return ReprocessResult{}, err
			}

			if used {
				toolContext = updatedContext
				result.ToolCalls = toolContext.Calls
				deterministicToolHandled = len(toolContext.Calls) > 0

				if toolContext.BookingCreate != nil {
					run := buildBookingCreatedDraftRun(*toolContext.BookingCreate)
					deterministicBookingRun = &run
					deterministicBookingHandled = true
				}
			} else {
				reply := buildBookingCreateMissingDataReply(bookingDraft)
				run := buildBookingContinuationDraftRun(reply, BookingNextAskPassengerDocuments, bookingDraft)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		}

		if !deterministicBookingHandled && deterministicBookingAction != BookingNextCallCreate {
			reply := buildBookingContinuationReply(bookingDraft, deterministicBookingAction)
			if strings.TrimSpace(reply) != "" {
				run := buildBookingContinuationDraftRun(reply, deterministicBookingAction, bookingDraft)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		}

		s.logReprocess(
			"chat reprocess event=passenger_slots_collected session_id=%s trigger=%s job_run_id=%s passenger_count=%d passenger_count_known=%t child_under_5_count=%d child_under_5_count_known=%t",
			persisted.Session.ID,
			trigger,
			jobRunID,
			bookingDraft.PassengerCount,
			bookingDraft.PassengerCountKnown,
			bookingDraft.ChildUnder5Count,
			bookingDraft.ChildUnder5CountKnown,
		)

	}
	if !passengerCountContext && !deterministicBookingHandled && !documentHandled {
		bookingDraft := collectBookingDraftContext(persisted.Session, history, currentTurn)
		passengerDocumentFlowContext := shouldTreatAsPassengerDocumentFlow(canonicalState.Phase, history, currentTurn, persisted.Session)
		if passengerDocumentFlowContext {
			if _, ok := parsePaymentCreateInput(persisted.Session, history, currentTurn, nil, nil); ok {
				passengerDocumentFlowContext = false
			}
		}
		bookingAction := decideNextBookingStep(bookingDraft)
		canHandlePassengerDocumentTurn := bookingAction != BookingNextAskPassengerClarification && bookingAction != BookingNextAwaitTripSelection
		if passengerDocumentFlowContext && canHandlePassengerDocumentTurn && hasDocumentExtractPDFMedia(collectCandidateMedia(candidates)) {
			run := buildUnsupportedPDFDocumentDraftRun(bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		} else if passengerDocumentFlowContext && canHandlePassengerDocumentTurn && looksLikeInvalidPassengerCPF(currentTurn) {
			run := buildInvalidPassengerCPFDraftRun(bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		} else if passengerDocumentFlowContext && canHandlePassengerDocumentTurn && shouldAskPassengerNameAfterCPF(persisted.Session, currentTurn, bookingDraft) {
			run := buildAskPassengerNameAfterCPFDraftRun(bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		}
		if !deterministicBookingHandled &&
			passengerDocumentFlowContext &&
			canHandlePassengerDocumentTurn &&
			shouldReplyPassengerDocumentsAlreadySentNotRecognized(currentTurn, bookingDraft) {
			reply := buildPassengerDocumentsAlreadySentNotRecognizedReply(bookingDraft)
			run := buildBookingContinuationDraftRun(reply, BookingNextAskPassengerDocuments, bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		}
		if !deterministicBookingHandled &&
			passengerDocumentFlowContext &&
			canHandlePassengerDocumentTurn &&
			canDraftPassengerDocumentConfirmation(bookingDraft, bookingAction) &&
			shouldDraftPassengerDocumentConfirmation(persisted.Session, history, currentTurn, bookingDraft) {
			run := buildPassengerDocumentConfirmationDraftRun(bookingDraft)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		}
		if !deterministicBookingHandled &&
			passengerDocumentFlowContext &&
			canHandlePassengerDocumentTurn &&
			bookingAction == BookingNextAskPassengerDocuments {
			reply := buildBookingContinuationReply(bookingDraft, bookingAction)
			if strings.TrimSpace(reply) != "" {
				run := buildBookingContinuationDraftRun(reply, bookingAction, bookingDraft)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		}
		if !deterministicBookingHandled && bookingAction == BookingNextCallCreate {
			updatedContext, used, err := s.resolveContextualActionTools(
				ctx,
				persisted.Session,
				history,
				currentTurn,
				toolContext,
			)
			if err != nil {
				return ReprocessResult{}, err
			}
			if used {
				toolContext = updatedContext
				result.ToolCalls = toolContext.Calls
				deterministicToolHandled = len(toolContext.Calls) > 0

				if toolContext.BookingCreate != nil {
					run := buildBookingCreatedDraftRun(*toolContext.BookingCreate)
					deterministicBookingRun = &run
					deterministicBookingHandled = true
				} else if toolContext.PaymentCreate != nil {
					run := buildPaymentCreateDraftRun(*toolContext.PaymentCreate)
					deterministicBookingRun = &run
					deterministicBookingHandled = true
				}
			} else if canDraftPassengerDocumentConfirmation(bookingDraft, bookingAction) &&
				shouldDraftPassengerDocumentConfirmation(persisted.Session, history, currentTurn, bookingDraft) {
				run := buildPassengerDocumentConfirmationDraftRun(bookingDraft)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			} else if passengerDocumentFlowContext && canHandlePassengerDocumentTurn {
				reply := buildBookingCreateMissingDataReply(bookingDraft)
				run := buildBookingContinuationDraftRun(reply, BookingNextAskPassengerDocuments, bookingDraft)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		}
		if !deterministicBookingHandled && bookingAction == BookingNextAskLapChildAssignment {
			reply := buildBookingContinuationReply(bookingDraft, bookingAction)
			if strings.TrimSpace(reply) != "" {
				run := buildBookingContinuationDraftRun(reply, bookingAction, bookingDraft)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
				memory["booking_draft_context"] = map[string]interface{}{
					"passenger_count":             bookingDraft.PassengerCount,
					"passenger_count_known":       bookingDraft.PassengerCountKnown,
					"child_under_5_count":         bookingDraft.ChildUnder5Count,
					"child_under_5_count_known":   bookingDraft.ChildUnder5CountKnown,
					"lap_child_assignment_known":  bookingDraft.LapChildAssignmentKnown,
					"lap_child_passenger_indexes": bookingDraft.LapChildPassengerIndexes,
					"needs_lap_child_assignment":  bookingDraft.NeedsLapChildAssignment,
					"passenger_details_count":     bookingDraft.PassengerDetailsCount,
				}
			}
		}
	}
	if !deterministicBookingHandled && !deterministicToolHandled && !documentHandled && s.canCreateBookings() {
		createInput, ok := parseBookingCreateFromDocumentConfirmation(persisted.Session, history, currentTurn)
		if ok {
			updatedContext, err := s.executeBookingCreateTool(ctx, persisted.Session, toolContext, createInput)
			if err != nil {
				return ReprocessResult{}, err
			}
			toolContext = updatedContext
			result.ToolCalls = toolContext.Calls
			deterministicToolHandled = len(toolContext.Calls) > 0
			if toolContext.BookingCreate != nil {
				run := buildBookingCreatedDraftRun(*toolContext.BookingCreate)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		}
	}
	if !deterministicBookingHandled && !deterministicToolHandled && !documentHandled && s.canCreatePayments() {
		paymentInput, ok := parsePaymentCreateInput(
			persisted.Session,
			history,
			currentTurn,
			nil,
			nil,
		)
		if ok {
			updatedContext, err := s.executePaymentCreateTool(
				ctx,
				persisted.Session,
				toolContext,
				paymentInput,
			)
			if err != nil {
				return ReprocessResult{}, err
			}

			toolContext = updatedContext
			result.ToolCalls = toolContext.Calls
			deterministicToolHandled = len(toolContext.Calls) > 0

			if toolContext.PaymentCreate != nil {
				run := buildPaymentCreateDraftRun(*toolContext.PaymentCreate)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		}
	}
	unsupportedPackage, unsupportedPackageHandled := inferUnsupportedPackageQuery(currentTurn)
	if looksLikeReservationHowToProceedIntent(currentTurn) || looksLikeVerifyAllOptionsIntent(currentTurn) {
		unsupportedPackageHandled = false
	}
	if documentCollectionMediaTurn {
		if unsupportedPackageHandled {
			s.logReprocess(
				"chat reprocess event=fallback_out_of_service_blocked_reason=document_collection_media_turn session_id=%s trigger=%s job_run_id=%s",
				persisted.Session.ID,
				trigger,
				jobRunID,
			)
		}
		unsupportedPackageHandled = false
	} else if passengerCountContext {
		if unsupportedPackageHandled {
			s.logReprocess(
				"chat reprocess event=fallback_out_of_service_blocked_reason=passenger_count_context session_id=%s trigger=%s job_run_id=%s",
				persisted.Session.ID,
				trigger,
				jobRunID,
			)
		}
		unsupportedPackageHandled = false
	} else if shouldTreatAsPassengerDocumentFlow(canonicalState.Phase, history, currentTurn, persisted.Session) {
		if unsupportedPackageHandled {
			s.logReprocess(
				"chat reprocess event=fallback_out_of_service_blocked_reason=passenger_document_flow session_id=%s trigger=%s job_run_id=%s phase=%s",
				persisted.Session.ID,
				trigger,
				jobRunID,
				canonicalState.Phase,
			)
		}
		unsupportedPackageHandled = false
	}
	deterministicDecision := IntentDecision{Intent: IntentUnknown}
	if !unsupportedCargoHandled && !unsupportedPackageHandled && !deterministicBookingHandled && !documentCollectionMediaTurn {
		if intentRouterEnabled() && templateRealizerEnabled() {
			decision := routeDeterministicIntent(history, currentTurn, canonicalState, observedAt)
			deterministicDecision = decision
			if decision.Intent != IntentUnknown {
				s.logReprocess(
					"chat reprocess event=intent_router_decision session_id=%s trigger=%s job_run_id=%s intent=%s intent_source=%s phase_before=%s action=%s template_name=%s",
					persisted.Session.ID,
					trigger,
					jobRunID,
					decision.Intent,
					decision.Source,
					canonicalState.Phase,
					decision.Action,
					decision.TemplateName,
				)
			}
			if looksLikeBareCPF(currentTurn) {
				s.logReprocess(
					"chat reprocess event=cpf_reply_routed session_id=%s trigger=%s job_run_id=%s phase=%s last_bot_asked_payer_cpf=%t intent=%s intent_source=%s document=%s",
					persisted.Session.ID,
					trigger,
					jobRunID,
					canonicalState.Phase,
					lastAssistantAskedPayerCPF(history),
					decision.Intent,
					decision.Source,
					maskDocumentForLog(currentTurn),
				)
			}

			if !deterministicBookingHandled && decision.Action == "safe_fallback" {
				bookingDraft := collectBookingDraftContext(persisted.Session, history, currentTurn)
				reply, ok := buildSafeFallbackReplyForPhase(canonicalState, bookingDraft, currentTurn)
				if ok && strings.TrimSpace(reply) != "" {
					run := buildSafeFallbackDraftRun(reply, canonicalState, decision.Source)
					deterministicBookingRun = &run
					deterministicBookingHandled = true
					rolloutMetadata.DecisionSource = "deterministic"
					rolloutMetadata.DecisionValid = boolPtr(true)
					rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
				}
			}

			if decision.Intent == IntentSelectAvailabilityOption &&
				decision.SelectedOptionIndex > 0 &&
				decision.TemplateName == TemplateAskPassengerCount &&
				hasPreviousAvailabilityList(history) {

				reply, ok := realizeIntentResponseTemplate(decision)
				if ok && strings.TrimSpace(reply) != "" {
					canonicalState = applyIntentDecisionToCanonicalState(canonicalState, decision)
					agentState["canonical_state"] = canonicalState
					memory["canonical_state"] = canonicalState
					memory["intent_decision"] = map[string]interface{}{
						"intent":                string(decision.Intent),
						"intent_source":         decision.Source,
						"selected_option_index": decision.SelectedOptionIndex,
						"template_name":         string(decision.TemplateName),
						"action":                decision.Action,
					}

					run := buildTemplateDraftRunFromDecision(decision, reply)
					deterministicBookingRun = &run
					deterministicBookingHandled = true
					rolloutMetadata.DecisionSource = "deterministic"
					rolloutMetadata.DecisionValid = boolPtr(true)
					rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
				}
			}
			if !deterministicBookingHandled && decision.Action == "tool" && decision.Intent == IntentAvailabilitySearch && decision.AvailabilityInput != nil {
				context, err := s.executeAvailabilitySearchIntentTool(ctx, persisted.Session, *decision.AvailabilityInput)
				if err != nil {
					return ReprocessResult{}, err
				}

				toolContext = mergeAgentToolContexts(toolContext, context)
				result.ToolCalls = toolContext.Calls
				deterministicToolHandled = len(toolContext.Calls) > 0

				toolFacts := map[string]interface{}{}
				if toolContext.Availability != nil {
					toolFacts[toolNameAvailabilitySearch] = buildAvailabilityToolResponsePayload(*toolContext.Availability)
				}
				mergeToolFactsIntoCanonicalState(&canonicalState, toolFacts)
				agentState["canonical_state"] = canonicalState
				memory["canonical_state"] = canonicalState
				memory["intent_decision"] = map[string]interface{}{
					"intent":        string(decision.Intent),
					"intent_source": decision.Source,
					"action":        decision.Action,
				}

				toolContext, err = s.executePricingQuoteFromAvailabilityTool(ctx, persisted.Session, toolContext, currentTurn)
				if err != nil {
					return ReprocessResult{}, err
				}
				result.ToolCalls = toolContext.Calls
				rolloutMetadata.DecisionSource = "deterministic"
				rolloutMetadata.DecisionValid = boolPtr(true)
				rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
				rolloutMetadata.ToolCallCount = len(toolContext.Calls)
				toolContext = mergeToolCallRequestMetadata(toolContext, rolloutMetadata)
				if canRealizeAvailabilityToolDecisionWithoutLLM(decision, toolContext) {
					run := buildAvailabilityTemplateDraftRun(decision, *toolContext.Availability)
					deterministicBookingRun = &run
					deterministicBookingHandled = true
					memory["intent_decision"] = map[string]interface{}{
						"intent":        string(decision.Intent),
						"intent_source": decision.Source,
						"action":        "tool_template",
						"template_name": asString(run.RequestPayload["template_name"]),
					}
				}
			}
			if !deterministicBookingHandled && canRealizeWithoutLLM(decision, canonicalState) {
				reply, ok := realizeIntentResponseTemplate(decision)
				if ok {
					canonicalState = applyIntentDecisionToCanonicalState(canonicalState, decision)
					agentState["canonical_state"] = canonicalState
					memory["canonical_state"] = canonicalState
					memory["intent_decision"] = map[string]interface{}{
						"intent":                string(decision.Intent),
						"intent_source":         decision.Source,
						"selected_option_index": decision.SelectedOptionIndex,
						"template_name":         string(decision.TemplateName),
					}
					run := buildTemplateDraftRunFromDecision(decision, reply)
					deterministicBookingRun = &run
					deterministicBookingHandled = true
					rolloutMetadata.DecisionSource = "deterministic"
					rolloutMetadata.DecisionValid = boolPtr(true)
					rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
				}
			}
		}
		if !unsupportedCargoHandled && !unsupportedPackageHandled && !deterministicBookingHandled && !deterministicToolHandled && !documentCollectionMediaTurn {
			bookingDraft := collectBookingDraftContext(persisted.Session, history, currentTurn)
			if shouldUseSafePhaseFallbackForCurrentTurn(canonicalState, bookingDraft, history) &&
				shouldApplySafeFallbackAfterDecision(deterministicDecision) {
				reply, ok := buildSafeFallbackReplyForPhase(canonicalState, bookingDraft, currentTurn)
				if ok && strings.TrimSpace(reply) != "" {
					run := buildSafeFallbackDraftRun(reply, canonicalState, "protected_phase_no_deterministic_decision")
					deterministicBookingRun = &run
					deterministicBookingHandled = true
					rolloutMetadata.DecisionSource = "safe_phase_fallback"
					rolloutMetadata.DecisionValid = boolPtr(true)
					rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
				}
			}
		}
	}
	if !unsupportedCargoHandled && !unsupportedPackageHandled && !deterministicBookingHandled && !deterministicToolHandled && !documentCollectionMediaTurn && jsonDecisionLayerEnabledForMode(agentMode) && s.canRunJSONDecisionAgent() {
		compactInput := buildJSONDecisionCompactInput(currentTurn, canonicalState, history)
		decision, jsonRun, err := s.jsonRunner.RunIntentDecision(ctx, RunJSONDecisionInput{
			SystemPrompt:   buildJSONDecisionSystemPrompt(),
			CompactInput:   compactInput,
			SchemaName:     "intent_decision",
			Schema:         intentDecisionJSONSchema(),
			IdempotencyKey: draftID + ":intent_decision",
			Session:        persisted.Session,
		})
		if err != nil {
			rolloutMetadata.DecisionSource = "json_agent"
			rolloutMetadata.DecisionValid = boolPtr(false)
			rolloutMetadata.ValidationErrors = []string{"json_decision_runner_error"}
			rolloutMetadata.FallbackReason = "json_decision_runner_error"
			s.logReprocess(
				"chat reprocess event=json_decision_failed session_id=%s trigger=%s job_run_id=%s error=%v",
				persisted.Session.ID,
				trigger,
				jobRunID,
				err,
			)
			if agentMode == chatAgentModeJSONOnly {
				validation := AgentDecisionValidationResult{
					Decision: IntentDecisionJSON{Action: jsonDecisionActionClarify, Intent: string(IntentUnknown)},
					Valid:    false,
					Reasons:  []string{"json_decision_runner_error"},
				}
				run := buildJSONDecisionClarificationDraftRun(validation, rolloutMetadata)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		} else {
			validated := validateAgentIntentDecision(decision, canonicalState)
			rolloutMetadata.DecisionSource = "json_agent"
			rolloutMetadata.DecisionValid = boolPtr(validated.Valid)
			rolloutMetadata.DecisionConfidence = float64Ptr(validated.Decision.Confidence)
			rolloutMetadata.ValidationErrors = append([]string(nil), validated.Reasons...)
			if !validated.Valid {
				rolloutMetadata.FallbackReason = strings.Join(validated.Reasons, ",")
			}
			memory["json_intent_decision"] = map[string]interface{}{
				"domain":               validated.Decision.Domain,
				"intent":               validated.Decision.Intent,
				"action":               validated.Decision.Action,
				"confidence":           validated.Decision.Confidence,
				"valid":                validated.Valid,
				"reasons":              validated.Reasons,
				"provider_response_id": jsonRun.ProviderResponseID,
			}
			agentState["json_intent_decision"] = memory["json_intent_decision"]
			if updatedSession, persistErr := s.persistOpenAIContinuityMetadata(ctx, persisted.Session, jsonRun.ProviderResponseID, jsonRun.ProviderConversationID, jsonRun.Model); persistErr != nil {
				return ReprocessResult{}, persistErr
			} else {
				persisted.Session = updatedSession
			}
			s.logReprocess(
				"chat reprocess event=json_decision_validated session_id=%s trigger=%s job_run_id=%s intent=%s action=%s valid=%t",
				persisted.Session.ID,
				trigger,
				jobRunID,
				validated.Decision.Intent,
				validated.Decision.Action,
				validated.Valid,
			)
			if validated.Valid {
				if specialistAgentsEnabled() && validated.Decision.Action == jsonDecisionActionSpecialist && s.canRunSpecialistPlanner() {
					plannerRun, plannerErr := s.runSpecialistPlanner(ctx, persisted.Session, validated.Decision, currentTurn, canonicalState, history, draftID)
					if plannerErr != nil {
						rolloutMetadata.FallbackReason = "specialist_planner_runner_error"
						rolloutMetadata.ValidationErrors = append(rolloutMetadata.ValidationErrors, "specialist_planner_runner_error")
						s.logReprocess(
							"chat reprocess event=specialist_planner_failed session_id=%s trigger=%s job_run_id=%s domain=%s error=%v",
							persisted.Session.ID,
							trigger,
							jobRunID,
							validated.Decision.Domain,
							plannerErr,
						)
					} else {
						memory["specialist_action_plan"] = map[string]interface{}{
							"domain":               plannerRun.Domain,
							"valid":                plannerRun.Validation.Valid,
							"reasons":              plannerRun.Validation.Reasons,
							"provider_response_id": plannerRun.JSONRun.ProviderResponseID,
						}
						agentState["specialist_action_plan"] = memory["specialist_action_plan"]
						if updatedSession, persistErr := s.persistOpenAIContinuityMetadata(ctx, persisted.Session, plannerRun.JSONRun.ProviderResponseID, plannerRun.JSONRun.ProviderConversationID, plannerRun.JSONRun.Model); persistErr != nil {
							return ReprocessResult{}, persistErr
						} else {
							persisted.Session = updatedSession
						}
						if !plannerRun.Validation.Valid {
							rolloutMetadata.FallbackReason = strings.Join(plannerRun.Validation.Reasons, ",")
							rolloutMetadata.ValidationErrors = append(rolloutMetadata.ValidationErrors, plannerRun.Validation.Reasons...)
							if agentMode == chatAgentModeJSONOnly {
								run := buildJSONDecisionClarificationDraftRun(AgentDecisionValidationResult{Decision: validated.Decision, Valid: false, Reasons: plannerRun.Validation.Reasons}, rolloutMetadata)
								deterministicBookingRun = &run
								deterministicBookingHandled = true
							}
						} else {
							for _, request := range plannerRun.Validation.ApprovedToolRequests {
								switch request.ToolName {
								case toolNameAvailabilitySearch:
									input := specialistToolRequestToAvailabilityInput(request)
									context, err := s.executeAvailabilitySearchIntentTool(ctx, persisted.Session, input)
									if err != nil {
										return ReprocessResult{}, err
									}
									toolContext = mergeAgentToolContexts(toolContext, context)
									toolFacts := map[string]interface{}{}
									if toolContext.Availability != nil {
										toolFacts[toolNameAvailabilitySearch] = buildAvailabilityToolResponsePayload(*toolContext.Availability)
									}
									mergeToolFactsIntoCanonicalState(&canonicalState, toolFacts)
									agentState["canonical_state"] = canonicalState
									memory["canonical_state"] = canonicalState
									toolContext, err = s.executePricingQuoteFromAvailabilityTool(ctx, persisted.Session, toolContext, currentTurn)
									if err != nil {
										return ReprocessResult{}, err
									}
								case toolNameBookingCreate:
									if s.canCreateBookings() {
										context, used, err := s.resolveContextualActionTools(ctx, persisted.Session, history, currentTurn, toolContext)
										if err != nil {
											return ReprocessResult{}, err
										}
										if used {
											toolContext = context
										}
									}
								case toolNamePaymentCreate:
									if s.canCreatePayments() {
										context, err := s.executePaymentCreateTool(ctx, persisted.Session, toolContext, specialistToolRequestToPaymentCreateInput(request))
										if err != nil {
											return ReprocessResult{}, err
										}
										toolContext = mergeAgentToolContexts(toolContext, context)
									}
								}
							}
							rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
							rolloutMetadata.ToolCallCount = len(toolContext.Calls)
							toolContext = mergeToolCallRequestMetadata(toolContext, rolloutMetadata)
							result.ToolCalls = toolContext.Calls
							deterministicToolHandled = len(toolContext.Calls) > 0
							if toolContext.Availability != nil {
								input := AvailabilitySearchInput{}
								intentDecision := IntentDecision{Intent: IntentAvailabilitySearch, Source: "general_specialist", Action: "tool", AvailabilityInput: &input}
								run := buildAvailabilityTemplateDraftRun(intentDecision, *toolContext.Availability)
								deterministicBookingRun = &run
								deterministicBookingHandled = true
							} else if toolContext.BookingCreate != nil {
								run := buildBookingCreatedDraftRun(*toolContext.BookingCreate)
								deterministicBookingRun = &run
								deterministicBookingHandled = true
							}
						}
					}
				}
				if !deterministicBookingHandled && !deterministicToolHandled {
					switch {
					case validated.Decision.Action == jsonDecisionActionTool && validated.Decision.Intent == string(IntentAvailabilitySearch) && validated.Decision.AvailabilityInput != nil:
						input := availabilityInputFromSchedulingPlan(*validated.Decision.AvailabilityInput)
						context, err := s.executeAvailabilitySearchIntentTool(ctx, persisted.Session, input)
						if err != nil {
							return ReprocessResult{}, err
						}
						toolContext = mergeAgentToolContexts(toolContext, context)
						toolFacts := map[string]interface{}{}
						if toolContext.Availability != nil {
							toolFacts[toolNameAvailabilitySearch] = buildAvailabilityToolResponsePayload(*toolContext.Availability)
						}
						mergeToolFactsIntoCanonicalState(&canonicalState, toolFacts)
						agentState["canonical_state"] = canonicalState
						memory["canonical_state"] = canonicalState
						toolContext, err = s.executePricingQuoteFromAvailabilityTool(ctx, persisted.Session, toolContext, currentTurn)
						if err != nil {
							return ReprocessResult{}, err
						}
						rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
						rolloutMetadata.ToolCallCount = len(toolContext.Calls)
						toolContext = mergeToolCallRequestMetadata(toolContext, rolloutMetadata)
						result.ToolCalls = toolContext.Calls
						intentDecision := IntentDecision{
							Intent:            IntentAvailabilitySearch,
							Source:            "json_agent",
							Action:            "tool",
							AvailabilityInput: &input,
						}
						if toolContext.Availability != nil {
							run := buildAvailabilityTemplateDraftRun(intentDecision, *toolContext.Availability)
							deterministicBookingRun = &run
							deterministicBookingHandled = true
						}
						deterministicToolHandled = len(toolContext.Calls) > 0
					case validated.Decision.Action == jsonDecisionActionTool && validated.Decision.Intent == string(IntentPaymentCreate) && validated.Decision.PaymentInput != nil && s.canCreatePayments():
						input := paymentCreateInputFromPlan(*validated.Decision.PaymentInput)
						context, err := s.executePaymentCreateTool(ctx, persisted.Session, toolContext, input)
						if err != nil {
							return ReprocessResult{}, err
						}
						toolContext = mergeAgentToolContexts(toolContext, context)
						rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
						rolloutMetadata.ToolCallCount = len(toolContext.Calls)
						toolContext = mergeToolCallRequestMetadata(toolContext, rolloutMetadata)
						result.ToolCalls = toolContext.Calls
						deterministicToolHandled = len(toolContext.Calls) > 0
						if agentMode == chatAgentModeJSONOnly {
							validation := AgentDecisionValidationResult{Decision: validated.Decision, Valid: false, Reasons: []string{"payment_template_not_available"}}
							rolloutMetadata.FallbackReason = "payment_template_not_available"
							run := buildJSONDecisionClarificationDraftRun(validation, rolloutMetadata)
							deterministicBookingRun = &run
							deterministicBookingHandled = true
						}
					case agentMode == chatAgentModeJSONOnly:
						validation := AgentDecisionValidationResult{Decision: validated.Decision, Valid: false, Reasons: []string{"json_decision_action_not_executable"}}
						rolloutMetadata.FallbackReason = "json_decision_action_not_executable"
						run := buildJSONDecisionClarificationDraftRun(validation, rolloutMetadata)
						deterministicBookingRun = &run
						deterministicBookingHandled = true
					}
				}
			} else if agentMode == chatAgentModeJSONOnly {
				run := buildJSONDecisionClarificationDraftRun(validated, rolloutMetadata)
				deterministicBookingRun = &run
				deterministicBookingHandled = true
			}
		}
	}
	if !unsupportedCargoHandled && !unsupportedPackageHandled && !deterministicBookingHandled && !deterministicToolHandled && !documentCollectionMediaTurn && agentMode == chatAgentModeJSONOnly && !s.canRunJSONDecisionAgent() {
		rolloutMetadata.DecisionSource = "json_agent"
		rolloutMetadata.DecisionValid = boolPtr(false)
		rolloutMetadata.ValidationErrors = []string{"json_decision_runner_unavailable"}
		rolloutMetadata.FallbackReason = "json_decision_runner_unavailable"
		validation := AgentDecisionValidationResult{
			Decision: IntentDecisionJSON{Action: jsonDecisionActionClarify, Intent: string(IntentUnknown)},
			Valid:    false,
			Reasons:  []string{"json_decision_runner_unavailable"},
		}
		run := buildJSONDecisionClarificationDraftRun(validation, rolloutMetadata)
		deterministicBookingRun = &run
		deterministicBookingHandled = true
	}
	if !unsupportedCargoHandled && !unsupportedPackageHandled && !deterministicBookingHandled && !deterministicToolHandled && !documentCollectionMediaTurn && !s.canRunAgent() {
		if jsonDecisionLayerEnabledForMode(agentMode) {
			if rolloutMetadata.DecisionSource == "" {
				rolloutMetadata.DecisionSource = "json_agent"
				rolloutMetadata.DecisionValid = boolPtr(false)
				rolloutMetadata.ValidationErrors = []string{"free_form_llm_unavailable"}
				rolloutMetadata.FallbackReason = "free_form_llm_unavailable"
			}
			validation := AgentDecisionValidationResult{
				Decision: IntentDecisionJSON{Action: jsonDecisionActionClarify, Intent: string(IntentUnknown)},
				Valid:    false,
				Reasons:  append([]string(nil), rolloutMetadata.ValidationErrors...),
			}
			run := buildJSONDecisionClarificationDraftRun(validation, rolloutMetadata)
			deterministicBookingRun = &run
			deterministicBookingHandled = true
		} else {
			s.logReprocess(
				"chat reprocess event=runner_disabled_after_json_decision session_id=%s trigger=%s job_run_id=%s",
				persisted.Session.ID,
				trigger,
				jobRunID,
			)
			return result, nil
		}
	}
	if !freeFormLLMFallbackEnabledForMode(agentMode) && deterministicBookingHandled == false && !deterministicToolHandled && !unsupportedCargoHandled && !unsupportedPackageHandled && !documentCollectionMediaTurn {
		s.logReprocess(
			"chat reprocess event=legacy_prompt_fallback_disabled session_id=%s trigger=%s job_run_id=%s",
			persisted.Session.ID,
			trigger,
			jobRunID,
		)
		return result, nil
	}
	if !unsupportedCargoHandled && !unsupportedPackageHandled && !deterministicBookingHandled && !deterministicToolHandled && !documentCollectionMediaTurn {
		var err error
		s.logReprocess(
			"chat reprocess event=resolve_agent_tool_context_start session_id=%s trigger=%s job_run_id=%s",
			persisted.Session.ID,
			trigger,
			jobRunID,
		)
		toolContext, err = s.resolveAgentToolContext(ctx, persisted.Session, history, memory)
		if err != nil {
			return ReprocessResult{}, err
		}
		s.logReprocess(
			"chat reprocess event=resolve_agent_tool_context_done session_id=%s trigger=%s job_run_id=%s tool_call_count=%d",
			persisted.Session.ID,
			trigger,
			jobRunID,
			len(toolContext.Calls),
		)
	}
	result.ToolCalls = toolContext.Calls

	if !unsupportedCargoHandled && !unsupportedPackageHandled && !deterministicBookingHandled && !documentAttempted {
		documentContext, handled, err := s.resolveDocumentExtractContext(ctx, persisted.Session, candidates, memory, draftID)
		if err != nil {
			return ReprocessResult{}, err
		}
		documentHandled = handled
		toolContext = mergeAgentToolContexts(toolContext, documentContext)
		result.ToolCalls = toolContext.Calls
	}

	var run RunAgentResult
	var userPrompt string

	if unsupportedCargoHandled {
		userPrompt = buildAgentUserPrompt(persisted.Session, memory, toolContext)
		run = buildUnsupportedCargoDraftRun(unsupportedCargo)
	} else if unsupportedPackageHandled {
		userPrompt = buildAgentUserPrompt(persisted.Session, memory, toolContext)
		run = buildUnsupportedPackageDraftRun(unsupportedPackage)
	} else if deterministicBookingHandled && deterministicBookingRun != nil {
		run = *deterministicBookingRun
		if run.Model != "template_realizer" {
			userPrompt = buildAgentUserPrompt(persisted.Session, memory, toolContext)
		}
	} else if toolContext.BookingCreate != nil {
		run = buildBookingCreatedDraftRun(*toolContext.BookingCreate)
	} else if documentHandled && toolContext.DocumentExtract != nil {
		bookingDraft := collectBookingDraftContext(persisted.Session, history, currentTurn)
		documentPassengers := bookingPassengersFromDocumentExtract(*toolContext.DocumentExtract, persisted.Session, bookingDraft.TripDate)
		if strings.EqualFold(strings.TrimSpace(toolContext.DocumentExtract.Mode), "EXTRACTED") &&
			bookingDraft.ChildUnder5Count > 0 &&
			len(documentPassengers) > 0 &&
			countLapChildPassengers(documentPassengers) == 0 {
			bookingDraft.HasPassengerDetails = true
			bookingDraft.PassengerDetails = documentPassengers
			bookingDraft.PassengerDetailsCount = len(documentPassengers)
			bookingDraft.NeedsLapChildAssignment = true
			run = buildBookingContinuationDraftRun(buildAskLapChildAssignmentReply(bookingDraft), BookingNextAskLapChildAssignment, bookingDraft)
		} else {
			userPrompt = buildAgentUserPrompt(persisted.Session, memory, toolContext)
			run = buildDocumentExtractDraftRun(*toolContext.DocumentExtract)
		}
	} else {
		userPrompt = buildAgentUserPrompt(persisted.Session, memory, toolContext)
		s.logReprocess(
			"chat reprocess event=runner_run_start session_id=%s trigger=%s job_run_id=%s current_turn_count=%d tool_call_count=%d",
			persisted.Session.ID,
			trigger,
			jobRunID,
			len(candidates),
			len(toolContext.Calls),
		)
		run, err = s.runner.Run(ctx, RunAgentInput{
			Session:          persisted.Session,
			CurrentTurnIDs:   candidateMessageIDs(candidates),
			CurrentTurnMedia: collectCandidateMedia(candidates),
			SystemPrompt:     systemPrompt,
			UserPrompt:       userPrompt,
			IdempotencyKey:   draftID,
		})
		if err != nil {
			s.logReprocess(
				"chat reprocess event=runner_run_failed session_id=%s trigger=%s job_run_id=%s error=%v",
				persisted.Session.ID,
				trigger,
				jobRunID,
				err,
			)
			return ReprocessResult{}, fmt.Errorf("%w: %v", ErrAgentRunFailed, err)
		}
		if updatedSession, persistErr := s.persistOpenAIContinuityMetadata(ctx, persisted.Session, run.ProviderResponseID, run.ProviderConversationID, run.Model); persistErr != nil {
			return ReprocessResult{}, persistErr
		} else {
			persisted.Session = updatedSession
		}
		s.logReprocess(
			"chat reprocess event=runner_run_done session_id=%s trigger=%s job_run_id=%s has_reply=%t",
			persisted.Session.ID,
			trigger,
			jobRunID,
			strings.TrimSpace(run.ReplyText) != "",
		)
		if rolloutMetadata.DecisionSource == "" || rolloutMetadata.FallbackReason != "" {
			rolloutMetadata.DecisionSource = "legacy_llm"
		}
	}

	if containsOutOfDomainSchedulingVocabulary(run.ReplyText) ||
		shouldReplaceLoopingDraftWithSafeFallback(history, run.ReplyText, toolContext.Calls, phaseBefore, canonicalState.Phase) {
		bookingDraft := collectBookingDraftContext(persisted.Session, history, currentTurn)
		if reply, ok := buildSafeFallbackReplyForPhase(canonicalState, bookingDraft, currentTurn); ok && strings.TrimSpace(reply) != "" {
			run = buildSafeFallbackDraftRun(reply, canonicalState, "unsafe_or_looping_draft_replaced")
			rolloutMetadata.DecisionSource = "safe_phase_fallback"
			rolloutMetadata.DecisionValid = boolPtr(true)
			rolloutMetadata.FallbackReason = "unsafe_or_looping_draft_replaced"
		}
	}

	if unsupportedCargoHandled || unsupportedPackageHandled {
		rolloutMetadata.DecisionSource = "template_realizer"
		rolloutMetadata.DecisionValid = boolPtr(true)
	}
	if run.Model == "template_realizer" && rolloutMetadata.DecisionSource == "" {
		rolloutMetadata.DecisionSource = "template_realizer"
	}
	rolloutMetadata.CanonicalPhaseAfter = canonicalState.Phase
	rolloutMetadata.ToolCallCount = len(toolContext.Calls)
	toolContext = mergeToolCallRequestMetadata(toolContext, rolloutMetadata)
	result.ToolCalls = toolContext.Calls
	applyRolloutMetadataToRun(&run, rolloutMetadata)

	runAt := time.Now().UTC()
	autoSendPolicy := evaluateDraftAutoSendPolicy(candidates, toolContext.Calls, run.ReplyText)
	draftAgentState := buildDraftGeneratedAgentState(persisted.Session.Metadata, candidates, draftID, run, toolContext.Calls, autoSendPolicy, runAt)
	if canonicalStateEnabled() {
		draftAgentState["canonical_state"] = canonicalState
	}
	draftBuffer := buildDraftGeneratedBufferState(persisted.Session.Metadata, candidates, draftID, runAt)
	draftPayload, draftNormalizedPayload := buildAgentDraftPayload(persisted.Session, candidates, draftID, systemPrompt, userPrompt, run, toolContext, autoSendPolicy, runAt)
	draftAgentState[structuredInterpreterShadowKey] = shadow
	draftPayload[structuredInterpreterShadowKey] = shadow
	draftNormalizedPayload[structuredInterpreterShadowKey] = shadow

	s.logReprocess(
		"chat reprocess event=save_agent_draft_start session_id=%s trigger=%s job_run_id=%s idempotency_key=%s tool_call_count=%d",
		sessionID,
		trigger,
		jobRunID,
		draftID,
		len(toolContext.Calls),
	)
	draft, err := s.store.SaveAgentDraft(ctx, SaveAgentDraftInput{
		SessionID:         sessionID,
		IdempotencyKey:    draftID,
		Body:              strings.TrimSpace(run.ReplyText),
		SenderName:        "SHABAS",
		ProcessingStatus:  messageStatusAutomationDraft,
		Payload:           draftPayload,
		NormalizedPayload: draftNormalizedPayload,
		Agent:             draftAgentState,
		Buffer:            draftBuffer,
		RecordedAt:        runAt,
	})
	if err != nil {
		if IsUniqueViolation(err) {
			s.logReprocess(
				"chat reprocess event=save_agent_draft_conflict session_id=%s trigger=%s job_run_id=%s idempotency_key=%s",
				sessionID,
				trigger,
				jobRunID,
				draftID,
			)
			existing, findErr := s.store.FindMessageByKeys(ctx, "", draftID)
			if findErr != nil {
				return ReprocessResult{}, findErr
			}
			if existing != nil && existing.SessionID == sessionID {
				currentSession, getErr := s.GetSession(ctx, sessionID)
				if getErr != nil {
					return ReprocessResult{}, getErr
				}
				result.Session = currentSession
				result.Draft = existing
				result.Idempotent = true
				result.Reason = "draft_already_generated"
				return result, nil
			}
		}
		s.logReprocess(
			"chat reprocess event=save_agent_draft_failed session_id=%s trigger=%s job_run_id=%s idempotency_key=%s error=%v",
			sessionID,
			trigger,
			jobRunID,
			draftID,
			err,
		)
		return ReprocessResult{}, err
	}
	s.logReprocess(
		"chat reprocess event=save_agent_draft_done session_id=%s trigger=%s job_run_id=%s draft_message_id=%s",
		draft.Session.ID,
		trigger,
		jobRunID,
		draft.Message.ID,
	)

	result.Session = draft.Session
	result.ToolCalls = toolContext.Calls
	result.Draft = &draft.Message
	result.Reason = "draft_generated"
	return s.finishReprocessWithAutoSend(ctx, result, trigger, jobRunID)
}

func isBookingRegressionDraftText(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}

	fullRouteAgain := strings.Contains(folded, "de qual cidade voce sai") &&
		strings.Contains(folded, "para qual cidade vai") &&
		strings.Contains(folded, "data")

	genericRouteAgain := strings.Contains(folded, "origem") &&
		strings.Contains(folded, "destino") &&
		strings.Contains(folded, "data")

	return fullRouteAgain || genericRouteAgain
}

func isOutOfScopeDuringBookingDraftText(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}

	return strings.Contains(folded, "fora de atendimento") ||
		strings.Contains(folded, "atendemos apenas") ||
		strings.Contains(folded, "rota nao esta disponivel") ||
		strings.Contains(folded, "rota não esta disponivel") ||
		strings.Contains(folded, "outras rotas")
}

func detectBookingAutoSendBlockReason(history []Message, draft Message) string {
	draftContext := collectBookingDraftContext(Session{}, history, "")
	if !draftContext.IsAdvancedBookingFlow() {
		return ""
	}

	body := strings.TrimSpace(draft.Body)
	if isBookingRegressionDraftText(body) {
		return draftAutoSendReasonBookingFlowRegression
	}

	if isOutOfScopeDuringBookingDraftText(body) {
		return draftAutoSendReasonOutOfScopeDuringBooking
	}

	return ""
}

// Generic revisor mark
func (s *Service) markDraftAutoSendReviewRequired(ctx context.Context, result ReprocessResult, reason string) (ReprocessResult, error) {
	observedAt := time.Now().UTC()

	updated, err := s.store.UpdateDraftAutoSendState(ctx, UpdateDraftAutoSendStateInput{
		SessionID:      result.Session.ID,
		DraftMessageID: result.Draft.ID,
		AutoSendStatus: draftAutoSendStatusReviewNeeded,
		AutoSendReasons: mergeDistinctStrings(
			readDraftAutoSendReasons(*result.Draft),
			reason,
		),
		Payload: map[string]interface{}{
			"auto_send_status":       draftAutoSendStatusReviewNeeded,
			"auto_send_blocked":      true,
			"auto_send_blocked_at":   observedAt.UTC().Format(time.RFC3339Nano),
			"auto_send_block_reason": reason,
		},
		Agent: map[string]interface{}{
			"status":                 agentStatusDraftGenerated,
			"auto_send_status":       draftAutoSendStatusReviewNeeded,
			"auto_send_reasons":      mergeDistinctStrings(readDraftAutoSendReasons(*result.Draft), reason),
			"auto_send_blocked_at":   observedAt.UTC().Format(time.RFC3339Nano),
			"auto_send_block_reason": reason,
		},
	})
	if err != nil {
		return ReprocessResult{}, err
	}

	result.Session = updated.Session
	result.Draft = &updated.Message
	result.Reason = reason
	result.Idempotent = true
	return result, nil
}

func (s *Service) RetryDraftAutoSend(ctx context.Context, input RetryDraftAutoSendInput) (RetryDraftAutoSendResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return RetryDraftAutoSendResult{}, err
	}

	messages, err := s.store.ListMessages(ctx, sessionID, normalizeListMessagesFilter(ListMessagesFilter{Limit: 100}))
	if err != nil {
		return RetryDraftAutoSendResult{}, err
	}

	draft := findLatestDraftMessage(messages)
	if draft == nil {
		return RetryDraftAutoSendResult{}, ErrDraftNotFound
	}
	if !strings.EqualFold(readDraftAutoSendStatus(*draft), draftAutoSendStatusRetryPending) {
		return RetryDraftAutoSendResult{}, ErrDraftAutoSendRetryNotAllowed
	}

	if s.shouldBlockDraftAutoSend(session, *draft) {
		blocked, err := s.markDraftAutoSendBlocked(ctx, ReprocessResult{
			Session: session,
			Draft:   draft,
		}, session)
		if err != nil {
			return RetryDraftAutoSendResult{}, err
		}
		return RetryDraftAutoSendResult{
			Session:    blocked.Session,
			Status:     "blocked",
			Reason:     blocked.Reason,
			Draft:      blocked.Draft,
			Idempotent: true,
		}, nil
	}

	idempotencyKey := buildAutoSendReplyIdempotencyKey(draft.ID)
	existing, err := s.store.FindReplyByIdempotency(ctx, sessionID, idempotencyKey)
	if err != nil {
		return RetryDraftAutoSendResult{}, err
	}
	if existing == nil {
		return RetryDraftAutoSendResult{}, ErrDraftAutoSendRetryNotAllowed
	}
	existing.Draft = draft

	if strings.TrimSpace(existing.Outbound.ProviderMessageID) != "" || strings.EqualFold(existing.Outbound.Status, "SENT") {
		return RetryDraftAutoSendResult{
			Session:    existing.Session,
			Status:     "skipped",
			Reason:     "draft_auto_send_already_sent",
			Draft:      draft,
			Message:    &existing.Message,
			Outbound:   &existing.Outbound,
			Idempotent: true,
		}, nil
	}

	observedAt := time.Now().UTC()
	updatedDraft, err := s.store.UpdateDraftAutoSendState(ctx, UpdateDraftAutoSendStateInput{
		SessionID:       sessionID,
		DraftMessageID:  draft.ID,
		AutoSendStatus:  draftAutoSendStatusRetryPending,
		AutoSendReasons: readDraftAutoSendReasons(*draft),
		Payload:         buildDraftAutoSendRetryRequestedPayload(*draft, input, observedAt),
		Agent:           buildDraftAutoSendRetryRequestedAgentState(session.Metadata, *draft, input, observedAt),
	})
	if err != nil {
		return RetryDraftAutoSendResult{}, err
	}
	draft = &updatedDraft.Message
	existing.Session = updatedDraft.Session
	existing.Draft = draft

	if !s.canDeliverReply(existing.Outbound) {
		return RetryDraftAutoSendResult{
			Session:    existing.Session,
			Status:     "skipped",
			Reason:     "draft_auto_send_not_deliverable",
			Draft:      draft,
			Message:    &existing.Message,
			Outbound:   &existing.Outbound,
			Idempotent: true,
		}, nil
	}

	delivered, err := s.deliverReply(ctx, *existing)
	if err != nil {
		return RetryDraftAutoSendResult{}, err
	}

	return RetryDraftAutoSendResult{
		Session:  delivered.Session,
		Status:   "accepted",
		Reason:   "draft_auto_send_retried",
		Draft:    delivered.Draft,
		Message:  &delivered.Message,
		Outbound: &delivered.Outbound,
	}, nil
}

func (s *Service) ApplyPresenceSignal(ctx context.Context, input ApplyPresenceSignalInput) (ApplyPresenceSignalResult, error) {
	channel := strings.ToUpper(strings.TrimSpace(input.Channel))
	if channel == "" {
		channel = "WHATSAPP"
	}

	contactKey := strings.TrimSpace(input.ContactKey)
	if contactKey == "" {
		return ApplyPresenceSignalResult{}, ErrContactKeyRequired
	}

	presenceStatus := strings.ToUpper(strings.TrimSpace(input.PresenceStatus))
	if presenceStatus == "" {
		return ApplyPresenceSignalResult{}, ErrPresenceRequired
	}

	sessions, err := s.store.ListSessions(ctx, normalizeListSessionsFilter(ListSessionsFilter{
		Channel:    channel,
		ContactKey: contactKey,
		Limit:      1,
	}))
	if err != nil {
		return ApplyPresenceSignalResult{}, err
	}
	if len(sessions) == 0 {
		return ApplyPresenceSignalResult{
			Status:        "skipped",
			PresenceState: presenceStatus,
			Reason:        "session_not_found",
		}, nil
	}

	session := sessions[0]
	if hasHumanOwnership(session) {
		return ApplyPresenceSignalResult{
			Session:       session,
			Status:        "skipped",
			PresenceState: presenceStatus,
			Reason:        agentBlockReasonHumanOwnerActive,
		}, nil
	}

	observedAt := time.Now().UTC()
	if input.ObservedAt != nil {
		observedAt = input.ObservedAt.UTC()
	}

	buffer, reason, changed := applyPresenceSignalToBuffer(
		session.Metadata,
		presenceStatus,
		observedAt,
		time.Duration(s.cfg.ChatDebounceWindowMS)*time.Millisecond,
	)
	if !changed {
		return ApplyPresenceSignalResult{
			Session:       session,
			Status:        "skipped",
			PresenceState: presenceStatus,
			Reason:        reason,
		}, nil
	}

	if session, err = s.store.UpdateSessionBufferState(ctx, UpdateSessionBufferStateInput{
		SessionID: session.ID,
		Buffer:    buffer,
	}); err != nil {
		return ApplyPresenceSignalResult{}, err
	}

	return ApplyPresenceSignalResult{
		Session:       session,
		Status:        "accepted",
		PresenceState: presenceStatus,
		Reason:        reason,
	}, nil
}

func (s *Service) GetSession(ctx context.Context, id string) (Session, error) {
	item, err := s.store.GetSession(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}
	return decorateSessionDraftSummary(item, s.chatReviewSLASeconds(), time.Now().UTC()), nil
}

func (s *Service) ListMessages(ctx context.Context, sessionID string, filter ListMessagesFilter) ([]Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	return s.store.ListMessages(ctx, sessionID, normalizeListMessagesFilter(filter))
}

func (s *Service) GetStructuredInterpreterShadowReport(ctx context.Context, filter StructuredInterpreterShadowReportFilter) (StructuredInterpreterShadowReportResponse, error) {
	filter = normalizeStructuredInterpreterShadowReportFilter(filter)
	if filter.SessionID == "" {
		return StructuredInterpreterShadowReportResponse{}, ErrShadowReportSessionRequired
	}
	messages, err := s.store.ListStructuredInterpreterShadowMessages(ctx, filter)
	if err != nil {
		return StructuredInterpreterShadowReportResponse{}, err
	}

	items := StructuredInterpreterShadowReportItemsFromMessages(messages)
	return StructuredInterpreterShadowReportResponse{
		Filter:             filter,
		LoadedMessageCount: len(messages),
		ReportItemCount:    len(items),
		Report:             BuildStructuredInterpreterShadowReport(items),
	}, nil
}

func (s *Service) GetCurrentDraft(ctx context.Context, sessionID string) (CurrentDraftResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return CurrentDraftResult{}, err
	}

	messages, err := s.store.ListMessages(ctx, sessionID, normalizeListMessagesFilter(ListMessagesFilter{Limit: 250}))
	if err != nil {
		return CurrentDraftResult{}, err
	}

	draft := findLatestDraftMessage(messages)
	if draft == nil {
		return CurrentDraftResult{}, ErrDraftNotFound
	}

	result := CurrentDraftResult{
		Session:             session,
		Draft:               *draft,
		DraftStatus:         strings.TrimSpace(draft.ProcessingStatus),
		AgentStatus:         readAgentStatus(session.Metadata),
		DraftIdempotencyKey: firstNonEmpty(asString(draft.NormalizedPayload["draft_idempotency_key"]), asString(draft.Payload["draft_idempotency_key"]), strings.TrimSpace(draft.IdempotencyKey)),
		GeneratedAt:         firstParsedTime(draft.NormalizedPayload["generated_at"], draft.Payload["generated_at"]),
		ReviewedAt:          firstParsedTime(draft.NormalizedPayload["reviewed_at"], draft.Payload["reviewed_at"]),
		ReviewedByUserID:    firstNonEmpty(asString(draft.NormalizedPayload["reviewed_by_user_id"]), asString(draft.Payload["reviewed_by_user_id"])),
		ReviewMode:          firstNonEmpty(asString(draft.NormalizedPayload["review_mode"]), asString(draft.Payload["review_mode"])),
		ReviewAction:        firstNonEmpty(asString(draft.NormalizedPayload["review_action"]), asString(draft.Payload["review_action"])),
		Model:               firstNonEmpty(asString(draft.NormalizedPayload["model"]), asString(draft.Payload["model"])),
		ProviderResponseID:  firstNonEmpty(asString(draft.NormalizedPayload["provider_response_id"]), asString(draft.Payload["provider_response_id"])),
		AutoSendStatus:      firstNonEmpty(asString(draft.NormalizedPayload["auto_send_status"]), asString(draft.Payload["auto_send_status"])),
		AutoSendReasons: firstNonEmptyStringSlice(
			asStringSlice(draft.NormalizedPayload["auto_send_reasons"]),
			asStringSlice(draft.Payload["auto_send_reasons"]),
		),
		AutoSendLastAttemptAt: firstParsedTime(
			draft.NormalizedPayload["auto_send_last_attempt_at"],
			draft.Payload["auto_send_last_attempt_at"],
		),
		AutoSendRetryAt: firstParsedTime(
			draft.NormalizedPayload["auto_send_retry_pending_at"],
			draft.Payload["auto_send_retry_pending_at"],
		),
		AutoSendRetryRequestedAt: firstParsedTime(
			draft.NormalizedPayload["auto_send_retry_requested_at"],
			draft.Payload["auto_send_retry_requested_at"],
		),
		AutoSendRetryRequestedBy: firstNonEmpty(
			asString(draft.NormalizedPayload["auto_send_retry_requested_by"]),
			asString(draft.Payload["auto_send_retry_requested_by"]),
		),
		AutoSendRetryRequestCount: maxInt(
			asInt(draft.NormalizedPayload["auto_send_retry_request_count"]),
			asInt(draft.Payload["auto_send_retry_request_count"]),
		),
		AutoSendRetryRequestReason: firstNonEmpty(
			asString(draft.NormalizedPayload["auto_send_retry_request_reason"]),
			asString(draft.Payload["auto_send_retry_request_reason"]),
		),
		AutoSendBlockedAt: firstParsedTime(
			draft.NormalizedPayload["auto_send_blocked_at"],
			draft.Payload["auto_send_blocked_at"],
		),
		AutoSendLastErrorText: firstNonEmpty(
			asString(draft.NormalizedPayload["auto_send_last_error_text"]),
			asString(draft.Payload["auto_send_last_error_text"]),
		),
		AutoSendBlockReason: firstNonEmpty(
			asString(draft.NormalizedPayload["auto_send_block_reason"]),
			asString(draft.Payload["auto_send_block_reason"]),
		),
		AutoSendLastReplyID: firstNonEmpty(
			asString(draft.NormalizedPayload["auto_send_last_reply_message_id"]),
			asString(draft.Payload["auto_send_last_reply_message_id"]),
		),
		AutoSendLastOutboundID: firstNonEmpty(
			asString(draft.NormalizedPayload["auto_send_last_outbound_id"]),
			asString(draft.Payload["auto_send_last_outbound_id"]),
		),
		CurrentTurnMessageIDs: firstNonEmptyStringSlice(
			asStringSlice(draft.NormalizedPayload["current_turn_message_ids"]),
			asStringSlice(draft.Payload["current_turn_message_ids"]),
		),
		ToolNames: firstNonEmptyStringSlice(
			asStringSlice(draft.NormalizedPayload["tool_names"]),
			extractToolNamesFromCalls(asInterfaceSliceMaps(draft.Payload["tool_calls"])),
		),
		ToolCalls:   asInterfaceSliceMaps(draft.Payload["tool_calls"]),
		ToolContext: asMap(draft.Payload["tool_context"]),
	}
	if result.ToolCallCount == 0 {
		result.ToolCallCount = readInt(draft.NormalizedPayload["tool_call_count"])
		if result.ToolCallCount == 0 {
			result.ToolCallCount = len(result.ToolCalls)
		}
	}
	result.AutoSendIssueActive = isProblematicDraftAutoSendStatus(result.AutoSendStatus)
	if linked := findLinkedReplyMessage(messages, draft.ID); linked != nil {
		result.LinkedReply = linked
	}
	return result, nil
}

func normalizeListSessionsFilter(filter ListSessionsFilter) ListSessionsFilter {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Channel = strings.ToUpper(strings.TrimSpace(filter.Channel))
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	filter.HandoffStatus = strings.ToUpper(strings.TrimSpace(filter.HandoffStatus))
	filter.ContactKey = strings.TrimSpace(filter.ContactKey)
	filter.AgentStatus = strings.ToUpper(strings.TrimSpace(filter.AgentStatus))
	filter.DraftReviewStatus = strings.ToUpper(strings.TrimSpace(filter.DraftReviewStatus))
	filter.DraftAutoSendStatus = strings.ToUpper(strings.TrimSpace(filter.DraftAutoSendStatus))
	filter.OrderBy = strings.ToUpper(strings.TrimSpace(filter.OrderBy))
	return filter
}

func normalizeListMessagesFilter(filter ListMessagesFilter) ListMessagesFilter {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

func normalizeStructuredInterpreterShadowReportFilter(filter StructuredInterpreterShadowReportFilter) StructuredInterpreterShadowReportFilter {
	filter.SessionID = strings.TrimSpace(filter.SessionID)
	if filter.Limit <= 0 {
		filter.Limit = 200
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

func findLatestDraftMessage(messages []Message) *Message {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Direction != "OUTBOUND" {
			continue
		}
		if !isAutomationDraftStatus(message.ProcessingStatus) {
			continue
		}
		item := message
		return &item
	}
	return nil
}

func findLinkedReplyMessage(messages []Message, draftID string) *Message {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Direction != "OUTBOUND" || isAutomationDraftStatus(message.ProcessingStatus) {
			continue
		}
		if firstNonEmpty(
			asString(message.Payload["draft_message_id"]),
			asString(message.NormalizedPayload["draft_message_id"]),
		) != draftID {
			continue
		}
		item := message
		return &item
	}
	return nil
}

func readAgentStatus(metadata map[string]interface{}) string {
	agent := asMap(metadata["agent"])
	return strings.TrimSpace(asString(agent["status"]))
}

func decorateSessionDraftSummary(session Session, reviewSLASeconds int, observedAt time.Time) Session {
	agent := asMap(session.Metadata["agent"])
	if len(agent) == 0 {
		return session
	}

	session.AgentStatus = strings.TrimSpace(asString(agent["status"]))
	session.DraftIdempotencyKey = firstNonEmpty(
		asString(agent["draft_idempotency_key"]),
		asString(agent["reviewed_draft_message_id"]),
	)
	session.DraftGeneratedAt = firstParsedTime(agent["draft_generated_at"], agent["requested_at"])
	session.DraftReviewedAt = firstParsedTime(agent["reviewed_at"])
	session.DraftReviewedByUserID = strings.TrimSpace(asString(agent["reviewed_by_user_id"]))
	session.DraftReviewAction = strings.TrimSpace(asString(agent["review_action"]))
	session.DraftToolNames = firstNonEmptyStringSlice(asStringSlice(agent["tool_names"]))
	session.DraftToolCallCount = readInt(agent["tool_calls_count"])
	session.DraftModel = firstNonEmpty(asString(agent["draft_model"]), asString(agent["model"]))
	session.DraftProviderResponseID = firstNonEmpty(asString(agent["provider_response_id"]))
	session.DraftAutoSendStatus = firstNonEmpty(asString(agent["auto_send_status"]))
	session.DraftAutoSendReasons = firstNonEmptyStringSlice(asStringSlice(agent["auto_send_reasons"]))
	session.DraftReviewSLASeconds = reviewSLASeconds

	switch session.AgentStatus {
	case agentStatusDraftGenerated:
		session.HasAutomationDraft = true
		session.DraftReviewStatus = "PENDING_REVIEW"
		session.DraftReviewPriority = "LOW"
		if session.DraftGeneratedAt != nil {
			ageSeconds := int(observedAt.Sub(session.DraftGeneratedAt.UTC()).Seconds())
			if ageSeconds < 0 {
				ageSeconds = 0
			}
			session.DraftPendingAgeSeconds = ageSeconds
			session.DraftPendingAgeBucket = classifyDraftPendingAge(ageSeconds, reviewSLASeconds)
			session.DraftReviewPriority = classifyDraftReviewPriority(session.DraftPendingAgeBucket)
			session.DraftReviewOverdue = ageSeconds >= reviewSLASeconds
		}
		session = decorateSessionDraftAlert(session)
	case agentStatusDraftReviewed:
		session.HasAutomationDraft = true
		session.DraftReviewStatus = "REVIEWED"
	case agentStatusDraftAutoSent:
		session.HasAutomationDraft = true
		session.DraftReviewStatus = "AUTO_SENT"
		session.DraftReviewPriority = "REVIEWED"
	}

	return session
}

func isProblematicDraftAutoSendStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case draftAutoSendStatusRetryPending, draftAutoSendStatusBlockedHuman:
		return true
	default:
		return false
	}
}

func (s *Service) chatReviewSLASeconds() int {
	minutes := s.cfg.ChatReviewSLAMinutes
	if minutes <= 0 {
		minutes = 15
	}
	return minutes * 60
}

func classifyDraftPendingAge(ageSeconds int, reviewSLASeconds int) string {
	if ageSeconds < 0 {
		ageSeconds = 0
	}
	if reviewSLASeconds <= 0 {
		reviewSLASeconds = 900
	}
	if ageSeconds >= reviewSLASeconds {
		return "OVERDUE"
	}
	if ageSeconds >= maxInt(reviewSLASeconds/2, 1) {
		return "DUE_SOON"
	}
	return "FRESH"
}

func classifyDraftReviewPriority(bucket string) string {
	switch strings.ToUpper(strings.TrimSpace(bucket)) {
	case "OVERDUE":
		return "HIGH"
	case "DUE_SOON":
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func decorateSessionDraftAlert(session Session) Session {
	switch strings.ToUpper(strings.TrimSpace(session.DraftPendingAgeBucket)) {
	case "OVERDUE":
		session.DraftReviewAlertActive = true
		session.DraftReviewAlertLevel = "CRITICAL"
		session.DraftReviewAlertCode = "DRAFT_REVIEW_OVERDUE"
		session.DraftReviewAlertMessage = "Draft aguardando revisao acima do SLA."
	case "DUE_SOON":
		session.DraftReviewAlertActive = true
		session.DraftReviewAlertLevel = "WARNING"
		session.DraftReviewAlertCode = "DRAFT_REVIEW_DUE_SOON"
		session.DraftReviewAlertMessage = "Draft proximo de estourar o SLA de revisao."
	}
	return session
}

func decorateSessionsSummaryAlert(summary SessionsSummary) SessionsSummary {
	switch {
	case summary.OverdueReviewCount > 0:
		summary.HasReviewAlert = true
		summary.ReviewAlertLevel = "CRITICAL"
		summary.ReviewAlertCode = "REVIEW_QUEUE_OVERDUE"
		summary.ReviewAlertMessage = "Fila com drafts acima do SLA de revisao."
		summary.ReviewAlertSessionCount = summary.OverdueReviewCount
	case summary.DueSoonReviewCount > 0:
		summary.HasReviewAlert = true
		summary.ReviewAlertLevel = "WARNING"
		summary.ReviewAlertCode = "REVIEW_QUEUE_DUE_SOON"
		summary.ReviewAlertMessage = "Fila com drafts proximos de estourar o SLA."
		summary.ReviewAlertSessionCount = summary.DueSoonReviewCount
	}
	return summary
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func firstParsedTime(values ...interface{}) *time.Time {
	for _, value := range values {
		text := strings.TrimSpace(asString(value))
		if text == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
		if parsed, err := time.Parse(time.RFC3339, text); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		items := make([]string, 0, len(value))
		for _, item := range value {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func asStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, raw := range typed {
			if text := strings.TrimSpace(asString(raw)); text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func asInterfaceSliceMaps(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(typed))
		for _, raw := range typed {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			items = append(items, item)
		}
		return items
	default:
		return nil
	}
}

func asMap(value interface{}) map[string]interface{} {
	item, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return item
}

func extractToolNamesFromCalls(toolCalls []map[string]interface{}) []string {
	items := make([]string, 0, len(toolCalls))
	for _, item := range toolCalls {
		if name := strings.TrimSpace(asString(item["tool_name"])); name != "" {
			items = append(items, name)
		}
	}
	return items
}

func readInt(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func hasHumanOwnership(session Session) bool {
	return session.HandoffStatus == "HUMAN" && strings.TrimSpace(session.CurrentOwnerUserID) != ""
}

func resolveInboundProcessingStatus(explicit string, direction string, session Session, debounceEnabled bool) string {
	if direction != "INBOUND" {
		if explicit != "" {
			return explicit
		}
		return "RECEIVED"
	}

	if hasHumanOwnership(session) && (explicit == "" || explicit == "RECEIVED" || explicit == "BUFFERED_PENDING") {
		return "HUMAN_OWNED_PENDING"
	}
	if explicit != "" {
		return explicit
	}
	if debounceEnabled {
		return "BUFFERED_PENDING"
	}
	return "RECEIVED"
}

func annotateOwnershipBlock(input map[string]interface{}, session Session, direction string, processingStatus string) map[string]interface{} {
	if len(input) == 0 && (!hasHumanOwnership(session) || direction != "INBOUND") {
		return input
	}

	output := map[string]interface{}{}
	for key, value := range input {
		output[key] = value
	}
	if hasHumanOwnership(session) && direction == "INBOUND" {
		output["agent_blocked_by_human"] = true
		output["agent_block_reason"] = agentBlockReasonHumanOwnerActive
		output["current_owner_user_id"] = session.CurrentOwnerUserID
		output["handoff_status"] = session.HandoffStatus
		output["processing_status"] = processingStatus
	}
	return output
}

func (s *Service) canDeliverReply(outbound ReplyOutbound) bool {
	if s.sender == nil || !s.sender.Enabled() {
		return false
	}
	if strings.TrimSpace(outbound.ProviderMessageID) != "" {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(outbound.Status)) {
	case "", "MANUAL_PENDING", "AUTOMATION_PENDING", "SEND_FAILED":
		return true
	default:
		return false
	}
}

func (s *Service) canRunAgent() bool {
	return s.runner != nil && s.runner.Enabled()
}

func (s *Service) canRunJSONDecisionAgent() bool {
	return s.jsonRunner != nil && s.jsonRunner.Enabled()
}

func (s *Service) persistOpenAIContinuityMetadata(ctx context.Context, session Session, providerResponseID string, providerConversationID string, providerModel string) (Session, error) {
	metadata := map[string]interface{}{}
	if trimmed := strings.TrimSpace(providerResponseID); trimmed != "" {
		metadata["provider_response_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(providerConversationID); trimmed != "" {
		metadata["provider_conversation_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(providerModel); trimmed != "" {
		metadata["provider_model"] = trimmed
	}
	if len(metadata) == 0 {
		return session, nil
	}
	updated, err := s.store.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
		SessionID: session.ID,
		Metadata:  metadata,
	})
	if err != nil {
		return Session{}, err
	}
	return updated, nil
}

func buildStructuredInterpreterShadowIdempotencyKey(sessionID string, candidates []Message) string {
	parts := make([]string, 0, len(candidates)+1)
	parts = append(parts, sessionID)
	for _, message := range candidates {
		parts = append(parts, message.ID)
	}
	return "chat-openai-interpreter-shadow-" + deterministicID(strings.Join(parts, "|"))
}

func deterministicID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:32]
}

func (s *Service) deliverReply(ctx context.Context, result ReplyResult) (ReplyResult, error) {
	if result.Draft != nil && isBotAutoReplyMessage(result.Message) {
		currentSession, err := s.GetSession(ctx, result.Session.ID)
		if err != nil {
			return ReplyResult{}, err
		}
		result.Session = currentSession
		if s.shouldBlockDraftAutoSend(currentSession, *result.Draft) {
			return s.blockAutoSendAttempt(ctx, result, currentSession)
		}
	}

	delivery, err := s.sender.SendReply(ctx, SendReplyInput{
		Session:  result.Session,
		Message:  result.Message,
		Outbound: result.Outbound,
	})
	if err != nil {
		if _, markErr := s.store.MarkReplyDeliveryFailure(ctx, MarkReplyDeliveryFailureInput{
			SessionID:  result.Session.ID,
			MessageID:  result.Message.ID,
			OutboundID: result.Outbound.ID,
			ErrorText:  err.Error(),
		}); markErr != nil {
			return ReplyResult{}, fmt.Errorf("%w: %v (mark failure: %v)", ErrReplyDeliveryFailed, err, markErr)
		}
		return ReplyResult{}, fmt.Errorf("%w: %v", ErrReplyDeliveryFailed, err)
	}

	updated, err := s.store.MarkReplyDeliverySent(ctx, MarkReplyDeliverySentInput{
		SessionID:         result.Session.ID,
		MessageID:         result.Message.ID,
		OutboundID:        result.Outbound.ID,
		ProviderMessageID: strings.TrimSpace(delivery.ProviderMessageID),
		ProviderStatus:    normalizeReplyProviderStatus(delivery.ProviderStatus),
		Payload:           delivery.Payload,
		SentAt:            delivery.SentAt,
	})
	if err != nil {
		return ReplyResult{}, err
	}
	if updated.Draft == nil {
		updated.Draft = result.Draft
	}

	return updated, nil
}

func (s *Service) maybeAutoSendDraft(ctx context.Context, result ReprocessResult) (ReprocessResult, error) {
	if result.Draft == nil {
		return result, nil
	}
	currentSession, err := s.GetSession(ctx, result.Session.ID)
	if err != nil {
		return ReprocessResult{}, err
	}
	result.Session = currentSession
	if s.shouldBlockDraftAutoSend(currentSession, *result.Draft) {
		return s.markDraftAutoSendBlocked(ctx, result, currentSession)
	}
	messages, err := s.store.ListMessages(ctx, result.Session.ID, normalizeListMessagesFilter(ListMessagesFilter{Limit: 50}))
	if err != nil {
		return ReprocessResult{}, err
	}

	if result.Draft != nil {
		if reason := detectBookingAutoSendBlockReason(messages, *result.Draft); reason != "" {
			blockState := deriveCanonicalConversationState(result.Session, messages, "")
			s.logReprocess(
				"chat reprocess event=auto_send_blocked reason=%s session_id=%s draft_message_id=%s phase=%s",
				reason,
				result.Session.ID,
				result.Draft.ID,
				blockState.Phase,
			)
			return s.markDraftAutoSendReviewRequired(ctx, result, reason)
		}
	}
	if !s.canAutoSendDraft(result.Session, *result.Draft) {
		s.logReprocess(
			"chat reprocess event=maybe_auto_send_skipped session_id=%s draft_message_id=%s reason=can_auto_send_false auto_send_status=%s auto_send_reasons=%v sender_enabled=%t handoff_status=%s current_owner_user_id=%s draft_direction=%s draft_processing_status=%s draft_body_present=%t",
			result.Session.ID,
			result.Draft.ID,
			readDraftAutoSendStatus(*result.Draft),
			readDraftAutoSendReasons(*result.Draft),
			s.sender != nil && s.sender.Enabled(),
			result.Session.HandoffStatus,
			strings.TrimSpace(result.Session.CurrentOwnerUserID),
			result.Draft.Direction,
			result.Draft.ProcessingStatus,
			strings.TrimSpace(result.Draft.Body) != "",
		)
		return result, nil
	}

	idempotencyKey := buildAutoSendReplyIdempotencyKey(result.Draft.ID)
	if existing, err := s.store.FindReplyByIdempotency(ctx, result.Session.ID, idempotencyKey); err != nil {
		return ReprocessResult{}, err
	} else if existing != nil {
		if existing.Draft == nil {
			existing.Draft = result.Draft
		}
		if s.canDeliverReply(existing.Outbound) {
			delivered, err := s.deliverReply(ctx, *existing)
			if err != nil {
				return ReprocessResult{}, err
			}
			result.Session = delivered.Session
			if delivered.Draft != nil {
				result.Draft = delivered.Draft
			}
			result.Reason = "draft_auto_sent"
			return result, nil
		}
		result.Session = existing.Session
		if existing.Draft != nil {
			result.Draft = existing.Draft
		}
		if strings.TrimSpace(existing.Outbound.ProviderMessageID) != "" || strings.EqualFold(existing.Outbound.Status, "SENT") {
			result.Reason = "draft_auto_sent"
		}
		return result, nil
	}

	reply, err := s.store.CreateAutomationReply(ctx, CreateAutomationReplyInput{
		SessionID:      result.Session.ID,
		DraftMessageID: result.Draft.ID,
		IdempotencyKey: idempotencyKey,
		SenderName:     firstNonEmpty(strings.TrimSpace(result.Draft.SenderName), "SHABAS"),
		Metadata: map[string]interface{}{
			"automation_trigger": "AUTO_SEND_ELIGIBLE",
		},
	}, time.Duration(s.cfg.ChatDebounceWindowMS)*time.Millisecond)
	if err != nil {
		if IsUniqueViolation(err) {
			return s.maybeAutoSendDraft(ctx, result)
		}
		if errors.Is(err, ErrReprocessRequiresBot) {
			return s.markDraftAutoSendBlocked(ctx, result, currentSession)
		}
		return ReprocessResult{}, err
	}
	reply.Draft = result.Draft
	if s.canDeliverReply(reply.Outbound) {
		delivered, err := s.deliverReply(ctx, reply)
		if err != nil {
			return ReprocessResult{}, err
		}
		result.Session = delivered.Session
		if delivered.Draft != nil {
			result.Draft = delivered.Draft
		}
		result.Reason = "draft_auto_sent"
		return result, nil
	}

	result.Session = reply.Session
	result.Reason = "draft_auto_send_pending"
	return result, nil
}

func (s *Service) finishReprocessWithAutoSend(ctx context.Context, result ReprocessResult, trigger string, jobRunID string) (ReprocessResult, error) {
	draftID := ""
	if result.Draft != nil {
		draftID = strings.TrimSpace(result.Draft.ID)
	}
	s.logReprocess(
		"chat reprocess event=maybe_auto_send_draft_start session_id=%s trigger=%s job_run_id=%s reason=%s has_draft=%t draft_message_id=%s",
		result.Session.ID,
		trigger,
		jobRunID,
		strings.TrimSpace(result.Reason),
		result.Draft != nil,
		draftID,
	)
	updated, err := s.maybeAutoSendDraft(ctx, result)
	if err != nil {
		s.logReprocess(
			"chat reprocess event=maybe_auto_send_draft_failed session_id=%s trigger=%s job_run_id=%s error=%v",
			result.Session.ID,
			trigger,
			jobRunID,
			err,
		)
		return ReprocessResult{}, err
	}
	updatedDraftID := ""
	if updated.Draft != nil {
		updatedDraftID = strings.TrimSpace(updated.Draft.ID)
	}
	s.logReprocess(
		"chat reprocess event=maybe_auto_send_draft_done session_id=%s trigger=%s job_run_id=%s reason=%s has_draft=%t draft_message_id=%s",
		updated.Session.ID,
		trigger,
		jobRunID,
		strings.TrimSpace(updated.Reason),
		updated.Draft != nil,
		updatedDraftID,
	)
	return updated, nil
}

func (s *Service) canAutoSendDraft(session Session, draft Message) bool {
	if s.sender == nil || !s.sender.Enabled() {
		return false
	}
	if session.HandoffStatus != "BOT" || strings.TrimSpace(session.CurrentOwnerUserID) != "" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(draft.Direction), "OUTBOUND") {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(draft.ProcessingStatus), messageStatusAutomationDraft) {
		return false
	}
	if strings.TrimSpace(draft.Body) == "" {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(readDraftAutoSendStatus(draft))) {
	case draftAutoSendStatusEligible, draftAutoSendStatusRetryPending:
		return true
	default:
		return false
	}
}

func readDraftAutoSendStatus(draft Message) string {
	return firstNonEmpty(
		asString(draft.NormalizedPayload["auto_send_status"]),
		asString(draft.Payload["auto_send_status"]),
	)
}

func readDraftAutoSendReasons(draft Message) []string {
	return firstNonEmptyStringSlice(
		asStringSlice(draft.NormalizedPayload["auto_send_reasons"]),
		asStringSlice(draft.Payload["auto_send_reasons"]),
	)
}

func isBotAutoReplyMessage(message Message) bool {
	return strings.EqualFold(
		firstNonEmpty(asString(message.Payload["mode"]), asString(message.NormalizedPayload["mode"])),
		"BOT_AUTO_REPLY",
	)
}

func (s *Service) shouldBlockDraftAutoSend(session Session, draft Message) bool {
	if s.sender == nil || !s.sender.Enabled() {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(readDraftAutoSendStatus(draft))) {
	case draftAutoSendStatusEligible, draftAutoSendStatusRetryPending:
		return session.HandoffStatus != "BOT" || strings.TrimSpace(session.CurrentOwnerUserID) != ""
	default:
		return false
	}
}

func (s *Service) markDraftAutoSendBlocked(ctx context.Context, result ReprocessResult, session Session) (ReprocessResult, error) {
	observedAt := time.Now().UTC()
	updated, err := s.store.UpdateDraftAutoSendState(ctx, UpdateDraftAutoSendStateInput{
		SessionID:      session.ID,
		DraftMessageID: result.Draft.ID,
		AutoSendStatus: draftAutoSendStatusBlockedHuman,
		AutoSendReasons: mergeDistinctStrings(
			readDraftAutoSendReasons(*result.Draft),
			draftAutoSendReasonHumanHandoff,
		),
		Payload: map[string]interface{}{
			"auto_send_blocked":      true,
			"auto_send_blocked_at":   observedAt.UTC().Format(time.RFC3339Nano),
			"auto_send_block_reason": draftAutoSendReasonHumanHandoff,
			"handoff_status":         session.HandoffStatus,
		},
		Agent: buildDraftAutoSendBlockedAgentState(session.Metadata, *result.Draft, session, observedAt),
	})
	if err != nil {
		return ReprocessResult{}, err
	}
	result.Session = updated.Session
	result.Draft = &updated.Message
	result.Reason = "draft_auto_send_blocked_human"
	result.Idempotent = true
	return result, nil
}

func (s *Service) blockAutoSendAttempt(ctx context.Context, result ReplyResult, session Session) (ReplyResult, error) {
	errorText := "auto-send blocked by active human handoff"
	if strings.TrimSpace(result.Outbound.ProviderMessageID) == "" && !strings.EqualFold(strings.TrimSpace(result.Outbound.Status), "SEND_FAILED") {
		failed, err := s.store.MarkReplyDeliveryFailure(ctx, MarkReplyDeliveryFailureInput{
			SessionID:  session.ID,
			MessageID:  result.Message.ID,
			OutboundID: result.Outbound.ID,
			ErrorText:  errorText,
		})
		if err != nil {
			return ReplyResult{}, err
		}
		result.Message = failed.Message
		result.Outbound = failed.Outbound
	}

	updated, err := s.store.UpdateDraftAutoSendState(ctx, UpdateDraftAutoSendStateInput{
		SessionID:      session.ID,
		DraftMessageID: result.Draft.ID,
		AutoSendStatus: draftAutoSendStatusBlockedHuman,
		AutoSendReasons: mergeDistinctStrings(
			readDraftAutoSendReasons(*result.Draft),
			draftAutoSendReasonHumanHandoff,
		),
		Payload: map[string]interface{}{
			"auto_send_blocked":      true,
			"auto_send_blocked_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"auto_send_block_reason": draftAutoSendReasonHumanHandoff,
			"handoff_status":         session.HandoffStatus,
		},
		Agent: buildDraftAutoSendBlockedAgentState(session.Metadata, *result.Draft, session, time.Now().UTC()),
	})
	if err != nil {
		return ReplyResult{}, err
	}
	result.Session = updated.Session
	draft := updated.Message
	result.Draft = &draft
	return result, nil
}

func normalizeReplyProviderStatus(status string) string {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	if normalized == "" {
		return "SENT"
	}
	return normalized
}

func normalizeReplyMediaType(raw string, mimeType string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "IMAGE":
		return "IMAGE", true
	case "AUDIO":
		return "AUDIO", true
	case "DOCUMENT":
		return "DOCUMENT", true
	case "":
		mime := strings.ToLower(strings.TrimSpace(mimeType))
		switch {
		case strings.HasPrefix(mime, "image/"):
			return "IMAGE", true
		case strings.HasPrefix(mime, "audio/"):
			return "AUDIO", true
		default:
			return "DOCUMENT", true
		}
	default:
		return "", false
	}
}

func defaultReplyMediaBody(mediaType string) string {
	switch strings.ToUpper(strings.TrimSpace(mediaType)) {
	case "IMAGE":
		return "[imagem]"
	case "AUDIO":
		return "[audio]"
	default:
		return "[documento]"
	}
}

func defaultReplyMediaMIMEType(mediaType string) string {
	switch strings.ToUpper(strings.TrimSpace(mediaType)) {
	case "IMAGE":
		return "image/jpeg"
	case "AUDIO":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}

func defaultReplyMediaFileName(mediaType string, mimeType string) string {
	ext := "bin"
	switch strings.ToUpper(strings.TrimSpace(mediaType)) {
	case "IMAGE":
		ext = "jpg"
	case "AUDIO":
		ext = "ogg"
	default:
		ext = extensionForMIMEType(mimeType)
	}
	return fmt.Sprintf("media-%d.%s", time.Now().UTC().Unix(), ext)
}

func extensionForMIMEType(mimeType string) string {
	mime := strings.ToLower(strings.TrimSpace(mimeType))
	switch mime {
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "application/pdf":
		return "pdf"
	case "audio/mpeg":
		return "mp3"
	case "audio/mp4", "audio/m4a":
		return "m4a"
	case "audio/webm":
		return "webm"
	case "audio/wav":
		return "wav"
	case "audio/ogg":
		return "ogg"
	default:
		return "bin"
	}
}

func buildQueuedAutomationDraftPayload(session Session, draftID string, metadata map[string]interface{}, observedAt time.Time) (map[string]interface{}, map[string]interface{}) {
	payload := map[string]interface{}{
		"mode":                     "AUTOMATION_DRAFT",
		"draft_idempotency_key":    draftID,
		"generated_at":             observedAt.UTC().Format(time.RFC3339Nano),
		"current_turn_message_ids": []string{},
		"session_channel":          session.Channel,
		"contact_key":              session.ContactKey,
	}
	normalized := map[string]interface{}{
		"mode":                     "AUTOMATION_DRAFT",
		"draft_idempotency_key":    draftID,
		"generated_at":             observedAt.UTC().Format(time.RFC3339Nano),
		"agent_status":             agentStatusDraftGenerated,
		"current_turn_message_ids": []string{},
		"tool_call_count":          0,
	}
	for key, value := range metadata {
		payload[key] = value
		normalized[key] = value
	}
	return payload, normalized
}

func buildQueuedAutomationDraftAgentState(metadata map[string]interface{}, draftID string, draftMetadata map[string]interface{}, observedAt time.Time) map[string]interface{} {
	state := cloneNestedMetadataMap(metadata, "agent")
	state["status"] = agentStatusDraftGenerated
	state["draft_idempotency_key"] = draftID
	state["draft_generated_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["requested_at"] = observedAt.UTC().Format(time.RFC3339Nano)
	state["draft_message_ids"] = []string{}
	state["tool_calls_count"] = 0
	if source := strings.TrimSpace(asString(draftMetadata["source"])); source != "" {
		state["source"] = source
	}
	if jobName := strings.TrimSpace(asString(draftMetadata["automation_job_name"])); jobName != "" {
		state["automation_job_name"] = jobName
	}
	if draftModel := strings.TrimSpace(asString(draftMetadata["draft_model"])); draftModel != "" {
		state["draft_model"] = draftModel
	}
	if paymentID := strings.TrimSpace(asString(draftMetadata["payment_id"])); paymentID != "" {
		state["payment_id"] = paymentID
	}
	if bookingID := strings.TrimSpace(asString(draftMetadata["booking_id"])); bookingID != "" {
		state["booking_id"] = bookingID
	}
	return state
}

func normalizeTimePointer(input *time.Time) *time.Time {
	if input == nil {
		return nil
	}
	value := input.UTC()
	return &value
}
