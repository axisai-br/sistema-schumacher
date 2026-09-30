//go:build eval

package evals

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"text/tabwriter"
	"time"

	"schumacher-tur/api/internal/atendimento/agente"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/llm/openai"
)

const taxaMinimaReserva = 0.9

func envOu(nome, padrao string) string {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		return v
	}
	return padrao
}

func envInt(nome string, padrao int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(nome))); err == nil && v > 0 {
		return v
	}
	return padrao
}

// novoOpenAI cria o cliente real (um so serve agente, cliente e juiz: o nome
// do modelo vai em cada pedido).
func novoOpenAI(t testing.TB) *openai.Cliente {
	t.Helper()
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if key == "" {
		t.Skip("OPENAI_API_KEY não definida; evals com LLM real foram ignorados")
	}
	return openai.Novo(openai.Config{
		APIKey: key, BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Modelo: envOu("EVAL_MODELO_AGENTE", "gpt-4.1-mini"), Timeout: 90 * time.Second,
	})
}

type configRodada struct {
	real          llm.Modelo
	modeloAgente  string
	modeloCliente string
	modeloJuiz    string
}

func rodarUma(ctx context.Context, c Caso, cfg configRodada) *Execucao {
	var modelo llm.Modelo = cfg.real
	if c.LLMFora {
		modelo = ModeloQueFalha{}
	}
	amb := NovoAmbiente(modelo, ConfigAmbiente{
		ModeloNome: cfg.modeloAgente,
		Juiz:       agente.NovoJuizLLM(modelo, cfg.modeloJuiz),
	})
	cli := &ClienteLLM{Modelo: cfg.real, ModeloNome: cfg.modeloCliente}
	return Executar(ctx, c, amb, cli)
}

func salvarTranscricao(t *testing.T, ex *Execucao, n int) {
	if err := os.MkdirAll("saida", 0o755); err != nil {
		t.Logf("saida/: %v", err)
		return
	}
	p := filepath.Join("saida", fmt.Sprintf("%s-%d.txt", ex.Caso, n))
	if err := os.WriteFile(p, []byte(FormatarTranscricao(ex)), 0o644); err != nil {
		t.Logf("%s: %v", p, err)
	}
}

type placar struct {
	caso     Caso
	sucessos int
	total    int
}

func TestEvalCasos(t *testing.T) {
	real := novoOpenAI(t)
	k := envInt("EVAL_K", 3)
	cfg := configRodada{
		real:          real,
		modeloAgente:  envOu("EVAL_MODELO_AGENTE", "gpt-4.1-mini"),
		modeloCliente: envOu("EVAL_MODELO_CLIENTE", "gpt-4.1-mini"),
	}
	cfg.modeloJuiz = envOu("EVAL_MODELO_JUIZ", cfg.modeloAgente)
	sem := make(chan struct{}, envInt("EVAL_PARALELO", 4))

	casos, err := CarregarCasos()
	if err != nil {
		t.Fatal(err)
	}
	var filtro *regexp.Regexp
	if f := strings.TrimSpace(os.Getenv("EVAL_CASOS")); f != "" { // regex sobre o nome
		filtro = regexp.MustCompile(f)
	}

	var placares []placar
	for _, c := range casos {
		if filtro != nil && !filtro.MatchString(c.Nome) {
			continue
		}
		c := c
		var p placar
		t.Run(c.Nome, func(t *testing.T) {
			execs := make([]*Execucao, k)
			var wg sync.WaitGroup
			for i := 0; i < k; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
					defer cancel()
					execs[i] = rodarUma(ctx, c, cfg)
				}(i)
			}
			wg.Wait()
			p = placar{caso: c, total: k}
			for i, ex := range execs {
				salvarTranscricao(t, ex, i+1)
				if ex.OK() {
					p.sucessos++
					continue
				}
				if ex.Erro != nil {
					t.Logf("execução %d: ERRO de infraestrutura: %v", i+1, ex.Erro)
				}
				for _, f := range ex.Falhas {
					t.Logf("execução %d: %s", i+1, f)
				}
			}
			if c.Critico && p.sucessos < p.total {
				t.Errorf("caso crítico %s: pass^%d = %d/%d (exige 100%%)", c.Nome, k, p.sucessos, p.total)
			}
		})
		placares = append(placares, p)
	}

	// Tabela e taxa agregada dos casos de reserva.
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "CASO\tSUCESSOS/K\tTAXA\tGRUPO\tCRITICO\n")
	var okRes, totRes int
	for _, p := range placares {
		fmt.Fprintf(w, "%s\t%d/%d\t%.0f%%\t%s\t%v\n", p.caso.Nome, p.sucessos, p.total, 100*float64(p.sucessos)/float64(max(p.total, 1)), p.caso.Grupo, p.caso.Critico)
		if p.caso.Grupo == "reserva" {
			okRes += p.sucessos
			totRes += p.total
		}
	}
	w.Flush()
	t.Logf("modelo agente=%s cliente=%s juiz=%s K=%d\n%s", cfg.modeloAgente, cfg.modeloCliente, cfg.modeloJuiz, k, b.String())
	if totRes > 0 {
		taxa := float64(okRes) / float64(totRes)
		t.Logf("taxa agregada dos casos de reserva: %d/%d = %.0f%%", okRes, totRes, 100*taxa)
		if taxa < taxaMinimaReserva {
			t.Errorf("taxa dos casos de reserva %.0f%% < %.0f%%", 100*taxa, 100*taxaMinimaReserva)
		}
	}
}
