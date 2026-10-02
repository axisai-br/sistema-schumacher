// Package agente orquestra um turno de atendimento: contexto, loop LLM e
// ferramentas, checagens de saida e envio (ou transferencia para humano).
package agente

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/politica"
)

const (
	TextoTransferencia = "Vou te passar para um atendente da nossa equipe, que continua por aqui. 😊"
	TextoTecnico       = "Tive um problema técnico aqui. Vou chamar um atendente para continuar com você."

	limiteErrosSeguidos = 3
)

type FonteCatalogo interface {
	TextoCatalogo(ctx context.Context) (string, error)
}

type PreparadorMidia interface {
	Preparar(ctx context.Context, m conversa.Mensagem) (texto string, extra map[string]any, err error)
}

type Notificador interface {
	AvisarTransferencia(ctx context.Context, c conversa.Conversa, motivo string, resumo string) error
}

type Avaliacao struct{ PedeHumano, Irritacao, ForaDoAssunto float64 } // 0..1

type Juiz interface {
	Avaliar(ctx context.Context, ultimas []conversa.Mensagem) (Avaliacao, error)
}

type Deps struct {
	Store       conversa.Store
	Canal       canal.Canal
	Modelo      llm.Modelo
	Ferramentas *ferramentas.Registro
	Catalogo    FonteCatalogo
	Midia       PreparadorMidia // opcional
	Juiz        Juiz            // opcional
	Roteador    Roteador        // opcional; quando presente substitui o Juiz
	Cidades     FonteCidades    // opcional; cidades atendidas para o Roteador
	Notificador Notificador     // opcional
	Agora       func() time.Time
	Log         *log.Logger
}

type Config struct {
	Modelo          string
	MaxPassos       int     // 6
	HistoricoN      int     // 20
	LimiarHumano    float64 // 0.7
	LimiarIrritacao float64 // 0.75
	LimiarRota      float64 // 0.8: confianca minima do Roteador para agir sozinho
	// OrcamentoTurno limita o tempo das chamadas ao LLM no turno (padrao 50s):
	// estourou, o turno responde com dados ja obtidos em vez de esperar.
	OrcamentoTurno time.Duration
	// SinalPorPagante vai no contexto do LLM (valores do sinal antes da
	// reserva). Padrao 250, o mesmo das ferramentas.
	SinalPorPagante float64
	// Motor: MotorAgente (padrao, LLM com ferramentas) ou MotorComandos (LLM
	// so extrai JSON; codigo decide e responde por template).
	Motor string
	// OrcamentoExtrator limita a extracao no motor por comandos (padrao 25s).
	OrcamentoExtrator time.Duration
	// ModeloExtratorReserva e tentado quando o extrator principal nao devolve
	// JSON valido (vazio = sem cascata).
	ModeloExtratorReserva string
}

type Agente struct {
	d   Deps
	cfg Config
	loc *time.Location
}

func Novo(d Deps, cfg Config) *Agente {
	if cfg.MaxPassos <= 0 {
		cfg.MaxPassos = 6
	}
	if cfg.HistoricoN <= 0 {
		cfg.HistoricoN = 20
	}
	if cfg.LimiarHumano <= 0 {
		cfg.LimiarHumano = 0.7
	}
	if cfg.LimiarIrritacao <= 0 {
		cfg.LimiarIrritacao = 0.75
	}
	if cfg.LimiarRota <= 0 {
		cfg.LimiarRota = 0.8
	}
	if cfg.SinalPorPagante <= 0 {
		cfg.SinalPorPagante = 250
	}
	if cfg.Motor == "" {
		cfg.Motor = MotorAgente
	}
	if cfg.OrcamentoExtrator <= 0 {
		cfg.OrcamentoExtrator = 25 * time.Second
	}
	if cfg.OrcamentoTurno <= 0 {
		cfg.OrcamentoTurno = 50 * time.Second
	}
	if d.Agora == nil {
		d.Agora = time.Now
	}
	if d.Log == nil {
		d.Log = log.New(io.Discard, "", 0)
	}
	if d.Ferramentas == nil {
		d.Ferramentas = ferramentas.NovoRegistro()
	}
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*3600)
	}
	return &Agente{d: d, cfg: cfg, loc: loc}
}

// turno guarda o estado de trabalho de um turno em andamento.
type turno struct {
	c          conversa.Conversa
	id         string
	inicio     time.Time
	antes      conversa.Estado
	estado     conversa.Estado
	entradaIDs []string
	passos     []conversa.Passo
	tokIn      int
	tokOut     int
	modelo     string
	resultados []string // JSON das saidas de ferramenta deste turno
	anteriores []string // JSON das saidas de ferramenta dos ultimos turnos
	nPre       int      // ferramentas pre-executadas pelo codigo (ids pre_1, pre_2...)
	rota       *Rota    // veredito do Roteador neste turno (nil sem roteador ou se falhou)
	// diaEspecifico: o cliente citou um dia exato neste turno ("dia 8", "15/10").
	diaEspecifico bool
	// Motor por comandos: extracao do LLM e o que dela foi usado na Rota.
	comandos  bool
	ext       *Extracao
	extUsados []string
	// sombra: extracao rodando em paralelo com o motor atual (MotorSombra);
	// o resultado so vai para os passos do turno, para comparacao.
	sombra chan conversa.Passo
}

// transf e o pedido interno de transferencia para humano.
type transf struct {
	motivo  string
	tecnico bool // usa a mensagem de problema tecnico
}

func (t *transf) Error() string { return "transferir para humano: " + t.motivo }

func clonarEstado(e conversa.Estado) conversa.Estado {
	b, err := json.Marshal(e)
	if err != nil {
		return e
	}
	var out conversa.Estado
	if err := json.Unmarshal(b, &out); err != nil {
		return e
	}
	return out
}

// Processar executa um turno. A conversa ja foi reivindicada (lease) pelo
// worker; c.UltimaEntradaEm e o marco do turno. Qualquer erro devolvido antes
// de enviar deixa a pendencia aberta (o worker tenta de novo); apos 3 turnos
// seguidos com ERRO desde PendenteDesde, faz a transferencia padrao.
func (a *Agente) Processar(ctx context.Context, c conversa.Conversa) error {
	if c.PendenteDesde == nil || c.UltimaEntradaEm == nil {
		return nil
	}
	tc := &turno{
		c: c, id: uuid.NewString(), inicio: a.d.Agora(),
		antes: clonarEstado(c.Estado), estado: clonarEstado(c.Estado),
	}
	err := a.executar(ctx, tc)
	if err == nil {
		return nil
	}
	var t *transf
	if errors.As(err, &t) {
		err = a.transferir(ctx, tc, t.motivo, t.tecnico)
		if err == nil {
			return nil
		}
	}
	a.registrarErro(ctx, tc, err)
	return err
}

func (a *Agente) executar(ctx context.Context, tc *turno) error {
	c := tc.c
	if a.errosSeguidos(ctx, c) >= limiteErrosSeguidos {
		return &transf{motivo: "falha repetida ao processar a conversa", tecnico: true}
	}

	entradas, err := a.d.Store.EntradasPendentes(ctx, c.ID, *c.PendenteDesde)
	if err != nil {
		return fmt.Errorf("entradas pendentes: %w", err)
	}
	if len(entradas) == 0 {
		return a.d.Store.ConcluirPendencia(ctx, c.ID, *c.UltimaEntradaEm)
	}
	tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "politica", Saida: politica.Versao()})
	var textosCliente []string
	for _, e := range entradas {
		tc.entradaIDs = append(tc.entradaIDs, e.ID)
		if a.d.Midia != nil && e.Midia != nil && e.Midia["preparada"] != true && (e.Tipo != conversa.TipoTexto || strings.TrimSpace(e.Texto) == "") {
			t0 := a.d.Agora()
			texto, extra, err := a.d.Midia.Preparar(ctx, e)
			if err != nil {
				return fmt.Errorf("preparar midia: %w", err)
			}
			if extra == nil {
				extra = map[string]any{}
			}
			extra["preparada"] = true
			if err := a.d.Store.AtualizarTexto(ctx, e.ID, texto, extra); err != nil {
				return fmt.Errorf("atualizar texto da midia: %w", err)
			}
			e.Texto = texto
			tc.passos = append(tc.passos, conversa.Passo{Tipo: "midia", Nome: string(e.Tipo), Saida: texto, DuracaoMS: a.d.Agora().Sub(t0).Milliseconds()})
		}
		textosCliente = append(textosCliente, e.Texto)
	}

	// Atalho deterministico: pedido explicito de humano.
	for _, t := range textosCliente {
		if pedeHumano(t) {
			tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "atalho_humano"})
			return &transf{motivo: "cliente pediu atendente"}
		}
	}

	hist, err := a.d.Store.Historico(ctx, c.ID, a.cfg.HistoricoN)
	if err != nil {
		return fmt.Errorf("historico: %w", err)
	}

	tc.anteriores = a.saidasAnteriores(ctx, c.ID)
	if a.cfg.Motor == MotorSombra {
		tc.sombra = make(chan conversa.Passo, 1)
		est := clonarEstado(tc.estado)
		go func() {
			p, _ := a.passoSombra(ctx, est, hist)
			tc.sombra <- p
		}()
	}
	if p, ok := ferramentas.ResolverQuando(strings.Join(textosCliente, " "), a.d.Agora().In(a.loc)); ok && p.De.Equal(p.Ate) {
		tc.diaEspecifico = true
	}

	var pre []llm.Mensagem // chamada de ferramenta executada antes do LLM (pre-busca)
	if a.cfg.Motor == MotorComandos && a.d.Roteador != nil {
		texto, tr := a.turnoComandos(ctx, tc, hist)
		if tr != nil {
			return tr
		}
		return a.concluirComResposta(ctx, tc, texto)
	}
	if a.d.Roteador != nil {
		res := a.rotear(ctx, tc, hist)
		if res.transf != nil {
			return res.transf
		}
		if res.resposta != "" {
			return a.concluirComResposta(ctx, tc, res.resposta)
		}
		pre = res.pre
	} else if a.d.Juiz != nil {
		ultimas := hist
		if len(ultimas) > 6 {
			ultimas = ultimas[len(ultimas)-6:]
		}
		t0 := a.d.Agora()
		av, err := a.d.Juiz.Avaliar(ctx, ultimas)
		p := conversa.Passo{Tipo: "checagem", Nome: "juiz", Saida: av, DuracaoMS: a.d.Agora().Sub(t0).Milliseconds()}
		if err != nil {
			p.Erro = err.Error()
			a.d.Log.Printf("agente: juiz falhou (ignorado): %v", err)
		}
		tc.passos = append(tc.passos, p)
		if err == nil {
			if av.PedeHumano >= a.cfg.LimiarHumano {
				return &transf{motivo: "cliente pediu atendente (juiz)"}
			}
			if av.Irritacao >= a.cfg.LimiarIrritacao {
				return &transf{motivo: "cliente irritado (juiz)"}
			}
		}
	}

	catalogo := ""
	if a.d.Catalogo != nil {
		catalogo, err = a.d.Catalogo.TextoCatalogo(ctx)
		if err != nil {
			return fmt.Errorf("catalogo: %w", err)
		}
	}
	agora := a.d.Agora().In(a.loc)
	instr := a.instrucoes(tc, catalogo, agora)
	msgs := append(mapearHistorico(hist), pre...)

	ctxG, cancelG := context.WithTimeout(ctx, a.cfg.OrcamentoTurno)
	defer cancelG()
	seguro := false // resposta montada em codigo (dispensa a checagem de fatos)
	texto, msgs, err := a.gerar(ctxG, tc, instr, msgs)
	if err != nil {
		if texto, seguro = a.recuperar(ctx, tc, err); !seguro {
			return a.semRecuperacao(ctx, err)
		}
	}

	// Checagem de fatos: valores, datas e horarios precisam ter origem, e rotas
	// com horarios precisam ter sido buscadas (ex.: volta inventada com as datas
	// da ida).
	cidades := a.nomesCidades(ctx)
	if prob := a.problemasResposta(tc, texto, catalogo, agora, textosCliente, cidades); !seguro && prob != "" {
		tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "fatos", Saida: "sem origem: " + prob})
		msgs = append(msgs,
			llm.Mensagem{Papel: llm.PapelAssistente, Texto: texto},
			llm.Mensagem{Papel: llm.PapelUsuario, Texto: fmt.Sprintf("Sua resposta citou %s que não vieram das ferramentas. Reescreva usando só dados das ferramentas; para outra rota (por exemplo, a volta), chame buscar_viagens antes.", prob)},
		)
		texto, _, err = a.gerar(ctxG, tc, instr, msgs)
		if err != nil {
			if texto, seguro = a.recuperar(ctx, tc, err); !seguro {
				return a.semRecuperacao(ctx, err)
			}
		}
		if prob = a.problemasResposta(tc, texto, catalogo, agora, textosCliente, cidades); !seguro && prob != "" {
			// Se houve busca neste turno, responde com as opcoes reais montadas
			// em codigo em vez de transferir.
			if px := pixDoTurno(tc.resultados); len(px) > 0 {
				tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "fatos", Saida: "sem origem apos reescrita: " + prob + "; respondendo com os PIX gerados"})
				texto = textoPix(px)
			} else if bs := opcoesDoTurno(tc.resultados); len(bs) > 0 {
				tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "fatos", Saida: "sem origem apos reescrita: " + prob + "; respondendo com as opcoes da busca"})
				texto = textoOpcoes(bs[len(bs)-1])
			} else if seg := a.respostaSegura(ctx, tc); seg != "" {
				tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "fatos", Saida: "sem origem apos reescrita: " + prob + "; resposta segura"})
				texto = seg
			} else {
				tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "fatos", Saida: "sem origem apos reescrita: " + prob})
				return &transf{motivo: "resposta com dados sem origem nas ferramentas: " + prob}
			}
		}
	}

	// Checagem de forma (modelos menores): texto quebrado, acao afirmada sem a
	// ferramenta ou CPF do cliente ignorado. Uma reescrita; se insistir, a
	// resposta e montada em codigo a partir das pendencias.
	if !seguro {
		if prob := problemasForma(texto, tc, textosCliente); prob != "" {
			tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "forma", Saida: prob})
			msgs = append(msgs,
				llm.Mensagem{Papel: llm.PapelAssistente, Texto: texto},
				llm.Mensagem{Papel: llm.PapelUsuario, Texto: "Problema na sua resposta: " + prob + ". Corrija agora: chame a ferramenta necessária ANTES de dizer que algo foi feito (por exemplo, registrar_passageiros com os nomes e documentos que o cliente mandou), ou não afirme que foi feito. Responda em português simples, sem marcações internas."},
			)
			novo, _, err := a.gerar(ctxG, tc, instr, msgs)
			var t *transf
			if err != nil && errors.As(err, &t) && !t.tecnico {
				return err
			}
			if err == nil && problemasForma(novo, tc, textosCliente) == "" && a.problemasResposta(tc, novo, catalogo, agora, textosCliente, cidades) == "" {
				texto = novo
			} else {
				texto = ""
				if px := pixDoTurno(tc.resultados); len(px) > 0 {
					texto = textoPix(px)
				} else if len(cpfsNaoRegistrados(textosCliente, tc.estado)) > 0 {
					// O modelo nao registrou os dados que o cliente mandou: o
					// codigo registra (fotos e "nome cpf ...") e segue.
					novos, _ := passageirosDeFotos(textosCliente, agora)
					novos = append(novos, passageirosDeTexto(textosCliente)...)
					if _, ok := a.registrarEmCodigo(ctx, tc, novos); ok {
						texto = textoRegistrados(tc.estado) + "\n\n" + textoProximoPasso(tc.estado)
					}
				}
				if texto == "" {
					texto = textoProximoPasso(tc.estado)
				}
				tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "forma", Saida: "persistiu apos reescrita; resposta montada em codigo"})
			}
		}
	}

	// Checagem de loop: mesma resposta que a ultima do bot.
	ultimoBot := ""
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Autor == conversa.AutorBot && !envioFalhou(hist[i]) {
			ultimoBot = hist[i].Texto
			break
		}
	}
	if ultimoBot != "" && similaridade(texto, ultimoBot) >= 0.9 {
		tc.estado.Falhas++
		tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "loop", Saida: fmt.Sprintf("falhas=%d", tc.estado.Falhas)})
	} else if estadoAvancou(tc.antes, tc.estado) {
		tc.estado.Falhas = 0
	}
	if tc.estado.Falhas >= 2 {
		return &transf{motivo: "bot repetindo a mesma resposta"}
	}

	return a.concluirComResposta(ctx, tc, texto)
}

// concluirComResposta e o fim comum do turno com resposta pronta (LLM ou template):
// descarta se chegou mensagem nova, salva o estado, envia, registra o turno e
// conclui a pendencia.
func (a *Agente) concluirComResposta(ctx context.Context, tc *turno, texto string) error {
	c := tc.c
	// Ultima barreira: persona, ferramentas, texto degenerado, markdown, CPF.
	if novo, motivos := filtrarSaida(texto, tc.estado); len(motivos) > 0 {
		tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "saida",
			Saida: map[string]any{"motivos": motivos, "original": texto}})
		texto = novo
	}
	// Mensagem nova durante o turno: descarta e mantem a pendencia.
	chegou, err := a.d.Store.ChegouEntradaDepois(ctx, c.ID, *c.UltimaEntradaEm)
	if err != nil {
		return fmt.Errorf("checar mensagem nova: %w", err)
	}
	if chegou {
		a.salvarEstadoSeMudou(ctx, tc)
		a.registrarTurno(ctx, tc, conversa.ResultadoDescartadoMsgNova, texto, "")
		return nil
	}

	if _, err := a.d.Store.SalvarEstado(ctx, c.ID, tc.estado, c.Versao); err != nil {
		return fmt.Errorf("salvar estado: %w", err)
	}
	if err := a.enviar(ctx, tc, texto); err != nil {
		return err
	}
	a.registrarTurno(ctx, tc, conversa.ResultadoEnviado, texto, "")
	if err := a.d.Store.ConcluirPendencia(ctx, c.ID, *c.UltimaEntradaEm); err != nil {
		return fmt.Errorf("concluir pendencia: %w", err)
	}
	return nil
}

// gerar roda o loop LLM <-> ferramentas ate vir texto sem chamadas. Devolve o
// texto e as mensagens acumuladas. Transferir de ferramenta e erro do modelo
// viram *transf.
func (a *Agente) gerar(ctx context.Context, tc *turno, instr string, msgs []llm.Mensagem) (string, []llm.Mensagem, error) {
	defs := a.d.Ferramentas.Defs()
	for passo := 0; passo < a.cfg.MaxPassos; passo++ {
		t0 := a.d.Agora()
		resp, err := a.d.Modelo.Gerar(ctx, llm.Pedido{
			Modelo: a.cfg.Modelo, Instrucoes: instr, Mensagens: msgs, Ferramentas: defs,
		})
		ps := conversa.Passo{Tipo: "llm", Nome: a.cfg.Modelo, DuracaoMS: a.d.Agora().Sub(t0).Milliseconds()}
		if err != nil {
			ps.Erro = err.Error()
			tc.passos = append(tc.passos, ps)
			if ctx.Err() != nil {
				return "", msgs, ctx.Err()
			}
			a.d.Log.Printf("agente: modelo falhou: %v", err)
			return "", msgs, &transf{motivo: "falha do modelo de linguagem", tecnico: true}
		}
		tc.tokIn += resp.TokensEntrada
		tc.tokOut += resp.TokensSaida
		if resp.Modelo != "" {
			tc.modelo = resp.Modelo
		}
		ps.Saida = map[string]any{"texto": resp.Texto, "chamadas": len(resp.Chamadas)}
		tc.passos = append(tc.passos, ps)

		if len(resp.Chamadas) == 0 {
			texto := strings.TrimSpace(resp.Texto)
			if texto == "" {
				return "", msgs, &transf{motivo: motivoSemTexto, tecnico: true}
			}
			return texto, msgs, nil
		}
		msgs = append(msgs, llm.Mensagem{Papel: llm.PapelAssistente, Texto: resp.Texto, Chamadas: resp.Chamadas})
		for _, ch := range resp.Chamadas {
			t1 := a.d.Agora()
			var saida ferramentas.Saida
			if ch.Nome == "escolher_viagem" && !escolhaPermitida(tc) {
				// Guarda: o modelo nao escolhe viagem pelo cliente.
				saida = ferramentas.Saida{OK: false, Motivo: "cliente_nao_escolheu", Dados: map[string]any{
					"mensagem": "O cliente ainda nao escolheu uma opcao nesta mensagem. Mostre as opcoes e espere ele escolher; nao escolha por ele.",
				}}
			} else if ch.Nome == "criar_reserva" && !pagamentoEscolhido(tc) {
				// Guarda: o modelo nao escolhe a forma de pagamento pelo cliente.
				saida = ferramentas.Saida{OK: false, Motivo: "cliente_nao_escolheu_pagamento", Dados: map[string]any{
					"mensagem": "O cliente ainda nao escolheu entre integral e sinal. Pergunte e espere a resposta antes de criar a reserva.",
				}}
			} else {
				saida = a.d.Ferramentas.Executar(ctx, &ferramentas.Contexto{
					Conversa: tc.c, Estado: &tc.estado, Agora: a.d.Agora(),
				}, ch.Nome, ch.Argumentos)
			}
			js, err := json.Marshal(saida)
			if err != nil {
				js = []byte(`{"ok":false,"motivo":"saida_invalida"}`)
			}
			tc.resultados = append(tc.resultados, string(js))
			var entrada any = ch.Argumentos
			if !json.Valid(ch.Argumentos) {
				entrada = string(ch.Argumentos)
			}
			tc.passos = append(tc.passos, conversa.Passo{
				Tipo: "ferramenta", Nome: ch.Nome, Entrada: entrada, Saida: saida,
				DuracaoMS: a.d.Agora().Sub(t1).Milliseconds(),
			})
			msgs = append(msgs, llm.Mensagem{Papel: llm.PapelFerramenta, ChamadaID: ch.ID, Texto: string(js)})
			if saida.Transferir {
				motivo := tc.estado.MotivoHumano
				if motivo == "" {
					motivo = saida.Motivo
				}
				if motivo == "" {
					motivo = "transferido pelo agente"
				}
				return "", msgs, &transf{motivo: motivo}
			}
		}
	}
	return "", msgs, &transf{motivo: motivoLimitePassos, tecnico: true}
}

// problemasResposta junta itens sem origem e rotas nao buscadas; vazio se ok.
func (a *Agente) problemasResposta(tc *turno, texto, catalogo string, agora time.Time, cliente, cidades []string) string {
	var p []string
	if f := itensSemOrigem(texto, a.fontes(tc, catalogo, agora), cliente); len(f) > 0 {
		p = append(p, listarItens(f))
	}
	if rs := rotasSemBusca(texto, cidades, rotasConhecidas(tc)); len(rs) > 0 {
		p = append(p, "a rota "+strings.Join(rs, ", ")+" sem busca")
	}
	return strings.Join(p, "; ")
}

func (a *Agente) nomesCidades(ctx context.Context) []string {
	if a.d.Cidades == nil {
		return nil
	}
	cs, err := a.d.Cidades.Cidades(ctx)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Nome)
	}
	return out
}

// fontes reune o que e confiavel para a checagem de fatos.
func (a *Agente) fontes(tc *turno, catalogo string, agora time.Time) []string {
	est, _ := json.Marshal(tc.estado)
	antes, _ := json.Marshal(tc.antes)
	f := []string{catalogo, string(est), string(antes), agora.Format("02/01/2006")}
	f = append(f, tc.anteriores...)
	f = append(f, valoresDerivados(tc), TextoSituacao(tc.estado, a.cfg.SinalPorPagante), TextoSituacao(tc.antes, a.cfg.SinalPorPagante))
	return append(f, tc.resultados...)
}

// valoresDerivados: contas simples que o modelo pode fazer com precos reais
// (preco x pessoas, soma dos trechos x pessoas), para nao barrar "2 x R$ 950 =
// R$ 1.900".
func valoresDerivados(tc *turno) string {
	var precos []float64
	for _, e := range []conversa.Estado{tc.antes, tc.estado} {
		for _, o := range e.Opcoes {
			precos = append(precos, o.Preco)
		}
		soma := 0.0
		for _, t := range e.Trechos {
			precos = append(precos, t.Viagem.Preco)
			soma += t.Viagem.Preco
		}
		if soma > 0 {
			precos = append(precos, soma)
		}
	}
	var b strings.Builder
	visto := map[float64]bool{}
	for _, p := range precos {
		if p <= 0 || visto[p] {
			continue
		}
		visto[p] = true
		for k := 2; k <= 10; k++ {
			fmt.Fprintf(&b, "%.2f ", p*float64(k))
		}
	}
	return b.String()
}

// saidasAnteriores devolve as saidas de ferramenta dos ultimos turnos: dados
// mostrados ao cliente ha pouco continuam valendo como fonte.
func (a *Agente) saidasAnteriores(ctx context.Context, conversaID string) []string {
	turnos, err := a.d.Store.Turnos(ctx, conversaID, 3)
	if err != nil {
		return nil
	}
	var out []string
	for _, t := range turnos {
		for _, p := range t.Passos {
			if p.Tipo != "ferramenta" || p.Saida == nil {
				continue
			}
			if js, err := json.Marshal(p.Saida); err == nil {
				out = append(out, string(js))
			}
		}
	}
	return out
}

// transferir e o caminho unico de transferencia para humano.
//
// Decisao: a mensagem padrao e enviada mesmo que tenha chegado mensagem nova
// durante o turno. Ela nao depende do conteudo (nao ha resposta velha a
// descartar) e evita silencio; como o status vira HUMANO o worker nao
// reprocessa, e o atendente ve a conversa inteira. Por isso a pendencia e
// concluida ate a ultima entrada atual.
func (a *Agente) transferir(ctx context.Context, tc *turno, motivo string, tecnico bool) error {
	c := tc.c
	texto := TextoTransferencia
	if tecnico {
		texto = TextoTecnico
	}
	tc.estado.MotivoHumano = motivo
	if err := a.enviar(ctx, tc, texto); err != nil {
		return fmt.Errorf("enviar transferencia: %w", err)
	}
	// Apos o envio, falhas sao registradas mas nao repetem o envio.
	_, err := a.d.Store.SalvarEstado(ctx, c.ID, tc.estado, c.Versao)
	if errors.Is(err, conversa.ErrVersaoConflito) {
		if cur, e2 := a.d.Store.Obter(ctx, c.ID); e2 == nil {
			_, err = a.d.Store.SalvarEstado(ctx, c.ID, tc.estado, cur.Versao)
		}
	}
	if err != nil {
		a.d.Log.Printf("agente: salvar estado (transferencia): %v", err)
	}
	atual, err := a.d.Store.MudarStatus(ctx, c.ID, conversa.StatusHumano, "", motivo)
	if err != nil {
		return fmt.Errorf("mudar status: %w", err)
	}
	if a.d.Notificador != nil {
		atual.Estado = tc.estado
		if err := a.d.Notificador.AvisarTransferencia(ctx, atual, motivo, tc.estado.Resumo()); err != nil {
			a.d.Log.Printf("agente: notificar transferencia: %v", err)
		}
	}
	a.registrarTurno(ctx, tc, conversa.ResultadoHumano, texto, "")
	ate := *c.UltimaEntradaEm
	if atual.UltimaEntradaEm != nil && atual.UltimaEntradaEm.After(ate) {
		ate = *atual.UltimaEntradaEm
	}
	if err := a.d.Store.ConcluirPendencia(ctx, c.ID, ate); err != nil {
		a.d.Log.Printf("agente: concluir pendencia (transferencia): %v", err)
	}
	return nil
}

// enviar grava a saida ANTES de chamar o canal (o eco fromMe do webhook pode
// chegar antes de Enviar retornar) e depois confirma o resultado do envio.
func (a *Agente) enviar(ctx context.Context, tc *turno, texto string) error {
	c := tc.c
	msg, err := a.d.Store.RegistrarSaida(ctx, c.ID, conversa.AutorBot, texto, "", tc.id)
	if err != nil {
		return fmt.Errorf("registrar saida: %w", err)
	}
	provID, err := a.d.Canal.Enviar(ctx, c.Contato, texto)
	if err != nil {
		if e2 := a.d.Store.ConfirmarEnvio(ctx, msg.ID, "", err.Error()); e2 != nil {
			a.d.Log.Printf("agente: confirmar envio (falha): %v", e2)
		}
		return fmt.Errorf("enviar: %w", err)
	}
	if err := a.d.Store.ConfirmarEnvio(ctx, msg.ID, provID, ""); err != nil {
		a.d.Log.Printf("agente: confirmar envio: %v", err)
	}
	return nil
}

// errosSeguidos conta turnos ERRO consecutivos (do mais recente para tras)
// criados desde PendenteDesde.
func (a *Agente) errosSeguidos(ctx context.Context, c conversa.Conversa) int {
	turnos, err := a.d.Store.Turnos(ctx, c.ID, limiteErrosSeguidos+2)
	if err != nil {
		a.d.Log.Printf("agente: listar turnos: %v", err)
		return 0
	}
	n := 0
	for i := len(turnos) - 1; i >= 0; i-- {
		t := turnos[i]
		if t.Resultado != conversa.ResultadoErro || t.CriadoEm.Before(*c.PendenteDesde) {
			break
		}
		n++
	}
	return n
}

func (a *Agente) salvarEstadoSeMudou(ctx context.Context, tc *turno) {
	if !estadoAvancou(tc.antes, tc.estado) {
		return
	}
	e := tc.estado
	e.Falhas = tc.antes.Falhas
	if _, err := a.d.Store.SalvarEstado(ctx, tc.c.ID, e, tc.c.Versao); err != nil {
		a.d.Log.Printf("agente: salvar estado (descartado): %v", err)
	}
}

func (a *Agente) registrarErro(ctx context.Context, tc *turno, cause error) {
	a.registrarTurno(ctx, tc, conversa.ResultadoErro, "", cause.Error())
}

func (a *Agente) registrarTurno(ctx context.Context, tc *turno, resultado, resposta, erro string) {
	if tc.sombra != nil {
		select {
		case p := <-tc.sombra:
			if p.Nome != "" {
				tc.passos = append(tc.passos, p)
			}
		case <-time.After(a.cfg.OrcamentoExtrator + 5*time.Second):
		}
		tc.sombra = nil
	}
	modelo := tc.modelo
	if modelo == "" {
		modelo = a.cfg.Modelo
	}
	ids := tc.entradaIDs
	if ids == nil {
		ids = []string{}
	}
	_, err := a.d.Store.RegistrarTurno(ctx, conversa.Turno{
		ID: tc.id, ConversaID: tc.c.ID, EntradaIDs: ids, Passos: tc.passos,
		EstadoAntes: tc.antes, EstadoDepois: tc.estado, Resposta: resposta,
		Resultado: resultado, Modelo: modelo, Erro: erro,
		TokensEntrada: tc.tokIn, TokensSaida: tc.tokOut,
		LatenciaMS: a.d.Agora().Sub(tc.inicio).Milliseconds(),
	})
	if err != nil {
		a.d.Log.Printf("agente: registrar turno (%s): %v", resultado, err)
	}
}
