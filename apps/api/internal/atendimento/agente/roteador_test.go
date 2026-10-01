package agente

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

type roteadorFake struct {
	rt   Rota
	err  error
	n    int
	last EntradaRota
}

func (r *roteadorFake) Rotear(_ context.Context, e EntradaRota) (Rota, error) {
	r.n++
	r.last = e
	return r.rt, r.err
}

type cidadesFake struct{}

func (cidadesFake) Cidades(context.Context) ([]ferramentas.Cidade, error) {
	return []ferramentas.Cidade{
		{StopID: "1", Nome: "Santa Inês", UF: "MA"}, {StopID: "2", Nome: "Monção", UF: "MA"},
		{StopID: "3", Nome: "Chapecó", UF: "SC"}, {StopID: "4", Nome: "Videira", UF: "SC"},
	}, nil
}

func fxRoteador(t *testing.T, rt Rota, ferrs ...ferramentas.Ferramenta) (*fx, *roteadorFake) {
	t.Helper()
	f := novoFx(t, ferrs...)
	r := &roteadorFake{rt: rt}
	f.deps.Roteador = r
	f.deps.Cidades = cidadesFake{}
	return f, r
}

func passo(t conversa.Turno, nome string) *conversa.Passo {
	for i := range t.Passos {
		if t.Passos[i].Nome == nome {
			return &t.Passos[i]
		}
	}
	return nil
}

func decisao(t *testing.T, tn conversa.Turno) string {
	t.Helper()
	p := passo(tn, "roteador")
	if p == nil {
		t.Fatal("sem passo roteador")
	}
	return p.Saida.(map[string]any)["decisao"].(string)
}

func processar(t *testing.T, f *fx) {
	t.Helper()
	if err := f.ag.Processar(context.Background(), f.c); err != nil {
		t.Fatal(err)
	}
}

func TestRoteadorTransfereHumano(t *testing.T) {
	f, _ := fxRoteador(t, Rota{PedeHumano: 0.9})
	f.iniciar("quero uma pessoa")
	processar(t, f)
	if f.conversa().Status != conversa.StatusHumano || len(f.modelo.pedidos) != 0 {
		t.Fatal("deveria transferir sem LLM")
	}
	if d := decisao(t, f.ultimoTurno()); d != "transfere_humano" {
		t.Errorf("decisao = %s", d)
	}
}

func TestRoteadorTransfereIrritacao(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Irritacao: 0.8})
	f.iniciar("péssimo")
	processar(t, f)
	if f.conversa().Status != conversa.StatusHumano || len(f.modelo.pedidos) != 0 {
		t.Fatal("deveria transferir sem LLM")
	}
	if d := decisao(t, f.ultimoTurno()); d != "transfere_irritacao" {
		t.Errorf("decisao = %s", d)
	}
}

func TestRoteadorSaudacaoTemplate(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoSaudacao, ConfIntencao: 0.95})
	f.iniciar("oi")
	processar(t, f)
	if len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 || f.canal.envios[0] != TextoSaudacao {
		t.Fatalf("envios=%q pedidos=%d", f.canal.envios, len(f.modelo.pedidos))
	}
	tn := f.ultimoTurno()
	if tn.Resultado != conversa.ResultadoEnviado || tn.Resposta != TextoSaudacao || decisao(t, tn) != "template_saudacao" {
		t.Errorf("turno: %+v", tn)
	}
	if f.conversa().PendenteDesde != nil {
		t.Error("pendencia deveria estar concluida")
	}
}

func TestRoteadorSaudacaoComBotJaRespondeuVaiAoLLM(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoSaudacao, ConfIntencao: 0.95})
	f.modelo.fila = []llm.Resposta{texto("Oi de novo! Para onde?")}
	f.iniciar("oi")
	f.botOut("Olá!")
	f.iniciar("oi")
	processar(t, f)
	if len(f.modelo.pedidos) != 1 {
		t.Fatalf("deveria chamar o LLM, pedidos=%d", len(f.modelo.pedidos))
	}
}

func TestRoteadorCidadesTemplate(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoCidadesAtendidas, ConfIntencao: 0.9})
	f.iniciar("quais cidades vocês atendem?")
	processar(t, f)
	if len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 {
		t.Fatalf("envios=%q pedidos=%d", f.canal.envios, len(f.modelo.pedidos))
	}
	e := f.canal.envios[0]
	for _, s := range []string{"Maranhão: Santa Inês e Monção.", "Santa Catarina: Chapecó e Videira.", "De qual cidade você sai ou pra onde quer ir?"} {
		if !strings.Contains(e, s) {
			t.Errorf("faltou %q em %q", s, e)
		}
	}
	if d := decisao(t, f.ultimoTurno()); d != "template_cidades" {
		t.Errorf("decisao = %s", d)
	}
}

func TestRoteadorTemplateRespeitaMensagemNova(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoSaudacao, ConfIntencao: 0.95})
	f.iniciar("oi")
	f.ag = Novo(f.deps, Config{Modelo: "m-teste"})
	// Chega outra mensagem depois do marco do turno (f.c ainda e a versao antiga).
	if _, _, _, err := f.store.RegistrarEntrada(context.Background(), conversa.NovaEntrada{
		Canal: "WHATSAPP", Contato: "5511@s.whatsapp.net", Telefone: "5511", Nome: "Ana",
		Autor: conversa.AutorCliente, Tipo: conversa.TipoTexto, Texto: "quero ir a Videira", ProvedorID: "in-x",
	}); err != nil {
		t.Fatal(err)
	}
	processar(t, f)
	if len(f.canal.envios) != 0 || f.ultimoTurno().Resultado != conversa.ResultadoDescartadoMsgNova {
		t.Fatalf("deveria descartar: envios=%q", f.canal.envios)
	}
}

func TestRoteadorPreBuscaUmaChamadaAoLLM(t *testing.T) {
	f, _ := fxRoteador(t, Rota{
		Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95,
		Destino: "Chapecó", ConfDestino: 0.97, Origem: CidadeNaoInformada, ConfOrigem: 0.9,
	}, ferrFake{nome: "buscar_viagens", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Destino = &conversa.Parada{StopID: "3", Nome: "Chapecó", UF: "SC"}
		return ferramentas.Saida{OK: true, Dados: map[string]any{"opcoes": []map[string]any{{"numero": 1}}}}
	}})
	f.modelo.fila = []llm.Resposta{texto("Achei viagens pra Chapecó. Qual você prefere?")}
	f.iniciar("tem viagem pra chapecó?")
	processar(t, f)
	if len(f.modelo.pedidos) != 1 {
		t.Fatalf("esperava 1 chamada ao LLM, houve %d", len(f.modelo.pedidos))
	}
	ms := f.modelo.pedidos[0].Mensagens
	n := len(ms)
	if n < 3 || len(ms[n-2].Chamadas) != 1 || ms[n-2].Chamadas[0].ID != "pre_1" || ms[n-2].Chamadas[0].Nome != "buscar_viagens" ||
		ms[n-1].Papel != llm.PapelFerramenta || ms[n-1].ChamadaID != "pre_1" || !strings.Contains(ms[n-1].Texto, `"ok":true`) {
		t.Fatalf("mensagens: %+v", ms)
	}
	if got := string(ms[n-2].Chamadas[0].Argumentos); got != `{"destino":"Chapecó"}` {
		t.Errorf("args = %s", got)
	}
	tn := f.ultimoTurno()
	if decisao(t, tn) != "pre_busca" || passo(tn, "buscar_viagens") == nil || tn.EstadoDepois.Destino == nil {
		t.Errorf("passos: %+v", tn.Passos)
	}
	if len(f.canal.envios) != 1 {
		t.Errorf("envios=%q", f.canal.envios)
	}
}

func TestRoteadorPreBuscaFalhaSegueFluxoNormal(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95, Destino: "Chapecó", ConfDestino: 0.97},
		ferrFake{nome: "buscar_viagens", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
			c.Estado.Destino = &conversa.Parada{Nome: "lixo"}
			return ferramentas.Saida{OK: false, Motivo: "erro_busca"}
		}})
	f.modelo.fila = []llm.Resposta{texto("Tive um problema, tenta de novo?")}
	f.iniciar("tem viagem pra chapecó?")
	processar(t, f)
	ms := f.modelo.pedidos[0].Mensagens
	if len(ms) != 1 || len(ms[0].Chamadas) != 0 {
		t.Fatalf("nao deveria injetar chamada: %+v", ms)
	}
	tn := f.ultimoTurno()
	if decisao(t, tn) != "pre_busca_falhou" || tn.EstadoDepois.Destino != nil {
		t.Errorf("estado nao deve mudar com busca falha: %+v", tn.EstadoDepois)
	}
}

func TestRoteadorBaixaConfiancaOuCasosSemPreBuscaVaoAoLLM(t *testing.T) {
	casos := map[string]Rota{
		"intencao baixa":        {Intencao: IntencaoSaudacao, ConfIntencao: 0.5},
		"busca confianca baixa": {Intencao: IntencaoBuscarViagens, ConfIntencao: 0.6, Destino: "Chapecó", ConfDestino: 0.99},
		"cidade baixa":          {Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95, Destino: "Chapecó", ConfDestino: 0.5},
		"nao atendida":          {Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95, Destino: CidadeNaoAtendida, ConfDestino: 0.95},
		"com data":              {Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95, Destino: "Chapecó", ConfDestino: 0.95, DetalhesExtras: 0.9},
		"outra intencao":        {Intencao: IntencaoDuvidaInformativa, ConfIntencao: 0.95},
	}
	for nome, rt := range casos {
		t.Run(nome, func(t *testing.T) {
			f, _ := fxRoteador(t, rt, buscarViagens())
			f.modelo.fila = []llm.Resposta{texto("Resposta do LLM")}
			f.iniciar("mensagem")
			processar(t, f)
			if len(f.modelo.pedidos) != 1 || passo(f.ultimoTurno(), "buscar_viagens") != nil {
				t.Fatalf("pedidos=%d passos=%+v", len(f.modelo.pedidos), f.ultimoTurno().Passos)
			}
			if d := decisao(t, f.ultimoTurno()); d != "llm" {
				t.Errorf("decisao = %s", d)
			}
		})
	}
}

func TestRoteadorErroSegueNoLLM(t *testing.T) {
	f, r := fxRoteador(t, Rota{})
	r.err = errors.New("jev fora do ar")
	f.modelo.fila = []llm.Resposta{texto("Oi! Para onde?")}
	f.iniciar("oi")
	processar(t, f)
	if len(f.modelo.pedidos) != 1 || len(f.canal.envios) != 1 {
		t.Fatalf("deveria responder pelo LLM")
	}
	p := passo(f.ultimoTurno(), "roteador")
	if p == nil || p.Erro == "" {
		t.Errorf("passo: %+v", p)
	}
}

// ---- roteador Jev (servidor falso) ----

func jevFalso(t *testing.T, resposta string, corpo *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if corpo != nil {
			_ = json.Unmarshal(b, corpo)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(resposta))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRoteadorJevRequisicaoEResposta(t *testing.T) {
	var corpo map[string]any
	srv := jevFalso(t, `{"answers":{
	 "pede_humano":{"type":"noul","noul":0.05},
	 "irritacao":{"type":"score","score":0.4,"confidence":0.9},
	 "menciona_data_ou_pessoas":{"type":"noul","noul":0.1},
	 "intencao":{"type":"choice","choice":"buscar_viagens","confidence":0.93},
	 "origem":{"type":"choice","choice":"nao_informado","confidence":0.88},
	 "destino":{"type":"choice","choice":"Chapecó","confidence":0.97},
	 "opcao_escolhida":{"type":"choice","choice":"2","confidence":0.7}}}`, &corpo)
	r := NovoRoteadorJev("k", nil).(*roteadorJev)
	r.c.url = srv.URL
	rt, err := r.Rotear(context.Background(), EntradaRota{
		Mensagens: []conversa.Mensagem{
			{Autor: conversa.AutorBot, Texto: "Oi!"},
			{Autor: conversa.AutorCliente, Texto: "tem pra chapecó?"},
		},
		Estado: conversa.Estado{Opcoes: []conversa.Opcao{
			{Numero: 1, Origem: "Videira", Destino: "Chapecó", Data: "2026-10-15", Horario: "08:00"},
			{Numero: 2, Origem: "Videira", Destino: "Chapecó", Data: "2026-10-16", Horario: "09:00"},
		}},
		Cidades: []ferramentas.Cidade{{Nome: "Chapecó", UF: "SC"}, {Nome: "Videira", UF: "SC"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Intencao != IntencaoBuscarViagens || rt.ConfIntencao != 0.93 || rt.Destino != "Chapecó" || rt.ConfDestino != 0.97 ||
		rt.Origem != CidadeNaoInformada || rt.Opcao != "2" || rt.PedeHumano != 0.05 || rt.Irritacao != 0.2 || rt.DetalhesExtras != 0.1 {
		t.Errorf("rota: %+v", rt)
	}
	qs := corpo["questions"].(map[string]any)
	for _, k := range []string{"pede_humano", "irritacao", "intencao", "origem", "destino", "opcao_escolhida", "menciona_data_ou_pessoas"} {
		if _, ok := qs[k]; !ok {
			t.Errorf("faltou a pergunta %s", k)
		}
	}
	crit := qs["origem"].(map[string]any)["criteria"].(map[string]any)
	if _, ok := crit["Videira"]; !ok || crit[CidadeNaoAtendida] == nil || crit[CidadeNaoInformada] == nil {
		t.Errorf("criteria origem: %v", crit)
	}
	st := corpo["state"].(map[string]any)
	if st["latest_customer_message"] != "tem pra chapecó?" || st["current_options"] == nil || st["served_cities"] == nil {
		t.Errorf("state: %v", st)
	}
	if corpo["model"] != "jev-latest" {
		t.Errorf("model = %v", corpo["model"])
	}
}

func TestRoteadorJevSemOpcoesSemCidades(t *testing.T) {
	var corpo map[string]any
	srv := jevFalso(t, `{"answers":{"intencao":{"type":"choice","choice":"saudacao_apenas","confidence":0.99}}}`, &corpo)
	r := NovoRoteadorJev("k", nil).(*roteadorJev)
	r.c.url = srv.URL
	rt, err := r.Rotear(context.Background(), EntradaRota{Mensagens: []conversa.Mensagem{{Autor: conversa.AutorCliente, Texto: "oi"}}})
	if err != nil || rt.Intencao != IntencaoSaudacao {
		t.Fatalf("%+v %v", rt, err)
	}
	qs := corpo["questions"].(map[string]any)
	for _, k := range []string{"origem", "destino", "opcao_escolhida"} {
		if _, ok := qs[k]; ok {
			t.Errorf("pergunta %s nao deveria existir", k)
		}
	}
}

func TestRoteadorJevCidadeDesconhecidaViraVazio(t *testing.T) {
	srv := jevFalso(t, `{"answers":{"destino":{"type":"choice","choice":"Atlantida","confidence":0.99}}}`, nil)
	r := NovoRoteadorJev("k", nil).(*roteadorJev)
	r.c.url = srv.URL
	rt, err := r.Rotear(context.Background(), EntradaRota{Cidades: []ferramentas.Cidade{{Nome: "Chapecó", UF: "SC"}}})
	if err != nil || rt.Destino != "" || rt.ConfDestino != 0 {
		t.Fatalf("%+v %v", rt, err)
	}
}

func TestRoteadorJevTimeoutEErro(t *testing.T) {
	lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer lento.Close()
	r := NovoRoteadorJev("k", nil).(*roteadorJev)
	r.c.url, r.timeout = lento.URL, 100*time.Millisecond
	t0 := time.Now()
	if _, err := r.Rotear(context.Background(), EntradaRota{}); err == nil || time.Since(t0) > time.Second {
		t.Fatalf("esperava timeout rapido: %v (%v)", err, time.Since(t0))
	}
	ruim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer ruim.Close()
	r.c.url, r.timeout = ruim.URL, time.Second
	if _, err := r.Rotear(context.Background(), EntradaRota{}); err == nil {
		t.Fatal("esperava erro de status")
	}
}

func TestTimeoutPadraoDoRoteador(t *testing.T) {
	if NovoRoteadorJev("k", nil).(*roteadorJev).timeout != 3*time.Second {
		t.Error("timeout padrao deve ser 3s")
	}
}
