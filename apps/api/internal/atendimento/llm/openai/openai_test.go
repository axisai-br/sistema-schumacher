package openai

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

func novoTeste(url string) *Cliente {
	c := Novo(Config{APIKey: "sk-secreta", BaseURL: url, Modelo: "m-padrao"})
	c.backoff = []time.Duration{time.Millisecond, time.Millisecond}
	return c
}

func TestPayloadECampos(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/responses" {
			t.Errorf("path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Write([]byte(`{"model":"m-real","output":[{"type":"message","content":[{"type":"output_text","text":"Oi "},{"type":"output_text","text":"tudo bem"}]}],"usage":{"input_tokens":10,"output_tokens":3}}`))
	}))
	defer srv.Close()
	c := novoTeste(srv.URL)
	resp, err := c.Gerar(context.Background(), llm.Pedido{
		Instrucoes: "seja breve",
		Mensagens: []llm.Mensagem{
			{Papel: llm.PapelUsuario, Texto: "oi"},
			{Papel: llm.PapelAssistente, Texto: "buscando", Chamadas: []llm.ChamadaFerramenta{{ID: "c1", Nome: "buscar", Argumentos: json.RawMessage(`{"a":1}`)}}},
			{Papel: llm.PapelFerramenta, ChamadaID: "c1", Texto: `{"ok":true}`},
		},
		Ferramentas: []llm.DefFerramenta{{Nome: "buscar", Descricao: "d", Parametros: json.RawMessage(`{"type":"object"}`)}},
		SaidaJSON:   json.RawMessage(`{"type":"object"}`),
		MaxTokens:   200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Texto != "Oi tudo bem" || resp.TokensEntrada != 10 || resp.TokensSaida != 3 || resp.Modelo != "m-real" {
		t.Fatalf("resp %+v", resp)
	}
	if auth != "Bearer sk-secreta" {
		t.Fatal("auth")
	}
	if got["model"] != "m-padrao" || got["instructions"] != "seja breve" || got["store"] != false || got["max_output_tokens"].(float64) != 200 {
		t.Fatalf("payload %v", got)
	}
	in := got["input"].([]any)
	if len(in) != 4 {
		t.Fatalf("input %v", in)
	}
	if in[0].(map[string]any)["role"] != "user" {
		t.Fatal("user")
	}
	if in[1].(map[string]any)["role"] != "assistant" {
		t.Fatal("assistant")
	}
	fc := in[2].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "c1" || fc["name"] != "buscar" || fc["arguments"] != `{"a":1}` {
		t.Fatalf("fc %v", fc)
	}
	fo := in[3].(map[string]any)
	if fo["type"] != "function_call_output" || fo["call_id"] != "c1" || fo["output"] != `{"ok":true}` {
		t.Fatalf("fo %v", fo)
	}
	tool := got["tools"].([]any)[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "buscar" || tool["strict"] != false {
		t.Fatalf("tool %v", tool)
	}
	f := got["text"].(map[string]any)["format"].(map[string]any)
	if f["type"] != "json_schema" || f["name"] != "saida" || f["strict"] != true {
		t.Fatalf("format %v", f)
	}
}

func TestModeloDoPedido(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Write([]byte(`{"output":[]}`))
	}))
	defer srv.Close()
	if _, err := novoTeste(srv.URL).Gerar(context.Background(), llm.Pedido{Modelo: "outro"}); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "outro" {
		t.Fatal(got["model"])
	}
}

func TestParseFunctionCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"output":[{"type":"function_call","call_id":"x9","name":"buscar_viagens","arguments":"{\"destino\":\"Chapeco\"}"}]}`))
	}))
	defer srv.Close()
	resp, err := novoTeste(srv.URL).Gerar(context.Background(), llm.Pedido{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Chamadas) != 1 || resp.Chamadas[0].ID != "x9" || resp.Chamadas[0].Nome != "buscar_viagens" ||
		string(resp.Chamadas[0].Argumentos) != `{"destino":"Chapeco"}` {
		t.Fatalf("%+v", resp.Chamadas)
	}
}

func TestRetryEm500(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()
	resp, err := novoTeste(srv.URL).Gerar(context.Background(), llm.Pedido{})
	if err != nil || resp.Texto != "ok" || n != 3 {
		t.Fatalf("err=%v n=%d resp=%+v", err, n, resp)
	}
}

func TestRetryEsgotado(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(429)
	}))
	defer srv.Close()
	if _, err := novoTeste(srv.URL).Gerar(context.Background(), llm.Pedido{}); err == nil || n != 3 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestSemRetryEm400ESemChaveNoErro(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"chave sk-secreta invalida"}`))
	}))
	defer srv.Close()
	_, err := novoTeste(srv.URL).Gerar(context.Background(), llm.Pedido{})
	if err == nil || n != 1 {
		t.Fatalf("err=%v n=%d", err, n)
	}
	if strings.Contains(err.Error(), "sk-secreta") {
		t.Fatal("vazou a chave")
	}
}

func TestRetryEmErroDeRede(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	_, err := novoTeste(url).Gerar(context.Background(), llm.Pedido{})
	if err == nil || strings.Contains(err.Error(), "sk-secreta") {
		t.Fatal(err)
	}
}
