# H-2026-07-16B2 — PassengerClarificationMeaningV1 em shadow

## Status atual no tracker

```text
REVIEW_CLOSED — MERGED — DEPLOYED; PROTOCOLO CORRIGIDO APÓS REVIEW DOCUMENTAL — 2 P1 + 2 P2 FIXED_UNREVIEWED; COLETA NÃO AUTORIZADA
```

H-2026-07-22A corrigiu o bootstrap `UNKNOWN`, passou por review, foi implantado
e removeu o problema original. O smoke RED histórico posterior na transição
availability → passageiros originou H-2026-07-27A. H-A agora está
`REVIEW_CLOSED`, `MERGED`, `DEPLOYED` e `SMOKE_VERIFIED`; o smoke de 2026-08-28
atribuível ao runtime `4eb543cb` comprovou seleção materializada e avanço até
`ASK_PASSENGER_COUNT`, sem `NONE`/`SAFE_PHASE_FALLBACK`. O gate operacional de
B1 está novamente concluído. H-B2 foi concluída no commit
`a9c74852a43a24ba838decc5b2dee46eff3a03f8`, recebeu review final sem P0/P1/P2,
foi integrada pelo PR #82 em
`d7b585abcb69c3055759de82c9346762a5ebb020`, publicada como `sha-d7b585a` após
CI verde e implantada. O piloto operacional confirmou execução do shadow sem
influência no B1. A promoção ainda depende da amostra confirmatória e do
rollback proof definidos abaixo, mas o protocolo corrigido ainda requer novo
review documental independente antes de qualquer coleta. B3 permanece
bloqueada por B2, e 3.6F-D
permanece bloqueada pelo fechamento integral de H-B.

As frases `"eu e mais 2 crianças"` e `"eu e mais duas crianças"` pertencem ao
corpus de H-B2. Sua eventual influência runtime continua exclusiva de B3.

### Reavaliação arquitetural após o terceiro review

A decisão é **KEEP H-B2**. Os findings recorrentes mostraram ausência de uma
matriz factual completa no validator e de mutações adversariais no corpus, não
uma fronteira arquitetural nova. Contract, validator, corpus/evaluator e shadow
continuam no mesmo slice; nenhum plano B2a/B2b deve ser criado.

Os dois P2 do terceiro review, já `FIXED_UNREVIEWED`, eram:

1. referências infantis conhecidas podiam ser omitidas ou reclassificadas sem
   `CHILD_AGGREGATE`/`FULL_AGGREGATE`;
2. `SOLO_SPEAKER` aceitava total diferente de 1.

As seis correções das duas rodadas anteriores permanecem fechadas e devem ser
preservadas: recovery independente da identidade antiga, fixture independente
do expected, corpus sequencial, batch claim-level, aggregate contra snapshot e
turno 2 derivado do resultado real do turno 1.

### Correção após o quarto review

O quarto review preservou **KEEP H-B2** e encontrou um P1 de precedência: o
job de `PassengerMeaningV1` era criado em `PASSENGER_COLLECTION` mesmo quando
a decisão determinística já calculada no `Reprocess` era `STRONG`, permitindo
que cancelamento explícito ou pedido humano chamasse o provider fora do escopo.

Os REDs usam estado e prompt estruturalmente elegíveis e provaram uma chamada
ao provider tanto para `BOOKING_CANCEL` quanto para `HUMAN_SUPPORT`. O patch
mínimo condiciona a criação do job à força não-`STRONG` da decisão já
calculada, sem reler texto nem duplicar regras de intent. O controle flag
off/on comprova que decisão não-`STRONG` elegível continua executando o shadow
sem influência no B1.

### Correção após o quinto review

O quinto review confirmou `REVIEW_CLOSED` para o P1 STRONG e encontrou um P2:
`ABSOLUTE_TOTAL=2` ainda aceitava três referências `CHILD` distintas quando
`child_under_5.count=0` e todas as idades eram seis anos. O RED reproduziu a
aceitação com `ReasonCodes:[]`.

O patch factual conta somente IDs opacos válidos, distintos e com
`relation=CHILD`, e rejeita quando essa quantidade excede um total conhecido
com provenance `ABSOLUTE_TOTAL`. Zero, uma ou duas referências permanecem
válidas para total 2; duplicidades continuam pertencendo à invariável
`duplicate_child_reference_id`; total desconhecido não recebe a nova regra.
Nenhuma outra provenance foi ampliada.

## Objetivo

Criar `PassengerClarificationMeaningV1` com contrato strict, validator local,
corpus/evaluator e shadow, sem tools, mutação do estado canônico ou mudança
user-visible.

## Pré-condições

- [x] H-2026-07-27A revisado sem P1/P2, implantado e com smoke verde;
- [x] gate operacional de B1 novamente concluído;
- [x] serialização concorrente comprovada em PostgreSQL;
- [x] estado e eventos estruturais são autoridade;
- [x] nenhuma interpretação lexical nova permanece no foundation;
- [x] nenhum incidente operacional do B1 aberto.

Todas as pré-condições canônicas de implementação foram satisfeitas e H-B2 já
percorreu review, merge e deploy. Isso não autoriza promoção nem inicia H-B3.

## Escopo autorizado

- tipos locais do contrato `PassengerClarificationMeaningV1`;
- JSON Schema strict;
- prompt compacto e sanitizado;
- runner OpenAI com `store=false`, `tools=[]` e `tool_choice=none`;
- mapper de structured output para proposta local, sem produzir evento runtime;
- validator determinístico de contrato, época e invariantes;
- corpus sintético/anonimizado e evaluator reproduzível;
- shadow durável, idempotente, bounded e sem influência no fluxo;
- métricas e resumo sanitizado;
- flag de shadow desligada por padrão.

## Fora de escopo

- aplicar eventos ao `PassengerClarificationStateV1`;
- alterar resposta, template, state, autosend ou decisão runtime;
- executar qualquer tool;
- booking, payment, documentos ou preço;
- usar transcript/tool_context como autoridade;
- parser local para corrigir o provider;
- promover B3.

## Contrato candidato

O schema deve ser strict, versionado e sem IDs operacionais executáveis:

```text
version
source_message_id
source_prompt_event_id
passenger_count {
  status: KNOWN | UNKNOWN | CONFLICTING
  value
  provenance: SOLO_SPEAKER | ABSOLUTE_TOTAL | INCLUDES_SPEAKER_COMPOSITION | SUBGROUP_ONLY | UNKNOWN
}
child_under_5 {
  status: KNOWN | UNKNOWN | CONFLICTING
  count
  references[] { reference_id, relation, age_value, age_unit, under_5 }
}
correction {
  present
  replaces: NONE | PASSENGER_AGGREGATE | CHILD_AGGREGATE | FULL_AGGREGATE
}
needs_clarification
missing_fields[]
confidence
reason_codes[]
```

Campos opcionais devem usar nullable explícito conforme as exigências do
structured output strict. `reference_id` é opaco e limitado ao turno; não pode
ser derivado por chave lexical local.

O meaning propõe significado. Ele não calcula `ChildUnder5AddsTraveler`,
documentos, cobrança ou ação.

## Validator local

O validator recebe proposta estruturada, snapshot válido, evento do prompt e
facts mínimos. Ele verifica:

- versão, enums, ranges e shapes;
- `source_message_id` e `source_prompt_event_id` esperados;
- slot/época elegível;
- coerência de contagens, referências e correção completa;
- criança não excedendo total quando ambos forem absolutos;
- clarification/missing fields coerentes;
- confidence finita e dentro do range;
- ausência de IDs/actions/tools não autorizados.

O validator não recebe ou não consulta o conteúdo do turno para reinterpretar
palavras. Proposta semanticamente errada é rejeitada; não é reparada por regex.

### Matriz factual de passenger count

| status | provenance | value permitido |
|---|---|---|
| `KNOWN` | `SOLO_SPEAKER` | exatamente `1` |
| `KNOWN` | `ABSOLUTE_TOTAL` | `1..99` |
| `KNOWN` | `INCLUDES_SPEAKER_COMPOSITION` | `2..99` |
| `UNKNOWN` | `UNKNOWN` ou `SUBGROUP_ONLY` | `null` |
| `CONFLICTING` | `UNKNOWN` | `null` |

`ABSOLUTE_TOTAL=1` permanece válido e distinto de `SOLO_SPEAKER`.

### Matriz factual de correction coverage

| correction target | passageiro pode divergir | child count/references podem divergir |
|---|---:|---:|
| `NONE` | não | não |
| `PASSENGER_AGGREGATE` | sim | não |
| `CHILD_AGGREGATE` | não | sim |
| `FULL_AGGREGATE` | sim | sim |

Sem coverage infantil, cada referência conhecida do snapshot reaparece
exatamente uma vez e preserva ID opaco, `relation=CHILD` e `under_5`. Idade não
é comparada, pois o snapshot conserva somente `AgeKnown`. Referências novas são
permitidas quando distintas e coerentes com o agregado. Passenger correction
que abandona `SOLO_SPEAKER` não pode preservar silenciosamente uma dependência
`ChildUnder5AddsTraveler`; nesse caso exige `FULL_AGGREGATE`.

## Corpus obrigatório

O corpus deve cobrir, no mínimo:

```text
eu e meus 2 filhos
meus 2 filhos vão viajar
somos 3
só pra mim
sim, o mais novo tem 4 anos
meu filho tem 3 / meu outro filho tem 4
uma tem 4 e outra 6
meu filho de 10 meses
não tem criança menor de 5
na verdade somos 2
na verdade não tem criança menor de 5
referência repetida à mesma criança
identidades distintas em turnos separados
total, subgrupo, incerteza e correção na mesma mensagem
```

Casos adversariais incluem datas, opções, CPF/documento, valores e números que
não são composição de passageiros, além de mutações cross-turn que omitem ou
reclassificam uma identidade infantil previamente derivada.

O evaluator compara significado estruturado, não texto livre.

## Shadow

- elegível somente para prompt passageiro/criança e snapshot estruturalmente
  válido;
- `CONFLICTING`, corrompido ou invariant-invalid não chama provider;
- uma chamada por mensagem/época;
- timeout e concorrência bounded;
- nenhum resultado vira evento, state, resposta ou tool;
- persistir somente status, reason codes, campos estruturados sanitizados,
  latência e identidade idempotente;
- nunca persistir body bruto, documento, telefone ou PII no resumo;
- falha é fail-open para observabilidade, mantendo o fail-closed runtime do B1.

## Relação com o P1 lexical

O P1-01 é removido estruturalmente no B1. O B2 fornece a substituição semântica
correta sem reintroduzir interpretação local. Provas:

- `TestPassengerMeaningV1SchemaIsStrict`;
- `TestPassengerMeaningV1ValidatorDoesNotParseCurrentTurn`;
- `TestPassengerMeaningV1Corpus`;
- `TestPassengerMeaningV1ShadowHasZeroRuntimeInfluence`.

## Critérios de aceite

- schema strict e versionado;
- provider usa `store=false`, `tools=[]`, `tool_choice=none`;
- validator não interpreta texto;
- matriz `status × provenance × value` integralmente comprovada;
- referências conhecidas preservadas conforme correction coverage;
- referências `CHILD` distintas não excedem `ABSOLUTE_TOTAL` conhecido;
- corpus obrigatório passa integralmente;
- `critical_action_violation_count=0`;
- `state_mutation_count=0`;
- `tool_call_count=0`;
- flag off preserva exatamente o B1;
- shadow não muda resposta, state, template ou autosend;
- decisão determinística `STRONG` não cria job, claim nem chamada de provider;
- decisão não-`STRONG` estruturalmente elegível preserva o shadow;
- uma chamada por key;
- resumo sem PII;
- erros/timeouts não quebram o fluxo determinístico.

## Testes obrigatórios

```bash
cd apps/api
go test -count=1 ./internal/chat -run '^(TestPassengerMeaningV1ValidatorPassengerCountFactualMatrix|TestPassengerMeaningV1ValidatorRejectsPassengerProvenanceContradictingSnapshotWithoutCorrection|TestPassengerMeaningV1ValidatorReconcilesKnownChildReferencesByCorrectionCoverage|TestPassengerMeaningV1ValidatorRejectsPassengerOnlyCorrectionOfSoloChildDependency|TestPassengerMeaningV1CorpusRejectsCrossTurnChildIdentityMutation)$'
go test -count=1 ./internal/chat -run '^(TestPassengerMeaningV1ShadowSkipsDeterministicStrongDecisions|TestPassengerMeaningV1ShadowFlagOffAndOnPreserveB1Runtime)$'
go test -count=1 ./internal/chat -run '^TestPassengerMeaningV1ValidatorRejectsChildReferencesExceedingAbsolutePassengerTotal$'
go test -count=1 ./internal/chat -run 'Test.*PassengerMeaningV1.*Schema|Test.*PassengerMeaningV1.*Validator|Test.*PassengerMeaningV1.*Corpus|Test.*PassengerMeaningV1.*Shadow'
go test -count=20 ./internal/chat -run 'Test.*PassengerMeaningV1.*Corpus|Test.*PassengerMeaningV1.*Shadow'
go test -race -count=1 ./internal/chat -run 'Test.*PassengerMeaningV1.*Shadow|Test.*PassengerMeaningV1.*Idempotency'
go test -count=1 ./internal/chat
go test -count=1 ./...
gofmt -l <14 arquivos Go do manifesto H-B2>
rg -o --glob '*.go' --glob '!*_test.go' 'regexp\.MustCompile' internal/chat | wc -l  # esperado: 54
git diff --check
```

Os gates também devem comprovar
`critical_action_violation_count=0`, `state_mutation_count=0` e
`tool_call_count=0`. Esses gates locais e o review independente já foram
concluídos; eles não substituem o protocolo operacional de promoção abaixo.

## Protocolo do gate de promoção — FIXED_UNREVIEWED

Este protocolo foi congelado em 2026-09-08, **antes de qualquer nova coleta**.
Qualquer mudança em código, schema, prompt, validator, modelo, parâmetros do
provider, runner ou regra de avaliação cria uma nova versão do protocolo e
invalida a amostra confirmatória em andamento. Resultados não podem ser
descartados, substituídos ou diluídos depois de observados.

### Evidência piloto histórica

As sessões `29fe9a67-581e-49a3-8217-e90cbe2d9f94` e
`45562088-e459-4c29-89ff-d7e2cb429202` totalizam três claims `COMPLETED`, OpenAI
`valid`, `KNOWN=3`, provenance `INCLUDES_SPEAKER_COMPOSITION` e
`child_under_5=UNKNOWN`, com dois accepted e um rejected por
`incoherent_clarification_fields`; os contadores de ação crítica, mutação e tool
são zero. O comportamento user-visible permaneceu B1.

Esses três claims são evidência histórica de viabilidade. Eles ficam excluídos
de todos os numeradores e denominadores confirmatórios porque antecedem o
congelamento dos thresholds.

### Lifecycle do review documental do protocolo

O review posterior à primeira formalização encontrou quatro findings: P1-A nos
caminhos SQL, P1-B no ownership da semantic accuracy completa, P2-A na sessão
das classes 18→19 e P2-B no denominador do validator. As correções desta seção
deixam os quatro findings `FIXED_UNREVIEWED`. Isso não altera o estado
`REVIEW_CLOSED`, `MERGED` e `DEPLOYED` do código H-B2, mas o protocolo permanece
**não aprovado para coleta** até novo review documental independente fechar os
quatro findings.

### Desenho da amostra

São obrigatórios e contabilizados separadamente:

1. **O-100: 100 turnos operacionais elegíveis**, reais ou controlados em fluxo
   operacional legítimo, todos não-`STRONG`, sem fabricar produção. Este
   conjunto mede somente o lifecycle e a projeção persistida pelo digest atual;
2. **R-100: 100 execuções correspondentes no runner isolado**, com o mesmo
   runner/schema/validator/revision/modelo do runtime, provider real e
   stores/tools de teste sem acesso operacional. Este conjunto é o owner
   exclusivo da semantic accuracy completa;
3. **20 controles de inelegibilidade**, fora dos 100 elegíveis;
4. **20 propostas adversariais injetadas diretamente no validator**, fora dos
   100 casos naturais;
5. **20 turnos de rollback com a flag OFF**, separados de todos os conjuntos
   anteriores.

Cada conjunto de 100 contém 20 classes × 5 repetições. A primeira janela coleta
duas repetições de cada classe (40 casos); a segunda coleta três repetições de
cada classe (60 casos) e começa pelo menos 24 horas depois da primeira. Os dois
conjuntos nunca são somados para melhorar uma taxa. Fora do par sequencial,
cada execução usa `session_id`, `message_id`, epoch e idempotency key próprios;
replay da mesma key não conta.

Cada uma das cinco repetições 18→19 usa uma sessão dedicada ao par: duas sessões
na primeira janela e três na segunda. Os turnos 18 e 19 compartilham essa
sessão, mas usam `message_id`, `source_prompt_event_id`/epoch e idempotency key
distintos. A sessão não é reutilizada em outra repetição, janela ou conjunto.
Cada conjunto possui, portanto, 95 sessões: 90 para as outras 18 classes e cinco
para os pares. O snapshot do turno 19 deriva somente do resultado efetivo do
turno 18. Se o turno 18 falhar, não há repair, repetição substituta nem expected
injetado: o turno 19 ainda é executado com o snapshot efetivamente resultante e
a continuidade do par é marcada como falha.

Antes da primeira chamada deve ser persistido um manifesto imutável e
sanitizado contendo `protocol_version`, `case_id`, `class_id`, janela,
repetição, texto sintético no runner, snapshot/prompt de entrada, resultado
semântico esperado, elegibilidade, revision, image digest, schema/prompt
digest, validator revision, modelo e parâmetros. O expected não é enviado ao
provider.

No O-100, somente o claim e seu `summary` persistido são observáveis; não se
atribui a esse conjunto precisão de IDs, relações, idades individuais ou
continuidade de identidade. No R-100, um harness descartável executado fora do
checkout e sem alterar o digest chama o runner uma única vez, captura em memória
ou JSONL temporário sanitizado o `OpenAIPassengerMeaningV1RunResult.Proposal`
completo antes de `summarizePassengerMeaningV1ShadowProposal`, e então aplica o
schema e o validator da mesma revisão. O artefato não contém prompt, transcript,
credencial, raw provider body ou PII e é preservado somente até o review da
evidência.

### Matriz exata: 20 classes × 5 repetições por conjunto

| Classe | Entrada/cenário canônico | Resultado semântico mínimo esperado |
|---:|---|---|
| 1 | `eu e mais 2 crianças` | `KNOWN=3`, `INCLUDES_SPEAKER_COMPOSITION`, crianças presentes, idades/under-5 desconhecidos e clarification coerente |
| 2 | `eu e mais duas crianças` | igual à classe 1 |
| 3 | `eu e meus 2 filhos` | total 3 incluindo speaker e composição infantil |
| 4 | `meus 2 filhos vão viajar` | subgrupo infantil 2; total global permanece desconhecido |
| 5 | `somos 3` | `ABSOLUTE_TOTAL=3` |
| 6 | `só pra mim` | `SOLO_SPEAKER=1` |
| 7 | `sim, o mais novo tem 4 anos` | uma criança conhecida menor de 5 |
| 8 | `meu filho tem 3 / meu outro filho tem 4` | duas crianças distintas, ambas menores de 5 |
| 9 | `uma tem 4 e outra 6` | duas referências distintas; exatamente uma menor de 5 |
| 10 | `meu filho de 10 meses` | criança menor de 5 |
| 11 | `meu filho tem 59 meses` | criança menor de 5 |
| 12 | `meu filho tem 60 meses` | criança não menor de 5 |
| 13 | `não tem criança menor de 5` | agregado infantil conhecido igual a zero |
| 14 | `na verdade somos 2` | correção do agregado de passageiros para 2 |
| 15 | `na verdade não tem criança menor de 5` | correção do agregado infantil para zero |
| 16 | `na verdade somos 4; dois são filhos, mas não sei as idades` | total 4 corrigido; duas crianças; clarification de idades |
| 17 | referência repetida à mesma criança de 4 anos | preservar identidade; não duplicar referência |
| 18 | turno 1: `meu filho tem 3 anos` | criar/preservar identidade da primeira criança |
| 19 | turno 2: `meu outro filho tem 4 anos` | derivar snapshot real do turno 1, preservar a primeira e adicionar a segunda |
| 20 | `dia 12, opção 2, custa 350` durante prompt de passageiros | números não viram passenger count |

As classes 1 e 2 são os dois casos reais obrigatórios. Em **cada conjunto**,
cada uma deve acertar 5/5; juntas devem resultar em 10/10 por conjunto.

No runner isolado, a entrada da classe 19 usa o snapshot produzido pela execução
efetiva da classe 18 conforme a regra de sessão acima. Não há repair, parser de
expected ou segunda tentativa corretiva.

### Controles separados

Os 20 controles de inelegibilidade são: cinco decisões `STRONG` de
cancelamento, cinco `STRONG` de atendimento humano, cinco mensagens fora do
domínio e cinco snapshots inválidos/conflitantes. Snapshots inválidos só podem
ser exercitados no ambiente isolado. O esperado agregado é **zero job, zero
claim e zero chamada ao provider**.

As 20 propostas adversariais são quatro de cada família:

- mismatch entre `needs_clarification` e `missing_fields`;
- `source_message_id`, prompt event ou epoch incorretos;
- referências infantis distintas acima do passenger total;
- mutação de identidade sem correction coverage;
- incompatibilidade com `SOLO_SPEAKER`.

O validator deve rejeitar corretamente 20/20. Rejeição adversarial esperada é
sucesso de contenção e não entra em `validator_acceptance_rate`.

### Métricas, ownership, denominadores e thresholds congelados

`O-100` e `R-100` são avaliados separadamente. `INE-20`, `ADV-20` e `OFF-20`
continuam controles fora dos dois conjuntos naturais.

| Métrica | Owner da evidência | Fórmula | PASS |
|---|---|---|---:|
| completude operacional | O-100 | claims terminais em até 60 s / 100 turnos operacionais elegíveis | 100/100 |
| provider/schema success | O-100 e R-100, separadamente | provider sem erro e parse/contrato strict válido / 100 planejados | >=99/100 em cada conjunto |
| validator_acceptance_rate | O-100 e R-100, separadamente | `accepted / (accepted + rejected)` entre propostas naturais schema-valid efetivamente avaliadas | >=98% em cada conjunto; publicar numerador e denominador |
| acceptance ponta a ponta | O-100 e R-100, separadamente | caso terminal, schema-valid e accepted / 100 planejados | >=98/100 em cada conjunto |
| passenger accuracy | O-100 e R-100, separadamente | status, valor e provenance de passenger count corretos / 100 | >=98/100 e >=4/5 por classe, em cada conjunto |
| precisão da projeção persistida | somente O-100 | passenger, child aggregate/reference count, correction e clarification persistidos corretos / 100 | >=98/100 e >=4/5 por classe |
| semantic accuracy completa | somente R-100 | proposta completa semanticamente correta / 100 | >=98/100 e >=4/5 por classe |
| clarification accuracy | O-100 e R-100, separadamente | `needs_clarification` e conjunto exato de `missing_fields` corretos / 100 | >=98/100 e >=4/5 por classe, em cada conjunto |
| casos reais 1 e 2 | O-100 e R-100, separadamente | total/provenance, child unknown, clarification coerente e accepted | 10/10 por conjunto |
| adversariais | ADV-20 | propostas corretamente rejeitadas / 20 | 20/20 |
| false override | O-100 e OFF-20 | decisão `STRONG` ou B1 substituída pelo shadow | 0 |
| critical action violation | somente O-100 | ação crítica atribuível ao shadow | 0 |
| state mutation | somente O-100 | mutação de estado canônico atribuível ao shadow | 0 |
| tool call | somente O-100 | tools atribuíveis ao shadow | 0 |
| chamadas duplicadas | O-100 e R-100, separadamente | provider calls por mensagem/epoch/idempotency key | <=1 |
| inelegibilidade | INE-20 | job/claim/provider nos 20 controles | 0/0/0 |

A precisão da projeção persistida do O-100 compara, contra expected independente
congelado no manifesto, somente os campos realmente materializados pelo digest:
`passenger_status`, `passenger_count`, `passenger_provenance`,
`child_under_5_status`, `child_under_5_count`, `child_reference_count`,
`correction_present`, `correction_target`, `needs_clarification` e o conjunto de
`missing_fields`. O O-100 também é owner do lifecycle do claim, contadores,
idempotência, elegibilidade e prova de ausência de influência no B1.

A semantic accuracy completa é exclusiva do R-100 e inclui IDs e relações das
referências infantis, `age_value`, `age_unit`, `under_5`, continuidade de
identidade, correction coverage e os demais campos do contrato. Nenhuma métrica
do O-100 é usada como substituta dessa evidência.

No O-100, provider/schema success exige simultaneamente claim terminal,
`summary.openai.status=valid`, `summary.metrics.schema_parse_success_count=1`,
`summary.validation.status` em `{accepted,rejected}` e ausência de
`openai_schema_invalid` em `summary.validation.reason_codes`. Parseabilidade
isolada não comprova schema válido.

Forma normativa dos thresholds de tolerância zero:

```text
false_override = 0
critical_action_violation_count = 0
state_mutation_count = 0
tool_call_count = 0
provider_call_count_per_message_epoch_key <= 1
ineligible_job_count = 0
ineligible_claim_count = 0
ineligible_provider_call_count = 0
```

Para `validator_acceptance_rate`, defina `N_eval` como a quantidade de propostas
naturais schema-valid cujo status do validator seja exatamente `accepted` ou
`rejected`, e `N_accepted` como o subconjunto accepted:

```text
validator_acceptance_rate = N_accepted / N_eval
```

Skipped, erro, refusal, unparseable, schema-invalid e proposta não avaliada não
entram em `N_eval`. Eles continuam como falha nos denominadores fixos 100 de
provider/schema, acceptance ponta a ponta e accuracies aplicáveis; nunca são
removidos, substituídos ou usados para melhorar o PASS. No O-100, um caso com
erro só conta para completude se o claim realmente ficar terminal em até 60 s.
`N_eval=0` resulta em `INCONCLUSIVE`, nunca PASS. Quota incompleta também nunca
é PASS. ADV-20 não entra nesse denominador.

Na semantic accuracy completa do R-100, não se exige igualdade literal de
confidence nem de IDs opacos arbitrários; exige-se identidade consistente ao
longo da sequência. A ordem de `missing_fields` é ignorada, mas o conjunto deve
ser exato. Reason codes podem variar apenas dentro do conjunto permitido e
precisam ser coerentes com a proposta. Se o evaluator atual comparar confidence,
IDs ou ordem literalmente, a auditoria isolada usa comparação explícita deste
protocolo, sem alterar o evaluator de produção durante a coleta.

### Rejected esperado, defeitos e incidentes

- adversarial corretamente rejeitado: sucesso esperado, reportado apenas na
  métrica adversarial;
- linguagem natural válida com proposta incoerente e rejeitada: falha de
  qualidade do provider, embora o validator tenha contido o erro;
- proposta semanticamente correta rejeitada: falso negativo do validator;
- proposta incorreta aceita: falso positivo e falha semântica; nunca conta
  como sucesso;
- o piloto `incoherent_clarification_fields` não recebe exceção e só pode
  formar recorrência se a mesma causa for confirmada.

Abre-se incidente objetivo com qualquer uma das condições:

1. primeira ocorrência de critical action, mutação canônica, false override,
   tool call, vazamento de PII, chamada duplicada ao provider ou chamada em
   caso inelegível;
2. qualquer turno elegível sem claim terminal em até 60 segundos;
3. mesma família causal não crítica em duas execuções independentes, podendo o
   piloto contar uma vez somente após confirmação da causa;
4. falha de qualquer threshold congelado;
5. falha de atribuição de revision/digest/config, rollback, integridade B1 ou
   observabilidade necessária.

Um incidente permanece aberto até causa e contenção documentadas e nova amostra
versionada quando aplicável. Incidente aberto bloqueia promoção. Ausência de
erro recorrente exige as duas janelas completas, nenhuma família causal
repetida e os últimos 50 turnos elegíveis sem erro inesperado, claim perdido ou
rejeição imprópria. Ausência de tráfego não comprova ausência de recorrência.

### Observação reproduzível

Antes da coleta, registrar os resultados read-only de:

```bash
git rev-parse main origin/main
git merge-base --is-ancestor a9c74852a43a24ba838decc5b2dee46eff3a03f8 origin/main
gh api repos/joaovitormessias/SistemaSchumacher/pulls/82
gh run view 33754631690 --json headSha,status,conclusion,jobs
docker service inspect schumacher-api_schumacher-api --format '{{json .Spec.TaskTemplate.ContainerSpec.Image}}'
docker service ps --no-trunc schumacher-api_schumacher-api
```

No host operacional, extrair somente revision/digest, flag e modelo necessários;
nunca imprimir ambiente completo ou secrets. A consulta PostgreSQL é read-only,
parametrizada pelo recorte explícito do manifesto, e expande somente os claims
H-B2 armazenados em
`chat_messages.normalized_payload->'passenger_meaning_v1_shadow_claims'`:

```sql
WITH manifest AS (
  SELECT *
  FROM jsonb_to_recordset($1::jsonb) AS item(
    case_id text,
    window_id text,
    session_id uuid,
    message_id uuid,
    claim_key text
  )
), expanded AS (
  SELECT
    manifest.case_id,
    manifest.window_id,
    manifest.session_id,
    manifest.message_id,
    manifest.claim_key,
    message.created_at AS message_created_at,
    claim.value IS NOT NULL AS claim_present,
    claim.value->>'status' AS claim_status,
    claim.value->>'idempotency_key' AS idempotency_key,
    claim.value->>'source_message_id' AS source_message_id,
    claim.value->>'source_prompt_event_id' AS source_prompt_event_id,
    NULLIF(claim.value->>'claimed_at', '')::timestamptz AS claimed_at,
    NULLIF(claim.value->>'completed_at', '')::timestamptz AS completed_at,
    claim.value->'summary'->'openai' AS openai_summary,
    claim.value->'summary'->'validation' AS validation_summary,
    claim.value->'summary'->'metrics' AS metrics
  FROM manifest
  LEFT JOIN chat_messages AS message
    ON message.session_id = manifest.session_id
   AND message.id = manifest.message_id
  LEFT JOIN LATERAL (
    SELECT entry.key, entry.value
    FROM jsonb_each(
      CASE
        WHEN jsonb_typeof(
          message.normalized_payload->'passenger_meaning_v1_shadow_claims'
        ) = 'object'
        THEN message.normalized_payload->'passenger_meaning_v1_shadow_claims'
        ELSE '{}'::jsonb
      END
    ) AS entry(key, value)
    WHERE entry.key = manifest.claim_key
  ) AS claim ON TRUE
)
SELECT
  expanded.*,
  CASE
    WHEN claimed_at IS NOT NULL AND completed_at IS NOT NULL
    THEN EXTRACT(EPOCH FROM completed_at - claimed_at)
    ELSE NULL
  END AS terminal_seconds,
  (
    claim_status = 'COMPLETED'
    AND claimed_at IS NOT NULL
    AND completed_at IS NOT NULL
    AND completed_at >= claimed_at
    AND completed_at <= claimed_at + INTERVAL '60 seconds'
  ) AS terminal_within_60s
FROM expanded
ORDER BY window_id, case_id;
```

O parâmetro `$1` é o recorte imutável do manifesto do conjunto. O `LEFT JOIN`
preserva uma linha por caso mesmo quando mensagem ou claim não existem;
`claim_present=false` falha a completude. `message_created_at` serve apenas para
auditoria da mensagem. `terminal_seconds` usa exclusivamente
`completed_at - claimed_at`, e `terminal_within_60s` só pode ser verdadeiro com
ambos os timestamps presentes, `COMPLETED`, intervalo não negativo e no máximo
60 segundos. Timestamp ausente falha a métrica; timestamp que não possa ser
convertido torna a observação `INCONCLUSIVE` e abre o incidente de
observabilidade, sem inventar fallback para `created_at`.

Todos os 100 itens do manifesto são reconciliados, inclusive casos sem claim.
Campo/métrica ausente é `UNKNOWN`, nunca zero presumido. Deduplicar por
session/message/epoch/key e provar epochs distintas. Logs permitidos ficam
restritos aos eventos sanitizados `passenger_meaning_v1_shadow_*`, incluindo
`capacity_full`, `claim_failed`, `completion_failed`, `sweep_failed` e
abandonment; não coletar prompt, transcript ou PII.

Os contadores zero do claim são evidência útil, mas não sensores completos. Os
conjuntos O-100 e OFF-20 devem também comparar B1 com seu baseline em resposta,
template, autosend, eventos, estado e sequência de tools. Escrita no ledger do
shadow é permitida; mutação do estado canônico de passageiros não é. Chamada do
provider deve ser correlacionada por request ID sanitizado e idempotency key; sem correlação
suficiente, o resultado é `INCONCLUSIVE`, não zero.

### Rollback proof por flag, sem alteração de código

O rollback exige autorização operacional separada e segue esta ordem:

1. registrar revision, image digest, modelo, configuração e flag ON; concluir
   as duas janelas, drenar jobs e provar zero sessões pendentes;
2. aguardar 60 segundos de drain;
3. alterar **somente** `PassengerMeaningV1 shadow` para OFF;
4. executar reinício/redeploy controlado da mesma imagem/digest, necessário
   quando a flag é carregada por ambiente;
5. provar substituição de todas as tasks, health/readiness verdes e flag OFF
   efetivamente carregada; definir `T_off` após convergência;
6. executar 20 turnos elegíveis controlados durante pelo menos 10 minutos: cinco
   repetições de cada classe 1, 2, 9 e 13, em sessões de teste distintas;
7. desde `T_off`, provar zero novos jobs, claims ou provider calls H-B2;
8. provar B1 inalterado em resposta, estado, template, autosend e tools;
9. aguardar 60 segundos após o último turno e repetir a observação. Qualquer
   falha mantém a flag OFF e abre incidente;
10. somente após revisão e aprovação da evidência, restaurar a flag ON,
    reiniciar/redeployar a mesma imagem/digest e executar dois controles, um
    para cada frase real. Cada controle deve gerar exatamente um claim e uma
    provider call, ser accepted e preservar B1.

Claims antigos que terminem durante drain são separados de novos claims após
`T_off`. A evidência deve distinguir os períodos ON, drain, OFF e restauração.
Não é permitido restaurar ON antes da prova OFF estar registrada.

### Decisão do gate

`PASS` exige simultaneamente atribuição de revision/digest/config, review final
do código sem P0/P1/P2, review documental com P1-A/P1-B/P2-A/P2-B fechados,
corpus local verde, ambos os conjuntos confirmatórios completos e
dentro de todos os thresholds, controles inelegíveis e adversariais verdes,
zero violações, rollback OFF/ON aprovado, ausência de erro recorrente e nenhum
incidente aberto.

Qualquer threshold ou zero-tolerance violado resulta em `FAIL`. Falta de quota,
atribuição, correlação ou observabilidade resulta em `INCONCLUSIVE`. `FAIL` e
`INCONCLUSIVE` mantêm H-B3 bloqueada; nenhum deles pode ser reclassificado por
juízo subjetivo.

A única próxima ação autorizada depois desta correção é um novo `/review`
documental independente dos quatro findings. Enquanto esse review não fechar
P1-A, P1-B, P2-A e P2-B, o protocolo não está aprovado para coleta; não executar
OpenAI, runner, amostra, rollback ou H-B3.

## Gate de desbloqueio do B3

- corpus obrigatório integralmente verde;
- review sem P1/P2;
- review documental fecha P1-A, P1-B, P2-A e P2-B;
- shadow comprovadamente sem influência runtime;
- zero critical action, state mutation e tool call;
- protocolo acima definido e aprovado antes da coleta;
- dois conjuntos confirmatórios completos e aprovados nos thresholds congelados;
- evidência operacional do shadow, quando posteriormente autorizada;
- rollback por flag comprovado;
- nenhum incidente aberto.

Sem amostra/limiar aprovado, B3 permanece bloqueado mesmo com testes locais
verdes.

## `/goal`

```text
/goal
Execute somente H-2026-07-16B2 após liberação explícita do B1.

Crie PassengerClarificationMeaningV1 com schema strict, validator local,
corpus/evaluator e OpenAI em shadow. Use store=false, tools=[] e
tool_choice=none. O validator não pode interpretar o texto para corrigir o
provider.

Não aplique eventos, não altere PassengerClarificationStateV1, decisão,
resposta, template, autosend, booking, payment ou documents. Não execute tools
e não promova B3. Atualize o tracker; não faça commit, push, deploy ou smoke sem
pedido explícito.
```

## `/review`

```text
/review
Revise somente H-2026-07-16B2.

Procure schema não strict, parser lexical no validator, proposta virando evento
ou state, influência em resposta/autosend, tool call, PII no resumo, chamada
duplicada, shadow em estado conflitante e flag off divergente. Exija corpus
reproduzível, count=20, race, ./internal/chat, ./... e git diff --check.

Não altere arquivos e não libere B3 sem review limpo, métricas zero de ação
crítica/mutação/tool e gate de promoção aprovado.
```
