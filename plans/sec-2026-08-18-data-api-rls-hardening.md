# SEC-2026-08-18 — Hardening Supabase Data API / RLS

## 1. Estado reconciliado

Baseline do planejamento:

```text
branch: wip/h-2026-07-27a-recovery
HEAD/origin-main: 5dfd9a09a5c93ba5fb2d7d68d59d322ce0bf0436
checkpoint contido: 4eb543cb27cfa6527c0f225383d2f07fb9d38b97
working tree na reconciliação: limpo
```

Este track é independente da fila funcional. Não altera os status ou gates de
H-2026-07-27A, H-2026-07-16B ou 3.6F.

O inventário de produção foi coletado com `transaction_read_only=on`. Foram
confirmadas 29 tabelas, 15 routines e uma sequence no schema `public`. Não há
policies nem tabelas em publication.

## 2. Fronteira de confiança e ameaça

```text
Frontend
  -> Supabase Auth/session
  -> token autenticado
  -> API Schumacher

Data API/PostgREST
  -> anon/authenticated
  -> grants + RLS + policies

API Go
  -> PostgreSQL direto como postgres
  -> BYPASSRLS
```

Storage e Auth são superfícies separadas. Nenhuma tabela `public` foi
comprovada como `DATA_API_REQUIRED`.

Evidência versionada e operacional:

- zero uso de `supabase.from`, `client.from`, `/rest/v1`, `/graphql/v1`,
  `supabase.rpc` ou `/rpc/` no repo;
- telas de negócio usam `apps/app/src/services/api.ts`;
- aproximadamente 142 horas e 625 requests no Kong, com zero `/rest/v1` e
  zero `/graphql/v1`;
- operações de negócio conhecidas passam pela API Go.

Essa evidência é forte, mas não prova a inexistência de consumidor externo
raro. Ausência de evidência deixa o objeto `UNKNOWN_BLOCKED`.

### Finding: FUNCTION/RPC AUTHORITY EXPOSURE

Nove functions `public` são catalogalmente elegíveis como RPC para anon,
authenticated e service_role. Todas as 15 routines possuem `EXECUTE`
diretamente para PUBLIC, anon, authenticated, postgres e service_role.

Como são `SECURITY INVOKER`, o efeito depende também dos privilégios SQL da
role chamadora. A composição entre `EXECUTE RPC` e grants amplos de tabela é
exposição de autoridade comprovada, não exploração comprovada.

## 3. Grants, RLS e policies

- `REVOKE` remove autoridade SQL/Data API.
- RLS limita operações de roles sem `BYPASSRLS`.
- Policy não concede grant.
- `ENABLE RLS` não remove grant.
- `postgres` e `service_role` possuem `BYPASSRLS`.
- FORCE RLS é proibido.
- Nenhuma policy permissiva será criada para restaurar funcionamento sem um
  consumidor Data API comprovado.
- `service_role` permanece inalterado nesta versão.

## 4. Inventário das 29 tabelas

Legenda: `ALL7` significa `SELECT, INSERT, UPDATE, DELETE, TRUNCATE,
REFERENCES, TRIGGER`; `-` significa nenhum privilégio efetivo. Todas possuem
zero policies e FORCE RLS desabilitado.

| Tabela | Owner | RLS | anon/auth/service | Consumidor comprovado | Classe | Confiança | Dependências relevantes |
|---|---|---:|---|---|---|---|---|
| affiliate_recipients | postgres | false | ALL7/ALL7/ALL7 | API users/affiliate | BACKEND_ONLY | alta | usuários |
| affiliate_withdrawals | postgres | false | ALL7/ALL7/ALL7 | API affiliate | BACKEND_ONLY | alta | user_profiles |
| automation_job_runs | postgres | false | ALL7/ALL7/ALL7 | jobs de automation | BACKEND_ONLY | alta | user_profiles |
| available_segments | postgres | false | ALL7/ALL7/ALL7 | availability, imports e refresh | BACKEND_ONLY | média-alta | stops, routes, trips |
| booking_payment_details | postgres | false | ALL7/ALL7/ALL7 | bookings/payments/reports/trips | BACKEND_ONLY | alta | bookings; trigger |
| bookings | postgres | false | ALL7/ALL7/ALL7 | bookings/chat/automation/payments | BACKEND_ONLY | alta | stops, trips; trigger |
| chat_handoffs | postgres | false | ALL7/ALL7/ALL7 | chat repository | BACKEND_ONLY | alta | user_profiles, chat_sessions |
| chat_messages | postgres | false | ALL7/ALL7/ALL7 | chat e automation | BACKEND_ONLY | alta | chat_sessions |
| chat_sessions | postgres | false | ALL7/ALL7/ALL7 | chat, painel e workers | BACKEND_ONLY | alta | user_profiles |
| chat_tool_calls | postgres | false | ALL7/ALL7/ALL7 | ledger de tools do chat | BACKEND_ONLY | alta | messages, sessions |
| fiscal_documents | confirmar no pré-check | false | -/-/- | API fiscal/trip operations | BACKEND_ONLY | alta | trips |
| manifest_data | postgres | false | ALL7/ALL7/ALL7 | read model/imports/reports | BACKEND_ONLY | média-alta | bookings, trips; triggers |
| outbound_messages | postgres | false | ALL7/ALL7/ALL7 | chat delivery/automation | BACKEND_ONLY | alta | jobs, sessions |
| pagarme_webhook_events | postgres no checkpoint | true | -/-/ALL7 | webhook/idempotência affiliate | BACKEND_ONLY | alta | já endurecida |
| passengers | postgres | false | ALL7/ALL7/ALL7 | bookings/trips/reports/chat | BACKEND_ONLY | alta | bookings, stops, trips; trigger |
| payment_events | postgres | false | ALL7/ALL7/ALL7 | ledger payments | BACKEND_ONLY | alta | payments |
| payments | postgres | false | ALL7/ALL7/ALL7 | payments/automation/chat/reports | BACKEND_ONLY | alta | bookings |
| roles | postgres no checkpoint | true | -/-/ALL7 | users/profile/affiliate | BACKEND_ONLY | alta | user_roles; já endurecida |
| route_segment_prices | postgres | false | ALL7/ALL7/ALL7 | availability/pricing/routes/trips | BACKEND_ONLY | alta | stops, routes; trigger |
| routes | postgres | false | ALL7/ALL7/ALL7 | routes/trips/availability/imports | BACKEND_ONLY | alta | trips, prices |
| sheet_sync_queue | postgres | false | ALL7/ALL7/ALL7 | consumidor não comprovado | UNKNOWN_BLOCKED | baixa | view, triggers, sequence |
| stops | postgres | false | ALL7/ALL7/ALL7 | routes/trips/availability/bookings | BACKEND_ONLY | alta | prices, trips, bookings |
| travel_authorization_check_items | confirmar | false | -/-/- | nenhum | UNKNOWN_BLOCKED | baixa | travel_authorizations |
| travel_authorizations | confirmar | false | -/-/- | nenhum | UNKNOWN_BLOCKED | baixa | check items |
| trip_contractors | confirmar | false | -/-/- | nenhum | UNKNOWN_BLOCKED | baixa | não comprovadas |
| trip_stops | postgres | false | ALL7/ALL7/ALL7 | trips/routes/availability/pricing | BACKEND_ONLY | alta | stops, trips |
| trips | postgres | false | ALL7/ALL7/ALL7 | trips/availability/bookings/reports | BACKEND_ONLY | alta | routes; trigger |
| user_profiles | postgres | false | ALL7/ALL7/ALL7 | users/profile/chat | BACKEND_ONLY | alta | affiliate/automation/chat |
| user_roles | postgres | false | ALL7/ALL7/ALL7 | users/profile/affiliate authz | BACKEND_ONLY | alta | roles |

Resultado: 25 `BACKEND_ONLY`, zero `DATA_API_REQUIRED`, zero
`SYSTEM_AUTH_STORAGE` e quatro `UNKNOWN_BLOCKED`.

## 5. Dossiê das seis tabelas críticas

| Tabela | Consumidor real | Data API/anon/auth | Mudança mínima futura | Smoke positivo obrigatório |
|---|---|---|---|---|
| user_roles | users/profile/affiliate via PostgreSQL | nenhuma necessidade comprovada | revoke anon/auth + ENABLE RLS, sem policy | login, `/users/me`, alteração controlada de acesso |
| user_profiles | users/profile/chat via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | bootstrap de profile e leitura administrativa |
| passengers | bookings/trips/reports/chat via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | reserva controlada, detalhes e relatório |
| bookings | bookings/chat/automation/payments via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | list/get e fixture segura de reserva/expiração |
| booking_payment_details | bookings/payments/reports via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | leitura e atualização transacional controlada |
| payments | payments/automation/chat/reports via PostgreSQL | nenhuma | revoke anon/auth + ENABLE RLS | list/status e idempotência sem cobrança real |

Em todas, `service_role` permanece como está e o impacto esperado na API Go é
nulo por causa do acesso direto como `postgres`. Essa expectativa precisa ser
comprovada por smoke separado.

## 6. Inventário das routines

Todas são owner `postgres`, `SECURITY INVOKER`, `VOLATILE`, parallel `UNSAFE`
e possuem grants diretos de EXECUTE para PUBLIC, anon, authenticated,
postgres e service_role.

| Signature | Finalidade/efeito provado | Consumidor | RPC | Classe | Confiança |
|---|---|---|---:|---|---|
| build_sheet_sync_payload(text,text) | menções a bookings/passengers/trips/payment/manifest; corpo não versionado | nenhum | sim | UNKNOWN_BLOCKED | baixa |
| claim_sheet_sync_batch(integer) | relacionado à fila; corpo não versionado | worker não encontrado | sim | UNKNOWN_BLOCKED | baixa |
| enqueue_manifest_for_booking(text) | menção a manifest; corpo não versionado | nenhum | sim | UNKNOWN_BLOCKED | baixa |
| enqueue_sheet_sync(text,text,sheet_sync_operation) | relacionado à fila | nenhum | sim | UNKNOWN_BLOCKED | baixa |
| mark_sheet_sync_done(bigint) | relacionado à fila | worker não encontrado | sim | UNKNOWN_BLOCKED | baixa |
| mark_sheet_sync_retry(bigint,text,integer,integer) | relacionado à fila | worker não encontrado | sim | UNKNOWN_BLOCKED | baixa |
| refresh_available_segments_for_route(text) | lê trips/stops/prices; recria available_segments | API routes/trips por SQL direto | sim | BACKEND_ONLY | alta |
| refresh_manifest_data() | lê fontes; trunca/recria manifest | nenhum caller versionado | sim | UNKNOWN_BLOCKED | média |
| refresh_manifest_data_for_booking(text) | lê fontes; apaga/recria manifest do booking | nenhum caller versionado | sim | UNKNOWN_BLOCKED | média |
| set_route_segment_prices_updated_at() | atualiza NEW.updated_at | trigger versionado | não | TRIGGER_INTERNAL | alta |
| set_sheet_sync_queue_updated_at() | corpo ausente | trigger da fila | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_booking_payment_details() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_bookings() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_passengers() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |
| trg_sheet_sync_trips() | corpo ausente | trigger da tabela | não | TRIGGER_INTERNAL | média |

Não há DDL de sheet sync no HEAD nem no histórico Git local. O endpoint
`/automation/jobs/sheet-sync/run` retorna `NotImplemented`; existem variáveis
de configuração Google Sheets, mas nenhum worker/cliente implementado.

## 7. Sequence

Existe somente `public.sheet_sync_queue_id_seq`, owner `postgres`, bigint, com
`SELECT/UPDATE/USAGE` para anon, authenticated, postgres e service_role. A
dependência interna tipo `i` com `sheet_sync_queue.id` foi comprovada, mas o
DDL da coluna e o mecanismo de geração não foram encontrados. A ausência de
linha em `column_default` não autoriza inferência. Classificação:
`UNKNOWN_BLOCKED`.

## 8. Protocolo comum dos lotes

Cada `/goal` autoriza exatamente um lote ou sublote.

Pré-check:

- confirmar SHA/deploy alvo e frescor do inventário;
- conferir owner, RLS, FORCE, policies, ACL direto e privilégios efetivos;
- confirmar ausência de publication e de novo consumidor Data API/RPC;
- capturar ACL anterior em evidência sanitizada;
- abortar em qualquer divergência.

Mudança padrão de tabela `BACKEND_ONLY` exposta:

1. revogar os sete privilégios de anon e authenticated;
2. habilitar RLS;
3. não criar policy;
4. preservar service_role, postgres, owner e FORCE.

Verificação:

- RLS true, FORCE false e policies zero;
- anon/auth sem privilégio direto ou efetivo;
- service_role idêntico;
- Data API anon e authenticated negada em leitura e escrita;
- consumidor pela API Go verde.

Rollback de tabela originalmente exposta:

1. desabilitar RLS;
2. restaurar os sete privilégios capturados para anon/authenticated;
3. preservar service_role;
4. repetir pós-check e smoke da API.

### Gate `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`

Enquanto `sheet_sync_queue`, `sheet_sync_queue_id_seq` ou as routines de
sheet sync permanecerem `UNKNOWN_BLOCKED`:

- nenhuma verificação em produção pode provocar deliberadamente
  `INSERT`/`UPDATE`/`DELETE` em `trips`, `bookings`, `passengers` ou
  `booking_payment_details` apenas para testar trigger ou enqueue;
- toda prova comportamental de trigger/enqueue que possa escrever na fila ou
  avançar sua sequence ocorre somente em PostgreSQL efêmero/de teste;
- em produção, os lotes afetados ficam limitados a pré/pós-checks read-only e
  aos smokes legítimos que não escrevam nesses objetos-fonte; uma operação
  orgânica observada não autoriza criar uma operação sintética;
- o lote 2B, os lotes 9, 11, 12 e 13, assim como seus dependentes 10, 14 e
  15, não podem contornar este gate;
- qualquer verificação que exija a escrita indireta permanece bloqueada até a
  reconciliação e reclassificação explícita do subsistema sheet sync.

O gate não reclassifica nem autoriza mutação de `sheet_sync_queue`, sua
sequence ou suas routines `UNKNOWN_BLOCKED`.

## 9. Rollout em lotes

### Lote 1 — default ACL de TABLE

Revogar ALL7 do default ACL de `postgres` em `public` para anon/authenticated.
Não altera tabelas existentes. Verificar com `pg_default_acl` e objeto
descartável somente em PostgreSQL efêmero. Rollback: restaurar ALL7.

### Lote 2A — novas FUNCTIONs `public` em DDL versionado

**Decisão aprovada: opção B.** O Lote 2A passa a definir uma política de
hardening explícito, atômico e por assinatura para toda nova FUNCTION
`public` criada por migration versionada. O default ACL global de FUNCTION
permanece inalterado e fora da solução; não haverá revoke cross-schema nem
grant global a service_role.

Cada nova FUNCTION coberta deverá:

1. usar schema `public` explícito no DDL;
2. ser criada e endurecida na mesma transação;
3. revogar `EXECUTE`, pela identidade/assinatura exata, de PUBLIC, anon e
   authenticated;
4. declarar explicitamente a política de service_role para aquela assinatura,
   sem grant global ou inferência por convenção;
5. comprovar que postgres preserva sua autoridade de owner e que nenhum objeto
   preexistente ou schema não público foi alterado.

DDL manual, externo ou fora do fluxo versionado não é protegido por esta
política e permanece risco residual `UNKNOWN_BLOCKED`. A aprovação da opção B
limita deliberadamente a garantia ao DDL versionado; não declara inexistência
de creators ou consumidores externos.

#### Histórico preservado do bloqueio public-only

O objetivo anterior tentava revogar EXECUTE de PUBLIC, anon e authenticated
somente no default ACL de futuras functions `postgres`-owned em `public`.
PostgreSQL soma defaults por schema ao default global; portanto,
`ALTER DEFAULT PRIVILEGES ... IN SCHEMA public REVOKE EXECUTE ... FROM PUBLIC`
não neutraliza o EXECUTE global nativo de PUBLIC. O revoke eficaz é global e
afeta futuras functions `postgres`-owned em todos os schemas.

O review anterior fechou quatro P1: revoke global sem autorização
cross-schema; grant global incondicional capaz de criar autoridade nova para
service_role; inventário cross-schema insuficiente; e rollback não derivado
integralmente de snapshot. A migration 0023 foi removida, nenhum efeito
executável permaneceu e o review final terminou sem P0/P1/P2. Esses findings
continuam fechados e não são reabertos pela opção B.

#### Pré-check operacional READ-ONLY — PASS

O pré-check autorizado terminou em `ROLLBACK`, com
`transaction_read_only=on`, sem GRANT, REVOKE, DDL ou DML. Evidência
sanitizada:

```text
remote main: b559b326b0cd27ad0b2f71ed3572a14b6b7673c8
checkout servidor/API OCI: 4eb543cb27cfa6527c0f225383d2f07fb9d38b97
PostgreSQL: 15.8
PGRST_DB_SCHEMAS: public,storage,graphql_public
schemas não sistêmicos: 14
schemas com CREATE para postgres: extensions, gsheets_raw, public, realtime,
  supabase_functions
default nativo de FUNCTION para postgres: EXECUTE para PUBLIC
default ACL explícito de FUNCTION para postgres: public, storage,
  supabase_functions
public: 29 tables / 15 routines / 1 sequence / 0 policies
public routines owner postgres: 15
postgres-owned routines fora de public: 0
public routines com owner diferente de postgres: 0
default TABLE public: endurecido pelo Lote 1
default SEQUENCE: inalterado
fim da transação: ROLLBACK
```

O checkout operacional contém SQLs históricos untracked; eles permanecem
fora do escopo, não são limpos, movidos ou versionados e não integram o
working tree local desta rodada.

#### REDs, prova e rollback do gate

O gate implementado em `apps/api/internal/migrationguard` provou localmente:

- RED: nova FUNCTION `public` sem hardening herda EXECUTE de PUBLIC;
- PASS: DDL schema-qualified e revoke por assinatura deixam
  PUBLIC/anon/authenticated sem EXECUTE direto ou efetivo;
- service_role termina exatamente conforme a política declarada e postgres
  permanece efetivo como owner;
- overload sem ACL própria, DDL não qualificado ou hardening incompleto falham;
- FUNCTION de outro schema/owner e objects preexistentes permanecem idênticos.

Antes do patch, sete migrations inválidas temporárias posteriores à 0022 não
foram rejeitadas por `go test -count=1 ./...`. Depois do patch, os casos
dirigidos são rejeitados, a fixture canônica válida passa, migrations até 0022
ficam fora do enforcement e o diretório atual passa. O CI existente já executa
a suíte completa; não foi necessário alterar workflow.

A prova transacional em PostgreSQL 15.19 confirmou o RED nativo e o PASS das
quatro autoridades. Functions preexistentes, schema não público e default ACL
ficaram idênticos ao snapshot. A transação terminou em `ROLLBACK` e o banco
efêmero foi removido.

O primeiro review do gate encontrou 3 P1 + 1 P2: ACL FUNCTION/ROUTINE não
canônica ignorada; mutations pós-criação de owner/schema ignoradas; ausência
de framing transacional explícito e de `END`; nomes `.sql` numericamente
iniciados e malformados ignorados. REDs dirigidos reproduziram os quatro
bypasses.

A correção local exige agora `BEGIN` antes da criação e `COMMIT` ou `END`
somente depois da política ACL completa; rejeita fechamento intermediário,
ACL ou modifiers fora das formas exatas, `ALTER FUNCTION/ROUTINE`, mudanças de
role/owner e nomes numéricos posteriores à 0022 fora de
`NNNN_description.sql`. Os testes dirigidos e `go test -count=1 ./...`
passaram.

Como a semântica transacional mudou, a prova PostgreSQL 15.19 foi repetida. O
RED terminou em ROLLBACK; a fixture válida usou `BEGIN`/`END` e persistiu
atomicamente apenas a FUNCTION endurecida. As quatro autoridades, os objetos
preexistentes, o schema não público e default ACL permaneceram conforme os
invariantes; o container foi removido.

O review seguinte encontrou 2 P1 adicionais: um bloco executável `DO
$tag$...$tag$` podia ocultar SQL dinâmico do gate, e o alias legado `ALTER
GROUP ... ADD/DROP USER` podia mudar membership sem ser reconhecido. Os quatro
P1 arquiteturais históricos e os 3 P1 + 1 P2 anteriores permaneceram
fechados.

REDs dirigidos reproduziram os dois bypasses. A correção fail-closed rejeita
qualquer comando externo `DO` nas migrations sob enforcement e toda mutação
`ALTER GROUP`, sem interpretar PL/pgSQL. O splitter continua descartando com
segurança comentários e corpos dollar-quoted de `CREATE FUNCTION`, que não
são comandos externos. Os testes dirigidos, toda a suíte migrationguard e
`go test -count=1 ./...` em Go 1.22 passaram. A prova PostgreSQL 15.19 não foi
repetida, pois a semântica ACL/transacional não mudou.

O review seguinte encontrou 3 P1: membership `GRANT/REVOKE` standalone era
descartado quando não havia FUNCTION; SELECT/CALL podia executar efeitos
ocultos de uma FUNCTION criada; e `DROP FUNCTION/ROUTINE/PROCEDURE` era
ignorado. REDs dirigidos reproduziram todos os casos, incluindo cada variante
de membership e DROP.

A correção fail-closed torna membership e DROP findings incondicionais sob
enforcement. Em migrations que criam FUNCTION, CALL e SELECT contendo forma
de chamada também falham sem tentativa de interpretar efeitos arbitrários;
SELECT literal, agrupamento aritmético, comentários e corpos dollar-quoted
continuam aceitos. Os testes dirigidos e toda a suíte migrationguard passaram
offline em Go 1.22. `go test -count=1 ./...` foi tentado, mas ficou BLOQUEADO
antes da compilação porque o host não possui Go e o container offline não
continha os módulos nem acesso à rede; este gate não é declarado PASS. A prova
PostgreSQL 15.19 não foi repetida porque ACL e framing transacional não
mudaram.

O review atual preservou as regressões anteriores e encontrou 2 P1: o nome de
role citado `"on"` confundia a classificação textual de ACL; e VALUES/INSERT,
além de outras formas executáveis, podiam chamar FUNCTION porque a detecção
enumerava apenas SELECT/CALL. REDs reproduziram GRANT/REVOKE para `"on"`,
VALUES e INSERT...VALUES; a cobertura do splitter incluiu identificador citado
com `--`.

A correção classifica membership pela ordem de keywords não citadas (`ON`
antes de `TO/FROM` para ACL; `TO/FROM` primeiro para membership), preserva
identificadores entre aspas duplas e adota allowlist para qualquer migration
que crie FUNCTION. Somente framing explícito, CREATE FUNCTION canônico e ACLs
exatos são aceitos; SELECT, CALL, VALUES, INSERT, UPDATE, DELETE, WITH e todo
statement não modelado falham fechado. Os 2 REDs dirigidos, toda a suíte
migrationguard e as regressões anteriores passaram offline em Go 1.22.
`go test -count=1 ./...` permanece BLOQUEADO antes da compilação pela ausência
de módulos no container offline e não é declarado PASS. A prova PostgreSQL
15.19 não foi repetida porque ACL e framing transacional não mudaram.

O review atual encontrou 1 P1 + 1 P2. Uma escape string `E'...'` contendo
`\'` podia fazer o splitter fundir statement executável posterior ao CREATE
FUNCTION; um prefixo numérico maior que `int` fazia `Atoi` falhar e o arquivo
numeric-looking ser ignorado. Os REDs reproduziram zero findings nos dois
casos.

A correção tokeniza `E'...'` respeitando backslash escapes e aspas duplicadas,
preservando strings simples e dollar-quoted bodies. A classificação de nomes
rejeita lexicalmente todo `.sql` iniciado por dígito que não corresponda ao
formato canônico; conversão numérica só ocorre após o match seguro de quatro
dígitos. Os testes dirigidos, toda a suíte migrationguard e as regressões
anteriores passaram offline em Go 1.22. `go test -count=1 ./...` permanece
BLOQUEADO antes da compilação pela ausência de módulos no container offline e
não é declarado PASS. A prova PostgreSQL 15.19 não foi repetida porque ACL e
framing transacional não mudaram.

O review posterior fechou o 1 P1 + 1 P2 anteriores, preservou as regressões e
encontrou um novo P1 arquitetural: `$tag$` dentro de identificador PostgreSQL
não citado era interpretado como dollar quote pelo splitter. O RED obrigatório
com `cover$tag$()`, `cover()` e `decoy$tag$()` falhou antes do patch com zero
findings, embora somente `cover()` tivesse ACL endurecida.

A opção B foi então materializada como três camadas explícitas, somente com Go
stdlib: lexer único com tokens/posição, parser estrutural mínimo consumindo
apenas tokens e policy state-machine allowlist. O lexer preserva `$` em
identificadores, reconhece dollar quotes somente em boundary válida, distingue
`$1`, strings simples, `E'...'`, quoted identifiers e comentários, suporta
comentários de bloco aninhados e falha fechado em NUL ou construct não
terminado. Statements são divididos somente por `;` lexicalmente estrutural.

O parser não usa regex, `strings.Fields` ou busca textual sobre SQL bruto para
decisões estruturais. Em migration posterior à 0022 que crie FUNCTION, a
policy aceita um único BEGIN, um ou mais blocos adjacentes CREATE FUNCTION
`public` canônico + REVOKE exato de PUBLIC/anon/authenticated + GRANT direto a
service_role e um COMMIT/END final. Qualquer statement extra ou sintaxe não
modelada falha fechado.

A prova PostgreSQL 15 efêmera confirmou que o RED é SQL válido e cria três
assinaturas distintas: apenas `cover()` perde EXECUTE para anon/authenticated;
`cover$tag$()` e `decoy$tag$()` mantêm o EXECUTE herdado de PUBLIC, e as três
permanecem owner postgres. O container foi removido.

```text
RED obrigatório pré-patch -> FAIL com zero findings
RED obrigatório pós-patch -> PASS; sete statements preservados
matriz lexical e regressões históricas -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
go test -run=^$ -fuzz=FuzzLexer -fuzztime=30s ./internal/migrationguard ->
  PASS; 1.629.683 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
```

A suíte completa usou rede apenas dentro de container descartável para obter
dependências já fixadas no `go.mod`; nenhum módulo foi adicionado ou alterado.

O rollback é por objeto e orientado pelo snapshot. Antes de criar ou substituir
uma assinatura, capturar existência, owner, identidade completa, ACL direto,
grant option e autoridade efetiva das roles relevantes. Para objeto novo, o
rollback da feature remove somente a assinatura criada; se ela precisar
permanecer, restaurar apenas o delta para o ACL contrafactual capturado dos
defaults vigentes. Para `CREATE OR REPLACE` autorizado em lote próprio,
restaurar exatamente o ACL anterior. Nenhum rollback pode usar GRANT ou REVOKE
blanket/incondicional.

O review seguinte encontrou três P1 no redesenho: a boundary de identificador
usava categorias Unicode diferentes da classe byte-a-byte do PostgreSQL 15 e
permitia que U+0301 antes de `$tag$` fundisse statements; a normalização de
tipos eliminava boundaries e confundia `double precision` com
`doubleprecision`; e `DROP OWNED` não era classificado como mutação proibida.

Os três REDs falharam antes do patch com zero findings. A correção local alinha
identificadores não citados à classe lexical relevante do PostgreSQL 15,
preserva boundaries entre tokens word/number na identidade canônica de tipos
e rejeita `DROP OWNED` incondicionalmente em migrations pós-0022. A matriz de
tipos cobre compound types, schema-qualified, arrays, typmods e nomes de
argumento opcionais sem aceitar ACL de outro overload.

A prova PostgreSQL 15.19 efêmera confirmou que os identificadores com U+0301
antes de `$tag$` são SQL válido e que os dois CREATE TABLE e o GRANT
intermediário executam como três statements distintos. A prova terminou em
`ROLLBACK` e o container foi removido.

```text
3 REDs pré-patch -> FAIL com zero findings
3 P1 dirigidos pós-patch -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
go test -run=^$ -fuzz=FuzzLexer -fuzztime=30s ./internal/migrationguard ->
  PASS; 1.311.947 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
```

O review seguinte confirmou fechados os três P1 pós-redesenho e encontrou 1 P1
+ 1 P2: DDL de EXTENSION podia criar routines sem passar pela allowlist quando
não havia `CREATE FUNCTION`, e o resumo superior do tracker/handoff ainda
registrava somente o antigo finding de dollar quote.

O RED exato `CREATE EXTENSION pgcrypto WITH SCHEMA public;` falhou antes do
patch com zero findings. A correção classifica estruturalmente `CREATE`,
`ALTER` e `DROP EXTENSION` como mutações que exigem autorização separada em
migrations posteriores à 0022, sem interpretar scripts de extensão. A prova
PostgreSQL 15.19 efêmera materializou 36 routines de `pgcrypto` em `public`,
todas executáveis por `anon` via PUBLIC; terminou em `ROLLBACK` e removeu o
container.

```text
RED CREATE EXTENSION pré-patch -> FAIL com zero findings
CREATE/ALTER/DROP EXTENSION pós-patch -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
go test -run=^$ -fuzz=FuzzLexer -fuzztime=30s ./internal/migrationguard ->
  PASS; 2.031.915 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
```

O review seguinte confirmou o P2 documental e encontrou um P1 lexical: tab
vertical (`\v`), whitespace válido no PostgreSQL 15, era emitido como token e
permitia que CREATE/ALTER/DROP EXTENSION, CREATE/DROP FUNCTION,
DROP/REASSIGN OWNED e ALTER GROUP escapassem da classificação estrutural.

Os oito REDs retornaram zero findings antes do patch. A correção acrescenta
`\v` somente ao conjunto de whitespace de `isSQLSpace`, ao lado de espaço,
`\t`, `\n`, `\r` e `\f`. Testes cobrem os separadores isolados/combinados e
preservação de strings, quoted identifiers, comentários e dollar-quoted
bodies. Nenhuma policy ou parser ganhou busca textual ou caso por keyword.

```text
8 REDs com tab vertical pré-patch -> FAIL; zero findings em todos
8 comandos + matriz de whitespace/contextos pós-patch -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
go test -run=^$ -fuzz=FuzzLexer -fuzztime=30s ./internal/migrationguard ->
  PASS; 1.371.301 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
```

O review seguinte encontrou um P1 de policy: fora das migrations com CREATE
FUNCTION, qualquer statement classificado como `statementUnknown` terminava
sem finding. O RED concreto com CREATE AGGREGATE provou que uma routine podia
ser criada com EXECUTE herdado de PUBLIC; CREATE/ALTER/DROP AGGREGATE, ALTER
TABLE ... ADD COLUMN, DROP SCHEMA ... CASCADE, CREATE TRIGGER, INSERT, as seis
formas de whitespace e múltiplos unknowns no mesmo arquivo retornaram zero
findings antes do patch.

A policy global agora rejeita cada `statementUnknown` com filename e byte
offset. A correção não adiciona AGGREGATE a uma denylist, não altera lexer ou
parser e não cria gramática PostgreSQL geral. Arquivos vazios/só comentários,
FUNCTION canônica, findings específicos, regressões históricas e migrations
até 0022 permanecem preservados.

```text
REDs obrigatórios pré-patch -> FAIL; zero findings em todos
REDs pós-patch + seis whitespaces + múltiplos unknowns -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
go test -run=^$ -fuzz=FuzzLexer -fuzztime=30s ./internal/migrationguard ->
  PASS; 2.630.215 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
git diff --check / git diff --cached --check -> PASS
```

PostgreSQL 15.19 efêmero criou o AGGREGATE em transação, confirmou
`prokind='a'` e EXECUTE herdado de PUBLIC. O `ROLLBACK` confirmou ausência do
objeto e da role de prova; o container foi removido.

**Estado registrado naquela rodada:** o fail-closed de `statementUnknown`
estava implementado localmente e aguardava novo `/review`. O review seguinte
confirmou esse P1 fechado e encontrou um P1 nos kinds transacionais modelados.

Sem CREATE FUNCTION, `BEGIN`, `COMMIT`, `END`, `START TRANSACTION`, `ROLLBACK`
e `ABORT`, isolados ou combinados, terminavam com zero findings. REDs
pré-patch reproduziram cada comando isolado, `BEGIN`/`COMMIT`, `BEGIN`/`END` e
a ausência do finding transacional antes/depois de unknown.

A policy agora rejeita cada kind transacional quando a state machine de
FUNCTION não está ativa. Quando há CREATE FUNCTION, somente o BEGIN inicial e
o COMMIT/END final consumidos pela state machine permanecem permitidos;
framing incompleto, transação extra, `START TRANSACTION`, `ROLLBACK` e `ABORT`
continuam rejeitados. Lexer e parser não foram alterados.

```text
REDs transacionais pré-patch -> FAIL; zero findings nos casos sem FUNCTION
REDs pós-patch + adjacência com unknown -> PASS
FUNCTION canônica + framing inválido -> PASS
go test -count=1 -v ./internal/migrationguard -> PASS em Go 1.22
go test -run=^$ -fuzz=FuzzLexer -fuzztime=30s ./internal/migrationguard ->
  PASS; 1.806.552 execuções
go test -count=1 ./... -> PASS em Go 1.22
gofmt -l -> vazio
git diff --check / git diff --cached --check -> PASS
```

PostgreSQL efêmero não foi repetido porque o patch é exclusivamente de policy.
Migration 0022 permanece intacta, 0023 ausente e nenhum SQL de produção,
deploy, smoke, commit ou push foi executado.

**Estado registrado naquela rodada:** o gate estava **EM CORREÇÃO APÓS
REVIEW**; o P1 transacional estava corrigido localmente, `statementUnknown`
permanecia fail-closed e a próxima ação era um novo `/review`. O Lote 2A ainda
não estava concluído naquele checkpoint. Esse estado foi posteriormente
superado pelo review limpo e pela integração do PR #77; o checkpoint vigente é
o bloco pós-review/pós-merge da seção 14. O Lote 2A está concluído e integrado
em `main`; o Lote 2B continua não autorizado. Nenhum SQL de produção ou
migration 0023 foi executado, e DDL manual/externo permanece
`UNKNOWN_BLOCKED`.

### Lote 2B — RPC backend-only atual

Alvo exato: `refresh_available_segments_for_route(text)`. Revogar EXECUTE de
PUBLIC, anon e authenticated; preservar postgres/service_role.

RED seguro somente em banco efêmero. O pós-check de produção mantém a negação
de `POST /rest/v1/rpc/refresh_available_segments_for_route` para anon e
authenticated.

Enquanto o `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` estiver ativo, a prova
positiva do consumidor backend em produção:

- exclui explicitamente qualquer create/update/write pelo fluxo `trips`;
- não pode provocar deliberadamente trigger/enqueue de sheet sync;
- deve usar um call path versionado cuja inspeção prévia prove ausência de
  escrita em `trips`, `bookings`, `passengers` e
  `booking_payment_details`, seguido por verificação read-only do resultado;
- permanece bloqueada se nenhum caminho assim puder ser comprovado; ausência
  de DML não dispensa a prova de saúde do consumidor.

Qualquer prova funcional que precise de DML em `trips` ocorre exclusivamente
em PostgreSQL efêmero/de teste. O caminho candidato por
`route_segment_prices` só pode ser usado em produção após o pré-check provar
que o fluxo implantado escreve apenas fora das tabelas-fonte de sheet sync;
essa descrição não autoriza o smoke. Rollback: restaurar os três grants
diretos.

### Lote 2C — trigger functions

Alvos: as seis functions `TRIGGER_INTERNAL`. Antes da mutação, obter as cinco
definições de sheet sync, confirmar triggers habilitados e provar em
PostgreSQL efêmero que triggers já criados continuam funcionando sem EXECUTE
de PUBLIC/anon/auth. Depois, revogar dessas três autoridades e preservar
postgres/service_role. Rollback: restaurar grants diretos nas seis signatures.

Status: **BLOCKED pela prova técnica e DDL ausente das cinco functions de
sheet sync**.

### Lote 2D — routines UNKNOWN

As oito routines `UNKNOWN_BLOCKED` não sofrem mutação. Para liberar, obter
definição implantada e consumidor/protocolo legítimo. Status: **BLOCKED**.

### Lote 3A — default privileges futuros de SEQUENCE

Revogar `SELECT/UPDATE/USAGE` do default ACL para anon/authenticated;
preservar postgres/service_role. Verificar em banco efêmero. Rollback:
restaurar os três privilégios.

### Lote 3B — sequence atual

Alvo: `sheet_sync_queue_id_seq`. Nenhuma mutação até esclarecer o DDL de
`sheet_sync_queue.id`, produtor, consumidor, role e protocolo. Status:
**BLOCKED**.

### Lote 4 — identidade interna

`user_profiles`, `user_roles`; depende de `roles` permanecer endurecida.
Smoke: Auth, `/users/me`, listagem e alteração controlada de acesso.

### Lote 5 — afiliados

`affiliate_recipients`, `affiliate_withdrawals`; depende do lote 4. Smoke de
balance/histórico; saque somente sem transferência real.

### Lote 6 — estado de chat

`chat_sessions`, `chat_messages`, `chat_tool_calls`, `chat_handoffs`. Smoke de
sessão, mensagens, handoff e ledger controlado.

### Lote 7 — delivery e jobs

`outbound_messages`, `automation_job_runs`. Smoke sem envio real.

### Lote 8 — referência de rotas

`routes`, `stops`. Smoke de routes/stops/availability.

### Lote 9 — viagens e paradas

`trips`, `trip_stops`; depende do lote 8 e do
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`. A prova de que o trigger de
`trips` continua enfileirando é obrigatória somente em PostgreSQL efêmero/de
teste. Enquanto o guard estiver ativo, nenhum smoke em produção provoca DML
nessas tabelas apenas para testar o enqueue; a parcela de verificação que
exigir essa escrita permanece bloqueada.

### Lote 10 — preço e projeção de disponibilidade

`route_segment_prices`, `available_segments`; depende de 2B, 8 e 9. Smoke de
preços, availability e refresh controlado. Por depender do lote 9, também
respeita o `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` e não pode usar DML
sintético em `trips` como atalho de verificação em produção.

### Lote 11 — reservas

`bookings`; depende de 8, 9 e do `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`.
A prova de enqueue do trigger ocorre somente em PostgreSQL efêmero/de teste.
Enquanto o guard estiver ativo, não há fixture nem DML deliberado em produção
apenas para provar o trigger; essa parcela do lote permanece bloqueada.

### Lote 12 — passageiros

`passengers`; depende de 11 e do `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`.
Smoke de consumidor sem escrita deliberada nessa tabela em produção enquanto
o guard estiver ativo; prova comportamental do trigger somente em PostgreSQL
efêmero/de teste.

### Lote 13 — detalhe financeiro da reserva

`booking_payment_details`; depende de 11 e do
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`. Smoke transacional que escreva
nessa tabela e prova do trigger ocorrem somente em PostgreSQL efêmero/de teste
enquanto o guard estiver ativo.

### Lote 14 — pagamentos e eventos

`payments`, `payment_events`; depende de 11 e 13. Nenhuma cobrança real ou
replay de webhook real. Também respeita o
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`: não pode provocar escrita em
`bookings` ou `booking_payment_details` em produção como efeito colateral de
seu smoke enquanto o subsistema permanecer UNKNOWN.

### Lote 15 — manifesto derivado

`manifest_data`; depende de 2D, 9, 11, 12 e 13. Status: **BLOCKED até
classificar as routines de refresh/enqueue de manifesto**. Mesmo após essa
classificação, não pode avançar sem satisfazer o
`SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`.

### Lote 16 — documento fiscal já sem grants

`fiscal_documents`. Confirmar owner; habilitar apenas RLS, sem mudar grants ou
criar policy. Rollback: desabilitar RLS.

### Lote 17 — UNKNOWN_BLOCKED

Sem mutação:

- `sheet_sync_queue` e sua sequence;
- seis routines RPC/callable de sheet sync sem consumidor;
- `travel_authorizations`;
- `travel_authorization_check_items`;
- `trip_contractors`.

Para liberar sheet sync, identificar worker/scheduler, local, protocolo,
role/credencial lógica, DDL implantado e atividade atual.

## 10. Invariantes

- BACKEND_ONLY não permanece acessível a anon/authenticated.
- UNKNOWN_BLOCKED não sofre mutação direta nem indireta deliberada.
- Nenhum smoke em produção provoca DML nas tabelas-fonte apenas para provar
  trigger/enqueue enquanto o `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD`
  estiver ativo.
- O pós-check de produção do lote 2B nunca usa write pelo fluxo `trips`; uma
  prova positiva sem caminho backend seguro e comprovado permanece bloqueada.
- Revoke de anon/auth não basta enquanto PUBLIC mantiver EXECUTE.
- Nenhuma routine BACKEND_ONLY permanece executável por PUBLIC/anon/auth.
- service_role permanece inalterado; nenhum grant global pode ser criado sem
  snapshot e autorização compatíveis.
- trigger function só é endurecida após prova efêmera.
- o default ACL global de FUNCTION permanece fora da solução do Lote 2A;
- toda nova FUNCTION `public` coberta por DDL versionado é schema-qualified e
  endurecida atomicamente por assinatura antes do commit;
- DDL manual/externo permanece risco residual `UNKNOWN_BLOCKED` e nunca é
  declarado protegido pela política versionada;
- hardening de RPC não substitui tabela e vice-versa.
- smoke API não prova segurança Data API e negação Data API não prova saúde da API.
- nenhum segredo aparece em output.

## 11. Gates canônicos

1. plano aprovado;
2. `/goal` de somente um lote;
3. pré-check SQL read-only;
4. `SHEET_SYNC_UNKNOWN_PRODUCTION_WRITE_GUARD` satisfeito no lote 2B, nos
   lotes 9, 11, 12, 13 e em todos os dependentes;
5. RED seguro quando necessário, exclusivamente efêmero se puder escrever em
   objeto UNKNOWN;
6. migration mínima;
7. pós-check SQL;
8. smoke Data API negativo anon;
9. smoke Data API negativo authenticated;
10. smoke positivo do consumidor dentro do guard;
11. smoke API Go quando aplicável e dentro do guard;
12. `/review` completo;
13. correção de P0/P1/P2;
14. novo `/review` limpo;
15. reconciliação documental;
16. autorização explícita do lote seguinte.

Nenhum lote seguinte é liberado automaticamente.

## 12. Definition of Done

- todas as 29 tabelas classificadas definitivamente;
- nenhum BACKEND_ONLY acessível a anon/auth sem justificativa;
- qualquer DATA_API_REQUIRED com grants mínimos e policies explícitas;
- UNKNOWN resolvido ou comprovadamente fora da superfície;
- default privileges de tables/sequences endurecidos nos lotes próprios e
  política por assinatura de novas FUNCTIONs `public` versionadas implementada;
- functions/RPC e sequences atuais classificadas;
- smokes negativos e positivos verdes;
- API Go saudável pelo PostgreSQL direto;
- autoridade da Data API de anon/authenticated reduzida conforme as
  classificações e os grants/policies mínimos do plano;
- a chave antiga associada ao incidente permanece rotacionada/revogada
  conforme o checkpoint operacional; esse é um controle histórico separado,
  não uma garantia produzida por este hardening;
- risco residual aceito e registrado: `postgres` e `service_role` permanecem
  privilegiados e com `BYPASSRLS`; uma credencial válida comprometida de uma
  dessas roles conserva autoridade até o backlog específico ser executado;
- review final sem P0/P1/P2;
- tracker e handoff reconciliados.

## 13. Backlog fora do track

- migrar a API de `postgres` para role PostgreSQL dedicada;
- reavaliar grants de service_role somente com prova de não uso;
- tratar separadamente o risco de comprometimento de credenciais válidas de
  `postgres`/`service_role`, que não é encerrado por este track;
- avaliar redução futura dos schemas expostos pelo PostgREST.

## 14. Estado após materialização

**Status:** SEC-2026-08-18 — LOTE 2A / OPÇÃO B CONCLUÍDO E INTEGRADO EM MAIN;
REVIEW FINAL SEM P0/P1/P2; COMMIT
`d1b74906ae73f6e543440fdaf69077b28a2264bf`; PR #77 MERGEADO; MERGE/MAIN
`6ffc50fa217fdbc1ed1a315e69d3fa978125e695`; MIGRATION GUARD FAIL-CLOSED
INTEGRADO; `statementUnknown` E TRANSAÇÕES FORA DA FUNCTION CANÔNICA
PROTEGIDOS; 0022 PRESERVADA E 0023 AUSENTE; LOTE 2B E PRODUÇÃO NÃO
AUTORIZADOS.

Este arquivo documenta a política versionada aprovada, a evidência operacional
sanitizada e o gate integrado. O workflow `Publish API to GHCR` #157 terminou
com SUCCESS; publicação da imagem não equivale a deploy ou smoke. Não há
migration executável do Lote 2A, mudança de default ACL global, efeito
cross-schema, SQL de produção, deploy ou smoke deste lote. DDL externo
permanece `UNKNOWN_BLOCKED`. O Lote 2B e todos os sucessores permanecem
bloqueados até autorização explícita separada.

O último review não repetiu `go test ./...` por limitação ambiental, sem
observar falha de código; `migrationguard`, fuzz por 30s, gofmt e diff checks
passaram, e o goal anterior registrou a suíte completa PASS. O review declarou
os testes suficientes e o working tree seguro. Estado desta reconciliação:
**Lote 2A concluído e integrado / reconciliação documental corrigida
localmente / aguardando novo `/review` / Lote 2B não autorizado**. O track
funcional mantém sua própria próxima ação sem alteração.
