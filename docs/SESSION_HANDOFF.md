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
3 P1 + 1 P2 DE ENTREGA TEMPORAL E PROVENIÊNCIA
H-2026-07-16B2 bloqueada por H-2026-07-27A e pelo gate operacional de B1
3.6F-D bloqueada por H-B
```

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

O status vigente é **H-2026-07-27A — EM CORREÇÃO APÓS REVIEW — 3 P1 + 1 P2
DE ENTREGA TEMPORAL E PROVENIÊNCIA**. A correção local está verde, mas não há
declaração de review limpo nem segurança para commit. B2 e 3.6F-D continuam
bloqueadas.

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
novo /review dirigido aos 3 P1 + 1 P2 de entrega temporal e proveniência de H-2026-07-27A
```

H-2026-07-22A está corrigido e deployado, mas o smoke real reabriu o gate com
H-2026-07-27A. A correção dos 3 P1 + 1 P2 e os testes locais estão verdes; ainda
não há novo review limpo, commit, push, deploy ou novo smoke.
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
