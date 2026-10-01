# Atendimento v2 — plano de reescrita do atendimento por IA

Branch: `atendimento-v2` (a partir de `main` @ `e11718e`, 2026-09-30)

## 1. Por que reescrever

Diagnóstico do módulo atual (`apps/api/internal/chat`), medido em 2026-09-30:

| Métrica | Valor |
|---|---|
| Código de produção do chat | 44.700 linhas / 81 arquivos (resto da API inteira: 26.700) |
| Testes do chat | 56.900 linhas |
| Funções de heurística de texto (`looksLike*`, `parse*`, `detect*`, `is*`…) | 218, com 409 `strings.Contains` |
| Funções que releem mensagens do próprio bot (`lastAssistantAsked*`…) | 36 |
| Saídas possíveis de resposta dentro de `Reprocess` | 58 |
| Flags de ambiente `CHAT_*` | 50 |
| Commits no chat / commits de correção | 140 / 91 |
| Conversas reais desde 03/09 / reservas criadas | 12 / 0 |

Falhas observadas nas conversas reais:

- Loop "A passagem é só para você ou vai mais alguém junto?" (24 envios); a trava anti-loop
  (`unsafe_or_looping_draft_replaced`) substitui a resposta pela mesma pergunta.
- "Preciso de ajuda" não transfere para humano.
- Contradição "temos SC" (template fixo) × "atendemos só SC e MA" (regra) × texto livre do LLM
  prometendo São Paulo.
- Quatro dados obrigatórios antes da primeira busca; o cliente responde várias perguntas para depois
  ouvir que não há opção.
- Dado já informado ("3 pessoas sendo 1 criança") é perdido na etapa seguinte.
- Falhas de infraestrutura: mensagem perdida por sobrescrita do buffer, resposta velha enviada
  depois de mensagem nova, falha do LLM sem retry nem humano, áudio não transcrito ignorado.

Causa raiz: **o código tenta entender linguagem com regex e deixa o LLM por último**; várias camadas
disputam a mesma resposta; o estado é reconstruído relendo texto. Não se corrige isso com mais
regras.

## 2. Referências usadas

| Referência | O que aproveitamos |
|---|---|
| [Anthropic — Building Effective Agents](https://www.anthropic.com/engineering/building-effective-agents) | Usar o padrão mais simples que passe na avaliação: um LLM com boas ferramentas, sem framework pesado. |
| [openai/openai-cs-agents-demo](https://github.com/openai/openai-cs-agents-demo) | Atendimento de companhia aérea com ferramentas, guardrails de relevância/jailbreak e handoff. Aqui: **um** agente só (o domínio é pequeno), não 5. |
| [Rasa CALM — flows](https://rasa.com/docs/reference/primitives/flows/) | Separar entendimento (LLM) de regra de negócio (código). O fluxo da reserva vive no código como estado + validações, não no prompt. |
| [Parlant](https://github.com/emcie-co/parlant) | Diretrizes curtas e explícitas por situação, com rastreabilidade de qual regra foi aplicada. |
| [τ²-bench (Sierra)](https://github.com/sierra-research/tau2-bench) | Avaliar o agente com política + ferramentas + tarefas + **usuário simulado**, conferindo o estado final no banco, não o texto. |
| [Chatwoot Agent Bot API](https://www.chatwoot.com/features/chatbots) | Handoff com contexto completo para o humano; o cliente não repete nada. |

## 3. Princípios

1. **O LLM conversa e entende; o código guarda fatos e decide o que tem efeito.**
2. **Um agente, um prompt, uma lista de ferramentas.** Nada de roteador de intenção, templates por
   fase ou pipelines shadow.
3. **Nenhum fato sai da cabeça do modelo.** Rota, data, preço, vaga, reserva e pagamento só vêm do
   resultado de uma ferramenta no próprio turno ou do estado salvo.
4. **Estado explícito e pequeno**, salvo em banco. Nunca inferido relendo mensagens.
5. **Toda escrita passa por validação no backend** (a ferramenta rejeita e explica o motivo; o
   modelo repergunta).
6. **Humano sempre a um passo**: pedido explícito, irritação, 2 falhas seguidas ou erro técnico.
7. **Avaliação com conversas reais** substitui gates documentais.
8. **Módulos pequenos com interfaces**: trocar canal (Evolution → Cloud API / Chatwoot) ou provedor
   de LLM sem mexer no resto.

## 4. Arquitetura

```text
Evolution webhook ──► canal/evolution ──► inbox (mensagens) ──► fila por conversa
                                                                    │ (lock por conversa,
                                                                    │  debounce 2 s)
                                                                    ▼
                                                             agente.Turno()
                                                   ┌────────────────┼──────────────────┐
                                                   ▼                ▼                  ▼
                                             política.md      estado (JSON)     ferramentas
                                          + catálogo de rotas                  (availability,
                                                                               bookings, payments)
                                                                    │
                                                                    ▼
                                            checagens de saída (fatos, loop, handoff)
                                                                    │
                                                                    ▼
                                                 canal/evolution.Enviar ──► WhatsApp
```

### 4.1 Módulos (novo pacote `apps/api/internal/atendimento`)

| Pacote | Responsabilidade | Tamanho estimado |
|---|---|---|
| `canal` | Interface `Canal` + `evolution` (receber webhook normalizado, baixar mídia, enviar texto). Registra também mensagens `fromMe` como `HUMANO`. | ~400 linhas |
| `midia` | Transcrição de áudio e leitura de imagem de documento (reaproveitar código atual de `automation`). | ~200 |
| `conversa` | Tabelas `conversas`, `mensagens`, `turnos`; status `BOT/HUMANO/ENCERRADA`; lock por conversa. | ~400 |
| `fila` | Worker que pega conversas com mensagem pendente (`FOR UPDATE SKIP LOCKED`), aplica debounce e chama o agente. Concorrência configurável. | ~200 |
| `agente` | Loop LLM ↔ ferramentas (máx. 6 passos), montagem do contexto, checagens de saída. | ~350 |
| `llm` | Interface `Modelo` + implementação OpenAI Responses API (function calling). | ~200 |
| `ferramentas` | Uma função por ferramenta, com schema JSON e validação; chama os serviços de domínio existentes. | ~700 |
| `politica` | `politica.md` (regras de negócio em português, 1 página) + catálogo gerado do banco. | ~100 + texto |
| `evals` | Casos em YAML + runner com usuário simulado + asserções de estado. | ~500 |

**Total estimado: 2.500 a 3.000 linhas**, contra 44.700 hoje.

Reaproveitados sem mudança: `availability`, `bookings`, `payments`, `pricing`, `auth`, `users`,
o app interno (`apps/app`) para a fila de atendimento humano.

### 4.2 Estado da conversa

Salvo em `conversas.estado` (JSONB), com versão para concorrência otimista:

```go
type Estado struct {
    Origem       *Parada      `json:"origem,omitempty"`   // resolvida por ferramenta
    Destino      *Parada      `json:"destino,omitempty"`
    Opcoes       []Opcao      `json:"opcoes,omitempty"`   // última busca mostrada ao cliente
    Viagem       *Opcao       `json:"viagem,omitempty"`   // escolhida
    Passageiros  []Passageiro `json:"passageiros,omitempty"`
    Pagamento    string       `json:"pagamento,omitempty"` // "integral" | "sinal"
    ReservaID    string       `json:"reserva_id,omitempty"`
    PagamentoID  string       `json:"pagamento_id,omitempty"`
    Falhas       int          `json:"falhas"`             // tentativas sem avanço seguidas
}

type Passageiro struct {
    Nome      string `json:"nome"`
    Documento string `json:"documento"` // CPF/RG validado
    Crianca5  bool   `json:"crianca_ate_5"`
}
```

Não há campo "fase". O que falta é calculado a partir dos campos (`Estado.Pendencias()`) e vai no
contexto do modelo como lista curta: `["escolher viagem", "nome e documento de 2 passageiros"]`.

### 4.3 Ferramentas

| Ferramenta | Efeito | Validação no backend |
|---|---|---|
| `listar_rotas()` | Lê o catálogo: cidades MA/SC atendidas e preço por cidade. | — |
| `buscar_viagens(origem?, destino?, data_de?, data_ate?, pessoas?)` | Resolve nomes de cidade (aliases + sem acento + similaridade), chama `availability.Search`, grava `Opcoes`. Aceita **só destino** e devolve as próximas datas. | Cidade fora do catálogo retorna `nao_atendida` + lista de cidades atendidas. |
| `escolher_viagem(opcao)` | Grava `Viagem`. | A opção precisa existir em `Opcoes` e ter vaga. |
| `registrar_passageiros(lista)` | Substitui `Passageiros`. | CPF válido, nome com 2+ palavras, total ≤ vagas. |
| `criar_reserva(pagamento)` | `bookings.Create`, grava `ReservaID`. Idempotente. | Estado completo; recusa com a lista do que falta. |
| `gerar_pix()` | `payments.Create`, devolve copia-e-cola. | Reserva existente; sem cobrança duplicada. |
| `consultar_reserva()` | Status da reserva/pagamento pelo telefone ou código. | — |
| `transferir_para_humano(motivo)` | Status `HUMANO`, alerta a equipe com resumo. | — |

Cada ferramenta devolve JSON curto com `ok`, `dados` e, em caso de erro, `motivo` em português para o
modelo explicar ao cliente.

### 4.4 Contexto enviado ao modelo em cada turno

1. `politica.md`: quem somos, rotas, preços, criança até 5 anos, sinal de R$ 250, só PIX, bagagem,
   embarque, telefone do suporte, tom (curto, 1 pergunta por vez), quando chamar humano.
2. Catálogo de cidades (gerado do banco no início de cada turno, cache de 5 min).
3. `Estado` atual + `Pendencias()`.
4. Últimas 20 mensagens da conversa (texto; áudio já transcrito; imagem descrita).

### 4.5 Checagens de saída (código, não prompt)

- **Fatos**: todo valor `R$`, data `dd/mm` e horário da resposta precisa aparecer no resultado de
  ferramenta do turno ou no `Estado`. Se não aparecer, o turno é refeito uma vez com o aviso; na
  segunda falha, humano.
- **Loop**: se a resposta repete a pergunta anterior do bot, `Falhas++`; com `Falhas >= 2`,
  transfere para humano com mensagem ao cliente.
- **Pedido de humano**: palavras como atendente, humano, pessoa ou ajuda, e irritação detectada
  pelo modelo (campo `transferir` na saída estruturada), transferem direto.
- **Mensagem nova durante o turno**: antes de enviar, se chegou mensagem nova do cliente, descarta a
  resposta e refaz o turno com tudo.
- **Erro de LLM/ferramenta**: 2 tentativas com backoff; depois, mensagem padrão + humano. Nunca
  silêncio.

### 4.6 Humano no circuito

- Status `HUMANO` pausa o bot na conversa. Volta para `BOT` por ação no app interno ou após 12 h
  sem mensagem do atendente.
- Mensagem enviada pelo celular da empresa (`fromMe`) também coloca a conversa em `HUMANO`.
- O alerta de transferência leva um resumo gerado do `Estado` (rota, viagem, passageiros, o que
  falta), para o atendente não perguntar de novo.

### 4.7 Banco

Migrações novas, sem tocar nas tabelas `chat_*` atuais:

- `atd_conversas (id, canal, contato, nome, status, estado jsonb, versao, atualizado_em)`
- `atd_mensagens (id, conversa_id, direcao, autor[CLIENTE|BOT|HUMANO], tipo, texto, midia jsonb, provedor_id unique, criado_em)`
- `atd_turnos (id, conversa_id, mensagens_entrada, chamadas_ferramenta jsonb, estado_antes, estado_depois, resposta, modelo, tokens, latencia_ms, erro, criado_em)`
- `atd_cidades_alias (alias, stop_id)` para "fraiburgo", "moncao", "santa ines" etc.

`atd_turnos` é o log completo de cada decisão: substitui os shadows e permite revisar qualquer
conversa em um único lugar.

## 5. Avaliação (substitui os gates)

- `evals/casos/*.yaml`: um arquivo por cenário, com objetivo do cliente simulado e asserções.
  Primeiro lote: as 12 conversas reais anonimizadas + `docs/PRODUCTION_CONVERSATION_CASES.md`.
- Runner no estilo τ²-bench: um LLM faz o papel do cliente a partir do objetivo, conversa com o
  agente contra um banco de teste semeado, e ao fim confere o **estado**, por exemplo "reserva
  criada na viagem X com 2 pagantes + 1 criança".
- Asserções proibitivas: nunca repetir a mesma pergunta 3 vezes, nunca citar preço fora do catálogo,
  nunca dizer "temos" sem busca com resultado, sempre transferir quando pedirem humano.
- Métrica: taxa de sucesso por cenário em 5 execuções (pass^k). Critério de virada: ≥ 90% nos
  cenários de reserva e 100% nos de handoff e segurança.
- Roda no CI com modelo barato; roda completo antes de cada deploy.

Cenários mínimos:

1. "Tem viagem pra Chapecó?" → mostra as próximas datas sem pedir 4 dados.
2. "Passagem de SC pro Maranhão" → pergunta a cidade de SC, não manda tabela de preço de ida.
3. "São Paulo" / "Cocal do Sul para Pomerode" → explica claramente as cidades atendidas, uma vez.
4. Quantidade informada na primeira mensagem ("3 pessoas sendo 1 criança") não é perguntada de novo.
5. Respostas livres de quantidade: "só eu", "somos 3", "eu mais 2 crianças", "vai mais junto".
6. "Preciso de ajuda" em qualquer ponto → humano.
7. Reserva completa até o PIX do sinal.
8. Cliente muda de ideia no meio (troca data ou destino).
9. Áudio com transcrição falha → pede para escrever, não fica em silêncio.
10. Duas mensagens seguidas durante o processamento → uma resposta coerente com as duas.
11. Mensagem fora do assunto (venda de doces, assunto pessoal) → resposta curta, sem insistir em
    reserva.
12. LLM fora do ar → mensagem padrão + humano.

## 6. Etapas

| # | Entrega | Critério de pronto |
|---|---|---|
| 0 | Paliativo no fluxo atual: segunda repetição de pergunta ou pedido de ajuda → humano. | Em produção enquanto o v2 é construído. |
| 1 | `conversa`, `fila`, `canal/evolution`, migrações. Bot "eco" em número de teste. | Mensagens, debounce, lock e `fromMe` funcionando ponta a ponta. |
| 2 | `llm`, `agente`, `politica.md`, ferramentas de leitura (`listar_rotas`, `buscar_viagens`, `consultar_reserva`, `transferir_para_humano`). | Cenários 1, 2, 3, 6 e 11 passando. |
| 3 | Ferramentas de escrita (`escolher_viagem`, `registrar_passageiros`, `criar_reserva`, `gerar_pix`) + checagens de saída. | Cenários 4, 5, 7, 8, 12 passando. |
| 4 | `midia` (áudio e documento por foto) + runner de evals no CI. | Todos os cenários ≥ critério. |
| 5 | Rollout: flag `ATENDIMENTO_V2_TELEFONES` (lista) → número interno → 20% → 100%. | Uma semana em 100% sem incidente. |
| 6 | Remoção de `internal/chat`, das flags `CHAT_*` e dos loops antigos. | Código antigo apagado. |

Etapas 1–4 cabem em 2 a 3 semanas de uma pessoa.

## 7. Decisões em aberto

- Modelo: manter OpenAI (chave e integração já existem); a interface `llm.Modelo` permite trocar.
- Canal: manter Evolution agora; migrar para a API oficial do WhatsApp ou para Chatwoot é só um
  novo `canal`.
- Onde o atendente humano trabalha: app interno atual (`apps/app`) ou Chatwoot. O plano assume o
  app atual.
- Tabela de preços: hoje duplicada entre `route_segment_prices` e textos fixos no código. No v2,
  só o banco.
- Deploy: o workflow `Publish API to GHCR` falha na etapa `authorize-production` desde abril neste
  repositório; é preciso definir de onde sai a imagem de produção antes da etapa 5.
