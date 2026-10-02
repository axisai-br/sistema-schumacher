package ferramentas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/availability"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/pricing"
)

// ---- fonte de catalogo ----

type fonteFake struct {
	cidades  []Cidade
	aliases  map[string]string
	chamadas int
}

func (f *fonteFake) Cidades(context.Context) ([]Cidade, error) {
	f.chamadas++
	return f.cidades, nil
}
func (f *fonteFake) Aliases(context.Context) (map[string]string, error) { return f.aliases, nil }

func cidadesReais() []Cidade {
	return []Cidade{
		{"ma-si", "Santa Inês", "MA", 950},
		{"ma-mo", "Monção", "MA", 950},
		{"ma-im", "Igarapé do Meio", "MA", 950},
		{"sc-fr", "Fraiburgo", "SC", 950},
		{"sc-mc", "Monte Carlo", "SC", 950},
		{"sc-vi", "Videira", "SC", 950},
		{"sc-cn", "Campos Novos", "SC", 1000},
		{"sc-ch", "Chapecó", "SC", 1100},
		{"sc-co", "Concórdia", "SC", 1100},
		{"sc-ip", "Ipumirim", "SC", 1100},
		{"sc-pe", "Petrolândia", "SC", 1100},
		{"sc-it", "Ituporanga", "SC", 1100},
		{"sc-se", "Seara", "SC", 1100},
	}
}

var agoraFixa = time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)

func novoCatalogoFake() *Catalogo {
	return NovoCatalogo(&fonteFake{cidades: cidadesReais()}, func() time.Time { return agoraFixa })
}

// ---- buscador ----

type buscadorFake struct {
	resultados func(f availability.SearchFilter) []availability.SearchResult
	filtros    []availability.SearchFilter
	err        error
}

func (b *buscadorFake) Search(_ context.Context, f availability.SearchFilter) ([]availability.SearchResult, error) {
	b.filtros = append(b.filtros, f)
	if b.err != nil {
		return nil, b.err
	}
	return b.resultados(f), nil
}

func viagem(trip, origemID, destinoID, origem, destino, data, hora string, vagas int, preco float64) availability.SearchResult {
	return availability.SearchResult{
		TripID: trip, BoardStopID: "ts-" + trip + "-" + origemID, AlightStopID: "ts-" + trip + "-" + destinoID,
		OriginStopID: origemID, DestinationStopID: destinoID,
		OriginDisplayName: origem, DestinationDisplayName: destino,
		OriginDepartTime: hora, TripDate: data, SeatsAvailable: vagas, Price: preco,
		Status: "ACTIVE", TripStatus: "SCHEDULED",
	}
}

// buscador simples sobre uma lista fixa, respeitando stops e datas.
func buscadorDe(base ...availability.SearchResult) *buscadorFake {
	return &buscadorFake{resultados: func(f availability.SearchFilter) []availability.SearchResult {
		var out []availability.SearchResult
		for _, r := range base {
			if f.OriginStopID != "" && r.OriginStopID != f.OriginStopID {
				continue
			}
			if f.DestinationStopID != "" && r.DestinationStopID != f.DestinationStopID {
				continue
			}
			if f.DateFrom != nil && r.TripDate < f.DateFrom.Format("2006-01-02") {
				continue
			}
			if f.DateTo != nil && r.TripDate > f.DateTo.Format("2006-01-02") {
				continue
			}
			out = append(out, r)
		}
		if f.Limit > 0 && len(out) > f.Limit {
			out = out[:f.Limit]
		}
		return out
	}}
}

// ---- reservas ----

type reservasFake struct {
	criadas    []bookings.CreateBookingInput
	porChave   map[string]bookings.BookingDetails
	errCreate  error
	errPorTrip map[string]error // falha so para essa viagem
	total      float64
	itens      []bookings.BookingListItem
	status     string
}

func (r *reservasFake) Create(_ context.Context, in bookings.CreateBookingInput) (bookings.BookingDetails, error) {
	r.criadas = append(r.criadas, in)
	if err := r.errPorTrip[in.TripID]; err != nil {
		return bookings.BookingDetails{}, err
	}
	if r.errCreate != nil {
		return bookings.BookingDetails{}, r.errCreate
	}
	if r.porChave == nil {
		r.porChave = map[string]bookings.BookingDetails{}
	}
	if d, ok := r.porChave[in.IdempotencyKey]; ok {
		return d, nil
	}
	n := len(r.porChave) + 1
	st := r.status
	if st == "" {
		st = "PENDING"
	}
	d := bookings.BookingDetails{Booking: bookings.Booking{
		ID: fmt.Sprintf("bk-%d", n), ReservationCode: fmt.Sprintf("SCH%04d", n), Status: st,
		TotalAmount: in.TotalAmount, DepositAmount: in.DepositAmount, RemainderAmount: in.RemainderAmount,
	}}
	for _, p := range in.Passengers {
		d.Passengers = append(d.Passengers, bookings.BookingPassenger{Name: p.Name, Document: p.Document, IsLapChild: p.IsLapChild})
	}
	r.porChave[in.IdempotencyKey] = d
	return d, nil
}

func (r *reservasFake) Get(_ context.Context, id string) (bookings.BookingDetails, error) {
	for _, d := range r.porChave {
		if d.Booking.ID == id {
			if r.status != "" {
				d.Booking.Status = r.status
			}
			return d, nil
		}
	}
	return bookings.BookingDetails{}, errors.New("nao encontrada")
}

func (r *reservasFake) List(_ context.Context, f bookings.ListFilter) ([]bookings.BookingListItem, error) {
	var out []bookings.BookingListItem
	for _, it := range r.itens {
		if f.ReservationCode == "" || it.ReservationCode == f.ReservationCode {
			out = append(out, it)
		}
	}
	return out, nil
}

// ---- pagamentos ----

type pagamentosFake struct {
	criados    []payments.CreatePaymentInput
	status     map[string]string
	semPix     bool
	errCreate  error
	cancelados []string
	sinais     map[string]float64
}

func (p *pagamentosFake) Create(_ context.Context, in payments.CreatePaymentInput) (payments.Payment, json.RawMessage, error) {
	if p.errCreate != nil {
		return payments.Payment{}, nil, p.errCreate
	}
	p.criados = append(p.criados, in)
	id := fmt.Sprintf("pay-%d", len(p.criados))
	if p.status == nil {
		p.status = map[string]string{}
	}
	p.status[id] = "PENDING"
	raw := json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201PIX` + id + `","expires_at":"2026-09-30T16:00:00Z"}}]}`)
	if p.semPix {
		raw = json.RawMessage(`{}`)
	}
	return payments.Payment{ID: id, BookingID: in.BookingID, Amount: in.Amount, Method: "PIX", Status: "PENDING", CreatedAt: agoraFixa}, raw, nil
}

func (p *pagamentosFake) GetStatus(_ context.Context, id string) (payments.PaymentStatusResponse, error) {
	st, ok := p.status[id]
	if !ok {
		return payments.PaymentStatusResponse{}, errors.New("nao encontrado")
	}
	var amt float64
	var booking string
	_ = booking
	for i, c := range p.criados {
		if fmt.Sprintf("pay-%d", i+1) == id {
			amt = c.Amount
		}
	}
	meta := json.RawMessage(`{"order":{"charges":[{"last_transaction":{"qr_code":"000201PIX` + id + `"}}]}}`)
	return payments.PaymentStatusResponse{ID: id, Status: st, Amount: amt, Metadata: meta}, nil
}

func (p *pagamentosFake) List(_ context.Context, f payments.PaymentListFilter) ([]payments.Payment, error) {
	var out []payments.Payment
	for i, c := range p.criados {
		id := fmt.Sprintf("pay-%d", i+1)
		if f.BookingID == "" || c.BookingID == f.BookingID {
			out = append(out, payments.Payment{ID: id, BookingID: c.BookingID, Amount: c.Amount, Status: p.status[id], CreatedAt: agoraFixa})
		}
	}
	return out, nil
}

// ---- helpers de teste ----

type ambiente struct {
	cat *Catalogo
	b   *buscadorFake
	q   *cotadorFake
	r   *reservasFake
	p   *pagamentosFake
	reg *Registro
	ctx *Contexto
}

func viagensPadrao() []availability.SearchResult {
	return []availability.SearchResult{
		viagem("t1", "sc-ch", "ma-si", "Chapecó/SC", "Santa Inês/MA", "2026-10-10", "18:00", 12, 1100),
		viagem("t2", "sc-ch", "ma-si", "Chapecó/SC", "Santa Inês/MA", "2026-10-20", "18:00", 1, 1100),
		viagem("t3", "sc-vi", "ma-mo", "Videira/SC", "Monção/MA", "2026-10-12", "19:30", 3, 950),
		viagem("t4", "ma-si", "sc-ch", "Santa Inês/MA", "Chapecó/SC", "2026-11-25", "07:00", 20, 1100),
	}
}

func novoAmbiente(t *testing.T) *ambiente {
	t.Helper()
	cat := novoCatalogoFake()
	b := buscadorDe(viagensPadrao()...)
	r, p, q := &reservasFake{}, &pagamentosFake{}, &cotadorFake{}
	return &ambiente{
		cat: cat, b: b, q: q, r: r, p: p,
		reg: Padrao(cat, b, q, r, p, Config{}),
		ctx: &Contexto{
			Conversa: conversa.Conversa{ID: "conv-1", Telefone: "5549999887766"},
			Estado:   &conversa.Estado{},
			Agora:    agoraFixa,
		},
	}
}

// exec roda a ferramenta e devolve a saida serializada como o modelo a veria.
func (a *ambiente) exec(t *testing.T, nome, args string) (Saida, map[string]any) {
	t.Helper()
	s := a.reg.Executar(context.Background(), a.ctx, nome, json.RawMessage(args))
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return s, m
}

func dadosDe(m map[string]any) map[string]any {
	d, _ := m["dados"].(map[string]any)
	return d
}

// cotadorFake cota o preco de tabela (ajustes por trip) e registra as chamadas.
type cotadorFake struct {
	ajuste map[string]float64 // trip_id -> preco cotado; ausente = preco da busca
	err    error
	ins    []pricing.QuoteInput
}

func (c *cotadorFake) Quote(_ context.Context, in pricing.QuoteInput) (pricing.QuoteResult, error) {
	c.ins = append(c.ins, in)
	if c.err != nil {
		return pricing.QuoteResult{}, c.err
	}
	if v, ok := c.ajuste[in.TripID]; ok {
		return pricing.QuoteResult{FinalAmount: v}, nil
	}
	for _, r := range viagensPadrao() {
		if r.TripID == in.TripID {
			return pricing.QuoteResult{FinalAmount: r.Price}, nil
		}
	}
	return pricing.QuoteResult{}, errors.New("viagem desconhecida")
}

// item devolve o i-esimo item da lista dados[chave] (trechos, pix, reservas).
func item(m map[string]any, chave string, i int) map[string]any {
	l, _ := dadosDe(m)[chave].([]any)
	if i >= len(l) {
		return nil
	}
	r, _ := l[i].(map[string]any)
	return r
}

func (p *pagamentosFake) CancelarPendente(_ context.Context, id string) error {
	if p.status[id] == "PAID" {
		return payments.ErrPagamentoJaPago
	}
	p.cancelados = append(p.cancelados, id)
	p.status[id] = "CANCELLED"
	return nil
}

func (p *pagamentosFake) DefinirSinalReserva(_ context.Context, bookingID string, valor float64) error {
	if p.sinais == nil {
		p.sinais = map[string]float64{}
	}
	p.sinais[bookingID] = valor
	return nil
}
