package ferramentas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"schumacher-tur/api/internal/availability"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/pricing"
)

// Buscador e o recorte de availability.Service usado pelas ferramentas.
type Buscador interface {
	Search(ctx context.Context, f availability.SearchFilter) ([]availability.SearchResult, error)
}

// Cotador e o recorte de pricing.Service usado para cotar o preco real
// (o mesmo que bookings.Service.Create usa para validar os valores).
type Cotador interface {
	Quote(ctx context.Context, in pricing.QuoteInput) (pricing.QuoteResult, error)
}

// Reservas e o recorte de bookings.Service usado pelas ferramentas.
type Reservas interface {
	Create(ctx context.Context, in bookings.CreateBookingInput) (bookings.BookingDetails, error)
	Get(ctx context.Context, id string) (bookings.BookingDetails, error)
	List(ctx context.Context, f bookings.ListFilter) ([]bookings.BookingListItem, error)
}

// Pagamentos e o recorte de payments.Service usado pelas ferramentas.
type Pagamentos interface {
	Create(ctx context.Context, in payments.CreatePaymentInput) (payments.Payment, json.RawMessage, error)
	GetStatus(ctx context.Context, id string) (payments.PaymentStatusResponse, error)
	List(ctx context.Context, f payments.PaymentListFilter) ([]payments.Payment, error)
}

// Config parametriza as ferramentas.
type Config struct {
	// SinalPorPagante e o valor do sinal por passageiro pagante (padrao 250).
	SinalPorPagante float64
	// Fuso e o fuso para "hoje" (padrao America/Sao_Paulo).
	Fuso *time.Location
}

const sinalPadrao = 250.0

func (c Config) comPadroes() Config {
	if c.SinalPorPagante <= 0 {
		c.SinalPorPagante = sinalPadrao
	}
	if c.Fuso == nil {
		if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
			c.Fuso = loc
		} else {
			c.Fuso = time.FixedZone("BRT", -3*3600)
		}
	}
	return c
}

// Padrao registra as 9 ferramentas do atendimento v2 na ordem canonica.
func Padrao(cat *Catalogo, b Buscador, q Cotador, r Reservas, p Pagamentos, cfg Config) *Registro {
	cfg = cfg.comPadroes()
	return NovoRegistro(
		&listarRotas{cat: cat},
		&buscarViagens{cat: cat, b: b, fuso: cfg.Fuso},
		&escolherViagem{cat: cat, b: b, q: q, fuso: cfg.Fuso},
		&removerTrecho{},
		&registrarPassageiros{},
		&criarReserva{r: r, q: q, cfg: cfg},
		&gerarPix{r: r, p: p, cfg: cfg},
		&consultarReserva{r: r, p: p},
		&transferirParaHumano{},
	)
}

// ---- helpers de saida e argumentos ----

func falha(motivo, mensagem string) Saida {
	s := Saida{OK: false, Motivo: motivo}
	if mensagem != "" {
		s.Dados = map[string]any{"mensagem": mensagem}
	}
	return s
}

func falhaDados(motivo string, dados map[string]any) Saida {
	return Saida{OK: false, Motivo: motivo, Dados: dados}
}

func sucesso(dados any) Saida { return Saida{OK: true, Dados: dados} }

func lerArgs(raw json.RawMessage, dst any) *Saida {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return nil
	}
	if err := json.Unmarshal(t, dst); err != nil {
		s := falha("argumentos_invalidos", "Os argumentos da ferramenta estao em formato invalido; revise os tipos e tente de novo.")
		return &s
	}
	return nil
}

// inteiro aceita numero JSON ou string numerica ("3").
type inteiro int

func (i *inteiro) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*i = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*i = inteiro(int(f))
	return nil
}

func defJSON(schema string) json.RawMessage { return json.RawMessage(schema) }

func formatarReais(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	cents := int64(v*100 + 0.5)
	inteira, frac := cents/100, cents%100
	s := strconv.FormatInt(inteira, 10)
	var partes []string
	for len(s) > 3 {
		partes = append([]string{s[len(s)-3:]}, partes...)
		s = s[:len(s)-3]
	}
	partes = append([]string{s}, partes...)
	out := "R$ " + strings.Join(partes, ".")
	if frac != 0 {
		out += fmt.Sprintf(",%02d", frac)
	}
	if neg {
		out = "-" + out
	}
	return out
}

func arredondar(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

var diasSemana = [...]string{"domingo", "segunda-feira", "terca-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sabado"}

func diaSemana(data string) string {
	t, err := time.Parse("2006-01-02", data)
	if err != nil {
		return ""
	}
	return diasSemana[t.Weekday()]
}

func apenasDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func primeiroNome(nome string) string {
	f := strings.Fields(nome)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}
