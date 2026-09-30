# API (apps/api)

Este pacote sera a API Go do sistema.

## Atendimento v2

Agente de atendimento por WhatsApp (pacote `internal/atendimento`), desligado por padrao. Com `ATENDIMENTO_V2_ENABLED=true` a API monta o modulo, inicia o worker, expoe `POST /webhooks/evolution/v2` (autenticado por `EVOLUTION_WEBHOOK_SECRET`) e as rotas da equipe em `/atendimento/conversas`. O webhook antigo (`/webhooks/evolution`) passa a encaminhar `messages.upsert` ao v2 quando o telefone esta liberado; os demais eventos seguem no fluxo antigo. Exige `OPENAI_API_KEY` e `EVOLUTION_BASE_URL`, `EVOLUTION_API_KEY`, `EVOLUTION_INSTANCE`; sem eles a API nao sobe.

| Variavel | Padrao | Descricao |
| --- | --- | --- |
| `ATENDIMENTO_V2_ENABLED` | `false` | Liga o modulo. |
| `ATENDIMENTO_V2_TELEFONES` | vazio | Telefones (so digitos, separados por virgula) atendidos pelo v2; vazio = todos. |
| `ATENDIMENTO_V2_MODELO` | `OPENAI_MODEL` | Modelo do agente. |
| `ATENDIMENTO_V2_CONCORRENCIA` | `4` | Conversas processadas em paralelo. |
| `ATENDIMENTO_V2_DEBOUNCE_MS` | `2000` | Espera por novas mensagens antes de responder. |
| `ATENDIMENTO_V2_SINAL_POR_PAGANTE` | `250` | Sinal (R$) por passageiro pagante. |
| `ATENDIMENTO_V2_JUIZ` | `llm` | `llm`, `jev` ou `off`. `jev` exige `TYPESAFE_API_KEY`; sem ela cai para `llm` (com log). |
| `TYPESAFE_API_KEY` | vazio | Chave do Jev (TypeSafe). |
| `ATENDIMENTO_V2_ALERTA_WEBHOOK_URL` | `CHAT_REVIEW_ALERT_WEBHOOK_URL` | Webhook de aviso de transferencia para humano. |
