package agente

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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

// semTextoLivre: no motor por comandos todo pedido ao LLM e extracao JSON.
func semTextoLivre(t *testing.T, f *fx) {
	t.Helper()
	for i, p := range f.modelo.pedidos {
		if len(p.SaidaJSON) == 0 || len(p.Ferramentas) > 0 {
			t.Fatalf("pedido %d nao e extracao (SaidaJSON=%d, ferramentas=%d)", i, len(p.SaidaJSON), len(p.Ferramentas))
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
