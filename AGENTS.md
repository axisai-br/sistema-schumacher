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
- O tracker é fonte de contexto e rastreio, não backlog autoexecutável.
- Execute somente a etapa ou hotfix explicitamente pedido no `/goal`.
- Não avance para a próxima etapa sem pedido explícito.
- Se encontrar melhoria fora do escopo, registre como observação/backlog no tracker, mas não implemente.
- Para tarefas com plano detalhado, leia também o arquivo informado em `plans/`.
- `plans/` contém planos locais/operacionais e pode estar ignorado pelo Git.
- Ao final da execução, atualize `docs/EXECUTION_TRACKER.md` com:

  - status;
  - arquivos alterados;
  - testes executados;
  - resultado do review;
  - necessidade de teste em produção;
  - próxima ação recomendada.
- Não faça commit nem push, salvo pedido explícito.

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
