# ADR-2026-07 — Estado durável de passageiros por eventos

## Status

SUPERSEDED por
`ADR-2026-07-passenger-authority-and-serialization.md` após o sexto review.

Este documento permanece somente como histórico. Nenhuma decisão abaixo deve
ser usada como contrato canônico de implementação.

## Contexto

Cinco rodadas de review demonstraram que a fonte canônica não pode continuar
sendo a reinterpretação do histórico textual. `Service.Reprocess` carrega uma
janela limitada, `BookingDraftContext` reparsa mensagens, prompts só abrem época
quando uma resposta também é reconhecida e mudanças no parser alteram o replay.
Além disso, referências infantis e a decisão de criança adicionar viajante não
possuem proveniência durável.

O repositório já possui persistência atômica de `chat_sessions.metadata.memory`
junto da marcação das mensagens em `SaveReprocessSnapshot`. Portanto não há
prova de necessidade de migration: o H-B usará esse snapshot JSON versionado.

## Decisão

### Snapshot canônico

Criar `PassengerClarificationStateV1`, serializado em
`metadata.memory.passenger_clarification_state_v1`. O snapshot preserva:

- versão do contrato;
- `PassengerCount`, proveniência e estado do slot;
- `ChildUnder5Count`, estado do slot e referências estruturadas já aceitas;
- `ChildUnder5AddsTraveler` e a origem imutável dessa decisão;
- ID do prompt que abriu a época atual de cada slot;
- status `OPEN`, `PENDING`, `ANSWERED` ou `CONFLICTING`;
- reason codes fechados;
- IDs das últimas mensagens/eventos aplicados para idempotência;
- marcador de bootstrap concluído.

O snapshot, e não o texto histórico, é a autoridade após sua primeira
persistência.

### Eventos estruturais

O reducer recebe apenas estado e eventos, nunca texto:

```text
PASSENGER_PROMPT_OPENED
CHILD_PROMPT_OPENED
PASSENGER_COUNT_SET
CHILD_COUNT_SET
SLOT_CORRECTED
SLOT_INVALIDATED
```

Cada evento contém tipo, slot, ID da mensagem de origem, ID do prompt/época,
valor e proveniência tipada quando aplicáveis, reason code e os dados
estruturais mínimos já aceitos pela camada de extração.

Eventos repetidos pelo mesmo ID são no-op. A ordem de aplicação é cronológica e
o resultado precisa satisfazer `reduce(persist(state), duplicate(events)) ==
persist(state)`.

### Semântica de época e correção

Um prompt enviado abre/reset sua época independentemente de a resposta seguinte
ser reconhecida. O slot fica `OPEN`/`PENDING`, deixa de ser acionável e não pode
reutilizar resposta de época anterior.

Uma correção aceita substitui o slot inteiro. Em especial,
`CHILD_COUNT_SET=0` limpa referências, `ChildUnder5AddsTraveler`, a origem dessa
decisão e qualquer proveniência infantil anterior.

`ChildUnder5AddsTraveler` é materializado no evento que responde ao prompt
infantil aberto após uma declaração `SOLO_SPEAKER`. Menções posteriores não o
recalculam usando o prompt corrente.

### Integração no Reprocess

`Service.Reprocess` deve:

1. carregar `PassengerClarificationStateV1` da metadata da sessão;
2. se ausente, executar no máximo um bootstrap conservador e marcar
   `BootstrapCompleted=true`;
3. converter prompts ainda não aplicados e o turno atual em eventos;
4. reduzir e validar o estado;
5. incluir o novo snapshot no `memory` entregue ao
   `SaveReprocessSnapshot`, persistindo-o atomicamente com as mensagens;
6. entregar o estado pronto ao `BookingDraftContext`.

O draft produzido pelo `Reprocess` carrega seu evento de prompt no payload,
mas não abre a época enquanto permanece `AUTOMATION_DRAFT`. Depois que esse
outbound se torna confiável/enviado, o `Reprocess` seguinte consome o evento
estrutural pelo ID da mensagem fonte antes de interpretar a resposta. Assim,
um draft bloqueado ou nunca enviado não invalida estado válido, e um prompt
efetivamente enviado abre a época mesmo quando a resposta seguinte é
desconhecida.

Depois do bootstrap, nenhuma reexecução percorre o histórico para recompor
passageiros. A janela histórica pode continuar existindo para outros domínios,
mas não é fonte canônica deste estado.

### BookingDraftContext

`BookingDraftContext` recebe `PassengerClarificationStateV1` validado e apenas
copia os campos necessários. Sua varredura histórica pode continuar tratando
seleção, documentos e ferramentas, mas não extrai, corrige ou recalcula estado
de passageiros.

### Bootstrap e linguagem não suportada

O bootstrap de sessão legada é conservador: somente evidência já estruturada e
inequívoca pode preencher slot. Ausência, ambiguidade ou linguagem familiar
aberta deixa o slot desconhecido e exige clarification segura. O marcador
persistido impede novo bootstrap em cada replay.

Identidades familiares abertas deixam de ser inferidas por regex ou chaves
lexicais. Elas pertencem ao contrato futuro
`PassengerClarificationMeaningV1`; até sua implementação em slice próprio,
bloqueiam avanço sem LLM, `booking_create`, payment ou tools.

## Invariantes

- prompt novo invalida a resposta acionável da época anterior;
- correção substitui o slot inteiro;
- zero infantil limpa todo o agregado infantil;
- proveniência de `ChildUnder5AddsTraveler` sobrevive a prompts posteriores;
- estado persiste além do `LIMIT 50` e sobrevive a restart;
- mudança no parser não altera snapshot já persistido;
- reducer não recebe texto e `BookingDraftContext` não extrai passageiros do
  histórico;
- replay não reaplica eventos;
- estado desconhecido ou conflitante produz clarification segura;
- documentos continuam cobrindo todos os viajantes e pagamento somente os
  pagantes.

## Consequências

O H-B troca reconstrução conveniente por um contrato explícito de estado e
eventos. O payload JSON versionado exige decodificação defensiva e testes de
round-trip, mas evita migration e reutiliza a transação existente.

O review local e a matriz de testes continuam obrigatórios. Esta decisão não
autoriza commit, push, deploy ou smoke e não libera 3.6F-D.
