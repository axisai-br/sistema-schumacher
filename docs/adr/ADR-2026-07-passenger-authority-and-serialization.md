# ADR-2026-07 — Autoridade e serialização do estado de passageiros

## Status

H-B — EM ANDAMENTO — B1 concluída; B2 PRÓXIMA.

B1 foi concluída após review final sem P1/P2 e está segura para commit. O gate
predecessor de B2 foi satisfeito, mas B2 permanece apenas como próxima slice e
ainda não foi iniciada. B3 continua bloqueada por B2, e 3.6F-D continua
bloqueada até o fechamento integral de H-B. Esta ADR permanece como a decisão
arquitetural do B1.

Esta ADR supersede as decisões canônicas de:

- `ADR-2026-07-passenger-clarification-state.md`;
- `ADR-2026-07-passenger-state-durable-events.md`.

As duas ADRs anteriores permanecem como histórico das tentativas que levaram a
esta separação em três slices.

### Aplicação após o sétimo review

A correção mantém esta decisão e explicita três consequências operacionais:

- bootstrap sem snapshot consulta toda a sessão por artefatos estruturados,
  sem selecionar corpo e sem depender da janela do `Reprocess`;
- somente a transição canônica de entrega, registrada com status de envio e
  marcador de entrega, aplica `prompt_event`; falha, pendência, draft e retry
  pendente não são autoridade;
- todo escritor que possa substituir `chat_sessions.metadata` relê a linha sob
  `FOR UPDATE`; escritores por caminho JSONB não substituem o documento stale.

Guardrails locais `STRONG`, pedido humano e cancelamento precedem o gate de
passageiros. O mesmo gate continua proibindo LLM, shadow, extração documental e
tools quando o estado é inseguro, mas permite respostas informativas locais sem
efeito que preservem o prompt pendente.

## Contexto

O sexto review demonstrou que adicionar um snapshot e um reducer tipado não é
suficiente quando o mesmo patch continua:

- interpretando linguagem familiar com regex e listas de frases;
- inicializando o snapshot a partir de transcript limitado;
- perdendo o evento quando o draft e o outbound enviado divergem;
- consultando o `ActivePrompt` textual em vez da época persistida;
- executando LLMs ou tools antes do fail-closed;
- recuperando contagem de `tool_context.booking_create`;
- avançando booking/payment com slots desconhecidos;
- preservando dependências inválidas depois de correção;
- gravando snapshots concorrentes sem serialização por sessão.

Essas falhas pertencem a três responsabilidades distintas: fundação de estado,
interpretação de linguagem e promoção runtime. Elas não devem continuar no
mesmo slice.

## Decisão

### 1. Quem interpreta e quem reduz

```text
PassengerClarificationMeaningV1 interpreta linguagem humana.
Validator local valida contrato, época e invariantes.
Mapper aceito produz eventos estruturados.
Reducer recebe somente estado + eventos estruturados.
Backend executa somente depois de estado válido persistido.
```

O reducer nunca recebe texto, transcript, tokens, regex, listas de frases ou
`ActivePromptContext` inferido do corpo. O validator não redescobre significado
no texto e não corrige proposta semântica por parsing local.

O slice de fundação B1 não implementa interpretação de linguagem. Ele remove do
foundation as regras lexicais adicionadas pelo H-B e aceita somente eventos já
estruturados. O contrato, corpus e shadow de significado pertencem ao B2. A
influência runtime pertence exclusivamente ao B3.

### 2. Autoridade antes e depois do booking

Antes da criação do booking, a única autoridade de composição é
`PassengerClarificationStateV1`, versionada e serializada por sessão.

Depois que booking e passageiros foram persistidos, a autoridade passa a ser:

```text
booking persistido
+ passageiros persistidos
+ classificação persistida de lap child
```

O snapshot pré-booking pode permanecer como trilha de auditoria, mas não pode
sobrescrever booking/passengers persistidos nem ser reconstruído para alterar
payment.

Nunca são fontes de autoridade:

- transcript ou corpo de mensagens;
- janela histórica do `Reprocess`;
- `tool_context`;
- payload histórico de `booking_create`;
- draft não enviado;
- inferência baseada apenas em `PassengerCount == 1`;
- proposta de provider ainda não validada.

`BookingDraftContext` é uma projeção: copia o snapshot pré-booking validado e os
facts não relacionados a composição que ainda pertencem ao histórico. Ele não
extrai, corrige, reduz ou recupera contagem de passageiros.

### 3. Bootstrap legado

Uma sessão sem V1 executa no máximo um bootstrap. São aceitos somente artefatos
estruturados já persistidos e versionados, como eventos canônicos válidos e,
quando já houver booking, booking/passengers persistidos.

São proibidos no bootstrap:

- reparsear inbound;
- inferir pelo texto do prompt;
- consultar `tool_context` para recuperar contagem;
- variar o resultado conforme `LIMIT 50` ou versão do parser.

Sem evidência estrutural suficiente, o bootstrap persiste slots
`PENDING`/desconhecidos, `BootstrapCompleted=true` e exige clarification segura.

### 4. Evento do prompt efetivamente enviado

O evento de prompt acompanha o outbound efetivamente enviado. O draft pode
carregar um `pending_prompt_event`, mas não abre época.

Nos fluxos de auto-send e review controlado:

1. o outbound criado para envio recebe uma cópia canônica do `prompt_event`;
2. o envio externo ocorre sem lock de sessão aberto;
3. ao registrar o envio confiável, uma transação curta aplica o evento ao
   snapshot e marca o outbound como enviado;
4. o `event_id` é estável e a reaplicação é no-op;
5. recovery/retry usa o evento do próprio outbound, sem depender de o draft
   fonte continuar na janela histórica.

Um draft bloqueado, editado sem evento compatível ou nunca enviado não altera a
época. Um outbound confiável abre a época mesmo se o corpo deixar de ser
reconhecido por qualquer detector textual.

### 5. Época persistida como contexto

A pertinência da resposta ao slot vem do `PromptMessageID`/época persistida, não
de `evidence.ActivePrompt.Kind` inferido do texto.

Se o slot infantil aberto pertence ao prompt persistido e a proveniência do
total é `SOLO_SPEAKER`, um evento infantil positivo materializa
`ChildUnder5AddsTraveler` e sua origem. Mudanças posteriores no texto do prompt
não podem alterar essa decisão.

### 6. Correção substitui agregado completo

Uma correção aceita substitui o agregado afetado e todas as dependências que
dele derivam. Não existe correção escalar que preserve silenciosamente
`ChildUnder5AddsTraveler`, referências ou origem incompatíveis.

Exemplos:

- correção infantil para zero limpa contagem, referências, `adds_traveler` e
  origem;
- correção de `SOLO_SPEAKER` para total absoluto precisa declarar a nova
  composição completa; quando isso não for possível, invalida também a relação
  infantil dependente e produz clarification;
- `só pra mim` + criança, seguido de correção estrutural para total 2 que inclui
  a criança, resulta em dois documentos e `adds_traveler=false`, nunca três.

### 7. Fail-closed antes de trabalho externo

Estado corrompido, conflitante ou com invariantes inválidas bloqueia todo LLM e
toda tool antes de shadow, Travel V2, document extraction, booking ou payment.

No B1, slots desconhecidos/pendentes também bloqueiam os LLMs e tools hoje
existentes e produzem clarification determinística. Depois de B1 concluído, o
B2 pode executar somente o `PassengerClarificationMeaningV1` em shadow sobre
estado estruturalmente válido e fase elegível, sem mutar estado ou resposta. O
B3 pode promover somente sob seus gates específicos. Nenhuma dessas exceções
autoriza LLM genérico, Travel V2 ou tool crítica em estado inseguro.

Quando booking já existe mas qualquer slot obrigatório pré-booking permanece
desconhecido ou conflitante, a decisão é clarification/handoff seguro antes de
payment. `BookingCreated` não tem precedência sobre validação de autoridade.

### 8. Serialização por sessão

Toda atualização de `PassengerClarificationStateV1` usa uma transação curta por
sessão, com uma destas estratégias equivalentes:

- `SELECT ... FOR UPDATE` da linha de `chat_sessions`, reload do snapshot mais
  recente, redução/validação pura e persistência; ou
- revision/CAS, com update condicional e retry bounded ao detectar conflito.

O slice B1 deve escolher uma estratégia e registrá-la no diff. Em ambos os
casos:

- eventos possuem IDs estáveis;
- o estado mais recente é relido dentro da seção serializada;
- dois `Reprocess` concorrentes preservam a união dos eventos aceitos;
- retries são limitados e falham fechados ao esgotar;
- nenhuma chamada a provider, sender, LLM, tool ou rede ocorre com lock ou
  transação de sessão abertos;
- a transação não inclui espera por debounce nem processamento documental;
- a persistência do evento de prompt enviado segue a mesma disciplina.

Ledgers de IDs garantem idempotência, mas não substituem lock/CAS contra lost
update.

## Sequência de implementação

```text
H-B1  fundação de autoridade, eventos, serialização e fail-closed
  ↓ review sem P1/P2 + testes de concorrência + smoke autorizado
H-B2  PassengerClarificationMeaningV1 strict, validator, corpus e shadow
  ↓ review sem P1/P2 + evidência shadow sem influência runtime
H-B3  promoção runtime gated para prompt passageiro/criança e decisão fraca
  ↓ review sem P1/P2 + rollout/smoke autorizado
H-B umbrella concluída
  ↓
3.6F-D pode ser reavaliada para desbloqueio
```

## Invariantes

- texto não é estado;
- transcript e `tool_context` não reconstroem passageiros;
- pre-booking e post-booking possuem autoridades distintas e explícitas;
- prompt só abre época quando o outbound correspondente é confiavelmente
  enviado;
- evento enviado sobrevive à saída do draft da janela histórica;
- reducer e projeção não interpretam linguagem;
- correção substitui o agregado completo;
- slots inválidos nunca avançam booking/payment;
- chamadas externas nunca ocorrem sob lock de sessão;
- concorrência não perde eventos;
- idempotência vale entre retries, restart e múltiplas instâncias;
- payment continua derivado dos passageiros persistidos e pagantes;
- documentos continuam cobrindo todos os viajantes.

## Consequências

O H-B deixa de ser um hotfix único e passa a ser umbrella. A fundação pode
temporariamente perguntar novamente em sessões legadas sem evidência
estruturada; esse custo é preferível a fabricar composição pelo transcript.

A interpretação aberta deixa de pressionar o backend com regex, mas exige
contrato, corpus, shadow e promoção próprios. O B3 não pode ser antecipado para
recuperar comportamento antes de B1 e B2 fecharem seus gates.

## Gate atual e próxima ação

O gate B1 -> B2 está satisfeito pelo fechamento de B1 após review final sem
P1/P2. A próxima ação é preparar o commit de B1; B2 exige um novo `/goal`
explícito e não foi iniciada nesta composição. H-B permanece em andamento, B3
permanece bloqueada por B2 e 3.6F-D permanece bloqueada pelo fechamento integral
de H-B.

Esta ADR não autoriza implementação de B2, commit, push, deploy ou smoke nesta
correção documental.
