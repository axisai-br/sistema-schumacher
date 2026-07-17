# Hotfix H-2026-07-16A — Travel V2 shadow operacional

## Status esperado no tracker

```text
PRÓXIMA
```

## Objetivo

Descobrir e corrigir a causa pela qual o Travel V2 shadow habilitado não cria claims e o recovery registra `sweep_failed` sem causa observável.

## Evidências já confirmadas

```text
CHAT_OPENAI_TRAVEL_V2_SHADOW_ENABLED=true no PID 1
API conectada ao PostgreSQL/Supabase correto
migration 0021 aplicada
coluna e índice presentes
SELECT e UPDATE permitidos
candidate query do sweeper: PASS
query completa de recovery manual: PASS, 0/0/0
query manual de criação de claim: PASS com ROLLBACK
mensagens reais: nenhum travel_query_v2_shadow_claims
recovery: sweep_failed recorrente
```

Não repetir investigação de migration, permissão ou SQL já comprovado sem nova evidência.

## Escopo autorizado

- scheduler do shadow V2;
- criação/conclusão de claim;
- recovery loop;
- logs sanitizados;
- testes unitários e integração PostgreSQL;
- tracker.

Não incluir passenger flow, templates, evaluator ou arbitragem runtime.

## Implementação obrigatória

Instrumentar reasons fechados:

```text
scheduler:
disabled
empty_idempotency_key
incompatible_store
capacity_full
scheduled

job:
started
claim_acquired
claim_in_progress
claim_completed_reused
claim_failed
provider_started
provider_completed
completion_failed
completion_completed

recovery:
sweep_started
sweep_done
sweep_failed
```

Erros PostgreSQL podem registrar apenas:

```text
operation
error_class
SQLSTATE
timeout
canceled
```

Proibido registrar SQL, payload, body, telefone, CPF, documento, segredo ou `DATABASE_URL`.

## Critérios de aceite

- causa raiz comprovada;
- mensagens reais geram claim;
- claim termina `COMPLETED`;
- `recovery_due_at` volta a `NULL`;
- zero candidatos no recovery é sucesso;
- scheduler libera slots em sucesso, erro e panic;
- provider não roda quando claim falha;
- nenhuma segunda resposta ou alteração user-visible;
- novo smoke não apresenta `sweep_failed`.

## Testes

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*Travel.*V2.*Shadow|Test.*Shadow.*Schedule|Test.*Shadow.*Claim|Test.*Shadow.*Recovery'
go test -race -count=1 ./internal/chat -run 'Test.*Travel.*V2.*Shadow|Test.*Shadow.*Recovery'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

## `/goal`

```text
/goal
Execute somente H-2026-07-16A.

Leia AGENTS.md, docs/EXECUTION_TRACKER.md, docs/SESSION_HANDOFF.md e este plano. A flag V2 está ativa, banco/migration/permissões e probes SQL passaram, mas mensagens reais não criam claims e o recovery registra sweep_failed.

Instrumente reasons fechados no scheduler, job, claim e recovery; registre somente operation/error_class/SQLSTATE/timeout/canceled. Encontre e corrija a causa real, não apenas os logs.

Teste agendamento único, idempotency key, store interfaces, zero candidatos, claim IN_PROGRESS→COMPLETED, slots em erro/panic e ausência de provider quando claim falha.

Não alterar passenger flow, templates, evaluator, arbitragem ou comportamento user-visible. Atualize o tracker. Não commit, push ou deploy.
```

## `/review`

```text
/review
Revise somente H-2026-07-16A.

Confirme scheduler observável em toda saída, idempotency key não vazia, repository compatível, Reprocess bem-sucedido agenda uma vez, erro não agenda, claim termina COMPLETED, zero candidatos é sucesso, SQLSTATE é sanitizado, slots são liberados, provider não roda após falha de claim e não há efeito user-visible.

Informe P1/P2, causa raiz comprovada, se está seguro para commit e qual smoke executar. Não altere arquivos.
```
