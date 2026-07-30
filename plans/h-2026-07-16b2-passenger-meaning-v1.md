# H-2026-07-16B2 — PassengerClarificationMeaningV1 em shadow

## Status atual no tracker

```text
BLOQUEADA por H-2026-07-27A / gate operacional de H-2026-07-16B1.
```

H-2026-07-22A corrigiu o bootstrap `UNKNOWN`, passou por review, foi implantado
e removeu o problema original. O smoke real posterior ficou RED na transição
availability → passageiros: oito resultados brutos foram confundidos com a
única opção apresentada, não existia `availability_prompt_event_v1` e a
confirmação `"sim"` terminou em `SAFE_PHASE_FALLBACK`, sem `booking_create`.
H-2026-07-27A está **EM CORREÇÃO APÓS REVIEW — 3 P1 + 1 P2 DE ENTREGA
TEMPORAL E PROVENIÊNCIA**. A correção local tornou o delivery monotônico,
tratou `INVALID` entregue como barreira temporal, fechou `DRAFT_REVIEW` não
aprovado e provou metadata hostil no `Repository.CreateReply` real com
PostgreSQL obrigatório. Novo review dirigido continua pendente. Este diff não
implementa contrato, validator, corpus ou shadow de B2. B3 permanece bloqueada
por B2, e 3.6F-D permanece bloqueada pelo fechamento integral de H-B.

## Objetivo

Criar `PassengerClarificationMeaningV1` com contrato strict, validator local,
corpus/evaluator e shadow, sem tools, mutação do estado canônico ou mudança
user-visible.

## Pré-condições

- H-2026-07-27A revisado sem P1/P2, implantado e com smoke verde;
- gate operacional de B1 novamente concluído;
- serialização concorrente comprovada em PostgreSQL;
- estado e eventos estruturais são autoridade;
- nenhuma interpretação lexical nova permanece no foundation;
- nenhum incidente operacional do B1 aberto.

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
não são composição de passageiros.

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
- corpus obrigatório passa integralmente;
- `critical_action_violation_count=0`;
- `state_mutation_count=0`;
- `tool_call_count=0`;
- flag off preserva exatamente o B1;
- shadow não muda resposta, state, template ou autosend;
- uma chamada por key;
- resumo sem PII;
- erros/timeouts não quebram o fluxo determinístico.

## Testes obrigatórios

```bash
cd apps/api
go test -count=1 ./internal/chat -run 'Test.*PassengerMeaningV1.*Schema|Test.*PassengerMeaningV1.*Validator|Test.*PassengerMeaningV1.*Corpus|Test.*PassengerMeaningV1.*Shadow'
go test -count=20 ./internal/chat -run 'Test.*PassengerMeaningV1.*Corpus|Test.*PassengerMeaningV1.*Shadow'
go test -race -count=1 ./internal/chat -run 'Test.*PassengerMeaningV1.*Shadow|Test.*PassengerMeaningV1.*Idempotency'
go test -count=1 ./internal/chat
go test -count=1 ./...
git diff --check
```

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
