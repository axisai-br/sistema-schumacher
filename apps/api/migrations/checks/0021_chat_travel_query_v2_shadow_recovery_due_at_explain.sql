\set ON_ERROR_STOP on

begin;

create temporary table chat_messages (
  id bigint primary key,
  travel_query_v2_shadow_recovery_due_at timestamptz
);

create index idx_chat_messages_travel_query_v2_shadow_recovery_due_at
  on chat_messages (travel_query_v2_shadow_recovery_due_at, id)
  where travel_query_v2_shadow_recovery_due_at is not null;

-- Many ordinary messages must stay outside the partial index.
insert into chat_messages (id, travel_query_v2_shadow_recovery_due_at)
select value, null
from generate_series(1, 20000) value;

-- More due messages than one runtime batch prove that LIMIT remains effective.
insert into chat_messages (id, travel_query_v2_shadow_recovery_due_at)
select 20000 + value, statement_timestamp() - interval '1 minute'
from generate_series(1, 120) value;

analyze chat_messages;
set local enable_seqscan = off;

do $$
declare
  candidate_count integer;
  candidate_plan jsonb;
begin
  execute $explain$
    explain (format json, costs off)
    select message.id
    from chat_messages message
    where message.travel_query_v2_shadow_recovery_due_at is not null
      and message.travel_query_v2_shadow_recovery_due_at <= statement_timestamp()
    order by message.travel_query_v2_shadow_recovery_due_at, message.id
    limit 50
    for update of message skip locked
  $explain$ into candidate_plan;

  if candidate_plan::text not like '%idx_chat_messages_travel_query_v2_shadow_recovery_due_at%' then
    raise exception 'candidate query did not use recovery index: %', candidate_plan;
  end if;

  select count(*)
  into candidate_count
  from (
    select message.id
    from chat_messages message
    where message.travel_query_v2_shadow_recovery_due_at is not null
      and message.travel_query_v2_shadow_recovery_due_at <= statement_timestamp()
    order by message.travel_query_v2_shadow_recovery_due_at, message.id
    limit 50
    for update of message skip locked
  ) candidates;

  if candidate_count <> 50 then
    raise exception 'candidate batch was %, expected 50', candidate_count;
  end if;
end $$;

rollback;
