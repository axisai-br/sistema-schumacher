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
| 7 | H-2026-07-16A | **EM VALIDAÇÃO OPERACIONAL — PATCH LOCAL VERDE** | `plans/h-2026-07-16a-travel-v2-shadow-operacional.md` | SQLSTATE 22P02 atribuído à codificação `[]byte` dos parâmetros `::jsonb`; patch textual aguarda novo smoke. |
| 8 | H-2026-07-16B | **PENDENTE após H-2026-07-16A** | `plans/h-2026-07-16b-passenger-child-state.md` | corrigir contagem de passageiros e loop de criança menor de 5. |
| 9 | 3.6F-D | **BLOQUEADA por H-2026-07-16B** | `plans/3.6f-d-corpus-evaluator-v2.md` | corpus/evaluator V2 reproduzíveis após os hotfixes. |
| 10 | 3.6F-E | **PENDENTE após 3.6F-D** | `plans/3.6f-e-observabilidade-v2.md` | métricas V2 sanitizadas e read-only. |
| 11 | 3.6F-F | **PENDENTE** | `plans/3.6f-f-templates-seguros.md` | templates seguros. |
| 12 | 3.6F-G | **PENDENTE** | `plans/3.6f-g-earliest-available.md` | `EARLIEST_AVAILABLE` read-only. |
| 13 | 3.6F-H | **PENDENTE** | `plans/3.6f-h-route-coverage.md` | cobertura de rota read-only. |
| 14 | 3.6F-I | **PENDENTE** | `plans/3.6f-i-arbitragem-runtime-weak.md` | arbitragem gated sobre `WEAK`/`FALLBACK`. |

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
próxima ação única: executar H-2026-07-16A; H-2026-07-16B e 3.6F-D permanecem bloqueados/pendentes
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
próxima ação única: revisar e, por fluxo autorizado, implantar o patch 22P02 e executar o smoke sanitizado; H-2026-07-16B e 3.6F-D permanecem bloqueados
```

### 8.10 Bug user-visible — H-2026-07-16B (2026-07-16)

**Status:** **PENDENTE após H-2026-07-16A**.

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
