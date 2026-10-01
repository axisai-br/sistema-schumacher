package agente

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

// ---- fakes ----

type canalFake struct {
	mu     sync.Mutex
	envios []string
	erro   error
}

func (f *canalFake) Nome() string { return "WHATSAPP" }
func (f *canalFake) Normalizar(context.Context, []byte) (canal.Entrada, bool, error) {
	return canal.Entrada{}, false, nil
}
func (f *canalFake) Enviar(_ context.Context, _ string, texto string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.erro != nil {
		return "", f.erro
	}
	f.envios = append(f.envios, texto)
	return fmt.Sprintf("prov-%d", len(f.envios)), nil
}
func (f *canalFake) BaixarMidia(context.Context, conversa.Mensagem) (string, string, error) {
	return "", "", nil
}

type modeloFake struct {
	fila    []llm.Resposta
	repetir *llm.Resposta
	erro    error
	antes   func(n int)
	pedidos []llm.Pedido
}

func (m *modeloFake) Gerar(_ context.Context, p llm.Pedido) (llm.Resposta, error) {
	m.pedidos = append(m.pedidos, p)
	if m.antes != nil {
		m.antes(len(m.pedidos))
	}
	if m.erro != nil {
		return llm.Resposta{}, m.erro
	}
	if len(m.fila) > 0 {
		r := m.fila[0]
		m.fila = m.fila[1:]
		return r, nil
	}
	if m.repetir != nil {
		return *m.repetir, nil
	}
	return llm.Resposta{}, errors.New("fila do modelo vazia")
}

func texto(t string) llm.Resposta { return llm.Resposta{Texto: t, TokensEntrada: 10, TokensSaida: 5} }
func chamada(nome, args string) llm.Resposta {
	return llm.Resposta{Chamadas: []llm.ChamadaFerramenta{{ID: "call-1", Nome: nome, Argumentos: json.RawMessage(args)}}}
}

type ferrFake struct {
	nome string
	fn   func(c *ferramentas.Contexto) ferramentas.Saida
}

func (f ferrFake) Def() llm.DefFerramenta {
	return llm.DefFerramenta{Nome: f.nome, Descricao: "fake", Parametros: json.RawMessage(`{"type":"object"}`)}
}
func (f ferrFake) Executar(_ context.Context, c *ferramentas.Contexto, _ json.RawMessage) ferramentas.Saida {
	return f.fn(c)
}

type catalogoFake struct{}

func (catalogoFake) TextoCatalogo(context.Context) (string, error) {
	return "Santa Inês/MA - Chapecó/SC: R$ 950,00", nil
}

type notifFake struct {
	chamadas int
	motivo   string
	resumo   string
}

func (n *notifFake) AvisarTransferencia(_ context.Context, _ conversa.Conversa, motivo, resumo string) error {
	n.chamadas++
	n.motivo, n.resumo = motivo, resumo
	return nil
}

type juizFake struct {
	av  Avaliacao
	err error
}

func (j juizFake) Avaliar(context.Context, []conversa.Mensagem) (Avaliacao, error) {
	return j.av, j.err
}

type midiaFake struct{}

func (midiaFake) Preparar(context.Context, conversa.Mensagem) (string, map[string]any, error) {
	return "[áudio não compreendido]", map[string]any{"transcricao_status": "FALHA"}, nil
}

// ---- fixture ----

type fx struct {
	t      *testing.T
	store  *conversa.StoreMem
	canal  *canalFake
	modelo *modeloFake
	notif  *notifFake
	deps   Deps
	ag     *Agente
	c      conversa.Conversa
}

func novoFx(t *testing.T, ferrs ...ferramentas.Ferramenta) *fx {
	t.Helper()
	var mu sync.Mutex
	rel := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := conversa.NewStoreMem(func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		rel = rel.Add(time.Second)
		return rel
	})
	f := &fx{t: t, store: store, canal: &canalFake{}, modelo: &modeloFake{}, notif: &notifFake{}}
	f.deps = Deps{
		Store: store, Canal: f.canal, Modelo: f.modelo,
		Ferramentas: ferramentas.NovoRegistro(ferrs...),
		Catalogo:    catalogoFake{}, Notificador: f.notif,
		Agora: func() time.Time { return time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC) },
	}
	return f
}

func (f *fx) iniciar(textos ...string) {
	f.t.Helper()
	f.ag = Novo(f.deps, Config{Modelo: "m-teste"})
	for i, tx := range textos {
		c, _, _, err := f.store.RegistrarEntrada(context.Background(), conversa.NovaEntrada{
			Canal: "WHATSAPP", Contato: "5511@s.whatsapp.net", Telefone: "5511", Nome: "Ana",
			Autor: conversa.AutorCliente, Tipo: conversa.TipoTexto, Texto: tx, ProvedorID: fmt.Sprintf("in-%d", i),
		})
		if err != nil {
			f.t.Fatal(err)
		}
		f.c = c
	}
}

func (f *fx) conversa() conversa.Conversa {
	c, err := f.store.Obter(context.Background(), f.c.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}

func (f *fx) ultimoTurno() conversa.Turno {
	ts, _ := f.store.Turnos(context.Background(), f.c.ID, 50)
	if len(ts) == 0 {
		f.t.Fatal("sem turnos")
	}
	return ts[len(ts)-1]
}

func (f *fx) botOut(t string) {
	if _, err := f.store.RegistrarSaida(context.Background(), f.c.ID, conversa.AutorBot, t, "", ""); err != nil {
		f.t.Fatal(err)
	}
}

func buscarViagens() ferramentas.Ferramenta {
	return ferrFake{nome: "buscar_viagens", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Destino = &conversa.Parada{StopID: "s1", Nome: "Chapecó", UF: "SC"}
		return ferramentas.Saida{OK: true, Dados: map[string]any{"opcoes": []map[string]any{
			{"numero": 1, "data": "2026-10-15", "horario": "08:00", "preco": 950.0},
		}}}
	}}
}

func transferirFerr() ferramentas.Ferramenta {
	return ferrFake{nome: "transferir_para_humano", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		return ferramentas.Saida{OK: true, Motivo: "duvida fora do alcance", Transferir: true}
	}}
}

// ---- testes ----

func TestRespostaSimplesEnviada(t *testing.T) {
	f := novoFx(t)
	f.modelo.fila = []llm.Resposta{texto("Oi, Ana! De qual cidade você vai sair?")}
	f.iniciar("oi")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 1 || f.canal.envios[0] != "Oi, Ana! De qual cidade você vai sair?" {
		t.Fatalf("envios %v", f.canal.envios)
	}
	c := f.conversa()
	if c.PendenteDesde != nil {
		t.Fatal("pendencia nao concluida")
	}
	tu := f.ultimoTurno()
	if tu.Resultado != conversa.ResultadoEnviado || tu.TokensEntrada != 10 || tu.TokensSaida != 5 || tu.Modelo != "m-teste" || len(tu.EntradaIDs) != 1 {
		t.Fatalf("turno %+v", tu)
	}
	h, _ := f.store.Historico(context.Background(), f.c.ID, 10)
	if len(h) != 2 || h[1].Autor != conversa.AutorBot || h[1].TurnoID != tu.ID {
		t.Fatalf("historico %+v", h)
	}
	ins := f.modelo.pedidos[0].Instrucoes
	for _, s := range []string{"Shabas", "# CATÁLOGO", "R$ 950,00", "# ESTADO DA RESERVA", "Pendências:", "Nome do cliente: Ana", "30/09/2026 12:00"} {
		if !contem(ins, s) {
			t.Errorf("instrucoes sem %q", s)
		}
	}
}

func contem(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestFerramentaDepoisTexto(t *testing.T) {
	f := novoFx(t, buscarViagens())
	f.modelo.fila = []llm.Resposta{
		chamada("buscar_viagens", `{"destino":"Chapecó"}`),
		texto("Achei uma opção: 1) 15/10 às 08:00 por R$ 950,00. Quer essa?"),
	}
	f.iniciar("tem viagem pra Chapecó?")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 1 {
		t.Fatalf("envios %v", f.canal.envios)
	}
	c := f.conversa()
	if c.Estado.Destino == nil || c.Estado.Destino.Nome != "Chapecó" {
		t.Fatalf("estado nao salvo: %+v", c.Estado)
	}
	tu := f.ultimoTurno()
	var nFerr int
	for _, p := range tu.Passos {
		if p.Tipo == "ferramenta" && p.Nome == "buscar_viagens" {
			nFerr++
		}
	}
	if nFerr != 1 || tu.EstadoAntes.Destino != nil || tu.EstadoDepois.Destino == nil {
		t.Fatalf("turno %+v", tu)
	}
	// o segundo pedido ao modelo leva a chamada e o resultado
	ms := f.modelo.pedidos[1].Mensagens
	n := len(ms)
	if ms[n-2].Papel != llm.PapelAssistente || len(ms[n-2].Chamadas) != 1 || ms[n-1].Papel != llm.PapelFerramenta || ms[n-1].ChamadaID != "call-1" {
		t.Fatalf("mensagens %+v", ms)
	}
}

func TestMensagemNovaDuranteTurno(t *testing.T) {
	f := novoFx(t)
	f.modelo.repetir = ptr(texto("Olá! Para onde vai?"))
	f.iniciar("oi")
	f.modelo.antes = func(int) {
		_, _, _, err := f.store.RegistrarEntrada(context.Background(), conversa.NovaEntrada{
			Canal: "WHATSAPP", Contato: "5511@s.whatsapp.net", Autor: conversa.AutorCliente,
			Tipo: conversa.TipoTexto, Texto: "quero ir pra Chapecó", ProvedorID: "in-novo",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 0 {
		t.Fatal("nao deveria enviar")
	}
	if f.conversa().PendenteDesde == nil {
		t.Fatal("pendencia deveria continuar")
	}
	if tu := f.ultimoTurno(); tu.Resultado != conversa.ResultadoDescartadoMsgNova {
		t.Fatalf("resultado %s", tu.Resultado)
	}
}

func ptr[T any](v T) *T { return &v }

func TestPedidoDeAjudaTransfereSemLLM(t *testing.T) {
	f := novoFx(t)
	f.iniciar("Preciso de ajuda")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.modelo.pedidos) != 0 {
		t.Fatal("LLM nao deveria ser chamado")
	}
	if len(f.canal.envios) != 1 || f.canal.envios[0] != TextoTransferencia {
		t.Fatalf("envios %v", f.canal.envios)
	}
	c := f.conversa()
	if c.Status != conversa.StatusHumano || c.Estado.MotivoHumano == "" || c.PendenteDesde != nil {
		t.Fatalf("conversa %+v", c)
	}
	if f.notif.chamadas != 1 || f.notif.resumo == "" {
		t.Fatalf("notif %+v", f.notif)
	}
	if tu := f.ultimoTurno(); tu.Resultado != conversa.ResultadoHumano {
		t.Fatal(tu.Resultado)
	}
}

func TestJuizTransfere(t *testing.T) {
	f := novoFx(t)
	f.deps.Juiz = juizFake{av: Avaliacao{Irritacao: 0.9}}
	f.iniciar("vocês são um lixo")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if f.conversa().Status != conversa.StatusHumano || len(f.modelo.pedidos) != 0 {
		t.Fatal("deveria transferir sem LLM")
	}
}

func TestJuizComErroEIgnorado(t *testing.T) {
	f := novoFx(t)
	f.deps.Juiz = juizFake{err: errors.New("fora do ar")}
	f.modelo.fila = []llm.Resposta{texto("Oi! Para onde você vai?")}
	f.iniciar("oi")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 1 || f.conversa().Status != conversa.StatusBot {
		t.Fatal("deveria responder normalmente")
	}
}

func TestFatosReescreveUmaVez(t *testing.T) {
	f := novoFx(t)
	f.modelo.fila = []llm.Resposta{
		texto("A passagem custa R$ 999,00."),
		texto("A passagem custa R$ 950,00."),
	}
	f.iniciar("quanto custa pra Chapecó?")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 1 || f.canal.envios[0] != "A passagem custa R$ 950,00." {
		t.Fatalf("envios %v", f.canal.envios)
	}
	ms := f.modelo.pedidos[1].Mensagens
	ult := ms[len(ms)-1]
	if ult.Papel != llm.PapelUsuario || !contem(ult.Texto, "999,00") || !contem(ult.Texto, "não vieram das ferramentas") {
		t.Fatalf("instrucao de reescrita: %+v", ult)
	}
}

// Sem busca no turno e sem rota no estado: em vez de transferir, pede a rota.
func TestFatosFalhaDuasVezesPedeRota(t *testing.T) {
	f := novoFx(t)
	f.modelo.repetir = ptr(texto("Sai dia 20/10 às 09:30 por R$ 999,00."))
	f.iniciar("quanto custa?")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.modelo.pedidos) != 2 {
		t.Fatalf("pedidos %d", len(f.modelo.pedidos))
	}
	if len(f.canal.envios) != 1 || f.canal.envios[0] != TextoPedirRota || f.conversa().Status != conversa.StatusBot {
		t.Fatalf("envios %v", f.canal.envios)
	}
}

func TestRepeticaoTransfere(t *testing.T) {
	f := novoFx(t)
	pergunta := "Qual é a sua cidade de origem?"
	f.modelo.repetir = ptr(texto(pergunta))
	f.iniciar("oi")
	f.botOut(pergunta)
	// primeira repeticao: envia e marca Falhas=1
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 1 || f.conversa().Estado.Falhas != 1 {
		t.Fatalf("envios=%v falhas=%d", f.canal.envios, f.conversa().Estado.Falhas)
	}
	// cliente responde algo e o bot repete de novo: transfere
	_, _, _, _ = f.store.RegistrarEntrada(context.Background(), conversa.NovaEntrada{
		Canal: "WHATSAPP", Contato: "5511@s.whatsapp.net", Autor: conversa.AutorCliente,
		Tipo: conversa.TipoTexto, Texto: "hã?", ProvedorID: "in-x",
	})
	c := f.conversa()
	if err := f.ag.Processar(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 2 || f.canal.envios[1] != TextoTransferencia || f.conversa().Status != conversa.StatusHumano {
		t.Fatalf("envios %v", f.canal.envios)
	}
}

func TestEstadoAvancandoZeraFalhas(t *testing.T) {
	f := novoFx(t, buscarViagens())
	f.iniciar("oi")
	if _, err := f.store.SalvarEstado(context.Background(), f.c.ID, conversa.Estado{Falhas: 1}, 0); err != nil {
		t.Fatal(err)
	}
	f.c = f.conversa()
	f.botOut("Para onde você vai?")
	f.modelo.fila = []llm.Resposta{chamada("buscar_viagens", `{}`), texto("Qual dia você prefere viajar?")}
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if f.conversa().Estado.Falhas != 0 || len(f.canal.envios) != 1 {
		t.Fatalf("falhas=%d", f.conversa().Estado.Falhas)
	}
}

func TestErroDoModeloMensagemTecnicaETransfere(t *testing.T) {
	f := novoFx(t)
	f.modelo.erro = errors.New("openai fora do ar")
	f.iniciar("oi")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 1 || f.canal.envios[0] != TextoTecnico {
		t.Fatalf("envios %v", f.canal.envios)
	}
	if f.conversa().Status != conversa.StatusHumano || f.notif.chamadas != 1 {
		t.Fatal("deveria transferir e notificar")
	}
}

// Limite de passos depois de uma busca: responde com as opcoes reais.
func TestMaxPassosComBuscaRespondeOpcoes(t *testing.T) {
	f := novoFx(t, buscarViagens())
	f.modelo.repetir = ptr(chamada("buscar_viagens", `{}`))
	f.iniciar("oi")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.modelo.pedidos) != 6 || len(f.canal.envios) != 1 || !strings.Contains(f.canal.envios[0], "1. qui 15/10 às 08:00") || f.conversa().Status != conversa.StatusBot {
		t.Fatalf("pedidos=%d envios=%v", len(f.modelo.pedidos), f.canal.envios)
	}
}

// Limite de passos sem nada aproveitavel: transferencia tecnica.
func TestSemTextoAposMaxPassosTransfere(t *testing.T) {
	f := novoFx(t, transferirFerr())
	f.modelo.repetir = ptr(chamada("ferramenta_inexistente", `{}`))
	f.iniciar("oi")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if len(f.canal.envios) != 1 || f.canal.envios[0] != TextoTecnico {
		t.Fatalf("pedidos=%d envios=%v", len(f.modelo.pedidos), f.canal.envios)
	}
}

func TestFerramentaTransferir(t *testing.T) {
	f := novoFx(t, transferirFerr())
	f.modelo.fila = []llm.Resposta{chamada("transferir_para_humano", `{"motivo":"x"}`)}
	f.iniciar("quero cancelar minha passagem")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	c := f.conversa()
	if c.Status != conversa.StatusHumano || c.Estado.MotivoHumano != "duvida fora do alcance" || f.notif.motivo != "duvida fora do alcance" {
		t.Fatalf("conversa %+v notif %+v", c, f.notif)
	}
	if len(f.canal.envios) != 1 || f.canal.envios[0] != TextoTransferencia {
		t.Fatalf("envios %v", f.canal.envios)
	}
}

func TestConflitoDeVersaoERetornaErro(t *testing.T) {
	f := novoFx(t)
	f.modelo.repetir = ptr(texto("Oi! Para onde você vai?"))
	f.iniciar("oi")
	// outro escritor altera o estado: a versao de f.c fica velha
	if _, err := f.store.SalvarEstado(context.Background(), f.c.ID, conversa.Estado{}, 0); err != nil {
		t.Fatal(err)
	}
	err := f.ag.Processar(context.Background(), f.c)
	if !errors.Is(err, conversa.ErrVersaoConflito) {
		t.Fatalf("err=%v", err)
	}
	if len(f.canal.envios) != 0 || f.conversa().PendenteDesde == nil {
		t.Fatal("nao deveria enviar nem concluir")
	}
	if tu := f.ultimoTurno(); tu.Resultado != conversa.ResultadoErro || tu.Erro == "" {
		t.Fatalf("turno %+v", tu)
	}
	// apos 3 erros seguidos, transfere mesmo com a versao velha
	for i := 0; i < 2; i++ {
		if err := f.ag.Processar(context.Background(), f.c); err == nil {
			t.Fatal("esperava erro")
		}
	}
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if f.conversa().Status != conversa.StatusHumano || len(f.canal.envios) != 1 || f.canal.envios[0] != TextoTecnico {
		t.Fatalf("envios %v status %s", f.canal.envios, f.conversa().Status)
	}
}

func TestErroDeEnvioMantemPendencia(t *testing.T) {
	f := novoFx(t)
	f.canal.erro = errors.New("evolution fora")
	f.modelo.repetir = ptr(texto("Oi! Para onde você vai?"))
	f.iniciar("oi")
	if err := f.ag.Processar(context.Background(), f.c); err == nil {
		t.Fatal("esperava erro")
	}
	if f.conversa().PendenteDesde == nil {
		t.Fatal("pendencia deveria continuar")
	}
	if tu := f.ultimoTurno(); tu.Resultado != conversa.ResultadoErro {
		t.Fatalf("turno %+v", tu)
	}
	h, _ := f.store.Historico(context.Background(), f.c.ID, 10)
	if len(h) != 2 || h[1].Midia["envio_status"] != "FALHOU" || h[1].Midia["envio_erro"] == "" || h[1].ProvedorID != "" {
		t.Fatalf("saida deveria estar marcada como falha: %+v", h)
	}
	// reenvio: a saida que falhou nao conta como "ultima do bot" nem entra no contexto
	f.canal.erro = nil
	if err := f.ag.Processar(context.Background(), f.conversa()); err != nil { // o worker recarrega a conversa
		t.Fatal(err)
	}
	if f.conversa().Estado.Falhas != 0 || len(f.canal.envios) != 1 {
		t.Fatal("reenvio deveria funcionar sem contar repeticao")
	}
	for _, m := range f.modelo.pedidos[len(f.modelo.pedidos)-1].Mensagens {
		if m.Papel == llm.PapelAssistente {
			t.Fatalf("saida falha vazou para o contexto: %+v", m)
		}
	}
}

func TestSaidaGravadaAntesDoEnvio(t *testing.T) {
	f := novoFx(t)
	f.modelo.repetir = ptr(texto("Oi! Para onde você vai?"))
	f.iniciar("oi")
	var visto bool
	f.deps.Canal = canalHook{canalFake: f.canal, antes: func() {
		h, _ := f.store.Historico(context.Background(), f.c.ID, 10)
		visto = len(h) == 2 && h[1].Autor == conversa.AutorBot && h[1].ProvedorID == ""
	}}
	f.ag = Novo(f.deps, Config{Modelo: "m"})
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	if !visto {
		t.Fatal("RegistrarSaida deveria acontecer antes de Canal.Enviar")
	}
	h, _ := f.store.Historico(context.Background(), f.c.ID, 10)
	if h[1].ProvedorID != "prov-1" {
		t.Fatalf("provedor_id nao confirmado: %+v", h[1])
	}
	ok, _ := f.store.ProvedorIDConhecido(context.Background(), "prov-1")
	if !ok {
		t.Fatal("provedor id deveria ser conhecido")
	}
}

func TestTransferenciaConfirmaEnvio(t *testing.T) {
	f := novoFx(t)
	f.iniciar("quero um atendente")
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	h, _ := f.store.Historico(context.Background(), f.c.ID, 10)
	if len(h) != 2 || h[1].ProvedorID != "prov-1" {
		t.Fatalf("%+v", h)
	}
	// falha no envio da transferencia: erro, status continua BOT
	f2 := novoFx(t)
	f2.canal.erro = errors.New("fora")
	f2.iniciar("quero um atendente")
	if err := f2.ag.Processar(context.Background(), f2.c); err == nil {
		t.Fatal("esperava erro")
	}
	h2, _ := f2.store.Historico(context.Background(), f2.c.ID, 10)
	if f2.conversa().Status != conversa.StatusBot || f2.conversa().PendenteDesde == nil || h2[1].Midia["envio_status"] != "FALHOU" {
		t.Fatalf("%+v", h2)
	}
}

type canalHook struct {
	*canalFake
	antes func()
}

func (c canalHook) Enviar(ctx context.Context, contato, texto string) (string, error) {
	c.antes()
	return c.canalFake.Enviar(ctx, contato, texto)
}

func TestMidiaPreparada(t *testing.T) {
	f := novoFx(t)
	f.deps.Midia = midiaFake{}
	f.modelo.fila = []llm.Resposta{texto("Não consegui ouvir seu áudio. Pode escrever, por favor?")}
	f.iniciar()
	f.ag = Novo(f.deps, Config{Modelo: "m"})
	c, _, _, _ := f.store.RegistrarEntrada(context.Background(), conversa.NovaEntrada{
		Canal: "WHATSAPP", Contato: "5511@s.whatsapp.net", Autor: conversa.AutorCliente,
		Tipo: conversa.TipoAudio, Midia: map[string]any{"url": "x"}, ProvedorID: "aud-1",
	})
	f.c = c
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
	h, _ := f.store.Historico(context.Background(), f.c.ID, 10)
	if h[0].Texto != "[áudio não compreendido]" || h[0].Midia["preparada"] != true || h[0].Midia["transcricao_status"] != "FALHA" {
		t.Fatalf("msg %+v", h[0])
	}
	ms := f.modelo.pedidos[0].Mensagens
	if ms[0].Texto != "[áudio não compreendido]" {
		t.Fatalf("modelo viu %+v", ms)
	}
	if len(f.canal.envios) != 1 {
		t.Fatal("deveria responder")
	}
}

func TestHistoricoMapeado(t *testing.T) {
	out := mapearHistorico([]conversa.Mensagem{
		{Autor: conversa.AutorCliente, Texto: "oi"},
		{Autor: conversa.AutorBot, Texto: "olá"},
		{Autor: conversa.AutorHumano, Texto: "posso ajudar"},
	})
	if out[0].Papel != llm.PapelUsuario || out[1].Papel != llm.PapelAssistente || out[2].Texto != "[atendente humano] posso ajudar" {
		t.Fatalf("%+v", out)
	}
}

func TestSemEntradasConcluiPendencia(t *testing.T) {
	f := novoFx(t)
	f.iniciar("oi")
	c := f.c
	antes := c.PendenteDesde.Add(time.Hour) // nada pendente a partir daqui
	c.PendenteDesde = &antes
	if err := f.ag.Processar(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 0 {
		t.Fatal("nada a fazer")
	}
}
