// Package evolution implementa canal.Canal para WhatsApp via Evolution API.
package evolution

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
)

const (
	limiteImagem = 8 * 1024 * 1024
	limiteAudio  = 25 * 1024 * 1024
	limitePDF    = 10 * 1024 * 1024
)

// Config configura o canal Evolution.
type Config struct {
	BaseURL, APIKey, Instancia string
	HTTP                       *http.Client
}

// Canal implementa canal.Canal.
type Canal struct {
	baseURL, apiKey, instancia string
	http                       *http.Client
}

var _ canal.Canal = (*Canal)(nil)

// Novo cria o canal. Sem HTTP informado, usa um cliente com timeout de 45s.
func Novo(cfg Config) *Canal {
	c := cfg.HTTP
	if c == nil {
		c = &http.Client{Timeout: 45 * time.Second}
	}
	return &Canal{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		instancia: strings.TrimSpace(cfg.Instancia),
		http:      c,
	}
}

func (c *Canal) Nome() string { return "WHATSAPP" }

// ---- recepcao ----

type payload struct {
	Event string `json:"event"`
	Data  struct {
		Key struct {
			RemoteJID    string `json:"remoteJid"`
			RemoteJIDAlt string `json:"remoteJidAlt"`
			FromMe       bool   `json:"fromMe"`
			ID           string `json:"id"`
		} `json:"key"`
		PushName         string                 `json:"pushName"`
		Message          map[string]interface{} `json:"message"`
		MessageType      string                 `json:"messageType"`
		MessageTimestamp interface{}            `json:"messageTimestamp"`
	} `json:"data"`
	DateTime string `json:"date_time"`
}

func decodificar(corpo []byte) (payload, error) {
	var p payload
	if err := json.Unmarshal(corpo, &p); err == nil && p.Event != "" {
		return p, nil
	}
	var env struct {
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(corpo, &env); err != nil || len(env.Body) == 0 {
		return payload{}, errors.New("evolution: payload invalido")
	}
	p = payload{}
	if err := json.Unmarshal(env.Body, &p); err != nil || p.Event == "" {
		return payload{}, errors.New("evolution: payload invalido")
	}
	return p, nil
}

// Normalizar converte o webhook da Evolution em canal.Entrada.
func (c *Canal) Normalizar(_ context.Context, corpo []byte) (canal.Entrada, bool, error) {
	p, err := decodificar(corpo)
	if err != nil {
		return canal.Entrada{}, false, err
	}
	if strings.TrimSpace(p.Event) != "messages.upsert" {
		return canal.Entrada{}, false, nil
	}
	contato := strings.TrimSpace(p.Data.Key.RemoteJID)
	if contato == "" {
		contato = strings.TrimSpace(p.Data.Key.RemoteJIDAlt)
	}
	if contato == "" || strings.HasSuffix(contato, "@g.us") || contato == "status@broadcast" {
		return canal.Entrada{}, false, nil
	}

	msg := p.Data.Message
	tipoMsg := strings.TrimSpace(p.Data.MessageType)
	if tipoMsg == "" {
		tipoMsg = inferirTipo(msg)
	}

	e := canal.Entrada{
		Contato:         contato,
		Telefone:        normalizarTelefone(contato),
		Nome:            strings.TrimSpace(p.Data.PushName),
		DoProprioNumero: p.Data.Key.FromMe,
		ProvedorID:      strings.TrimSpace(p.Data.Key.ID),
		RecebidaEm:      resolverTimestamp(p),
	}
	midia := map[string]interface{}{
		"message_id": e.ProvedorID,
		"remote_jid": contato,
		"from_me":    p.Data.Key.FromMe,
	}

	switch tipoMsg {
	case "conversation", "extendedTextMessage":
		texto := strings.TrimSpace(asString(msg["conversation"]))
		if texto == "" {
			if ext, ok := msg["extendedTextMessage"].(map[string]interface{}); ok {
				texto = strings.TrimSpace(asString(ext["text"]))
			}
		}
		if texto == "" {
			return canal.Entrada{}, false, nil
		}
		e.Tipo = conversa.TipoTexto
		e.Texto = texto
		return e, true, nil
	case "audioMessage", "audio", "ptt", "voice":
		m, _ := msg["audioMessage"].(map[string]interface{})
		e.Tipo = conversa.TipoAudio
		midia["mime"] = mimeDe(m, "audio/ogg")
		seg := inteiro(m["seconds"])
		if seg <= 0 {
			seg = inteiro(m["duration"])
		}
		if seg > 0 {
			midia["segundos"] = seg
		}
	case "imageMessage":
		m, _ := msg["imageMessage"].(map[string]interface{})
		e.Tipo = conversa.TipoImagem
		e.Texto = strings.TrimSpace(asString(m["caption"]))
		midia["mime"] = mimeDe(m, "image/jpeg")
	case "documentMessage", "documentWithCaptionMessage":
		m, _ := msg["documentMessage"].(map[string]interface{})
		if m == nil {
			if w, ok := msg["documentWithCaptionMessage"].(map[string]interface{}); ok {
				if inner, ok := w["message"].(map[string]interface{}); ok {
					m, _ = inner["documentMessage"].(map[string]interface{})
				}
			}
		}
		e.Tipo = conversa.TipoDocumento
		nome := primeiro(asString(m["fileName"]), asString(m["file_name"]), asString(m["title"]))
		e.Texto = primeiro(asString(m["caption"]), nome)
		midia["mime"] = mimeDe(m, "application/pdf")
		if nome != "" {
			midia["nome_arquivo"] = nome
		}
	case "videoMessage", "stickerMessage", "locationMessage", "liveLocationMessage",
		"contactMessage", "contactsArrayMessage":
		e.Tipo = conversa.TipoOutro
		midia["tipo_original"] = tipoMsg
		if m, ok := msg[tipoMsg].(map[string]interface{}); ok {
			e.Texto = strings.TrimSpace(asString(m["caption"]))
		}
	default:
		// protocolo, reacao, sem conteudo util
		return canal.Entrada{}, false, nil
	}
	e.Midia = midia
	return e, true, nil
}

func inferirTipo(msg map[string]interface{}) string {
	for _, k := range []string{"conversation", "extendedTextMessage", "audioMessage", "imageMessage", "documentMessage", "videoMessage", "stickerMessage", "locationMessage", "contactMessage"} {
		if _, ok := msg[k]; ok {
			return k
		}
	}
	return ""
}

func resolverTimestamp(p payload) time.Time {
	if ts := int64(inteiro(p.Data.MessageTimestamp)); ts > 0 {
		return time.Unix(ts, 0).UTC()
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(p.DateTime)); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}

func normalizarTelefone(contato string) string {
	s := strings.TrimSpace(contato)
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---- midia ----

// BaixarMidia baixa a midia da mensagem como data URL.
func (c *Canal) BaixarMidia(ctx context.Context, m conversa.Mensagem) (string, string, error) {
	id := strings.TrimSpace(asString(m.Midia["message_id"]))
	jid := strings.TrimSpace(asString(m.Midia["remote_jid"]))
	fromMe, _ := m.Midia["from_me"].(bool)
	mime := strings.TrimSpace(asString(m.Midia["mime"]))
	if c.baseURL == "" || c.instancia == "" || c.apiKey == "" {
		return "", "", errors.New("evolution: configuracao de midia incompleta")
	}
	if id == "" || jid == "" {
		return "", "", errors.New("evolution: mensagem sem message_id/remote_jid para baixar midia")
	}

	max := limiteImagem
	switch m.Tipo {
	case conversa.TipoAudio:
		max = limiteAudio
	case conversa.TipoDocumento:
		max = limitePDF
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"message":      map[string]interface{}{"key": map[string]interface{}{"id": id, "remoteJid": jid, "fromMe": fromMe}},
		"convertToMp4": false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/getBase64FromMediaMessage/"+c.instancia, bytes.NewReader(reqBody))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("evolution: baixar midia: %w", err)
	}
	defer resp.Body.Close()

	limiteResp := int64(max)*4/3 + 64*1024
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limiteResp+1))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("evolution: baixar midia status=%d body=%s", resp.StatusCode, resumir(string(raw), 300))
	}
	if int64(len(raw)) > limiteResp {
		return "", "", fmt.Errorf("evolution: midia maior que o limite de %d MB", max/(1024*1024))
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", "", fmt.Errorf("evolution: resposta de midia invalida: %w", err)
	}
	b64 := primeiro(
		caminho(parsed, "base64"),
		caminho(parsed, "data", "base64"),
		caminho(parsed, "message", "base64"),
		caminho(parsed, "data", "message", "base64"),
		caminho(parsed, "data", "message", "base64Message"),
	)
	if i := strings.Index(b64, "base64,"); strings.HasPrefix(b64, "data:") && i >= 0 {
		b64 = b64[i+len("base64,"):]
	}
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return "", "", errors.New("evolution: resposta sem base64")
	}
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", "", fmt.Errorf("evolution: base64 invalido: %w", err)
	}
	if len(dec) == 0 {
		return "", "", errors.New("evolution: midia vazia")
	}
	if len(dec) > max {
		return "", "", fmt.Errorf("evolution: midia de %d bytes excede o limite de %d MB", len(dec), max/(1024*1024))
	}
	if mime == "" {
		mime = primeiro(caminho(parsed, "mimetype"), caminho(parsed, "data", "mimetype"))
	}
	if mime == "" {
		mime = http.DetectContentType(dec)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(dec), mime, nil
}

// ---- envio ----

// Enviar envia texto e devolve o ID da mensagem no provedor.
func (c *Canal) Enviar(ctx context.Context, contato, texto string) (string, error) {
	numero := normalizarTelefone(contato)
	texto = strings.TrimSpace(texto)
	if numero == "" {
		return "", errors.New("evolution: destinatario vazio")
	}
	if texto == "" {
		return "", errors.New("evolution: texto vazio")
	}
	if c.baseURL == "" || c.instancia == "" || c.apiKey == "" {
		return "", errors.New("evolution: configuracao de envio incompleta")
	}
	corpo, _ := json.Marshal(map[string]interface{}{
		"number":      numero,
		"text":        texto,
		"delay":       1200,
		"linkPreview": false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/message/sendText/"+c.instancia, bytes.NewReader(corpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("evolution: enviar: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("evolution: ler resposta: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("evolution: enviar status=%d body=%s", resp.StatusCode, resumir(string(raw), 300))
	}
	var parsed map[string]interface{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", errors.New("evolution: resposta de envio invalida")
		}
	}
	id := primeiro(caminho(parsed, "key", "id"), caminho(parsed, "data", "key", "id"), caminho(parsed, "id"))
	if id == "" {
		return "", errors.New("evolution: resposta de envio sem id da mensagem")
	}
	return id, nil
}

// ---- utilidades ----

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func primeiro(vs ...string) string {
	for _, v := range vs {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func mimeDe(m map[string]interface{}, padrao string) string {
	return primeiro(asString(m["mimetype"]), asString(m["mimeType"]), asString(m["mime_type"]), padrao)
}

func inteiro(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		var n int
		fmt.Sscanf(strings.TrimSpace(t), "%d", &n)
		return n
	}
	return 0
}

func caminho(m map[string]interface{}, chaves ...string) string {
	var cur interface{} = m
	for _, k := range chaves {
		mm, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = mm[k]
	}
	s, _ := cur.(string)
	return s
}

func resumir(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
