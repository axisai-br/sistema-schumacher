package provedor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/llm/chatcompat"
	"schumacher-tur/api/internal/atendimento/llm/openai"
)

func mapa(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestConfigDoAmbientePadraoNvidia(t *testing.T) {
	c := ConfigDoAmbiente(mapa(map[string]string{"NVIDIA_API_KEY": " k "}))
	if c.Provedor != "nvidia" || c.APIKey != "k" || c.BaseURL != "https://integrate.api.nvidia.com/v1" ||
		c.Modelo != "z-ai/glm-5.3" || c.ModoJSON != "nvext" || c.EsforcoRaciocinio != "low" ||
		c.Temperatura == nil || *c.Temperatura != 0.3 {
		t.Errorf("%+v", c)
	}
	if c.VariavelChave() != "NVIDIA_API_KEY" || c.Descricao() != "nvidia · z-ai/glm-5.3" {
		t.Errorf("%q %q", c.VariavelChave(), c.Descricao())
	}
}

func TestConfigDoAmbienteNvidiaCustom(t *testing.T) {
	c := ConfigDoAmbiente(mapa(map[string]string{
		"LLM_PROVEDOR": "NVIDIA", "NVIDIA_API_KEY": "k", "NVIDIA_BASE_URL": "http://x/v1", "ATENDIMENTO_V2_MODELO": "meta/llama-3.3-70b-instruct",
		"LLM_MODO_JSON": "Prompt", "LLM_REASONING_EFFORT": "HIGH", "LLM_TEMPERATURA": "0.1", "OPENAI_MODEL": "gpt-x",
	}))
	if c.Provedor != "nvidia" || c.BaseURL != "http://x/v1" || c.Modelo != "meta/llama-3.3-70b-instruct" || c.ModoJSON != "prompt" ||
		c.EsforcoRaciocinio != "high" || *c.Temperatura != 0.1 {
		t.Errorf("%+v", c)
	}
	c = ConfigDoAmbiente(mapa(map[string]string{"LLM_TEMPERATURA": "abc"}))
	if *c.Temperatura != 0.3 {
		t.Errorf("temperatura invalida deveria cair no padrao: %v", *c.Temperatura)
	}
}

func TestConfigDoAmbienteOpenAI(t *testing.T) {
	c := ConfigDoAmbiente(mapa(map[string]string{"LLM_PROVEDOR": "openai", "OPENAI_API_KEY": "sk", "NVIDIA_API_KEY": "nv"}))
	if c.Provedor != "openai" || c.APIKey != "sk" || c.BaseURL != "https://api.openai.com/v1" || c.Modelo != "gpt-4.1-mini" || c.ModoJSON != "" {
		t.Errorf("%+v", c)
	}
	if c.VariavelChave() != "OPENAI_API_KEY" || c.Descricao() != "openai · gpt-4.1-mini" {
		t.Errorf("%q %q", c.VariavelChave(), c.Descricao())
	}
	c = ConfigDoAmbiente(mapa(map[string]string{"LLM_PROVEDOR": "openai", "OPENAI_MODEL": "gpt-a"}))
	if c.Modelo != "gpt-a" {
		t.Errorf("OPENAI_MODEL: %q", c.Modelo)
	}
	c = ConfigDoAmbiente(mapa(map[string]string{"LLM_PROVEDOR": "openai", "OPENAI_MODEL": "gpt-a", "ATENDIMENTO_V2_MODELO": "gpt-b", "OPENAI_BASE_URL": "http://o"}))
	if c.Modelo != "gpt-b" || c.BaseURL != "http://o" {
		t.Errorf("%+v", c)
	}
}

func TestNovoTipos(t *testing.T) {
	m, err := Novo(Config{Provedor: "nvidia", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*chatcompat.Cliente); !ok {
		t.Errorf("nvidia: %T", m)
	}
	m, err = Novo(Config{Provedor: "openai", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*openai.Cliente); !ok {
		t.Errorf("openai: %T", m)
	}
}

func TestNovoErros(t *testing.T) {
	if _, err := Novo(Config{Provedor: "nvidia"}); err == nil || !strings.Contains(err.Error(), "NVIDIA_API_KEY") {
		t.Errorf("nvidia sem chave: %v", err)
	}
	if _, err := Novo(Config{Provedor: "openai", APIKey: " "}); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Errorf("openai sem chave: %v", err)
	}
	if _, err := Novo(Config{Provedor: "foo", APIKey: "k"}); err == nil || !strings.Contains(err.Error(), "foo") {
		t.Errorf("desconhecido: %v", err)
	}
}

func TestNvidiaEndToEndOffline(t *testing.T) {
	var auth, path, corpo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		b, _ := io.ReadAll(r.Body)
		corpo = string(b)
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"oi"}}]}`))
	}))
	defer srv.Close()
	m, err := Novo(Config{Provedor: "nvidia", APIKey: "segredo", BaseURL: srv.URL, Modelo: "z-ai/glm-5.3", EsforcoRaciocinio: "low"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Gerar(context.Background(), llm.Pedido{Mensagens: []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: "x"}}})
	if err != nil || r.Texto != "oi" || auth != "Bearer segredo" || path != "/chat/completions" || !strings.Contains(corpo, `"reasoning_effort":"low"`) {
		t.Errorf("err=%v r=%+v auth=%q path=%q corpo=%s", err, r, auth, path, corpo)
	}
}

func TestReservaEHedgeDoAmbiente(t *testing.T) {
	c := ConfigDoAmbiente(mapa(map[string]string{"NVIDIA_API_KEY": "k"}))
	if c.ModeloReserva != "nvidia/nemotron-3.5-lightning-30b-a3b" || c.HedgeApos != 3*time.Second || !c.SemRaciocinioReserva || c.SemRaciocinio {
		t.Errorf("padroes: %+v", c)
	}
	if !strings.Contains(c.DescricaoReserva(), "nemotron-3.5-lightning-30b-a3b") || !strings.Contains(c.DescricaoReserva(), "3s") {
		t.Errorf("descricao reserva: %q", c.DescricaoReserva())
	}
	c = ConfigDoAmbiente(mapa(map[string]string{"LLM_MODELO_RESERVA": "x/y", "LLM_HEDGE_MS": "1500", "LLM_SEM_RACIOCINIO": "false"}))
	if c.ModeloReserva != "x/y" || c.HedgeApos != 1500*time.Millisecond || c.SemRaciocinioReserva || c.SemRaciocinio {
		t.Errorf("custom: %+v", c)
	}
	c = ConfigDoAmbiente(mapa(map[string]string{"LLM_SEM_RACIOCINIO": "true"}))
	if !c.SemRaciocinio || !c.SemRaciocinioReserva {
		t.Errorf("sem raciocinio explicito vale para os dois: %+v", c)
	}
	if c = ConfigDoAmbiente(mapa(map[string]string{"LLM_MODELO_RESERVA": "off"})); c.ModeloReserva != "" || c.DescricaoReserva() != "" {
		t.Errorf("off: %+v", c)
	}
	if c = ConfigDoAmbiente(mapa(map[string]string{"LLM_HEDGE_MS": "0"})); c.HedgeApos != 0 || c.DescricaoReserva() != "" {
		t.Errorf("hedge 0 desliga: %+v", c)
	}
	if c = ConfigDoAmbiente(mapa(map[string]string{"LLM_PROVEDOR": "openai"})); c.ModeloReserva != "" {
		t.Errorf("openai nao tem reserva: %+v", c)
	}
}

func TestNovoComReservaMontaHedge(t *testing.T) {
	m, err := Novo(Config{Provedor: "nvidia", APIKey: "k", ModeloReserva: "r", HedgeApos: time.Second, SemRaciocinioReserva: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*chatcompat.Cliente); ok {
		t.Errorf("deveria ser o modelo com hedge: %T", m)
	}
	m, err = Novo(Config{Provedor: "nvidia", APIKey: "k", ModeloReserva: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*chatcompat.Cliente); !ok {
		t.Errorf("sem hedge deveria devolver so o principal: %T", m)
	}
}
