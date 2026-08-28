# Session handoff — sistema-schumacher

## Objetivo do projeto

Corrigir estruturalmente os comportamentos errados observados nas conversas sem transformar o backend em um parser geral de regex.

Arquitetura:

```text
OpenAI interpreta linguagem
validator local verifica contrato/facts
backend executa somente ações autorizadas
```

## Estado canônico atual

```text
3.6F-A concluída
3.6F-B concluída
3.6F-C concluída em código; H-2026-07-16A teve smoke operacional verde
H-2026-07-16B em andamento
H-2026-07-16B1 concluída em código, com gate operacional reaberto
H-2026-07-22A — problema original corrigido e deployado;
smoke RED na transição availability → passageiros
H-2026-07-27A — EM CORREÇÃO APÓS REVIEW —
1 P1 CORRIGIDO LOCALMENTE; AGUARDANDO NOVO REVIEW
H-2026-07-16B2 bloqueada por H-2026-07-27A e pelo gate operacional de B1
3.6F-D bloqueada por H-B
```

Esta é a única declaração vigente de H-2026-07-27A neste handoff. Todos os
status e próximas ações nas seções cronológicas abaixo são históricos e estão
**SUPERADOS** por este bloco.

## Incidente H-2026-07-27A

O smoke real posterior ao deploy de H-2026-07-22A comprovou uma lacuna nova:

```text
availability_search: 8 resultados brutos no tool_context
resposta ao cliente: somente 03/08/2026 + confirmação de opção única
availability_prompt_event_v1: ausente
availability_option_count: 8
resposta do cliente: "sim"
AvailabilitySelectionStateV1: NONE
resposta final: SAFE_PHASE_FALLBACK
booking_create: zero chamadas
```

O runtime confundia resultados brutos da tool com opções apresentadas e
dependia do body para reconhecer a identidade do prompt. H-2026-07-27A separa
agora três camadas:

```text
raw tool results
presented options
selected/bookable option
```

`AvailabilityPromptEventV1` persiste a projeção apresentada no draft e no
outbound efetivamente enviado. O evento contém `version`,
`kind=AVAILABILITY_OPTION_CHOICE`, `source_message_id`,
`presented_option_count` e, por opção, `display_index`, `result_index`,
`trip_id`, `board_stop_id`, `alight_stop_id` e `trip_date`.

O backend realiza deterministicamente a apresentação de
`EARLIEST_AVAILABLE`, escolhe o primeiro resultado estruturalmente válido e
produz a resposta unitária. `InferActivePromptContext`, o router, o interpreter
e a materialização preferem o evento; o body permanece fallback legado.
`display_index` é resolvido contra `presented_options`, não contra a posição
bruta. O evento sozinho não concede `BOOKABLE`: somente confirmação válida e
`SELECTION_MATERIALIZED` materializam snapshot completo.

Não houve parser/regex novo, B2, migration, provider, commit, push, deploy ou
smoke nesta rodada. O inventário de produção permanece em 54
`regexp.MustCompile`.

RED/PASS reais:

```text
RED — go test -count=1 ./internal/chat -run '^TestAvailabilityPromptEventV1'
      runner=1, evento ausente e confirmação podendo repetir availability_search
PASS — focused count=20 — 1.485s
PASS — focused race — 1.539s
PASS — availability/passenger/booking/human/cancel — 0.129s
PASS — go test -count=1 ./internal/chat — 4.121s
PASS — go test -count=1 ./... — internal/chat 4.145s; demais pacotes verdes
PASS — regexp.MustCompile produção = 54
PASS — gofmt
PASS — git diff --check
```

A matriz cobre A–I, inclusive 8 resultados brutos/1 apresentado, 5
apresentados com `"sim"` ambíguo, `display_index=1 → result_index=3`, status
invisíveis, não reabertura durante `ASK_PASSENGER_COUNT`, precedência
humano/cancelamento, zero booking sem materialização e reload/restart.

O review seguinte encontrou três P1 na proveniência da apresentação:

- `BOT_AUTO_REPLY` exigia body equivalente antes de reconciliar eventos
  estruturais equivalentes;
- o bootstrap legado lia evento/facts de mensagem `INBOUND` marcada
  `PROCESSED`;
- `DRAFT_REVIEW / APPROVED_AS_IS` carregava o evento entregue, mas não
  resolvia o `tool_context` do draft validado.

Agora `resolveDeliveredPromptSourceMessageWithIndex` é a abstração comum dos
dois modos de entrega e também alimenta a busca do bootstrap legado. O outbound
entregue define ID, `source_message_id`, body, status e evento; somente o draft
`OUTBOUND`, ligado por `draft_message_id` e validado, fornece facts. A
comparação estrutural normaliza e compara kind, option count e todos os campos
das opções. Body é fallback apenas se os dois lados não possuem evento.

Evento inválido, ausente ou divergente, source não confiável, entrega pending e
revisão `EDITED` não herdam facts. Um draft vinculado não volta a ser candidato
do bootstrap se a entrega falhar na reconciliação. `APPROVED_AS_IS` preserva a
referência durável e projeta o evento para a identidade entregue, sem copiar
`tool_context` para o outbound.

RED/PASS da correção após review:

```text
RED — body diferente: HasCurrentFacts=false
RED — evento divergente: facts antigos herdados
RED — APPROVED_AS_IS: HasCurrentFacts=false
RED — prompt INBOUND fabricado: AvailabilitySelectionStateV1=BOOKABLE
PASS — revisão EDITED já falhava fechado
PASS — testes novos dirigidos, count=20 — 1.325s
PASS — go test -race -count=1 ./internal/chat — 23.499s
PASS — availability/passenger/booking/human/cancel — 3.185s
PASS — go test -count=1 ./internal/chat — 4.289s
PASS — go test -count=1 ./... — internal/chat 4.290s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt
PASS — git diff --check
```

O segundo review dirigido encontrou mais três P1. A correção local agora:

- exige status `SENT`, `DELIVERED`, `READ` ou `AUTOMATION_SENT` junto do
  `delivery_recorded_at` canônico;
- falha fechado quando a chave `availability_prompt_event_v1` está presente,
  mas o evento não decodifica ou diverge, sem retorno ao legado;
- filtra metadata reservada do cliente, preservando a proveniência
  `DRAFT_REVIEW / APPROVED_AS_IS` e impedindo `tool_context` injetado.

RED real:

```text
FAIL — statuses não confirmados e status enviado sem evidência abriam prompt e podiam reconstruir BOOKABLE
FAIL — evento malformado caía em intent/template/tool_context legado e virava BOOKABLE
FAIL — metadata do cliente substituía mode/draft/review e persistia facts forjados
```

PASS local:

```text
PASS — focused count=20 — 3.582s
PASS — go test -race -count=1 ./internal/chat — 24.519s
PASS — availability/passenger/booking/human/cancel — 2.918s
PASS — go test -count=1 ./internal/chat — 4.311s
PASS — go test -count=1 ./... — internal/chat 4.306s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt
PASS — git diff --check
```

O terceiro review dirigido encontrou cinco P1 adicionais. A correção local
centraliza toda mensagem candidata de availability em quatro classes:
`UNDELIVERED`, `ABSENT_LEGACY`, `VALID_STRUCTURAL` e `INVALID`.

- `UNDELIVERED` e `INVALID` nunca abrem prompt, fornecem facts, participam do
  bootstrap ou chegam aos gates de booking;
- fallback legado existe somente para `ABSENT_LEGACY` entregue;
- o allowlist único é `SENT`, `DELIVERY_ACK`, `DELIVERED`, `READ` e
  `AUTOMATION_SENT`, sempre com `delivery_recorded_at`;
- evento estrutural exige as duas cópias, decode, igualdade e consistência com
  source/count/options/facts;
- todos os readers de active prompt, facts, selection, enriquecimento de
  snapshot, booking draft e `booking_create` usam a classificação;
- `APPROVED_AS_IS` preserva body, kind/canal, evento e facts do draft validado,
  bloqueando metadata reservada e campos/aliases de mídia; mídia humana
  legítima fora da revisão continua verde.

RED/PASS reais desta rodada:

```text
RED — approved TEXT virou AUDIO; legado não entregue abriu prompt; DELIVERY_ACK virou UNKNOWN; cópias inválidas vazaram facts e enriqueceram snapshot
PASS — testes novos count=20 — 0.650s
PASS — go test -race -count=1 ./internal/chat — 28.873s
PASS — availability/passenger/booking/human/cancel/review/autosend/delivery — 3.740s
PASS — go test -count=1 ./internal/chat — 5.114s
PASS — go test -count=1 ./... — internal/chat 6.579s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt; gofmt -l sem saída
PASS — git diff --check
```

A primeira tentativa do race parou antes dos testes por quota de `/tmp`; após
limpar somente o cache Go temporário desta tarefa, o mesmo comando passou
integralmente.

O quarto review dirigido encontrou 3 P1 + 1 P2:

- o webhook podia regredir `DELIVERY_ACK` para `SERVER_ACK` tardio;
- `INVALID` entregue mais novo podia ressuscitar uma lista legada anterior;
- `DRAFT_REVIEW` não aprovado ou divergente podia virar fallback legado;
- a proteção contra metadata de mídia hostil ainda não estava provada pelo
  `Repository.CreateReply` real.

A correção usa uma política monotônica compartilhada entre webhook, sender e
readers; preserva o maior status/evidência; transforma `INVALID` entregue em
barreira temporal de active prompt, facts, replay, booking draft e
`booking_create`; e aceita `DRAFT_REVIEW` somente com
`APPROVED_AS_IS` consistente nas duas cópias.

A prova P2 percorreu `Service.Reply`, `Repository.CreateReply`, sender e
`MarkReplyDeliverySent` reais em PostgreSQL 16.14. Mesmo com
`media_kind=AUDIO`, base64 e facts forjados, o sender e as linhas reais de
`chat_messages`/`outbound_messages` permaneceram texto e resolveram os facts
do draft validado. O modo PostgreSQL obrigatório falha explicitamente sem URL.

RED/PASS desta rodada:

```text
RED — ACK regrediu, INVALID entregue reviveu legado e DRAFT_REVIEW não aprovado virou ABSENT_LEGACY
PASS — testes focados count=20 — 1.891s
PASS — go test -race -count=1 ./internal/chat — 26.423s
PASS — regressões delivery/review/availability/passenger/booking/human/cancel/media — chat 0.674s; automation 0.008s
PASS SEM SKIP — PostgreSQL 16.14 obrigatório — chat 1.245s; automation 0.151s
PASS — go test -count=1 ./internal/chat — 4.682s
PASS — go test -count=1 ./... — internal/chat 7.017s; automation 0.244s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt; gofmt -l sem saída
PASS — git diff --check
```

A primeira tentativa do race parou antes dos testes por cota de disco. Somente
o cache Go temporário desta tarefa foi limpo; a repetição e a execução final
passaram.

O review posterior preservou as correções anteriores e encontrou **2 P1 + 1
P2** adicionais:

- `tool_context` presente e não-mapa em uma das cópias podia ser ignorado e
  deixar a outra cópia isolada fornecer facts como `ABSENT_LEGACY`;
- uma projeção entregue vinculada a um draft com facts inválidos podia remover
  a barreira antes de reclassificar a mensagem efetiva resolvida;
- datas persistidas com espaços eram normalizadas e aceitas apesar do contrato
  canônico exato `YYYY-MM-DD`.

O status registrado naquela rodada era **H-2026-07-27A — EM CORREÇÃO APÓS REVIEW — 2 P1 + 1 P2
DE AUTORIDADE FACTUAL, BARREIRA RESOLVIDA E DATAS CANÔNICAS**.

As regressões anteriores à correção reproduziram os três defeitos: 15
representações temporais com whitespace foram aceitas; uma cópia
`tool_context` não-mapa foi ignorada; e uma projeção resolvida para facts
`INVALID` removeu a barreira. O comando dirigido falhou como esperado com
`internal/chat 0.016s`.

A correção local agora marca a cópia não-mapa como presente e inválida,
reclassifica a mensagem efetiva antes de remover a barreira e exige datas
persistidas exatamente em `YYYY-MM-DD`, sem `TrimSpace`.

Gates executados em `golang:1.23` via Docker, pois o host não possui Go/gofmt:

```text
PASS — testes dirigidos count=1 — 0.028s
PASS — testes dirigidos count=20 — 0.446s
PASS — go test -race -count=1 ./internal/chat — 51.081s
PASS — availability/passenger/booking/review/delivery/human/cancel — 6.436s
PASS — go test -count=1 ./internal/chat — 9.164s
PASS — go test -count=1 ./... — internal/chat 9.054s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt; gofmt -l internal/chat sem saída
PASS — git diff --check; git diff --cached --check
```

Não há declaração de review limpo nem segurança para commit. Não houve commit,
push, PR, merge, deploy ou smoke. B2 e 3.6F-D continuam bloqueadas.

O review seguinte encontrou **1 P1 + 1 P2**:

- uma projeção `DRAFT_REVIEW/APPROVED_AS_IS` entregue, resolvida para uma
  pergunta apenas de passageiros com `tool_context` não-mapa, criava barreira
  de availability e apagava `BOOKABLE` anterior;
- `results[].trip_date` persistido ainda aceitava whitespace porque o decode e
  o validador normalizavam o texto.

O status registrado naquela rodada era **H-2026-07-27A — EM CORREÇÃO APÓS REVIEW — 1 P1 + 1
P2**.

O RED dirigido reproduziu a barreira indevida e todas as representações
inválidas pedidas para `results[].trip_date`; o comando encerrou em RED com
`internal/chat 0.021s`. A correção local agora:

- resolve a projeção e só aplica a barreira da mensagem efetiva quando ela é
  candidata de availability;
- não usa modo de review, ação aprovada ou `tool_context` não-mapa isolados
  como prova de domínio;
- exige `results[].trip_date` presente como string canônica exata
  `YYYY-MM-DD`, com parse + format idênticos e sem `TrimSpace`;
- mantém chave ausente na semântica legada já existente, enquanto opção
  estrutural apresentada continua exigindo data.

Os testes transversais cobrem active prompt, finders, option count, bootstrap,
`AvailabilitySelectionStateV1`, booking draft e `booking_create`. O prompt de
passageiros preserva `PASSENGER_COUNT` e o `BOOKABLE` anterior; data inválida
falha fechado e produz `Presented=nil`. Projeção de availability `INVALID`,
`BOT_AUTO_REPLY` irresolvida, `UNDELIVERED`, projeção válida e
humano/cancelamento `STRONG` continuam verdes.

Gates executados em `golang:1.23` via Docker:

```text
PASS — testes dirigidos count=1 — 0.042s
PASS — testes dirigidos count=20 — 0.893s
PASS — regressão isolada da fixture canônica — 0.008s
PASS — go test -race -count=1 ./internal/chat — 44.106s
PASS — availability/passenger/booking/review/delivery/human/cancel — 5.711s
PASS — go test -count=1 ./internal/chat — 8.121s
PASS — go test -count=1 ./... — internal/chat 8.256s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt; gofmt -l internal/chat sem saída
PASS — git diff --check; git diff --cached --check
```

O primeiro race encontrou fixtures de validação que persistiam
`trip_date=""`; a fixture foi ajustada para data futura canônica e o race foi
repetido integralmente. Uma tentativa anterior do runner não chegou aos testes
porque `gofmt` não estava no `PATH` da shell do contêiner; os comandos seguintes
usaram os binários absolutos.

Não houve B2, 3.6F-D, parser, regex, migration, provider, PostgreSQL, commit,
push, PR, merge, deploy ou smoke. Não há declaração de review limpo nem
segurança para commit.

O review seguinte encontrou **1 P1 DE DOMÍNIO DO FALLBACK TEXTUAL**: uma
projeção `BOT_AUTO_REPLY` ou `DRAFT_REVIEW/APPROVED_AS_IS` entregue e
irresolvida, mas explicitamente pertencente a pagamento ou a outro domínio,
ainda podia ser marcada como availability apenas pelo body genérico. O prompt
`"Pode pagar no PIX ou no cartão. Qual opção você prefere?"` criava barreira e
apagava a autoridade `BOOKABLE` anterior.

O status vigente é **H-2026-07-27A — EM CORREÇÃO APÓS REVIEW — 1 P1 DE
DOMÍNIO DO FALLBACK TEXTUAL**.

O RED final cobriu `BOT_AUTO_REPLY` e review aprovado com intent de pagamento,
template de passageiros, prompt kind documental, template humano e intent de
cancelamento. Os 10 subcasos falharam com `candidate=true`; o comando dirigido
encerrou em RED com `internal/chat 0.008s`.

A correção mantém evento, `availability_search`, intent/template de
availability, seleção/snapshot e marcador de autoridade com precedência. Antes
somente do fallback pelo body, metadata estrutural pertencente a outro domínio
agora retorna `candidate=false`. Não houve vocabulário, regex, parser ou
refactor amplo.

Os testes provam zero barreira, preservação do finder e do `BOOKABLE` anterior
no bootstrap, `AvailabilitySelectionStateV1`, booking draft e
`booking_create`, além da continuidade da metadata do domínio próprio e zero
publicação como `ABSENT_LEGACY`. Lista legada real sem metadata continua no
fallback; projeção real de availability irresolvida e availability estrutural
continuam fail-closed/autoridade conforme o contrato.

Gates executados em `golang:1.23` via Docker:

```text
PASS — testes dirigidos pós-patch, count=1 — internal/chat 0.077s
PASS — testes dirigidos e controles, count=20 — internal/chat 2.136s
PASS — go test -race -count=1 ./internal/chat — 47.710s
PASS — availability/payment/passenger/document/review/delivery/human/cancel — 6.672s
PASS — go test -count=1 ./internal/chat — 9.280s
PASS — go test -count=1 ./... — internal/chat 9.076s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt; gofmt -l internal/chat sem saída
PASS — git diff --check; git diff --cached --check
```

Não houve B2, 3.6F-D, parser, regex, migration, provider, PostgreSQL, commit,
push, PR, merge, deploy ou smoke. Não há declaração de review limpo nem
segurança para commit.

O review atual encontrou **2 P1 + 2 P2 DE RECONCILIAÇÃO DE DOMÍNIO E FALLBACK
LEGADO**. `messageMayCarryAvailabilityPromptV1` aceitava metadata availability
de uma cópia antes de reconciliar a outra; o fallback textual genérico
classificava projeções reais de pagamento, passageiros, documentos, suporte e
cancelamento; quatro sinais canônicos não eram positivos; e o body real de
`buildEarliestAvailabilityReply` não era reconhecido.

O status vigente é **H-2026-07-27A — EM CORREÇÃO APÓS REVIEW — 2 P1 + 2 P2
DE RECONCILIAÇÃO DE DOMÍNIO E FALLBACK LEGADO**.

Os REDs foram executados antes do patch de produção. Os 40 subcasos de
projeção usaram o shape real do repository, sem `intent`, `template_name` ou
prompt kind artificiais, nos dois modos de entrega e com draft ausente,
duplicado, posterior ou inválido. A matriz também cobriu cópias conflitantes,
unilaterais, vazias, desconhecidas ou não-string, os quatro sinais omitidos e
os dois builders legados:

```text
RED — TestAvailabilityPromptDomainReconciliationV1
RED — TestAvailabilityPromptTextFallbackUsesRealProjectionShapeV1
RED — TestAvailabilityPromptLegacyBuilderFallbacksV1
FAIL esperado — internal/chat 0.020s
```

Agora cada cópia é classificada como `AVAILABILITY`, `NON_AVAILABILITY`,
`ABSENT` ou `INVALID`, e a decisão ocorre apenas após reconciliação. Evento,
facts ou seleção bilateral válida mantêm precedência estrutural. Sem essa
precedência, metadata availability precisa ser bilateral, reconhecida,
coerente e idêntica. Metadata não-availability ou qualquer conflito,
malformação ou unilateralidade retorna false. Metadata availability bilateral
ainda mantém o domínio quando os facts são `INVALID`, preservando a barreira
fail-closed.

Os context fallbacks de opção/data e prompt kinds availability são lidos nos
containers reais, inclusive `template_data`. O fallback textual aceita somente
lista numerada sequencial e EARLIEST com as constantes exatas dos builders; o
option count renderizado reutiliza o mesmo reconhecedor. Nenhum regex, parser
geral ou vocabulário novo foi adicionado.

Respostas informativas out-of-turn permanecem fora do domínio de authority. A
continuidade do active prompt exige `active_prompt_source_message_id` e
`active_prompt_kind` bilaterais, mantém o body do lembrete e ancora facts e
option count no prompt de availability anterior, único e entregue. Fixtures
canônicas foram alinhadas ao writer real, que persiste `tool_context`/seleção
nas duas cópias.

Gates executados em `golang:1.23` via Docker:

```text
PASS — testes dirigidos e controles, count=20 — internal/chat 7.284s
PASS — go test -race -count=1 ./internal/chat — 53.380s
PASS — availability/payment/passenger/document/review/delivery/human/cancel — 7.144s
PASS — go test -count=1 ./internal/chat — 9.710s
PASS — go test -count=1 ./... — internal/chat 9.412s; demais pacotes verdes
PASS — inventário de produção em internal/chat = 54 regexp.MustCompile
PASS — gofmt -l internal/chat sem saída
OBSERVAÇÃO — gofmt -l . lista somente arquivos preexistentes fora de internal/chat
PASS — git diff --check; git diff --cached --check
```

Arquivos alterados nesta rodada:

```text
apps/api/internal/chat/availability_prompt_event_v1.go
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/interpreter.go
apps/api/internal/chat/response_realizer.go
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/passenger_clarification_test_helper_test.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/availability_selection_state_v1_test.go
apps/api/internal/chat/booking_create_router_test.go
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
```

Não há review limpo nem segurança para commit. Não houve B2, 3.6F-D, migration,
provider, PostgreSQL, commit, push, PR, merge, deploy ou smoke. A única próxima
ação é: `/review` dirigido aos 2 P1 + 2 P2. H-2026-07-16B2 e 3.6F-D
permanecem bloqueadas.

## Incidente H-2026-07-22A

Confirmado:

- sessão nova após limpeza;
- primeira mensagem `"oi"` recebeu `ASK_PASSENGER_COUNT`;
- `"monção para videira na data mais próxima"` também recebeu
  `ASK_PASSENGER_COUNT`;
- `tool_call_count=0`, sem execução de `availability_search`;
- `history_count=1`;
- `newPassengerClarificationStateV1` inicia os slots como desconhecidos;
- o gate anterior tratava esse bootstrap como inseguro globalmente em
  `Service.Reprocess`.

O hotfix deve distinguir bootstrap desconhecido, contexto de passageiros ativo
e estado inválido/conflitante. Não enfraquecer o fail-closed depois do prompt de
passageiros entregue. Não implementar B2, parser, regex ou provider.

O review anterior encontrou dois P1, que permanecem corrigidos localmente:

- `HasEvidence=false` contraditório com evidência estrutural era aceito e
  liberava o shadow V1; o validator agora rejeita o snapshot, que vira
  `invalidPassengerClarificationStateV1` e permanece fail-closed;
- opção visível sem `trip_id`, `board_stop_id` ou `alight_stop_id` podia abrir
  `ASK_PASSENGER_COUNT`; a seleção agora exige materialização completa e, se
  incompleta, retorna fallback determinístico sem índice, snapshot, época de
  passageiros ou trabalho externo.

O controle positivo permanece: `UNKNOWN` fresco literal continua válido e não
bloqueia greeting, coleta de rota ou `availability_search`; opção completa
materializa rota/snapshot antes de anexar o prompt pendente de passageiros.

O review anterior seguinte encontrou três P1, que também permanecem corrigidos
localmente:

- tentativa numérica/contextual diante de prompt de availability agora exige
  materialização mesmo quando o router devolve fallback; falha cria ou preserva
  marker + boundary, mantém zero runner/JSON/shadows/claim/tools e impede reuso
  posterior do número bruto;
- o fallback de availability vem logo depois de guardrails `STRONG`, inclusive
  com `passengerUnsafe=true`; opção incompleta permanece em trip selection sem
  `ASK_PASSENGER_COUNT`, índice, snapshot ou evento de passageiros;
- prompt event usa allowlist de template + action. Cancelamento, humano, info e
  `SAFE_PHASE_FALLBACK` genérico não abrem epoch; delivery de pergunta real de
  passageiros continua aplicando o evento estrutural.

Número sem prompt/marker de availability não abre o gate. Uma tentativa
repetida sob marker existente continua fail-closed, preserva o boundary e não
reexecuta o router. As duas correções anteriores continuam cobertas.

O review anterior mais recente encontrou outros três P1, que permanecem
corrigidos localmente:

- o gate foi centralizado em `NONE`, `MATERIALIZE`, `CLARIFY_PRESERVE` e
  `FAIL_CLOSED_INVALIDATE`; confirmação já reconhecida, como `"ok"`, só é
  seleção quando há prompt ativo de availability com uma opção;
- opção única completa materializa índice `1` e snapshot antes de passageiros;
  sem facts completos, a mesma confirmação grava marker + boundary e mantém
  zero runner/JSON/shadows/claim/tools;
- várias opções completas + `"essa msm"`, e índice fora do range, usam
  `CLARIFY_PRESERVE`: pedem número, não criam marker, reanexam os facts válidos
  e permitem que o `"1"` posterior selecione normalmente;
- booking draft e `booking_create` não reutilizam mais número bruto histórico.
  Só índice explícito do turno contra facts atuais completos ou evidência
  bookable persistida com snapshot completo possui autoridade; não há fallback
  automático de opção única em `booking_create`.

O inventário de `findLatestSelectedOptionIndex` ficou sem call site de produção;
o helper residual também ignora texto e retorna somente evidência bookable. Um
`"1"` falho seguido de availability nova e `"quero reservar"` não produz
`BookingCreateInput`, enquanto uma seleção bookable persistida mantém o fluxo
normal. Os cinco P1 anteriores continuam corrigidos e cobertos.

O review anterior encontrou três P1 adicionais, que permanecem corrigidos
localmente:

- identidade do prompt e facts de continuidade foram separados. O gate só
  classifica seleção quando `ActivePrompt.Kind` é
  `AVAILABILITY_OPTION_CHOICE` e a fonte corresponde ao outbound confiável que
  realmente pergunta qual opção; `ASK_PASSENGER_COUNT` pode carregar o mesmo
  `tool_context`, mas respostas `"1"`/`"sim"` continuam pertencendo ao prompt
  de passageiros e não trocam a viagem materializada;
- humano/cancelamento `STRONG` é executado antes da classificação de seleção.
  Os quatro turnos mistos exigidos não materializam opção, não criam marker ou
  snapshot e não despacham runner, shadows ou tools;
- `BookingDraftContext.HasBookableSelection` registra autoridade explícita.
  Somente seleção atual materializada contra facts completos ou seleção
  bookable persistida ativa a flag; opção única exibida, campos de viagem ou
  `tool_context` isolados apenas enriquecem o contexto. Todos os entrypoints de
  `booking_create`, inclusive confirmação de documentos, exigem a flag.

Os REDs reais mostraram a opção `2` sendo substituída pela `1` a partir da
resposta de passageiros, `"sim"` virando fallback de availability, os turnos
mistos de humano/cancelamento perdendo a precedência e confirmação documental
sem snapshot produzindo `BookingCreateInput` com índice `0` e uma chamada de
`booking_create`. Os novos testes e o controle positivo com snapshot persistido
estão verdes. Os oito P1 anteriores continuam corrigidos e cobertos.

O review anterior encontrou **1 P1**, corrigido localmente e preservado pelo
review mais recente:

- o estado canônico de routing era derivado com `currentTurn`; por isso
  `"opção 1, quero cancelar"` e
  `"primeira opção, quero falar com atendente"` chegavam ao router já com
  `SelectedOptionIndex=1` e `trip/board/alight` da opção `1`, mesmo quando a
  resposta final respeitava o intent `STRONG`;
- `bookingDraftRoutingBaselineProjection`,
  `collectBookingDraftContextForRoutingBaseline` e
  `deriveCanonicalConversationStateForRoutingBaseline` agora constroem o estado
  de roteamento somente com sessão/histórico persistido. Seleção bookable anterior
  é preservada; seleção do turno atual não é projetada;
- humano/cancelamento `STRONG` usa e persiste esse baseline, não chama o
  classificador de seleção e não cria índice, snapshot, rota, prompt event ou
  mudança de marker/boundary;
- somente `MATERIALIZE` aplica a opção atual completa ao estado canônico. O
  controle `"opção 1"` continua persistindo snapshot/rota e só então pergunta
  passageiros;
- a opção `2` bookable anterior permanece intacta quando o turno misto menciona
  a opção `1`; o turno seguinte entrega ao router e ao shadow V1 o baseline, sem
  reutilizar a opção rejeitada;
- a precedência foi mantida estreita: cancelamento real, handoff, limpeza de
  `pending_question`, shadows e tools fora de selection context continuam no
  comportamento anterior.

RED real: as duas frases com `"opção 1"`/`"primeira opção"` chegaram ao router
com a opção `1` materializada. Um caso complementar com marker/boundary existentes
retornou `CONTEXT_FALLBACK_AVAILABILITY_OPTION` em vez de cancelamento. A matriz
final — `count=20`, race, regressões amplas, pacote, suíte completa e inventário
de 54 `regexp.MustCompile` — está verde. Os três P1 do review anterior permanecem
corrigidos.

O review anterior encontrou **1 P1**, corrigido localmente e preservado pela
rodada atual:

- o baseline já excluía `currentTurn`, mas
  `mergeAvailabilityPayloadIntoBookingDraft` ainda promovia o único resultado
  completo. Com `SelectedOptionIndex=0` e `HasBookableSelection=false`, o
  router recebia `trip/board/alight`, data, horário, preço e moeda como se
  houvesse item escolhido;
- `BookingDraftContext` agora explicita as políticas `ENVELOPE_ONLY` e
  `BOOKABLE_SELECTION`. O baseline de routing usa `ENVELOPE_ONLY`;
- envelope/filtro e item selecionado são merges separados. O fallback de
  resultado único foi removido: sem seleção bookable persistida, o baseline
  mantém `HasAvailabilityShown` e os endpoints do filtro, mas nenhum fato do
  item;
- uma seleção bookable persistida continua autoritativa e é preservada
  exatamente. Uma nova lista unitária não selecionada não substitui nem apaga a
  opção anterior;
- somente `MATERIALIZE` aplica a opção atual ao canonical, persiste
  índice/snapshot/trip/stops e abre `ASK_PASSENGER_COUNT`.

O RED unitário encontrou todos os fatos do item único no baseline sem
autoridade. Os quatro turnos exigidos (`"opção 1, quero cancelar"`,
`"essa msm, quero falar com atendente"`, `"ok, quero cancelar"` e
`"1, quero falar com uma pessoa"`) entregaram o item ao router; ele também
reapareceu no router/interpreter do turno seguinte. O controle preservou a
opção `2` bookable anterior, e `"ok"`/`"1"` puros continuaram materializando a
opção única. O `count=20` dirigido, race, regressões amplas, pacote, suíte
completa, inventário de 54 `regexp.MustCompile` e `git diff --check` estão
verdes.

O review anterior encontrou **2 P1**, que permanecem corrigidos localmente:

- uma rejeição posterior específica da opção `1` ocultava a seleção bookable
  anterior da opção `2`. O snapshot ainda podia preencher `trip/board/alight`,
  deixando autoridade falsa e IDs selecionados coexistirem;
- `PackageName` não integrava a autoridade selecionada. O baseline `STRONG`
  podia combinar opção `2`/rota/data/preço do pacote `A` com pacote `B` de um
  envelope posterior não selecionado;
- `latestAvailabilitySelectionEvidence` agora resolve autoridade temporalmente:
  acumula rejeições do mais novo para o mais antigo, encerra em rejeição global,
  restringe rejeição específica ao índice/data alvo e preserva os blockers
  `incomplete`/`metadata-only`;
- `HasBookableSelection`, índice, IDs, endpoints, pacote, data, horário, preço e
  moeda passam a ser aplicados como um único agregado;
- `PackageName` integra evidência, item, booking draft, snapshot persistido e
  canonical. Com seleção bookable, availability posterior não selecionada não
  entra no estado canônico nem em memory/metadata;
- snapshot antigo sem pacote só recupera o valor do item com os mesmos
  `trip_id + board_stop_id + alight_stop_id`; o campo de pacote do filtro/envelope
  não é usado como fallback;
- `MATERIALIZE` posterior substitui todo o agregado, e o reload/turno seguinte
  reconstrói autoridade, IDs e pacote coerentes.

Os REDs reproduziram `authority=false` para a opção `2` não rejeitada e o
baseline híbrido opção `2`/pacote `B`. O `count=20` dirigido, race, regressões
H-012/document/lap-child/payment/human/cancel/availability/passenger gate,
`./internal/chat`, `./...`, inventário de 54 `regexp.MustCompile` e
`git diff --check` estão verdes. Os P1 anteriores permanecem corrigidos. Não há
declaração de review limpo.

O review atual encontrou **3 P1**, corrigidos localmente e ainda aguardando novo
review:

- o scan reverso pulava `S2` rejeitada e ressuscitava `S1`, apesar de
  `MATERIALIZE` de `S2` já ter substituído integralmente a autoridade;
- rejeição específica/total não preservava o ID do prompt de origem. A opção
  `1` rejeitada na lista `B` podia remover a opção `1` selecionada na lista `A`,
  inclusive com a mesma data;
- recuperação de snapshot legado incompleto examinava envelopes posteriores.
  Um item não selecionado com os mesmos `trip/board/alight` podia preencher
  `package-b` antes do evento selecionado `A`.

`latestAvailabilitySelectionEvidence` agora é um reducer cronológico.
`MATERIALIZE` substitui a autoridade anterior; rejeitar a autoridade corrente
deixa `NONE`, sem restauração de seleção superseded. Rejeitar `S1` depois de
`S2` preserva `S2`, e blockers `incomplete`/`metadata-only` continuam
fail-closed. Snapshot propagado em turno posterior mantém o
`SelectionMessageID` original e não rematerializa autoridade stale.

A seleção persiste `selection_message_id` e
`availability_prompt_source_message_id`. Rejeições reutilizam
`active_prompt_source_message_id`; lembretes out-of-turn mantêm a fonte
estrutural da lista original. Rejeição específica ou total só afeta autoridade
do mesmo prompt source.

Snapshot legado só recupera campos do próprio evento de seleção ou da mensagem
exata indicada por `AvailabilityPromptSourceMessageID`, com
`trip/board/alight` compatíveis. Envelope posterior nunca participa; sem fonte
exata, a lacuna permanece vazia. Sem autoridade após rejeitar `S2`,
`booking_create` permanece fechado.

Os REDs reproduziram ressurreição de `S1`, colisão lista `A`/lista `B` e
injeção de `package-b`. O `count=20` dirigido, race, regressões
H-012/document/lap-child/payment/human/cancel/availability/passenger gate,
`./internal/chat`, `./...` e inventário de 54 `regexp.MustCompile` estão
verdes. `git diff --check` também passou após a atualização documental. Não há
declaração de review limpo.

O review seguinte preservou essas correções e encontrou **3 P1 DE AUTORIDADE
DURÁVEL**, corrigidos localmente e ainda aguardando novo review:

- `booking_create` podia usar como fonte o outbound unitário que já perguntava
  passageiros, em vez do prompt original da lista;
- projeção propagada podia adquirir autoridade quando o `MATERIALIZE` e a
  rejeição saíam da janela;
- rejeição simples não persistia prompt source, portanto deixava de atingir a
  seleção depois do truncamento.

`AvailabilitySelectionStateV1` agora é a autoridade normal versionada em
`metadata.memory`. Ele persiste `NONE | BOOKABLE | REJECTED | INVALIDATED`,
IDs do evento e da projeção, prompt source, índice, snapshot completo,
tombstone/rejeições e `applied_event_ids`. Os eventos
`SELECTION_MATERIALIZED`, `SELECTION_REJECTED` e `SELECTION_INVALIDATED` são
reduzidos sem texto na mesma seção serializada do estado de passageiros.

`MATERIALIZED` substitui o agregado; rejeição da autoridade corrente limpa o
agregado e mantém tombstone; rejeição de outro prompt não altera a seleção;
projeções copiadas usam `materializes_authority=false`. A rejeição reconhecida
no turno atual grava o prompt source real no `normalized_payload` do inbound
dentro da transação.

Booking draft e `booking_create` consomem primeiro o estado durável.
`booking_create` não usa a lista visível como autoridade e produz zero input sem
`BOOKABLE`. A seleção explícita só progride depois de confirmar que o evento
materializado consta no estado devolvido pela mesma seção serializada.

O bootstrap legado percorre uma única vez a sessão estruturada completa, sem
ler `body` e sem `LIMIT 50`, e persiste imediatamente o estado. Sem prova exata,
permanece `NONE`/fail-closed. O controle fake concorrente preservou ambos os
eventos. Naquela rodada, a prova PostgreSQL havia ficado em `SKIP`; a rodada
atual substitui essa lacuna por execução real sem `SKIP`.

O `count=20` dirigido, race, regressões
H-012/document/lap-child/payment/human/cancel/availability/passenger gate,
`./internal/chat`, `./...`, inventário de 54 `regexp.MustCompile` e
`git diff --check` estão verdes. Não há declaração de review limpo.

O review anterior preservou essas correções e encontrou **4 P1 DE AUTORIDADE
DURÁVEL**. Marker/boundary passaram a participar do bootstrap; identidade e
fonte legadas ficaram explícitas; a ordem passou a ser hidratada dos campos
reais da mensagem. A rodada seguinte comprovou, porém, que a aplicação ainda
era incremental e podia divergir do restart.

O review anterior encontrou **4 P1 DE REPLAY CANÔNICO**. Essa rodada
introduziu o replay completo sob o lock, tornou o PostgreSQL 16 obrigatório e
removeu o descarte incremental por cursor. O review seguinte preservou essas
correções e encontrou **5 P1 adicionais** na fronteira legada e no gate de
propriedade:

1. `selection_message_id` não era resolvido contra uma mensagem inbound real da
   mesma sessão;
2. a materialização legada herdava a ordem da projeção outbound, não da mensagem
   inbound selecionada;
3. um outbound genérico com `availability_search` copiado, mas sem
   `intent/template_name` explícitos, ainda podia virar prompt source;
4. duplicatas idênticas no mesmo batch recebiam ordinais e `EventID` distintos;
5. a propriedade de replay não incluía `MATERIALIZE B`, `REJECT A`,
   `INVALIDATE` e duplicata no mesmo batch.

O patch manual atual:

- resolve `selection_message_id` somente para uma mensagem `INBOUND` anterior à
  projeção, dentro do stream da sessão; identidade ausente, outbound ou
  causalmente posterior falha fechado;
- usa a ordem da mensagem inbound de seleção e exige que a lista-fonte seja
  anterior a essa seleção;
- exige declaração estrutural positiva de lista por
  `IntentAvailabilitySearch` ou `TemplateAvailabilityList`;
- normaliza e remove duplicatas semânticas antes de ordenar e atribuir ordinais;
- amplia a propriedade para os seis batches causais, cobrindo materializações
  A/B, rejeições A/B, invalidação, projeção e duplicata no mesmo batch, em todas
  as `720` permutações de aquisição, com reload e restart.

### Reconciliação da suíte em 2026-07-27

A primeira aplicação manual deixou `internal/chat`, `internal/chat -race` e
`./...` em RED. O patch canônico passava isoladamente; as falhas em cascata
vinham de fixtures que fabricavam autoridade em projeções `OUTBOUND`, sem
`INBOUND` real e sem lista-fonte estrutural.

A correção foi somente de testes:

- `persistedAvailabilitySelectionPayloadForTest` passou a exigir identidade e
  fonte explícitas para uma projeção autoritativa;
- `materializePersistedAvailabilitySelectionForTest` passou a usar apenas o
  replay dos eventos estruturados;
- os helpers canônicos criam prompt estrutural, seleção `INBOUND` real, evento
  explícito e projeção não autoritativa, inclusive no `fakeStore`;
- as fixtures legadas agora contêm a timeline completa e causal;
- o wrapper de `booking_create` não materializa mais seleção a partir de texto.

Não surgiu reprodução independente contra o repository real; nenhum código de
produção adicional foi necessário além do patch manual já existente.

Gates finais executados:

```text
PASS — matriz AvailabilitySelection replay/order/legacy/projection/invalidation, count=20 — 15.562s
PASS — provas funcionais 1–10, count=20 — 17.939s
PASS — go test -race -count=1 ./internal/chat — 23.570s
PASS — regressões H-012/document/lap-child/payment/human/out-of-turn — 1.682s
PASS — regressões cancel/passenger/availability — 3.832s
PASS SEM SKIP — PostgreSQL 16 real efêmero, duas pools, lock invertido e count=20 — 9.895s
PASS — go test -count=1 ./internal/chat — 4.243s
PASS — go test -count=1 ./... — internal/chat 4.862s; demais pacotes verdes
PASS — inventário de produção: 54 regexp.MustCompile
PASS — gofmt
PASS — git diff --check
```

Esse era o checkpoint pré-review final: o contêiner PostgreSQL efêmero havia
sido removido e ainda não existia declaração de review limpo. O estado está
preservado como histórico/superseded pelo resultado seguinte.

### Review final limpo de H-2026-07-22A — histórico

O review final considerou H-2026-07-22A **seguro para commit** e não encontrou
P1/P2. Confirmou os 10 controles, incluindo `count=20`, race, suítes amplas,
PostgreSQL **16.14** real sem `SKIP`, inventário de 54
`regexp.MustCompile`, `gofmt` e `git diff --check`.

Status naquele checkpoint:

```text
H-2026-07-22A — REVIEW FINAL SEM P1/P2 — SEGURO PARA COMMIT;
DEPLOY E SMOKE PENDENTES
```

Esse checkpoint foi seguido por commit, push e deploy. O smoke real posterior
ficou RED na transição availability → passageiros e é a evidência que abriu
H-2026-07-27A. O estado canônico atual está no início deste handoff.

## Correção atual de H-2026-07-27A

O status vigente está declarado exclusivamente no início deste handoff; as
rodadas abaixo preservam somente a cronologia superada.

O nono review encontrou seis falhas: precedência antes de reconciliar todos os
artefatos, facts legados unilaterais, fallback textual mais amplo que os
builders, validação da projeção bruta, fonte sem unicidade/causalidade global e
consumers relendo facts do lembrete.

Agora uma única reconciliação compara bilateralmente evento, facts,
seleção/snapshot, marcador, eventos passenger/pending e metadata de domínio.
Conflito ou artefato inválido não publica prompt/facts; availability inválida
sem domínio concorrente continua fail-closed. Facts `ABSENT_LEGACY` exigem duas
cópias válidas e idênticas. O fallback aceita somente as formas reais de lista
e EARLIEST dos builders.

A fonte out-of-turn é resolvida por ID globalmente único, anterior e causal.
Projeções são resolvidas primeiro e somente a mensagem efetiva fornece
`Presented`, facts e option count. Clarificações persistem bilateralmente
`active_prompt_kind` e `active_prompt_source_message_id`; o body do lembrete
mantém a continuidade, mas seu `tool_context` não substitui a fonte. Selection
state, booking draft e `booking_create` consomem a mesma autoridade.

RED/PASS reais:

```text
RED — sete testes dirigidos reproduziram os seis mecanismos antes do patch
PASS — testes dirigidos, count=20 — 0.381s
PASS — go test -race -count=1 ./internal/chat — 63.755s
PASS — H-012/document/lap-child/payment/human/out-of-turn — 3.608s
PASS — cancel/passenger/availability/review/delivery/booking — 8.801s
PASS — go test -count=1 ./internal/chat — 11.627s
PASS — go test -count=1 ./... — internal/chat 11.281s; demais pacotes verdes
PASS — inventário de produção = 54 regexp.MustCompile
PASS — gofmt -l internal/chat sem saída
PASS — git diff --check; git diff --cached --check
```

O host não possui Go/gofmt; os gates foram executados em `golang:1.23` via
Docker com caches temporários fora do worktree. Não houve B2, 3.6F-D,
migration, provider, PostgreSQL, regex, parser geral, commit, push, PR, merge,
deploy ou smoke. Não há review limpo nem segurança para commit; deploy e smoke
continuam pendentes.

O décimo review encontrou 3 P1 + 1 P2: candidatura availability apagada por
reconciliação inválida, `INVALID`/`UNDELIVERED` ainda aceitos no merge
canônico, enrichment de snapshot pelas mesmas classes e uma cópia stale do
plano na raiz. Os três REDs reproduziram esses mecanismos antes do patch.

A correção atual torna `authority.Class` o gate único: facts/selection
decodificados não são autoridade. `INVALID` entregue continua candidato,
forma barreira e fornece zero autoridade; `UNDELIVERED` fornece zero
autoridade e não forma barreira apenas por não estar entregue. Somente
`VALID_STRUCTURAL` e `ABSENT_LEGACY` entregue e confiável permanecem positivos.
Projeções passenger/payment/documento não duplicam artifacts availability; a
seleção vive no evento/estado canônico. Links out-of-turn persistem apenas a
referência bilateral à fonte, sem transformar o lembrete em autoridade
factual. A cópia stale foi removida e o plano em `plans/` é a fonte única.

Gates observados:

```text
PASS — 3 REDs pós-patch, count=20 — 0.398s
PASS — go test -race -count=1 ./internal/chat — 53.653s
PASS — suíte transversal completa — 7.292s
PASS — go test -count=1 ./internal/chat — 10.001s
PASS — go test -count=1 ./... — internal/chat 9.603s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário = 54 regexp.MustCompile
PASS — plano stale ausente; plano canônico presente
PASS — git diff --check
```

Essas evidências são locais e não substituem novo review. O hotfix permanece
em correção; não há segurança para commit, e deploy/smoke não foram executados.

O décimo primeiro review encontrou dois P1. O primeiro fazia o body canônico
de lista/EARLIEST perder candidatura quando coexistia com metadata ou evento
passenger/payment; agora a evidência do body é preservada e o conflito entregue
fica `INVALID`, com barreira e zero autoridade. O segundo bloqueava o
enrichment de uma projeção `ABSENT_LEGACY` entregue com seleção bilateral
completa e IDs confiáveis, mas sem `availability_search`; agora esse ramo usa
somente a `Selection.SelectedResult` reconciliada após o helper de confiança.
`INVALID` e `UNDELIVERED` continuam sem candidatos. A auditoria dos callers
também fechou source mismatch: projection, selection message, prompt source e
índice precisam coincidir exatamente com o evento materializado.

Evidências desta rodada em `golang:1.23` via Docker:

```text
PASS — REDs dirigidos pós-patch, count=20 — 0.472s
PASS — race internal/chat — 53.650s
PASS — regressões transversais — 7.962s
PASS — internal/chat — 7.838s
PASS — ./... — internal/chat 9.373s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — regexp.MustCompile = 54
PASS — plano stale ausente; plano canônico presente
PASS — git diff --check; git diff --cached --check
```

Não houve B2, 3.6F-D, parser, regex, migration, refactor amplo, commit, push,
PR, deploy ou smoke. Os gates locais não substituem novo review nem tornam o
hotfix seguro para commit.

O décimo segundo review confirmou o body canônico e encontrou dois P1 no
enrichment legado: o prompt source estrutural ainda fornecia campos ausentes,
e snapshots parcialmente conflitantes preservavam autoridade. O P2 apontou
status canônicos concorrentes.

Agora o ramo `ABSENT_LEGACY` resolve exclusivamente a projeção exata, usa o
source somente para validar causalidade/identidade e aceita campos apenas do
`Selection.SelectedResult` reconciliado. Todos os campos já preenchidos são
comparados; qualquer divergência mantém o snapshot original e remove
`MaterializesAuthority`, impedindo `BOOKABLE` e `booking_create`. Package e
outros campos opcionais só sobrevivem quando persistidos na própria seleção.
`VALID_STRUCTURAL`, `INVALID` e `UNDELIVERED` preservam seus gates.

Evidências finais em `golang:1.23` via Docker:

```text
PASS — REDs e controles dirigidos pós-patch, count=20 — 1.071s
PASS — race internal/chat — 49.026s
PASS — regressões transversais — 7.418s
PASS — internal/chat — 7.587s
PASS — ./... — internal/chat 9.007s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — regexp.MustCompile = 54
PASS — plano stale ausente; plano canônico presente
PASS — git diff --check; git diff --cached --check
```

Não houve B2, 3.6F-D, parser, regex, migration, refactor amplo, commit, push,
PR, deploy ou smoke. Não há review limpo nem segurança para commit;
deploy/smoke continuam pendentes.

A rodada acima está **SUPERADA** pelo décimo terceiro review. Esse review
confirmou os gates e encontrou 2 P1 + 1 P2: source ID vazio/ausente ainda era
inferido do histórico, zero/vazio persistido ainda era tratado como ausência,
e `AGENTS.md` duplicava status operacional volátil.

A correção atual exige `availability_prompt_source_message_id` não vazio no
`SelectedResult`, igualdade exata com o evento e resolução única/causal apenas
desse ID. O scan de fallback foi removido. Uma máscara interna preserva a
presença dos 20 campos usados pelo gate/merge; chave ausente pode ser
preenchida, mas zero, vazio ou qualquer valor presente divergente mantém o
snapshot e remove autoridade antes de `BOOKABLE`/`booking_create`. O tracker
voltou a ser a única fonte de status atual; as declarações operacionais das
rodadas anteriores estão superadas.

Evidências observadas em `golang:1.23` via Docker:

```text
PASS — REDs dirigidos pós-patch, count=20 — 0.884s
PASS — race internal/chat — 60.441s
PASS — regressões transversais — chat 11.605s; demais pacotes verdes
PASS — internal/chat — 11.477s
PASS — ./... — internal/chat 11.110s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — regexp.MustCompile = 54
PASS — git diff --check; git diff --cached --check
```

H-2026-07-27A permanece em correção após review. Não houve B2, 3.6F-D,
parser, regex, migration, refactor amplo, commit, push, PR, deploy ou smoke.
Não há review limpo nem segurança para commit; deploy/smoke continuam
pendentes no fluxo posterior autorizado.

A rodada do décimo terceiro review também está **SUPERADA** pelo review
atual. O estado vigente é o bloco canônico no topo deste handoff; contagens e
próximas ações anteriores são apenas históricas.

O review atual encontrou 3 P1 + 1 P2. A correção local reutiliza a barreira
`INVALID` central para impedir que um source anterior volte a materializar
autoridade; valida os tipos dos 20 campos antes de marcar presença; e carrega
a máscara no evento live até a serialização, preservando `price=0`,
`seats_available=0` e string vazia presente sem confundir com chave ausente.
O bloco operacional antigo do tracker foi marcado como histórico/superado.

Evidências observadas em `golang:1.23` via Docker:

```text
PASS — REDs dirigidos pós-patch, count=20 — internal/chat 0.105s
PASS — go test -race -count=1 ./internal/chat — 63.006s
PASS — regressões transversais — chat 11.888s; demais pacotes verdes
PASS — go test -count=1 ./internal/chat — 11.847s
PASS — go test -count=1 ./... — internal/chat 11.156s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — regexp.MustCompile = 54
PASS — git diff --check; git diff --cached --check
```

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Não há review
limpo nem segurança para commit. Não houve B2, 3.6F-D, parser, regex,
migration, refactor amplo, commit, push, PR, deploy ou smoke. Deploy/smoke
continuam pendentes no fluxo posterior autorizado. Próxima ação única: novo
`/review`.

A rodada do décimo quarto review está **SUPERADA** pelo review atual. O único
P1 encontrado mostrou que o bootstrap legado usava o índice da selection para
limitar também a busca de barreira, deixando de observar `INVALID` entregue
entre selection e projection.

A correção separa o limite causal do source do índice causal da
materialização. O source continua resolvido somente antes da selection, mas a
barreira central é consultada até a projection. Os controles comprovam
`INVALID` nos dois intervalos, source novo após barreira, barreira posterior
não retroativa, `UNDELIVERED` sem barreira e mensagens passenger/payment/document fora
do domínio availability.

Evidências observadas em `golang:1.23` via Docker:

```text
PASS — REDs dirigidos pós-patch, count=20 — internal/chat 1.279s
PASS — go test -race -count=1 ./internal/chat — 65.178s
PASS — regressões transversais — chat 13.341s; demais pacotes verdes
PASS — go test -count=1 ./internal/chat — 13.056s
PASS — go test -count=1 ./... — internal/chat 13.078s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — regexp.MustCompile = 54
PASS — git diff --check; git diff --cached --check
```

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Não há review
limpo nem segurança para commit. Não houve B2, 3.6F-D, parser, regex,
migration, refactor amplo, commit, push, PR, deploy ou smoke. Deploy/smoke
continuam pendentes no fluxo posterior autorizado. Próxima ação única: novo
`/review`.

## Bug H-2026-07-16B

```text
"eu e meus 2 filhos" → observado 2; esperado 3
"sim, o mais novo tem 4 anos" em ASK_CHILD_UNDER_5
→ estado não avançou e pergunta repetiu
```

Esse bug é determinístico e separado de TravelQueryMeaningV2.

## Próxima ação

Executar somente:

```text
/review dirigido às correções locais do review atual de H-2026-07-27A
```

H-2026-07-22A está corrigido e deployado, mas o smoke real reabriu o gate com
H-2026-07-27A. A correção do único achado do review mais recente e os gates
locais estão verdes, mas ainda não há novo review limpo, commit, push, deploy
ou novo smoke.
B2 permanece bloqueada até review, deploy e smoke verdes de H-2026-07-27A.
3.6F-D permanece bloqueada pelo fechamento integral de H-B.

## Arquivos que a nova sessão deve ler

1. `AGENTS.md`
2. `docs/EXECUTION_TRACKER.md`
3. `docs/SESSION_HANDOFF.md`
4. `docs/PRODUCTION_CONVERSATION_CASES.md`
5. `plans/00-plano-mestre-travel-semantic-v2.md`
6. plano do hotfix ativo em correção após review

Não carregar todos os planos no prompt operacional do Codex. Eles podem ficar versionados no repositório para consulta futura.

## Evidências que podem ser fornecidas sem risco

- SHA atual e branch;
- nomes de migrations;
- flags booleanas;
- resultados resumidos de queries;
- SQLSTATE sanitizado;
- logs sem query string, telefone, CPF, documento ou segredo;
- transcrições anonimizadas.

Nunca fornecer:

- `DATABASE_URL`;
- API keys;
- webhook secret;
- payload de documento;
- telefone/CPF reais;
- logs com query string sensível.

## Critério para pedir revisão ampla do GitHub

Não fazer auditoria completa do repositório em toda nova sessão.

Faça reconciliação ampla apenas quando:

- começa nova fase arquitetural;
- tracker e GitHub divergem;
- houve merge grande fora da sequência;
- código atual contradiz o plano;
- o próximo hotfix ainda não tem causa/localização delimitada.

No trabalho diário, peça análise dirigida aos arquivos e call paths do goal atual.

## Track independente SEC-2026-08-18 — Data API / grants / RLS

**Status:** **LOTE 1 OPERACIONALMENTE CONCLUÍDO; LOTE 2A / OPÇÃO B APROVADA;
4 P1 HISTÓRICOS PERMANECEM FECHADOS; PRÉ-CHECK READ-ONLY PASS; GATE EM
CORREÇÃO APÓS REVIEW; REGRESSÕES ANTERIORES PRESERVADAS; REDESENHO
ARQUITETURAL IMPLEMENTADO LOCALMENTE; P2 DOCUMENTAL FECHADO; POLICY FAIL-CLOSED
DE `statementUnknown` E P1 TRANSACIONAL CORRIGIDOS LOCALMENTE; MATRIZ DE
REGRESSÃO E SUÍTE GO COMPLETA PASS; AGUARDANDO NOVO `/review`; PRODUÇÃO E
SUCESSORES NÃO AUTORIZADOS.**

**Próxima ação operacional:** nenhuma autorizada. A próxima ação única é o
novo `/review` do gate redesenhado localmente. Não há migration executável,
SQL de produção ou autorização para o Lote 2B.
O plano canônico foi materializado em:

```text
plans/sec-2026-08-18-data-api-rls-hardening.md
```

Baseline do planejamento: `5dfd9a09a5c93ba5fb2d7d68d59d322ce0bf0436`,
contendo o checkpoint de segurança
`4eb543cb27cfa6527c0f225383d2f07fb9d38b97`.

O inventário read-only confirmou 29 tabelas, 15 routines e uma sequence em
`public`. `roles` e `pagarme_webhook_events` já estão endurecidas. Nenhuma
tabela ou routine DATA_API_REQUIRED foi comprovada.

Finding principal: nove routines são elegíveis como RPC e todas as 15 possuem
EXECUTE direto para PUBLIC, anon e authenticated, além de postgres e
service_role. Como várias tabelas relacionadas ainda possuem grants amplos,
há exposição de autoridade comprovada, sem evidência de exploração.

Permanecem bloqueados:

- `sheet_sync_queue`, `sheet_sync_queue_id_seq` e seis routines RPC/callable
  de sheet sync sem consumidor comprovado;
- `refresh_manifest_data()` e
  `refresh_manifest_data_for_booking(text)` sem caller comprovado;
- cinco definições de trigger functions de sheet sync ausentes do repo;
- `travel_authorizations`, `travel_authorization_check_items` e
  `trip_contractors` sem consumidor identificado.

O plano separa default ACL de tables, functions e sequences; RPC atual;
trigger functions; tabelas por domínio; e objetos UNKNOWN_BLOCKED. PUBLIC deve
ser revogado explicitamente nas functions, pois revogar apenas anon/auth não
remove autoridade herdada de PUBLIC. service_role, postgres, BYPASSRLS e FORCE
RLS não mudam neste track.

As correções foram incorporadas ao commit documental e validadas pelo review
final sem P0/P1/P2:

- o pós-check de produção do Lote 2B exclui qualquer write pelo fluxo `trips`.
  A prova positiva do consumidor backend continua obrigatória por caminho
  versionado previamente comprovado sem escrita nas tabelas-fonte de sheet
  sync, seguido por verificação read-only; sem caminho seguro, a prova fica
  bloqueada. DML em `trips` para essa prova é exclusivamente efêmero/de teste;
- o `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` continua impedindo que smokes
  em produção provoquem DML deliberado nas tabelas-fonte apenas para testar
  trigger/enqueue enquanto fila, sequence e routines de sheet sync continuam
  UNKNOWN. A prova comportamental correspondente é exclusivamente efêmera/de
  teste; o Lote 2B, os lotes 9, 11, 12, 13 e seus dependentes 10, 14 e 15
  herdam o guard;
- a correção P2 continua limitando o DoD à redução da autoridade Data API de
  anon/authenticated. A chave antiga do incidente permanece
  rotacionada/revogada conforme o checkpoint operacional, como controle
  histórico separado. Credenciais válidas comprometidas de `postgres` ou
  `service_role` ainda preservariam autoridade porque essas roles continuam
  privilegiadas/com `BYPASSRLS`; esse risco residual é aceito e fica em
  backlog separado.

O plano canônico materializado e revisado foi registrado no commit
`b8bfe9e9afd44ed5e1faf5aa1f5ee0653cd3de61`; o working tree estava limpo
imediatamente após esse commit. Não houve migration, SQL, alteração de
banco/ambiente, deploy ou smoke, e nenhum lote SQL foi iniciado ou autorizado.

### Rodada local do Lote 1 (2026-08-19)

Na branch `sec/data-api-rls-hardening`, baseline
`2385527bd0423d7eff356bc96f4d6612ac739e0c` e working tree inicial limpo, foi
criada somente a migration:

```text
apps/api/migrations/0022_harden_public_table_default_privileges.sql
```

Ela remove de futuras TABLES `postgres`-owned em `public` os sete default
privileges de anon/authenticated. Tabelas existentes, ACLs atuais, RLS,
policies, FORCE, functions, sequences, postgres, service_role e objetos
UNKNOWN_BLOCKED permanecem fora da mudança.

PostgreSQL 16 efêmero comprovou RED com ALL7 antes da migration; PASS no
`pg_default_acl` e em nova tabela; preservação de postgres/service_role,
tabela preexistente, RLS/policy, functions e sequences; e rollback exato com
restauração dos sete privilégios para anon/authenticated. `git diff --check`
passou. Nenhum teste de aplicação foi necessário para esta migration isolada.

Ao encerrar a implementação local, o pré-check read-only e a aplicação no
banco real permaneciam pendentes. Não houve conexão ou SQL em produção,
deploy ou smoke, e nenhum sucessor foi autorizado.

### Estado pós-review/pós-merge do Lote 1 (2026-08-19)

O review final terminou sem P0/P1/P2 e declarou o working tree seguro para
commit e o Lote 1 seguro para pré-check/aplicação. A PR #69, com head
`b7bd633108f0477b4bc5285e74d7c07c0502fb65`, foi mergeada na `main` pelo
commit `dfcfac5b4a7fead7ef2dd3575948fb7a7c50fb6c`, integrando a migration
`0022_harden_public_table_default_privileges.sql`.

O Lote 1 não foi aplicado no banco de produção. Não houve deploy ou smoke, e
nenhum sucessor está autorizado. O próximo gate operacional é somente o
pré-check READ-ONLY no banco real definido pelo plano. A aplicação da
migration permanece uma rodada separada, condicionada ao pré-check verde e a
autorização explícita posterior; este handoff não autoriza SQL nem Lote 2A.

Validação desta reconciliação: `git diff --check` PASS; somente tracker e
handoff estão modificados. Nenhum teste de aplicação ou produção foi
executado, pois a mudança é somente documental.

O P2 documental da reconciliação pós-merge foi corrigido e validado por review
sem P0/P1/P2. O working tree documental foi declarado seguro para commit.

A migration `0022_harden_public_table_default_privileges.sql` continua
integrada na `main`, mas ainda não foi aplicada no banco de produção.

A única próxima ação operacional é o pré-check READ-ONLY no banco real.
Qualquer aplicação mutável da migration depende de pré-check verde e nova
autorização explícita. Nenhum sucessor, deploy ou smoke está autorizado.

### Fechamento operacional do Lote 1 (2026-08-20)

Este checkpoint supera apenas o estado operacional pendente registrado na
reconciliação pós-merge acima; o histórico da implementação e das tentativas
permanece preservado.

Referência reconciliada:

```text
branch: sec/data-api-rls-hardening
HEAD: 83af9b7548000715e8431c2382d694fa6c48a44e
working tree inicial: limpo
```

O pré-check READ-ONLY em produção passou em PostgreSQL 15.8 e confirmou 29
tabelas, 15 routines, uma sequence, zero policies e zero objetos em
publication.

A primeira tentativa de executar a migration 0022 abortou antes de `COMMIT`
por erro exclusivamente no wrapper do pós-check. Ela não é contabilizada como
aplicação. Uma verificação READ-ONLY posterior comprovou rollback integral e
ausência de mudança persistida.

A aplicação final usou o blob canônico
`b7a724eaf9065eb772e8ebde2091c53db9980200` e concluiu:

```text
PRECHECK_IN_TRANSACTION_PASS
2 ALTER DEFAULT PRIVILEGES
POSTCHECK_IN_TRANSACTION_PASS
COMMIT
MIGRATION_0022_COMMIT_DONE
```

O pós-check persistido confirmou zero default privileges de TABLE para anon e
authenticated, `ALL7` preservado para postgres e service_role e default
privileges de FUNCTION e SEQUENCE inalterados. Nenhum rollback foi necessário
após a aplicação final.

Os smokes somente leitura passaram sem violar o
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`:

```text
anon: 401 / SQLSTATE 42501
authenticated: 403 / SQLSTATE 42501
/health: 200
/ready: 200
GET /routes?limit=1&offset=0: 200
LOTE1_POST_CHANGE_SMOKE_PASS
```

O review final posterior à aplicação terminou sem P0/P1/P2. Com pré-check,
aplicação, pós-check, smokes e review verdes, o Lote 1 está aplicado e
operacionalmente concluído. Nenhum teste adicional em produção é necessário
para este lote.

Arquivos alterados nesta reconciliação:

```text
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
```

Validação documental: `git diff --check` PASS; `git status --short` e
`git diff --stat` confirmam somente os dois documentos; o diff completo foi
revisado; plano, migration e track funcional permanecem sem alteração. Não
houve teste de aplicação nesta rodada exclusivamente documental. O working
tree está pronto para `/review`, sem commit.

Este fechamento documental não executou SQL, deploy, smoke ou rollback e não
autoriza o Lote 2A nem qualquer outro sucessor. Qualquer próximo lote exige
novo `/goal`, pré-check próprio e autorização explícita.

### Lote 2A — correção dos 4 P1 e bloqueio public-only (2026-08-21)

O review rejeitou a implementação local anterior com quatro P1:

1. revoke global de PUBLIC sem autorização cross-schema;
2. grant global incondicional capaz de criar autoridade nova para service_role;
3. inventário cross-schema insuficiente, tratado indevidamente como reconciliado;
4. rollback com GRANT/REVOKE incondicionais, não derivado de snapshot completo.

A prova em PostgreSQL 16.15 efêmero permanece válida somente como evidência de
semântica: defaults schema-locais são somados ao global; portanto,
`IN SCHEMA public REVOKE EXECUTE ... FROM PUBLIC` não remove o EXECUTE global
nativo. O único revoke eficaz é global e altera futuras functions
`postgres`-owned fora de `public`. Não há mecanismo
`ALTER DEFAULT PRIVILEGES` public-only que cumpra o objetivo sem esse efeito.

Tratamento local dos P1:

- a migration executável 0023 foi removida; não existe efeito cross-schema no
  working tree;
- nenhum grant global a service_role permanece;
- schemas, deployers e consumidores fora de `public` permanecem
  `UNKNOWN_BLOCKED`; ausência de DDL/caller no repo não é inventário real;
- qualquer proposta futura deverá capturar individualmente PUBLIC global,
  service_role global e PUBLIC/anon/authenticated schema-locais. O rollback
  deverá restaurar cada valor exatamente conforme o snapshot, sem comando
  incondicional que possa criar ou remover autoridade.

Sob a autorização vigente, restrita a `public`, o Lote 2A está bloqueado. Não
foi criado event trigger, wrapper, workaround ou arquitetura alternativa. A
liberação exige nova autorização explícita para alcance cross-schema ou outro
mecanismo canônico aprovado em `/goal` separado.

Arquivos alterados após a correção:

```text
plans/sec-2026-08-18-data-api-rls-hardening.md
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
```

`apps/api/migrations/0023_harden_public_function_default_privileges.sql` não
existe mais. Migration 0022, Lote 1, routines existentes, TABLE/SEQUENCE, RLS,
código, infra e track funcional permanecem intactos. Não houve SQL em
produção, deploy, smoke, rollback de produção, commit ou push.

Validação local: a prova PostgreSQL 16.15 anterior foi mantida somente para a
semântica public-only/global; nenhum novo SQL foi necessário. `git diff
--check` passou; staged e untracked estão vazios; `git status --short` e o diff
completo mostram somente plano SEC, tracker e handoff. O diretório de
migrations termina em 0022 e não contém migration executável do Lote 2A.
Nenhum teste de aplicação foi necessário para esta correção documental.

Checkpoint canônico após o novo review:

```text
review: sem P0/P1/P2
4 P1 anteriores fechados: SIM
Lote 2A: UNKNOWN_BLOCKED no escopo public-only
migration/SQL executável do Lote 2A: NÃO
Lote 2A seguro para produção: NÃO
Lote 2B autorizado: NÃO
SQL/deploy/smoke autorizado: NÃO
```

Esta reconciliação substitui somente o estado volátil **EM CORREÇÃO /
AGUARDANDO REVIEW** pelo checkpoint limpo acima e preserva integralmente o
histórico e o tratamento dos quatro P1. `UNKNOWN_BLOCKED` não equivale à
conclusão operacional do Lote 2A. Qualquer retomada exige nova decisão e
autorização explícita para alcance cross-schema ou solução canônica distinta
em `/goal` separado; nenhum sucessor é liberado automaticamente. O working
tree documental fica pronto para `/review` final, sem commit.

### Lote 2A — opção B aprovada e pré-check READ-ONLY PASS (2026-08-24)

A decisão arquitetural posterior aprovou substituir a tentativa de default
ACL public-only pela opção B: política explícita, atômica e por assinatura
para novas FUNCTIONs `public` criadas por DDL versionado. O default ACL global
de FUNCTION permanece inalterado e fora da solução. Não haverá revoke
cross-schema nem grant global a service_role.

Invariantes aprovados para o futuro gate:

- toda nova FUNCTION coberta usa `public` explicitamente;
- criação e hardening ocorrem na mesma transação;
- PUBLIC, anon e authenticated perdem EXECUTE pela assinatura exata;
- a política de service_role é declarada por assinatura;
- postgres permanece efetivo como owner;
- routines existentes e schemas não públicos permanecem inalterados;
- DDL manual/externo não é declarado protegido e permanece risco residual
  `UNKNOWN_BLOCKED`.

O pré-check operacional autorizado passou em PostgreSQL 15.8 com
`transaction_read_only=on`. Evidência sanitizada:

```text
remote main: b559b326b0cd27ad0b2f71ed3572a14b6b7673c8
checkout servidor/API OCI: 4eb543cb27cfa6527c0f225383d2f07fb9d38b97
PGRST_DB_SCHEMAS: public,storage,graphql_public
schemas não sistêmicos: 14
CREATE para postgres: extensions, gsheets_raw, public, realtime,
  supabase_functions
default nativo FUNCTION/postgres: EXECUTE para PUBLIC
default ACL explícito FUNCTION/postgres: public, storage, supabase_functions
public routines: 15, todas owner postgres
postgres-owned routines fora de public: 0
public routines com owner diferente de postgres: 0
public: 29 tables / 15 routines / 1 sequence / 0 policies
default TABLE public: endurecido pelo Lote 1
default SEQUENCE: inalterado
fim da transação: ROLLBACK
GRANT/REVOKE/DDL/DML executado: NÃO
```

Os SQLs históricos untracked no checkout operacional permanecem intocados,
fora do versionamento e fora do working tree local desta rodada.

O gate foi implementado em `apps/api/internal/migrationguard`, usando somente
a biblioteca padrão Go e o `go test -count=1 ./...` já executado pelo CI. Ele
inspeciona somente migrations posteriores à 0022, associa cada nova FUNCTION
à assinatura exata e exige framing explícito `BEGIN` antes do DDL e
`COMMIT`/`END` somente depois de todos os REVOKEs de
PUBLIC/anon/authenticated e do GRANT direto sem grant option a service_role.
Overload sem ACL individual, schema ausente ou diferente de `public`,
`CREATE OR REPLACE FUNCTION`, ACL divergente e `ALTER DEFAULT PRIVILEGES` de
FUNCTION falham fechados. Migrations históricas permanecem fora do
enforcement e o diretório atual, encerrado em 0022, passa.

RED anterior ao patch: sete fixtures temporárias `9001`–`9007` cobriram
FUNCTION `public` sem ACL, schema ausente, hardening incompleto, ausência de
service_role, overload parcialmente endurecido, schema não público e default
ACL global. A suíte completa terminou verde indevidamente, provando que o
estado anterior não rejeitava nenhuma dessas violações; as fixtures foram
removidas antes do patch.

PASS local:

```text
go test -count=1 -v ./internal/migrationguard -> PASS
go test -count=1 ./... -> PASS
fixture válida schema public + assinatura exata + três revokes + service_role -> PASS
migrations <= 0022 fora do enforcement -> PASS
diretório atual de migrations -> PASS
```

O review seguinte encontrou 3 P1 + 1 P2: variantes ACL como `GRANT ALL`
ignoradas; `ALTER FUNCTION/ROUTINE` capaz de mudar owner/schema ignorado;
ausência de transação explícita e de suporte a `END`; nomes numéricos
malformados como `0023.sql` e `0023-add_function.sql` ignorados. O review
declarou o gate incorreto, os REDs insuficientes e o working tree inseguro para
commit; os quatro P1 históricos permaneceram fechados.

RED dirigido confirmou os quatro bypasses antes da correção. O patch agora:

- aceita somente ACL exata por assinatura: REVOKE de
  PUBLIC/anon/authenticated e GRANT direto a service_role;
- rejeita todas as demais ACLs FUNCTION/ROUTINE, modifiers, mutations
  `ALTER FUNCTION/ROUTINE`, mudanças da role de execução, `REASSIGN OWNED` e
  membership no mesmo arquivo;
- exige `BEGIN` antes de cada criação e `COMMIT` ou `END` após todos os ACLs,
  rejeitando ausência, fechamento intermediário ou framing alternativo;
- rejeita `.sql` numericamente iniciado acima de 0022 que não siga
  `NNNN_description.sql`, preservando arquivos legítimos não numéricos.

PASS após a correção:

```text
3 P1 + 1 P2 dirigidos -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS
go test -count=1 ./... -> FULL_SUITE_PASS
gofmt -l -> vazio
```

A prova foi repetida em PostgreSQL 15.19 por causa da mudança transacional. O
RED sem hardening herdou EXECUTE de PUBLIC e terminou em ROLLBACK. A fixture
válida usou `BEGIN` + DDL/ACL exatos + `END`; após o commit,
PUBLIC/anon/authenticated estavam sem EXECUTE, service_role mantinha somente o
grant direto, postgres permanecia owner e functions preexistentes, schema
`private` e `pg_default_acl` estavam idênticos. O container efêmero foi
removido.

O review atual encontrou 2 P1 adicionais: o splitter descartava SQL dinâmico
executável em `DO $tag$...$tag$`, e `ALTER GROUP ... ADD/DROP USER` escapava
do bloqueio de membership. Os quatro P1 arquiteturais históricos e os 3 P1 +
1 P2 da rodada anterior permaneceram fechados.

REDs dirigidos reproduziram os dois bypasses antes do patch. A correção local
rejeita qualquer comando executável `DO` sob enforcement, sem tentar analisar
PL/pgSQL, e trata `ALTER GROUP` como alias de mutação de role já proibida.
Comentários e corpos dollar-quoted de `CREATE FUNCTION` continuam ignorados.

```text
2 P1 dirigidos -> PASS
3 P1 + 1 P2 da rodada anterior -> PASS na suíte migrationguard
go test -count=1 -v ./internal/migrationguard -> PASS
go test -count=1 ./... -> PASS em container Go 1.22
gofmt -l -> vazio
```

A prova PostgreSQL 15.19 não foi repetida porque o patch altera somente a
detecção estática, sem mudar ACL ou framing transacional.

O review seguinte encontrou 3 P1: membership `GRANT/REVOKE` standalone era
descartado sem FUNCTION; uma FUNCTION com SQL dinâmico podia ser executada por
SELECT/CALL; e DROP de FUNCTION/ROUTINE/PROCEDURE era ignorado. Os 3 P1 + 1 P2
anteriores e os quatro P1 arquiteturais permaneceram fechados; as correções
dos 2 P1 da rodada imediatamente anterior foram preservadas.

REDs dirigidos reproduziram GRANT e REVOKE standalone, SELECT malicioso, CALL
e as três variantes de DROP. A correção emite finding incondicional para
membership e DROP pós-0022 e rejeita SELECT com forma de chamada ou CALL em
migration que cria FUNCTION. SELECT literal, agrupamento aritmético,
comentários e corpos dollar-quoted permanecem aceitos.

```text
3 P1 dirigidos -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS offline em Go 1.22
regressões anteriores -> PASS na suíte migrationguard
gofmt -l -> vazio
go test -count=1 ./... -> BLOQUEADO antes da compilação: host sem Go e
  container offline sem módulos em cache/rede
```

A prova PostgreSQL 15.19 não foi repetida porque a política ACL/transacional
não mudou.

O review atual preservou as regressões anteriores e encontrou 2 P1: a role
citada `"on"` era confundida com a keyword `ON`; e chamadas por VALUES, INSERT
ou outras formas executáveis escapavam da detecção limitada a SELECT/CALL.

REDs reproduziram GRANT/REVOKE para `"on"`, VALUES e INSERT...VALUES. Uma
regressão com `"x--y"` também comprovou que o splitter não inicia comentário
dentro de identificador citado. A correção classifica membership pela ordem
estrutural de `ON` e `TO/FROM`, preserva aspas duplas no splitter e troca a
enumeração de chamadas por uma allowlist: em arquivo que cria FUNCTION, apenas
BEGIN, CREATE FUNCTION canônico, ACL exata e COMMIT/END são aceitos.

```text
2 P1 dirigidos -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS offline em Go 1.22
regressões anteriores -> PASS na suíte migrationguard
gofmt -l -> vazio
go test -count=1 ./... -> BLOQUEADO antes da compilação: host sem Go e
  container offline sem módulos em cache/rede
```

A prova PostgreSQL 15.19 não foi repetida porque ACL e framing transacional
não mudaram.

O review atual encontrou 1 P1 + 1 P2: `E'...'` com aspa escapada podia fundir
INSERT ao CREATE FUNCTION, e prefixo numérico maior que `int` era ignorado
quando `Atoi` falhava. REDs reproduziram ambos com zero findings.

O lexer agora reconhece escape strings em boundary lexical e consome
backslash escapes/aspas duplicadas corretamente. Nomes `.sql` numeric-looking
fora de `NNNN_description.sql` falham sem conversão numérica; somente o nome
canônico de quatro dígitos chega a `Atoi` e pode ser classificado como
histórico até 0022.

```text
1 P1 + 1 P2 dirigidos -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS offline em Go 1.22
regressões anteriores -> PASS na suíte migrationguard
gofmt -l -> vazio
go test -count=1 ./... -> BLOQUEADO antes da compilação: host sem Go e
  container offline sem módulos em cache/rede
```

A prova PostgreSQL 15.19 não foi repetida porque ACL e framing transacional
não mudaram.

Não foi criada migration 0023 nem alterada 0022, routines existentes,
TABLE/SEQUENCE, RLS/policies/FORCE, roles/BYPASSRLS, CI, infra ou track
funcional. Nenhum SQL de produção, deploy, smoke, commit ou push foi
executado. DDL manual/externo continua fora da cobertura e permanece
`UNKNOWN_BLOCKED`.

**Estado atual:** Lote 2A em correção após review; 1 P1 + 1 P2 atuais
corrigidos localmente, regressões anteriores preservadas e aguardando novo
`/review`. A suíte Go completa permanece bloqueada pelo ambiente e não é
declarada PASS.
Não declarar conclusão antes de review sem P0/P1/P2. Lote 2B, produção e todos
os sucessores permanecem não autorizados.

O track funcional mantém a próxima ação declarada no topo deste handoff.

### Checkpoint do redesenho arquitetural do migration guard (2026-08-25)

O review posterior fechou o 1 P1 + 1 P2 anteriores e encontrou um novo P1:
`$tag$` dentro de identificador não citado fundia statements no splitter. O
RED com `cover$tag$()`, `cover()` e `decoy$tag$()` falhou antes do patch com
zero findings, confirmando que somente `cover()` podia ser endurecida sem o
gate perceber as outras duas FUNCTIONs.

O gate foi redesenhado em lexer único, parser estrutural sobre tokens e policy
state-machine allowlist, somente com Go stdlib. Dollar quotes agora exigem
boundary lexical válida; `$` continua parte de identificador e `$1` permanece
parâmetro. Strings simples/escape, quoted identifiers, comentários de linha e
blocos aninhados são tokenizados sem apagar estrutura; constructs não
terminados e NUL falham fechado. Em migration com FUNCTION, somente BEGIN,
blocos CREATE/REVOKE/GRANT canônicos adjacentes e COMMIT/END são aceitos.

Evidência:

```text
RED pré-patch -> FAIL com zero findings
RED pós-patch -> PASS e sete statements preservados
PostgreSQL 15 efêmero -> três assinaturas distintas; somente cover() endurecida
go test -count=1 -v ./internal/migrationguard -> PASS
fuzz FuzzLexer por 30s -> PASS; 1.629.683 execuções
go test -count=1 ./... -> PASS
gofmt -l -> vazio
```

O container PostgreSQL foi removido. A suíte completa baixou somente módulos
já fixados no `go.mod` dentro de container descartável; nenhuma dependência
foi adicionada.

Migration 0022 permanece intacta e 0023 ausente. Não houve mudança em
migrations reais, routines, runtime, CI, infra, RLS, roles ou default ACL;
nenhum SQL de produção, deploy, smoke, commit ou push foi executado. DDL
manual/externo permanece `UNKNOWN_BLOCKED`.

**Estado atual:** Lote 2A em correção após review; redesenho implementado
localmente e aguardando novo `/review`. Não declarar conclusão ou segurança
para commit antes de review sem P0/P1/P2. Lote 2B e produção permanecem
bloqueados.

### Correção dos três P1 pós-redesenho do migration guard (2026-08-25)

O review encontrou três P1 no gate local: U+0301 era boundary incorreta antes
de `$tag$`; `double precision` colidia com `doubleprecision` na assinatura; e
`DROP OWNED BY postgres CASCADE` passava sem finding. REDs dirigidos
reproduziram os três bypasses com zero findings antes da correção.

A correção usa a classe multibyte byte-a-byte do PostgreSQL 15 para
identificadores não citados, preserva boundaries word/number na identidade
canônica dos tipos e classifica `DROP OWNED` junto das mutações proibidas de
autoridade/objetos preexistentes. Compound types, schema-qualified, arrays,
typmods e nomes opcionais possuem casos positivos exatos.

```text
3 P1 dirigidos -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
fuzz FuzzLexer por 30s -> PASS; 1.311.947 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
```

A prova PostgreSQL 15.19 efêmera confirmou que os identificadores com U+0301
e `$tag$` são válidos e que os três statements da reprodução permanecem
distintos. A transação terminou em `ROLLBACK` e o container foi removido.

Migration 0022 permanece intacta e 0023 ausente. Não houve alteração em
migrations reais, dependências, CI, runtime, infra, RLS, roles ou default ACL;
nenhum SQL de produção, deploy, smoke, commit ou push foi executado.

**Estado registrado naquela rodada:** os três P1 pós-redesenho estavam
corrigidos localmente e aguardavam novo `/review`. O review seguinte confirmou
esses três fechados e abriu o P1 de EXTENSION + P2 documental registrado
abaixo.

### Bloqueio de DDL de EXTENSION e correção do resumo SEC (2026-08-26)

O review confirmou fechados os três P1 pós-redesenho e encontrou 1 P1 + 1 P2.
O gate aceitava `CREATE EXTENSION pgcrypto WITH SCHEMA public;` com zero
findings quando não havia `CREATE FUNCTION`, enquanto o resumo canônico ainda
citava somente o antigo P1 de dollar quote.

O parser estrutural agora classifica `CREATE`, `ALTER` e `DROP EXTENSION` como
mutações proibidas que exigem autorização separada, sem busca textual no SQL
bruto nem inspeção dos scripts da extensão. O RED de CREATE e os casos de
ALTER/DROP passam; a fixture histórica confirma que migrations até 0022
continuam fora do enforcement.

```text
RED CREATE EXTENSION pré-patch -> FAIL com zero findings
CREATE/ALTER/DROP EXTENSION pós-patch -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
fuzz FuzzLexer por 30s -> PASS; 2.031.915 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
```

PostgreSQL 15.19 efêmero confirmou 36 routines de `pgcrypto` em `public`,
todas executáveis por `anon` via PUBLIC. A transação terminou em `ROLLBACK` e
o container foi removido. Migration 0022 permanece intacta e 0023 ausente;
nenhum SQL de produção, deploy, smoke, commit ou push foi executado.

**Estado registrado naquela rodada:** o P1 de EXTENSION + P2 documental
estavam corrigidos localmente. O review seguinte confirmou o P2 documental,
mas encontrou o bypass lexical por tab vertical registrado abaixo.

### Correção do whitespace PostgreSQL com tab vertical (2026-08-26)

O review encontrou um P1: `\v`, aceito como whitespace pelo PostgreSQL 15,
virava token no lexer e impedia a classificação estrutural. CREATE/ALTER/DROP
EXTENSION, CREATE/DROP FUNCTION, DROP/REASSIGN OWNED e ALTER GROUP retornaram
zero findings nas reproduções pré-patch.

`isSQLSpace` agora reconhece espaço, `\t`, `\n`, `\r`, `\f` e `\v`. A correção
fica integralmente no lexer e preserva parser estrutural e policy. Testes
dirigidos cobrem separadores isolados/combinados e contextos opacos de string,
quoted identifier, comentário e dollar quote.

```text
8 REDs com tab vertical pré-patch -> FAIL; zero findings em todos
8 comandos + matriz de whitespace/contextos pós-patch -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
fuzz FuzzLexer por 30s -> PASS; 1.371.301 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
```

PostgreSQL 15.19 não foi repetido porque a aceitação de `\v` já estava
comprovada e não houve mudança em SQL, ACL ou framing. Migration 0022
permanece intacta e 0023 ausente; nenhum SQL de produção, deploy, smoke,
commit ou push foi executado.

**Estado registrado naquela rodada:** o P1 de whitespace estava corrigido
localmente e aguardava novo `/review`. O review seguinte encontrou o P1 de
`statementUnknown` registrado abaixo.

### Fail-closed global de `statementUnknown` (2026-08-28)

O review seguinte encontrou um P1 na policy: migrations posteriores à 0022
sem CREATE FUNCTION aceitavam todo statement não modelado. O caso concreto
`CREATE AGGREGATE` podia materializar uma routine com EXECUTE herdado de PUBLIC
e retornar zero findings.

REDs pré-patch comprovaram zero findings para CREATE/ALTER/DROP AGGREGATE,
ALTER TABLE ... ADD COLUMN, DROP SCHEMA ... CASCADE, CREATE TRIGGER, INSERT,
as seis formas de whitespace PostgreSQL e dois unknowns no mesmo arquivo.
`globalFindings` agora emite um finding por `statementUnknown`, sempre com
filename e byte offset. Não houve mudança no lexer ou parser nem denylist de
AGGREGATE.

```text
REDs obrigatórios pré-patch -> FAIL; zero findings em todos
REDs pós-patch + migrations vazias/só comentários -> PASS
FUNCTION canônica + regressões históricas -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
fuzz FuzzLexer por 30s -> PASS; 2.630.215 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
git diff --check / git diff --cached --check -> PASS
```

PostgreSQL 15.19 efêmero confirmou `prokind='a'` e EXECUTE herdado de PUBLIC
no AGGREGATE descartável. O `ROLLBACK` removeu aggregate e role, e o container
foi removido. Migration 0022 permanece intacta e 0023 ausente; nenhum SQL de
produção, deploy, smoke, commit ou push foi executado.

**Estado registrado naquela rodada:** o fail-closed de `statementUnknown`
estava implementado localmente e aguardava novo `/review`. O review seguinte
confirmou esse P1 fechado e encontrou o P1 transacional registrado abaixo.

### Correção de transações fora da FUNCTION canônica (2026-08-28)

O review encontrou um P1 remanescente: migrations posteriores à 0022 sem
CREATE FUNCTION aceitavam `BEGIN`, `COMMIT`, `END`, `START TRANSACTION`,
`ROLLBACK` e `ABORT` com zero findings. Os pares `BEGIN; COMMIT;` e `BEGIN;
END;` também passavam, embora nenhum statement transacional estivesse sendo
consumido pela state machine de FUNCTION.

REDs pré-patch reproduziram todos os zeros e provaram que, ao lado de unknown,
somente o unknown era reportado. `globalFindings` agora rejeita cada kind
transacional quando a state machine de FUNCTION não está ativa, sempre com
filename e byte offset. Com CREATE FUNCTION, a state machine existente mantém
somente BEGIN inicial e COMMIT/END final; framing incompleto ou extra,
`START TRANSACTION`, `ROLLBACK` e `ABORT` continuam rejeitados.

```text
REDs transacionais pré-patch -> FAIL; zero findings nos casos sem FUNCTION
REDs pós-patch + adjacência com unknown -> PASS
FUNCTION canônica + framing inválido -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
fuzz FuzzLexer por 30s -> PASS; 1.806.552 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
git diff --check / git diff --cached --check -> PASS
```

Lexer/parser, migrations, módulos, CI, runtime e infra permaneceram intactos.
PostgreSQL efêmero não foi repetido porque o patch é somente de policy; teste
em produção nesta rodada: NÃO. Migration 0022 permanece intacta, 0023 ausente
e não houve SQL, deploy, smoke, commit ou push.

**Estado vigente:** Lote 2A **EM CORREÇÃO APÓS REVIEW**; P1 transacional
corrigido localmente e aguardando novo `/review`. O fail-closed de
`statementUnknown` permanece preservado; Lote 2A não está concluído, e Lote 2B
e produção permanecem bloqueados.
