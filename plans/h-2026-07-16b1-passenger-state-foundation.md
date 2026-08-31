# H-2026-07-16B1 — Fundação do estado de passageiros

## Status após o fechamento operacional de H-2026-07-27A

```text
CONCLUÍDA — GATE OPERACIONAL ENCERRADO
```

O review final histórico fechou B1 sem P1/P2. A evidência operacional de
2026-07-22 reabriu o gate por um bloqueio global em sessão nova; H-2026-07-22A
corrigiu e deployou o problema original, e seu smoke RED posterior originou o
hotfix H-2026-07-27A. H-A agora está `REVIEW_CLOSED`, `MERGED`, `DEPLOYED` e
`SMOKE_VERIFIED`: a seleção foi materializada e avançou até
`ASK_PASSENGER_COUNT`, sem `NONE`/`SAFE_PHASE_FALLBACK`. Não resta incidente
operacional aberto de B1. H-B2 passa apenas a próxima, não iniciada e dependente
de `/goal` e autorização próprios.

Plano corretivo: `plans/h-2026-07-22a-fresh-session-passenger-gate.md`.

### Review final — sem P1/P2

O review final confirmou que os casos legados `"1"` e `"essa msm"` falham
fechados, nenhuma claim V2 é tentada e os shadows V1/V2 permanecem zerados
depois da janela assíncrona. A correção final não alterou produção.

Evidência aceita para o fechamento:

- matrizes obrigatórias com `-count=20` verdes;
- race, regressões, `./internal/chat` e `./...` verdes;
- PostgreSQL real executado sem `SKIP`;
- inventário de produção em 54 `regexp.MustCompile`;
- `git diff --check` verde.

B1 está concluído. O desbloqueio alcança somente B2 como próximo slice; B2 não
é implementado neste fechamento documental.

Os blocos de reviews anteriores abaixo são registros históricos das respectivas
rodadas. Todos foram superseded pelo review final sem P1/P2 e não definem o
status vigente de B1.

### Review anterior — 1 P1 de prova

O review encontrou uma lacuna restante na observação do Travel V2 assíncrono:
`travel.calls == 0` era verificado antes da espera negativa em
`store.claimAttempts`, sem uma nova asserção depois dos 25 ms.

Naquela rodada, a correção permaneceu exclusivamente em teste. Os shadows V1/V2
e o store compatível continuaram habilitados, a espera negativa limitada foi
preservada e, imediatamente depois dela, o teste passou a verificar novamente
`travel.calls == 0` e `openAI.calls == 0`. As provas anteriores de zero runner,
JSON runner, availability, booking, payment, payment status e `ToolCalls`
permaneceram inalteradas. Naquele momento, B1 aguardava novo review e B2 ainda
não havia sido iniciado. Esse estado foi posteriormente superseded pelo review
final sem P1/P2.

### Review anterior — 2 P1 de prova

O review manteve o B1 aberto por duas provas específicas ainda ausentes:

1. o cenário legado sem boundary e sem outbound mascarador exercitava somente
   a seleção numérica `"1"`, não a referência contextual `"essa msm"`;
2. o draft era verificado apenas por índice e snapshot, sem procurar
   `trip_id`, `board_stop_id` e `alight_stop_id` em maps ou listas aninhados.

A correção permanece exclusivamente em teste:

- o marker legado sem boundary agora tem casos independentes para `"1"` e
  `"essa msm"`, ambos com availability antiga completa e nenhum outbound
  posterior confiável;
- history, canonical state, active prompt e decisão são capturados diretamente
  no deterministic router;
- um helper recursivo percorre maps e slices e falha se qualquer uma das três
  chaves de rota existir em qualquer profundidade do history ou dos dois
  payloads do draft;
- o controle positivo numérico foi preservado e um controle contextual usa uma
  nova availability pós-boundary com IDs distintos, exige a nova seleção e
  rejeita a reaparição dos IDs antigos.

O teste focado passou sem alteração de produção. Os 2 P1 eram lacunas de prova.
Naquele momento, B1 permanecia em correção local e aguardava novo review. Esse
estado foi posteriormente superseded pelo review final sem P1/P2.

### Review anterior — 1 P1 de prova

O review comprovou que o caso de marker legado sem boundary estava mascarado
por um segundo outbound `AUTOMATION_SENT` sem availability facts.
`latestReliableAssistantMessage` parava nessa mensagem posterior e já não
alcançava a availability antiga, portanto o teste passava mesmo sem depender da
lógica de boundary/overlay.

A correção ficou exclusivamente na fixture e nas asserções do teste:

- o caso principal agora contém somente uma availability antiga
  `AUTOMATION_SENT`, visível, completa e confiável antes do marker booleano;
- não existe outbound posterior confiável que masque essa availability;
- o deterministic router captura diretamente history e canonical state;
- a prova exige history sem prompt/facts/índice/snapshot de availability,
  canonical state sem trip/stops antigos e nenhum active prompt de escolha;
- intent, draft, marker, boundary conservadora e zero trabalho externo
  continuam verificados.

O teste focado passou sem alteração de produção. Assim, o P1 era uma lacuna de
prova; `service.go`, `availability_invalidation_history.go` e os demais arquivos
de produção não foram alterados nesta correção. Naquele momento, B1 permanecia
em correção local e aguardava novo review. Esse estado foi posteriormente
superseded pelo review final sem P1/P2.

### Review anterior — 1 P1 da fronteira causal

Mesmo com `canonical_availability_facts_invalidated=true` e o canonical state
sanitizado antes do router, `InferActivePromptContext`,
`routeDeterministicIntent` e os helpers de seleção ainda recebiam o histórico
completo. Se o outbound posterior fosse DRAFT/BLOCKED/invisível,
`latestReliableAssistantMessage` voltava até a availability antiga `SENT`,
produzia `SELECT_AVAILABILITY_OPTION`, persistia a viagem anterior e limpava o
marker.

O **P1 anterior permanece corrigido**: a limpeza estrutural continua
persistida, preserva fatos independentes e mantém a materialização atômica da
seleção atual completa. Esta rodada adiciona apenas causalidade durável e um
overlay read-only para availability; reducer, repository pós-booking, parser,
regex, Travel V2 e B2 permanecem fora de escopo.

RED real:
`TestPreInvalidationAvailabilityHistoryBoundaryDoesNotReachRouter` reproduziu
availability antiga `SENT` + marker/boundary posteriores + DRAFT posterior +
`essa msm` e capturou `SELECT_AVAILABILITY_OPTION` para a viagem antiga.

Correção local:

- marker persiste
  `canonical_availability_facts_invalidated_after_message_id` com o inbound
  causal e timestamp somente como fallback;
- overlay read-only remove estruturalmente availability, índice, snapshot,
  prompt de escolha e mirror pré-boundary sem mutar o transcript;
- estado, active prompt, router, booking draft, interpreters e helpers de
  seleção do `Reprocess` usam o mesmo overlay;
- DRAFT, BLOCKED, MANUAL_PENDING, SEND_FAILED, item incompleto e
  `BOT_AUTO_REPLY` sem fonte confiável não reabrem seleção;
- somente availability posterior, enviada/visível e completa materializa
  índice + snapshot e remove marker + boundary;
- sessão legada sem boundary estabelece fronteira conservadora no primeiro
  turno; timestamp impede reautorização quando o message ID sai do `LIMIT 50`;
- passageiro, documentos, booking, payment, handoff, cancelamento e endpoints
  independentes são preservados, sem regex ou vocabulário novo.

### Review anterior — 1 P1 de ordenação

Quando `canonical_availability_facts_invalidated=true` já está persistido, o
`Reprocess` ainda chamava `InferActivePromptContext` e
`routeDeterministicIntent` com o estado reconstruído do histórico antes de
aplicar a invalidação. O router podia receber índice, IDs de rota e
`LastToolFacts.availability_search` antigos mesmo que LLMs e persistência
posteriores já vissem o estado limpo.

RED real: `TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState`
capturou rota/facts stale no input do router para stale, blocker posterior e
item incompleto; o controle positivo também capturou o estado antigo antes de
o router avaliar a nova availability.

Correção local: o marcador passou a ser lido imediatamente depois do reload e
da derivação pós-`ApplyPassengerClarificationEventsV1`; a invalidação sincroniza
estado estruturado, canonical state, agent e memory antes de active prompt e
router.

### Review anterior — 1 P1 de persistência

Quando `passengerUnsafe=true` e a seleção atual é stale, invisível, bloqueada
ou incompleta, o fail-closed não materializa a seleção, mas o
`canonicalState` derivado anteriormente ainda pode persistir rota e
`LastToolFacts.availability_search` antigos em
`metadata.agent.canonical_state`.

Os **três P1 anteriores permanecem corrigidos**: a seleção materializável
continua atômica e completa, os quatro casos adversariais continuam sem
autoridade 1/0 pré-semeada e a autoridade `POST_BOOKING` continua refletindo
somente passageiros ativos. Esta correção não reabre reducer, parser, regex,
repository pós-booking, Travel V2 nem B2.

### Review anterior — 3 P1

1. **P1-A:** seleção incompleta ainda pode materializar índice e snapshot sem
   `trip_id`, `board_stop_id` ou `alight_stop_id`;
2. **P1-B:** os casos stale, invisível e bloqueado ainda não exercitam o ramo
   fail-closed sem snapshot/eventos ou autoridade 1/0;
3. **P1-C:** autoridade `POST_BOOKING` persistida não é atualizada quando
   passageiros são desativados depois do bootstrap inicial.

Os **7 P1 anteriores e o filtro inicial de `is_active=true` permaneciam
corrigidos naquela rodada**. A rodada centralizava a atomicidade da seleção,
cobria os quatro casos adversariais sem autoridade e atualizava/removia o
snapshot pós-booking a cada reload. B2 não seria iniciado.

RED real daquele review intermediário: o helper aceitava snapshot incompleto;
a regressão stale usava o ramo seguro com autoridade pré-semeada; e o reload
`POST_BOOKING` reutilizava contagens anteriores após soft-delete. As reproduções
locais confirmaram antes do patch o vazamento de índice/snapshot nos casos stale
e incompleto e, no PostgreSQL 16, o reload 4/2 permaneceu 4/2 depois da
desativação parcial.

Correção local dos três P1:

- `selectedAvailabilityItemForMaterialization` concentra a exigência dos três
  IDs e é usada pelos writers de índice, snapshot, booking draft e rota
  canônica;
- a matriz fail-closed usa `newFakeStore()` sem snapshot/eventos nem autoridade
  1/0 e cobre stale, invisível, blocker e item incompleto com zero trabalho
  externo; o caso positivo completo continua persistido além do `LIMIT 50`;
- o repository consulta a composição ativa em todo reload sob `FOR UPDATE`,
  persiste 4/2 -> 2/1, limpa 0 ativo para UNKNOWN e retorna erro quando a
  consulta não pode ser executada, sem fallback stale.

### Review anterior — 1 P1 + 1 P2

1. **P1:** o ramo `STRONG` do fail-closed pode gerar `ASK_PASSENGER_COUNT` sem
   anexar atomicamente a opção de availability selecionada e seus facts; quando
   o inbound original sai do `LIMIT 50`, a seleção deixa de ser recuperável;
2. **P2:** a autoridade pós-booking inclui passageiros inativos no total, em
   lap children e na projeção documental.

Os **7 P1 do sétimo review permaneciam corrigidos naquela rodada**. A correção
tratava somente esses dois achados, reutilizando os builders de availability
existentes e filtrando a autoridade pós-booking por `is_active=true`; B2 não
seria iniciado.

RED real do novo review: seleção fail-closed sem snapshot/eventos perde o
snapshot completo após truncamento da janela, e passageiros inativos alteram a
autoridade pós-booking. As regressões reproduziram `selected_option_index=0` e,
em PostgreSQL real, `PassengerCount=4`/`ChildUnder5Count=2` antes do patch.

### Correção local dos dois achados — 2026-07-21

- o fail-closed reutiliza `attachCurrentAvailabilitySelectionContext` e
  `attachSelectedAvailabilityResultToTemplateRun`; uma opção só é preservada
  quando deriva da lista atual, visível e completa, e o mesmo draft recebe
  índice, snapshot, `trip_id`, `board_stop_id`, `alight_stop_id` e facts;
- a regressão usa duas opções e seleciona a segunda sem pré-semear autoridade
  de passageiros; depois retira o inbound da janela de 50 e confirma a mesma
  seleção pelo draft, com zero LLM, JSON, shadows ou tools;
- a autoridade pós-booking agrega somente `booking_passengers.is_active=true`;
  total, lap children e projeção documental usam esse mesmo conjunto;
- a integração PostgreSQL cobre adulto/criança ativos e inativos e comprova que
  booking sem passageiro ativo não fabrica autoridade válida;
- `TestPassengerStateApplyEventsSerializesSessionPostgres` apenas alinha o nome
  da prova concorrente existente à regexp PostgreSQL obrigatória.

### Sétimo review histórico — 7 P1, superseded pelo review final

1. bootstrap sem snapshot dependia da janela limitada de 50 mensagens;
2. SEND_FAILED e MANUAL_PENDING podiam reexecutar prompt_event não entregue;
3. correção para proveniência não solo podia preservar adds_traveler stale;
4. escritores read-modify-write de chat_sessions.metadata podiam perder o
   snapshot concorrente;
5. a fixture concorrente clonava mapas fora do mutex e falhava no race gate;
6. o fake de Reprocess reparsava transcript ou fabricava autoridade 1/0;
7. o gate inseguro de passageiros podia suprimir guardrails STRONG e dúvidas
   paralelas locais seguras.

### Correção local dos sete P1 — 2026-07-20

- bootstrap sem snapshot consulta eventos estruturados de toda a sessão por uma
  query dedicada que não seleciona `body`; o `history` limitado do `Reprocess`
  deixou de ser entrada do bootstrap;
- replay de `passenger_prompt_event` exige status canônico de envio e
  `delivery_recorded_at`; draft, `MANUAL_PENDING`, `SEND_FAILED` e retry
  pendente não abrem slot;
- toda correção de passageiro com proveniência diferente de `SOLO_SPEAKER`
  limpa `ChildUnder5AddsTraveler` e sua origem antes da reconciliação;
- escritores que substituem metadata relêem a sessão sob `FOR UPDATE`; os
  demais atualizam caminhos JSONB sem substituir o snapshot concorrente;
- a fixture concorrente clona sessão, mensagens e mapas enquanto ainda detém o
  mutex; o race gate completo passou;
- o fake de `Reprocess` não reparsa bodies nem fabrica autoridade 1/0: sem
  snapshot/evento estruturado ele persiste `UNKNOWN`, e as regressões semeiam
  autoridade explicitamente;
- humano/cancelamento e decisões locais `STRONG` aplicáveis precedem o gate;
  perguntas paralelas informativas usam somente template local, preservam o
  prompt pendente e continuam sem LLM, shadow, document extraction ou tool.

## Objetivo

Estabelecer autoridade durável, eventos estruturais, serialização por sessão,
propagação do prompt efetivamente enviado, fail-closed e
`BookingDraftContext` como projeção. Este slice não interpreta linguagem.

## Dependência e sucessor

- predecessor: replanejamento documental do umbrella H-2026-07-16B;
- gate corretivo: H-2026-07-22A corrigido/revisado/deployado; o blocker do RED
  histórico foi fechado operacionalmente por H-2026-07-27A;
- sucessor: H-2026-07-16B2, **PRÓXIMA — NÃO INICIADA; AGUARDANDO AUTORIZAÇÃO
  PRÓPRIA**;
- 3.6F-D permanece bloqueada pelo umbrella H-B.

## Fonte arquitetural

`docs/adr/ADR-2026-07-passenger-authority-and-serialization.md`.

## Escopo autorizado

- `PassengerClarificationStateV1` versionado e seu validator de invariantes;
- eventos estruturais e reducer puro sem texto;
- bootstrap único a partir apenas de evidência estruturada;
- aplicação atômica/idempotente de eventos por sessão;
- row lock curto ou revision/CAS com retry bounded;
- cópia do `prompt_event` para o outbound efetivamente enviado;
- aplicação do prompt no registro de envio confiável;
- fail-closed antes de qualquer LLM, shadow ou tool;
- projeção do estado em `BookingDraftContext`;
- precedência da autoridade pós-booking;
- correção como substituição do agregado completo;
- testes unitários, integração concorrente e regressões do sexto review.

## Fora de escopo

- interpretar texto de passageiro/criança;
- adicionar regex, `containsAnyFolded`, sinônimos, tokens ou listas de frases;
- `PassengerClarificationMeaningV1`, schema strict, prompt ou provider;
- corpus, evaluator, shadow ou promoção runtime de meaning;
- Travel V2;
- mudança direta em booking/payment/preço;
- migration sem prova concreta de necessidade;
- deploy ou smoke sem autorização posterior.

## Implementação planejada

### 1. Remover interpretação da fundação

- retirar do diff H-B as novas regex e listas lexicais de família;
- manter o inventário do pacote sem crescimento em relação ao baseline do
  sexto review: 54 `regexp.MustCompile`;
- remover tipos intermediários que carreguem texto/`CorrectionCue` ao reducer;
- B1 recebe somente eventos estruturais já produzidos por fonte autorizada;
- linguagem sem evento permanece desconhecida e gera clarification segura.

### 2. Autoridade e bootstrap

- persistir `PassengerClarificationStateV1` em metadata versionada;
- bootstrap aceita somente eventos canônicos persistidos ou autoridade
  pós-booking;
- transcript, `tool_context` e bodies não participam;
- sem evidência suficiente: slots desconhecidos, bootstrap concluído e sem
  nova tentativa no replay.

### 3. Evento do outbound enviado

- draft guarda apenas `pending_prompt_event`;
- criação do outbound de auto-send ou review controlado copia o evento;
- registro de envio confiável aplica o evento idempotentemente na mesma seção
  serializada que registra sua entrega;
- retry/recovery usa o evento do outbound e não depende do draft estar no
  `LIMIT 50`;
- draft bloqueado/não enviado não abre época.

### 4. Contexto e correções

- pertinência infantil deriva do slot/época persistidos;
- `ChildUnder5AddsTraveler` deriva da proveniência persistida do total e do
  prompt infantil persistido, não de `ActivePrompt.Kind` textual;
- correção substitui o agregado e limpa dependências incompatíveis;
- correção de total sem composição completa invalida a relação infantil;
- correção completa para total 2 incluindo a criança produz dois documentos e
  `adds_traveler=false`.

### 5. Fail-closed e projeção

- validar snapshot imediatamente após aplicar eventos;
- antes de OpenAI shadow, Travel V2, document extraction ou qualquer tool,
  retornar clarification segura quando o estado estiver desconhecido,
  conflitante, corrompido ou inválido;
- `BookingDraftContext` copia a autoridade pré-booking e nunca recupera
  `passenger_count` de `booking_create` histórico;
- quando booking já existe, booking/passengers persistidos são autoridade;
- checks de slots obrigatórios precedem `BookingCreated` e payment.

### 6. Serialização

- escolher row lock curto ou revision/CAS;
- reler o snapshot mais recente dentro da seção serializada;
- reduzir, validar e persistir eventos por IDs estáveis;
- concorrência preserva eventos de ambos os turnos;
- retry é bounded e termina fail-closed;
- nenhuma chamada externa ocorre sob lock/transação.

## Matriz dos nove P1

| ID | Correção em B1 | Prova obrigatória |
|---|---|---|
| P1-01 | remover parser lexical novo; B1 não interpreta família | `TestPassengerStateFoundationDoesNotAddLexicalFamilyRules` + inventário `regexp.MustCompile == 54` |
| P1-02 | bootstrap somente de evento/estado estruturado | `TestPassengerStateBootstrapUsesStructuredEvidenceOnly` |
| P1-03 | copiar evento ao outbound enviado e aplicá-lo no delivery | `TestPassengerPromptEventFollowsReviewedAndAutoSentOutboundBeyondHistoryWindow` |
| P1-04 | derivar contexto infantil da época persistida | `TestPassengerChildAddsTravelerUsesPersistedPromptEpoch` |
| P1-05 | gate inseguro antes de todos os LLMs/tools | `TestPassengerGateAfterDeliveredPromptStopsExternalWorkBeforeDispatch` |
| P1-06 | projeção ignora `booking_create.passenger_count` histórico | `TestBookingDraftProjectionIgnoresBookingCreatePassengerCount` |
| P1-07 | validar slots antes de booking criado/payment | `TestBookingCreatedWithUnknownPassengerSlotsFailsClosed` |
| P1-08 | correção substitui agregado e dependências | `TestPassengerAggregateCorrectionClearsDependentAddsTraveler` |
| P1-09 | serialização evita lost update | `TestPassengerStateConcurrentReprocessPreservesBothEvents` e `TestPassengerStateApplyEventsSerializesSessionPostgres` |

## Critérios de aceite

- nenhuma interpretação de linguagem em B1;
- zero crescimento de regex/listas lexicais;
- reducer recebe somente estado + eventos;
- bootstrap independe de transcript, parser e janela;
- outbound enviado carrega o evento mesmo sem o draft na janela;
- época persistida sobrevive a mudança do corpo/template;
- estado inseguro produz zero chamadas a qualquer LLM, shadow ou tool;
- `BookingDraftContext` é projeção e não recuperação;
- booking criado não avança payment com slot obrigatório inválido;
- correção nunca deixa `adds_traveler` incompatível;
- duas instâncias concorrentes não perdem evento;
- nenhuma chamada externa sob lock;
- replay/retry/restart são idempotentes;
- H-012, documentos, lap child e payment permanecem verdes.

## Testes obrigatórios

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'TestPassengerStateFoundation|TestPassengerStateBootstrap|TestPassengerPromptEvent|TestPassengerChildAddsTraveler|TestPassengerUnsafeState|TestBookingDraftProjection|TestBookingCreatedWithUnknownPassengerSlots|TestPassengerAggregateCorrection|TestPassengerStateConcurrent'
go test -count=20 ./internal/chat -run 'Test.*Passenger.*State|Test.*Passenger.*Prompt|Test.*Passenger.*Correction|Test.*Booking.*Passenger'
go test -race -count=1 ./internal/chat -run 'Test.*Passenger.*State|Test.*Passenger.*Prompt|Test.*Passenger.*Correction|Test.*Booking.*Passenger'
go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Payment|Test.*LapChild|Test.*Passenger.*Document|Test.*Document.*Passenger|Test.*Booking.*Document'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

O teste de serialização real deve executar contra PostgreSQL controlado e não
pode ser substituído apenas por fake store ou `-race`. Se a URL de teste não
estiver disponível localmente, o slice permanece sem prova completa até a
execução autorizada em CI/ambiente de integração.

Auditoria lexical obrigatória:

```bash
rg -n 'regexp\.MustCompile' internal/chat
rg -n 'containsAnyFolded|explicitChildIdentity|childExplicitIdentity' internal/chat
```

## Gate de desbloqueio do B2 — cumprido

- todos os critérios e testes acima comprovados;
- teste PostgreSQL de concorrência executado, não apenas skipped;
- review sem P1/P2;
- tracker atualizado;
- smoke de runtime/repository executado somente se posteriormente autorizado e
  exigido pelo review/AGENTS;
- nenhum achado operacional aberto.

## Resultado da execução local — 2026-07-20

Implementado:

- `PassengerClarificationStateV1` versionado, validator e reducer puro;
- bootstrap único apenas de eventos normalizados estruturados ou de
  booking/passengers persistidos;
- `SELECT ... FOR UPDATE` curto por sessão para reler, reduzir, validar e
  persistir o estado, com preservação do snapshot mais recente no
  `SaveReprocessSnapshot` concorrente;
- `pending_prompt_event` no draft, cópia canônica para o outbound efetivo e
  aplicação idempotente no registro confiável de envio;
- fail-closed antes de shadow OpenAI, Travel V2, document extraction e tools;
- `BookingDraftContext` como projeção do snapshot, sem reconstrução de
  composição ou época por body, transcript ou `tool_context.booking_create`;
- precedência pós-booking a partir do booking e passageiros persistidos;
- invalidação fechada de artefato estrutural malformado;
- correção do agregado e proteção de booking/payment com slots obrigatórios.

Arquivos de produção e contrato alterados:

```text
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/availability_draft.go
apps/api/internal/chat/availability_invalidation_history.go
apps/api/internal/chat/booking_create_router.go
apps/api/internal/chat/booking_draft_context.go
apps/api/internal/chat/conversation_state_machine.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/model.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/service.go
apps/api/internal/chat/tool_router.go
apps/api/internal/chat/passenger_clarification_evidence.go
apps/api/internal/chat/passenger_clarification_reducer.go
```

Cobertura alterada ou adicionada:

```text
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/agent_rollout_test.go
apps/api/internal/chat/availability_draft_test.go
apps/api/internal/chat/booking_create_router_test.go
apps/api/internal/chat/booking_draft_context_test.go
apps/api/internal/chat/cargo_router_test.go
apps/api/internal/chat/chat_flow_guardrails_test.go
apps/api/internal/chat/handler_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/interpreter_shadow_report_endpoint_test.go
apps/api/internal/chat/openai_interpreter_assist_test.go
apps/api/internal/chat/openai_travel_query_v2_test.go
apps/api/internal/chat/tool_router_test.go
apps/api/internal/chat/passenger_clarification_evidence_test.go
apps/api/internal/chat/passenger_clarification_reducer_test.go
apps/api/internal/chat/passenger_clarification_repository_test.go
apps/api/internal/chat/passenger_clarification_state_v1_test.go
apps/api/internal/chat/passenger_clarification_test_helper_test.go
```

Validação da correção anterior — superseded pelo review de 3 P1:

```text
RED confirmado pelo novo review — seleção fail-closed perde snapshot após LIMIT 50; autoridade pós-booking conta passageiros inativos
PASS PRESERVADO — os 7 P1 do sétimo review permanecem corrigidos
RED — TestSelectedAvailabilitySelectionPassengerFailClosedPersistsBeyondHistoryWindow: selected_option_index=0 antes do patch
RED PostgreSQL — TestPassengerPostBookingAuthorityIgnoresInactivePassengersPostgres: PassengerCount=4 e ChildUnder5Count=2 antes do filtro
PASS — GOCACHE=/tmp/schumacher-b1-go-build CHAT_PASSENGER_STATE_POSTGRES_TEST_URL=<PostgreSQL 16 efêmero> go test -count=20 ./internal/chat -run 'Test.*Selection.*Passenger|Test.*Selected.*Availability|Test.*PostBooking.*Authority|Test.*Inactive.*Passenger'
PASS — GOCACHE=/tmp/schumacher-b1-go-build go test -race -count=1 ./internal/chat
PASS SEM SKIP — GOCACHE=/tmp/schumacher-b1-go-build CHAT_PASSENGER_STATE_POSTGRES_TEST_URL=<PostgreSQL 16 efêmero> go test -count=1 ./internal/chat -run 'Test.*Passenger.*Postgres|Test.*PostBooking.*Authority|Test.*Concurrent'
PASS PostgreSQL — TestPassengerStateApplyEventsSerializesSessionPostgres e TestPassengerPostBookingAuthorityIgnoresInactivePassengersPostgres executados em PostgreSQL 16 real
PASS — GOCACHE=/tmp/schumacher-b1-go-build go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Document|Test.*LapChild|Test.*Payment|Test.*HumanSupport|Test.*OutOfTurn'
PASS — GOCACHE=/tmp/schumacher-b1-go-build go test -count=1 ./internal/chat
PASS — GOCACHE=/tmp/schumacher-b1-go-build go test -count=1 ./...
PASS — inventário de produção confirmado em 54 regexp.MustCompile
PASS — git diff --check
```

Registro histórico daquela rodada. Este bloco foi superseded pelo review final
sem P1/P2 e não define o status vigente de B1. Resultado daquele review
intermediário: **1 P1 + 1 P2**. Os 7 P1 anteriores permaneciam corrigidos; os
dois achados daquela rodada e toda a matriz obrigatória passaram, inclusive
PostgreSQL real sem `SKIP`. **Naquele momento, nenhum novo review havia sido
executado depois do patch e a rodada ainda não declarava review limpo. Esse
estado foi posteriormente superseded pelo review final sem P1/P2**.

Matriz exigida naquela rodada para os três P1:

```text
RED reproduzido — stale e item incompleto vazavam índice/snapshot; PostgreSQL mantinha 4/2 após desativar dois passageiros
PASS PRESERVADO — os 7 P1 anteriores e o filtro SQL inicial is_active=true permanecem corrigidos
PASS — count=20 das reproduções de seleção, controle LIMIT 50, serialização PostgreSQL e refresh pós-booking
PASS — go test -race -count=1 ./internal/chat
PASS SEM SKIP — PostgreSQL 16 real: serialização e 4 ativos -> 2 ativos -> 0 ativo, incluindo erro de consulta sem fallback
PASS — regressões H-012/document/lap-child/payment/human/out-of-turn
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — inventário de produção em 54 regexp.MustCompile
PASS — git diff --check
```

Registro histórico daquela rodada. Este bloco foi superseded pelo review final
sem P1/P2 e não define o status vigente de B1. Resultado daquele review
intermediário: **3 P1**. Os três achados foram corrigidos localmente e a matriz
obrigatória passou. Naquele momento, nenhum novo review havia sido executado e
a rodada ainda não declarava review limpo. Esse estado foi posteriormente
superseded pelo review final sem P1/P2.

Matriz exigida naquela rodada para o P1:

A correção local centraliza a invalidação de seleção/availability não
materializável: limpa índice, IDs de rota, data, horário, preço, moeda, pacote e
`LastToolFacts.availability_search`, preservando origem/destino independentes e
outros fatos de tools. O estado sanitizado é sincronizado antes da persistência
em `structuredCanonicalState`, `structuredInput.State`, `agent`, `memory` e no
draft; um marcador estrutural impede que o histórico limitado reintroduza os
fatos antigos em turnos posteriores até existir nova availability atual,
visível e completa. A matriz A-D recarrega a sessão e a segunda passagem
confirma que os interpreters recebem o estado limpo; o controle positivo
continua persistindo seleção completa e sobrevivendo ao `LIMIT 50`.

```text
RED confirmado pelo review — canonical_state persistido conserva rota e availability facts stale no fail-closed
PASS PRESERVADO — os três P1 anteriores permanecem corrigidos
PASS — count=20 de fail-closed availability, controle positivo LIMIT 50 e canonical state stale
PASS — go test -race -count=1 ./internal/chat
PASS — regressões H-012/document/lap-child/payment/human/out-of-turn
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — inventário de produção em 54 regexp.MustCompile
PASS — git diff --check
```

Registro histórico daquela rodada. Este bloco foi superseded pelo review final
sem P1/P2 e não define o status vigente de B1. Resultado daquele review
intermediário: **1 P1**. O achado foi corrigido localmente e a matriz obrigatória
passou. Naquele momento, nenhum novo review havia sido executado e a rodada
ainda não declarava review limpo. Esse estado foi posteriormente superseded
pelo review final sem P1/P2.

Matriz exigida para o P1 de ordenação antes do router:

```text
RED reproduzido — router recebe rota/facts stale antes da invalidação marcada
PASS PRESERVADO — limpeza persistida, fatos independentes e seleção completa permanecem corrigidos
PASS — count=20 das regressões BeforeRouter/InvalidatedRouter/FreshAvailability
PASS — go test -race -count=1 ./internal/chat
PASS — regressões H-012/document/lap-child/payment/human/out-of-turn
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — inventário de produção em 54 regexp.MustCompile
PASS — git diff --check
```

Registro histórico daquela rodada. Este bloco foi superseded pelo review final
sem P1/P2 e não define o status vigente de B1. Resultado daquele review
intermediário: **1 P1**. O RED foi reproduzido, o achado foi corrigido localmente
e a matriz obrigatória passou. Naquele momento, nenhum novo review havia sido
executado e a rodada ainda não declarava review limpo. Esse estado foi
posteriormente superseded pelo review final sem P1/P2.

Matriz histórica do P1 da fronteira causal — superseded pelo review final:

```text
RED reproduzido — availability antiga SENT + marker/boundary + DRAFT posterior + "essa msm" selecionava a viagem pré-boundary
PASS — count=20 da matriz HistoryBoundary/PreInvalidationRouter/PostInvalidationFreshAvailability
PASS — DRAFT/BLOCKED/MANUAL_PENDING/SEND_FAILED/item incompleto/BOT_AUTO_REPLY sem fonte mantêm marker + boundary, sem índice/snapshot/trip/stops e com zero LLM/shadow/tools
PASS — controle positivo posterior, enviado e completo seleciona somente a nova viagem, persiste índice + snapshot atomicamente e limpa marker + boundary
PASS — boundary fora do LIMIT 50 usa timestamp conservador; marker legado sem boundary falha fechado e adquire fronteira atual
PASS — overlay read-only não muta histórico e preserva passageiro, booking, payment, handoff, humano/cancelamento STRONG e endpoints independentes
PASS — go test -race -count=1 ./internal/chat
PASS — regressões H-012/document/lap-child/payment/human/out-of-turn
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — inventário de produção em 54 regexp.MustCompile
PASS — git diff --check
```

Correção local da lacuna de prova apontada no review anterior:

```text
PASS — fixture principal sem outbound posterior: availability antiga SENT, completa e confiável + marker legado sem boundary + estado de passageiros seguro + turno atual "1"
PASS — input capturado do router sem availability_search, selected_option_index, selected_availability_result, trip/stops ou active prompt de escolha antigos
PASS — saída sem SELECT_AVAILABILITY_OPTION, índice ou snapshot; marker ativo e boundary conservadora persistida pelo inbound atual; zero LLM/JSON/shadows/tools
PASS — go test -count=1 ./internal/chat -run 'Test.*Legacy.*Availability.*Invalidation|TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState'
PASS — go test -count=20 ./internal/chat -run 'Test.*AvailabilityInvalidation.*HistoryBoundary|Test.*PreInvalidation.*Router|Test.*FreshAvailability'
PASS — go test -race -count=1 ./internal/chat
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Document|Test.*LapChild|Test.*Payment|Test.*HumanSupport|Test.*OutOfTurn'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — inventário de produção confirmado em 54 regexp.MustCompile
PASS — git diff --check
PASS — nenhuma alteração em arquivo de produção nesta correção
```

Correção local dos 2 P1 de prova do review anterior:

```text
PASS — marker legado sem boundary falha fechado com "1" e "essa msm", sem outbound posterior confiável
PASS — history, canonical state, active prompt e IntentDecision capturados diretamente no router nos dois caminhos
PASS — helper recursivo rejeita trip_id, board_stop_id e alight_stop_id em qualquer profundidade dos maps/slices do history e do draft
PASS — controle positivo numérico preservado; controle contextual seleciona somente a availability nova pós-boundary, com IDs distintos dos antigos
PASS — fixture executável usa base fakeStore para semeadura/consulta e fakeTravelQueryV2ShadowClaimStore no Service, com shadows V1/V2 habilitados
PASS — nos casos legados "1" e "essa msm": openAI.calls=0 e travel.calls=0 antes da janela, e nenhum claimAttempts durante a espera negativa limitada; runner, JSON runner, availability, booking, payment, payment_status e tools também permanecem em zero
PASS — go test -count=20 ./internal/chat -run '^TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState$'
PASS — go test -count=20 ./internal/chat -run 'TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState|Test.*AvailabilityInvalidation.*HistoryBoundary|Test.*PreInvalidation.*Router|Test.*FreshAvailability'
PASS — go test -race -count=1 ./internal/chat
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Document|Test.*LapChild|Test.*Payment|Test.*HumanSupport|Test.*OutOfTurn'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l => 54
PASS — git diff --check
PASS — nenhuma alteração em arquivo de produção nesta correção
```

Correção local do P1 de prova restante naquele review intermediário:

```text
PASS — shadows V1/V2 e fakeTravelQueryV2ShadowClaimStore permanecem habilitados nos casos legados "1" e "essa msm"
PASS — espera negativa limitada em store.claimAttempts preservada; imediatamente após a janela, travel.calls=0 e openAI.calls=0 são verificados novamente
PASS — zero runner, JSON runner, availability, booking, payment, payment_status e ToolCalls preservado
PASS — go test -count=20 ./internal/chat -run '^TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState$'
PASS — go test -count=20 ./internal/chat -run 'TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState|Test.*AvailabilityInvalidation.*HistoryBoundary|Test.*PreInvalidation.*Router|Test.*FreshAvailability'
PASS — go test -race -count=1 ./internal/chat
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Document|Test.*LapChild|Test.*Payment|Test.*HumanSupport|Test.*OutOfTurn'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l => 54
PASS — git diff --check
PASS — nenhuma alteração em arquivo de produção nesta correção
```

Resultado histórico do review final: **sem P1/P2**. Esse resultado teve o gate
operacional reaberto pela evidência de 2026-07-22; H-2026-07-22A corrigiu e
deployou o problema original, e H-2026-07-27A fechou o blocker posterior com
review, merge, deploy e smoke verificados. Assim, H-B1 está concluída e B2 é a
próxima, ainda não iniciada. H-B permanece em andamento, B3 permanece
bloqueada por B2 e 3.6F-D continua bloqueada pelo fechamento integral de H-B.

O teste em produção/smoke não foi executado na rodada histórica de B1; a
evidência operacional posterior está reconciliada no tracker. B1 continua sem
interpretar linguagem; esse contrato pertence exclusivamente a B2.

## `/goal`

```text
/goal
Execute somente H-2026-07-16B1.

Implemente a fundação de autoridade e serialização definida na ADR de
passenger authority. B1 não interpreta linguagem: remova o parser lexical novo
do H-B e não adicione regex, listas ou sinônimos.

Use somente estado e eventos estruturais, bootstrap estruturado, prompt_event
no outbound efetivamente enviado, fail-closed antes de LLMs/tools,
BookingDraftContext como projeção, correção de agregado completo e atualização
serializada por sessão com lock curto ou CAS/retry bounded. Não mantenha lock
durante chamadas externas.

Não implemente PassengerClarificationMeaningV1, provider, schema, corpus,
shadow ou runtime assist. Não avance B2. Atualize o tracker e não faça commit,
push, deploy ou smoke sem pedido explícito.
```

## `/review`

```text
/review
Revise somente H-2026-07-16B1.

Procure os nove P1 do sexto review, crescimento lexical, bootstrap por texto,
evento preso ao draft, contexto derivado do body, chamada externa antes do
fail-closed, recuperação via tool_context, payment antes de slots válidos,
correção parcial e lost update concorrente. Confirme que nenhuma chamada
externa mantém lock de sessão.

Exija teste PostgreSQL de concorrência, count=20, race, H-012/document/lap-child/payment,
./internal/chat, ./... e git diff --check. Não altere arquivos e não libere B2
com P1/P2 ou prova concorrente ausente.
```
