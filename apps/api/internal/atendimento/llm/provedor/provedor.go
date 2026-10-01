// Package provedor escolhe o cliente llm.Modelo (NVIDIA NIM ou OpenAI) a partir
// da configuracao do ambiente.
package provedor

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/llm/chatcompat"
	"schumacher-tur/api/internal/atendimento/llm/hedge"
	"schumacher-tur/api/internal/atendimento/llm/openai"
)

const (
	Nvidia = "nvidia"
	OpenAI = "openai"

	NvidiaBaseURLPadrao       = "https://integrate.api.nvidia.com/v1"
	NvidiaModeloPadrao        = "z-ai/glm-5.3"
	NvidiaModeloReservaPadrao = "nvidia/nemotron-3.5-lightning-30b-a3b"
	HedgeMSPadrao             = 15000
	OpenAIBaseURLPadrao       = "https://api.openai.com/v1"
	OpenAIModeloPadrao        = "gpt-4.1-mini"

	temperaturaNvidiaPadrao = 0.3
	esforcoNvidiaPadrao     = "low"
)

type Config struct {
	Provedor          string // "nvidia" | "openai"
	APIKey            string
	BaseURL           string
	Modelo            string
	ModoJSON          string        // so nvidia: "nvext" | "response_format" | "prompt"
	EsforcoRaciocinio string        // so nvidia: low|medium|high|max; vazio nao envia
	Temperatura       *float64      // so nvidia; nil usa o padrao do cliente
	Timeout           time.Duration // 0 usa o padrao do cliente
	// So nvidia. Modelo reserva do hedge (vazio = sem reserva) e quanto esperar o
	// principal antes de disparar o mesmo pedido no reserva (0 = sem hedge).
	ModeloReserva string
	HedgeApos     time.Duration
	// SemRaciocinio desliga o raciocinio (chat_template_kwargs) no principal;
	// SemRaciocinioReserva, no modelo reserva.
	SemRaciocinio        bool
	SemRaciocinioReserva bool
}

// ConfigDoAmbiente le as variaveis de ambiente (get costuma ser os.Getenv).
func ConfigDoAmbiente(get func(string) string) Config {
	val := func(k string) string { return strings.TrimSpace(get(k)) }
	ou := func(vs ...string) string {
		for _, v := range vs {
			if v != "" {
				return v
			}
		}
		return ""
	}
	prov := strings.ToLower(val("LLM_PROVEDOR"))
	if prov == "" {
		prov = Nvidia
	}
	c := Config{Provedor: prov}
	switch prov {
	case OpenAI:
		c.APIKey = val("OPENAI_API_KEY")
		c.BaseURL = ou(val("OPENAI_BASE_URL"), OpenAIBaseURLPadrao)
		c.Modelo = ou(val("ATENDIMENTO_V2_MODELO"), val("OPENAI_MODEL"), OpenAIModeloPadrao)
	default: // nvidia (e desconhecidos: Novo devolve o erro)
		c.APIKey = val("NVIDIA_API_KEY")
		c.BaseURL = ou(val("NVIDIA_BASE_URL"), NvidiaBaseURLPadrao)
		c.Modelo = ou(val("ATENDIMENTO_V2_MODELO"), NvidiaModeloPadrao)
		c.ModoJSON = ou(strings.ToLower(val("LLM_MODO_JSON")), chatcompat.ModoNvext)
		c.EsforcoRaciocinio = ou(strings.ToLower(val("LLM_REASONING_EFFORT")), esforcoNvidiaPadrao)
		t := temperaturaNvidiaPadrao
		if n, err := strconv.ParseFloat(val("LLM_TEMPERATURA"), 64); err == nil && n >= 0 {
			t = n
		}
		c.Temperatura = &t
		c.ModeloReserva = reservaDoAmbiente(val("LLM_MODELO_RESERVA"), c.Modelo)
		c.HedgeApos = HedgeMSPadrao * time.Millisecond
		if n, err := strconv.Atoi(val("LLM_HEDGE_MS")); err == nil && n >= 0 {
			c.HedgeApos = time.Duration(n) * time.Millisecond
		}
		c.SemRaciocinioReserva = true
		switch strings.ToLower(val("LLM_SEM_RACIOCINIO")) {
		case "":
		case "1", "true", "yes", "y", "on":
			c.SemRaciocinio, c.SemRaciocinioReserva = true, true
		default:
			c.SemRaciocinio, c.SemRaciocinioReserva = false, false
		}
	}
	return c
}

// reservaDoAmbiente resolve LLM_MODELO_RESERVA: vazio usa o padrao; "off", "none"
// ou "-" desligam; o mesmo modelo do principal tambem nao gera reserva.
func reservaDoAmbiente(v, principal string) string {
	switch strings.ToLower(v) {
	case "":
		v = NvidiaModeloReservaPadrao
	case "off", "none", "-":
		return ""
	}
	if v == principal {
		return ""
	}
	return v
}

// VariavelChave e o nome da variavel de ambiente que guarda a chave do provedor.
func (c Config) VariavelChave() string {
	if strings.EqualFold(strings.TrimSpace(c.Provedor), OpenAI) {
		return "OPENAI_API_KEY"
	}
	return "NVIDIA_API_KEY"
}

// Descricao resume provedor e modelo; nunca inclui a chave.
func (c Config) Descricao() string {
	return strings.ToLower(strings.TrimSpace(c.Provedor)) + " · " + strings.TrimSpace(c.Modelo)
}

// DescricaoReserva resume o modelo reserva e o hedge; vazio se nao houver.
func (c Config) DescricaoReserva() string {
	if !c.temReserva() {
		return ""
	}
	return strings.TrimSpace(c.ModeloReserva) + " (hedge " + c.HedgeApos.String() + ")"
}

func (c Config) temReserva() bool {
	return strings.EqualFold(strings.TrimSpace(c.Provedor), Nvidia) && strings.TrimSpace(c.ModeloReserva) != "" && c.HedgeApos > 0
}

// Novo cria o cliente do provedor escolhido.
func Novo(c Config) (llm.Modelo, error) {
	prov := strings.ToLower(strings.TrimSpace(c.Provedor))
	switch prov {
	case Nvidia, OpenAI:
	default:
		return nil, fmt.Errorf("LLM_PROVEDOR %q desconhecido (use %q ou %q)", c.Provedor, Nvidia, OpenAI)
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("defina %s para usar o provedor %s", c.VariavelChave(), prov)
	}
	if prov == OpenAI {
		return openai.Novo(openai.Config{APIKey: c.APIKey, BaseURL: c.BaseURL, Modelo: c.Modelo, Timeout: c.Timeout}), nil
	}
	cfg := chatcompat.Config{
		APIKey: c.APIKey, BaseURL: c.BaseURL, Modelo: c.Modelo, ModoJSON: c.ModoJSON,
		EsforcoRaciocinio: c.EsforcoRaciocinio, Temperatura: c.Temperatura, Timeout: c.Timeout,
		SemRaciocinio: c.SemRaciocinio,
	}
	principal := chatcompat.Novo(cfg)
	if !c.temReserva() {
		return principal, nil
	}
	cfg.Modelo = strings.TrimSpace(c.ModeloReserva)
	cfg.SemRaciocinio = c.SemRaciocinioReserva
	return hedge.Novo(hedge.Config{
		Principal: principal, Reserva: chatcompat.Novo(cfg),
		NomeReserva: cfg.Modelo, HedgeApos: c.HedgeApos,
	}), nil
}
