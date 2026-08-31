# H-2026-07-16B — Umbrella do estado de passageiros

## Status no tracker

```text
EM ANDAMENTO — próximo slice H-B2 aguarda autorização própria.
```

O H-B não é mais um slice executável. Ele organiza três slices independentes e
permanece aberto até todos concluírem seus gates. B1 está concluída: o problema
original de H-2026-07-22A foi corrigido/deployado e o blocker posterior do
smoke RED histórico foi fechado por H-2026-07-27A com review, merge, deploy e
smoke verificados. B2 ainda não foi iniciada; B3 e 3.6F-D permanecem
bloqueadas.

## Histórico — motivo do replanejamento após o sexto review

Naquele sexto review histórico, o patch então vigente possuía snapshot V1,
eventos, ledgers e reducer tipado, mas ainda misturava autoridade,
interpretação de linguagem e runtime. Naquela rodada, os nove P1 demonstravam
dependência de parser/janela, perda do evento do prompt enviado, fail-closed
tardio, fontes concorrentes de estado, correção parcial e lost update entre
Reprocess concorrentes.

Esse diagnóstico foi superseded pela implementação e pelo review final sem
P1/P2 de B1; ele não descreve a composição vigente.

Continuar corrigindo tudo no mesmo diff repetiria a sobreposição de
responsabilidades apontada pelos reviews. A decisão canônica passa a ser
`docs/adr/ADR-2026-07-passenger-authority-and-serialization.md`.

## Fila canônica do umbrella

| Ordem | Slice | Status | Responsabilidade única |
|---|---|---|---|
| 1 | H-2026-07-16B1 | **CONCLUÍDA — GATE OPERACIONAL ENCERRADO** | fonte durável preservada; bootstrap `UNKNOWN` fresco não bloqueia o atendimento antes de contexto de passageiros |
| 2 | H-2026-07-22A | **CORRIGIDO E DEPLOYADO — RED HISTÓRICO ORIGINOU H-A; BLOCKER FECHADO** | identidade do prompt e facts de continuidade preservam `STRONG` e exigem autoridade bookable explícita sem enfraquecer o `UNKNOWN` fresco |
| 3 | H-2026-07-16B2 | **PRÓXIMA — NÃO INICIADA; AGUARDANDO AUTORIZAÇÃO PRÓPRIA** | `PassengerClarificationMeaningV1` strict, validator local, corpus e shadow; sem tools ou mudança user-visible |
| 4 | H-2026-07-16B3 | **BLOQUEADA por H-B2** | promoção gated somente em prompt passageiro/criança e decisão `WEAK`/`FALLBACK`/`UNKNOWN`; sem booking/payment direto |
| 5 | 3.6F-D | **BLOQUEADA por H-B** | só pode ser reavaliada depois do fechamento integral do umbrella |

Planos executáveis:

- `plans/h-2026-07-16b1-passenger-state-foundation.md`;
- `plans/h-2026-07-22a-fresh-session-passenger-gate.md`;
- `plans/h-2026-07-16b2-passenger-meaning-v1.md`;
- `plans/h-2026-07-16b3-passenger-meaning-runtime.md`.

Cada `/goal` executa exatamente um slice. Não executar B1, B2 e B3 no mesmo PR.

## Destino dos nove P1

| ID | Achado do sexto review | Slice que corrige | Teste que prova |
|---|---|---|---|
| P1-01 | parser lexical novo de família e crescimento de regex/listas | B1 remove a interpretação da fundação; B2 fornece a substituição semântica em contrato próprio | B1: `TestPassengerStateFoundationDoesNotAddLexicalFamilyRules` e inventário `regexp.MustCompile == 54`; B2: `TestPassengerMeaningV1ValidatorDoesNotParseCurrentTurn` |
| P1-02 | bootstrap reparsa inbound da janela histórica | B1 | `TestPassengerStateBootstrapUsesStructuredEvidenceOnly` |
| P1-03 | evento fica no draft e não acompanha o outbound enviado | B1 | `TestPassengerPromptEventFollowsReviewedAndAutoSentOutboundBeyondHistoryWindow` |
| P1-04 | contexto infantil depende de `ActivePrompt.Kind` textual | B1 | `TestPassengerChildAddsTravelerUsesPersistedPromptEpoch` |
| P1-05 | estado inseguro chega a shadows, LLMs e tools | B1 | `TestPassengerGateAfterDeliveredPromptStopsExternalWorkBeforeDispatch` |
| P1-06 | `BookingDraftContext` recupera contagem de `booking_create` | B1 | `TestBookingDraftProjectionIgnoresBookingCreatePassengerCount` |
| P1-07 | booking criado avança payment antes de validar slots | B1 | `TestBookingCreatedWithUnknownPassengerSlotsFailsClosed` |
| P1-08 | correção de total preserva `adds_traveler` incompatível | B1 | `TestPassengerAggregateCorrectionClearsDependentAddsTraveler` |
| P1-09 | dois `Reprocess` podem perder atualização do snapshot | B1 | `TestPassengerStateConcurrentReprocessPreservesBothEvents` e `TestPassengerStateApplyEventsSerializesSessionPostgres` |

O P1-01 só fica integralmente encerrado quando B1 comprovar a remoção lexical e
B2 comprovar que o significado aberto não voltou ao validator/reducer. Isso não
autoriza antecipar B2 dentro do diff do B1.

## Contratos globais

- interpreter interpreta linguagem;
- reducer recebe somente eventos estruturados;
- `PassengerClarificationStateV1` é a fonte canônica pré-booking;
- booking/passengers persistidos são a fonte pós-booking;
- `tool_context` e transcript não reconstroem estado;
- `prompt_event` acompanha o outbound efetivamente enviado;
- envio confiável aplica `prompt_event` idempotentemente;
- correção substitui o agregado completo;
- estado inválido bloqueia LLMs e tools antes de qualquer dispatch;
- atualização do snapshot usa row lock curto ou revision/CAS com retry bounded;
- nenhum lock/transação permanece aberto durante chamada externa;
- `BookingDraftContext` é somente projeção da autoridade aplicável.

## Gates de desbloqueio

### B1 -> B2

- nove regressões estruturais implementadas e verdes;
- inventário lexical sem crescimento;
- teste concorrente real em PostgreSQL executado, não apenas skipped;
- matriz focada, repetida, race, H-012/document/lap-child/payment,
  `./internal/chat`, `./...` e `git diff --check` verde;
- review sem P1/P2;
- nenhuma chamada externa sob lock e nenhum incidente operacional aberto.
- H-2026-07-22A revisado e implantado; seu RED histórico originou H-A, cujo
  fechamento operacional comprovou seleção materializada e avanço seguro até
  `ASK_PASSENGER_COUNT`, sem `NONE`/`SAFE_PHASE_FALLBACK`.

### B2 -> B3

- contrato strict, validator sem parser, corpus e evaluator verdes;
- shadow com zero influência user-visible, mutação de state e tool call;
- review sem P1/P2;
- amostra e limiares de promoção aprovados no tracker;
- rollback por flag comprovado e nenhuma evidência operacional pendente.

### B3 -> fechamento do H-B

- promoção restrita a prompt passageiro/criança e
  `WEAK`/`FALLBACK`/`UNKNOWN`;
- `STRONG` e estado inválido inelegíveis;
- nenhuma tool, booking ou payment direto;
- serialização/idempotência do B1 preservada;
- review sem P1/P2, rollout/rollback e smoke quando explicitamente autorizados;
- nenhum incidente aberto.

Somente depois desses gates o tracker pode concluir H-B e reavaliar o
desbloqueio de 3.6F-D.

## Fora de escopo deste replanejamento

- implementar provider, schema, reducer, repository ou runtime;
- alterar código de produção ou testes;
- executar commit, push, deploy ou smoke;
- iniciar B2, B3 ou 3.6F-D sem `/goal` e autorização próprios.

## Próxima ação única

Todos os gates B1 → B2 estão satisfeitos. Mediante novo `/goal` e autorização
explícita, a próxima ação possível é iniciar somente H-B2. Esta reconciliação
não inicia contrato, validator, corpus ou shadow de B2. B3 permanece bloqueada
por B2, e 3.6F-D permanece bloqueada pelo fechamento integral de H-B. Este
umbrella não deve ser usado como objetivo de implementação.
