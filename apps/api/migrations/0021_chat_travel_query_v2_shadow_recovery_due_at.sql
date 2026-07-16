alter table public.chat_messages
  add column if not exists travel_query_v2_shadow_recovery_due_at timestamptz;

-- The V2 shadow has not been deployed yet, so production has no rows to
-- backfill. This one-time marker keeps legacy claims created by tests or local
-- environments recoverable without making the runtime sweeper scan the table.
update public.chat_messages message
set travel_query_v2_shadow_recovery_due_at = statement_timestamp()
where message.travel_query_v2_shadow_recovery_due_at is null
  and jsonb_typeof(message.normalized_payload -> 'travel_query_v2_shadow_claims') = 'object'
  and exists (
    select 1
    from jsonb_each(message.normalized_payload -> 'travel_query_v2_shadow_claims') claim(key, value)
    where claim.value ->> 'status' = 'IN_PROGRESS'
  );

create index if not exists idx_chat_messages_travel_query_v2_shadow_recovery_due_at
  on public.chat_messages (travel_query_v2_shadow_recovery_due_at, id)
  where travel_query_v2_shadow_recovery_due_at is not null;
