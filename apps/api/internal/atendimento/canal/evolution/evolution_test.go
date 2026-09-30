package evolution

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/conversa"
)

func upsert(key, msg, tipo string) []byte {
	return []byte(`{"event":"messages.upsert","instance":"belle","data":{"key":` + key +
		`,"pushName":"Maria","message":` + msg + `,"messageType":"` + tipo + `","messageTimestamp":1772544357}}`)
}

const keyOK = `{"remoteJid":"554988709047@s.whatsapp.net","fromMe":false,"id":"MSG-1"}`

func TestNormalizarTexto(t *testing.T) {
	c := Novo(Config{})
	e, ok, err := c.Normalizar(context.Background(), upsert(keyOK, `{"conversation":" 06645648109 "}`, "conversation"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if e.Tipo != conversa.TipoTexto || e.Texto != "06645648109" || e.Contato != "554988709047@s.whatsapp.net" ||
		e.Telefone != "554988709047" || e.Nome != "Maria" || e.ProvedorID != "MSG-1" || e.DoProprioNumero {
		t.Fatalf("entrada inesperada: %+v", e)
	}
	if e.RecebidaEm.Unix() != 1772544357 {
		t.Fatalf("timestamp: %v", e.RecebidaEm)
	}
	e, ok, _ = c.Normalizar(context.Background(), upsert(keyOK, `{"extendedTextMessage":{"text":"oi"}}`, "extendedTextMessage"))
	if !ok || e.Texto != "oi" {
		t.Fatalf("extended: %+v", e)
	}
}

func TestNormalizarEnvelopeEAlt(t *testing.T) {
	c := Novo(Config{})
	corpo := `{"body":{"event":"messages.upsert","data":{"key":{"remoteJid":"","remoteJidAlt":"5511999990000@s.whatsapp.net","id":"X"},"message":{"conversation":"oi"},"messageType":"conversation"}}}`
	e, ok, err := c.Normalizar(context.Background(), []byte(corpo))
	if err != nil || !ok || e.Contato != "5511999990000@s.whatsapp.net" {
		t.Fatalf("%+v ok=%v err=%v", e, ok, err)
	}
	if e.RecebidaEm.IsZero() {
		t.Fatal("RecebidaEm deveria ter fallback")
	}
}

func TestNormalizarAudio(t *testing.T) {
	c := Novo(Config{})
	e, ok, err := c.Normalizar(context.Background(), upsert(keyOK,
		`{"audioMessage":{"mimetype":"audio/ogg; codecs=opus","seconds":7,"ptt":true}}`, "audioMessage"))
	if err != nil || !ok || e.Tipo != conversa.TipoAudio {
		t.Fatalf("%+v ok=%v err=%v", e, ok, err)
	}
	if e.Midia["mime"] != "audio/ogg; codecs=opus" || e.Midia["segundos"] != 7 ||
		e.Midia["message_id"] != "MSG-1" || e.Midia["from_me"] != false ||
		e.Midia["remote_jid"] != "554988709047@s.whatsapp.net" {
		t.Fatalf("midia: %#v", e.Midia)
	}
}

func TestNormalizarImagemDocumentoOutro(t *testing.T) {
	c := Novo(Config{})
	e, ok, _ := c.Normalizar(context.Background(), upsert(keyOK,
		`{"imageMessage":{"caption":"foto do documento","mimetype":"image/jpeg","url":"https://x/doc.jpg"}}`, "imageMessage"))
	if !ok || e.Tipo != conversa.TipoImagem || e.Texto != "foto do documento" || e.Midia["mime"] != "image/jpeg" {
		t.Fatalf("imagem: %+v", e)
	}
	e, ok, _ = c.Normalizar(context.Background(), upsert(keyOK,
		`{"imageMessage":{"mimetype":"image/png"}}`, "imageMessage"))
	if !ok || e.Texto != "" {
		t.Fatalf("imagem sem legenda: %+v", e)
	}
	e, ok, _ = c.Normalizar(context.Background(), upsert(keyOK,
		`{"documentMessage":{"fileName":"rg.pdf","mimetype":"application/pdf"}}`, "documentMessage"))
	if !ok || e.Tipo != conversa.TipoDocumento || e.Texto != "rg.pdf" || e.Midia["nome_arquivo"] != "rg.pdf" {
		t.Fatalf("doc: %+v", e)
	}
	e, ok, _ = c.Normalizar(context.Background(), upsert(keyOK,
		`{"documentMessage":{"fileName":"rg.pdf","caption":"meu RG"}}`, "documentMessage"))
	if !ok || e.Texto != "meu RG" {
		t.Fatalf("doc legenda: %+v", e)
	}
	for _, tp := range []string{"videoMessage", "stickerMessage", "locationMessage", "contactMessage"} {
		e, ok, _ = c.Normalizar(context.Background(), upsert(keyOK, `{"`+tp+`":{}}`, tp))
		if !ok || e.Tipo != conversa.TipoOutro {
			t.Fatalf("%s: %+v ok=%v", tp, e, ok)
		}
	}
}

func TestNormalizarFromMe(t *testing.T) {
	c := Novo(Config{})
	e, ok, _ := c.Normalizar(context.Background(), upsert(
		`{"remoteJid":"554988709047@s.whatsapp.net","fromMe":true,"id":"M2"}`, `{"conversation":"ja te respondo"}`, "conversation"))
	if !ok || !e.DoProprioNumero {
		t.Fatalf("%+v ok=%v", e, ok)
	}
}

func TestNormalizarIgnorados(t *testing.T) {
	c := Novo(Config{})
	casos := map[string][]byte{
		"grupo":         upsert(`{"remoteJid":"120363@g.us","fromMe":false,"id":"G"}`, `{"conversation":"oi"}`, "conversation"),
		"status":        upsert(`{"remoteJid":"status@broadcast","fromMe":false,"id":"S"}`, `{"conversation":"oi"}`, "conversation"),
		"reacao":        upsert(keyOK, `{"reactionMessage":{"text":"x"}}`, "reactionMessage"),
		"protocolo":     upsert(keyOK, `{"protocolMessage":{"type":0}}`, "protocolMessage"),
		"vazio":         upsert(keyOK, `{}`, ""),
		"texto-vazio":   upsert(keyOK, `{"conversation":"  "}`, "conversation"),
		"evento-status": []byte(`{"event":"messages.update","instance":"belle","data":{"key":` + keyOK + `,"status":"READ"}}`),
		"presenca":      []byte(`{"event":"presence.update","data":{"id":"x@s.whatsapp.net","presences":{}}}`),
	}
	for nome, corpo := range casos {
		_, ok, err := c.Normalizar(context.Background(), corpo)
		if ok || err != nil {
			t.Errorf("%s: ok=%v err=%v", nome, ok, err)
		}
	}
	if _, _, err := c.Normalizar(context.Background(), []byte(`nao-json`)); err == nil {
		t.Error("esperava erro para payload invalido")
	}
}

func TestBaixarMidia(t *testing.T) {
	raw := []byte("OggS-audio-bytes")
	var got map[string]interface{}
	var path, apikey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		apikey = r.Header.Get("apikey")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte(`{"base64":"` + base64.StdEncoding.EncodeToString(raw) + `","mimetype":"audio/ogg"}`))
	}))
	defer srv.Close()
	c := Novo(Config{BaseURL: srv.URL + "/", APIKey: "k", Instancia: "belle"})
	m := conversa.Mensagem{Tipo: conversa.TipoAudio, Midia: map[string]interface{}{
		"message_id": "MSG-1", "remote_jid": "5549@s.whatsapp.net", "from_me": false, "mime": "audio/ogg",
	}}
	url, mime, err := c.BaixarMidia(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/chat/getBase64FromMediaMessage/belle" || apikey != "k" {
		t.Fatalf("path=%s apikey=%s", path, apikey)
	}
	key := got["message"].(map[string]interface{})["key"].(map[string]interface{})
	if key["id"] != "MSG-1" || key["remoteJid"] != "5549@s.whatsapp.net" {
		t.Fatalf("req: %#v", got)
	}
	if mime != "audio/ogg" || url != "data:audio/ogg;base64,"+base64.StdEncoding.EncodeToString(raw) {
		t.Fatalf("url=%s mime=%s", url, mime)
	}
}

func TestBaixarMidiaErros(t *testing.T) {
	big := base64.StdEncoding.EncodeToString(make([]byte, limiteImagem+10))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"base64":"` + big + `"}`))
	}))
	defer srv.Close()
	c := Novo(Config{BaseURL: srv.URL, APIKey: "k", Instancia: "i"})
	m := conversa.Mensagem{Tipo: conversa.TipoImagem, Midia: map[string]interface{}{"message_id": "a", "remote_jid": "b"}}
	if _, _, err := c.BaixarMidia(context.Background(), m); err == nil || !strings.Contains(err.Error(), "limite") {
		t.Fatalf("esperava erro de limite, got %v", err)
	}
	if _, _, err := c.BaixarMidia(context.Background(), conversa.Mensagem{Tipo: conversa.TipoImagem}); err == nil {
		t.Fatal("esperava erro sem message_id")
	}
}

func TestBaixarMidiaStatusErro(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`not found`))
	}))
	defer srv.Close()
	c := Novo(Config{BaseURL: srv.URL, APIKey: "segredo", Instancia: "i"})
	_, _, err := c.BaixarMidia(context.Background(), conversa.Mensagem{Tipo: conversa.TipoAudio, Midia: map[string]interface{}{"message_id": "a", "remote_jid": "b"}})
	if err == nil || !strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "segredo") {
		t.Fatalf("err=%v", err)
	}
}

func TestEnviar(t *testing.T) {
	var got map[string]interface{}
	var path, apikey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		apikey = r.Header.Get("apikey")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"key":{"remoteJid":"x","fromMe":true,"id":"OUT-9"},"status":"PENDING"}`))
	}))
	defer srv.Close()
	c := Novo(Config{BaseURL: srv.URL, APIKey: "k", Instancia: "belle"})
	id, err := c.Enviar(context.Background(), "554988709047@s.whatsapp.net", "Olá!")
	if err != nil || id != "OUT-9" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	if path != "/message/sendText/belle" || apikey != "k" || got["number"] != "554988709047" || got["text"] != "Olá!" {
		t.Fatalf("path=%s got=%#v", path, got)
	}
	if c.Nome() != "WHATSAPP" {
		t.Fatal("Nome")
	}
}

func TestEnviarErroHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"message":"numero invalido"}`))
	}))
	defer srv.Close()
	c := Novo(Config{BaseURL: srv.URL, APIKey: "segredo", Instancia: "i"})
	_, err := c.Enviar(context.Background(), "5511@s.whatsapp.net", "oi")
	if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "numero invalido") || strings.Contains(err.Error(), "segredo") {
		t.Fatalf("err=%v", err)
	}
}
