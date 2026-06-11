# apps/api/AGENTS.md

## Contexto da API

Esta é a API Go principal do Schumacher Tur.

Módulos importantes:

- `internal/chat`: sessão, memória, prompt, LLM, tools, reprocessamento, reservas via conversa.
- `internal/automation`: webhooks Evolution, envio de mensagens, jobs, buffers.
- `internal/bookings`: reservas.
- `internal/availability`: disponibilidade e busca de viagens.
- `internal/payments`: pagamentos, Pagarme e webhooks financeiros.
- `internal/shared/config`: configuração de ambiente.

## Regras de implementação

- Sempre procurar helpers existentes antes de criar novos.
- Usar `gofmt` em arquivos alterados.
- Preferir funções pequenas.
- Evitar duplicação entre `chat` e `payments`.
- Dado vindo de LLM/OCR/webhook deve ser validado antes de virar estado persistido.
- Em pagamentos, validar documento antes de chamar provider externo.
- Em webhook, validar assinatura antes de alterar estado.
- Em reservas e mensagens, preservar idempotência.

## Testes

Depois de alteração na API, rode pelo menos o pacote afetado:

```bash
go test ./internal/chat
go test ./internal/payments
go test ./internal/automation
go test ./cmd/api
```

Quando mexer em fluxo transversal:

```bash
go test ./internal/chat ./internal/automation ./internal/payments ./internal/bookings ./internal/availability ./cmd/api
```

## Segurança operacional

- Não logar CPF completo, token, secret, API key ou webhook secret.
- Não usar fallback inseguro com `PAGARME_SECRET_KEY` para validar webhook.
- Não criar nova cobrança em fallback de consulta de PIX.
- Não aceitar documento só por tamanho.
