// Package evals avalia o agente de atendimento v2 no estilo tau2-bench: um LLM
// faz o papel do cliente, conversa com o agente real e, ao fim, checamos o
// estado final e as regras proibidas.
//
// Este arquivo traz os fakes em memoria dos servicos de dominio (busca,
// cotacao, reservas, pagamentos, catalogo e canal) com dados realistas.
package evals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/availability"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/pricing"
)

// VagasPorViagem e a lotacao de cada viagem das fixtures.
const VagasPorViagem = 46

// paradaFix descreve uma cidade atendida.
type paradaFix struct {
	ID, Nome, UF string
	Preco        float64 // preco da passagem MA<->cidade (so SC)
}

// Ordem de cada sentido: MA primeiro na ida, SC primeiro na volta.
var paradasMA = []paradaFix{
	{"ma-santa-ines", "Santa Inês", "MA", 0},
	{"ma-moncao", "Monção", "MA", 0},
	{"ma-igarape", "Igarapé do Meio", "MA", 0},
}

var paradasSC = []paradaFix{
	{"sc-fraiburgo", "Fraiburgo", "SC", 950},
	{"sc-monte-carlo", "Monte Carlo", "SC", 950},
	{"sc-videira", "Videira", "SC", 950},
	{"sc-campos-novos", "Campos Novos", "SC", 1000},
	{"sc-petrolandia", "Petrolândia", "SC", 1100},
	{"sc-ituporanga", "Ituporanga", "SC", 1100},
	{"sc-concordia", "Concórdia", "SC", 1100},
	{"sc-seara", "Seara", "SC", 1100},
	{"sc-ipumirim", "Ipumirim", "SC", 1100},
	{"sc-chapeco", "Chapecó", "SC", 1100},
}

// horario de embarque (hora do dia) de cada parada, por sentido.
var horaIda = map[string]string{
	"ma-santa-ines": "07:30", "ma-moncao": "08:40", "ma-igarape": "09:30",
	"sc-fraiburgo": "05:00", "sc-monte-carlo": "05:40", "sc-videira": "06:20", "sc-campos-novos": "07:30",
	"sc-petrolandia": "09:00", "sc-ituporanga": "10:10", "sc-concordia": "12:30",
	"sc-seara": "13:15", "sc-ipumirim": "14:00", "sc-chapeco": "15:30",
}

var horaVolta = map[string]string{
	"sc-chapeco": "06:00", "sc-ipumirim": "07:10", "sc-seara": "07:50", "sc-concordia": "08:40",
	"sc-ituporanga": "11:00", "sc-petrolandia": "12:15", "sc-campos-novos": "14:00",
	"sc-videira": "15:10", "sc-monte-carlo": "15:50", "sc-fraiburgo": "16:30",
	"ma-igarape": "18:40", "ma-moncao": "19:30", "ma-santa-ines": "20:30",
}

type paradaViagem struct {
	TripStopID string
	StopID     string
	Sequencia  int
	Horario    string
}

// ViagemFix e uma viagem das fixtures.
type ViagemFix struct {
	ID      string
	RouteID string
	Data    string // AAAA-MM-DD
	Sentido string // "MA>SC" ou "SC>MA"
	Vagas   int
	paradas []paradaViagem
}

// Fixtures guarda as cidades e viagens e serve de fonte unica para os fakes.
type Fixtures struct {
	Hoje time.Time // meia-noite (UTC) do dia local

	mu      sync.Mutex
	cidades []ferramentas.Cidade
	porID   map[string]paradaFix
	viagens []*ViagemFix
}

// NovasFixtures monta cidades e ~13 viagens futuras relativas a hoje: MA>SC
// nas segundas e SC>MA nas quintas por 6 semanas, mais uma MA>SC no dia 12 do
// proximo mes (ou deste, se ainda nao passou).
func NovasFixtures(hoje time.Time) *Fixtures {
	h := time.Date(hoje.Year(), hoje.Month(), hoje.Day(), 0, 0, 0, 0, time.UTC)
	f := &Fixtures{Hoje: h, porID: map[string]paradaFix{}}
	for _, p := range append(append([]paradaFix{}, paradasMA...), paradasSC...) {
		f.porID[p.ID] = p
		preco := p.Preco
		if p.UF == "MA" {
			preco = 950
		}
		f.cidades = append(f.cidades, ferramentas.Cidade{StopID: p.ID, Nome: p.Nome, UF: p.UF, PrecoBase: preco})
	}
	n := 0
	prox := func(dia time.Weekday) time.Time {
		d := h.AddDate(0, 0, 2) // nunca no mesmo dia ou no dia seguinte
		for d.Weekday() != dia {
			d = d.AddDate(0, 0, 1)
		}
		return d
	}
	seg, qui := prox(time.Monday), prox(time.Thursday)
	for i := 0; i < 6; i++ {
		n++
		f.viagens = append(f.viagens, f.novaViagem(fmt.Sprintf("trip-%02d", n), "MA>SC", seg.AddDate(0, 0, 7*i)))
		n++
		f.viagens = append(f.viagens, f.novaViagem(fmt.Sprintf("trip-%02d", n), "SC>MA", qui.AddDate(0, 0, 7*i)))
	}
	dia12 := time.Date(h.Year(), h.Month(), 12, 0, 0, 0, 0, time.UTC)
	if dia12.Before(h.AddDate(0, 0, 2)) {
		dia12 = time.Date(h.Year(), h.Month()+1, 12, 0, 0, 0, 0, time.UTC)
	}
	if dia12.Weekday() != time.Monday || dia12.After(seg.AddDate(0, 0, 35)) { // nao duplica a viagem de segunda
		n++
		f.viagens = append(f.viagens, f.novaViagem(fmt.Sprintf("trip-%02d", n), "MA>SC", dia12))
	}
	sort.SliceStable(f.viagens, func(i, j int) bool { return f.viagens[i].Data < f.viagens[j].Data })
	return f
}

func (f *Fixtures) novaViagem(id, sentido string, data time.Time) *ViagemFix {
	var ordem []paradaFix
	horas := horaIda
	if sentido == "MA>SC" {
		ordem = append(append(ordem, paradasMA...), paradasSC...)
	} else {
		horas = horaVolta
		for i := len(paradasSC) - 1; i >= 0; i-- {
			ordem = append(ordem, paradasSC[i])
		}
		for i := len(paradasMA) - 1; i >= 0; i-- {
			ordem = append(ordem, paradasMA[i])
		}
	}
	v := &ViagemFix{ID: id, RouteID: "rota-" + strings.ToLower(strings.ReplaceAll(sentido, ">", "-")), Data: data.Format("2006-01-02"), Sentido: sentido, Vagas: VagasPorViagem}
	for i, p := range ordem {
		v.paradas = append(v.paradas, paradaViagem{TripStopID: "ts-" + id + "-" + p.ID, StopID: p.ID, Sequencia: i + 1, Horario: horas[p.ID]})
	}
	return v
}

// Cidades devolve as cidades atendidas.
func (f *Fixtures) Cidades() []ferramentas.Cidade {
	return append([]ferramentas.Cidade(nil), f.cidades...)
}

// Viagens devolve uma copia das viagens (para inspecao).
func (f *Fixtures) Viagens() []ViagemFix {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ViagemFix, len(f.viagens))
	for i, v := range f.viagens {
		out[i] = *v
	}
	return out
}

// Ocupar tira n vagas de uma viagem (para testar viagem lotada).
func (f *Fixtures) Ocupar(tripID string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.viagens {
		if v.ID == tripID {
			v.Vagas = max(0, v.Vagas-n)
		}
	}
}

// Preco devolve o preco do trecho (so entre MA e SC, nos dois sentidos).
func (f *Fixtures) Preco(origemID, destinoID string) (float64, bool) {
	o, ok1 := f.porID[origemID]
	d, ok2 := f.porID[destinoID]
	if !ok1 || !ok2 || o.UF == d.UF {
		return 0, false
	}
	if o.UF == "SC" {
		return o.Preco, true
	}
	return d.Preco, true
}

// Precos devolve todos os precos distintos de passagem.
func (f *Fixtures) Precos() []float64 {
	seen := map[float64]bool{}
	var out []float64
	for _, p := range paradasSC {
		if !seen[p.Preco] {
			seen[p.Preco] = true
			out = append(out, p.Preco)
		}
	}
	sort.Float64s(out)
	return out
}

// ---- fonte de catalogo ----

type FonteCatalogoFake struct{ fx *Fixtures }

func (f *Fixtures) FonteCatalogo() *FonteCatalogoFake { return &FonteCatalogoFake{fx: f} }

func (s *FonteCatalogoFake) Cidades(context.Context) ([]ferramentas.Cidade, error) {
	return s.fx.Cidades(), nil
}
func (s *FonteCatalogoFake) Aliases(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

var _ ferramentas.FonteCatalogo = (*FonteCatalogoFake)(nil)

// ---- buscador (mesma semantica de availability.Repository.Search) ----

type BuscadorFake struct{ fx *Fixtures }

func (f *Fixtures) Buscador() *BuscadorFake { return &BuscadorFake{fx: f} }

var _ ferramentas.Buscador = (*BuscadorFake)(nil)

func normBusca(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" /", "/", "/ ", "/").Replace(s)
	return semAcentos(s)
}

func (b *BuscadorFake) Search(_ context.Context, flt availability.SearchFilter) ([]availability.SearchResult, error) {
	fx := b.fx
	fx.mu.Lock()
	defer fx.mu.Unlock()
	limit := flt.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	hoje := fx.Hoje.Format("2006-01-02")
	var out []availability.SearchResult
	for _, v := range fx.viagens {
		if !flt.IncludePast && v.Data < hoje {
			continue
		}
		if flt.TripDate != nil {
			if v.Data != flt.TripDate.Format("2006-01-02") {
				continue
			}
		} else {
			if flt.DateFrom != nil && v.Data < flt.DateFrom.Format("2006-01-02") {
				continue
			}
			if flt.DateTo != nil && v.Data > flt.DateTo.Format("2006-01-02") {
				continue
			}
		}
		if flt.RouteID != "" && v.RouteID != flt.RouteID {
			continue
		}
		if flt.Qty > 0 && v.Vagas < flt.Qty {
			continue
		}
		for _, bd := range v.paradas {
			for _, al := range v.paradas {
				if al.Sequencia <= bd.Sequencia { // stop_sequence crescente
					continue
				}
				preco, ok := fx.Preco(bd.StopID, al.StopID)
				if !ok {
					continue
				}
				po, pd := fx.porID[bd.StopID], fx.porID[al.StopID]
				dispOrigem, dispDestino := po.Nome+"/"+po.UF, pd.Nome+"/"+pd.UF
				if flt.OriginStopID != "" {
					if bd.StopID != flt.OriginStopID {
						continue
					}
				} else if flt.Origin != "" && normBusca(dispOrigem) != normBusca(flt.Origin) {
					continue
				}
				if flt.DestinationStopID != "" {
					if al.StopID != flt.DestinationStopID {
						continue
					}
				} else if flt.Destination != "" && normBusca(dispDestino) != normBusca(flt.Destination) {
					continue
				}
				out = append(out, availability.SearchResult{
					SegmentID: v.ID + ":" + bd.TripStopID + ":" + al.TripStopID,
					TripID:    v.ID, RouteID: v.RouteID,
					BoardStopID: bd.TripStopID, AlightStopID: al.TripStopID,
					OriginStopID: bd.StopID, DestinationStopID: al.StopID,
					OriginDisplayName: dispOrigem, DestinationDisplayName: dispDestino,
					OriginDepartTime: bd.Horario, TripDate: v.Data,
					SeatsAvailable: max(v.Vagas, 0), Price: preco, Currency: "BRL",
					Status: "ACTIVE", TripStatus: "SCHEDULED",
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i], out[j]
		if a.TripDate != c.TripDate {
			return a.TripDate < c.TripDate
		}
		if a.OriginDepartTime != c.OriginDepartTime {
			return a.OriginDepartTime < c.OriginDepartTime
		}
		if a.OriginDisplayName != c.OriginDisplayName {
			return a.OriginDisplayName < c.OriginDisplayName
		}
		return a.DestinationDisplayName < c.DestinationDisplayName
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---- cotador ----

type CotadorFake struct{ fx *Fixtures }

func (f *Fixtures) Cotador() *CotadorFake { return &CotadorFake{fx: f} }

var _ ferramentas.Cotador = (*CotadorFake)(nil)

// segmento localiza a viagem e as paradas de embarque/desembarque.
func (f *Fixtures) segmento(tripID, boardTS, alightTS string) (*ViagemFix, paradaViagem, paradaViagem, bool) {
	for _, v := range f.viagens {
		if v.ID != tripID {
			continue
		}
		var b, a paradaViagem
		var okb, oka bool
		for _, p := range v.paradas {
			if p.TripStopID == boardTS {
				b, okb = p, true
			}
			if p.TripStopID == alightTS {
				a, oka = p, true
			}
		}
		return v, b, a, okb && oka
	}
	return nil, paradaViagem{}, paradaViagem{}, false
}

func (c *CotadorFake) Quote(_ context.Context, in pricing.QuoteInput) (pricing.QuoteResult, error) {
	c.fx.mu.Lock()
	defer c.fx.mu.Unlock()
	_, b, a, ok := c.fx.segmento(in.TripID, in.BoardStopID, in.AlightStopID)
	if !ok || b.Sequencia >= a.Sequencia {
		return pricing.QuoteResult{}, errors.New("trecho desconhecido")
	}
	preco, ok := c.fx.Preco(b.StopID, a.StopID)
	if !ok {
		return pricing.QuoteResult{}, errors.New("sem preco para o trecho")
	}
	return pricing.QuoteResult{
		TripID: in.TripID, BoardStopID: in.BoardStopID, AlightStopID: in.AlightStopID,
		BaseAmount: preco, CalcAmount: preco, FinalAmount: preco, Currency: "BRL", FareMode: "AUTO",
	}, nil
}

// ---- reservas ----

type ReservasFake struct {
	fx *Fixtures

	mu       sync.Mutex
	Entradas []bookings.CreateBookingInput // tudo que chegou em Create (validas)
	detalhes []bookings.BookingDetails
	porChave map[string]int
	viagem   map[string]string // booking id -> trip id
}

func (f *Fixtures) Reservas() *ReservasFake {
	return &ReservasFake{fx: f, porChave: map[string]int{}, viagem: map[string]string{}}
}

var _ ferramentas.Reservas = (*ReservasFake)(nil)

func tipoDocValido(t string) bool {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "CPF", "RG", "CNH", "CERTIDAO_NASCIMENTO":
		return true
	}
	return false
}

// Create valida como bookings.Service: viagem e trechos, nome de cada
// passageiro, tipo e presenca de documento (criancas de colo ficam isentas),
// valores e vagas.
func (r *ReservasFake) Create(_ context.Context, in bookings.CreateBookingInput) (bookings.BookingDetails, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pax := in.Passengers
	if len(pax) == 0 && strings.TrimSpace(in.Passenger.Name) != "" {
		pax = []bookings.PassengerInput{in.Passenger}
	}
	if strings.TrimSpace(in.TripID) == "" || len(pax) == 0 {
		return bookings.BookingDetails{}, bookings.ErrMissingFields
	}
	if strings.TrimSpace(in.BoardStopID) == "" || strings.TrimSpace(in.AlightStopID) == "" {
		return bookings.BookingDetails{}, bookings.ErrMissingStops
	}
	for _, p := range pax {
		if strings.TrimSpace(p.Name) == "" {
			return bookings.BookingDetails{}, bookings.ErrPassengerNameRequired
		}
		doc := strings.TrimSpace(p.Document)
		if doc == "" && !p.IsLapChild {
			return bookings.BookingDetails{}, bookings.ErrPassengerDocumentType
		}
		if doc != "" && !tipoDocValido(p.DocumentType) {
			return bookings.BookingDetails{}, bookings.ErrPassengerDocumentType
		}
	}
	if in.TotalAmount < 0 || in.DepositAmount < 0 || in.RemainderAmount < 0 {
		return bookings.BookingDetails{}, bookings.ErrNegativeAmounts
	}
	if math.Abs(in.DepositAmount+in.RemainderAmount-in.TotalAmount) > 0.01 {
		return bookings.BookingDetails{}, bookings.ErrInvalidAmounts
	}
	if i, ok := r.porChave[in.IdempotencyKey]; ok && in.IdempotencyKey != "" {
		return r.detalhes[i], nil
	}

	r.fx.mu.Lock()
	defer r.fx.mu.Unlock()
	v, b, a, ok := r.fx.segmento(in.TripID, in.BoardStopID, in.AlightStopID)
	if !ok || b.Sequencia >= a.Sequencia {
		return bookings.BookingDetails{}, bookings.ErrSeatNotInTrip
	}
	ocupam := 0
	for _, p := range pax {
		if !p.IsLapChild {
			ocupam++
		}
	}
	if v.Vagas < ocupam {
		return bookings.BookingDetails{}, bookings.ErrNoSeatsAvailable
	}
	preco, _ := r.fx.Preco(b.StopID, a.StopID)
	if esperado := preco * float64(ocupam); math.Abs(esperado-in.TotalAmount) > 0.01 {
		return bookings.BookingDetails{}, bookings.ErrInvalidAmounts
	}
	v.Vagas -= ocupam

	n := len(r.detalhes) + 1
	src := ""
	if in.Source != nil {
		src = *in.Source
	}
	exp := time.Now().Add(time.Hour)
	d := bookings.BookingDetails{Booking: bookings.Booking{
		ID: fmt.Sprintf("bk-%d", n), TripID: in.TripID, Status: "PENDING",
		ReservationCode: fmt.Sprintf("SCH%04d", n), Source: src,
		TotalAmount: in.TotalAmount, DepositAmount: in.DepositAmount, RemainderAmount: in.RemainderAmount,
		ExpiresAt: &exp, CreatedAt: time.Now(),
	}}
	for i, p := range pax {
		d.Passengers = append(d.Passengers, bookings.BookingPassenger{
			ID: fmt.Sprintf("bp-%d-%d", n, i+1), BookingID: d.Booking.ID, TripID: in.TripID,
			Name: p.Name, Document: p.Document, DocumentType: strings.ToUpper(p.DocumentType),
			Phone: p.Phone, IsLapChild: p.IsLapChild,
			BoardStopID: in.BoardStopID, AlightStopID: in.AlightStopID,
		})
	}
	if len(d.Passengers) > 0 {
		d.Passenger = d.Passengers[0]
	}
	r.Entradas = append(r.Entradas, in)
	r.detalhes = append(r.detalhes, d)
	if in.IdempotencyKey != "" {
		r.porChave[in.IdempotencyKey] = len(r.detalhes) - 1
	}
	r.viagem[d.Booking.ID] = in.TripID
	return d, nil
}

func (r *ReservasFake) Get(_ context.Context, id string) (bookings.BookingDetails, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.detalhes {
		if d.Booking.ID == id {
			return d, nil
		}
	}
	return bookings.BookingDetails{}, errors.New("reserva nao encontrada")
}

func (r *ReservasFake) List(_ context.Context, f bookings.ListFilter) ([]bookings.BookingListItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []bookings.BookingListItem
	for _, d := range r.detalhes {
		b := d.Booking
		if f.BookingID != "" && b.ID != f.BookingID {
			continue
		}
		if f.ReservationCode != "" && b.ReservationCode != f.ReservationCode {
			continue
		}
		if f.TripID != "" && b.TripID != f.TripID {
			continue
		}
		if f.Status != "" && !strings.EqualFold(b.Status, f.Status) {
			continue
		}
		it := bookings.BookingListItem{
			ID: b.ID, TripID: b.TripID, Status: b.Status, ReservationCode: b.ReservationCode,
			TotalAmount: b.TotalAmount, DepositAmount: b.DepositAmount, RemainderAmount: b.RemainderAmount,
			ExpiresAt: b.ExpiresAt, CreatedAt: b.CreatedAt,
		}
		if len(d.Passengers) > 0 {
			it.PassengerName, it.PassengerPhone = d.Passengers[0].Name, d.Passengers[0].Phone
		}
		out = append(out, it)
	}
	return out, nil
}

// Reservas devolve as reservas criadas.
func (r *ReservasFake) Reservas() []bookings.BookingDetails {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bookings.BookingDetails(nil), r.detalhes...)
}

// ---- pagamentos ----

type PagamentosFake struct {
	mu       sync.Mutex
	Entradas []payments.CreatePaymentInput
	pags     []payments.Payment
	pix      map[string]string
}

func NovosPagamentos() *PagamentosFake { return &PagamentosFake{pix: map[string]string{}} }

var _ ferramentas.Pagamentos = (*PagamentosFake)(nil)

func (p *PagamentosFake) Create(_ context.Context, in payments.CreatePaymentInput) (payments.Payment, json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(in.BookingID) == "" || in.Amount <= 0 {
		return payments.Payment{}, nil, errors.New("pagamento invalido")
	}
	if !strings.EqualFold(in.Method, "PIX") {
		return payments.Payment{}, nil, errors.New("metodo nao suportado")
	}
	if in.Customer == nil || strings.TrimSpace(in.Customer.Document) == "" {
		return payments.Payment{}, nil, errors.New("cliente com documento obrigatorio para PIX")
	}
	id := fmt.Sprintf("pay-%d", len(p.pags)+1)
	codigo := fmt.Sprintf("00020126580014br.gov.bcb.pix0136eval-%s5204000053039865802BR5913SCHUMACHER TUR6009SAO PAULO62070503***6304ABCD", id)
	pay := payments.Payment{ID: id, BookingID: in.BookingID, Amount: in.Amount, Method: "PIX", Status: "PENDING", CreatedAt: time.Now()}
	p.Entradas = append(p.Entradas, in)
	p.pags = append(p.pags, pay)
	p.pix[id] = codigo
	exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	raw := json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"` + codigo + `","expires_at":"` + exp + `"}}]}`)
	return pay, raw, nil
}

func (p *PagamentosFake) GetStatus(_ context.Context, id string) (payments.PaymentStatusResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pay := range p.pags {
		if pay.ID == id {
			exp := pay.CreatedAt.Add(time.Hour).UTC().Format(time.RFC3339)
			meta := json.RawMessage(`{"order":{"charges":[{"last_transaction":{"qr_code":"` + p.pix[id] + `","expires_at":"` + exp + `"}}]}}`)
			return payments.PaymentStatusResponse{ID: id, Status: pay.Status, Amount: pay.Amount, Metadata: meta}, nil
		}
	}
	return payments.PaymentStatusResponse{}, errors.New("pagamento nao encontrado")
}

func (p *PagamentosFake) List(_ context.Context, f payments.PaymentListFilter) ([]payments.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []payments.Payment
	for _, pay := range p.pags {
		if f.BookingID == "" || pay.BookingID == f.BookingID {
			out = append(out, pay)
		}
	}
	return out, nil
}

// Pagamentos devolve os pagamentos criados.
func (p *PagamentosFake) Pagamentos() []payments.Payment {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]payments.Payment(nil), p.pags...)
}

// CodigoPix devolve o copia-e-cola de um pagamento.
func (p *PagamentosFake) CodigoPix(id string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pix[id]
}

// ---- canal ----

// Enviada e uma mensagem registrada pelo canal fake.
type Enviada struct {
	Contato, Texto string
	ProvedorID     string
}

// CanalFake registra os envios. Implementa canal.Canal.
type CanalFake struct {
	mu       sync.Mutex
	Enviadas []Enviada
	// Falha, se nao nil, faz Enviar devolver esse erro.
	Falha error
}

func (c *CanalFake) Nome() string { return "WHATSAPP" }

func (c *CanalFake) Normalizar(context.Context, []byte) (canal.Entrada, bool, error) {
	return canal.Entrada{}, false, nil
}

func (c *CanalFake) Enviar(_ context.Context, contato, texto string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Falha != nil {
		return "", c.Falha
	}
	id := fmt.Sprintf("fake-%d", len(c.Enviadas)+1)
	c.Enviadas = append(c.Enviadas, Enviada{Contato: contato, Texto: texto, ProvedorID: id})
	return id, nil
}

func (c *CanalFake) BaixarMidia(context.Context, conversa.Mensagem) (string, string, error) {
	return "", "", errors.New("canal fake: sem midia")
}

// Textos devolve os textos enviados a partir do indice de.
func (c *CanalFake) Textos(de int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for i := de; i < len(c.Enviadas); i++ {
		out = append(out, c.Enviadas[i].Texto)
	}
	return out
}

// Total devolve quantas mensagens foram enviadas.
func (c *CanalFake) Total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Enviadas)
}

func semAcentos(s string) string {
	return strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n",
	).Replace(strings.ToLower(s))
}

// CancelarPendente marca o PIX como cancelado (pago nao cancela).
func (p *PagamentosFake) CancelarPendente(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.pags {
		if p.pags[i].ID == id {
			if p.pags[i].Status == "PAID" {
				return payments.ErrPagamentoJaPago
			}
			p.pags[i].Status = "CANCELLED"
			return nil
		}
	}
	return errors.New("pagamento nao encontrado")
}

// DefinirSinalReserva nao tem efeito nos fakes (o valor do PIX vem do estado).
func (p *PagamentosFake) DefinirSinalReserva(context.Context, string, float64) error { return nil }
