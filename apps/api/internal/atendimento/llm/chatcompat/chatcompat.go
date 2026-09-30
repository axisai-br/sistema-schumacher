// Package chatcompat implementa llm.Modelo sobre a API Chat Completions
// compativel com OpenAI (NVIDIA NIM e similares), via net/http.
package chatcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
)

const (
	ModoNvext          = "nvext"
	ModoResponseFormat = "response_format"
	ModoPrompt         = "prompt"
)

type Config struct {
	APIKey            string
	BaseURL           string // padrao https://integrate.api.nvidia.com/v1
	Modelo            string
	ModoJSON          string        // "nvext" (padrao) | "response_format" | "prompt"
	Timeout           time.Duration // padrao 60s
	HTTP              *http.Client
	Temperatura       *float64 // padrao 0.2
	EsforcoRaciocinio string   // "reasoning_effort" (low|medium|high|max); vazio nao envia
	Seed              *int     // opcional
}

type Cliente struct {
	apiKey  string
	baseURL string
	modelo  string
	modo    string
	temp    float64
	esforco string
	seed    *int
	http    *http.Client
	backoff []time.Duration
}

var _ llm.Modelo = (*Cliente)(nil)

func Novo(cfg Config) *Cliente {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://integrate.api.nvidia.com/v1"
	}
	hc := cfg.HTTP
	if hc == nil {
		t := cfg.Timeout
		if t <= 0 {
			t = 60 * time.Second
		}
		hc = &http.Client{Timeout: t}
	}
	modo := strings.ToLower(strings.TrimSpace(cfg.ModoJSON))
	switch modo {
	case ModoNvext, ModoResponseFormat, ModoPrompt:
	default:
		modo = ModoNvext
	}
	temp := 0.2
	if cfg.Temperatura != nil {
		temp = *cfg.Temperatura
	}
	return &Cliente{
		apiKey:  strings.TrimSpace(cfg.APIKey),
		baseURL: base,
		modelo:  strings.TrimSpace(cfg.Modelo),
		modo:    modo,
		temp:    temp,
		esforco: strings.ToLower(strings.TrimSpace(cfg.EsforcoRaciocinio)),
		seed:    cfg.Seed,
		http:    hc,
		backoff: []time.Duration{1 * time.Second, 3 * time.Second},
	}
}

type erroHTTP struct {
	status int
	corpo  string
}

func (e *erroHTTP) Error() string { return fmt.Sprintf("chatcompat: status %d: %s", e.status, e.corpo) }

func (e *erroHTTP) retentavel() bool { return e.status == 429 || e.status >= 500 }

func (c *Cliente) usaPrompt(p llm.Pedido) bool {
	return len(p.SaidaJSON) > 0 && len(p.Ferramentas) == 0 && c.modo == ModoPrompt
}

func (c *Cliente) montarPayload(p llm.Pedido) map[string]any {
	modelo := strings.TrimSpace(p.Modelo)
	if modelo == "" {
		modelo = c.modelo
	}
	system := strings.TrimSpace(p.Instrucoes)
	if c.usaPrompt(p) {
		if system != "" {
			system += "\n\n"
		}
		system += "Responda APENAS com um JSON válido seguindo este schema: " + string(p.SaidaJSON)
	}
	msgs := make([]any, 0, len(p.Mensagens)+1)
	if system != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": system})
	}
	for _, m := range p.Mensagens {
		switch m.Papel {
		case llm.PapelFerramenta:
			msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": m.ChamadaID, "content": m.Texto})
		case llm.PapelAssistente:
			msg := map[string]any{"role": "assistant"}
			if strings.TrimSpace(m.Texto) != "" {
				msg["content"] = m.Texto
			} else if len(m.Chamadas) > 0 {
				msg["content"] = nil
			} else {
				msg["content"] = ""
			}
			if len(m.Chamadas) > 0 {
				tcs := make([]any, 0, len(m.Chamadas))
				for _, ch := range m.Chamadas {
					args := string(ch.Argumentos)
					if strings.TrimSpace(args) == "" {
						args = "{}"
					}
					tcs = append(tcs, map[string]any{
						"id": ch.ID, "type": "function",
						"function": map[string]any{"name": ch.Nome, "arguments": args},
					})
				}
				msg["tool_calls"] = tcs
			}
			msgs = append(msgs, msg)
		case llm.PapelSistema:
			msgs = append(msgs, map[string]any{"role": "system", "content": m.Texto})
		default:
			msgs = append(msgs, map[string]any{"role": "user", "content": m.Texto})
		}
	}
	maxTok := p.MaxTokens
	if maxTok <= 0 {
		maxTok = 1024
		if c.esforco != "" {
			maxTok = 4096 // modelos de raciocinio gastam tokens antes da resposta
		}
	}
	payload := map[string]any{
		"model":       modelo,
		"messages":    msgs,
		"max_tokens":  maxTok,
		"temperature": c.temp,
		"stream":      false,
	}
	if c.esforco != "" {
		payload["reasoning_effort"] = c.esforco
	}
	if c.seed != nil {
		payload["seed"] = *c.seed
	}
	if len(p.Ferramentas) > 0 {
		tools := make([]any, 0, len(p.Ferramentas))
		for _, f := range p.Ferramentas {
			params := f.Parametros
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			tools = append(tools, map[string]any{
				"type":     "function",
				"function": map[string]any{"name": f.Nome, "description": f.Descricao, "parameters": params},
			})
		}
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	} else if len(p.SaidaJSON) > 0 {
		switch c.modo {
		case ModoNvext:
			payload["nvext"] = map[string]any{"guided_json": p.SaidaJSON}
		case ModoResponseFormat:
			payload["response_format"] = map[string]any{
				"type":        "json_schema",
				"json_schema": map[string]any{"name": "saida", "schema": p.SaidaJSON},
			}
		}
	}
	return payload
}

// respostaAPI ignora de proposito reasoning_content/reasoning: so content e
// tool_calls interessam.
type respostaAPI struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   *string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *Cliente) Gerar(ctx context.Context, p llm.Pedido) (llm.Resposta, error) {
	if c.apiKey == "" {
		return llm.Resposta{}, errors.New("chatcompat: chave de API nao configurada")
	}
	corpo, err := json.Marshal(c.montarPayload(p))
	if err != nil {
		return llm.Resposta{}, err
	}
	var ultimo error
	for tentativa := 0; tentativa <= len(c.backoff); tentativa++ {
		if tentativa > 0 {
			select {
			case <-ctx.Done():
				return llm.Resposta{}, ctx.Err()
			case <-time.After(c.backoff[tentativa-1]):
			}
		}
		resp, err := c.uma(ctx, corpo, p)
		if err == nil {
			return resp, nil
		}
		ultimo = err
		if ctx.Err() != nil {
			return llm.Resposta{}, err
		}
		var eh *erroHTTP
		if errors.As(err, &eh) && !eh.retentavel() {
			return llm.Resposta{}, err
		}
	}
	return llm.Resposta{}, ultimo
}

func (c *Cliente) uma(ctx context.Context, corpo []byte, p llm.Pedido) (llm.Resposta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(corpo))
	if err != nil {
		return llm.Resposta{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return llm.Resposta{}, errors.New("chatcompat: falha de rede: " + c.limpar(err.Error()))
	}
	defer resp.Body.Close()
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return llm.Resposta{}, errors.New("chatcompat: falha ao ler resposta: " + c.limpar(err.Error()))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return llm.Resposta{}, &erroHTTP{status: resp.StatusCode, corpo: resumir(c.limpar(string(dados)), 300)}
	}
	var r respostaAPI
	if err := json.Unmarshal(dados, &r); err != nil {
		return llm.Resposta{}, fmt.Errorf("chatcompat: resposta invalida: %w", err)
	}
	if len(r.Choices) == 0 {
		return llm.Resposta{}, errors.New("chatcompat: resposta sem choices")
	}
	msg := r.Choices[0].Message
	out := llm.Resposta{Modelo: r.Model, TokensEntrada: r.Usage.PromptTokens, TokensSaida: r.Usage.CompletionTokens}
	for i, tc := range msg.ToolCalls {
		id := strings.TrimSpace(tc.ID)
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		out.Chamadas = append(out.Chamadas, llm.ChamadaFerramenta{ID: id, Nome: tc.Function.Name, Argumentos: normalizarArgs(tc.Function.Arguments)})
	}
	texto := ""
	if msg.Content != nil {
		texto = RemoverPensamento(*msg.Content)
	}
	if len(out.Chamadas) == 0 && len(p.Ferramentas) > 0 {
		if ch, ok := chamadaEmTexto(texto, p.Ferramentas); ok {
			out.Chamadas = []llm.ChamadaFerramenta{ch}
			texto = ""
		}
	}
	if c.usaPrompt(p) {
		if j, ok := ExtrairObjetoJSON(texto); ok {
			texto = j
		}
	}
	out.Texto = strings.TrimSpace(texto)
	return out, nil
}

func (c *Cliente) limpar(s string) string {
	if c.apiKey != "" {
		s = strings.ReplaceAll(s, c.apiKey, "[redigido]")
	}
	return s
}

func resumir(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// normalizarArgs aceita arguments como string JSON ou objeto.
func normalizarArgs(raw json.RawMessage) json.RawMessage {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return json.RawMessage("{}")
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err == nil {
			s = strings.TrimSpace(s)
			if s == "" {
				return json.RawMessage("{}")
			}
			return json.RawMessage(s)
		}
	}
	return json.RawMessage(t)
}

var reThink = regexp.MustCompile(`(?s)<think>.*?</think>`)

// RemoverPensamento descarta blocos <think>...</think> (e um <think> nao fechado).
func RemoverPensamento(s string) string {
	s = reThink.ReplaceAllString(s, "")
	if i := strings.Index(s, "<think>"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "</think>"); i >= 0 {
		s = s[i+len("</think>"):]
	}
	return strings.TrimSpace(s)
}

var reFence = regexp.MustCompile("(?s)^```[a-zA-Z0-9]*\\s*(.*?)\\s*```$")

func tirarFence(s string) string {
	s = strings.TrimSpace(s)
	if m := reFence.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return s
}

// ExtrairObjetoJSON devolve o primeiro objeto JSON balanceado do texto
// (tolera cercas ```json e texto ao redor).
func ExtrairObjetoJSON(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for ini := strings.IndexByte(s, '{'); ini >= 0; {
		dec := json.NewDecoder(strings.NewReader(s[ini:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err == nil {
			return string(raw), true
		}
		prox := strings.IndexByte(s[ini+1:], '{')
		if prox < 0 {
			break
		}
		ini += 1 + prox
	}
	return "", false
}

var reToolCallTag = regexp.MustCompile(`(?s)^<tool_call>\s*(.*?)\s*(?:</tool_call>)?$`)

// chamadaEmTexto reconhece uma chamada de ferramenta escrita como texto JSON
// ({"name":..., "parameters"|"arguments":...}), com ou sem <tool_call>/cercas.
func chamadaEmTexto(texto string, defs []llm.DefFerramenta) (llm.ChamadaFerramenta, bool) {
	t := strings.TrimSpace(texto)
	if m := reToolCallTag.FindStringSubmatch(t); m != nil {
		t = m[1]
	}
	t = tirarFence(t)
	if !strings.HasPrefix(t, "{") {
		return llm.ChamadaFerramenta{}, false
	}
	var o struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
		Arguments  json.RawMessage `json:"arguments"`
	}
	dec := json.NewDecoder(strings.NewReader(t))
	if err := dec.Decode(&o); err != nil {
		return llm.ChamadaFerramenta{}, false
	}
	if strings.TrimSpace(t[dec.InputOffset():]) != "" {
		return llm.ChamadaFerramenta{}, false
	}
	o.Name = strings.TrimSpace(o.Name)
	conhecida := false
	for _, d := range defs {
		if d.Nome == o.Name {
			conhecida = true
			break
		}
	}
	if !conhecida {
		return llm.ChamadaFerramenta{}, false
	}
	args := o.Parameters
	if len(bytes.TrimSpace(args)) == 0 {
		args = o.Arguments
	}
	return llm.ChamadaFerramenta{ID: "call_1", Nome: o.Name, Argumentos: normalizarArgs(args)}, true
}
