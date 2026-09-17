# Contrato versionado do deploy da API

Fontes dos três wrappers privilegiados. O workflow confere o SHA-256 de cada
arquivo instalado contra o checksum calculado das fontes deste mesmo build.
SHA256SUMS também é conferido no runner GitHub-hosted antes da publicação.
Alterar um wrapper exige atualizar o manifesto e revisar ambos.

## Contrato v3

- schumacher-api-deploy prepare <run_id>-<run_attempt>-<sha40> <imagem@digest> <actor>:
  valida a referência, verifica a convergência anterior, registra a tentativa e
  faz login/pull. Recebe o token GHCR por stdin. Não atualiza o serviço.
- schumacher-api-deploy apply <mesma-tentativa> <mesma-imagem@digest> <actor>:
  recebe também token transitório por stdin para o gate final de main. Sob o
  flock, valida o journal, ownership e imagem anterior, grava started durável e
  consulta main imediatamente antes de atualizar schumacher-api_schumacher-api.
  O token desta consulta fica em memória/stdin do curl; não entra em argv,
  arquivo ou log. Origin/repositório/ref HTTPS são constantes do wrapper.
  Erro HTTP/timeout/JSON/ref/SHA recusa a mutação e registra aborted.
- schumacher-api-verify <imagem@digest>: exige imagem do ServiceSpec e de todas
  as tasks desejadas, exatamente uma réplica configurada e uma task Running,
  com update state completed. Ausência de UpdateStatus ou rollback_completed
  também são recusados. Enumera todas as tasks sem filtro desired-state.
  Tasks históricas com CurrentState shutdown/complete/failed/rejected e
  DesiredState shutdown/remove podem existir, inclusive com digest antigo.
  Qualquer task extra ativa ou incerta (inclusive orphaned/remove como
  CurrentState) bloqueia convergência. Deploy e rollback usam esse verificador.
- schumacher-api-verify --lock: validação read-only de base e lock, antes de
  deploy/rollback abrirem o descritor que receberá flock.
- schumacher-api-verify --ledger: validação read-only compartilhada do journal,
  chamada internamente por deploy/rollback sob o flock; não repara registros.
- schumacher-api-rollback <mesma-tentativa> <actor>: recebe token por stdin, restaura
  exclusivamente o digest anterior dessa tentativa e verifica convergência.
  Uma tentativa antiga nunca pode reverter uma tentativa sucessora ativa.

Estado root-only em /var/lib/schumacher-api-deploy, modo 0700. Um flock
serializa chamadas mutáveis. Cada tentativa tem id, parent, previous, candidate
e phase. id deve coincidir com o nome do diretório; parent liga ao active que
precedeu prepare (ou NONE no primeiro registro). Arquivos são root-only 0600,
diretórios 0700, todos root:root, sem symlinks; nenhum registro é executado/sourceado.

O lock deve ser arquivo regular root:root 0600, nunca symlink (mesmo pendente).
Ausência permite somente criação exclusiva com noclobber; depois da validação
compartilhada ele é aberto em append, sem truncamento. Wrappers nunca removem
nem substituem esse inode. Base existente é validada antes de qualquer escrita;
bootstrap só existe em prepare, por mkdir exclusivo, sem seguir base symlink.
Base ausente em apply/rollback ou attempts ausente em journal existente é erro.
O invariante de exclusão depende de /var/lib e ancestrais administrativos
root-owned, sem escrita do runner, e de base root:root 0700. Assim usuário
não privilegiado não pode substituir base/lock entre validação e abertura.
Operações de root fora dos wrappers devem respeitar o mesmo lock e não trocar
o inode; root malicioso não faz parte da fronteira protegida.

O ledger inclui obrigatoriamente cada diretório docker (root:root 0700).
config.json é opcional antes do login/depois do cleanup; quando presente exige
arquivo regular root:root 0600. apply exige sua presença. Esses modos são
compatíveis com mkdir 0700 e CreateTemp/rename usados pelo
[Docker CLI](https://github.com/docker/cli/blob/master/cli/config/configfile/file.go).
Diretório ausente, tipos inesperados, symlinks, owner/grupo/modos divergentes
ou arquivos temporários/extras em docker bloqueiam a tentativa e sucessores.
Não há chmod/reparo automático que esconda corrupção.

Validação ocorre na entrada, após login/pull e novamente antes de cleanup,
inclusive nos traps EXIT. Cleanup só remove config.json após ledger íntegro;
não usa remoção recursiva nem atravessa diretórios symlink. Corrupção surgida
durante um comando deixa evidência intacta para recuperação explícita.

active aponta para a ponta de uma única cadeia que deve conter todos os
registros. Arquivo ausente, campo inválido, ciclo, órfão, temporário de escrita
parcial ou ownership retrocedido bloqueia prepare/apply/rollback. Nenhuma
tentativa prepared/started/rolling_back pode ficar fora da ponta active.
Prepare só aceita um predecessor completed/rolled_back/aborted, registra o novo
proprietário e o mantém até o sucessor legítimo. Não há remoção de active.

| Phase | Significado | Rollback |
|---|---|---|
| ausente/inconsistente | recuperação não comprovável | erro, nunca not_needed |
| prepared | nenhuma mutação do serviço iniciada | registra aborted sem alterar serviço |
| aborted | gate recusado/preparação cancelada antes da mutação | idempotente, sem alterar serviço |
| started | Docker pode ter iniciado uma mutação | restaura a imagem desta tentativa |
| completed | imagem candidata convergiu | ainda elegível se o gate HTTP falhar |
| rolling_back | recuperação pode ter iniciado uma mutação | retry da mesma tentativa; sucessor bloqueado |
| rolled_back | imagem anterior convergiu | revalida sem reaplicar |

active é publicado em prepare; started e rolling_back são gravados com rename
e sync antes das respectivas chamadas mutáveis. Escrita interrompida entre
registro e ponteiro é recusada pelo validador e exige recuperação explícita.
Falha/cancelamento do step de deploy também chama rollback.
Credenciais ficam temporariamente no diretório root-only da tentativa; login
usa stdin, saídas do registry são omitidas e o config é removido ao encerrar
apply/rollback. Não há Docker genérico ou instalação pelo usuário do runner.

## Autorização e ordenação

O helper hosted exige push main não forçado/não criado/não deletado, after igual
ao SHA, before igual ao primeiro pai, exatamente dois pais e segundo pai igual
ao head da PR merged no mesmo repositório/base main.

Para impedir reset para o pai seguido de replay não forçado, exige também a
primeira tentativa e a única execução push desse workflow/SHA no histórico
da API, criada entre o merge e 120 segundos depois. Usa created_at da execução,
nunca o início do job: demora de fila/testes/build não reduz esse prazo.
Replay histórico fora dessa janela é recusado mesmo se o histórico de Actions
expirou. Squash/rebase, reruns de produção, criação atrasada da execução e
respostas incompletas da API falham fechados. Uma nova PR/merge é necessária
para nova autorização; publicação manual continua disponível sem deploy.
O próprio job deploy-production exige github.run_attempt == 1, inclusive
quando Re-run failed jobs reutiliza outputs anteriores. SHAs usados na prova
da transição são strings de 40 hexadecimais minúsculos antes da comparação.

No self-hosted, contents: read existe somente para consultar o HEAD atual de
main. As consultas externas são rejeições antecipadas; a decisão final ocorre
dentro de apply, após flock, leituras Docker e sync, sem liberar o lock antes
de service update. packages: read permite pull.
Não há checkout ou download das fontes no runner de produção.
As duas consultas antecipadas também usam curl --config -: header secreto
somente por stdin, URL/repo/ref constantes, HTTPS, timeout e HTTP 200 exato,
JSON/ref/SHA validados. GH_TOKEN é copiado para variável shell não exportada,
removido do ambiente antes dos filhos e limpo ao sair; tracing fica desligado.
Prepare/apply/rollback recebem token somente por stdin. Isso não elimina a
exposição inicial do ambiente do step ao próprio runner/root comprometido.

Merges não adquirem o flock local. Por isso, após update/convergência, apply
consulta main novamente: avanço ou erro deixa started e falha, tornando
obrigatório o rollback da mesma tentativa no workflow. Não existe operação
atômica comum ao GitHub e Swarm: avanço durante a requisição Docker é detectado
e compensado, não descrito como impossível. A corrida durante as esperas
internas antes da consulta final é recusada sem service update.

## Validação local

Na raiz: node infra/deploy/test.cjs. Requer Node, Bash, Ruby/Psych, jq e curl,
já disponíveis no ambiente de validação; não instala dependências. Os testes
também rodam no CI GitHub-hosted. Docker, sudo e GitHub são fakes determinísticos;
as cópias temporárias dos wrappers substituem apenas caminhos/UID/GID. Os gates HTTP
executam curl real contra loopback para 200, 301, 302, 307, 400 e 500.

Rodar também bash -n nos três wrappers e
(cd infra/deploy && sha256sum --check SHA256SUMS).
O teste faz parsing YAML com Ruby/Psych e bash -n dos steps. Isso não substitui
actionlint, nem executa o scheduler, permissões efetivas ou APIs do GitHub.
Novas regressões exercitam tasks antigas ainda Running e história terminal,
sentinelas com lock/credenciais symlink, tipos e modos reais, preservação do
inode/conteúdo do lock, cleanup pós-login, bloqueio de sucessores e captura do
argv/ambiente das duas consultas. Owner/grupo incorretos são injetados via stat
fake, sem root/chown. O fake de service ps respeita o filtro quando fornecido.

## Gate operacional posterior — instalação não executada

Sincronização futura requer autorização separada: instalar os três arquivos
canônicos exatamente como revisados, root:root 0755 em /usr/local/sbin, com
diretórios pais root-owned sem escrita do runner. Conferir o manifesto no host
antes de habilitar qualquer deploy. Não instalar fontes por checkout no runner.
O workflow recusará wrappers legados/divergentes até essa sincronização.
Conferir também que o sudoers existente admite os novos argumentos destes
mesmos três comandos; ele não é alterado por este slice.

O primeiro prepare exige que o serviço existente já tenha referência resolvida
com digest, topologia 1/1 e update completed; referência por tag falha fechada.
Sincronizar os três wrappers v3 juntos e revisar o journal existente: registros
v1 sem id/parent e v2 com credenciais/lock fora do novo contrato são recusados,
sem migração/reparo automático nesta rodada. Confirmar também os modos produzidos
pelo Docker CLI instalado e a propriedade/permissões dos ancestrais de /var/lib.
Nenhuma stack, arquivo de ambiente, secret ou sudoers é alterado pelos wrappers.
O contrato exige que operações administrativas e atualização dos wrappers sejam
coordenadas fora de um deploy em andamento; root é a fronteira de confiança.

Morte do host/runner ou SIGKILL pode impedir o step de recuperação. O registro
durável permite recuperar a mesma tentativa; started/rolling_back pendente
bloqueia prepare do sucessor. Credenciais temporárias remanescentes exigem
limpeza operacional e expiram com o token. Corrupção/ausência parcial do registro
falha fechada e exige investigação, nunca fallback para previous-image global.

Convergência real do Swarm, consistência/latência das APIs e entrega/cancelamento
de jobs exigem validação operacional posterior. Testes com fakes não provam
comportamento do scheduler ou atomicidade entre APIs.
