package atendimento

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/shared/config"
)

type (
	buscaNula      struct{ ferramentas.Buscador }
	cotacaoNula    struct{ ferramentas.Cotador }
	reservasNula   struct{ ferramentas.Reservas }
	pagamentosNulo struct{ ferramentas.Pagamentos }
)

func TestMontar(t *testing.T) {
	ctx := context.Background()
	// pgxpool conecta de forma preguicosa: nao precisa de banco real.
	pool, err := pgxpool.New(ctx, "postgres://u:p@127.0.0.1:1/db")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	lg := log.New(io.Discard, "", 0)
	dom := Dominio{Busca: buscaNula{}, Cotacao: cotacaoNula{}, Reservas: reservasNula{}, Pagamentos: pagamentosNulo{}}
	ok := config.Config{
		LLMProvedor: "nvidia", NvidiaAPIKey: "nv", EvolutionBaseURL: "http://evo", EvolutionAPIKey: "k", EvolutionInstance: "i",
		EvolutionWebhookSecret: "s", AtendimentoV2Juiz: "jev", AtendimentoV2Concorrencia: 2, AtendimentoV2DebounceMS: 100,
	}

	casos := []struct {
		nome string
		mut  func(*config.Config, *Dominio)
		erro string
	}{
		{"sem nvidia", func(c *config.Config, _ *Dominio) { c.NvidiaAPIKey = "" }, "NVIDIA_API_KEY"},
		{"sem openai", func(c *config.Config, _ *Dominio) { c.LLMProvedor, c.OpenAIAPIKey = "openai", "" }, "OPENAI_API_KEY"},
		{"provedor desconhecido", func(c *config.Config, _ *Dominio) { c.LLMProvedor = "xyz" }, "xyz"},
		{"sem evolution", func(c *config.Config, _ *Dominio) { c.EvolutionBaseURL, c.EvolutionInstance = "", "" }, "EVOLUTION_BASE_URL, EVOLUTION_INSTANCE"},
		{"dominio incompleto", func(_ *config.Config, d *Dominio) { d.Cotacao = nil }, "Cotacao"},
	}
	for _, c := range casos {
		cfg, d := ok, dom
		c.mut(&cfg, &d)
		_, _, err := Montar(ctx, pool, cfg, d, lg)
		if err == nil || !strings.Contains(err.Error(), c.erro) {
			t.Errorf("%s: erro %v, esperado conter %q", c.nome, err, c.erro)
		}
	}
	if _, _, err := Montar(ctx, nil, ok, dom, lg); err == nil {
		t.Error("pool nil deveria falhar")
	}

	// jev sem chave cai para llm; ok.
	h, w, err := Montar(ctx, pool, ok, dom, lg)
	if err != nil || h == nil || w == nil {
		t.Fatalf("montagem valida falhou: %v", err)
	}

	// openai como alternativa, e nvidia sem OPENAI_API_KEY (audio degrada, nao falha).
	oa := ok
	oa.LLMProvedor, oa.OpenAIAPIKey, oa.OpenAIModel = "openai", "sk", "gpt-x"
	if _, w, err := Montar(ctx, pool, oa, dom, lg); err != nil || w == nil {
		t.Fatalf("montagem openai falhou: %v", err)
	}
}

func TestConfigLLM(t *testing.T) {
	temp := 0.3
	c := ConfigLLM(config.Config{LLMProvedor: "nvidia", NvidiaAPIKey: "nv", LLMTemperatura: &temp, OpenAIModel: "gpt-x"})
	if c.Provedor != "nvidia" || c.APIKey != "nv" || c.Modelo != "z-ai/glm-5.3" || c.ModoJSON != "nvext" ||
		c.EsforcoRaciocinio != "low" || c.Temperatura == nil || *c.Temperatura != 0.3 {
		t.Errorf("nvidia: %+v", c)
	}
	c = ConfigLLM(config.Config{LLMProvedor: "openai", OpenAIAPIKey: "sk", OpenAIModel: "gpt-x", NvidiaAPIKey: "nv"})
	if c.Provedor != "openai" || c.APIKey != "sk" || c.Modelo != "gpt-x" {
		t.Errorf("openai: %+v", c)
	}
	if c = ConfigLLM(config.Config{AtendimentoV2Modelo: "meta/llama-3.3-70b-instruct"}); c.Provedor != "nvidia" || c.Modelo != "meta/llama-3.3-70b-instruct" {
		t.Errorf("padrao: %+v", c)
	}
}
