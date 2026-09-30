// Package fila executa o processamento das conversas pendentes do atendimento v2.
package fila

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

// Processador trata uma conversa devida (implementado pelo pacote agente).
type Processador interface {
	Processar(ctx context.Context, c conversa.Conversa) error
}

type Config struct {
	Concorrencia        int
	Debounce            time.Duration
	Intervalo           time.Duration
	IntervaloReativacao time.Duration
}

func (c Config) comPadroes() Config {
	if c.Concorrencia <= 0 {
		c.Concorrencia = 4
	}
	if c.Debounce <= 0 {
		c.Debounce = 2 * time.Second
	}
	if c.Intervalo <= 0 {
		c.Intervalo = time.Second
	}
	if c.IntervaloReativacao <= 0 {
		c.IntervaloReativacao = time.Minute
	}
	return c
}

type Worker struct {
	store conversa.Store
	proc  Processador
	cfg   Config
	log   *log.Logger
	wg    sync.WaitGroup
}

func NovoWorker(s conversa.Store, p Processador, cfg Config, lg *log.Logger) *Worker {
	if lg == nil {
		lg = log.Default()
	}
	return &Worker{store: s, proc: p, cfg: cfg.comPadroes(), log: lg}
}

// Iniciar dispara as goroutines e retorna; elas param quando ctx for cancelado.
func (w *Worker) Iniciar(ctx context.Context) {
	for i := 0; i < w.cfg.Concorrencia; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.loop(ctx)
		}()
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.loopReativacao(ctx)
	}()
}

// Parar espera as goroutines terminarem (o ctx de Iniciar deve estar cancelado).
func (w *Worker) Parar() { w.wg.Wait() }

func (w *Worker) loop(ctx context.Context) {
	for ctx.Err() == nil {
		achou, err := w.passo(ctx)
		if err != nil || !achou {
			if !dormir(ctx, w.cfg.Intervalo) {
				return
			}
		}
	}
}

func (w *Worker) loopReativacao(ctx context.Context) {
	t := time.NewTicker(w.cfg.IntervaloReativacao)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := w.store.ReativarPausadas(ctx, time.Now()); err != nil {
				if ctx.Err() == nil {
					w.log.Printf("atendimento fila: reativar pausadas: %v", err)
				}
			} else if n > 0 {
				w.log.Printf("atendimento fila: %d conversa(s) reativada(s)", n)
			}
		}
	}
}

func dormir(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// passo reivindica e processa no maximo uma conversa. achou=false se nada devido.
func (w *Worker) passo(ctx context.Context) (achou bool, err error) {
	_, achou, err = w.passoID(ctx)
	return achou, err
}

func (w *Worker) passoID(ctx context.Context) (id string, achou bool, err error) {
	lock, ok, err := w.store.ReivindicarDevida(ctx, w.cfg.Debounce)
	if err != nil {
		if ctx.Err() == nil {
			w.log.Printf("atendimento fila: reivindicar: %v", err)
		}
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	c := lock.Conversa()
	defer func() {
		lctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if lerr := lock.Liberar(lctx); lerr != nil {
			w.log.Printf("atendimento fila: liberar lease conversa_id=%s: %v", c.ID, lerr)
		}
	}()
	if perr := w.processar(ctx, c); perr != nil {
		w.log.Printf("atendimento fila: processar conversa_id=%s: %v", c.ID, perr)
		return c.ID, true, perr
	}
	return c.ID, true, nil
}

func (w *Worker) processar(ctx context.Context, c conversa.Conversa) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return w.proc.Processar(ctx, c)
}

// ExecutarUmaVez processa, em sequencia, as conversas devidas agora. Para no
// primeiro erro (a conversa com falha seria reivindicada de novo em seguida).
func (w *Worker) ExecutarUmaVez(ctx context.Context) (processadas int, err error) {
	for ctx.Err() == nil {
		_, achou, perr := w.passoID(ctx)
		if perr != nil {
			return processadas, perr
		}
		if !achou {
			return processadas, nil
		}
		processadas++
	}
	return processadas, ctx.Err()
}
