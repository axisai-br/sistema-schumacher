# AGENTS.md

## Projeto

Este monorepo é o sistema Schumacher Tur.

Áreas principais:

- `apps/api`: API Go principal.
- `apps/app`: app interno de atendimentos.
- `apps/web`: site público.
- `infra/swarm`: stacks Docker Swarm.
- `docs-sistema`: documentação técnica canônica.

O backend atual concentra regras novas de atendimento, reserva, pagamento, disponibilidade e automação. O `n8n` é legado/apoio histórico, não deve ser tratado como fonte principal para novas regras.

## Regras gerais

- Antes de alterar código, investigue o fluxo atual no repo.
- Não faça deploy.
- Não altere arquivos de produção, secrets ou ambiente real.
- Não vaze secrets em logs, testes ou documentação.
- Prefira patches pequenos e testáveis.
- Preserve contratos públicos da API, exceto quando a tarefa pedir explicitamente mudança de contrato.
- Sempre explique:
  - arquivos alterados;
  - comportamento antes/depois;
  - testes executados;
  - riscos restantes, se houver.

## Comandos comuns

### API Go

```bash
cd apps/api
go test ./internal/chat ./internal/automation ./internal/payments ./internal/bookings ./internal/availability ./cmd/api
```

Para alterações pontuais, rode testes do pacote afetado.

### Frontend

```bash
cd apps/app
npm run build
```

Ajuste os comandos caso o projeto use outro package manager.

## Commits

Quando eu pedir sugestão de commit, não execute `git commit` nem `git push`.

Analise o diff atual e sugira mensagens no padrão:

```txt
tipo(escopo): ação objetiva
```

## Tipos permitidos

- `fix`: correção de bug
- `feat`: nova funcionalidade
- `refactor`: reorganização interna sem mudar comportamento esperado
- `test`: testes
- `docs`: documentação
- `chore`: manutenção operacional
- `ci`: GitHub Actions, build ou publicação
- `perf`: desempenho

## Escopos comuns

- `api`
- `api/chat`
- `api/payments`
- `api/automation`
- `api/availability`
- `api/bookings`
- `app`
- `app/atendimentos`
- `web`
- `infra/swarm`
- `docs`
- `ci`

## Workflow com tracker de execução

- Sempre leia `docs/EXECUTION_TRACKER.md` antes de iniciar uma tarefa de arquitetura, etapa, hotfix ou bug.
- Se `docs/SESSION_HANDOFF.md` existir, leia-o logo depois do tracker para reconciliar evidências operacionais recentes.
- O tracker é fonte de contexto e rastreio, não backlog autoexecutável.
- Execute somente a etapa ou hotfix explicitamente pedido no `/goal`.
- Não avance para a próxima etapa sem pedido explícito.
- Se encontrar melhoria fora do escopo, registre como observação/backlog no tracker, mas não implemente.
- Para tarefas com plano detalhado, leia também o arquivo informado em `plans/`.
- `plans/` contém planos canônicos e deve ser versionado. Rascunhos locais devem usar `plans/local/` ou o sufixo `.local.md`.
- Ao final da execução, atualize `docs/EXECUTION_TRACKER.md` com:

  - status;
  - arquivos alterados;
  - testes executados;
  - resultado do review;
  - necessidade de teste em produção;
  - próxima ação recomendada.
- Não faça commit nem push, salvo pedido explícito.

### Gate operacional atual

- H-2026-07-27A está **EM CORREÇÃO APÓS REVIEW — 3 P1 + 1 P2 DE ENTREGA TEMPORAL E PROVENIÊNCIA**.
- A única próxima ação é um novo `/review` dirigido ao hotfix; evidência local verde não declara review limpo nem autoriza commit, push, deploy ou smoke.
- H-2026-07-16B2 e 3.6F-D permanecem bloqueadas.

### Validação padrão para mudanças em `apps/api/internal/chat`

```bash
cd apps/api
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

Use essa validação para qualquer etapa, hotfix ou bug que altere o fluxo de chat.


## Gate operacional e observabilidade

- Review local limpo não substitui smoke obrigatório quando o slice altera runtime, banco, worker ou deploy.
- Evidência de produção pode bloquear o sucessor sem apagar o histórico de review local.
- Se uma feature habilitada não produzir o artefato esperado, registre incidente operacional no tracker antes de avançar.
- Workers/background jobs devem registrar reasons fechados e erros sanitizados.
- Para PostgreSQL, registrar no máximo `operation`, `error_class`, `SQLSTATE`, timeout ou cancelamento.
- Nunca registrar SQL completo, payload, body, telefone, CPF, documento, segredo ou `DATABASE_URL`.
- Não considerar uma etapa operacionalmente encerrada enquanto o smoke obrigatório estiver pendente ou falhando.

## Disciplina de escopo

- Um `/goal` executa uma única etapa ou hotfix.
- Um PR deve concentrar uma responsabilidade principal.
- Achado fora do escopo vira backlog ou novo hotfix; não ampliar silenciosamente o slice.
- Se o review exigir uma terceira rodada corretiva, reavaliar arquitetura e divisão do slice antes de continuar empilhando remendos.

## Formato da resposta

Quando sugerir commit, responda com:

- commit recomendado;
- por que as mudanças pertencem ao mesmo commit;
- alternativa em commits menores, se o diff misturar responsabilidades;
- arquivos que entrariam em cada commit sugerido.

Se o diff misturar responsabilidades demais, avise antes de sugerir um commit único.

## Fluxo de trabalho esperado

1. Investigar arquivos e funções relacionadas.
2. Propor plano curto.
3. Implementar patch pequeno.
4. Rodar testes.
5. Usar `/diff` para revisar alterações.
6. Usar `/review` antes de considerar a tarefa finalizada.

## Restrições

- Não criar dependência externa sem justificar.
- Não mexer em Docker Swarm, GHCR ou infra sem pedido explícito.
- Não alterar n8n salvo pedido explícito.
- Não modificar schema de banco sem migration.
- Não criar cobrança duplicada em fluxos Pagarme.
- Não quebrar idempotência de reservas, pagamentos, mensagens ou webhooks.
