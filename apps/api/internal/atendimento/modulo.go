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
		"LLM_MODELO_RESERVA":    cfg.LLMModeloReserva,
	}
	if cfg.LLMTemperatura != nil {
		env["LLM_TEMPERATURA"] = strconv.FormatFloat(*cfg.LLMTemperatura, 'f', -1, 64)
	}
	if cfg.LLMHedgeMS != nil {
		env["LLM_HEDGE_MS"] = strconv.Itoa(*cfg.LLMHedgeMS)
	}
	if cfg.LLMSemRaciocinio != nil {
		env["LLM_SEM_RACIOCINIO"] = strconv.FormatBool(*cfg.LLMSemRaciocinio)
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

	store := conversa.NewStorePGExpira(pool, time.Duration(cfg.AtendimentoV2EstadoExpiraH)*time.Hour)
	canal := evolution.Novo(evolution.Config{BaseURL: cfg.EvolutionBaseURL, APIKey: cfg.EvolutionAPIKey, Instancia: cfg.EvolutionInstance})
	lg.Printf("atendimento v2: llm = %s", pc.Descricao())
	if r := pc.DescricaoReserva(); r != "" {
		lg.Printf("atendimento v2: llm reserva = %s", r)
	}
	if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
		lg.Printf("atendimento v2: OPENAI_API_KEY vazia; audios nao serao transcritos ([audio nao compreendido])")
	}
	mc := midia.Config{
		OpenAIAPIKey:      cfg.OpenAIAPIKey,
		OpenAIBaseURL:     cfg.OpenAIBaseURL,
		ModeloTranscricao: cfg.OpenAITranscriptionModel,
		ModeloVisao:       cfg.OpenAIVisionModel,
	}
	visaoOpenAI := strings.TrimSpace(cfg.OpenAIAPIKey) != "" && strings.TrimSpace(cfg.OpenAIVisionModel) != ""
	if pc.Provedor == provedor.Nvidia && visaoOpenAI {
		// LLM na NVIDIA, mas visao na OpenAI quando configurada: os modelos de
		// visao gratuitos da NVIDIA leem documento de forma instavel.
		lg.Printf("atendimento v2: visao (fotos de documento) = openai %s", cfg.OpenAIVisionModel)
	} else if pc.Provedor == provedor.Nvidia {
		// Visao pela NVIDIA (Chat Completions com image_url); transcricao segue na OpenAI.
		mc.ModeloVisao = ""
		mc.Visao = midia.VisaoConfig{
			Provedor: provedor.Nvidia, APIKey: pc.APIKey, BaseURL: pc.BaseURL,
			Modelo:            firstNonEmpty(cfg.AtendimentoV2ModeloVisao, modelo),
			EsforcoRaciocinio: pc.EsforcoRaciocinio,
		}
		// Sem ATENDIMENTO_V2_MODELO_VISAO a leitura de foto usa o modelo de texto;
		// se ele nao for multimodal, toda foto vira "[imagem recebida]".
		lg.Printf("atendimento v2: visao (fotos de documento) = %s", mc.Visao.Modelo)
		if strings.TrimSpace(cfg.AtendimentoV2ModeloVisao) == "" {
			lg.Printf("atendimento v2: ATENDIMENTO_V2_MODELO_VISAO vazio; a visao usa o modelo do agente (%s), que precisa aceitar imagem", modelo)
		}
	}
	cat := ferramentas.NovoCatalogo(ferramentas.NovaFontePG(pool), time.Now)
	mc.Cidades = func(ctx context.Context) []string {
		cs, err := cat.Cidades(ctx)
		if err != nil {
			return nil
		}
		nomes := make([]string, 0, len(cs))
		for _, c := range cs {
			nomes = append(nomes, c.Nome)
		}
		return nomes
	}
	prep := midia.Novo(canal, mc)

	reg := ferramentas.Padrao(cat, dom.Busca, dom.Cotacao, dom.Reservas, dom.Pagamentos, ferramentas.Config{SinalPorPagante: cfg.AtendimentoV2SinalPorPagante})

	var juiz agente.Juiz
	var roteador agente.Roteador
	switch cfg.AtendimentoV2Juiz {
	case "off":
	case "jev":
		if strings.TrimSpace(cfg.TypesafeAPIKey) == "" {
			lg.Printf("atendimento v2: ATENDIMENTO_V2_JUIZ=jev sem TYPESAFE_API_KEY; usando juiz llm")
			juiz = agente.NovoJuizLLM(cliente, modelo)
		} else {
			// O roteador Jev (1 requisicao por turno) substitui o juiz.
			roteador = agente.NovoRoteadorJev(cfg.TypesafeAPIKey, nil)
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
		Roteador:    roteador,
		Cidades:     cat,
		Notificador: agente.NovoNotificadorWebhook(cfg.AtendimentoV2AlertaWebhookURL, nil),
		Log:         lg,
	}, agente.Config{Modelo: modelo, SinalPorPagante: cfg.AtendimentoV2SinalPorPagante,
		Motor: cfg.AtendimentoV2Motor, ModeloExtratorReserva: cfg.LLMModeloExtratorReserva})

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
