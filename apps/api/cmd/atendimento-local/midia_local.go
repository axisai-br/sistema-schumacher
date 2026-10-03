package main

// /foto <arquivo> e /audio <arquivo>: passam um arquivo real pelo mesmo
// leitor de midia da producao (internal/atendimento/midia) e mandam o texto
// resultante como mensagem do cliente ("[foto de documento: ...]",
// transcricao do audio ou "[áudio não compreendido]").
//
// Imagem: usa a visao do provedor (NVIDIA: ATENDIMENTO_V2_MODELO_VISAO ou o
// modelo do agente; OpenAI: OPENAI_VISION_MODEL). Audio: transcricao da
// OpenAI (precisa de OPENAI_API_KEY), como em producao.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"schumacher-tur/api/internal/atendimento/canal"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm/provedor"
	"schumacher-tur/api/internal/atendimento/midia"
)

// canalArquivo entrega a midia a partir de um data URL ja montado.
type canalArquivo struct{ dataURL, mime string }

func (c canalArquivo) Nome() string { return "LOCAL" }
func (c canalArquivo) Normalizar(context.Context, []byte) (canal.Entrada, bool, error) {
	return canal.Entrada{}, false, nil
}
func (c canalArquivo) Enviar(context.Context, string, string) (string, error) { return "", nil }
func (c canalArquivo) BaixarMidia(context.Context, conversa.Mensagem) (string, string, error) {
	return c.dataURL, c.mime, nil
}

var mimePorExtensao = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp",
	".ogg": "audio/ogg", ".opus": "audio/ogg", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".wav": "audio/wav",
}

// lerMidiaLocal devolve o texto que a producao geraria para o arquivo.
func (s *sessao) lerMidiaLocal(ctx context.Context, tipo conversa.Tipo, caminho string) (string, map[string]any, error) {
	caminho = strings.Trim(strings.TrimSpace(caminho), `"`)
	if caminho == "" {
		return "", nil, errors.New("informe o caminho do arquivo")
	}
	b, err := os.ReadFile(caminho)
	if err != nil {
		return "", nil, err
	}
	mime := mimePorExtensao[strings.ToLower(filepath.Ext(caminho))]
	if mime == "" {
		return "", nil, fmt.Errorf("extensão não suportada: %s", filepath.Ext(caminho))
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	mc := midia.Config{
		OpenAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:     os.Getenv("OPENAI_BASE_URL"),
		ModeloTranscricao: os.Getenv("OPENAI_TRANSCRIPTION_MODEL"),
		ModeloVisao:       os.Getenv("OPENAI_VISION_MODEL"),
	}
	// Como em producao: com OPENAI_API_KEY e OPENAI_VISION_MODEL a visao vai para
	// a OpenAI mesmo com o LLM na NVIDIA.
	visaoOpenAI := mc.OpenAIAPIKey != "" && mc.ModeloVisao != ""
	if s.cfg.Prov.Provedor == provedor.Nvidia && !visaoOpenAI {
		modelo := strings.TrimSpace(os.Getenv("ATENDIMENTO_V2_MODELO_VISAO"))
		if modelo == "" {
			modelo = s.cfg.Modelo
		}
		mc.ModeloVisao = ""
		mc.Visao = midia.VisaoConfig{Provedor: provedor.Nvidia, APIKey: s.cfg.Prov.APIKey, BaseURL: s.cfg.Prov.BaseURL, Modelo: modelo}
	}
	p := midia.Novo(canalArquivo{dataURL: dataURL, mime: mime}, mc)
	texto, extra, err := p.Preparar(ctx, conversa.Mensagem{Tipo: tipo, Midia: map[string]any{"mimetype": mime}})
	return texto, extra, err
}

// cmdMidia le o arquivo, mostra o que a midia virou e manda como mensagem.
func (s *sessao) cmdMidia(ctx context.Context, tipo conversa.Tipo, caminho string) {
	texto, extra, err := s.lerMidiaLocal(ctx, tipo, caminho)
	if err != nil {
		s.printf("  [erro: %v]\n", err)
		return
	}
	s.printf("  [mídia lida como: %s]\n", texto)
	for _, k := range []string{"visao_status", "visao_modelo", "visao_erro", "transcricao_status", "transcricao_erro"} {
		if v, ok := extra[k]; ok {
			s.printf("  [%s: %v]\n", k, v)
		}
	}
	if s.enviar(ctx, texto) {
		s.processar(ctx)
	}
}
