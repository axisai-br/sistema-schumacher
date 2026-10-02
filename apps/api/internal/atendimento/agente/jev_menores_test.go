package agente

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

// estadoPronto: viagem escolhida e um adulto com CPF; falta pagamento.
func estadoPronto() conversa.Estado {
	return conversa.Estado{
		Trechos: []conversa.Trecho{{Viagem: conversa.Opcao{Numero: 1, TripID: "t1", Origem: "Monção", Destino: "Fraiburgo", Data: "2026-10-05", Horario: "08:40", Preco: 950, Vagas: 40}}},
		Passageiros: []conversa.Passageiro{
			{Nome: "Ana Souza", Documento: "52998224725", TipoDocumento: "CPF"},
			{Nome: "Lia Souza", CriancaAte5: true},
		},
	}
}

func ferrsFechamento(chamadas *[]string) []ferramentas.Ferramenta {
	return []ferramentas.Ferramenta{
		ferrFake{nome: "criar_reserva", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
			*chamadas = append(*chamadas, "criar_reserva")
			c.Estado.Trechos[0].ReservaID = "r1"
			c.Estado.Pagamento = "sinal"
			return ferramentas.Saida{OK: true, Dados: map[string]any{
				"pagamento": "sinal", "total_geral": 950.0, "valor_a_pagar_agora": 250.0, "restante_no_embarque_total": 700.0,
				"trechos": []map[string]any{{"ok": true, "codigo_reserva": "ABC123"}},
			}}
		}},
		ferrFake{nome: "gerar_pix", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
			*chamadas = append(*chamadas, "gerar_pix")
			c.Estado.Trechos[0].PagamentoID = "p1"
			return ferramentas.Saida{OK: true, Dados: map[string]any{"pix": []map[string]any{
				{"rota": "Monção para Fraiburgo", "data": "2026-10-05", "valor": 250.0, "pix_copia_e_cola": "000201PIXCODE"},
			}}}
		}},
	}
}

func comEstado(t *testing.T, f *fx, e conversa.Estado) {
	t.Helper()
	if _, err := f.store.SalvarEstado(context.Background(), f.c.ID, e, f.c.Versao); err != nil {
		t.Fatal(err)
	}
	f.c = f.conversa()
}

func TestPreFechamentoSemLLM(t *testing.T) {
	var chamadas []string
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoFormaPagamento, ConfIntencao: 0.95, Pagamento: PagamentoSinal, ConfPagamento: 0.95}, ferrsFechamento(&chamadas)...)
	f.iniciar("o sinal")
	comEstado(t, f, estadoPronto())
	processar(t, f)
	if len(f.modelo.pedidos) != 0 {
		t.Fatalf("nao deveria chamar o LLM, pedidos=%d", len(f.modelo.pedidos))
	}
	if strings.Join(chamadas, ",") != "criar_reserva,gerar_pix" {
		t.Fatalf("chamadas=%v", chamadas)
	}
	if len(f.canal.envios) != 1 {
		t.Fatalf("envios=%q", f.canal.envios)
	}
	e := f.canal.envios[0]
	for _, s := range []string{"000201PIXCODE", "ABC123", "R$ 700", "embarque"} {
		if !strings.Contains(e, s) {
			t.Errorf("faltou %q em %q", s, e)
		}
	}
	if d := decisao(t, f.ultimoTurno()); d != "pre_fechamento" {
		t.Errorf("decisao=%s", d)
	}
	if c := f.conversa(); c.Estado.Trechos[0].PagamentoID != "p1" {
		t.Errorf("estado nao salvo: %+v", c.Estado)
	}
}

func TestPreFechamentoFaltandoPassageiroVaiAoLLM(t *testing.T) {
	var chamadas []string
	f, _ := fxRoteador(t, Rota{Pagamento: PagamentoIntegral, ConfPagamento: 0.95}, ferrsFechamento(&chamadas)...)
	f.modelo.fila = []llm.Resposta{texto("Antes preciso do CPF da Ana.")}
	f.iniciar("integral")
	e := estadoPronto()
	e.Passageiros[0].Documento = ""
	comEstado(t, f, e)
	processar(t, f)
	if len(chamadas) != 0 || len(f.modelo.pedidos) != 1 {
		t.Fatalf("chamadas=%v pedidos=%d", chamadas, len(f.modelo.pedidos))
	}
}

func TestPreFechamentoPorConfirmacao(t *testing.T) {
	for nome, c := range map[string]struct {
		bot    string
		fechar bool
	}{
		"pedido de fechamento": {"Posso fechar a reserva e gerar o PIX?", true},
		"outra pergunta":       {"Quer ver outras datas também?", false},
	} {
		t.Run(nome, func(t *testing.T) {
			var chamadas []string
			f, _ := fxRoteador(t, Rota{Confirma: 0.97, Pagamento: PagamentoNenhum, ConfPagamento: 0.9}, ferrsFechamento(&chamadas)...)
			f.modelo.fila = []llm.Resposta{texto("Certo!")}
			f.iniciar("oi")
			f.botOut(c.bot)
			f.iniciar("sim")
			e := estadoPronto()
			e.Pagamento = "sinal"
			comEstado(t, f, e)
			processar(t, f)
			if got := len(chamadas) == 2; got != c.fechar {
				t.Fatalf("fechou=%v chamadas=%v", got, chamadas)
			}
		})
	}
}

func TestAplicarQuantidade(t *testing.T) {
	var e conversa.Estado
	if !aplicarQuantidade(Rota{Adultos: "1", ConfAdultos: 0.95, Criancas: "2", ConfCriancas: 0.9}, &e) || e.PessoasInformadas != 3 || e.CriancasInformadas != 2 {
		t.Fatalf("%+v", e)
	}
	for _, rt := range []Rota{
		{Adultos: "1", ConfAdultos: 0.95, Criancas: QuantidadeNaoInformada, ConfCriancas: 0.9},
		{Adultos: "2", ConfAdultos: 0.5, Criancas: "0", ConfCriancas: 0.9},
	} {
		var e2 conversa.Estado
		if aplicarQuantidade(rt, &e2) {
			t.Errorf("nao deveria aplicar %+v", rt)
		}
	}
	comPax := conversa.Estado{Passageiros: []conversa.Passageiro{{Nome: "A B"}}}
	if aplicarQuantidade(Rota{Adultos: "3", ConfAdultos: 1, Criancas: "0", ConfCriancas: 1}, &comPax) {
		t.Error("com passageiros nao muda")
	}
}

func TestTextoQuebrado(t *testing.T) {
	for _, s := range []string{
		"<tool_call>\n{\"name\":\"buscar\"}",
		"Perellsellsellsellsellsellsells deep",
		"ok </think> resposta",
	} {
		if textoQuebrado(s) == "" {
			t.Errorf("deveria detectar: %q", s)
		}
	}
	for _, s := range []string{
		"Legal! Escolhi a viagem de 05/10 às 08:40, R$ 950.",
		"PIX:\n00020101021226820014br.gov.bcb.pix2560pix.stone.com.br",
		"Hahaha, claro! 😊",
	} {
		if q := textoQuebrado(s); q != "" {
			t.Errorf("falso positivo %q: %s", s, q)
		}
	}
}

func TestAfirmacoesSemAcao(t *testing.T) {
	vazio := conversa.Estado{}
	if as := afirmacoesSemAcao("Pronto, registrei os passageiros e sua reserva está confirmada!", vazio); len(as) != 2 {
		t.Fatalf("as=%v", as)
	}
	if as := afirmacoesSemAcao("Anotei seu CPF: 529.982.247-25.", vazio); len(as) != 1 {
		t.Fatalf("anotei: %v", as)
	}
	feito := estadoPronto()
	if as := afirmacoesSemAcao("Registrei os passageiros e adicionei a viagem.", feito); len(as) != 0 {
		t.Fatalf("estado ja tem: %v", as)
	}
	if as := afirmacoesSemAcao("Me manda o nome e o CPF de cada passageiro?", vazio); len(as) != 0 {
		t.Fatalf("pergunta nao e afirmacao: %v", as)
	}
}

func TestCpfsNaoRegistrados(t *testing.T) {
	e := conversa.Estado{Trechos: estadoPronto().Trechos}
	if cs := cpfsNaoRegistrados([]string{"joao souza cpf 529.982.247-25"}, e); len(cs) != 1 {
		t.Fatalf("cs=%v", cs)
	}
	if cs := cpfsNaoRegistrados([]string{"cpf 111.111.111-11"}, e); len(cs) != 0 {
		t.Fatalf("cpf invalido: %v", cs)
	}
	if cs := cpfsNaoRegistrados([]string{"cpf 52998224725"}, estadoPronto()); len(cs) != 0 {
		t.Fatalf("ja registrado: %v", cs)
	}
}

func TestFormaReescreveDepoisRespondeEmCodigo(t *testing.T) {
	f := novoFx(t)
	f.modelo.fila = []llm.Resposta{texto("Perfeito, registrei os passageiros!"), texto("Prontinho, registrei tudo.")}
	f.iniciar("ana souza cpf 52998224725")
	comEstado(t, f, conversa.Estado{Trechos: estadoPronto().Trechos})
	processar(t, f)
	if len(f.modelo.pedidos) != 2 {
		t.Fatalf("esperava 1 reescrita, pedidos=%d", len(f.modelo.pedidos))
	}
	if len(f.canal.envios) != 1 || !strings.Contains(f.canal.envios[0], "nome completo e o CPF") {
		t.Fatalf("envios=%q", f.canal.envios)
	}
	if p := passo(f.ultimoTurno(), "forma"); p == nil {
		t.Error("sem passo forma")
	}
}

func TestFormaReescritaBoaEAceita(t *testing.T) {
	f := novoFx(t)
	f.modelo.fila = []llm.Resposta{texto("<tool_call>registrar"), texto("Qual a data de nascimento da criança?")}
	f.iniciar("vai minha filha junto")
	processar(t, f)
	if len(f.canal.envios) != 1 || f.canal.envios[0] != "Qual a data de nascimento da criança?" {
		t.Fatalf("envios=%q", f.canal.envios)
	}
}

func TestRotaDeRespostasNovasPerguntas(t *testing.T) {
	c := func(v float64) *float64 { return &v }
	rt := rotaDeRespostas(map[string]respostaJev{
		"forma_pagamento": {Choice: "sinal", Confidence: c(0.93)},
		"confirma":        {Noul: c(0.8)},
		"adultos":         {Choice: "1", Confidence: c(0.9)},
		"criancas_ate_5":  {Choice: "2", Confidence: c(0.88)},
	}, EntradaRota{})
	if rt.Pagamento != PagamentoSinal || rt.ConfPagamento != 0.93 || rt.Confirma != 0.8 ||
		rt.Adultos != "1" || rt.Criancas != "2" || rt.ConfCriancas != 0.88 {
		t.Fatalf("%+v", rt)
	}
	if rt := rotaDeRespostas(map[string]respostaJev{"forma_pagamento": {Choice: "boleto", Confidence: c(1)}}, EntradaRota{}); rt.Pagamento != "" {
		t.Fatalf("opcao fora da lista: %+v", rt)
	}
}

func TestPerguntasJevCondicionais(t *testing.T) {
	q := perguntasJev(EntradaRota{})
	if q["forma_pagamento"] != nil || q["adultos"] == nil {
		t.Fatalf("sem viagem: so quantidade; %v", q)
	}
	q = perguntasJev(EntradaRota{Estado: estadoPronto()})
	if q["forma_pagamento"] == nil || q["confirma"] == nil || q["adultos"] != nil {
		t.Fatalf("com viagem e passageiros: pagamento sim, quantidade nao; %v", q)
	}
}

func TestPassageirosDeFotos(t *testing.T) {
	hoje := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	ps, avisos := passageirosDeFotos([]string{
		"[foto de documento: nome JOAO VITOR DA SOUZA, CPF 529.982.247-25]",
		"[foto de documento: certidão de nascimento, nome ANA CLARA SOUZA, nascimento 03/04/2023] essa é minha filha",
		"[foto de documento: certidão de nascimento, nome PEDRO SOUZA, nascimento 03/04/2015]",
		"[imagem: uma paisagem]",
	}, hoje)
	if len(ps) != 2 {
		t.Fatalf("ps=%+v", ps)
	}
	if ps[0].Nome != "Joao Vitor da Souza" || ps[0].Documento != "52998224725" || ps[0].TipoDocumento != "CPF" || ps[0].CriancaAte5 {
		t.Errorf("titular: %+v", ps[0])
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "Pedro Souza (11 anos)") {
		t.Errorf("avisos=%v", avisos)
	}
	if ps[1].Nome != "Ana Clara Souza" || !ps[1].CriancaAte5 || ps[1].Documento != "" {
		t.Errorf("crianca: %+v", ps[1])
	}
}

func TestPassageirosDeTexto(t *testing.T) {
	ps := passageirosDeTexto([]string{"paula reis cpf 52998224725, marcos reis cpf 111.444.777-35 e a bebe sofia reis de 2 anos"})
	if len(ps) != 3 || !ps[2].CriancaAte5 || ps[0].Nome != "Paula Reis" || ps[1].Nome != "Marcos Reis" || ps[1].Documento != "11144477735" {
		t.Fatalf("ps=%+v", ps)
	}
	for _, s := range []string{"meu cpf é 52998224725", "o cpf 52998224725", "cpf: 52998224725"} {
		if ps := passageirosDeTexto([]string{s}); len(ps) != 0 {
			t.Errorf("%q nao tem nome: %+v", s, ps)
		}
	}
}

func TestPreRegistroPorFoto(t *testing.T) {
	var recebido string
	reg := ferrFake{nome: "registrar_passageiros", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Passageiros = []conversa.Passageiro{{Nome: "Joao Souza", Documento: "52998224725", TipoDocumento: "CPF"}}
		recebido = "ok"
		return ferramentas.Saida{OK: true}
	}}
	f, _ := fxRoteador(t, Rota{}, reg)
	f.modelo.fila = []llm.Resposta{texto("Confere: Joao Souza, CPF final 725?")}
	f.iniciar("[foto de documento: nome JOAO SOUZA, CPF 529.982.247-25]")
	comEstado(t, f, conversa.Estado{Trechos: estadoPronto().Trechos})
	processar(t, f)
	if recebido != "ok" || decisao(t, f.ultimoTurno()) != "pre_registro" || len(f.conversa().Estado.Passageiros) != 1 {
		t.Fatalf("decisao=%s estado=%+v", decisao(t, f.ultimoTurno()), f.conversa().Estado)
	}
}

func TestFormaRegistraCPFEmCodigo(t *testing.T) {
	reg := ferrFake{nome: "registrar_passageiros", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Passageiros = []conversa.Passageiro{{Nome: "Lucas Alves", Documento: "52998224725", TipoDocumento: "CPF"}}
		return ferramentas.Saida{OK: true}
	}}
	f := novoFx(t, reg)
	f.modelo.fila = []llm.Resposta{texto("Obrigado, Lucas!"), texto("Certo, Lucas.")}
	f.iniciar("lucas alves cpf 52998224725")
	comEstado(t, f, conversa.Estado{Trechos: estadoPronto().Trechos})
	processar(t, f)
	if len(f.canal.envios) != 1 || !strings.Contains(f.canal.envios[0], "Lucas Alves") || !strings.Contains(f.canal.envios[0], "integral") {
		t.Fatalf("envios=%q", f.canal.envios)
	}
}

func buscaFake() ferramentas.Ferramenta {
	return ferrFake{nome: "buscar_viagens", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Origem = &conversa.Parada{StopID: "2", Nome: "Monção", UF: "MA"}
		c.Estado.Destino = &conversa.Parada{StopID: "4", Nome: "Videira", UF: "SC"}
		ops := []conversa.Opcao{{Numero: 1, TripID: "t1", Origem: "Monção", Destino: "Videira", Data: "2026-10-05", Horario: "08:40", Preco: 950, Vagas: 40}}
		c.Estado.Opcoes = ops
		return ferramentas.Saida{OK: true, Dados: map[string]any{"opcoes": ops, "total": 1}}
	}}
}

func TestTemplateOpcoesSemLLM(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95, Origem: "Monção", ConfOrigem: 0.95, Destino: "Videira", ConfDestino: 0.95, SoIsso: 0.95}, buscaFake())
	f.iniciar("de monção pra videira")
	processar(t, f)
	if len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 || !strings.Contains(f.canal.envios[0], "1. seg 05/10 às 08:40, R$ 950") {
		t.Fatalf("pedidos=%d envios=%q", len(f.modelo.pedidos), f.canal.envios)
	}
	if d := decisao(t, f.ultimoTurno()); d != "template_opcoes" {
		t.Errorf("decisao=%s", d)
	}
}

func TestComPerguntaExtraVaiAoLLM(t *testing.T) {
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoBuscarViagens, ConfIntencao: 0.95, Origem: "Monção", ConfOrigem: 0.95, Destino: "Videira", ConfDestino: 0.95, SoIsso: 0.2}, buscaFake())
	f.modelo.fila = []llm.Resposta{texto("Tem sim, a viagem de 05/10 às 08:40 por R$ 950. O ônibus tem ar.")}
	f.iniciar("de monção pra videira, o onibus tem ar?")
	processar(t, f)
	if len(f.modelo.pedidos) != 1 || decisao(t, f.ultimoTurno()) != "pre_busca" {
		t.Fatalf("pedidos=%d", len(f.modelo.pedidos))
	}
}

func TestTemplateEscolhaPedePassageiros(t *testing.T) {
	esc := ferrFake{nome: "escolher_viagem", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Trechos = []conversa.Trecho{{Viagem: c.Estado.Opcoes[0]}}
		return ferramentas.Saida{OK: true}
	}}
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoEscolherOpcao, ConfIntencao: 0.95, Opcao: "1", ConfOpcao: 0.95, SoIsso: 0.9}, esc)
	f.iniciar("a primeira")
	comEstado(t, f, conversa.Estado{PessoasInformadas: 3, CriancasInformadas: 2, Opcoes: []conversa.Opcao{{Numero: 1, TripID: "t1", Origem: "Monção", Destino: "Videira", Data: "2026-10-05", Horario: "08:40", Preco: 950}}})
	processar(t, f)
	if len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 {
		t.Fatalf("pedidos=%d envios=%q", len(f.modelo.pedidos), f.canal.envios)
	}
	e := f.canal.envios[0]
	for _, s := range []string{"Escolhido", "Monção → Videira", "3 passageiros", "2 criança"} {
		if !strings.Contains(e, s) {
			t.Errorf("faltou %q em %q", s, e)
		}
	}
}

func TestGuardaEscolhaSemPedido(t *testing.T) {
	var escolheu bool
	esc := ferrFake{nome: "escolher_viagem", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		escolheu = true
		return ferramentas.Saida{OK: true}
	}}
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoBuscarViagens, ConfIntencao: 0.6}, esc)
	f.modelo.fila = []llm.Resposta{chamada("escolher_viagem", `{"opcao":1}`), texto("Qual opção você prefere?")}
	f.iniciar("quero ir pra videira mês que vem")
	processar(t, f)
	if escolheu {
		t.Fatal("guarda deveria recusar escolher_viagem sem pedido do cliente")
	}
	p := passo(f.ultimoTurno(), "escolher_viagem")
	if p == nil || p.Saida.(ferramentas.Saida).Motivo != "cliente_nao_escolheu" {
		t.Fatalf("passo=%+v", p)
	}
	// Com dia exato, a escolha passa.
	f2, _ := fxRoteador(t, Rota{Intencao: IntencaoBuscarViagens, ConfIntencao: 0.6}, esc)
	f2.modelo.fila = []llm.Resposta{chamada("escolher_viagem", `{"opcao":1}`), texto("Escolhido!")}
	f2.iniciar("quero ir pra videira dia 15")
	processar(t, f2)
	if !escolheu {
		t.Fatal("com dia exato a escolha deveria passar")
	}
}

func estadoReservado() conversa.Estado {
	e := estadoPronto()
	e.Trechos[0].ReservaID, e.Trechos[0].PagamentoID, e.Pagamento = "r1", "p1", "integral"
	return e
}

func TestPosReservaTrocaPagamentoSemLLM(t *testing.T) {
	var args string
	troca := ferrFake{nome: "trocar_pagamento", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Pagamento = "sinal"
		c.Estado.Trechos[0].PagamentoID = "p2"
		args = "ok"
		return ferramentas.Saida{OK: true, Dados: map[string]any{"pix": []map[string]any{
			{"rota": "Monção para Fraiburgo", "data": "2026-10-05", "valor": 250.0, "pix_copia_e_cola": "000201NOVO"},
		}, "restante_no_embarque_total": 700.0}}
	}}
	f, _ := fxRoteador(t, Rota{PosReserva: PosTrocarPagamento, ConfPosReserva: 0.95, Pagamento: PagamentoSinal, ConfPagamento: 0.9}, troca)
	f.iniciar("ah, melhor pagar só o sinal")
	comEstado(t, f, estadoReservado())
	processar(t, f)
	if args != "ok" || len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 {
		t.Fatalf("pedidos=%d envios=%q", len(f.modelo.pedidos), f.canal.envios)
	}
	e := f.canal.envios[0]
	for _, s := range []string{"troquei para sinal", "000201NOVO", "R$ 700"} {
		if !strings.Contains(e, s) {
			t.Errorf("faltou %q em %q", s, e)
		}
	}
	if strings.Contains(e, "Reserva feita") {
		t.Errorf("cabecalho de reserva nova: %q", e)
	}
}

func TestPosReservaTrocaJaPagoTransfere(t *testing.T) {
	troca := ferrFake{nome: "trocar_pagamento", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		return ferramentas.Saida{OK: false, Motivo: "ja_pago"}
	}}
	f, _ := fxRoteador(t, Rota{PosReserva: PosTrocarPagamento, ConfPosReserva: 0.95}, troca)
	f.iniciar("quero pagar só o sinal")
	comEstado(t, f, estadoReservado())
	processar(t, f)
	if f.conversa().Status != conversa.StatusHumano {
		t.Fatalf("deveria transferir: %+v", f.conversa().Status)
	}
}

func TestPosReservaJaPaguei(t *testing.T) {
	for st, quer := range map[string]string{"PAID": "Pagamento confirmado", "PENDING": "Ainda não apareceu"} {
		cons := ferrFake{nome: "consultar_reserva", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
			return ferramentas.Saida{OK: true, Dados: map[string]any{"reservas": []map[string]any{{"codigo_reserva": "ABC", "pagamento": map[string]any{"status": st}}}}}
		}}
		f, _ := fxRoteador(t, Rota{PosReserva: PosJaPaguei, ConfPosReserva: 0.9}, cons)
		f.iniciar("já paguei")
		comEstado(t, f, estadoReservado())
		processar(t, f)
		if len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 || !strings.Contains(f.canal.envios[0], quer) {
			t.Fatalf("%s: envios=%q", st, f.canal.envios)
		}
	}
}

func TestPosReservaTrocaViagemTransfere(t *testing.T) {
	f, _ := fxRoteador(t, Rota{PosReserva: PosTrocarViagem, ConfPosReserva: 0.9})
	f.iniciar("consigo mudar pra dia 15?")
	comEstado(t, f, estadoReservado())
	processar(t, f)
	if f.conversa().Status != conversa.StatusHumano || len(f.modelo.pedidos) != 0 {
		t.Fatal("deveria transferir sem LLM")
	}
}

func TestIdaEVoltaNumaMensagem(t *testing.T) {
	var datas []string
	esc := ferrFake{nome: "escolher_viagem", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		n := len(c.Estado.Trechos)
		o := conversa.Opcao{TripID: fmt.Sprintf("t%d", n+1), Origem: "Monção", Destino: "Videira", Data: "2026-10-08", Horario: "08:40", Preco: 950}
		if n == 1 {
			o.Origem, o.Destino, o.Data = "Videira", "Monção", "2026-10-12"
		}
		c.Estado.Trechos = append(c.Estado.Trechos, conversa.Trecho{Viagem: o})
		datas = append(datas, o.Data)
		return ferramentas.Saida{OK: true}
	}}
	f, _ := fxRoteador(t, Rota{Origem: "Monção", ConfOrigem: 0.95, Destino: "Videira", ConfDestino: 0.95, SoIsso: 0.9}, esc)
	f.iniciar("de monção pra videira, ida dia 8 e volta dia 12")
	processar(t, f)
	if len(datas) != 2 || len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 {
		t.Fatalf("datas=%v pedidos=%d envios=%q", datas, len(f.modelo.pedidos), f.canal.envios)
	}
	e := f.canal.envios[0]
	for _, s := range []string{"Trecho 1: Monção → Videira", "Trecho 2: Videira → Monção", "12/10"} {
		if !strings.Contains(e, s) {
			t.Errorf("faltou %q em %q", s, e)
		}
	}
}

func TestNaoAoFechamento(t *testing.T) {
	var chamadas []string
	f, _ := fxRoteador(t, Rota{Nega: 0.95, Confirma: 0.05}, ferrsFechamento(&chamadas)...)
	f.iniciar("oi")
	f.botOut("Posso fechar a reserva e gerar o PIX?")
	f.iniciar("não, pera")
	e := estadoPronto()
	e.Pagamento = "sinal"
	comEstado(t, f, e)
	processar(t, f)
	if len(chamadas) != 0 || len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 || f.canal.envios[0] != TextoNaoFechar {
		t.Fatalf("chamadas=%v envios=%q", chamadas, f.canal.envios)
	}
}

func TestCorrecaoVaiAoLLMSemFechar(t *testing.T) {
	var chamadas []string
	f, _ := fxRoteador(t, Rota{CorrigePassageiro: 0.9, Pagamento: PagamentoSinal, ConfPagamento: 0.95}, ferrsFechamento(&chamadas)...)
	f.modelo.fila = []llm.Resposta{texto("Qual é o CPF certo?")}
	f.iniciar("o cpf tá errado, é sinal mas corrige o cpf")
	comEstado(t, f, estadoPronto())
	processar(t, f)
	if len(chamadas) != 0 || len(f.modelo.pedidos) != 1 || decisao(t, f.ultimoTurno()) != "llm_correcao" {
		t.Fatalf("chamadas=%v pedidos=%d", chamadas, len(f.modelo.pedidos))
	}
}

func TestCertidaoAcimaDe5Pede(t *testing.T) {
	f, _ := fxRoteador(t, Rota{})
	f.iniciar("[foto de documento: certidão de nascimento, nome PEDRO SOUZA, nascimento 03/04/2015]")
	processar(t, f)
	if len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 || !strings.Contains(f.canal.envios[0], "precisa de CPF ou RG") {
		t.Fatalf("envios=%q", f.canal.envios)
	}
}

func TestExtrairPassageirosTexto(t *testing.T) {
	ps, completo := extrairPassageirosTexto("paula reis cpf 52998224725, marcos reis cpf 111.444.777-35, bebê sofia reis")
	if !completo || len(ps) != 3 || ps[2].Nome != "Sofia Reis" || !ps[2].CriancaAte5 {
		t.Fatalf("completo=%v ps=%+v", completo, ps)
	}
	ps, completo = extrairPassageirosTexto("paula reis cpf 52998224725 e a bebe sofia reis de 2 anos")
	if !completo || len(ps) != 2 || ps[1].Nome != "Sofia Reis" || !ps[1].CriancaAte5 {
		t.Fatalf("idade: completo=%v ps=%+v", completo, ps)
	}
	ps, completo = extrairPassageirosTexto("meu nome é paula reis cpf 52998224725, o onibus tem wifi?")
	if completo || len(ps) != 1 || ps[0].Nome != "Paula Reis" {
		t.Fatalf("com pergunta: completo=%v ps=%+v", completo, ps)
	}
	if ps, _ := extrairPassageirosTexto("pedro lima de 9 anos"); len(ps) != 0 {
		t.Fatalf("acima de 5 sem documento fica para o LLM: %+v", ps)
	}
	if ps, _ := extrairPassageirosTexto("somos 2 adultos e 1 bebê"); len(ps) != 0 {
		t.Fatalf("quantidade nao e passageiro: %+v", ps)
	}
}

func TestRegistroPorTextoTemplate(t *testing.T) {
	var lista int
	reg := ferrFake{nome: "registrar_passageiros", fn: func(c *ferramentas.Contexto) ferramentas.Saida {
		c.Estado.Passageiros = []conversa.Passageiro{{Nome: "Lucas Alves", Documento: "52998224725", TipoDocumento: "CPF"}}
		lista++
		return ferramentas.Saida{OK: true}
	}}
	f, _ := fxRoteador(t, Rota{}, reg)
	f.iniciar("lucas alves cpf 52998224725")
	comEstado(t, f, conversa.Estado{Trechos: estadoPronto().Trechos})
	processar(t, f)
	if lista != 1 || len(f.modelo.pedidos) != 0 || len(f.canal.envios) != 1 || !strings.Contains(f.canal.envios[0], "Lucas Alves") || !strings.Contains(f.canal.envios[0], "integral") {
		t.Fatalf("pedidos=%d envios=%q", len(f.modelo.pedidos), f.canal.envios)
	}
}

func TestGuardaCriarReservaSemPagamento(t *testing.T) {
	var chamadas []string
	f, _ := fxRoteador(t, Rota{Intencao: IntencaoOutro, ConfIntencao: 0.8}, ferrsFechamento(&chamadas)...)
	f.modelo.fila = []llm.Resposta{chamada("criar_reserva", `{"pagamento":"sinal"}`), texto("Você prefere integral ou sinal?")}
	f.iniciar("posso pagar depois?")
	comEstado(t, f, estadoPronto())
	processar(t, f)
	if len(chamadas) != 0 {
		t.Fatalf("guarda deveria recusar criar_reserva: %v", chamadas)
	}
}

func TestPixDepoisDeReservaSemPix(t *testing.T) {
	var chamadas []string
	f, _ := fxRoteador(t, Rota{Pagamento: PagamentoSinal, ConfPagamento: 0.95}, ferrsFechamento(&chamadas)...)
	f.iniciar("pode ser o sinal")
	e := estadoPronto()
	e.Trechos[0].ReservaID, e.Pagamento = "r1", "sinal"
	comEstado(t, f, e)
	processar(t, f)
	if strings.Join(chamadas, ",") != "gerar_pix" || len(f.modelo.pedidos) != 0 || !strings.Contains(f.canal.envios[0], "000201PIXCODE") {
		t.Fatalf("chamadas=%v envios=%q", chamadas, f.canal.envios)
	}
}

func TestTextoOpcoesAgrupaPorRota(t *testing.T) {
	ops := []conversa.Opcao{
		{Numero: 1, Origem: "Fraiburgo", Destino: "Monção", Data: "2026-10-08", Horario: "16:30", Preco: 950},
		{Numero: 2, Origem: "Fraiburgo", Destino: "Santa Inês", Data: "2026-10-08", Horario: "16:30", Preco: 950},
		{Numero: 3, Origem: "Fraiburgo", Destino: "Monção", Data: "2026-10-15", Horario: "16:30", Preco: 950},
	}
	s := textoOpcoes(ops)
	if strings.Count(s, "Opções de Fraiburgo → Monção") != 1 || strings.Index(s, "3. qui 15/10") > strings.Index(s, "Santa Inês") {
		t.Fatalf("%s", s)
	}
}
