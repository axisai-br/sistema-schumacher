# Hotfix H-2026-07-16B — Contagem de passageiros e criança menor de 5

## Status esperado no tracker

```text
PENDENTE após H-2026-07-16A
```

## Objetivo

Corrigir o estado determinístico de passageiros sem misturar o problema com TravelQueryMeaningV2.

## Evidências de produção

```text
"eu e meus 2 filhos que são criança"
→ passenger_count persistido como 2
→ esperado: 3 viajantes

"sim o mais novo de 4 anos"
durante ASK_CHILD_UNDER_5
→ child_under_5_count_known=false
→ pergunta ASK_CHILD_UNDER_5 repetida
→ esperado: child_under_5_count=1 e avanço do fluxo
```

## Escopo autorizado

- parser/normalizador de quantidade de passageiros;
- BookingDraftContext;
- consumo de ActivePrompt `ASK_CHILD_UNDER_5`;
- cálculo de `expected_document_count`;
- transição de prompt;
- testes.

Sem alterar Travel V2 shadow, provider, corpus, booking/payment ou preço.

## Regras

- “eu e meus 2 filhos” = 3 viajantes;
- “meus 2 filhos” sozinho só vira 2 quando não houver inclusão explícita do interlocutor;
- “somos 3” = total 3;
- resposta afirmativa com idade durante `ASK_CHILD_UNDER_5` deve consumir o prompt;
- “o mais novo tem 4 anos” identifica ao menos uma criança menor de 5;
- não inventar idade/quantidade quando a frase for ambígua;
- criança não pagante continua exigindo documento conforme o contrato atual;
- não duplicar passageiro em fluxos combinados;
- não repetir a mesma pergunta após estado conhecido.

## Critérios de aceite

- total correto para adulto + filhos;
- `child_under_5_count_known=true` após confirmação inequívoca;
- avanço para documento/atribuição apropriada;
- `expected_document_count` consistente;
- sem loop;
- pagamento continua contando somente passageiros pagantes;
- H-012 permanece verde.

## Testes mínimos

```text
eu e meus 2 filhos
para mim e meu filho de 4 anos
somos 3
meus 2 filhos vão viajar
sim
sim, o mais novo tem 4 anos
uma tem 4 e outra 6
não tem criança menor de 5
```

## `/goal`

```text
/goal
Execute somente H-2026-07-16B.

Corrija a contagem relativa de passageiros e o consumo da resposta de criança menor de 5. "eu e meus 2 filhos" deve representar 3 viajantes. Durante ASK_CHILD_UNDER_5, "sim, o mais novo tem 4 anos" deve marcar uma criança menor de 5 e avançar sem repetir a pergunta.

Preserve expected_document_count, documentos de todos os viajantes, cobrança somente dos pagantes e H-012. Não alterar Travel V2, provider, corpus, payment ou booking_create. Atualize o tracker. Não commit, push ou deploy.
```

## `/review`

```text
/review
Revise somente H-2026-07-16B.

Procure contagem off-by-one, duplicação de viajante, idade não consumida, loop de ASK_CHILD_UNDER_5, expected_document_count incorreto, criança omitida dos documentos e cobrança indevida.

Confirme os cenários combinados/sequenciais, H-012, ausência de mudança em Travel V2 e informe P1/P2 e segurança para commit/smoke. Não altere arquivos.
```
