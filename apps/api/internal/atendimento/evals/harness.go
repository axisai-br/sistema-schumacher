package evals

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/agente"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

//go:embed casos/*.json
var casosFS embed.FS

// Espera e o estado final esperado. Campos ponteiro so sao checados quando
// presentes no JSON.
type Espera struct {
	// Status: "BOT", "HUMANO" ou "BOT|HUMANO" (qualquer um dos dois serve).
	Status        string `json:"status"`
	ReservaCriada *bool  `json:"reserva_criada,omitempty"`
	PixGerado     *bool  `json:"pix_gerado,omitempty"`
	// Reservas, Pix e Trechos: quantidades exatas (uma reserva e um PIX por trecho).
	Reservas                 *int   `json:"reservas,omitempty"`
	Pix                      *int   `json:"pix,omitempty"`
	Trechos                  *int   `json:"trechos,omitempty"`
	Destino                  string `json:"destino,omitempty"`
	Origem                   string `json:"origem,omitempty"`
	Pagantes                 *int   `json:"pagantes,omitempty"`
	Criancas                 *int   `json:"criancas,omitempty"`
	MinOpcoesMostradas       *int   `json:"min_opcoes_mostradas,omitempty"`
	MaxRespostasPrimeiroTurn *int   `json:"max_respostas_primeiro_turno,omitempty"`
}

// Caso e um cenario de avaliacao (um arquivo em casos/).
type Caso struct {
	Nome            string `json:"nome"`
	Descricao       string `json:"descricao"`
	ObjetivoCliente string `json:"objetivo_cliente"`
	PrimeiraMsg     string `json:"primeira_mensagem"`
	// MensagensSeguidas sao enviadas logo depois da primeira, antes do
	// primeiro Processar (cliente que manda duas mensagens seguidas).
	MensagensSeguidas []string `json:"mensagens_seguidas,omitempty"`
	MaxTurnos         int      `json:"max_turnos"`
	Espera            Espera   `json:"espera"`
	// Proibido: regex que nao pode casar em nenhuma mensagem do bot.
	Proibido []string `json:"proibido,omitempty"`
	// ExigeNoBot: cada regex precisa casar em alguma mensagem do bot.
	ExigeNoBot []string `json:"exige_no_bot,omitempty"`
	// ProibidoPrimeiraResposta / ExigePrimeiraResposta valem so para o texto
	// enviado no primeiro turno.
	ProibidoPrimeiraResposta []string `json:"proibido_primeira_resposta,omitempty"`
	ExigePrimeiraResposta    []string `json:"exige_primeira_resposta,omitempty"`
	// Critico: pass^K precisa ser 1 (handoff e seguranca).
	Critico bool `json:"critico,omitempty"`
	// Grupo "reserva" entra na taxa agregada de casos de reserva (>= 0,9).
	Grupo string `json:"grupo,omitempty"`
	// LLMFora roda o caso com um modelo que sempre falha (sem rede).
	LLMFora bool `json:"llm_fora,omitempty"`
}

// CarregarCasos le e valida todos os JSON embutidos em casos/.
func CarregarCasos() ([]Caso, error) {
	ents, err := casosFS.ReadDir("casos")
	if err != nil {
		return nil, err
	}
	var out []Caso
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := casosFS.ReadFile("casos/" + e.Name())
		if err != nil {
			return nil, err
		}
		var c Caso
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if err := c.Validar(); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if want := strings.TrimSuffix(e.Name(), ".json"); c.Nome != want {
			return nil, fmt.Errorf("%s: nome %q deve ser igual ao nome do arquivo", e.Name(), c.Nome)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nome < out[j].Nome })
	return out, nil
}

// Validar confere campos obrigatorios e compila as regex.
func (c Caso) Validar() error {
	var faltam []string
	for k, v := range map[string]string{
		"nome": c.Nome, "descricao": c.Descricao, "objetivo_cliente": c.ObjetivoCliente,
		"primeira_mensagem": c.PrimeiraMsg, "espera.status": c.Espera.Status,
	} {
		if strings.TrimSpace(v) == "" {
			faltam = append(faltam, k)
		}
	}
	if c.MaxTurnos <= 0 {
		faltam = append(faltam, "max_turnos")
	}
	if len(faltam) > 0 {
		sort.Strings(faltam)
		return fmt.Errorf("campos obrigatorios ausentes: %s", strings.Join(faltam, ", "))
	}
	for _, s := range strings.Split(c.Espera.Status, "|") {
		if s != "BOT" && s != "HUMANO" {
			return fmt.Errorf("espera.status invalido: %q", c.Espera.Status)
		}
	}
	for nome, lista := range map[string][]string{
		"proibido": c.Proibido, "exige_no_bot": c.ExigeNoBot,
		"proibido_primeira_resposta": c.ProibidoPrimeiraResposta, "exige_primeira_resposta": c.ExigePrimeiraResposta,
	} {
		for _, r := range lista {
			if _, err := regexp.Compile(r); err != nil {
				return fmt.Errorf("%s: regex invalida %q: %w", nome, r, err)
			}
		}
	}
	return nil
}

// ---- ambiente ----

// ConfigAmbiente parametriza o Ambiente.
type ConfigAmbiente struct {
	ModeloNome string // nome do modelo do agente (vai no pedido ao llm.Modelo)
	OrcamentoTurno time.Duration // 0 usa o padrao do agente
	Juiz       agente.Juiz
	Roteador   agente.Roteador  // opcional; substitui o Juiz
	Agora      func() time.Time // padrao time.Now
	// SinalPorPagante e o valor do sinal por pagante (padrao das ferramentas: 250).
	SinalPorPagante float64
}

// Ambiente reune store em memoria, canal fake, fakes de dominio, ferramentas
// reais e o agente real.
type Ambiente struct {
	Fx         *Fixtures
	Store      *conversa.StoreMem
	Canal      *CanalFake
	Reservas   *ReservasFake
	Pagamentos *PagamentosFake
	Catalogo   *ferramentas.Catalogo
	Agente     *agente.Agente
	Agora      func() time.Time
	// NomeCliente e o nome enviado nas entradas do cliente (padrao "Cliente Eval").
	NomeCliente string
	seqLocal    int
}

// fusoSP devolve America/Sao_Paulo (com reserva fixa em UTC-3).
func fusoSP() *time.Location {
	if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return loc
	}
	return time.FixedZone("BRT", -3*3600)
}

// NovoAmbiente monta o agente real sobre os fakes, com o modelo dado.
func NovoAmbiente(modelo llm.Modelo, cfg ConfigAmbiente) *Ambiente {
	agora := cfg.Agora
	if agora == nil {
		agora = time.Now
	}
	fx := NovasFixtures(agora().In(fusoSP()))
	cat := ferramentas.NovoCatalogo(fx.FonteCatalogo(), agora)
	reservas, pagamentos := fx.Reservas(), NovosPagamentos()
	reg := ferramentas.Padrao(cat, fx.Buscador(), fx.Cotador(), reservas, pagamentos, ferramentas.Config{SinalPorPagante: cfg.SinalPorPagante})
	store := conversa.NewStoreMem(agora)
	canalFake := &CanalFake{}
	nome := cfg.ModeloNome
	if nome == "" {
		nome = "modelo-eval"
	}
	ag := agente.Novo(agente.Deps{
		Store: store, Canal: canalFake, Modelo: modelo, Ferramentas: reg,
		Catalogo: cat, Juiz: cfg.Juiz, Roteador: cfg.Roteador, Cidades: cat, Agora: agora,
	}, agente.Config{Modelo: nome, OrcamentoTurno: cfg.OrcamentoTurno})
	return &Ambiente{
		Fx: fx, Store: store, Canal: canalFake, Reservas: reservas, Pagamentos: pagamentos,
		Catalogo: cat, Agente: ag, Agora: agora,
	}
}

// ValorPermitido diz se um valor em reais pode aparecer numa mensagem do bot:
// precos de tabela (e multiplos ate 10 pessoas), sinais de R$ 250 por pagante
// (e o restante), e os valores de reservas e pagamentos realmente criados.
func (a *Ambiente) ValorPermitido(v float64) bool {
	eq := func(x float64) bool { return abs(x-v) < 0.005 }
	for _, p := range a.Fx.Precos() {
		for k := 1; k <= 10; k++ {
			if eq(p*float64(k)) || eq(250*float64(k)) || eq(p*float64(k)-250*float64(k)) {
				return true
			}
		}
	}
	for _, r := range a.Reservas.Reservas() {
		b := r.Booking
		if eq(b.TotalAmount) || eq(b.DepositAmount) || eq(b.RemainderAmount) {
			return true
		}
	}
	for _, p := range a.Pagamentos.Pagamentos() {
		if eq(p.Amount) {
			return true
		}
	}
	return false
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ---- cliente simulado ----

// Msg e uma mensagem da transcricao.
type Msg struct {
	Autor string // "CLIENTE" ou "BOT"
	Texto string
}

// Cliente gera a proxima mensagem do cliente simulado. Devolve "[FIM]" (ou um
// texto que o contenha) quando o objetivo foi atingido ou nao ha o que fazer.
type Cliente interface {
	Proxima(ctx context.Context, objetivo string, historico []Msg) (string, error)
}

// Marcador de fim de conversa do cliente simulado.
const MarcadorFim = "[FIM]"

const instrucoesCliente = `Você está simulando um CLIENTE real conversando por WhatsApp com o atendimento de uma empresa de ônibus (Schumacher Tur, viagens entre Maranhão e Santa Catarina).

Regras:
- Escreva SOMENTE a próxima mensagem do cliente, em português do Brasil, como no WhatsApp: curta (1 a 2 frases), informal, com erros leves de digitação e pouca pontuação. Nunca use aspas, markdown, listas nem explique o que está fazendo.
- Responda só o que o atendente perguntou, uma coisa por vez. Não entregue dados que não foram pedidos. Se perguntarem algo que você não sabe, diga que não sabe.
- Use EXATAMENTE os nomes, documentos e dados que estão no seu objetivo; nunca invente dados pessoais novos.
- Nunca diga que é uma IA nem que isto é um teste.
- Quando o seu objetivo foi atingido, ou o atendente transferiu para uma pessoa, ou não há mais o que fazer, responda exatamente: ` + MarcadorFim + `

O SEU OBJETIVO:
`

// ClienteLLM e o cliente simulado por um llm.Modelo.
type ClienteLLM struct {
	Modelo     llm.Modelo
	ModeloNome string
}

func (c *ClienteLLM) Proxima(ctx context.Context, objetivo string, hist []Msg) (string, error) {
	// Da perspectiva do cliente simulado, as falas do cliente sao "assistant"
	// e as do bot sao "user".
	var msgs []llm.Mensagem
	for _, m := range hist {
		papel := llm.PapelUsuario
		if m.Autor == "CLIENTE" {
			papel = llm.PapelAssistente
		}
		if n := len(msgs); n > 0 && msgs[n-1].Papel == papel {
			msgs[n-1].Texto += "\n" + m.Texto
			continue
		}
		msgs = append(msgs, llm.Mensagem{Papel: papel, Texto: m.Texto})
	}
	if len(msgs) == 0 || msgs[len(msgs)-1].Papel != llm.PapelUsuario {
		msgs = append(msgs, llm.Mensagem{Papel: llm.PapelUsuario, Texto: "(escreva a sua próxima mensagem)"})
	}
	resp, err := c.Modelo.Gerar(ctx, llm.Pedido{
		Modelo: c.ModeloNome, Instrucoes: instrucoesCliente + objetivo, Mensagens: msgs, MaxTokens: 200,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Texto), nil
}

// ClienteRoteiro responde com mensagens fixas, em ordem, e depois [FIM]. Serve
// para testes offline do proprio harness.
type ClienteRoteiro struct {
	Falas []string
	i     int
}

func (c *ClienteRoteiro) Proxima(context.Context, string, []Msg) (string, error) {
	if c.i >= len(c.Falas) {
		return MarcadorFim, nil
	}
	c.i++
	return c.Falas[c.i-1], nil
}

// ModeloQueFalha e um llm.Modelo que sempre devolve erro (LLM fora do ar).
type ModeloQueFalha struct{ Err error }

func (m ModeloQueFalha) Gerar(context.Context, llm.Pedido) (llm.Resposta, error) {
	if m.Err != nil {
		return llm.Resposta{}, m.Err
	}
	return llm.Resposta{}, errors.New("llm fora do ar (simulado)")
}

// ---- execucao de um caso ----

// Execucao e o resultado de uma rodada de um caso.
type Execucao struct {
	Caso        string
	Transcricao []Msg
	Status      conversa.Status
	Estado      conversa.Estado
	Turnos      int
	Primeira    []string // mensagens do bot no primeiro turno
	MaxOpcoes   int
	Amb         *Ambiente
	Erro        error    // erro de infraestrutura (nao e falha do agente)
	Falhas      []string // regras violadas
}

// OK diz se a execucao passou em todas as checagens.
func (e *Execucao) OK() bool { return e.Erro == nil && len(e.Falhas) == 0 }

const (
	contatoEval  = "5549991000001@s.whatsapp.net"
	telefoneEval = "5549991000001"
)

func (a *Ambiente) registrarCliente(ctx context.Context, n int, texto string) error {
	tipo := conversa.TipoTexto
	if strings.HasPrefix(texto, "[áudio") {
		tipo = conversa.TipoAudio
	}
	_, _, _, err := a.Store.RegistrarEntrada(ctx, conversa.NovaEntrada{
		Canal: "WHATSAPP", Contato: contatoEval, Telefone: telefoneEval, Nome: a.nomeCliente(),
		Autor: conversa.AutorCliente, Tipo: tipo, Texto: texto,
		ProvedorID: fmt.Sprintf("eval-in-%d", n), RecebidaEm: a.Agora(),
	})
	return err
}

func (a *Ambiente) nomeCliente() string {
	if a.NomeCliente != "" {
		return a.NomeCliente
	}
	return "Cliente Eval"
}

// EnviarDoCliente registra uma mensagem do cliente como entrada pendente, sem
// processar (uso interativo: o chamador decide quando rodar o agente).
func (a *Ambiente) EnviarDoCliente(ctx context.Context, texto string) error {
	a.seqLocal++
	return a.registrarCliente(ctx, 1_000_000+a.seqLocal, texto)
}

// ConversaAtual devolve a (unica) conversa do ambiente; erro se ainda nao
// houve nenhuma entrada.
func (a *Ambiente) ConversaAtual(ctx context.Context) (conversa.Conversa, error) {
	return a.lerConversa(ctx, "")
}

// Executar roda um caso: o cliente simulado conversa com o agente real ate o
// fim do objetivo, transferencia para humano ou max_turnos; depois aplica as
// checagens.
func Executar(ctx context.Context, c Caso, amb *Ambiente, cliente Cliente) *Execucao {
	ex := &Execucao{Caso: c.Nome, Amb: amb}
	entradas := 0
	enviar := func(texto string) error {
		entradas++
		ex.Transcricao = append(ex.Transcricao, Msg{"CLIENTE", texto})
		return amb.registrarCliente(ctx, entradas, texto)
	}
	if err := enviar(c.PrimeiraMsg); err != nil {
		ex.Erro = err
		return ex
	}
	for _, m := range c.MensagensSeguidas {
		if err := enviar(m); err != nil {
			ex.Erro = err
			return ex
		}
	}

	var convID string
	for turno := 1; turno <= c.MaxTurnos; turno++ {
		ex.Turnos = turno
		conv, err := amb.lerConversa(ctx, convID)
		if err != nil {
			ex.Erro = err
			return ex
		}
		convID = conv.ID
		antes := amb.Canal.Total()
		if err := amb.Agente.Processar(ctx, conv); err != nil {
			ex.Erro = fmt.Errorf("turno %d: agente.Processar: %w", turno, err)
			return ex
		}
		resp := amb.Canal.Textos(antes)
		if turno == 1 {
			ex.Primeira = resp
		}
		for _, t := range resp {
			ex.Transcricao = append(ex.Transcricao, Msg{"BOT", t})
		}
		conv, err = amb.Store.Obter(ctx, convID)
		if err != nil {
			ex.Erro = err
			return ex
		}
		ex.Status, ex.Estado = conv.Status, conv.Estado
		if n := len(conv.Estado.Opcoes); n > ex.MaxOpcoes {
			ex.MaxOpcoes = n
		}
		if len(resp) == 0 {
			ex.Falhas = append(ex.Falhas, fmt.Sprintf("turno %d: o bot não respondeu (silêncio)", turno))
			break
		}
		if conv.Status == conversa.StatusHumano {
			break
		}
		if turno == c.MaxTurnos {
			break
		}
		prox, err := cliente.Proxima(ctx, c.ObjetivoCliente, ex.Transcricao)
		if err != nil {
			ex.Erro = fmt.Errorf("cliente simulado: %w", err)
			return ex
		}
		if prox == "" || strings.Contains(prox, MarcadorFim) {
			break
		}
		if err := enviar(prox); err != nil {
			ex.Erro = err
			return ex
		}
	}
	if ex.Erro == nil {
		ex.Falhas = append(ex.Falhas, Avaliar(c, ex)...)
	}
	return ex
}

func (a *Ambiente) lerConversa(ctx context.Context, id string) (conversa.Conversa, error) {
	if id != "" {
		return a.Store.Obter(ctx, id)
	}
	lista, err := a.Store.Listar(ctx, conversa.FiltroLista{Limite: 1})
	if err != nil {
		return conversa.Conversa{}, err
	}
	if len(lista) == 0 {
		return conversa.Conversa{}, errors.New("conversa não criada")
	}
	return a.Store.Obter(ctx, lista[0].ID)
}

// ---- checagens ----

// TextoNorm: minusculas, sem acento, pontuacao vira espaco, espacos colapsados.
func TextoNorm(s string) string {
	s = semAcentos(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// casa aplica a regex ao texto original e ao texto minusculo sem acentos
// (com pontuacao preservada), para que os casos possam ser escritos sem acento.
func casa(re *regexp.Regexp, texto string) bool {
	return re.MatchString(texto) || re.MatchString(semAcentos(texto))
}

var reReais = regexp.MustCompile(`R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{1,2})?|[0-9]+(?:,[0-9]{1,2})?)`)

// ValoresReais extrai os valores "R$ ..." de um texto.
func ValoresReais(texto string) []float64 {
	var out []float64
	for _, m := range reReais.FindAllStringSubmatch(texto, -1) {
		s := strings.ReplaceAll(m[1], ".", "")
		s = strings.ReplaceAll(s, ",", ".")
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// Avaliar aplica `espera`, `proibido`, `exige_no_bot` e as regras globais.
func Avaliar(c Caso, ex *Execucao) []string {
	var f []string
	var bot []string
	for _, m := range ex.Transcricao {
		if m.Autor == "BOT" {
			bot = append(bot, m.Texto)
		}
	}
	e := c.Espera

	// Estado final.
	statusOK := false
	for _, s := range strings.Split(e.Status, "|") {
		if string(ex.Status) == s {
			statusOK = true
		}
	}
	if !statusOK {
		f = append(f, fmt.Sprintf("status final %s, esperado %s", ex.Status, e.Status))
	}
	reserva := len(ex.Amb.Reservas.Reservas()) > 0
	pix := len(ex.Amb.Pagamentos.Pagamentos()) > 0
	if e.ReservaCriada != nil && reserva != *e.ReservaCriada {
		f = append(f, fmt.Sprintf("reserva criada = %v, esperado %v", reserva, *e.ReservaCriada))
	}
	if e.PixGerado != nil && pix != *e.PixGerado {
		f = append(f, fmt.Sprintf("pix gerado = %v, esperado %v", pix, *e.PixGerado))
	}
	if e.Reservas != nil && len(ex.Amb.Reservas.Reservas()) != *e.Reservas {
		f = append(f, fmt.Sprintf("reservas = %d, esperado %d", len(ex.Amb.Reservas.Reservas()), *e.Reservas))
	}
	if e.Pix != nil && len(ex.Amb.Pagamentos.Pagamentos()) != *e.Pix {
		f = append(f, fmt.Sprintf("pix = %d, esperado %d", len(ex.Amb.Pagamentos.Pagamentos()), *e.Pix))
	}
	if e.Trechos != nil && len(ex.Estado.Trechos) != *e.Trechos {
		f = append(f, fmt.Sprintf("trechos = %d, esperado %d", len(ex.Estado.Trechos), *e.Trechos))
	}
	if e.Destino != "" && (ex.Estado.Destino == nil || TextoNorm(ex.Estado.Destino.Nome) != TextoNorm(e.Destino)) {
		f = append(f, fmt.Sprintf("destino = %s, esperado %s", nomeParada(ex.Estado.Destino), e.Destino))
	}
	if e.Origem != "" && (ex.Estado.Origem == nil || TextoNorm(ex.Estado.Origem.Nome) != TextoNorm(e.Origem)) {
		f = append(f, fmt.Sprintf("origem = %s, esperada %s", nomeParada(ex.Estado.Origem), e.Origem))
	}
	if e.Pagantes != nil && ex.Estado.Pagantes() != *e.Pagantes {
		f = append(f, fmt.Sprintf("pagantes = %d, esperado %d", ex.Estado.Pagantes(), *e.Pagantes))
	}
	if e.Criancas != nil {
		n := 0
		for _, p := range ex.Estado.Passageiros {
			if p.CriancaAte5 {
				n++
			}
		}
		if n != *e.Criancas {
			f = append(f, fmt.Sprintf("crianças = %d, esperado %d", n, *e.Criancas))
		}
	}
	if e.MinOpcoesMostradas != nil && ex.MaxOpcoes < *e.MinOpcoesMostradas {
		f = append(f, fmt.Sprintf("opções mostradas = %d, mínimo %d", ex.MaxOpcoes, *e.MinOpcoesMostradas))
	}
	if e.MaxRespostasPrimeiroTurn != nil && len(ex.Primeira) > *e.MaxRespostasPrimeiroTurn {
		f = append(f, fmt.Sprintf("%d mensagens no primeiro turno, máximo %d", len(ex.Primeira), *e.MaxRespostasPrimeiroTurn))
	}

	// Regex.
	for _, r := range c.Proibido {
		re := regexp.MustCompile(r)
		for _, t := range bot {
			if casa(re, t) {
				f = append(f, fmt.Sprintf("proibido /%s/ casou em: %q", r, resumo(t)))
				break
			}
		}
	}
	for _, r := range c.ExigeNoBot {
		re := regexp.MustCompile(r)
		achou := false
		for _, t := range bot {
			if casa(re, t) {
				achou = true
				break
			}
		}
		if !achou {
			f = append(f, fmt.Sprintf("exige_no_bot /%s/ não casou em nenhuma mensagem do bot", r))
		}
	}
	primeira := strings.Join(ex.Primeira, "\n")
	for _, r := range c.ProibidoPrimeiraResposta {
		if casa(regexp.MustCompile(r), primeira) {
			f = append(f, fmt.Sprintf("proibido na primeira resposta /%s/ casou em: %q", r, resumo(primeira)))
		}
	}
	for _, r := range c.ExigePrimeiraResposta {
		if !casa(regexp.MustCompile(r), primeira) {
			f = append(f, fmt.Sprintf("exige_primeira_resposta /%s/ não casou em: %q", r, resumo(primeira)))
		}
	}

	// Regras globais.
	f = append(f, RegrasGlobais(bot, ex.Amb.ValorPermitido)...)
	return f
}

// RegrasGlobais: o bot nunca manda 3 mensagens seguidas iguais (normalizadas) e
// nunca cita valor em R$ fora do permitido.
func RegrasGlobais(bot []string, valorPermitido func(float64) bool) []string {
	var f []string
	rep := 1
	for i := 1; i < len(bot); i++ {
		if TextoNorm(bot[i]) != "" && TextoNorm(bot[i]) == TextoNorm(bot[i-1]) {
			rep++
			if rep == 3 {
				f = append(f, fmt.Sprintf("3 mensagens seguidas iguais do bot: %q", resumo(bot[i])))
			}
		} else {
			rep = 1
		}
	}
	for _, t := range bot {
		for _, v := range ValoresReais(t) {
			if valorPermitido != nil && !valorPermitido(v) {
				f = append(f, fmt.Sprintf("valor R$ %.2f fora das fixtures em: %q", v, resumo(t)))
			}
		}
	}
	return f
}

func resumo(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 140 {
		return string(r[:140]) + "..."
	}
	return s
}

func nomeParada(p *conversa.Parada) string {
	if p == nil {
		return "(nenhum)"
	}
	return p.Nome
}

// FormatarTranscricao gera o texto salvo em saida/<caso>-<n>.txt.
func FormatarTranscricao(ex *Execucao) string {
	var b strings.Builder
	fmt.Fprintf(&b, "caso: %s\nstatus final: %s\nturnos: %d\n", ex.Caso, ex.Status, ex.Turnos)
	if ex.Erro != nil {
		fmt.Fprintf(&b, "ERRO: %v\n", ex.Erro)
	}
	if len(ex.Falhas) == 0 && ex.Erro == nil {
		b.WriteString("resultado: OK\n")
	} else {
		b.WriteString("resultado: FALHOU\n")
		for _, f := range ex.Falhas {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	b.WriteString("\n--- conversa ---\n")
	for _, m := range ex.Transcricao {
		fmt.Fprintf(&b, "[%s] %s\n", m.Autor, m.Texto)
	}
	b.WriteString("\n--- estado final ---\n")
	b.WriteString(ex.Estado.Resumo())
	b.WriteString("\n")
	return b.String()
}
