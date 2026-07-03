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

Hotfix local concluído em 2026-06-30; validar em produção/homologação as variações reais:

```text
Hotfix H-2026-06-30 — opção única e pagamento informativo
Entrada: "isso msm" após lista com uma opção
Esperado: SELECT_AVAILABILITY_OPTION + ASK_PASSENGER_COUNT

Entrada: "o pagamento faz logo ou só no dia mesmo?"
Esperado: PAYMENT_OPTIONS_INFO, sem payment_status

Entrada: "o que é passageiro pagante?"
Esperado: PAYING_PASSENGER_INFO

Review P1/P2 anteriores corrigidos localmente:
Quando a última pergunta é integral/sinal, respostas como "quero pagar o sinal", "vou pagar só o sinal", "sinal por passageiro pagante", "quero pagar integral" e "vou pagar tudo agora" preservam PAYMENT_PREFERENCE e avançam para payment_create.

Respostas curtas como "só o sinal", "apenas o sinal" e "o valor integral" também preservam PAYMENT_PREFERENCE no prompt ativo de integral/sinal.

Review P1 adicional corrigido localmente:
Quando a última pergunta é integral/sinal, frases reais de status como "já paguei o sinal", "o pagamento do sinal caiu?", "já paguei integral" e "confirma se meu pagamento entrou?" vencem PAYMENT_PREFERENCE e roteiam para PAYMENT_STATUS_QUERY.
```

Hotfix local concluído em 2026-06-30; deploy e smoke confirmados em produção/homologação em 2026-07-01:

```text
Hotfix H-2026-06-30B — "essa msm" e auto-send do pagamento informativo
Entrada: "essa msm" após lista com uma opção
Esperado: SELECT_AVAILABILITY_OPTION + ASK_PASSENGER_COUNT

Entrada: "essa msm" após lista com múltiplas opções
Esperado: CONTEXT_FALLBACK_AVAILABILITY_OPTION, sem selecionar opção 1

Entrada: "dia 13/07, ai o pagamento faz logo ou só no dia mesmo?"
Esperado: PAYMENT_OPTIONS_INFO + AUTO_SEND_ELIGIBLE, sem operational_claim_without_tool

Review P2 corrigido localmente:
PAYMENT_OPTIONS_INFO e PAYING_PASSENGER_INFO não viram ActivePromptPaymentPreference.
Após PAYMENT_OPTIONS_INFO, "quero reservar", rota/cidade, data ou "como faço pra reservar" não caem em CONTEXT_FALLBACK_PAYMENT_PREFERENCE.
```

Bug residual de H-2026-06-30B corrigido localmente em 2026-07-01; pendente deploy/validação em produção/homologação:

```text
Hotfix H-2026-07-01 — "essa msm" fallback após opção única real
Entrada: "essa msm" após lista atual renderizada com exatamente uma opção
Problema corrigido localmente: não cai mais em CONTEXT_FALLBACK_AVAILABILITY_OPTION quando a lista atual tem tool_context/facts atuais correspondentes
Esperado com facts atuais na mesma mensagem: SELECT_AVAILABILITY_OPTION + selected_option_index=1 + ASK_PASSENGER_COUNT

Entrada: "essa msm" após lista atual renderizada com uma opção, mas sem tool_context atual correspondente
Esperado: CONTEXT_FALLBACK_AVAILABILITY_OPTION, sem selecionar opção 1 e sem ASK_PASSENGER_COUNT

Entrada: "essa msm" após lista atual renderizada com múltiplas opções
Esperado: CONTEXT_FALLBACK_AVAILABILITY_OPTION, sem selecionar opção 1

Causa corrigida:
optionCount renderizado continua servindo para reconhecer contexto visual
seleção contextual de opção única agora exige facts atuais da mesma mensagem/lista
rendered count sem facts atuais não autoriza ASK_PASSENGER_COUNT
deriveCanonicalConversationState preserva facts mais recentes em LastToolFacts, em vez de deixar facts antigos sobrescreverem a disponibilidade atual

Review P2 adicional corrigido localmente em 2026-07-01:
InterpretStructuredTurn não autoriza SELECT_AVAILABILITY_OPTION pelo count renderizado quando a última lista não tem tool_context/facts atuais da mesma mensagem.
InterpretStructuredTurn aceita "essa msm" como selected_option_index=1 quando a lista atual tem exatamente uma opção visível e facts atuais correspondentes.
ValidateStructuredInterpretation aplica o mesmo gate de facts atuais para propostas SELECT_AVAILABILITY_OPTION vindas de shadow/runtime assist.
Validator aceita "essa msm" como selected_option_index=1 somente quando a lista atual tem exatamente uma opção visível e facts atuais correspondentes.
Listas com mais de 5 results continuam limitadas às 5 opções visíveis: índices 1..5 válidos, índice 6 rejeitado.

Arquivos alterados nesta correção:
apps/api/internal/chat/interpreter.go
apps/api/internal/chat/interpreter_validation.go
apps/api/internal/chat/interpreter_test.go
apps/api/internal/chat/interpreter_validation_test.go
docs/EXECUTION_TRACKER.md

Testes executados:
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*Structured.*Selection|Test.*ValidateStructuredInterpretation'
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*ActivePrompt|Test.*Essa.*Msm|Test.*Stale.*Facts|Test.*Capped.*Availability|Test.*Structured.*Selection|Test.*ValidateStructuredInterpretation|Test.*InterpreterCase|Test.*Payment.*Info|Test.*AutoSend'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check

Resultado do review:
P2 de gate structured/validation corrigidos localmente.

Necessidade de teste em produção/homologação:
validar "essa msm" e "1" após lista atual com uma opção e tool_context atual; validar que lista renderizada sem tool_context atual cai em CONTEXT_FALLBACK_AVAILABILITY_OPTION.

Próxima ação recomendada:
solicitar novo /review antes de commit; depois, se aprovado, preparar commit do hotfix.
```

Hotfix local concluído em 2026-07-02; pendente review/commit/deploy/validação em produção/homologação:

```text
Hotfix H-2026-07-02 — active prompt delivery mirror sem tool_context
Entrada real: "13/07" → lista com 1 opção → "essa msm"
Problema corrigido localmente: active prompt não escolhe mais o espelho BOT_AUTO_REPLY/PENDING sem tool_context como fonte canônica quando há draft source enriquecido.
Esperado: SELECT_AVAILABILITY_OPTION + selected_option_index=1 + ASK_PASSENGER_COUNT

Causa corrigida:
latestReliableAssistantMessage escolhia a mensagem BOT_AUTO_REPLY/PENDING mais recente, sem tool_context, em vez do draft AUTOMATION_SENT enriquecido referenciado por draft_message_id.

Decisão tomada:
BOT_AUTO_REPLY passa a resolver draft_message_id no próprio histórico.
Se o draft source existe, é OUTBOUND, é confiável/enviado, não é outro BOT_AUTO_REPLY e tem body equivalente, o active prompt usa o draft source.
Se não houver source confiável, o espelho não expõe tool_context/facts atuais e não autoriza seleção operacional de disponibilidade.
Facts de availability não são mergeados diretamente de BOT_AUTO_REPLY; a fonte canônica continua sendo o draft enviado.

Regressões cobertas:
BOT_AUTO_REPLY/PENDING sem tool_context + draft AUTOMATION_SENT com tool_context aceita "essa msm" como opção 1.
"5" após lista source com 1 opção cai em CONTEXT_FALLBACK_AVAILABILITY_OPTION, sem selected_option_index.
BOT_AUTO_REPLY sem draft source confiável cai em CONTEXT_FALLBACK_AVAILABILITY_OPTION, mesmo com LastToolFacts antigo.
InterpretStructuredTurn e ValidateStructuredInterpretation aplicam o mesmo gate.
Prompts não-disponibilidade com mirror/draft continuam reconhecidos: pergunta de passageiros, criança até 5 anos e payment options info informativo.

Arquivos alterados nesta correção:
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/conversation_state_machine.go
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/interpreter_test.go
apps/api/internal/chat/interpreter_validation_test.go
apps/api/internal/chat/booking_create_router_test.go
docs/EXECUTION_TRACKER.md

Testes executados:
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*ActivePrompt|Test.*Essa.*Msm|Test.*Delivery.*Mirror|Test.*BotAutoReply|Test.*Reliable.*Assistant|Test.*Structured.*Selection|Test.*ValidateStructuredInterpretation|Test.*Booking.*Selection|Test.*Payment.*Info|Test.*AutoSend'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check

Resultado do review:
Revisão local do diff sem achados P1/P2. Solicitar /review antes de commit.

Necessidade de teste em produção/homologação:
validar o fluxo real "13/07" → lista com 1 opção persistida como draft AUTOMATION_SENT + mirror BOT_AUTO_REPLY/PENDING → "essa msm".
validar que BOT_AUTO_REPLY sem draft source confiável não seleciona disponibilidade.

Não houve alteração em:
OpenAI runtime assist
OpenAI shadow/schema/runner
vector base
embeddings
File Search
planner
banco/migrations
infra
n8n
booking_create
payment_create
booking_cancel
document_extract
payment_status

Próxima ação recomendada:
solicitar /review antes de commit; depois, se aprovado, preparar commit do hotfix e validar em produção/homologação após deploy.
```

3.6F/vector/File Search/planner continuam não iniciados.
  
Etapas 3.6A, 3.6B e 3.6C executadas localmente em 2026-06-29; P2 do review da 3.6C corrigido localmente em 2026-06-30 antes de qualquer uso no fluxo real.

```text
Etapa 3.6C — Avaliação local do corpus canônico
Baseline local atual após H-2026-06-30: 30 casos avaliados; 22 passaram; 8 falharam; 0 pulados
Produção não mudou
```

Etapa 3.6D executada localmente em 2026-06-30; produção continua sem promoção da OpenAI para decisão real.

```text
OpenAI interpreter continua shadow
ValidateStructuredInterpretation avalia a proposta OpenAI em shadow
Payload structured_interpreter_shadow registra openai_validation
Runtime real, tools, canonical_state, auto-send, planner e vector base não foram promovidos/alterados
```

Etapa 3.6E executada localmente em 2026-07-01; produção depende de habilitar `CHAT_OPENAI_INTERPRETER_ASSIST_ENABLED`.

```text
OpenAI Interpreter Runtime Assist Gated para UNKNOWN/baixa confiança
Sem vector base, sem File Search e sem planner
Determinístico, tools/templates locais e fallback seguro continuam vencendo
Proposta OpenAI só influencia se confidence >= 0.70 e passar por ValidateStructuredInterpretation
booking_create, payment_create, booking_cancel, document_extract e payment_status continuam bloqueados para proposta OpenAI

Review P2 restante corrigido localmente:
confidence gate agora é aplicado antes de qualquer fallback user-visible; shadow + assist reutilizam uma única chamada ao provider OpenAI por turno.
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
"isso msm" após lista com uma opção → SELECT_AVAILABILITY_OPTION / ASK_PASSENGER_COUNT.
"essa msm" após lista com uma opção → SELECT_AVAILABILITY_OPTION / ASK_PASSENGER_COUNT.
"essa msm" após lista com múltiplas opções → CONTEXT_FALLBACK_AVAILABILITY_OPTION.
"o pagamento faz logo ou só no dia mesmo?" → PAYMENT_OPTIONS_INFO.
"dia 13/07, ai o pagamento faz logo ou só no dia mesmo?" → PAYMENT_OPTIONS_INFO / AUTO_SEND_ELIGIBLE.
"o que é passageiro pagante?" → PAYING_PASSENGER_INFO.
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

## Etapa 3.5E — Out-of-turn informational interruptions

**Status:** Concluída localmente em 2026-07-02; P2 de perguntas de pagamento de um lado só corrigido localmente; P2 de prioridade do active prompt no `Reprocess` corrigido localmente; P2 restantes de guardrails/SkipReason corrigidos localmente em 2026-07-03; P2 de `document_extract` attempted com `handled=false` corrigido localmente em 2026-07-03; pendente review, commit, deploy e validação em produção/homologação.

**O que mudou:** criada camada determinística para dúvidas informativas laterais durante prompts ativos de reserva. A ordem efetiva fica: guardrails críticos, resposta clara ao active prompt, dúvida informativa fora de turno, fallback contextual.

**Comportamento antes:** em `ASK_PASSENGER_COUNT`, perguntas como `ai o pagamento faz logo ou só no dia mesmo?`, `paga agora?`, `paga no dia?`, `precisa pagar agora?`, `tem que pagar agora?`, `pode pagar no embarque?` ou `quais documentos precisa?` podiam ser consumidas pelo bloco de continuação de passageiros e reemitir a pergunta, sem responder a dúvida.

**Comportamento depois:** durante `ASK_PASSENGER_COUNT`, `ASK_CHILD_UNDER_5`, `ASK_PASSENGER_DOCUMENTS`, `DOCUMENT_CONFIRMATION`, `PAYMENT_PREFERENCE` e `PAYER_CPF`, dúvidas sobre pagamento, passageiro pagante, documentos, criança, bagagem, embarque e contato de suporte usam template informativo fechado e anexam `Para continuar:` com o prompt pendente. O `canonical_state` não é alterado por esses templates.

**Arquivos principais:**

```text
apps/api/internal/chat/out_of_turn_info.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/response_realizer.go
apps/api/internal/chat/service.go
apps/api/internal/chat/interpreter_shadow.go
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/agent.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/response_realizer_test.go
apps/api/internal/chat/availability_draft_test.go
apps/api/internal/chat/interpreter_shadow_test.go
docs/EXECUTION_TRACKER.md
```

**Templates/intents adicionados:**

```text
DOCUMENT_REQUIREMENTS_INFO
CHILD_POLICY_INFO
BAGGAGE_INFO
BOARDING_INFO
HUMAN_SUPPORT_INFO

DOCUMENT_REQUIREMENTS_INFO_QUESTION
CHILD_POLICY_INFO_QUESTION
BAGGAGE_INFO_QUESTION
BOARDING_INFO_QUESTION
HUMAN_SUPPORT_INFO_QUESTION
```

**Regressões cobertas:**

```text
ASK_PASSENGER_COUNT + "ai o pagamento faz logo ou só no dia mesmo?"
→ PAYMENT_OPTIONS_INFO + lembrete de CONTEXT_FALLBACK_PASSENGER_COUNT
→ sem availability_search, booking_create, payment_create, OpenAI assist ou chamada LLM/document_extract
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

ASK_PASSENGER_COUNT + "paga agora?"
ASK_PASSENGER_COUNT + "paga no dia?"
ASK_PASSENGER_COUNT + "precisa pagar agora?"
ASK_PASSENGER_COUNT + "tem que pagar agora?"
ASK_PASSENGER_COUNT + "pode pagar no embarque?"
→ PAYMENT_OPTIONS_INFO + lembrete de CONTEXT_FALLBACK_PASSENGER_COUNT
→ sem fallback de ASK_PASSENGER_COUNT
→ sem availability_search, booking_create, payment_create, OpenAI assist ou chamada LLM/document_extract
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

ASK_PASSENGER_COUNT + "quais documentos precisa?"
→ DOCUMENT_REQUIREMENTS_INFO + lembrete de CONTEXT_FALLBACK_PASSENGER_COUNT
→ sem availability_search, booking_create, payment_create, OpenAI assist ou chamada LLM/document_extract
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

Após resposta informativa com "Para continuar:", "só eu" continua sendo PASSENGER_COUNT_REPLY.

ASK_PASSENGER_COUNT + "só eu, paga agora?"
ASK_PASSENGER_COUNT + "só pra mim, paga no dia?"
ASK_PASSENGER_COUNT + "apenas eu, pagamento faz logo?"
ASK_PASSENGER_COUNT + "é só pra mim, pode pagar no embarque?"
→ PASSENGER_COUNT_REPLY vence PAYMENT_OPTIONS_INFO
→ fluxo continua para ASK_CHILD_UNDER_5 sem pedir o cliente repetir "só eu"
→ sem availability_search, booking_create, payment_create ou chamada LLM/document_extract
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info, porque o branch out-of-turn não é elegível

ASK_PAYMENT_CHOICE + "quero cancelar, paga agora?"
→ BOOKING_CANCEL vence PAYMENT_OPTIONS_INFO
→ sem payment_create
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

ASK_PAYMENT_CHOICE + "quero falar com atendente, paga agora?"
→ HUMAN_HANDOFF vence PAYMENT_OPTIONS_INFO
→ sem payment_create
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

ASK_PASSENGER_COUNT + "paga agora? posso levar uma moto?"
→ UNSUPPORTED_CARGO vence PAYMENT_OPTIONS_INFO
→ sem availability_search, booking_create ou payment_create
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

ASK_PASSENGER_COUNT + "paga agora?"
→ PAYMENT_OPTIONS_INFO com lembrete de CONTEXT_FALLBACK_PASSENGER_COUNT
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

ASK_PASSENGER_DOCUMENTS + imagem com legenda "paga agora?"
→ document_extract vence o shortcut informativo
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

ASK_PASSENGER_DOCUMENTS + imagem com legenda "paga agora?" + document_extract FAILED/handled=false
→ shortcut out-of-turn fica bloqueado por documentCollectionMediaTurn/documentAttempted
→ não emite PAYMENT_OPTIONS_INFO
→ registra tool_call document_extract FAILED com DOCUMENT_EXTRACT_ERROR
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

CHAT_INTENT_ROUTER_ENABLED=false + ASK_PASSENGER_COUNT + "paga agora?"
CHAT_TEMPLATE_REALIZER_ENABLED=false + ASK_PASSENGER_COUNT + "paga agora?"
→ shortcut out-of-turn fica desabilitado
→ não emite PAYMENT_OPTIONS_INFO
→ shadow OpenAI não usa SkipReason=deterministic_out_of_turn_info

PAYMENT_PREFERENCE + "vou pagar o sinal"
→ PAYMENT_PREFERENCE preservado.

PAYMENT_PREFERENCE + "quero pagar integral"
→ PAYMENT_PREFERENCE preservado.

PAYMENT_PREFERENCE + "pode pagar só o sinal?"
→ PAYMENT_OPTIONS_INFO com lembrete de PAYMENT_PREFERENCE.

ASK_PASSENGER_COUNT + "já paguei"
ASK_PASSENGER_COUNT + "pagamento caiu?"
→ PAYMENT_STATUS_QUERY preservado.

"qual telefone do suporte?"
→ HUMAN_SUPPORT_INFO

"quero falar com atendente"
→ HUMAN_HANDOFF preservado.
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'TestServiceOutOfTurnInfoDocumentMediaDoesNotRecordShadowSkip|TestServiceOutOfTurnInfoDocumentMediaFailureDoesNotUseShortcut|TestServiceOutOfTurnInfoShortcutRequiresFinalRouterDecision|TestOutOfTurnInfoDuringPassengerCountDoesNotCallTools|TestPassengerCountAnswerWithPaymentQuestionDoesNotUseOutOfTurnShortcut'
go test -count=1 ./internal/chat -run 'TestServiceOutOfTurnInfoShortcutRequiresFinalRouterDecision|TestServiceOutOfTurnInfoDocumentMediaDoesNotRecordShadowSkip|TestOutOfTurnInfoDuringPassengerCountDoesNotCallTools|TestPassengerCountAnswerWithPaymentQuestionDoesNotUseOutOfTurnShortcut|TestIntentRouterOutOfTurn|TestIntentRouterPassengerCountAnswerWinsOverOutOfTurnPaymentQuestion|TestRunStructuredInterpreterShadowSkipReasonDoesNotCallOpenAI'
go test -count=1 ./internal/chat -run 'TestIntentRouterPassengerCountAnswerWinsOverOutOfTurnPaymentQuestion|TestPassengerCountAnswerWithPaymentQuestionDoesNotUseOutOfTurnShortcut|TestOutOfTurnInfoDuringPassengerCountDoesNotCallTools'
go test -count=1 ./internal/chat -run 'TestIntentRouter.*OutOfTurn|TestIntentRouterOneSidedPayment|TestIntentRouterPaymentStatusWins|TestIntentRouterPaymentPreference|TestOutOfTurnInfoDuringPassengerCountDoesNotCallTools|TestRunStructuredInterpreterShadow'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review local:** P2 restantes corrigidos. O `service.go` não usa mais `buildOutOfTurnInfoDecision` isolado como prova de que o shortcut venceu; ele pré-computa a decisão completa com `routeDeterministicIntent` e só permite o shortcut quando `CHAT_INTENT_ROUTER_ENABLED` e `CHAT_TEMPLATE_REALIZER_ENABLED` estão habilitados, a decisão final tem `Source=deterministic_out_of_turn_info`, `Action=template`, template informativo e realização local sem LLM. O gate antecipado também exige `!documentCollectionMediaTurn` e `!documentAttempted`, além de `!documentHandled`, para impedir que imagem/documento em `ASK_PASSENGER_DOCUMENTS` com caption como `paga agora?` vire `PAYMENT_OPTIONS_INFO` depois de tentativa de `document_extract` com `handled=false`/falha. O `SkipReason=deterministic_out_of_turn_info` foi removido nesta etapa para evitar observabilidade falsa quando outro handler posterior vence, como `document_extract` em imagem com legenda de pagamento. Assim, guardrails de cancelamento, handoff humano e carga não suportada vencem antes de PAYMENT_OPTIONS_INFO, respostas claras ao active prompt continuam vencendo dúvidas laterais, flags de rollback desabilitam o shortcut e turnos documentais tentados permanecem no fluxo documental. A mudança não altera OpenAI schema/prompt/runner, vector base, embeddings, File Search, planner, banco/migrations, infra, n8n, booking_create, payment_create, booking_cancel ou document_extract.

**Riscos restantes:** detecção é determinística por frases e pode não cobrir todas as variações reais de dúvidas laterais; templates de bagagem/embarque são deliberadamente conservadores e podem exigir ajuste de texto após validação operacional; pedidos explícitos de humano continuam interrompendo o fluxo por guardrail e não recebem lembrete do prompt.

**Necessidade de teste em produção/homologação:** validar `ASK_PASSENGER_COUNT → paga agora?/paga no dia?/precisa pagar agora?/tem que pagar agora?/pode pagar no embarque? → só eu`; validar `ASK_PASSENGER_COUNT → só eu, paga agora?` e `ASK_PASSENGER_COUNT → só pra mim, paga no dia?`; validar `PAYMENT_PREFERENCE → pode pagar só o sinal?`; validar que `já paguei` e `pagamento caiu?` continuam status; validar que `ASK_PAYMENT_CHOICE → quero cancelar, paga agora?` não vira PAYMENT_OPTIONS_INFO; validar que `ASK_PAYMENT_CHOICE → quero falar com atendente, paga agora?` continua handoff; validar que `ASK_PASSENGER_COUNT → paga agora? posso levar uma moto?` continua carga não suportada; validar em homologação `ASK_PASSENGER_DOCUMENTS → imagem/documento com legenda paga agora?` tanto com extração bem-sucedida quanto com falha de extração.

**Próxima etapa recomendada:** solicitar `/review` antes de commit; depois, se aprovado, preparar commit da etapa 3.5E.

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

## Hotfix H-2026-06-30 — Seleção de opção única e pagamento informativo

**Status:** Concluído localmente em 2026-06-30; pendente validação em produção.

**Motivo:** dois bugs reais de produção antes da etapa 3.6D:

```text
1. Cliente responde "isso msm" para lista com uma única opção e recebia fallback de opção.
2. Cliente pergunta "o pagamento faz logo ou só no dia mesmo?" e a dúvida comercial podia virar PAYMENT_STATUS_QUERY.
```

**O que mudou:**

```text
confirmações contextuais como "isso msm", "isso mesmo", "sim", "certo" e "pode ser" selecionam opção 1 somente quando há exatamente uma opção
múltiplas opções + "ok/sim/certo/isso mesmo/isso msm" continuam em fallback contextual e não selecionam opção 1
criado template PAYMENT_OPTIONS_INFO com explicação fixa de integral/sinal
criado template PAYING_PASSENGER_INFO com definição fixa de passageiro pagante
dúvidas comerciais de pagamento roteam para template fechado sem cair no payment_status genérico
consultas reais de status como "paguei", "já paguei o sinal", "o pagamento do sinal caiu?", "já paguei integral", "confirma se meu pagamento entrou?", "pagamento aprovado?", "qual o status do pagamento?" e "já caiu?" continuam PAYMENT_STATUS_QUERY
review P1 anterior corrigido: respostas afirmativas ao prompt ativo de integral/sinal são priorizadas antes dos templates PAYMENT_OPTIONS_INFO/PAYING_PASSENGER_INFO
review P2 corrigido: respostas curtas como "só o sinal", "apenas o sinal" e "o valor integral" também viram PAYMENT_PREFERENCE antes dos templates informativos
review P1 adicional corrigido: status real de pagamento vence PAYMENT_PREFERENCE durante ActivePromptPaymentPreference, mesmo mencionando "sinal" ou "integral"
perguntas reais no prompt ativo, como "como funciona o pagamento?", "paga agora ou no embarque?" e "o que é passageiro pagante?", continuam recebendo templates informativos
templates informativos não alteram canonical_state
```

**Arquivos alterados:**

```text
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/response_realizer.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/response_realizer_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/chat_flow_guardrails_test.go
apps/api/internal/chat/testdata/interpreter_cases.jsonl
docs/EXECUTION_TRACKER.md
```

**Observação de escopo:** `chat_flow_guardrails_test.go` foi ajustado apenas para remover a expectativa antiga de `PAYMENT_METHODS` em `"quais formas de pagamento?"`, porque o plano do hotfix passou essa frase para `PAYMENT_OPTIONS_INFO`. Não houve alteração em `Service.Reprocess`, OpenAI, shadow runtime, validator, tools, `payment_create`, `payment_status` executor, `booking_create`, banco, endpoints, auto-send, infra, n8n, vector base, planner ou 3.6D.

**Corpus canônico:**

```text
adicionados availability_option_single_isso_msm
adicionados payment_info_now_or_boarding
adicionados paying_passenger_definition
baseline local após novos casos: 30 total; 22 passaram; 8 falharam; 0 pulados
as novas falhas de payment_info/paying_passenger são esperadas no evaluator 3.6C porque o hotfix é no roteador determinístico, não no interpreter local
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*Payment.*Info|Test.*PayingPassenger|Test.*InterpreterCase'
go test -count=1 ./internal/chat -run 'TestGuardrailPaymentAmountChoiceStillAllowsPaymentCreate'
go test -count=1 ./internal/chat
go test -count=1 ./...
go test -count=1 ./internal/chat -run 'TestInterpreterCaseEvaluationLoadsAndEvaluatesCorpus' -v
git diff --check
```

Após correção do review P2, executados novamente:

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*Payment.*Info|Test.*PayingPassenger|Test.*InterpreterCase'
go test -count=1 ./internal/chat -run 'TestGuardrailPaymentAmountChoiceStillAllowsPaymentCreate'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

Após correção do review P1 adicional de status vs preferência, executados novamente:

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*Payment.*Info|Test.*PayingPassenger|Test.*PaymentPreference|Test.*PaymentStatus|Test.*Availability.*Option|Test.*InterpreterCase'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** P1/P2 anteriores e P1 adicional corrigidos em 2026-06-30. O roteamento agora prioriza frases reais de status antes da resposta ao `ActivePromptPaymentPreference`, e `detectActivePromptPaymentPreferenceReply` também rejeita essas frases como preferência. Respostas diretas como "só o sinal", "apenas o sinal", "o valor integral", "quero pagar o sinal" e "quero pagar integral" continuam `PAYMENT_PREFERENCE`; perguntas comerciais reais continuam em templates informativos. Diff local revisado; alterações restritas ao hotfix. Múltiplas opções continuam protegidas contra seleção automática; status real de pagamento continua `PAYMENT_STATUS_QUERY`; dúvidas comerciais usam template fechado e não chamam tool de payment_status no fluxo testado antes de reserva.

**Necessidade de teste em produção:** sim. Validar em produção/homologação:

```text
lista com uma opção → "isso msm" → ASK_PASSENGER_COUNT
"o pagamento faz logo ou só no dia mesmo?" → PAYMENT_OPTIONS_INFO
"o que é passageiro pagante?" → PAYING_PASSENGER_INFO
"sinal"/"só o sinal"/"apenas o sinal"/"quero pagar o sinal"/"vou pagar só o sinal"/"sinal por passageiro pagante" após pergunta integral/sinal → PAYMENT_PREFERENCE + payment_create
"integral"/"o valor integral"/"quero pagar integral"/"vou pagar tudo agora" após pergunta integral/sinal → PAYMENT_PREFERENCE + payment_create
"paguei"/"já paguei o sinal"/"o pagamento do sinal caiu?"/"já paguei integral"/"confirma se meu pagamento entrou?"/"pagamento aprovado?"/"já caiu?" continuam fluxo de status real
```

**Riscos restantes:** frases comerciais muito diferentes ainda podem cair em fallback/LLM; o corpus 3.6C agora registra casos informativos que o interpreter local não cobre, então não usar esse baseline como bloqueio do hotfix determinístico.

**Próxima ação recomendada:** validar H-2026-06-29 e H-2026-06-30 em produção/homologação; retomar 3.6D somente após validação explícita.

---

## Hotfix H-2026-06-30B — "essa msm" e auto-send do PAYMENT_OPTIONS_INFO

**Status:** Concluído localmente em 2026-06-30; P2 de review corrigido localmente; deploy/smoke confirmado em produção/homologação em 2026-07-01. Bug residual identificado em 2026-07-01 na variação real com lista atual renderizada sem `tool_context` confiável/facts antigos; tratado no Hotfix H-2026-07-01.

**Motivo:** dois bugs reais observados em produção após os hotfixes anteriores:

```text
1. Cliente respondeu "essa msm" em lista com uma única opção e caiu em fallback de opção.
2. Cliente perguntou "dia 13/07, ai o pagamento faz logo ou só no dia mesmo?"; o template PAYMENT_OPTIONS_INFO foi gerado, mas ficou como AUTOMATION_DRAFT por operational_claim_without_tool.
```

**O que mudou:**

```text
looksLikeContextualAvailabilitySelection reconhece "essa msm", "esse msm" e "esta msm" como confirmações contextuais fechadas
essas confirmações selecionam opção 1 somente no caminho já protegido por optionCount == 1
múltiplas opções + "essa msm"/"isso msm"/"sim"/"ok"/"certo" continuam em CONTEXT_FALLBACK_AVAILABILITY_OPTION e não selecionam opção 1
PAYMENT_OPTIONS_INFO fica elegível para auto-send por comparação normalizada exata com paymentOptionsInfoReply
textos dinâmicos com rota, data, horário, disponibilidade ou preço sem tool continuam bloqueados por operational_claim_without_tool
review P2 corrigido: paymentOptionsInfoReply e payingPassengerInfoReply não são inferidos como ActivePromptPaymentPreference
ASK_PAYMENT_CHOICE continua sendo inferido como ActivePromptPaymentPreference
após PAYMENT_OPTIONS_INFO, próximos turnos de reserva/rota/data não caem no fallback de preferência de pagamento
```

**Arquivos alterados:**

```text
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/agent.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/response_realizer_test.go
docs/EXECUTION_TRACKER.md
```

**Corpus canônico:** não alterado. O evaluator 3.6C não mede auto-send, e o fixture atual de opção única já tem limitação conhecida para representar corretamente listas com uma única opção.

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*Payment.*Info|Test.*AutoSend|Test.*InterpreterCase'
go test -count=1 ./internal/chat -run 'Test.*ActivePrompt|Test.*Payment.*Info|Test.*AutoSend|Test.*Availability.*Option|Test.*InterpreterCase'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** P2 corrigido localmente. O texto fechado `PAYMENT_OPTIONS_INFO` continua auto-send eligible, mas deixa de virar prompt ativo de preferência de pagamento; `PAYING_PASSENGER_INFO` também permanece informativo. Prompts reais como `ASK_PAYMENT_CHOICE` continuam `ActivePromptPaymentPreference`. Diff local restrito ao hotfix. Não houve alteração em `Service.Reprocess`, OpenAI primary, OpenAI shadow validation da 3.6D, `interpreter_shadow.go`, vector base, embeddings, File Search, planner, banco, endpoints, infra, n8n, `booking_create`, `payment_create`, `payment_status` ou `document_extract`. A exceção de auto-send é estreita para o texto fechado `paymentOptionsInfoReply`.

**Necessidade de teste em produção:** concluída em 2026-07-01, conforme confirmação do usuário. Fluxo real validado:

```text
13/07
essa msm
dia 13/07, ai o pagamento faz logo ou só no dia mesmo?
```

Esperado:

```text
"essa msm" após lista com uma opção → ASK_PASSENGER_COUNT
"dia 13/07..." → PAYMENT_OPTIONS_INFO com AUTO_SEND_ELIGIBLE
lista com múltiplas opções + "essa msm" → fallback contextual, sem selecionar opção 1
após PAYMENT_OPTIONS_INFO, "quero reservar"/rota/cidade/"13/07"/"como faço pra reservar" não caem em CONTEXT_FALLBACK_PAYMENT_PREFERENCE
```

**Resultado do smoke/deploy:** PR #36 confirmado mergeado na `main` (`7dabc80158ec8abb8cef2d6c7a087eb45720ec76`, contendo `33d26adf3e0be143812dd5fd8d96b6b1beaf710b`). Usuário confirmou deploy do passo anterior e teste do fluxo em produção/homologação em 2026-07-01, com `essa msm` selecionando a opção única, pergunta de passageiros, `PAYMENT_OPTIONS_INFO` enviado automaticamente e `quero reservar` sem cair em `CONTEXT_FALLBACK_PAYMENT_PREFERENCE`.

**Riscos restantes:** variações novas fora das frases fechadas ainda podem cair em fallback ou LLM; a exceção de auto-send depende do texto estático `paymentOptionsInfoReply`, então alteração futura nesse template deve manter os testes de política.

**Próxima ação recomendada:** iniciar 3.6E Runtime Assist Gated, sem vector base, File Search, planner, banco, infra ou n8n.

---

## Hotfix H-2026-07-01 — "essa msm" fallback após opção única real

**Status:** Concluído localmente em 2026-07-01; pendente deploy e validação em produção/homologação.

**Motivo:** bug real observado após o H-2026-06-30B:

```text
13/07
essa msm
dia 13/07, ai o pagamento faz logo ou só no dia mesmo?
```

Na variação real, `"essa msm"` após uma lista de disponibilidade com exatamente uma opção ainda podia cair em `CONTEXT_FALLBACK_AVAILABILITY_OPTION` em vez de `SELECT_AVAILABILITY_OPTION + ASK_PASSENGER_COUNT`.

**Causa:** o roteamento determinístico dependia de `tool_context`/`LastToolFacts` para saber `optionCount == 1`. Quando a última mensagem do bot tinha a lista renderizada com uma opção, mas sem `tool_context` confiável, e/ou quando facts antigos de disponibilidade tinham múltiplas opções, o contador efetivo podia virar `0` ou `>1`. Além disso, `deriveCanonicalConversationState` podia deixar tool facts antigos sobrescreverem os mais recentes.

**O que mudou:**

```text
availabilityOptionCountFromRenderedPrompt conta opções numeradas no corpo renderizado da última mensagem de disponibilidade
availabilityOptionCountFromMessage continua expondo o count visual para reconhecer ActivePromptContext, incluindo tool_context atual capado ao limite renderizado
latestReliableAssistantMessage centraliza a mesma mensagem outbound confiável usada pelo ActivePromptContext
currentAvailabilitySelectionPromptContext usa essa mensagem confiável e amarra a seleção operacional ao tool_context.availability_search da mesma mensagem
AUTOMATION_DRAFT, AUTOMATION_REVIEWED e AUTOMATION_PENDING invisíveis ao cliente são ignorados como fonte de lista ativa
AUTOMATION_SENT continua confiável mesmo quando carrega mode antigo de draft no payload normalizado
rendered count sem facts atuais correspondentes não autoriza SELECT_AVAILABILITY_OPTION nem ASK_PASSENGER_COUNT
InterpretStructuredTurn também rejeita SELECT_AVAILABILITY_OPTION quando o prompt renderizado não tem facts atuais correspondentes
ValidateStructuredInterpretation usa o mesmo gate de facts atuais e a mesma mensagem confiável do router/structured local
deriveCanonicalConversationState percorre o histórico do mais antigo para o mais recente
mergeToolFactsIntoCanonicalState sobrescreve campos/facts com valores não vazios mais recentes
deriveCanonicalConversationState ignora availability_search de AUTOMATION_DRAFT/AUTOMATION_REVIEWED/AUTOMATION_PENDING ao popular canonical_state.LastToolFacts
canonical_state.LastToolFacts[availability_search] permanece apontando para a lista enviada ao cliente quando há draft/reviewed/pending posterior com facts invisíveis
BookingDraftContext também ignora availability_search desses outbounds invisíveis para não pré-preencher rota/trip_id com lista não vista
findLatestAvailabilityContext reutiliza o mesmo filtro e não usa availability_search de AUTOMATION_DRAFT/AUTOMATION_REVIEWED/AUTOMATION_PENDING
parseBookingCreateInput/resolveBookingCreateSelection resolvem trip_id/board_stop_id/alight_stop_id pela lista visível/confiável, sem contaminação de draft invisível posterior
"essa msm" após lista renderizada com 1 opção e facts atuais correspondentes seleciona option_index=1 e pergunta quantidade de passageiros
"essa msm" após lista renderizada com 1 opção sem facts atuais correspondentes cai em fallback contextual, sem selecionar opção 1
"essa msm" após lista renderizada com múltiplas opções continua em fallback contextual, sem selecionar opção 1
rascunho invisível posterior não autoriza opção não vista e não invalida seleção válida da lista já enviada ao cliente
respostas explícitas como "1" ou "5" continuam selecionando listas atuais em que availability_search trouxe mais linhas do que as 5 renderizadas
```

**Arquivos alterados:**

```text
apps/api/internal/chat/active_prompt_context.go
apps/api/internal/chat/booking_draft_context.go
apps/api/internal/chat/booking_create_router.go
apps/api/internal/chat/conversation_state_machine.go
apps/api/internal/chat/interpreter.go
apps/api/internal/chat/interpreter_validation.go
apps/api/internal/chat/intent_router.go
apps/api/internal/chat/active_prompt_context_test.go
apps/api/internal/chat/booking_create_router_test.go
apps/api/internal/chat/conversation_state_machine_test.go
apps/api/internal/chat/incremental_flow_test.go
apps/api/internal/chat/intent_router_test.go
apps/api/internal/chat/interpreter_test.go
apps/api/internal/chat/interpreter_validation_test.go
apps/api/internal/chat/tool_router_test.go
docs/EXECUTION_TRACKER.md
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'TestFindLatestAvailabilityContextIgnoresInvisibleAvailabilityFacts|TestParseBookingCreateInputIgnoresInvisibleAvailabilityFactsWhenResolvingSelection'
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*ActivePrompt|Test.*Essa.*Msm|Test.*Stale.*Facts|Test.*Capped.*Availability|Test.*Structured.*Selection|Test.*ValidateStructuredInterpretation|Test.*Reliable.*Assistant|Test.*Canonical.*Availability|Test.*DeriveCanonical.*Availability|Test.*InterpreterCase|Test.*Payment.*Info|Test.*AutoSend'
go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*ActivePrompt|Test.*Essa.*Msm|Test.*Stale.*Facts|Test.*Capped.*Availability|Test.*Structured.*Selection|Test.*ValidateStructuredInterpretation|Test.*Reliable.*Assistant|Test.*Canonical.*Availability|Test.*DeriveCanonical.*Availability|Test.*FindLatestAvailability|Test.*BookingCreate.*Availability|Test.*ParseBookingCreate.*Availability|Test.*InterpreterCase|Test.*Payment.*Info|Test.*AutoSend'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** review final sem bloqueios. A seleção contextual por lista renderizada agora exige vínculo com facts atuais da mesma mensagem confiável usada pelo ActivePromptContext; rendered count sem facts atuais não autoriza `ASK_PASSENGER_COUNT`; stale availability facts não podem ser usados para avançar seleção visual atual. `AUTOMATION_DRAFT`, `AUTOMATION_REVIEWED` e `AUTOMATION_PENDING` posteriores à lista enviada são ignorados no gate operacional, no `canonical_state`, no `BookingDraftContext` e no lookup compartilhado `findLatestAvailabilityContext`; assim `parseBookingCreateInput`/`resolveBookingCreateSelection` resolve a opção aceita contra a mesma lista visível/confiável e não contra um draft posterior. `AUTOMATION_SENT` permanece confiável mesmo quando reaproveita `mode: AUTOMATION_DRAFT` do draft no payload normalizado, preservando compatibilidade com mensagens já enviadas pela automação. O count de facts atuais considera apenas opções visíveis futuras, respeitando o cap renderizado de 5, então uma busca atual com mais resultados no payload continua selecionável por índices renderizados válidos. O interpretador estruturado e o validator também aplicam o gate de facts atuais pela mesma mensagem confiável e não retornam/aceitam `SELECT_AVAILABILITY_OPTION` para confirmação contextual com lista rendered-only/stale facts. Revisão final confirmou que os filtros solicitados, `go test -count=1 ./internal/chat`, `go test -count=1 ./...` e `git diff --check` passam. Não houve alteração em OpenAI runtime assist, OpenAI shadow, schema/prompt/runner, vector base, embeddings, File Search, planner, banco/migrations, infra, n8n, `booking_create`, `payment_create`, `booking_cancel`, `document_extract` ou `payment_status`.

**Necessidade de teste em produção:** sim. Validar em produção/homologação:

```text
"essa msm" após lista com exatamente 1 opção e tool_context atual correspondente → SELECT_AVAILABILITY_OPTION + ASK_PASSENGER_COUNT
"essa msm" após lista com exatamente 1 opção sem tool_context atual correspondente → CONTEXT_FALLBACK_AVAILABILITY_OPTION, sem selecionar opção 1
"essa msm" após lista com múltiplas opções → CONTEXT_FALLBACK_AVAILABILITY_OPTION, sem selecionar opção 1
lista enviada com 1 opção + AUTOMATION_DRAFT/AUTOMATION_REVIEWED/AUTOMATION_PENDING posterior invisível → "essa msm" e "1" selecionam a opção enviada
lista enviada com 1 opção + AUTOMATION_DRAFT posterior com 5 opções invisíveis → "5" cai em CONTEXT_FALLBACK_AVAILABILITY_OPTION, sem selected_option_index=5
"dia 13/07, ai o pagamento faz logo ou só no dia mesmo?" → PAYMENT_OPTIONS_INFO + AUTO_SEND_ELIGIBLE
após PAYMENT_OPTIONS_INFO, "quero reservar" não cai em CONTEXT_FALLBACK_PAYMENT_PREFERENCE
```

**Riscos restantes:** se o texto renderizado de disponibilidade mudar para um formato sem linhas numeradas (`1.`, `1)` ou `1 -`), o contador visual não será inferido pelo corpo; se a mensagem de disponibilidade for enviada sem `tool_context` atual, o sistema deve preferir fallback seguro em vez de avançar reserva. A confiabilidade da lista ativa depende de `ProcessingStatus`/`mode` continuarem distinguindo mensagem enviada de rascunho invisível. Deploy e smoke não foram feitos nesta execução.

**Próxima ação recomendada:** preparar commit do hotfix determinístico; depois validar em produção/homologação somente os cenários listados acima após deploy controlado. Não iniciar 3.6F/vector/File Search/planner sem pedido explícito.

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

**Baseline local atual após H-2026-06-30:**

```text
Total: 30
Passed: 22
Failed: 8
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
availability_option_single_isso_msm — caso canônico do hotfix fica registrado, mas o fixture atual do evaluator infere lista múltipla para essa frase e não representa a condição real de opção única
lap_child_assignment_001 — interpreter local ainda não interpreta "10" como LAP_CHILD_ASSIGNMENT_ANSWER neste fixture
payment_info_now_or_boarding — dúvida comercial de pagamento é coberta pelo roteador determinístico/template, não pelo interpreter local 3.6C
paying_passenger_definition — definição comercial é coberta pelo roteador determinístico/template, não pelo interpreter local 3.6C
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

**Próxima etapa recomendada:** revisar a 3.6D executada localmente; depois escolher explicitamente entre etapa intermediária de relatório/observabilidade da validação OpenAI em shadow, alinhamento das falhas de baseline locais, ou 3.6E. Não iniciar 3.6E/3.7 sem pedido explícito.

---

## Etapa 3.6D — OpenAI Interpreter Assist Gated

**Status:** Concluída localmente em 2026-06-30; em review.

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

**Arquivos principais:**

```text
apps/api/internal/chat/interpreter_shadow.go
apps/api/internal/chat/interpreter_shadow_test.go
apps/api/internal/chat/handler_test.go
docs/EXECUTION_TRACKER.md
```

**O que mudou:**

```text
RunStructuredInterpreterShadow agora registra um bloco openai_validation
propostas OpenAI válidas passam por ValidateStructuredInterpretation usando currentTurn, history, canonical state e ActivePromptContext inferido/fornecido
resultado accepted/rejected/skipped fica apenas no payload de shadow
schema estrutural inválido, OpenAI disabled, shadow disabled, erro OpenAI sem proposta parseável e resultado ausente viram validation skipped
output OpenAI parseável, com enums válidos, mas rejeitado semanticamente pelo runner passa pelo validator local e vira accepted/rejected
decisão real do bot continua vindo do fluxo atual
resultado da validação OpenAI não aciona tool, não escolhe template real e não altera canonical_state
```

**Campos adicionados ao payload `structured_interpreter_shadow`:**

```json
{
  "openai_validation": {
    "status": "accepted|rejected|skipped",
    "accepted": true,
    "reject_reason": "",
    "fallback_template": ""
  }
}
```

**Casos cobertos:**

```text
accepted: GREETING em saudação
accepted: AVAILABILITY_SEARCH / NEW_REQUEST para "como faço pra fazer uma reserva?"
accepted: PAYMENT_PREFERENCE sinal após pergunta integral/sinal
rejected: PAYMENT_PREFERENCE pix após pergunta integral/sinal, com CONTEXT_FALLBACK_PAYMENT_PREFERENCE
rejected: SELECT_AVAILABILITY_OPTION para "ok" sem active prompt confiável
rejected: AVAILABILITY_SEARCH normal para destino Bahia, com UNSUPPORTED_PACKAGE
rejected: PASSENGER_DOCUMENTS_PROVIDED para CPF inválido em documentos
rejected no caminho real do runner: safety.executes_tool=true
rejected no caminho real do runner: PAYMENT_PREFERENCE pix após pergunta integral/sinal
rejected no caminho real do runner: SELECT_AVAILABILITY_OPTION sem lista/contexto confiável
skipped: shadow disabled
skipped: OpenAI disabled
skipped no caminho real do runner: schema estrutural inválido
skipped: erro OpenAI
skipped: resultado OpenAI ausente
```

**Testes executados:**

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*Shadow.*Validation|Test.*OpenAI.*Validation|Test.*ValidateStructuredInterpretation'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** P2 corrigido. Quando o runner OpenAI retorna `ErrOpenAIStructuredInterpreterInvalidOutput` com `result.Validation.Interpretation` parseável e enums válidos, o shadow agora valida essa proposta com `ValidateStructuredInterpretation` e registra `openai_validation.status=rejected` com `reject_reason`/`fallback_template` quando aplicável. Schema estrutural inválido continua `skipped/openai_schema_invalid`. Diff local revisado; alterações restritas à etapa 3.6D. Não houve promoção para OpenAI primary. Não houve alteração em decisão real do bot, execução de tools, `booking_create`, `payment_create`, `document_extract`, `booking_cancel`, endpoints, banco/migrations, auto-send, infra, n8n, vector base, embeddings, File Search, planner ou etapas 3.6E/3.7.

**Necessidade de teste em produção:** nenhuma para decisão operacional, porque a etapa só adiciona observabilidade no payload de shadow. Se shadow estiver habilitado em homologação/produção, validar apenas que `structured_interpreter_shadow.openai_validation` aparece sem alterar resposta, tools ou auto-send.

**Riscos restantes:** o relatório agregado atual ainda não expõe métricas específicas de `openai_validation`; se a próxima decisão depender de taxa de accepted/rejected por intent, criar etapa explícita de observabilidade antes de qualquer uso operacional.

**Próxima etapa recomendada:** 3.6E foi solicitada explicitamente e executada localmente em 2026-07-01; ver seção seguinte. Não iniciar OpenAI primary, vector base, planner ou 3.7 sem novo pedido explícito.
  
---  
  
## Etapa 3.6E — OpenAI Interpreter Runtime Assist Gated sem vector

**Status:** Concluída localmente em 2026-07-01; review P1/P2 corrigido localmente; pendente novo review e decisão operacional de habilitação.

**Pré-condição cumprida:** H-2026-06-30B teve PR #36 mergeado na `main` e o usuário confirmou deploy/smoke em produção/homologação em 2026-07-01 com o fluxo:

```text
13/07
essa msm
dia 13/07, ai o pagamento faz logo ou só no dia mesmo?
quero reservar
```

**O que faz:** permite que o OpenAI structured interpreter influencie o runtime real apenas quando o caminho local não resolveu com segurança:

```text
local structured interpreter = UNKNOWN ou confidence < 0.70
deterministic router/tools/templates/fallback seguro não resolveram
→ OpenAI structured interpreter propõe interpretação
→ ValidateStructuredInterpretation valida localmente
→ conversão segura para template fechado/continuação textual/pergunta segura
→ fallback seguro se rejeitado
```

**Arquivos principais:**

```text
apps/api/internal/shared/config/config.go
apps/api/internal/chat/interpreter_shadow.go
apps/api/internal/chat/openai_interpreter_assist.go
apps/api/internal/chat/openai_interpreter_assist_test.go
apps/api/internal/chat/service.go
apps/api/internal/chat/response_realizer_test.go
docs/EXECUTION_TRACKER.md
```

**O que mudou:**

```text
criado OpenAI Interpreter Runtime Assist Gated
adicionada flag CHAT_OPENAI_INTERPRETER_ASSIST_ENABLED, desligada por padrão
assist só é considerado se local = UNKNOWN ou local confidence < 0.70
deterministicDecision, deterministicToolHandled, deterministicBookingHandled e documentCollectionMediaTurn bloqueiam o assist
runner OpenAI estruturado existente continua com store=false e tools=[]
confidence gate OpenAI é aplicado antes de qualquer fallback runtime user-visible
se confidence < 0.70, o assist retorna rejected/openai_confidence_below_threshold com fallback_template vazio
proposta OpenAI parseável com enums válidos e confidence >= 0.70 passa por ValidateStructuredInterpretation antes de qualquer fallback runtime
propostas com safety side effects, baixa confiança OpenAI, schema inválido ou erro não executam decisão
shadow + assist habilitados no mesmo turno reutilizam o resultado do shadow e fazem no máximo uma chamada ao provider OpenAI
metadata openai_interpreter_assist é gravada no draft para accepted, rejected e skipped
reasons brutos da OpenAI não são persistidos; metadata mantém apenas campos controlados e controlled_reason_codes do backend
```

**Conversões permitidas nesta etapa:**

```text
AVAILABILITY_SEARCH → nunca executa availability/pricing por proposta OpenAI; input completo é rejected/openai_assist_tool_action_not_allowed
AVAILABILITY_SEARCH de início de reserva → somente template seguro de pergunta de rota/data, sem tool call
SELECT_AVAILABILITY_OPTION → somente com active prompt de opção e índice validado
PASSENGER_COUNT_REPLY → continuação textual/template de reserva
LAP_CHILD_ASSIGNMENT_ANSWER → continuação textual/template de reserva, sem booking_create
```

**Bloqueios explícitos:**

```text
booking_create não pode ser acionado por proposta OpenAI
payment_create não pode ser acionado por proposta OpenAI
booking_cancel não pode ser acionado por proposta OpenAI
document_extract não pode ser acionado por proposta OpenAI
payment_status não pode ser acionado por proposta OpenAI
availability_search não pode ser acionado por proposta OpenAI
pricing_quote não pode ser acionado por proposta OpenAI
sem vector base
sem embeddings
sem File Search
sem planner
sem banco/migrations
sem infra
sem n8n
```

**Testes executados:**

```bash
cd apps/api
gofmt -w internal/chat/interpreter_shadow.go internal/chat/openai_interpreter_assist.go internal/chat/openai_interpreter_assist_test.go internal/chat/service.go
go test -count=1 ./internal/chat -run 'Test.*OpenAI.*Assist|Test.*Interpreter.*Assist|Test.*StructuredInterpreter.*Shadow|Test.*ValidateStructuredInterpretation|Test.*IntentRouter|Test.*Payment.*Info|Test.*Availability.*Option|Test.*AutoSend'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

**Resultado do review:** P1/P2 corrigidos localmente; P2 restantes corrigidos em 2026-07-01. Proposta OpenAI aceita não gera `Action == "tool"` nem aciona availability/pricing. Propostas de baixa confiança agora são rejeitadas por `openai_confidence_below_threshold` antes de qualquer fallback user-visible, com `fallback_template` vazio, mesmo que o validator pudesse rejeitar com fallback contextual. Quando `CHAT_OPENAI_INTERPRETER_SHADOW_ENABLED` e `CHAT_OPENAI_INTERPRETER_ASSIST_ENABLED` estão ambos habilitados, o runtime reutiliza o resultado OpenAI do shadow no assist e evita segunda chamada ao provider no mesmo turno. Metadata `skipped` continua persistida mesmo com `considered=false`; reasons livres da OpenAI não são persistidos e seguem substituídos por códigos controlados do backend. Diff revisado localmente; integração continua depois do roteador determinístico e do safe phase fallback, antes do JSON/free-form LLM. Determinístico continua vencendo. OpenAI assist não adiciona vector base, embeddings, File Search, planner, function tools ou execução direta de tools críticas. A metadata não salva texto bruto do cliente, CPF/RG/CNH, telefone, PIX, booking_id real ou payload bruto.

**Necessidade de teste em produção:** sim, apenas se a flag `CHAT_OPENAI_INTERPRETER_ASSIST_ENABLED` for habilitada. Validar que drafts com `openai_interpreter_assist.status` accepted/rejected/skipped não executam tools críticas nem availability/pricing por proposta OpenAI e que fallback seguro vence rejeições do validator.

**Riscos restantes:** com a flag desligada, produção não muda. Com a flag ligada, o assist pode chamar OpenAI em turnos `UNKNOWN`/baixa confiança quando shadow estiver desligado; quando shadow e assist estiverem ligados juntos, a chamada é reutilizada. A utilidade real fica limitada a templates/perguntas seguras e continua sem execução de tools por proposta OpenAI. Métricas agregadas específicas do assist ainda não foram adicionadas ao endpoint de relatório. Não houve vector base, embeddings, File Search, planner, banco, migrations, infra, n8n ou tools críticas acionadas por OpenAI.

**Próxima etapa recomendada:** pedir novo `/review` desta 3.6E corrigida antes de commit. Depois decidir explicitamente entre observabilidade agregada do runtime assist, alinhamento das falhas locais do corpus 3.6C ou uma nova etapa de vector shadow; não iniciar vector/File Search/planner sem novo pedido explícito.
  
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

## H-009 — Status real no prompt integral/sinal

**Status:** Corrigido localmente em 2026-06-30; pendente validação em produção.

**Sintoma:** durante `ActivePromptPaymentPreference`, frases como "já paguei o sinal" ou "o pagamento do sinal caiu?" podiam ser aceitas como `PAYMENT_PREFERENCE` porque mencionavam "sinal" ou "integral".

**Correção aplicada:** `PAYMENT_STATUS_QUERY` passou a vencer a resposta de preferência, e o helper de preferência ativa rejeita frases de status antes de aceitar `sinal`/`integral`.

**Testes executados:** `go test -count=1 ./internal/chat -run 'Test.*Payment.*Info|Test.*PayingPassenger|Test.*PaymentPreference|Test.*PaymentStatus|Test.*Availability.*Option|Test.*InterpreterCase'`; `go test -count=1 ./internal/chat`; `go test -count=1 ./...`; `git diff --check`.

---

## H-010 — `essa msm` em opção única renderizada caía em fallback

**Status:** Corrigido localmente em 2026-07-01; pendente deploy e validação em produção/homologação.

**Sintoma:** `"essa msm"` após lista atual com exatamente uma opção podia cair em `CONTEXT_FALLBACK_AVAILABILITY_OPTION`.

**Causa:** `optionCount` não era inferido do corpo renderizado da última mensagem de disponibilidade, facts antigos de `availability_search` podiam sobrescrever facts mais recentes em `LastToolFacts`, e a primeira correção pós-count permitia seleção operacional usando somente o count renderizado sem facts atuais correspondentes. O ajuste inicial também comparava o count renderizado com o total bruto do payload, rejeitando listas atuais capadas, e o interpretador estruturado ainda podia aceitar confirmação contextual com facts antigos.

**Correção aplicada:** contador determinístico de opções numeradas renderizadas na última mensagem de disponibilidade; seleção contextual de opção única exige `tool_context.availability_search` atual na mesma mensagem/lista; count atual usa apenas opções visíveis futuras e respeita o cap renderizado de 5; rendered count sem facts atuais cai em fallback seguro também no interpretador estruturado; facts mais recentes passam a vencer em `deriveCanonicalConversationState`.

**Testes executados:** `go test -count=1 ./internal/chat -run 'TestIntentRouterSelectsExplicitOptionFromCappedCurrentAvailabilityFacts|TestIntentRouterDoesNotSelectEssaMsmFromRenderedSingleAvailabilityOptionWithStaleFacts|TestIntentRouterSelectsEssaMsmFromRenderedSingleAvailabilityOptionWithCurrentFacts|TestIntentRouterDoesNotSelectEssaMsmFromRenderedMultipleAvailabilityOptions|TestAvailabilityOptionEssaMsmRenderedSingleOptionWithStaleFactsUsesFallback|TestInterpretStructuredTurnDoesNotSelectRenderedSingleAvailabilityOptionWithStaleFacts|TestSelectAvailabilityOptionContextualConfirmationsAskPassengerCount|TestDeriveCanonicalConversationStateKeepsLatestAvailabilityFacts|TestInferActivePromptContextReadsAvailabilityOptionCountFromRenderedPrompt'`; `go test -count=1 ./internal/chat -run 'Test.*Availability.*Option|Test.*ActivePrompt|Test.*Essa.*Msm|Test.*Stale.*Facts|Test.*InterpreterCase|Test.*Payment.*Info|Test.*AutoSend'`; `go test -count=1 ./internal/chat`; `go test -count=1 ./...`; `git diff --check`.

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
