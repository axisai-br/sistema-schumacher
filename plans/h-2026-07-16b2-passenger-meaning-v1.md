# H-2026-07-16B2 — PassengerClarificationMeaningV1 em shadow

## Status atual no tracker

```text
EM CORREÇÃO APÓS QUINTO REVIEW — 1 P2 CORRIGIDO LOCALMENTE; KEEP H-B2; AGUARDANDO NOVO `/review`
```

H-2026-07-22A corrigiu o bootstrap `UNKNOWN`, passou por review, foi implantado
e removeu o problema original. O smoke RED histórico posterior na transição
availability → passageiros originou H-2026-07-27A. H-A agora está
`REVIEW_CLOSED`, `MERGED`, `DEPLOYED` e `SMOKE_VERIFIED`; o smoke de 2026-08-28
atribuível ao runtime `4eb543cb` comprovou seleção materializada e avanço até
`ASK_PASSENGER_COUNT`, sem `NONE`/`SAFE_PHASE_FALLBACK`. O gate operacional de
B1 está novamente concluído. H-B2 foi implementada localmente em working tree
não commitado. O quinto review fechou o P1 STRONG e encontrou um P2 factual,
agora corrigido com gates locais verdes, mas ainda `FIXED_UNREVIEWED` até novo
review independente.
B3 permanece bloqueada por B2, e 3.6F-D permanece bloqueada pelo fechamento
integral de H-B.

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

Todas as pré-condições canônicas estão satisfeitas. Isso não inicia H-B2 nem
autoriza implementação sem `/goal` próprio.

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
`tool_call_count=0`. Resultado local verde mantém H-B2 em correção até novo
`/review` independente sem P1/P2.

## Gate de desbloqueio do B3

- corpus obrigatório integralmente verde;
- review sem P1/P2;
- shadow comprovadamente sem influência runtime;
- zero critical action, state mutation e tool call;
- amostra e limiares de promoção definidos e aprovados no tracker antes do B3;
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
