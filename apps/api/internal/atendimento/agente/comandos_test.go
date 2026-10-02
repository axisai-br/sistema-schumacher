package agente

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

// regFake grava a lista recebida em registrar_passageiros (como a real).
type regFake struct{}

func (regFake) Def() llm.DefFerramenta {
	return llm.DefFerramenta{Nome: "registrar_passageiros", Descricao: "fake", Parametros: json.RawMessage(`{"type":"object"}`)}
}
func (regFake) Executar(_ context.Context, c *ferramentas.Contexto, args json.RawMessage) ferramentas.Saida {
	var a struct {
		Passageiros []struct {
			Nome          string `json:"nome"`
			Documento     string `json:"documento"`
			TipoDocumento string `json:"tipo_documento"`
			Crianca       bool   `json:"crianca_ate_5"`
		} `json:"passageiros"`
	}
	if json.Unmarshal(args, &a) != nil {
		return ferramentas.Saida{OK: false, Motivo: "argumentos_invalidos"}
	}
	c.Estado.Passageiros = nil
	for _, p := range a.Passageiros {
		c.Estado.Passageiros = append(c.Estado.Passageiros, conversa.Passageiro{Nome: p.Nome, Documento: p.Documento, TipoDocumento: p.TipoDocumento, CriancaAte5: p.Crianca})
	}
	return ferramentas.Saida{OK: true}
}

// fxComandos: fixture com Jev fake e motor por comandos; o modelo fake
// responde a extracao com o JSON dado (repetido para a votacao).
func fxComandos(t *testing.T, rt Rota, extracao string, ferrs ...ferramentas.Ferramenta) *fx {
	t.Helper()
	f, _ := fxRoteador(t, rt, ferrs...)
	r := texto(extracao)
	f.modelo.repetir = &r
	return f
}

func iniciarComandos(f *fx, textos ...string) {
	f.iniciar(textos...)
	f.ag = Novo(f.deps, Config{Modelo: "m-teste", Motor: MotorComandos})
}

// semTextoLivre: no motor por comandos o LLM nunca recebe ferramentas.
func semTextoLivre(t *testing.T, f *fx) {
	t.Helper()
	for i, p := range f.modelo.pedidos {
		if len(p.Ferramentas) > 0 {
			t.Fatalf("pedido %d com ferramentas (%d): o LLM nao pode agir no motor por comandos", i, len(p.Ferramentas))
		}
	}
}

func TestComandosPagamentoAmbiguoNaoFecha(t *testing.T) {
	var chamadas []string
	f := fxComandos(t, Rota{}, `{"pedido":"concorda","pagamento":"sinal","confirma":"sim","assunto":"nenhum"}`, ferrsFechamento(&chamadas)...)
	iniciarComandos(f, "pode 👍")
	comEstado(t, f, estadoPronto())
	processar(t, f)
	semTextoLivre(t, f)
	if len(chamadas) != 0 {
		t.Fatalf("\"pode 👍\" nao escolhe forma de pagamento: chamadas=%v", chamadas)
	}
	if e := f.canal.envios[0]; !strings.Contains(strings.ToLower(e), "integral") {
		t.Errorf("deveria perguntar integral ou sinal: %q", e)
	}
}

func TestComandosPagamentoEscritoFecha(t *testing.T) {
	var chamadas []string
	f := fxComandos(t, Rota{}, `{"pedido":"escolhe sinal","pagamento":"sinal","confirma":"nenhum","assunto":"nenhum"}`, ferrsFechamento(&chamadas)...)
	iniciarComandos(f, "vou pagar só a entrada")
	comEstado(t, f, estadoPronto())
	processar(t, f)
	semTextoLivre(t, f)
	if strings.Join(chamadas, ",") != "criar_reserva,gerar_pix" {
		t.Fatalf("chamadas=%v", chamadas)
	}
	if !strings.Contains(f.canal.envios[0], "000201PIXCODE") {
		t.Errorf("sem PIX: %q", f.canal.envios[0])
	}
}

func TestComandosPerguntaSemRespostaNaoInventa(t *testing.T) {
	f := fxComandos(t, Rota{}, `{"pedido":"pergunta bagagem","pagamento":"nenhum","confirma":"nenhum","assunto":"bagagem"}`)
	iniciarComandos(f, "quanto de bagagem posso levar?")
	processar(t, f)
	semTextoLivre(t, f)
	if e := f.canal.envios[0]; !strings.Contains(e, TextoNaoSei) {
		t.Errorf("esperava TextoNaoSei: %q", e)
	}
}

func TestComandosValoresCalculados(t *testing.T) {
	f := fxComandos(t, Rota{}, `{"pedido":"pergunta sinal","pagamento":"nenhum","confirma":"nenhum","assunto":"valores"}`)
	iniciarComandos(f, "quanto fica o sinal?")
	e := estadoPronto()
	e.Passageiros = append(e.Passageiros, conversa.Passageiro{Nome: "Rui Souza", Documento: "11144477735", TipoDocumento: "CPF"})
	comEstado(t, f, e)
	processar(t, f)
	for _, s := range []string{"R$ 1.900", "R$ 500", "R$ 1.400"} {
		if !strings.Contains(f.canal.envios[0], s) {
			t.Errorf("faltou %q em %q", s, f.canal.envios[0])
		}
	}
}

func TestComandosPersona(t *testing.T) {
	f := fxComandos(t, Rota{}, `{"pedido":"pergunta se e robo","pagamento":"nenhum","confirma":"nenhum","assunto":"outro"}`)
	iniciarComandos(f, "você é um robô? qual modelo, GPT?")
	processar(t, f)
	if e := f.canal.envios[0]; !strings.Contains(e, TextoPersona) || strings.Contains(strings.ToLower(e), "nvidia") {
		t.Errorf("persona: %q", e)
	}
}

func TestComandosExtratorQuebradoSegueProximoPasso(t *testing.T) {
	f := fxComandos(t, Rota{}, `isso nao e json -0-0-0-0-0-0-0-0`)
	iniciarComandos(f, "oi, tudo bem?")
	processar(t, f)
	if len(f.canal.envios) != 1 || f.canal.envios[0] != TextoPedirRota {
		t.Errorf("esperava pedir a rota: %q", f.canal.envios)
	}
}

func TestComandosVotacaoPassageiros(t *testing.T) {
	f, _ := fxRoteador(t, Rota{}, regFake{})
	// 3 chamadas: Carlos em 2, Zeca so em 1 (alucinacao) -> so Carlos entra.
	f.modelo.fila = []llm.Resposta{
		texto(`{"pedido":"p","passageiros":[{"nome":"carlos lima","documento":"84434891030","tipo_documento":"CPF"}],"pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}`),
		texto(`{"pedido":"p","passageiros":[{"nome":"carlos lima","documento":"84434891030","tipo_documento":"CPF"},{"nome":"zeca silva","documento":"52998224725","tipo_documento":"CPF"}],"pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}`),
		texto(`{"pedido":"p","passageiros":[],"pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}`),
	}
	iniciarComandos(f, "o passageiro é o carlinhos, carlos lima, documento 844.348.910-30 rg nao tenho")
	comEstado(t, f, estadoPronto())
	processar(t, f)
	if n := len(f.modelo.pedidos); n != 3 {
		t.Fatalf("esperava 3 chamadas de extracao (votacao), veio %d", n)
	}
	e := f.canal.envios[0]
	if !strings.Contains(e, "Carlos Lima") || strings.Contains(e, "Zeca") {
		t.Errorf("votacao: %q", e)
	}
}

// regConta grava a lista e anota a chamada.
type regConta struct{ chamadas *[]string }

func (r regConta) Def() llm.DefFerramenta { return regFake{}.Def() }
func (r regConta) Executar(ctx context.Context, c *ferramentas.Contexto, args json.RawMessage) ferramentas.Saida {
	*r.chamadas = append(*r.chamadas, "registrar_passageiros")
	return regFake{}.Executar(ctx, c, args)
}

func opcaoDia(n int, data, hora string) conversa.Opcao {
	return conversa.Opcao{Numero: n, TripID: "t" + string(rune('0'+n)), Origem: "Santa Inês", Destino: "Chapecó", Data: data, Horario: hora, Preco: 1000, Vagas: 40}
}

func buscaOpcoes(chamadas *[]string, ops []conversa.Opcao) ferramentas.Ferramenta {
	return ferrFake{nome: "buscar_viagens", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		*chamadas = append(*chamadas, "buscar_viagens")
		c.Estado.Origem = &conversa.Parada{StopID: "1", Nome: "Santa Inês", UF: "MA"}
		c.Estado.Destino = &conversa.Parada{StopID: "3", Nome: "Chapecó", UF: "SC"}
		c.Estado.Opcoes = append([]conversa.Opcao(nil), ops...)
		return ferramentas.Saida{OK: true, Dados: map[string]any{"opcoes": ops}}
	}}
}

func escolhePrimeira(chamadas *[]string) ferramentas.Ferramenta {
	return ferrFake{nome: "escolher_viagem", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		*chamadas = append(*chamadas, "escolher_viagem")
		if len(c.Estado.Opcoes) == 0 {
			return ferramentas.Saida{OK: false, Motivo: "sem_opcao"}
		}
		c.Estado.Trechos = append(c.Estado.Trechos, conversa.Trecho{Viagem: c.Estado.Opcoes[0]})
		return ferramentas.Saida{OK: true}
	}}
}

func TestComandosTudoJuntoFecha(t *testing.T) {
	var chamadas []string
	ops := []conversa.Opcao{opcaoDia(1, "2026-10-12", "07:30")}
	ferrs := append([]ferramentas.Ferramenta{regConta{&chamadas}, buscaOpcoes(&chamadas, ops), escolhePrimeira(&chamadas)}, ferrsFechamento(&chamadas)...)
	f := fxComandos(t, Rota{
		Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95,
		Origem: "Santa Inês", ConfOrigem: 0.95, Destino: "Chapecó", ConfDestino: 0.95,
	}, `{"pedido":"rota data passageiros e sinal","origem":"Santa Inês","destino":"Chapecó","passageiros":[{"nome":"francisco alves lima","documento":"39053344705","tipo_documento":"CPF"},{"nome":"antonia lima","documento":"71460238001","tipo_documento":"CPF"}],"pagamento":"sinal","confirma":"nenhum","assunto":"nenhum"}`,
		ferrs...)
	iniciarComandos(f, "Boa noite! Quero 2 passagens de Santa Inês para Chapecó no dia 12/10, eu Francisco Alves Lima CPF 39053344705 e minha esposa Antonia Lima CPF 71460238001, vou pagar só o sinal.")
	processar(t, f)
	semTextoLivre(t, f)
	if strings.Join(chamadas, ",") != "registrar_passageiros,buscar_viagens,escolher_viagem,criar_reserva,gerar_pix" {
		t.Fatalf("chamadas=%v", chamadas)
	}
	e := f.canal.envios[0]
	if !strings.Contains(e, "000201PIXCODE") || strings.Contains(strings.ToLower(e), "esposa antonia") {
		t.Errorf("resposta=%q", e)
	}
	ps := f.conversa().Estado.Passageiros
	if len(ps) != 2 || !strings.Contains(ps[0].Nome, "Francisco") || !strings.Contains(ps[1].Nome, "Antonia") {
		t.Fatalf("passageiros=%+v", ps)
	}
}

func TestComandosTudoJuntoDuasOpcoesPergunta(t *testing.T) {
	var chamadas []string
	ops := []conversa.Opcao{opcaoDia(1, "2026-10-12", "07:30"), opcaoDia(2, "2026-10-12", "14:00")}
	ferrs := append([]ferramentas.Ferramenta{regConta{&chamadas}, buscaOpcoes(&chamadas, ops), escolhePrimeira(&chamadas)}, ferrsFechamento(&chamadas)...)
	f := fxComandos(t, Rota{
		Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95,
		Origem: "Santa Inês", ConfOrigem: 0.95, Destino: "Chapecó", ConfDestino: 0.95,
	}, `{"pedido":"rota data passageiros e sinal","origem":"Santa Inês","destino":"Chapecó","passageiros":[{"nome":"francisco alves lima","documento":"39053344705","tipo_documento":"CPF"},{"nome":"antonia lima","documento":"71460238001","tipo_documento":"CPF"}],"pagamento":"sinal","confirma":"nenhum","assunto":"nenhum"}`,
		ferrs...)
	iniciarComandos(f, "Quero 2 passagens de Santa Inês para Chapecó no dia 12/10, Francisco Alves Lima CPF 39053344705 e Antonia Lima CPF 71460238001, sinal.")
	processar(t, f)
	if strings.Contains(strings.Join(chamadas, ","), "escolher_viagem") || strings.Contains(strings.Join(chamadas, ","), "criar_reserva") {
		t.Fatalf("duas opcoes no dia nao fecham: %v", chamadas)
	}
	e := f.canal.envios[0]
	if !strings.Contains(e, "Qual delas") || !strings.Contains(e, "Francisco") {
		t.Errorf("deveria listar passageiros e perguntar a viagem: %q", e)
	}
}

func TestComandosPassageiroESinalFecha(t *testing.T) {
	var chamadas []string
	ferrs := append([]ferramentas.Ferramenta{regConta{&chamadas}}, ferrsFechamento(&chamadas)...)
	f := fxComandos(t, Rota{}, `{"pedido":"manda o jose e escolhe sinal","passageiros":[{"nome":"josé francisco dos santos de souza filho","documento":"84434891030","tipo_documento":"CPF"}],"pagamento":"sinal","confirma":"nenhum","assunto":"nenhum"}`,
		ferrs...)
	iniciarComandos(f, "josé francisco dos santos de souza filho, cpf 844.348.910-30 sinal")
	comEstado(t, f, estadoPronto())
	processar(t, f)
	semTextoLivre(t, f)
	if strings.Join(chamadas, ",") != "registrar_passageiros,criar_reserva,gerar_pix" {
		t.Fatalf("chamadas=%v", chamadas)
	}
	ps := f.conversa().Estado.Passageiros
	var achou bool
	for _, p := range ps {
		if p.Nome == "José Francisco dos Santos de Souza Filho" {
			achou = true
		}
	}
	if !achou || len(ps) < 3 {
		t.Fatalf("nome composto nao preservado: %+v", ps)
	}
	if !strings.Contains(f.canal.envios[0], "000201PIXCODE") {
		t.Errorf("sem PIX: %q", f.canal.envios[0])
	}
}

func TestSombraRegistraExtracaoSemMudarResposta(t *testing.T) {
	f := fxComandos(t, Rota{Intencao: IntencaoSaudacao, ConfIntencao: 0.95}, `{"pedido":"cumprimenta","pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}`)
	f.iniciar("oi")
	f.ag = Novo(f.deps, Config{Modelo: "m-teste", Motor: MotorSombra})
	processar(t, f)
	if len(f.canal.envios) != 1 || f.canal.envios[0] != TextoSaudacao {
		t.Fatalf("a resposta continua sendo a do motor atual: %q", f.canal.envios)
	}
	if p := passo(f.ultimoTurno(), "extrator_sombra"); p == nil {
		t.Fatal("sem passo extrator_sombra")
	}
}

func TestEnriquecerOrigemNovaComDestinoDoEstado(t *testing.T) {
	e := conversa.Estado{
		Origem:  &conversa.Parada{Nome: "Santa Inês"},
		Destino: &conversa.Parada{Nome: "Videira"},
		Opcoes: []conversa.Opcao{
			{Numero: 1, Origem: "Santa Inês", Destino: "Videira"},
			{Numero: 2, Origem: "Monção", Destino: "Videira"},
			{Numero: 3, Origem: "Igarapé do Meio", Destino: "Videira"},
		},
	}
	rt := Rota{Intencao: IntencaoOutro, ConfIntencao: 0.6, Origem: "Igarapé do Meio", ConfOrigem: 0.95, Destino: CidadeNaoInformada, ConfDestino: 0.9}
	got, _ := enriquecerRota(rt, nil, e, "saindo de igarape do meio", nil, time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	if got.Intencao != IntencaoBuscarViagens {
		t.Fatalf("origem nova sobre lista mista deve buscar: %+v", got)
	}
	a := &Agente{cfg: Config{LimiarRota: 0.8}}
	if args, ok := a.argsPreBusca(got, e, false); !ok || args["origem"] != "Igarapé do Meio" || args["destino"] != "Videira" {
		t.Fatalf("args=%v ok=%v", args, ok)
	}
}

func TestComandosRespostaLivreForaDoFluxo(t *testing.T) {
	f, _ := fxRoteador(t, Rota{})
	f.modelo.fila = []llm.Resposta{
		texto(`{"pedido":"pergunta se tem onibus amanha cedo","pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}`),
		texto("Nossas viagens ligam o Maranhão a Santa Catarina. Me diz de qual cidade você sai e pra onde vai que eu te mostro as datas."),
	}
	iniciarComandos(f, "vcs fazem viagem pra onde mesmo? to meio perdido")
	processar(t, f)
	semTextoLivre(t, f)
	if e := f.canal.envios[0]; !strings.Contains(e, "Nossas viagens ligam") {
		t.Fatalf("deveria usar a resposta do LLM: %q", e)
	}
	if p := passo(f.ultimoTurno(), "resposta_livre"); p == nil {
		t.Fatal("sem passo resposta_livre")
	}
}

func TestComandosRespostaLivreComAcaoInventadaVoltaProTemplate(t *testing.T) {
	f, _ := fxRoteador(t, Rota{})
	f.modelo.fila = []llm.Resposta{
		texto(`{"pedido":"quer viajar","pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}`),
		texto("Pronto, reservei sua passagem! Reserva confirmada."),
	}
	iniciarComandos(f, "quero viajar semana que vem")
	processar(t, f)
	if e := f.canal.envios[0]; strings.Contains(strings.ToLower(e), "reservei") || e != TextoPedirRota {
		t.Fatalf("resposta com acao inventada deve cair no template: %q", e)
	}
}
