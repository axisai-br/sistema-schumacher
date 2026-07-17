# Índice dos planos

## Ordem canônica atual

1. `p0-a-reconciliar-deploy-smoke.md`
2. `p0-b-corrigir-fixtures-temporais.md`
3. `p0-c-ci-test-gate.md`
4. `3.6f-a-contrato-travel-query-meaning-v2.md`
5. `3.6f-b-validator-v2.md`
6. `3.6f-c-openai-v2-shadow.md`
7. `h-2026-07-16a-travel-v2-shadow-operacional.md`
8. `h-2026-07-16b-passenger-child-state.md`
9. `3.6f-d-corpus-evaluator-v2.md`
10. `3.6f-e-observabilidade-v2.md`
11. `3.6f-f-templates-seguros.md`
12. `3.6f-g-earliest-available.md`
13. `3.6f-h-route-coverage.md`
14. `3.6f-i-arbitragem-runtime-weak.md`

Leia também:

- `00-plano-mestre-travel-semantic-v2.md`
- `docs/EXECUTION_TRACKER.md`
- `docs/SESSION_HANDOFF.md`, quando existir

## Regra

O tracker é a autoridade sobre a etapa atual. Hotfixes operacionais ou bugs user-visible podem interromper a sequência arquitetural.

O Codex recebe somente:

1. `AGENTS.md`, carregado automaticamente;
2. `docs/EXECUTION_TRACKER.md`;
3. `docs/SESSION_HANDOFF.md`, quando aplicável;
4. o plano da única etapa marcada como `PRÓXIMA`.

Não executar múltiplos slices ou hotfixes no mesmo `/goal` ou PR.
