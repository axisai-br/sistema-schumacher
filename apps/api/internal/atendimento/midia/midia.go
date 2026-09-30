// Package midia converte mensagens de entrada (audio, imagem, documento) em
// texto para o agente de atendimento.
package midia

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
)

const (
	modeloTranscricaoPadrao = "gpt-4o-mini-transcribe"
	openAIBaseURLPadrao     = "https://api.openai.com/v1"

	promptVocabulario = "Português do Brasil. Atendimento Schumacher Tur. O cliente pode responder perguntas de reserva. " +
		"Destinos comuns em Santa Catarina: Fraiburgo, Monte Carlo, Videira, Campos Novos, Chapecó, Concórdia, Ipumirim, Petrolândia, Ituporanga, Seara. " +
		"Origens comuns no Maranhão: Santa Inês, Monção, Igarapé do Meio. " +
		"Preserve de forma literal respostas curtas sobre passageiros e crianças, como 'só eu', 'só pra mim', 'é só pra mim', 'a passagem é só pra mim', " +
		"'vou sozinho', 'sou só eu', 'não tem criança', 'sem criança', 'não vai criança' e 'tem uma criança'. " +
		"Não troque 'só pra mim' por uma frase ambígua. Transcreva nomes de cidades conforme essa lista."

	promptVisao = "Analise a imagem enviada por um cliente de uma empresa de transporte rodoviário. " +
		"Diga se é uma foto de documento de identificação (RG, CPF, CNH) e, se for, extraia nome completo, CPF e RG exatamente como aparecem " +
		"(use string vazia para o que não estiver legível ou não existir). Em 'descricao' escreva uma frase curta em português descrevendo a imagem."
)

// Config configura o Preparador.
type Config struct {
	OpenAIAPIKey      string
	OpenAIBaseURL     string
	ModeloTranscricao string // padrao "gpt-4o-mini-transcribe"
	ModeloVisao       string // vazio desabilita a leitura de imagens
	HTTP              *http.Client
}

// Preparador transforma mensagens de entrada em texto.
type Preparador struct {
	canal   canal.Canal
	apiKey  string
	baseURL string
	modeloT string
	modeloV string
	http    *http.Client
}

// Novo cria o Preparador.
func Novo(c canal.Canal, cfg Config) *Preparador {
	h := cfg.HTTP
	if h == nil {
		h = &http.Client{Timeout: 60 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.OpenAIBaseURL), "/")
	if base == "" {
		base = openAIBaseURLPadrao
	}
	mt := strings.TrimSpace(cfg.ModeloTranscricao)
	if mt == "" {
		mt = modeloTranscricaoPadrao
	}
	return &Preparador{
		canal:   c,
		apiKey:  strings.TrimSpace(cfg.OpenAIAPIKey),
		baseURL: base,
		modeloT: mt,
		modeloV: strings.TrimSpace(cfg.ModeloVisao),
		http:    h,
	}
}

// Preparar converte a mensagem em texto para o agente. Falhas de midia nao
// viram erro: o texto carrega um marcador e extra descreve o problema.
func (p *Preparador) Preparar(ctx context.Context, m conversa.Mensagem) (string, map[string]any, error) {
	switch m.Tipo {
	case conversa.TipoTexto:
		return m.Texto, nil, nil
	case conversa.TipoAudio:
		texto, extra := p.audio(ctx, m)
		return texto, extra, nil
	case conversa.TipoImagem:
		texto, extra := p.imagem(ctx, m)
		return texto, extra, nil
	case conversa.TipoDocumento:
		nome := strings.TrimSpace(strVal(m.Midia["nome_arquivo"]))
		if nome == "" {
			nome = "sem nome"
		}
		return comLegenda("[documento recebido: "+nome+"]", legendaDe(m, nome)), nil, nil
	default:
		return "[mensagem de um tipo que não consigo ler]", nil, nil
	}
}

// legendaDe ignora o texto quando ele e apenas o nome do arquivo (o canal usa
// o nome como texto do documento sem legenda).
func legendaDe(m conversa.Mensagem, nomeArquivo string) string {
	t := strings.TrimSpace(m.Texto)
	if t == nomeArquivo {
		return ""
	}
	return t
}

func comLegenda(base, legenda string) string {
	legenda = strings.TrimSpace(legenda)
	if legenda == "" {
		return base
	}
	return base + " " + legenda
}

// ---- audio ----

func (p *Preparador) audio(ctx context.Context, m conversa.Mensagem) (string, map[string]any) {
	falha := func(err error) (string, map[string]any) {
		log.Printf("atendimento_midia_audio_falhou conversa=%s erro=%v", m.ConversaID, err)
		return "[áudio não compreendido]", map[string]any{
			"transcricao_status": "FALHOU",
			"transcricao_erro":   err.Error(),
		}
	}
	if p.apiKey == "" {
		return falha(errors.New("chave da OpenAI nao configurada"))
	}
	dataURL, mimeType, err := p.canal.BaixarMidia(ctx, m)
	if err != nil {
		return falha(fmt.Errorf("baixar audio: %w", err))
	}
	texto, err := p.transcrever(ctx, dataURL, mimeType)
	if err != nil {
		return falha(err)
	}
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return falha(errors.New("transcricao vazia"))
	}
	return texto, map[string]any{
		"transcricao_status": "OK",
		"transcricao_modelo": p.modeloT,
	}
}

func (p *Preparador) transcrever(ctx context.Context, dataURL, mimeType string) (string, error) {
	dec, err := decodificarDataURL(dataURL)
	if err != nil {
		return "", err
	}
	tipo := tipoMIME(mimeType)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "audio"+extensaoAudio(tipo))
	if err != nil {
		return "", err
	}
	if _, err := part.Write(dec); err != nil {
		return "", err
	}
	for k, v := range map[string]string{
		"model":           p.modeloT,
		"language":        "pt",
		"prompt":          promptVocabulario,
		"response_format": "json",
	} {
		if err := w.WriteField(k, v); err != nil {
			return "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	raw, status, err := p.do(req)
	if err != nil {
		return "", fmt.Errorf("transcricao: %w", err)
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("transcricao status=%d body=%s", status, resumir(string(raw), 300))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("transcricao: resposta invalida: %w", err)
	}
	return out.Text, nil
}

// ---- imagem ----

type leituraImagem struct {
	EDocumento bool   `json:"e_documento"`
	Nome       string `json:"nome"`
	CPF        string `json:"cpf"`
	RG         string `json:"rg"`
	Descricao  string `json:"descricao"`
}

func (p *Preparador) imagem(ctx context.Context, m conversa.Mensagem) (string, map[string]any) {
	padrao := comLegenda("[imagem recebida]", m.Texto)
	if p.modeloV == "" || p.apiKey == "" {
		return padrao, nil
	}
	dataURL, _, err := p.canal.BaixarMidia(ctx, m)
	if err != nil {
		log.Printf("atendimento_midia_imagem_falhou conversa=%s etapa=baixar erro=%v", m.ConversaID, err)
		return padrao, map[string]any{"visao_status": "FALHOU", "visao_erro": err.Error()}
	}
	l, err := p.lerImagem(ctx, dataURL)
	if err != nil {
		log.Printf("atendimento_midia_imagem_falhou conversa=%s etapa=visao erro=%v", m.ConversaID, err)
		return padrao, map[string]any{"visao_status": "FALHOU", "visao_erro": err.Error()}
	}
	extra := map[string]any{"visao_status": "OK", "visao_modelo": p.modeloV, "e_documento": l.EDocumento}
	if l.EDocumento {
		var partes []string
		if v := strings.TrimSpace(l.Nome); v != "" {
			partes = append(partes, "nome "+v)
		}
		if v := strings.TrimSpace(l.CPF); v != "" {
			partes = append(partes, "CPF "+v)
		}
		if v := strings.TrimSpace(l.RG); v != "" {
			partes = append(partes, "RG "+v)
		}
		base := "[foto de documento]"
		if len(partes) > 0 {
			base = "[foto de documento: " + strings.Join(partes, ", ") + "]"
		}
		return comLegenda(base, m.Texto), extra
	}
	desc := strings.TrimSpace(l.Descricao)
	if desc == "" {
		return padrao, extra
	}
	return comLegenda("[imagem: "+desc+"]", m.Texto), extra
}

func (p *Preparador) lerImagem(ctx context.Context, dataURL string) (leituraImagem, error) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"e_documento": map[string]any{"type": "boolean"},
			"nome":        map[string]any{"type": "string"},
			"cpf":         map[string]any{"type": "string"},
			"rg":          map[string]any{"type": "string"},
			"descricao":   map[string]any{"type": "string"},
		},
		"required":             []string{"e_documento", "nome", "cpf", "rg", "descricao"},
		"additionalProperties": false,
	}
	reqBody, err := json.Marshal(map[string]any{
		"model": p.modeloV,
		"input": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": promptVisao},
				map[string]any{"type": "input_image", "image_url": dataURL},
			},
		}},
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "leitura_imagem", "strict": true, "schema": schema,
		}},
	})
	if err != nil {
		return leituraImagem{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(reqBody))
	if err != nil {
		return leituraImagem{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	raw, status, err := p.do(req)
	if err != nil {
		return leituraImagem{}, fmt.Errorf("visao: %w", err)
	}
	if status < 200 || status >= 300 {
		return leituraImagem{}, fmt.Errorf("visao status=%d body=%s", status, resumir(string(raw), 300))
	}
	var resp struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return leituraImagem{}, fmt.Errorf("visao: resposta invalida: %w", err)
	}
	texto := strings.TrimSpace(resp.OutputText)
	if texto == "" {
		for _, o := range resp.Output {
			for _, c := range o.Content {
				if c.Type == "output_text" && strings.TrimSpace(c.Text) != "" {
					texto = strings.TrimSpace(c.Text)
				}
			}
		}
	}
	if texto == "" {
		return leituraImagem{}, errors.New("visao: resposta sem texto")
	}
	var l leituraImagem
	if err := json.Unmarshal([]byte(texto), &l); err != nil {
		return leituraImagem{}, fmt.Errorf("visao: json estruturado invalido: %w", err)
	}
	return l, nil
}

// ---- utilidades ----

func (p *Preparador) do(req *http.Request) ([]byte, int, error) {
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return raw, resp.StatusCode, err
}

func decodificarDataURL(d string) ([]byte, error) {
	d = strings.TrimSpace(d)
	if i := strings.Index(d, "base64,"); strings.HasPrefix(d, "data:") && i >= 0 {
		d = d[i+len("base64,"):]
	}
	if d == "" {
		return nil, errors.New("audio vazio")
	}
	dec, err := base64.StdEncoding.DecodeString(d)
	if err != nil {
		return nil, fmt.Errorf("base64 do audio invalido: %w", err)
	}
	if len(dec) == 0 {
		return nil, errors.New("audio vazio")
	}
	return dec, nil
}

func tipoMIME(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	t, _, err := mime.ParseMediaType(s)
	if err != nil {
		t = s
	}
	return strings.ToLower(strings.TrimSpace(t))
}

func extensaoAudio(tipo string) string {
	switch tipo {
	case "audio/ogg", "application/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/webm":
		return ".webm"
	case "audio/flac":
		return ".flac"
	default:
		return ".ogg"
	}
}

func strVal(v any) string {
	s, _ := v.(string)
	return s
}

func resumir(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
