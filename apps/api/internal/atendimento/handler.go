// Package atendimento expoe o webhook e as rotas da equipe do atendimento v2.
package atendimento

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/auth"
	httpx "schumacher-tur/api/internal/shared/http"
)

// ErrPayloadInvalido indica corpo de webhook que nao pode ser interpretado (HTTP 400).
var ErrPayloadInvalido = errors.New("atendimento: payload invalido")

type Deps struct {
	Store          conversa.Store
	Canal          canal.Canal
	SegredoWebhook string
	Log            *log.Logger
	Agora          func() time.Time
	JanelaEco      time.Duration // padrao 60s
	// TelefonesPermitidos restringe o v2 a estes telefones (so digitos). Vazio = todos.
	TelefonesPermitidos []string
}

type Handler struct{ d Deps }

func NovoHandler(d Deps) *Handler {
	if d.Log == nil {
		d.Log = log.Default()
	}
	if d.Agora == nil {
		d.Agora = time.Now
	}
	if d.JanelaEco <= 0 {
		d.JanelaEco = 60 * time.Second
	}
	return &Handler{d: d}
}

// Recebimento e o resultado do tratamento de um evento do webhook.
type Recebimento struct {
	Status     string `json:"status"` // "registrado" | "duplicada" | "ignorado"
	Motivo     string `json:"motivo,omitempty"`
	ConversaID string `json:"conversa_id,omitempty"`
	MensagemID string `json:"mensagem_id,omitempty"`
}

func (h *Handler) RegisterWebhooks(r chi.Router) {
	r.Post("/webhooks/evolution/v2", h.webhook)
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/atendimento/conversas", h.listar)
	r.Get("/atendimento/conversas/{id}", h.detalhe)
	r.Post("/atendimento/conversas/{id}/assumir", h.assumir)
	r.Post("/atendimento/conversas/{id}/devolver", h.devolver)
	r.Post("/atendimento/conversas/{id}/encerrar", h.encerrar)
	r.Post("/atendimento/conversas/{id}/responder", h.responder)
}

// Receber normaliza e registra um evento do canal. Nao chama LLM.
func (h *Handler) Receber(ctx context.Context, corpo []byte) (Recebimento, error) {
	e, ok, err := h.d.Canal.Normalizar(ctx, corpo)
	if err != nil {
		return Recebimento{}, fmt.Errorf("%w: %v", ErrPayloadInvalido, err)
	}
	if !ok {
		return Recebimento{Status: "ignorado", Motivo: "evento_ignorado"}, nil
	}
	autor := conversa.AutorCliente
	if e.DoProprioNumero {
		if e.ProvedorID != "" {
			conhecido, err := h.d.Store.ProvedorIDConhecido(ctx, e.ProvedorID)
			if err != nil {
				return Recebimento{}, err
			}
			if conhecido {
				return Recebimento{Status: "ignorado", Motivo: "eco_bot"}, nil
			}
		}
		eco, err := h.d.Store.SaidaBotRecente(ctx, e.Contato, e.Texto, h.d.JanelaEco)
		if err != nil {
			return Recebimento{}, err
		}
		if eco {
			return Recebimento{Status: "ignorado", Motivo: "eco_bot"}, nil
		}
		autor = conversa.AutorHumano
	}
	c, m, dup, err := h.d.Store.RegistrarEntrada(ctx, conversa.NovaEntrada{
		Canal: h.d.Canal.Nome(), Contato: e.Contato, Telefone: e.Telefone, Nome: e.Nome,
		Autor: autor, Tipo: e.Tipo, Texto: e.Texto, Midia: e.Midia,
		ProvedorID: e.ProvedorID, RecebidaEm: e.RecebidaEm,
	})
	if err != nil {
		return Recebimento{}, err
	}
	if dup {
		return Recebimento{Status: "duplicada", ConversaID: c.ID, MensagemID: m.ID}, nil
	}
	return Recebimento{Status: "registrado", ConversaID: c.ID, MensagemID: m.ID}, nil
}

func (h *Handler) autorizado(w http.ResponseWriter, r *http.Request) bool {
	expected := strings.TrimSpace(h.d.SegredoWebhook)
	if expected == "" {
		return true
	}
	candidates := []string{
		r.Header.Get("X-Evolution-Webhook-Secret"),
		r.Header.Get("X-Webhook-Secret"),
		r.URL.Query().Get("webhookSecret"),
	}
	if a := strings.TrimSpace(r.Header.Get("Authorization")); a != "" {
		if token := strings.TrimSpace(strings.TrimPrefix(a, "Bearer ")); token != "" && token != a {
			candidates = append(candidates, token)
		}
	}
	for _, c := range candidates {
		if strings.TrimSpace(c) == expected {
			return true
		}
	}
	httpx.WriteError(w, http.StatusUnauthorized, "EVOLUTION_WEBHOOK_UNAUTHORIZED", "invalid evolution webhook secret", nil)
	return false
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	if !h.autorizado(w, r) {
		return
	}
	corpo, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "could not read request body", err.Error())
		return
	}
	res, err := h.Receber(r.Context(), corpo)
	if err != nil {
		if errors.Is(err, ErrPayloadInvalido) {
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
		h.d.Log.Printf("atendimento webhook: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "ATENDIMENTO_WEBHOOK_ERROR", "could not process webhook", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// ---- rotas da equipe ----

type conversaDTO struct {
	ID            string     `json:"id"`
	Canal         string     `json:"canal"`
	Telefone      string     `json:"telefone"`
	Nome          string     `json:"nome"`
	Status        string     `json:"status"`
	ResponsavelID string     `json:"responsavel_id"`
	AtualizadoEm  time.Time  `json:"atualizado_em"`
	PendenteDesde *time.Time `json:"pendente_desde"`
	Pendente      bool       `json:"pendente"`
	HumanoAte     *time.Time `json:"humano_ate"`
	Resumo        string     `json:"resumo"`
	MotivoHumano  string     `json:"motivo_humano"`
}

func toConversaDTO(c conversa.Conversa) conversaDTO {
	return conversaDTO{
		ID: c.ID, Canal: c.Canal, Telefone: c.Telefone, Nome: c.Nome, Status: string(c.Status),
		ResponsavelID: c.ResponsavelID, AtualizadoEm: c.AtualizadoEm, PendenteDesde: c.PendenteDesde,
		Pendente: c.PendenteDesde != nil, HumanoAte: c.HumanoAte,
		Resumo: c.Estado.Resumo(), MotivoHumano: c.Estado.MotivoHumano,
	}
}

type mensagemDTO struct {
	ID         string         `json:"id"`
	Direcao    string         `json:"direcao"`
	Autor      string         `json:"autor"`
	Tipo       string         `json:"tipo"`
	Texto      string         `json:"texto"`
	Midia      map[string]any `json:"midia,omitempty"`
	ProvedorID string         `json:"provedor_id,omitempty"`
	TurnoID    string         `json:"turno_id,omitempty"`
	CriadoEm   time.Time      `json:"criado_em"`
}

func toMensagemDTO(m conversa.Mensagem) mensagemDTO {
	return mensagemDTO{ID: m.ID, Direcao: string(m.Direcao), Autor: string(m.Autor), Tipo: string(m.Tipo),
		Texto: m.Texto, Midia: m.Midia, ProvedorID: m.ProvedorID, TurnoID: m.TurnoID, CriadoEm: m.CriadoEm}
}

type turnoDTO struct {
	ID            string           `json:"id"`
	EntradaIDs    []string         `json:"entrada_ids"`
	Passos        []conversa.Passo `json:"passos"`
	EstadoAntes   conversa.Estado  `json:"estado_antes"`
	EstadoDepois  conversa.Estado  `json:"estado_depois"`
	Resposta      string           `json:"resposta"`
	Resultado     string           `json:"resultado"`
	Modelo        string           `json:"modelo"`
	Erro          string           `json:"erro,omitempty"`
	TokensEntrada int              `json:"tokens_entrada"`
	TokensSaida   int              `json:"tokens_saida"`
	LatenciaMS    int64            `json:"latencia_ms"`
	CriadoEm      time.Time        `json:"criado_em"`
}

func toTurnoDTO(t conversa.Turno) turnoDTO {
	return turnoDTO{ID: t.ID, EntradaIDs: t.EntradaIDs, Passos: t.Passos, EstadoAntes: t.EstadoAntes,
		EstadoDepois: t.EstadoDepois, Resposta: t.Resposta, Resultado: t.Resultado, Modelo: t.Modelo,
		Erro: t.Erro, TokensEntrada: t.TokensEntrada, TokensSaida: t.TokensSaida,
		LatenciaMS: t.LatenciaMS, CriadoEm: t.CriadoEm}
}

func (h *Handler) erroStore(w http.ResponseWriter, err error) {
	if errors.Is(err, conversa.ErrNaoEncontrada) {
		httpx.WriteError(w, http.StatusNotFound, "NOT_FOUND", "conversa nao encontrada", nil)
		return
	}
	h.d.Log.Printf("atendimento: %v", err)
	httpx.WriteError(w, http.StatusInternalServerError, "ATENDIMENTO_ERROR", "erro interno", nil)
}

func (h *Handler) listar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := conversa.FiltroLista{Status: conversa.Status(strings.ToUpper(strings.TrimSpace(q.Get("status")))), Busca: strings.TrimSpace(q.Get("q"))}
	switch f.Status {
	case "", conversa.StatusBot, conversa.StatusHumano, conversa.StatusEncerrada:
	default:
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "status invalido", nil)
		return
	}
	if v := q.Get("limite"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "limite invalido", nil)
			return
		}
		f.Limite = n
	}
	if f.Limite == 0 || f.Limite > 200 {
		f.Limite = 50
	}
	lista, err := h.d.Store.Listar(r.Context(), f)
	if err != nil {
		h.erroStore(w, err)
		return
	}
	type item struct {
		conversaDTO
		UltimaMensagem *mensagemDTO `json:"ultima_mensagem,omitempty"`
	}
	out := make([]item, 0, len(lista))
	for _, c := range lista {
		it := item{conversaDTO: toConversaDTO(c)}
		if ms, err := h.d.Store.Historico(r.Context(), c.ID, 1); err == nil && len(ms) > 0 {
			d := toMensagemDTO(ms[len(ms)-1])
			it.UltimaMensagem = &d
		}
		out = append(out, it)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"conversas": out})
}

func (h *Handler) detalhe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := h.d.Store.Obter(r.Context(), id)
	if err != nil {
		h.erroStore(w, err)
		return
	}
	ms, err := h.d.Store.Historico(r.Context(), id, 100)
	if err != nil {
		h.erroStore(w, err)
		return
	}
	ts, err := h.d.Store.Turnos(r.Context(), id, 20)
	if err != nil {
		h.erroStore(w, err)
		return
	}
	msgs := make([]mensagemDTO, 0, len(ms))
	for _, m := range ms {
		msgs = append(msgs, toMensagemDTO(m))
	}
	turnos := make([]turnoDTO, 0, len(ts))
	for _, t := range ts {
		turnos = append(turnos, toTurnoDTO(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"conversa": toConversaDTO(c), "estado": c.Estado, "mensagens": msgs, "turnos": turnos, "resumo": c.Estado.Resumo(),
	})
}

func (h *Handler) mudar(w http.ResponseWriter, r *http.Request, st conversa.Status, resp, motivo string) {
	c, err := h.d.Store.MudarStatus(r.Context(), chi.URLParam(r, "id"), st, resp, motivo)
	if err != nil {
		h.erroStore(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toConversaDTO(c))
}

func (h *Handler) usuario(w http.ResponseWriter, r *http.Request) (string, bool) {
	uid, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(uid) == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "usuario nao autenticado", nil)
		return "", false
	}
	return uid, true
}

func (h *Handler) assumir(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.usuario(w, r)
	if !ok {
		return
	}
	h.mudar(w, r, conversa.StatusHumano, uid, "assumida pela equipe")
}

func (h *Handler) devolver(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.usuario(w, r); !ok {
		return
	}
	h.mudar(w, r, conversa.StatusBot, "", "")
}

func (h *Handler) encerrar(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.usuario(w, r)
	if !ok {
		return
	}
	h.mudar(w, r, conversa.StatusEncerrada, uid, "encerrada pela equipe")
}

func (h *Handler) responder(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.usuario(w, r); !ok {
		return
	}
	var in struct {
		Texto string `json:"texto"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "json invalido", err.Error())
		return
	}
	texto := strings.TrimSpace(in.Texto)
	if texto == "" {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "texto obrigatorio", nil)
		return
	}
	id := chi.URLParam(r, "id")
	c, err := h.d.Store.Obter(r.Context(), id)
	if err != nil {
		h.erroStore(w, err)
		return
	}
	if c.Status != conversa.StatusHumano {
		httpx.WriteError(w, http.StatusConflict, "CONFLICT", "assuma a conversa antes de responder", nil)
		return
	}
	provID, err := h.d.Canal.Enviar(r.Context(), c.Contato, texto)
	if err != nil {
		h.d.Log.Printf("atendimento: enviar conversa_id=%s: %v", id, err)
		httpx.WriteError(w, http.StatusBadGateway, "CANAL_ERROR", "falha ao enviar mensagem", nil)
		return
	}
	m, err := h.d.Store.RegistrarSaida(r.Context(), id, conversa.AutorHumano, texto, provID, "")
	if err != nil {
		h.erroStore(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMensagemDTO(m))
}

// Atende informa se o v2 deve tratar o evento: o payload precisa normalizar
// para uma mensagem e, havendo allowlist, o telefone precisa estar nela.
// Eventos ignorados ou com erro de normalizacao retornam false.
func (h *Handler) Atende(ctx context.Context, corpo []byte) bool {
	e, ok, err := h.d.Canal.Normalizar(ctx, corpo)
	if err != nil || !ok {
		return false
	}
	if len(h.d.TelefonesPermitidos) == 0 {
		return true
	}
	for _, t := range h.d.TelefonesPermitidos {
		if t == e.Telefone {
			return true
		}
	}
	return false
}
