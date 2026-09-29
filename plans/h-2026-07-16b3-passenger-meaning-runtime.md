# H-2026-07-16B3 — Promoção runtime de PassengerClarificationMeaningV1

## Status esperado no tracker

```text
BLOCKED por H-2026-07-16B2 — CONFIRMATORY GATE NOT_EXECUTED
```

Esta definição não inicia H-B3. O protocolo corrigido em
`plans/h-2026-07-16b2-passenger-meaning-v1.md`, seção **Protocolo do gate de
promoção — REVIEW_CLOSED / APPROVED_FOR_COLLECTION**, teve review documental
encerrado sem P0/P1/P2 e foi integrado pelo PR #83.
`APPROVED_FOR_COLLECTION != PASS`: os dois conjuntos de 100 casos, controles de inelegibilidade e
adversariais e rollback proof OFF/ON permanecem não executados.
Todos precisam resultar em `PASS`; resultado `FAIL` ou
`INCONCLUSIVE` mantém este slice bloqueado.

## Objetivo

Promover significado validado para eventos estruturais somente quando o active
prompt for passageiro/criança e a decisão determinística for
`WEAK`/`FALLBACK`/`UNKNOWN`, sem booking ou payment direto.

## Pré-condições

- B1 concluído, serializado e operacionalmente validado;
- B2 `REVIEW_CLOSED`, merged e deployed;
- corpus obrigatório verde;
- shadow sem influência runtime e sem violações críticas;
- review documental do protocolo encerrado sem P0/P1/P2, incluindo P1-A,
  P1-B, P2-A, P2-B e o P1 posterior do plano mestre; condição satisfeita;
- amostra confirmatória operacional e do runner isolado aprovadas conforme o
  protocolo H-B2;
- controles de inelegibilidade e adversariais aprovados;
- rollback OFF/ON comprovado sem alteração de código;
- nenhum incidente aberto e ausência de erro recorrente comprovada;
- flag de rollout e rollback definidas antes da implementação.

O delivery, review e os gates locais do código B2 estão encerrados. O protocolo
de promoção está `REVIEW_CLOSED / APPROVED_FOR_COLLECTION`; amostra,
controles operacionais, prova de ausência de recorrência e rollback proof
permanecem pendentes. A próxima ação canônica é novo `/review` documental
independente da reconciliação pós-merge, antes de indicar coleta como próxima
ação. H-B3 permanece **BLOCKED** até PASS integral de H-B2, e nenhum
arquivo ou fluxo de H-B3 está autorizado para execução.

## Escopo autorizado

- classificação de força específica para decisões de passenger clarification;
- gate de elegibilidade por prompt, força, estado e flag;
- reutilização ou chamada única do runner B2;
- validação local obrigatória;
- mapper `PassengerClarificationMeaningV1 -> eventos B1`;
- aplicação serializada/idempotente pelos mecanismos do B1;
- clarification/template seguro depois da persistência;
- rollout gradual, métricas, auditoria sanitizada e kill switch.

## Fora de escopo

- override de decisão `STRONG`;
- prompt que não seja passageiro ou criança;
- estado corrompido, conflitante ou invariant-invalid;
- booking_create, payment, cancel, document extraction ou qualquer tool direta;
- mutação de booking/passengers pós-booking;
- Travel V2, vector, retrieval ou planner;
- segunda resposta ao mesmo inbound;
- ativação ampla sem homologação/smoke autorizados.

## Gate runtime

Todos os predicados devem ser verdadeiros:

```text
flag enabled
ActivePrompt.Kind in {PASSENGER_COUNT, LAP_CHILD_QUESTION}
PassengerClarificationStateV1 estruturalmente válido
slot esperado pertence ao prompt_event persistido
deterministic strength in {WEAK, FALLBACK, UNKNOWN}
meaning schema valid
validator accepted
confidence >= limiar aprovado
idempotency key ainda não aplicada
nenhum guardrail STRONG
```

Qualquer falha mantém a decisão determinística/fail-closed do B1. Confidence
sozinha nunca autoriza promoção.

## Aplicação

1. obter meaning validado sem tool;
2. mapear a proposta para eventos estruturais, incluindo substituição de
   agregado em correções;
3. aplicar eventos pela transação serializada do B1;
4. reler/retornar o snapshot persistido;
5. produzir somente template determinístico seguro;
6. booking/payment, quando eventualmente elegíveis em turno posterior,
   continuam passando pelas readiness e autoridades existentes.

O meaning não chama booking/payment e não escolhe documentos. A criação do
evento e a resposta não mantêm lock durante provider ou sender.

## Força da decisão

- `STRONG`: cancelamento/humano explícito, documento/mídia em fase documental,
  guardrails de payment, evento já estruturado válido e bloqueio de segurança;
- `WEAK`: heurística determinística que não fecha composição;
- `FALLBACK`: clarification/fallback contextual;
- `UNKNOWN`: ausência de significado determinístico para a resposta ao prompt.

`STRONG` não pode ser sobrescrito, mesmo com confidence 1.0.

## Critérios de aceite

- somente prompts passageiro/criança são elegíveis;
- somente `WEAK`/`FALLBACK`/`UNKNOWN` são elegíveis;
- `STRONG` e estados inválidos fazem zero chamada de promoção;
- meaning rejeitado não produz evento;
- evento aceito usa serialização/idempotência do B1;
- correção substitui agregado completo;
- no máximo uma chamada e uma resposta por inbound;
- nenhuma tool, booking ou payment direto;
- flag off reproduz B1 byte-a-byte nos campos funcionais;
- kill switch interrompe influência sem deploy;
- logs/resumos não contêm PII;
- `critical_action_violation_count=0`.

## Testes obrigatórios

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*PassengerMeaningRuntime.*Eligibility|Test.*PassengerMeaningRuntime.*Strong|Test.*PassengerMeaningRuntime.*Correction|Test.*PassengerMeaningRuntime.*Idempotency|Test.*PassengerMeaningRuntime.*NoTools'
go test -count=20 ./internal/chat -run 'Test.*PassengerMeaningRuntime.*Eligibility|Test.*PassengerMeaningRuntime.*Correction|Test.*PassengerMeaningRuntime.*Idempotency'
go test -race -count=1 ./internal/chat -run 'Test.*PassengerMeaningRuntime.*Concurrent|Test.*PassengerMeaningRuntime.*Idempotency'
go test -count=1 ./internal/chat -run 'Test.*H012|Test.*Payment|Test.*LapChild|Test.*Passenger.*Document|Test.*Document.*Passenger|Test.*Booking.*Document'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

Regressões mínimas:

```text
prompt passageiro + UNKNOWN + "eu e meus 2 filhos" -> meaning validado -> evento total 3
prompt criança + FALLBACK + "sim, o mais novo tem 4" -> evento infantil aceito
prompt documentos + mesma frase -> inelegível
STRONG documental/payment -> inelegível
estado CONFLICTING -> zero provider/tool
correção "na verdade somos 2" -> agregado substituído, nunca 3 documentos
retry concorrente -> um efeito, sem lost update
flag off -> comportamento B1
```

## Gate de fechamento do umbrella H-B

- todos os testes e métricas acima comprovados;
- review sem P1/P2;
- rollout controlado e rollback validados;
- smoke somente após autorização explícita;
- nenhuma violação crítica ou incidente aberto;
- tracker atualizado com evidência local e operacional;
- somente então H-B pode concluir e 3.6F-D pode ser reavaliada.

## `/goal`

```text
/goal
Execute somente H-2026-07-16B3 após liberação explícita do B2.

Promova PassengerClarificationMeaningV1 somente para active prompt de
passageiro/criança e decisão determinística WEAK/FALLBACK/UNKNOWN, atrás de
flag desligada por padrão. Schema e validator aceitos são obrigatórios;
confidence sozinha não basta.

Converta meaning apenas em eventos B1 e aplique-os pela serialização por sessão.
Não faça booking, payment, cancel, document extraction ou qualquer tool direta.
STRONG e estado inválido são inelegíveis. Preserve uma chamada, uma resposta,
idempotência, observabilidade sanitizada e rollback. Não avance 3.6F-D.
```

## `/review`

```text
/review
Revise somente H-2026-07-16B3.

Procure promoção fora de prompt passageiro/criança, override de STRONG,
confidence como prova única, estado inválido chegando ao provider, tool/booking/payment
direto, correção parcial, lost update, dupla chamada/resposta, flag off
divergente, lock durante chamada externa e ausência de rollback.

Exija count=20, race, H-012/document/lap-child/payment, ./internal/chat, ./...,
git diff --check e smoke somente se autorizado. Não altere arquivos nem libere
3.6F-D sem review e gates operacionais completos.
```
