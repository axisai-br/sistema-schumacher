alter table public.passengers
  add column if not exists cpf text,
  add column if not exists rg text,
  add column if not exists cnh text,
  add column if not exists birth_date date,
  add column if not exists birth_certificate_number text,
  add column if not exists birth_city text;

alter table public.passengers
  alter column birth_date type date
  using case
    when nullif(btrim(birth_date::text), '') ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' then birth_date::date
    when nullif(btrim(birth_date::text), '') ~ '^[0-9]{2}-[0-9]{2}-[0-9]{4}$' then to_date(birth_date::text, 'DD-MM-YYYY')
    else null
  end;

update public.passengers
set cpf = nullif(regexp_replace(cpf, '[^0-9]', '', 'g'), '')
where cpf is not null;

update public.passengers
set birth_certificate_number = nullif(regexp_replace(birth_certificate_number, '[^0-9]', '', 'g'), '')
where birth_certificate_number is not null;

-- Operational validation plan:
-- The constraints below use NOT VALID to avoid blocking rollout while legacy rows
-- are audited. PostgreSQL still checks new inserts and updates. After correcting
-- any legacy rows that violate the rules, validate the constraints with:
-- alter table public.passengers validate constraint passengers_cpf_digits_check;
-- alter table public.passengers validate constraint passengers_birth_certificate_number_digits_check;
-- alter table public.passengers validate constraint passengers_birth_date_range_check;
-- If public.booking_passengers exists in the target database, also run:
-- alter table public.booking_passengers validate constraint booking_passengers_cpf_digits_check;
-- alter table public.booking_passengers validate constraint booking_passengers_birth_certificate_number_digits_check;
-- alter table public.booking_passengers validate constraint booking_passengers_birth_date_range_check;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'passengers_cpf_digits_check'
      and conrelid = 'public.passengers'::regclass
  ) then
    alter table public.passengers
      add constraint passengers_cpf_digits_check
      check (cpf is null or cpf ~ '^[0-9]{11}$') not valid;
  end if;
end $$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'passengers_birth_certificate_number_digits_check'
      and conrelid = 'public.passengers'::regclass
  ) then
    alter table public.passengers
      add constraint passengers_birth_certificate_number_digits_check
      check (birth_certificate_number is null or birth_certificate_number ~ '^[0-9]{32}$') not valid;
  end if;
end $$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'passengers_birth_date_range_check'
      and conrelid = 'public.passengers'::regclass
  ) then
    alter table public.passengers
      add constraint passengers_birth_date_range_check
      check (birth_date is null or birth_date between date '1900-01-01' and current_date) not valid;
  end if;
end $$;

do $$
begin
  if to_regclass('public.booking_passengers') is not null then
    alter table public.booking_passengers
      add column if not exists cpf text,
      add column if not exists rg text,
      add column if not exists cnh text,
      add column if not exists birth_date date,
      add column if not exists birth_certificate_number text,
      add column if not exists birth_city text;

    alter table public.booking_passengers
      alter column birth_date type date
      using case
        when nullif(btrim(birth_date::text), '') ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' then birth_date::date
        when nullif(btrim(birth_date::text), '') ~ '^[0-9]{2}-[0-9]{2}-[0-9]{4}$' then to_date(birth_date::text, 'DD-MM-YYYY')
        else null
      end;

    update public.booking_passengers
    set cpf = nullif(regexp_replace(cpf, '[^0-9]', '', 'g'), '')
    where cpf is not null;

    update public.booking_passengers
    set birth_certificate_number = nullif(regexp_replace(birth_certificate_number, '[^0-9]', '', 'g'), '')
    where birth_certificate_number is not null;

    if not exists (
      select 1 from pg_constraint
      where conname = 'booking_passengers_cpf_digits_check'
        and conrelid = 'public.booking_passengers'::regclass
    ) then
      alter table public.booking_passengers
        add constraint booking_passengers_cpf_digits_check
        check (cpf is null or cpf ~ '^[0-9]{11}$') not valid;
    end if;

    if not exists (
      select 1 from pg_constraint
      where conname = 'booking_passengers_birth_certificate_number_digits_check'
        and conrelid = 'public.booking_passengers'::regclass
    ) then
      alter table public.booking_passengers
        add constraint booking_passengers_birth_certificate_number_digits_check
        check (birth_certificate_number is null or birth_certificate_number ~ '^[0-9]{32}$') not valid;
    end if;

    if not exists (
      select 1 from pg_constraint
      where conname = 'booking_passengers_birth_date_range_check'
        and conrelid = 'public.booking_passengers'::regclass
    ) then
      alter table public.booking_passengers
        add constraint booking_passengers_birth_date_range_check
        check (birth_date is null or birth_date between date '1900-01-01' and current_date) not valid;
    end if;
  end if;
end $$;
