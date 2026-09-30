package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"schumacher-tur/api/internal/atendimento/agente"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/evals"
	"schumacher-tur/api/internal/atendimento/llm"
)

const (
	rotuloBot     = "Shabas> "
	textoAudio    = "[áudio não compreendido]"
	ajudaComandos = `comandos:
  texto          mensagem do cliente (processa um turno do agente)
  +texto         enfileira a mensagem SEM processar (a próxima linha normal processa todas juntas)
  /estado        estado da reserva (JSON), pendências e status da conversa
  /turno         passos do último turno (ferramentas, checagens, juiz, tokens, latência)
  /reset         nova conversa e fixtures novas
  /bot           se a conversa foi transferida, volta para BOT
  /audio         envia "` + textoAudio + `" como mensagem
  /nome <nome>   define o nome do cliente na conversa
  /reservas      reservas e PIX criados nos fakes
  /salvar        grava a transcrição agora
  /ajuda         mostra esta ajuda
  /sair          encerra (salva a transcrição)`
)

// sessao e uma conversa local: agente real + fakes em memoria.
type sessao struct {
	cfg    config
	modelo llm.Modelo
	out    io.Writer
	agora  func() time.Time
	dirOut string // pasta das transcricoes

	amb      *evals.Ambiente
	nome     string
	transc   []evals.Msg
	nTurnos  int
	ultimo   *conversa.Turno
	iniciada time.Time
	salvo    string
}

func novaSessao(cfg config, modelo llm.Modelo, out io.Writer, agora func() time.Time, dirOut string) *sessao {
	if agora == nil {
		agora = relogioCrescente(time.Now)
	}
	s := &sessao{cfg: cfg, modelo: modelo, out: out, agora: agora, dirOut: dirOut, iniciada: agora()}
	s.novoAmbiente()
	return s
}

// criarJuiz monta o juiz conforme ATENDIMENTO_V2_JUIZ. O segundo retorno e um
// aviso (vazio se nao houver).
func criarJuiz(cfg config, m llm.Modelo) (agente.Juiz, string) {
	switch cfg.Juiz {
	case "off":
		return nil, ""
	case "jev":
		if cfg.TypesafeKey != "" {
			return agente.NovoJuizJev(cfg.TypesafeKey, nil), ""
		}
		return agente.NovoJuizLLM(m, cfg.Modelo), "ATENDIMENTO_V2_JUIZ=jev sem TYPESAFE_API_KEY; usando juiz llm"
	default:
		return agente.NovoJuizLLM(m, cfg.Modelo), ""
	}
}

func (s *sessao) novoAmbiente() {
	juiz, _ := criarJuiz(s.cfg, s.modelo)
	s.amb = evals.NovoAmbiente(s.modelo, evals.ConfigAmbiente{
		ModeloNome: s.cfg.Modelo, Juiz: juiz, Agora: s.agora, SinalPorPagante: s.cfg.Sinal,
	})
	s.amb.NomeCliente = s.nome
	s.nTurnos = 0
	s.ultimo = nil
}

func (s *sessao) printf(f string, a ...any) { fmt.Fprintf(s.out, f, a...) }

func (s *sessao) cabecalho() {
	hoje := s.agora().In(fusoSP())
	s.printf("Simulador local do atendimento v2 (agente real + LLM real; dados de teste em memória)\n")
	s.printf("  LLM: %s · juiz: %s · hoje (fixtures): %s\n", s.cfg.Descricao, s.cfg.Juiz, dataExtenso(hoje))
	s.printf("  Nada é gravado no banco nem enviado ao WhatsApp. /ajuda lista os comandos; /sair encerra.\n\n")
}

var diasSemana = []string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}

func dataExtenso(t time.Time) string {
	return fmt.Sprintf("%s, %s", diasSemana[t.Weekday()], t.Format("02/01/2006"))
}

func fusoSP() *time.Location {
	if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return loc
	}
	return time.FixedZone("BRT", -3*3600)
}

// Linha trata uma linha digitada (ou de roteiro). Devolve true para sair.
func (s *sessao) Linha(ctx context.Context, raw string) bool {
	t := strings.TrimSpace(raw)
	switch {
	case t == "":
		return false
	case strings.HasPrefix(t, "/"):
		return s.comando(ctx, t)
	case strings.HasPrefix(t, "+"):
		if txt := strings.TrimSpace(t[1:]); txt != "" {
			if s.enviar(ctx, txt) {
				s.printf("  [na fila: %d mensagem(ns) aguardando; a próxima linha normal processa todas]\n", s.pendentes(ctx))
			}
		}
		return false
	default:
		if s.enviar(ctx, t) {
			s.processar(ctx)
		}
		return false
	}
}

func (s *sessao) pendentes(ctx context.Context) int {
	conv, err := s.amb.ConversaAtual(ctx)
	if err != nil || conv.PendenteDesde == nil {
		return 0
	}
	e, err := s.amb.Store.EntradasPendentes(ctx, conv.ID, *conv.PendenteDesde)
	if err != nil {
		return 0
	}
	return len(e)
}

func (s *sessao) enviar(ctx context.Context, texto string) bool {
	if err := s.amb.EnviarDoCliente(ctx, texto); err != nil {
		s.printf("  [erro ao registrar a mensagem: %v]\n", err)
		return false
	}
	s.transc = append(s.transc, evals.Msg{Autor: "CLIENTE", Texto: texto})
	return true
}

// processar roda um turno do agente sobre tudo que esta pendente.
func (s *sessao) processar(ctx context.Context) {
	conv, err := s.amb.ConversaAtual(ctx)
	if err != nil {
		s.printf("  [nenhuma conversa ainda]\n")
		return
	}
	if conv.Status == conversa.StatusHumano {
		s.printf("  [conversa com atendente humano: mensagem registrada, mas NÃO processada pelo bot. Use /bot para devolver ao bot]\n")
		return
	}
	antes := s.amb.Canal.Total()
	t0 := time.Now()
	perr := s.amb.Agente.Processar(ctx, conv)
	dur := time.Since(t0)

	for _, txt := range s.amb.Canal.Textos(antes) {
		s.imprimirBot(txt)
		s.transc = append(s.transc, evals.Msg{Autor: "BOT", Texto: txt})
	}
	if perr != nil {
		s.printf("  [erro no turno: %v]\n", perr)
	}
	if turnos, err := s.amb.Store.Turnos(ctx, conv.ID, 0); err == nil && len(turnos) > s.nTurnos {
		s.nTurnos = len(turnos)
		tn := turnos[len(turnos)-1]
		s.ultimo = &tn
		s.printf("  %s\n", infoTurno(tn, dur))
	}
	if atual, err := s.amb.Store.Obter(ctx, conv.ID); err == nil && atual.Status == conversa.StatusHumano {
		motivo := atual.Estado.MotivoHumano
		if motivo == "" {
			motivo = "(não informado)"
		}
		s.printf("*** conversa transferida para atendente humano — motivo: %s ***\n", motivo)
		s.printf("    (novas mensagens serão registradas mas não processadas até você usar /bot)\n")
		s.transc = append(s.transc, evals.Msg{Autor: "SISTEMA", Texto: "conversa transferida para atendente humano — motivo: " + motivo})
	}
}

func (s *sessao) imprimirBot(txt string) {
	pad := strings.Repeat(" ", len(rotuloBot))
	for i, l := range strings.Split(strings.TrimRight(txt, "\n"), "\n") {
		if i == 0 {
			s.printf("%s%s\n", rotuloBot, l)
		} else {
			s.printf("%s%s\n", pad, l)
		}
	}
}

func fmtTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk tokens", float64(n)/1000)
	}
	return fmt.Sprintf("%d tokens", n)
}

func fmtDur(d time.Duration) string { return fmt.Sprintf("%.1fs", d.Seconds()) }

func ferramentasDoTurno(t conversa.Turno) []string {
	var out []string
	for _, p := range t.Passos {
		if p.Tipo == "ferramenta" {
			out = append(out, p.Nome)
		}
	}
	return out
}

func infoTurno(t conversa.Turno, dur time.Duration) string {
	f := "nenhuma"
	if l := ferramentasDoTurno(t); len(l) > 0 {
		f = strings.Join(l, ", ")
	}
	return fmt.Sprintf("[ferramentas: %s · %s · %s]", f, fmtDur(dur), fmtTokens(t.TokensEntrada+t.TokensSaida))
}

func (s *sessao) comando(ctx context.Context, t string) bool {
	partes := strings.Fields(t)
	cmd := strings.ToLower(partes[0])
	arg := strings.TrimSpace(strings.TrimPrefix(t, partes[0]))
	switch cmd {
	case "/sair", "/exit", "/quit":
		return true
	case "/ajuda", "/help", "/?":
		s.printf("%s\n", ajudaComandos)
	case "/estado":
		s.cmdEstado(ctx)
	case "/turno":
		s.cmdTurno()
	case "/reset":
		s.transc = append(s.transc, evals.Msg{Autor: "SISTEMA", Texto: "/reset: nova conversa e fixtures novas"})
		s.novoAmbiente()
		s.printf("  [nova conversa e fixtures novas]\n")
	case "/bot":
		conv, err := s.amb.ConversaAtual(ctx)
		switch {
		case err != nil:
			s.printf("  [nenhuma conversa ainda]\n")
		case conv.Status == conversa.StatusBot:
			s.printf("  [a conversa já está com o bot]\n")
		default:
			if _, err := s.amb.Store.MudarStatus(ctx, conv.ID, conversa.StatusBot, "", ""); err != nil {
				s.printf("  [erro: %v]\n", err)
			} else {
				s.transc = append(s.transc, evals.Msg{Autor: "SISTEMA", Texto: "/bot: conversa devolvida ao bot"})
				s.printf("  [conversa devolvida ao bot]\n")
			}
		}
	case "/audio":
		if s.enviar(ctx, textoAudio) {
			s.processar(ctx)
		}
	case "/nome":
		if arg == "" {
			s.printf("  uso: /nome <nome>  (atual: %q)\n", s.nome)
			break
		}
		s.nome = arg
		s.amb.NomeCliente = arg
		s.printf("  [nome do cliente: %s (vale a partir da próxima mensagem)]\n", arg)
	case "/reservas":
		s.cmdReservas()
	case "/salvar":
		if p, err := s.salvar(); err != nil {
			s.printf("  [erro ao salvar: %v]\n", err)
		} else {
			s.printf("  [transcrição salva em %s]\n", p)
		}
	default:
		s.printf("  comando desconhecido: %s (veja /ajuda)\n", cmd)
	}
	return false
}

func (s *sessao) cmdEstado(ctx context.Context) {
	conv, err := s.amb.ConversaAtual(ctx)
	if err != nil {
		s.printf("  [nenhuma mensagem ainda]\n")
		return
	}
	b, _ := json.MarshalIndent(conv.Estado, "", "  ")
	s.printf("%s\n", b)
	s.printf("Pendências:\n")
	if p := conv.Estado.Pendencias(); len(p) == 0 {
		s.printf("  (nenhuma)\n")
	} else {
		for _, x := range p {
			s.printf("  - %s\n", x)
		}
	}
	s.printf("Status da conversa: %s\n", conv.Status)
}

func truncar(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func jsonCurto(v any, n int) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return truncar(x, n)
	case json.RawMessage:
		return truncar(string(x), n)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return truncar(string(b), n)
}

func (s *sessao) cmdTurno() {
	t := s.ultimo
	if t == nil {
		s.printf("  [ainda não houve turno]\n")
		return
	}
	s.printf("Turno %s — resultado: %s · modelo: %s · %d passos\n", t.ID, t.Resultado, t.Modelo, len(t.Passos))
	s.printf("  tokens: %d entrada + %d saída · latência: %dms\n", t.TokensEntrada, t.TokensSaida, t.LatenciaMS)
	if t.Erro != "" {
		s.printf("  erro: %s\n", t.Erro)
	}
	for i, p := range t.Passos {
		s.printf("  %d. [%s] %s (%dms)\n", i+1, p.Tipo, p.Nome, p.DuracaoMS)
		if e := jsonCurto(p.Entrada, 400); e != "" {
			s.printf("       entrada: %s\n", e)
		}
		if o := jsonCurto(p.Saida, 400); o != "" {
			rot := "saída"
			if p.Tipo == "ferramenta" {
				rot = "resultado"
			}
			s.printf("       %s: %s\n", rot, o)
		}
		if p.Erro != "" {
			s.printf("       erro: %s\n", p.Erro)
		}
	}
	if t.Resposta != "" {
		s.printf("  resposta: %s\n", truncar(t.Resposta, 300))
	}
}

func (s *sessao) cmdReservas() {
	res := s.amb.Reservas.Reservas()
	pags := s.amb.Pagamentos.Pagamentos()
	if len(res) == 0 && len(pags) == 0 {
		s.printf("  [nenhuma reserva nem PIX criados]\n")
		return
	}
	for i, r := range res {
		b := r.Booking
		s.printf("Reserva %d: código %s · id %s · status %s · viagem %s\n", i+1, b.ReservationCode, b.ID, b.Status, b.TripID)
		s.printf("  total R$ %.2f · sinal R$ %.2f · restante R$ %.2f\n", b.TotalAmount, b.DepositAmount, b.RemainderAmount)
		for j, p := range r.Passengers {
			s.printf("  passageiro %d: %s (%s)\n", j+1, p.Name, p.DocumentType)
		}
	}
	for i, p := range pags {
		s.printf("PIX %d: id %s · reserva %s · R$ %.2f · %s\n", i+1, p.ID, p.BookingID, p.Amount, p.Status)
		s.printf("  copia-e-cola (falso): %s\n", s.amb.Pagamentos.CodigoPix(p.ID))
	}
}

// texto monta a transcricao da sessao.
func (s *sessao) texto() string {
	var b strings.Builder
	fmt.Fprintf(&b, "simulador local do atendimento v2\niniciado em: %s\nLLM: %s\njuiz: %s\n", s.iniciada.Format("2006-01-02 15:04:05"), s.cfg.Descricao, s.cfg.Juiz)
	b.WriteString("\n--- conversa ---\n")
	for _, m := range s.transc {
		fmt.Fprintf(&b, "[%s] %s\n", m.Autor, m.Texto)
	}
	if conv, err := s.amb.ConversaAtual(context.Background()); err == nil {
		fmt.Fprintf(&b, "\n--- estado final (status %s) ---\n%s\n", conv.Status, conv.Estado.Resumo())
	}
	return b.String()
}

// salvar grava a transcricao (mesmo arquivo durante toda a sessao).
func (s *sessao) salvar() (string, error) {
	if s.salvo == "" {
		s.salvo = filepath.Join(s.dirOut, "local-"+s.iniciada.Format("20060102-150405")+".txt")
	}
	if err := os.MkdirAll(filepath.Dir(s.salvo), 0o755); err != nil {
		return "", err
	}
	return s.salvo, os.WriteFile(s.salvo, []byte(s.texto()), 0o644)
}

// relogioCrescente devolve um relogio estritamente crescente: no Windows o
// time.Now tem resolucao grosseira e o store em memoria compara instantes
// (mensagens no mesmo tick seriam tratadas como pendentes de novo).
func relogioCrescente(base func() time.Time) func() time.Time {
	var mu sync.Mutex
	var ultimo time.Time
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		t := base()
		if !t.After(ultimo) {
			t = ultimo.Add(time.Microsecond)
		}
		ultimo = t
		return t
	}
}
