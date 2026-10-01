package hedge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/llm/chatcompat"
)

// servidor responde com o texto dado depois de atraso (ou com o status de erro,
// se status != 0). Conta chamadas e requisicoes canceladas pelo cliente.
func servidor(t *testing.T, texto string, atraso time.Duration, status int, chamadas, cancelados *atomic.Int32, modelos chan<- string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas.Add(1)
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if modelos != nil {
			s, _ := m["model"].(string)
			modelos <- s
		}
		select {
		case <-time.After(atraso):
		case <-r.Context().Done():
			if cancelados != nil {
				cancelados.Add(1)
			}
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"erro":"x"}`))
			return
		}
		resp, _ := json.Marshal(map[string]any{"model": texto, "choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": texto}}}})
		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func cliente(url, nome string) llm.Modelo {
	return chatcompat.Novo(chatcompat.Config{APIKey: "k", BaseURL: url, Modelo: nome})
}

func TestPrincipalRapidoNaoAcionaReserva(t *testing.T) {
	var cp, cr atomic.Int32
	p := servidor(t, "principal", 0, 0, &cp, nil, nil)
	r := servidor(t, "reserva", 0, 0, &cr, nil, nil)
	m := Novo(Config{Principal: cliente(p.URL, "mp"), Reserva: cliente(r.URL, "mr"), HedgeApos: 300 * time.Millisecond})
	resp, err := m.Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "oi"}}})
	if err != nil || resp.Texto != "principal" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	time.Sleep(400 * time.Millisecond)
	if cr.Load() != 0 {
		t.Errorf("reserva nao deveria ser chamado: %d", cr.Load())
	}
}

func TestPrincipalLentoReservaVenceECancelaPrincipal(t *testing.T) {
	var cp, cr, cancel atomic.Int32
	modelos := make(chan string, 4)
	p := servidor(t, "principal", 3*time.Second, 0, &cp, &cancel, nil)
	r := servidor(t, "reserva", 10*time.Millisecond, 0, &cr, nil, modelos)
	m := Novo(Config{Principal: cliente(p.URL, "mp"), Reserva: cliente(r.URL, "mr"), NomeReserva: "mr", HedgeApos: 100 * time.Millisecond})
	t0 := time.Now()
	resp, err := m.Gerar(context.Background(), llm.Pedido{Modelo: "mp", Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "oi"}}})
	if err != nil || resp.Texto != "reserva" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if d := time.Since(t0); d > 1500*time.Millisecond {
		t.Errorf("demorou %v", d)
	}
	if got := <-modelos; got != "mr" {
		t.Errorf("reserva recebeu modelo %q (nao pode herdar o do principal)", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for cancel.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if cancel.Load() != 1 {
		t.Errorf("principal deveria ter sido cancelado")
	}
}

func TestPrincipalVenceDepoisDoHedge(t *testing.T) {
	var cp, cr atomic.Int32
	p := servidor(t, "principal", 250*time.Millisecond, 0, &cp, nil, nil)
	r := servidor(t, "reserva", 2*time.Second, 0, &cr, nil, nil)
	m := Novo(Config{Principal: cliente(p.URL, "mp"), Reserva: cliente(r.URL, "mr"), HedgeApos: 100 * time.Millisecond})
	resp, err := m.Gerar(context.Background(), llm.Pedido{})
	if err != nil || resp.Texto != "principal" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if cr.Load() != 1 {
		t.Errorf("reserva deveria ter sido disparado: %d", cr.Load())
	}
}

func TestPrincipalFalhaUsaReservaImediatamente(t *testing.T) {
	var cp, cr atomic.Int32
	p := servidor(t, "x", 0, 400, &cp, nil, nil) // 400 nao tem retry
	r := servidor(t, "reserva", 0, 0, &cr, nil, nil)
	m := Novo(Config{Principal: cliente(p.URL, "mp"), Reserva: cliente(r.URL, "mr"), HedgeApos: 30 * time.Second})
	t0 := time.Now()
	resp, err := m.Gerar(context.Background(), llm.Pedido{})
	if err != nil || resp.Texto != "reserva" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if time.Since(t0) > 2*time.Second {
		t.Errorf("deveria usar o reserva sem esperar o hedge")
	}
}

func TestPrincipalFalhaDepoisDoHedgeEsperaReserva(t *testing.T) {
	var cp, cr atomic.Int32
	p := servidor(t, "x", 200*time.Millisecond, 400, &cp, nil, nil)
	r := servidor(t, "reserva", 400*time.Millisecond, 0, &cr, nil, nil)
	m := Novo(Config{Principal: cliente(p.URL, "mp"), Reserva: cliente(r.URL, "mr"), HedgeApos: 50 * time.Millisecond})
	resp, err := m.Gerar(context.Background(), llm.Pedido{})
	if err != nil || resp.Texto != "reserva" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestAmbosFalham(t *testing.T) {
	var cp, cr atomic.Int32
	p := servidor(t, "x", 0, 400, &cp, nil, nil)
	r := servidor(t, "x", 0, 400, &cr, nil, nil)
	m := Novo(Config{Principal: cliente(p.URL, "mp"), Reserva: cliente(r.URL, "mr"), HedgeApos: time.Second})
	if _, err := m.Gerar(context.Background(), llm.Pedido{}); err == nil {
		t.Fatal("esperava erro")
	}
	if cp.Load() != 1 || cr.Load() != 1 {
		t.Errorf("chamadas: %d %d", cp.Load(), cr.Load())
	}
}

func TestContextoCancelado(t *testing.T) {
	var cp, cr atomic.Int32
	p := servidor(t, "p", 3*time.Second, 0, &cp, nil, nil)
	r := servidor(t, "r", 3*time.Second, 0, &cr, nil, nil)
	m := Novo(Config{Principal: cliente(p.URL, "mp"), Reserva: cliente(r.URL, "mr"), HedgeApos: 50 * time.Millisecond})
	ctx, cancelar := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelar()
	t0 := time.Now()
	if _, err := m.Gerar(ctx, llm.Pedido{}); err == nil {
		t.Fatal("esperava erro")
	}
	if time.Since(t0) > time.Second {
		t.Errorf("nao respeitou o contexto")
	}
}

func TestSemReservaDevolvePrincipal(t *testing.T) {
	var cp atomic.Int32
	p := cliente(servidor(t, "p", 0, 0, &cp, nil, nil).URL, "mp")
	if Novo(Config{Principal: p}) != p {
		t.Error("sem reserva deveria devolver o principal")
	}
}
