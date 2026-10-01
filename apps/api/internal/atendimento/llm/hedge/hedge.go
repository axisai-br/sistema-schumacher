// Package hedge implementa llm.Modelo com um modelo principal e um reserva:
// se o principal demora (ou falha), o mesmo pedido vai ao reserva e vale a
// primeira resposta bem-sucedida. Reduz a latencia de cauda de endpoints lentos.
package hedge

import (
	"context"
	"errors"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
)

// HedgeAposPadrao e o tempo de espera pelo principal antes de acionar o reserva.
const HedgeAposPadrao = 3 * time.Second

type Config struct {
	Principal llm.Modelo
	Reserva   llm.Modelo
	// NomeReserva e o nome do modelo enviado no Pedido ao reserva. Vazio deixa o
	// cliente reserva usar o modelo dele (o Pedido.Modelo do principal nunca
	// vai para o reserva).
	NomeReserva string
	// HedgeApos e quanto esperar o principal antes de disparar o reserva
	// (padrao 3s).
	HedgeApos time.Duration
}

type modelo struct {
	cfg Config
}

var _ llm.Modelo = (*modelo)(nil)

// Novo cria o llm.Modelo com hedge. Sem reserva devolve o principal.
func Novo(cfg Config) llm.Modelo {
	if cfg.Reserva == nil {
		return cfg.Principal
	}
	if cfg.HedgeApos <= 0 {
		cfg.HedgeApos = HedgeAposPadrao
	}
	return &modelo{cfg: cfg}
}

type resultado struct {
	resp    llm.Resposta
	err     error
	reserva bool
}

func (m *modelo) Gerar(ctx context.Context, p llm.Pedido) (llm.Resposta, error) {
	ctx, cancelar := context.WithCancel(ctx)
	defer cancelar() // cancela a chamada que perdeu

	pr := p
	pr.Modelo = m.cfg.NomeReserva

	ch := make(chan resultado, 2) // com buffer: a goroutine perdedora nunca bloqueia
	go func() {
		r, err := m.cfg.Principal.Gerar(ctx, p)
		ch <- resultado{r, err, false}
	}()
	emVoo := 1
	disparado := false
	disparar := func() {
		disparado = true
		emVoo++
		go func() {
			r, err := m.cfg.Reserva.Gerar(ctx, pr)
			ch <- resultado{r, err, true}
		}()
	}

	timer := time.NewTimer(m.cfg.HedgeApos)
	defer timer.Stop()
	var errPrincipal, errReserva error
	for emVoo > 0 {
		select {
		case <-ctx.Done():
			return llm.Resposta{}, ctx.Err()
		case <-timer.C:
			if !disparado {
				disparar()
			}
		case r := <-ch:
			emVoo--
			if r.err == nil {
				return r.resp, nil
			}
			if r.reserva {
				errReserva = r.err
			} else {
				errPrincipal = r.err
			}
			// Principal falhou antes do hedge: usa o reserva imediatamente.
			if !disparado {
				disparar()
			}
		}
	}
	if errPrincipal != nil && errReserva != nil {
		return llm.Resposta{}, errors.Join(errPrincipal, errReserva)
	}
	if errPrincipal != nil {
		return llm.Resposta{}, errPrincipal
	}
	return llm.Resposta{}, errReserva
}
