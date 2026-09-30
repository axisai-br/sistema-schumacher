# Evals do atendimento v2

Avaliação automática no estilo tau2-bench: um LLM faz o papel do **cliente** de WhatsApp, conversa com o
**agente real** (ferramentas reais, `Store` em memória, canal fake, serviços de domínio falsos com dados
realistas) e, no fim, checamos o **estado** (status, reserva, PIX, rota, passageiros) e as **regras proibidas**.

## Como rodar

```bash
cd apps/api

# Offline (sem rede, roda no CI): valida os casos, os fakes e o próprio harness,
# inclusive o caso llm_fora (LLM fora do ar -> mensagem técnica + humano).
go test ./internal/atendimento/evals

# Com LLM real (gasta tokens). Padrão: NVIDIA (LLM_PROVEDOR=nvidia):
NVIDIA_API_KEY=nvapi-... go test -tags eval ./internal/atendimento/evals -v -timeout 30m

# Com a OpenAI como alternativa:
LLM_PROVEDOR=openai OPENAI_API_KEY=sk-... go test -tags eval ./internal/atendimento/evals -v -timeout 30m

# Só alguns casos, K=5:
EVAL_CASOS='pede_ajuda|irritado' EVAL_K=5 go test -tags eval -run TestEvalCasos ./internal/atendimento/evals -v

# Só a comparação dos juízes (LLM x Jev):
go test -tags eval -run TestJuiz ./internal/atendimento/evals -v
```

Sem a chave do provedor escolhido (`NVIDIA_API_KEY` ou `OPENAI_API_KEY`) os testes com a tag `eval` são
ignorados (skip). Agente, cliente simulado e juiz usam o mesmo provedor (`provedor.ConfigDoAmbiente`); só o
nome do modelo muda.

## Variáveis

| Variável | Padrão | Uso |
| --- | --- | --- |
| `LLM_PROVEDOR` | `nvidia` | `nvidia` (NIM, Chat Completions) ou `openai` (Responses API) |
| `NVIDIA_API_KEY` | (obrigatória com `nvidia`) | chave da NVIDIA |
| `NVIDIA_BASE_URL` | `https://integrate.api.nvidia.com/v1` | base alternativa |
| `LLM_MODO_JSON` | `nvext` | saída estruturada na NVIDIA: `nvext`, `response_format` ou `prompt` |
| `LLM_REASONING_EFFORT` | `low` | esforço de raciocínio (NVIDIA): `low`, `medium`, `high` ou `max` |
| `LLM_TEMPERATURA` | `0.6` | temperatura (NVIDIA) |
| `OPENAI_API_KEY` | (obrigatória com `openai`) | chave da OpenAI |
| `OPENAI_BASE_URL` | API da OpenAI | base alternativa (proxy) |
| `EVAL_MODELO_AGENTE` | padrão do provedor (`moonshotai/kimi-k3` / `gpt-4.1-mini`) | modelo do agente |
| `EVAL_MODELO_CLIENTE` | igual ao do provedor | modelo do cliente simulado |
| `EVAL_MODELO_JUIZ` | igual ao do agente | modelo do juiz LLM |
| `EVAL_K` | `3` | execuções por caso (pass^K) |
| `EVAL_PARALELO` | `4` | chamadas de execução em paralelo |
| `EVAL_CASOS` | todos | regex sobre o nome dos casos |
| `TYPESAFE_API_KEY` | vazio | se existir, `TestJuizLLMvsJev` também avalia o juiz Jev |

## O que é checado

Por execução, um caso passa se **todas** as condições valem:

- `espera`: `status` (`BOT`, `HUMANO` ou `BOT|HUMANO`), e, quando presentes, `reserva_criada`, `pix_gerado`,
  `origem`, `destino`, `pagantes`, `criancas`, `min_opcoes_mostradas` (maior número de opções no estado durante a
  conversa) e `max_respostas_primeiro_turno`.
- `proibido`: nenhuma regex casa em mensagens do bot. `exige_no_bot`: todas casam em alguma mensagem do bot.
  `proibido_primeira_resposta` / `exige_primeira_resposta`: o mesmo, só sobre o que o bot enviou no 1º turno.
- Regras globais: o bot responde em todo turno (nunca fica em silêncio); nunca envia 3 mensagens seguidas com o
  mesmo texto (normalizado); nenhum valor `R$` fora do que existe nas fixtures (preços, múltiplos até 10 pessoas,
  sinal de R$ 250 por pagante, e os valores de reservas/pagamentos realmente criados).

As regex casam contra o texto original **e** contra a versão minúscula sem acentos; escreva os padrões sem acento.

Critério de falha do teste: casos com `"critico": true` (handoff e segurança) precisam de pass^K = 1; os casos com
`"grupo": "reserva"` precisam de taxa agregada >= 90%. A tabela (caso, sucessos/K) sai no log, e as transcrições
em `saida/<caso>-<n>.txt` (ignoradas pelo git).

## Custo aproximado

Cada execução tem 4 a 16 turnos do agente (1 a 6 chamadas de LLM cada, com política e catálogo no prompt) mais a
chamada do cliente simulado e do juiz a cada turno. Com `gpt-4.1-mini`, 17 casos x K=3 custam da ordem de
US$ 0,50 a 1,50 e levam de 5 a 15 minutos com `EVAL_PARALELO=4`. `TestJuizLLMvsJev` faz 30 chamadas por juiz
(centavos). Modelos maiores (`gpt-4.1`, `gpt-5`) multiplicam o custo por 5 a 10. Valores estimados: confira o
consumo real no painel da OpenAI.

## Fixtures

`fixtures.go` simula, em memória, `availability` (mesma semântica do `Repository.Search`: trecho embarque ->
desembarque com `stop_sequence` crescente, datas, `OnlyActive`, `Qty`, `Limit`), `pricing`, `bookings` (valida
nome, documento, valores, vagas e idempotência), `payments` (PIX com copia-e-cola começando em `00020126`),
o catálogo de cidades (MA: Santa Inês, Monção, Igarapé do Meio; SC: Fraiburgo, Monte Carlo, Videira R$ 950,
Campos Novos R$ 1.000, Chapecó, Concórdia, Ipumirim, Petrolândia, Ituporanga, Seara R$ 1.100) e um canal que
registra os envios. As 13 viagens são relativas a "hoje": MA>SC nas segundas e SC>MA nas quintas por 6 semanas
(46 vagas cada) e uma MA>SC no dia 12. `NovoAmbiente(modelo, cfg)` monta tudo com o `llm.Modelo` dado.

## Como adicionar um caso

1. Crie `casos/<nome>.json` (o `nome` deve ser igual ao nome do arquivo; campos desconhecidos são recusados):

```json
{
  "nome": "meu_caso",
  "descricao": "O que o caso prova.",
  "objetivo_cliente": "Você é o Fulano, 40 anos, CPF 123.456.789-09 ... (2ª pessoa, persona e dados que ele tem).",
  "primeira_mensagem": "Oi, quero ir pra Videira",
  "mensagens_seguidas": [],
  "max_turnos": 8,
  "critico": false,
  "grupo": "reserva",
  "espera": {"status": "BOT", "reserva_criada": true, "pix_gerado": true, "destino": "Videira", "pagantes": 1},
  "proibido": ["regex"],
  "exige_no_bot": ["regex"]
}
```

2. Use somente CPFs válidos (o teste offline confere os que aparecem no `objetivo_cliente`).
3. Inclua o nome em `casosEsperados` de `casos_test.go`.
4. Rode `go test ./internal/atendimento/evals` (valida o JSON) e depois o runner com `EVAL_CASOS=meu_caso`.

O cliente simulado responde `[FIM]` quando o objetivo foi atingido; o loop também termina quando o status vira
`HUMANO` ou em `max_turnos`. `"llm_fora": true` roda o caso com um modelo que sempre falha (sem rede).

## Testar conversas localmente

Para conversar com o agente pelo terminal (sem WhatsApp, banco ou resto do sistema, com dados de teste em
memória) e para rodar os casos deste pacote com o cliente simulado, use o simulador
`apps/api/cmd/atendimento-local`: veja o
[README do simulador](../../../cmd/atendimento-local/README.md) (chave em `apps/api/.env.atendimento-local`,
`go run ./cmd/atendimento-local`, `-roteiro`, `-caso`, dica de UTF-8 no PowerShell). Reservas e PIX são falsos.
