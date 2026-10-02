package agente

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/llm/chatcompat"
)

// Motor por comandos, etapa "entender": o LLM so EXTRAI o que o cliente disse
// num JSON de esquema fixo (sem ferramentas, sem politica, sem texto livre
// para o cliente). Quem decide e executa e o codigo (rotear/fluxo). Saida em
// JSON restrito elimina texto degenerado e eco do prompt; o contexto curto
// cabe bem em modelo pequeno.

// Extracao e o que o extrator devolve.
type Extracao struct {
	Pedido       string            `json:"pedido"`
	Origem       string            `json:"origem,omitempty"`
	Destino      string            `json:"destino,omitempty"`
	Quando       string            `json:"quando,omitempty"`
	Opcao        int               `json:"opcao,omitempty"`
	Pessoas      int               `json:"pessoas,omitempty"`
	CriancasAte5 int               `json:"criancas_ate_5,omitempty"`
	Passageiros  []PassageiroExtr  `json:"passageiros,omitempty"`
	Corrigir     []CorrecaoExtr    `json:"corrigir,omitempty"`
	Remover      []string          `json:"remover,omitempty"`
	Pagamento    string            `json:"pagamento"`
	Confirma     string            `json:"confirma"`
	Assunto      string            `json:"assunto"`
	Humano       bool              `json:"humano,omitempty"`
	Desistir     bool              `json:"desistir,omitempty"`
	Votos        int               `json:"-"` // chamadas que concordaram (passageiros)
	extras       map[string]string // nao serializado
}

type PassageiroExtr struct {
	Nome          string `json:"nome"`
	Documento     string `json:"documento,omitempty"`
	TipoDocumento string `json:"tipo_documento,omitempty"`
	Idade         int    `json:"idade,omitempty"`
}

type CorrecaoExtr struct {
	Nome      string `json:"nome"`
	Documento string `json:"documento"`
}

// assuntosFAQ sao os valores aceitos em "assunto" (chaves de faq.md + valores).
var assuntosFAQ = []string{"valores", "garantia_vaga", "formas_pagamento", "bagagem", "animais", "duracao", "comodidades",
	"descontos", "comprovante", "embarque", "documentos", "criancas", "outro", "nenhum"}

func esquemaExtracao() json.RawMessage {
	enumJSON := func(v []string) string { b, _ := json.Marshal(v); return string(b) }
	return json.RawMessage(`{"type":"object","properties":{
"pedido":{"type":"string","description":"o que o cliente quer nesta mensagem, em ate 12 palavras"},
"origem":{"type":"string"},"destino":{"type":"string"},
"quando":{"type":"string","description":"data ou periodo exatamente como o cliente escreveu"},
"opcao":{"type":"integer","description":"numero da opcao escolhida entre as mostradas; 0 se nao escolheu"},
"pessoas":{"type":"integer","description":"total de pessoas que vao viajar, se o cliente disse; 0 se nao"},
"criancas_ate_5":{"type":"integer"},
"passageiros":{"type":"array","items":{"type":"object","properties":{
  "nome":{"type":"string"},"documento":{"type":"string"},
  "tipo_documento":{"type":"string","enum":["CPF","RG","CNH",""]},"idade":{"type":"integer"}},"required":["nome"]}},
"corrigir":{"type":"array","items":{"type":"object","properties":{"nome":{"type":"string"},"documento":{"type":"string"}},"required":["nome","documento"]}},
"remover":{"type":"array","items":{"type":"string"}},
"pagamento":{"type":"string","enum":["integral","sinal","nenhum"]},
"confirma":{"type":"string","enum":["sim","nao","nenhum"]},
"assunto":{"type":"string","enum":` + enumJSON(assuntosFAQ) + `},
"humano":{"type":"boolean"},"desistir":{"type":"boolean"}},
"required":["pedido","pagamento","confirma","assunto"]}`)
}

const instrucoesExtrator = `Você extrai dados de mensagens de clientes de uma empresa de ônibus (Maranhão <-> Santa Catarina) no WhatsApp.
Responda SÓ com o JSON do esquema. Não converse, não explique, não invente.

Regras:
- Use só o que o CLIENTE escreveu nas mensagens novas. Mensagens do atendente servem só de contexto.
- passageiros: cada pessoa com nome completo escrito pelo cliente; documento só com os dígitos que ele mandou; idade se ele disse. Não repita quem já está registrado, a não ser que ele mande dado novo.
- corrigir: quando ele corrige o documento de alguém já registrado ("errei o cpf da maria, o certo é ...").
- remover: primeiro nome de quem não vai mais.
- pagamento: "integral" ou "sinal" SÓ se ele escreveu a forma ("sinal", "entrada", "integral", "tudo", "valor total"). "pode", "ok", "👍", "sim" não são forma de pagamento: use "nenhum".
- confirma: "sim"/"nao" quando responde a uma pergunta de sim ou não do atendente.
- opcao: número da opção que ele escolheu entre as mostradas ("a primeira" = 1, "a do dia 12" = a opção desse dia). 0 se não escolheu.
- assunto: se ele PERGUNTA algo (bagagem, animais, valores, pagamento com cartão, garantia da vaga...), a chave do assunto; "outro" se é pergunta de outro tema; "nenhum" se não perguntou nada.
- humano: true só se pede para falar com uma pessoa/atendente.
- desistir: true só se desiste da compra.

Exemplos:
Cliente: "somos 3, eu e minhas 2 filhas de 4 e 9 anos" -> {"pedido":"informa 3 pessoas","pessoas":3,"criancas_ate_5":1,"pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}
Cliente: "joana prado 111.444.777-35 e o tiago prado rg 4512887" -> {"pedido":"manda passageiros","passageiros":[{"nome":"joana prado","documento":"11144477735","tipo_documento":"CPF"},{"nome":"tiago prado","documento":"4512887","tipo_documento":"RG"}],"pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}
Cliente: "errei o cpf da maria, o certo é 987.654.321-00" -> {"pedido":"corrige cpf da maria","corrigir":[{"nome":"maria","documento":"98765432100"}],"pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}
Cliente: "pode ser, manda o pix" -> {"pedido":"quer fechar","pagamento":"nenhum","confirma":"sim","assunto":"nenhum"}
Cliente: "quanto fica o sinal? e aceita cartão?" -> {"pedido":"pergunta valores e cartao","pagamento":"nenhum","confirma":"nenhum","assunto":"valores"}
Cliente: "vou pagar só a entrada" -> {"pedido":"escolhe sinal","pagamento":"sinal","confirma":"nenhum","assunto":"nenhum"}
Cliente: "o bruno não vai mais, vai o carlos lima cpf 84434891030" -> {"pedido":"troca passageiro","remover":["bruno"],"passageiros":[{"nome":"carlos lima","documento":"84434891030","tipo_documento":"CPF"}],"pagamento":"nenhum","confirma":"nenhum","assunto":"nenhum"}`

// extrair chama o extrator (votacao com 3 chamadas quando a mensagem traz
// passageiros) e devolve nil se nada valido voltou no prazo.
func (a *Agente) extrair(ctx context.Context, tc *turno, hist []conversa.Mensagem) *Extracao {
	texto := textoRecenteCliente(hist)
	if strings.TrimSpace(texto) == "" {
		return nil
	}
	ped := llm.Pedido{
		Modelo:     a.cfg.Modelo,
		Instrucoes: instrucoesExtrator,
		Mensagens:  []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: contextoExtrator(tc.estado, hist)}},
		SaidaJSON:  esquemaExtracao(),
		MaxTokens:  700,
	}
	n := 1
	if reCPF.MatchString(texto) || reVariosNomes.MatchString(texto) {
		n = 3 // votacao: passageiros so entram se 2 de 3 concordarem
	}
	ctxE, cancel := context.WithTimeout(ctx, a.cfg.OrcamentoExtrator)
	defer cancel()
	t0 := a.d.Agora()
	res := a.chamarExtrator(ctxE, ped, n)
	if len(res) == 0 && a.cfg.ModeloExtratorReserva != "" && ctx.Err() == nil {
		ped.Modelo = a.cfg.ModeloExtratorReserva
		ctxR, cancelR := context.WithTimeout(ctx, a.cfg.OrcamentoExtrator)
		res = a.chamarExtrator(ctxR, ped, 1)
		cancelR()
	}
	var ex *Extracao
	if len(res) > 0 {
		ex = votar(res)
	}
	tc.passos = append(tc.passos, conversa.Passo{Tipo: "comandos", Nome: "extrator", Saida: map[string]any{"chamadas": n, "validas": len(res), "extracao": ex},
		DuracaoMS: a.d.Agora().Sub(t0).Milliseconds()})
	return ex
}

var reVariosNomes = regexp.MustCompile(`(?i)\b(rg|cnh|anos|beb[eê]|filh[oa]|esposa|marido)\b`)

func (a *Agente) chamarExtrator(ctx context.Context, ped llm.Pedido, n int) []Extracao {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var out []Extracao
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := a.d.Modelo.Gerar(ctx, ped)
			if err != nil {
				return
			}
			bruto, ok := chatcompat.ExtrairObjetoJSON(chatcompat.RemoverPensamento(resp.Texto))
			if !ok {
				return
			}
			var ex Extracao
			if json.Unmarshal([]byte(bruto), &ex) != nil {
				return
			}
			mu.Lock()
			out = append(out, ex)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// votar junta n extracoes: campos simples da primeira; passageiros e
// correcoes so os que aparecem na maioria (com 1 extracao, todos).
func votar(res []Extracao) *Extracao {
	ex := res[0]
	ex.Votos = len(res)
	if len(res) == 1 {
		return &ex
	}
	maioria := len(res)/2 + 1
	chaveP := func(p PassageiroExtr) string {
		return semAcento(strings.ToLower(strings.Join(strings.Fields(p.Nome), " "))) + "|" + soDigitos(p.Documento)
	}
	cont := map[string]int{}
	primeiro := map[string]PassageiroExtr{}
	var ordem []string
	for _, r := range res {
		visto := map[string]bool{}
		for _, p := range r.Passageiros {
			k := chaveP(p)
			if visto[k] {
				continue
			}
			visto[k] = true
			if cont[k] == 0 {
				primeiro[k] = p
				ordem = append(ordem, k)
			}
			cont[k]++
		}
	}
	ex.Passageiros = nil
	for _, k := range ordem {
		if cont[k] >= maioria {
			ex.Passageiros = append(ex.Passageiros, primeiro[k])
		}
	}
	// Pagamento: so vale se a maioria concordar.
	votosPag := map[string]int{}
	for _, r := range res {
		votosPag[r.Pagamento]++
	}
	if votosPag[ex.Pagamento] < maioria {
		ex.Pagamento = PagamentoNenhum
	}
	return &ex
}

// contextoExtrator: conversa recente, situacao resumida e as mensagens novas.
func contextoExtrator(e conversa.Estado, hist []conversa.Mensagem) string {
	var b strings.Builder
	ini := max(len(hist)-8, 0)
	novas := 0
	for i := len(hist) - 1; i >= 0 && hist[i].Autor == conversa.AutorCliente; i-- {
		novas++
	}
	b.WriteString("CONVERSA RECENTE:\n")
	for i := ini; i < len(hist)-novas; i++ {
		m := hist[i]
		quem := "Atendente"
		if m.Autor == conversa.AutorCliente {
			quem = "Cliente"
		}
		fmt.Fprintf(&b, "%s: %s\n", quem, truncarRunas(strings.TrimSpace(m.Texto), 400))
	}
	b.WriteString("\nSITUAÇÃO:\n")
	b.WriteString(e.Resumo())
	if len(e.Opcoes) > 0 && len(e.Trechos) == 0 {
		b.WriteString("\nOpções mostradas:")
		for _, o := range e.Opcoes {
			d, err := time.Parse("2006-01-02", o.Data)
			dia := o.Data
			if err == nil {
				dia = diasSemanaCurto[d.Weekday()] + " " + d.Format("02/01")
			}
			fmt.Fprintf(&b, "\n%d. %s → %s, %s às %s, R$ %.0f", o.Numero, o.Origem, o.Destino, dia, o.Horario, o.Preco)
		}
	}
	b.WriteString("\n\nMENSAGENS NOVAS DO CLIENTE:\n")
	for i := len(hist) - novas; i < len(hist); i++ {
		b.WriteString(strings.TrimSpace(hist[i].Texto))
		b.WriteString("\n")
	}
	return b.String()
}

func truncarRunas(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---- reconciliacao: Extracao -> Rota e passageiros ----

var (
	reFormaPagamento = regexp.MustCompile(`(?i)\b(sinal|entrada|integral|inteiro|tudo|total|completo|à vista|a vista)\b`)
	reSinalEscolha   = regexp.MustCompile(`(?i)\b(\d{1,2}|primeir[ao]|segund[ao]|terceir[ao]|quart[ao]|quint[ao]|sext[ao]|[uú]ltim[ao]|essa|esta|dessa|desta|dia \d{1,2}|\d{1,2}/\d{1,2}|a de|a do|mais cedo|mais barat[ao]|segunda|quinta)\b`)
)

// enriquecerRota completa a Rota do Jev com o extrator, so onde o Jev nao
// teve certeza e o texto do cliente sustenta a extracao.
func enriquecerRota(rt Rota, ex *Extracao, e conversa.Estado, texto string, cidades []ferramentas.Cidade) (Rota, []string) {
	if ex == nil {
		return rt, nil
	}
	var usados []string
	// Pagamento: so com a palavra da forma no texto.
	if rt.ConfPagamento < limiarFechar && (ex.Pagamento == PagamentoIntegral || ex.Pagamento == PagamentoSinal) && reFormaPagamento.MatchString(texto) {
		rt.Pagamento, rt.ConfPagamento = ex.Pagamento, 0.95
		usados = append(usados, "pagamento")
	}
	// Escolha de opcao: numero existente + sinal de escolha no texto.
	if ex.Opcao > 0 && rt.PedeVolta < 0.5 && !(rt.Intencao == IntencaoEscolherOpcao && rt.ConfIntencao >= limiarOpcao && rt.ConfOpcao >= limiarOpcao) && reSinalEscolha.MatchString(texto) {
		for _, o := range e.Opcoes {
			if o.Numero == ex.Opcao {
				rt.Intencao, rt.ConfIntencao, rt.Opcao, rt.ConfOpcao = IntencaoEscolherOpcao, 0.95, fmt.Sprint(ex.Opcao), 0.95
				usados = append(usados, "opcao")
				break
			}
		}
	}
	// Quantidade.
	if ex.Pessoas > 0 && ex.Pessoas <= 9 && (rt.ConfAdultos < limiarQuantidade || rt.ConfCriancas < limiarQuantidade) {
		cr := min(max(ex.CriancasAte5, 0), ex.Pessoas-1)
		rt.Adultos, rt.ConfAdultos = fmt.Sprint(ex.Pessoas-cr), 0.95
		rt.Criancas, rt.ConfCriancas = fmt.Sprint(cr), 0.95
		usados = append(usados, "quantidade")
	}
	// Cidades: so nome que bate com uma cidade atendida.
	if rt.ConfOrigem < 0.8 || rt.Origem == "" || rt.Origem == CidadeNaoInformada {
		if c := cidadeAtendida(ex.Origem, cidades); c != "" {
			rt.Origem, rt.ConfOrigem = c, 0.9
			usados = append(usados, "origem")
		}
	}
	if rt.ConfDestino < 0.8 || rt.Destino == "" || rt.Destino == CidadeNaoInformada {
		if c := cidadeAtendida(ex.Destino, cidades); c != "" {
			rt.Destino, rt.ConfDestino = c, 0.9
			usados = append(usados, "destino")
		}
	}
	if ex.Confirma == "sim" && rt.Confirma < 0.5 {
		rt.Confirma = 0.9
		usados = append(usados, "confirma")
	}
	if ex.Confirma == "nao" && rt.Nega < 0.5 {
		rt.Nega = 0.9
		usados = append(usados, "nega")
	}
	return rt, usados
}

func cidadeAtendida(nome string, cidades []ferramentas.Cidade) string {
	n := semAcento(strings.ToLower(strings.TrimSpace(nome)))
	if n == "" {
		return ""
	}
	for _, c := range cidades {
		if semAcento(strings.ToLower(c.Nome)) == n {
			return c.Nome
		}
	}
	return ""
}

// passageirosExtraidos converte os passageiros do extrator, so os com dado
// suficiente: CPF valido, RG/CNH, ou crianca ate 5 anos (so nome).
func passageirosExtraidos(ex *Extracao) []conversa.Passageiro {
	if ex == nil {
		return nil
	}
	var out []conversa.Passageiro
	for _, p := range ex.Passageiros {
		nome := limparNome(p.Nome)
		if nome == "" {
			continue
		}
		doc := soDigitos(p.Documento)
		tipo := strings.ToUpper(p.TipoDocumento)
		switch {
		case doc != "" && (tipo == "CPF" || tipo == "") && ferramentas.ValidarCPF(doc):
			out = append(out, conversa.Passageiro{Nome: nome, Documento: doc, TipoDocumento: "CPF", CriancaAte5: p.Idade > 0 && p.Idade <= 5})
		case doc != "" && (tipo == "RG" || tipo == "CNH"):
			out = append(out, conversa.Passageiro{Nome: nome, Documento: normDoc(p.Documento), TipoDocumento: tipo, CriancaAte5: p.Idade > 0 && p.Idade <= 5})
		case doc == "" && p.Idade > 0 && p.Idade <= 5:
			out = append(out, conversa.Passageiro{Nome: nome, CriancaAte5: true})
		}
	}
	return out
}
