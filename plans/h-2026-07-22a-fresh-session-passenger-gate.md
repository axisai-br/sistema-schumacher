# H-2026-07-22A — Gate de passageiros em sessão nova

## Status no tracker

```text
H-2026-07-22A — EM CORREÇÃO APÓS REVIEW — 4 P1 DE REPLAY CANÔNICO.
```

Este hotfix reabre o gate operacional de H-2026-07-16B1. Enquanto review,
deploy e smoke não estiverem verdes, H-2026-07-16B2 permanece bloqueada.

## Evidência operacional

Em sessão nova após limpeza:

```text
"oi" -> ASK_PASSENGER_COUNT
"monção para videira na data mais próxima" -> ASK_PASSENGER_COUNT
tool_call_count=0
availability_search não executada
history_count=1
```

O incidente é user-visible e impede globalmente o início normal do atendimento,
inclusive uma consulta que já contém rota e `EARLIEST_AVAILABLE`.

## Causa comprovada

1. `newPassengerClarificationStateV1` inicializa os dois slots como
   desconhecidos e sem evidência estruturada;
2. `passengerClarificationStateUnsafeV1` tratava qualquer slot desconhecido
   como estado inseguro;
3. `Service.Reprocess` incorporava esse resultado globalmente em
   `externalWorkUnsafe`;
4. o fail-closed escolhia `BookingNextAskPassengerClarification` antes da
   seleção de viagem;
5. testes abrangentes ocultavam o defeito ao usar
   `newFakeStoreWithPassengerAuthority` em vez do `newFakeStore()` literal de
   uma sessão nova.

## Objetivo

Separar explicitamente:

- estado inválido ou conflitante, sempre fail-closed;
- contexto de passageiros ativo, estabelecido por evidência estruturada;
- slots desconhecidos;
- bootstrap durável `UNKNOWN`, ainda sem contexto de passageiros, que não é um
  bloqueio global.

Uma sessão nova deve seguir greeting, coleta de rota ou disponibilidade. Depois
que o prompt de passageiros for efetivamente entregue, slots desconhecidos ou
conflitantes continuam bloqueando LLMs, shadows e tools.

## Escopo autorizado

- ajustar somente a classificação do gate de
  `PassengerClarificationStateV1`;
- reutilizar a mesma noção de contexto ativo na projeção de
  `BookingDraftContext`;
- adicionar regressões com `newFakeStore()` para o bootstrap real;
- fortalecer fixtures antigas de fail-closed para que possuam prompt de
  passageiros realmente entregue;
- centralizar a classificação estrutural da seleção de availability em quatro
  estados e preservar listas completas durante clarificação ambígua;
- restringir booking draft e `booking_create` a índice explícito atual ou
  seleção bookable persistida com snapshot completo;
- atualizar tracker, handoff e planos afetados pela reabertura do gate.

## Fora de escopo

- implementar H-2026-07-16B2 ou `PassengerClarificationMeaningV1`;
- adicionar parser lexical, regex ou regras abertas de família;
- alterar provider, contrato OpenAI, execução/contrato de booking, payment,
  banco ou migrations além da autoridade de seleção exigida pelos P1 atuais;
- executar commit, push, deploy ou smoke;
- liberar B2 somente por evidência local.

## Semântica do gate

```text
invalid_or_conflicting = validate(state) falha OR conflito estrutural
active_passenger_context = state.HasEvidence
unknown_slots = !PassengerCountKnown OR !ChildUnder5CountKnown

unsafe = invalid_or_conflicting OR
         (active_passenger_context AND unknown_slots)
```

`HasEvidence=false` só é válido no `UNKNOWN` literal, sem evidência estrutural.
Se o snapshot contiver IDs de prompt/última mensagem, eventos/mensagens
aplicados, slot `OPEN`/`ANSWERED`/`CONFLICTING` ou contagem conhecida, o
validator rejeita a contradição e o repository/fake store a converte em
`invalidPassengerClarificationStateV1`. Não há reparo silencioso do booleano.
`STRONG` humano/cancelamento continua precedendo o gate.

O gate de seleção de availability usa classificação estrutural centralizada:

```text
NONE
MATERIALIZE
CLARIFY_PRESERVE
FAIL_CLOSED_INVALIDATE
```

`MATERIALIZE` exige opção atual completa. `CLARIFY_PRESERVE` bloqueia trabalho
externo somente no turno e mantém a lista válida disponível. Somente
`FAIL_CLOSED_INVALIDATE` cria/preserva marker + boundary. Confirmações genéricas
não são seleção fora de prompt de availability.

Antes de classificar o turno, o serviço usa um baseline explicitamente nomeado:

```text
bookingDraftRoutingBaselineProjection
collectBookingDraftContextForRoutingBaseline
deriveCanonicalConversationStateForRoutingBaseline
```

Esse baseline usa sessão e histórico persistido, preserva seleção bookable
anterior e não materializa índice/deíxis/data do `currentTurn`. O turno é roteado
contra o baseline. Humano/cancelamento `STRONG` mantém e persiste o baseline;
somente `MATERIALIZE` aplica uma seleção atual completa ao estado canônico.

A projeção de disponibilidade do booking draft é explícita:

```text
ENVELOPE_ONLY
BOOKABLE_SELECTION
```

`bookingDraftRoutingBaselineProjection` usa `ENVELOPE_ONLY`. Essa política
separa o merge de envelope/filtro do merge de item selecionado e nunca promove
o fallback de resultado único. Seleção bookable persistida continua sendo
projetada exatamente; item atual só entra depois de `MATERIALIZE`.

A seleção bookable é um agregado atômico e durável. A autoridade normal não é
mais resolvida de janela de histórico: `AvailabilitySelectionStateV1`,
versionado em `metadata.memory`, registra status `NONE | BOOKABLE | REJECTED |
INVALIDATED`, evento e projeção da seleção, prompt source, índice, snapshot
completo, tombstone/rejeições e `applied_event_ids`.

Os eventos `SELECTION_MATERIALIZED`, `SELECTION_REJECTED` e
`SELECTION_INVALIDATED` são reduzidos sem texto sob o mesmo lock de sessão já
usado por `PassengerClarificationStateV1`. `MATERIALIZED` substitui o agregado
inteiro; rejeitar a autoridade corrente limpa o agregado e mantém tombstone;
rejeição de outro prompt não altera a seleção atual; projeções com
`materializes_authority=false` nunca concedem autoridade.

`BookingDraftContext` e `booking_create` consultam primeiro esse estado.
Transcript só participa do bootstrap legado, executado e persistido uma única
vez sobre a sessão estruturada completa, sem `body` e sem `LIMIT 50`. Recuperação
legada continua restrita ao próprio evento/projeção selecionada ou ao prompt
source exato; sem prova exata, o estado permanece `NONE`/fail-closed.

## Regressões obrigatórias

1. sessão nova + `"oi"`: sem pergunta/evento de passageiros; greeting/fallback
   normal;
2. sessão nova + `"quero uma passagem"`: coleta origem/destino/data antes de
   passageiros;
3. rota/data suficiente ou `EARLIEST_AVAILABLE`: `availability_search` uma vez,
   `tool_call_count=1`, sem perguntar passageiros antes do resultado;
4. seleção válida: persistir opção completa e só então perguntar quantidade de
   passageiros;
5. prompt de passageiros efetivamente entregue: desconhecido/conflitante
   continua fail-closed, com zero LLM/shadow/tool;
6. estado inválido ou conflitante: sempre fail-closed;
7. decisões locais `STRONG` de humano/cancelamento continuam vencedoras.
8. `HasEvidence=false` com qualquer evidência estrutural mínima é inválido,
   vira o sentinel fail-closed e não libera runner, JSON runner, shadows V1/V2,
   claim, availability, documento, booking, payment ou tool call;
9. em sessão nova, opção atual sem `trip_id`, `board_stop_id` ou
   `alight_stop_id`, escolhida por `"1"` ou `"essa msm"`, não abre passageiros,
   não persiste índice/snapshot/IDs e retorna fallback determinístico de
   seleção; a opção completa permanece o controle positivo.

### Regressões dos 3 P1 do review anterior

1. prompt renderizado sem tool facts + `"1"` e `"essa msm"` exige
   materialização e falha fechado;
2. índice fora do range não persiste seleção nem executa trabalho externo;
3. cada ID obrigatório ausente mantém trip selection;
4. repetição em turno posterior não reutiliza o número bruto;
5. `passengerUnsafe` + opção incompleta retorna fallback de availability;
6. opção completa materializa e só então pergunta passageiros;
7. cancelamento/humano em estado inseguro não cria novo prompt event;
8. prompt real de passageiros continua criando evento após delivery.

### Regressões dos 3 P1 do review anterior mais recente

1. opção única completa + `"ok"` materializa índice `1`, snapshot completo e só
   então pergunta passageiros;
2. opção única sem facts ou com `trip_id`, `board_stop_id` ou `alight_stop_id`
   ausente + `"ok"` usa `FAIL_CLOSED_INVALIDATE`;
3. várias opções completas + `"essa msm"` usa `CLARIFY_PRESERVE`, pede índice,
   não cria marker e não abre prompt de passageiros;
4. resposta posterior `"1"` seleciona a opção correta;
5. índice fora do range preserva a lista e não contamina o turno posterior;
6. `"1"` falho + availability nova + `"quero reservar"` não autoriza
   `booking_create`;
7. seleção bookable persistida continua permitindo o fluxo normal;
8. caminhos de clarificação/invalidação mantêm zero
   runner/JSON/shadows/claims/tools;
9. `"ok"` fora de prompt de availability não abre o gate de seleção.

### Regressões dos 3 P1 do novo review

1. `ASK_PASSENGER_COUNT` pode carregar facts de availability por continuidade,
   mas não reabre a identidade do prompt de seleção;
2. lista com várias opções -> seleção da opção `2` -> prompt de passageiros
   entregue com os facts anteriores -> resposta `"1"` mantém a opção `2` e
   grava `PassengerCount=1` pela evidência estrutural do turno;
3. a mesma sequência com `"sim"` continua como resposta ao prompt de
   passageiros, sem fallback de availability;
4. os quatro turnos mistos `"essa msm, quero falar com um atendente"`,
   `"1, quero falar com atendente"`, `"essa msm, quero cancelar"` e
   `"1, quero cancelar"` preservam a decisão `STRONG`, sem classificação,
   seleção, marker, snapshot ou trabalho externo;
5. um único resultado completo exibido, índice bruto histórico `"1"`,
   passageiros/documentos completos e confirmação `"sim"`, sem snapshot
   bookable persistido, produz zero `BookingCreateInput` e zero
   `booking_create`;
6. a mesma confirmação com seleção bookable persistida continua funcionando.

### Regressões do P1 do review anterior

1. sem seleção anterior, os turnos `"opção 1, quero cancelar"`,
   `"primeira opção, quero falar com atendente"`, `"essa msm, quero cancelar"`
   e `"1, quero falar com uma pessoa"` preservam intent/template `STRONG` e
   persistem baseline sem índice/snapshot/trip/stops novos;
2. nesses casos, `BookingDraftContext.HasBookableSelection=false`, draft e
   memory não carregam seleção e não há passenger prompt event;
3. com opção `2` bookable anterior, `"opção 1, quero cancelar"` mantém a opção
   `2` exatamente igual em router state, memory e metadata; a opção `1` não vira
   seleção;
4. o controle puro `"opção 1"` usa `MATERIALIZE`, persiste índice, snapshot e
   IDs completos e só então pergunta passageiros;
5. marker + boundary preexistentes permanecem idênticos e não impedem o
   cancelamento `STRONG` de vencer o gate;
6. no turno seguinte ao misto `STRONG`, router e shadow V1 recebem estado sem a
   opção rejeitada como seleção atual;
7. cancelamento real, handoff, pending question, shadows e tools fora de
   selection context preservam o comportamento anterior.

### Regressões do P1 do review anterior

1. uma opção completa, sem seleção anterior, mais
   `"opção 1, quero cancelar"`, `"essa msm, quero falar com atendente"`,
   `"ok, quero cancelar"` ou `"1, quero falar com uma pessoa"` preserva o
   intent/template `STRONG`;
2. o baseline desses casos mantém `HasAvailabilityShown` e endpoints do filtro,
   mas tem índice `0`, `HasBookableSelection=false` e nenhum trip/stops, data,
   horário, preço, moeda ou pacote derivado do item;
3. canonical, memory, metadata e draft não promovem item/snapshot, não criam
   evento de passageiros e não alteram marker/boundary;
4. opção `2` bookable persistida permanece exatamente igual quando uma nova
   lista unitária não selecionada é seguida de turno misto que menciona a opção
   `1`;
5. os controles puros `"ok"` e `"1"` usam `MATERIALIZE`, persistem
   índice/snapshot/trip/stops e então produzem `ASK_PASSENGER_COUNT`;
6. no turno seguinte ao misto `STRONG`, router e interpreter não recebem o item
   único como seleção.

### Regressões dos 3 P1 do review anterior

1. `S1/A → S2/B → rejeição de S2`: nenhuma autoridade sobrevive, `S1` não
   ressuscita nem por snapshot propagado posterior e `booking_create` permanece
   fechado;
2. `S1/A → S2/B → rejeição de S1`: `S2/B` continua autoritativa;
3. seleção da opção `1` na lista `A` + rejeição da opção `1` na lista `B`:
   `A` permanece, inclusive na colisão de índice e data;
4. a mesma rejeição originada na lista `A` remove a autoridade;
5. snapshot legado sem pacote + envelope posterior com os mesmos
   `trip/board/alight` e pacote `B`: `B` não é usado;
6. pacote `A` só é recuperado do próprio evento de seleção ou da mensagem exata
   indicada por `AvailabilityPromptSourceMessageID`;
7. sem fonte exata, o pacote permanece vazio;
8. `MATERIALIZE` posterior substitui todo o agregado e o reload preserva
   autoridade, `SelectionMessageID`, `AvailabilityPromptSourceMessageID`, IDs
   da viagem e pacote.

### Regressões dos 3 P1 de autoridade durável

1. lista unitária materializada e depois rejeitada mantém `booking_create`
   fechado, mesmo quando a projeção de passageiros permanece na janela;
2. `S1 → S2 → rejeição de S2` termina sem autoridade e nunca recupera `S1`;
3. rejeição da opção `1` na lista `B` não remove a seleção da opção `1` na lista
   `A`;
4. rejeição simples persiste no inbound o prompt source real e mantém esse
   source quando o prompt original sai da janela;
5. janela contendo somente projeção copiada/superseded não cria autoridade;
6. reload/restart preserva o tombstone;
7. estado `BOOKABLE` continua alimentando booking draft e o fluxo positivo de
   booking;
8. estados `NONE`, `REJECTED` e `INVALIDATED` produzem zero
   `BookingCreateInput`;
9. materialização do turno só progride quando o evento correspondente consta no
   estado devolvido pela mesma seção serializada;
10. bootstrap usa a sessão estruturada completa uma única vez, e as aplicações
    concorrentes fake/PostgreSQL não perdem eventos.

### Regressões dos 4 P1 atuais de replay canônico

1. seleção legada sem `selection_message_id` explícito permanece `NONE`; o ID
   do outbound/projeção nunca é usado como fallback;
2. apenas uma lista anterior estrutural, confiável, completa e compatível pode
   ser prompt source; seleção, passageiros, documentos, pagamento e continuação
   não são candidatos, e zero ou múltiplos candidatos deixam `NONE`;
3. todas as seis permutações de materialização antiga `A`, rejeição nova `B` de
   outra fonte e projeção não autoritativa produzem o mesmo estado, inclusive
   com duplicação e restart após cada permutação;
4. quando `B` adquire o lock antes e `A` chega depois, o replay causal aplica
   `A` antes de `B`, preserva `A` como `BOOKABLE` e mantém a rejeição de `B`;
5. marker/boundary bloqueia autoridade pré-boundary, materialização explícita
   pós-boundary substitui a invalidação e marker sem boundary permanece
   `INVALIDATED` conservador;
6. `BookingDraftContext` e `booking_create` consomem somente a projeção
   reconstruída/validada; história limitada e envelope atual não concedem
   autoridade;
7. com `CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1`, URL ausente falha em vez
   de executar `SKIP`;
8. PostgreSQL 16 real, duas pools e lock invertido reproduzem exatamente o
   estado live após reload/restart.

As regressões de sessão nova devem usar `newFakeStore()` sem semear autoridade
de passageiros. Os casos pós-prompt devem registrar entrega e
`passenger_prompt_event`, não simular contexto ativo apenas por fixture.

## Validação obrigatória

Executar em `apps/api`:

```bash
go test -count=20 ./internal/chat -run \
'Test.*AvailabilitySelection.*(Replay|Order|Legacy|Projection|Invalidation)'
go test -race -count=1 ./internal/chat
CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1 \
CHAT_PASSENGER_STATE_POSTGRES_TEST_URL='<POSTGRES_16_URL>' \
go test -count=20 ./internal/chat -run \
'Test.*AvailabilitySelection.*Postgres'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

Também confirmar que o inventário de `regexp.MustCompile` em produção não
cresceu.

## Correção local após reviews de 2026-07-22 e 2026-07-23

```text
REVIEW ANTERIOR 1 — 2 P1 preservados como corrigidos
REVIEW ANTERIOR 2 — 3 P1 preservados como corrigidos
REVIEW ANTERIOR 3 — 3 P1 preservados como corrigidos
REVIEW ANTERIOR 4 — 3 P1 preservados como corrigidos
REVIEW ANTERIOR 5 — 1 P1 preservado como corrigido
REVIEW ANTERIOR 6 — 1 P1 preservado como corrigido
REVIEW ANTERIOR 7 — 2 P1 preservados como corrigidos
REVIEW ANTERIOR 8 — 3 P1 preservados como corrigidos
REVIEW ANTERIOR 9 — 3 P1 DE AUTORIDADE DURÁVEL preservados como corrigidos
REVIEW ANTERIOR 10 — 4 P1 DE AUTORIDADE DURÁVEL preservados como corrigidos
REVIEW ATUAL — 4 P1 DE REPLAY CANÔNICO
RED NOVO — ASK_PASSENGER_COUNT com facts de continuidade reativou seleção: "1" trocou opção 2 pela 1 e "sim" virou fallback de availability
RED NOVO — os quatro turnos mistos deíxis/índice + humano/cancelamento foram classificados antes de STRONG; houve fallback e até runner legado
RED NOVO — resultado único exibido + índice bruto + documentos completos + "sim" produziu BookingCreateInput com índice 0 e chamou booking_create uma vez
PASS PRESERVADO — os três P1 do review anterior permanecem corrigidos
RED P1 DO REVIEW ANTERIOR — "opção 1, quero cancelar" e "primeira opção, quero falar com atendente" chegaram ao router com opção 1 materializada
RED P1 DO REVIEW ANTERIOR — marker/boundary existentes fizeram fallback de availability vencer cancelamento STRONG
RED DE ESCOPO INTERMEDIÁRIO — precedência STRONG global desviou cancelamentos reais, pending_question e shadows; política foi restringida ao prompt de opções ou ao turno realmente misto
PASS — go test -count=20 ./internal/chat -run 'Test.*Strong.*Selection.*State|Test.*Mixed.*Strong.*Availability|Test.*Strong.*Preserves.*Bookable|Test.*Selection.*After.*Strong'
RED P1 RESTANTE ATUAL — baseline sem seleção e com resultado único promoveu trip/stops, data, horário, preço e moeda apesar de index=0/bookable=false
RED P1 RESTANTE ATUAL — item único vazou para os quatro turnos STRONG e reapareceu no router/interpreter do turno seguinte
PASS CONTROLE — opção 2 bookable anterior permaneceu exata; "ok"/"1" puros materializaram a opção única antes de ASK_PASSENGER_COUNT
PASS — go test -count=20 ./internal/chat -run 'Test.*Single.*Option.*Strong.*Baseline|Test.*Routing.*Baseline.*Unselected|Test.*Strong.*Preserves.*Bookable|Test.*Pure.*Single.*Option.*Materialize'
RED P1 ATUAL 1 — rejeição posterior da opção 1 ocultou a opção 2 bookable não rejeitada e devolveu authority=false
RED P1 ATUAL 2 — baseline combinou IDs/data/preço da opção 2 com package-unselected-b de envelope posterior
PASS — go test -count=20 ./internal/chat -run 'Test.*Single.*Option.*Strong.*Baseline|Test.*Routing.*Baseline.*Unselected|Test.*Strong.*Preserves.*Bookable|Test.*Pure.*Single.*Option.*Materialize|TestBookingDraftResolvesBookableAuthorityAcrossLaterAvailabilityRejections|TestRoutingBaselineKeepsSelectedPackageAtomicAgainstLaterUnselectedEnvelope|TestMaterializeReplacesPriorBookableSelectionAggregateAndSurvivesReload'
RED P1 ATUAL 1 — S1/A ressuscitou depois de S2/B materializada e rejeitada; booking_create permaneceu aberto com S1
RED P1 ATUAL 2 — rejeição da opção 1/lista B removeu a seleção opção 1/lista A na colisão de índice e data
RED P1 ATUAL 3 — envelope posterior compatível injetou package-b no snapshot legado antes do próprio evento/fonte A
PASS — go test -count=20 ./internal/chat -run 'TestBookingDraft(ReducesBookableAuthorityWithoutResurrectingSupersededSelection|ScopesAvailabilityRejectionToPromptSource|LegacyRecoveryUsesOnlySelectionEventOrExactPromptSource|ResolvesBookableAuthorityAcrossLaterAvailabilityRejections)$|TestRoutingBaselineKeepsSelectedPackageAtomicAgainstLaterUnselectedEnvelope$|TestMaterializeReplacesPriorBookableSelectionAggregateAndSurvivesReload$|TestAvailabilitySelectionAfterSpecificRejected(Option|Date)OutOfTurnPayment$|TestSelectedAvailabilitySelectionPassengerFailClosedPersistsBeyondHistoryWindow$'
RED P1 DURÁVEL 1 — booking_create comparava rejeição com outbound de passageiros, não com o prompt source persistido da lista
RED P1 DURÁVEL 2 — projeção propagada podia virar autoridade quando o MATERIALIZE e o tombstone saíam da janela
RED P1 DURÁVEL 3 — rejeição simples não persistia prompt source e deixava de atingir a seleção após truncamento
RED P1 ATUAL 1 — bootstrap restaurou seleção pré-boundary apesar do marker canônico de invalidação
RED P1 ATUAL 2 — projeção PASSENGER_COUNT_REPLY sem materializes_authority virou autoridade
RED P1 ATUAL 3 — seleção legada fabricou o outbound de seleção como prompt source e escapou da rejeição original
RED P1 ATUAL 4 — aquisição do lock fora da ordem das mensagens permitiu seleção/reabertura stale
PASS — go test -count=20 ./internal/chat -run 'TestAvailabilitySelectionStateV1|TestBookingDraft(ReducesBookableAuthorityWithoutResurrectingSupersededSelection|ScopesAvailabilityRejectionToPromptSource|LegacyRecoveryUsesOnlySelectionEventOrExactPromptSource|ResolvesBookableAuthorityAcrossLaterAvailabilityRejections)$|TestRoutingBaselineKeepsSelectedPackageAtomicAgainstLaterUnselectedEnvelope$|TestMaterializeReplacesPriorBookableSelectionAggregateAndSurvivesReload$|TestAvailabilitySelectionAfterSpecificRejected(Option|Date)OutOfTurnPayment$|TestSelectedAvailabilitySelectionPassengerFailClosedPersistsBeyondHistoryWindow$'
RED P1 REPLAY 1 — bootstrap legada inventava selection_message_id a partir da projeção
RED P1 REPLAY 2 — seleção/projeção anterior podia ser aceita como prompt source legado
RED P1 REPLAY 3 — LastAppliedEventOrder descartava evento durável atrasado e fazia live divergir do restart
RED P1 REPLAY 4 — ausência da URL PostgreSQL ainda resultava em SKIP com suíte verde
PASS — go test -count=20 ./internal/chat -run 'Test.*AvailabilitySelection.*(Replay|Order|Legacy|Projection|Invalidation)'
PASS — go test -race -count=1 ./internal/chat
PASS — modo PostgreSQL obrigatório falha quando CHAT_PASSENGER_STATE_POSTGRES_TEST_URL está ausente
PASS SEM SKIP — CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1 + PostgreSQL 16 real efêmero + duas pools + count=20; lock invertido e live/restart idênticos
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l => 54
PASS — git diff --check
STATUS — H-2026-07-22A — EM CORREÇÃO APÓS REVIEW — 4 P1 DE REPLAY CANÔNICO; aguardando novo review
```

Os oito P1 dos reviews anteriores permanecem corrigidos: snapshot
`HasEvidence=false` contraditório continua inválido, `UNKNOWN` literal fresco
continua permissivo, opção incompleta não abre passageiros, fallback-routed
selection não libera trabalho externo e cancelamento/humano não criam prompt
event espúrio; confirmação de opção única exige facts completos, listas válidas
são preservadas durante clarificação e índice histórico bruto não vira seleção.

Os três P1 do review anterior mais recente foram fechados com a classificação centralizada
`NONE`/`MATERIALIZE`/`CLARIFY_PRESERVE`/`FAIL_CLOSED_INVALIDATE`. Confirmação de
opção única só materializa com facts completos; deíxis ambígua e índice fora do
range preservam a lista e permitem o índice posterior; falha estrutural cria
marker + boundary. Booking draft e `booking_create` não consultam mais número
histórico bruto nem aplicam fallback automático de opção única: apenas índice
explícito atual ou snapshot bookable persistido possui autoridade.

Os três P1 do review anterior separam identidade do prompt de facts de
continuidade, executam humano/cancelamento `STRONG` antes do gate de seleção e
adicionam `BookingDraftContext.HasBookableSelection`. A flag só nasce de uma
seleção explícita atual materializada contra facts completos ou da seleção
bookable persistida; payload de availability e opção única isolados não
concedem autoridade. Todos os entrypoints de
`booking_create`, inclusive confirmação de documentos, exigem essa autoridade.

O P1 do review anterior separa o baseline de routing da projeção da seleção atual.
`deriveCanonicalConversationStateForRoutingBaseline` preserva seleção bookable
persistida e exclui a seleção do `currentTurn`; humano/cancelamento `STRONG` usa
esse baseline sem classificar ou persistir a opção misturada. Apenas
`MATERIALIZE` chama `applyMaterializedAvailabilitySelectionToCanonicalState`.
As frases exatas exigidas, a opção `2` anterior, marker/boundary, o controle puro
e o turno seguinte estão cobertos.

O P1 do review anterior separa o envelope/filtro do item selecionado dentro do
booking draft. `bookingDraftRoutingBaselineProjection` usa `ENVELOPE_ONLY`;
resultado único não projeta mais trip/stops nem fatos do item. A política
`BOOKABLE_SELECTION` continua disponível para a projeção autorizada, e uma
seleção bookable persistida permanece exata em ambas. Apenas `MATERIALIZE`
aplica a opção atual ao canonical. Os quatro turnos `STRONG`, os controles
`"ok"`/`"1"`, a opção `2` anterior e o próximo router/interpreter estão
cobertos.

Os 3 P1 do review anterior substituem a busca reversa por um reducer cronológico.
`MATERIALIZE` substitui a autoridade anterior; rejeitar `S2` deixa `NONE` sem
ressuscitar `S1`, enquanto rejeitar `S1` depois de `S2` preserva a autoridade
corrente. Snapshot propagado preserva o `SelectionMessageID` original e não é
novo `MATERIALIZE`. `SelectionMessageID` e
`AvailabilityPromptSourceMessageID` acompanham a evidência e o snapshot
persistido. Rejeições específicas/totais usam o mesmo
prompt source estrutural, então índice/data iguais em listas diferentes não se
contaminam. Recuperação legada só consulta o próprio evento de seleção ou a
mensagem exata indicada pela fonte; envelope posterior nunca preenche pacote ou
outro campo ausente. `booking_create` permanece fechado sem autoridade.

Os 3 P1 anteriores de autoridade durável removem essa responsabilidade do reducer de
histórico normal. `AvailabilitySelectionStateV1` e seus eventos tipados são
persistidos na mesma transação serializada do estado de passageiros. A rejeição
do inbound grava o prompt source real; `BOOKABLE`, tombstones e
`applied_event_ids` sobrevivem a reload e truncamento; projeções copiadas ficam
explicitamente sem autoridade. Booking draft e `booking_create` usam o estado
durável e não consultam a lista visível como fonte de autoridade. A seleção do
turno só avança após comprovar que seu `SELECTION_MATERIALIZED` foi aplicado na
mesma seção serializada.

Os 4 P1 anteriores fecharam bootstrap, compatibilidade legada e concorrência. O
primeiro bootstrap converte marker/boundary em `SELECTION_INVALIDATED` sob a
mesma transação; seleção anterior não sobrevive e materialização explícita
posterior pode reabrir. Ausência de `materializes_authority` significa `false`
e não existe fallback de `selection_message_id` para projeção. Compatibilidade
legada exige intent estrutural de seleção, snapshot completo e fonte anterior
exata/inequívoca, resolvida sem body ou parser. Eventos carregam ordem canônica
do banco e o estado mantém `LastAppliedEventOrder`; batches são ordenados e
eventos antigos não substituem seleção/tombstone. A prova PostgreSQL real com
duas pools passou sem `SKIP`.

Os 4 P1 atuais substituem a atualização incremental por replay canônico
completo sob o lock da sessão. O evento atual é persistido estruturalmente no
inbound, todos os eventos da sessão são carregados sem `body` e sem `LIMIT 50`,
a ordem é hidratada somente de `received_at`, `created_at`, `message_id` e
ordinal do banco, e o reducer sempre parte do estado zero. `LastAppliedEventOrder`
é recalculado depois do replay e nunca descarta evento. A identidade legada sem
`selection_message_id` explícito falha fechado; prompt source só pode ser uma
única lista anterior estrutural compatível. Consumidores leem exclusivamente a
projeção validada. A propriedade cobre todas as permutações, duplicação e
restart, e o gate PostgreSQL 16 obrigatório passou `count=20` com duas pools.

Commit, push, deploy e smoke não foram executados. O status permanece
`EM CORREÇÃO APÓS REVIEW — 4 P1 DE REPLAY CANÔNICO`; os P1 da rodada
anterior e os demais P1 históricos permanecem corrigidos, não há declaração de
review limpo e B2 continua bloqueada.

## Gate operacional

O próximo gate é um novo review sem P1/P2. O desbloqueio de B2 exige
adicionalmente commit/push/deploy e smoke do cenário real, todos em rodada
explicitamente autorizada. Este `/goal` não autoriza nenhuma dessas operações
externas.

Smoke futuro mínimo:

```text
sessão realmente limpa
"oi" não abre ASK_PASSENGER_COUNT
rota + EARLIEST_AVAILABLE executa availability_search exatamente uma vez
seleção válida persiste a opção e então abre o prompt de passageiros
pós-prompt desconhecido/conflitante permanece fail-closed
```

## `/goal`

```text
Corrija somente os 4 P1 atuais de H-2026-07-22A.
Não iniciar B2, commit, push, deploy ou smoke.

Dentro do lock da sessão: persistir o evento atual na mensagem, carregar todos
os eventos estruturados sem body/LIMIT 50, carregar marker/boundary, hidratar a
ordem somente do banco, ordenar por timestamp canônico/created_at/message_id/
ordinal, reduzir desde zero, validar e persistir a projeção completa.

Não usar LastAppliedEventOrder para descartar eventos atrasados. Identidade
legada sem selection_message_id explícito resulta NONE. Prompt source legado
exige exatamente uma lista estrutural confiável e compatível; projeções e
continuações não são candidatas. Provar invariância para qualquer permutação,
duplicação e restart.

CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1 deve falhar sem URL. Com URL, usar
PostgreSQL 16 real, duas pools, lock invertido e comparação live/restart em
count=20. Executar também race, suítes amplas, regexp=54 e git diff --check.
Atualizar tracker/handoff/plano e não declarar review limpo.
```

## `/review`

```text
Revise somente H-2026-07-22A.

Procure UNKNOWN fresco ainda bloqueando greeting/rota/disponibilidade, estado
inválido ou conflitante escapando do fail-closed, prompt pendente confundido
com prompt entregue, `HasEvidence=false` contradizendo campos estruturais,
seleção incompleta abrindo `ASK_PASSENGER_COUNT` antes de materialização,
índice/snapshot/IDs vazando para payload/canonical/booking draft, chamada
externa pós-prompt, precedência STRONG quebrada, dependência de parser/regex e
qualquer implementação de B2. Verifique também confirmação de opção única sem
facts escapando do gate, deíxis/índice fora do range apagando lista válida,
facts não reanexados ao fallback de clarificação, número histórico reaparecendo
em booking draft/`booking_create`, fallback automático de opção única sem
snapshot bookable e marker/boundary ignorado fora de `Service.Reprocess`.
Verifique ainda `ASK_PASSENGER_COUNT` com facts de continuidade sendo confundido
com prompt de availability, seleção classificando antes de humano/cancelamento
`STRONG` e confirmação de documentos chegando a `booking_create` sem
`HasBookableSelection`. Para o P1 anterior, confirme que routing usa baseline
sem seleção do `currentTurn`, que opção bookable anterior é preservada, que
turnos mistos `STRONG` não alteram índice/snapshot/trip/stops nem marker/boundary,
que somente `MATERIALIZE` aplica a seleção atual e que o turno seguinte não
recebe a opção rejeitada como estado atual. Confirme também que o baseline usa
`ENVELOPE_ONLY`, que o merge de envelope não promove o único resultado, que
trip/stops/data/horário/preço/moeda/pacote do item ficam ausentes sem autoridade
bookable, que uma lista unitária nova não substitui seleção persistida e que
`"ok"`/`"1"` puros continuam materializando a opção completa. Para os 3 P1
atuais, confirme o reducer cronológico e a impossibilidade de ressuscitar
seleção superseded; a preservação de `S2` quando a rejeição alveja `S1`; o
escopo por `AvailabilityPromptSourceMessageID` em rejeições específicas e
totais; colisões de índice/data entre listas; persistência de
`SelectionMessageID`; recuperação legada limitada ao próprio evento ou à fonte
exata; ausência de pacote de envelope posterior; bloqueio de `booking_create`
sem autoridade; substituição integral por `MATERIALIZE`; e coerência após
reload.

Para os 3 P1 de autoridade durável, confirme também: estado V1 versionado e
validado; reducer sem texto; aplicação e persistência de evento inbound sob o
mesmo lock de sessão; bootstrap único da sessão estruturada completa, sem
`body`/`LIMIT 50`; projeções `materializes_authority=false`; tombstone e prompt
source sobrevivendo a reload; `S1 → S2 → rejeição de S2` terminando sem
autoridade; rejeição da lista `B` sem afetar seleção da lista `A`; booking draft
e `booking_create` priorizando o estado; ausência de call site de produção de
`latestVisibleAvailabilitySelectionContextWithSource` como autoridade; zero
`BookingCreateInput` sem `BOOKABLE`; e prova de que a materialização do turno
foi aplicada na mesma seção serializada antes de qualquer progressão.

Para os 4 P1 atuais, confirme também: evento atual persistido estruturalmente
antes do replay; consulta de todos os eventos da sessão sem `body`/`LIMIT 50`;
marker/boundary participando da mesma reconstrução; ordem hidratada apenas dos
campos reais da mensagem; reducer sempre iniciado do zero; nenhuma comparação
com `LastAppliedEventOrder` descartando evento atrasado; ID legado ausente
permanecendo ausente/`NONE`; nenhuma seleção ou continuação aceita como prompt
source; exatamente uma lista estrutural compatível exigida; consumidores
falhando fechado sem projeção persistida e validada; igualdade de estado em
todas as permutações, duplicações, reloads e restarts; rejeição `B` aplicada
depois da materialização causal `A` preservando `A` quando as fontes diferem;
modo PostgreSQL obrigatório falhando sem URL; PostgreSQL 16 real, duas pools,
lock invertido e `count=20` sem `SKIP`; e zero `booking_create` sem `BOOKABLE`.
```
