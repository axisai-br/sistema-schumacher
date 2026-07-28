# Napkin

## Corrections
| Date | Source | What Went Wrong | What To Do Instead |
|------|--------|-----------------|--------------------|
| 2026-04-01 | self | Em `chi`, montar subrouter em `/trips/{tripId}` no `trip_operations` sobrescreveu rotas irmÃ£s de `trips` (ex.: `/trips/{tripId}/segment-prices`) e gerou `404 page not found`. | Evitar subrouter compartilhado em caminho paramÃ©trico quando outro mÃ³dulo tambÃ©m usa `/trips/{tripId}*`; registrar rotas completas (`/trips/{tripId}/...`) para coexistÃªncia previsÃ­vel. |
| 2026-04-01 | self | Teste manual de `PATCH` via PowerShell corrompeu acentuaÃ§Ã£o em `route_name` por encoding de terminal. | Evitar atualizar campos com acento via shell PowerShell; preferir valores ASCII em testes manuais ou atualizar via app/API cliente UTF-8 garantido. |

## User Preferences
- (accumulate here as you learn them)
- Adequar o sistema atual ao banco real `schumacher` (MCP `supabase-schumacher`) focado em venda de passagens.
- Na tela de Viagens, priorizar visÃ£o por viagem real do Ã´nibus e exposiÃ§Ã£o de paradas da viagem; evitar UI com todas as combinaÃ§Ãµes de trechos abertas por padrÃ£o.
- A tela `Viagens` deve representar rotas ativas (nÃ£o combinaÃ§Ãµes/subrotas) e abrir paradas dentro da prÃ³pria tela.
- Exibir no front dados de lotacao/capacidade de viagem (ex.: vagas e assentos totais) quando houver no backend.

## Patterns That Work
- (approaches that succeeded)
- Para snapshot de estrutura do banco antigo, salvar em `docs-sistema/database/` com data no nome.
- `git grep` funcionou para localizar credenciais/config quando `rg` falhou no ambiente.
- Quando aparecer `404 page not found` apenas em alguns endpoints irmÃ£os, validar conflito de roteamento entre handlers antes de mexer em SQL/negÃ³cio.

## Patterns That Don't Work
- (approaches that failed and why)
- `rg` pode falhar com "Acesso negado" neste ambiente; usar `git grep` ou `Select-String` como fallback.

## Domain Notes
- (project/domain context that matters)
- Banco ativo de passagens (`supabase-schumacher`, projeto `balloihmdzatwynlblad`) usa schema legado textual; novas melhorias devem manter compatibilidade com `available_segments` durante transição.
- Gestão de rotas agora inclui geolocalização de parada (`latitude`/`longitude` em `stops`); validação exige o par completo e faixas válidas (lat -90..90, long -180..180).
- Na criação de rota (`Nova rota`), origem e destino também devem ter coordenadas explícitas para garantir persistência geográfica já no cadastro inicial.
- Requisito de UX atualizado: coordenadas devem ser resolvidas automaticamente pelo nome da cidade (geocoding), sem depender de digitação manual de latitude/longitude no cadastro.
- Em transações de criação de rota/parada, evitar depender de erro `23505` para continuar fluxo; conflito em insert aborta a transação e causa `SQLSTATE 25P02`.
- Para geocoding no Nominatim, preferir resultados com `addresstype` de município/cidade (`municipality`, `city`, `town`, etc.) para evitar coordenadas de rua/POI.

| 2026-04-01 | self | Endpoint de busca de cidades retornava erro quando Nominatim nao encontrava resultados e o front mostrava falha (500) durante digitacao. | Para autocomplete, retornar lista vazia ([]) quando nao houver candidatos; reservar erro para falha real de integracao. |
| 2026-04-01 | self | Strings com acento em Go ficaram corrompidas por encoding de terminal no Windows. | Preferir escapes Unicode (\\uXXXX) em testes/normalizacao de acentos quando houver risco de codepage no shell. |

- O atendimento WhatsApp usa Evolution -> `/webhooks/evolution` -> `chat.Ingest` -> buffer server-side -> `Reprocess`/OpenAI -> draft/auto-send -> Evolution; a tela `/atendimentos` consome `/chat/*` e SSE.
- No fluxo WhatsApp, `CHAT_BUFFER_AUTO_FLUSH_ENABLED` e `CHAT_AUTO_SEND_RETRY_ENABLED` ficam desligados no `.env.example`; sem override no `/etc/schumacher/api.env`, mensagens podem permanecer pendentes e drafts não são enviados.
- Ao revisar atendimento, conferir RBAC/identidade no contexto: rotas de chat e usuários estão apenas atrás de autenticação, enquanto owner IDs são aceitos no payload; handoff/resolve/reply precisam validar o usuário autenticado.
- A revisão de drafts de automação, inclusive booking/payment, ainda não tem UI correspondente em `apps/app/src/pages/Atendimentos`; mudanças futuras devem expor draft, motivo de revisão e aprovação controlada.

