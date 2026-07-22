# Índice dos planos

## Ordem canônica atual

1. `p0-a-reconciliar-deploy-smoke.md`
2. `p0-b-corrigir-fixtures-temporais.md`
3. `p0-c-ci-test-gate.md`
4. `3.6f-a-contrato-travel-query-meaning-v2.md`
5. `3.6f-b-validator-v2.md`
6. `3.6f-c-openai-v2-shadow.md`
7. `h-2026-07-16a-travel-v2-shadow-operacional.md`
8. `h-2026-07-16b-passenger-child-state.md` — umbrella não executável
9. `h-2026-07-16b1-passenger-state-foundation.md`
10. `h-2026-07-16b2-passenger-meaning-v1.md`
11. `h-2026-07-16b3-passenger-meaning-runtime.md`
12. `3.6f-d-corpus-evaluator-v2.md`
13. `3.6f-e-observabilidade-v2.md`
14. `3.6f-f-templates-seguros.md`
15. `3.6f-g-earliest-available.md`
16. `3.6f-h-route-coverage.md`
17. `3.6f-i-arbitragem-runtime-weak.md`

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

H-2026-07-16B é somente umbrella. Para essa fila, abrir exclusivamente o plano
do filho que estiver marcado como `PRÓXIMA`; B2 não pode ser incluído no diff de
B1 e B3 não pode ser incluído no diff de B2.
