package fila

import (
	"context"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

type relogio struct {
	mu sync.Mutex
	t  time.Time
}

func (r *relogio) Agora() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.t }
func (r *relogio) Avancar(d time.Duration) {
	r.mu.Lock()
	r.t = r.t.Add(d)
	r.mu.Unlock()
}

type procFake struct {
	store  conversa.Store
	mu     sync.Mutex
	conts  map[string]int
	panic  bool
	erro   error
	atraso time.Duration
}

func (p *procFake) Processar(ctx context.Context, c conversa.Conversa) error {
	p.mu.Lock()
	p.conts[c.ID]++
	p.mu.Unlock()
	if p.atraso > 0 {
		time.Sleep(p.atraso)
	}
	if p.panic {
		panic("boom")
	}
	if p.erro != nil {
		return p.erro
	}
	return p.store.ConcluirPendencia(ctx, c.ID, *c.UltimaEntradaEm)
}

func (p *procFake) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, v := range p.conts {
		n += v
	}
	return n
}

func novo(t *testing.T) (*conversa.StoreMem, *relogio, *procFake, *Worker) {
	t.Helper()
	rel := &relogio{t: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	s := conversa.NewStoreMem(rel.Agora)
	p := &procFake{store: s, conts: map[string]int{}}
	w := NovoWorker(s, p, Config{Concorrencia: 4, Debounce: 2 * time.Second, Intervalo: 5 * time.Millisecond, IntervaloReativacao: 10 * time.Millisecond}, log.New(io.Discard, "", 0))
	return s, rel, p, w
}

func entrada(t *testing.T, s conversa.Store, contato, prov, texto string) conversa.Conversa {
	t.Helper()
	c, _, _, err := s.RegistrarEntrada(context.Background(), conversa.NovaEntrada{
		Canal: "WHATSAPP", Contato: contato, Telefone: contato, Autor: conversa.AutorCliente,
		Tipo: conversa.TipoTexto, Texto: texto, ProvedorID: prov,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDebounce(t *testing.T) {
	s, rel, p, w := novo(t)
	entrada(t, s, "5511", "p1", "oi")
	n, err := w.ExecutarUmaVez(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("esperava 0, got %d err=%v", n, err)
	}
	rel.Avancar(3 * time.Second)
	n, err = w.ExecutarUmaVez(context.Background())
	if err != nil || n != 1 || p.total() != 1 {
		t.Fatalf("esperava 1, got %d err=%v", n, err)
	}
	if n, _ = w.ExecutarUmaVez(context.Background()); n != 0 {
		t.Fatalf("pendencia concluida, got %d", n)
	}
}

func TestDuasMensagensUmProcessar(t *testing.T) {
	s, rel, p, w := novo(t)
	entrada(t, s, "5511", "p1", "oi")
	rel.Avancar(time.Second)
	entrada(t, s, "5511", "p2", "tudo bem?")
	rel.Avancar(3 * time.Second)
	n, _ := w.ExecutarUmaVez(context.Background())
	if n != 1 || p.total() != 1 {
		t.Fatalf("n=%d total=%d", n, p.total())
	}
}

func TestConcorrenciaExatamenteUma(t *testing.T) {
	s, rel, p, w := novo(t)
	p.atraso = 5 * time.Millisecond
	for i := 0; i < 10; i++ {
		entrada(t, s, "55"+string(rune('a'+i)), "p"+string(rune('a'+i)), "oi")
	}
	rel.Avancar(3 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	w.Iniciar(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for p.total() < 10 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	w.Parar()
	if len(p.conts) != 10 {
		t.Fatalf("conversas processadas: %d", len(p.conts))
	}
	for id, v := range p.conts {
		if v != 1 {
			t.Fatalf("conversa %s processada %d vezes", id, v)
		}
	}
}

func TestPanicNaoDerrubaELiberaLease(t *testing.T) {
	s, rel, p, w := novo(t)
	entrada(t, s, "5511", "p1", "oi")
	rel.Avancar(3 * time.Second)
	p.panic = true
	n, err := w.ExecutarUmaVez(context.Background())
	if err == nil || n != 0 || p.total() != 1 {
		t.Fatalf("n=%d err=%v total=%d", n, err, p.total())
	}
	// lease liberado: da para reivindicar de novo
	p.panic = false
	n, err = w.ExecutarUmaVez(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

type storeContador struct {
	conversa.Store
	n atomic.Int32
}

func (s *storeContador) ReativarPausadas(ctx context.Context, agora time.Time) (int, error) {
	s.n.Add(1)
	return s.Store.ReativarPausadas(ctx, agora)
}

func TestReativarPausadasChamado(t *testing.T) {
	_, _, p, _ := novo(t)
	sc := &storeContador{Store: p.store}
	w := NovoWorker(sc, p, Config{Concorrencia: 1, Intervalo: 5 * time.Millisecond, IntervaloReativacao: 10 * time.Millisecond}, log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	w.Iniciar(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for sc.n.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	w.Parar()
	if sc.n.Load() < 2 {
		t.Fatalf("ReativarPausadas chamado %d vezes", sc.n.Load())
	}
}
