package chatcompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
)

const chave = "nvapi-SEGREDO-123"

// servidor devolve um httptest que captura o ultimo corpo e responde com resp.
func servidor(t *testing.T, resp string, corpo *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("rota inesperada %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+chave {
			t.Errorf("auth = %q", got)
		}
		if corpo != nil {
			b, _ := io.ReadAll(r.Body)
			m := map[string]any{}
			if err := json.Unmarshal(b, &m); err != nil {
				t.Errorf("corpo invalido: %v", err)
			}
			*corpo = m
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func novo(url, modo string) *Cliente {
	c := Novo(Config{APIKey: chave, BaseURL: url, Modelo: "meta/llama-3.3-70b-instruct", ModoJSON: modo})
	c.backoff = []time.Duration{time.Millisecond, time.Millisecond}
	return c
}

func resp(content string) string {
	b, _ := json.Marshal(map[string]any{"model": "m", "choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}})
	return string(b)
}

var ferrs = []llm.DefFerramenta{{Nome: "buscar_viagens", Descricao: "busca", Parametros: json.RawMessage(`{"type":"object","properties":{"destino":{"type":"string"}}}`)}}

func TestPayloadSystemToolsHistorico(t *testing.T) {
	var corpo map[string]any
	srv := servidor(t, resp("ok"), &corpo)
	_, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{
		Instrucoes:  "Voce e a Sol.",
		Ferramentas: ferrs,
		Mensagens: []llm.Mensagem{
			{Papel: llm.PapelUsuario, Texto: "quero ir a Videira"},
			{Papel: llm.PapelAssistente, Chamadas: []llm.ChamadaFerramenta{{ID: "c1", Nome: "buscar_viagens", Argumentos: json.RawMessage(`{"destino":"Videira"}`)}}},
			{Papel: llm.PapelFerramenta, ChamadaID: "c1", Texto: `{"viagens":[]}`},
			{Papel: llm.PapelAssistente, Texto: "nao achei"},
			{Papel: llm.PapelSistema, Texto: "lembrete"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if corpo["model"] != "meta/llama-3.3-70b-instruct" || corpo["tool_choice"] != "auto" || corpo["stream"] != false ||
		corpo["max_tokens"].(float64) != 1024 || corpo["temperature"].(float64) != 0.2 {
		t.Errorf("campos basicos: %v", corpo)
	}
	if _, ok := corpo["reasoning_effort"]; ok {
		t.Error("reasoning_effort nao deveria ser enviado")
	}
	msgs := corpo["messages"].([]any)
	if len(msgs) != 6 {
		t.Fatalf("msgs = %d", len(msgs))
	}
	m0 := msgs[0].(map[string]any)
	if m0["role"] != "system" || m0["content"] != "Voce e a Sol." {
		t.Errorf("system: %v", m0)
	}
	m2 := msgs[2].(map[string]any)
	tc := m2["tool_calls"].([]any)[0].(map[string]any)
	fn := tc["function"].(map[string]any)
	if m2["role"] != "assistant" || m2["content"] != nil || tc["id"] != "c1" || tc["type"] != "function" ||
		fn["name"] != "buscar_viagens" || fn["arguments"] != `{"destino":"Videira"}` {
		t.Errorf("assistant tool_calls: %v", m2)
	}
	m3 := msgs[3].(map[string]any)
	if m3["role"] != "tool" || m3["tool_call_id"] != "c1" || m3["content"] != `{"viagens":[]}` {
		t.Errorf("tool: %v", m3)
	}
	if msgs[5].(map[string]any)["role"] != "system" {
		t.Errorf("system extra: %v", msgs[5])
	}
	tool := corpo["tools"].([]any)[0].(map[string]any)
	f := tool["function"].(map[string]any)
	if tool["type"] != "function" || f["name"] != "buscar_viagens" || f["description"] != "busca" || f["parameters"] == nil {
		t.Errorf("tools: %v", tool)
	}
}

func TestSemFerramentasSemToolChoice(t *testing.T) {
	var corpo map[string]any
	srv := servidor(t, resp("oi"), &corpo)
	if _, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "oi"}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := corpo["tool_choice"]; ok {
		t.Error("tool_choice sem ferramentas")
	}
	if _, ok := corpo["tools"]; ok {
		t.Error("tools sem ferramentas")
	}
}

func TestRaciocinioSeedTemperatura(t *testing.T) {
	var corpo map[string]any
	srv := servidor(t, `{"model":"kimi","choices":[{"message":{"content":"ola","reasoning_content":"pensando...","reasoning":"x"}}],"usage":{"prompt_tokens":7,"completion_tokens":9}}`, &corpo)
	temp, seed := 0.6, 42
	c := Novo(Config{APIKey: chave, BaseURL: srv.URL, Modelo: "moonshotai/kimi-k3", EsforcoRaciocinio: "low", Temperatura: &temp, Seed: &seed})
	r, err := c.Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "oi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if corpo["reasoning_effort"] != "low" || corpo["seed"].(float64) != 42 || corpo["temperature"].(float64) != 0.6 || corpo["max_tokens"].(float64) != 4096 {
		t.Errorf("payload: %v", corpo)
	}
	if r.Texto != "ola" || r.TokensEntrada != 7 || r.TokensSaida != 9 || r.Modelo != "kimi" {
		t.Errorf("resposta: %+v", r)
	}
}

func TestParseToolCallsArgumentosStringEObjeto(t *testing.T) {
	srv := servidor(t, `{"model":"m","choices":[{"message":{"content":null,"tool_calls":[
		{"id":"a1","type":"function","function":{"name":"buscar_viagens","arguments":"{\"destino\":\"Videira\"}"}},
		{"id":"","type":"function","function":{"name":"buscar_viagens","arguments":{"destino":"Chapeco"}}}]}}],
		"usage":{"prompt_tokens":10,"completion_tokens":5}}`, nil)
	r, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Ferramentas: ferrs, Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Texto != "" || len(r.Chamadas) != 2 || r.TokensEntrada != 10 || r.TokensSaida != 5 || r.Modelo != "m" {
		t.Fatalf("resposta: %+v", r)
	}
	if r.Chamadas[0].ID != "a1" || string(r.Chamadas[0].Argumentos) != `{"destino":"Videira"}` {
		t.Errorf("chamada 0: %+v", r.Chamadas[0])
	}
	if r.Chamadas[1].ID != "call_2" || string(r.Chamadas[1].Argumentos) != `{"destino":"Chapeco"}` {
		t.Errorf("chamada 1: %+v", r.Chamadas[1])
	}
}

func TestJSONGuiado(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}}}`)
	ped := llm.Pedido{Instrucoes: "juiz", SaidaJSON: schema, Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}}

	var corpo map[string]any
	srv := servidor(t, resp(`{"a":"1"}`), &corpo)
	r, err := novo(srv.URL, "nvext").Gerar(context.Background(), ped)
	if err != nil || r.Texto != `{"a":"1"}` {
		t.Fatalf("nvext: %v %+v", err, r)
	}
	if g := corpo["nvext"].(map[string]any)["guided_json"].(map[string]any); g["type"] != "object" {
		t.Errorf("guided_json: %v", corpo["nvext"])
	}
	if _, ok := corpo["response_format"]; ok {
		t.Error("nvext nao deve enviar response_format")
	}

	srv = servidor(t, resp(`{"a":"1"}`), &corpo)
	if _, err := novo(srv.URL, "response_format").Gerar(context.Background(), ped); err != nil {
		t.Fatal(err)
	}
	rf := corpo["response_format"].(map[string]any)
	js := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || js["name"] != "saida" || js["schema"] == nil {
		t.Errorf("response_format: %v", rf)
	}
	if _, ok := corpo["nvext"]; ok {
		t.Error("nao deve enviar nvext")
	}

	srv = servidor(t, resp("Claro!\n```json\n{\"a\": \"2\"}\n```\nEspero ter ajudado."), &corpo)
	r, err = novo(srv.URL, "prompt").Gerar(context.Background(), ped)
	if err != nil || r.Texto != `{"a": "2"}` {
		t.Fatalf("prompt: %v %q", err, r.Texto)
	}
	sys := corpo["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.HasPrefix(sys, "juiz") || !strings.Contains(sys, "Responda APENAS com um JSON válido seguindo este schema: {") {
		t.Errorf("system: %q", sys)
	}
	if _, ok := corpo["nvext"]; ok {
		t.Error("prompt nao envia nvext")
	}
}

func TestSaidaJSONIgnoradaComFerramentas(t *testing.T) {
	var corpo map[string]any
	srv := servidor(t, resp("x"), &corpo)
	_, err := novo(srv.URL, "nvext").Gerar(context.Background(), llm.Pedido{
		Ferramentas: ferrs, SaidaJSON: json.RawMessage(`{"type":"object"}`), Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := corpo["nvext"]; ok {
		t.Error("nvext com ferramentas")
	}
}

func TestToolCallComoTexto(t *testing.T) {
	casos := map[string]string{
		"json puro":  `{"name":"buscar_viagens","parameters":{"destino":"Videira"}}`,
		"tag":        `<tool_call>{"name":"buscar_viagens","arguments":{"destino":"Videira"}}</tool_call>`,
		"cerca json": "```json\n{\"name\":\"buscar_viagens\",\"parameters\":{\"destino\":\"Videira\"}}\n```",
	}
	for nome, conteudo := range casos {
		srv := servidor(t, resp(conteudo), nil)
		r, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Ferramentas: ferrs, Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Chamadas) != 1 || r.Chamadas[0].Nome != "buscar_viagens" || string(r.Chamadas[0].Argumentos) != `{"destino":"Videira"}` || r.Texto != "" {
			t.Errorf("%s: %+v", nome, r)
		}
	}
	// nome desconhecido ou sem ferramentas: continua texto
	srv := servidor(t, resp(`{"name":"outra","parameters":{}}`), nil)
	r, _ := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Ferramentas: ferrs, Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if len(r.Chamadas) != 0 || r.Texto == "" {
		t.Errorf("desconhecida: %+v", r)
	}
	srv = servidor(t, resp(`{"name":"buscar_viagens","parameters":{}}`), nil)
	r, _ = novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if len(r.Chamadas) != 0 || r.Texto == "" {
		t.Errorf("sem ferramentas: %+v", r)
	}
}

func TestPensamentoRemovido(t *testing.T) {
	srv := servidor(t, resp("<think>hmm, vou pensar\nmuito</think>\n\nOi, tudo bem?"), nil)
	r, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err != nil || r.Texto != "Oi, tudo bem?" {
		t.Errorf("%v %q", err, r.Texto)
	}
}

func TestContentNull(t *testing.T) {
	srv := servidor(t, `{"choices":[{"message":{"content":null}}]}`, nil)
	r, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err != nil || r.Texto != "" || len(r.Chamadas) != 0 {
		t.Errorf("%v %+v", err, r)
	}
}

func TestRetry503(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			http.Error(w, "indisponivel", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(resp("depois")))
	}))
	defer srv.Close()
	r, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err != nil || r.Texto != "depois" || n != 3 {
		t.Errorf("err=%v texto=%q tentativas=%d", err, r.Texto, n)
	}
}

func TestSemRetry400EChaveNoErro(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"modelo invalido, chave ` + chave + ` rejeitada ` + strings.Repeat("x", 500) + `"}`))
	}))
	defer srv.Close()
	_, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err == nil || n != 1 {
		t.Fatalf("err=%v tentativas=%d", err, n)
	}
	if strings.Contains(err.Error(), chave) || !strings.Contains(err.Error(), "400") {
		t.Errorf("erro: %v", err)
	}
	if len(err.Error()) > 350 {
		t.Errorf("erro longo demais: %d", len(err.Error()))
	}
}

func TestRetryEsgotadoEErroDeRede(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		http.Error(w, "429 "+chave, http.StatusTooManyRequests)
	}))
	_, err := novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err == nil || n != 3 || strings.Contains(err.Error(), chave) {
		t.Errorf("err=%v tentativas=%d", err, n)
	}
	srv.Close() // conexao recusada: erro de rede, tambem tenta 3 vezes e nao vaza a chave
	_, err = novo(srv.URL, "").Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err == nil || strings.Contains(err.Error(), chave) {
		t.Errorf("rede: %v", err)
	}
}

func TestSemChave(t *testing.T) {
	_, err := Novo(Config{BaseURL: "http://x"}).Gerar(context.Background(), llm.Pedido{})
	if err == nil {
		t.Error("esperava erro")
	}
}
