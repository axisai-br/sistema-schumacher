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
H-2026-07-22A — EM CORREÇÃO APÓS REVIEW — 4 P1 DE REPLAY CANÔNICO
H-2026-07-16B2 bloqueada até review, deploy e smoke verdes do hotfix
3.6F-D bloqueada por H-B
```

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

O review atual encontrou **4 P1 DE REPLAY CANÔNICO**, corrigidos localmente e
ainda aguardando novo review:

- seleção legada sem `selection_message_id` ainda podia receber um ID inventado
  da projeção;
- uma seleção/projeção anterior podia ser aceita como prompt source quando a
  lista original não estava disponível;
- evento durável atrasado podia ser ignorado depois do avanço do cursor, fazendo
  live e restart produzirem estados diferentes;
- o teste PostgreSQL obrigatório ainda executava `SKIP` quando a URL não estava
  configurada.

Agora cada aplicação, dentro do lock da sessão, persiste o evento atual no
inbound, carrega todos os eventos estruturados da sessão sem `body` e sem
`LIMIT 50`, incorpora marker/boundary, hidrata a ordem somente de
`received_at`, `created_at`, `message_id` e ordinal, ordena e reduz desde o
estado zero. A projeção completa só é persistida depois de validada.
`LastAppliedEventOrder` é recalculado; nunca decide se um evento participa.

Legado sem ID explícito permanece `NONE`. Prompt source exige exatamente uma
lista anterior confiável, estrutural, completa e compatível; seleção,
passageiros, documentos, pagamento e continuação não são candidatos. História
limitada e envelope atual não concedem autoridade a booking draft ou
`booking_create`: ambos consomem somente a projeção reconstruída e validada.

A propriedade fake percorre todas as seis permutações de materialização antiga
`A`, rejeição nova `B` de outra fonte e projeção não autoritativa, reaplica
duplicatas e reinicia após cada permutação. O estado live, o reload e o
bootstrap são idênticos; `A` permanece `BOOKABLE` e a rejeição de `B` fica
registrada.

O modo `CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1` falha sem URL. Com
PostgreSQL 16 real efêmero, duas pools e lock invertido, o teste obrigatório
passou `count=20` e comprovou igualdade live/restart. O `count=20` dirigido e
race também passaram; `go test -count=1 ./internal/chat`, `go test -count=1
./...`, regexp=54 e `git diff --check` passaram. Não houve commit, push, deploy
ou smoke e não há declaração de review limpo.

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
plans/h-2026-07-22a-fresh-session-passenger-gate.md
```

A matriz local dos 4 P1 atuais de replay canônico inclui `count=20` dirigido,
race e PostgreSQL 16 real obrigatório com duas pools, lock invertido e
live/restart idênticos. As suítes amplas, o inventário de 54
`regexp.MustCompile` em produção e `git diff --check` passaram. O status permanece
`H-2026-07-22A — EM CORREÇÃO APÓS REVIEW — 4 P1 DE REPLAY CANÔNICO`: os P1 da
rodada anterior e os demais P1 históricos permanecem corrigidos, sem declaração
de review limpo. A próxima ação é um novo `/review` dirigido aos 4 P1 de replay
canônico, sem commit, push, deploy ou smoke. B2 só pode ser reavaliada depois de
review, deploy e smoke verdes. Só depois do fechamento integral de H-B pode-se
reavaliar 3.6F-D.

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
