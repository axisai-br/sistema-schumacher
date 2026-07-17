# Session handoff — sistema-schumacher

## Objetivo do projeto

Corrigir estruturalmente os comportamentos errados observados nas conversas sem transformar o backend em um parser geral de regex.

Arquitetura:

```text
OpenAI interpreta linguagem
validator local verifica contrato/facts
backend executa somente ações autorizadas
```

## Estado canônico atual

```text
3.6F-A concluída
3.6F-B concluída
3.6F-C concluída em código, mas com gate operacional reaberto
H-2026-07-16A PRÓXIMA
H-2026-07-16B pendente após H-A
3.6F-D bloqueada por H-B
```

## Incidente H-2026-07-16A

Confirmado:

- V1 shadow=false;
- V1 assist=false;
- Travel V2 shadow=true no PID 1;
- Supabase self-hosted/PostgreSQL correto;
- migration 0021, coluna e índice presentes;
- SELECT/UPDATE permitidos;
- candidate query do sweeper passou;
- query completa de recovery manual passou com 0/0/0;
- query manual de criação de claim passou com rollback;
- mensagens reais não criaram `travel_query_v2_shadow_claims`;
- recovery registrou `sweep_failed` a cada ciclo.

Não voltar a investigar migration/permissão/SQL básico sem nova evidência. O próximo trabalho é scheduler/job/claim/recovery e logging sanitizado.

## Bug H-2026-07-16B

```text
"eu e meus 2 filhos" → observado 2; esperado 3
"sim, o mais novo tem 4 anos" em ASK_CHILD_UNDER_5
→ estado não avançou e pergunta repetiu
```

Esse bug é determinístico e separado de TravelQueryMeaningV2.

## Próxima ação

Executar somente:

```text
plans/h-2026-07-16a-travel-v2-shadow-operacional.md
```

Depois review, correções P1/P2, smoke. Em seguida H-B. Só então 3.6F-D.

## Arquivos que a nova sessão deve ler

1. `AGENTS.md`
2. `docs/EXECUTION_TRACKER.md`
3. `docs/SESSION_HANDOFF.md`
4. `docs/PRODUCTION_CONVERSATION_CASES.md`
5. `plans/00-plano-mestre-travel-semantic-v2.md`
6. plano da única etapa `PRÓXIMA`

Não carregar todos os planos no prompt operacional do Codex. Eles podem ficar versionados no repositório para consulta futura.

## Evidências que podem ser fornecidas sem risco

- SHA atual e branch;
- nomes de migrations;
- flags booleanas;
- resultados resumidos de queries;
- SQLSTATE sanitizado;
- logs sem query string, telefone, CPF, documento ou segredo;
- transcrições anonimizadas.

Nunca fornecer:

- `DATABASE_URL`;
- API keys;
- webhook secret;
- payload de documento;
- telefone/CPF reais;
- logs com query string sensível.

## Critério para pedir revisão ampla do GitHub

Não fazer auditoria completa do repositório em toda nova sessão.

Faça reconciliação ampla apenas quando:

- começa nova fase arquitetural;
- tracker e GitHub divergem;
- houve merge grande fora da sequência;
- código atual contradiz o plano;
- o próximo hotfix ainda não tem causa/localização delimitada.

No trabalho diário, peça análise dirigida aos arquivos e call paths do goal atual.
