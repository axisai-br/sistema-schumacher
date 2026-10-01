# API (apps/api)

Este pacote sera a API Go do sistema.

## Atendimento v2

Agente de atendimento por WhatsApp (pacote `internal/atendimento`), desligado por padrao. Com `ATENDIMENTO_V2_ENABLED=true` a API monta o modulo, inicia o worker, expoe `POST /webhooks/evolution/v2` (autenticado por `EVOLUTION_WEBHOOK_SECRET`) e as rotas da equipe em `/atendimento/conversas`. O webhook antigo (`/webhooks/evolution`) passa a encaminhar `messages.upsert` ao v2 quando o telefone esta liberado; os demais eventos seguem no fluxo antigo. Exige a chave do provedor de LLM escolhido (`NVIDIA_API_KEY` por padrao, ou `OPENAI_API_KEY` com `LLM_PROVEDOR=openai`) e `EVOLUTION_BASE_URL`, `EVOLUTION_API_KEY`, `EVOLUTION_INSTANCE`; sem eles a API nao sobe.

### Provedor de LLM

O agente e o juiz usam a API da NVIDIA (NVIDIA NIM, `POST {NVIDIA_BASE_URL}/chat/completions`, compativel com OpenAI) por padrao; a OpenAI (Responses API) continua disponivel com `LLM_PROVEDOR=openai`. Com a NVIDIA o modelo padrao e `z-ai/glm-5.3`, escolhido em teste de 30/09/2026 (tool calling correto, ~8 s por resposta). Alternativas disponiveis: `deepseek-ai/deepseek-v4.1-flash` (correto, porem ~40 s), `nvidia/nemotron-3-super-120b-a12b` (rapido, mas tende a nao chamar ferramentas) e `moonshotai/kimi-k3` (modelo de raciocinio, ~30 s; so funciona com `LLM_TEMPERATURA=1`, com 0.6 degenera). O conteudo de raciocinio e descartado. Se o modelo devolver a chamada de ferramenta como texto JSON (comum em Llama), o cliente converte para chamada real. A saida estruturada (juiz) usa `nvext.guided_json` por padrao (`LLM_MODO_JSON`).

Audio e visao: com `LLM_PROVEDOR=nvidia`, a leitura de imagens usa a NVIDIA (`ATENDIMENTO_V2_MODELO_VISAO`, padrao = modelo do agente); a transcricao de audio ainda usa a OpenAI e `OPENAI_API_KEY` passa a ser opcional (sem ela o audio vira "[audio nao compreendido]", com um aviso no log).

| Variavel | Padrao | Descricao |
| --- | --- | --- |
| `LLM_PROVEDOR` | `nvidia` | `nvidia` ou `openai`. |
| `NVIDIA_API_KEY` | vazio | Chave da NVIDIA (obrigatoria com `nvidia`). |
| `NVIDIA_BASE_URL` | `https://integrate.api.nvidia.com/v1` | Base da API da NVIDIA. |
| `LLM_MODO_JSON` | `nvext` | Saida JSON na NVIDIA: `nvext` (guided_json), `response_format` ou `prompt` (schema no prompt + extracao do JSON). |
| `LLM_REASONING_EFFORT` | `low` | `reasoning_effort` enviado na NVIDIA: `low`, `medium`, `high` ou `max` (baixo = menor latencia). |
| `LLM_TEMPERATURA` | `0.3` | Temperatura na NVIDIA. |
| `ATENDIMENTO_V2_MODELO_VISAO` | modelo do agente | Modelo de visao (so `nvidia`). |

| Variavel | Padrao | Descricao |
| --- | --- | --- |
| `ATENDIMENTO_V2_ENABLED` | `false` | Liga o modulo. |
| `ATENDIMENTO_V2_TELEFONES` | vazio | Telefones (so digitos, separados por virgula) atendidos pelo v2; vazio = todos. |
| `ATENDIMENTO_V2_MODELO` | `z-ai/glm-5.3` (nvidia) / `OPENAI_MODEL` ou `gpt-4.1-mini` (openai) | Modelo do agente. |
| `ATENDIMENTO_V2_CONCORRENCIA` | `4` | Conversas processadas em paralelo. |
| `ATENDIMENTO_V2_DEBOUNCE_MS` | `2000` | Espera por novas mensagens antes de responder. |
| `ATENDIMENTO_V2_SINAL_POR_PAGANTE` | `250` | Sinal (R$) por passageiro pagante. |
| `ATENDIMENTO_V2_JUIZ` | `llm` | `llm`, `jev` ou `off`. `jev` exige `TYPESAFE_API_KEY`; sem ela cai para `llm` (com log). |
| `TYPESAFE_API_KEY` | vazio | Chave do Jev (TypeSafe). |
| `ATENDIMENTO_V2_ALERTA_WEBHOOK_URL` | `CHAT_REVIEW_ALERT_WEBHOOK_URL` | Webhook de aviso de transferencia para humano. |
