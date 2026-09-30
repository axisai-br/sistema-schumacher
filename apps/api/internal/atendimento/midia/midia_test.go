package midia

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
)

type canalFake struct {
	dataURL, mime string
	err           error
	chamadas      int
}

func (c *canalFake) Nome() string { return "WHATSAPP" }
func (c *canalFake) Normalizar(context.Context, []byte) (canal.Entrada, bool, error) {
	return canal.Entrada{}, false, nil
}
func (c *canalFake) Enviar(context.Context, string, string) (string, error) { return "", nil }
func (c *canalFake) BaixarMidia(context.Context, conversa.Mensagem) (string, string, error) {
	c.chamadas++
	return c.dataURL, c.mime, c.err
}

const audioURL = "data:audio/ogg;base64,T2dnUw=="

func TestTextoEOutros(t *testing.T) {
	p := Novo(&canalFake{}, Config{})
	ctx := context.Background()
	if s, _, err := p.Preparar(ctx, conversa.Mensagem{Tipo: conversa.TipoTexto, Texto: "oi"}); err != nil || s != "oi" {
		t.Fatalf("texto: %q %v", s, err)
	}
	if s, _, _ := p.Preparar(ctx, conversa.Mensagem{Tipo: conversa.TipoOutro}); s != "[mensagem de um tipo que não consigo ler]" {
		t.Fatalf("outro: %q", s)
	}
	m := conversa.Mensagem{Tipo: conversa.TipoDocumento, Texto: "rg.pdf", Midia: map[string]any{"nome_arquivo": "rg.pdf"}}
	if s, _, _ := p.Preparar(ctx, m); s != "[documento recebido: rg.pdf]" {
		t.Fatalf("doc: %q", s)
	}
	m.Texto = "meu RG"
	if s, _, _ := p.Preparar(ctx, m); s != "[documento recebido: rg.pdf] meu RG" {
		t.Fatalf("doc legenda: %q", s)
	}
}

func TestAudioOK(t *testing.T) {
	var campos = map[string]string{}
	var auth, ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		ct = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
		}
		for k, v := range r.MultipartForm.Value {
			campos[k] = v[0]
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Errorf("file: %v", err)
		} else {
			b, _ := io.ReadAll(f)
			campos["_arquivo"] = h.Filename + ":" + string(b)
		}
		_, _ = w.Write([]byte(`{"text":" quero ir para Fraiburgo "}`))
	}))
	defer srv.Close()
	p := Novo(&canalFake{dataURL: audioURL, mime: "audio/ogg; codecs=opus"}, Config{OpenAIAPIKey: "sk-x", OpenAIBaseURL: srv.URL})
	s, extra, err := p.Preparar(context.Background(), conversa.Mensagem{Tipo: conversa.TipoAudio})
	if err != nil || s != "quero ir para Fraiburgo" {
		t.Fatalf("s=%q err=%v", s, err)
	}
	if extra["transcricao_status"] != "OK" || extra["transcricao_modelo"] != "gpt-4o-mini-transcribe" {
		t.Fatalf("extra=%v", extra)
	}
	if auth != "Bearer sk-x" || !strings.HasPrefix(ct, "multipart/form-data") {
		t.Fatalf("auth=%s ct=%s", auth, ct)
	}
	if campos["language"] != "pt" || campos["model"] != "gpt-4o-mini-transcribe" ||
		!strings.Contains(campos["prompt"], "Fraiburgo") || campos["_arquivo"] != "audio.ogg:Ogg"+"S" {
		t.Fatalf("campos=%v", campos)
	}
}

func TestAudioFalhas(t *testing.T) {
	ctx := context.Background()
	m := conversa.Mensagem{Tipo: conversa.TipoAudio}
	check := func(nome string, p *Preparador) {
		t.Helper()
		s, extra, err := p.Preparar(ctx, m)
		if err != nil || s != "[áudio não compreendido]" || extra["transcricao_status"] != "FALHOU" || extra["transcricao_erro"] == "" {
			t.Errorf("%s: s=%q extra=%v err=%v", nome, s, extra, err)
		}
	}
	vazio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"text":"  "}`)) }))
	defer vazio.Close()
	erro := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("oops"))
	}))
	defer erro.Close()
	check("sem chave", Novo(&canalFake{dataURL: audioURL}, Config{}))
	check("download", Novo(&canalFake{err: errors.New("x")}, Config{OpenAIAPIKey: "k", OpenAIBaseURL: erro.URL}))
	check("vazio", Novo(&canalFake{dataURL: audioURL, mime: "audio/ogg"}, Config{OpenAIAPIKey: "k", OpenAIBaseURL: vazio.URL}))
	check("http 500", Novo(&canalFake{dataURL: audioURL, mime: "audio/ogg"}, Config{OpenAIAPIKey: "k", OpenAIBaseURL: erro.URL}))
}

func servidorVisao(t *testing.T, saida string, corpo *map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if corpo != nil {
			_ = json.Unmarshal(b, corpo)
		}
		resp := map[string]any{"output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": saida}}}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestImagemDocumento(t *testing.T) {
	var req map[string]any
	srv := servidorVisao(t, `{"e_documento":true,"nome":"Maria Silva","cpf":"123.456.789-09","rg":"","descricao":"rg"}`, &req)
	defer srv.Close()
	p := Novo(&canalFake{dataURL: "data:image/jpeg;base64,AAAA", mime: "image/jpeg"},
		Config{OpenAIAPIKey: "k", OpenAIBaseURL: srv.URL, ModeloVisao: "gpt-4o-mini"})
	s, extra, err := p.Preparar(context.Background(), conversa.Mensagem{Tipo: conversa.TipoImagem, Texto: "meu documento"})
	if err != nil {
		t.Fatal(err)
	}
	if s != "[foto de documento: nome Maria Silva, CPF 123.456.789-09] meu documento" {
		t.Fatalf("s=%q", s)
	}
	if extra["e_documento"] != true {
		t.Fatalf("extra=%v", extra)
	}
	if req["model"] != "gpt-4o-mini" {
		t.Fatalf("req=%v", req)
	}
	raw, _ := json.Marshal(req)
	for _, frag := range []string{`"input_image"`, `data:image/jpeg;base64,AAAA`, `"json_schema"`, `"e_documento"`} {
		if !strings.Contains(string(raw), frag) {
			t.Errorf("request sem %s: %s", frag, raw)
		}
	}
}

func TestImagemComum(t *testing.T) {
	srv := servidorVisao(t, `{"e_documento":false,"nome":"","cpf":"","rg":"","descricao":"uma paisagem"}`, nil)
	defer srv.Close()
	p := Novo(&canalFake{dataURL: "data:image/jpeg;base64,AAAA"}, Config{OpenAIAPIKey: "k", OpenAIBaseURL: srv.URL, ModeloVisao: "m"})
	s, _, _ := p.Preparar(context.Background(), conversa.Mensagem{Tipo: conversa.TipoImagem})
	if s != "[imagem: uma paisagem]" {
		t.Fatalf("s=%q", s)
	}
}

func TestImagemFallbacks(t *testing.T) {
	ctx := context.Background()
	m := conversa.Mensagem{Tipo: conversa.TipoImagem, Texto: "olha"}
	// visao desabilitada: nem baixa
	cf := &canalFake{dataURL: "data:image/jpeg;base64,AAAA"}
	if s, _, err := Novo(cf, Config{OpenAIAPIKey: "k"}).Preparar(ctx, m); err != nil || s != "[imagem recebida] olha" || cf.chamadas != 0 {
		t.Fatalf("desabilitada: %q %v %d", s, err, cf.chamadas)
	}
	// erro de download
	if s, extra, err := Novo(&canalFake{err: errors.New("x")}, Config{OpenAIAPIKey: "k", ModeloVisao: "m"}).Preparar(ctx, m); err != nil || s != "[imagem recebida] olha" || extra["visao_status"] != "FALHOU" {
		t.Fatalf("download: %q %v %v", s, extra, err)
	}
	// erro HTTP e JSON invalido
	e500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer e500.Close()
	if s, _, err := Novo(cf, Config{OpenAIAPIKey: "k", OpenAIBaseURL: e500.URL, ModeloVisao: "m"}).Preparar(ctx, m); err != nil || s != "[imagem recebida] olha" {
		t.Fatalf("500: %q %v", s, err)
	}
	ruim := servidorVisao(t, `nao e json`, nil)
	defer ruim.Close()
	if s, _, err := Novo(cf, Config{OpenAIAPIKey: "k", OpenAIBaseURL: ruim.URL, ModeloVisao: "m"}).Preparar(ctx, m); err != nil || s != "[imagem recebida] olha" {
		t.Fatalf("json ruim: %q %v", s, err)
	}
}
