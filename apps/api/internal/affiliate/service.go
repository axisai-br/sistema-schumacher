package affiliate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

var (
	ErrUnauthorized         = errors.New("affiliate unauthorized")
	ErrRoleRequired         = errors.New("financeiro role required")
	ErrRecipientNotLinked   = errors.New("recipient not linked")
	ErrInsufficientBalance  = errors.New("insufficient available balance")
	ErrInvalidAmount        = errors.New("invalid withdraw amount")
	ErrWebhookUnauthorized  = errors.New("invalid webhook credentials")
	ErrPagarmeNotConfigured = errors.New("pagarme secret key is required")
)

type Service struct {
	repo   *Repository
	client *PagarmeClient
	cfg    config.Config
}

func NewService(repo *Repository, cfg config.Config) *Service {
	return &Service{
		repo:   repo,
		client: NewPagarmeClient(cfg.PagarmeAPIBaseURL, cfg.PagarmeSecretKey),
		cfg:    cfg,
	}
}

func (s *Service) GetUserContext(ctx context.Context, userID string) (AuthorizationContext, error) {
	return s.repo.GetAuthorizationContext(ctx, userID)
}

func (s *Service) GetBalance(ctx context.Context, userID string) (BalanceResponse, error) {
	authCtx, err := s.authorize(ctx, userID)
	if err != nil {
		return BalanceResponse{}, err
	}
	if strings.TrimSpace(s.cfg.PagarmeSecretKey) == "" {
		return BalanceResponse{}, ErrPagarmeNotConfigured
	}

	balance, raw, err := s.client.GetRecipientBalance(ctx, authCtx.RecipientID)
	if err != nil {
		log.Printf("event=affiliate_balance_error user_id=%s err=%v", userID, err)
		return BalanceResponse{}, s.mapProviderError(err, raw)
	}
	details, _, detailsErr := s.client.GetRecipientDetails(ctx, authCtx.RecipientID)
	if detailsErr != nil {
		log.Printf("event=affiliate_recipient_details_error user_id=%s err=%v", userID, detailsErr)
	} else {
		balance.AnticipationInfo = &details.AnticipationInfo
	}
	balance.CanWithdraw = balance.AvailableAmount > 0
	if !balance.CanWithdraw {
		balance.WithdrawBlockReason = "Sem saldo disponivel para saque"
	}
	return balance, nil
}

func (s *Service) Withdraw(ctx context.Context, userID string, amount int64) (WithdrawResponse, error) {
	if amount <= 0 {
		return WithdrawResponse{}, ErrInvalidAmount
	}
	authCtx, err := s.authorize(ctx, userID)
	if err != nil {
		return WithdrawResponse{}, err
	}
	if strings.TrimSpace(s.cfg.PagarmeSecretKey) == "" {
		return WithdrawResponse{}, ErrPagarmeNotConfigured
	}

	balance, raw, err := s.client.GetRecipientBalance(ctx, authCtx.RecipientID)
	if err != nil {
		log.Printf("event=affiliate_balance_before_withdraw_error user_id=%s err=%v", userID, err)
		return WithdrawResponse{}, s.mapProviderError(err, raw)
	}
	if amount > balance.AvailableAmount {
		return WithdrawResponse{}, ErrInsufficientBalance
	}

	idempotencyKey := newIdempotencyKey(userID, amount, time.Now().UTC())
	providerRequest, _ := json.Marshal(map[string]interface{}{
		"amount":       amount,
		"recipient_id": authCtx.RecipientID,
	})
	withdrawal, err := s.repo.InsertWithdrawal(ctx, userID, authCtx.RecipientID, amount, firstNonEmpty(balance.Currency, "BRL"), idempotencyKey, providerRequest)
	if err != nil {
		return WithdrawResponse{}, err
	}

	transfer, reqRaw, respRaw, err := s.client.CreateTransfer(ctx, amount, authCtx.RecipientID, idempotencyKey)
	if err != nil {
		_ = s.repo.MarkWithdrawalFailed(ctx, withdrawal.ID, respRaw, maskProviderError(err.Error()))
		log.Printf("event=affiliate_withdraw_provider_error withdrawal_id=%s user_id=%s err=%v", withdrawal.ID, userID, err)
		return WithdrawResponse{
			WithdrawalID:    withdrawal.ID,
			Status:          "failed",
			Message:         s.safePublicError("Nao foi possivel processar o saque no momento"),
			RequestedAmount: amount,
			Currency:        firstNonEmpty(balance.Currency, "BRL"),
		}, nil
	}

	_ = reqRaw
	if err := s.repo.MarkWithdrawalFromTransfer(ctx, withdrawal.ID, transfer); err != nil {
		return WithdrawResponse{}, err
	}

	outStatus := "pending"
	message := "Saque solicitado com sucesso"
	switch normalizeTransferStatus(transfer.Status) {
	case "SUCCESS":
		outStatus = "success"
		message = "Saque concluido com sucesso"
	case "FAILED":
		outStatus = "failed"
		message = "Saque recusado"
	}

	return WithdrawResponse{
		WithdrawalID:    withdrawal.ID,
		TransferID:      transfer.ID,
		Status:          outStatus,
		Message:         message,
		RequestedAmount: amount,
		Currency:        firstNonEmpty(balance.Currency, "BRL"),
	}, nil
}

func (s *Service) ListWithdrawalsHistory(ctx context.Context, userID string, filter ListFilter) (WithdrawalsHistoryResponse, error) {
	if _, err := s.authorize(ctx, userID); err != nil {
		return WithdrawalsHistoryResponse{}, err
	}

	pendingTransferIDs, err := s.repo.FindPendingTransferIDs(ctx, userID, filter)
	if err == nil && strings.TrimSpace(s.cfg.PagarmeSecretKey) != "" {
		for _, transferID := range pendingTransferIDs {
			transfer, getErr := s.client.GetTransfer(ctx, transferID)
			if getErr != nil {
				log.Printf("event=affiliate_transfer_sync_error transfer_id=%s user_id=%s err=%v", transferID, userID, getErr)
				continue
			}
			if updateErr := s.repo.SyncWithdrawalByTransferID(ctx, transferID, transfer); updateErr != nil {
				log.Printf("event=affiliate_transfer_sync_update_error transfer_id=%s user_id=%s err=%v", transferID, userID, updateErr)
			}
		}
	}

	items, total, err := s.repo.ListWithdrawals(ctx, userID, filter)
	if err != nil {
		return WithdrawalsHistoryResponse{}, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	return WithdrawalsHistoryResponse{
		Items: items,
		Pagination: Pagination{
			Limit:         limit,
			Offset:        offset,
			TotalEstimate: total,
		},
	}, nil
}

func (s *Service) HandleWebhook(ctx context.Context, body []byte, signature string, basicUser string, basicPass string) error {
	if strings.TrimSpace(s.cfg.PagarmeWebhookBasicUser) == "" || strings.TrimSpace(s.cfg.PagarmeWebhookBasicPass) == "" {
		return ErrWebhookUnauthorized
	}
	if strings.TrimSpace(basicUser) != strings.TrimSpace(s.cfg.PagarmeWebhookBasicUser) {
		return ErrWebhookUnauthorized
	}
	if strings.TrimSpace(basicPass) != strings.TrimSpace(s.cfg.PagarmeWebhookBasicPass) {
		return ErrWebhookUnauthorized
	}

	eventID, eventType, transferID := extractWebhookIdentifiers(body)
	var eventIDPtr, eventTypePtr, transferIDPtr, signaturePtr *string
	if eventID != "" {
		eventIDPtr = &eventID
	}
	if eventType != "" {
		eventTypePtr = &eventType
	}
	if transferID != "" {
		transferIDPtr = &transferID
	}
	if strings.TrimSpace(signature) != "" {
		signaturePtr = &signature
	}

	webhookRecord, err := s.repo.SaveWebhookEvent(ctx, eventIDPtr, eventTypePtr, transferIDPtr, signaturePtr, body)
	if err != nil {
		if errors.Is(err, ErrWebhookDuplicate) {
			return nil
		}
		return err
	}

	var processingErr error
	if transferID != "" && strings.TrimSpace(s.cfg.PagarmeSecretKey) != "" {
		var transfer PagarmeTransferResult
		transfer, processingErr = s.client.GetTransfer(ctx, transferID)
		if processingErr == nil {
			processingErr = s.repo.SyncWithdrawalByTransferID(ctx, transferID, transfer)
		}
	}
	if err := s.repo.MarkWebhookProcessed(ctx, webhookRecord.ID, processingErr); err != nil {
		return err
	}
	return processingErr
}

func (s *Service) authorize(ctx context.Context, userID string) (AuthorizationContext, error) {
	authCtx, err := s.repo.GetAuthorizationContext(ctx, userID)
	if err != nil {
		return AuthorizationContext{}, err
	}
	if !authCtx.HasRole {
		return AuthorizationContext{}, ErrRoleRequired
	}
	if !authCtx.HasRecipient {
		return AuthorizationContext{}, ErrRecipientNotLinked
	}
	return authCtx, nil
}

func (s *Service) mapProviderError(err error, raw []byte) error {
	if s.cfg.AppEnv == "production" {
		return errors.New("provider unavailable")
	}
	if len(raw) == 0 {
		return err
	}
	return fmt.Errorf("%w: %s", err, string(raw))
}

func (s *Service) safePublicError(message string) string {
	if strings.TrimSpace(message) == "" {
		return "Erro inesperado"
	}
	return message
}

func extractWebhookIdentifiers(payload []byte) (string, string, string) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return "", "", ""
	}

	eventID := readString(data, "id")
	eventType := readString(data, "type")
	if eventType == "" {
		eventType = readString(data, "event")
	}

	transferID := readString(data, "transfer_id")
	if transferID == "" {
		if nested, ok := data["data"].(map[string]interface{}); ok {
			transferID = readString(nested, "transfer_id")
			if transferID == "" {
				if transfer, ok := nested["transfer"].(map[string]interface{}); ok {
					transferID = readString(transfer, "id")
				}
			}
		}
	}
	return eventID, eventType, transferID
}

func readString(obj map[string]interface{}, key string) string {
	v, ok := obj[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func payloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
