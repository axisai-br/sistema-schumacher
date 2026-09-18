# Plano mestre — Travel Semantic Interpretation V2

## Identificação

- Repositório: `joaovitormessias/sistema-schumacher`
- Base de referência: confirmar `HEAD`/`origin/main` no início de cada sessão; última reconciliação operacional em 2026-07-16.
- Tracker canônico: `docs/EXECUTION_TRACKER.md`
- Fase: `3.6F — Travel Semantic Interpretation V2`
- Regra operacional: executar somente o primeiro slice marcado como `PRÓXIMA` no tracker.

## Objetivo

Corrigir estruturalmente os erros observados em produção sem transformar o backend em um catálogo crescente de regex.

Arquitetura alvo:

```text
mensagem atual
+ estado canônico
+ active prompt
+ facts atuais
→ fast-path/guardrails locais
→ OpenAI Travel Interpreter V2 em structured output
→ validator local por invariantes
→ decisão segura
→ template fechado ou tool read-only
→ backend executa
```

A OpenAI interpreta linguagem. Ela não executa tools, não cria reservas, não gera pagamentos e não fornece fatos operacionais como fonte de verdade.

## Problemas que esta fase cobre

1. `saindo de Seara` interpretado como destino.
2. `a que tiver mais perto` tratado como data inválida.
3. `passa em Santa Cecília?` tratado como pacote não suportado.
4. `passa perto de Lebon Régis?` sem consulta de cobertura/proximidade.
5. `esta opção`, `essa aí` e referências dêiticas frágeis.
6. `1` não selecionando opção atual confiável.
7. broad state vencendo a intenção explícita do turno atual.
8. perguntas institucionais repetindo tabela pública.
9. `meu Deus`, `obrigado`, `entendi` acionando fallback incorreto.
10. intenção de poltrona específica sem suporte e sem continuação da reserva.
11. shadow V2 habilitado sem criação de claims e recovery emitindo `sweep_failed` sem causa observável.
12. `eu e meus 2 filhos` contado como 2, não 3, e resposta `sim, o mais novo tem 4 anos` repetindo `ASK_CHILD_UNDER_5`.

## Decisões arquiteturais

### V2 paralela

Não modificar diretamente o `StructuredInterpretation` V1.

Criar:

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

O V1 continua em produção. O V2 entra em shadow, ganha corpus e observabilidade e somente depois pode influenciar runtime.

### Força da decisão

Adicionar conceito tipado:

```go
type DecisionStrength string

const (
    DecisionStrengthStrong   DecisionStrength = "STRONG"
    DecisionStrengthWeak     DecisionStrength = "WEAK"
    DecisionStrengthFallback DecisionStrength = "FALLBACK"
)
```

A OpenAI V2 poderá arbitrar apenas decisões `WEAK` ou `FALLBACK`.

Nunca poderá sobrescrever:

- cancelamento explícito;
- pedido explícito de humano;
- mídia/documento no fluxo documental;
- índice válido em lista atual;
- guardrails de pagamento;
- tools críticas;
- bloqueios de segurança.

### Semântica da poltrona

```go
type SeatRequestMode string

const (
    SeatRequestNone               SeatRequestMode = "NONE"
    SeatRequestBookTravel         SeatRequestMode = "BOOK_TRAVEL"
    SeatRequestChooseSpecificSeat SeatRequestMode = "CHOOSE_SPECIFIC_SEAT"
)
```

Regras:

```text
"quero reservar uma passagem"
→ BOOK_TRAVEL

"quero escolher o assento"
"quero a poltrona 12"
"quero uma poltrona específica"
→ CHOOSE_SPECIFIC_SEAT
```

Para `CHOOSE_SPECIFIC_SEAT`:

```text
Para escolher ou reservar uma poltrona específica, fale com o suporte da Schumacher Tur: +55 49 9886-2222.
```

A resposta também deve oferecer continuação da reserva normal de acordo com o contexto atual.

Não criar handoff real, não selecionar viagem e não chamar `booking_create`.

### Preferência temporal

```go
type DateMode string

const (
    DateModeUnspecified       DateMode = "UNSPECIFIED"
    DateModeExact             DateMode = "EXACT"
    DateModeEarliestAvailable DateMode = "EARLIEST_AVAILABLE"
    DateModeAnyAvailable      DateMode = "ANY_AVAILABLE"
)
```

No primeiro rollout:

```text
EARLIEST_AVAILABLE
→ buscar viagens futuras
→ mostrar a primeira opção
→ pedir confirmação
```

Não avançar automaticamente para passageiros.

### Cobertura de rota

A consulta read-only deve distinguir:

```go
type RouteCoverageResult struct {
    QueryLocation         string
    MatchMode             RouteCoverageMode
    StopExists            bool
    HasActiveFutureTrip   bool
    CoversRequestedRoute  bool
    Stops                 []RouteCoverageStop
}
```

Existir no catálogo não prova cobertura na rota, direção ou data solicitada.

Não afirmar “ponto mais próximo” sem dados geográficos e regra operacional confiáveis.

## Ordem canônica de execução

```text
P0-A  Reconciliar SHA, flags e smoke do H-012
P0-B  Corrigir fixtures temporais
P0-C  Adicionar gate de testes ao CI

3.6F-A  Contrato local TravelQueryMeaningV2
3.6F-B  Validator V2
3.6F-C  OpenAI V2 em shadow

H-2026-07-16A  Corrigir ausência de claims e observabilidade do recovery V2
H-2026-07-16B  Umbrella do estado de passageiros
H-2026-07-16B1 Fundação de autoridade, eventos, serialização e fail-closed
H-2026-07-22A  Corrigir gate global de passageiros em sessões novas
H-2026-07-16B2 PassengerClarificationMeaningV1 strict, corpus e shadow
H-2026-07-16B3 Promoção runtime gated do meaning de passageiros

3.6F-D  Corpus e evaluator V2
3.6F-E  Observabilidade V2
3.6F-F  Templates seguros e semântica não operacional
3.6F-G  EARLIEST_AVAILABLE read-only
3.6F-H  route_coverage_lookup read-only
3.6F-I  Arbitragem runtime sobre decisões WEAK/FALLBACK
```

Vector retrieval permanece futuro condicional. Só iniciar se o shadow V2 provar que o problema restante é falta de exemplos, e não contrato ou validator.

## Invariantes globais

Nenhum slice pode quebrar:

```text
1 inbound → no máximo 1 resposta final
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

## Métricas para promoção

Antes de qualquer promoção runtime:

```text
schema_parse_success_rate
validator_acceptance_rate
intent_accuracy
location_role_accuracy
date_mode_accuracy
option_reference_accuracy
clarification_accuracy
false_override_count
critical_action_violation_count
latency_p95
cost_per_turn
fallback_rate
```

Gate absoluto:

```text
critical_action_violation_count = 0
```

Para `PassengerClarificationMeaningV1`, o protocolo especializado de promoção,
incluindo matriz, denominadores, thresholds, incidentes, ausência de erro
recorrente e rollback OFF/ON, está definido em
`plans/h-2026-07-16b2-passenger-meaning-v1.md`, seção **Protocolo do gate de
promoção — FIXED_UNREVIEWED**. O protocolo está formalizado e corrigido, mas
aguarda review documental independente e não autoriza coleta. Somente um review
sem P0/P1/P2 poderá aprová-lo para execução; mesmo depois disso, H-B3 permanece
bloqueada até o `PASS` completo da amostra confirmatória e do rollback proof. O
tracker concentra o estado vigente e volátil de execução; este plano mestre não
o duplica.

## Uso com o Codex

1. Ler `AGENTS.md`.
2. Ler `docs/EXECUTION_TRACKER.md`.
3. Confirmar HEAD.
4. Abrir apenas o arquivo do slice atual.
5. Usar o `/goal` contido no arquivo.
6. Implementar somente o slice.
7. Rodar testes.
8. Usar o `/review` contido no arquivo.
9. Corrigir P1/P2.
10. Atualizar `docs/EXECUTION_TRACKER.md`.
11. Não avançar automaticamente para o slice seguinte.


## Gate operacional adicionado em 2026-07-16

O review local do 3.6F-C ficou limpo, mas o smoke implantado revelou:

```text
flag V2=true no processo da API
migration 0021, índice e permissões presentes
query completa de recovery executada manualmente com sucesso
query manual de claim executada com sucesso e rollback
nenhum travel_query_v2_shadow_claims criado por mensagens reais
recovery emitindo sweep_failed a cada ciclo sem erro sanitizado
```

Conclusão:

- o código do slice permanece revisado;
- a promoção da fila foi reaberta por evidência operacional;
- o 3.6F-D ficou bloqueado por H-2026-07-16A naquele incidente e, após a
  conclusão operacional de H-A, permanece bloqueado pelo umbrella
  H-2026-07-16B;
- o bug de passageiros/criança é separado em H-2026-07-16B;
- não misturar infraestrutura shadow com estado de passageiros no mesmo PR.

## Limite do contrato TravelQueryMeaningV2

`TravelQueryMeaningV2` não representa quantidade de passageiros, idades ou criança menor de 5 anos. Esses fatos não devem ser fabricados pelo evaluator do 3.6F-D.

O H-2026-07-16B é um umbrella, não um hotfix executável. H-B1 constrói somente a
fundação estrutural; H-B2 cria o contrato próprio, validator, corpus e shadow de
`PassengerClarificationMeaningV1`; H-B3 promove esse meaning apenas sob gates
restritos. A decisão canônica está em
`docs/adr/ADR-2026-07-passenger-authority-and-serialization.md`.

## Replanejamento H-B após o sexto review

```text
H-B1 — fonte durável, eventos, serialização por sessão, prompt enviado,
        fail-closed e BookingDraftContext como projeção; sem linguagem
  ↓ review sem P1/P2 e prova concorrente real
H-B2 — PassengerClarificationMeaningV1 strict, validator, corpus e shadow;
        sem tools ou efeito user-visible
  ↓ protocolo com review documental sem P0/P1/P2 + amostra confirmatória e
    rollback proof em PASS
H-B3 — runtime somente em prompt passageiro/criança e decisão
        WEAK/FALLBACK/UNKNOWN; sem booking/payment direto
  ↓ review e gates operacionais completos
H-B concluída
  ↓
3.6F-D pode ser reavaliada
```

O reducer recebe somente eventos estruturados. Antes do booking, a autoridade é
`PassengerClarificationStateV1`; depois do booking, são booking/passengers
persistidos. Transcript e `tool_context` não reconstroem composição. Correções
substituem o agregado completo, estados inválidos bloqueiam LLMs/tools e toda
atualização por sessão usa transação curta com row lock ou revision/CAS e retry
bounded, sem chamada externa sob lock.

## Definition of Done operacional

Para mudanças em runtime, worker, banco ou deploy:

```text
testes locais
review sem P1/P2
tracker atualizado
migration aplicada antes do binário, quando houver
smoke no ambiente alvo
artefato esperado persistido
ausência de erro recorrente
rollback conhecido
```

Review local limpo sem smoke obrigatório não libera o sucessor.
