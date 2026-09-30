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
		OpenAIAPIKey: "k", OpenAIModel: "m", EvolutionBaseURL: "http://evo", EvolutionAPIKey: "k", EvolutionInstance: "i",
		EvolutionWebhookSecret: "s", AtendimentoV2Juiz: "jev", AtendimentoV2Concorrencia: 2, AtendimentoV2DebounceMS: 100,
	}

	casos := []struct {
		nome string
		mut  func(*config.Config, *Dominio)
		erro string
	}{
		{"sem openai", func(c *config.Config, _ *Dominio) { c.OpenAIAPIKey = "" }, "OPENAI_API_KEY"},
		{"sem evolution", func(c *config.Config, _ *Dominio) { c.EvolutionBaseURL, c.EvolutionInstance = "", "" }, "EVOLUTION_BASE_URL, EVOLUTION_INSTANCE"},
		{"sem modelo", func(c *config.Config, _ *Dominio) { c.OpenAIModel = "" }, "ATENDIMENTO_V2_MODELO"},
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
}
