package atendimento

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/auth"
)

// canalFake interpreta o corpo como JSON ja no formato de canal.Entrada simplificado.
type canalFake struct {
	enviados []string
	provSeq  int
}

type corpoFake struct {
	Ignorar bool   `json:"ignorar"`
	Invalid bool   `json:"invalido"`
	Contato string `json:"contato"`
	Texto   string `json:"texto"`
	Prov    string `json:"prov"`
	FromMe  bool   `json:"from_me"`
}

func (c *canalFake) Nome() string { return "WHATSAPP" }
func (c *canalFake) Normalizar(_ context.Context, corpo []byte) (canal.Entrada, bool, error) {
	var b corpoFake
	if err := json.Unmarshal(corpo, &b); err != nil || b.Invalid {
		return canal.Entrada{}, false, errors.New("corpo invalido")
	}
	if b.Ignorar {
		return canal.Entrada{}, false, nil
	}
	return canal.Entrada{Contato: b.Contato, Telefone: b.Contato, Nome: "Fulano", DoProprioNumero: b.FromMe,
		Tipo: conversa.TipoTexto, Texto: b.Texto, ProvedorID: b.Prov}, true, nil
}
func (c *canalFake) Enviar(_ context.Context, contato, texto string) (string, error) {
	c.enviados = append(c.enviados, contato+"|"+texto)
	c.provSeq++
	return "out-" + string(rune('0'+c.provSeq)), nil
}
func (c *canalFake) BaixarMidia(context.Context, conversa.Mensagem) (string, string, error) {
	return "", "", errors.New("nao suportado")
}

type ambiente struct {
	s   *conversa.StoreMem
	c   *canalFake
	h   *Handler
	r   chi.Router
	now time.Time
}

func novoAmbiente(t *testing.T, segredo string) *ambiente {
	t.Helper()
	a := &ambiente{now: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	a.s = conversa.NewStoreMem(func() time.Time { return a.now })
	a.c = &canalFake{}
	a.h = NovoHandler(Deps{Store: a.s, Canal: a.c, SegredoWebhook: segredo, Log: log.New(io.Discard, "", 0), Agora: func() time.Time { return a.now }})
	a.r = chi.NewRouter()
	a.h.RegisterWebhooks(a.r)
	a.r.Group(func(g chi.Router) {
		g.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if u := r.Header.Get("X-Test-User"); u != "" {
					r = r.WithContext(auth.WithUser(r.Context(), auth.AuthUser{ID: u}))
				}
				next.ServeHTTP(w, r)
			})
		})
		a.h.RegisterRoutes(g)
	})
	return a
}

func (a *ambiente) do(method, url, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	a.r.ServeHTTP(w, req)
	return w
}

func (a *ambiente) webhook(t *testing.T, body string) Recebimento {
	t.Helper()
	w := a.do("POST", "/webhooks/evolution/v2", body, nil)
	if w.Code != 200 {
		t.Fatalf("webhook status %d: %s", w.Code, w.Body)
	}
	var r Recebimento
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWebhookSegredo(t *testing.T) {
	a := novoAmbiente(t, "s3")
	body := `{"contato":"5511","texto":"oi","prov":"a1"}`
	if w := a.do("POST", "/webhooks/evolution/v2", body, nil); w.Code != 401 {
		t.Fatalf("sem segredo: %d", w.Code)
	}
	if w := a.do("POST", "/webhooks/evolution/v2", body, map[string]string{"X-Webhook-Secret": "errado"}); w.Code != 401 {
		t.Fatalf("segredo errado: %d", w.Code)
	}
	for name, w := range map[string]*httptest.ResponseRecorder{
		"header": a.do("POST", "/webhooks/evolution/v2", `{"contato":"5511","texto":"1","prov":"h"}`, map[string]string{"X-Evolution-Webhook-Secret": "s3"}),
		"query":  a.do("POST", "/webhooks/evolution/v2?webhookSecret=s3", `{"contato":"5511","texto":"2","prov":"q"}`, nil),
		"bearer": a.do("POST", "/webhooks/evolution/v2", `{"contato":"5511","texto":"3","prov":"b"}`, map[string]string{"Authorization": "Bearer s3"}),
	} {
		if w.Code != 200 {
			t.Fatalf("%s: %d", name, w.Code)
		}
	}
}

func TestWebhookPayloadInvalido400(t *testing.T) {
	a := novoAmbiente(t, "")
	if w := a.do("POST", "/webhooks/evolution/v2", `{"invalido":true}`, nil); w.Code != 400 {
		t.Fatalf("esperava 400, got %d", w.Code)
	}
}

func TestClienteRegistradoEPendente(t *testing.T) {
	a := novoAmbiente(t, "")
	r := a.webhook(t, `{"contato":"5511","texto":"oi","prov":"a1"}`)
	if r.Status != "registrado" || r.ConversaID == "" {
		t.Fatalf("%+v", r)
	}
	c, _ := a.s.Obter(context.Background(), r.ConversaID)
	if c.Status != conversa.StatusBot || c.PendenteDesde == nil || c.Canal != "WHATSAPP" {
		t.Fatalf("%+v", c)
	}
	ms, _ := a.s.Historico(context.Background(), c.ID, 10)
	if len(ms) != 1 || ms[0].Autor != conversa.AutorCliente || ms[0].Texto != "oi" {
		t.Fatalf("%+v", ms)
	}
}

func TestDuplicada(t *testing.T) {
	a := novoAmbiente(t, "")
	a.webhook(t, `{"contato":"5511","texto":"oi","prov":"a1"}`)
	if r := a.webhook(t, `{"contato":"5511","texto":"oi","prov":"a1"}`); r.Status != "duplicada" {
		t.Fatalf("%+v", r)
	}
}

func TestEventoIgnorado(t *testing.T) {
	a := novoAmbiente(t, "")
	if r := a.webhook(t, `{"ignorar":true}`); r.Status != "ignorado" {
		t.Fatalf("%+v", r)
	}
	lista, _ := a.s.Listar(context.Background(), conversa.FiltroLista{})
	if len(lista) != 0 {
		t.Fatal("nao deveria criar conversa")
	}
}

func criarConversa(t *testing.T, a *ambiente, contato string) conversa.Conversa {
	t.Helper()
	c, _, _, err := a.s.RegistrarEntrada(context.Background(), conversa.NovaEntrada{Canal: "WHATSAPP", Contato: contato, Telefone: contato,
		Autor: conversa.AutorCliente, Tipo: conversa.TipoTexto, Texto: "ola", ProvedorID: "in-" + contato})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEcoBotPorProvedorID(t *testing.T) {
	a := novoAmbiente(t, "")
	c := criarConversa(t, a, "5511")
	if _, err := a.s.RegistrarSaida(context.Background(), c.ID, conversa.AutorBot, "resposta", "out-1", ""); err != nil {
		t.Fatal(err)
	}
	r := a.webhook(t, `{"contato":"5511","texto":"outro texto","prov":"out-1","from_me":true}`)
	if r.Status != "ignorado" {
		t.Fatalf("%+v", r)
	}
	got, _ := a.s.Obter(context.Background(), c.ID)
	if got.Status != conversa.StatusBot {
		t.Fatalf("status %s", got.Status)
	}
}

func TestEcoBotPorTextoRecente(t *testing.T) {
	a := novoAmbiente(t, "")
	c := criarConversa(t, a, "5511")
	if _, err := a.s.RegistrarSaida(context.Background(), c.ID, conversa.AutorBot, "Ola! Como posso ajudar?", "", ""); err != nil {
		t.Fatal(err)
	}
	a.now = a.now.Add(10 * time.Second)
	r := a.webhook(t, `{"contato":"5511","texto":"Ola! Como posso ajudar? ","prov":"zz","from_me":true}`)
	if r.Status != "ignorado" {
		t.Fatalf("%+v", r)
	}
	// fora da janela: vira mensagem humana
	a.now = a.now.Add(5 * time.Minute)
	r = a.webhook(t, `{"contato":"5511","texto":"Ola! Como posso ajudar?","prov":"zz2","from_me":true}`)
	if r.Status != "registrado" {
		t.Fatalf("%+v", r)
	}
}

func TestFromMeHumanoPausa(t *testing.T) {
	a := novoAmbiente(t, "")
	c := criarConversa(t, a, "5511")
	r := a.webhook(t, `{"contato":"5511","texto":"Oi, aqui e a atendente","prov":"h1","from_me":true}`)
	if r.Status != "registrado" {
		t.Fatalf("%+v", r)
	}
	got, _ := a.s.Obter(context.Background(), c.ID)
	if got.Status != conversa.StatusHumano || got.PendenteDesde != nil {
		t.Fatalf("%+v", got)
	}
	ms, _ := a.s.Historico(context.Background(), c.ID, 10)
	if ms[len(ms)-1].Autor != conversa.AutorHumano {
		t.Fatalf("%+v", ms)
	}
}

func TestRotasEquipe(t *testing.T) {
	a := novoAmbiente(t, "")
	c := criarConversa(t, a, "5511")
	u := map[string]string{"X-Test-User": "user-1"}

	// listar
	w := a.do("GET", "/atendimento/conversas?status=BOT&q=5511", "", u)
	if w.Code != 200 {
		t.Fatalf("listar %d %s", w.Code, w.Body)
	}
	var lista struct {
		Conversas []map[string]any `json:"conversas"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &lista)
	if len(lista.Conversas) != 1 || lista.Conversas[0]["pendente"] != true || lista.Conversas[0]["id"] != c.ID {
		t.Fatalf("%s", w.Body)
	}
	if um, _ := lista.Conversas[0]["ultima_mensagem"].(map[string]any); um["texto"] != "ola" {
		t.Fatalf("ultima_mensagem: %s", w.Body)
	}
	if w := a.do("GET", "/atendimento/conversas?status=XYZ", "", u); w.Code != 400 {
		t.Fatalf("status invalido: %d", w.Code)
	}

	// detalhe
	w = a.do("GET", "/atendimento/conversas/"+c.ID, "", u)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mensagens"`) || !strings.Contains(w.Body.String(), `"turnos"`) {
		t.Fatalf("detalhe %d %s", w.Code, w.Body)
	}

	// responder exige HUMANO
	w = a.do("POST", "/atendimento/conversas/"+c.ID+"/responder", `{"texto":"oi"}`, u)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "assuma a conversa antes de responder") {
		t.Fatalf("responder sem assumir: %d %s", w.Code, w.Body)
	}

	// assumir sem usuario -> 401
	if w := a.do("POST", "/atendimento/conversas/"+c.ID+"/assumir", "", nil); w.Code != 401 {
		t.Fatalf("assumir sem usuario: %d", w.Code)
	}
	// assumir
	w = a.do("POST", "/atendimento/conversas/"+c.ID+"/assumir", "", u)
	if w.Code != 200 {
		t.Fatalf("assumir %d %s", w.Code, w.Body)
	}
	got, _ := a.s.Obter(context.Background(), c.ID)
	if got.Status != conversa.StatusHumano || got.ResponsavelID != "user-1" || got.Estado.MotivoHumano != "assumida pela equipe" {
		t.Fatalf("%+v", got)
	}

	// responder
	w = a.do("POST", "/atendimento/conversas/"+c.ID+"/responder", `{"texto":"  Ola, tudo bem?  "}`, u)
	if w.Code != 200 {
		t.Fatalf("responder %d %s", w.Code, w.Body)
	}
	if len(a.c.enviados) != 1 || a.c.enviados[0] != "5511|Ola, tudo bem?" {
		t.Fatalf("%v", a.c.enviados)
	}
	if ok, _ := a.s.ProvedorIDConhecido(context.Background(), "out-1"); !ok {
		t.Fatal("saida deveria estar registrada com provedor id")
	}
	if w := a.do("POST", "/atendimento/conversas/"+c.ID+"/responder", `{"texto":" "}`, u); w.Code != 400 {
		t.Fatalf("texto vazio: %d", w.Code)
	}

	// devolver
	w = a.do("POST", "/atendimento/conversas/"+c.ID+"/devolver", "", u)
	got, _ = a.s.Obter(context.Background(), c.ID)
	if w.Code != 200 || got.Status != conversa.StatusBot || got.ResponsavelID != "" {
		t.Fatalf("devolver %d %+v", w.Code, got)
	}

	// encerrar
	w = a.do("POST", "/atendimento/conversas/"+c.ID+"/encerrar", "", u)
	got, _ = a.s.Obter(context.Background(), c.ID)
	if w.Code != 200 || got.Status != conversa.StatusEncerrada {
		t.Fatalf("encerrar %d %+v", w.Code, got)
	}
}

func TestRotasEquipe404(t *testing.T) {
	a := novoAmbiente(t, "")
	u := map[string]string{"X-Test-User": "user-1"}
	for _, rq := range [][2]string{
		{"GET", "/atendimento/conversas/nao-existe"},
		{"POST", "/atendimento/conversas/nao-existe/assumir"},
		{"POST", "/atendimento/conversas/nao-existe/devolver"},
		{"POST", "/atendimento/conversas/nao-existe/encerrar"},
	} {
		if w := a.do(rq[0], rq[1], "", u); w.Code != 404 {
			t.Fatalf("%v: %d %s", rq, w.Code, w.Body)
		}
	}
	if w := a.do("POST", "/atendimento/conversas/nao-existe/responder", `{"texto":"x"}`, u); w.Code != 404 {
		t.Fatalf("responder: %d", w.Code)
	}
}

func TestAtendeAllowlist(t *testing.T) {
	novo := func(lista []string) *Handler {
		return NovoHandler(Deps{Store: conversa.NewStoreMem(time.Now), Canal: &canalFake{}, TelefonesPermitidos: lista, Log: log.New(io.Discard, "", 0)})
	}
	ctx := context.Background()
	casos := []struct {
		nome  string
		lista []string
		corpo string
		want  bool
	}{
		{"lista vazia aceita todos", nil, `{"contato":"5549111"}`, true},
		{"telefone na lista", []string{"5549111"}, `{"contato":"5549111"}`, true},
		{"telefone fora da lista", []string{"5549222"}, `{"contato":"5549111"}`, false},
		{"evento ignorado", nil, `{"ignorar":true}`, false},
		{"payload invalido", nil, `{"invalido":true}`, false},
		{"ignorado com lista", []string{"5549111"}, `{"ignorar":true,"contato":"5549111"}`, false},
	}
	for _, c := range casos {
		if got := novo(c.lista).Atende(ctx, []byte(c.corpo)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.nome, got, c.want)
		}
	}
}
