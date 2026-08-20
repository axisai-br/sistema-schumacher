# Execution Tracker Canônico — sistema-schumacher

> Fonte de verdade operacional para ordem, status e gates dos próximos slices.
>
> O histórico anterior permanece útil como registro, mas não define a fila de execução.

---

## 1. Autoridade e uso

### 1.1 Fontes canônicas

Este tracker foi reconciliado com:

```text
AGENTS.md
plans/README.md
plans/00-plano-mestre-travel-semantic-v2.md
plans/<slice>.md
```

Em caso de dúvida:

1. `AGENTS.md` define as regras gerais do repositório.
2. Este tracker define a ordem e o status canônico dos slices.
3. `plans/README.md` e o plano mestre definem a sequência arquitetural.
4. O arquivo do slice atual define escopo, critérios de aceite, testes e review daquele slice.

### 1.2 Regras operacionais

- Trabalhar somente na primeira etapa marcada como `PRÓXIMA`.
- Executar um único slice por `/goal` e por PR.
- Não avançar automaticamente para o slice seguinte.
- Abrir apenas o plano do slice autorizado para execução.
- Corrigir todos os achados P1/P2 antes de considerar o slice concluído.
- Atualizar este tracker ao final de cada slice com status, arquivos, testes, review, necessidade de teste em produção e próxima ação.
- Não fazer commit, push, deploy ou alteração de ambiente sem pedido explícito.
- Recomendações antigas do tracker histórico não reabrem trabalho nem alteram esta fila.

### 1.3 Significado dos status

| Status | Significado |
|---|---|
| `PRÓXIMA` | Única etapa autorizada para o próximo `/goal` de execução. |
| `BLOQUEADA por <slice>` | Aguarda conclusão e atualização canônica do predecessor indicado. |
| `PENDENTE após <slice>` | Está ordenada, mas ainda não foi liberada. |
| `PENDENTE` | Pertence à sequência, condicionada à conclusão dos predecessores. |
| `FUTURO CONDICIONAL` | Não integra a fila atual; exige decisão posterior baseada em evidência. |

---

## 2. Estado arquitetural de referência

### 2.1 Capacidades já existentes

```text
Evolution webhook
→ ingest/buffer
→ Service.Reprocess
→ roteador determinístico
→ ActivePromptContext
→ interpreter estruturado local V1
→ OpenAI structured interpreter V1
→ shadow OpenAI V1
→ validator local V1
→ runtime assist V1 gated
→ templates fechados
→ auto-send controlado
→ tools operacionais
```

Também já existem no fluxo atual:

```text
seleção contextual de disponibilidade com facts atuais
proteção contra facts stale ou invisíveis
persistência atômica de selected_option_index + selected_availability_result
rejeição específica de opção ou data
out-of-turn informational replies
suporte administrativo de notas e financeiro
estado canônico e snapshot versionado de passageiros/documentos
readiness única para booking_create
observabilidade de shadow e runtime assist V1
```

### 2.2 Lacunas que motivam a fase V2

O contrato V1 não representa de forma explícita:

```text
papel de uma localidade como origem, destino, via ou referência próxima
preferência temporal EARLIEST_AVAILABLE ou ANY_AVAILABLE
consulta de cobertura de rota
consulta de parada próxima
referência de opção por índice, data ou deíxis
pedido de passagem versus escolha de poltrona específica
tema institucional estruturado
força STRONG, WEAK ou FALLBACK de uma decisão determinística
```

Consequências conhecidas:

- o provider pode compreender a frase sem possuir campos para expressar o significado;
- broad state pode vencer uma intenção explícita do turno atual;
- novas variações de linguagem pressionam o crescimento de helpers e regex;
- retrieval não corrige a ausência de contrato ou de validação factual.

### 2.3 Dívidas de fundação antes da fase V2

- As fixtures temporais citadas no P0-B precisam se tornar determinísticas sem alterar regras de produção.
- A publicação da API precisa do gate de testes definido no P0-C.
- Vector/File Search não deve anteceder contrato, validator, corpus, shadow e métricas V2.

---

## 3. Decisão arquitetural canônica

```text
Contrato pertence ao domínio Schumacher.
OpenAI interpreta linguagem humana.
Validator local verifica invariantes e fatos.
Backend executa.
```

Não criar um parser determinístico geral para linguagem aberta.

O interpreter local permanece como:

```text
fast-path de alta precisão
override para segurança
fallback de indisponibilidade do provider
comparador em shadow
```

O V2 deve nascer paralelo ao `StructuredInterpretation` V1. Nenhum slice inicial substitui o V1 diretamente.

### 3.1 Fluxo alvo

```text
mensagem atual
+ estado canônico
+ active prompt
+ facts atuais
→ fast-path e guardrails locais
→ OpenAI Travel Interpreter V2 em structured output
→ validator local por invariantes
→ decisão segura
→ template fechado ou tool read-only autorizada
→ backend executa
```

OpenAI não cria reservas, pagamentos ou cancelamentos, não executa tools diretamente e não é fonte de verdade para fatos operacionais.

---

## 4. Contrato futuro Travel Semantic V2

### 4.1 Agregado semântico

O contrato paralelo deve usar o nome `TravelQueryMeaningV2`:

```go
type TravelQueryMeaningV2 struct {
    Intent             TravelQueryIntent
    TurnMeaning        TurnMeaning
    Origin             *LocationMeaning
    Destination        *LocationMeaning
    MentionedLocations []LocationMeaning
    DatePreference     DatePreference
    OptionReference    OptionReference
    RouteCoverage      RouteCoverageMeaning
    SeatRequest        SeatRequestMode
    InstitutionalTopic InstitutionalTopic
    NeedsClarification bool
    MissingFields      []string
    Confidence         float64
    Reasons            []string
}
```

Regras do agregado:

- `MentionedLocations` é uma lista.
- `ACKNOWLEDGEMENT` pertence a `TurnMeaning`, não a `TravelQueryIntent`.
- O contrato não contém IDs operacionais executáveis.
- Confidence não substitui evidência factual nem validator local.
- Schema strict não autoriza side effect.

### 4.2 Papéis de localidade, data e opção

```go
type LocationRole string

const (
    LocationRoleUnknown         LocationRole = "UNKNOWN"
    LocationRoleOrigin          LocationRole = "ORIGIN"
    LocationRoleDestination     LocationRole = "DESTINATION"
    LocationRoleVia             LocationRole = "VIA"
    LocationRoleNearbyReference LocationRole = "NEARBY_REFERENCE"
)

type DateMode string

const (
    DateModeUnspecified       DateMode = "UNSPECIFIED"
    DateModeExact             DateMode = "EXACT"
    DateModeEarliestAvailable DateMode = "EARLIEST_AVAILABLE"
    DateModeAnyAvailable      DateMode = "ANY_AVAILABLE"
)

type OptionReferenceKind string

const (
    OptionReferenceNone    OptionReferenceKind = "NONE"
    OptionReferenceIndex   OptionReferenceKind = "INDEX"
    OptionReferenceDate    OptionReferenceKind = "DATE"
    OptionReferenceDeictic OptionReferenceKind = "DEICTIC"
)
```

### 4.3 Cobertura de rota

`RouteCoverageMeaning` representa a proposta semântica de cobertura ou proximidade. Seu shape e seus enums devem ser fechados no 3.6F-A; sua validação ocorre no 3.6F-B e o lookup read-only somente no 3.6F-H.

O contrato deve distinguir, no mínimo:

```text
parada exata
referência próxima
local consultado
necessidade de esclarecimento
```

Existência no catálogo não prova cobertura na rota, direção ou data solicitada. Não afirmar “ponto mais próximo” sem geodados e regra operacional confiáveis.

### 4.4 Poltrona

```go
type SeatRequestMode string

const (
    SeatRequestNone               SeatRequestMode = "NONE"
    SeatRequestBookTravel         SeatRequestMode = "BOOK_TRAVEL"
    SeatRequestChooseSpecificSeat SeatRequestMode = "CHOOSE_SPECIFIC_SEAT"
)
```

`CHOOSE_SPECIFIC_SEAT` não autoriza seleção de viagem, handoff real nem `booking_create`. O tratamento futuro usa template fechado de suporte e oferece continuação contextual da reserva normal.

### 4.5 Tema institucional

`InstitutionalTopic` é um enum fechado para classificar perguntas institucionais sem convertê-las em tabela pública de preços. Os valores exatos e invariantes pertencem ao 3.6F-A e ao validator do 3.6F-B.

Sem fonte canônica, a resposta deve encaminhar para suporte; o sistema não pode inventar sede ou informação institucional.

### 4.6 Força da decisão

```go
type DecisionStrength string

const (
    DecisionStrengthStrong   DecisionStrength = "STRONG"
    DecisionStrengthWeak     DecisionStrength = "WEAK"
    DecisionStrengthFallback DecisionStrength = "FALLBACK"
)
```

Regras:

- `STRONG`: cancelamento explícito, humano explícito, documento/mídia no fluxo correto, guardrails de pagamento e índice válido em lista atual.
- `WEAK`: broad state, unsupported inferido e inferência de tabela pública.
- `FALLBACK`: fallback contextual.
- O V2 somente poderá arbitrar `WEAK` ou `FALLBACK` após os gates da fila.
- `STRONG` nunca pode ser sobrescrito.

---

## 5. Fila canônica de execução

| Ordem | Slice | Status canônico | Plano | Objetivo resumido |
|---:|---|---|---|---|
| 1 | P0-A | **CONCLUÍDA — PASS CONTROLADO** | `plans/p0-a-reconciliar-deploy-smoke.md` | review final concluído sem P1/P2. |
| 2 | P0-B | **CONCLUÍDA** | `plans/p0-b-corrigir-fixtures-temporais.md` | fixtures temporais estabilizadas. |
| 3 | P0-C | **CONCLUÍDA — GATE REMOTO VALIDADO** | `plans/p0-c-ci-test-gate.md` | CI bloqueia publicação quando testes falham. |
| 4 | 3.6F-A | **CONCLUÍDA — DEPLOY CONFIRMADO** | `plans/3.6f-a-contrato-travel-query-meaning-v2.md` | contrato V2 local. |
| 5 | 3.6F-B | **CONCLUÍDA — REVIEW FINAL SEM P1/P2** | `plans/3.6f-b-validator-v2.md` | validator factual V2. |
| 6 | 3.6F-C | **CONCLUÍDA EM CÓDIGO — GATE OPERACIONAL REABERTO** | `plans/3.6f-c-openai-v2-shadow.md` | review local limpo; smoke real não criou claims e recovery falhou. |
| 7 | H-2026-07-16A | **CONCLUÍDO — SMOKE OPERACIONAL VERDE** | `plans/h-2026-07-16a-travel-v2-shadow-operacional.md` | 8 claims reais terminalizados em `COMPLETED`; sem novo `sweep_failed`. |
| 8 | H-2026-07-16B | **EM ANDAMENTO — gate operacional de B1 reaberto** | `plans/h-2026-07-16b-passenger-child-state.md` | Umbrella não executável; H-2026-07-27A interrompe a fila antes de B2. |
| 9 | H-2026-07-16B1 | **CONCLUÍDA EM CÓDIGO — GATE OPERACIONAL REABERTO por H-2026-07-27A** | `plans/h-2026-07-16b1-passenger-state-foundation.md` | H-2026-07-22A corrigiu o bootstrap e foi deployado; o smoke reabriu o gate na autoridade de opções apresentadas. |
| 10 | H-2026-07-22A | **CORRIGIDO E DEPLOYADO — SMOKE OPERACIONAL RED** | `plans/h-2026-07-22a-fresh-session-passenger-gate.md` | o problema original foi removido, mas o smoke falhou na transição availability → passageiros. |
| 11 | H-2026-07-27A | **EM CORREÇÃO APÓS REVIEW — 1 P1 CORRIGIDO LOCALMENTE; AGUARDANDO NOVO REVIEW** | `plans/h-2026-07-27a-structural-reconciliation-fixes.md` | o gate temporal recebe separadamente o limite causal do source e o índice da projeção/materialização; `INVALID` entre selection e projection falha fechado sem tornar barreira posterior retroativa; próxima ação única: novo `/review`. |
| 12 | H-2026-07-16B2 | **BLOQUEADA por H-2026-07-27A / gate operacional de H-B1** | `plans/h-2026-07-16b2-passenger-meaning-v1.md` | meaning strict só pode iniciar após review, deploy e smoke verdes do hotfix ativo. |
| 13 | H-2026-07-16B3 | **BLOQUEADA por H-2026-07-16B2** | `plans/h-2026-07-16b3-passenger-meaning-runtime.md` | promoção gated sem booking/payment direto. |
| 14 | 3.6F-D | **BLOQUEADA por H-2026-07-16B** | `plans/3.6f-d-corpus-evaluator-v2.md` | corpus/evaluator V2 somente após fechamento integral do umbrella H-B. |
| 15 | 3.6F-E | **PENDENTE após 3.6F-D** | `plans/3.6f-e-observabilidade-v2.md` | métricas V2 sanitizadas e read-only. |
| 16 | 3.6F-F | **PENDENTE** | `plans/3.6f-f-templates-seguros.md` | templates seguros. |
| 17 | 3.6F-G | **PENDENTE** | `plans/3.6f-g-earliest-available.md` | `EARLIEST_AVAILABLE` read-only. |
| 18 | 3.6F-H | **PENDENTE** | `plans/3.6f-h-route-coverage.md` | cobertura de rota read-only. |
| 19 | 3.6F-I | **PENDENTE** | `plans/3.6f-i-arbitragem-runtime-weak.md` | arbitragem gated sobre `WEAK`/`FALLBACK`. |

A fila acima é a única declaração canônica vigente. Status e próximas ações
registrados nas seções cronológicas abaixo são históricos e estão
**SUPERADOS** por esta tabela, salvo indicação explícita de que atualizam a
própria fila.

### Regra de desbloqueio

Um predecessor só libera o sucessor quando:

```text
escopo concluído
testes exigidos executados
review sem P1/P2 pendente
tracker atualizado
```

A liberação altera somente o próximo status; não autoriza executar dois slices no mesmo `/goal` ou PR.

Para mudanças em runtime, banco, worker ou deploy, review local limpo não substitui smoke obrigatório. Evidência operacional pode reabrir o gate e inserir hotfix antes do sucessor.

---

## 6. Gates específicos da fila

### Fundação P0

- P0-B pode alterar testes, helpers de fixture e clock controlável; não pode mudar regras de disponibilidade em produção.
- P0-C pode alterar o workflow de publicação da API; não pode mudar tags, registry, secrets, Dockerfile, Swarm ou deploy.

### Contrato, validator e shadow

- 3.6F-A cria tipos e testes puros; não altera V1, router, `Service.Reprocess`, tools, templates, state ou flags.
- 3.6F-B é puro/local; não chama provider, router, runtime ou tools.
- 3.6F-C preserva `store=false`, `tools=[]`, no máximo uma chamada V2 por turno e zero efeito user-visible.
- 3.6F-D usa corpus sintético ou anonimizado e não alimenta a proposta com o expected.
- 3.6F-E é read-only, exige `session_id`, usa allowlists e não expõe body ou PII.

### Influência segura e tools read-only

- 3.6F-F não chama tools e não altera booking, pagamento, documento ou cancelamento.
- 3.6F-G consulta apenas viagens futuras, não seleciona automaticamente, não avança para passageiros e não chama `booking_create`.
- 3.6F-H usa consulta interna read-only, sem migration ou endpoint público, e considera rota, direção e viagem futura ativa.
- 3.6F-I permanece atrás de flag desligada por padrão, não sobrescreve `STRONG` e não libera tools críticas, vector ou planner.

---

## 7. Invariantes globais

Nenhum slice pode quebrar:

```text
1 inbound → no máximo 1 resposta outbound final
facts atuais vencem fatos antigos
mensagem invisível não vira contexto visível
seleção exige facts atuais confiáveis
selected_option_index nunca existe sozinho para booking
rejeição do usuário bloqueia somente o alvo correto
nova availability posterior pode substituir blocker antigo
booking_create exige readiness canônica
documentos OUTBOUND não viram evidência
OpenAI usa store=false e tools=[]
OpenAI não executa tool diretamente
schema strict não substitui validator factual
H-012 permanece verde
```

Guardrails `STRONG` permanentes:

```text
cancelamento explícito
pedido explícito de humano
mídia ou documento no fluxo documental
índice válido em lista atual
sinal ou integral no prompt correto
CPF no prompt correto
guardrails de pagamento
bloqueios de segurança
```

Gate absoluto para promoção runtime:

```text
critical_action_violation_count = 0
```

---

## 8. Validação e atualização do tracker

### 8.1 Validação padrão para mudanças em chat

```bash
cd apps/api
go test -count=1 ./internal/chat -run '<testes focados>'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

Enquanto a baseline temporal ainda não estiver restaurada, separar explicitamente falhas preexistentes de regressões do slice. Nunca alterar produção apenas para acomodar fixture antiga.

### 8.2 Campos obrigatórios após executar um slice

```text
status
arquivos alterados
comportamento antes/depois
testes executados e resultados
resultado do review
necessidade de teste em produção
riscos restantes
próxima ação única
```

Não marcar etapa como concluída sem evidência correspondente ao critério de aceite do seu plano.

### 8.3 Registro operacional — P0-A (2026-07-14)

**Status:** **CONCLUÍDA — PASS CONTROLADO**.

O review final foi concluído sem achados P1/P2. Não permanece status ou ação de review pendente no P0-A.

#### Baseline e deploy

```text
HEAD local e origin/main nesta execução: eacacd35350e415cd2470b157968db8f6d2a34c6
SHA implantado verificado anteriormente em 2026-07-14: d5e9ae9ef57333fb309d2a8447a76ba0f5332a60
serviço: schumacher-api_schumacher-api, 1/1
imagem: ghcr.io/joaovitormessias/sistema-schumacher-api:main
digest implantado: sha256:d9db37a9f1313dfa65df4b320a33409779513f46fc88508c18ac0f21075e0fde
```

O commit `eacacd3` contém somente a canonicalização documental posterior ao deploy. `git diff --stat d5e9ae9..eacacd3 -- apps/api` não apresentou diferenças; portanto o código da API exercitado deterministicamente abaixo é o mesmo do SHA implantado. A nova tentativa read-only de inspecionar o Swarm exigiu senha de `sudo` e foi interrompida sem alteração de estado; o SHA, a imagem, o digest e as flags implantadas abaixo são a evidência operacional já verificada anteriormente nesta data.

#### Flags efetivas e saúde

```text
CHAT_OPENAI_INTERPRETER_SHADOW_ENABLED=true
CHAT_OPENAI_INTERPRETER_ASSIST_ENABLED ausente -> false pelo config
CHAT_AGENT_MODE ausente -> legacy pelo config
GET https://api.schumachertursc.com.br/health -> 200 {"status":"ok"}
GET https://api.schumachertursc.com.br/ready -> 200 {"status":"ready"}
```

Os fallbacks de flags foram conferidos em `apps/api/internal/shared/config/config.go`: booleano ausente é `false` e `CHAT_AGENT_MODE` usa `legacy` como default.

#### Smoke H-012 implantado até a fronteira sem side effect

| Passo | Evidência persistida |
|---|---|
| opção `1` | `ASK_PASSENGER_COUNT`, `selected_option_index=1` |
| `só pra mim` | `ASK_CHILD_UNDER_5` |
| `sim` | `ASK_PASSENGER_DOCUMENTS`, `expected_document_count=2` |
| documento adulto sintético | snapshot `1/2`; permaneceu em `ASK_PASSENGER_DOCUMENTS` |
| documento criança sintético | snapshot `2/2`; avançou para `ASK_LAP_CHILD_ASSIGNMENT` |
| atribuição `2` | snapshot v1 com adulto + criança, fonte `EXPLICIT_ASSIGNMENT`; `CONFIRM_EXTRACTED_DOCUMENT` |
| confirmação final `sim` | não executada em produção; zero chamadas de `booking_create` na sessão implantada |

O smoke implantado chegou corretamente a `CONFIRM_EXTRACTED_DOCUMENT` e comprovou o bloqueio de `booking_create` antes da confirmação final. Ele não é usado isoladamente como prova de PASS.

#### Evidência controlada do gate final no mesmo SHA

Um archive temporário exato de `d5e9ae9ef57333fb309d2a8447a76ba0f5332a60` executou:

```text
go test -count=1 ./internal/chat -run '^(TestReprocessSequentialChildThenAdultDocumentExtractMergesPassengers|TestReprocessLapChildAssignmentDraftsDocumentConfirmationBeforeBookingCreate)$' -v
PASS
ok schumacher-tur/api/internal/chat
```

Em conjunto, os dois testes comprovam somente:

- fluxo com dois passageiros;
- zero chamadas de `booking_create` antes da confirmação final;
- exatamente uma chamada de `booking_create` depois de uma confirmação final.

A composição das duas evidências classifica o P0-A como **PASS CONTROLADO**: o serviço implantado foi exercitado somente até a fronteira sem side effect, enquanto o gate final e a cardinalidade de uma única chamada foram comprovados em ambiente determinístico e seguro no mesmo SHA da API implantada.

#### Justificativa de segurança

A viagem e os stops sintéticos do smoke não existem nas tabelas operacionais; confirmar essa sessão em produção terminaria em falha e não provaria o caminho de sucesso. Substituí-los por uma viagem real faria `booking_create` persistir `bookings`, `passengers` e `booking_payment_details` e ocupar assentos reais. Como o alvo não possui dry-run ou sandbox configurado, a confirmação final em produção não foi executada.

#### Fechamento

```text
arquivos alterados: docs/EXECUTION_TRACKER.md
mudança funcional: nenhuma
testes Go: dois testes focados no archive exato do SHA implantado; PASS
verificações: git fetch --prune origin; HEAD/origin-main; diff de apps/api entre os SHAs; health; ready; git diff --check
resultado do review: review final concluído sem P1/P2
teste em produção: não executar confirmação final sem sandbox; fronteira sem side effect já verificada
riscos restantes: SHA/flags do Swarm não foram relidos após a exigência de sudo; permanecem registrados pela verificação anterior de 2026-07-14
próxima ação única: executar somente o Slice 3.6F-B mediante novo /goal explícito.
```

### 8.4 Registro operacional — P0-B (2026-07-14)

**Status:** **CONCLUÍDA**.

#### Evidência Git e review final

Na base atual `HEAD=42eb73c08ac331d28d3edf1fdec8cf50c96d43fc`:

```text
commit do P0-B: b3843585f7af0b285086a9a4e6c53ddcb86f0d9a
merge da PR #53: 7dbc6eef92edd82fe611c94e4d0bb511deb51da9
git merge-base b384358 HEAD: b3843585f7af0b285086a9a4e6c53ddcb86f0d9a
git merge-base --is-ancestor b384358 HEAD: exit 0
```

A PR #53 está `MERGED` e o commit `b384358` é o segundo pai do merge `7dbc6ee`, confirmando que a correção integra a base atual. O review final foi concluído sem achados P1/P2.

#### Baseline reproduzida e classificação

No worktree limpo anterior ao patch, a execução focada dos nove cenários apresentou oito testes falhando e `TestOpenAIInterpreterAssistSelectionTemplateDraftRequiresAtomicAttach` passando, embora também usasse as mesmas fixtures fixas. As falhas deslocavam índices e snapshots porque datas de 06/07 e 13–17/07/2026 já eram passadas ou representavam o dia da execução.

As baselines amplas anteriores ao patch confirmaram que não havia falha adicional fora desse conjunto:

```text
go test -count=1 ./internal/chat -> FAIL somente nos oito cenários temporais reproduzidos
go test -count=1 ./... -> FAIL somente em internal/chat pelos mesmos oito cenários
```

#### Estratégia aplicada

Os nove testes passaram a usar um clock de teste capturado em UTC e fixtures derivadas desse clock:

```text
passado explícito: observedAt - 7 dias
hoje explícito: observedAt
futuro imediato explícito: observedAt + 1 dia
opções futuras dos fluxos integrados: observedAt + 7 a + 11 dias
```

Datas de entrada, IDs de viagem e expectativas são derivados dos itens da fixture. O cenário de atomic attach valida passado, hoje e futuro com o mesmo `ObservedAt` capturado pelo teste. Nenhuma regra, filtro ou arquivo de produção foi alterado.

#### Defeito encontrado pelo review

O review encontrou um P2 na fronteira dezembro→janeiro: `availabilityTestDayMonth` gerava mensagens apenas em `dd/mm`, enquanto `extractTripDate` associa entradas sem ano ao ano de `observedAt`. Assim, uma viagem relativa em janeiro do ano seguinte podia ser interpretada como janeiro do ano anterior e deixar de resolver a opção futura correta. Os gates executados em 14/07 e com timezone alternativo não cobriam essa virada anual.

#### Correção dos achados P2

O helper de entrada dinâmica foi substituído por `availabilityTestDateInput`, que formata a mensagem como `dd/mm/aaaa`. O formato `dd/mm` permaneceu somente no helper separado de expectativa dos metadados de rejeição, pois esse contrato armazena dia e mês. Nenhuma chamada a `extractTripDate` ou código de produção foi alterada.

`TestAvailabilityDateSelectionAcrossYearBoundaryPreservesFutureYear` fixa `observedAt` em 27/12/2026, envia `03/01/2027`, exige a seleção da viagem futura de 2027 e confirma que o snapshot não usa 03/01/2026.

#### Arquivos alterados

```text
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/openai_interpreter_assist_test.go
docs/EXECUTION_TRACKER.md
```

#### Validação executada

Os comandos Go usaram `GOCACHE=/tmp/schumacher-go-build` e `GOTMPDIR=/tmp`. Uma tentativa intermediária de link falhou por quota de disco; após limpar somente o cache Go temporário, todas as execuções finais passaram:

```text
nove testes focados -> PASS
TestAvailabilityDateSelectionAcrossYearBoundaryPreservesFutureYear -> PASS
nove testes focados + fronteira anual com TZ=Pacific/Kiritimati -> PASS
go test -count=1 ./internal/chat -> PASS
go test -count=1 ./... -> PASS
git diff --check -> PASS
```

#### Fechamento

```text
comportamento antes: datas fixas alteravam a lista visível conforme o calendário real e deslocavam seleção, rejeição e snapshot
comportamento depois: categorias passado/hoje/futuro são relativas ao clock UTC, entradas dinâmicas preservam o ano e as expectativas acompanham a fixture
mudança funcional de produção: nenhuma
resultado do review: os dois achados P2 foram corrigidos; review final concluído sem P1/P2
teste em produção: não necessário; alteração exclusiva de testes e helpers de fixture
riscos restantes: nenhum dentro do escopo do P0-B; a fronteira anual foi coberta pelas validações UTC e Pacific/Kiritimati
próxima ação única: executar somente o Slice 3.6F-B mediante novo /goal explícito.
```

### 8.5 Registro operacional — P0-C (2026-07-14)

**Status:** **CONCLUÍDA — GATE REMOTO VALIDADO**.

A execução local deste slice ocorreu por novo `/goal` explícito na branch `ci/p0-c-test-gate`, baseada no commit `b384358` do P0-B. A PR #54 está `MERGED` em `42eb73c08ac331d28d3edf1fdec8cf50c96d43fc`, que é o `HEAD` da base atual; `git merge-base --is-ancestor 42eb73c HEAD` retornou exit 0.

#### Workflow antes e depois

Antes do slice, `.github/workflows/publish-api-ghcr.yml` possuía somente o job `publish-api`: não havia job de testes nem dependência anterior ao build/push da imagem.

O workflow agora possui o job `test-api`, que:

```text
faz checkout com actions/checkout@v4
configura o Go de apps/api/go.mod com actions/setup-go@v5
usa apps/api/go.sum como dependency path do cache
executa go test -count=1 ./internal/chat
executa go test -count=1 ./...
```

`publish-api` recebeu somente `needs: test-api`. Com isso, uma falha na suíte impede o job de publicação, enquanto o fluxo existente de checkout, Buildx, login no GHCR, metadata e build/push permanece inalterado. As tags `main` e SHA, as permissões `contents: read`/`packages: write`, o uso de `GITHUB_TOKEN`, o contexto, o Dockerfile e `workflow_dispatch` foram preservados. Nenhum job ou comando de deploy foi adicionado ou executado.

#### Arquivos alterados

```text
.github/workflows/publish-api-ghcr.yml
docs/EXECUTION_TRACKER.md
```

#### Validação local e remota executada

Os comandos Go locais usaram `GOCACHE=/tmp/schumacher-go-build` e `GOTMPDIR=/tmp`:

```text
go test -count=1 ./internal/chat -> PASS
go test -count=1 ./... -> PASS
ruby YAML.parse_file e checagem estrutural do gate em .github/workflows/publish-api-ghcr.yml -> PASS
git diff --check -> PASS
```

O GitHub Actions executou o workflow `Publish API to GHCR` no SHA de merge `42eb73c`:

```text
run: 29351723680
evento: push em main
status final do workflow: success
test-api: success; 2026-07-14T16:56:52Z → 2026-07-14T16:58:17Z
publish-api: success; 2026-07-14T16:58:20Z → 2026-07-14T16:59:26Z
```

`publish-api` iniciou somente depois da conclusão bem-sucedida de `test-api`, confirmando remotamente o encadeamento `test-api -> publish-api`. O workflow preserva `needs: test-api` no job `publish-api`.

#### Fechamento

```text
comportamento antes: build/push da API podia iniciar sem um gate anterior da suíte Go
comportamento depois: publish-api depende de test-api e só pode iniciar após os dois comandos Go passarem
mudança funcional de produção: nenhuma
resultado do review: review final concluído sem P1/P2; merge da PR #54 e gate remoto confirmados
teste em produção: não necessário; o gate foi validado no GitHub Actions e não executa deploy
riscos restantes: nenhum dentro do escopo do P0-C; o workflow publicou a imagem, sem alterar o fluxo de deploy
próxima ação única: executar somente o Slice 3.6F-B mediante novo /goal explícito.
```

### 8.6 Registro operacional — 3.6F-A (2026-07-14)

**Status:** **CONCLUÍDA — DEPLOY CONFIRMADO**.

#### Evidência Git, publicação e deploy

```text
commit do contrato 3.6F-A: b6020a3364e6e962f52e83df10aa12629bb1807b
merge da PR #55 em main: 32ab3bcc1ce12288d7c5f251a59b6ecc5f293aad
git merge-base --is-ancestor b6020a3 origin/main: exit 0
GitHub Actions run: 29355071178
test-api: success; 2026-07-14T17:45:13Z → 2026-07-14T17:46:03Z
publish-api: success; 2026-07-14T17:46:07Z → 2026-07-14T17:47:10Z
tags publicadas: main, sha-32ab3bc
digest publicado: sha256:0131fa77a8f372fb1ba25f60dab43f181e6e91d627dfab7fca1dd5df8e784ec1
serviço: schumacher-api_schumacher-api
deploy: concluído e confirmado pelo responsável operacional
health: HTTP 200 {"status":"ok"}
ready: HTTP 200 {"status":"ready"}
```

O job `publish-api` iniciou somente após o sucesso de `test-api`. A imagem contém o contrato local do 3.6F-A, mas o slice não possui integração runtime nem comportamento user-visible.

#### Contrato criado

O agregado local `TravelQueryMeaningV2` foi criado em paralelo ao `StructuredInterpretation` V1. O contrato contém:

```text
TravelQueryIntent
LocationMeaning e LocationRole
DatePreference e DateMode
OptionReference e OptionReferenceKind
RouteCoverageMeaning e RouteCoverageMode
SeatRequestMode
InstitutionalTopic
DecisionStrength
```

`TravelQueryMeaningV2` preserva origem, destino, lista de localidades mencionadas, preferência de data, referência de opção, proposta semântica de cobertura, modo de poltrona, tema institucional, necessidade de esclarecimento, campos ausentes, confidence e reasons. `ACKNOWLEDGEMENT` foi adicionado como `TurnMeaning`, não como `TravelQueryIntent`. `DecisionStrength` permanece um tipo separado do agregado, preservando a separação entre significado semântico e arbitragem futura.

O contrato não contém IDs operacionais, dependências externas, provider, banco, HTTP, parser, regex, execução de tool, persistência, resposta ao cliente ou integração runtime.

#### Invariantes testadas

```text
valores explícitos de todos os novos enums
estado zero não acionável
origem, destino, via e referência próxima com papéis explícitos
MentionedLocations preservada como lista
EXACT, EARLIEST_AVAILABLE e ANY_AVAILABLE
referência de opção por INDEX, DATE e DEICTIC
BOOK_TRAVEL distinto de CHOOSE_SPECIFIC_SEAT
pergunta institucional tipada
cobertura por parada exata e referência próxima
DecisionStrength STRONG, WEAK e FALLBACK distintos
necessidade de esclarecimento, missing fields, confidence e reasons
ACKNOWLEDGEMENT restrito a TurnMeaning
```

#### Arquivos alterados

```text
apps/api/internal/chat/travel_query_meaning_v2.go
apps/api/internal/chat/travel_query_meaning_v2_test.go
docs/EXECUTION_TRACKER.md
```

#### Validação executada

Os gates Go finais usaram o cache padrão fora do sandbox porque o cache temporário isolado atingiu a quota de disco durante as tentativas amplas:

```text
go test -count=1 ./internal/chat -run 'Test.*TravelQueryMeaningV2|Test.*Travel.*Contract|Test.*DecisionStrength' -> PASS
go test -count=1 ./internal/chat -> PASS
go test -count=1 ./... -> PASS
git diff --check -> PASS
```

#### Fechamento

```text
comportamento antes: não existia contrato V2 capaz de representar papéis de localidade, data relativa, referência de opção, cobertura, poltrona e tema institucional
comportamento depois: existe contrato Go local e puro para essas semânticas, sem consumo pelo fluxo atual
mudança funcional de runtime: nenhuma
resultado do review: review final concluído sem achados P1/P2
teste em produção: deploy, health e ready confirmados; não há cenário funcional específico porque o contrato não está integrado ao runtime
riscos restantes: a validação de enums, shapes e evidências pertence ao 3.6F-B e não bloqueia o fechamento do 3.6F-A
próxima ação única: executar somente o Slice 3.6F-B mediante novo /goal explícito
```

### 8.7 Registro operacional — 3.6F-B (2026-07-14)

**Status:** **CONCLUÍDA — REVIEW FINAL SEM P1/P2**.

O primeiro review canônico identificou 3 P1 e 6 P2. O segundo identificou
1 P1 e 3 P2. O terceiro identificou 4 P2, o quarto identificou 3 P2,
o quinto identificou 1 P1 e 2 P2 e o sexto identificou 2 P1 e 2 P2.
O sétimo review identificou 2 P1.
O oitavo review identificou 1 P2.
O nono review identificou inicialmente 2 P2; após as correções e a validação
local, o review final não encontrou P1/P2.

As correções dos cinco primeiros ciclos foram implementadas e testadas
localmente. O sexto review, porém, revelou um problema arquitetural:
o validator passou a reinterpretar linguagem natural em vez de apenas
validar contrato, fatos e invariantes.

O 3.6F-B está concluído e seguro para commit. Somente o 3.6F-C foi liberado
como próxima etapa; sua execução depende de novo `/goal` explícito.

#### Decisão arquitetural após o sexto review

Os achados revelaram que o validator passou a reinterpretar linguagem
natural por tokens, pontuação, caixa, conectores e janelas numéricas.

Decisão canônica:

- OpenAI interpreta linguagem humana;
- validator verifica contrato, fatos e invariantes;
- casos linguísticos adversariais pertencem ao corpus/evaluator;
- não serão adicionadas novas regras semânticas locais para corrigir
  os achados do sexto review;
- o código semântico criado nos ciclos anteriores foi removido do validator e
  retirado do caminho de aceitação;
- o 3.6F-C permaneceu bloqueado até o review final da refatoração
  arquitetural, concluído sem P1/P2.

#### API local criada

```text
ValidateTravelQueryMeaningV2(TravelQueryValidationInputV2) TravelQueryValidationResultV2
TravelQueryValidationStatus: ACCEPTED | REJECTED
TravelQueryValidationResultV2: somente Status + ReasonCodes
TravelQueryLocationEvidenceV2: nome canônico + StopID opcional
TravelQueryAvailabilityFactsV2: opções visíveis + identidade temporal da mensagem fonte
```

`TravelQueryValidationInputV2` recebe a proposta `TravelQueryMeaningV2`, `currentTurn`, estado canônico, `ActivePromptContext`, histórico, `observedAt`, facts visíveis de disponibilidade, catálogo de localidades/paradas e a força da decisão existente. `CurrentTurn` é consultado somente para rejeitar entrada vazia; seu conteúdo não descobre ou altera significado. A aceitação é exclusivamente semântica: o resultado não contém tool, template, mutação de estado, mensagem, seleção, booking, pagamento ou handoff.

#### Invariantes e reason codes

O validator cobre estado zero/intent desconhecido, enums fechados, confidence, coerência de clarification/missing fields, combinações semânticas, papéis e evidências de localidade, data exata, `EARLIEST_AVAILABLE`, referência de opção, facts atuais, cobertura, institucional, poltrona e preservação de `STRONG`.

```text
EMPTY_CURRENT_TURN
UNKNOWN_INTENT
INVALID_ENUM_VALUE
INVALID_CONFIDENCE
LOW_CONFIDENCE_REQUIRES_CLARIFICATION
INCOHERENT_CLARIFICATION
INCONSISTENT_SEMANTIC_COMBINATION
CONTRADICTORY_LOCATION_ROLE
LOCATION_NOT_IN_CATALOG
LOCATION_NOT_GROUNDED
OBSERVED_AT_REQUIRED
INVALID_EXACT_DATE
EXACT_DATE_IN_PAST
EARLIEST_AVAILABLE_ROUTE_REQUIRED
CURRENT_AVAILABILITY_FACTS_REQUIRED
CURRENT_AVAILABILITY_FACTS_INVALID
INVALID_OPTION_REFERENCE
OPTION_REFERENCE_MISMATCH
OPTION_INDEX_OUT_OF_RANGE
OPTION_DATE_NOT_VISIBLE
AMBIGUOUS_OPTION_REFERENCE
INVALID_ROUTE_COVERAGE
ROUTE_COVERAGE_EVIDENCE_REQUIRED
INVALID_SEAT_REQUEST
INVALID_INSTITUTIONAL_TOPIC
STRONG_DECISION_PROTECTED
```

A implementação anterior adicionou marcadores direcionais, matching lexical
de localidades, extração contextual de índices e reconhecimento de UF.
O sexto review demonstrou que essas regras estavam formando um segundo
interpreter local; esses mecanismos foram removidos do validator e de seus
testes unitários durante a correção arquitetural.

#### Histórico das tentativas anteriores

As seções abaixo preservam os achados e correções dos cinco primeiros ciclos.
Elas são registro histórico, não definição da arquitetura final.

As correções baseadas em parsing de frases, tokens, prefixos, caixa,
pontuação ou janelas numéricas foram retiradas da suíte factual e
reclassificadas para o shadow/corpus pelos planos 3.6F-C/D.

#### Achados do review canônico e correções

| Prioridade | Achado | Correção local |
|---|---|---|
| P1 | `INDEX` era aceito sem evidência explícita do índice | `INDEX` agora exige índice positivo explícito e compatível no turno; deíxis como `essa ai` não pode ser convertida silenciosamente, com uma ou várias opções. |
| P1 | grounding de origem/destino aceitava o endpoint oposto | estado canônico e opções visíveis agora fundamentam origem somente como origem e destino somente como destino. |
| P1 | opção visível incompleta podia ser tratada como fact confiável | toda opção agora exige `TripID`, `BoardStopID`, `AlightStopID` e data ISO válida não passada. |
| P2 | `""` era aceito nos enums discriminadores | `DateMode`, `OptionReferenceKind`, `RouteCoverageMode`, `SeatRequestMode` e `InstitutionalTopic` agora exigem sentinelas explícitas do contrato. |
| P2 | shape de `OptionReference` podia escapar pelo bypass de clarification | o shape discriminado de `NONE`, `INDEX`, `DATE` e `DEICTIC` é validado antes da coerência de clarification. |
| P2 | cidade duplicada podia receber UF arbitrária | cidade presente em mais de uma UF exige UF explícita no turno ou evidência inequívoca em estado/facts atuais. |
| P2 | coverage não era cruzado com `LocationRole` | `EXACT_STOP` exige `VIA`; `NEARBY_REFERENCE` exige `NEARBY_REFERENCE`, independentemente dos marcadores lexicais. |
| P2 | `option_reference` podia ser declarado ausente sem ambiguidade real | deíxis/data resolvida por uma única opção rejeita clarification falsa; ausência real ou múltiplas opções continuam esclarecíveis. |
| P2 | acknowledgement podia ser combinado com intent operacional | `ACKNOWLEDGEMENT` agora é rejeitado quando combinado com qualquer intent de viagem do agregado V2. |

#### Segundo review canônico e correções

| Prioridade | Achado | Correção local |
|---|---|---|
| P1 | presença dos dois endpoints no turno permitia inverter `Origin` e `Destination` | a construção explícita e catalogada `de A para B` agora fundamenta `A` somente como origem e `B` somente como destino; a regressão principal usa `Fraiburgo/SC → Santa Inês/MA`. |
| P2 | `ExistingDecisionStrength=""` era aceito | a allowlist agora contém somente `STRONG`, `WEAK` e `FALLBACK`; vazio e desconhecido são rejeitados, `STRONG` continua protegido. |
| P2 | cidade curta podia usar como grounding o prefixo de outra cidade | o matcher por tokens prioriza a correspondência canônica mais longa no mesmo ponto; `Santa Cecilia do Pavao/PR` não fundamenta `Santa Cecilia/SC`, enquanto `Santa Cecilia/SC` explícita continua válida. |
| P2 | clarification por `DATE` não possuía regressões dos dois ramos | uma única opção na data rejeita clarification falsa; duas opções na mesma data aceitam ambiguidade real; data ausente, inválida ou invisível continua rejeitada. |

#### Terceiro review canônico e correções

| Prioridade | Achado | Correção local e regressão |
|---|---|---|
| P2 | acknowledgement puro não podia ser validado | somente `TurnMeaning=ACKNOWLEDGEMENT` com `Intent=UNKNOWN`, payload de viagem vazio e sentinelas tipadas válidas é aceito; intents operacionais e payload em origem, destino, local mencionado, data, opção, coverage, poltrona, institucional ou clarification são rejeitados. |
| P2 | `State.Route` stale fundamentava endpoints antes dos facts atuais | endpoints das opções atuais, visíveis e confiáveis agora são autoritativos para origem/destino; a proposta dos facts é aceita apesar do estado antigo divergente e a rota fundamentada apenas pelo estado stale é rejeitada. |
| P2 | UF incompatível na cidade longa liberava fallback para o prefixo curto | o matcher fixa primeiro a maior sequência lexical catalogada e só então valida a UF; `Santa Cecilia do Pavao/SC` não fundamenta a cidade longa `/PR` nem `Santa Cecilia/SC`, enquanto as menções explícitas corretas da longa `/PR` e da curta `/SC` continuam aceitas. |
| P2 | `INDEX` exigia que o turno inteiro fosse um número | a validação extrai somente token inteiro positivo e delimitado, compatível com a opção proposta e dentro dos facts visíveis; aceita `quero a opção 2`, `pode ser a 2` e `escolho 2`, e rejeita ausência, zero, negativo, mismatch, fora do range, número em palavra/data/telefone e índices contraditórios. |

As regressões do terceiro review foram executadas primeiro contra o validator anterior e reproduziram os quatro defeitos antes do patch funcional. A extração de índice usa somente limites de tokens e separadores numéricos locais; não foi criado parser geral ou regex ampla.

#### Quarto review canônico e correções

| Prioridade | Achado | Correção local e regressão |
|---|---|---|
| P2 | clarification por `option_reference` aceitava um índice explícito resolvível no turno | antes de considerar a referência ausente, o validator cruza o índice com facts atuais; índice explícito único e in-range torna a clarification incoerente, enquanto índice ausente, contraditório ou fora do range continua esclarecível. |
| P2 | qualquer inteiro positivo isolado podia fundamentar `INDEX` | bare number permanece válido somente após o gate de prompt/facts atuais; respostas naturais exigem janela curta com contexto explícito de opção, preservando `quero a opção 2`, `pode ser a 2` e `escolho 2`, e rejeitando dia, idade, passageiros, data, telefone, CPF, valor, palavra e números contraditórios. |
| P2 | palavra minúscula após cidade podia ser consumida como sigla de UF | o matcher passou a preservar valor normalizado, caixa original e separador anterior; esse ciclo bloqueou `Santa Cecilia se tiver vaga`, mas o caso em caixa alta ainda exigiu o endurecimento registrado no quinto review. |

As regressões do quarto review foram executadas contra o validator anterior e reproduziram os três defeitos antes do patch funcional. A janela numérica e o token lexical com metadados mínimos permanecem locais e determinísticos, sem parser geral ou regex ampla.

#### Quinto review canônico e correções

| Prioridade | Achado | Correção local e regressão |
|---|---|---|
| P1 | `DATE`, `DEICTIC` ou clarification falsa podiam prevalecer apesar de índice explícito e resolvível no turno | o validator detecta primeiro o índice explícito contra os facts atuais; quando ele é único e in-range, somente `INDEX` com o mesmo valor é aceito. As regressões preservam o `INDEX` compatível e rejeitam `DATE`, `DEICTIC`, `NONE` com clarification e índice incompatível. |
| P2 | a extração numérica varria o turno inteiro e deixava data, quantidade ou outro número competir com o índice | respostas naturais agora coletam somente números em janelas locais de referência de opção; `opção 2 no dia 15/07/2030` e `opção 2 para 3 passageiros` preservam o índice 2, enquanto alternativas coordenadas como `opção 1 ou 2` permanecem ambíguas e números sem contexto de opção são ignorados. |
| P2 | `SE` em caixa alta e separado somente por espaço ainda podia ser consumido como UF | a UF adjacente à cidade exige delimitador explícito; somente `/SE`, `- SE`, `, SE` e `(SE)` são consumidos, e tanto `se` quanto `SE` conjuntivos sem delimitador são rejeitados como evidência de Sergipe. |

As regressões do quinto review cobrem a precedência factual do índice, a limitação da janela numérica e as formas aceitas/rejeitadas de UF. As correções permanecem locais ao validator puro e não adicionam parser geral, provider, catálogo, integração runtime ou mudança de contrato.

#### Sexto review canônico e decisão de refatoração

| Prioridade | Achado | Destino correto |
|---|---|---|
| P1 | `opção 1 ou 2` podia ser resolvida silenciosamente por DATE | interpretação de ambiguidade pertence ao OpenAI V2 e ao corpus do 3.6F-D |
| P1 | `opção 3` fora do range podia ser reinterpretada como DATE | o interpreter propõe INDEX; o validator apenas valida range e facts |
| P2 | `daqui a 2 dias` podia fundamentar INDEX=2 | papel linguístico do número pertence ao interpreter |
| P2 | cidade homônima podia ser fundamentada por facts coletivamente ambíguos | validator deve exigir fact estruturado inequívoco ou clarification |

O sexto review não será corrigido com novas regras lexicais. Ele abriu uma
refatoração da fronteira entre interpreter e validator.

#### Refatoração arquitetural implementada

- removidos parsing `de A para B`, marcadores de origem/destino/via/proximidade,
  tokenização lexical, prefix matching e interpretação de UF;
- removidas extração de índice, janelas numéricas, classificação de números e
  qualquer comparação entre `CurrentTurn` e `OptionReference`;
- removida interpretação local de deíxis; `DEICTIC` já estruturado é validado
  somente pela cardinalidade dos facts e pela clarification;
- `INDEX` já estruturado exige valor positivo, facts atuais confiáveis e range;
- `DATE` já estruturada exige shape ISO e ocorrência nos facts atuais, com
  clarification quando mais de uma opção compartilha a data;
- origem/destino são cruzados com catálogo, state ou endpoints dos facts; facts
  atuais vencem state antigo;
- quando várias opções possuem endpoints diferentes, elas não fundamentam
  coletivamente uma cidade homônima; um `INDEX` válido limita o grounding à
  opção resolvida;
- nomes canônicos com UF inexistente no catálogo não fazem fallback para outra
  UF da mesma cidade;
- testes linguísticos foram removidos da suíte do validator e registrados nos
  planos 3.6F-C/D; os testes locais agora cobrem contrato, facts e invariantes.

Casos movidos para o plano do corpus 3.6F-D:

- `opção 1 ou 2`;
- `opção 3`;
- `daqui a 2 dias`;
- `Santa Cecilia se tiver vaga`;
- cidades homônimas com e sem fact inequívoco.

#### Sétimo review canônico e correções (2026-07-15)

| Prioridade | Achado | Correção local e regressão |
|---|---|---|
| P1 | o lookup de identidade canônica ainda chamava `normalizeCanonicalLocationKey`, que aplica `NormalizeIncomingCustomerText` e convertia `Freiburg/SC` em `Fraiburgo/SC` | o validator passou a usar normalização canônica dedicada e estável, limitada a trim, caixa, acentos e separador; ASR, aliases, fuzzy, fonética, prefixos, substituições e fallback de cidade ficam proibidos nesse lookup. `Freiburg/SC` agora é rejeitada quando somente `Fraiburgo/SC` existe no catálogo, e a identidade canônica correta continua aceita. |
| P1 | endpoints de facts atuais eram fundamentados por `OriginDisplayName`/`DestinationDisplayName`, ignorando `OriginStopID`/`DestinationStopID` | cada StopID agora precisa resolver de forma única no catálogo e é a identidade autoritativa do endpoint. Display serve somente para apresentação/consistência: divergência invalida o fact; display sem UF pode ser consistente com a cidade resolvida, mas nunca escolhe identidade. StopID vazio, desconhecido ou ambíguo invalida os facts atuais. |

Sem `INDEX` resolvido, todas as opções visíveis precisam concordar pelos StopIDs
para fundamentar coletivamente um endpoint. Com `INDEX` válido, somente os IDs
da opção selecionada são considerados. As regressões do sétimo review cobrem:

- `Freiburg/SC` não corresponde a `Fraiburgo/SC`;
- a identidade canônica correta continua aceita;
- `DestinationStopID=SE_SANTA_CECILIA` com display `Santa Cecilia/SC` não
  fundamenta a proposta `/SC`;
- StopID correto com display sem UF usa a identidade do catálogo;
- StopID inexistente ou ambíguo invalida os facts atuais;
- opções com StopIDs diferentes não fundamentam coletivamente um endpoint;
- `INDEX` válido limita o grounding aos IDs da opção selecionada.

Esta correção não alterou `TravelQueryMeaningV2`, não adicionou parsing ou
integração runtime e não implementou os slices 3.6F-C/D.

#### Oitavo review canônico e correção (2026-07-15)

| Prioridade | Achado | Correção local e regressão |
|---|---|---|
| P2 | facts aplicáveis ao prompt/histórico atual, porém inválidos, retornavam o mesmo `ok=false` usado para ausência/staleness e liberavam fallback para `State.Route`; sem `OptionReference`, uma proposta podia ser aceita pelo state stale | o envelope passou a receber classificação interna explícita `NOT_APPLICABLE`, `VALID` ou `INVALID`. Somente `NOT_APPLICABLE` permite evidência secundária do state; `INVALID` encerra a validação com `CURRENT_AVAILABILITY_FACTS_INVALID`, inclusive em `AVAILABILITY_SEARCH` com referência `NONE`. |

A classificação separa identidade/aplicabilidade do envelope da validade das
opções. Source/prompt/histórico divergente, invisível ou stale é
`NOT_APPLICABLE`. Envelope atual com StopID vazio/desconhecido/ambíguo, display
divergente, opção incompleta, contagem inconsistente ou data inválida/passada é
`INVALID`. Envelope atual completo é `VALID`, autoritativo sobre `State.Route`.

As regressões do oitavo review cobrem:

- StopID desconhecido e ambíguo com state compatível;
- display divergente com state compatível;
- opção incompleta com state compatível;
- data inválida e passada com state compatível;
- proposta sem `OptionReference` com facts atuais inválidos;
- facts atuais válidos vencendo state divergente;
- facts ausentes permitindo state compatível;
- facts stale/não aplicáveis sem classificação falsa como inválidos;
- `INDEX` válido limitado à opção selecionada.

Esta correção permanece local e pura, sem alteração do contrato ou integração
runtime.

#### Nono review canônico, correções e fechamento (2026-07-15)

| Prioridade | Achado | Correção local e regressão |
|---|---|---|
| P2 | facts atuais inválidos eram classificados somente depois da coerência de clarification, permitindo que origem/destino do `State.Route` produzissem `INCOHERENT_CLARIFICATION` antes da rejeição factual terminal | a classificação é criada uma única vez logo após os gates de entrada vazia e proteção de `STRONG`, antes de clarification e das demais validações que podem consultar a rota efetiva. `INVALID` agora termina imediatamente com seu reason code fechado; os gates anteriores preservam precedência. |
| P2 | `EARLIEST_AVAILABLE` sem origem/destino na proposta podia completar a rota pelo `State.Route` stale mesmo com facts atuais válidos e divergentes | quando os facts são `VALID`, a rota factual é derivada exclusivamente dos `OriginStopID`/`DestinationStopID` resolvidos de forma única no catálogo. As opções relevantes precisam concordar nos dois endpoints; divergência ou ausência de rota única retorna `EARLIEST_AVAILABLE_ROUTE_REQUIRED`, sem fallback para state ou identidade por display. |

O contexto interno propaga a mesma classificação e a rota factual derivada para
clarification, grounding, `EARLIEST_AVAILABLE`, referência de opção e coverage;
nenhum helper reclassifica o envelope. `NOT_APPLICABLE` continua sendo o único
estado que permite consultar `State.Route`, enquanto `INVALID` permanece
terminal e `VALID` mantém autoridade sobre state stale.

As regressões do nono review cobrem:

- StopID desconhecido, display divergente, opção incompleta e data inválida
  diante de clarification de origem/destino com state preenchido;
- precedência de `EMPTY_CURRENT_TURN` e `STRONG_DECISION_PROTECTED` diante dos
  mesmos facts inválidos;
- `EARLIEST_AVAILABLE` aceito quando opções atuais concordam por StopID apesar
  de `State.Route` divergente;
- rejeição de `EARLIEST_AVAILABLE` quando opções atuais válidas não definem uma
  rota única, mesmo que o state ofereça rota compatível;
- preservação do fallback por state somente para facts `NOT_APPLICABLE`;
- rejeição terminal de facts `INVALID` no fluxo `EARLIEST_AVAILABLE`.

Esta correção não altera contrato, não usa display como identidade e não
adiciona parser, provider, side effect ou integração runtime.

O review final posterior às correções não encontrou P1/P2. O validator foi
confirmado como estritamente factual: facts são classificados antes de
clarification e grounding; `INVALID` é terminal; `VALID` usa somente facts
atuais; `NOT_APPLICABLE` pode usar `State.Route`; e `EARLIEST_AVAILABLE`
deriva a rota somente de StopIDs concordantes. O Slice 3.6F-B está seguro para
commit. Produção não mudou porque o validator continua sem consumidor runtime.

#### Regra operacional futura de cobertura — somente registro

Não implementar no 3.6F-B:

- cobertura poderá ser confirmada somente para rotas `SC → MA` ou `MA → SC`;
- rotas `SC → SC` e `MA → MA` não deverão ser confirmadas automaticamente;
- nesses casos, o fluxo futuro deverá orientar contato com suporte e oferecer consulta `SC ↔ MA`;
- o lookup e a decisão de cobertura pertencem ao 3.6F-H;
- o template e o número oficial pertencem ao slice apropriado e deverão usar fonte canônica, sem hardcode ou número inventado.

Resposta futura esperada, ainda sem implementação:

```text
Referente a esse trajeto, peço que entre em contato com o suporte: [número oficial]. Caso queira verificar outra rota de SC para MA ou de MA para SC, estou à disposição.
```

#### Arquivos alterados

```text
apps/api/internal/chat/travel_query_validation_v2.go
apps/api/internal/chat/travel_query_validation_v2_test.go
docs/EXECUTION_TRACKER.md
plans/3.6f-b-validator-v2.md
```

O contrato `travel_query_meaning_v2.go` não foi alterado.

Composição prevista do commit do 3.6F-B:

```text
apps/api/internal/chat/travel_query_validation_v2.go
apps/api/internal/chat/travel_query_validation_v2_test.go
docs/EXECUTION_TRACKER.md
```

`plans/3.6f-b-validator-v2.md` é fonte canônica local, mas está ignorado pela
regra `plans` do `.gitignore`; suas mudanças ficam fora do commit. O
`.gitignore` não foi alterado.

#### Validação executada no fechamento após o review final

As correções foram formatadas e validadas com o cache Go padrão:

```text
gofmt nos dois arquivos Go -> PASS
go test -count=1 ./internal/chat -run 'TestValidateTravelQueryMeaningV2ClassifiesInvalidFactsBeforeRouteClarification|TestEarliestAvailableValidationUsesAvailabilityFactsRoutePrecedence' -> PASS
go test -count=1 ./internal/chat -run 'TestValidateTravelQueryMeaningV2|Test.*LocationRole.*Validation|Test.*Earliest.*Validation|Test.*Seat.*Validation|Test.*Coverage.*Validation' -> PASS
go test -count=1 ./internal/chat -> PASS
go test -count=1 ./... -> PASS
git diff --check -> PASS
```

#### Fechamento do 3.6F-B

```text
comportamento antes: clarification podia consultar State.Route antes da classificação de facts inválidos; EARLIEST_AVAILABLE com facts válidos podia completar endpoints ausentes pelo state stale
comportamento depois: a classificação factual ocorre uma única vez antes de clarification; INVALID é terminal, VALID fornece rota somente por StopIDs concordantes e somente NOT_APPLICABLE permite fallback para State.Route
mudança funcional de runtime: nenhuma; o validator V2 não possui consumidor runtime
resultado do review: o nono review encontrou inicialmente 2 P2; após as correções, o review final não encontrou P1/P2 e confirmou o validator como estritamente factual
status final: CONCLUÍDA — REVIEW FINAL SEM P1/P2; seguro para commit
teste em produção: não necessário; função não integrada
riscos restantes: o future interpreter deverá produzir a identidade canônica e os StopIDs corretos; esses riscos pertencem ao shadow e aos slices posteriores, não reabrem o 3.6F-B
próxima ação única: executar somente o 3.6F-C mediante novo /goal explícito
```

### 8.8 Registro operacional — 3.6F-C (2026-07-15 a 2026-07-16)

**Status:** **CONCLUÍDA EM CÓDIGO — GATE OPERACIONAL REABERTO**.

O review local final permaneceu sem P1/P2. O smoke implantado de 2026-07-16, porém, não criou claims V2 e o recovery registrou `sweep_failed` recorrente. Isso não apaga o review do código, mas bloqueia a promoção da fila até H-2026-07-16A.

O primeiro review encontrou **3 P1 e 4 P2**. O 3.6F-D voltou a ficar
bloqueado até que os achados sejam corrigidos, validados e submetidos a novo
review sem P1/P2.

As sete correções foram implementadas e validadas localmente neste ciclo. O
status permanece `EM CORREÇÃO APÓS REVIEW` porque o novo review ainda é um gate
obrigatório; esta validação local não libera o 3.6F-D.

O segundo review encontrou **2 P2** adicionais: `missing_fields` ainda era
case-insensitive dentro do validator factual, e uma falha de
`CompleteTravelQueryV2Shadow` podia deixar o claim `IN_PROGRESS` sem estado
terminal. As duas correções estão implementadas localmente, mas o status segue
`EM CORREÇÃO APÓS REVIEW` e o 3.6F-D permanece bloqueado até novo review limpo.

O terceiro review encontrou **1 P2**: a recuperação do lease ainda dependia de
outro `Reprocess` para a mesma key. Sem novo turno, o ledger durável podia ficar
`IN_PROGRESS` indefinidamente. A correção adiciona sweeper durável e
independente de `Reprocess`, mas não antecipa aprovação: o status continua
`EM CORREÇÃO APÓS REVIEW` e o 3.6F-D permanece bloqueado até review limpo.

O quarto review encontrou **1 P1**: embora o lote transformado tivesse limite,
a seleção ainda precisava varrer `chat_messages` e abrir o ledger JSONB antes
do `LIMIT`. Em tabelas grandes, timeouts repetidos poderiam impedir progresso.
A correção exigiu migration para um marcador relacional indexável; o status
continua `EM CORREÇÃO APÓS REVIEW` e o 3.6F-D permanece bloqueado.

O quinto review encontrou **1 P2**: a fixture PostgreSQL de recovery não criava
as sessões-pai e omitia `chat_messages.direction`, portanto o teste falhava no
schema canônico limpo antes de exercitar o lifecycle. A preparação canônica foi
corrigida sem alterar migration ou runtime; o status continua
`EM CORREÇÃO APÓS REVIEW` e o 3.6F-D permanece bloqueado.

O sexto review encontrou **1 P2**: o teste alterava `enable_seqscan` no nível
da sessão e descartava o erro do `RESET`, podendo devolver ao pool uma conexão
com configuração artificial. O isolamento foi movido para `SET LOCAL` em
transação com finalização verificada; o status continua
`EM CORREÇÃO APÓS REVIEW` e o 3.6F-D permanece bloqueado.

Após as seis correções e a reexecução dos gates, o review final não encontrou
P1/P2 e o código foi considerado seguro para commit. A liberação operacional foi
posteriormente reaberta pelo smoke implantado: a flag estava ativa, mas nenhum claim
foi persistido e o recovery falhou sem causa observável. H-2026-07-16A tornou-se a
única etapa `PRÓXIMA`; o 3.6F-D voltou a ficar bloqueado.

O OpenAI Travel Interpreter V2 foi integrado em paralelo ao V1 somente como
shadow. A flag `CHAT_OPENAI_TRAVEL_V2_SHADOW_ENABLED` permanece desligada por
padrão. A proposta V2 e o resultado factual do validator são persistidos
somente como resumo sanitizado; nenhum dado do shadow alimenta router, decisão,
template, resposta, tool, estado canônico ou autosend.

#### Schema, prompt e runner

O structured output `travel_query_meaning_v2` cobre todos os campos de
`TravelQueryMeaningV2`, exige todos os campos, fecha enums e usa
`additionalProperties=false` em todos os objetos. Origem e destino são campos
obrigatórios nullable. O schema não contém tool, template, ação ou ID
operacional executável.

O prompt compacto recebe somente:

- mensagem atual, com CPF/RG/CNH/documentos e telefone redigidos;
- data observada e estado canônico mínimo de rota/seleção;
- `ActivePromptContext` sem body ou identidade da mensagem;
- facts estruturados da lista atual visível e confiável, sem IDs operacionais;
- catálogo mínimo de localidades conhecidas somente quando os facts atuais não
  bastam.

As instruções delegam à OpenAI a interpretação de origem/destino, papéis de
localidade, papel contextual de números, `INDEX`/`DATE`/`DEICTIC`, UF versus
conjunção, ambiguidade/clarification, `EARLIEST_AVAILABLE`, coverage, poltrona,
institucional e `ACKNOWLEDGEMENT`. O validator V2 permaneceu factual e não
ganhou parsing linguístico.

O runner usa Responses API com `store=false`, `tools=[]` e `tool_choice=none`.
`missing_fields` é comparado após somente `TrimSpace`, de forma exata e
case-sensitive; `ORIGIN` não é aceito como `origin`. Schema inválido, recusa,
erro HTTP, timeout e resultado ausente geram status seguros. Toda proposta
parseável, inclusive schema-invalid, passa por `ValidateTravelQueryMeaningV2`.

#### Correções do primeiro review

| Prioridade | Achado | Correção e regressão |
|---|---|---|
| P1 | texto livre do provider podia entrar no resumo | `Origin`, `Destination`, `MentionedLocations` e `QueryLocation` persistem somente nome canônico com correspondência exata no catálogo confiável; nome completo, e-mail, endereço e texto arbitrário viram `__redacted_unallowlisted`, inclusive em proposta rejeitada. |
| P1 | novo `Reprocess` ou concorrência repetia a chamada V2 | o inbound mantém ledger durável em `normalized_payload.travel_query_v2_shadow_claims[IdempotencyKey]`; `UPDATE ... WHERE claim ausente` concede `ACQUIRED` uma vez, `IN_PROGRESS` é ignorado com status seguro e `COMPLETED` reutiliza o resumo sem chamar o provider. Refusal, erro, timeout e panic também terminam em `COMPLETED`. |
| P1 | provider síncrono bloqueava/cancelava o fluxo real | o job é agendado somente no retorno bem-sucedido de `Reprocess`, depois de draft/autosend; usa contexto independente, timeout de 10 segundos, completion com contexto próprio, limite global de quatro jobs e recuperação de panic. Fila cheia falha aberta sem bloquear. |
| P2 | `missing_fields=["ORIGIN"]` passava por lowercase | validação agora é exata e case-sensitive antes do mapping. |
| P2 | display `Seara` duplicava `SC_SEARA` | StopID único já conhecido preserva `Seara/SC`; display não cria identidade, e StopID desconhecido ou ambíguo é omitido do catálogo. |
| P2 | mídia documental com legenda `segue` podia usar `FALLBACK` | a mesma regra pura `shouldRunDocumentExtract(memory)` roda antes de montar o job e força `DecisionStrengthStrong`; o validator retorna `STRONG_DECISION_PROTECTED`. |
| P2 | 3.6F-D foi liberado antes do review limpo | 3.6F-C voltou a `EM CORREÇÃO APÓS REVIEW` e 3.6F-D ficou `BLOQUEADA por 3.6F-C`. |

#### Correções do segundo review

| Prioridade | Achado | Correção e regressão |
|---|---|---|
| P2 | proposta parseável com `missing_fields=["ORIGIN"]` podia combinar schema inválido com `validation.accepted=true` | runner, validator e helpers usam somente `TrimSpace` e allowlist exata case-sensitive; `origin` continua válido, enquanto `ORIGIN`, `Origin` e desconhecidos são rejeitados. O validator sempre roda para proposta parseável, mas `effectiveAccepted = schemaValid && factualAccepted`, e schema inválido acrescenta razão fechada `openai_schema_invalid`. |
| P2 | falha de completion podia deixar claim `IN_PROGRESS` indefinidamente | persistência terminal faz três tentativas com contextos independentes e backoff de 10/20 ms, sempre sem repetir provider e preservando o mesmo summary. O claim guarda `claimed_at`, `lease_expires_at` e epoch; o lease excede timeout do provider, orçamento máximo dos retries e margem de segurança. Após expirar, `ClaimTravelQueryV2Shadow` converte atomicamente o órfão em `COMPLETED` com `travel_query_v2_shadow_execution_abandoned`; claim recente é ignorado e `COMPLETED` é reutilizado. |

#### Correção do terceiro review (2026-07-16)

| Prioridade | Achado | Correção e regressão |
|---|---|---|
| P2 | claim órfão só era examinado por outro `Reprocess` da mesma key | `RecoverExpiredTravelQueryV2ShadowClaims` varre lotes limitados de mensagens, usa o relógio do PostgreSQL e `FOR UPDATE SKIP LOCKED`, preserva o ledger JSONB e terminaliza atomicamente todos os claims expirados da mensagem com summary fechado. O loop da API executa imediatamente no startup e depois a cada ticker, mesmo com a flag V2 desligada, com contexto da aplicação, timeout por sweep, execução sequencial, log sanitizado e retry no ciclo seguinte. |

Claims legados `IN_PROGRESS` sem lease numérico recebem primeiro um lease de
graça persistido de um minuto pelo relógio do PostgreSQL; permanecem recentes
durante essa janela e são terminalizados pelo sweep posterior. A regra evita
encerrar uma execução antiga imediatamente, mas também evita estado indefinido.
Nenhum caminho do recovery chama provider, router, resposta, draft, state,
tool ou autosend.

#### Correção do quarto review (2026-07-16)

| Prioridade | Achado | Correção e regressão |
|---|---|---|
| P1 | `LIMIT 50` era aplicado somente depois de scan de `chat_messages` e `jsonb_each` | a migration `0021` adiciona `travel_query_v2_shadow_recovery_due_at timestamptz` e índice B-tree parcial por `(travel_query_v2_shadow_recovery_due_at, id)`. Aquisição, completion e recovery recalculam o marcador atomicamente com o ledger. A candidate query usa somente marcador vencido, `ORDER BY`, `LIMIT` e `FOR UPDATE SKIP LOCKED`; o JSONB é aberto apenas no CTE posterior ao lote materializado. |

O backfill único da migration marca somente claims legados `IN_PROGRESS` de
testes/ambientes locais; a feature ainda não foi implantada e não existem
claims V2 de produção. Em runtime, mensagens comuns ficam fora do índice
parcial. Claims futuros mantêm o menor lease, mensagens sem pendência recebem
`NULL`, e duas réplicas consomem lotes distintos. O EXPLAIN PostgreSQL real
confirmou `Limit -> LockRows -> Index Scan` usando
`idx_chat_messages_travel_query_v2_shadow_recovery_due_at`.

#### Correção do quinto review (2026-07-16)

| Prioridade | Achado | Correção e regressão |
|---|---|---|
| P2 | o teste PostgreSQL inseria mensagens sem `direction` e referenciava sessões inexistentes | a fixture cria duas `chat_sessions` determinísticas e distintas com os campos obrigatórios do schema canônico. Um helper único insere mensagens unitárias e em lote com `direction=INBOUND`, sessão existente, `normalized_payload`, marcador nullable ou devido e `created_at`; o cleanup remove filhos antes dos pais. O teste foi executado com `-race` em PostgreSQL 16 limpo após `0001`, `0019` e `0021`. |

Nenhum `NOT NULL` ou foreign key foi relaxado e nenhum default foi criado.
`repository.go`, candidate query, recovery loop e as migrations canônicas
permaneceram intactos nesta correção.

#### Correção do sexto review (2026-07-16)

| Prioridade | Achado | Correção e regressão |
|---|---|---|
| P2 | o teste descartava erro de `RESET enable_seqscan` e podia contaminar a conexão devolvida ao pool | a conexão registra `SHOW enable_seqscan=on`, abre transação, executa `SET LOCAL enable_seqscan=off`, confirma o valor local e roda o `EXPLAIN` na mesma transação. O rollback é obrigatório e verificado em sucesso e em falha simulada; se falhar, a conexão é fechada e o erro é combinado com o erro principal. Depois do rollback, a mesma conexão é conferida diretamente e após nova aquisição pelo pool, com PID idêntico, valor inicial restaurado e consulta posterior normal. |

O plano continua `Limit -> LockRows -> Index Scan` no índice parcial. Não há
`SET`/`RESET` de sessão nesse caminho de teste, e a conexão só é liberada após
rollback bem-sucedido ou fechamento forçado.

#### Review final e contrato de fechamento

O review final confirmou, sem P1/P2:

- schema strict completo para `TravelQueryMeaningV2`;
- Responses API com `store=false`, `tools=[]` e `tool_choice=none`;
- no máximo uma chamada V2 por key, com claim durável e atômico;
- recovery indexado, bounded e seguro entre réplicas;
- shadow executado fora do caminho crítico e com fila fail-open;
- resumo sanitizado sem texto livre do provider ou PII;
- validator factual obrigatório para toda proposta parseável;
- decisões `STRONG` protegidas;
- V1 intacto;
- zero influência do V2 sobre decisão, resposta, template, tool, state ou
  autosend;
- flag `CHAT_OPENAI_TRAVEL_V2_SHADOW_ENABLED` desligada por padrão.

#### Claim e resumo shadow persistidos

Cada IdempotencyKey usa um registro no `normalized_payload` do inbound como
fonte detalhada. A coluna relacional adicionada pela migration `0021` é apenas
o marcador indexável do recovery; não substitui o ledger e não existe mutex/map
em memória como fonte de verdade:

```text
travel_query_v2_shadow_claims[IdempotencyKey]
status = IN_PROGRESS | COMPLETED
idempotency_key
claimed_at + lease_expires_at enquanto IN_PROGRESS
summary somente quando COMPLETED
chat_messages.travel_query_v2_shadow_recovery_due_at = menor lease pendente | NULL
```

O resumo terminal contém somente:

```text
status e error_code fechados
intent e turn_meaning
origin, destination e mentioned_locations canônicos ou marcador fechado
date_preference
option_reference
route_coverage
seat_request
institutional_topic
needs_clarification e missing_fields fechados
confidence
provider_response_id sanitizado e latency_ms
validation.status, accepted e reason_codes factuais
```

Não são persistidos body atual, histórico, body do prompt ativo, documentos,
CPF, telefone, nome completo, e-mail, endereço, texto arbitrário, reasons
livres do provider, request/response brutos ou payload sensível.

#### Casos cobertos

```text
de Fraiburgo para Santa Inês -> ORIGIN Fraiburgo/SC + DESTINATION Santa Ines/MA
opção 1 ou 2 -> NONE + clarification, sem seleção silenciosa
opção 3 com duas opções -> INDEX=3 antes de OPTION_INDEX_OUT_OF_RANGE
daqui a 2 dias -> data temporal, nunca INDEX
opção 2 no dia 15 -> INDEX=2 e data preservados separadamente
Santa Cecilia se tiver vaga -> cidade sem UF inventada
Santa Cecilia/SE -> UF explícita preservada
essa com uma e várias opções -> DEICTIC com clarification quando ambígua
EARLIEST_AVAILABLE, coverage, poltrona, institucional e acknowledgement
schema inválido, refusal, erro, timeout, panic, resultado ausente e dado sensível
missing_fields ORIGIN rejeitado como schema-invalid
missing_fields origin aceito após TrimSpace; ORIGIN, Origin e booking_id rejeitados sem aliases
proposta parseável schema-invalid sempre com validation.accepted=false e openai_schema_invalid
nome completo, e-mail, endereço e texto livre substituídos por marcador fechado
claim concorrente IN_PROGRESS sem segunda chamada
novo Reprocess reutilizando COMPLETED sem segunda chamada
refusal, erro, timeout e panic persistidos como COMPLETED sem retry do provider
primeira e segunda completion falhando e terceira persistindo o mesmo summary terminal
falha das três completions mantendo IN_PROGRESS recente sem nova chamada ao provider
claim órfão após lease convertido pelo sweeper em COMPLETED abandonado sem novo Reprocess
sweep imediato no startup inclusive com flag V2 desligada, e encerramento por cancelamento
claim recente e COMPLETED preservados; múltiplos expirados da mesma mensagem recuperados
duas instâncias concorrentes terminalizando uma única vez com lote bounded
storage indisponível ou panic em um ciclo recuperado no próximo sem vazar detalhe no log
lease legado ausente reparado com graça durável e terminalizado no ciclo posterior
recuperação stale e reutilização de COMPLETED preservando uma chamada por key
10.000 mensagens comuns fora do marcador e 120 vencidas processadas em 50/50/20
candidate query sem jsonb_each/lateral antes do LIMIT indexado
claim adquirido marcando o mesmo lease; último completion limpando o marcador
claim futuro mantendo o próximo due_at e múltiplos expirados da mensagem no mesmo sweep
duas réplicas PostgreSQL usando SKIP LOCKED sem processar a mesma mensagem
fixture canônica criando sessões-pai antes de inserts unitários e em lote com direction INBOUND
SET LOCAL enable_seqscan=off restrito à transação do EXPLAIN e rollback verificado
falha simulada após SET LOCAL encerrando a transação sem mascarar erro principal
mesmo backend readquirido do pool com enable_seqscan restaurado e consulta normal
provider lento não bloqueando draft/autosend nem herdando cancelamento do contexto principal
display Seara + SC_SEARA preservando somente Seara/SC; StopID desconhecido omitido
imagem documental com legenda segue produzindo STRONG_DECISION_PROTECTED
lista visível confiável prevalecendo sobre draft invisível mais novo
decisão STRONG rejeitada com STRONG_DECISION_PROTECTED
V1 e V2 chamados uma vez, com resposta real e tools inalterados
```

#### Arquivos alterados

```text
apps/api/.env.example
apps/api/cmd/api/main.go
apps/api/internal/chat/openai_travel_query_v2_prompt.go
apps/api/internal/chat/openai_travel_query_v2_runner.go
apps/api/internal/chat/openai_travel_query_v2_schema.go
apps/api/internal/chat/openai_travel_query_v2_test.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/service.go
apps/api/internal/chat/travel_query_v2_shadow.go
apps/api/internal/chat/travel_query_v2_shadow_background.go
apps/api/internal/chat/travel_query_v2_shadow_repository_test.go
apps/api/internal/chat/travel_query_validation_v2.go
apps/api/internal/chat/travel_query_validation_v2_test.go
apps/api/internal/shared/config/config.go
apps/api/migrations/0021_chat_travel_query_v2_shadow_recovery_due_at.sql
apps/api/migrations/checks/0021_chat_travel_query_v2_shadow_recovery_due_at_explain.sql
docs/EXECUTION_TRACKER.md
plans/3.6f-c-openai-v2-shadow.md
```

#### Composição prevista do commit

O futuro staging normal do 3.6F-C deve conter exatamente os arquivos tracked
modificados e untracked confirmados por `git status`, `git diff --name-only` e
`git ls-files --others --exclude-standard`:

```text
apps/api/.env.example
apps/api/cmd/api/main.go
apps/api/internal/chat/openai_travel_query_v2_prompt.go
apps/api/internal/chat/openai_travel_query_v2_runner.go
apps/api/internal/chat/openai_travel_query_v2_schema.go
apps/api/internal/chat/openai_travel_query_v2_test.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/service.go
apps/api/internal/chat/travel_query_v2_shadow.go
apps/api/internal/chat/travel_query_v2_shadow_background.go
apps/api/internal/chat/travel_query_v2_shadow_repository_test.go
apps/api/internal/chat/travel_query_validation_v2.go
apps/api/internal/chat/travel_query_validation_v2_test.go
apps/api/internal/shared/config/config.go
apps/api/migrations/0021_chat_travel_query_v2_shadow_recovery_due_at.sql
apps/api/migrations/checks/0021_chat_travel_query_v2_shadow_recovery_due_at_explain.sql
docs/EXECUTION_TRACKER.md
```

A migration `0021`, seu check SQL, os novos arquivos Go e as correções
case-sensitive de `travel_query_validation_v2.go` e
`travel_query_validation_v2_test.go` fazem parte obrigatória dessa composição.

#### Plano local ignorado

`plans/3.6f-c-openai-v2-shadow.md` é fonte canônica local, porém está ignorado
pela regra `plans` do `.gitignore` e não entrará em um `git add` normal. O
`.gitignore` permanece inalterado. O plano ficará fora do futuro commit, salvo
uso explícito de `git add -f`; esse comando não foi e não será usado neste
fechamento.

#### Validação executada

```text
gofmt nos arquivos Go alterados -> PASS
PostgreSQL 16 efêmero limpo: migrations canônicas 0001, 0019 e 0021 -> PASS
fixture canônica: duas chat_sessions-pai, insert unitário e inserts em lote com direction=INBOUND e session_id válido -> PASS
psql migrations/checks/0021_chat_travel_query_v2_shadow_recovery_due_at_explain.sql -> PASS com 20.000 mensagens comuns, 120 vencidas e lote 50
EXPLAIN candidate query -> Limit -> LockRows -> Index Scan using idx_chat_messages_travel_query_v2_shadow_recovery_due_at
isolamento do EXPLAIN: SHOW inicial on; SET LOCAL + SHOW off na transação; rollback verificado; mesma PID readquirida com SHOW on e SELECT 1 -> PASS
falha simulada após SET LOCAL -> erro principal preservado, rollback verificado e configuração restaurada -> PASS
go test -count=1 ./internal/chat -run 'TestTravelQueryV2ShadowRecovery(Migration|Candidate|Explain)' -> PASS
CHAT_TRAVEL_V2_SHADOW_POSTGRES_TEST_URL=<postgres efêmero> go test -race -count=1 ./internal/chat -run TestTravelQueryV2ShadowRecoveryPostgresMarkerLifecycleAndBoundedProgress -> PASS
go test -count=1 ./internal/chat -run 'Test.*OpenAI.*Travel.*V2|Test.*Travel.*Shadow|Test.*Travel.*Schema|Test.*Travel.*Prompt|TestValidateTravelQueryMeaningV2' -> PASS
go test -race -count=1 ./internal/chat -run 'TestTravelQueryV2Shadow(ExhaustedCompletionRetriesAreRecoveredBySweeperWithoutReprocess|Recovery)' -> PASS
go test -count=1 ./internal/chat -> FAIL somente em TestParseBookingCreateInputSpecificRejectedOptionAllowsOtherOption e TestAvailabilitySelectionAfterSpecificRejectedOptionWithoutPayment
go test -count=1 ./... -> FAIL nos mesmos dois testes; demais pacotes PASS
HEAD limpo: go test -count=1 ./internal/chat -run 'TestParseBookingCreateInputSpecificRejectedOptionAllowsOtherOption|TestAvailabilitySelectionAfterSpecificRejectedOptionWithoutPayment' -> reproduz as mesmas duas falhas
git diff --check -> PASS
```

As duas falhas amplas são baseline temporal preexistente e foram reproduzidas
no `HEAD` limpo em 2026-07-16. Nenhuma falha adicional apareceu; o código de
produção fora do recovery não foi alterado para acomodar esse baseline.

#### Fechamento final do 3.6F-C

```text
comportamento antes das correções: além das lacunas dos reviews anteriores, o teste PostgreSQL de recovery alterava enable_seqscan na sessão e ignorava erro do RESET antes de devolver a conexão ao pool
comportamento depois das correções: resumo usa identidades canônicas/markers fechados; accepted exige schema e validator; claim JSONB atômico mantém marcador relacional indexável; candidate query lê lote limitado antes de abrir JSONB; recovery durável de startup/ticker não repete provider; fixture PostgreSQL cria pais canônicos e usa direction INBOUND; EXPLAIN usa SET LOCAL em transação com rollback verificado e prova de ausência de vazamento na mesma conexão readquirida; execução bounded ocorre depois do fluxo real em contexto próprio; enum exato, StopID canônico e mídia STRONG estão cobertos
mudança funcional user-visible: nenhuma; flag V2 desligada por padrão, background é fail-open e o resultado não é consumido por decisão, resposta, template, tool, estado canônico ou autosend
resultado do review: primeiro review encontrou 3 P1 e 4 P2; segundo review encontrou 2 P2; terceiro review encontrou 1 P2; quarto review encontrou 1 P1; quinto review encontrou 1 P2; sexto review encontrou 1 P2; após todas as correções, o review final não encontrou P1/P2
status de código: CONCLUÍDA — REVIEW FINAL SEM P1/P2; gate operacional reaberto pelo smoke
commit/push/deploy: nenhum executado neste ciclo
teste em produção: executado em 2026-07-16; flag V2 ativa, porém nenhum claim criado e recovery com sweep_failed recorrente
ordem de deploy futura: aplicar obrigatoriamente a migration 0021 antes do novo binário
riscos restantes: enquanto a API ou o storage estiverem indisponíveis nenhum sweep pode persistir a transição, mas startup/ticks posteriores retomam o recovery; a fila bounded pode descartar shadow para preservar o fluxo real; qualidade semântica, custo e latência reais pertencem aos próximos slices de corpus, evaluator e observabilidade
próxima ação histórica daquela rodada: executar H-2026-07-16A; H-2026-07-16B e 3.6F-D permaneciam bloqueados/pendentes
```

---

### 8.9 Incidente operacional — H-2026-07-16A (2026-07-16)

**Status:** **EM VALIDAÇÃO OPERACIONAL — PATCH LOCAL VERDE**.

#### Sintoma

```text
CHAT_OPENAI_TRAVEL_V2_SHADOW_ENABLED=true no processo PID 1
mensagens reais processadas normalmente
nenhum normalized_payload.travel_query_v2_shadow_claims criado
travel_v2_shadow_recovery event=sweep_failed a cada ciclo
```

#### Evidências já eliminadas

```text
API conectada ao PostgreSQL/Supabase correto
schema public
migration 0021 aplicada
coluna e índice presentes
usuário com SELECT e UPDATE
candidate query com LIMIT/FOR UPDATE SKIP LOCKED: PASS
query completa de recovery manual: PASS, 0 mensagens/0 claims/0 reparos
query manual de criação de claim: PASS dentro de transação com ROLLBACK
nenhum claim IN_PROGRESS preso
```

Conclusão: o defeito restante está no fluxo Go de scheduler/job/claim/recovery ou na observabilidade, não na existência da migration, permissão ou SQL básico já comprovado.

#### Gate de fechamento

```text
reason fechado para toda saída do scheduler
erro sanitizado com operation/error_class/SQLSTATE
mensagem real gera claim
claim termina COMPLETED
recovery_due_at volta a NULL
nenhum sweep_failed recorrente
zero efeito user-visible
```

Não registrar SQL, payload, body, telefone, CPF, documento, segredo ou `DATABASE_URL`.

#### Execução local do hotfix (2026-07-17)

O call path real foi reconciliado de `StartChatBufferFlushLoop` até
`Service.Reprocess`, scheduler, claim, provider, completion e recovery. A suíte
anterior já comprovava o lifecycle feliz com store fake, mas deixava saídas
operacionais silenciosas.

Lacunas comprovadas no código anterior:

- o defer de `Reprocess` só chamava o scheduler quando o job já existia; um
  retorno bem-sucedido anterior à construção do job não produzia reason, e
  `disabled` não era observável pelo fluxo real;
- scheduler, claim e completion descartavam erro/panic sem classificação;
- recovery reduzia erro e panic ao mesmo `event=sweep_failed` sem
  `operation`, `error_class`, `sqlstate`, `timeout` ou `canceled`;
- recovery com zero candidatos era sucesso, mas não emitia `sweep_done`.

O patch local agora preserva `event` para compatibilidade e registra também
`reason` fechado:

```text
scheduler: disabled, empty_idempotency_key, incompatible_store, capacity_full, scheduled
job: started, claim_acquired, claim_in_progress, claim_completed_reused, claim_failed, provider_started, provider_completed, completion_failed, completion_completed
recovery: sweep_started, sweep_done, sweep_failed
```

Falhas de storage registram somente:

```text
operation=claim|completion|recovery
error_class=postgres|timeout|canceled|panic|storage
sqlstate=<codigo ou vazio>
timeout=true|false
canceled=true|false
```

O texto do erro nunca entra no log. Panic de claim/completion/recovery é
convertido em classe fechada, o slot global é liberado pelo defer e o provider
não roda após falha da claim. O retorno real continua fail-open: o shadow não
entra em resposta, template, tool, state ou autosend.

#### Evidência local

```text
SYSTEM_BUFFER_FLUSH bem-sucedido -> exatamente um scheduled, claim COMPLETED, uma chamada de provider e um único outbound draft
Reprocess com erro -> zero scheduled e zero provider
retorno idempotente anterior ao job -> empty_idempotency_key, zero claim e zero provider
flag desligada -> disabled pelo fluxo real
store incompatível e capacidade cheia -> reasons próprios
claim com erro, timeout ou panic -> claim_failed sanitizado, zero provider e slot liberado
claim IN_PROGRESS e COMPLETED -> reasons próprios, sem segunda chamada
completion com falha -> completion_failed sanitizado
recovery 0/0/0 -> sweep_started + sweep_done, nunca sweep_failed
SQLSTATE 57014 sintético -> somente metadados permitidos; mensagem sensível ausente
```

#### Arquivos alterados no H-2026-07-16A

```text
apps/api/internal/chat/service.go
apps/api/internal/chat/travel_query_v2_shadow_background.go
apps/api/internal/chat/openai_travel_query_v2_test.go
docs/EXECUTION_TRACKER.md
```

#### Validação executada

Os comandos Go usaram `GOCACHE=/tmp/schumacher-h16a-go-build` e
`GOTMPDIR=/tmp` após o cache padrão retornar erro de filesystem read-only.

```text
go test -count=1 ./internal/chat -run 'Test.*Travel.*V2.*Shadow|Test.*Shadow.*Schedule|Test.*Shadow.*Claim|Test.*Shadow.*Recovery' -> PASS
go test -race -count=1 ./internal/chat -run 'Test.*Travel.*V2.*Shadow|Test.*Shadow.*Recovery' -> PASS
go test -count=20 ./internal/chat -run 'TestTravelQueryV2ShadowScheduler|TestReprocessSystemBufferFlushSchedulesTravelQueryV2ShadowExactlyOnce|TestReprocessTravelQueryV2SchedulerObservesDisabledAndEarlyIdempotentReturns|TestReprocessTravelQueryV2ErrorDoesNotSchedule' -> PASS
go test -count=20 ./internal/automation -run '^(TestStartChatBufferFlushLoopRunsSystemCycleWhenEnabled|TestRunChatBufferFlushProcessesDueBuffers)$' -> PASS
go test -race -count=20 ./internal/chat -run '^(TestReprocessSystemBufferFlushSchedulesTravelQueryV2ShadowExactlyOnce|TestTravelQueryV2ShadowSchedulerReleasesSlotOnClaimErrorAndPanic|TestTravelQueryV2ShadowRecoveryZeroCandidatesIsSuccessfulSweep)$' -> PASS
go test -count=1 ./internal/chat -> PASS
go test -count=1 ./... -> PASS
PostgreSQL 16 efêmero: migrations 0001 e 0019 -> PASS
CHAT_TRAVEL_V2_SHADOW_POSTGRES_TEST_URL=<PostgreSQL 16 efêmero> go test -race -count=1 -v ./internal/chat -run '^TestTravelQueryV2ShadowRecoveryPostgresMarkerLifecycleAndBoundedProgress$' -> PASS
git diff --check -> PASS
```

Na execução de observabilidade descrita acima, o repository e o SQL não foram
alterados. A integração PostgreSQL então executada em 2026-07-17 comprovou por
chamada real do repository o lifecycle claim ->
`COMPLETED`, o retorno de `recovery_due_at` a `NULL` e o progresso bounded. Os
probes manuais de produção anteriores permanecem evidência histórica e não são
reclassificados como execução nova deste patch.

#### Review local e gate restante

O review local encontrou um P2: a primeira versão dos logs removia o campo
`event` já usado operacionalmente. A correção preserva `event` e adiciona
`reason`; o review final local não encontrou P1/P2.

O patch de observabilidade permitiu obter a causa específica descrita abaixo.

#### Correção do SQLSTATE 22P02 (2026-07-17)

A nova evidência de produção fechou o caminho até a falha:

```text
scheduler -> scheduled
job -> started
ClaimTravelQueryV2Shadow -> postgres SQLSTATE 22P02
RecoverExpiredTravelQueryV2ShadowClaims -> postgres SQLSTATE 22P02
pgx -> QueryExecModeExec
```

A hipótese foi confirmada no repository. Os quatro payloads destinados a
parâmetros SQL com cast `::jsonb` eram retornados diretamente por
`json.Marshal` como `[]byte`:

```text
aquisição da claim -> recordPayload
recuperação de claim expirada -> recoveredPayload
completion -> recordPayload
recovery em lote -> terminalPayload
```

Em `QueryExecModeExec`, o pgx envia parâmetros sem descrever previamente os
OIDs e infere a codificação pelo tipo Go. O `[]byte` é codificado como `bytea`;
o conteúdo resultante não é JSON textual válido para o cast `::jsonb`, causando
`22P02`. O patch centraliza somente esses quatro marshals em
`encodeTravelQueryV2ShadowJSON`, que devolve `string` contendo JSON válido. As
queries e todos os casts `::jsonb` foram preservados.

O teste unitário cobre validade e round trip do JSON textual. A integração
PostgreSQL agora fixa `pgx.QueryExecModeExec`, reproduzindo a configuração da
API ao exercitar claim, completion e recovery. A URL de integração PostgreSQL
não estava disponível nesta execução, portanto esse teste permaneceu `SKIP`
local e ainda exige execução com banco efêmero ou no CI apropriado.

#### Arquivos alterados na correção 22P02

```text
apps/api/internal/chat/repository.go
apps/api/internal/chat/travel_query_v2_shadow_repository_test.go
docs/EXECUTION_TRACKER.md
```

#### Validação da correção 22P02

Os comandos Go usaram `GOCACHE=/tmp/schumacher-go-build` e `GOTMPDIR=/tmp`.
A primeira tentativa de `go test -count=1 ./...` foi inconclusiva por quota de
disco; após limpar somente caches Go temporários antigos, a repetição passou.

```text
gofmt -w internal/chat/repository.go internal/chat/travel_query_v2_shadow_repository_test.go -> PASS
go test -count=1 ./internal/chat -run 'TestEncodeTravelQueryV2ShadowJSON|Test.*Travel.*V2.*Shadow' -> PASS
go test -race -count=1 ./internal/chat -run 'Test.*Travel.*V2.*Shadow|Test.*Shadow.*Recovery' -> PASS
go test -count=1 ./internal/chat -> PASS
go test -count=1 ./... -> PASS na repetição após liberar caches temporários
CHAT_TRAVEL_V2_SHADOW_POSTGRES_TEST_URL=<ausente> TestTravelQueryV2ShadowRecoveryPostgresMarkerLifecycleAndBoundedProgress -> SKIP local
git diff --check -> PASS
```

O review local da correção 22P02 não encontrou P1/P2. H-2026-07-16A permanece
em validação operacional: o patch local verde não substitui o novo smoke com o
mesmo `QueryExecModeExec` da produção.

```text
mudança user-visible: nenhuma
commit/push/deploy: não executados
teste em produção: obrigatório após implantação autorizada do patch
smoke exigido: nova mensagem segura -> scheduled -> claim_acquired -> provider_started/provider_completed -> completion_completed; claim COMPLETED; recovery_due_at NULL; sweep_done 0/0/0; nenhum sweep_failed
se houver falha: usar somente reason, operation, error_class, sqlstate, timeout e canceled; não registrar payload ou SQL
riscos restantes: integração PostgreSQL em QueryExecModeExec não executada localmente por ausência da URL; fechamento operacional depende do novo smoke; capacity_full continua fail-open por contrato
próxima ação histórica daquela rodada: revisar e, por fluxo autorizado, implantar o patch 22P02 e executar o smoke sanitizado; H-2026-07-16B e 3.6F-D permaneciam bloqueados
```

### 8.10 Bug user-visible — H-2026-07-16B (2026-07-16)

**Status:** **EM ANDAMENTO — gate operacional de B1 reaberto por
H-2026-07-27A; B2 bloqueada**.

O H-B agora é umbrella não executável. O 3.6F-D permanece **BLOQUEADA por
H-2026-07-16B**.

#### Sexto review — 9 P1 e divisão do umbrella

O sexto review histórico comprovou que snapshot V1, versão, ledgers, reducer
tipado e limpeza da correção zero ainda não fechavam a raiz temporal, de
autoridade e proveniência. Naquele sexto review histórico, o patch então vigente
não era seguro para commit, deploy ou smoke. Esse estado foi posteriormente
superseded pelo review final sem P1/P2 de B1.

| ID | P1 | Slice responsável | Prova obrigatória |
|---|---|---|---|
| P1-01 | parser lexical novo de família e crescimento de regex/listas | B1 remove a interpretação da fundação; B2 fornece o meaning semântico separado | B1: `TestPassengerStateFoundationDoesNotAddLexicalFamilyRules` e inventário `regexp.MustCompile == 54`; B2: `TestPassengerMeaningV1ValidatorDoesNotParseCurrentTurn` |
| P1-02 | bootstrap reparsa inbound da janela histórica | B1 | `TestPassengerStateBootstrapUsesStructuredEvidenceOnly` |
| P1-03 | `prompt_event` não acompanha o outbound efetivamente enviado | B1 | `TestPassengerPromptEventFollowsReviewedAndAutoSentOutboundBeyondHistoryWindow` |
| P1-04 | contexto infantil deriva de `ActivePrompt.Kind`, não da época persistida | B1 | `TestPassengerChildAddsTravelerUsesPersistedPromptEpoch` |
| P1-05 | estado inseguro chega a shadows, LLMs e tools | B1 | `TestPassengerGateAfterDeliveredPromptStopsExternalWorkBeforeDispatch` |
| P1-06 | `BookingDraftContext` recupera contagem de `booking_create` histórico | B1 | `TestBookingDraftProjectionIgnoresBookingCreatePassengerCount` |
| P1-07 | booking criado avança payment antes da validação dos slots | B1 | `TestBookingCreatedWithUnknownPassengerSlotsFailsClosed` |
| P1-08 | correção de total preserva `ChildUnder5AddsTraveler` incompatível | B1 | `TestPassengerAggregateCorrectionClearsDependentAddsTraveler` |
| P1-09 | `Reprocess` concorrentes podem perder atualização do snapshot | B1 | `TestPassengerStateConcurrentReprocessPreservesBothEvents` e `TestPassengerStateApplyEventsSerializesSessionPostgres` |

Decisão histórica — **SUPERADA** pela fila canônica no topo deste tracker:

```text
H-B1 — CONCLUÍDA EM CÓDIGO — GATE OPERACIONAL REABERTO
  evidência de sessão nova mostrou bootstrap UNKNOWN bloqueando antes do
  contexto de passageiros
H-2026-07-22A — CORRIGIDO E DEPLOYADO; SMOKE OPERACIONAL RED
  bootstrap invalidado, autoridade explícita, source legado exato e ordem causal
H-2026-07-27A — registro operacional histórico; status e contagens SUPERADOS
  consultar exclusivamente a fila canônica no topo deste tracker
H-B2 — BLOQUEADA por H-2026-07-27A / gate operacional de H-B1
  PassengerClarificationMeaningV1 strict + validator + corpus + shadow;
  só inicia após review, deploy e smoke verdes do hotfix ativo
H-B3 — BLOQUEADA por H-B2
  promoção somente em prompt passageiro/criança e decisão
  WEAK/FALLBACK/UNKNOWN; sem booking/payment direto
3.6F-D — BLOQUEADA por H-B
```

A autoridade arquitetural é
`docs/adr/ADR-2026-07-passenger-authority-and-serialization.md`. As ADRs
anteriores são somente históricas. O plano futuro isolado de meaning foi
superseded pelo B2.

Critérios de desbloqueio:

- B1 libera B2 somente com as nove regressões, inventário lexical sem
  crescimento, teste PostgreSQL concorrente real, matriz completa, review sem
  P1/P2 e H-2026-07-22A revisado, implantado e com smoke verde;
- B2 libera B3 somente com contrato strict, corpus/evaluator, shadow sem
  influência runtime, métricas críticas zeradas, limiares aprovados e review
  sem P1/P2;
- B3 fecha o umbrella somente com gates runtime, review sem P1/P2,
  rollout/rollback e smoke quando explicitamente autorizado, sem incidente
  aberto;
- somente o fechamento integral do umbrella permite reavaliar 3.6F-D.

Arquivos documentais deste replanejamento:

```text
docs/adr/ADR-2026-07-passenger-authority-and-serialization.md
docs/adr/ADR-2026-07-passenger-clarification-state.md
docs/adr/ADR-2026-07-passenger-state-durable-events.md
plans/README.md
plans/00-plano-mestre-travel-semantic-v2.md
plans/h-2026-07-16b-passenger-child-state.md
plans/h-2026-07-16b1-passenger-state-foundation.md
plans/h-2026-07-16b2-passenger-meaning-v1.md
plans/h-2026-07-16b3-passenger-meaning-runtime.md
plans/p-2026-07-passenger-clarification-meaning-v1.md
docs/EXECUTION_TRACKER.md
```

Resultado do sexto review: **9 P1; H-B não concluída**. Código de produção e
testes não foram alterados por este replanejamento.

Validação executada neste replanejamento documental:

```text
PASS — git diff --check
PASS — whitespace check dos sete artefatos documentais untracked
PASS — existência dos quatro novos artefatos canônicos
PASS — checksum do diff de apps/api e hashes dos seis arquivos Go untracked idênticos ao baseline anterior às edições documentais
SKIP JUSTIFICADO — testes Go não executados; nenhum código de produção ou teste foi alterado nesta rodada
```

Commit, push, deploy e smoke não foram executados. Próxima ação única: executar
somente H-2026-07-16B1 por `/goal` explícito.

#### Execução local do H-2026-07-16B1 — 2026-07-20 a 2026-07-21

**Status do slice:** **CONCLUÍDA — REVIEW FINAL SEM P1/P2 — SEGURA PARA
COMMIT**.

#### Review final — sem P1/P2

O review final confirmou as provas numérica e contextual, ausência de claim V2,
shadows V1/V2 zerados depois da janela assíncrona e nenhuma alteração de
produção na correção final. O B1 está seguro para commit e libera somente o B2
como próximo slice; isso não autoriza iniciar B2 nesta rodada nem conclui o
umbrella H-B.

Evidência de fechamento:

```text
PASS — review final sem P1/P2
PASS — matrizes obrigatórias com -count=20
PASS — go test -race -count=1 ./internal/chat
PASS — regressões H-012/document/lap-child/payment/human/out-of-turn
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS SEM SKIP — PostgreSQL real
PASS — inventário de produção em 54 regexp.MustCompile
PASS — git diff --check
```

Os blocos de reviews intermediários abaixo são registros históricos das
respectivas rodadas. Todos foram superseded pelo review final sem P1/P2 e não
definem o status vigente de B1 nem a fila canônica atual.

O review anterior encontrou uma lacuna de prova restante, sem demonstrar
defeito novo de produção:

- **P1 pós-janela assíncrona:** `travel.calls == 0` era verificado antes da
  espera negativa em `claimAttempts`; a ausência de claim durante 25 ms não era
  seguida por uma observação direta de que Travel V2 e o shadow V1 continuavam
  com zero chamadas.

Naquela rodada, a correção permaneceu exclusivamente em
`incremental_flow_test.go`: as asserções anteriores e a espera negativa
limitada foram preservadas, e `travel.calls == 0` e `openAI.calls == 0`
passaram a ser verificados novamente imediatamente depois da janela. O status
histórico daquela rodada, posteriormente superseded pelo review final de B1,
era: H-B permanecia aberto/em andamento e ainda não liberava os sucessores; B2
estava bloqueada por B1, B3 por B2 e 3.6F-D pelo umbrella H-B.

O review anterior encontrou duas lacunas de prova, sem demonstrar defeito
novo de produção:

- **P1 contextual:** o único marker legado sem boundary e sem outbound
  mascarador usava `"1"`; `"essa msm"` aparecia apenas em cenário com boundary
  explícita e DRAFT posterior;
- **P1 de árvore do draft:** índice e snapshot eram verificados, mas
  `trip_id`, `board_stop_id` e `alight_stop_id` podiam permanecer em
  `template_data`, request/response, tool context ou listas aninhadas.

A correção ficou exclusivamente em `incremental_flow_test.go`: os casos legado
numérico e contextual agora usam availability antiga completa sem outbound
posterior confiável e capturam history, canonical state, active prompt e
decisão diretamente no router. Um helper recursivo percorre maps e slices e
rejeita as três chaves de rota em qualquer profundidade do history e dos dois
payloads do draft. O controle positivo numérico foi preservado; o novo controle
contextual usa availability pós-boundary com IDs distintos e comprova que
somente a viagem nova é selecionada. O teste focado passou sem alteração de
produção.

O review anterior encontrou uma lacuna de prova no cenário de marker legado
sem boundary:

- **P1 de prova:** o caso principal continha uma availability antiga completa
  seguida por outro outbound `AUTOMATION_SENT` sem facts. Como
  `latestReliableAssistantMessage` parava no outbound posterior, o router já
  não alcançava a availability antiga mesmo sem a lógica de boundary/overlay.

A correção foi exclusivamente na fixture e nas asserções: o outbound posterior
foi removido do caso principal, que agora contém somente availability antiga
`AUTOMATION_SENT`, visível, completa e confiável, marker booleano sem boundary,
estado de passageiros seguro e turno atual `"1"`. O teste captura diretamente
history e canonical state do deterministic router e comprova ausência de
availability facts, índice, snapshot, trip/stops e active prompt de escolha
antigos. O teste focado passou sem alteração em `service.go`,
`availability_invalidation_history.go` ou qualquer outro arquivo de produção.

O review anterior encontrou a lacuna causal restante:

- **P1:** `canonical_availability_facts_invalidated=true` sanitizava o estado,
  mas `InferActivePromptContext`, `routeDeterministicIntent` e os helpers de
  materialização ainda recebiam o histórico completo. Um outbound posterior
  invisível fazia `latestReliableAssistantMessage` recuar até uma availability
  `SENT` anterior à invalidação, selecionar a viagem antiga e limpar o marker.

Raiz confirmada: o marker era booleano e não definia qual inbound separava os
facts invalidados de uma nova evidência entregue. RED real:
`TestPreInvalidationAvailabilityHistoryBoundaryDoesNotReachRouter` retornou
`SELECT_AVAILABILITY_OPTION` para a availability pré-boundary antes do patch.

Correção local estrita do P1:

- o marker persiste também
  `canonical_availability_facts_invalidated_after_message_id` com o ID do
  inbound causal e timestamp somente como fallback;
- um overlay read-only oculta `availability_search`, índice, snapshot, prompt
  de escolha e mirror `BOT_AUTO_REPLY` pré-boundary sem alterar o transcript;
- o mesmo overlay alimenta estado canônico, active prompt, router, booking
  draft, interpreters e todos os helpers de seleção do `Reprocess`;
- DRAFT, BLOCKED, MANUAL_PENDING, SEND_FAILED, mirror sem fonte e item
  incompleto posteriores à boundary não reabrem seleção;
- somente prompt posterior, confiavelmente enviado, visível e com
  `trip_id`/`board_stop_id`/`alight_stop_id` completos permite seleção atômica
  e remove marker + boundary;
- marker legado sem boundary recebe fronteira conservadora no primeiro turno;
  se o message ID saiu do `LIMIT 50`, o timestamp mantém toda evidência antiga
  fechada;
- contexto independente de passageiro, documento, booking, payment, handoff,
  cancelamento e endpoints permanece disponível. Não houve regex ou
  vocabulário novo, nem alteração de reducer, repository, Travel V2 ou B2.

O review anterior encontrou um novo bloqueador de ordenação:

- **P1:** quando `canonical_availability_facts_invalidated=true` já está
  persistido, `InferActivePromptContext` e `routeDeterministicIntent` ainda
  recebem o estado reconstruído do histórico antes da invalidação; o router
  pode observar índice, IDs de rota e `LastToolFacts.availability_search`
  antigos.

O **P1 anterior permanece corrigido**: a limpeza estrutural continua
persistida, preserva fatos independentes e mantém a materialização atômica da
seleção atual completa. Esta rodada altera somente a ordem da limpeza antes dos
consumidores; reducer, repository pós-booking, parser, regex, Travel V2 e B2
permanecem fora de escopo.

RED real: `TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState`
capturou rota/facts stale no input do router para stale, blocker posterior e
item incompleto; o controle positivo também capturou o estado antigo antes de
o router avaliar a nova availability.

Correção local: o marcador agora é lido imediatamente depois do reload e da
derivação pós-`ApplyPassengerClarificationEventsV1`; a invalidação sincroniza
`structuredCanonicalState`, `structuredInput.State`, `canonicalState`, `agent`
e `memory` antes de active prompt e router. Depois do router, somente uma
seleção atual, visível e completa materializa availability e limpa o marcador.

Um review intermediário, então o mais recente, encontrou um bloqueador
restante:

- **P1:** quando o estado de passageiros é inseguro e a seleção atual é stale,
  invisível, bloqueada ou incompleta, o fail-closed não materializa a seleção,
  mas ainda persiste em `metadata.agent.canonical_state` rota e
  `LastToolFacts.availability_search` derivados do histórico antigo.

Os **três P1 anteriores permaneciam corrigidos naquela rodada**: a
materialização continuava atômica e exigia os três IDs, a matriz adversarial
continuava sem autoridade 1/0 pré-semeada e a autoridade `POST_BOOKING`
continuava atualizada pela composição ativa. A correção daquela rodada deveria
invalidar somente fatos derivados da availability não confiável, preservar
origem/destino independentes e manter o controle positivo completo. O status
histórico daquela rodada, posteriormente superseded pelo review final de B1,
era: H-B permanecia aberto/em andamento e ainda não liberava os sucessores; B2
estava bloqueada por B1, B3 por B2 e 3.6F-D pelo umbrella H-B.

O review anterior encontrou três bloqueadores:

- **P1-A:** o writer fail-closed ainda materializa índice e snapshot quando o
  item visível não possui `trip_id`, `board_stop_id` ou `alight_stop_id`;
- **P1-B:** a cobertura stale/invisível/blocker usa autoridade 1/0 pré-semeada
  e não exercita o ramo fail-closed sem `PassengerClarificationStateV1`;
- **P1-C:** depois de persistir autoridade `POST_BOOKING`, o reload descarta a
  consulta ativa mais recente e reutiliza o snapshot antigo, inclusive quando
  todos os passageiros são desativados.

Os **7 P1 anteriores e o filtro inicial de `is_active=true` permaneciam
corrigidos naquela rodada**. Aquele review não reabria bootstrap estruturado,
entrega do prompt, limpeza de derivados, serialização, race, fake, precedência
`STRONG` ou o filtro SQL inicial; exigia completar a atomicidade, a matriz
adversarial e o refresh da autoridade pós-booking. O status histórico daquela
rodada, posteriormente superseded pelo review final de B1, era: H-B permanecia
aberto/em andamento e ainda não liberava os sucessores; B2 estava bloqueada por
B1, B3 por B2 e 3.6F-D pelo umbrella H-B.

RED real daquele review intermediário: snapshot incompleto ainda podia ser
persistido; os casos stale/invisível/blocker não comprovavam fail-closed sem
autoridade; e uma desativação posterior não atualizava nem removia autoridade
`POST_BOOKING` stale.
As reproduções locais confirmaram antes do patch o vazamento de índice/snapshot
nos casos stale e incompleto e, no PostgreSQL 16, o reload 4/2 permaneceu 4/2
depois da desativação parcial.

Correção local dos três P1 em 2026-07-21:

- uma única política de materialização exige `trip_id`, `board_stop_id` e
  `alight_stop_id` do item atual antes de expor índice, snapshot ou rota; o
  índice e o snapshot são anexados atomicamente por todos os writers revisados;
- stale, outbound invisível, blocker posterior e item atual incompleto percorrem
  o fail-closed sem snapshot/eventos nem autoridade 1/0, retornam
  `ASK_PASSENGER_COUNT` sem fatos de seleção e não acionam LLM, shadows,
  document extraction ou tools; o controle positivo completo permanece
  recuperável depois do `LIMIT 50`;
- o reload sob `FOR UPDATE` consulta sempre booking e passageiros ativos,
  substitui e persiste 4/2 -> 2/1, limpa 0 ativo para UNKNOWN e propaga erro de
  consulta sem reutilizar snapshot stale.

O novo review de 2026-07-21 encontrou dois bloqueadores restantes:

- **P1:** no fail-closed de uma sessão sem snapshot/eventos de passageiros, a
  seleção válida podia gerar `ASK_PASSENGER_COUNT` sem persistir atomicamente
  `selected_option_index`, `selected_availability_result` e os facts completos
  de rota; após a mensagem sair do `LIMIT 50`, a seleção era perdida;
- **P2:** a autoridade pós-booking incluía `booking_passengers.is_active=false`
  no total e em lap children, inflando também a projeção documental.

Os **7 P1 do sétimo review permaneciam corrigidos naquela rodada**. Aquele novo
RED não reabria bootstrap, confirmação de entrega, limpeza de `adds_traveler`,
serialização dos writers, race da fixture, autoridade do fake nem precedência
dos guardrails `STRONG`. O status histórico daquela rodada, posteriormente
superseded pelo review final de B1, era: H-B permanecia aberto/em andamento e
ainda não liberava os sucessores; B2 estava bloqueada por B1, B3 por B2 e
3.6F-D pelo umbrella H-B.

RED real do novo review: as reproduções adversariais confirmaram perda da
seleção após truncamento da janela e contagem de passageiros pós-booking
inativos. As novas regressões reproduziram os dois defeitos antes do patch:
`TestSelectedAvailabilitySelectionPassengerFailClosedPersistsBeyondHistoryWindow`
encontrou `selected_option_index=0`, e
`TestPassengerPostBookingAuthorityIgnoresInactivePassengersPostgres` encontrou
`PassengerCount=4`/`ChildUnder5Count=2` quando a autoridade ativa era `2/1`.

Correção local dos dois achados em 2026-07-21:

- o ramo fail-closed reutiliza `attachCurrentAvailabilitySelectionContext` e
  `attachSelectedAvailabilityResultToTemplateRun`; somente uma lista atual,
  visível e completa materializa, no mesmo draft, índice, snapshot selecionado,
  IDs de rota e facts de availability, sem novo caminho metadata-only;
- a consulta da autoridade pós-booking aplica
  `booking_passengers.is_active = true` ao conjunto agregado; o mesmo conjunto
  controla total, lap children e `ExpectedDocumentCount`, e total ativo zero
  retorna ausência de autoridade pós-booking;
- a regressão de seleção não pré-semeia snapshot/eventos de passageiros,
  remove o inbound da janela de 50 e comprova a projeção pelo draft persistido,
  com zero LLM, JSON, shadows e tools;
- a integração PostgreSQL cobre adulto/criança ativos e inativos, além de
  booking sem passageiro ativo. O teste de serialização existente recebeu o
  sufixo `Postgres` para ser selecionado pela regexp obrigatória sem mudar sua
  lógica.

Arquivos desta correção do novo review:

```text
apps/api/internal/chat/service.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/passenger_clarification_repository_test.go
docs/EXECUTION_TRACKER.md
plans/h-2026-07-16b1-passenger-state-foundation.md
plans/h-2026-07-16b-passenger-child-state.md
```

O sétimo review histórico encontrou sete bloqueadores: bootstrap dependente do
`LIMIT 50`; replay de prompt sem entrega confirmada; `adds_traveler` stale após
correção não solo; lost update em escritores de metadata; corrida na fixture
concorrente; fake que reparsa/fabrica autoridade; e precedência incorreta do
gate de passageiros sobre guardrails `STRONG`. Naquela rodada histórica, H-B
permanecia aberto/em andamento e ainda não liberava os sucessores; B2 estava
bloqueada por B1, B3 por B2 e 3.6F-D pelo fechamento integral do umbrella H-B.
Esse estado foi posteriormente superseded pelo review final de B1. As evidências
de execução abaixo registram aquela rodada e ficam superseded pelo fechamento
posterior.

RED real do sétimo review: as reproduções adversariais confirmaram dependência
da janela, replay de prompt não entregue, derivado stale após correção,
lost update de metadata, corrida na fixture, autoridade inventada/reparseada no
fake e precedência incorreta do gate sobre decisões locais seguras/`STRONG`.

Antes, composição e época ainda podiam ser reconstruídas por transcript/body,
`ActivePrompt` textual ou `tool_context.booking_create`; o evento do prompt
ficava associado ao draft; snapshot concorrente podia sobrescrever atualização
mais recente; e slots inseguros ainda alcançavam shadow ou tools.

Depois:

- `PassengerClarificationStateV1` versionado é a autoridade pré-booking, com
  validator, proveniência, épocas, reasons e ledgers idempotentes;
- bootstrap usa somente eventos canônicos em `normalized_payload` ou
  booking/passengers persistidos; ausência de evidência e artefato estrutural
  malformado falham fechados;
- reducer recebe somente estado e eventos tipados, sem texto, transcript,
  regex, tokens ou `ActivePromptContext`;
- atualização usa transação curta com `SELECT ... FOR UPDATE`, reload do estado
  mais recente, redução/validação e persistência; `SaveReprocessSnapshot`
  preserva o snapshot concorrente mais novo;
- draft guarda `pending_prompt_event`; auto-send e review aprovado copiam o
  evento para o outbound efetivo, e `MarkReplyDeliverySent` aplica a época na
  mesma transação curta que registra o envio;
- estado desconhecido, conflitante, corrompido ou inválido bloqueia OpenAI
  shadow, Travel V2, document extraction e qualquer tool antes do dispatch;
- `BookingDraftContext` apenas projeta o snapshot para composição/época, sem
  recuperar passenger count histórico nem inferir lap child para preencher
  slot desconhecido;
- booking/payment continuam bloqueados até os slots obrigatórios válidos; após
  booking, booking e passageiros persistidos têm precedência.

Correções adicionais após o sétimo review:

- bootstrap dedicado lê a sessão inteira somente por colunas estruturadas e
  nunca seleciona `body`;
- `delivery_recorded_at` junto de status canônico de envio é obrigatório para
  aplicar ou reexecutar `passenger_prompt_event`;
- correção não solo limpa `adds_traveler` e origem para qualquer proveniência
  diferente de `SOLO_SPEAKER`;
- `UpdateDraftAutoSendState`, falha/retry e demais substituições de metadata
  relêem a linha sob `FOR UPDATE`; updates por caminho JSONB não substituem
  snapshot concorrente;
- fake de `Reprocess` sem evidência retorna `UNKNOWN`, com autoridade de cada
  cenário semeada explicitamente; clones concorrentes ficam sob o mutex;
- humano/cancelamento e decisões `STRONG` aplicáveis precedem o fail-closed;
  dúvida paralela segura usa template local e preserva o prompt pendente sem
  disparar LLM, shadow, document extraction ou tool.

Arquivos de produção e contrato alterados:

```text
apps/api/internal/chat/availability_draft.go
apps/api/internal/chat/active_prompt_context.go
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

Testes e fixtures alterados/adicionados:

```text
apps/api/internal/chat/agent_rollout_test.go
apps/api/internal/chat/active_prompt_context_test.go
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

Documentação atualizada neste slice:

```text
docs/EXECUTION_TRACKER.md
docs/adr/ADR-2026-07-passenger-authority-and-serialization.md
docs/adr/ADR-2026-07-passenger-clarification-state.md
docs/adr/ADR-2026-07-passenger-state-durable-events.md
plans/00-plano-mestre-travel-semantic-v2.md
plans/README.md
plans/h-2026-07-16b-passenger-child-state.md
plans/h-2026-07-16b1-passenger-state-foundation.md
plans/h-2026-07-16b2-passenger-meaning-v1.md
plans/h-2026-07-16b3-passenger-meaning-runtime.md
plans/p-2026-07-passenger-clarification-meaning-v1.md
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
dois achados daquela rodada foram corrigidos e a matriz local, inclusive
PostgreSQL real sem `SKIP`, passou. **Naquele momento, nenhum novo review havia
sido executado depois do patch e a rodada ainda não declarava review limpo.
Esse estado foi posteriormente superseded pelo review final sem P1/P2**.

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

Matriz exigida para o P1 da fronteira causal do histórico:

```text
RED reproduzido — availability antiga SENT + boundary + DRAFT posterior + "essa msm" produzia SELECT_AVAILABILITY_OPTION sobre a viagem antiga
PASS — go test -count=20 ./internal/chat -run 'Test.*AvailabilityInvalidation.*HistoryBoundary|Test.*PreInvalidation.*Router|Test.*PostInvalidation.*FreshAvailability'
PASS — matriz DRAFT/BLOCKED/MANUAL_PENDING/SEND_FAILED/item incompleto/BOT_AUTO_REPLY sem fonte mantém marker + boundary, sem índice/snapshot/trip/stops e com zero LLM/shadow/tools
PASS — controle positivo seleciona somente a availability posterior, enviada e completa, persiste índice + snapshot atomicamente e remove marker + boundary
PASS — boundary fora do LIMIT 50 usa timestamp conservador sem reautorizar availability antiga
PASS — overlay não muta o histórico e preserva passageiro, booking, payment, handoff, humano/cancelamento STRONG e endpoints independentes
PASS — go test -race -count=1 ./internal/chat
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Document|Test.*LapChild|Test.*Payment|Test.*HumanSupport|Test.*OutOfTurn'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — inventário de produção confirmado em 54 regexp.MustCompile
PASS — git diff --check
```

Correção local da lacuna de prova do review anterior:

```text
PASS — fixture principal sem outbound posterior confiável: availability antiga SENT, completa e confiável + marker legado sem boundary + passageiro seguro + turno atual "1"
PASS — router recebe history sem availability_search, selected_option_index, selected_availability_result ou prompt de escolha antigos e canonical state sem trip/stops antigos
PASS — intent diferente de SELECT_AVAILABILITY_OPTION; draft sem índice/snapshot; marker preservado; boundary conservadora persistida usando o inbound atual
PASS — zero LLM, JSON runner, shadows V1/V2, availability_search, booking, payment, payment_status e tools
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
PASS — history, canonical state, active prompt e IntentDecision capturados diretamente no router para ambos os caminhos
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

Resultado histórico do review final daquela rodada: **sem P1/P2; H-B1 então
considerada segura para commit**. A evidência operacional de 2026-07-22
supersede o desbloqueio descrito abaixo: H-2026-07-22A reabre o gate de B1 e
volta a bloquear B2.

Naquele fechamento, H-2026-07-16B permaneceu **EM ANDAMENTO**, H-B2 passou a
**PRÓXIMA**, H-B3 permaneceu **BLOQUEADA por H-B2** e 3.6F-D continuou
**BLOQUEADA pelo fechamento integral de H-B**.

Teste em produção/smoke: **não executado e não autorizado nesta rodada**.
Commit, push e deploy: **não executados naquela rodada**. A próxima ação atual
está no registro de H-2026-07-22A e não autoriza executar B2.

#### Manifesto completo do commit de B1

**1. Produção B1**

```text
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/availability_draft.go
apps/api/internal/chat/availability_invalidation_history.go
apps/api/internal/chat/booking_create_router.go
apps/api/internal/chat/booking_draft_context.go
apps/api/internal/chat/conversation_state_machine.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/model.go
apps/api/internal/chat/passenger_clarification_evidence.go
apps/api/internal/chat/passenger_clarification_reducer.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/response_realizer.go
apps/api/internal/chat/service.go
apps/api/internal/chat/tool_router.go
```

**2. Testes B1**

```text
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/agent_rollout_test.go
apps/api/internal/chat/availability_draft_test.go
apps/api/internal/chat/booking_create_router_test.go
apps/api/internal/chat/booking_draft_context_test.go
apps/api/internal/chat/cargo_router_test.go
apps/api/internal/chat/chat_flow_guardrails_test.go
apps/api/internal/chat/conversation_state_machine_test.go
apps/api/internal/chat/handler_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/interpreter_shadow_report_endpoint_test.go
apps/api/internal/chat/openai_interpreter_assist_test.go
apps/api/internal/chat/openai_travel_query_v2_test.go
apps/api/internal/chat/passenger_clarification_evidence_test.go
apps/api/internal/chat/passenger_clarification_reducer_test.go
apps/api/internal/chat/passenger_clarification_repository_test.go
apps/api/internal/chat/passenger_clarification_state_v1_test.go
apps/api/internal/chat/passenger_clarification_test_helper_test.go
apps/api/internal/chat/tool_router_test.go
```

**3. Documentação/ADR B1**

```text
docs/EXECUTION_TRACKER.md
docs/adr/ADR-2026-07-passenger-authority-and-serialization.md
docs/adr/ADR-2026-07-passenger-clarification-state.md
docs/adr/ADR-2026-07-passenger-state-durable-events.md
plans/h-2026-07-16b1-passenger-state-foundation.md
```

**4. Planejamento futuro B2/B3**

```text
plans/00-plano-mestre-travel-semantic-v2.md
plans/README.md
plans/h-2026-07-16b-passenger-child-state.md
plans/h-2026-07-16b2-passenger-meaning-v1.md
plans/h-2026-07-16b3-passenger-meaning-runtime.md
plans/p-2026-07-passenger-clarification-meaning-v1.md
```

**5. Fora de escopo**

```text
nenhum arquivo identificado na auditoria do working tree
```

#### Histórico das cinco rodadas anteriores — superseded pelo sexto review

#### Reprodução

```text
entrada: "eu e meus 2 filhos que são criança"
observado: passenger_count=2
esperado: 3 viajantes

active prompt: ASK_CHILD_UNDER_5
entrada: "sim o mais novo de 4 anos"
observado: child_under_5_count_known=false e pergunta repetida
esperado: child_under_5_count=1, estado conhecido e avanço do fluxo
```

Este bug pertence ao estado determinístico de passageiros, não ao contrato `TravelQueryMeaningV2`.

#### Primeiro review — 4 P1

| Prioridade | Achado | Impacto |
|---|---|---|
| P1 | total da viagem e subgrupo familiar não são reconciliados por significado/força | em `somos 3, meus 2 filhos vão viajar comigo`, o subgrupo 2 podia vencer o total explícito 3 e reduzir `expected_document_count`. |
| P1 | incerteza não está limitada ao slot qualificado | em `somos 3, mas não sei se alguma criança tem até 5 anos`, a incerteza infantil podia apagar o total inequívoco e repetir a pergunta de passageiros. |
| P1 | idade sem unidade preservada interpreta meses como anos | em `meu filho de 10 meses`, o valor 10 podia ser tratado como anos e excluir indevidamente a criança de colo. |
| P1 | menções de idade são contadas como pessoas sem preservar a força/identidade da evidência | referências repetidas podiam somar a mesma criança ou uma referência singular posterior podia reduzir uma contagem exata anterior. |

Os quatro achados bloqueiam o fechamento de H-2026-07-16B. O 3.6F-D
permanece **BLOQUEADA por H-2026-07-16B** até correção, validação e novo review
sem P1/P2. Nenhum smoke está autorizado neste ciclo.

#### Correção local após o review

- candidatos de total explícito, composição com interlocutor e subgrupo familiar
  são extraídos separadamente e reconciliados por significado; total explícito
  compatível prevalece e contradição mantém o total desconhecido com marcador
  explícito, sem reutilizar silenciosamente total histórico;
- a incerteza é limitada ao primeiro slot qualificado depois do marcador,
  inclusive quando a informação certa e a incerta estão na mesma cláusula;
- menções de idade preservam `anos` ou `meses`; meses são comparados como meses,
  e números sem unidade só são aceitos com contexto infantil inequívoco;
- evidência infantil distingue contagem exata, confirmação de ao menos uma
  criança, referência a criança já mencionada e desconhecido; o merge histórico
  compara força da evidência, não quantidade de menções;
- referências repetidas não somam pessoas, referência singular não reduz
  contagem exata anterior e o mesmo histórico produz o mesmo estado no replay.

#### Segundo review — 4 P1

| Prioridade | Achado | Impacto |
|---|---|---|
| P1 | conflito histórico antigo vence correção explícita posterior | `somos 2, meus 3 filhos vão viajar comigo` seguido de `somos 3` podia reconstruir `PassengerCount=0`, conhecido falso e conflito verdadeiro. |
| P1 | total de viajantes e contagem infantil não possuem reconciliação cruzada | `somos 1, duas têm 3 e 4 anos` podia manter total 1 e duas crianças conhecidas, pedir somente um documento e avançar para estado impossível. |
| P1 | referências infantis explicitamente distintas são colapsadas | `meu filho tem 3 anos e minha filha tem 4 anos` podia produzir somente uma criança menor de 5 e permitir cobrança indevida da outra. |
| P1 | o active prompt não é propagado até o cálculo de documentos | após `só pra mim` e `ASK_CHILD_UNDER_5`, `uma tem 4` podia ser reconhecida no parser contextual e depois reinterpretada sem contexto, deixando `expected_document_count=1`. |

As alegações anteriores de precedência, replay e documentos esperados deixaram
de ser suficientes diante deste review. A rodada foi aberta como **EM CORREÇÃO
APÓS SEGUNDO REVIEW — 4 P1**; após o RED/PASS abaixo, o status atual é
**CORREÇÕES DOS 4 P1 IMPLEMENTADAS — MATRIZ LOCAL VERDE; AGUARDANDO NOVO
REVIEW**. O 3.6F-D continua **BLOQUEADA por H-2026-07-16B**. Não há autorização
para commit, push, deploy ou smoke.

#### Correção após o segundo review

- `passenger_clarification_evidence.go` concentra tipos, extração contextual,
  origem/recência da evidência e política explícita de reconciliação;
- conflito antigo não possui mais precedência por causa do valor numérico do
  enum: `somos 3` posterior restaura `PassengerCount=3`, conhecido verdadeiro e
  conflito falso;
- a reconciliação cruzada fecha o conjunto quando
  `ChildUnder5Count > PassengerCount`, mantém os slots não acionáveis e direciona
  para `ASK_PASSENGER_CLARIFICATION`, sem documentos ou booking;
- referências explicitamente distintas `meu filho` e `minha filha` produzem
  duas identidades; repetição ou possível correferência não incrementa;
- `Service.Reprocess` extrai o turno uma vez com `ActivePromptContext` e propaga
  a mesma evidência para `collectBookingDraftContextWithCurrentEvidence` e para
  o cálculo de documentos, sem reparse contextual em helper posterior;
- depois de `só pra mim` + `ASK_CHILD_UNDER_5` + `uma tem 4`, o estado preserva
  `ChildUnder5AddsTraveler=true` e `ExpectedDocumentCount=2`.

Os quatro cenários também foram reconstruídos a partir do evento persistido no
histórico com `currentTurn="ok"`, sem alteração dos valores no replay.

O teste incremental
`TestPassengerCountChildUnder5SecondReviewP1ServiceOutcomesAndReplay`, incluído
nos filtros `count=20` e `-race`, percorre `Service.Reprocess` para os quatro P1
e confirma estado, `expected_document_count`, template final, zero chamada ao
LLM, zero chamada a `booking_create`, zero tool call e replay do mesmo draft.

#### Terceiro review — 3 P1 e reavaliação arquitetural

| Prioridade | Achado | Impacto |
|---|---|---|
| P1 | conflito cross-turn entre total e crianças não passa por validação após a redução completa | `somos 1` seguido de `ASK_CHILD_UNDER_5` e `duas têm 3 e 4 anos` mantinha total 1 e duas crianças conhecidos, calculava três documentos e avançava um estado impossível. |
| P1 | correção infantil explícita recente perde para evidência antiga | `não tem criança menor de 5` seguido de `na verdade, uma tem 4` preservava o zero antigo apenas porque era `EXACT`, omitindo a criança corrigida. |
| P1 | marcador explícito `outro filho` colapsa na mesma identidade | `meu filho tem 3 anos e meu outro filho tem 4 anos` retornava somente uma criança menor de 5 e podia cobrar indevidamente a segunda. |

Na terceira rodada histórica, os três achados tornavam o patch inseguro para
commit, deploy e smoke. A rodada não continuaria empilhando regras de
precedência no merge então vigente. A decisão
registrada em `docs/adr/ADR-2026-07-passenger-clarification-state.md` separa:

```text
extração por turno
-> redução temporal pura em ordem cronológica
-> validação cruzada do estado completo
-> aplicação do estado validado ao BookingDraftContext
```

`passenger_clarification_evidence.go` ficou restrito à extração de evidências.
`passenger_clarification_reducer.go` produz um único
`PassengerClarificationState`, e `BookingDraftContext` não reparsa texto nem
reabre histórico para calcular `ChildUnder5AddsTraveler` ou documentos.

#### RED/PASS após o terceiro review

```text
RED — TestPassengerReducerThirdReviewServiceOutcomesAndReplay/cross_turn_children_above_total_are_not_actionable: passenger_count=1, child_under_5_count=2 e expected_document_count=3
RED — TestPassengerReducerThirdReviewServiceOutcomesAndReplay/explicit_child_correction_replaces_old_zero: child_under_5_count=0 e expected_document_count=1
RED — TestPassengerReducerThirdReviewServiceOutcomesAndReplay/explicit_other_son_preserves_two_identities: child_under_5_count=1
RED — TestPassengerReducerCrossTurnChildConflict com documentos antigos: expected_document_count=2 no estado conflitante
RED — TestBookingDraftConflictingPassengerCountDoesNotReuseHistoricalBookingTotal: ação ask_booking_payment_preference no estado conflitante
PASS — go test -count=20 ./internal/chat -run 'Test.*Passenger.*Reducer|Test.*Passenger.*Correction|Test.*Child.*Identity|Test.*Child.*Conflict'
PASS — go test -race -count=1 ./internal/chat -run 'Test.*Passenger.*Reducer|Test.*Passenger.*Correction|Test.*Child.*Identity'
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Payment|Test.*LapChild|Test.*Passenger.*Document'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — git diff --check
```

`TestPassengerReducerThirdReviewServiceOutcomesAndReplay` confirma os quatro
resultados user-visible, zero chamada a LLM, `booking_create`, payment ou tools
e replay idempotente. Os testes puros confirmam redução cronológica, fronteira
de correção por slot, identidade explícita e repetição sem duplicação.

#### Quarto review — 3 P1 temporais

| Prioridade | Achado | Impacto |
|---|---|---|
| P1 | um novo `ASK_CHILD_UNDER_5` não abre uma nova época do slot | zero exato antigo continua vencendo `uma tem 4` respondido ao prompt mais recente quando não há `CorrectionCue` lexical. |
| P1 | identidades infantis não participam da redução entre turnos | `meu filho tem 3 anos` seguido de `meu outro filho tem 4 anos` substitui a contagem anterior e termina com somente uma criança. |
| P1 | total absoluto igual a 1 é confundido com declaração solo | `somos 1` pode habilitar `ChildUnder5AddsTraveler` como se fosse `só pra mim`, produzindo dois documentos em vez de composição conflitante. |

Esta rodada corrige somente a perda de informação entre evidência, reducer e
`BookingDraftContext`. É proibido adicionar ou ampliar regex, listas de
expressões, `containsAnyFolded` ou parsing linguístico. A evidência deve
preservar a época do active prompt pela posição no histórico, a proveniência
tipada do total e as referências infantis necessárias para redução temporal.

Precedência por slot:

```text
época mais recente do prompt
-> turno mais recente dentro da época
-> força somente entre evidências da mesma época
```

Somente proveniência `SOLO_SPEAKER` pode habilitar
`ChildUnder5AddsTraveler=true`. `ABSOLUTE_TOTAL` com uma criança adicional deve
produzir estado conflitante, `ExpectedDocumentCount=0`, clarification segura e
zero LLM, tools, `booking_create` ou payment.

RED real antes da correção:

```text
TestPassengerPromptEpochRecentChildAnswerWinsOlderExactZero -> ChildUnder5Count=0 e ExpectedDocumentCount=1
TestChildIdentityReducerAccumulatesDistinctReferencesAcrossTurns -> ChildUnder5Count=1
TestPassengerProvenanceAbsoluteTotalOneWithContextualChildConflicts -> PassengerCount=1, ChildUnder5AddsTraveler=true e ExpectedDocumentCount=2
TestPassengerTemporalFourthReviewServiceOutcomesAndReplay/absolute_total_one_with_child_is_closed_conflict -> passenger_count_known=true e expected_document_count=2
TestChildIdentityReducerScalarEvidenceDoesNotReplaceReferenceSet -> ChildReferences preservava 2 identidades, mas ChildUnder5Count regredia de 2 para 1
```

O controle sem novo prompt e os cenários solo/repetição permaneceram verdes no
mesmo comando focado.

PASS real após a correção:

```text
PASS — go test -count=20 ./internal/chat -run 'Test.*Passenger.*Reducer|Test.*Prompt.*Epoch|Test.*Passenger.*Provenance|Test.*Child.*Identity|Test.*Temporal'
PASS — go test -race -count=1 ./internal/chat -run 'Test.*Passenger.*Reducer|Test.*Prompt.*Epoch|Test.*Passenger.*Provenance|Test.*Child.*Identity|Test.*Temporal'
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Payment|Test.*LapChild|Test.*Passenger.*Document'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — git diff --check
```

Auditoria do patch do quarto review: nenhuma regex, lista de expressões ou uso
de `containsAnyFolded` foi adicionado ou ampliado. A época do prompt, a
proveniência tipada e `ChildReferences` atravessam evidência, reducer e
`BookingDraftContext`; uma contagem escalar posterior na mesma época não apaga
o conjunto de identidades já reduzido. H-2026-07-16B permanece aberta até novo
review sem P1/P2 e 3.6F-D continua **BLOQUEADA por H-2026-07-16B**.

#### Quinto review — 5 P1 e reestruturação final

| Prioridade | Achado | Impacto |
|---|---|---|
| P1 | identidades infantis continuam derivadas de regex e chaves lexicais | variações semanticamente equivalentes ficam ignoradas ou colapsadas; a correção não é estrutural. |
| P1 | `BookingDraftContext` reconstrói o estado reparseando todo o histórico disponível | `Service.Reprocess` carrega somente uma janela limitada; proveniência e referências somem quando mensagens antigas saem do `LIMIT 50`, e mudanças no parser alteram o replay. |
| P1 | `ChildUnder5AddsTraveler` é recalculado usando o prompt ativo da evidência infantil mais recente | uma menção posterior da mesma criança, já fora do prompt infantil original, pode reduzir `expected_document_count` de dois para um. |
| P1 | um prompt novo só abre época quando o mesmo turno também contém evidência infantil reconhecida | prompt sem resposta reconhecida mantém zero antigo como conhecido e permite avanço com slot stale. |
| P1 | correção para zero não limpa o agregado infantil anterior | referências antigas elevam novamente a contagem após `CHILD_COUNT_SET=0`, preservando documento ou lap child incorreto. |

Na quinta rodada histórica, os cinco achados invalidavam a reconstrução textual
como fonte canônica e tornavam o patch inseguro para commit, deploy ou smoke. A
decisão final daquela rodada está em
`docs/adr/ADR-2026-07-passenger-state-durable-events.md`; a ADR anterior fica
parcialmente superseded. O H-B passa a persistir um
`PassengerClarificationStateV1` versionado em metadata já salva atomicamente
pelo `Reprocess`, com eventos estruturais, slots por época, proveniência e
ledger de mensagens aplicadas.

Restrições desta reestruturação:

- remover regex e chaves lexicais de identidade infantil adicionadas pelo H-B;
- não adicionar regex, `containsAnyFolded`, sinônimos ou listas de frases;
- reducer recebe somente estado e eventos estruturais, nunca texto;
- `BookingDraftContext` recebe o estado pronto e não percorre histórico para
  extrair passageiros;
- linguagem familiar aberta sem contrato estruturado bloqueia avanço com
  clarification segura;
- sessões sem V1 fazem no máximo um bootstrap conservador, marcado no snapshot;
- `PassengerClarificationMeaningV1` permanece **FUTURO CONDICIONAL**, conforme
  `plans/p-2026-07-passenger-clarification-meaning-v1.md`.

O 3.6F-D continua **BLOQUEADA por H-2026-07-16B**. Não há autorização para
commit, push, deploy ou smoke, e esta rodada não deve declarar review limpo.

#### Implementação local após o quinto review

- criado `PassengerClarificationStateV1` versionado em
  `metadata.memory.passenger_clarification_state_v1`, com slots, proveniência,
  origem imutável de `ChildUnder5AddsTraveler`, reason codes e ledger
  idempotente;
- reducer limitado a `PASSENGER_PROMPT_OPENED`, `CHILD_PROMPT_OPENED`,
  `PASSENGER_COUNT_SET`, `CHILD_COUNT_SET`, `SLOT_CORRECTED` e
  `SLOT_INVALIDATED`, sem texto como entrada;
- prompt estrutural é anexado ao draft, mas só outbound confiável/enviado abre
  a época no `Reprocess` seguinte; draft bloqueado não reseta estado;
- estado do turno é persistido por `SaveReprocessSnapshot` na mesma transação
  que marca as mensagens processadas;
- `BookingDraftContext` recebe o estado pronto; sua varredura histórica continua
  apenas para disponibilidade, documentos e tools;
- correção infantil para zero substitui o slot e limpa referências,
  `ChildUnder5AddsTraveler` e sua origem;
- linguagem familiar aberta sem representação estrutural produz
  `UNSUPPORTED_FAMILY_IDENTITY` e clarification segura, sem LLM,
  `booking_create`, payment ou tools;
- bootstrap legado é único e conservador; replay/restart usa o snapshot V1 e
  não recompõe o agregado a partir da janela textual.

O resultado local não conclui H-B: novo review ainda não foi executado e não há
declaração de review limpo. O 3.6F-D permanece bloqueado.

#### Arquivos desta reestruturação final

```text
apps/api/internal/chat/availability_draft.go
apps/api/internal/chat/booking_create_router.go
apps/api/internal/chat/booking_create_router_test.go
apps/api/internal/chat/booking_draft_context.go
apps/api/internal/chat/booking_draft_context_test.go
apps/api/internal/chat/conversation_state_machine.go
apps/api/internal/chat/handler_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/interpreter.go
apps/api/internal/chat/interpreter_test.go
apps/api/internal/chat/interpreter_validation.go
apps/api/internal/chat/interpreter_validation_test.go
apps/api/internal/chat/model.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/service.go
apps/api/internal/chat/tool_router.go
apps/api/internal/chat/tool_router_test.go
apps/api/internal/chat/passenger_clarification_evidence.go
apps/api/internal/chat/passenger_clarification_evidence_test.go
apps/api/internal/chat/passenger_clarification_test_helper_test.go
apps/api/internal/chat/passenger_clarification_reducer.go
apps/api/internal/chat/passenger_clarification_reducer_test.go
apps/api/internal/chat/passenger_clarification_state_v1_test.go
docs/adr/ADR-2026-07-passenger-clarification-state.md
docs/adr/ADR-2026-07-passenger-state-durable-events.md
docs/EXECUTION_TRACKER.md
plans/h-2026-07-16b-passenger-child-state.md
plans/p-2026-07-passenger-clarification-meaning-v1.md
```

#### Validação final desta reestruturação

```text
PASS — testes focados de estado/eventos, cinco P1, truncamento, replay, restart e linguagem aberta
PASS — go test -count=20 ./internal/chat -run 'Test.*Passenger.*Count|Test.*Child.*Under.*5|Test.*Family|Test.*Booking.*Draft|Test.*Correction|Test.*Conflict|TestPassengerClarificationStateV1'
PASS — go test -race -count=1 ./internal/chat -run 'Test.*Passenger.*Count|Test.*Child.*Under.*5|Test.*Booking.*Draft|Test.*Correction|Test.*Conflict|TestPassengerClarificationStateV1'
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Payment|Test.*LapChild|Test.*Passenger.*Document|Test.*Document.*Passenger|Test.*Booking.*Document'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — git diff --check
```

Resultado do review: o quinto review é a entrada desta correção; nenhum novo
review foi executado depois do patch. Teste em produção/smoke: não executado nem
autorizado. Próxima ação recomendada: executar novo review local do diff; manter
H-B aberta e 3.6F-D bloqueada até um resultado sem P1/P2 e autorização posterior
específica para qualquer operação externa.

#### Histórico superseded das quatro rodadas anteriores

As seções abaixo permanecem somente como registro das tentativas anteriores à
arquitetura de snapshot/eventos duráveis e não descrevem mais o call path
canônico.

#### Gate de fechamento

```text
contagem relativa correta
sem duplicação de viajante
idade/afirmação consumida no prompt correto
expected_document_count consistente
sem loop
cobrança somente de passageiros pagantes
H-012 verde
```

#### Causa raiz comprovada

O parser aceitava a forma relativa `eu|pra mim|para mim e mais N pessoas`,
mas não possuía uma forma semântica para grupos familiares quantificados.
Por isso, `eu e meus 2 filhos` caía no atalho genérico `eu e meu`, que fixava
`passenger_count=2`; `meus 2 filhos` sem interlocutor e `somos 3` também não
tinham reconhecimento próprio.

Para criança, o padrão de idade exigia um substantivo imediato como
`filho|filha|criança` e não consumia referências como `o mais novo` nem a
composição `uma tem 4 e outra 6`. Além disso, o `sim` contextual existia apenas
na reconstrução de `BookingDraftContext`: `InterpretStructuredTurn`, validator
e intent router consultavam o parser puro, retornavam estado desconhecido e
permitiam a repetição de `ASK_CHILD_UNDER_5`.

#### Caminho determinístico reconciliado

```text
InferActivePromptContext
  -> ActivePromptLapChildQuestion para ASK_CHILD_UNDER_5
  -> InterpretStructuredTurn / ValidateStructuredInterpretation
  -> routeActivePromptAnswer
  -> Service.Reprocess
  -> extractPassengerClarificationTurnEvidence(activePrompt)
  -> collectBookingDraftContextWithCurrentEvidence
  -> passengerClarificationEvidenceTimeline (histórico antigo -> novo -> atual)
  -> ReducePassengerClarificationEvidence
  -> applyPassengerClarificationStateToBookingDraft
  -> expectedPassengerDocumentCount
  -> decideNextBookingStep
  -> bookingContinuationTemplateName
```

A normalização agora distingue total absoluto de composição relativa:

- `eu|pra mim|para mim + meus/minhas N filhos/filhas` soma o interlocutor uma única vez;
- `meus/minhas N filhos/filhas` preserva somente o grupo informado;
- `somos N` representa total, sem nova soma;
- idades com referente explícito preservam unidade; pares sem unidade como `uma tem 4 e outra 6` só são consumidos em contexto infantil inequívoco;
- marcadores de incerteza invalidam somente o slot que qualificam;
- `sim` só confirma ao menos uma criança quando o active prompt é o de criança.

O reducer atribui valores absolutos, sem incrementos. Correção explícita recente
abre uma nova fronteira para o slot corrigido; sem correção, referência singular
continua sem reduzir contagem exata. A validação cruzada só ocorre depois da
redução completa. Reprocessar o mesmo histórico não aumenta `PassengerCount`
nem `ChildUnder5Count`.

#### Invariantes preservados

```text
"eu e meus 2 filhos" -> passenger_count=3
"para mim e meu filho de 4 anos" -> passenger_count=2; child_under_5_count=1
"meus 2 filhos vão viajar" -> passenger_count=2
"somos 3" -> passenger_count=3
"sim, o mais novo tem 4 anos" em ASK_CHILD_UNDER_5 -> child_under_5_count=1; known=true
"uma tem 4 e outra 6" em contexto infantil inequívoco -> child_under_5_count=1
"duas têm 3 e 4 anos" + "a mais nova tem 3" -> child_under_5_count=2
"meu filho de 10 meses" -> child_under_5_count=1
"não tem criança menor de 5" -> child_under_5_count=0; known=true
expected_document_count = total real de viajantes
criança não pagante continua exigindo documento
atribuição de qual passageiro é a criança continua posterior quando necessária
cobrança continua excluindo passageiros com IsLapChild
```

O fluxo combinado de 3 viajantes e 1 criança avança para
`ASK_PASSENGER_DOCUMENTS`, solicita 3 documentos e não repete
`ASK_CHILD_UNDER_5`. Nenhum código de payment foi alterado; a regra existente
`countChargeableBookingPassengers` continua ignorando `IsLapChild`.

#### Arquivos alterados no H-2026-07-16B

- `apps/api/internal/chat/booking_create_router.go`
- `apps/api/internal/chat/booking_create_router_test.go`
- `apps/api/internal/chat/passenger_clarification_evidence.go`
- `apps/api/internal/chat/passenger_clarification_evidence_test.go`
- `apps/api/internal/chat/passenger_clarification_reducer.go`
- `apps/api/internal/chat/passenger_clarification_reducer_test.go`
- `apps/api/internal/chat/booking_draft_context.go`
- `apps/api/internal/chat/booking_draft_context_test.go`
- `apps/api/internal/chat/interpreter.go`
- `apps/api/internal/chat/interpreter_test.go`
- `apps/api/internal/chat/interpreter_validation.go`
- `apps/api/internal/chat/interpreter_validation_test.go`
- `apps/api/internal/chat/intent_router.go`
- `apps/api/internal/chat/intent_router_test.go`
- `apps/api/internal/chat/service.go`
- `apps/api/internal/chat/incremental_flow_test.go`
- `docs/adr/ADR-2026-07-passenger-clarification-state.md`
- `docs/EXECUTION_TRACKER.md`
- `plans/h-2026-07-16b-passenger-child-state.md`

#### Validação local

```text
RED comprovado antes da correção — total/subgrupo retornou 2 em vez de 3; incerteza infantil apagou total 3; 10 meses retornou child_under_5_count=0; referência singular reduziu contagem exata 2 para 1
PASS — go test -count=20 ./internal/chat -run 'Test.*Passenger.*Count|Test.*Child.*Under.*5|Test.*Family|Test.*Booking.*Draft'
PASS — go test -race -count=1 ./internal/chat -run 'Test.*Passenger.*Count|Test.*Child.*Under.*5|Test.*Booking.*Draft'
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Payment|Test.*LapChild'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — git diff --check
```

Os resultados acima pertencem à correção do primeiro review. O segundo review
comprovou quatro P1 adicionais, portanto eles não demonstram invariantes
completos, patch seguro ou review limpo.

#### RED/PASS após o segundo review

```text
RED — TestPassengerCorrectionLatestExactTotalClearsOlderConflict: PassengerCount=0, known=false, conflicting=true
RED — TestPassengerChildConflictRejectsChildrenAboveKnownTripTotal: PassengerCount=1 e ChildUnder5Count=2 conhecidos
RED — TestPassengerChildUnder5DistinctNamedChildrenAreCountedSeparately: ChildUnder5Count=1
RED — TestPassengerChildUnder5ActivePromptEvidenceAddsStandaloneTravelerDocument: ChildUnder5AddsTraveler=false e ExpectedDocumentCount=1
PASS — go test -count=20 ./internal/chat -run 'Test.*Passenger.*Count|Test.*Child.*Under.*5|Test.*Family|Test.*Booking.*Draft|Test.*Correction|Test.*Conflict'
PASS — go test -race -count=1 ./internal/chat -run 'Test.*Passenger.*Count|Test.*Child.*Under.*5|Test.*Booking.*Draft'
PASS — go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Payment|Test.*LapChild'
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — git diff --check
```

Riscos restantes: a reestruturação durável motivada pelos cinco P1 está em
execução; linguagem familiar aberta permanece bloqueada e pertence ao plano
**FUTURO CONDICIONAL** de `PassengerClarificationMeaningV1`; não houve smoke nem
teste em produção. Esses riscos mantêm H-2026-07-16B aberta e 3.6F-D bloqueada.

#### Gate operacional

```text
commit/push/deploy: não executados
smoke/teste em produção: não executado nem autorizado neste ciclo; não classificado como pendente antes de novo review sem P1/P2
3.6F-D: permanece bloqueada por H-2026-07-16B
próxima ação histórica daquela rodada: concluir estado/eventos duráveis, executar a matriz local e então solicitar novo /review sem executar commit, push, deploy ou smoke
```

### 8.11 Hotfix user-visible — H-2026-07-22A (2026-07-22)

**Status atual:** **CORRIGIDO E DEPLOYADO — SMOKE OPERACIONAL RED; gate
reaberto por H-2026-07-27A**.

#### Evidência operacional e causa

Em sessão nova após limpeza, tanto `"oi"` quanto
`"monção para videira na data mais próxima"` caíam diretamente em
`ASK_PASSENGER_COUNT`; `tool_call_count=0`, `availability_search` não era
executada e `history_count=1`.

A causa foi comprovada no gate global de `Service.Reprocess`:

1. `newPassengerClarificationStateV1` cria um bootstrap durável com ambos os
   slots desconhecidos e `HasEvidence=false`;
2. `passengerClarificationStateUnsafeV1` tratava qualquer slot desconhecido
   como inseguro;
3. `externalWorkUnsafe` incorporava esse resultado antes de greeting, seleção
   de viagem e disponibilidade;
4. o fail-closed caía em `BookingNextAskPassengerClarification`;
5. testes amplos com `newFakeStoreWithPassengerAuthority` mascaravam o estado
   literal de uma sessão criada por `newFakeStore()`.

#### RED e correção local

O RED foi iniciado com `newFakeStore()` e confirmou:

```text
RED — sessão nova + "oi" retornou pergunta de passageiros
RED — sessão nova + "quero uma passagem" retornou pergunta de passageiros
RED — rota + EARLIEST_AVAILABLE executou availability_search 0 vezes
RED — seleção não chegou à materialização e tool_call_count permaneceu 0
RED — bootstrap UNKNOWN fresco foi classificado como unsafe=true
```

O patch separa três conceitos:

```text
invalid_or_conflicting = validate(state) falha OR conflito estrutural
active_passenger_context = state.HasEvidence
unknown_slots = !PassengerCountKnown OR !ChildUnder5CountKnown

unsafe = invalid_or_conflicting OR
         (active_passenger_context AND unknown_slots)
```

Assim, `UNKNOWN` fresco não bloqueia greeting, coleta de rota ou disponibilidade.
Estado inválido/conflitante continua sempre fail-closed. Depois de um prompt de
passageiros realmente entregue, slots desconhecidos ou conflitantes continuam
bloqueando runner, JSON runner, shadows V1/V2 e tools. Humano e cancelamento
`STRONG` mantêm precedência local. Não houve parser, regex, provider nem código
de B2.

#### Correção local dos 2 P1 do review anterior

O review seguinte encontrou dois P1 remanescentes:

1. um snapshot persistido com `HasEvidence=false`, slot `OPEN` e
   `PassengerPromptMessageID` real ainda era aceito pelo validator, classificava
   o gate como seguro e liberava o shadow V1;
2. uma seleção em sessão nova podia chegar a `ASK_PASSENGER_COUNT` e anexar
   `pending_prompt_event` mesmo quando a opção visível não possuía `trip_id`,
   `board_stop_id` ou `alight_stop_id` suficiente para materialização.

Os REDs reproduziram `shadow=1` no primeiro caso e `ASK_PASSENGER_COUNT` nas
seis combinações do segundo (`3` IDs ausentes x `"1"`/`"essa msm"`). A correção:

- faz `validatePassengerClarificationStateV1` rejeitar `HasEvidence=false`
  quando existe evidência estrutural de passageiros, incluindo IDs de prompt e
  de última mensagem, IDs aplicados, slots não `PENDING` e contagens conhecidas;
- mantém o `UNKNOWN` literal, sem qualquer campo estrutural, válido;
- transforma snapshot persistido contraditório em
  `invalidPassengerClarificationStateV1`, mantendo `externalWorkUnsafe=true`;
- exige `selectedAvailabilityItemForMaterialization` antes de permitir a
  transição de seleção para passageiros;
- invalida os fatos incompletos na fronteira causal e impede reconstrução
  metadata-only de `SelectedOptionIndex` enquanto o marcador canônico estiver
  ativo.

Com isso, opção incompleta retorna fallback determinístico de seleção sem abrir
época de passageiros, sem índice/snapshot/IDs de rota e sem trabalho externo. A
opção completa continua materializando índice, snapshot e rota atomicamente
antes de produzir `ASK_PASSENGER_COUNT` e o evento pendente normal.

#### Correção local dos 3 P1 do review anterior

O review seguinte preservou as duas correções acima e encontrou três caminhos
remanescentes:

1. uma resposta numérica ou contextual diante de prompt de availability podia
   cair no fallback do router e escapar da exigência de materialização;
2. com `passengerUnsafe=true`, uma seleção não materializável perdia a
   precedência para `ASK_PASSENGER_COUNT` e abria novo evento de prompt;
3. `passengerClarificationPromptEventForRunV1` inferia prompt apenas por slots
   desconhecidos em `SAFE_PHASE_FALLBACK`, fazendo cancelamento entregue trocar
   indevidamente o epoch de passageiros.

Os REDs reais confirmaram marker/boundary ausentes para prompt sem tool facts e
índice fora do range, reaparecimento posterior do número bruto como
`SelectedOptionIndex=1`, `ASK_PASSENGER_COUNT` com `pending_prompt_event` para
opção incompleta em contexto de passageiros e evento espúrio no cancelamento.
O allowlist também começou RED para os templates contextuais positivos de
passageiro e criança.

A correção:

- cria uma decisão estrutural de tentativa de seleção usando prompt ativo,
  contexto visível, marker durável e resposta atual, sem depender apenas do
  `IntentSelectAvailabilityOption` final;
- exige materialização para tentativa contextual/numérica no contexto de
  availability; a falha cria ou preserva marker + boundary, mantém
  `externalWorkUnsafe=true` e não anexa índice, snapshot ou facts;
- não abre o gate para número fora de contexto; com marker já existente, uma
  repetição permanece fail-closed sem mover o boundary nem executar novamente o
  router;
- impede `findLatestSelectedOptionIndex` de varrer o histórico enquanto o
  marker canônico estiver ativo;
- coloca o fallback de availability imediatamente depois dos guardrails
  `STRONG`, inclusive quando `passengerUnsafe=true`;
- substitui a inferência por estado/body por allowlist de template + action.
  `SAFE_PHASE_FALLBACK`, cancelamento, humano, info paralela e demais
  guardrails não criam prompt event; perguntas reais de passageiro/criança
  continuam criando evento somente após delivery.

As duas correções anteriores permanecem cobertas: snapshot contraditório com
`HasEvidence=false` continua inválido/fail-closed, e `UNKNOWN` literal fresco
continua permissivo antes de contexto de passageiros.

#### Correção local dos 3 P1 do review anterior mais recente

O review anterior mais recente preservou os cinco P1 anteriores como corrigidos
e encontrou três lacunas novas na mesma fronteira:

1. confirmação já reconhecida por `looksLikeBookingCreateConfirmation`, como
   `"ok"`, não entrava no gate quando o prompt de availability tinha uma única
   opção; sem facts completos, o turno escapava sem marker e liberava shadows;
2. deíxis ambígua em lista completa com várias opções, como `"essa msm"`, e
   índice fora do range eram tratados como falha destrutiva, ocultando uma lista
   válida e impedindo a resposta numérica posterior;
3. `resolveBookingCreateSelection` ainda consultava
   `findLatestSelectedOptionIndex`, permitindo combinar um `"1"` falho antigo
   com uma disponibilidade nova e autorizar `booking_create` sem seleção
   bookable persistida.

Os REDs confirmaram `SAFE_PHASE_FALLBACK` sem marker para `"ok"` diante de
opção incompleta, marker indevido para deíxis/índice fora do range e
`BookingCreateInput` válido ao combinar o índice bruto antigo com a opção nova.

A correção centraliza a classificação estrutural do turno em quatro estados,
sem regex nova:

```text
NONE
MATERIALIZE
CLARIFY_PRESERVE
FAIL_CLOSED_INVALIDATE
```

- `MATERIALIZE` resolve índice único/expresso apenas contra o prompt atual e
  exige `trip_id`, `board_stop_id` e `alight_stop_id`; confirmação de opção
  única usa índice `1`, persiste snapshot completo e só então pergunta
  passageiros;
- `CLARIFY_PRESERVE` cobre deíxis ambígua e índice fora do range diante de lista
  completa, bloqueia runner/JSON/shadows/claim/tools somente no turno, não cria
  marker e reanexa os facts ao fallback para a lista continuar selecionável;
- `FAIL_CLOSED_INVALIDATE` cobre confirmação/índice/deíxis sem facts completos,
  persiste marker + boundary e mantém zero trabalho externo;
- `"ok"` fora de prompt de availability permanece `NONE`;
- booking draft e `booking_create` aceitam somente índice explícito do turno
  resolvido contra facts completos atuais ou
  `latestAvailabilitySelectionEvidence` bookable com snapshot completo;
- o fallback de número bruto foi removido de `resolveBookingCreateSelection`, o
  fallback automático de opção única sem seleção persistida foi removido e os
  call sites de produção de `findLatestSelectedOptionIndex` foram zerados;
- o overlay do marker/boundary agora também é aplicado nas duas projeções de
  autoridade, impedindo que fatos pré-boundary reapareçam fora de
  `Service.Reprocess`.

Com isso, `"essa msm"` preserva a lista e a resposta seguinte `"1"` materializa
a opção correta; índice fora do range não contamina o turno seguinte; `"1"`
falho + disponibilidade nova + `"quero reservar"` produz zero autorização de
`booking_create`; seleção bookable persistida continua no fluxo normal. Os
cinco P1 anteriores permanecem cobertos e verdes.

#### Correção local dos 3 P1 do novo review

O novo review preservou os oito P1 anteriores como corrigidos e encontrou três
lacunas adicionais:

1. `promptContext.OptionCount > 0` confundia facts de continuidade com
   identidade de prompt. Depois de selecionar a opção `2`, o
   `ASK_PASSENGER_COUNT` ainda carregava o `tool_context` da lista; a resposta
   de passageiros `"1"` podia rematerializar a opção `1`, e `"sim"` podia virar
   fallback de availability;
2. a classificação de seleção ocorria antes de preservar uma decisão
   humano/cancelamento `STRONG`. Turnos mistos com índice ou deíxis podiam ser
   reescritos como seleção ou clarificação;
3. uma única opção completa exibida podia preencher os campos de viagem do
   booking draft sem provar seleção. Com índice bruto histórico, passageiros e
   documentos completos, a confirmação `"sim"` podia chegar a
   `booking_create` sem autoridade bookable persistida.

Os REDs reais confirmaram troca da opção `2` pela `1`, fallback de availability
para `"sim"`, perda dos quatro guardrails `STRONG` exigidos e
`BookingCreateInput` com `SelectedOptionIndex=0`; a regressão integrada chamou
`booking_create` uma vez antes do patch.

A correção:

- separa identidade do prompt de facts de continuidade. A seleção só é
  classificada quando `ActivePrompt.Kind` é
  `AVAILABILITY_OPTION_CHOICE`, a fonte é o último outbound confiável que
  realmente pergunta qual opção e ID/body correspondem à fonte do active
  prompt. `ASK_PASSENGER_COUNT` pode transportar facts anteriores sem adquirir
  identidade de prompt de availability;
- detecta humano/cancelamento `STRONG` antes de
  `classifyAvailabilitySelectionTurn` e executa o guardrail local antes de
  qualquer materialização, clarificação ou invalidação;
- adiciona `BookingDraftContext.HasBookableSelection`. Apenas seleção explícita
  atual materializada contra facts completos ou a seleção bookable persistida
  ativa a flag. `mergeAvailabilityPayloadIntoBookingDraft` pode enriquecer os
  campos, mas opção única, trip IDs e `tool_context` não concedem autoridade;
- exige `HasBookableSelection` em todos os entrypoints de `booking_create`,
  inclusive confirmação de documentos. O controle positivo com seleção
  bookable persistida continua produzindo a reserva normalmente.

Não houve regex/parser novo, implementação de B2, commit, push, deploy ou
smoke. Os oito P1 anteriores permanecem cobertos e verdes.

#### Correção local do P1 do review anterior

O review anterior preservou como corrigidos os três P1 da rodada precedente e
encontrou uma projeção prematura restante: `deriveCanonicalConversationState`
chamava `BookingDraftContext` com o `currentTurn` antes do roteamento. Em turnos
mistos como `"opção 1, quero cancelar"` e
`"primeira opção, quero falar com atendente"`, o intent `STRONG` vencia a
resposta, mas índice e `trip/board/alight` da opção mencionada já podiam ser
gravados no estado canônico.

Os REDs reais confirmaram:

```text
RED — "opção 1, quero cancelar" chegou ao router com SelectedOptionIndex=1 e trip/board/alight da opção 1
RED — "primeira opção, quero falar com atendente" chegou ao router com SelectedOptionIndex=1 e trip/board/alight da opção 1
RED complementar — com marker + boundary existentes, "opção 1, quero cancelar" perdeu o cancelamento STRONG para CONTEXT_FALLBACK_AVAILABILITY_OPTION
RED de escopo intermediário — antecipar todo humano/cancelamento desviou cancelamentos reais, impediu limpeza de pending_question e suprimiu shadows esperados; a precedência foi então restringida ao prompt de opções ou ao turno realmente misto
```

A correção estrutural:

- introduz `bookingDraftRoutingBaselineProjection`,
  `collectBookingDraftContextForRoutingBaseline` e
  `deriveCanonicalConversationStateForRoutingBaseline`; o baseline usa sessão e
  histórico persistido, preserva seleção bookable anterior e não materializa
  seleção do turno atual;
- roteia `currentTurn` contra esse baseline e só chama
  `classifyAvailabilitySelectionTurn` quando não há humano/cancelamento `STRONG`
  com precedência sobre seleção;
- usa e persiste o baseline no guardrail `STRONG`, sem índice/snapshot/evento de
  passageiros e sem criar, remover ou mover marker/boundary;
- mantém `NONE`, `CLARIFY_PRESERVE` e `FAIL_CLOSED_INVALIDATE` inalterados;
  somente `MATERIALIZE` chama
  `applyMaterializedAvailabilitySelectionToCanonicalState` e substitui a rota
  por uma opção atual completa;
- preserva atomicamente uma seleção bookable anterior, inclusive a opção `2`,
  quando o turno misto menciona a opção `1`;
- mantém cancelamento real, handoff, pending question, shadows e tools fora de
  selection context no fluxo anterior.

Não houve alteração de parser, regex, provider, contratos OpenAI, B2,
booking/payment, commit, push, deploy ou smoke.

#### Correção local do P1 do review anterior

O review mais recente preservou a correção acima e encontrou uma segunda
projeção prematura dentro do próprio baseline. Mesmo sem projetar
`currentTurn`, `mergeAvailabilityPayloadIntoBookingDraft` ainda usava o único
resultado completo como fallback e copiava `trip_id`, `board_stop_id`,
`alight_stop_id`, data, horário, preço e moeda enquanto
`HasBookableSelection=false`. Assim, turnos mistos `STRONG` recebiam uma rota
que parecia selecionada por item, embora não houvesse seleção persistida.

O RED real confirmou a promoção nos quatro turnos exigidos:
`"opção 1, quero cancelar"`, `"essa msm, quero falar com atendente"`,
`"ok, quero cancelar"` e `"1, quero falar com uma pessoa"`. O mesmo item
reaparecia no router e no interpreter do turno seguinte. O teste unitário do
booking draft registrou `SelectedOptionIndex=0` e
`HasBookableSelection=false`, mas ainda encontrou todos os IDs e fatos do único
resultado.

A correção:

- torna a política de disponibilidade explícita em `BookingDraftContext`:
  `ENVELOPE_ONLY` para o baseline de routing e `BOOKABLE_SELECTION` para a
  projeção autorizada;
- divide o merge em envelope/filtro e item selecionado; o fallback
  `len(results)==1` foi removido, portanto resultado único não concede fatos de
  item;
- em `ENVELOPE_ONLY`, mantém `HasAvailabilityShown` e origem/destino do filtro,
  mas não deriva do item índice, trip/stops, data, horário, preço, moeda ou
  pacote;
- permite que `ENVELOPE_ONLY` recupere o item apenas quando
  `latestAvailabilitySelectionEvidence` já é bookable, preservando exatamente
  a opção persistida — inclusive opção `2` diante de uma nova lista unitária
  não selecionada;
- mantém `MATERIALIZE` como único caminho do serviço que aplica uma opção atual
  completa ao estado canônico, persiste índice/snapshot/trip/stops e então
  produz `ASK_PASSENGER_COUNT`.

As expectativas legadas que tratavam uma opção única apenas exibida como rota
selecionada passaram a validar somente o envelope. A fixture de quantidade de
passageiros que pressupunha viagem escolhida recebeu o snapshot bookable
persistido correspondente; nenhum contrato de booking/payment foi alterado.

Não houve alteração de parser, regex, provider, contratos OpenAI, B2,
booking/payment, commit, push, deploy ou smoke.

#### Correção local dos 2 P1 do review anterior

O review anterior preservou os P1 anteriores como corrigidos e encontrou
duas violações restantes da mesma raiz: a seleção bookable ainda não era tratada
como agregado atômico.

1. uma rejeição posterior específica da opção `1` fazia
   `latestAvailabilitySelectionEvidence` ocultar a seleção bookable anterior da
   opção `2`, embora essa opção não tivesse sido rejeitada. O booking draft podia
   então combinar `HasBookableSelection=false` com `trip/board/alight` recuperados
   do snapshot;
2. o pacote não integrava `availabilitySelectionEvidence` nem
   `BookingDraftContext`. O replay genérico podia combinar índice/IDs/data/preço
   da opção `2` e pacote `B` de um envelope posterior não selecionado.

Os REDs reproduziram exatamente `authority=false` para a opção `2` não rejeitada
e o baseline `STRONG` com rota da opção `2`/pacote `B`.

A correção:

- percorre o histórico do mais novo para o mais antigo, acumula rejeições
  posteriores, encerra em rejeição de todo o contexto e faz rejeição específica
  bloquear somente índice/data correspondente;
- preserva os blockers `incomplete` e `metadata-only`; uma seleção bookable
  anterior só sobrevive quando não foi bloqueada pelas rejeições acumuladas;
- usa a autoridade resolvida para aplicar de uma vez
  `HasBookableSelection`, índice, `trip/board/alight`, endpoints, pacote, data,
  horário, preço e moeda. Snapshot selecionado não pode mais preencher IDs
  quando a autoridade é falsa;
- inclui `PackageName` na evidência, no item, no booking draft, no snapshot
  persistido e na projeção canônica;
- quando existe seleção bookable, o canonical replay descarta availability não
  selecionada e publica em `LastToolFacts` somente o agregado selecionado. Assim,
  pacote de envelope posterior não aparece em state, memory ou metadata;
- snapshot legado sem `package_name` recupera pacote somente do item cujo
  `trip_id + board_stop_id + alight_stop_id` corresponde à seleção. Filtro
  incompatível e o campo de pacote do envelope não fornecem esse fallback;
- `MATERIALIZE` de uma opção válida posterior substitui todo o agregado e a
  reconstrução após reload preserva autoridade, IDs e pacote coerentes.

Não houve parser, regex, provider, código B2, alteração de booking/payment,
commit, push, deploy ou smoke.

#### Correção local dos 3 P1 do review atual

O review atual preservou as correções anteriores, mas demonstrou que o scan
reverso ainda não modelava identidade nem supersessão:

1. depois de `S1/A → S2/B`, uma rejeição de `S2` pulava a autoridade corrente e
   ressuscitava `S1`, embora `MATERIALIZE` de `S2` já tivesse substituído o
   agregado anterior;
2. rejeição específica por índice/data não carregava a identidade do prompt.
   Assim, rejeitar a opção `1` da lista `B` também removia uma seleção da opção
   `1` feita na lista `A`;
3. snapshot legado incompleto podia recuperar pacote e demais lacunas de um
   envelope posterior não selecionado com o mesmo índice e os mesmos
   `trip/board/alight`, criando novamente um agregado híbrido.

Os REDs reproduziram a ressurreição de `S1`, a colisão de índice/data entre
listas e a recuperação indevida de `package-b` nos casos com fonte própria,
fonte exata e fonte ausente.

A correção:

- substitui a busca reversa por reducer cronológico. Cada `MATERIALIZE`
  bookable substitui integralmente a autoridade anterior; rejeitar a autoridade
  corrente produz `NONE`, sem procurar seleção superseded. Rejeitar `S1` depois
  de `S2` não altera `S2`, e blockers `incomplete`/`metadata-only` preservam o
  fail-closed. Snapshot apenas propagado em resposta posterior preserva o
  `SelectionMessageID` original e não funciona como novo `MATERIALIZE`;
- mantém `SelectionMessageID` e `AvailabilityPromptSourceMessageID` na
  evidência. O snapshot persiste `selection_message_id` e
  `availability_prompt_source_message_id`; o ID da mensagem de seleção é
  anexado antes de salvar o draft;
- lê a fonte da rejeição em `active_prompt_source_message_id`, já emitido pelo
  caminho out-of-turn. Lembretes reutilizam o ID estrutural da lista original,
  não o ID do envelope de resposta;
- aplica rejeição específica ou total somente à autoridade com o mesmo
  `AvailabilityPromptSourceMessageID`. Colisão de índice ou data em outra lista
  não remove a seleção ativa;
- limita recuperação legada ao próprio evento de seleção ou à mensagem exata
  indicada por `AvailabilityPromptSourceMessageID`, sempre com
  `trip/board/alight` compatíveis. Envelope posterior nunca preenche lacunas; sem
  fonte exata, o campo permanece vazio;
- mantém `booking_create` fechado quando a rejeição remove a autoridade
  corrente. `MATERIALIZE` posterior continua substituindo o agregado inteiro e
  o reload preserva autoridade, IDs e pacote.

Não houve parser, regex, provider, código B2, mudança de contrato público,
commit, push, deploy ou smoke.

#### Correção local dos 3 P1 de autoridade durável

O review seguinte preservou as correções anteriores e demonstrou que a
autoridade normal ainda dependia de uma janela limitada de histórico:

1. `booking_create` podia resolver como fonte o outbound unitário de seleção que
   já perguntava passageiros, em vez do prompt source original persistido;
2. uma projeção propagada podia se promover a nova autoridade quando o
   `MATERIALIZE` e o tombstone saíam da janela;
3. rejeição simples não persistia `active_prompt_source_message_id`; depois do
   truncamento, ela deixava de atingir a seleção correspondente.

A raiz foi removida com `AvailabilitySelectionStateV1`, versionado em
`metadata.memory`, com status `NONE | BOOKABLE | REJECTED | INVALIDATED`,
`selection_event_message_id`, `selection_projection_message_id`,
`availability_prompt_source_message_id`, índice, snapshot completo,
tombstone/rejeições e `applied_event_ids`.

Os eventos tipados `SELECTION_MATERIALIZED`, `SELECTION_REJECTED` e
`SELECTION_INVALIDATED` são normalizados e reduzidos sem texto na mesma
transação que já serializa `PassengerClarificationStateV1` pelo lock da sessão.
`MATERIALIZED` substitui o agregado inteiro; rejeitar a autoridade corrente
limpa o agregado e persiste tombstone; rejeição de outro prompt fica registrada
sem alterar a seleção atual; projeções/copied snapshots usam
`materializes_authority=false`.

Ao reconhecer rejeição no turno atual, o serviço resolve o prompt source antes
do replay e persiste o evento no `normalized_payload` do inbound dentro da
mesma transação. `SaveReprocessSnapshot` preserva o estado bloqueado pelo lock e
não o sobrescreve com memory stale.

`BookingDraftContext` e todos os entrypoints de `booking_create` consultam
primeiro o estado durável. `booking_create` não usa
`latestVisibleAvailabilitySelectionContextWithSource` como autoridade e produz
zero input sem `BOOKABLE`. A materialização explícita do turno só progride
depois de confirmar que seu evento consta no estado retornado pela mesma seção
serializada.

O bootstrap legado consulta uma única vez todas as mensagens estruturadas da
sessão, sem selecionar `body` e sem `LIMIT 50`, reduz o que tem prova exata e
persiste imediatamente o resultado. Projeção falsa nunca cria estado; sem
prova exata, o resultado é `NONE`/fail-closed. Recuperação de lacunas permanece
limitada ao evento/projeção selecionada ou ao prompt source exato.

Produção desta rodada:

```text
apps/api/internal/chat/availability_selection_state_v1.go
apps/api/internal/chat/model.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/service.go
apps/api/internal/chat/booking_draft_context.go
apps/api/internal/chat/booking_create_router.go
```

Testes desta rodada:

```text
apps/api/internal/chat/availability_selection_state_v1_test.go
apps/api/internal/chat/availability_selection_repository_test.go
apps/api/internal/chat/passenger_clarification_repository_test.go
apps/api/internal/chat/handler_test.go
apps/api/internal/chat/passenger_clarification_test_helper_test.go
apps/api/internal/chat/booking_draft_context_test.go
apps/api/internal/chat/openai_interpreter_assist_test.go
```

Não houve migration, parser, regex, provider, código B2, commit, push, deploy ou
smoke.

#### Correção local dos 4 P1 anteriores de autoridade durável

O review anterior preservou os achados anteriores como corrigidos, mas encontrou
quatro caminhos em que artefatos legados ou eventos concorrentes ainda podiam
reconceder autoridade:

1. o primeiro bootstrap de `AvailabilitySelectionStateV1` ignorava
   `canonical_availability_facts_invalidated` e sua boundary, permitindo que
   seleção estruturada anterior voltasse como `BOOKABLE`;
2. projeção legada completa sem `materializes_authority` podia usar o próprio
   outbound como `selection_message_id` e virar nova materialização;
3. seleção legada sem prompt source fabricava o outbound de seleção como fonte,
   portanto a rejeição estruturada apontada para a lista original não a atingia;
4. duas reduções concorrentes eram decididas pela ordem de aquisição do lock,
   não pela ordem causal das mensagens.

A correção mantém a responsabilidade dentro do estado durável:

- no primeiro bootstrap, o repository lê estado, marker, boundary e transcript
  estruturado na mesma transação. A boundary vira
  `SELECTION_INVALIDATED` ordenado; materialização pré-boundary é removida e
  materialização posterior pode substituir a invalidação. Marker legado sem
  boundary produz `INVALIDATED` conservador até nova materialização explícita;
- ausência de `materializes_authority` significa `false`, sem fallback de
  `selection_message_id` para o ID da projeção. Projeções de passageiros,
  documentos e continuação nunca materializam. Compatibilidade legada exige
  intent estrutural `SELECT_AVAILABILITY_OPTION`, snapshot completo e fonte
  exata/inequívoca;
- fonte ausente é recuperada apenas de outbound anterior confiável cujo item no
  mesmo índice possui exatamente os mesmos `trip_id`, `board_stop_id` e
  `alight_stop_id`. Não há leitura de `body`, parser lexical ou uso do outbound
  de seleção como prompt source; zero ou múltiplas fontes deixam o estado
  `NONE`;
- eventos e tombstones carregam ordem
  `received_at, created_at, message_id, event_ordinal`; o estado persiste
  `last_applied_event_order`. O repository sobrescreve ordem/ID fornecidos pelo
  caller com a mensagem real do banco, ordena batches e impede evento mais
  antigo de substituir seleção ou tombstone atual;
- a prova concorrente executou em PostgreSQL 16 real, com duas pools e caller
  enviando ordem deliberadamente falsa. S2 permaneceu autoritativa quando
  aplicada antes de S1 pelo lock, e rejeição nova permaneceu `REJECTED` quando a
  materialização antiga adquiriu o lock depois.

Não houve migration, parser, regex, provider, código B2, mudança de contrato
público, commit, push, deploy ou smoke.

#### Correção local dos 4 P1 do review anterior de replay canônico

O review atual preservou as correções anteriores, mas comprovou quatro falhas
restantes no contrato de replay:

1. legado sem `selection_message_id` explícito ainda podia fabricar identidade
   a partir do outbound/projeção;
2. seleção ou continuação propagada podia ser aceita como prompt source quando
   a lista original já não estava disponível;
3. `LastAppliedEventOrder` podia descartar evento durável atrasado, deixando o
   estado live dependente da ordem de aquisição do lock e diferente do restart;
4. a prova PostgreSQL ainda retornava `SKIP` com URL ausente, mesmo quando
   deveria ser gate obrigatório.

A regra agora é única dentro do lock da sessão:

1. hidratar o evento atual a partir da mensagem real e persistir sua forma
   estrutural no inbound;
2. carregar todas as mensagens da sessão necessárias aos eventos estruturados,
   sem `body` e sem `LIMIT 50`;
3. incorporar marker/boundary legado;
4. hidratar ordem somente de `received_at`, `created_at`, `message_id` e ordinal;
5. ordenar todos os eventos e reduzir sempre desde
   `newAvailabilitySelectionStateV1()`;
6. validar e persistir a projeção completa.

`LastAppliedEventOrder` é apenas resultado do replay e não exclui evento.
Identidade legada ausente permanece `NONE`. Prompt source legado exige
exatamente uma lista anterior confiável, estrutural, completa e compatível;
seleção, passageiros, documentos, pagamento e continuação não são candidatos.
Sem projeção reconstruída e validada, booking draft e `booking_create` falham
fechado.

A propriedade cobre as seis permutações de materialização antiga `A`, rejeição
nova `B` de outra fonte e projeção não autoritativa, com duplicação e restart em
cada permutação. Quando `B` obtém o lock primeiro e `A` chega depois, o replay
ordena causalmente `A` antes de `B`: `A` permanece `BOOKABLE` e a rejeição de
`B` continua no ledger. Live, reload e bootstrap são idênticos.

O modo `CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1` agora falha sem URL. A
prova executou `count=20` em PostgreSQL 16 real efêmero, com duas pools, lock
invertido e comparação byte a byte entre live e restart, sem `SKIP`.

Não houve migration, parser, regex, provider, código B2, mudança de contrato
público, commit, push, deploy ou smoke.

#### Regressões e arquivos alterados

As regressões cobrem confirmação `"ok"` com opção única completa, sem facts e
com cada ID obrigatório ausente; deíxis ambígua com várias opções completas;
seleção posterior por `"1"`; índice fora do range sem contaminação; índice falho
seguido de disponibilidade nova e tentativa de `booking_create`; seleção
bookable persistida; confirmação fora de prompt; zero trabalho externo nos
caminhos de clarificação/invalidação; identidade do prompt depois de
`ASK_PASSENGER_COUNT`; os quatro turnos mistos `STRONG`; ausência/presença de
autoridade bookable na confirmação documental; e todos os casos dos oito P1
anteriores. O P1 do review anterior acrescentou as frases exatas com
`"opção 1"` e `"primeira opção"`, baseline sem seleção anterior, preservação
integral da opção `2` anterior, controle positivo puro `"opção 1"`,
marker/boundary preexistentes e o turno seguinte capturado diretamente no
router e no shadow V1.

O P1 do review anterior cobre opção única completa e não selecionada nos quatro
turnos `STRONG` exigidos; baseline unitário somente com envelope; ausência de
item em canonical, memory, metadata, draft e próximo router/interpreter;
preservação exata da opção `2` diante de uma nova lista unitária; e os controles
positivos puros `"ok"`/`"1"` materializando a opção antes de
`ASK_PASSENGER_COUNT`.

Os 3 P1 do review anterior acrescentaram:

- `S1/A → S2/B → rejeição de S2`, resultando em nenhuma autoridade, sem
  ressurreição de `S1` nem mesmo por projeção stale posterior, e sem abertura
  de `booking_create`;
- `S1/A → S2/B → rejeição de S1`, preservando `S2/B` integralmente;
- seleção da opção `1` na lista `A` + rejeição da opção `1`/mesma data na lista
  `B`, preservando `A`; a mesma rejeição originada em `A` remove a autoridade;
- snapshot legado sem pacote + envelope posterior compatível/pacote `B`, sem
  uso de `B`;
- recuperação de pacote `A` somente pelo próprio evento de seleção ou pela
  mensagem exata indicada pelo prompt source; fonte ausente mantém pacote vazio;
- `MATERIALIZE` posterior de opção/pacote `B` substituindo todo o agregado;
- reload/turno seguinte reconstruindo autoridade, `SelectionMessageID`,
  `AvailabilityPromptSourceMessageID`, IDs da viagem e pacote sem drift.

Os 3 P1 de autoridade durável acrescentam:

- lista unitária selecionada e rejeitada permanece sem autoridade e com zero
  `booking_create`, mesmo quando só a projeção posterior está na janela;
- `S1 → S2 → rejeição de S2` termina sem autoridade e sem ressurreição de `S1`;
- rejeição da lista `B` não limpa a seleção da lista `A`;
- rejeição simples persiste o prompt source no inbound e reload mantém o
  tombstone;
- janela apenas com projeção superseded permanece `NONE`;
- `BOOKABLE` alimenta o booking draft e o controle positivo de booking, enquanto
  `NONE`/`REJECTED`/`INVALIDATED` produzem zero input;
- materialização ausente do estado devolvido falha fechado, com zero runner e
  zero `booking_create`;
- bootstrap atravessa mais de 50 mensagens estruturadas uma única vez;
- concorrência fake preserva ambos os eventos.

Os 4 P1 anteriores de autoridade durável acrescentaram:

- marker/boundary antigo bloqueia seleção pré-boundary, enquanto materialização
  explícita pós-boundary volta a `BOOKABLE`;
- marker legado sem boundary termina e permanece `INVALIDATED` conservador;
- projeções `PASSENGER_COUNT_REPLY`, documentos e continuação, sem autoridade
  explícita, permanecem `NONE` e produzem zero `booking_create`;
- seleção legada resolve somente o prompt original estrutural exato; fonte
  ausente ou ambígua permanece `NONE`, e rejeição legada atinge a fonte
  recuperada;
- S2 aplicada antes de S1 continua S2; rejeição nova aplicada antes de
  materialização antiga continua `REJECTED`;
- restart/bootstrap reproduzem byte a byte os estados finais de seleção e
  rejeição;
- PostgreSQL real com duas pools confirma serialização, hidratação da ordem pelo
  banco e zero `booking_create` sem `BOOKABLE`.

Os 4 P1 do review anterior de replay canônico acrescentaram:

- legado sem ID explícito permanece `NONE`, sem fallback para projeção;
- lista-fonte deve ser estrutural, confiável, completa, compatível e única;
  seleção/projeção e continuações são rejeitadas como fonte;
- todas as permutações de `A`, rejeição `B` de outra fonte e projeção
  não autoritativa, inclusive com duplicatas, geram o mesmo estado;
- live, reload e bootstrap após cada permutação são idênticos;
- rejeição `B` que adquire o lock antes de materialização causal anterior `A`
  não elimina `A`, mas permanece registrada;
- marker/boundary participa do mesmo replay; marker sem boundary permanece
  conservadoramente `INVALIDATED`;
- história limitada e envelope atual não são autoridade de leitura;
- modo PostgreSQL obrigatório falha sem URL e passa em PostgreSQL 16 real,
  duas pools, lock invertido e `count=20`.

Produção:

```text
apps/api/internal/chat/passenger_clarification_reducer.go
apps/api/internal/chat/passenger_clarification_evidence.go
apps/api/internal/chat/availability_invalidation_history.go
apps/api/internal/chat/availability_selection_state_v1.go
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/availability_draft.go
apps/api/internal/chat/booking_draft_context.go
apps/api/internal/chat/booking_create_router.go
apps/api/internal/chat/conversation_state_machine.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/repository.go
apps/api/internal/chat/service.go
apps/api/internal/chat/tool_router.go
```

Testes:

```text
apps/api/internal/chat/passenger_clarification_gate_test.go
apps/api/internal/chat/passenger_clarification_evidence_test.go
apps/api/internal/chat/passenger_clarification_state_v1_test.go
apps/api/internal/chat/passenger_clarification_test_helper_test.go
apps/api/internal/chat/availability_selection_state_v1_test.go
apps/api/internal/chat/availability_selection_repository_test.go
apps/api/internal/chat/passenger_clarification_repository_test.go
apps/api/internal/chat/booking_create_router_test.go
apps/api/internal/chat/booking_draft_context_test.go
apps/api/internal/chat/chat_flow_guardrails_test.go
apps/api/internal/chat/conversation_state_machine_test.go
apps/api/internal/chat/handler_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/interpreter_test.go
```

Documentação:

```text
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
plans/README.md
plans/00-plano-mestre-travel-semantic-v2.md
plans/h-2026-07-16b-passenger-child-state.md
plans/h-2026-07-16b1-passenger-state-foundation.md
plans/h-2026-07-16b2-passenger-meaning-v1.md
plans/h-2026-07-22a-fresh-session-passenger-gate.md
```

#### Validação e review

A evidência das rodadas anteriores continua registrada abaixo. A matriz dos
4 P1 do review anterior de replay canônico foi executada integralmente em `apps/api`, sem
reduzir os comandos exigidos:

```text
RED NOVO — ASK_PASSENGER_COUNT com facts de continuidade reativou seleção: "1" trocou opção 2 pela 1 e "sim" virou fallback de availability
RED NOVO — os quatro turnos mistos deíxis/índice + humano/cancelamento foram classificados antes de STRONG; houve fallback e até runner legado
RED NOVO — resultado único exibido + índice bruto + documentos completos + "sim" produziu BookingCreateInput com índice 0 e chamou booking_create uma vez
PASS PRESERVADO — identidade real do prompt, precedência STRONG e HasBookableSelection continuam corrigidos
RED P1 DO REVIEW ANTERIOR — router recebeu opção 1 materializada antes do cancelamento/handoff STRONG
RED P1 DO REVIEW ANTERIOR — marker/boundary preexistentes fizeram fallback de availability vencer cancelamento STRONG
PASS — go test -count=20 ./internal/chat -run 'Test.*Strong.*Selection.*State|Test.*Mixed.*Strong.*Availability|Test.*Strong.*Preserves.*Bookable|Test.*Selection.*After.*Strong'
RED P1 RESTANTE — baseline unitário com uma opção não selecionada manteve index=0/bookable=false, mas promoveu trip/board/alight, data, horário, preço e moeda do item
RED P1 RESTANTE — os quatro turnos STRONG receberam o item único no router; ele reapareceu no router/interpreter do turno seguinte
PASS CONTROLE — opção 2 bookable anterior permaneceu exata diante da nova lista unitária; "ok"/"1" puros materializaram a opção atual e produziram ASK_PASSENGER_COUNT
PASS — go test -count=20 ./internal/chat -run 'Test.*Single.*Option.*Strong.*Baseline|Test.*Routing.*Baseline.*Unselected|Test.*Strong.*Preserves.*Bookable|Test.*Pure.*Single.*Option.*Materialize'
RED P1 ATUAL 1 — rejeição posterior da opção 1 devolveu authority=false para a opção 2 bookable não rejeitada
RED P1 ATUAL 2 — baseline preservou IDs/data/preço da opção 2, mas recebeu package-unselected-b do envelope posterior
PASS — go test -count=20 ./internal/chat -run 'Test.*Single.*Option.*Strong.*Baseline|Test.*Routing.*Baseline.*Unselected|Test.*Strong.*Preserves.*Bookable|Test.*Pure.*Single.*Option.*Materialize|TestBookingDraftResolvesBookableAuthorityAcrossLaterAvailabilityRejections|TestRoutingBaselineKeepsSelectedPackageAtomicAgainstLaterUnselectedEnvelope|TestMaterializeReplacesPriorBookableSelectionAggregateAndSurvivesReload'
RED P1 ATUAL 1 — S1/A ressuscitou depois de S2/B materializada e rejeitada; booking_create permaneceu aberto com S1
RED P1 ATUAL 2 — rejeição da opção 1/lista B removeu a seleção opção 1/lista A na colisão de índice e data
RED P1 ATUAL 3 — envelope posterior compatível injetou package-b no snapshot legado antes do próprio evento/fonte A
PASS — go test -count=20 ./internal/chat -run 'TestBookingDraft(ReducesBookableAuthorityWithoutResurrectingSupersededSelection|ScopesAvailabilityRejectionToPromptSource|LegacyRecoveryUsesOnlySelectionEventOrExactPromptSource|ResolvesBookableAuthorityAcrossLaterAvailabilityRejections)$|TestRoutingBaselineKeepsSelectedPackageAtomicAgainstLaterUnselectedEnvelope$|TestMaterializeReplacesPriorBookableSelectionAggregateAndSurvivesReload$|TestAvailabilitySelectionAfterSpecificRejected(Option|Date)OutOfTurnPayment$|TestSelectedAvailabilitySelectionPassengerFailClosedPersistsBeyondHistoryWindow$'
RED P1 DURÁVEL 1 — booking_create comparava a rejeição com o outbound unitário de passageiros, não com o prompt source original
RED P1 DURÁVEL 2 — projeção propagada podia adquirir autoridade quando MATERIALIZE/tombstone saíam da janela
RED P1 DURÁVEL 3 — rejeição simples perdia a fonte depois do truncamento porque nenhum evento a persistia
RED P1 ANTERIOR 1 — bootstrap legado ignorou marker/boundary e restaurou seleção pré-boundary como BOOKABLE
RED P1 ANTERIOR 2 — projeção PASSENGER_COUNT_REPLY sem autoridade explícita virou nova materialização
RED P1 ANTERIOR 3 — seleção legada fabricou o outbound de seleção como prompt source e escapou da rejeição da lista original
RED P1 ANTERIOR 4 — lock adquirido fora de ordem permitiu S1 antiga substituir S2 e materialização antiga reabrir rejeição nova
PASS RODADA ANTERIOR — go test -count=20 ./internal/chat -run 'TestAvailabilitySelectionStateV1|TestBookingDraft(ReducesBookableAuthorityWithoutResurrectingSupersededSelection|ScopesAvailabilityRejectionToPromptSource|LegacyRecoveryUsesOnlySelectionEventOrExactPromptSource|ResolvesBookableAuthorityAcrossLaterAvailabilityRejections)$|TestRoutingBaselineKeepsSelectedPackageAtomicAgainstLaterUnselectedEnvelope$|TestMaterializeReplacesPriorBookableSelectionAggregateAndSurvivesReload$|TestAvailabilitySelectionAfterSpecificRejected(Option|Date)OutOfTurnPayment$|TestSelectedAvailabilitySelectionPassengerFailClosedPersistsBeyondHistoryWindow$'
RED P1 REPLAY 1 — seleção legada sem selection_message_id recebeu identidade fabricada da projeção
RED P1 REPLAY 2 — seleção/projeção anterior foi aceita como prompt source sem lista original
RED P1 REPLAY 3 — evento durável atrasado foi descartado pelo cursor e live divergiu do restart
RED P1 REPLAY 4 — URL PostgreSQL ausente ainda produziu SKIP com suíte verde
PASS — go test -count=20 ./internal/chat -run 'Test.*AvailabilitySelection.*(Replay|Order|Legacy|Projection|Invalidation)'
PASS — go test -race -count=1 ./internal/chat
PASS — modo obrigatório falha quando CHAT_PASSENGER_STATE_POSTGRES_TEST_URL está ausente
PASS SEM SKIP — CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1 + PostgreSQL 16 real efêmero + duas pools + count=20; lock invertido e live/restart idênticos
PASS — go test -count=1 ./internal/chat
PASS — go test -count=1 ./...
PASS — rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l => 54
PASS — git diff --check
```

Resultado do review atual: **5 P1 DE IDENTIDADE E REPLAY CANÔNICO**.
O patch manual resolve a identidade da seleção contra o stream inbound da
sessão, deriva a ordem do evento selecionado, exige lista-fonte explicitamente
estrutural, remove duplicatas antes do ordinal e amplia a propriedade para os
seis batches causais em 720 permutações. Os P1 anteriores permanecem no escopo
de regressão.

#### Reconciliação da suíte executada em 2026-07-27

A primeira aplicação manual dos 5 P1 deixou `go test -race -count=1
./internal/chat`, `go test -count=1 ./internal/chat` e `go test -count=1 ./...`
em RED. As falhas convergiam para `HasBookableSelection=false` e
`ROUTE_SELECTION`, enquanto a matriz canônica do patch passava isoladamente.
A causa raiz confirmada foi incompatibilidade das fixtures: projeções
`OUTBOUND` fabricavam `selection_message_id`/prompt source sem a mensagem
`INBOUND` correspondente e listas antigas carregavam apenas `tool_context`, sem
`intent`/`template_name` estrutural.

A reconciliação ficou restrita a testes. Os helpers de seleção agora:

- persistem estado/evento moderno explicitamente, com mensagem `INBOUND` real,
  prompt source real e projeção sem autoridade;
- representam bootstrap legado pela timeline completa lista `OUTBOUND` →
  seleção `INBOUND` → projeção `OUTBOUND`;
- exigem IDs explícitos quando a fixture declara `materializes_authority=true`;
- reconstroem `materializePersistedAvailabilitySelectionForTest` somente pelo
  replay estruturado e não por evidência projetada;
- não criam seleção a partir do texto do wrapper de `booking_create`.

Não houve RED independente que pudesse ocorrer no repository real. Portanto
nenhum código de produção adicional foi alterado para acomodar a suíte; o único
delta de produção permanece o patch manual em
`availability_selection_state_v1.go`.

Validação final realmente executada em `apps/api`:

```text
PASS — grupo de fixtures diretamente afetado
PASS — go test -count=20 ./internal/chat -run 'Test.*AvailabilitySelection.*(Replay|Order|Legacy|Projection|Invalidation)' — 15.562s
PASS — provas funcionais 1–10 em count=20 — 17.939s
PASS — go test -race -count=1 ./internal/chat — 23.570s
PASS — regressões H-012/document/lap-child/payment/human/out-of-turn — 1.682s
PASS — regressões cancel/passenger/availability — 3.832s
PASS SEM SKIP — PostgreSQL 16 real efêmero, duas pools, lock invertido, count=20, live/reload/restart — 9.895s
PASS — go test -count=1 ./internal/chat — 4.243s
PASS — go test -count=1 ./... — internal/chat 4.862s e demais pacotes verdes
PASS — rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l => 54
PASS — gofmt sem arquivos pendentes
PASS — git diff --check
```

#### Checkpoint pré-review final — histórico/superseded

No checkpoint imediatamente anterior ao review final, o PostgreSQL 16 havia
sido criado somente para a prova e removido em seguida; ainda não existia
declaração de review limpo, e commit, push, deploy, smoke e teste em produção
não haviam sido executados. Esse status intermediário foi superseded pelo
review final abaixo.

#### Review final limpo — checkpoint histórico anterior ao deploy

O review final concluiu que o patch está **seguro para commit** e não encontrou
P1/P2. Foram confirmados os 10 controles: `count=20`, race, suítes amplas,
PostgreSQL **16.14** real sem `SKIP`, inventário de 54 `regexp.MustCompile`,
`gofmt` e `git diff --check`.

Status naquele checkpoint:

```text
H-2026-07-22A — REVIEW FINAL SEM P1/P2 — SEGURO PARA COMMIT;
DEPLOY E SMOKE PENDENTES
```

Esse checkpoint foi seguido por commit, push e deploy. O smoke real posterior
ficou RED na transição availability → passageiros e é registrado em
H-2026-07-27A abaixo. H-B2 permanece **BLOQUEADA**.

---

### 8.12 Incidente operacional — H-2026-07-27A (2026-07-27)

**Status histórico de abertura — SUPERADO PELA FILA CANÔNICA:**
H-2026-07-27A — **EM CORREÇÃO APÓS REVIEW — 5 P1 + 1 P2 DE RESOLUÇÃO ÚNICA
DE DOMÍNIO, FONTE E FACTS**.

H-2026-07-22A corrigiu o bootstrap de sessão nova, passou pelo review final e
foi deployado. O smoke real posterior reabriu o gate em uma fronteira diferente
da transição availability → passageiros:

```text
availability_search retornou 8 resultados no tool_context
a resposta apresentou somente 03/08/2026 e pediu confirmação dessa opção
availability_prompt_event_v1 estava ausente
availability_option_count foi inferido como 8
o cliente respondeu "sim"
AvailabilitySelectionStateV1 permaneceu NONE
a resposta final foi SAFE_PHASE_FALLBACK
booking_create não executou
```

#### Causa

O runtime usava a quantidade bruta de `tool_context.results` como aproximação
das opções apresentadas. A LLM podia mostrar somente uma opção, enquanto o
backend via oito. A identidade do prompt também dependia do body, e
`"Deseja seguir com essa opção?"` não era reconhecido pelo fallback lexical.

O hotfix não adiciona essa frase ao parser. Ele separa explicitamente:

```text
raw tool results
presented options
selected/bookable option
```

#### Contrato e fluxo implementados

`AvailabilityPromptEventV1` é persistido sem migration e contém:

```text
version
kind = AVAILABILITY_OPTION_CHOICE
source_message_id
presented_option_count
presented_options[] {
  display_index
  result_index
  trip_id
  board_stop_id
  alight_stop_id
  trip_date
}
```

O backend realiza deterministicamente a apresentação unitária do caminho atual
de `EARLIEST_AVAILABLE`, escolhendo o primeiro resultado estruturalmente válido
e produzindo a resposta, sem escolha silenciosa da LLM. Para listas, persiste
exatamente até cinco opções apresentadas. Os resultados brutos continuam
preservados no `tool_context`; o evento registra apenas a projeção visível.

O draft recebe o evento com sua própria identidade. O envio automático ou a
aprovação sem edição projeta o mesmo contrato no `BOT_AUTO_REPLY`/outbound com
o `source_message_id` real do outbound, mantendo ligação ao draft que contém os
facts brutos. A projeção é validada contra `trip_id`, stops e data do resultado
bruto.

`InferActivePromptContext`, router, interpreter e os consumidores de facts
preferem o evento estrutural. Quando ele existe, o option count e o contexto
visível vêm de `presented_option_count/presented_options`, nunca do comprimento
bruto. O body permanece somente fallback legado.

`MATERIALIZE` resolve `display_index` contra a projeção apresentada e recupera
o item bruto apontado por `result_index`, produzindo snapshot completo.
O evento de prompt sozinho continua sem autoridade: `BOOKABLE` só aparece após
confirmação válida do cliente e `SELECTION_MATERIALIZED`. Sem materialização,
todos os entrypoints de `booking_create` permanecem fechados.

Não houve B2, mudança de `TravelQueryMeaningV2`, migration, provider, parser ou
regex novo. O inventário de produção permanece em 54
`regexp.MustCompile`.

#### RED real

Antes da implementação, a nova matriz reproduziu o comportamento:

```text
go test -count=1 ./internal/chat -run '^TestAvailabilityPromptEventV1'
FAIL — runner=1 ainda escolhia silenciosamente a apresentação
FAIL — availability_prompt_event_v1 não existia
FAIL — confirmações podiam executar availability_search pela segunda vez
```

#### PASS local

```text
PASS — go test -count=20 ./internal/chat -run '^TestAvailabilityPromptEventV1' — 1.485s
PASS — go test -race -count=1 ./internal/chat -run '^TestAvailabilityPromptEventV1' — 1.539s
PASS — matriz focada availability/passenger/booking/human/cancel — 0.129s
PASS — go test -count=1 ./internal/chat — 4.121s
PASS — go test -count=1 ./... — internal/chat 4.145s; demais pacotes verdes
PASS — rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l => 54
PASS — gofmt
PASS — git diff --check
```

A matriz A–I confirma:

1. oito resultados brutos e somente um apresentado materializam essa opção com
   `"sim"`, abrem `ASK_PASSENGER_COUNT` e mantêm auto-send verde;
2. `"1"`, `"essa msm"` e `"pode ser"` materializam a mesma opção unitária;
3. cinco opções apresentadas não transformam `"sim"` em opção 1 e retornam
   `CLARIFY_PRESERVE`;
4. `display_index=1` pode apontar para `result_index=3` e materializa exatamente
   o quarto item bruto;
5. draft não enviado, `SEND_FAILED`, `REVIEW_REQUIRED` e `PENDING` isolado não
   abrem active prompt;
6. `ASK_PASSENGER_COUNT` com `tool_context` não reabre availability;
7. humano e cancelamento `STRONG` continuam vencendo;
8. sem `SELECTION_MATERIALIZED`, `booking_create` recebe zero chamadas;
9. prompt source, projeção e seleção permanecem coerentes após reload/restart.

#### Correção após review — 3 P1 de proveniência e entrega estrutural

O review dirigido preservou os P1 anteriores e encontrou três falhas na
reconciliação entre a mensagem entregue e o draft que contém os facts:

1. `BOT_AUTO_REPLY` comparava body antes do evento estrutural equivalente;
2. o bootstrap legado podia aceitar evento + `tool_context` de mensagem
   `INBOUND`;
3. `DRAFT_REVIEW / APPROVED_AS_IS` projetava o evento, mas não resolvia os
   facts do `reviewedDraft`.

A correção criou uma única
`resolveDeliveredPromptSourceMessageWithIndex`, usada por
`BOT_AUTO_REPLY`, `DRAFT_REVIEW / APPROVED_AS_IS` e pelo bootstrap legado.
A mensagem efetiva usa ID, body, status e evento validados do outbound
entregue; somente o draft `OUTBOUND` ligado e validado fornece
`tool_context`. Eventos são comparados estruturalmente após normalização de
kind, count e todos os campos de `presented_options`. Body só participa do
fallback legado quando nenhum dos dois lados contém evento.

Entrega ausente, pending, source inválida, evento ausente/inválido/divergente
ou revisão `EDITED` falham fechado. O bootstrap rejeita direção não
`OUTBOUND` antes de evento/facts e não volta ao draft quando existe uma
entrega vinculada que falhou na reconciliação. `APPROVED_AS_IS` mantém
`draft_message_id` durável e o evento com a identidade do outbound; nenhuma
cópia de facts foi adicionada ao outbound.

RED real antes do patch:

```text
FAIL — BOT_AUTO_REPLY com body diferente: OptionCount=1, HasCurrentFacts=false
FAIL — evento divergente: facts do draft ainda eram herdados
FAIL — APPROVED_AS_IS: OptionCount=1, HasCurrentFacts=false
FAIL — evento/tool_context INBOUND + projeção posterior: estado BOOKABLE
PASS — revisão EDITED já permanecia fail-closed
```

PASS real após o patch:

```text
PASS — testes novos dirigidos, count=20 — 1.325s
PASS — go test -race -count=1 ./internal/chat — 23.499s
PASS — availability/passenger/booking/human/cancel — 3.185s
PASS — go test -count=1 ./internal/chat — 4.289s
PASS — go test -count=1 ./... — internal/chat 4.290s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt
PASS — git diff --check
```

A matriz dirigida cobre body divergente com evento equivalente, divergência
estrutural, inbound fabricado, `APPROVED_AS_IS`, `EDITED`, source entregue no
bootstrap, supressão do fallback ao draft vinculado e bloqueio de
`BookingCreateInput`. As regressões anteriores de múltiplas opções,
`display_index → result_index`, `ASK_PASSENGER_COUNT`, humano/cancelamento
`STRONG`, evento sem confirmação, pending isolado e reload/restart continuam
verdes.

Não há declaração de review limpo. Não foram executados commit, push, deploy ou
smoke nesta rodada. Próxima ação única: novo `/review` dirigido aos três P1 de
H-2026-07-27A. H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

#### Correção após o segundo review — entrega confirmada, evento fail-closed e metadata reservada

O segundo review dirigido confirmou as reproduções anteriores e encontrou
três novas brechas P1:

1. status vazio, `RECEIVED`, `PROCESSED` ou desconhecido ainda podia autorizar
   um evento estrutural, e até status nominalmente enviado podia ser aceito sem
   a evidência canônica de delivery;
2. `availability_prompt_event_v1` presente, mas malformado ou inconsistente,
   podia cair no fallback legado e reconstruir `BOOKABLE`;
3. metadata do cliente em `APPROVED_AS_IS` podia sobrescrever a proveniência
   reservada e injetar `tool_context`, fazendo o outbound fornecer facts em
   lugar do draft revisado.

A correção usa uma confirmação única de delivery: somente `SENT`, `DELIVERED`,
`READ` ou `AUTOMATION_SENT`, sempre com `delivery_recorded_at` canônico,
autorizam o prompt estrutural. A resolução da entrega copia essa evidência ao
source efetivo sem promover payload arbitrário.

A presença da chave `availability_prompt_event_v1` agora é separada da
validação do conteúdo. Decode inválido, cópias divergentes ou
`source_message_id` incorreto falham fechado em active prompt, router,
interpreter, bootstrap e consumidores de facts; body e projeção legada só
continuam disponíveis quando o evento estrutural realmente não existe.

`CreateReply` e `CreateAutomationReply` filtram metadata reservada antes de
compor a mensagem e o payload. Modo, IDs, revisão, eventos estruturais,
autoridade, tool payloads e evidência de delivery permanecem sob controle da
API. `APPROVED_AS_IS` preserva a ligação canônica e resolve facts somente do
draft `OUTBOUND` validado; `tool_context` enviado pelo cliente não é
persistido.

RED real antes do patch:

```text
FAIL — vazio/RECEIVED/PROCESSED/desconhecido com delivery_recorded_at abriam o prompt
FAIL — SENT/AUTOMATION_SENT sem delivery_recorded_at abriam o prompt
FAIL — os mesmos casos podiam reconstruir AvailabilitySelectionStateV1=BOOKABLE
FAIL — evento divergente, source_message_id incorreto ou decode inválido caíam no legado e viravam BOOKABLE
FAIL — metadata do cliente persistia mode/draft/review falsos e tool_context forjado
```

PASS local realmente executado:

```text
PASS — go test -count=20 ./internal/chat -run '^TestAvailabilityPromptEventV1|^TestAvailabilitySelectionStateV1LegacyAuthorityRequiresExactStructuralSource$' — 3.582s
PASS — go test -race -count=1 ./internal/chat — 24.519s
PASS — go test -count=1 ./internal/chat -run 'Availability|Passenger|Booking|Human|Cancel' — 2.918s
PASS — go test -count=1 ./internal/chat — 4.311s
PASS — go test -count=1 ./... — internal/chat 4.306s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt
PASS — git diff --check
```

Não há declaração de review limpo nem segurança para commit. Não foram
executados commit, push, deploy ou smoke. Próxima ação única: novo `/review`
dirigido a estas três brechas P1. H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.

#### Correção após o terceiro review — 5 P1 de autoridade de entrega e visibilidade

O terceiro review dirigido preservou as correções anteriores e encontrou cinco
brechas na fronteira entre entrega confirmada, evento estrutural e conteúdo
efetivamente exibido:

1. o allowlist era aplicado somente a prompts com evento; legado sem evento e
   sem entrega confirmada ainda abria prompt e fornecia facts;
2. `DELIVERY_ACK`, embora seja delivery no fluxo Evolution, removia a
   autoridade de um prompt estrutural já entregue;
3. uma única cópia válida de `availability_prompt_event_v1` era suficiente;
4. evento inválido ainda podia alimentar leitores legados de facts, booking
   draft e `booking_create`;
5. metadata de `APPROVED_AS_IS` podia transformar draft de texto aprovado em
   mídia e materializar facts de conteúdo que o cliente não viu.

A política agora é única e classifica toda mensagem candidata:

```text
UNDELIVERED
ABSENT_LEGACY
VALID_STRUCTURAL
INVALID
```

`UNDELIVERED` cobre direção não `OUTBOUND`, draft/pending/falha/status
desconhecido, status fora do allowlist e ausência de
`delivery_recorded_at`. O allowlist único contém somente `SENT`,
`DELIVERY_ACK`, `DELIVERED`, `READ` e `AUTOMATION_SENT`; não há aliases.

`ABSENT_LEGACY` exige evento ausente nas duas cópias e entrega confirmada.
`VALID_STRUCTURAL` exige evento presente em `Payload` e
`NormalizedPayload`, decodificável, idêntico e consistente com ID, count,
opções e facts. Presença unilateral, decode inválido, divergência ou
inconsistência resulta `INVALID`. Somente `ABSENT_LEGACY` pode entrar no
fallback legado.

Active prompt, option count, `findLatestAvailabilityContext`,
`shouldMergeAvailabilityFactsFromMessage`, contextos visible/trusted,
booking draft, enriquecimento de snapshot legado, bootstrap de seleção e
`booking_create` consomem essa classificação. `UNDELIVERED` e `INVALID` não
fornecem facts, não materializam seleção e não reconstroem `BOOKABLE`.

Em `APPROVED_AS_IS`, metadata comum continua preservada, mas modo, IDs de
draft/review, eventos, `tool_context`, body/text e os campos/aliases reais de
mídia do sender são reservados. Um draft `TEXT` aprovado permanece `TEXT`,
com o body, evento e facts validados; mídia humana legítima fora da revisão
continua disponível.

RED real antes do patch:

```text
go test -count=1 ./internal/chat -run '^TestAvailabilityPromptEventV1(ApprovedDraftReviewPreservesReservedProvenanceAgainstMetadata|LegacyDeliveryClassification|DeliveryAckPreservesStructuralPrompt|InvalidCopiesFailClosedAcrossReaders)$'
FAIL — APPROVED_AS_IS virou AUDIO e persistiu body/text/media hostis
FAIL — legado vazio/RECEIVED/PROCESSED/desconhecido ou SENT sem evidence abriu prompt
FAIL — DELIVERY_ACK transformou prompt estrutural válido em UNKNOWN
FAIL — evento unilateral abriu prompt; evento malformado/divergente vazou facts em readers legados
FAIL — as quatro variantes INVALID enriqueceram package/price de snapshot tipado a partir do tool_context bruto
```

PASS local realmente executado:

```text
PASS — go test -count=20 ./internal/chat -run '^TestAvailabilityPromptEventV1(LegacyDeliveryClassification|DeliveryAckPreservesStructuralPrompt|InvalidCopiesFailClosedAcrossReaders|ApprovedDraftReviewPreservesReservedProvenanceAgainstMetadata)$' — 0.650s
PASS — go test -race -count=1 ./internal/chat — 28.873s
PASS — go test -count=1 ./internal/chat -run '(Availability|Passenger|Booking|Human|Cancel|Review|AutoSend|Delivery)' — 3.740s
PASS — go test -count=1 ./internal/chat — 5.114s
PASS — go test -count=1 ./... — internal/chat 6.579s; demais pacotes verdes
PASS — rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l => 54
PASS — gofmt; gofmt -l sem saída
PASS — git diff --check
```

Nota operacional: a primeira tentativa do race não chegou aos testes porque
`/tmp` ficou sem quota durante a compilação. Foi limpo somente o
`GOCACHE=/tmp/schumacher-h27-go-build`; o mesmo comando foi repetido e passou
integralmente.

A matriz A–I cobre legado não entregue e entregue, `DELIVERY_ACK`, cópias
unilaterais/malformadas/divergentes, todos os readers de facts, enriquecimento
de snapshot, booking draft e gate de `booking_create`, aprovação `TEXT` com
metadata hostil, mídia humana legítima, `BOT_AUTO_REPLY` equivalente, revisão
`EDITED`, pending, `ASK_PASSENGER_COUNT`, humano/cancelamento `STRONG` e gates
de booking.

Não há declaração de review limpo nem segurança para commit. Não foram
executados commit, push, deploy ou smoke. A única próxima ação é um novo
`/review` dirigido aos cinco P1 de H-2026-07-27A. H-2026-07-16B2 e 3.6F-D
permanecem bloqueadas.

#### Correção após o quarto review — 3 P1 + 1 P2 de entrega temporal e proveniência

O quarto review dirigido preservou as reproduções anteriores e encontrou três
brechas P1 e uma lacuna P2:

1. `RecordEvolutionStatus` substituía `DELIVERY_ACK` por `SERVER_ACK` tardio,
   mesmo preservando parte da evidência de entrega;
2. um `INVALID` entregue mais novo era ignorado no scan reverso e ressuscitava
   uma lista legada antiga;
3. `DRAFT_REVIEW` com `EDITED`, action ausente/desconhecida ou cópias
   divergentes ainda podia cair no fallback legado;
4. a proteção contra metadata de mídia hostil era provada somente pelo
   `fakeStore`, não pelo `Repository.CreateReply` real.

A correção introduz uma política monotônica compartilhada por readers e
writers. `SERVER_ACK < SENT/AUTOMATION_SENT < DELIVERY_ACK < DELIVERED < READ`;
status desconhecido não promove nem regride. `RecordEvolutionStatus`,
`MarkReplyDeliverySent`, `MarkReplyDeliveryFailure` e a confirmação de prompt
usam a mesma normalização/rank. `delivery_recorded_at` e `delivered_at`, uma
vez confirmados, são preservados; payload do provider não pode substituir os
campos reservados de delivery.

Uma mensagem `OUTBOUND` confirmadamente entregue e classificada `INVALID`
agora é barreira temporal. Active prompt e os scans de facts param nela; o
overlay canônico remove autoridade de availability anterior; o replay de
seleção parte somente depois da barreira; booking draft e todos os entrypoints
de `booking_create` permanecem fechados. Um candidato `UNDELIVERED` continua
invisível e não cria a barreira.

`DRAFT_REVIEW` só é elegível quando `mode=DRAFT_REVIEW` e
`review_action=APPROVED_AS_IS` existem nas duas cópias e são consistentes.
`EDITED`, `REJECTED`, vazio, desconhecido, ausência ou divergência resultam
`INVALID`, sem body, intent, template ou `tool_context` legado.

O P2 passou por `Service.Reply` com `Repository.CreateReply` e
`MarkReplyDeliverySent` reais em PostgreSQL 16.14. Um draft availability
`TEXT` válido recebeu metadata hostil `AUDIO`/base64/tool facts forjados. As
linhas reais de `chat_messages` e `outbound_messages` e o input observado pelo
sender permaneceram texto, com body, `draft_message_id`,
`review_action=APPROVED_AS_IS`, evento e facts do draft validado. O modo
obrigatório falha explicitamente sem URL; não há `SKIP` silencioso no gate.

RED real antes do patch:

```text
FAIL — DELIVERY_ACK seguido por SERVER_ACK tornou o active prompt UNKNOWN e perdeu facts
FAIL — INVALID entregue mais novo reviveu a lista legada anterior
FAIL — EDITED, action ausente/desconhecida e cópias divergentes foram classificados ABSENT_LEGACY
PASS controle — SERVER_ACK só ganhou autoridade após DELIVERY_ACK
PASS controle — INVALID não entregue permaneceu invisível
FAIL esperado do gate — CHAT_REQUIRE_PASSENGER_STATE_POSTGRES_TEST=1 sem URL encerrou o teste explicitamente
```

PASS local realmente executado:

```text
PASS — testes focados count=20 — 1.891s
PASS — go test -race -count=1 ./internal/chat — 26.423s
PASS — delivery/review/availability/passenger/booking/human/cancel/media — chat 0.674s; automation 0.008s
PASS SEM SKIP — PostgreSQL 16.14 obrigatório: Repository.CreateReply, sender, webhook monotônico, replay e concorrência — chat 1.245s; automation 0.151s
PASS — go test -count=1 ./internal/chat — 4.682s
PASS — go test -count=1 ./... — internal/chat 7.017s; automation 0.244s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt; gofmt -l sem saída
PASS — git diff --check
```

A primeira tentativa do race não chegou aos testes por cota de disco. Foi
limpo somente o `GOCACHE=/tmp/schumacher-h27-go-build` criado para esta tarefa;
a repetição e a execução final do mesmo gate passaram integralmente.

Não houve B2, parser, regex, migration, mudança de contrato público, commit,
push, deploy ou smoke. Não há declaração de review limpo nem segurança para
commit. A única próxima ação é um novo `/review` dirigido aos 3 P1 + 1 P2 de
H-2026-07-27A. H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

#### Correção após o quinto review — 2 P1 + 1 P2 de autoridade factual, barreira resolvida e datas canônicas

O review posterior preservou as correções anteriores e encontrou três lacunas
adicionais no patch local:

1. `availabilityPromptRawContextsV1` ignorava `tool_context` presente e
   não-mapa em uma das cópias, permitindo que a outra cópia isolada fosse
   tratada como `ABSENT_LEGACY` confiável;
2. `deliveredInvalidAvailabilityPromptBarrierAtV1` removia a barreira assim
   que o vínculo da projeção com o draft era resolvido, sem reclassificar a
   mensagem efetiva e detectar facts `INVALID`;
3. `parsePersistedAvailabilityFilterDateV1` aplicava `TrimSpace`, aceitando
   representações persistidas diferentes do formato canônico exato
   `YYYY-MM-DD`.

As regressões dirigidas foram adicionadas antes da correção de produção e
reproduziram os três defeitos no patch anterior usando `golang:1.23` em Docker:

```text
RED — 15 combinações de whitespace em trip_date/date_from/date_to foram aceitas como VALID_STRUCTURAL
RED — tool_context não-mapa em NormalizedPayload retornou factsPresent=true, factsValid=true com a cópia de Payload isolada
RED — a projeção DRAFT_REVIEW resolvida para facts INVALID removeu a barreira temporal
FAIL esperado — go test dirigido anterior ao patch, exit 1; internal/chat 0.016s
```

A correção é mínima e permanece nos três pontos revisados:

- `availabilityPromptRawContextsV1` preserva presença e marca `valid=false`
  quando `tool_context` existe, mas não é mapa;
- `deliveredInvalidAvailabilityPromptBarrierAtV1` reclassifica a mensagem
  efetiva resolvida e só remove a barreira para `ABSENT_LEGACY` ou
  `VALID_STRUCTURAL`; `INVALID` permanece terminal para o scan;
- `parsePersistedAvailabilityFilterDateV1` não normaliza o texto e exige que
  parse + format reproduzam exatamente `YYYY-MM-DD`.

Validação local realmente executada:

```text
PASS — testes dirigidos pós-patch, count=1 — internal/chat 0.028s
PASS — testes dirigidos pós-patch, count=20 — internal/chat 0.446s
PASS — go test -race -count=1 ./internal/chat — 51.081s
PASS — availability/passenger/booking/review/delivery/human/cancel — 6.436s
PASS — go test -count=1 ./internal/chat — 9.164s
PASS — go test -count=1 ./... — internal/chat 9.054s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt nos três arquivos tocados; gofmt -l internal/chat sem saída
PASS — git diff --check
PASS — git diff --cached --check
```

O host não possui `go` nem `gofmt`; os comandos Go e gofmt foram executados em
`golang:1.23`, com o repositório montado e caches temporários isolados. Docker
server 29.6.2 estava acessível; PostgreSQL não integra o escopo desta rodada.

Não houve B2, Travel V2, parser, regex, migration, provider, funcionalidade
nova, commit, push, PR, merge, deploy ou smoke. Não há declaração de review
limpo nem segurança para commit. A única próxima ação é um novo `/review`
dirigido aos 2 P1 + 1 P2 deste ciclo. H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.

#### Correção após o sexto review — 1 P1 + 1 P2 de domínio da barreira e data canônica nos resultados

O review seguinte preservou as correções anteriores e encontrou duas lacunas:

1. uma projeção `DRAFT_REVIEW/APPROVED_AS_IS` entregue podia resolver um draft
   apenas de passageiros com `tool_context` não-mapa e transformar a
   classificação factual `INVALID` em barreira de availability, apagando a
   autoridade `BOOKABLE` anterior;
2. `results[].trip_date` ainda era normalizado com `TrimSpace`, e o validador
   compartilhado também aceitava whitespace ao redor da data.

O RED dirigido foi registrado antes do patch de produção em `golang:1.23` via
Docker:

```text
RED — projeção entregue resolvida para ASK_PASSENGER_COUNT criou barreira de availability
RED — results[].trip_date aceitou whitespace bilateral, vazio, null, não-string, formato alternativo e data impossível no decode
FAIL esperado — testes dirigidos, exit 1; internal/chat 0.021s
```

A correção permaneceu limitada aos dois achados:

- `deliveredInvalidAvailabilityPromptBarrierAtV1` resolve a projeção e aplica
  à mensagem efetiva o mesmo predicado de candidatura/barreira de
  availability; `mode`, `APPROVED_AS_IS` e `tool_context` não-mapa isolados
  não provam domínio;
- `results[].trip_date` presente agora exige string exata `YYYY-MM-DD`, sem
  normalização, com parse + format idênticos; valor inválido torna o contexto
  factual `INVALID` e deixa `Presented=nil`;
- a fixture de validação que persistia `trip_date=""` passou a usar data
  futura canônica, sem alterar produção fora do contrato revisado.

As regressões provam no mesmo histórico: zero barreira para o prompt de
passageiros, `ActivePrompt=PASSENGER_COUNT`, option count zero na mensagem
efetiva inválida, bootstrap/read state `BOOKABLE` preservados, finders sem
publicar a mensagem como `ABSENT_LEGACY`, booking draft e `booking_create`
preservados. Para `trip_date` inválida, active prompt, finders, option count,
bootstrap, `AvailabilitySelectionStateV1`, booking draft e `booking_create`
falham fechado. Projeção de availability com facts `INVALID`,
`BOT_AUTO_REPLY` irresolvida, `UNDELIVERED`, projeção válida e precedência
humano/cancelamento permanecem cobertas.

Validação local realmente executada:

```text
PASS — testes dirigidos pós-patch, count=1 — internal/chat 0.042s
PASS — testes dirigidos pós-patch, count=20 — internal/chat 0.893s
RED intermediário do race — fixtures com trip_date vazio perderam o prompt; internal/chat 43.453s
PASS — regressão isolada da fixture canônica — internal/chat 0.008s
PASS — go test -race -count=1 ./internal/chat — 44.106s
PASS — availability/passenger/booking/review/delivery/human/cancel — 5.711s
PASS — go test -count=1 ./internal/chat — 8.121s
PASS — go test -count=1 ./... — internal/chat 8.256s; demais pacotes verdes
PASS — regexp.MustCompile de produção = 54
PASS — gofmt; gofmt -l internal/chat sem saída
PASS — git diff --check; git diff --cached --check
```

A primeira tentativa de runner não chegou aos testes porque a shell do
contêiner não expôs `gofmt` no `PATH`; a repetição usou os binários por caminho
absoluto. O host continua sem Go/gofmt. Não houve PostgreSQL, B2, 3.6F-D,
parser, regex, migration, provider, commit, push, PR, merge, deploy ou smoke.

Arquivos alterados nesta rodada:

```text
apps/api/internal/chat/availability_prompt_event_v1.go
apps/api/internal/chat/availability_prompt_event_v1_test.go
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/booking_create_router.go
apps/api/internal/chat/interpreter_validation_test.go
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
```

Resultado do review: os dois achados (1 P1 + 1 P2) foram corrigidos localmente, mas ainda não
há novo review limpo nem segurança para commit. Teste em produção continua
pendente do fluxo posterior autorizado; não foi executado nesta rodada. A
ausência de `results[].trip_date` mantém a semântica legada existente, enquanto
qualquer chave presente inválida falha fechado; opções estruturais apresentadas
continuam exigindo data. A única próxima ação é um novo `/review` dirigido a
estes dois achados. H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

#### Correção após o sétimo review — 1 P1 de domínio do fallback textual

O review seguinte preservou os achados anteriores e encontrou uma brecha P1:
o fallback textual genérico de `messageMayCarryAvailabilityPromptV1` tratava
como availability uma projeção `BOT_AUTO_REPLY` ou
`DRAFT_REVIEW/APPROVED_AS_IS` entregue e irresolvida que pertencia
explicitamente a pagamento ou a outro domínio. O texto de pagamento
`"Pode pagar no PIX ou no cartão. Qual opção você prefere?"` criava barreira,
ocultava o contexto anterior e reduzia `BOOKABLE` para `NONE`.

O RED final, adicionado antes do patch de produção, cobriu os dois modos de
projeção e cinco sinais estruturais já existentes: intent de pagamento,
template de passageiros, prompt kind documental, template humano e intent de
cancelamento. Os 10 subcasos falharam com `candidate=true`:

```text
RED — TestAvailabilityPromptTextFallbackRejectsExplicitNonAvailabilityDomains
FAIL esperado — internal/chat 0.008s
```

A correção ficou restrita ao fallback textual. Artefatos reais de availability
continuam sendo avaliados primeiro: evento, `availability_search`, intents e
templates canônicos, seleção/snapshot e marcador de autoridade não foram
alterados. Somente antes da inferência pelo body, metadata com intent, template,
`active_prompt_kind` ou evento de passageiros pertencente a outro domínio
retorna `candidate=false`. Não foi adicionado vocabulário, regex ou parser.

A matriz transversal prova `candidate=false`, zero barreira, finder anterior
preservado, bootstrap e `AvailabilitySelectionStateV1` em `BOOKABLE`, booking
draft e `booking_create` com a seleção anterior, metadata do domínio próprio
preservada e zero publicação como `ABSENT_LEGACY`. A lista legada real sem
metadata estrutural continua reconhecida pelo fallback; availability estrutural
mantém precedência mesmo diante de metadata conflitante. Projeção de
availability irresolvida, `INVALID`, `UNDELIVERED`, projeção válida,
`results[].trip_date` canônica e precedência humano/cancelamento permanecem
verdes.

Validação local realmente executada em `golang:1.23` via Docker, pois o host
não possui Go/gofmt:

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

Arquivos alterados nesta rodada:

```text
apps/api/internal/chat/availability_prompt_event_v1.go
apps/api/internal/chat/active_prompt_context_test.go
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
```

Resultado do review: o P1 atual foi corrigido localmente, mas ainda não há novo
review limpo nem segurança para commit. Teste em produção continua pendente do
fluxo posterior autorizado; não foi executado nesta rodada. Não houve B2,
3.6F-D, parser, regex, migration, provider, PostgreSQL, commit, push, PR,
merge, deploy ou smoke. A única próxima ação é um novo `/review` dirigido a
este P1. H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

#### Correção após o oitavo review — 2 P1 + 2 P2 de reconciliação de domínio e fallback legado

O review atual preservou as correções anteriores e encontrou quatro lacunas na
decisão de domínio de `messageMayCarryAvailabilityPromptV1`:

1. uma evidência positiva isolada em `Payload` era aceita antes de reconciliar
   `NormalizedPayload`; cópia conflitante, vazia, desconhecida ou não-string
   podia apagar uma autoridade `BOOKABLE` anterior;
2. o fallback textual ainda aceitava perguntas genéricas de pagamento,
   passageiros, documentos, suporte e cancelamento em projeções reais sem
   `intent/template_name` quando o draft não resolvia;
3. os templates `CONTEXT_FALLBACK_AVAILABILITY_OPTION/DATE` e os prompt kinds
   `AVAILABILITY_OPTION_CHOICE/DATE_CHOICE` não eram reconhecidos como sinais
   canônicos bilaterais;
4. o body real produzido por `buildEarliestAvailabilityReply` não era
   reconhecido pelo fallback legado.

O RED foi executado antes do patch de produção. Ele usou o shape real das
projeções persistidas, sem injetar metadata de domínio no outbound, e cobriu
`BOT_AUTO_REPLY` e `DRAFT_REVIEW/APPROVED_AS_IS` com draft ausente, duplicado,
posterior ou inválido para cinco domínios não-availability. Também reproduziu
conflitos entre as cópias, os quatro sinais canônicos omitidos e o EARLIEST
legado:

```text
RED — TestAvailabilityPromptDomainReconciliationV1
RED — TestAvailabilityPromptTextFallbackUsesRealProjectionShapeV1
RED — TestAvailabilityPromptLegacyBuilderFallbacksV1
FAIL esperado — internal/chat 0.020s
```

A correção classifica cada cópia como `AVAILABILITY`, `NON_AVAILABILITY`,
`ABSENT` ou `INVALID` e só decide depois da reconciliação. Evento, facts ou
seleção bilateral válida mantêm precedência estrutural. Sem essa precedência,
metadata precisa ser bilateral, reconhecida, internamente coerente e idêntica.
Metadata availability bilateral continua identificando o domínio quando os
facts são `INVALID`, de modo que a barreira fail-closed permanece; metadata de
outro domínio ou qualquer divergência não cria candidatura.

O fallback final agora aceita somente as formas reais dos builders: lista
numerada sequencial e resposta unitária EARLIEST. `response_realizer.go`
compartilha as constantes exatas desses bodies, e o option count renderizado
usa o mesmo reconhecedor estrito. Nenhum regex, parser geral ou vocabulário
novo foi adicionado.

Respostas informativas out-of-turn não viram authority de availability. A
continuidade do active prompt usa apenas `active_prompt_source_message_id` e
`active_prompt_kind` bilaterais já persistidos em `template_data`: o body do
lembrete permanece ativo, mas facts e option count são ancorados no prompt de
availability anterior, único, entregue e sem barreira intermediária.

As fixtures antigas que representavam writes canônicos passaram a persistir
`tool_context` ou seleção nas duas cópias, como `buildAgentDraftPayload` já faz
em produção. Isso preservou seleção por índice/data, out-of-turn payment,
booking draft e `booking_create` sem relaxar a regra bilateral.

Validação local realmente executada em `golang:1.23` via Docker:

```text
PASS — testes dirigidos e controles, count=20 — internal/chat 7.284s
PASS — go test -race -count=1 ./internal/chat — 53.380s
PASS — availability/payment/passenger/document/review/delivery/human/cancel — 7.144s
PASS — go test -count=1 ./internal/chat — 9.710s
PASS — go test -count=1 ./... — internal/chat 9.412s; demais pacotes verdes
PASS — inventário de produção em internal/chat = 54 regexp.MustCompile
PASS — gofmt -l internal/chat sem saída
OBSERVAÇÃO — gofmt -l . ainda lista somente arquivos preexistentes fora de internal/chat
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

Status: **EM CORREÇÃO APÓS REVIEW — 2 P1 + 2 P2 DE RECONCILIAÇÃO DE DOMÍNIO
E FALLBACK LEGADO**. Não há declaração de review limpo nem segurança para
commit. Teste em produção continua pendente do fluxo posterior autorizado e
não foi executado. Não houve B2, 3.6F-D, migration, provider, PostgreSQL,
commit, push, PR, merge, deploy ou smoke. A única próxima ação é: `/review`
dirigido aos 2 P1 + 2 P2. H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.

#### Correção após o nono review — 5 P1 + 1 P2 de resolução única de domínio, fonte e facts

O review seguinte encontrou seis lacunas remanescentes na reconciliação e na
continuidade de availability:

1. evento bilateral de availability podia prevalecer antes de detectar evento
   de passageiros coexistente, e facts válidos podiam mascarar outro artefato
   availability malformado;
2. readers legados ainda publicavam `availability_search` unilateral;
3. o fallback aceitava listas e EARLIEST adulterados que os builders não
   emitem;
4. a âncora out-of-turn validava a projeção bruta em vez da mensagem efetiva
   resolvida;
5. a fonte era procurada somente no prefixo do histórico, sem unicidade global
   nem causalidade pelo timestamp canônico;
6. materialização, booking draft e `booking_create` ainda podiam reler facts do
   lembrete em vez da fonte ancorada.

As regressões foram adicionadas e executadas antes do patch de produção. O RED
dirigido reproduziu: conflito availability + passenger aceito, facts válidos
mascarando seleção malformada, facts unilaterais publicados como
`ABSENT_LEGACY`, lista PIX/EARLIEST livre reconhecidos, projeção válida rejeitada
como fonte, pagamento com facts copiados aceito como âncora, duplicata global e
timestamp posterior ignorados e consumers lendo o `tool_context` do lembrete.

A correção extrai uma única reconciliação por mensagem efetiva. Antes de
conceder domínio, prompt ou facts, ela compara nas duas cópias:
`availability_prompt_event_v1`, `availability_search`, seleção/snapshot,
marcador de autoridade, eventos passenger/pending, intent, template e prompt
kind. Artefato unilateral, divergente, malformado ou conflito entre domínios
não publica prompt/facts; conflito com outro domínio não cria barreira de
availability, enquanto evidência availability inválida e sem domínio
concorrente permanece fail-closed.

`ABSENT_LEGACY` publica facts somente quando existem exatamente duas cópias
válidas e `DeepEqual`. O reconhecedor textual aceita somente a gramática
fechada produzida por `buildAvailabilityListReply`,
`buildAvailabilityListReplyForResultIndexes` e
`buildEarliestAvailabilityReply`: header/suffix exatos, linhas sequenciais,
rota, data, horário e preço nas formas emitidas. Não foi adicionado regex nem
parser geral.

`resolveAvailabilityPromptEffectiveSourceByIDV1` exige ID globalmente único,
fonte anterior no slice e causal pelo timestamp canônico. Projeções
`BOT_AUTO_REPLY` e `DRAFT_REVIEW/APPROVED_AS_IS` são resolvidas pelo índice e o
domínio/autoridade é validado na mensagem efetiva. Active prompt,
materialização, selection state, booking draft e `booking_create` usam somente
`Presented`, facts e option count dessa fonte. O lembrete fornece apenas body e
continuidade. Clarificações de índice persistem bilateralmente somente
`active_prompt_kind` e `active_prompt_source_message_id`, sem transformar o
lembrete em autoridade factual própria.

O reader legado de prompt source que ainda percorria `tool_context`
independentemente foi removido. A recuperação de seleção compara o snapshot
somente contra o contexto visível devolvido pela autoridade reconciliada e,
quando existe `availability_prompt_source_message_id` explícito, resolve
exatamente essa fonte em vez de procurar envelopes semelhantes.

RED/PASS reais executados em `golang:1.23` via Docker, pois o host não possui
Go/gofmt:

```text
RED — sete testes dirigidos, exit 1, com falhas nos seis mecanismos acima
PASS — sete testes dirigidos pós-patch, count=20 — internal/chat 0.381s
PASS — go test -race -count=1 ./internal/chat — 63.755s
PASS — H-012/document/lap-child/payment/human/out-of-turn — 3.608s
PASS — cancel/passenger/availability/review/delivery/booking — 8.801s
PASS — go test -count=1 ./internal/chat — 11.627s
PASS — go test -count=1 ./... — internal/chat 11.281s; demais pacotes verdes
PASS — inventário de produção em internal/chat = 54 regexp.MustCompile
PASS — gofmt -l internal/chat sem saída
PASS — git diff --check; git diff --cached --check
```

Arquivos alterados nesta rodada:

```text
apps/api/internal/chat/availability_prompt_event_v1.go
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/availability_selection_state_v1.go
apps/api/internal/chat/booking_create_router.go
apps/api/internal/chat/conversation_state_machine.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/interpreter.go
apps/api/internal/chat/interpreter_cases_eval.go
apps/api/internal/chat/service.go
apps/api/internal/chat/availability_prompt_event_v1_test.go
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/booking_create_router_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/interpreter_test.go
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
```

Status: **EM CORREÇÃO APÓS REVIEW — 5 P1 + 1 P2 DE RESOLUÇÃO ÚNICA DE
DOMÍNIO, FONTE E FACTS**. Não há declaração de review limpo nem segurança para
commit. Deploy e smoke continuam pendentes e não foram executados. Não houve
B2, 3.6F-D, migration, provider, PostgreSQL, regex, parser geral, commit, push,
PR ou merge. A única próxima ação é: `/review` dirigido aos 5 P1 + 1 P2.
H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

#### Correção após o décimo review — 3 P1 + 1 P2 de entrega temporal e proveniência

O review mais recente encontrou quatro lacunas: candidatura apagada em
reconciliação inválida com evidência availability, merge canônico aceitando
`INVALID`/`UNDELIVERED`, enrichment de snapshot pelas mesmas classes e uma
cópia stale do plano na raiz.

Três REDs foram executados antes do patch e reproduziram exatamente os três
mecanismos funcionais. A correção separa dados decodificados de autoridade:
`reconciled.Facts/Selection` não autorizam consumo; somente
`VALID_STRUCTURAL` e `ABSENT_LEGACY` entregue e confiável podem alimentar
estado ou enrichment. `INVALID` entregue preserva `Candidate=true`, forma
barreira e fornece zero autoridade. `UNDELIVERED` fornece zero autoridade e
não forma barreira somente pela ausência de entrega.

Durante a reconciliação da suíte, writers de passenger/payment/documento que
copiavam facts availability para projeções de outro domínio passaram a formar
barreira inválida no replay. A contenção remove esses artifacts das projeções
não-availability e mantém a autoridade no evento/estado materializado. A
continuidade out-of-turn passou a persistir bilateralmente apenas o link à
fonte; `active_prompt_kind` desse link não é domínio factual da mensagem. Se o
lembrete carregar facts/selection availability, ele continua candidato e
inválido.

Evidências locais em `golang:1.23` via Docker:

```text
RED — 3 testes dirigidos falharam nos mecanismos esperados antes do patch
PASS — 3 REDs pós-patch, count=20 — 0.398s
PASS — go test -race -count=1 ./internal/chat — 53.653s
PASS — suíte transversal completa — 7.292s
PASS — go test -count=1 ./internal/chat — 10.001s
PASS — go test -count=1 ./... — internal/chat 9.603s; demais pacotes verdes
PASS — gofmt -l internal/chat sem saída
PASS — inventário de produção = 54 regexp.MustCompile
PASS — plano stale ausente; plano canônico presente
PASS — git diff --check
```

Arquivos desta rodada corretiva: `availability_prompt_event_v1.go`,
`conversation_state_machine.go`, `availability_selection_state_v1.go`,
`service.go` e testes/fixtures de chat diretamente afetados; plano canônico,
tracker e handoff foram atualizados. A cópia stale da raiz deixou de existir.

Status: **EM CORREÇÃO APÓS REVIEW — 3 P1 + 1 P2 DE ENTREGA TEMPORAL E
PROVENIÊNCIA**. Resultado do review ainda é o finding de 3 P1 + 1 P2; os
achados estão corrigidos localmente, mas não há novo review limpo nem segurança
para commit. Teste em produção permanece necessário no fluxo posterior
autorizado e não foi executado. Não houve commit, push, deploy ou smoke. A
próxima ação única é `/review` dirigido a estes 3 P1 + 1 P2. H-2026-07-16B2 e
3.6F-D permanecem bloqueadas.

#### Correção após o décimo primeiro review — 2 P1 de body canônico e enrichment ABSENT_LEGACY

O review atual encontrou duas lacunas remanescentes: body canônico de
availability perdia candidatura ao coexistir com passenger/payment, e o
enrichment `ABSENT_LEGACY` descartava seleção bilateral confiável quando não
havia `availability_search`.

Os REDs reproduziram `Candidate=false` nos três conflitos canônicos e zero
candidatos para a seleção legada confiável. A correção preserva o body como
evidência positiva, converte o conflito entregue em candidata `INVALID` com
barreira e zero autoridade, e permite que somente o ramo `ABSENT_LEGACY`
comprovado use seu próprio `Selection.SelectedResult` reconciliado. `INVALID` e
`UNDELIVERED` continuam fechados; passenger/payment normais e linguagem genérica
não criam barreira. Uma reprodução adicional de source mismatch comprovou e
fechou a exigência de projection, selection message, prompt source e índice
exatos antes do enrichment.

Arquivos desta rodada: `internal/chat/availability_prompt_event_v1.go`, seus
testes, `internal/chat/availability_selection_state_v1.go`, seus testes, plano
canônico, tracker e handoff.

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

Status: **EM CORREÇÃO APÓS REVIEW — 2 P1 DE BODY CANÔNICO E ENRICHMENT
ABSENT_LEGACY**. O resultado observado ainda não é um review limpo e não
autoriza commit. Teste em produção permanece necessário no fluxo posterior e
não foi executado. Não houve commit, push, PR, deploy ou smoke. Próxima ação
única: novo `/review` dirigido aos dois P1. H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.

#### Correção após o décimo segundo review — 2 P1 + 1 P2 de provenance exclusiva, compatibilidade e status

O review atual confirmou o body canônico e encontrou dois desvios no
enrichment `ABSENT_LEGACY`: o prompt source estrutural ainda podia fornecer
campos ausentes, e snapshots com campos preenchidos conflitantes permaneciam
materializando autoridade. Também encontrou declarações canônicas concorrentes
no tracker/handoff.

Os REDs reproduziram o vazamento de route/package/currency pelo source e a
preservação de autoridade para conflitos em trip date, route, origin,
destination, package, price, currency, trip, board e alight. O patch agora:

- resolve uma única projeção exata;
- usa o prompt source somente para validar causalidade, índice e IDs;
- usa exclusivamente o `Selection.SelectedResult` reconciliado da projeção;
- compara todos os campos já preenchidos antes do merge;
- remove `MaterializesAuthority` diante de mismatch, impedindo `BOOKABLE` e
  `booking_create`;
- mantém `VALID_STRUCTURAL` e `INVALID`/`UNDELIVERED` nos gates anteriores.

Fixtures antigas que recuperavam package do prompt source foram alinhadas ao
contrato: package sobrevive apenas quando está no `SelectedResult`. A fila
canônica passou a ser a única declaração vigente; os estados antigos são
cronologia explicitamente superada.

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

O status vigente permanece exclusivamente na fila canônica: não há review
limpo nem segurança para commit. Teste em produção
continua pendente no fluxo posterior autorizado. Não houve B2, 3.6F-D, parser,
regex, migration, refactor amplo, commit, push, PR, deploy ou smoke. Próxima
ação única: novo `/review`; H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

#### Correção após o décimo terceiro review — 2 P1 + 1 P2 de source explícito, presença e gate canônico

O review confirmou todos os gates da rodada anterior e encontrou três pontos
restritos: source ID vazio/ausente ainda podia ser descoberto no histórico;
zero numérico e string vazia persistidos ainda eram tratados como ausência; e
`AGENTS.md` mantinha status operacional volátil concorrente com este tracker.

Os REDs reproduziram o fallback para prompt compatível e a sobrescrita de
`price=0`, `seats_available=0` e `package_name=""`. O patch agora:

- exige source ID não vazio no `SelectedResult` e igualdade exata com o evento;
- resolve apenas esse ID, com unicidade e causalidade, sem scan de fallback;
- preserva presença dos 20 campos durante decode e enrichment;
- preenche somente campo ausente e falha fechado para qualquer campo presente
  divergente, mantendo o valor persistido;
- impede `BOOKABLE` e `BookingCreateInput` em conflito;
- mantém o tracker como única fonte de status operacional atual.

Fixtures positivas que dependiam da inferência histórica foram alinhadas para
persistir o source explícito. A rodada anterior e suas próximas ações estão
**SUPERADAS** por esta correção e pela fila canônica acima.

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

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Não há review limpo nem
segurança para commit. Teste em produção continua pendente no fluxo posterior
autorizado. Não houve B2, 3.6F-D, parser, regex, migration, refactor amplo,
commit, push, PR, deploy ou smoke. Próxima ação única: novo `/review`;
H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

#### Correção após o décimo quarto review — 3 P1 + 1 P2 de barreira, tipos e serialização

O review confirmou os gates anteriores e reproduziu três lacunas restritas:
um source explícito anterior a um prompt entregue `INVALID` ainda podia
materializar a seleção; a máscara marcava presença antes de validar `null` ou
tipo; e a serialização live perdia os bits de `price=0` e
`seats_available=0`. O bloco antigo chamado “Decisão canônica” ainda mantinha
uma contagem operacional superada.

Os REDs reproduziram os três caminhos. O patch agora:

- consulta a barreira `INVALID` central ao resolver o source exato e rejeita
  qualquer source anterior à barreira aplicável;
- classifica os 20 campos do snapshot como string, inteiro ou float e só
  marca presença após validação estrita; `null`, tipo incorreto, float não
  integral em campo inteiro e número não finito falham fechados;
- transporta a máscara no evento materializado live e serializa somente as
  chaves presentes, preservando zero/vazio presente e omitindo chave ausente;
- marca a antiga decisão do tracker como histórica e superada, sem contagem
  volátil concorrente.

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

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Evidência local
verde não declara review limpo nem segurança para commit. Teste em produção
continua pendente no fluxo posterior autorizado. Não houve B2, 3.6F-D,
parser, regex, migration, refactor amplo, commit, push, PR, deploy ou smoke.
Próxima ação única: novo `/review`; H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.

#### Correção após o décimo quinto review — 1 P1 no limite causal da projeção

O review confirmou todos os gates anteriores e encontrou um único P1: no
bootstrap legado, o resolver de source usava o índice da selection tanto para
causalidade quanto para limitar a barreira. Assim, um `INVALID` entregue entre
selection e projection não era observado.

O RED reproduziu exatamente `[source, selection, INVALID, projection]` com
`MaterializesAuthority=true`, `BOOKABLE` e caminho de booking antes do patch.
A correção agora propaga explicitamente dois limites ao mesmo gate central:

- `sourceBeforeIndex` preserva a exigência de source anterior à selection;
- `materializationIndex` é o índice causal da projection e limita
  `latestDeliveredInvalidAvailabilityPromptIndexV1`;
- `INVALID` antes ou depois da selection, mas anterior à projection, bloqueia;
- `INVALID` posterior à projection não altera retroativamente o helper e é
  aplicado pelo reducer/barreira normal;
- `UNDELIVERED`, passenger, payment e document fora do domínio availability continuam
  sem criar barreira; source novo após barreira permanece válido.

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

H-2026-07-27A permanece **EM CORREÇÃO APÓS REVIEW**. Evidência local
verde não declara review limpo nem segurança para commit. Teste em produção
continua pendente no fluxo posterior autorizado. Não houve B2, 3.6F-D,
parser, regex, migration, refactor amplo, commit, push, PR, deploy ou smoke.
Próxima ação única: novo `/review`; H-2026-07-16B2 e 3.6F-D permanecem
bloqueadas.

### 8.13 Slice de segurança — coexistência Supabase HS256/JWKS e credencial administrativa (2026-08-10)

**Status:** EM CORREÇÃO APÓS REVIEW — 1 P1 + 1 P2 DE REDAÇÃO DE CREDENCIAL E
LIFECYCLE JWKS; correção local verde e aguardando novo `/review`. O gate
transversal permanece vermelho por quebra preexistente comprovada no
commit-base.

Este slice foi executado somente no worktree/branch dedicado
`sec/supabase-key-migration`, sem alterar a fila arquitetural, os status de
H-2026-07-27A, H-2026-07-16B2 ou 3.6F-D, infraestrutura, Supabase remoto ou
produção.

Comportamento local implementado:

- `SUPABASE_JWT_SECRET` e `SUPABASE_JWKS_URL` podem coexistir;
- somente `HS256` usa o secret simétrico e somente `ES256` usa JWKS; qualquer
  outro algoritmo falha fechado;
- falha inicial do JWKS não impede a validação de HS256 e inicia uma única
  recuperação concorrente segura, com timeout e backoff explícitos, capaz de
  habilitar ES256 sem restart quando o endpoint volta;
- `kid` desconhecido dispara refresh do JWKS com rate limit, sem tempestade de
  requests;
- issuer, audience, subject, expiração, compatibilidade temporária de issuer e
  service tokens permanecem no fluxo existente;
- `SUPABASE_SECRET_KEY` é a credencial administrativa preferida e
  `SUPABASE_SERVICE_ROLE_KEY` permanece como fallback;
- a credencial administrativa e sua proveniência são resolvidas uma vez no
  construtor do handler e reutilizadas no readiness e nos requests;
- a chave opaca nova usa somente `apikey`; o fallback JWT legado preserva
  `apikey` e `Authorization: Bearer`, sem mudança operacional no ambiente
  atual;
- requests administrativos falham fechado em qualquer redirect antes de
  reenviar `apikey` ou `Authorization`;
- erros administrativos de transporte, leitura ou resposta remota são
  redigidos antes de chegar a handlers/clientes, cobrindo secret nova, legacy
  e formas bearer sem remover status e diagnóstico sanitizado;
- `SUPABASE_SECRET_KEY` presente só é aceita no formato `sb_secret_...`;
  valor inválido não cai silenciosamente para o service role;
- `Close` aguarda `Keyfunc` ES256 já ativo, impede nova consulta JWKS depois do
  início efetivo do fechamento e só então encerra o background; HS256 não
  depende desse lifecycle.

Arquivos alterados:

```text
apps/api/.env.example
apps/api/cmd/api/main.go
apps/api/internal/auth/middleware.go
apps/api/internal/auth/middleware_test.go
apps/api/internal/shared/config/config.go
apps/api/internal/shared/config/config_test.go
apps/api/internal/users/handler.go
apps/api/internal/users/handler_test.go
docs/EXECUTION_TRACKER.md
```

Testes e gates executados:

```text
PASS — go test -count=1 ./internal/auth ./internal/shared/config ./internal/users
PASS — go test -race -count=1 ./internal/auth ./internal/shared/config ./internal/users
PASS — pacotes isolados auth/config/users também ficaram verdes durante go test -count=1 ./...
PASS — gofmt aplicado aos arquivos Go alterados
PASS — git diff --check
FAIL BASELINE — go test -count=1 ./internal/chat ./internal/automation ./cmd/api
FAIL BASELINE — go test -count=1 ./...
```

O erro transversal é anterior ao slice e foi reproduzido em clone limpo de
`HEAD` (`1533596`): `internal/chat` referencia
`AvailabilityPromptPresentationV1`, `classifyAvailabilityPromptCandidateV1` e
outros símbolos ausentes da árvore versionada. O mesmo erro impede a
compilação de `internal/automation` e `cmd/api`. A correção de chat não foi
incluída para não misturar responsabilidades nem violar o escopo deste slice.

O primeiro review dirigido encontrou 4 P1 e 1 P2:

- indisponibilidade ES256 permanente após falha inicial do JWKS;
- janela de até uma hora para rotação com `kid` novo;
- allowlist assimétrica mais ampla que o contrato ES256;
- possível reenvio de `apikey` em redirect cross-origin;
- aceitação silenciosa de valor incompatível em `SUPABASE_SECRET_KEY`.

A correção local adiciona ciclo único de recuperação com mutex, cancelamento,
timeout de 10 segundos e backoff de 5 segundos até o teto de 1 minuto. O
`keyfunc` v1.9.0 permanece inalterado e agora usa `RefreshUnknownKID` com rate
limit de 5 segundos. `Authenticator.Close` encerra o retry e o background do
JWKS; `cmd/api` registra esse encerramento.

A política de algoritmo compara as instâncias exatas de `HS256` e `ES256`.
HS384/512, ES384/512, RS*, PS*, EdDSA, `none` e método desconhecido são
rejeitados antes da escolha da chave. Um ES384 corretamente assinado por uma
chave publicada no JWKS também é rejeitado.

O cliente HTTP administrativo possui `CheckRedirect` fail-closed para qualquer
3xx. A resolução da credencial reutiliza validação sanitizada de
`SUPABASE_SECRET_KEY`; valor presente e inválido bloqueia readiness e request,
mesmo quando o service role legado também existe. Erros não contêm o valor da
credencial.

RED antes da correção:

```text
FAIL de compilação dirigido — recuperação/configuração runtime JWKS e allowlist exata ainda inexistentes
FAIL conceitual confirmado pelo review — startup 503 nunca recuperava ES256
FAIL conceitual confirmado pelo review — kid novo aguardava refresh de até 1 hora
FAIL conceitual confirmado pelo review — redirects podiam reenviar apikey
FAIL conceitual confirmado pelo review — secret inválida vencia legacy silenciosamente
```

PASS local após a correção:

```text
PASS — go test -count=1 ./internal/auth ./internal/shared/config ./internal/users — auth 0.229s; config 0.003s; users 0.005s
PASS — adversariais count=20, inclusive lifecycle do retry/Close — auth 4.332s; config 0.003s; users 0.025s
PASS — go test -race -count=1 ./internal/auth ./internal/shared/config ./internal/users — auth 1.296s; config 1.021s; users 1.029s
PASS — gofmt; gofmt -l sem saída
PASS — git diff --check; git diff --cached --check
FAIL BASELINE PREEXISTENTE — go test -count=1 ./... — mesmos símbolos ausentes de chat em HEAD limpo; auth/config/users verdes
```

Resultado do review: os 4 P1 + 1 P2 possuem correção local e regressão
dirigida, mas o novo review ainda não foi executado; não há declaração de
review limpo nem segurança para commit.

O segundo review dirigido encontrou 1 P1 + 1 P2 restantes:

- body remoto não-2xx podia refletir `apikey`/bearer e alcançar
  `err.Error()`, o body dos handlers e eventuais logs do fluxo;
- `Close` podia executar `EndBackground` enquanto uma `Keyfunc` de `kid`
  desconhecido ainda aguardava o worker de refresh.

A correção redige todas as credenciais administrativas configuradas antes de
propagar erros de readiness, marshal, criação do request, transporte, leitura
do body ou resposta remota. Valor puro, `Bearer <credencial>`, bearer em caixa
baixa, texto misto e campo JSON são substituídos por `[REDACTED]`. O erro
preserva o status HTTP e a parte não sensível do diagnóstico. Esse fluxo não
produz logs próprios com body ou erro remoto.

Para o lifecycle, o `RLock` permanece adquirido durante toda a chamada
`jwks.Keyfunc`. `Close` solicita o writer lock, o que bloqueia novos readers,
aguarda validações iniciadas, marca o autenticador como fechado e só depois
chama `EndBackground`. Nova validação ES256 falha fechado; HS256 permanece
independente. Retry, double-close e encerramento continuam cobertos pelo
`closeOnce`, cancelamento e wait group existentes.

RED real antes da correção:

```text
FAIL — Close retornou enquanto Keyfunc de kid desconhecido ainda estava ativa
FAIL — 8/8 reflexões secret/legacy (pura, bearer, texto misto e JSON) apareceram em err.Error()
```

PASS local após a correção:

```text
PASS — dirigido P1/P2 — auth 0.029s; users 0.005s
PASS — go test -count=1 ./internal/auth ./internal/shared/config ./internal/users — auth 0.253s; config 0.003s; users 0.007s
PASS — adversariais count=20 — auth 4.817s; config 0.003s; users 0.078s
PASS — go test -race -count=1 ./internal/auth ./internal/shared/config ./internal/users — auth 1.302s; config 1.011s; users 1.031s
PASS — gofmt; gofmt -l sem saída
PASS — git diff --check; git diff --cached --check
FAIL BASELINE PREEXISTENTE — go test -count=1 ./... — mesmos símbolos ausentes de chat em HEAD limpo; auth/config/users verdes
```

Resultado do review atual: o 1 P1 + 1 P2 possuem correção local e regressão
dirigida, mas o próximo review ainda não foi executado; não há declaração de
review limpo nem segurança para commit.

Necessidade de teste em produção: sim, após review limpo e autorização futura
de publicação, com smoke separado para token HS256 legado, token ES256/JWKS e
request administrativo via `SUPABASE_SECRET_KEY`. Nenhum smoke foi executado
nesta rodada.

Não houve commit, push, PR, deploy, migration ou alteração de secret/ambiente.
A próxima ação única deste slice é `/review` dirigido ao 1 P1 + 1 P2 de
redação de credencial administrativa e lifecycle JWKS.

---

## H-2026-07-16A — Travel V2 Shadow operacional

**Status:** CONCLUÍDO

### Causa raiz comprovada

O repository serializava os payloads JSON do claim, completion e recovery
como `[]byte`. Em produção, o pool usa `pgx.QueryExecModeExec`, fazendo os
parâmetros serem inferidos como binários antes do cast `$n::jsonb`.

O PostgreSQL retornava SQLSTATE `22P02` (`invalid_text_representation`) nos
caminhos de claim e recovery.

A correção passou os payloads JSON como texto, preservando os casts `::jsonb`,
e adicionou cobertura PostgreSQL usando o mesmo QueryExecMode da produção.

### Smoke operacional

Em 2026-07-17, mensagens reais produziram:

- scheduler `scheduled`;
- job `started`;
- claim `claim_acquired`;
- provider `provider_started` e `provider_completed`;
- completion `completion_completed`;
- claims persistidos como `COMPLETED`;
- `travel_query_v2_shadow_recovery_due_at = NULL`;
- recovery `sweep_done 0/0/0`;
- nenhuma nova ocorrência de `sweep_failed`;
- nenhum efeito adicional user-visible causado pelo shadow.

Foram verificados 8 claims reais, todos terminalizados em `COMPLETED`.

Um request atingiu o timeout de 10 segundos, mas foi persistido como erro
terminal e não deixou claim órfão. O acompanhamento de latência fica fora
do escopo deste hotfix.

### Próximo gate

H-2026-07-16B — estado de passageiro/criança.
3.6F-D continua bloqueado até a conclusão de H-B.

## 9. Fases posteriores condicionais

### Retrieval

Vector/File Search permanece `FUTURO CONDICIONAL`. Só pode ser proposto depois que contrato, validator, shadow, corpus e métricas V2 demonstrarem que o problema residual é falta de exemplos, e não falta de contrato ou validação factual.

Retrieval nunca substitui facts operacionais e não pode conter dados sensíveis reais.

### Promoção primary

A promoção do OpenAI Travel Interpreter V2 para primary exige:

```text
corpus V2 estável
shadow V2 estável
reject reasons observáveis
fallback testado
custo e latência aceitáveis
zero side effect direto
rollback imediato
```

### Planner

O planner permanece futuro. Não iniciar antes de interpretação V2, validator V2, state canônico e ações seguras estarem estabilizados.

---

## 10. Track independente SEC-2026-08-18 — Data API / grants / RLS

**Status:** **LOTE 1 APLICADO E OPERACIONALMENTE CONCLUÍDO; PÓS-CHECK E
SMOKES VERDES; REVIEW FINAL SEM P0/P1/P2; NENHUM SUCESSOR AUTORIZADO.**

**Próxima ação operacional:** nenhuma autorizada. O Lote 2A depende de novo
`/goal` e autorização explícita; este fechamento não o autoriza.

Plano: `plans/sec-2026-08-18-data-api-rls-hardening.md`.

Este track de segurança é paralelo e não altera a fila, os status ou a próxima
ação de H-2026-07-27A, H-2026-07-16B ou 3.6F.

Inventário read-only reconciliado:

- 29 tabelas `public`, zero policies e zero tabelas em publication;
- `roles` e `pagarme_webhook_events` já endurecidas;
- 15 routines `public`, todas SECURITY INVOKER, com EXECUTE atual para
  PUBLIC/anon/authenticated/postgres/service_role;
- nove routines catalogalmente elegíveis como RPC;
- uma sequence, `sheet_sync_queue_id_seq`;
- default ACLs de tables, functions e sequences reexpõem objetos futuros.

Classificação vigente:

- tabelas: 25 BACKEND_ONLY e quatro UNKNOWN_BLOCKED;
- routines: uma BACKEND_ONLY, seis TRIGGER_INTERNAL e oito UNKNOWN_BLOCKED;
- sequence atual: UNKNOWN_BLOCKED;
- nenhuma tabela ou routine DATA_API_REQUIRED comprovada.

Bloqueios explícitos:

- consumidor, protocolo e DDL implantado de sheet sync não comprovados;
- duas routines de refresh de manifesto sem caller comprovado;
- mecanismo de geração de `sheet_sync_queue.id` não reconciliado;
- prova PostgreSQL efêmera pendente antes de revogar EXECUTE de trigger
  functions.

Correções incorporadas ao commit documental e validadas pelo review final:

- P1: o pós-check de produção do Lote 2B exclui explicitamente qualquer
  write pelo fluxo `trips`. A prova positiva do consumidor backend continua
  obrigatória, mas deve usar caminho versionado previamente comprovado sem
  escrita nas tabelas-fonte de sheet sync, com verificação read-only do
  resultado; se não houver caminho seguro, a prova permanece bloqueada. DML
  em `trips` para essa prova fica restrito a PostgreSQL efêmero/de teste;
- P1 anterior: o gate literal
  `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` bloqueia
  smokes em produção que provoquem DML deliberado em `trips`, `bookings`,
  `passengers` ou `booking_payment_details` apenas para provar trigger/enqueue.
  Essas provas ficam restritas a PostgreSQL efêmero/de teste; lotes 9, 11, 12,
  13 e seus dependentes 10, 14 e 15 não podem contornar o guard; o Lote 2B
  também está explicitamente submetido ao gate;
- P2: o DoD garante redução da autoridade Data API
  somente para anon/authenticated. A chave antiga do incidente permanece
  rotacionada/revogada como controle histórico separado. `postgres` e
  `service_role` continuam privilegiados/com `BYPASSRLS`; o comprometimento
  de uma credencial válida dessas roles permanece risco residual aceito e
  backlog separado.

Nenhum `/goal` pode conter mais de um lote ou sublote. Nenhum sucessor é
liberado automaticamente. A futura execução exige autorização explícita,
pré-check SQL, pós-check, smokes Data API negativos separados, smoke positivo
do consumidor, smoke da API quando aplicável e review limpo.

Arquivos desta materialização:

```text
plans/sec-2026-08-18-data-api-rls-hardening.md
docs/EXECUTION_TRACKER.md
docs/SESSION_HANDOFF.md
```

Fechamento da materialização documental anterior:

- review final: sem P0/P1/P2;
- commit: `b8bfe9e9afd44ed5e1faf5aa1f5ee0653cd3de61`;
- working tree: limpo imediatamente após o commit;
- mudança funcional: nenhuma;
- naquela rodada, migration, SQL, banco, deploy e smoke não foram executados;
- lote SQL iniciado ou autorizado: nenhum.

Validação desta reconciliação pós-commit: `git diff --check` PASS; working tree
limitado a `docs/EXECUTION_TRACKER.md` e `docs/SESSION_HANDOFF.md`; nenhum
teste de aplicação ou produção executado, pois a mudança é somente documental.

### Execução local do Lote 1 — default ACL de TABLE (2026-08-19)

Baseline confirmada antes da alteração:

```text
branch: sec/data-api-rls-hardening
HEAD: 2385527bd0423d7eff356bc96f4d6612ac739e0c
working tree: limpo
```

Arquivo criado:

```text
apps/api/migrations/0022_harden_public_table_default_privileges.sql
```

A migration revoga dos default privileges de `postgres` em `public`, somente
para futuras TABLES, os privilégios `SELECT`, `INSERT`, `UPDATE`, `DELETE`,
`TRUNCATE`, `REFERENCES` e `TRIGGER` de anon/authenticated. Não altera tabelas
existentes, RLS, policies, FORCE, functions, sequences, postgres ou
service_role. Nenhum objeto UNKNOWN_BLOCKED foi alterado.

Validação em PostgreSQL 16 efêmero:

```text
RED -> PASS: tabela criada antes da migration recebeu ALL7 para anon/authenticated
PASS -> pg_default_acl sem ALL7 de TABLE para anon/authenticated
PASS -> nova tabela sem ALL7 para anon/authenticated
PASS -> postgres/service_role preservados
PASS -> tabela preexistente manteve ACL, RLS e policy
PASS -> default ACLs e objetos novos de FUNCTION/SEQUENCE preservados
ROLLBACK -> PASS: conjunto original de default privileges restaurado exatamente
git diff --check -> PASS
```

O rollback validado, somente no banco efêmero, é:

```sql
alter default privileges for role postgres in schema public
  grant select, insert, update, delete, truncate, references, trigger
  on tables to anon;

alter default privileges for role postgres in schema public
  grant select, insert, update, delete, truncate, references, trigger
  on tables to authenticated;
```

Ele restaura exatamente os sete default privileges de TABLE. O rollback não
foi executado em produção.

### Reconciliação pós-review/pós-merge do Lote 1 (2026-08-19)

Checkpoint vigente:

```text
review final: sem P0/P1/P2
working tree seguro para commit: SIM
Lote 1 seguro para pré-check/aplicação em produção: SIM
PR: #69 — mergeada em main
head do PR: b7bd633108f0477b4bc5285e74d7c07c0502fb65
merge commit: dfcfac5b4a7fead7ef2dd3575948fb7a7c50fb6c
aplicação em produção: NÃO
deploy/smoke: NÃO
sucessor autorizado: NÃO
```

A migration `0022_harden_public_table_default_privileges.sql` está integrada
na `main` via PR #69. O próximo gate operacional é somente o pré-check
READ-ONLY no banco real definido pelo plano canônico. A aplicação da migration
é uma rodada separada: depende do pré-check verde e de autorização explícita
posterior. Este registro não autoriza SQL, deploy, smoke nem Lote 2A.

O P2 documental da reconciliação pós-merge foi corrigido e validado por review
sem P0/P1/P2. O working tree documental foi declarado seguro para commit.

A única próxima ação operacional do Lote 1 permanece o pré-check READ-ONLY no
banco real. A migration 0022 ainda não foi aplicada em produção. Qualquer SQL
mutável continua condicionado a pré-check verde e nova autorização explícita.
Nenhum sucessor, deploy ou smoke está autorizado.

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

O pré-check READ-ONLY em produção passou em PostgreSQL 15.8 e confirmou o
inventário esperado: 29 tabelas, 15 routines, uma sequence, zero policies e
zero objetos em publication.

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

O track funcional permanece inalterado.
