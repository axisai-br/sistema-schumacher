# Casos de produção anonimizados

> Fonte textual para corpus, testes e novas sessões. Não contém CPF, telefone, documento, segredo ou nome completo.

## Regra de uso

Cada caso deve preservar:

```text
contexto anterior
mensagem atual
resposta observada
estado/facts observados
comportamento esperado
owner da correção
```

As imagens originais são evidência auxiliar. O corpus deve usar transcrições anonimizadas, não depender de OCR ou do print.

## C-01 — Papel de localidade

- Contexto: cliente consulta viagem SC↔MA.
- Entrada: `saindo de Seara`
- Falha observada: Seara tratada como destino ou origem perdida.
- Esperado: `Seara/SC` com papel `ORIGIN`; perguntar somente destino/data ausente.
- Owner: 3.6F-D/I.

## C-02 — Preferência temporal

- Entrada: `a que tiver mais perto`
- Falha observada: pedido de data `dd/mm`.
- Esperado: `DateMode=EARLIEST_AVAILABLE`; lookup futuro read-only e confirmação.
- Owner: 3.6F-D/G.

## C-03 — Cobertura de rota

- Entrada: `passa em Santa Cecília?`
- Falha observada: tabela pública ou pacote não suportado.
- Esperado: proposta `RouteCoverage`; consulta read-only. Sem prova, encaminhar suporte.
- Owner: 3.6F-D/H.

## C-04 — Referência de opção

- Contexto: lista atual com uma ou mais opções.
- Entradas: `1`, `esta opção`, `essa aí`, `está aí`
- Falha observada: seleção não reconhecida ou pedido repetido.
- Esperado: INDEX/DEICTIC somente contra facts atuais confiáveis; clarification quando ambígua.
- Owner: 3.6F-D/I.

## C-05 — Poltrona específica

- Contexto: opção de viagem visível.
- Entrada: `quero reservar uma poltrona`
- Esperado: distinguir `BOOK_TRAVEL` de `CHOOSE_SPECIFIC_SEAT`.
- Para assento específico: suporte oficial + continuação contextual da reserva.
- Proibido: handoff real, seleção automática ou `booking_create`.
- Owner: 3.6F-D/F.

## C-06 — Institucional e acknowledgement

- Entradas: `de que cidade é a empresa?`, `meu Deus`, `obrigado`, `entendi`
- Falha observada: repetição de tabela ou unsupported.
- Esperado: tópico institucional fechado ou acknowledgement sem tool, preservando prompt pendente.
- Owner: 3.6F-D/F.

## C-07 — Passageiros

- Entrada: `eu e meus 2 filhos que são criança`
- Observado: `passenger_count=2`.
- Esperado: 3 viajantes.
- Owner: H-2026-07-16B.
- Fora do contrato TravelQueryMeaningV2 atual.

## C-08 — Criança menor de 5

- Active prompt: `ASK_CHILD_UNDER_5`.
- Entrada: `sim o mais novo de 4 anos`
- Observado: estado permaneceu desconhecido e a pergunta foi repetida.
- Esperado: `child_under_5_count=1`, estado conhecido e avanço.
- Owner: H-2026-07-16B.
- Fora do contrato TravelQueryMeaningV2 atual.

## C-09 — Shadow V2 operacional

- Flag V2 ativa.
- Mensagens reais sem claim.
- Recovery emitindo `sweep_failed`.
- Probes SQL manuais de recovery e claim passaram com rollback.
- Esperado: scheduler observável, claim `IN_PROGRESS→COMPLETED`, marcador `NULL` e zero erro recorrente.
- Owner: H-2026-07-16A.
