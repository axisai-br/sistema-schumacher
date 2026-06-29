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
  
Depois da validação, se houver pedido explícito:
  
```text  
Próxima etapa arquitetural: 3.6A — Local Interpretation Validator  
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
  
**Status:** Próxima etapa arquitetural após o hotfix H-2026-06-29.  
  
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
  
---  
  
## Etapa 3.6B — Corpus canônico versionado  
  
**Status:** Pendente.  
  
**O que faz:** cria arquivos versionados com casos canônicos estruturados, ainda sem vector DB.  
  
**Exemplo:**  
  
```json  
{  
"id": "reservation_help_001",  
"current_turn": "como faço pra fazer uma reserva?",  
"phase": "DISCOVERY",  
"active_prompt_kind": "UNKNOWN",  
"expected_intent": "AVAILABILITY_SEARCH",  
"expected_turn_meaning": "NEW_REQUEST",  
"expected_template": "ASK_RESERVATION_ROUTE_SC",  
"must_not": ["UNSUPPORTED_PACKAGE"],  
"notes": "Ajuda para reserva sem destino explícito real."  
}  
```  
  
**Categorias iniciais:**  
  
```text  
reservation_help  
unsupported_package  
availability_option  
availability_date  
passenger_count  
child_under_5  
lap_child_assignment  
documents  
document_confirmation  
payment_preference  
payer_cpf  
human_support  
booking_cancel  
```  
  
**Objetivo:** transformar bugs corrigidos em conhecimento testável e reutilizável.  
  
---  
  
## Etapa 3.6C — OpenAI Interpreter Assist Gated  
  
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
  
## Etapa 3.6D — Vector Base Shadow  
  
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
  
## Etapa 3.6E — Vector-assisted Interpreter para UNKNOWN/baixa confiança  
  
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
