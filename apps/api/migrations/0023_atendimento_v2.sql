-- Atendimento v2: conversas, mensagens, turnos do agente e aliases de cidades.
-- Acesso somente pela API (service_role / owner). Como a 0022 revoga os
-- privilegios padrao de anon/authenticated para novas tabelas, revogamos
-- tambem explicitamente aqui para nao depender da ordem de execucao.

create table if not exists atd_conversas (
  id uuid primary key default gen_random_uuid(),
  canal text not null default 'WHATSAPP',
  contato text not null,
  telefone text,
  nome text,
  status text not null default 'BOT',
  responsavel_id uuid,
  estado jsonb not null default '{}'::jsonb,
  versao int not null default 0,
  pendente_desde timestamptz,
  ultima_entrada_em timestamptz,
  humano_ate timestamptz,
  processando_ate timestamptz,
  criado_em timestamptz not null default now(),
  atualizado_em timestamptz not null default now(),
  unique (canal, contato)
);

create table if not exists atd_mensagens (
  id uuid primary key default gen_random_uuid(),
  seq bigint generated always as identity,
  conversa_id uuid not null references atd_conversas(id) on delete cascade,
  direcao text not null,
  autor text not null,
  tipo text not null default 'TEXTO',
  texto text,
  midia jsonb,
  provedor_id text unique,
  recebida_em timestamptz,
  turno_id uuid,
  criado_em timestamptz not null default now()
);

create table if not exists atd_turnos (
  id uuid primary key default gen_random_uuid(),
  conversa_id uuid not null references atd_conversas(id) on delete cascade,
  entrada_ids uuid[] not null default '{}',
  passos jsonb not null default '[]'::jsonb,
  estado_antes jsonb,
  estado_depois jsonb,
  resposta text,
  resultado text,
  modelo text,
  tokens_entrada int not null default 0,
  tokens_saida int not null default 0,
  latencia_ms int not null default 0,
  erro text,
  criado_em timestamptz not null default now()
);

create table if not exists atd_cidades_alias (
  alias text primary key,
  stop_id text not null references stops(stop_id)
);

create index if not exists idx_atd_conversas_fila
  on atd_conversas (pendente_desde)
  where status = 'BOT' and pendente_desde is not null;

create index if not exists idx_atd_conversas_atualizado
  on atd_conversas (atualizado_em desc);

create index if not exists idx_atd_mensagens_conversa_criado
  on atd_mensagens (conversa_id, criado_em);

create index if not exists idx_atd_turnos_conversa_criado
  on atd_turnos (conversa_id, criado_em);

revoke all on table atd_conversas, atd_mensagens, atd_turnos, atd_cidades_alias from anon, authenticated;
