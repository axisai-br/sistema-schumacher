# SEC-2026-08-18 — Hardening Supabase Data API / RLS

## 1. Estado reconciliado

Baseline do planejamento:

```text
branch: wip/h-2026-07-27a-recovery
HEAD/origin-main: 5dfd9a09a5c93ba5fb2d7d68d59d322ce0bf0436
checkpoint contido: 4eb543cb27cfa6527c0f225383d2f07fb9d38b97
working tree na reconciliação: limpo
```

Este track é independente da fila funcional. Não altera os status ou gates de
H-2026-07-27A, H-2026-07-16B ou 3.6F.

O inventário de produção foi coletado com `transaction_read_only=on`. Foram
confirmadas 29 tabelas, 15 routines e uma sequence no schema `public`. Não há
policies nem tabelas em publication.

## 2. Fronteira de confiança e ameaça

```text
Frontend
  -> Supabase Auth/session
  -> token autenticado
  -> API Schumacher

Data API/PostgREST
  -> anon/authenticated
  -> grants + RLS + policies

API Go
  -> PostgreSQL direto como postgres
  -> BYPASSRLS
```

Storage e Auth são superfícies separadas. Nenhuma tabela `public` foi
comprovada como `DATA_API_REQUIRED`.

Evidência versionada e operacional:

- zero uso de `supabase.from`, `client.from`, `/rest/v1`, `/graphql/v1`,
  `supabase.rpc` ou `/rpc/` no repo;
- telas de negócio usam `apps/app/src/services/api.ts`;
- aproximadamente 142 horas e 625 requests no Kong, com zero `/rest/v1` e
  zero `/graphql/v1`;
- operações de negócio conhecidas passam pela API Go.

Essa evidência é forte, mas não prova a inexistência de consumidor externo
raro. Ausência de evidência deixa o objeto `UNKNOWN_BLOCKED`.

### Finding: FUNCTION/RPC AUTHORITY EXPOSURE

Nove functions `public` são catalogalmente elegíveis como RPC para anon,
authenticated e service_role. Todas as 15 routines possuem `EXECUTE`
diretamente para PUBLIC, anon, authenticated, postgres e service_role.

Como são `SECURITY INVOKER`, o efeito depende também dos privilégios SQL da
role chamadora. A composição entre `EXECUTE RPC` e grants amplos de tabela é
exposição de autoridade comprovada, não exploração comprovada.

## 3. Grants, RLS e policies

- `REVOKE` remove autoridade SQL/Data API.
- RLS limita operações de roles sem `BYPASSRLS`.
- Policy não concede grant.
- `ENABLE RLS` não remove grant.
- `postgres` e `service_role` possuem `BYPASSRLS`.
- FORCE RLS é proibido.
- Nenhuma policy permissiva será criada para restaurar funcionamento sem um
  consumidor Data API comprovado.
- `service_role` permanece inalterado nesta versão.

## 4. Inventário das 29 tabelas

Legenda: `ALL7` significa `SELECT, INSERT, UPDATE, DELETE, TRUNCATE,
REFERENCES, TRIGGER`; `-` significa nenhum privilégio efetivo. Todas possuem
zero policies e FORCE RLS desabilitado.

| Tabela | Owner | RLS | anon/auth/service | Consumidor comprovado | Classe | Confiança | Dependências relevantes |
|---|---|---:|---|---|---|---|---|
| affiliate_recipients | postgres | false | ALL7/ALL7/ALL7 | API users/affiliate | BACKEND_ONLY | alta | usuários |
| affiliate_withdrawals | postgres | false | ALL7/ALL7/ALL7 | API affiliate | BACKEND_ONLY | alta | user_profiles |
| automation_job_runs | postgres | false | ALL7/ALL7/ALL7 | jobs de automation | BACKEND_ONLY | alta | user_profiles |
| available_segments | postgres | false | ALL7/ALL7/ALL7 | availability, imports e refresh | BACKEND_ONLY | média-alta | stops, routes, trips |
| booking_payment_details | postgres | false | ALL7/ALL7/ALL7 | bookings/payments/reports/trips | BACKEND_ONLY | alta | bookings; trigger |
| bookings | postgres | false | ALL7/ALL7/ALL7 | bookings/chat/automation/payments | BACKEND_ONLY | alta | stops, trips; trigger |
| chat_handoffs | postgres | false | ALL7/ALL7/ALL7 | chat repository | BACKEND_ONLY | alta | user_profiles, chat_sessions |
| chat_messages | postgres | false | ALL7/ALL7/ALL7 | chat e automation | BACKEND_ONLY | alta | chat_sessions |
| chat_sessions | postgres | false | ALL7/ALL7/ALL7 | chat, painel e workers | BACKEND_ONLY | alta | user_profiles |
| chat_tool_calls | postgres | false | ALL7/ALL7/ALL7 | ledger de tools do chat | BACKEND_ONLY | alta | messages, sessions |
| fiscal_documents | confirmar no pré-check | false | -/-/- | API fiscal/trip operations | BACKEND_ONLY | alta | trips |
| manifest_data | postgres | false | ALL7/ALL7/ALL7 | read model/imports/reports | BACKEND_ONLY | média-alta | bookings, trips; triggers |
| outbound_messages | postgres | false | ALL7/ALL7/ALL7 | chat delivery/automation | BACKEND_ONLY | alta | jobs, sessions |
| pagarme_webhook_events | postgres no checkpoint | true | -/-/ALL7 | webhook/idempotência affiliate | BACKEND_ONLY | alta | já endurecida |
| passengers | postgres | false | ALL7/ALL7/ALL7 | bookings/trips/reports/chat | BACKEND_ONLY | alta | bookings, stops, trips; trigger |
| payment_events | postgres | false | ALL7/ALL7/ALL7 | ledger payments | BACKEND_ONLY | alta | payments |
| payments | postgres | false | ALL7/ALL7/ALL7 | payments/automation/chat/reports | BACKEND_ONLY | alta | bookings |
| roles | postgres no checkpoint | true | -/-/ALL7 | users/profile/affiliate | BACKEND_ONLY | alta | user_roles; já endurecida |
| route_segment_prices | postgres | false | ALL7/ALL7/ALL7 | availability/pricing/routes/trips | BACKEND_ONLY | alta | stops, routes; trigger |
| routes | postgres | false | ALL7/ALL7/ALL7 | routes/trips/availability/imports | BACKEND_ONLY | alta | trips, prices |
| sheet_sync_queue | postgres | false | ALL7/ALL7/ALL7 | consumidor não comprovado | UNKNOWN_BLOCKED | baixa | view, triggers, sequence |
| stops | postgres | false | ALL7/ALL7/ALL7 | routes/trips/availability/bookings | BACKEND_ONLY | alta | prices, trips, bookings |
| travel_authorization_check_items | confirmar | false | -/-/- | nenhum | UNKNOWN_BLOCKED | baixa | travel_authorizations |
| travel_authorizations | confirmar | false | -/-/- | nenhum | UNKNOWN_BLOCKED | baixa | check items |
| trip_contractors | confirmar | false | -/-/- | nenhum | UNKNOWN_BLOCKED | baixa | não comprovadas |
| trip_stops | postgres | false | ALL7/ALL7/ALL7 | trips/routes/availability/pricing | BACKEND_ONLY | alta | stops, trips |
| trips | postgres | false | ALL7/ALL7/ALL7 | trips/availability/bookings/reports | BACKEND_ONLY | alta | routes; trigger |
| user_profiles | postgres | false | ALL7/ALL7/ALL7 | users/profile/chat | BACKEND_ONLY | alta | affiliate/automation/chat |
| user_roles | postgres | false | ALL7/ALL7/ALL7 | users/profile/affiliate authz | BACKEND_ONLY | alta | roles |

Resultado: 25 `BACKEND_ONLY`, zero `DATA_API_REQUIRED`, zero
`SYSTEM_AUTH_STORAGE` e quatro `UNKNOWN_BLOCKED`.

## 5. Dossiê das seis tabelas críticas

| Tabela | Consumidor real | Data API/anon/auth | Mudança mínima futura | Smoke positivo obrigatório |
|---|---|---|---|---|
| user_roles | users/profile/affiliate via PostgreSQL | nenhuma necessidade comprovada | revoke anon/auth + ENABLE RLS, sem policy | login, `/users/me`, alteração controlada de acesso |
| user_profiles | users/profile/chat via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | bootstrap de profile e leitura administrativa |
| passengers | bookings/trips/reports/chat via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | reserva controlada, detalhes e relatório |
| bookings | bookings/chat/automation/payments via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | list/get e fixture segura de reserva/expiração |
| booking_payment_details | bookings/payments/reports via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | leitura e atualização transacional controlada |
| payments | payments/automation/chat/reports via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | list/status e idempotência sem cobrança real |

Em todas, `service_role` permanece como está e o impacto esperado na API Go é
nulo por causa do acesso direto como `postgres`. Essa expectativa precisa ser
comprovada por smoke separado.

## 6. Inventário das routines

Todas são owner `postgres`, `SECURITY INVOKER`, `VOLATILE`, parallel `UNSAFE`
e possuem grants diretos de EXECUTE para PUBLIC, anon, authenticated,
postgres e service_role.

| Signature | Finalidade/efeito provado | Consumidor | RPC | Classe | Confiança |
|---|---|---|---:|---|---|
| build_sheet_sync_payload(text,text) | menções a bookings/passengers/trips/payment/manifest; corpo não versionado | nenhum | sim | UNKNOWN_BLOCKED | baixa |
| claim_sheet_sync_batch(integer) | relacionado à fila; corpo não versionado | worker não encontrado | sim | UNKNOWN_BLOCKED | baixa |
| enqueue_manifest_for_booking(text) | menção a manifest; corpo não versionado | nenhum | sim | UNKNOWN_BLOCKED | baixa |
| enqueue_sheet_sync(text,text,sheet_sync_operation) | relacionado à fila | nenhum | sim | UNKNOWN_BLOCKED | baixa |
| mark_sheet_sync_done(bigint) | relacionado à fila | worker não encontrado | sim | UNKNOWN_BLOCKED | baixa |
| mark_sheet_sync_retry(bigint,text,integer,integer) | relacionado à fila | worker não encontrado | sim | UNKNOWN_BLOCKED | baixa |
| refresh_available_segments_for_route(text) | lê trips/stops/prices; recria available_segments | API routes/trips por SQL direto | sim | BACKEND_ONLY | alta |
| refresh_manifest_data() | lê fontes; trunca/recria manifest | nenhum caller versionado | sim | UNKNOWN_BLOCKED | média |
| refresh_manifest_data_for_booking(text) | lê fontes; apaga/recria manifest do booking | nenhum caller versionado | sim | UNKNOWN_BLOCKED | média |
| set_route_segment_prices_updated_at() | atualiza NEW.updated_at | trigger versionado | não | TRIGGER_INTERNAL | alta |
| set_sheet_sync_queue_updated_at() | corpo ausente | trigger da fila | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_booking_payment_details() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_bookings() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_passengers() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_trips() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |

Não há DDL de sheet sync no HEAD nem no histórico Git local. O endpoint
`/automation/jobs/sheet-sync/run` retorna `NotImplemented`; existem variáveis
de configuração Google Sheets, mas nenhum worker/cliente implementado.

## 7. Sequence

Existe somente `public.sheet_sync_queue_id_seq`, owner `postgres`, bigint, com
`SELECT/UPDATE/USAGE` para anon, authenticated, postgres e service_role. A
dependência interna tipo `i` com `sheet_sync_queue.id` foi comprovada, mas o
DDL da coluna e o mecanismo de geração não foram encontrados. A ausência de
linha em `column_default` não autoriza inferência. Classificação:
`UNKNOWN_BLOCKED`.

## 8. Protocolo comum dos lotes

Cada `/goal` autoriza exatamente um lote ou sublote.

Pré-check:

- confirmar SHA/deploy alvo e frescor do inventário;
- conferir owner, RLS, FORCE, policies, ACL direto e privilégios efetivos;
- confirmar ausência de publication e de novo consumidor Data API/RPC;
- capturar ACL anterior em evidência sanitizada;
- abortar em qualquer divergência.

Mudança padrão de tabela `BACKEND_ONLY` exposta:

1. revogar os sete privilégios de anon e authenticated;
2. habilitar RLS;
3. não criar policy;
4. preservar service_role, postgres, owner e FORCE.

Verificação:

- RLS true, FORCE false e policies zero;
- anon/auth sem privilégio direto ou efetivo;
- service_role idêntico;
- Data API anon e authenticated negada em leitura e escrita;
- consumidor pela API Go verde.

Rollback de tabela originalmente exposta:

1. desabilitar RLS;
2. restaurar os sete privilégios capturados para anon/authenticated;
3. preservar service_role;
4. repetir pós-check e smoke da API.

### Gate `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`

Enquanto `sheet_sync_queue`, `sheet_sync_queue_id_seq` ou as routines de
sheet sync permanecerem `UNKNOWN_BLOCKED`:

- nenhuma verificação em produção pode provocar deliberadamente
  `INSERT`/`UPDATE`/`DELETE` em `trips`, `bookings`, `passengers` ou
  `booking_payment_details` apenas para testar trigger ou enqueue;
- toda prova comportamental de trigger/enqueue que possa escrever na fila ou
  avançar sua sequence ocorre somente em PostgreSQL efêmero/de teste;
- em produção, os lotes afetados ficam limitados a pré/pós-checks read-only e
  aos smokes legítimos que não escrevam nesses objetos-fonte; uma operação
  orgânica observada não autoriza criar uma operação sintética;
- o lote 2B, os lotes 9, 11, 12 e 13, assim como seus dependentes 10, 14 e
  15, não podem contornar este gate;
- qualquer verificação que exija a escrita indireta permanece bloqueada até a
  reconciliação e reclassificação explícita do subsistema sheet sync.

O gate não reclassifica nem autoriza mutação de `sheet_sync_queue`, sua
sequence ou suas routines `UNKNOWN_BLOCKED`.

## 9. Rollout em lotes

### Lote 1 — default ACL de TABLE

Revogar ALL7 do default ACL de `postgres` em `public` para anon/authenticated.
Não altera tabelas existentes. Verificar com `pg_default_acl` e objeto
descartável somente em PostgreSQL efêmero. Rollback: restaurar ALL7.

### Lote 2A — default privileges futuros de FUNCTION

Objetivo autorizado: revogar EXECUTE de PUBLIC, anon e authenticated somente
para futuras functions `postgres`-owned em `public`, preservando functions
existentes, postgres, service_role e o comportamento dos demais schemas.

**Status: UNKNOWN_BLOCKED no escopo public-only.** PostgreSQL soma defaults por
schema ao default global. Portanto,
`ALTER DEFAULT PRIVILEGES ... IN SCHEMA public REVOKE EXECUTE ... FROM PUBLIC`
não neutraliza o EXECUTE global nativo de PUBLIC. O revoke capaz de removê-lo
é global e altera futuras functions `postgres`-owned em todos os schemas. Esse
efeito cross-schema não está autorizado; editar este plano não constitui
autorização.

Também não é permitido conceder EXECUTE global incondicional a service_role:
se o snapshot real já tiver PUBLIC global ausente e service_role apenas em
`public` ou sem grant, esse comando criaria autoridade cross-schema nova.

O inventário operacional disponível cobre somente `public`. A ausência de DDL
ou caller no repo não prova ausência de schemas, deployers ou consumidores
externos. Todo alcance cross-schema permanece `UNKNOWN_BLOCKED` até inventário
real e evidência operacional suficientes ou aceitação explícita posterior do
risco. Nenhuma dessas condições foi satisfeita nesta rodada.

Não existe migration executável para o Lote 2A neste estado. Não criar event
trigger, wrapper ou workaround. Uma futura proposta que altere qualquer ACL
deverá capturar no pré-check cada valor individual — PUBLIC global,
service_role global e PUBLIC/anon/authenticated schema-locais — e o rollback
deverá restaurar cada item exatamente ao valor do snapshot, sem GRANT ou
REVOKE incondicional.

### Lote 2B — RPC backend-only atual

Alvo exato: `refresh_available_segments_for_route(text)`. Revogar EXECUTE de
PUBLIC, anon e authenticated; preservar postgres/service_role.

RED seguro somente em banco efêmero. O pós-check de produção mantém a negação
de `POST /rest/v1/rpc/refresh_available_segments_for_route` para anon e
authenticated.

Enquanto o `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` estiver ativo, a prova
positiva do consumidor backend em produção:

- exclui explicitamente qualquer create/update/write pelo fluxo `trips`;
- não pode provocar deliberadamente trigger/enqueue de sheet sync;
- deve usar um call path versionado cuja inspeção prévia prove ausência de
  escrita em `trips`, `bookings`, `passengers` e
  `booking_payment_details`, seguido por verificação read-only do resultado;
- permanece bloqueada se nenhum caminho assim puder ser comprovado; ausência
  de DML não dispensa a prova de saúde do consumidor.

Qualquer prova funcional que precise de DML em `trips` ocorre exclusivamente
em PostgreSQL efêmero/de teste. O caminho candidato por
`route_segment_prices` só pode ser usado em produção após o pré-check provar
que o fluxo implantado escreve apenas fora das tabelas-fonte de sheet sync;
essa descrição não autoriza o smoke. Rollback: restaurar os três grants
diretos.

### Lote 2C — trigger functions

Alvos: as seis functions `TRIGGER_INTERNAL`. Antes da mutação, obter as cinco
definições de sheet sync, confirmar triggers habilitados e provar em
PostgreSQL efêmero que triggers já criados continuam funcionando sem EXECUTE
de PUBLIC/anon/auth. Depois, revogar dessas três autoridades e preservar
postgres/service_role. Rollback: restaurar grants diretos nas seis signatures.

Status: **BLOCKED pela prova técnica e DDL ausente das cinco functions de
sheet sync**.

### Lote 2D — routines UNKNOWN

As oito routines `UNKNOWN_BLOCKED` não sofrem mutação. Para liberar, obter
definição implantada e consumidor/protocolo legítimo. Status: **BLOCKED**.

### Lote 3A — default privileges futuros de SEQUENCE

Revogar `SELECT/UPDATE/USAGE` do default ACL para anon/authenticated;
preservar postgres/service_role. Verificar em banco efêmero. Rollback:
restaurar os três privilégios.

### Lote 3B — sequence atual

Alvo: `sheet_sync_queue_id_seq`. Nenhuma mutação até esclarecer o DDL de
`sheet_sync_queue.id`, produtor, consumidor, role e protocolo. Status:
**BLOCKED**.

### Lote 4 — identidade interna

`user_profiles`, `user_roles`; depende de `roles` permanecer endurecida.
Smoke: Auth, `/users/me`, listagem e alteração controlada de acesso.

### Lote 5 — afiliados

`affiliate_recipients`, `affiliate_withdrawals`; depende do lote 4. Smoke de
balance/histórico; saque somente sem transferência real.

### Lote 6 — estado de chat

`chat_sessions`, `chat_messages`, `chat_tool_calls`, `chat_handoffs`. Smoke de
sessão, mensagens, handoff e ledger controlado.

### Lote 7 — delivery e jobs

`outbound_messages`, `automation_job_runs`. Smoke sem envio real.

### Lote 8 — referência de rotas

`routes`, `stops`. Smoke de routes/stops/availability.

### Lote 9 — viagens e paradas

`trips`, `trip_stops`; depende do lote 8 e do
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`. A prova de que o trigger de
`trips` continua enfileirando é obrigatória somente em PostgreSQL efêmero/de
teste. Enquanto o guard estiver ativo, nenhum smoke em produção provoca DML
nessas tabelas apenas para testar o enqueue; a parcela de verificação que
exigir essa escrita permanece bloqueada.

### Lote 10 — preço e projeção de disponibilidade

`route_segment_prices`, `available_segments`; depende de 2B, 8 e 9. Smoke de
preços, availability e refresh controlado. Por depender do lote 9, também
respeita o `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` e não pode usar DML
sintético em `trips` como atalho de verificação em produção.

### Lote 11 — reservas

`bookings`; depende de 8, 9 e do `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`.
A prova de enqueue do trigger ocorre somente em PostgreSQL efêmero/de teste.
Enquanto o guard estiver ativo, não há fixture nem DML deliberado em produção
apenas para provar o trigger; essa parcela do lote permanece bloqueada.

### Lote 12 — passageiros

`passengers`; depende de 11 e do `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`.
Smoke de consumidor sem escrita deliberada nessa tabela em produção enquanto
o guard estiver ativo; prova comportamental do trigger somente em PostgreSQL
efêmero/de teste.

### Lote 13 — detalhe financeiro da reserva

`booking_payment_details`; depende de 11 e do
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`. Smoke transacional que escreva
nessa tabela e prova do trigger ocorrem somente em PostgreSQL efêmero/de teste
enquanto o guard estiver ativo.

### Lote 14 — pagamentos e eventos

`payments`, `payment_events`; depende de 11 e 13. Nenhuma cobrança real ou
replay de webhook real. Também respeita o
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`: não pode provocar escrita em
`bookings` ou `booking_payment_details` em produção como efeito colateral de
seu smoke enquanto o subsistema permanecer UNKNOWN.

### Lote 15 — manifesto derivado

`manifest_data`; depende de 2D, 9, 11, 12 e 13. Status: **BLOCKED até
classificar as routines de refresh/enqueue de manifesto**. Mesmo após essa
classificação, não pode avançar sem satisfazer o
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`.

### Lote 16 — documento fiscal já sem grants

`fiscal_documents`. Confirmar owner; habilitar apenas RLS, sem mudar grants ou
criar policy. Rollback: desabilitar RLS.

### Lote 17 — UNKNOWN_BLOCKED

Sem mutação:

- `sheet_sync_queue` e sua sequence;
- seis routines RPC/callable de sheet sync sem consumidor;
- `travel_authorizations`;
- `travel_authorization_check_items`;
- `trip_contractors`.

Para liberar sheet sync, identificar worker/scheduler, local, protocolo,
role/credencial lógica, DDL implantado e atividade atual.

## 10. Invariantes

- BACKEND_ONLY não permanece acessível a anon/authenticated.
- UNKNOWN_BLOCKED não sofre mutação direta nem indireta deliberada.
- Nenhum smoke em produção provoca DML nas tabelas-fonte apenas para provar
  trigger/enqueue enquanto o `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`
  estiver ativo.
- O pós-check de produção do lote 2B nunca usa write pelo fluxo `trips`; uma
  prova positiva sem caminho backend seguro e comprovado permanece bloqueada.
- Revoke de anon/auth não basta enquanto PUBLIC mantiver EXECUTE.
- Nenhuma routine BACKEND_ONLY permanece executável por PUBLIC/anon/auth.
- service_role permanece inalterado; nenhum grant global pode ser criado sem
  snapshot e autorização compatíveis.
- trigger function só é endurecida após prova efêmera.
- default ACL futuro não recria autoridade para PUBLIC/anon/auth.
- hardening de RPC não substitui tabela e vice-versa.
- smoke API não prova segurança Data API e negação Data API não prova saúde da API.
- nenhum segredo aparece em output.

## 11. Gates canônicos

1. plano aprovado;
2. `/goal` de somente um lote;
3. pré-check SQL read-only;
4. `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` satisfeito no lote 2B, nos
   lotes 9, 11, 12, 13 e em todos os dependentes;
5. RED seguro quando necessário, exclusivamente efêmero se puder escrever em
   objeto UNKNOWN;
6. migration mínima;
7. pós-check SQL;
8. smoke Data API negativo anon;
9. smoke Data API negativo authenticated;
10. smoke positivo do consumidor dentro do guard;
11. smoke API Go quando aplicável e dentro do guard;
12. `/review` completo;
13. correção de P0/P1/P2;
14. novo `/review` limpo;
15. reconciliação documental;
16. autorização explícita do lote seguinte.

Nenhum lote seguinte é liberado automaticamente.

## 12. Definition of Done

- todas as 29 tabelas classificadas definitivamente;
- nenhum BACKEND_ONLY acessível a anon/auth sem justificativa;
- qualquer DATA_API_REQUIRED com grants mínimos e policies explícitas;
- UNKNOWN resolvido ou comprovadamente fora da superfície;
- default privileges de tables/functions/sequences endurecidos;
- functions/RPC e sequences atuais classificadas;
- smokes negativos e positivos verdes;
- API Go saudável pelo PostgreSQL direto;
- autoridade da Data API de anon/authenticated reduzida conforme as
  classificações e os grants/policies mínimos do plano;
- a chave antiga associada ao incidente permanece rotacionada/revogada
  conforme o checkpoint operacional; esse é um controle histórico separado,
  não uma garantia produzida por este hardening;
- risco residual aceito e registrado: `postgres` e `service_role` permanecem
  privilegiados e com `BYPASSRLS`; uma credencial válida comprometida de uma
  dessas roles conserva autoridade até o backlog específico ser executado;
- review final sem P0/P1/P2;
- tracker e handoff reconciliados.

## 13. Backlog fora do track

- migrar a API de `postgres` para role PostgreSQL dedicada;
- reavaliar grants de service_role somente com prova de não uso;
- tratar separadamente o risco de comprometimento de credenciais válidas de
  `postgres`/`service_role`, que não é encerrado por este track;
- avaliar redução futura dos schemas expostos pelo PostgREST.

## 14. Estado após materialização

**Status:** SEC-2026-08-18 — LOTE 2A COM REVIEW SEM P0/P1/P2 — 4 P1
anteriores fechados; `UNKNOWN_BLOCKED` no escopo public-only; nenhum lote SQL
autorizado.

Este arquivo documenta o checkpoint de review limpo e o bloqueio;
`UNKNOWN_BLOCKED` não equivale à conclusão operacional do Lote 2A. Não há
migration executável e não existe autorização para SQL, produção ou alcance
cross-schema. Qualquer retomada do Lote 2A exige nova decisão e autorização
explícita para o alcance cross-schema ou uma solução canônica distinta em
`/goal` separado. Nenhum sucessor é liberado automaticamente. A próxima ação
documental é o `/review` final desta reconciliação; o track funcional mantém
sua própria próxima ação sem alteração.
