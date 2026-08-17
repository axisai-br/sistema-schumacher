# H-2026-07-27A — Correções de reconciliação estrutural

## Resumo

Este plano é cumulativo e preserva as rodadas corretivas anteriores como
histórico concluído. A próxima execução deve tratar somente a fase pendente
descrita ao final deste documento.

### Estado das fases

| Fase | Escopo | Estado |
|---|---|---|
| 1 | facts bilaterais integrais, identidade entregue nos readers e unicidade/causalidade do draft estrutural | **CONCLUÍDA EM CÓDIGO E TESTES LOCAIS; review posterior encontrou novas lacunas** |
| 2 | filtros temporais completos, projeções legadas irresolvidas e identidade completa no índice entregue | **CONCLUÍDA EM CÓDIGO E TESTES LOCAIS; review posterior encontrou quatro novas lacunas** |
| 3 | semântica temporal, propagação de validade, escopo da barreira e precedência do scan ativo | **CONCLUÍDA EM CÓDIGO E TESTES LOCAIS; review posterior encontrou 3 P1 + 1 P2** |
| 4 | candidatura inválida, consumo por classe de autoridade e remoção da cópia stale do plano | **CONCLUÍDA EM CÓDIGO E TESTES LOCAIS; review posterior encontrou dois P1** |
| 5 | body canônico e enrichment legado confiável | **HISTÓRICO SUPERADO PELA FASE 6** |
| 6 | projeção exclusiva e compatibilidade integral | **HISTÓRICO; review posterior encontrou 2 P1 + 1 P2** |
| 7 | source explícito, presença persistida e status operacional canônico | **HISTÓRICO; review posterior encontrou 3 P1 + 1 P2** |
| 8 | barreira `INVALID`, tipos estritos e presença na serialização live | **HISTÓRICO; review posterior encontrou 1 P1** |
| 9 | limite da barreira até a projeção/materialização | **EXECUTADA LOCALMENTE; AGUARDANDO NOVO REVIEW** |

## Fase 1 — Histórico concluído

A fase anterior corrigiu exclusivamente os três findings daquele review:

1. facts divergentes ou unilaterais entre `Payload` e `NormalizedPayload`;
2. perda do ID entregue ao reutilizar o índice da projeção;
3. `draft_message_id` duplicado ou causalmente inválido.

Não há mudança de API pública, schema ou formato persistido.

### Causa raiz e invariantes

#### Causa raiz

- `availabilityPromptPresentedContextV1` valida cada contexto somente contra os
  IDs/data do evento, mas não exige duas cópias factuais nem igualdade integral
  entre elas.
- Os finders retornam corretamente o índice causal da projeção, porém
  `availabilityPromptSourceMessageIDAtHistoryIndex` lê a projeção bruta — agora
  `INVALID` — em vez da mensagem resolvida.
- `resolveDeliveredPromptSourceMessageWithIndex` encerra no primeiro ID
  encontrado antes da projeção e não verifica duplicidade nem timestamps
  causais.

#### Invariantes obrigatórios

- Um evento estrutural só possui facts reconciliáveis quando ambas as cópias
  contêm `availability_search`, decodificam completamente e produzem
  `AvailabilitySearchResult` integralmente iguais.
- A igualdade cobre filtro, preço, pacote, rota, horários, stops e todos os
  resultados, não apenas os IDs presentes no evento.
- Facts unilaterais ou divergentes tornam a mensagem/draft `INVALID`; apenas
  ausência bilateral do evento mantém o fallback legado.
- O índice retornado para uma projeção resolvida continua sendo o índice da
  entrega, preservando sua posição temporal.
- Qualquer reader que derive identidade desse índice deve resolver/classificar
  a mensagem e retornar o `source_message_id` entregue.
- `draft_message_id` deve identificar exatamente um draft no prefixo anterior
  à projeção.
- O draft deve estar antes da projeção no slice e, quando ambos possuem
  timestamp canônico, não pode ser temporalmente posterior. Timestamps iguais
  são aceitos; timestamp ausente usa a ordem do slice.
- Duplicidade, ausência ou causalidade inválida deixam a projeção estrutural
  irresolvida e mantêm a barreira temporal.

### Alterações concluídas

#### Reconciliação factual

Em `availability_prompt_event_v1.go`:

- tornar `availabilityPromptPresentedContextV1` estrito sem alterar o helper
  legado: exigir exatamente dois contextos factuais, na ordem `Payload` e
  `NormalizedPayload`, e igualdade estrutural integral antes de construir
  `Presented`;
- preservar `availabilityPromptRawContextsV1` para `ABSENT_LEGACY`;
- aplicar a regra à classificação direta, criação do evento outbound e
  reconciliação do draft de uma projeção.

Em `agent.go`, persistir os facts validados de `availability_search` também em
`NormalizedPayload` quando o writer cria um draft estrutural. Isso mantém os
writers canônicos conformes ao contrato bilateral, sem fallback unilateral no
reader.

#### Identidade resolvida

Em `booking_draft_context.go`:

- manter mensagem resolvida acompanhada do índice causal da projeção;
- fazer `availabilityPromptSourceMessageIDAtHistoryIndex` classificar/resolver
  o item do índice e obter a identidade por
  `availabilityPromptSourceMessageIDFromMessage`;
- retornar vazio se a resolução falhar, sem alterar as assinaturas dos finders
  nem dos readers em `availability_draft.go` e `tool_router.go`.

#### Unicidade e causalidade do draft

Em `active_prompt_context.go`, fortalecer o resolver existente:

- validar `deliveredIndex` e a identidade da mensagem nesse índice;
- coletar todos os matches exatos de `draft_message_id` no prefixo anterior;
- exigir exatamente um match, mesmo quando somente um duplicado pareça
  confiável;
- rejeitar draft posterior no slice ou posterior pelo timestamp canônico
  quando ambos os timestamps forem conhecidos;
- aplicar depois as validações existentes de modo, status, revisão, evento,
  equivalência e facts;
- reutilizar `canonicalAvailabilityHistoryMessageTime`, sem criar resolver
  paralelo.

Uma resolução recusada continua fazendo
`deliveredInvalidAvailabilityPromptBarrierAtV1` manter a barreira estrutural.

### Arquivos e call sites

Produção:

- `availability_prompt_event_v1.go`: igualdade bilateral dos facts e barreira;
- `agent.go`: writer bilateral dos facts estruturais;
- `active_prompt_context.go`: unicidade e causalidade do resolver;
- `booking_draft_context.go`: identidade entregue history-aware;
- `availability_draft.go` e `tool_router.go`: consumers corrigidos sem mudança
  de assinatura;
- `availability_invalidation_history.go`: timestamp canônico reutilizado.

Testes:

- `availability_prompt_event_v1_test.go`;
- `active_prompt_context_test.go`;
- `availability_draft_test.go`;
- `tool_router_test.go`.

### REDs e controles positivos

#### Facts bilaterais

Adicionar `TestAvailabilityPromptEventV1RequiresBilateralIdenticalFacts`,
table-driven, cobrindo facts em apenas uma cópia e divergência de preço,
pacote, rota, horário, stops, filtro e resultado não apresentado. Draft
divergente usado por BOT ou review aprovado deve permanecer irresolvido, criar
barreira e não recuperar prompt anterior.

Verificar `INVALID`, `Presented=nil`, zero merge de facts, zero bootstrap
`BOOKABLE` e zero autoridade para `booking_create`. O controle positivo mantém
cópias completas/idênticas como `VALID_STRUCTURAL` com os facts exatos.

#### Identidade da projeção

Adicionar:

- `TestAvailabilityDraftResolvedProjectionKeepsDeliveredPromptIdentity`;
- `TestParseAvailabilityDateSelectionInputResolvedProjectionKeepsDeliveredPromptIdentity`.

Cobrir BOT e `APPROVED_AS_IS`: finder preserva o índice da projeção e o helper
retorna o ID entregue; rejeições por índice e data vinculadas a esse ID
continuam bloqueando. Rejeição de outro prompt não bloqueia; sem rejeição,
opção e data permanecem selecionáveis.

#### Draft ambíguo ou não causal

Adicionar
`TestResolveDeliveredPromptSourceMessageV1RejectsAmbiguousOrCausallyInvalidDraft`,
table-driven para BOT e review aprovado, cobrindo ausência, duplicatas com
facts iguais/diferentes, duplicata inválida, draft posterior no slice e draft
anterior no slice com timestamp posterior.

Toda resolução recusada entregue mantém a barreira e impede prompt histórico.
Controles positivos: draft único causal resolve; timestamps iguais são
aceitos; timestamp ausente usa a ordem do slice; `UNDELIVERED` não cria
barreira.

### Riscos e compatibilidade

- Mensagens estruturais históricas com facts em somente uma cópia passam a
  falhar fechado intencionalmente e não caem no legado.
- IDs duplicados podem ocultar uma projeção legítima em histórico corrompido,
  preferindo segurança à escolha arbitrária.
- Timestamps inconsistentes podem invalidar um draft; igualdade é aceita e
  ausência recorre à ordem do histórico.
- A resolução adicional do helper de identidade repete uma busca no histórico,
  mas evita mudar interfaces ou criar cache/estado paralelo.
- Não incluir B2, 3.6F-D, os três P1 anteriores, parser/regex, banco,
  migrations, deploy ou smoke.
- Não atualizar tracker/handoff nem declarar review limpo.

### Validação concluída

Registrar primeiro o RED:

```bash
cd apps/api
go test -count=1 ./internal/chat -run '^(TestAvailabilityPromptEventV1RequiresBilateralIdenticalFacts|TestResolveDeliveredPromptSourceMessageV1RejectsAmbiguousOrCausallyInvalidDraft|TestAvailabilityDraftResolvedProjectionKeepsDeliveredPromptIdentity|TestParseAvailabilityDateSelectionInputResolvedProjectionKeepsDeliveredPromptIdentity)$'
```

Após o patch:

```bash
go test -count=20 ./internal/chat -run '^(TestAvailabilityPromptEventV1|TestResolveDeliveredPromptSourceMessageV1|TestAvailabilityDraftResolvedProjectionKeepsDeliveredPromptIdentity|TestParseAvailabilityDateSelectionInputResolvedProjectionKeepsDeliveredPromptIdentity)'
go test -race -count=1 ./internal/chat
go test -count=1 ./internal/chat -run '(Availability|Passenger|Booking|Human|Cancel|Review|AutoSend|Delivery)'
go test -count=1 ./internal/chat
go test -count=1 ./...
gofmt -w internal/chat/agent.go internal/chat/availability_prompt_event_v1.go internal/chat/availability_prompt_event_v1_test.go internal/chat/active_prompt_context.go internal/chat/active_prompt_context_test.go internal/chat/booking_draft_context.go internal/chat/availability_draft_test.go internal/chat/tool_router_test.go
gofmt -l internal/chat/agent.go internal/chat/availability_prompt_event_v1.go internal/chat/availability_prompt_event_v1_test.go internal/chat/active_prompt_context.go internal/chat/active_prompt_context_test.go internal/chat/booking_draft_context.go internal/chat/availability_draft_test.go internal/chat/tool_router_test.go
rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l
git diff --check
git status --short
```

O inventário deve permanecer em `54`. Se `go`/`gofmt` não estiverem
disponíveis localmente, executar os mesmos comandos em `golang:1.23` com
`apps/api` montado, sem modificar dependências nem arquivos fora do escopo.

Parar após os gates para novo `/review`, sem commit ou push.

## Fase 2 — Histórico concluído: filtros temporais, legado fail-closed e identidade completa

Esta fase foi concluída em código e testes locais. O review posterior preservou
seus controles e encontrou os quatro findings tratados exclusivamente na fase
3. O conteúdo abaixo permanece como registro canônico da rodada concluída.

### Objetivo e limites

Corrigir exclusivamente os três findings do review mais recente:

1. reconciliação incompleta dos filtros persistidos de availability;
2. projeções legadas entregues que permanecem irresolvidas sem formar barreira;
3. validação incompleta da identidade da mensagem em `deliveredIndex`.

Esta fase não inclui B2, 3.6F-D, PostgreSQL, status desconhecido do provider,
divergência de `mode`, mudança de API pública, schema, migration, parser/regex,
deploy ou smoke. A divergência de `mode` já conhecida permanece explicitamente
fora do escopo desta rodada.

### Causas raiz

#### Finding 1 — reconciliação incompleta de filtros

`availabilityPromptPresentedContextV1` compara os dois
`AvailabilitySearchResult` produzidos por `availabilityPromptRawContextsV1`,
mas esse helper delega o decode a `parseAvailabilityContextPayload`.
Atualmente o parser reconstrói `origin`, `destination`, `package_name`, `qtd`,
`trip_date` e resultados, porém ignora `date_from` e `date_to`, embora o writer
`buildAvailabilityToolResponsePayload` persista ambos. Assim, duas cópias
persistidas que diferem somente no intervalo convergem para a mesma projeção
parcial, passam no `reflect.DeepEqual` e podem ser classificadas como
`VALID_STRUCTURAL`. O `Presented` resultante também perde o intervalo.

#### Finding 2 — projeções legadas irresolvidas

`deliveredInvalidAvailabilityPromptBarrierAtV1` tenta resolver uma projeção
entregue somente quando ela possui `availability_prompt_event_v1`. Uma
`BOT_AUTO_REPLY` ou `DRAFT_REVIEW/APPROVED_AS_IS` bilateralmente sem evento
pode, portanto, permanecer `ABSENT_LEGACY` mesmo quando `draft_message_id` está
ausente, duplicado, incorreto ou não identifica um draft causalmente válido.
Sem resolução e sem barreira, os finders podem continuar varrendo o histórico
e recuperar facts de um draft ambíguo ou de um prompt válido anterior.

#### Finding 3 — identidade incompleta em `deliveredIndex`

`resolveDeliveredPromptSourceMessageWithIndex` valida a entrada do índice
somente por igualdade de `ID`. Depois dessa guarda, tipo, metadata de entrega,
timestamp, evento e `draft_message_id` são lidos do argumento externo
`delivered`. Em históricos com IDs duplicados ou cópias obsoletas, o índice
pode apontar para outra mensagem e ainda assim receber autoridade e posição
causal que não pertencem ao item persistido naquela posição.

### Invariantes obrigatórios

#### Reconciliação de filtros

- O decode factual usado pela classificação estrutural deve reconstruir todos
  os filtros efetivamente persistidos que influenciam seleção ou apresentação:
  `origin`, `destination`, `package_name`, `qtd`, `trip_date`, `date_from` e
  `date_to`. O `Limit` reconstruído deve continuar coerente com a quantidade de
  resultados persistidos e com a posterior projeção de `Presented`.
- `date_from` e `date_to` devem ser decodificados segundo a mesma semântica
  canônica de data usada pelo writer; valor persistido inválido não pode ser
  silenciosamente convertido em ausência numa cópia estrutural.
- Divergência somente em `date_from` ou somente em `date_to` entre `Payload` e
  `NormalizedPayload` resulta em `INVALID`, com `Presented=nil`.
- Intervalos bilaterais idênticos permanecem no `AvailabilitySearchResult`
  reconciliado e no `Presented`; a redução dos resultados apresentados pode
  ajustar apenas `Filter.Limit`, não apagar o intervalo.
- A fase não altera a política para divergência de `mode`.

#### Projeções legadas

- Toda `BOT_AUTO_REPLY` ou `DRAFT_REVIEW/APPROVED_AS_IS` entregue deve passar
  pelo resolver do draft vinculado, com ou sem evento estrutural.
- `draft_message_id` ausente, inconsistente entre cópias, incorreto, duplicado
  ou irresolvível torna a projeção entregue `INVALID`.
- Uma projeção entregue irresolvida forma barreira temporal e encerra a busca;
  nenhum finder pode recuperar facts de draft ambíguo nem de prompt anterior.
- `ABSENT_LEGACY` só é permitido quando a projeção é resolvida para exatamente
  um draft anterior, causalmente válido e confiável, e projeção e draft estão
  bilateralmente sem `availability_prompt_event_v1`.
- O fallback legado continua disponível para esse caso resolvido e somente
  para ele; a resolução não pode escolher arbitrariamente entre IDs iguais.
- Projeções `UNDELIVERED` continuam invisíveis e não criam barreira.

#### Identidade entregue

- `deliveredIndex` deve identificar a mesma mensagem entregue fornecida ao
  resolver, não apenas uma mensagem com o mesmo `ID`.
- A validação deve comparar os campos persistidos relevantes à autoridade:
  direção/tipo e kind, status e metadata canônica de entrega, timestamps
  canônicos, evento estrutural e `draft_message_id`, incluindo consistência
  bilateral das metadata usadas pelo contrato.
- Divergência de timestamp, metadata, evento ou `draft_message_id` entre o
  item do índice e o argumento entregue deve falhar fechado.
- Após validar o índice, a resolução deve usar uma única mensagem canônica —
  preferencialmente `history[deliveredIndex]` — e não continuar consumindo
  metadata, timestamp ou evento do argumento externo.
- Mensagens diferentes com o mesmo ID nunca compartilham autoridade por
  acidente; o controle positivo com identidade completa continua resolvendo.

### Tabela de classificação esperada

| Cenário | Entrega | Evento na projeção/draft | Resolução do draft | Classificação/resultado | Barreira |
|---|---|---|---|---|---|
| estrutural com `date_from` divergente | entregue | bilateral presente | n/a | `INVALID`, `Presented=nil` | sim |
| estrutural com `date_to` divergente | entregue | bilateral presente | n/a | `INVALID`, `Presented=nil` | sim |
| estrutural com intervalo idêntico | entregue | bilateral presente | n/a | `VALID_STRUCTURAL`, intervalo preservado | não |
| projeção legada com draft ausente/incorreto | entregue | bilateralmente ausente | falha | `INVALID` | sim |
| projeção legada com ID de draft duplicado | entregue | bilateralmente ausente | ambígua | `INVALID` | sim |
| projeção legada com exatamente um draft causal válido | entregue | bilateralmente ausente nos dois | única e válida | `ABSENT_LEGACY` resolvido | não |
| projeção irresolvida após prompt válido anterior | entregue | bilateralmente ausente | falha | `INVALID`; sem recuperação histórica | sim |
| projeção irresolvida | não entregue | qualquer | falha | `UNDELIVERED` | não |
| índice contém outra mensagem com o mesmo ID | entregue | qualquer | identidade diverge | falha fechada/`INVALID` no reader | sim quando candidata entregue |
| índice e argumento têm identidade persistida completa | entregue | conforme contrato | única e válida | resolução normal | não |

Em todo caso `INVALID`, exigir também: zero `Presented`, zero merge de facts,
zero bootstrap ou manutenção de `BOOKABLE` e zero `BookingCreateInput`.

### Funções e call sites afetados

Produção, com mudança mínima esperada:

- `booking_create_router.go`
  - `parseAvailabilityContextPayload`: decodificar integralmente os filtros
    persistidos, em especial `date_from` e `date_to`, sem alterar o contrato do
    payload;
  - `classifiedAvailabilityPromptMessageAtV1`,
    `findLatestAvailabilityContextWithSourceBefore` e
    `latestVisibleAvailabilitySelectionContextWithSource`: preservar o
    fail-closed e a barreira ao consumir projeções.
- `availability_prompt_event_v1.go`
  - `availabilityPromptRawContextsV1` e
    `availabilityPromptPresentedContextV1`: comparar facts completos e
    preservar o intervalo em `Presented`;
  - `classifyAvailabilityPromptCandidateV1`: classificar divergências de
    filtro como `INVALID`;
  - `deliveredInvalidAvailabilityPromptBarrierAtV1`: resolver também
    projeções legadas sem evento e transformar falha entregue em barreira.
- `active_prompt_context.go`
  - `resolveDeliveredPromptSourceMessageWithIndex`: validar identidade completa
    no índice, adotar o item persistido como mensagem canônica e exigir
    resolução única do draft também no caminho legado;
  - `deliveredPromptSourceReferenceV1`, `isTrustedDeliveredPromptOutboundV1` e
    `effectiveDeliveredPromptMessageV1`: ajustar somente se necessário para
    manter a mesma política compartilhada, sem resolver `mode` divergente.
- `booking_draft_context.go`
  - `availabilityPromptSourceMessageIDAtHistoryIndex` e
    `availabilityPromptSourceMessageIDBefore`: continuar consumindo apenas
    mensagem classificada/resolvida e respeitar a nova barreira.
- `availability_draft.go`
  - `availabilityDraftHasSelectedTrip`: não aceitar seleção recuperada através
    de projeção irresolvida.
- `tool_router.go`
  - `parseAvailabilityDateSelectionInput`: não reconstruir seleção/data após a
    barreira e preservar o intervalo do contexto válido.
- `booking_create_router.go` e seus callers de materialização/bootstrap:
  confirmar que `findLatestAvailabilityContextWithSource` não produz
  `BookingCreateInput` quando a candidata mais nova é inválida.

Testes a alterar, sem criar suíte paralela:

- `availability_prompt_event_v1_test.go`;
- `active_prompt_context_test.go`;
- quando necessário para provar consumers, `availability_draft_test.go` e
  `tool_router_test.go`.

### Estratégia mínima de correção

1. Estender o parser factual existente para decodificar os filtros omitidos,
   usando um helper pequeno e estrito para datas persistidas se necessário;
   manter a comparação bilateral já centralizada em
   `availabilityPromptPresentedContextV1`.
2. Generalizar a decisão de barreira para toda projeção entregue: tentar o
   mesmo resolver antes de permitir `ABSENT_LEGACY`, independentemente da
   presença do evento. Falha de resolução entregue retorna barreira; sucesso
   legado só continua se draft e projeção estiverem bilateralmente sem evento.
3. Fortalecer a guarda inicial do resolver com uma comparação de identidade
   persistida relevante à autoridade. Depois da guarda, substituir o argumento
   externo pelo item de `history[deliveredIndex]` e executar toda a resolução a
   partir dele.
4. Reutilizar os finders, classificador, timestamp canônico e resolver atuais;
   não criar um segundo caminho de recuperação legado nem alterar assinaturas
   públicas.

### REDs table-driven antes do patch

#### Filtros temporais

Estender `TestAvailabilityPromptEventV1RequiresBilateralIdenticalFacts` com
casos table-driven:

- somente `date_from` diverge;
- somente `date_to` diverge;
- intervalo bilateral idêntico é aceito e permanece em
  `authority.Presented.Filter.DateFrom/DateTo`.

Nos dois casos divergentes, provar `INVALID`, `Presented=nil`, zero merge de
facts, zero `BOOKABLE` e zero `BookingCreateInput`.

#### Projeção legada irresolvida

Adicionar ou estender um teste table-driven para os dois modos
`BOT_AUTO_REPLY` e `DRAFT_REVIEW/APPROVED_AS_IS`, cobrindo:

- projeção entregue com draft ausente;
- `draft_message_id` duplicado;
- `draft_message_id` incorreto/irresolvível;
- exatamente um draft legado anterior, causalmente válido e bilateralmente sem
  evento como controle positivo;
- prompt válido anterior seguido por projeção entregue irresolvida;
- prova de que `deliveredInvalidAvailabilityPromptBarrierAtV1`,
  `findLatestAvailabilityContextWithSource`, bootstrap e booking não atravessam
  a barreira;
- a mesma projeção `UNDELIVERED` permanece sem barreira.

#### Identidade completa do índice

Estender `TestResolveDeliveredPromptSourceMessageV1RejectsAmbiguousOrCausallyInvalidDraft`
ou adicionar teste table-driven dedicado em `active_prompt_context_test.go`:

- `history[deliveredIndex]` contém outra mensagem com o mesmo ID;
- divergência isolada de timestamp;
- divergência isolada de metadata canônica de entrega;
- divergência isolada de evento;
- divergência isolada de `draft_message_id`;
- controle positivo com identidade persistida completa.

Cada divergência deve provar falha de resolução e, quando a mensagem indexada é
uma candidata entregue, fail-closed sem recuperação histórica. O teste não deve
introduzir nem corrigir divergência de `mode`.

### Controles positivos e regressões

- intervalo bilateral idêntico continua `VALID_STRUCTURAL` e é apresentado
  integralmente;
- exactly one draft legado causalmente válido resolve para a identidade
  entregue e mantém o fallback legado existente;
- projeção estrutural válida continua resolvendo para o draft correto;
- identidade completa entre argumento e índice mantém o índice causal da
  entrega;
- timestamps iguais continuam aceitos; timestamps ausentes seguem a ordem do
  slice conforme contrato anterior;
- `UNDELIVERED` não cria barreira;
- seleção materializada válida continua podendo chegar a `BOOKABLE` e
  `BookingCreateInput` sem alterar idempotência.

### Riscos de regressão

- Históricos legados incompletos podem deixar de recuperar contexto quando a
  projeção entregue não possui vínculo unívoco; isso é fail-closed intencional.
- Decode temporal estrito pode invalidar payload histórico malformado que antes
  perdia silenciosamente o intervalo; cobrir ausência bilateral legítima e
  valores válidos para evitar falsos negativos.
- Comparação de identidade excessivamente ampla pode rejeitar cópias
  semanticamente idênticas por campos transitórios ou maps com representação
  equivalente. Limitar a comparação aos campos persistidos que definem
  autoridade e normalizar somente conforme o contrato já existente.
- Comparação excessivamente estreita mantém a confusão entre mensagens com ID
  duplicado. Os REDs isolados por campo devem fixar a fronteira mínima.
- A nova barreira pode afetar active prompt, bootstrap, seleção por data,
  booking draft e booking create; todos os readers devem ser exercitados no
  mesmo histórico inválido.
- Não alterar a semântica de `mode`, status desconhecido do provider ou regras
  de entrega fora dos status já reconhecidos nesta fase.

### Comandos de validação

Registrar primeiro os REDs dirigidos, antes do patch de produção:

```bash
cd apps/api
go test -count=1 ./internal/chat -run '^(TestAvailabilityPromptEventV1RequiresBilateralIdenticalFacts|TestResolveDeliveredPromptSourceMessageV1RejectsAmbiguousOrCausallyInvalidDraft|TestAvailabilityPromptEventV1LegacyProjectionRequiresResolvableDraft|TestResolveDeliveredPromptSourceMessageV1RequiresCompleteDeliveredIndexIdentity)$'
```

Após o patch, executar o gate focado com repetição e depois a validação padrão
de chat:

```bash
cd apps/api
go test -count=20 ./internal/chat -run '^(TestAvailabilityPromptEventV1RequiresBilateralIdenticalFacts|TestResolveDeliveredPromptSourceMessageV1RejectsAmbiguousOrCausallyInvalidDraft|TestAvailabilityPromptEventV1LegacyProjectionRequiresResolvableDraft|TestResolveDeliveredPromptSourceMessageV1RequiresCompleteDeliveredIndexIdentity)$'
go test -race -count=1 ./internal/chat
go test -count=1 ./internal/chat -run '(Availability|Passenger|Booking|Human|Cancel|Review|AutoSend|Delivery)'
go test -count=1 ./internal/chat
go test -count=1 ./...
gofmt -w internal/chat/availability_prompt_event_v1.go internal/chat/availability_prompt_event_v1_test.go internal/chat/active_prompt_context.go internal/chat/active_prompt_context_test.go internal/chat/booking_create_router.go internal/chat/booking_draft_context.go internal/chat/availability_draft.go internal/chat/availability_draft_test.go internal/chat/tool_router.go internal/chat/tool_router_test.go
gofmt -l internal/chat/availability_prompt_event_v1.go internal/chat/availability_prompt_event_v1_test.go internal/chat/active_prompt_context.go internal/chat/active_prompt_context_test.go internal/chat/booking_create_router.go internal/chat/booking_draft_context.go internal/chat/availability_draft.go internal/chat/availability_draft_test.go internal/chat/tool_router.go internal/chat/tool_router_test.go
rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l
git diff --check
git status --short
```

O inventário de produção deve permanecer em `54`. Não executar PostgreSQL,
deploy ou smoke nesta fase. Parar após os gates e solicitar novo `/review`, sem
commit ou push. Não atualizar tracker/handoff durante esta etapa de
planejamento.

## Fase 3 — Histórico concluído: validade temporal, barreira contextual e scan fail-closed

Esta fase foi concluída em código e testes locais. O review posterior preservou
seus controles e encontrou exclusivamente os 3 P1 + 1 P2 tratados na fase 4.
O conteúdo abaixo permanece como registro canônico da rodada concluída.

### Objetivo e limites

Corrigir exclusivamente os quatro findings do último review:

1. validar a semântica do intervalo, e não apenas cada data isoladamente;
2. propagar `valid=false` por todos os wrappers e consumidores de facts;
3. restringir a barreira a projeções que realmente pertençam ao fluxo de
   availability;
4. resolver a projeção válida antes da decisão de barreira e interromper o
   scan quando uma projeção de availability entregue for irresolvível.

Esta fase não altera API pública, schema, formato persistido, parser de
linguagem, regex, idempotência ou regras externas aos quatro findings. Não
inclui refactors de conveniência, deploy, smoke, commit ou push.

### Causas raiz

#### Finding 1 — datas parseáveis, mas intervalo semanticamente inválido

`parseAvailabilityContextPayloadWithValidityV1` valida `trip_date`,
`date_from` e `date_to` individualmente. Quando os dois limites existem, não
há validação de ordem; por isso um intervalo bilateralmente idêntico com
`date_from > date_to` ainda produz `valid=true`. Como
`availabilityPromptPresentedContextV1` compara as duas projeções já
decodificadas, o mesmo intervalo invertido nas duas cópias passa pelo
`reflect.DeepEqual` e pode chegar a `VALID_STRUCTURAL` e `Presented`.

Além disso, o contrato ainda não documenta a diferença entre chave ausente,
intervalo parcial válido, valor presente vazio/malformado e limite igual ao
outro. Sem essa fronteira, readers e writers podem representar a mesma
intenção temporal de formas distintas.

#### Finding 2 — validade descartada por wrappers e readers legados

`parseAvailabilityContextPayload` chama o parser com validade e descarta o
booleano. Seus call sites atuais são:

- `trustedAvailabilityContextFromPromptMessage` em `service.go`;
- `visibleAvailabilitySelectionContextFromHistoryMessage` em
  `booking_create_router.go`;
- `availabilityOptionCountFromMessageToolContext` em `interpreter.go`.

Há ainda um descarte equivalente em `availabilityPromptRawContextsV1`: ele
recebe `valid=false`, faz `continue` e transforma payload presente inválido em
zero contextos. Nos caminhos legados, zero contextos pode ser interpretado
como facts ausentes, mantendo `ABSENT_LEGACY` ou permitindo que outro reader
recupere body, contagem ou prompt anterior.

Assim, payload bilateralmente presente, porém malformado, pode desaparecer em
vez de produzir falha fechada. A perda alcança contagem de opções, contexto
visível/trusted, seleção, materialização, bootstrap e booking.

#### Finding 3 — modo de review usado como sinal de availability

`messageMayCarryAvailabilityPromptV1` considera `mode=DRAFT_REVIEW` suficiente
para marcar a mensagem como candidata. Em consequência, uma pergunta apenas
de passageiros enviada por `DRAFT_REVIEW/APPROVED_AS_IS`, sem evento, facts,
opções, snapshot ou semântica de availability, pode formar barreira só porque
o `draft_message_id` não resolve.

O modo de entrega e a ação de review provam como a mensagem foi produzida, não
qual domínio funcional seu conteúdo representa.

#### Finding 4 — scan ativo pode publicar projeção irresolvida

`latestReliableAssistantMessageWithIndex` tenta resolver a projeção, mas,
quando não há evento estrutural e a autoridade base é `ABSENT_LEGACY`, ainda
pode retornar `withoutPromptToolContext(message)`. Uma
`BOT_AUTO_REPLY` entregue com corpo de opções, sem evento e com draft
inexistente pode, portanto, ser publicada como mensagem assistente legada. O
scan passa a inferir pelo body ou pode continuar até contexto anterior, em vez
de respeitar a barreira temporal.

Nos finders reversos, a ordem `barreira antes de resolução/classificação`
também precisa ser explicitamente reconciliada: projeção válida deve resolver;
projeção de availability entregue e irresolvível deve encerrar; mensagem não
relacionada deve permanecer invisível para esse gate.

### Invariantes obrigatórios

#### Contrato temporal canônico

- `trip_date`, `date_from` e `date_to`, quando presentes, devem ser strings no
  formato estrito `YYYY-MM-DD`, decodificadas como data civil canônica em UTC,
  sem preservar horário ou offset.
- Chave ausente representa ausência legítima e produz ponteiro `nil`. Writer
  canônico omite a chave quando o ponteiro é `nil`; não persiste string vazia,
  `null`, timestamp ou formato alternativo.
- `date_from` presente e `date_to` ausente é intervalo aberto à direita válido,
  desde que `date_from` seja válido.
- `date_to` presente e `date_from` ausente é intervalo aberto à esquerda
  válido, desde que `date_to` seja válido.
- Os dois limites ausentes representam ausência de intervalo e são válidos.
- Os dois limites presentes são válidos somente quando
  `date_from <= date_to`; datas iguais representam intervalo de um único dia e
  são válidas.
- Chave presente com valor vazio, `null`, tipo não string ou data malformada é
  inválida; não equivale a chave ausente.
- Intervalo invertido é inválido mesmo quando as duas cópias persistem
  exatamente o mesmo valor.
- Presença, ausência e valor canônico de cada campo temporal devem coincidir
  entre `Payload` e `NormalizedPayload`. Divergência bilateral em qualquer
  campo, ou somente uma cópia inválida, resulta `INVALID`.
- `Presented` preserva `TripDate`, `DateFrom` e `DateTo`; somente `Filter.Limit`
  pode ser reduzido para a quantidade efetivamente apresentada.

#### Propagação de validade

- Nenhum wrapper pode converter `(resultado, false)` em resultado utilizável,
  zero value confiável ou contexto ausente.
- O caminho de autoridade deve usar um único decode que preserve pelo menos
  três estados: payload ausente, payload presente válido e payload presente
  inválido.
- `availabilityPromptRawContextsV1` deve preservar presença/invalidade das duas
  cópias. Payload de availability presente e inválido não pode virar slice
  vazio indistinguível de ausência.
- `ABSENT_LEGACY` significa ausência bilateral do evento estrutural, mas não
  autoriza facts inválidos. Se existir artefato factual de availability e seu
  decode falhar, a candidata é `INVALID`, não um legado sem contexto.
- `trustedAvailabilityContextFromPromptMessage`, contexto visível, contagem de
  opções, finders, bootstrap e enriquecimento de snapshot devem falhar fechado
  no mesmo payload inválido.
- Payload inválido produz zero `Presented`, zero contexto trusted/visible,
  zero `MATERIALIZE`, zero `SELECTION_MATERIALIZED`, zero `BOOKABLE`, zero
  `BookingCreateInput` e nenhuma manutenção de autoridade canônica anterior
  através da barreira aplicável.

#### Candidatura mínima à barreira de availability

Uma mensagem entregue só pertence a este gate quando ela própria, ou sua
projeção validamente resolvida, contém ao menos um artefato de availability:

- chave `availability_prompt_event_v1` em qualquer cópia, inclusive inválida;
- chave `tool_context.availability_search` em qualquer cópia, inclusive
  payload malformado;
- `selected_availability_result`, índice/snapshot de seleção ou marcador de
  autoridade associado a intent estrutural de seleção;
- intent `AVAILABILITY_SEARCH` ou `SELECT_AVAILABILITY_OPTION`;
- template canônico de lista/opção mais próxima;
- corpo reconhecível como lista/pergunta de escolha de availability.

`DRAFT_REVIEW`, `APPROVED_AS_IS` e `draft_message_id`, isoladamente ou em
conjunto, não são artefatos de availability. Uma pergunta de passageiros sem
nenhum dos sinais acima permanece uma mensagem assistente normal: não forma
barreira, não apaga `BOOKABLE` anterior e não é reclassificada como prompt de
availability.

Uma `BOT_AUTO_REPLY` ou revisão entregue que possua qualquer artefato acima e
não possa ser resolvida continua fail-closed. A redução do escopo não pode
transformar projeção real de availability irresolvida em mensagem legada
publicável.

#### Precedência e identidade no scan

- Para uma projeção entregue, verificar primeiro `UNDELIVERED`; esse estado
  permanece invisível e sem barreira.
- Depois, tentar `resolveDeliveredPromptSourceMessageWithIndex` usando a
  identidade completa já exigida e o item persistido em
  `history[deliveredIndex]`.
- Se resolver, classificar e publicar somente a mensagem efetiva resolvida. O
  índice causal retornado continua sendo o índice da entrega, e
  `source_message_id` continua sendo a identidade entregue.
- Se não resolver e houver artefato mínimo de availability, encerrar o scan
  imediatamente. Não retornar a projeção, não remover apenas o `tool_context`,
  não inferir `AVAILABILITY_OPTION_CHOICE` pelo body, não publicar
  `ABSENT_LEGACY` e não buscar prompt anterior.
- Se não resolver e não houver artefato de availability, a mensagem permanece
  fora deste gate e segue as regras normais do seu domínio.
- `latestReliableAssistantMessageWithIndex`,
  `classifiedAvailabilityPromptMessageAtV1`, os finders reversos e
  `latestDeliveredInvalidAvailabilityPromptIndexV1` devem observar a mesma
  ordem e a mesma decisão de candidatura.

### Tabela de estados esperados

| Cenário | Entrega | Artefato availability | Decode/resolução | Resultado | Barreira |
|---|---|---|---|---|---|
| intervalo completo com `from < to` nas duas cópias | confirmada | estrutural | válido e bilateralmente igual | `VALID_STRUCTURAL`, intervalo preservado | não |
| intervalo completo com `from = to` nas duas cópias | confirmada | estrutural | válido e bilateralmente igual | `VALID_STRUCTURAL`, um dia | não |
| intervalo invertido idêntico nas duas cópias | confirmada | estrutural | semanticamente inválido | `INVALID`, `Presented=nil` | sim |
| datas ausentes nas duas cópias | confirmada | estrutural/legado | ausência bilateral canônica | válido conforme demais facts | não |
| somente `date_from` ou somente `date_to`, igual nas duas cópias | confirmada | estrutural/legado | limite presente válido | intervalo parcial válido | não |
| valor vazio, `null`, tipo incorreto ou data malformada | confirmada | facts presentes | inválido | `INVALID`, zero readers | sim |
| data válida em uma cópia e inválida/ausente/divergente na outra | confirmada | facts presentes | divergência bilateral | `INVALID`, zero readers | sim |
| pergunta apenas de passageiros em review aprovado | confirmada | nenhum | draft irresolvido | fora do gate; mensagem normal | não |
| `BOOKABLE` válido seguido da pergunta não relacionada | confirmada | nenhum na mensagem nova | n/a | autoridade anterior preservada | não |
| `BOT_AUTO_REPLY` com corpo de opções, sem evento e draft inexistente | confirmada | body de availability | irresolvida | scan interrompido; sem prompt publicado | sim |
| projeção estrutural válida | confirmada | evento + facts | resolução única válida | mensagem efetiva entregue | não |
| projeção legada válida | confirmada | facts/lista legada | resolução única válida | `ABSENT_LEGACY` resolvido | não |
| projeção de availability irresolvida | não confirmada | qualquer | falha | `UNDELIVERED`, invisível | não |

Em toda linha `INVALID` ou de projeção entregue irresolvida, exigir também
zero active prompt de availability, zero materialização, zero bootstrap de
seleção e zero booking.

### Funções e call sites afetados

#### Decode e classificação factual

- `booking_create_router.go`
  - `parseAvailabilityContextPayloadWithValidityV1`: validar a relação entre
    os limites e preservar ausência/presença canônica;
  - `parsePersistedAvailabilityFilterDateV1`: distinguir chave ausente de
    valor presente inválido;
  - `parseAvailabilityContextPayload`: remover o descarte de validade ou
    deixar de usá-lo em todo caminho de autoridade;
  - `visibleAvailabilitySelectionContextFromHistoryMessage`,
    `findLatestAvailabilityContextWithSourceBefore` e
    `classifiedAvailabilityPromptMessageAtV1`: propagar invalidade e respeitar
    resolução/barreira.
- `availability_prompt_event_v1.go`
  - `availabilityPromptRawContextsV1`: devolver estado de presença e validade,
    não apenas contextos decodificados;
  - `availabilityPromptPresentedContextV1`,
    `availabilityPromptClassifiedContextV1` e
    `classifyAvailabilityPromptCandidateV1`: impedir que facts presentes
    inválidos virem ausência legada;
  - `messageMayCarryAvailabilityPromptV1`: substituir `DRAFT_REVIEW` isolado
    pelos sinais mínimos de domínio;
  - `deliveredInvalidAvailabilityPromptBarrierV1`,
    `deliveredInvalidAvailabilityPromptBarrierAtV1` e
    `latestDeliveredInvalidAvailabilityPromptIndexV1`: compartilhar
    candidatura e precedência.
- `tool_router.go`
  - `buildAvailabilityToolResponsePayload`: manter a representação canônica
    `YYYY-MM-DD` e omitir ponteiros nil;
  - `parseAvailabilityDateSelectionInput` e os callers de materialização:
    recusar contexto inválido antes de produzir seleção.
- `agent.go`: confirmar que as cópias factualizadas em `Payload` e
  `NormalizedPayload` usam o mesmo payload canônico, sem representação
  temporal alternativa.

#### Wrappers e consumers legados

- `service.go`
  - `trustedAvailabilityContextFromPromptMessage`,
    `visibleAvailabilityContextFromPromptMessageAt`,
    `attachCurrentAvailabilitySelectionContext`,
    `availabilityContextFromOutOfTurnActivePromptSource` e
    `classifyAvailabilitySelectionTurn`: nenhuma materialização pode continuar
    após `valid=false`.
- `interpreter.go`
  - `availabilityOptionCountFromMessageToolContext` e
    `availabilityOptionCountFromMessage`: payload inválido fornece contagem
    zero sem reabrir fallback por body.
- `intent_router.go`
  - `latestReliableAssistantMessageWithIndex` consumers,
    `currentAvailabilitySelectionPromptContext`,
    `activeAvailabilitySelectionPromptContext` e
    `currentAvailabilitySelectionPromptAvailabilityContextAt`: não inferir ou
    materializar depois da barreira.
- `booking_draft_context.go`
  - `availabilityPromptSourceMessageIDBefore`,
    `availabilityPromptSourceMessageIDAtHistoryIndex` e coleta do draft:
    manter identidade entregue e não atravessar invalidade.
- `availability_selection_state_v1.go`
  - `availabilitySelectionStateV1ForRead`,
    `availabilitySelectionStructuredEventsV1`,
    `availabilitySelectionSnapshotCandidatesFromMessageV1` e
    `exactLegacyAvailabilitySelectionPromptSourceV1`: payload inválido não
    enriquece snapshot nem reconstrói `BOOKABLE`.
- `availability_draft.go`
  - `availabilityDraftHasSelectedTrip` e
    `availabilityDraftOptionHasCompleteTripFacts`: nenhuma seleção é recuperada
    através da barreira.
- entrypoints de `booking_create` em `tool_router.go` e
  `booking_create_router.go`: `BookingCreateInput` exige estado materializado
  posterior a contexto válido.

#### Scan ativo e resolução

- `active_prompt_context.go`
  - `latestReliableAssistantMessageWithIndex`: aplicar a ordem
    `UNDELIVERED -> resolver -> candidato/barreira -> mensagem não relacionada`;
  - `InferActivePromptContext`: não inferir pelo body de projeção de
    availability irresolvida;
  - `resolveDeliveredPromptSourceMessageWithIndex`: preservar a mensagem
    persistida e o índice causal da entrega.
- `booking_create_router.go`
  - `classifiedAvailabilityPromptMessageAtV1` e os dois scans reversos:
    resolver a projeção válida antes de decidir a barreira e interromper em
    candidata entregue irresolvida.

### Estratégia mínima de correção

1. Centralizar o decode factual em
   `parseAvailabilityContextPayloadWithValidityV1`; validar datas e, depois do
   decode, a relação `DateFrom <= DateTo` quando ambos existem. Remover o
   wrapper de um retorno dos caminhos de autoridade e atualizar todos os seus
   call sites atuais para consumir explicitamente o booleano.
2. Fazer `availabilityPromptRawContextsV1` preservar `present` e `valid` por
   cópia. Usar esse resultado no classificador estrutural e legado para que
   facts presentes inválidos resultem `INVALID` em vez de contexto ausente.
3. Extrair uma decisão mínima de candidatura de availability baseada apenas
   nos artefatos listados neste plano. Reutilizá-la na barreira e no scan;
   remover `DRAFT_REVIEW` isolado como prova de domínio.
4. Reordenar o scan de projeções sem criar um resolver paralelo: ignorar
   `UNDELIVERED`, tentar o resolver existente, retornar a mensagem resolvida
   quando válida, interromper se a projeção entregue for candidata e somente
   então tratar a mensagem não relacionada pelas regras normais.
5. Exercitar todos os readers sobre o mesmo histórico inválido para provar que
   não existe caminho alternativo por body, tool context, snapshot, bootstrap
   ou booking.

### REDs antes do patch

#### Semântica temporal

Adicionar teste table-driven cobrindo:

- intervalo invertido idêntico em `Payload` e `NormalizedPayload`;
- intervalo válido com limites diferentes e com a mesma data;
- os dois limites ausentes;
- somente `date_from` presente e somente `date_to` presente;
- valor vazio, `null`, tipo não string, data impossível e formato alternativo;
- uma cópia válida e a outra malformada, ausente ou com valor divergente;
- preservação de `TripDate`, `DateFrom` e `DateTo` em `Presented` para os
  controles válidos.

Para os casos inválidos, provar `INVALID`, `Presented=nil` e barreira quando a
mensagem entregue for candidata.

#### Propagação de validade por todos os readers

Construir um prompt legado entregue com
`tool_context.availability_search` bilateralmente presente e a mesma data
malformada nas duas cópias. Provar, no mesmo cenário:

- `trustedAvailabilityContextFromPromptMessage=nil`;
- contexto visible/latest ausente;
- option count zero;
- finders não retornam facts;
- resposta inbound `"1"` produz zero `MATERIALIZE` e zero
  `SELECTION_MATERIALIZED`;
- `AvailabilitySelectionStateV1` não fica nem permanece `BOOKABLE` através da
  barreira;
- booking draft não ganha seleção e todos os entrypoints produzem zero
  `BookingCreateInput`.

#### Escopo da barreira

Para `DRAFT_REVIEW/APPROVED_AS_IS`, adicionar caso entregue com
`draft_message_id` irresolvido cujo body pergunta somente quantidade de
passageiros e não contém evento, tool facts, opções, snapshot, intent ou
template de availability. Provar:

- a mensagem não é candidata a availability;
- não existe barreira de availability;
- o active prompt continua sendo `PASSENGER_COUNT`;
- um estado `BOOKABLE` válido anterior permanece disponível;
- nenhum reader a reclassifica como `ABSENT_LEGACY` de availability.

#### Precedência do scan ativo

Adicionar `BOT_AUTO_REPLY` entregue, com corpo de opções reconhecível, sem
evento e com draft inexistente, após prompt válido anterior. Provar:

- a resolução falha;
- a mensagem é candidata a availability e forma barreira;
- `latestReliableAssistantMessageWithIndex` interrompe sem retornar a projeção
  nem o prompt anterior;
- `InferActivePromptContext` não infere `AVAILABILITY_OPTION_CHOICE`;
- `classifiedAvailabilityPromptMessageAtV1`, finders, materialização,
  bootstrap, booking draft e booking create permanecem fechados;
- `sourceIndex` e `source_message_id` não são fabricados.

### Controles positivos e regressões

- intervalo completo ordenado e intervalo de mesma data permanecem válidos;
- ausência bilateral dos limites e intervalos parciais válidos mantêm sua
  representação canônica;
- projeção estrutural válida continua resolvendo evento e facts do draft, com
  identidade e índice causal da entrega;
- projeção legada entregue, com exatamente um draft causal válido, continua
  resolvendo como legado confiável;
- pergunta de passageiros em review aprovado continua visível como prompt de
  passageiros e não afeta autoridade de availability anterior;
- `BOT_AUTO_REPLY` de availability entregue e irresolvida continua bloqueando
  o histórico;
- a mesma projeção `UNDELIVERED` permanece invisível, sem barreira, e não
  esconde prompt válido anterior;
- seleção válida posterior a prompt válido ainda pode materializar `BOOKABLE`
  e produzir `BookingCreateInput` sem alterar idempotência;
- humano e cancelamento fortes continuam com a precedência existente.

### Riscos de regressão

- Tratar intervalo parcial como inválido quebraria filtros abertos já
  representáveis; os controles de um único limite evitam esse estreitamento.
- Tratar chave vazia ou `null` como ausência reabriria a ambiguidade; a
  presença inválida deve continuar distinguível em todos os helpers.
- Uma candidatura ampla demais volta a bloquear perguntas de outros domínios;
  uma candidatura estreita demais deixa projeções reais de availability
  atravessarem o scan. A matriz separa modo de entrega de artefato funcional.
- Reordenar o scan sem preservar o índice da entrega pode quebrar rejeições,
  tombstones e identidade de seleção. Os controles devem afirmar índice e ID.
- Um reader legado esquecido pode reconstruir facts a partir do payload bruto;
  o teste transversal deve cobrir contagem, trusted/visible, materialização,
  bootstrap, draft e booking no mesmo histórico.
- A validação temporal deve operar sobre data civil canônica e não depender do
  horário local ou de `time.Now`, evitando divergência por timezone.

### Comandos de validação da futura implementação

Registrar primeiro os REDs dirigidos, antes do patch de produção:

```bash
cd apps/api
go test -count=1 ./internal/chat -run '^(TestAvailabilityContextTemporalRangeValidationV1|TestAvailabilityContextInvalidPayloadFailsClosedAcrossReaders|TestAvailabilityPromptBarrierScopesUnresolvedProjectionToAvailability|TestLatestReliableAssistantMessageStopsAtUnresolvedAvailabilityProjection)$'
```

Após o patch, executar o gate focado com repetição e a validação padrão de
chat:

```bash
cd apps/api
go test -count=20 ./internal/chat -run '^(TestAvailabilityContextTemporalRangeValidationV1|TestAvailabilityContextInvalidPayloadFailsClosedAcrossReaders|TestAvailabilityPromptBarrierScopesUnresolvedProjectionToAvailability|TestLatestReliableAssistantMessageStopsAtUnresolvedAvailabilityProjection)$'
go test -race -count=1 ./internal/chat
go test -count=1 ./internal/chat -run '(Availability|Passenger|Booking|Human|Cancel|Review|AutoSend|Delivery)'
go test -count=1 ./internal/chat
go test -count=1 ./...
gofmt -l internal/chat
rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l
git diff --check
git status --short
```

O inventário de produção deve permanecer em `54`. Parar após os gates e
solicitar novo `/review`, sem atualizar tracker/handoff, executar commit, push,
deploy ou smoke.

## Fase 4 — Executada localmente, aguardando review: classe de autoridade como gate único de consumo

### Objetivo e limites

Corrigir exclusivamente os quatro findings do review mais recente:

1. preservar `Candidate=true` quando uma reconciliação `INVALID` ainda contém
   evidência de domínio availability;
2. impedir que `INVALID` ou `UNDELIVERED` façam merge de facts/seleção no
   estado canônico;
3. impedir que `INVALID` ou `UNDELIVERED` enriqueçam snapshots de seleção;
4. remover a cópia parcial e stale do plano existente na raiz do repositório.

Princípio central desta fase:

```text
reconciled.Facts e reconciled.Selection = dados decodificados
authority.Class = autorização para consumo
```

Decodificar ou reconciliar bilateralmente um payload não lhe concede
autoridade. Somente `VALID_STRUCTURAL` e `ABSENT_LEGACY` com as provas legadas
exatas do reader podem alimentar estado, snapshot ou booking. `INVALID` e
`UNDELIVERED` fornecem zero autoridade em todos esses caminhos.

Esta fase não altera API pública, schema, formato persistido, delivery status,
parser, regex, idempotência, B2 ou 3.6F-D. Não inclui refactor amplo, commit,
push, deploy ou smoke.

### Causas raiz

#### Finding 1 — invalidade apaga a candidatura antes da barreira

`reconcileAvailabilityPromptMessageV1` possui retornos antecipados de
`Invalid=true` antes de consolidar `Candidate`. Isso ocorre, entre outros, em
conflitos de metadata availability com artefato passenger e em
`tool_context` não-map combinado com metadata ou body canônico de
availability. O payload continua contendo evidência suficiente para pertencer
ao domínio de availability, mas retorna `Candidate=false`.

`messageMayCarryAvailabilityPromptV1` expõe diretamente esse campo.
Consequentemente, uma mensagem `INVALID` e entregue pode deixar de formar
barreira em `deliveredInvalidAvailabilityPromptBarrierAtV1`; o scan então
atravessa a mensagem nova e recupera uma autoridade `BOOKABLE` histórica.

A causa não é a classe final — ela já é `INVALID` —, mas o acoplamento entre
duas decisões diferentes: pertencer ao gate de availability e possuir
autoridade factual.

#### Finding 2 — ramo default reconcede autoridade no merge canônico

`shouldMergeAvailabilityFactsFromMessage` trata explicitamente
`ABSENT_LEGACY` e `VALID_STRUCTURAL`, mas o ramo `default` também cobre
`INVALID` e `UNDELIVERED` e chama
`legacyAvailabilitySelectionProjectionFactsV1`. Esse helper consulta
`reconcileAvailabilityPromptMessageV1` diretamente e pode encontrar
`Facts/Selection` bilaterais retidos apesar da classe não autorizada.

Com isso, uma projeção outbound ainda não entregue ou uma mensagem inválida por
conflito de domínio pode atravessar
`mergeMessageToolFactsIntoCanonicalState`, restaurar
`LastToolFacts.availability_search`, seleção e rota e voltar a materializar
estado acionável.

#### Finding 3 — enrichment contorna a classe de autoridade

`availabilitySelectionSnapshotCandidatesFromMessageV1` possui o mesmo desenho:
os casos positivos são explícitos, mas o ramo `default` volta a chamar
`reconcileAvailabilityPromptMessageV1` e extrai `reconciled.Facts` e
`reconciled.Selection`. Isso contorna o `Facts=nil` aplicado por
`classifyAvailabilityPromptCandidateV1` a mensagens `INVALID` e permite usar
também mensagens `UNDELIVERED`.

Um evento legado materializado com snapshot incompleto e IDs coincidentes pode,
assim, receber data, pacote, preço ou outros campos de uma fonte inválida ou
ainda não entregue. O agregado enriquecido pode alcançar booking apesar de a
fonte nunca ter adquirido autoridade.

#### Finding 4 — duas fontes documentais com o mesmo nome

`h-2026-07-27a-structural-reconciliation-fixes.md` na raiz é uma cópia parcial
antiga, enquanto
`plans/h-2026-07-27a-structural-reconciliation-fixes.md` é o plano cumulativo
canônico. Manter ambas cria duas fontes concorrentes e viola a regra de que
planos versionáveis vivem em `plans/`; a cópia stale deve ser removida, sem
criar substituto.

### Invariantes obrigatórios

#### Candidatura, entrega, barreira e autoridade são decisões separadas

- `Candidate` responde somente se há evidência de que a mensagem pertence ao
  gate de availability; não afirma validade nem entrega.
- Evidência de availability continua candidata mesmo quando a reconciliação
  termina `INVALID`, inclusive por conflito com artefato passenger, metadata
  unilateral/conflitante ou `tool_context` malformado acompanhado de sinal
  canônico de availability.
- Uma candidata `INVALID` confirmadamente entregue forma barreira temporal,
  publica `Facts=nil` e `Presented=nil` e não permite recuperar autoridade
  histórica.
- Uma candidata `UNDELIVERED` fornece zero autoridade, mas não cria barreira
  somente por ainda não ter sido entregue; o prompt entregue anterior continua
  visível conforme as regras existentes.
- Mensagem exclusivamente passenger ou de outro domínio, sem evidência de
  availability, permanece `Candidate=false` e não cria barreira de
  availability.
- Resolver uma projeção não muda essas regras: a mensagem efetiva resolvida é
  classificada uma vez, e sua candidatura/classe decide barreira e consumo.

#### Matriz fechada de consumo por `authority.Class`

| Classe | Pode formar barreira? | Pode fornecer facts/selection? | Pode enriquecer snapshot? | Pode alimentar booking? |
|---|---:|---:|---:|---:|
| `VALID_STRUCTURAL` entregue | somente se inválida, portanto n/a nesta classe | sim, apenas `Presented`/seleção autorizada | sim, a partir de `Presented` | sim, após materialização bookable normal |
| `ABSENT_LEGACY` entregue e confiável | somente pelos blockers legados já definidos | sim, somente com prova legada exata do reader | sim, somente com fonte legada exata e compatível | sim, somente após autoridade bookable normal |
| `INVALID` entregue e candidata | sim | não | não | não |
| `UNDELIVERED` | não | não | não | não |

- Nenhum `default` de switch sobre `authority.Class` pode conceder autoridade.
  Classes positivas devem ser enumeradas; todas as demais retornam zero.
- `reconciled.Facts` e `reconciled.Selection` podem permanecer disponíveis
  internamente para diagnóstico determinístico da reconciliação, mas nenhum
  consumer de estado, snapshot ou booking pode lê-los sem antes passar pelo
  gate positivo da classe.
- Em `INVALID`/`UNDELIVERED`, remover apenas facts de availability do merge;
  facts independentes de outras tools continuam seguindo a política existente.
- Zero autoridade significa: nenhum `LastToolFacts.availability_search`, rota,
  índice, snapshot, `BOOKABLE`, `HasBookableSelection` ou
  `BookingCreateInput` derivado da mensagem.

#### Fonte documental única

- O único plano deste hotfix é
  `plans/h-2026-07-27a-structural-reconciliation-fixes.md`.
- A cópia stale na raiz deve deixar de existir; não movê-la para outro nome,
  não criar novo plano e não alterar outros planos nesta fase.

### Mudança mínima por arquivo

#### Produção

- `apps/api/internal/chat/availability_prompt_event_v1.go`
  - calcular/preservar candidatura de domínio antes dos retornos de
    reconciliação inválida relevantes;
  - manter `classifyAvailabilityPromptCandidateV1` como o único tradutor de
    reconciliação para classe de autoridade, zerando facts de `INVALID` como já
    ocorre;
  - preservar a regra `INVALID` entregue + candidata = barreira e
    `UNDELIVERED` = sem barreira;
  - não ampliar o fallback textual nem adicionar parser/regex.
- `apps/api/internal/chat/conversation_state_machine.go`
  - tornar `shouldMergeAvailabilityFactsFromMessage` allowlist explícita:
    apenas `VALID_STRUCTURAL` e `ABSENT_LEGACY` confiável podem retornar true;
  - retornar false explicitamente para `INVALID`, `UNDELIVERED` e qualquer
    classe futura/desconhecida;
  - preservar o merge de tool facts não relacionados a availability.
- `apps/api/internal/chat/availability_selection_state_v1.go`
  - remover o enrichment por `reconcileAvailabilityPromptMessageV1` do ramo
    default de `availabilitySelectionSnapshotCandidatesFromMessageV1`;
  - aceitar candidatos somente em `VALID_STRUCTURAL` ou `ABSENT_LEGACY`
    confiável, mantendo as provas existentes de fonte/IDs e compatibilidade;
  - `INVALID`, `UNDELIVERED` e classe futura/desconhecida retornam lista vazia.

#### Testes

- `apps/api/internal/chat/availability_prompt_event_v1_test.go`
  - adicionar a matriz de candidatura inválida/barreira e os controles de
    domínio/entrega;
  - cobrir o merge canônico fail-closed no mesmo payload que ainda retém
    `Facts/Selection` reconciliados.
- `apps/api/internal/chat/availability_selection_state_v1_test.go`
  - adicionar o enrichment de snapshot incompleto contra fontes `INVALID` e
    `UNDELIVERED`, mais os dois caminhos positivos autorizados.
- Reutilizar `active_prompt_context_test.go` ou testes já existentes somente se
  necessário para provar o scan completo; não criar uma suíte paralela nem
  alterar fixtures não relacionadas.

#### Documentação

- remover `h-2026-07-27a-structural-reconciliation-fixes.md` da raiz;
- manter e atualizar somente este plano canônico em `plans/`.

Nenhum outro arquivo é esperado. Se um RED exigir mudança fora dessa lista,
parar e reavaliar o escopo antes de implementar.

### REDs antes do patch de produção

#### Candidata inválida entregue forma barreira

Adicionar
`TestAvailabilityPromptInvalidDomainEvidencePreservesCandidateAndBarrierV1`,
table-driven, após um prompt `BOOKABLE` válido, cobrindo ao menos:

- metadata availability unilateral/conflitante com artefato passenger;
- `tool_context` não-map com metadata canônica bilateral de availability;
- `tool_context` não-map com body canônico real de lista/EARLIEST.

Antes do patch, provar o RED `Candidate=false` e recuperação indevida do prompt
antigo. Depois do patch, exigir `Candidate=true`, classe `INVALID`,
`Facts=nil`, `Presented=nil`, barreira no índice novo, zero finder/contexto
histórico, zero `BOOKABLE` e zero `BookingCreateInput`.

Controles no mesmo teste:

- mensagem passenger-only entregue: `Candidate=false`, sem barreira e
  `BOOKABLE` anterior preservado;
- as mesmas candidatas inválidas sem delivery confirmado: classe
  `UNDELIVERED`, zero autoridade e nenhuma barreira criada apenas pela ausência
  de entrega.

#### Merge canônico recusa classes não autorizadas

Adicionar
`TestCanonicalConversationStateRejectsUntrustedAvailabilityAuthorityClassesV1`,
table-driven para `INVALID` e `UNDELIVERED`. Construir payload bilateral com
facts e seleção decodificáveis, incluindo IDs completos, de forma que
`legacyAvailabilitySelectionProjectionFactsV1` retorne true e reproduza o
merge anterior.

Exigir que `shouldMergeAvailabilityFactsFromMessage=false` e que
`mergeMessageToolFactsIntoCanonicalState` não publique availability, seleção,
rota ou estado bookable. Incluir uma segunda tool independente no mesmo mapa e
provar que ela continua sendo mesclada conforme a regra atual.

Controles positivos:

- `VALID_STRUCTURAL` entregue usa somente `Presented` e continua mesclando;
- `ABSENT_LEGACY` entregue, bilateral e confiável continua mesclando pelo
  helper legado exato;
- legado ausente mas não confiável permanece fechado.

#### Enrichment de snapshot recusa classes não autorizadas

Adicionar
`TestAvailabilitySelectionSnapshotEnrichmentRejectsUntrustedAuthorityClassesV1`.
Usar evento legado materializado com snapshot incompleto que referencia, com
IDs coincidentes, uma mensagem contendo facts/seleção bilaterais. Executar a
mesma fixture como `INVALID` entregue e como `UNDELIVERED`.

Antes do patch, provar que data, pacote, preço ou outro campo ausente é
preenchido por `reconciled.Facts/Selection`. Depois do patch, exigir candidato
vazio, snapshot sem enrichment não autorizado, estado não bookable e zero
`BookingCreateInput`.

Controles positivos:

- `VALID_STRUCTURAL` entregue enriquece somente a partir de `Presented`;
- `ABSENT_LEGACY` entregue e com fonte exata/compatível mantém o enrichment
  necessário para compatibilidade;
- mismatch de `trip_id`, `board_stop_id` ou `alight_stop_id` continua sem
  enrichment em qualquer classe.

#### Fonte documental única

Antes da remoção, registrar que os dois caminhos existem e divergem. Depois,
exigir:

```bash
test ! -e ../../h-2026-07-27a-structural-reconciliation-fixes.md
test -f ../../plans/h-2026-07-27a-structural-reconciliation-fixes.md
```

### Controles positivos transversais

- prompt estrutural entregue válido continua abrindo active prompt e expondo
  exatamente as opções apresentadas;
- seleção materializada válida continua sobrevivendo a reload/restart e pode
  chegar a `BOOKABLE`/booking pelos gates existentes;
- legado entregue, bilateral e confiável continua funcional;
- `UNDELIVERED` permanece invisível e não apaga a autoridade entregue anterior;
- `INVALID` entregue candidata bloqueia autoridade anterior sem fornecer
  substituta;
- pergunta passenger-only entregue não cria barreira de availability;
- humano/cancelamento `STRONG`, H-012, documentos, lap child, payment e
  out-of-turn permanecem verdes;
- inventário de produção permanece em `54 regexp.MustCompile`.

### Riscos e contenções

- Preservar `Candidate=true` de forma ampla demais pode transformar corrupção
  exclusivamente passenger em barreira de availability. A implementação deve
  derivar candidatura de evidência positiva de availability observada antes da
  invalidação, não do fato genérico de a mensagem ser inválida.
- Preservar `Candidate=false` em qualquer retorno antecipado com evidência de
  availability mantém a ressurreição histórica. Os REDs precisam cobrir cada
  família de retorno antecipado identificada no review.
- Fechar os defaults pode revelar fixtures legadas que dependiam de payload
  não entregue ou inválido. Corrigir a fixture somente quando ela representar
  um writer canônico; não relaxar a classe para acomodá-la.
- O enrichment legado positivo deve continuar limitado à fonte exata e aos IDs
  compatíveis; a fase não autoriza busca por envelope semelhante ou fallback de
  índice/data.
- A remoção da cópia na raiz é intencional e não recuperável pelo working tree
  após commit; o conteúdo relevante já está preservado cumulativamente neste
  plano canônico.

### Gates da futura implementação

Registrar primeiro os três REDs funcionais antes de alterar produção. Depois
do patch mínimo, executar:

```bash
cd apps/api
go test -count=20 ./internal/chat -run '^(TestAvailabilityPromptInvalidDomainEvidencePreservesCandidateAndBarrierV1|TestCanonicalConversationStateRejectsUntrustedAvailabilityAuthorityClassesV1|TestAvailabilitySelectionSnapshotEnrichmentRejectsUntrustedAuthorityClassesV1)$'
go test -race -count=1 ./internal/chat
go test -count=1 ./internal/chat -run '(Availability|Passenger|Booking|Human|Cancel|Review|AutoSend|Delivery|Document|LapChild|Payment|OutOfTurn)'
go test -count=1 ./internal/chat
go test -count=1 ./...
gofmt -w internal/chat/availability_prompt_event_v1.go internal/chat/availability_prompt_event_v1_test.go internal/chat/conversation_state_machine.go internal/chat/availability_selection_state_v1.go internal/chat/availability_selection_state_v1_test.go
gofmt -l internal/chat/availability_prompt_event_v1.go internal/chat/availability_prompt_event_v1_test.go internal/chat/conversation_state_machine.go internal/chat/availability_selection_state_v1.go internal/chat/availability_selection_state_v1_test.go
rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l
test ! -e ../../h-2026-07-27a-structural-reconciliation-fixes.md
test -f ../../plans/h-2026-07-27a-structural-reconciliation-fixes.md
git diff --check
git status --short
```

Se `go`/`gofmt` continuarem indisponíveis no host, executar os mesmos comandos
em `golang:1.23` com caches temporários isolados e o repositório montado, sem
alterar dependências. O inventário deve permanecer em `54`.

Gate de saída:

```text
3 REDs reproduzidos antes do patch
3 REDs verdes em count=20 após o patch
race e suítes amplas verdes
INVALID/UNDELIVERED com zero autoridade em merge, snapshot e booking
INVALID entregue candidata com barreira; UNDELIVERED sem barreira
somente VALID_STRUCTURAL/ABSENT_LEGACY confiável nos caminhos positivos
uma única cópia canônica do plano
git diff --check verde
novo /review dirigido sem P1/P2
```

Evidência local verde não declara review limpo e não autoriza commit, push,
deploy ou smoke. Parar após os gates e solicitar novo `/review`; H-2026-07-16B2
e 3.6F-D permanecem bloqueadas.

### Evidência observada da execução

Os três REDs foram adicionados antes do patch e falharam pelos mecanismos
esperados:

- candidatas inválidas retornavam `Candidate=false` e não formavam barreira;
- `INVALID` e `UNDELIVERED` retornavam true no gate de merge canônico;
- as duas classes enriqueciam snapshot incompleto com facts decodificados.

O patch fechou os defaults de consumo, preservou candidatura quando existe
evidência positiva de availability e zerou `Facts/Selection` publicados pelas
classes não autorizadas. A suíte ampla revelou ainda que writers de outros
domínios copiavam os dados decodificados para a projeção outbound; pelo novo
contrato, essas cópias se tornavam candidatas inválidas e apagavam no replay a
autoridade materializada correta. A contenção mínima ficou em `service.go`:
projeções que não são prompt de availability deixam de persistir artifacts de
availability, enquanto estado/evento canônico continua sendo a autoridade.
Links out-of-turn persistem `template_data` bilateralmente e
`active_prompt_kind` é tratado apenas como referência à fonte anterior; a
presença de facts/selection availability no lembrete continua produzindo
candidata inválida, sem exceção de autoridade.

Fixtures e expectativas positivas foram ajustadas para provar a seleção no
estado/evento canônico, e não nos dados decodificados da projeção passenger.
A cópia stale do plano na raiz foi removida e este arquivo permanece como
fonte única.

Gates executados em `golang:1.23` via Docker, pois Go/gofmt não estão
disponíveis no host:

```text
PASS — 3 REDs dirigidos pós-patch, count=20 — internal/chat 0.398s
PASS — go test -race -count=1 ./internal/chat — internal/chat 53.653s
PASS — suíte transversal completa — 7.292s
PASS — go test -count=1 ./internal/chat — 10.001s
PASS — go test -count=1 ./... — internal/chat 9.603s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário de produção = 54 regexp.MustCompile
PASS — plano stale ausente e plano canônico presente
PASS — git diff --check
```

Status permanece **EM CORREÇÃO APÓS REVIEW — 3 P1 + 1 P2 DE ENTREGA TEMPORAL
E PROVENIÊNCIA**. Esses gates não declaram review limpo nem segurança para
commit. Não houve commit, push, deploy ou smoke; a próxima ação única é novo
`/review` dirigido a esta fase. H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

## Fase 5 — Histórico superado pela Fase 6: body canônico e enrichment legado confiável

### Escopo e reavaliação arquitetural

O review seguinte encontrou duas lacunas no mesmo limite arquitetural de
autoridade. A rodada permanece um hotfix único porque ambas são decisões locais
da reconciliação/classificação já centralizada; não foi criado parser, fallback,
reader ou fonte de autoridade paralelo. B2, 3.6F-D, migration, regex e refactor
amplo continuam fora do escopo.

### Causas raiz

1. `reconcileAvailabilityPromptMessageV1` reconhecia o body canônico somente
   quando não havia artefato passenger nem metadata incompatível. A evidência
   real de availability desaparecia exatamente no conflito que deveria produzir
   candidata `INVALID` e barreira.
2. `availabilitySelectionSnapshotCandidatesFromMessageV1` autorizava a classe
   `ABSENT_LEGACY`, mas só materializava `Facts`. Uma seleção bilateral completa,
   reconciliada e identificada, sem `availability_search`, passava no helper de
   confiança e ainda assim deixava `selected=nil`.

### Invariantes

- body canônico gerado pelos builders de lista ou EARLIEST é evidência positiva
  de domínio; linguagem genérica não é;
- body canônico com passenger/payment ou outro domínio incompatível resulta em
  `Candidate=true`, `Invalid=true`; quando entregue, forma barreira e fornece
  zero `Facts`, `Selection`, `Presented`, merge, enrichment ou booking;
- passenger/payment/document/human/cancel sem body canônico continuam fora do
  domínio; `UNDELIVERED` não forma barreira apenas por não ter sido entregue;
- `authority.Class` permanece o gate. `VALID_STRUCTURAL` usa somente
  `Presented`; `ABSENT_LEGACY` usa facts bilaterais confiáveis ou, na ausência
  deles, a própria `Selection.SelectedResult` reconciliada comprovada por
  `legacyAvailabilitySelectionProjectionFactsV1`;
- `INVALID` e `UNDELIVERED` não produzem candidatos de snapshot. O enrichment
  apenas completa campos ausentes quando trip/board/alight coincidem com a
  fonte exata; não inventa facts nem consulta envelopes posteriores.

### Mudança mínima por arquivo

- `availability_prompt_event_v1.go`: preservar candidatura pelo body canônico
  antes da invalidação e classificar conflito body-versus-domínio como inválido;
- `availability_prompt_event_v1_test.go`: cobrir lista+passenger metadata,
  EARLIEST+passenger event, body+payment metadata e controles;
- `availability_selection_state_v1.go`: preencher `selected` exclusivamente no
  ramo `ABSENT_LEGACY` confiável sem facts;
- `availability_selection_state_v1_test.go`: cobrir seleção bilateral sem
  `availability_search`, reload parcial, `INVALID`, `UNDELIVERED`, identidade
  ausente e `VALID_STRUCTURAL`.

### REDs, controles e gates observados

Antes do patch, os três conflitos de body retornaram
`ABSENT_LEGACY/Candidate=false`, e a seleção legada confiável retornou zero
candidatos de snapshot. A auditoria dos callers reproduziu ainda enrichment por
uma projeção cujo source ID não correspondia ao evento; o gate final exige
projection, selection message, prompt source e índice exatos. Depois do patch:

```text
PASS — REDs dirigidos pós-patch, count=20 — internal/chat 0.472s
PASS — go test -race -count=1 ./internal/chat — 53.650s
PASS — regressões transversais — 7.962s
PASS — go test -count=1 ./internal/chat — 7.838s
PASS — go test -count=1 ./... — internal/chat 9.373s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário de produção = 54 regexp.MustCompile
PASS — plano stale ausente; somente este plano canônico presente
PASS — git diff --check; git diff --cached --check
```

Os controles preservam mensagens passenger/payment normais sem barreira,
linguagem genérica sem candidatura, prompt estrutural válido, enrichment por
`Presented`, mismatch de identidade fechado e `INVALID`/`UNDELIVERED` sem
enrichment. Os gates locais não declaram review limpo nem segurança para
commit. O status e a próxima ação desta rodada são históricos e foram
**SUPERADOS PELA FASE 6**. Não houve commit, push, PR, deploy ou smoke;
H-2026-07-16B2 e 3.6F-D permaneceram bloqueadas.

## Fase 6 — Histórico superado pela Fase 7: projeção exclusiva e compatibilidade integral

### Escopo e causas raiz

O review seguinte confirmou a correção do body canônico e encontrou dois P1
restritos ao enrichment `ABSENT_LEGACY`, além de um P2 documental:

1. o loop aceitava a projeção legada e depois o prompt source estrutural, de
   modo que `Presented` da segunda mensagem completava campos ausentes na
   seleção;
2. o gate comparava apenas trip/board/alight antes de `mergeMissing`, permitindo
   combinar um snapshot parcialmente conflitante com o `SelectedResult`;
3. fila, cabeçalho do incidente e handoff mantinham declarações atuais
   concorrentes de rodadas diferentes.

A mudança permanece no boundary já existente de enrichment e não altera a
candidatura/body fechada, parser, regex, schema ou outros slices.

### Invariantes e mudança mínima

- uma única mensagem com `ProjectionMessageID` exato deve existir;
- se ela for `ABSENT_LEGACY`, somente seu `Selection.SelectedResult`
  reconciliado fornece campos;
- o prompt source é resolvido de forma exata e causal apenas para validar
  índice e trip/board/alight, nunca como fonte de enrichment;
- selection message, projection, prompt source e índice devem coincidir;
- todos os campos preenchidos no snapshot devem ser iguais aos da seleção;
  campo ausente pode ser completado, mas qualquer divergência mantém o valor
  original e remove `MaterializesAuthority`;
- evento desautorizado não produz `BOOKABLE` nem `BookingCreateInput`;
- `VALID_STRUCTURAL` continua positivo; `INVALID` e `UNDELIVERED` continuam
  sem enrichment;
- `EXECUTION_TRACKER.md` possui uma única declaração vigente na fila; todos os
  status cronológicos abaixo estão explicitamente superados. O handoff possui
  uma única declaração vigente no bloco inicial.

Produção foi alterada somente em
`availability_selection_state_v1.go`. Os REDs e controles ficaram em
`availability_selection_state_v1_test.go`; duas fixtures em
`booking_draft_context_test.go` e `conversation_state_machine_test.go` foram
alinhadas para persistir/esperar package exclusivamente na seleção. Tracker,
handoff e este plano registram a rodada.

### REDs e controles observados

Antes do patch:

- route/package/currency ausentes na projeção eram copiados de `Presented` do
  prompt source;
- conflitos em trip date, route, origin, destination, package, price,
  currency, trip, board e alight mantinham `MaterializesAuthority=true`;
- as suítes amplas expuseram duas expectativas legadas que ainda dependiam do
  package do source.

Depois do patch, os REDs cobrem fonte exclusiva, identidade em quatro partes,
compatibilidade integral, ausência versus igualdade, zero `BOOKABLE`/booking
em mismatch e os controles `VALID_STRUCTURAL`, `INVALID` e `UNDELIVERED`.

### Gates observados

Executados em `golang:1.23` via Docker:

```text
PASS — REDs e controles dirigidos pós-patch, count=20 — internal/chat 1.071s
PASS — go test -race -count=1 ./internal/chat — 49.026s
PASS — regressões transversais — 7.418s
PASS — go test -count=1 ./internal/chat — 7.587s
PASS — go test -count=1 ./... — internal/chat 9.007s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário de produção = 54 regexp.MustCompile
PASS — plano stale ausente; plano canônico presente
PASS — git diff --check; git diff --cached --check
```

O status continua **EM CORREÇÃO APÓS REVIEW — 2 P1 + 1 P2 DE
PROVENIÊNCIA EXCLUSIVA, COMPATIBILIDADE E STATUS CANÔNICO**. Evidência local
verde não declara segurança para commit. Não houve B2, 3.6F-D, parser, regex,
migration, refactor amplo, commit, push, PR, deploy ou smoke. Próxima ação
única: novo `/review`; H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

## Fase 7 — Histórico superado pela Fase 8: source explícito e presença persistida

### Escopo e causas raiz

O review seguinte confirmou os gates locais da Fase 6 e encontrou somente os
dois P1 e o P2 autorizados neste `/goal`:

1. `availability_prompt_source_message_id` vazio ou ausente no
   `SelectedResult` ainda acionava uma busca histórica por source compatível;
2. `price=0`, `seats_available=0` e strings vazias persistidas eram
   indistinguíveis de chave ausente no gate e no merge;
3. `AGENTS.md` duplicava um status operacional volátil já governado pelo
   tracker.

A correção permaneceu no enrichment e no decode de seleção. Não alterou
candidatura/body canônico, parser, regex, migration, schema público, B2,
3.6F-D ou outro contrato fechado.

### Invariantes e implementação observada

- o `SelectedResult` `ABSENT_LEGACY` precisa carregar source ID não vazio e
  exatamente igual ao ID esperado pelo evento;
- o resolver por ID continua exigindo unicidade e causalidade, mas não existe
  mais scan para descobrir source quando o ID está ausente;
- prompt source apenas valida identidade, índice e trip/board/alight; nenhum
  field do snapshot é copiado dele;
- uma máscara interna de presença cobre os 20 campos consumidos pelo gate e
  pelo merge, sem alterar JSON público ou schema persistido;
- decode de evento persistido preserva chave ausente, zero numérico e string
  vazia como estados distintos;
- campo presente exige presença e igualdade no `SelectedResult`; divergência
  mantém o snapshot original e remove `MaterializesAuthority` antes de
  `BOOKABLE` ou `booking_create`;
- campo ausente pode ser preenchido, inclusive quando o valor candidato é o
  zero-value legítimo;
- fixtures legadas positivas passaram a persistir o source ID explícito;
- `AGENTS.md` contém somente a regra permanente de consultar o tracker.

### REDs e controles observados

Antes do patch, os testes dirigidos reproduziram:

- source ID explicitamente vazio salvo por um prompt compatível no histórico;
- source ID omitido salvo pelo mesmo fallback;
- `price=0`, `seats_available=0` e `package_name=""` sobrescritos pelo
  `SelectedResult`, mantendo autoridade.

Depois do patch, os controles cobrem ID omitido/vazio/divergente/exato,
duplicidade, histórico compatível incapaz de inferir ID, ausência versus
zero/vazio presente, presente igual, os 20 campos do snapshot, reload,
redução e zero `BookingCreateInput` em conflito.

### Gates observados

Executados em `golang:1.23` via Docker:

```text
PASS — REDs dirigidos pós-patch, count=20 — internal/chat 0.884s
PASS — go test -race -count=1 ./internal/chat — 60.441s
PASS — regressões transversais — chat 11.605s; demais pacotes verdes
PASS — go test -count=1 ./internal/chat — 11.477s
PASS — go test -count=1 ./... — internal/chat 11.110s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário de produção = 54 regexp.MustCompile
PASS — git diff --check; git diff --cached --check
```

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Esta evidência local não
declara review limpo nem segurança para commit. Não houve commit, push, PR,
deploy ou smoke. Próxima ação única: novo `/review`; H-2026-07-16B2 e 3.6F-D
permanecem bloqueadas.

## Fase 8 — Executada localmente, aguardando review: barreira, tipos e serialização live

### Escopo e causas raiz

O review seguinte confirmou os gates da Fase 7 e encontrou somente os 3 P1 e
o P2 autorizados neste `/goal`:

1. o resolver por ID não verificava uma barreira `INVALID` posterior ao source;
2. a máscara marcava a chave presente antes de validar `null` ou o tipo;
3. o wrapper live descartava a máscara e `omitempty` removia zeros na
   persistência;
4. uma decisão histórica do tracker ainda se apresentava como canônica e
   carregava contagem superada.

### Invariantes e implementação observada

- source explícito, único e causal não atravessa a barreira `INVALID` central;
- os 20 campos do snapshot possuem classe de tipo auditável;
- chave ausente não marca presença; chave presente só marca após tipo válido;
- `null`, tipo incorreto, inteiro fracionário e float não finito falham
  fechados antes de `BOOKABLE` ou `BookingCreateInput`;
- a máscara acompanha o evento materializado live e o `MarshalJSON` interno
  persiste exatamente as chaves presentes;
- zero/vazio presente sobrevive ao round-trip, enquanto chave ausente continua
  omitida;
- a decisão antiga do tracker está explicitamente histórica/superada e sem
  contagem operacional concorrente.

### REDs e controles observados

Antes do patch, os testes reproduziram `BOOKABLE` com source anterior a
`INVALID`, decode de `null`/tipos errados como zero-value presente e perda de
`price=0`/`seats_available=0` no JSON. Depois do patch, os controles cobrem
fail closed e zero `booking_create`, as três classes de campo, zero/vazio
presente versus ausente e live → serialização → reload.

### Gates observados

Executados em `golang:1.23` via Docker:

```text
PASS — REDs dirigidos pós-patch, count=20 — internal/chat 0.105s
PASS — go test -race -count=1 ./internal/chat — 63.006s
PASS — regressões transversais — chat 11.888s; demais pacotes verdes
PASS — go test -count=1 ./internal/chat — 11.847s
PASS — go test -count=1 ./... — internal/chat 11.156s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário de produção = 54 regexp.MustCompile
PASS — git diff --check; git diff --cached --check
```

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Esta evidência local
não declara review limpo nem segurança para commit. Não houve B2, 3.6F-D,
parser, regex, migration, refactor amplo, commit, push, PR, deploy ou smoke.
Próxima ação única: novo `/review`; H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.

## Fase 9 — Executada localmente, aguardando review: limite causal da projeção

### Escopo e causa raiz

O review seguinte confirmou os gates da Fase 8 e encontrou um único P1. O
caller do bootstrap legado possuía `historyIndex` da projection e
`selectionHistoryIndex` da selection, mas enviava apenas o segundo ao helper
de source/barreira. Isso impedia o gate central de observar `INVALID` entre a
selection e a materialização.

### Invariantes e implementação observada

- `sourceBeforeIndex` limita a resolução única/causal do source antes da
  selection;
- `materializationIndex` limita a busca central de barreira no instante da
  projection;
- nenhuma barreira aplicável entre source e projection permite
  `MaterializesAuthority`, `BOOKABLE`, booking draft ou `BookingCreateInput`;
- barreira posterior à projection não é retroativa no helper e continua
  invalidando o estado final pelo caminho canônico existente;
- `UNDELIVERED` e mensagens passenger/payment/document fora do domínio availability
  não se tornam barreiras;
- source novo posterior a uma barreira antiga continua formando autoridade;
- source ID exato, tipos estritos, presence, round-trip e reconciliação de
  domínio permanecem inalterados.

### REDs e controles observados

Antes do patch, somente `[source, selection, INVALID, projection]` falhou:
o evento materializava autoridade. Depois do patch, uma tabela cobre as seis
ordens obrigatórias, incluindo estado, booking draft e zero booking input nos
caminhos fechados.

### Gates observados

Executados em `golang:1.23` via Docker:

```text
PASS — REDs dirigidos pós-patch, count=20 — internal/chat 1.279s
PASS — go test -race -count=1 ./internal/chat — 65.178s
PASS — regressões transversais — chat 13.341s; demais pacotes verdes
PASS — go test -count=1 ./internal/chat — 13.056s
PASS — go test -count=1 ./... — internal/chat 13.078s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário de produção = 54 regexp.MustCompile
PASS — git diff --check; git diff --cached --check
```

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Esta evidência local
não declara review limpo nem segurança para commit. Não houve B2, 3.6F-D,
parser, regex, migration, refactor amplo, commit, push, PR, deploy ou smoke.
Próxima ação única: novo `/review`; H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.
