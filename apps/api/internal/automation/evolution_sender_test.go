package automation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"schumacher-tur/api/internal/chat"
	"schumacher-tur/api/internal/shared/config"
)

func TestEvolutionSenderSendReply(t *testing.T) {
	var receivedPath string
	var receivedAPIKey string
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAPIKey = r.Header.Get("apikey")
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"key":{"id":"MSG-OUT-1"},
			"status":"SENT",
			"message":{"conversation":"ok"}
		}`))
	}))
	defer server.Close()

	sender := NewEvolutionSender(config.Config{
		EvolutionBaseURL:  server.URL,
		EvolutionAPIKey:   "secret-key",
		EvolutionInstance: "belle",
	})

	result, err := sender.SendReply(context.Background(), chat.SendReplyInput{
		Session: chat.Session{ContactKey: "5511888888888@s.whatsapp.net"},
		Message: chat.Message{Body: "ola"},
		Outbound: chat.ReplyOutbound{
			Recipient: "5511888888888@s.whatsapp.net",
		},
	})
	if err != nil {
		t.Fatalf("send reply: %v", err)
	}

	if receivedPath != "/message/sendText/belle" {
		t.Fatalf("unexpected path: %s", receivedPath)
	}
	if receivedAPIKey != "secret-key" {
		t.Fatalf("unexpected apikey header: %s", receivedAPIKey)
	}
	if got := asString(receivedBody["number"]); got != "5511888888888" {
		t.Fatalf("unexpected number: %s", got)
	}
	if got := asString(receivedBody["text"]); got != "ola" {
		t.Fatalf("unexpected text: %s", got)
	}
	textMessage, ok := receivedBody["textMessage"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected textMessage object")
	}
	if got := asString(textMessage["text"]); got != "ola" {
		t.Fatalf("unexpected textMessage.text: %s", got)
	}
	if result.ProviderMessageID != "MSG-OUT-1" {
		t.Fatalf("unexpected provider message id: %s", result.ProviderMessageID)
	}
	if result.ProviderStatus != "SENT" {
		t.Fatalf("unexpected provider status: %s", result.ProviderStatus)
	}
}

func TestEvolutionSenderRejectsResponseWithoutProviderMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"SENT"}`))
	}))
	defer server.Close()

	sender := NewEvolutionSender(config.Config{
		EvolutionBaseURL:  server.URL,
		EvolutionAPIKey:   "secret-key",
		EvolutionInstance: "belle",
	})

	_, err := sender.SendReply(context.Background(), chat.SendReplyInput{
		Session:  chat.Session{ContactKey: "5511888888888@s.whatsapp.net"},
		Message:  chat.Message{Body: "ola"},
		Outbound: chat.ReplyOutbound{Recipient: "5511888888888@s.whatsapp.net"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestEvolutionSenderSendMediaReply(t *testing.T) {
	var receivedPath string
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"key":{"id":"MSG-MEDIA-1"},
			"status":"PENDING"
		}`))
	}))
	defer server.Close()

	sender := NewEvolutionSender(config.Config{
		EvolutionBaseURL:  server.URL,
		EvolutionAPIKey:   "secret-key",
		EvolutionInstance: "belle",
	})

	result, err := sender.SendReply(context.Background(), chat.SendReplyInput{
		Session: chat.Session{ContactKey: "5511888888888@s.whatsapp.net"},
		Message: chat.Message{Body: "Documento do cliente"},
		Outbound: chat.ReplyOutbound{
			Recipient: "5511888888888@s.whatsapp.net",
			Payload: map[string]interface{}{
				"media_kind":      "document",
				"media_mime_type": "application/pdf",
				"media_file_name": "rg.pdf",
				"media_base64":    "ZmFrZS1wZGY=",
				"media_caption":   "Segue o documento",
			},
		},
	})
	if err != nil {
		t.Fatalf("send media reply: %v", err)
	}

	if receivedPath != "/message/sendMedia/belle" {
		t.Fatalf("unexpected path: %s", receivedPath)
	}
	if got := asString(receivedBody["mediatype"]); got != "document" {
		t.Fatalf("unexpected mediatype: %s", got)
	}
	if got := asString(receivedBody["mimetype"]); got != "application/pdf" {
		t.Fatalf("unexpected mimetype: %s", got)
	}
	if got := asString(receivedBody["fileName"]); got != "rg.pdf" {
		t.Fatalf("unexpected fileName: %s", got)
	}
	if got := asString(receivedBody["media"]); got != "ZmFrZS1wZGY=" {
		t.Fatalf("unexpected media payload")
	}
	if got := asString(receivedBody["caption"]); got != "Segue o documento" {
		t.Fatalf("unexpected caption: %s", got)
	}
	if result.ProviderMessageID != "MSG-MEDIA-1" {
		t.Fatalf("unexpected provider message id: %s", result.ProviderMessageID)
	}
}
