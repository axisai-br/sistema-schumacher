# H-2026-07-16B — Umbrella do estado de passageiros

## Status no tracker

```text
EM ANDAMENTO — B1 concluída; B2 PRÓXIMA.
```

O H-B não é mais um slice executável. Ele organiza três slices independentes e
permanece aberto até todos concluírem seus gates. B1 está concluída e segura
para commit; B2 está liberada somente como próxima slice e nenhuma implementação
de B2 foi iniciada. B3 e 3.6F-D permanecem bloqueadas.

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
| 1 | H-2026-07-16B1 | **CONCLUÍDA — REVIEW FINAL SEM P1/P2 — SEGURA PARA COMMIT** | fonte durável, eventos, serialização por sessão, propagação do prompt enviado, fail-closed e `BookingDraftContext` como projeção; sem interpretação de linguagem |
| 2 | H-2026-07-16B2 | **PRÓXIMA — ainda não iniciada** | `PassengerClarificationMeaningV1` strict, validator local, corpus e shadow; sem tools ou mudança user-visible |
| 3 | H-2026-07-16B3 | **BLOQUEADA por H-B2** | promoção gated somente em prompt passageiro/criança e decisão `WEAK`/`FALLBACK`/`UNKNOWN`; sem booking/payment direto |
| 4 | 3.6F-D | **BLOQUEADA por H-B** | só pode ser reavaliada depois do fechamento integral do umbrella |

Planos executáveis:

- `plans/h-2026-07-16b1-passenger-state-foundation.md`;
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
| P1-05 | estado inseguro chega a shadows, LLMs e tools | B1 | `TestPassengerUnsafeStateStopsExternalWorkBeforeDispatch` |
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
- liberar B2, B3 ou 3.6F-D por inferência documental.

## Próxima ação única

Preparar o commit de B1 a partir da composição auditada. Executar B2 somente em
outro `/goal` explícito; este PR não iniciou contrato, validator, corpus ou
shadow de B2. B3 permanece bloqueada por B2, e 3.6F-D permanece bloqueada pelo
fechamento integral de H-B. Este umbrella não deve ser usado como objetivo de
implementação.
