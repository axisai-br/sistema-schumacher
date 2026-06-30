____  
[[Execucao refatoracao arquitetura schumacher]]  
  
# Roadmap de execução — sistema-schumacher  
  
> Arquivo de acompanhamento para o Codex, ChatGPT e revisão humana.  
> 
> Objetivo: manter visível **em que etapa o projeto está**, **qual o próximo slice**, **quais hotfixes interferem no plano** e **quais regras não podem ser quebradas**. Nada de deixar o projeto virar aquele condomínio de decisões perdidas que todo backend eventualmente vira.  
  
---  
  
## Como usar este arquivo com o Codex  
  
### Fluxo recomendado  
  
1. Escolher a próxima etapa marcada como `Próxima` ou `Pendente`.  
2. Pedir ao ChatGPT um `/goal` curto, com até 4000 caracteres.  
3. Colocar o `/plan` detalhado em `plans/<nome-da-etapa>.md`.  
4. Mandar o `/goal` para o Codex.  
5. Se o Codex não pedir o `/plan`, enviar o conteúdo do plano na sequência ou referenciar o arquivo.  
6. Depois da implementação, pedir `/review`.  
7. Corrigir achados P1/P2 antes de commit.  
8. Rodar validações.  
9. Abrir PR.  
10. Após merge/teste em produção, atualizar este arquivo.  
  
### Regras para atualização deste arquivo  
  
Sempre que uma etapa for concluída, atualizar:  
  
- `Status`  
- `Arquivos principais`  
- `O que mudou`  
- `Testes executados`  
- `Riscos restantes`  
- `Próxima etapa recomendada`  
  
Sempre que surgir bug/hotfix, registrar em **Registro de hotfixes e bugs reais** com:  
  
- sintoma  
- causa  
- etapa relacionada  
- status  
- decisão tomada  
  
### Comandos padrão antes de commit  
  
```bash  
cd apps/api  
  
gofmt -w <arquivos-alterados>  
go test -count=1 ./internal/chat  
go test -count=1 ./...  
git diff --check  
  
cd ../..  
git status --short  
git diff --name-only  
```  
  
### Escopo que deve ser protegido por padrão  
  
Salvo quando a etapa pedir explicitamente, não alterar:  
  
```text  
Service.Reprocess  
OpenAI schema/prompt/runner  
shadow runtime  
booking_create  
payment_create  
document_extract  
booking_cancel  
Planner  
endpoints  
banco/migrations  
auto-send  
infra  
n8n  
```  
  
---  
  
# Estado atual resumido  
  
## Situação atual  
  
O sistema já tem uma base local forte para interpretação contextual:  
  
```text  
ActivePromptContext  
→ routeDeterministicIntent  
→ InterpretStructuredTurn  
→ fallback contextual por templates fechados  
→ shadow report local/OpenAI  
```  
  
O fluxo real de reserva está mais estável, mas ainda existem bugs de variação textual em produção.  
  
## Próxima decisão prática  
  
Hotfix local concluído em 2026-06-29; validar em produção/homologação a variação real:
  
```text  
Hotfix H-2026-06-29 — reservation-help variant  
Entrada: "como faço pra fazer uma reserva?"  
Problema corrigido localmente: não cai mais em UNSUPPORTED_PACKAGE
Esperado: ASK_RESERVATION_ROUTE_SC  
```  
  
Etapas 3.6A, 3.6B e 3.6C executadas localmente em 2026-06-29; P2 do review da 3.6C corrigido localmente em 2026-06-30 antes de qualquer uso no fluxo real.

```text
Etapa 3.6C — Avaliação local do corpus canônico
Baseline local: 27 casos avaliados; 22 passaram; 5 falharam; 0 pulados
Produção não mudou
```
  
---  
  
# Legenda de status  
  
| Status | Significado |  
|---|---|  
| `Concluída` | Implementada, testada e aceita. |  
| `Concluída, monitorar` | Funciona, mas deve ser observada em produção. |  
| `Em andamento` | Etapa atual ou parcialmente implementada. |  
| `Pendente` | Ainda não iniciada. |  
| `Bloqueada` | Depende de correção ou decisão anterior. |  
| `Futuro` | Planejada, mas não deve ser iniciada agora. |  
  
---  
  
# Etapas atualizadas  
  
## Etapa 0 — Fundação operacional do chat  
  
**Status:** Concluída, em manutenção.  
  
**O que faz:** mantém o fluxo base funcionando:  
  
```text  
Evolution webhook  
→ ingest  
→ buffer  
→ Service.Reprocess  
→ draft  
→ auto-send  
```  
  
**Objetivo:** garantir que mensagens entram, agrupam, processam e saem sem quebrar atendimento.  
  
**Risco atual:** alterações em `Service.Reprocess` são sensíveis e devem ser evitadas em slices pequenos, salvo quando explicitamente necessário.  
  
---  
  
## Etapa 1 — Roteamento determinístico e tools principais  
  
**Status:** Concluída, com hotfixes contínuos.  
  
**O que faz:** detecta intenções principais e aciona ferramentas/templates:  
  
```text  
availability_search  
booking_create  
payment_create  
booking_cancel  
document_extract  
safe templates  
```  
  
**Objetivo:** evitar que LLM decida ações críticas sem validação.  
  
**Observação:** o roteador ainda recebe hotfixes porque produção revela variações humanas que parser nenhum adivinha por osmose.  
  
---  
  
## Etapa 2 — Estabilização do fluxo real de reserva  
  
**Status:** Em andamento contínuo.  
  
**O que faz:** corrige bugs reais descobertos em teste/homologação/produção:  
  
```text  
início de reserva  
seleção de data  
seleção de opção  
passageiros  
criança  
criança de colo  
documentos  
confirmação documental  
pagamento  
fallbacks contextuais  
```  
  
**Exemplos já corrigidos:**  
  
```text  
"como faço uma reserva" não bloqueia auto-send  
"06/7" não vira unsupported  
"essa mesmo" vira seleção contextual quando há uma opção  
"essa mesmo" não seleciona opção 1 quando há múltiplas opções  
"10" funciona em lista de criança de colo com 10+ passageiros  
```  
  
**Bug corrigido localmente, pendente validação em produção:**
  
```text  
"como faço pra fazer uma reserva?" → ASK_RESERVATION_ROUTE_SC.
```  
  
**Objetivo:** deixar o fluxo real estável antes de aumentar autonomia com LLM.  
  
---  
  
## Etapa 3.1 — Interpreter estruturado local  
  
**Status:** Concluída.  
  
**O que faz:** classifica turnos de forma pura/local, sem OpenAI, sem tools, sem persistência.  
  
**Objetivo:** criar uma interpretação testável e determinística do turno.  
  
---  
  
## Etapa 3.2A — Contrato OpenAI structured interpreter  
  
**Status:** Concluída.  
  
**O que faz:** define schema, prompt e validação local rígida para o interpreter OpenAI.  
  
**Objetivo:** impedir saída solta, campos executores, enums inválidos e vazamento sensível.  
  
---  
  
## Etapa 3.2B — Runner OpenAI structured interpreter  
  
**Status:** Concluída.  
  
**O que faz:** chama a Responses API em modo structured, com schema strict.  
  
**Estado atual:** usa `store=false` e `tools=[]`. Não usa file search, vector store nem tools reais.  
  
**Objetivo:** obter interpretação alternativa da OpenAI sem permitir que ela execute fluxo.  
  
---  
  
## Etapa 3.3 — Shadow mode no Reprocess  
  
**Status:** Concluída.  
  
**O que faz:** roda interpreter local e OpenAI em paralelo, compara, salva resumo seguro e não altera o fluxo real.  
  
**Objetivo:** medir sem arriscar produção.  
  
---  
  
## Etapa 3.4A — Relatório puro/local de divergências  
  
**Status:** Concluída.  
  
**O que faz:** agrega resultados de shadow em memória:  
  
```text  
agreement  
disagreement  
status OpenAI  
latência  
erros  
validation_errors  
vazamento sensível  
```  
  
**Objetivo:** transformar shadow em métrica.  
  
---  
  
## Etapa 3.4B — Loader/adapter do relatório real  
  
**Status:** Concluída.  
  
**O que faz:** carrega `structured_interpreter_shadow` de `Message.Payload` / `Message.NormalizedPayload` e alimenta o relatório 3.4A.  
  
**Decisões importantes:**  
  
- mensagens sem shadow são ignoradas;  
- payload malformado não pode dar panic;  
- `validation_errors` no nível da mensagem e dentro do shadow devem ser preservados;  
- `Message.Body` não deve virar `SensitiveScanPayload` por padrão.  
  
**Objetivo:** medir divergências reais do banco com segurança.  
  
---  
  
## Etapa 3.4C — Endpoint real do relatório  
  
**Status:** Concluída.  
  
**O que faz:** expõe relatório real do shadow por rota autenticada/read-only.  
  
**Decisão importante:** `session_id` é obrigatório.  
  
**Motivo:** evitar scan global em `chat_messages` com predicado JSONB sem índice adequado. Relatório é para observar produção, não para derrubar o banco enquanto observa, esse clássico da gestão moderna.  
  
**Objetivo:** consultar divergências reais sem SQL manual inseguro.  
  
---  
  
## Etapa 3.5A — ActivePromptContext puro/local  
  
**Status:** Concluída.  
  
**O que faz:** cria uma camada pura que responde:  
  
```text  
qual era a última pergunta ativa do bot?  
```  
  
**Tipos principais:**  
  
```text  
UNKNOWN  
RESERVATION_ROUTE  
AVAILABILITY_DATE_CHOICE  
AVAILABILITY_OPTION_CHOICE  
PASSENGER_COUNT  
LAP_CHILD_QUESTION  
LAP_CHILD_ASSIGNMENT  
PASSENGER_DOCUMENTS  
DOCUMENT_CONFIRMATION  
PAYMENT_PREFERENCE  
PAYER_CPF  
```  
  
**Objetivo:** parar de interpretar respostas curtas isoladamente.  
  
---  
  
## Etapa 3.5B — ActivePromptContext no router  
  
**Status:** Concluída.  
  
**O que faz:** usa o contexto ativo em `routeDeterministicIntent` antes dos fallbacks genéricos.  
  
**Exemplo:**  
  
```text  
contexto ativo = AVAILABILITY_OPTION_CHOICE  
cliente = "essa mesmo"  
→ SELECT_AVAILABILITY_OPTION, quando há exatamente uma opção  
```  
  
**Proteções adicionadas:**  
  
```text  
cancelamento vence active prompt  
suporte humano vence active prompt  
reagendamento vence active prompt  
opção fora da lista não avança  
confirmação genérica não escolhe opção 1 em lista múltipla  
```  
  
**Objetivo:** impedir que `UNSUPPORTED_PACKAGE` capture respostas de outra etapa.  
  
---  
  
## Etapa 3.5C — ActivePromptContext no interpreter local  
  
**Status:** Concluída.  
  
**O que faz:** melhora `InterpretStructuredTurn` com a mesma noção de última pergunta ativa.  
  
**Exemplos cobertos:**  
  
```text  
"1" após lista de opções → SELECT_AVAILABILITY_OPTION  
"só eu" após pergunta de passageiros → PASSENGER_COUNT_REPLY  
"certo" após confirmação documental → DOCUMENT_CONFIRMATION  
"sinal" após pergunta de pagamento → PAYMENT_PREFERENCE  
"pix" após pergunta integral/sinal → UNKNOWN, não PAYMENT_PREFERENCE  
```  
  
**Objetivo:** reduzir `local=UNKNOWN` em turnos curtos e aumentar agreement com OpenAI quando ela estiver certa.  
  
---  
  
## Etapa 3.5D-A — Fallback contextual por templates fechados  
  
**Status:** Concluída, monitorar em produção.  
  
**O que faz:** quando o cliente responde algo ambíguo/inválido para a última pergunta ativa, o router retorna um template contextual fechado.  
  
**Decisão arquitetural:** fallback contextual usa `TemplateName`, não `reply_text` livre.  
  
**Templates adicionados:**  
  
```text  
CONTEXT_FALLBACK_AVAILABILITY_OPTION  
CONTEXT_FALLBACK_AVAILABILITY_DATE  
CONTEXT_FALLBACK_PASSENGER_COUNT  
CONTEXT_FALLBACK_CHILD_UNDER_5  
CONTEXT_FALLBACK_LAP_CHILD_ASSIGNMENT  
CONTEXT_FALLBACK_PASSENGER_DOCUMENTS  
CONTEXT_FALLBACK_DOCUMENT_CONFIRMATION  
CONTEXT_FALLBACK_PAYMENT_PREFERENCE  
CONTEXT_FALLBACK_PAYER_CPF  
```  
  
**Proteção importante:** templates `CONTEXT_FALLBACK_*` são no-op para `canonical_state`.  
  
**Exemplos:**  
  
```text  
Bot: Qual opção você prefere?  
Cliente: ok  
→ Não consegui identificar qual opção você escolheu. Responda com o número da opção, por exemplo: 1.  
  
Bot: Você prefere integral ou sinal?  
Cliente: pix  
→ O pagamento é por PIX. Você prefere pagar o valor integral ou apenas o sinal?  
```  
  
**Objetivo:** trocar fallback genérico burro por fallback útil e seguro.  
  
---  
  
## Hotfix H-2026-06-29 — Reservation-help variant  
  
**Status:** Concluído localmente em 2026-06-29; pendente validação em produção.
  
**Sintoma em produção:**  
  
```text  
Cliente: como faço pra fazer uma reserva?  
Bot: No momento atendemos apenas viagens dos pacotes Santa Catarina e Maranhao...  
```  
  
**Causa:** `inferUnsupportedPackageQuery` interpreta o trecho após `pra` como destino:  
  
```text  
pra fazer uma reserva  
→ destino extraído: fazer uma reserva  
→ unsupported  
```  
  
**Correção esperada:**  
  
```text  
"como faço pra fazer uma reserva?"  
→ ASK_RESERVATION_ROUTE_SC  
```  

**O que mudou:**

```text
reservation-help reconhece "como faço pra fazer uma reserva?"
destination fragment "fazer uma reserva" deixa de ser tratado como destino
destino real fora de cobertura continua unsupported
```
  
**Não quebrar:**  
  
```text  
"como faço pra reservar passagem para Bahia"  
→ UNSUPPORTED_PACKAGE  
"como faço pra fazer uma reserva para Bahia"
→ UNSUPPORTED_PACKAGE
```  
  
**Arquivos alterados:**
  
```text  
apps/api/internal/chat/intent_router.go  
apps/api/internal/chat/unsupported_package.go  
apps/api/internal/chat/intent_router_test.go  
apps/api/internal/chat/interpreter_test.go  
apps/api/internal/chat/incremental_flow_test.go  
docs/EXECUTION_TRACKER.md
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test(ReservationHowToProceedHelpersRecognizeNaturalReservationHelp|InferUnsupportedPackageQueryKeepsRealDestinationInReservationHelp|IntentRouterNaturalReservationHelpStartsReservationInDiscovery|InterpretStructuredTurnNaturalReservationHelpIsAvailabilityNewRequest|ReservationHowToProceedAsksRouteToSCWithoutPassengerCollection|ReservationHelpWithUnsupportedDestinationReturnsSupportWithoutOpenAI)$'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** diff revisado; alterações restritas aos arquivos permitidos do plano; sem mudança em `Service.Reprocess`, OpenAI, tools, banco, endpoints, auto-send, infra ou n8n.

**Riscos restantes:** precisa validação em produção/homologação para confirmar que a frase real gera `ASK_RESERVATION_ROUTE_SC` com auto-send esperado.

**Próxima ação recomendada:** validar o hotfix em produção/homologação; só iniciar 3.6A com novo pedido explícito.
  
**Objetivo:** corrigir variação textual real antes de iniciar a etapa 3.6A.  
  
---  
  
# Próxima fase: LLM-first gated interpretation  
  
A partir daqui, o objetivo deixa de ser “fazer parser local cada vez maior” e passa a ser:  
  
```text  
LLM propõe interpretação.  
Validador local aceita ou rejeita.  
Backend executa somente o que passou no contrato.  
```  
  
Regra central:  
  
```text  
Regras no prompt orientam.  
Regras no código garantem.  
```  
  
---  
  
## Etapa 3.6A — Local Interpretation Validator  
  
**Status:** Concluída localmente em 2026-06-29; em review.
  
**O que faz:** cria um validador local puro para uma `StructuredInterpretation`, seja ela vinda do interpreter local ou da OpenAI.  
  
**Entrada:**  
  
```text  
StructuredInterpretation  
CanonicalConversationState  
ActivePromptContext  
history  
currentTurn  
```  
  
**Saída:**  
  
```text  
ACCEPTED ou REJECTED  
reject_reason  
fallback_template opcional  
```  
  
**Exemplos:**  
  
```text  
Bot perguntou integral/sinal  
Cliente: pix  
LLM sugere PAYMENT_PREFERENCE  
Validator rejeita: pix_is_method_not_preference  
Fallback: CONTEXT_FALLBACK_PAYMENT_PREFERENCE  
```  
  
**Objetivo:** construir o sistema imunológico antes de permitir que o LLM influencie decisão real.  

**Arquivos principais:**

```text
apps/api/internal/chat/interpreter_validation.go
apps/api/internal/chat/interpreter_validation_test.go
docs/EXECUTION_TRACKER.md
```

**O que mudou:**

```text
criado ValidateStructuredInterpretation como validador puro/local
retorna Accepted, RejectReason e FallbackTemplate
valida ActivePromptContext, fase, slots, range, documentos, pagamento, CPF e unsupported destination
rejeita intents de resposta sem active prompt confiavel
nao altera Service.Reprocess nem fluxo real de producao
```

**Exemplos cobertos:**

```text
pix quando o bot perguntou integral/sinal → rejeita com CONTEXT_FALLBACK_PAYMENT_PREFERENCE
sinal/integral no mesmo contexto → aceita PAYMENT_PREFERENCE
opcao 10 em lista com 5 opcoes → rejeita com CONTEXT_FALLBACK_AVAILABILITY_OPTION
ok quando o bot pediu data/documentos/CPF → rejeita com fallback contextual
31/02 quando o bot pediu data → rejeita com CONTEXT_FALLBACK_AVAILABILITY_DATE
CPF invalido como documento de passageiro → rejeita com CONTEXT_FALLBACK_PASSENGER_DOCUMENTS
SELECT_AVAILABILITY_OPTION sem active prompt → rejeita com fallback de opcao
como faço pra fazer uma reserva? → aceita AVAILABILITY_SEARCH / NEW_REQUEST
como faço pra reservar passagem para Bahia → rejeita availability normal com UNSUPPORTED_PACKAGE
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'TestValidateStructuredInterpretation'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** P2 corrigidos: intents answer-only sem active prompt confiavel agora rejeitam; CPF invalido em documentos de passageiro rejeita; data invalida como `31/02` rejeita em vez de aceitar por formato. Diff local revisado; somente arquivos permitidos pela etapa 3.6A foram alterados/criados. Sem alteracao em `Service.Reprocess`, OpenAI, shadow runtime, tools, banco, endpoints, auto-send, infra, n8n, vector base ou planner.

**Riscos restantes:** o validator ainda nao esta integrado ao fluxo real; proximas etapas devem validar corpus/assistencia antes de qualquer decisao operacional baseada em proposta LLM.

**Próxima etapa recomendada:** 3.6B — Corpus canônico versionado, somente com novo pedido explícito.
  
---  
  
## Etapa 3.6B — Corpus canônico versionado

**Status:** Concluída localmente em 2026-06-29; em review.

**O que faz:** cria arquivos versionados com casos canônicos estruturados, ainda sem vector DB.

**Arquivos principais:**

```text
apps/api/internal/chat/testdata/interpreter_cases.jsonl
apps/api/internal/chat/interpreter_cases.go
apps/api/internal/chat/interpreter_cases_test.go
docs/EXECUTION_TRACKER.md
```

**O que mudou:**

```text
criado corpus JSONL versionado com 27 casos canonicos
criado loader puro/local LoadInterpreterCases e LoadInterpreterCasesFromReader
loader valida JSON linha a linha, campos obrigatorios e IDs duplicados
testes garantem categorias minimas, hotfixes de producao e erros claros de JSONL
nao altera runtime, Service.Reprocess, OpenAI, tools, banco, endpoints, auto-send, infra, n8n, vector base ou planner
```

**Categorias cobertas:**

```text
reservation_help
unsupported_package
availability_date
availability_option
contextual_fallback
passenger_count
child_under_5
lap_child_assignment
passenger_documents
document_confirmation
payment_preference
payer_cpf
human_support
booking_cancel
```

**Casos principais adicionados:**

```text
"como faço pra fazer uma reserva?" → AVAILABILITY_SEARCH / ASK_RESERVATION_ROUTE_SC, proibindo UNSUPPORTED_PACKAGE
"como faço pra reservar passagem para Bahia" → UNSUPPORTED_PACKAGE
"como faço pra fazer uma reserva para Bahia" → UNSUPPORTED_PACKAGE
"06/7" em pergunta de data → AVAILABILITY_SEARCH
"31/02" em pergunta de data → CONTEXT_FALLBACK_AVAILABILITY_DATE
"essa mesmo" com uma opção → SELECT_AVAILABILITY_OPTION
"ok" com múltiplas opções → CONTEXT_FALLBACK_AVAILABILITY_OPTION
"10" em lista com 10+ passageiros → LAP_CHILD_ASSIGNMENT_ANSWER
"pix" após pergunta integral/sinal → CONTEXT_FALLBACK_PAYMENT_PREFERENCE
CPF sintético válido do pagador → PAYMENT_CREATE
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'TestInterpreterCases'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** diff local revisado; alterações restritas aos arquivos permitidos pela etapa 3.6B. Sem mudança em runtime, OpenAI, vector, planner, tools, banco, endpoints, auto-send, infra ou n8n.

**Necessidade de teste em produção:** nenhuma nesta etapa; o corpus e o loader não influenciam comportamento de produção.

**Riscos restantes:** o corpus ainda não é executado contra interpreter local, OpenAI interpreter ou validator; isso pertence à próxima etapa.

**Próxima etapa recomendada:** etapa 3.6C executada localmente em 2026-06-29; ver seção seguinte.
  
---  
  
## Etapa 3.6C — Avaliação local do corpus canônico
  
**Status:** Concluída localmente em 2026-06-29; P2 de review corrigido localmente em 2026-06-30.
  
**O que faz:** cria avaliador puro/local para rodar o corpus canônico contra:

```text
InterpretStructuredTurn
ValidateStructuredInterpretation
```

**Arquivos principais:**

```text
apps/api/internal/chat/interpreter_cases_eval.go
apps/api/internal/chat/interpreter_cases_eval_test.go
docs/EXECUTION_TRACKER.md
```

**O que mudou:**

```text
criado EvaluateInterpreterCases como avaliador local do corpus
avaliador monta CanonicalConversationState, ActivePromptContext e history sinteticos
avaliador executa interpreter local e validator local por caso
validator principal valida apenas o output real do InterpretStructuredTurn
expected_intent do corpus nao alimenta proposta sintetica para simular sucesso local
validator_probe opcional fica separado e nao altera pass/fail principal
relatorio expõe total, passed, failed, skipped e resultado por caso
casos criticos do plano ficam protegidos por testes especificos
nao altera runtime, Service.Reprocess, OpenAI, shadow runtime, tools, banco, endpoints, auto-send, infra, n8n, vector base ou planner
```

**Baseline local:**

```text
Total: 27
Passed: 22
Failed: 5
Skipped: 0
```

**Casos críticos cobertos e passando:**

```text
reservation_help_001 — "como faço pra fazer uma reserva?" não vira UNSUPPORTED_PACKAGE
unsupported_package_001 — "como faço pra reservar passagem para Bahia" preserva UNSUPPORTED_PACKAGE
availability_option_003 — "ok" em lista múltipla não seleciona opção automaticamente e aponta CONTEXT_FALLBACK_AVAILABILITY_OPTION
payment_preference_003 — "pix" após integral/sinal não aceita PAYMENT_PREFERENCE e aponta CONTEXT_FALLBACK_PAYMENT_PREFERENCE
availability_date_002 — "31/02" em prompt de data aponta CONTEXT_FALLBACK_AVAILABILITY_DATE
passenger_documents_002 — "12345678900" em documentos aponta CONTEXT_FALLBACK_PASSENGER_DOCUMENTS
```

**Falhas de baseline registradas, fora de correção nesta etapa:**

```text
unsupported_package_003 — interpreter estruturado local retorna UNKNOWN para "quero passagem para Bahia"; unsupported_package fica coberto pelo roteador deterministico, nao pelo interpreter local
availability_option_002 — interpreter seleciona "essa mesmo" com uma opção, mas validator rejeita como CONTEXT_FALLBACK_AVAILABILITY_OPTION
lap_child_assignment_001 — interpreter local ainda não interpreta "10" como LAP_CHILD_ASSIGNMENT_ANSWER neste fixture
payer_cpf_001 — interpreter local retorna UNKNOWN para CPF valido em PAYER_CPF; caso nao passa mais por proposta sintetica baseada em expected_intent
human_support_001 — HUMAN_SUPPORT é intenção do roteador determinístico, não do interpreter estruturado local
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'TestInterpreterCaseEvaluationLoadsAndEvaluatesCorpus' -v
go test -count=1 ./internal/chat -run 'TestInterpreterCase'
go test -count=1 ./internal/chat -run TestInterpreterCase -v
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** P2 corrigido em 2026-06-30. O evaluator nao usa mais `expected_intent` para fabricar proposta principal quando o interpreter local retorna `UNKNOWN`; probes sinteticos ficam separados em `validator_probe` e nao contam para pass/fail principal. Diff local restrito aos arquivos permitidos pela etapa 3.6C; evaluator e testes são puros/locais. Sem mudança em `Service.Reprocess`, runtime, OpenAI, shadow runtime, tools, banco, endpoints, auto-send, infra, n8n, vector base, embeddings, File Search ou planner.

**Necessidade de teste em produção:** nenhuma nesta etapa; o avaliador não é integrado ao fluxo real.

**Riscos restantes:** baseline agora expõe 5 falhas reais do interpreter estruturado local/validator principal que devem ser analisadas em etapa explícita de alinhamento local ou aceitas como limite atual antes de qualquer assist OpenAI.

**Próxima etapa recomendada:** revisar a correção do P2 da 3.6C; depois escolher explicitamente entre corrigir/alinhar as 5 falhas de baseline ou iniciar 3.6D — OpenAI Interpreter Assist Gated em shadow. 3.6D não foi iniciada nesta correção.

---

## Etapa 3.6D — OpenAI Interpreter Assist Gated

**Status:** Pendente.

**O que faz:** roda OpenAI interpreter como assistente avaliado pelo validator local.

**Fluxo:**
  
```text  
local interpreter roda  
OpenAI interpreter roda  
validator local avalia proposta da OpenAI  
sistema registra accepted/rejected/reason  
fluxo real continua local  
```  
  
**Objetivo:** medir quando o LLM acerta onde o local falha, sem alterar produção.  
  
---  
  
## Etapa 3.6E — Vector Base Shadow
  
**Status:** Pendente.  
  
**O que faz:** adiciona busca vetorial em shadow sobre o corpus canônico.  
  
**Fluxo:**  
  
```text  
currentTurn + state + activePrompt  
→ vector retrieval de exemplos/regras  
→ topK exemplos recuperados  
→ log/relatório  
→ não altera decisão real  
```  
  
**Objetivo:** medir se busca semântica recupera exemplos úteis antes de usar em produção.  
  
**Importante:** não colocar dados sensíveis na vector base.  
  
Proibido:  
  
```text  
CPF real  
RG real  
CNH real  
foto de documento  
PIX copia e cola  
booking_id real  
telefone completo  
nome completo real  
payload bruto do WhatsApp  
status transacional de pagamento  
assentos/datas reais como fonte de verdade  
```  
  
---  
  
## Etapa 3.6F — Vector-assisted Interpreter para UNKNOWN/baixa confiança
  
**Status:** Pendente.  
  
**O que faz:** usa vector retrieval + OpenAI interpreter somente quando o local der `UNKNOWN` ou baixa confiança.  
  
**Fluxo:**  
  
```text  
local = UNKNOWN ou baixa confiança  
→ busca exemplos vetoriais  
→ OpenAI interpreter com exemplos  
→ validator local  
→ se aceito, usar decisão  
→ se rejeitado, fallback seguro  
```  
  
**Objetivo:** melhorar interpretação real sem deixar o LLM decidir sozinho.  
  
---  
  
## Etapa 3.7A — OpenAI Interpreter Primary Gated  
  
**Status:** Futuro.  
  
**O que faz:** OpenAI interpreter vira motor principal de entendimento, mas sempre validado localmente.  
  
**Fluxo:**  
  
```text  
cliente  
→ estado canônico local  
→ ActivePromptContext local  
→ vector retrieval opcional  
→ OpenAI interpreter  
→ schema fechado  
→ validator local  
→ decisão aceita ou fallback  
```  
  
**Objetivo:** usar LLM como cérebro semântico principal sem abrir mão dos contratos locais.  
  
---  
  
## Etapa 3.7B — Local interpreter vira validator/override/fallback  
  
**Status:** Futuro.  
  
**O que faz:** o interpreter local deixa de ser cérebro principal e passa a atuar como:  
  
```text  
fast-path para casos óbvios  
override para intenções críticas  
fallback quando LLM falha  
fonte de validação cruzada  
```  
  
**Fast-paths que devem continuar locais:**  
  
```text  
cancelar  
humano/suporte  
1, 2, 3 em lista válida  
sinal/integral  
CPF válido quando bot pediu CPF  
documento claramente detectável  
```  
  
**Objetivo:** reduzir parser local inchado sem perder segurança.  
  
---  
  
# Etapas do Planner  
  
## Etapa 4.0A — Planner proposal schema  
  
**Status:** Futuro.  
  
**O que faz:** define schema fechado para o planner propor próxima ação.  
  
**Exemplo:**  
  
```json  
{  
"next_action": "ASK_TEMPLATE",  
"template_name": "ASK_PAYMENT_CHOICE",  
"reason": "booking_created_without_payment_preference"  
}  
```  
  
**Objetivo:** padronizar planos antes de executar qualquer coisa.  
  
---  
  
## Etapa 4.0B — Local Plan Validator  
  
**Status:** Futuro.  
  
**O que faz:** valida plano proposto antes de qualquer tool/template.  
  
**Validações:**  
  
```text  
fase permite ação?  
slots obrigatórios existem?  
range de opção é válido?  
booking_id existe?  
payment preference existe?  
documentos suficientes?  
tool pode ser chamada agora?  
```  
  
**Objetivo:** impedir que planner vire executor freestyle.  
  
---  
  
## Etapa 4.0C — LLM Planner shadow  
  
**Status:** Futuro.  
  
**O que faz:** roda planner LLM em shadow e compara com fluxo real.  
  
**Objetivo:** medir qualidade do plano sem executar.  
  
---  
  
## Etapa 4.0D — LLM Planner gated para ações sem side effect  
  
**Status:** Futuro.  
  
**O que faz:** permite planner influenciar apenas ações seguras:  
  
```text  
templates de esclarecimento  
fallback contextual  
perguntas de coleta  
respostas informativas  
```  
  
**Objetivo:** iniciar uso real com baixo risco.  
  
---  
  
## Etapa 4.1 — LLM Planner gated para tools críticas  
  
**Status:** Futuro distante.  
  
**O que faz:** planner pode propor tool crítica, mas executor só roda se o validador local aprovar.  
  
**Tools críticas:**  
  
```text  
booking_create  
payment_create  
booking_cancel  
document_extract  
```  
  
**Objetivo:** autonomia controlada sem virar cassino operacional.  
  
---  
  
# Etapa 5 — Autonomia controlada  
  
**Status:** Futuro distante.  
  
**O que faz:** permitir que componentes não determinísticos influenciem partes do fluxo sob validação rígida.  
  
**Objetivo:** aumentar automação sem entregar a empresa para um autocomplete com autoestima.  
  
---  
  
# Registro de hotfixes e bugs reais  
  
## H-001 — Reservation start template bloqueado por auto-send  
  
**Status:** Corrigido.  
  
**Sintoma:** “como faço uma reserva” gerava draft ou bloqueio indevido.  
  
**Correção:** template de início de reserva seguro para auto-send.  
  
---  
  
## H-002 — Data `06/07` virava UNSUPPORTED_PACKAGE  
  
**Status:** Corrigido.  
  
**Sintoma:** cliente escolhia data após lista e sistema tratava como rota fora de atendimento.  
  
**Correção:** priorizar seleção de data antes de unsupported follow-up.  
  
---  
  
## H-003 — Data flexível `06/7`, `6/7`, `6/07`  
  
**Status:** Corrigido.  
  
**Sintoma:** parser aceitava só `dd/mm`.  
  
**Correção:** aceitar `d/m`, `dd/m`, `d/mm`, `dd/mm`.  
  
---  
  
## H-004 — `essa mesmo` caía em UNSUPPORTED_PACKAGE  
  
**Status:** Corrigido.  
  
**Sintoma:** resposta contextual após lista com uma opção virava rota fora de atendimento.  
  
**Correção:** seleção contextual com última pergunta ativa.  
  
---  
  
## H-005 — `essa mesmo` em lista múltipla escolhia opção 1  
  
**Status:** Corrigido.  
  
**Sintoma:** confirmação genérica podia avançar com opção errada.  
  
**Correção:** só seleção explícita avança em lista múltipla; ambíguo vira fallback contextual.  
  
---  
  
## H-006 — Criança de colo com opção `10`  
  
**Status:** Corrigido.  
  
**Sintoma:** lista `10. Maria` não era interpretada como opção 10.  
  
**Correção:** parser de índice inicial completo em lista numerada.  
  
---  
  
## H-007 — Fallback contextual alterava `canonical_state`  
  
**Status:** Corrigido.  
  
**Sintoma:** `CONTEXT_FALLBACK_*` podia mudar fase para `ROUTE_SELECTION`.  
  
**Correção:** fallback contextual é no-op para canonical_state.  
  
---  
  
## H-008 — `como faço pra fazer uma reserva?`  
  
**Status:** Corrigido localmente em 2026-06-29; pendente validação em produção.
  
**Sintoma:** frase natural de início de reserva cai em `UNSUPPORTED_PACKAGE`.  
  
**Causa provável:** `destinationAfterLastConnector` extrai `fazer uma reserva` como destino após `pra`.  
  
**Correção aplicada:** ampliadas as frases de `reservation-help` e ignorado o fragmento de ação `fazer uma reserva` como destino, sem suprimir destino real fora de atendimento como Bahia.

**Testes executados:** `go test -count=1 ./internal/chat`; `go test -count=1 ./...`; `git diff --check`.
  
---  
  
# Regra final de arquitetura  
  
```text  
LLM pode propor entendimento.  
Código local valida contrato.  
Backend executa.  
Fallback seguro responde quando não há confiança.  
```  
  
Ou, no português executivo insuportável:  
  
```text  
LLM é motor semântico.  
Validador local é compliance.  
Backend é mesa de operações.  
```  
  
Sem compliance, a mesa opera alavancada em cima de frase ambígua. E aí nem a Priscila salva no fechamento do trimestre.
