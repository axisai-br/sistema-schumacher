-- Habilita acesso da nova tela "Saldo" para todos os usuários atualmente cadastrados.
-- Regra aplicada: qualquer usuário existente em user_profiles recebe role "financeiro".
-- Observação: esta migration não cria vínculos de recipient_id, apenas libera acesso de UI/rota.

insert into roles (name)
values ('financeiro')
on conflict (name) do nothing;

insert into user_roles (user_id, role_id)
select up.id as user_id, r.id as role_id
from user_profiles up
join roles r on r.name = 'financeiro'
left join user_roles ur
  on ur.user_id = up.id
 and ur.role_id = r.id
where ur.user_id is null;
