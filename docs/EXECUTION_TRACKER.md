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
| 2 | P0-B | **CONCLUÍDA** | `plans/p0-b-corrigir-fixtures-temporais.md` | fixtures temporais estabilizadas e review final concluído sem P1/P2. |
| 3 | P0-C | **CONCLUÍDA — GATE REMOTO VALIDADO** | `plans/p0-c-ci-test-gate.md` | `publish-api` depende de `test-api`; execução remota concluída com sucesso. |
| 4 | 3.6F-A | **EM REVIEW — CONTRATO LOCAL VALIDADO** | `plans/3.6f-a-contrato-travel-query-meaning-v2.md` | Criar `TravelQueryMeaningV2` e tipos fechados, sem runtime. |
| 5 | 3.6F-B | **BLOQUEADA por 3.6F-A** | `plans/3.6f-b-validator-v2.md` | Criar `ValidateTravelQueryMeaningV2` puro por invariantes e evidências. |
| 6 | 3.6F-C | **PENDENTE** | `plans/3.6f-c-openai-v2-shadow.md` | Produzir e validar V2 em shadow, sem efeito user-visible. |
| 7 | 3.6F-D | **PENDENTE** | `plans/3.6f-d-corpus-evaluator-v2.md` | Versionar corpus e evaluator V2 reproduzíveis. |
| 8 | 3.6F-E | **PENDENTE** | `plans/3.6f-e-observabilidade-v2.md` | Expor métricas V2 sanitizadas e read-only. |
| 9 | 3.6F-F | **PENDENTE** | `plans/3.6f-f-templates-seguros.md` | Liberar somente templates seguros para poltrona, institucional e acknowledgement. |
| 10 | 3.6F-G | **PENDENTE** | `plans/3.6f-g-earliest-available.md` | Consultar `EARLIEST_AVAILABLE` read-only e exigir confirmação. |
| 11 | 3.6F-H | **PENDENTE** | `plans/3.6f-h-route-coverage.md` | Consultar cobertura de rota read-only sem inventar proximidade. |
| 12 | 3.6F-I | **PENDENTE** | `plans/3.6f-i-arbitragem-runtime-weak.md` | Permitir arbitragem gated somente sobre decisões `WEAK`/`FALLBACK`. |

### Regra de desbloqueio

Um predecessor só libera o sucessor quando:

```text
escopo concluído
testes exigidos executados
review sem P1/P2 pendente
tracker atualizado
```

A liberação altera somente o próximo status; não autoriza executar dois slices no mesmo `/goal` ou PR.

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
próxima ação única: executar o /review do 3.6F-A; não iniciar 3.6F-B.
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
próxima ação única: executar o /review do 3.6F-A; não iniciar 3.6F-B
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
próxima ação única: executar o /review do 3.6F-A; não iniciar 3.6F-B
```

### 8.6 Registro operacional — 3.6F-A (2026-07-14)

**Status:** **EM REVIEW — CONTRATO LOCAL VALIDADO**.

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

#### Fechamento provisório

```text
comportamento antes: não existia contrato V2 capaz de representar papéis de localidade, data relativa, referência de opção, cobertura, poltrona e tema institucional
comportamento depois: existe contrato Go local e puro para essas semânticas, sem consumo pelo fluxo atual
mudança funcional de runtime: nenhuma
resultado do review: revisão local sem achados P1/P2; review canônico do 3.6F-A permanece pendente
teste em produção: não necessário; o contrato não está integrado ao runtime
riscos restantes: enums e shapes ainda dependem do validator puro planejado para o 3.6F-B, que permanece bloqueado
próxima ação única: executar o /review do 3.6F-A; não iniciar 3.6F-B
```

---

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
