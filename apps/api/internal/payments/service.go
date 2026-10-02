package payments

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"schumacher-tur/api/internal/shared/config"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repo               *Repository
	client             *Client
	paymentRecipientID string
}

func NewService(repo *Repository, cfg config.Config) *Service {
	return &Service{
		repo:               repo,
		client:             NewClient(cfg.PagarmeBaseURL, cfg.PagarmeSecretKey),
		paymentRecipientID: strings.TrimSpace(cfg.PagarmePaymentRecipientID),
	}
}

func (s *Service) Create(ctx context.Context, input CreatePaymentInput) (Payment, json.RawMessage, error) {
	if input.Method != "PIX" {
		return Payment{}, nil, errors.New("invalid method")
	}
	if s.client == nil || strings.TrimSpace(s.client.apiKey) == "" {
		return Payment{}, nil, errors.New("PAGARME_SECRET_KEY is required for automatic payments")
	}
	if strings.TrimSpace(s.paymentRecipientID) == "" {
		return Payment{}, nil, errors.New("PAGARME_PAYMENT_RECIPIENT_ID is required for automatic payments")
	}

	desc := strings.TrimSpace(input.Description)
	if desc == "" {
		desc = "Passagem"
	}

	paymentID := uuid.NewString()
	bookingCtx, err := s.repo.GetBookingPaymentContext(ctx, input.BookingID)
	if err != nil {
		return Payment{}, nil, err
	}
	orderCode := strings.TrimSpace(bookingCtx.ReservationCode)
	if orderCode == "" {
		orderCode = paymentID
	}
	cents := int64(math.Round(input.Amount * 100))
	orderReq := OrderRequest{
		Code: orderCode,
		Items: []OrderItem{{
			Code:        orderCode,
			Amount:      cents,
			Description: desc,
			Quantity:    1,
		}},
		Payments: []OrderPayment{{
			PaymentMethod: "pix",
			Pix: &PixPayment{
				ExpiresIn: 3600,
			},
			Split: BuildSingleRecipientSplit(s.paymentRecipientID),
		}},
		Metadata: map[string]string{
			"booking_id":       input.BookingID,
			"payment_id":       paymentID,
			"reservation_code": bookingCtx.ReservationCode,
			"trip_id":          bookingCtx.TripID,
		},
	}
	if input.Customer != nil {
		if !IsSupportedDocument(input.Customer.Document) {
			return Payment{}, nil, errors.New("customer document must be a valid CPF or CNPJ")
		}
		orderReq.Customer = BuildCustomer(input.Customer, input.BookingID)
	}

	order, raw, err := s.client.CreateOrder(ctx, orderReq)
	if err != nil {
		return Payment{}, nil, err
	}
	if _, pixCode := ExtractCheckoutAndPix(raw); strings.TrimSpace(pixCode) == "" && strings.TrimSpace(order.ID) != "" {
		fetchedOrder, fetchedRaw, fetchErr := s.client.GetOrderByID(ctx, order.ID)
		if fetchErr != nil {
			log.Printf("event=pagarme_order_fetch_after_create_failed order_id=%s error=%v", strings.TrimSpace(order.ID), fetchErr)
		} else if _, fetchedPixCode := ExtractCheckoutAndPix(fetchedRaw); strings.TrimSpace(fetchedPixCode) != "" {
			order = fetchedOrder
			raw = fetchedRaw
		}
	}

	providerRef := strings.TrimSpace(order.PrimaryChargeID())
	if providerRef == "" {
		providerRef = strings.TrimSpace(order.ID)
	}
	if providerRef == "" {
		return Payment{}, raw, errors.New("pagarme response missing charge/order id")
	}

	meta, _ := json.Marshal(map[string]interface{}{"order": order})
	created, err := s.repo.CreateWithProvider(ctx, paymentID, input.BookingID, input.Amount, input.Method, "PAGARME", providerRef, meta)
	if err != nil {
		return Payment{}, raw, err
	}
	return created, raw, nil
}

func (s *Service) CreateManual(ctx context.Context, input ManualPaymentInput) (Payment, error) {
	payment, err := s.repo.CreateManual(ctx, input.BookingID, input.Amount, input.Method, input.Notes)
	if err != nil {
		return Payment{}, err
	}
	s.notifyPaymentConfirmed(ctx, payment)
	return payment, nil
}

func (s *Service) GetStatus(ctx context.Context, id string) (PaymentStatusResponse, error) {
	payment, err := s.repo.Get(ctx, id)
	if err != nil {
		return PaymentStatusResponse{}, err
	}
	return PaymentStatusResponse{
		ID:          payment.ID,
		Status:      payment.Status,
		Amount:      payment.Amount,
		Provider:    payment.Provider,
		ProviderRef: payment.ProviderRef,
		Metadata:    payment.Metadata,
	}, nil
}

func (s *Service) List(ctx context.Context, filter PaymentListFilter) ([]Payment, error) {
	return s.repo.List(ctx, filter)
}

func (s *Service) HandleWebhook(ctx context.Context, event WebhookEvent) error {
	if !event.IsPaidEvent() || event.ProviderRef == "" {
		return nil
	}
	payment, justMarkedPaid, err := s.repo.MarkPaidAndConfirmBooking(ctx, event.ProviderRef, event.Raw)
	if err != nil {
		if IsNotFound(err) {
			_ = s.repo.AddEvent(ctx, nil, event.Type, event.Raw)
			return nil
		}
		return err
	}
	if justMarkedPaid {
		s.notifyPaymentConfirmed(ctx, payment)
	}
	return nil
}

func (s *Service) Sync(ctx context.Context, paymentID string) (PaymentSyncResponse, error) {
	payment, err := s.repo.Get(ctx, paymentID)
	if err != nil {
		return PaymentSyncResponse{}, err
	}
	bookingStatus, err := s.repo.GetBookingStatus(ctx, payment.BookingID)
	if err != nil {
		return PaymentSyncResponse{}, err
	}
	if payment.Provider == nil || strings.TrimSpace(*payment.Provider) != "PAGARME" {
		return PaymentSyncResponse{Payment: payment, BookingStatus: bookingStatus, Synced: false}, nil
	}

	orderID := extractPagarmeOrderID(payment.Metadata)
	if orderID == "" {
		return PaymentSyncResponse{Payment: payment, BookingStatus: bookingStatus, Synced: false}, nil
	}

	order, raw, err := s.client.GetOrderByID(ctx, orderID)
	if err != nil {
		return PaymentSyncResponse{}, err
	}
	providerRef := strings.TrimSpace(order.PrimaryChargeID())
	if providerRef == "" && payment.ProviderRef != nil {
		providerRef = strings.TrimSpace(*payment.ProviderRef)
	}
	if isPaidOrderStatus(order) && providerRef != "" && payment.Status != "PAID" {
		updated, justMarkedPaid, err := s.repo.MarkPaidAndConfirmBooking(ctx, providerRef, raw)
		if err != nil {
			return PaymentSyncResponse{}, err
		}
		payment = updated
		if justMarkedPaid {
			s.notifyPaymentConfirmed(ctx, payment)
		}
	}

	bookingStatus, err = s.repo.GetBookingStatus(ctx, payment.BookingID)
	if err != nil {
		return PaymentSyncResponse{}, err
	}
	if payment.ProviderRef != nil {
		providerRef = strings.TrimSpace(*payment.ProviderRef)
	}
	log.Printf("event=payment_synced payment_id=%s booking_id=%s payment_status=%s booking_status=%s provider_ref=%s", payment.ID, payment.BookingID, payment.Status, bookingStatus, providerRef)
	return PaymentSyncResponse{Payment: payment, BookingStatus: bookingStatus, Synced: true}, nil
}

var (
	// ErrPagamentoJaPago: a cobranca ja foi paga; nada e cancelado.
	ErrPagamentoJaPago = errors.New("pagamento ja pago")
	// ErrPagamentoNaoCancelavel: a cobranca nao esta pendente no provedor.
	ErrPagamentoNaoCancelavel = errors.New("pagamento nao esta pendente")
)

// CancelarPendente cancela um PIX ainda nao pago: confere o pedido na Pagar.me
// e so cancela a cobranca se ela estiver pendente (cobranca paga nunca e
// cancelada, porque viraria estorno). Se o provedor mostrar que foi paga,
// registra o pagamento e devolve ErrPagamentoJaPago. Ja cancelado: nil.
func (s *Service) CancelarPendente(ctx context.Context, paymentID string) error {
	p, err := s.repo.Get(ctx, paymentID)
	if err != nil {
		return err
	}
	switch strings.ToUpper(strings.TrimSpace(p.Status)) {
	case "PAID":
		return ErrPagamentoJaPago
	case "CANCELLED", "FAILED", "REFUNDED":
		return nil
	}
	raw := json.RawMessage(`{}`)
	if p.Provider != nil && strings.TrimSpace(*p.Provider) == "PAGARME" {
		orderID := extractPagarmeOrderID(p.Metadata)
		if orderID == "" {
			return ErrPagamentoNaoCancelavel
		}
		order, oraw, err := s.client.GetOrderByID(ctx, orderID)
		if err != nil {
			return err
		}
		acao, cobranca := acaoCancelamento(order)
		switch acao {
		case cancelamentoPago:
			ref := order.PrimaryChargeID()
			if ref == "" && p.ProviderRef != nil {
				ref = strings.TrimSpace(*p.ProviderRef)
			}
			if ref != "" {
				if pago, novo, err := s.repo.MarkPaidAndConfirmBooking(ctx, ref, oraw); err == nil && novo {
					s.notifyPaymentConfirmed(ctx, pago)
				}
			}
			return ErrPagamentoJaPago
		case cancelamentoJaCancelado:
			raw = oraw
		case cancelamentoCancelar:
			if raw, err = s.client.CancelCharge(ctx, cobranca.ID); err != nil {
				return err
			}
		default:
			return ErrPagamentoNaoCancelavel
		}
	}
	return s.repo.MarcarCancelado(ctx, p.ID, raw)
}

const (
	cancelamentoPago          = "pago"
	cancelamentoJaCancelado   = "ja_cancelado"
	cancelamentoCancelar      = "cancelar"
	cancelamentoNaoCancelavel = "nao_cancelavel"
)

// acaoCancelamento decide, pelo pedido na Pagar.me, o que fazer antes de
// cancelar: pago (nunca cancela), ja cancelado, cancelar (cobranca pendente)
// ou nao cancelavel (qualquer outro estado, ex.: processando).
func acaoCancelamento(order OrderResponse) (string, OrderCharge) {
	if isPaidOrderStatus(order) {
		return cancelamentoPago, OrderCharge{}
	}
	var cobranca OrderCharge
	for _, ch := range order.Charges {
		if strings.TrimSpace(ch.ID) != "" {
			cobranca = ch
			break
		}
	}
	switch strings.ToLower(strings.TrimSpace(cobranca.Status)) {
	case "canceled", "cancelled", "failed":
		return cancelamentoJaCancelado, cobranca
	case "pending", "waiting_payment":
		return cancelamentoCancelar, cobranca
	}
	return cancelamentoNaoCancelavel, cobranca
}

// DefinirSinalReserva ajusta o valor que confirma a reserva (sinal ou total),
// enquanto nada foi pago. Usado quando o cliente troca integral/sinal.
func (s *Service) DefinirSinalReserva(ctx context.Context, bookingID string, valor float64) error {
	return s.repo.DefinirSinalReserva(ctx, bookingID, valor)
}

func parseProviderData(raw []byte) (interface{}, *string, *string) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	var parsed interface{}
	_ = json.Unmarshal(raw, &parsed)

	obj, ok := parsed.(map[string]interface{})
	if !ok {
		return parsed, nil, nil
	}

	checkout, pixCode := extractPagarmeCheckoutAndPix(obj)
	return parsed, checkout, pixCode
}

func ExtractCheckoutAndPix(raw json.RawMessage) (string, string) {
	_, checkout, pixCode := parseProviderData(raw)
	return stringValue(checkout), stringValue(pixCode)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func readStringFromMap(obj map[string]interface{}, key string) *string {
	raw, ok := obj[key]
	if !ok {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func extractPagarmeCheckoutAndPix(obj map[string]interface{}) (*string, *string) {
	checkout := readStringFromMap(obj, "qr_code_url")
	if checkout == nil {
		checkout = readStringFromMap(obj, "checkout_url")
	}
	pixCode := readStringFromMap(obj, "qr_code")
	if pixCode == nil {
		pixCode = readStringFromMap(obj, "pix_code")
	}
	if checkout != nil || pixCode != nil {
		return checkout, pixCode
	}

	if dataObj, ok := obj["data"].(map[string]interface{}); ok {
		checkout = readStringFromMap(dataObj, "url")
		if checkout == nil {
			checkout = readStringFromMap(dataObj, "qr_code_url")
		}
		pixCode = readStringFromMap(dataObj, "pixQrCode")
		if pixCode == nil {
			pixCode = readStringFromMap(dataObj, "qr_code")
		}
		if checkout != nil || pixCode != nil {
			return checkout, pixCode
		}
	}

	charges, _ := obj["charges"].([]interface{})
	for _, rawCharge := range charges {
		charge, ok := rawCharge.(map[string]interface{})
		if !ok {
			continue
		}
		lastTx, _ := charge["last_transaction"].(map[string]interface{})
		if lastTx == nil {
			continue
		}
		checkout := readStringFromMap(lastTx, "qr_code_url")
		if checkout == nil {
			checkout = readStringFromMap(lastTx, "checkout_url")
		}
		pixCode := readStringFromMap(lastTx, "qr_code")
		if pixCode == nil {
			pixCode = readStringFromMap(lastTx, "pix_code")
		}
		if checkout != nil || pixCode != nil {
			return checkout, pixCode
		}
	}
	return nil, nil
}

func extractPagarmeOrderID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Order *struct {
			ID string `json:"id"`
		} `json:"order"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if payload.Order == nil {
		return ""
	}
	return strings.TrimSpace(payload.Order.ID)
}

func isPaidOrderStatus(order OrderResponse) bool {
	switch strings.ToUpper(strings.TrimSpace(order.Status)) {
	case "PAID", "CLOSED":
		return true
	}
	for _, charge := range order.Charges {
		switch strings.ToUpper(strings.TrimSpace(charge.Status)) {
		case "PAID", "CAPTURED":
			return true
		}
	}
	return false
}

func (s *Service) notifyPaymentConfirmed(ctx context.Context, payment Payment) {
	_ = ctx
	if s == nil || payment.BookingID == "" {
		return
	}

	notification, err := s.repo.GetBookingNotificationContext(ctx, payment.BookingID)
	if err != nil {
		log.Printf("event=payment_notification_failed booking_id=%s payment_id=%s reason=booking_context error=%v", payment.BookingID, payment.ID, err)
		return
	}
	if strings.TrimSpace(notification.CustomerPhone) == "" {
		log.Printf("event=payment_notification_skipped booking_id=%s payment_id=%s reason=missing_phone", payment.BookingID, payment.ID)
		return
	}

	log.Printf("event=payment_notification_pending_automation_job booking_id=%s reservation_code=%s payment_id=%s payment_status=%s amount_due=%.2f customer_phone=%s", notification.BookingID, notification.ReservationCode, payment.ID, notification.PaymentStatus, notification.AmountDue, notification.CustomerPhone)
}
