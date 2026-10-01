# Testar conversas localmente

Simulador de conversas do atendimento v2 pelo terminal: o **agente real** + um **LLM real**, com dados de teste
em memória. Sem WhatsApp, sem banco de dados e sem o resto do sistema.

## Jeito mais fácil (Windows)

**Duplo clique em `testar-atendimento.cmd`** (na raiz do repositório) e cole a chave quando ele pedir. O script
cria o arquivo de credenciais, configura o console para UTF-8 e abre a conversa. Exemplos pelo PowerShell:

```powershell
.\testar-atendimento.ps1                          # conversa interativa
.\testar-atendimento.ps1 -Roteiro loop_passageiros
.\testar-atendimento.ps1 -Caso todos
.\testar-atendimento.ps1 -Lista
.\testar-atendimento.ps1 -NovasChaves             # troca as chaves
.\testar-atendimento.ps1 -Modelo meta/llama-3.3-70b-instruct   # só nesta execução
```

## Onde colocar a chave

Copie o modelo e preencha a chave (o arquivo `.env.atendimento-local` é ignorado pelo git):

```bash
cd apps/api
cp .env.atendimento-local.example .env.atendimento-local
# edite e preencha NVIDIA_API_KEY (obtenha em build.nvidia.com)
```

O arquivo é lido de `./.env.atendimento-local` (pasta atual) ou do caminho dado em `-env`. Formato `KEY=VALUE`;
linhas vazias e `#` são ignoradas; aspas são aceitas. Variáveis já definidas no ambiente do sistema têm
prioridade sobre o arquivo. Sem a chave, o simulador diz onde colocá-la e sai com código 2.

| Variável | Padrão | Uso |
| --- | --- | --- |
| `LLM_PROVEDOR` | `nvidia` | `nvidia` ou `openai` |
| `NVIDIA_API_KEY` | (obrigatória com nvidia) | chave da NVIDIA (build.nvidia.com) |
| `NVIDIA_BASE_URL` | `https://integrate.api.nvidia.com/v1` | base da API |
| `OPENAI_API_KEY` / `OPENAI_BASE_URL` | vazio | só com `LLM_PROVEDOR=openai` (ou para áudio) |
| `ATENDIMENTO_V2_MODELO` | `z-ai/glm-5.3` (nvidia), `gpt-4.1-mini` (openai) | modelo do agente e do juiz |
| `LLM_MODO_JSON` | `nvext` | `nvext`, `response_format` ou `prompt` (nvidia) |
| `LLM_REASONING_EFFORT` | `low` | `low`, `medium`, `high` ou `max`; mais alto = mais lento (nvidia) |
| `LLM_TEMPERATURA` | `0.3` | temperatura (nvidia) |
| `LLM_MODELO_RESERVA` | `nvidia/nemotron-3.5-lightning-30b-a3b` (nvidia) | modelo reserva do hedge; `off` desliga |
| `LLM_HEDGE_MS` | `3000` | se o principal não respondeu em N ms, o mesmo pedido vai ao reserva e vale a primeira resposta; `0` desliga o hedge |
| `LLM_SEM_RACIOCINIO` | `true` só no reserva | envia `chat_template_kwargs {"enable_thinking":false}`; se definida, vale para principal e reserva |
| `ATENDIMENTO_V2_JUIZ` | `llm` | `llm`, `jev` ou `off`. Com `jev` e `TYPESAFE_API_KEY`, o **roteador Jev** (1 requisição por turno: humano, irritação, intenção, origem/destino, opção) substitui o juiz e responde saudação e "quais cidades" sem LLM, além de pré-executar `buscar_viagens` |
| `TYPESAFE_API_KEY` | vazio | só se o juiz/roteador for `jev` |
| `ATENDIMENTO_V2_SINAL_POR_PAGANTE` | `250` | sinal por passageiro pagante (R$) |
| `EVAL_MODELO_CLIENTE` | igual ao do agente | modelo do cliente simulado (modo `-caso`) |

## Comandos

```bash
cd apps/api

go run ./cmd/atendimento-local                              # interativo
go run ./cmd/atendimento-local -roteiro loop_passageiros    # roteiro (nome da lista ou caminho de arquivo)
go run ./cmd/atendimento-local -caso pede_ajuda             # caso de avaliação (cliente simulado por LLM)
go run ./cmd/atendimento-local -caso todos -k 3             # todos os casos, 3 vezes cada
go run ./cmd/atendimento-local -lista                       # nomes dos casos e roteiros (não precisa de chave)
go run ./cmd/atendimento-local -env C:\caminho\meu.env      # outro arquivo de credenciais
```

### Modo interativo

Prompt `você> `. Cada linha é uma mensagem do cliente; o bot responde como `Shabas> ...` e, depois de cada turno,
aparece uma linha discreta com ferramentas usadas, tempo e tokens.

| Entrada | O que faz |
| --- | --- |
| `texto` | mensagem do cliente; roda um turno do agente |
| `+texto` | enfileira a mensagem **sem** processar; a próxima linha normal processa todas juntas |
| `/estado` | Estado da reserva (JSON), pendências e status da conversa |
| `/turno` | passos do último turno: chamadas de ferramenta (argumentos e resultado), checagens, roteador (intenção, confiança, decisão), juiz, tokens, latência |
| `/reset` | nova conversa e fixtures novas |
| `/bot` | se a conversa foi transferida para humano, volta para BOT |
| `/audio` | envia `[áudio não compreendido]` |
| `/nome <nome>` | nome do cliente na conversa |
| `/reservas` | reservas e PIX criados nos fakes |
| `/salvar` | grava a transcrição agora |
| `/ajuda`, `/sair` | ajuda; encerra |

Quando a conversa vira HUMANO o simulador avisa (`*** conversa transferida para atendente humano ... ***`) e, até
você usar `/bot`, as mensagens são registradas mas não processadas. Ao sair (`/sair`, Ctrl+C ou fim da entrada) a
transcrição é salva em `internal/atendimento/evals/saida/local-<AAAAMMDD-HHMMSS>.txt` (pasta ignorada pelo git).

### Modo roteiro

`-roteiro` executa, sem interação, as linhas de um arquivo de texto (o que o cliente digitaria; aceita `+`,
comandos como `/estado` e linhas `#` de comentário), imprime a conversa e salva a transcrição. Roteiros prontos,
tirados de conversas reais de produção, em `cmd/atendimento-local/roteiros/`: `loop_passageiros`, `sao_paulo`,
`videira_3_pessoas`, `ida_e_volta` e `doces`.

### Modo caso

`-caso <nome|todos>` roda os casos de avaliação de `internal/atendimento/evals/casos/` com o cliente simulado
por LLM, imprime a transcrição e `APROVADO`/`REPROVADO` com as falhas; `-k N` repete N vezes. Sai com código 1 se
algum caso reprovar.

## Dica para Windows (acentos no PowerShell)

Se os acentos aparecerem quebrados, rode antes:

```powershell
chcp 65001
$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8
```

(`testar-atendimento.ps1` já faz isso.)

## Os dados são de teste

São 13 cidades (Maranhão e Santa Catarina) e viagens fictícias, relativas à data de hoje (o cabeçalho mostra a
data usada). Reservas e PIX são **falsos**: nada é gravado no banco e nada é enviado ao WhatsApp. O LLM, porém, é
real: cada turno consome tokens do provedor configurado.
