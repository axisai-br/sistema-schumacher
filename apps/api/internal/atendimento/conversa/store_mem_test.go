package conversa

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
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

func novo(t *testing.T) (*StoreMem, *relogio) {
	t.Helper()
	r := &relogio{t: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	return NewStoreMem(r.Agora), r
}

func entradaCliente(r *relogio, prov, texto string) NovaEntrada {
	return NovaEntrada{Canal: "WHATSAPP", Contato: "5549@s.whatsapp.net", Telefone: "5549", Nome: "Ana",
		Autor: AutorCliente, Tipo: TipoTexto, Texto: texto, ProvedorID: prov, RecebidaEm: r.Agora()}
}

func TestRegistrarEntradaIdempotente(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c1, m1, dup, err := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	if err != nil || dup {
		t.Fatalf("err=%v dup=%v", err, dup)
	}
	r.Avancar(time.Minute)
	c2, m2, dup, err := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi de novo"))
	if err != nil || !dup {
		t.Fatalf("esperava duplicada, err=%v dup=%v", err, dup)
	}
	if m2.ID != m1.ID || c2.ID != c1.ID {
		t.Fatal("deveria devolver a existente")
	}
	if !c2.UltimaEntradaEm.Equal(*c1.UltimaEntradaEm) {
		t.Fatal("duplicada nao deve alterar ultima_entrada_em")
	}
	h, _ := s.Historico(ctx, c1.ID, 10)
	if len(h) != 1 {
		t.Fatalf("historico = %d, want 1", len(h))
	}
}

func TestPendenteDesdeEUltimaEntrada(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	t0 := r.Agora()
	s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	r.Avancar(5 * time.Second)
	t1 := r.Agora()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P2", "tem viagem?"))
	if c.PendenteDesde == nil || !c.PendenteDesde.Equal(t0) {
		t.Fatalf("pendente_desde = %v, want %v", c.PendenteDesde, t0)
	}
	if c.UltimaEntradaEm == nil || !c.UltimaEntradaEm.Equal(t1) {
		t.Fatalf("ultima_entrada_em = %v, want %v", c.UltimaEntradaEm, t1)
	}
	pend, _ := s.EntradasPendentes(ctx, c.ID, t0)
	if len(pend) != 2 {
		t.Fatalf("pendentes = %d", len(pend))
	}
	pend, _ = s.EntradasPendentes(ctx, c.ID, t1)
	if len(pend) != 1 || pend[0].Texto != "tem viagem?" {
		t.Fatalf("pendentes desde t1 = %+v", pend)
	}
}

func TestHumanoPausaEReativa(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	r.Avancar(time.Second)
	in := entradaCliente(r, "P2", "já te atendo")
	in.Autor = AutorHumano
	c, m, _, err := s.RegistrarEntrada(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusHumano || c.PendenteDesde != nil || m.Direcao != DirecaoSaida {
		t.Fatalf("conversa=%+v msg=%+v", c, m)
	}
	if c.HumanoAte == nil || !c.HumanoAte.Equal(in.RecebidaEm.Add(12*time.Hour)) {
		t.Fatalf("humano_ate = %v", c.HumanoAte)
	}
	if ok, _ := s.ProvedorIDConhecido(ctx, "P2"); !ok {
		t.Fatal("P2 deveria ser conhecido (saida)")
	}
	if ok, _ := s.ProvedorIDConhecido(ctx, "P1"); ok {
		t.Fatal("P1 e entrada, nao saida")
	}

	n, _ := s.ReativarPausadas(ctx, r.Agora().Add(11*time.Hour))
	if n != 0 {
		t.Fatalf("nao deveria reativar antes de 12h, n=%d", n)
	}
	n, _ = s.ReativarPausadas(ctx, r.Agora().Add(13*time.Hour))
	if n != 1 {
		t.Fatalf("n = %d, want 1", n)
	}
	c, _ = s.Obter(ctx, c.ID)
	if c.Status != StatusBot || c.HumanoAte != nil {
		t.Fatalf("conversa = %+v", c)
	}
}

func TestReativarIgnoraComResponsavelOuTransferenciaDoBot(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	c, err := s.MudarStatus(ctx, c.ID, StatusHumano, "user-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ReativarPausadas(ctx, r.Agora().Add(100*time.Hour)); n != 0 {
		t.Fatal("conversa com responsavel nao deve reativar")
	}
	c, _ = s.MudarStatus(ctx, c.ID, StatusHumano, "", "cliente pediu ajuda")
	if c.Estado.MotivoHumano != "cliente pediu ajuda" {
		t.Fatalf("motivo = %q", c.Estado.MotivoHumano)
	}
	if n, _ := s.ReativarPausadas(ctx, r.Agora().Add(100*time.Hour)); n != 0 {
		t.Fatal("transferencia do bot (sem humano_ate) nao deve reativar sozinha")
	}
	c, _ = s.MudarStatus(ctx, c.ID, StatusBot, "", "")
	if c.Status != StatusBot || c.ResponsavelID != "" || c.Estado.MotivoHumano != "" {
		t.Fatalf("conversa = %+v", c)
	}
}

func TestReivindicarRespeitaDebounceELease(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	debounce := 2 * time.Second

	if _, ok, _ := s.ReivindicarDevida(ctx, debounce); ok {
		t.Fatal("nao deveria estar devida antes do debounce")
	}
	r.Avancar(2 * time.Second)
	lock, ok, err := s.ReivindicarDevida(ctx, debounce)
	if err != nil || !ok || lock.Conversa().ID != c.ID {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, ok, _ := s.ReivindicarDevida(ctx, debounce); ok {
		t.Fatal("lease vigente: nao pode reivindicar de novo")
	}
	// lease expira
	r.Avancar(LeaseDuracao + time.Second)
	lock2, ok, _ := s.ReivindicarDevida(ctx, debounce)
	if !ok {
		t.Fatal("lease expirado deveria permitir reivindicar")
	}
	// Liberar e idempotente e libera de fato
	if err := lock2.Liberar(ctx); err != nil {
		t.Fatal(err)
	}
	_ = lock2.Liberar(ctx)
	l3, ok, _ := s.ReivindicarDevida(ctx, debounce)
	if !ok {
		t.Fatal("liberado deveria poder ser reivindicado")
	}
	_ = l3.Liberar(ctx)
	_ = lock.Liberar(ctx)

	// Sem pendencia, nao e devida.
	if err := s.ConcluirPendencia(ctx, c.ID, r.Agora()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ReivindicarDevida(ctx, debounce); ok {
		t.Fatal("sem pendente_desde nao e devida")
	}
}

func TestReivindicarHumanoNaoEDevida(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	_, _ = s.MudarStatus(ctx, c.ID, StatusHumano, "", "x")
	r.Avancar(time.Minute)
	if _, ok, _ := s.ReivindicarDevida(ctx, time.Second); ok {
		t.Fatal("HUMANO nao deve ser reivindicada")
	}
}

func TestReivindicarConcorrenteNaoPegaMesma(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	r.Avancar(10 * time.Second)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ganhou := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok, _ := s.ReivindicarDevida(ctx, time.Second); ok {
				mu.Lock()
				ganhou++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ganhou != 1 {
		t.Fatalf("ganharam %d, want 1", ganhou)
	}
}

func TestReivindicaMaisAntigaPrimeiro(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	a := entradaCliente(r, "A1", "a")
	a.Contato = "a@s"
	ca, _, _, _ := s.RegistrarEntrada(ctx, a)
	r.Avancar(time.Second)
	b := entradaCliente(r, "B1", "b")
	b.Contato = "b@s"
	s.RegistrarEntrada(ctx, b)
	r.Avancar(10 * time.Second)
	l, ok, _ := s.ReivindicarDevida(ctx, time.Second)
	if !ok || l.Conversa().ID != ca.ID {
		t.Fatal("deveria pegar a mais antiga")
	}
}

func TestSalvarEstadoConflito(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	v, err := s.SalvarEstado(ctx, c.ID, Estado{Falhas: 1}, c.Versao)
	if err != nil || v != c.Versao+1 {
		t.Fatalf("v=%d err=%v", v, err)
	}
	if _, err := s.SalvarEstado(ctx, c.ID, Estado{Falhas: 2}, c.Versao); !errors.Is(err, ErrVersaoConflito) {
		t.Fatalf("err = %v, want conflito", err)
	}
	got, _ := s.Obter(ctx, c.ID)
	if got.Estado.Falhas != 1 || got.Versao != v {
		t.Fatalf("estado = %+v", got)
	}
	if _, err := s.SalvarEstado(ctx, "nao-existe", Estado{}, 0); !errors.Is(err, ErrNaoEncontrada) {
		t.Fatalf("err = %v", err)
	}
}

func TestConcluirPendenciaNaoLimpaSeChegouEntradaNova(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	inicio := *c.UltimaEntradaEm
	r.Avancar(time.Second)
	s.RegistrarEntrada(ctx, entradaCliente(r, "P2", "mais uma coisa"))

	if chegou, _ := s.ChegouEntradaDepois(ctx, c.ID, inicio); !chegou {
		t.Fatal("deveria detectar entrada nova")
	}
	if err := s.ConcluirPendencia(ctx, c.ID, inicio); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Obter(ctx, c.ID)
	if got.PendenteDesde == nil {
		t.Fatal("pendente_desde nao deveria ser limpo")
	}
	if err := s.ConcluirPendencia(ctx, c.ID, *got.UltimaEntradaEm); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Obter(ctx, c.ID)
	if got.PendenteDesde != nil {
		t.Fatal("pendente_desde deveria ser limpo")
	}
	if chegou, _ := s.ChegouEntradaDepois(ctx, c.ID, *got.UltimaEntradaEm); chegou {
		t.Fatal("nao ha entrada posterior")
	}
}

func TestSaidaHistoricoTurnosEListar(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	r.Avancar(time.Second)
	turno, err := s.RegistrarTurno(ctx, Turno{ID: "turno-1", ConversaID: c.ID, Resultado: ResultadoEnviado})
	if err != nil || turno.ID != "turno-1" {
		t.Fatalf("turno=%+v err=%v", turno, err)
	}
	m, err := s.RegistrarSaida(ctx, c.ID, AutorBot, "Olá!", "OUT1", turno.ID)
	if err != nil || m.Direcao != DirecaoSaida || m.TurnoID != "turno-1" {
		t.Fatalf("m=%+v err=%v", m, err)
	}
	if ok, _ := s.ProvedorIDConhecido(ctx, "OUT1"); !ok {
		t.Fatal("OUT1 deveria ser conhecido")
	}
	h, _ := s.Historico(ctx, c.ID, 1)
	if len(h) != 1 || h[0].Texto != "Olá!" {
		t.Fatalf("historico = %+v", h)
	}
	h, _ = s.Historico(ctx, c.ID, 10)
	if len(h) != 2 || h[0].Autor != AutorCliente {
		t.Fatalf("historico = %+v", h)
	}
	ts, _ := s.Turnos(ctx, c.ID, 5)
	if len(ts) != 1 {
		t.Fatalf("turnos = %d", len(ts))
	}
	if l, _ := s.Listar(ctx, FiltroLista{Busca: "ana"}); len(l) != 1 {
		t.Fatalf("listar busca = %d", len(l))
	}
	if l, _ := s.Listar(ctx, FiltroLista{Status: StatusHumano}); len(l) != 0 {
		t.Fatalf("listar humano = %d", len(l))
	}
	if _, err := s.Obter(ctx, "x"); !errors.Is(err, ErrNaoEncontrada) {
		t.Fatalf("err = %v", err)
	}
}

func TestRelogioDoServidorNaoDoProvedor(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	in := entradaCliente(r, "P1", "oi")
	in.RecebidaEm = r.Agora().Add(-10 * time.Minute) // relogio do provedor atrasado
	c, m, _, _ := s.RegistrarEntrada(ctx, in)
	if !c.PendenteDesde.Equal(r.Agora()) || !c.UltimaEntradaEm.Equal(r.Agora()) || !m.CriadoEm.Equal(r.Agora()) {
		t.Fatalf("deveria usar agora(): %+v %+v", c, m)
	}
	in2 := entradaCliente(r, "P2", "eu")
	in2.Autor = AutorHumano
	in2.RecebidaEm = r.Agora().Add(-time.Hour)
	c, _, _, _ = s.RegistrarEntrada(ctx, in2)
	if !c.HumanoAte.Equal(r.Agora().Add(12 * time.Hour)) {
		t.Fatalf("humano_ate = %v", c.HumanoAte)
	}
}

func TestSaidaBotRecente(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	if _, err := s.RegistrarSaida(ctx, c.ID, AutorBot, "Olá, tudo bem?", "OUT1", ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SaidaBotRecente(ctx, c.Contato, "  Olá, tudo bem?\n", time.Minute); !ok {
		t.Fatal("deveria achar (trim)")
	}
	if ok, _ := s.SaidaBotRecente(ctx, c.Contato, "outro texto", time.Minute); ok {
		t.Fatal("texto diferente")
	}
	if ok, _ := s.SaidaBotRecente(ctx, "outro@s", "Olá, tudo bem?", time.Minute); ok {
		t.Fatal("outro contato")
	}
	r.Avancar(2 * time.Minute)
	if ok, _ := s.SaidaBotRecente(ctx, c.Contato, "Olá, tudo bem?", time.Minute); ok {
		t.Fatal("fora da janela")
	}
	// saida de humano nao conta
	if _, err := s.RegistrarSaida(ctx, c.ID, AutorHumano, "da equipe", "OUT2", ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SaidaBotRecente(ctx, c.Contato, "da equipe", time.Minute); ok {
		t.Fatal("autor HUMANO nao conta")
	}
}

func TestAtualizarTexto(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	in := entradaCliente(r, "P1", "")
	in.Tipo = TipoAudio
	in.Midia = map[string]any{"url": "x"}
	c, m, _, _ := s.RegistrarEntrada(ctx, in)
	if err := s.AtualizarTexto(ctx, m.ID, "quero ir pra Monção", map[string]any{"transcricao_status": "OK"}); err != nil {
		t.Fatal(err)
	}
	h, _ := s.Historico(ctx, c.ID, 10)
	if h[0].Texto != "quero ir pra Monção" || h[0].Midia["transcricao_status"] != "OK" || h[0].Midia["url"] != "x" {
		t.Fatalf("msg = %+v", h[0])
	}
	if err := s.AtualizarTexto(ctx, "nao-existe", "x", nil); !errors.Is(err, ErrNaoEncontrada) {
		t.Fatalf("err = %v", err)
	}
}

func TestEntradaDeClienteReabreEncerrada(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	v, _ := s.SalvarEstado(ctx, c.ID, Estado{Falhas: 2, ReservaID: "r1"}, c.Versao)
	if _, err := s.MudarStatus(ctx, c.ID, StatusEncerrada, "u1", ""); err != nil {
		t.Fatal(err)
	}
	r.Avancar(time.Minute)
	c2, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P2", "voltei"))
	if c2.Status != StatusBot || c2.ResponsavelID != "" || c2.HumanoAte != nil {
		t.Fatalf("conversa = %+v", c2)
	}
	if c2.Estado.ReservaID != "" || c2.Estado.Falhas != 0 || c2.Versao != v+1 {
		t.Fatalf("estado/versao = %+v v=%d", c2.Estado, c2.Versao)
	}
	if c2.PendenteDesde == nil || !c2.UltimaEntradaEm.Equal(r.Agora()) {
		t.Fatalf("pendencia = %+v", c2)
	}
}

func TestConfirmarEnvio(t *testing.T) {
	s, r := novo(t)
	ctx := context.Background()
	c, _, _, _ := s.RegistrarEntrada(ctx, entradaCliente(r, "P1", "oi"))
	ok1, _ := s.RegistrarSaida(ctx, c.ID, AutorBot, "resposta ok", "", "")
	ruim, _ := s.RegistrarSaida(ctx, c.ID, AutorBot, "resposta falha", "", "")
	if ok, _ := s.SaidaBotRecente(ctx, c.Contato, "resposta ok", time.Minute); !ok {
		t.Fatal("saida gravada antes do envio deve contar")
	}
	if err := s.ConfirmarEnvio(ctx, ok1.ID, "WA1", ""); err != nil {
		t.Fatal(err)
	}
	if known, _ := s.ProvedorIDConhecido(ctx, "WA1"); !known {
		t.Fatal("provedor_id deveria estar gravado")
	}
	if err := s.ConfirmarEnvio(ctx, ruim.ID, "", "timeout"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SaidaBotRecente(ctx, c.Contato, "resposta falha", time.Minute); ok {
		t.Fatal("envio FALHOU deve ser ignorado")
	}
	h, _ := s.Historico(ctx, c.ID, 10)
	if h[len(h)-1].Midia["envio_erro"] != "timeout" {
		t.Fatalf("midia = %+v", h[len(h)-1].Midia)
	}
	if err := s.ConfirmarEnvio(ctx, "nao-existe", "x", ""); !errors.Is(err, ErrNaoEncontrada) {
		t.Fatalf("err = %v", err)
	}
}
