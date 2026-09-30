package atendimento

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"schumacher-tur/api/internal/atendimento/agente"
	"schumacher-tur/api/internal/atendimento/canal/evolution"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/fila"
	"schumacher-tur/api/internal/atendimento/llm/provedor"
	"schumacher-tur/api/internal/atendimento/midia"
	"schumacher-tur/api/internal/shared/config"
)

// Dominio reune os servicos do sistema que as ferramentas do agente consomem.
type Dominio struct {
	Busca      ferramentas.Buscador
	Cotacao    ferramentas.Cotador
	Reservas   ferramentas.Reservas
	Pagamentos ferramentas.Pagamentos
}

// ConfigLLM monta a configuracao do provedor de LLM (agente e juiz) a partir do
// cfg, com os mesmos padroes de provedor.ConfigDoAmbiente.
func ConfigLLM(cfg config.Config) provedor.Config {
	env := map[string]string{
		"LLM_PROVEDOR":          cfg.LLMProvedor,
		"NVIDIA_API_KEY":        cfg.NvidiaAPIKey,
		"NVIDIA_BASE_URL":       cfg.NvidiaBaseURL,
		"OPENAI_API_KEY":        cfg.OpenAIAPIKey,
		"OPENAI_BASE_URL":       cfg.OpenAIBaseURL,
		"OPENAI_MODEL":          cfg.OpenAIModel,
		"ATENDIMENTO_V2_MODELO": cfg.AtendimentoV2Modelo,
		"LLM_MODO_JSON":         cfg.LLMModoJSON,
		"LLM_REASONING_EFFORT":  cfg.LLMEsforcoRaciocinio,
	}
	if cfg.LLMTemperatura != nil {
		env["LLM_TEMPERATURA"] = strconv.FormatFloat(*cfg.LLMTemperatura, 'f', -1, 64)
	}
	return provedor.ConfigDoAmbiente(func(k string) string { return env[k] })
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

// Montar conecta todas as pecas do atendimento v2. Nao inicia o worker: quem
// chama deve invocar Worker.Iniciar(ctx).
func Montar(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, dom Dominio, lg *log.Logger) (*Handler, *fila.Worker, error) {
	if lg == nil {
		lg = log.Default()
	}
	if pool == nil {
		return nil, nil, errors.New("atendimento v2: pool do banco e obrigatorio")
	}
	pc := ConfigLLM(cfg)
	cliente, err := provedor.Novo(pc)
	if err != nil {
		return nil, nil, fmt.Errorf("atendimento v2: %w", err)
	}
	modelo := pc.Modelo
	var faltaEvo []string
	if strings.TrimSpace(cfg.EvolutionBaseURL) == "" {
		faltaEvo = append(faltaEvo, "EVOLUTION_BASE_URL")
	}
	if strings.TrimSpace(cfg.EvolutionAPIKey) == "" {
		faltaEvo = append(faltaEvo, "EVOLUTION_API_KEY")
	}
	if strings.TrimSpace(cfg.EvolutionInstance) == "" {
		faltaEvo = append(faltaEvo, "EVOLUTION_INSTANCE")
	}
	if len(faltaEvo) > 0 {
		return nil, nil, fmt.Errorf("atendimento v2: configuracao da Evolution incompleta, faltando %s", strings.Join(faltaEvo, ", "))
	}
	var faltaDom []string
	if dom.Busca == nil {
		faltaDom = append(faltaDom, "Busca")
	}
	if dom.Cotacao == nil {
		faltaDom = append(faltaDom, "Cotacao")
	}
	if dom.Reservas == nil {
		faltaDom = append(faltaDom, "Reservas")
	}
	if dom.Pagamentos == nil {
		faltaDom = append(faltaDom, "Pagamentos")
	}
	if len(faltaDom) > 0 {
		return nil, nil, fmt.Errorf("atendimento v2: dominio incompleto, faltando %s", strings.Join(faltaDom, ", "))
	}

	store := conversa.NewStorePG(pool)
	canal := evolution.Novo(evolution.Config{BaseURL: cfg.EvolutionBaseURL, APIKey: cfg.EvolutionAPIKey, Instancia: cfg.EvolutionInstance})
	lg.Printf("atendimento v2: llm = %s", pc.Descricao())
	if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
		lg.Printf("atendimento v2: OPENAI_API_KEY vazia; audios nao serao transcritos ([audio nao compreendido])")
	}
	mc := midia.Config{
		OpenAIAPIKey:      cfg.OpenAIAPIKey,
		OpenAIBaseURL:     cfg.OpenAIBaseURL,
		ModeloTranscricao: cfg.OpenAITranscriptionModel,
		ModeloVisao:       cfg.OpenAIVisionModel,
	}
	if pc.Provedor == provedor.Nvidia {
		// Visao pela NVIDIA (Chat Completions com image_url); transcricao segue na OpenAI.
		mc.ModeloVisao = ""
		mc.Visao = midia.VisaoConfig{
			Provedor: provedor.Nvidia, APIKey: pc.APIKey, BaseURL: pc.BaseURL,
			Modelo:            firstNonEmpty(cfg.AtendimentoV2ModeloVisao, modelo),
			EsforcoRaciocinio: pc.EsforcoRaciocinio,
		}
	}
	prep := midia.Novo(canal, mc)

	cat := ferramentas.NovoCatalogo(ferramentas.NovaFontePG(pool), time.Now)
	reg := ferramentas.Padrao(cat, dom.Busca, dom.Cotacao, dom.Reservas, dom.Pagamentos, ferramentas.Config{SinalPorPagante: cfg.AtendimentoV2SinalPorPagante})

	var juiz agente.Juiz
	switch cfg.AtendimentoV2Juiz {
	case "off":
	case "jev":
		if strings.TrimSpace(cfg.TypesafeAPIKey) == "" {
			lg.Printf("atendimento v2: ATENDIMENTO_V2_JUIZ=jev sem TYPESAFE_API_KEY; usando juiz llm")
			juiz = agente.NovoJuizLLM(cliente, modelo)
		} else {
			juiz = agente.NovoJuizJev(cfg.TypesafeAPIKey, nil)
		}
	default:
		juiz = agente.NovoJuizLLM(cliente, modelo)
	}

	ag := agente.Novo(agente.Deps{
		Store:       store,
		Canal:       canal,
		Modelo:      cliente,
		Ferramentas: reg,
		Catalogo:    cat,
		Midia:       prep,
		Juiz:        juiz,
		Notificador: agente.NovoNotificadorWebhook(cfg.AtendimentoV2AlertaWebhookURL, nil),
		Log:         lg,
	}, agente.Config{Modelo: modelo})

	worker := fila.NovoWorker(store, ag, fila.Config{
		Concorrencia: cfg.AtendimentoV2Concorrencia,
		Debounce:     time.Duration(cfg.AtendimentoV2DebounceMS) * time.Millisecond,
	}, lg)

	h := NovoHandler(Deps{
		Store:               store,
		Canal:               canal,
		SegredoWebhook:      cfg.EvolutionWebhookSecret,
		Log:                 lg,
		TelefonesPermitidos: cfg.AtendimentoV2Telefones,
	})
	return h, worker, nil
}
