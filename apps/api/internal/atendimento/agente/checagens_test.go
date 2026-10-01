package agente

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
)

func TestItensSemOrigem(t *testing.T) {
	fontes := []string{
		`{"opcoes":[{"data":"2026-10-15","horario":"08:00:00","preco":1100.00}]}`,
		"Catalogo: Chapecó R$ 950,00",
	}
	casos := []struct {
		texto  string
		faltam int
	}{
		{"Sai dia 15/10 às 08:00 por R$ 1.100", 0},
		{"Sai dia 15/10/2026 às 8h por R$ 1100,00", 0},
		{"Custa 950 reais e sai 2026-10-15 às 08h00", 0},
		{"Custa R$ 950,00", 0},
		{"Custa R$ 999,00", 1},
		{"Sai dia 16/10", 1},
		{"Sai às 09:30", 1},
		{"Sai às 9h", 1},
		{"Tem 3 vagas, 2 pessoas", 0},
		{"R$ 1.100 dia 15/10 e R$ 500 dia 20/11 às 10:15", 3},
	}
	for _, c := range casos {
		got := itensSemOrigem(c.texto, fontes, nil)
		if len(got) != c.faltam {
			t.Errorf("%q: faltam %d (%v), esperado %d", c.texto, len(got), listarItens(got), c.faltam)
		}
	}
	// data dita pelo cliente pode ser repetida; valor nao
	if got := itensSemOrigem("Confirmo dia 20/11?", fontes, []string{"quero dia 20/11"}); len(got) != 0 {
		t.Fatal("data do cliente deveria valer")
	}
	if got := itensSemOrigem("R$ 500", fontes, []string{"pago R$ 500"}); len(got) != 1 {
		t.Fatal("valor do cliente nao vale")
	}
}

func TestSimilaridade(t *testing.T) {
	if similaridade("Qual é a sua cidade de origem?", "qual e a sua cidade de origem") < 0.9 {
		t.Fatal("iguais")
	}
	if similaridade("Qual é a sua cidade de origem?", "Para qual data você quer viajar?") >= 0.9 {
		t.Fatal("diferentes")
	}
}

func TestPedeHumano(t *testing.T) {
	for _, s := range []string{"Preciso de AJUDA", "quero falar com alguém", "me passa um atendente", "falar com uma pessoa"} {
		if !pedeHumano(s) {
			t.Errorf("%q", s)
		}
	}
	for _, s := range []string{"tem viagem pra Chapecó?", "somos 3"} {
		if pedeHumano(s) {
			t.Errorf("%q", s)
		}
	}
}

func msgs(ts ...string) []conversa.Mensagem {
	var out []conversa.Mensagem
	for i, t := range ts {
		a := conversa.AutorCliente
		if i%2 == 1 {
			a = conversa.AutorBot
		}
		out = append(out, conversa.Mensagem{Autor: a, Texto: t})
	}
	return out
}

func TestJuizJev(t *testing.T) {
	var body map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Write([]byte(`{"answers":{"pede_humano":{"noul":0.92},"irritacao":{"score":1.5},"fora_do_assunto":{"noul":0.1}}}`))
	}))
	defer srv.Close()
	j := NovoJuizJev("chave-jev", nil).(*juizJev)
	j.c.url = srv.URL
	av, err := j.Avaliar(context.Background(), msgs("oi", "olá", "quero falar com alguém"))
	if err != nil {
		t.Fatal(err)
	}
	if av.PedeHumano != 0.92 || av.Irritacao != 0.75 || av.ForaDoAssunto != 0.1 {
		t.Fatalf("%+v", av)
	}
	if auth != "Bearer chave-jev" || body["model"] != "jev-latest" {
		t.Fatalf("auth=%q body=%v", auth, body)
	}
	if body["state"] != "CLIENTE: oi\nBOT: olá\nCLIENTE: quero falar com alguém" {
		t.Fatalf("state %q", body["state"])
	}
	q := body["questions"].(map[string]any)
	if q["pede_humano"].(map[string]any)["type"] != "noul" || q["irritacao"].(map[string]any)["type"] != "score" {
		t.Fatalf("questions %v", q)
	}
}

func TestJuizJevRetry429(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(429)
			return
		}
		w.Write([]byte(`{"answers":{"pede_humano":{"noul":0.1},"irritacao":{"score":0},"fora_do_assunto":{"noul":0}}}`))
	}))
	defer srv.Close()
	j := NovoJuizJev("k", nil).(*juizJev)
	j.c.url, j.c.espera = srv.URL, time.Millisecond
	if _, err := j.Avaliar(context.Background(), msgs("oi")); err != nil || n != 2 {
		t.Fatalf("err=%v n=%d", err, n)
	}
	// erro 500: sem retry
	n = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(500)
	}))
	defer srv2.Close()
	j.c.url = srv2.URL
	if _, err := j.Avaliar(context.Background(), msgs("oi")); err == nil || n != 1 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestJuizLLM(t *testing.T) {
	m := &modeloFake{fila: []llm.Resposta{texto(`{"pede_humano":0.8,"irritacao":1.4,"fora_do_assunto":-1}`)}}
	j := NovoJuizLLM(m, "m-juiz")
	av, err := j.Avaliar(context.Background(), msgs("a", "b", "c", "d", "e", "f", "g", "h"))
	if err != nil {
		t.Fatal(err)
	}
	if av.PedeHumano != 0.8 || av.Irritacao != 1 || av.ForaDoAssunto != 0 {
		t.Fatalf("%+v", av)
	}
	p := m.pedidos[0]
	if p.Modelo != "m-juiz" || len(p.SaidaJSON) == 0 || !json.Valid(p.SaidaJSON) {
		t.Fatalf("pedido %+v", p)
	}
	if !contem(p.Mensagens[0].Texto, "CLIENTE: c") || contem(p.Mensagens[0].Texto, ": b") {
		t.Fatalf("deveria ter so as ultimas 6: %q", p.Mensagens[0].Texto)
	}
	m2 := &modeloFake{fila: []llm.Resposta{texto("nao e json")}}
	if _, err := NovoJuizLLM(m2, "").Avaliar(context.Background(), msgs("a")); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestNotificadorWebhook(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("metodo %s", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
	}))
	defer srv.Close()
	n := NovoNotificadorWebhook(srv.URL, nil)
	err := n.AvisarTransferencia(context.Background(), conversa.Conversa{ID: "c1", Telefone: "5511", Nome: "Ana"}, "motivo x", "resumo y")
	if err != nil {
		t.Fatal(err)
	}
	if body["tipo"] != "atendimento_transferido" || body["conversa_id"] != "c1" || body["telefone"] != "5511" ||
		body["nome"] != "Ana" || body["motivo"] != "motivo x" || body["resumo"] != "resumo y" {
		t.Fatalf("body %v", body)
	}
	if err := NovoNotificadorWebhook("", nil).AvisarTransferencia(context.Background(), conversa.Conversa{}, "", ""); err != nil {
		t.Fatal("url vazia e no-op")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	if err := NovoNotificadorWebhook(bad.URL, nil).AvisarTransferencia(context.Background(), conversa.Conversa{}, "", ""); err == nil {
		t.Fatal("esperava erro em 500")
	}
}
