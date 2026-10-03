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

	"schumacher-tur/api/internal/atendimento/llm"
)

// Modelo que recusa guided_json: o cliente refaz com o schema no prompt e,
// nos proximos pedidos, ja manda assim (sem pagar o 400 de novo).
func TestRecusaGuidedJSONCaiParaPrompt(t *testing.T) {
	var comNvext, semNvext atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"nvext"`) {
			comNvext.Add(1)
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"unknown field ` + "`guided_json`" + `"}}`))
			return
		}
		semNvext.Add(1)
		if !strings.Contains(string(b), "Responda APENAS com um JSON") {
			t.Errorf("o schema deveria ir no prompt: %s", b)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "aqui: {\"ok\":true} pronto"}}}})
	}))
	defer srv.Close()
	c := Novo(Config{APIKey: "k", BaseURL: srv.URL, Modelo: "m", ModoJSON: ModoNvext})
	p := llm.Pedido{Instrucoes: "x", Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "oi"}}, SaidaJSON: json.RawMessage(`{"type":"object"}`)}
	for i := 0; i < 2; i++ {
		r, err := c.Gerar(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		if r.Texto != `{"ok":true}` {
			t.Fatalf("texto=%q", r.Texto)
		}
	}
	if comNvext.Load() != 1 || semNvext.Load() != 2 {
		t.Fatalf("com nvext=%d sem=%d (esperado 1 e 2)", comNvext.Load(), semNvext.Load())
	}
}
