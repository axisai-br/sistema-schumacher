-- Prevent future postgres-owned tables in public from inheriting Data API
-- table privileges. Existing objects and service_role grants are unchanged.
alter default privileges for role postgres in schema public
  revoke select, insert, update, delete, truncate, references, trigger
  on tables from anon;

alter default privileges for role postgres in schema public
  revoke select, insert, update, delete, truncate, references, trigger
  on tables from authenticated;
