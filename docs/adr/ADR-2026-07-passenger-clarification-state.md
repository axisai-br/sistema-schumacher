# ADR-2026-07 — Estado de esclarecimento de passageiros

## Status

SUPERSEDED por
`ADR-2026-07-passenger-authority-and-serialization.md` após o sexto review.

Este documento permanece somente como histórico. Nenhuma decisão abaixo deve
ser usada como contrato canônico de implementação.

## Contexto

Quatro rodadas de review mostraram que extrair, reconciliar e aplicar evidência no
mesmo loop torna a reconstrução dependente da ordem de leitura e permite que
força sintática local substitua correções temporais. Os sintomas comprovados
incluem conflito cross-turn sem validação final, correção infantil explícita
perdendo para um zero antigo, identidades distintas colapsando e um total
absoluto igual a 1 sendo confundido com declaração solo.

O problema pertence ao estado determinístico de passageiros. Travel V2,
provider, corpus, booking_create, payment e preço permanecem fora do escopo.

## Decisão

O fluxo será dividido em quatro estágios com contratos independentes.

### 1. Extração por turno

`passenger_clarification_evidence.go` transforma somente um texto e seu contexto
em `PassengerClarificationTurnEvidence`. A evidência preserva:

- slots extraídos sem aplicar estado operacional;
- `ActivePromptContext`;
- `HistoryIndex`, origem do turno e época do active prompt derivada da posição
  do prompt no histórico;
- `CorrectionCue`;
- proveniência tipada do total de passageiros;
- referências infantis e marcadores explícitos de distinção.

A extração pode classificar a forma da evidência, mas não decide qual turno
vence, não valida a composição final e não altera `BookingDraftContext`.

### 2. Redução temporal pura

`passenger_clarification_reducer.go` expõe um reducer puro:

```text
[]PassengerClarificationTurnEvidence -> PassengerClarificationState
```

O chamador fornece evidências na ordem cronológica: histórico antigo,
histórico novo e turno atual. O reducer não depende de varredura reversa nem de
estado mutável do booking.

Cada slot reduz evidências por época mais recente do prompt, depois por turno
mais recente dentro da época e, somente dentro da mesma época, por força. Um
novo `ASK_CHILD_UNDER_5` abre uma nova fronteira infantil independentemente de
`CorrectionCue` lexical. Sem novo prompt, evidência fraca e possivelmente
correferente continua sem reduzir automaticamente uma contagem exata já
estabelecida.

### Proveniência do total

O total preserva proveniência tipada:

```text
UNKNOWN
SOLO_SPEAKER
ABSOLUTE_TOTAL
INCLUDES_SPEAKER_COMPOSITION
SUBGROUP_ONLY
```

Somente `SOLO_SPEAKER` permite `ChildUnder5AddsTraveler=true`. O reducer não
deriva essa semântica de `PassengerCount==1`. Uma criança posterior a
`ABSOLUTE_TOTAL=1` torna a composição conflitante e não acionável; uma criança
posterior a `SOLO_SPEAKER=1` produz dois viajantes/documentos.

### 3. Validação cruzada

A validação acontece uma única vez, depois da redução completa. Se
`ChildUnder5Count` e `PassengerCount` forem conhecidos e crianças excederem o
total, o estado final é não acionável e conflitante.

Nesse caso é proibido fazer clamp, aumentar o total, reduzir crianças ou
calcular documentos. O fluxo deve usar clarification segura com
`ExpectedDocumentCount=0`.

### 4. Aplicação ao booking

`BookingDraftContext` consome somente `PassengerClarificationState` já reduzido
e validado. A aplicação copia os slots e a semântica já calculada de criança que
adiciona viajante. Nenhum helper posterior pode reparsear texto ou reabrir o
histórico para reinterpretar passageiros, crianças ou documentos esperados.

## Identidade infantil

`ChildReferences` participa da redução e acumula referências distintas dentro
da mesma época. Referências explicitamente distintas preservam identidades
diferentes:

- `meu filho` e `meu outro filho` são duas identidades;
- `meu filho` repetido sem marcador de distinção é a mesma identidade;
- `meu filho` e `minha filha` permanecem distintos.

O modelo não tenta resolver identidade geral por regex. Ele conserva apenas os
marcadores explícitos necessários para impedir soma ou colapso silencioso.
Uma contagem infantil pode ser derivada de referências inequívocas, mas uma
contagem escalar posterior não substitui o conjunto já reduzido, e nenhuma
identidade ausente é inventada.

## Limite semântico futuro

Linguagem familiar aberta pertence a um `PassengerClarificationMeaning`
estruturado em slice separado. Este hotfix não adiciona nem amplia regex,
listas de expressões, `containsAnyFolded` ou parsing linguístico; ele corrige
somente a perda de informação entre evidência, reducer e
`BookingDraftContext`.

## Invariantes

- redução do mesmo histórico produz o mesmo estado;
- replay persistido não incrementa nem reduz contagens;
- estado cross-slot inválido não chama LLM, booking_create, payment ou tools;
- criança não pagante continua exigindo documento;
- cobrança permanece baseada somente nos passageiros pagantes;
- H-012, documentos e proveniência de lap child permanecem preservados.

## Consequências

O patch adiciona um arquivo de reducer e testes puros, mas reduz a quantidade de
política temporal espalhada pelo booking. A matriz local e o replay são gates
necessários, porém não substituem novo review. Commit, push, deploy e smoke não
estão autorizados nesta rodada.
