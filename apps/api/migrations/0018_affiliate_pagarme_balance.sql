create table if not exists affiliate_recipients (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references user_profiles(id) on delete cascade,
  recipient_id text not null,
  is_active boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (user_id)
);

create table if not exists affiliate_withdrawals (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references user_profiles(id) on delete cascade,
  recipient_id text not null,
  provider text not null default 'PAGARME',
  amount_cents bigint not null check (amount_cents > 0),
  currency text not null default 'BRL',
  status text not null default 'PENDING',
  transfer_id text,
  idempotency_key text not null,
  provider_request_payload jsonb,
  provider_response_payload jsonb,
  provider_error text,
  requested_at timestamptz not null default now(),
  processed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (idempotency_key),
  unique (transfer_id)
);

create table if not exists pagarme_webhook_events (
  id uuid primary key default gen_random_uuid(),
  provider text not null default 'PAGARME',
  event_id text,
  event_type text,
  transfer_id text,
  signature text,
  payload_hash text not null,
  payload jsonb not null,
  process_status text not null default 'RECEIVED',
  processing_error text,
  received_at timestamptz not null default now(),
  processed_at timestamptz,
  unique (provider, payload_hash)
);

create unique index if not exists idx_pagarme_webhook_events_provider_event
  on pagarme_webhook_events(provider, event_id)
  where event_id is not null;

create index if not exists idx_affiliate_withdrawals_user_created_at
  on affiliate_withdrawals(user_id, created_at desc);

create index if not exists idx_affiliate_withdrawals_recipient_created_at
  on affiliate_withdrawals(recipient_id, created_at desc);

create index if not exists idx_affiliate_withdrawals_status_created_at
  on affiliate_withdrawals(status, created_at desc);

create index if not exists idx_affiliate_recipients_recipient_id
  on affiliate_recipients(recipient_id);
