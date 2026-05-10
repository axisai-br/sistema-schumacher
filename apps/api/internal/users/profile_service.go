package users

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"schumacher-tur/api/internal/auth"
)

var ErrUserProfileNotConfigured = errors.New("user profile could not be configured from authenticated user")

type Profile struct {
	ID       string
	Email    string
	FullName string
}

type ProfileService struct {
	pool *pgxpool.Pool
}

func NewProfileService(pool *pgxpool.Pool) *ProfileService {
	return &ProfileService{pool: pool}
}

func (s *ProfileService) EnsureUserProfileIDFromAuth(ctx context.Context, user auth.AuthUser) (string, error) {
	profile, err := s.EnsureUserProfileFromAuth(ctx, user)
	if err != nil {
		return "", err
	}
	return profile.ID, nil
}

func (s *ProfileService) EnsureUserProfileFromAuth(ctx context.Context, user auth.AuthUser) (Profile, error) {
	user.ID = strings.TrimSpace(user.ID)
	if user.ID == "" || user.Service {
		return Profile{}, ErrUserProfileNotConfigured
	}
	parsedID, err := uuid.Parse(user.ID)
	if err != nil {
		return Profile{}, ErrUserProfileNotConfigured
	}
	user.ID = parsedID.String()

	if strings.TrimSpace(user.Email) == "" || strings.TrimSpace(user.Name) == "" {
		enriched, err := s.loadAuthUser(ctx, user.ID)
		if err == nil {
			if strings.TrimSpace(user.Email) == "" {
				user.Email = enriched.Email
			}
			if strings.TrimSpace(user.Name) == "" {
				user.Name = enriched.Name
			}
		} else if !isUndefinedTable(err) {
			return Profile{}, err
		}
	}

	email := strings.TrimSpace(user.Email)
	fullName := strings.TrimSpace(user.Name)
	if fullName == "" {
		fullName = email
	}
	if fullName == "" {
		fullName = user.ID
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback(ctx)

	var profile Profile
	if err := tx.QueryRow(ctx, `
		insert into user_profiles (id, email, full_name)
		values ($1::uuid, nullif($2, ''), nullif($3, ''))
		on conflict (id) do update
		set email = coalesce(nullif(excluded.email, ''), user_profiles.email),
		    full_name = coalesce(nullif(excluded.full_name, ''), user_profiles.full_name)
		returning id::text, coalesce(email, ''), coalesce(full_name, '')`,
		user.ID, email, fullName,
	).Scan(&profile.ID, &profile.Email, &profile.FullName); err != nil {
		if isUndefinedTable(err) || isUndefinedColumn(err) {
			return Profile{}, ErrUserProfileNotConfigured
		}
		return Profile{}, err
	}

	if err := ensureDefaultOperatorRole(ctx, tx, user.ID); err != nil {
		return Profile{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (s *ProfileService) LoadRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		select rl.name
		from user_roles ur
		join roles rl on rl.id = ur.role_id
		where ur.user_id = $1::uuid
		order by rl.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0, 4)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (s *ProfileService) HasActiveRecipient(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		select exists (
			select 1
			from affiliate_recipients
			where user_id = $1::uuid
			  and is_active = true
		)`,
		userID,
	).Scan(&exists)
	return exists, err
}

func (s *ProfileService) loadAuthUser(ctx context.Context, userID string) (auth.AuthUser, error) {
	var user auth.AuthUser
	var fullName string
	var name string
	err := s.pool.QueryRow(ctx, `
		select
			id::text,
			coalesce(email, ''),
			coalesce(raw_user_meta_data->>'full_name', ''),
			coalesce(raw_user_meta_data->>'name', '')
		from auth.users
		where id = $1::uuid`,
		userID,
	).Scan(&user.ID, &user.Email, &fullName, &name)
	user.Name = firstNonEmptyClaim(fullName, name, user.Email)
	return user, err
}

type roleTx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func ensureDefaultOperatorRole(ctx context.Context, tx roleTx, userID string) error {
	var roleID string
	if err := tx.QueryRow(ctx, `
		insert into roles(name)
		values ('operador')
		on conflict (name) do update set name = excluded.name
		returning id::text`).Scan(&roleID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		insert into user_roles (user_id, role_id)
		values ($1::uuid, $2::uuid)
		on conflict (user_id, role_id) do nothing`, userID, roleID)
	return err
}

func firstNonEmptyClaim(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
