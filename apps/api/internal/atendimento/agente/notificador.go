package agente

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

type notificadorWebhook struct {
	url  string
	http *http.Client
}

type notificadorNulo struct{}

func (notificadorNulo) AvisarTransferencia(context.Context, conversa.Conversa, string, string) error {
	return nil
}

// NovoNotificadorWebhook avisa a equipe por webhook. url vazia vira no-op.
func NovoNotificadorWebhook(url string, hc *http.Client) Notificador {
	url = strings.TrimSpace(url)
	if url == "" {
		return notificadorNulo{}
	}
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &notificadorWebhook{url: url, http: hc}
}

func (n *notificadorWebhook) AvisarTransferencia(ctx context.Context, c conversa.Conversa, motivo, resumo string) error {
	corpo, err := json.Marshal(map[string]any{
		"tipo":        "atendimento_transferido",
		"conversa_id": c.ID,
		"telefone":    c.Telefone,
		"nome":        c.Nome,
		"motivo":      motivo,
		"resumo":      resumo,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(corpo))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.http.Do(req)
	if err != nil {
		return fmt.Errorf("notificador: falha de rede")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("notificador: status %d", resp.StatusCode)
	}
	return nil
}
