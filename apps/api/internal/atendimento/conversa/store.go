package conversa

import (
	"context"
	"errors"
	"time"
)

var (
	ErrVersaoConflito = errors.New("conversa: conflito de versão")
	ErrNaoEncontrada  = errors.New("conversa: não encontrada")
)

// PausaHumanoPadrao e quanto o bot fica pausado depois de uma mensagem enviada
// pelo celular da empresa.
const PausaHumanoPadrao = 12 * time.Hour

// LeaseDuracao e por quanto tempo uma conversa reivindicada fica reservada ao
// worker. Se o worker morrer, o lease expira e outra goroutine assume.
//
// Escolha de projeto: a reivindicacao usa um LEASE (coluna processando_ate)
// em vez de manter uma transacao aberta com FOR UPDATE. Assim os demais
// metodos (SalvarEstado, ConcluirPendencia, RegistrarSaida...) usam o pool
// normal sem bloquear na linha travada, e um crash do worker nao prende a
// conversa para sempre. O FOR UPDATE SKIP LOCKED fica restrito ao UPDATE
// atomico que concede o lease.
const LeaseDuracao = 2 * time.Minute

type NovaEntrada struct {
	Canal, Contato, Telefone, Nome string
	Autor                          Autor
	Tipo                           Tipo
	Texto                          string
	Midia                          map[string]any
	ProvedorID                     string
	RecebidaEm                     time.Time
}

type FiltroLista struct {
	Status Status
	Busca  string // nome/telefone ilike
	Limite int
}

// Lock representa a reivindicacao (lease) de uma conversa por um worker.
type Lock interface {
	// Conversa e o retrato da conversa no momento da reivindicacao.
	Conversa() Conversa
	// Liberar encerra o lease. Idempotente; deve ser chamado sempre.
	Liberar(ctx context.Context) error
}

type Store interface {
	// Upsert da conversa por (canal, contato) + insert da mensagem. Idempotente por ProvedorID
	// (se ja existe, retorna existente e duplicada=true, sem alterar nada).
	// Todos os horarios usam o relogio do SERVIDOR (NovaEntrada.RecebidaEm nao e usado nos horarios da conversa).
	// Para Autor==CLIENTE: pendente_desde = coalesce(pendente_desde, agora); ultima_entrada_em = agora.
	// Para Autor==HUMANO (mensagem enviada pelo celular da empresa): direcao SAIDA, status=HUMANO,
	// humano_ate = agora + 12h, pendente_desde = null.
	RegistrarEntrada(ctx context.Context, in NovaEntrada) (c Conversa, m Mensagem, duplicada bool, err error)
	RegistrarSaida(ctx context.Context, conversaID string, autor Autor, texto string, provedorID string, turnoID string) (Mensagem, error)
	// Retorna true se o ProvedorID pertence a uma mensagem de saida ja registrada
	// (para ignorar eco do webhook fromMe das mensagens do bot).
	ProvedorIDConhecido(ctx context.Context, provedorID string) (bool, error)
	// SaidaBotRecente: true se existe SAIDA com autor BOT na conversa WHATSAPP desse contato, com texto
	// igual (trim) e criada ha menos de `janela`. Usado para distinguir eco do bot de mensagem humana fromMe.
	SaidaBotRecente(ctx context.Context, contato string, texto string, janela time.Duration) (bool, error)
	// AtualizarTexto atualiza o texto da mensagem e faz merge de midiaExtra no campo midia
	// (ex.: {"transcricao_status":"OK"}).
	AtualizarTexto(ctx context.Context, mensagemID string, texto string, midiaExtra map[string]any) error
	// ConfirmarEnvio registra o resultado do envio de uma SAIDA gravada antes de chamar o canal:
	// erroEnvio == "" grava provedor_id (se nao vazio); senao faz merge em midia
	// {"envio_status":"FALHOU","envio_erro":erroEnvio}. SaidaBotRecente ignora as FALHOU.
	ConfirmarEnvio(ctx context.Context, mensagemID string, provedorID string, erroEnvio string) error
	Obter(ctx context.Context, id string) (Conversa, error)
	// Historico devolve as ultimas `limite` mensagens em ordem cronologica crescente.
	Historico(ctx context.Context, conversaID string, limite int) ([]Mensagem, error)
	// EntradasPendentes devolve mensagens do CLIENTE com criado_em >= desde, em ordem.
	EntradasPendentes(ctx context.Context, conversaID string, desde time.Time) ([]Mensagem, error)
	// ReivindicarDevida concede lease sobre UMA conversa devida (status BOT, pendente_desde not null,
	// ultima_entrada_em <= agora-debounce, sem lease vigente), mais antiga primeiro.
	// ok=false se nada devido. Liberar deve ser chamado sempre.
	ReivindicarDevida(ctx context.Context, debounce time.Duration) (lock Lock, ok bool, err error)
	// ChegouEntradaDepois informa se ultima_entrada_em > t (relogio do servidor).
	ChegouEntradaDepois(ctx context.Context, conversaID string, t time.Time) (bool, error)
	// Concorrencia otimista: atualiza estado se versao==versaoEsperada, incrementa versao.
	// ErrVersaoConflito caso contrario.
	SalvarEstado(ctx context.Context, conversaID string, estado Estado, versaoEsperada int) (novaVersao int, err error)
	// ConcluirPendencia limpa pendente_desde somente se ultima_entrada_em <= ate.
	ConcluirPendencia(ctx context.Context, conversaID string, ate time.Time) error
	// MudarStatus troca o status. Para HUMANO grava estado.motivo_humano (se motivo != "") e nao
	// define humano_ate (transferencia do bot so volta por acao humana); para BOT limpa
	// responsavel, humano_ate e motivo. Nao incrementa versao (o turno em andamento ainda
	// consegue salvar o estado).
	MudarStatus(ctx context.Context, conversaID string, status Status, responsavelID string, motivo string) (Conversa, error)
	// RegistrarTurno grava o turno. Se t.ID vier preenchido, e mantido.
	RegistrarTurno(ctx context.Context, t Turno) (Turno, error)
	// Turnos devolve os ultimos `limite` turnos em ordem cronologica crescente.
	Turnos(ctx context.Context, conversaID string, limite int) ([]Turno, error)
	// Listar ordena por atualizado_em desc.
	Listar(ctx context.Context, f FiltroLista) ([]Conversa, error)
	// ReativarPausadas volta para BOT conversas HUMANO com humano_ate < agora e responsavel_id nulo.
	ReativarPausadas(ctx context.Context, agora time.Time) (int, error)
}
