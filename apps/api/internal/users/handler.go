package users

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"schumacher-tur/api/internal/auth"
	"schumacher-tur/api/internal/shared/config"
	httpx "schumacher-tur/api/internal/shared/http"
)

type Handler struct {
	pool     *pgxpool.Pool
	cfg      config.Config
	http     *http.Client
	profiles userProfileService
}

type userProfileService interface {
	EnsureUserProfileFromAuth(ctx context.Context, user auth.AuthUser) (Profile, error)
	LoadRoles(ctx context.Context, userID string) ([]string, error)
	HasActiveRecipient(ctx context.Context, userID string) (bool, error)
}

type MeResponse struct {
	UserID         string   `json:"user_id"`
	Roles          []string `json:"roles"`
	CanAccessSaldo bool     `json:"can_access_saldo"`
	HasRecipient   bool     `json:"has_recipient"`
}

type UserControlItem struct {
	UserID         string   `json:"user_id"`
	Email          string   `json:"email"`
	FullName       string   `json:"full_name"`
	Roles          []string `json:"roles"`
	CanAccessSaldo bool     `json:"can_access_saldo"`
	RecipientID    *string  `json:"recipient_id"`
	HasRecipient   bool     `json:"has_recipient"`
	IsActive       bool     `json:"is_active"`
}

type UserControlUpdateInput struct {
	CanAccessSaldo  *bool   `json:"can_access_saldo"`
	RecipientID     *string `json:"recipient_id"`
	RecipientActive *bool   `json:"recipient_active"`
}

type UserCreateInput struct {
	Email           string  `json:"email"`
	FullName        string  `json:"full_name"`
	Password        string  `json:"password"`
	CanAccessSaldo  bool    `json:"can_access_saldo"`
	RecipientID     *string `json:"recipient_id"`
	RecipientActive *bool   `json:"recipient_active"`
}

type UserUpdateInput struct {
	Email           *string `json:"email"`
	FullName        *string `json:"full_name"`
	CanAccessSaldo  *bool   `json:"can_access_saldo"`
	RecipientID     *string `json:"recipient_id"`
	RecipientActive *bool   `json:"recipient_active"`
}

type UserResetPasswordInput struct {
	Password string `json:"password"`
}

type supabaseAdminUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func NewHandler(pool *pgxpool.Pool, cfg config.Config, profiles ...userProfileService) *Handler {
	profileSvc := userProfileService(NewProfileService(pool))
	if len(profiles) > 0 && profiles[0] != nil {
		profileSvc = profiles[0]
	}
	return &Handler{
		pool:     pool,
		cfg:      cfg,
		http:     &http.Client{Timeout: 15 * time.Second},
		profiles: profileSvc,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Get("/me", h.me)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Patch("/{userId}", h.update)
		r.Patch("/{userId}/access", h.updateAccess)
		r.Post("/{userId}/reset-password", h.resetPassword)
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authenticated user", nil)
		return
	}
	bypass := auth.IsAuthBypass(r.Context())
	if bypass {
		httpx.WriteJSON(w, http.StatusOK, MeResponse{
			UserID:         userID,
			Roles:          []string{"financeiro"},
			CanAccessSaldo: true,
			HasRecipient:   true,
		})
		return
	}

	authUser, ok := auth.UserFromContext(r.Context())
	if !ok || strings.TrimSpace(authUser.ID) == "" {
		authUser = auth.AuthUser{ID: userID}
	}
	profile, err := h.profiles.EnsureUserProfileFromAuth(r.Context(), authUser)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserProfileNotConfigured):
			httpx.WriteError(w, http.StatusForbidden, "USER_PROFILE_NOT_CONFIGURED", "could not configure authenticated user profile", nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "USER_PROFILE_ERROR", "could not ensure user profile", err.Error())
		}
		return
	}

	roles, err := h.profiles.LoadRoles(r.Context(), profile.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "USER_ROLES_ERROR", "could not load user roles", err.Error())
		return
	}
	hasRecipient, err := h.profiles.HasActiveRecipient(r.Context(), profile.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "USER_RECIPIENT_ERROR", "could not load recipient linkage", err.Error())
		return
	}

	hasFinanceiro := false
	for _, role := range roles {
		if role == "financeiro" {
			hasFinanceiro = true
			break
		}
	}
	if bypass {
		// Local debug mode: keep frontend routes available even without Supabase auth.
		hasFinanceiro = true
		hasRecipient = true
	}

	httpx.WriteJSON(w, http.StatusOK, MeResponse{
		UserID:         profile.ID,
		Roles:          roles,
		CanAccessSaldo: hasFinanceiro,
		HasRecipient:   hasRecipient,
	})
}

func (h *Handler) loadRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := h.pool.Query(ctx, `
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

func (h *Handler) hasActiveRecipient(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := h.pool.QueryRow(ctx, `
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

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListFilter(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_QUERY", "invalid query parameters", err.Error())
		return
	}

	items, err := h.listUsers(r.Context(), filter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "USERS_LIST_ERROR", "could not list users", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input UserCreateInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}

	if err := validateCreateInput(input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.ensureAdminReady(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SUPABASE_ADMIN_NOT_CONFIGURED", err.Error(), nil)
		return
	}

	adminUser, err := h.supabaseAdminCreateUser(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "SUPABASE_ADMIN_CREATE_ERROR", "could not create auth user", err.Error())
		return
	}

	out, err := h.applyUserAccessUpdate(r.Context(), adminUser.ID, UserControlUpdateInput{
		CanAccessSaldo:  &input.CanAccessSaldo,
		RecipientID:     input.RecipientID,
		RecipientActive: input.RecipientActive,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "USER_ACCESS_UPDATE_ERROR", "could not apply user access", err.Error())
		return
	}
	if strings.TrimSpace(out.Email) == "" {
		out.Email = adminUser.Email
	}
	if strings.TrimSpace(out.FullName) == "" {
		out.FullName = input.FullName
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(chi.URLParam(r, "userId"))
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_USER_ID", "invalid user id", nil)
		return
	}

	var input UserUpdateInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	if err := validateUpdateInput(input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	if input.Email != nil || input.FullName != nil {
		if err := h.ensureAdminReady(); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "SUPABASE_ADMIN_NOT_CONFIGURED", err.Error(), nil)
			return
		}
		if err := h.supabaseAdminUpdateUser(r.Context(), userID, input); err != nil {
			httpx.WriteError(w, http.StatusBadGateway, "SUPABASE_ADMIN_UPDATE_ERROR", "could not update auth user", err.Error())
			return
		}
	}

	if input.FullName != nil {
		if err := h.updateUserProfileName(r.Context(), userID, strings.TrimSpace(*input.FullName)); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "USER_PROFILE_UPDATE_ERROR", "could not update profile", err.Error())
			return
		}
	}

	updateInput := UserControlUpdateInput{
		CanAccessSaldo:  input.CanAccessSaldo,
		RecipientID:     input.RecipientID,
		RecipientActive: input.RecipientActive,
	}

	out, err := h.applyUserAccessUpdate(r.Context(), userID, updateInput)
	if err != nil {
		switch {
		case errors.Is(err, errUserNotFound):
			httpx.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "user not found", nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "USER_UPDATE_ERROR", "could not update user", err.Error())
		}
		return
	}

	if input.Email != nil {
		out.Email = strings.TrimSpace(*input.Email)
	}
	if input.FullName != nil {
		out.FullName = strings.TrimSpace(*input.FullName)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(chi.URLParam(r, "userId"))
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_USER_ID", "invalid user id", nil)
		return
	}

	var input UserResetPasswordInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	input.Password = strings.TrimSpace(input.Password)
	if len(input.Password) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "password must contain at least 8 characters", nil)
		return
	}
	if err := h.ensureAdminReady(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SUPABASE_ADMIN_NOT_CONFIGURED", err.Error(), nil)
		return
	}
	if err := h.supabaseAdminResetPassword(r.Context(), userID, input.Password); err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "SUPABASE_ADMIN_RESET_ERROR", "could not reset user password", err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":  userID,
		"message":  "password reset successfully",
		"changed":  true,
		"datetime": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) updateAccess(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(chi.URLParam(r, "userId"))
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_USER_ID", "invalid user id", nil)
		return
	}

	var input UserControlUpdateInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid json", err.Error())
		return
	}
	if input.CanAccessSaldo == nil && input.RecipientID == nil && input.RecipientActive == nil {
		httpx.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "at least one field is required", nil)
		return
	}

	out, err := h.applyUserAccessUpdate(r.Context(), userID, input)
	if err != nil {
		switch {
		case errors.Is(err, errUserNotFound):
			httpx.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "user not found", nil)
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "USER_ACCESS_UPDATE_ERROR", "could not update user access", err.Error())
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type listFilter struct {
	Limit  int
	Offset int
	Search string
}

var errUserNotFound = errors.New("user not found")

func (h *Handler) hasUsersSchema(ctx context.Context) (bool, error) {
	var exists bool
	err := h.pool.QueryRow(ctx, `
		select to_regclass('public.user_profiles') is not null
	`).Scan(&exists)
	return exists, err
}

func parseListFilter(r *http.Request) (listFilter, error) {
	q := r.URL.Query()
	filter := listFilter{
		Limit:  50,
		Offset: 0,
		Search: strings.TrimSpace(q.Get("search")),
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return filter, errors.New("invalid limit")
		}
		if n > 200 {
			n = 200
		}
		filter.Limit = n
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return filter, errors.New("invalid offset")
		}
		filter.Offset = n
	}
	return filter, nil
}

func (h *Handler) listUsers(ctx context.Context, filter listFilter) ([]UserControlItem, error) {
	schemaReady, err := h.hasUsersSchema(ctx)
	if err != nil {
		return nil, err
	}
	if !schemaReady {
		return h.listUsersFromAuth(ctx, filter)
	}

	query := `
		select
			up.id::text as user_id,
			coalesce(up.email, '') as email,
			coalesce(up.full_name, '') as full_name,
			coalesce(array_agg(distinct rl.name) filter (where rl.name is not null), '{}') as roles,
			ar.recipient_id,
			coalesce(ar.is_active, false) as recipient_active
		from user_profiles up
		left join user_roles ur on ur.user_id = up.id
		left join roles rl on rl.id = ur.role_id
		left join affiliate_recipients ar on ar.user_id = up.id and ar.is_active = true
		where (
			$1 = '' or
			up.id::text ilike '%' || $1 || '%' or
			coalesce(up.email, '') ilike '%' || $1 || '%' or
			coalesce(up.full_name, '') ilike '%' || $1 || '%'
		)
		group by up.id, up.email, up.full_name, ar.recipient_id, ar.is_active
		order by up.created_at desc
		limit $2 offset $3`

	rows, err := h.pool.Query(ctx, query, filter.Search, filter.Limit, filter.Offset)
	if err != nil {
		if isUndefinedTable(err) || isUndefinedColumn(err) {
			return h.listUsersFromAuth(ctx, filter)
		}
		return nil, err
	}
	defer rows.Close()

	out := make([]UserControlItem, 0, filter.Limit)
	for rows.Next() {
		var item UserControlItem
		if err := rows.Scan(&item.UserID, &item.Email, &item.FullName, &item.Roles, &item.RecipientID, &item.HasRecipient); err != nil {
			return nil, err
		}
		item.CanAccessSaldo = hasRole(item.Roles, "financeiro")
		item.IsActive = true
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h *Handler) applyUserAccessUpdate(ctx context.Context, userID string, input UserControlUpdateInput) (UserControlItem, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return UserControlItem{}, err
	}
	defer tx.Rollback(ctx)

	exists, err := h.userExists(ctx, tx, userID)
	if err != nil {
		return UserControlItem{}, err
	}
	if !exists {
		return UserControlItem{}, errUserNotFound
	}

	if input.CanAccessSaldo != nil {
		var roleID string
		if err := tx.QueryRow(ctx, `
			insert into roles(name)
			values ('financeiro')
			on conflict (name) do update set name = excluded.name
			returning id::text`).Scan(&roleID); err != nil {
			return UserControlItem{}, err
		}

		if *input.CanAccessSaldo {
			if _, err := tx.Exec(ctx, `
				insert into user_roles (user_id, role_id)
				values ($1::uuid, $2::uuid)
				on conflict (user_id, role_id) do nothing`, userID, roleID); err != nil {
				return UserControlItem{}, err
			}
		} else {
			if _, err := tx.Exec(ctx, `
				delete from user_roles
				where user_id = $1::uuid
				and role_id = $2::uuid`, userID, roleID); err != nil {
				return UserControlItem{}, err
			}
		}
	}

	if input.RecipientID != nil || input.RecipientActive != nil {
		recipientID := ""
		if input.RecipientID != nil {
			recipientID = strings.TrimSpace(*input.RecipientID)
		}
		recipientActive := true
		if input.RecipientActive != nil {
			recipientActive = *input.RecipientActive
		}

		if input.RecipientID != nil {
			if recipientID == "" {
				if _, err := tx.Exec(ctx, `
					update affiliate_recipients
					set is_active = false, updated_at = now()
					where user_id = $1::uuid`, userID); err != nil && !isUndefinedTable(err) {
					return UserControlItem{}, err
				}
			} else {
				if _, err := tx.Exec(ctx, `
					insert into affiliate_recipients (user_id, recipient_id, is_active, created_at, updated_at)
					values ($1::uuid, $2, $3, now(), now())
					on conflict (user_id)
					do update set recipient_id = excluded.recipient_id, is_active = excluded.is_active, updated_at = now()`,
					userID, recipientID, recipientActive); err != nil && !isUndefinedTable(err) {
					return UserControlItem{}, err
				}
			}
		} else if input.RecipientActive != nil {
			if _, err := tx.Exec(ctx, `
				update affiliate_recipients
				set is_active = $2, updated_at = now()
				where user_id = $1::uuid`, userID, recipientActive); err != nil && !isUndefinedTable(err) {
				return UserControlItem{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return UserControlItem{}, err
	}

	return h.getUserControlByID(ctx, userID)
}

func (h *Handler) getUserControlByID(ctx context.Context, userID string) (UserControlItem, error) {
	items, err := h.listUsers(ctx, listFilter{Limit: 1, Offset: 0, Search: userID})
	if err != nil {
		return UserControlItem{}, err
	}
	for _, item := range items {
		if item.UserID == userID {
			return item, nil
		}
	}
	return UserControlItem{}, errUserNotFound
}

func validateCreateInput(input UserCreateInput) error {
	email := strings.TrimSpace(input.Email)
	if email == "" || !strings.Contains(email, "@") {
		return errors.New("valid email is required")
	}
	if len(strings.TrimSpace(input.Password)) < 8 {
		return errors.New("password must contain at least 8 characters")
	}
	if strings.TrimSpace(input.FullName) == "" {
		return errors.New("full_name is required")
	}
	return nil
}

func validateUpdateInput(input UserUpdateInput) error {
	if input.Email == nil && input.FullName == nil && input.CanAccessSaldo == nil && input.RecipientID == nil && input.RecipientActive == nil {
		return errors.New("at least one field is required")
	}
	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if email == "" || !strings.Contains(email, "@") {
			return errors.New("valid email is required")
		}
	}
	if input.FullName != nil && strings.TrimSpace(*input.FullName) == "" {
		return errors.New("full_name cannot be empty")
	}
	return nil
}

func (h *Handler) ensureAdminReady() error {
	if strings.TrimSpace(h.cfg.SupabaseURL) == "" {
		return errors.New("SUPABASE_URL is required for users admin operations")
	}
	if strings.TrimSpace(h.cfg.SupabaseServiceRoleKey) == "" {
		return errors.New("SUPABASE_SERVICE_ROLE_KEY is required for users admin operations")
	}
	return nil
}

func (h *Handler) supabaseAdminCreateUser(ctx context.Context, input UserCreateInput) (supabaseAdminUser, error) {
	payload := map[string]any{
		"email":         strings.TrimSpace(input.Email),
		"password":      strings.TrimSpace(input.Password),
		"email_confirm": true,
		"user_metadata": map[string]any{
			"full_name": strings.TrimSpace(input.FullName),
		},
	}

	data, err := h.doSupabaseAdminRequest(ctx, http.MethodPost, "/auth/v1/admin/users", payload)
	if err != nil {
		return supabaseAdminUser{}, err
	}

	var out supabaseAdminUser
	if err := json.Unmarshal(data, &out); err != nil {
		return supabaseAdminUser{}, err
	}
	if strings.TrimSpace(out.ID) == "" {
		return supabaseAdminUser{}, errors.New("supabase admin response missing user id")
	}
	return out, nil
}

func (h *Handler) supabaseAdminUpdateUser(ctx context.Context, userID string, input UserUpdateInput) error {
	payload := map[string]any{}
	if input.Email != nil {
		payload["email"] = strings.TrimSpace(*input.Email)
		payload["email_confirm"] = true
	}
	if input.FullName != nil {
		payload["user_metadata"] = map[string]any{
			"full_name": strings.TrimSpace(*input.FullName),
		}
	}
	if len(payload) == 0 {
		return nil
	}

	_, err := h.doSupabaseAdminRequest(ctx, http.MethodPut, "/auth/v1/admin/users/"+url.PathEscape(userID), payload)
	return err
}

func (h *Handler) supabaseAdminResetPassword(ctx context.Context, userID, password string) error {
	payload := map[string]any{
		"password": strings.TrimSpace(password),
	}
	_, err := h.doSupabaseAdminRequest(ctx, http.MethodPut, "/auth/v1/admin/users/"+url.PathEscape(userID), payload)
	return err
}

func (h *Handler) doSupabaseAdminRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}

	endpoint := strings.TrimRight(h.cfg.SupabaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", h.cfg.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+h.cfg.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := h.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return data, nil
	}

	var errBody map[string]any
	message := strings.TrimSpace(string(data))
	if json.Unmarshal(data, &errBody) == nil {
		for _, key := range []string{"message", "msg", "error_description", "error"} {
			if val, ok := errBody[key].(string); ok && strings.TrimSpace(val) != "" {
				message = val
				break
			}
		}
	}
	return nil, fmt.Errorf("supabase admin returned status %d: %s", res.StatusCode, message)
}

func (h *Handler) updateUserProfileName(ctx context.Context, userID, fullName string) error {
	schemaReady, err := h.hasUsersSchema(ctx)
	if err != nil || !schemaReady {
		return err
	}
	if _, err := h.pool.Exec(ctx, `
		update user_profiles
		set full_name = $2, updated_at = now()
		where id = $1::uuid`, userID, fullName); err != nil {
		if isUndefinedColumn(err) {
			_, err = h.pool.Exec(ctx, `
				update user_profiles
				set full_name = $2
				where id = $1::uuid`, userID, fullName)
		}
		if isUndefinedTable(err) || isUndefinedColumn(err) {
			return nil
		}
		return err
	}
	return nil
}

func (h *Handler) listUsersFromAuth(ctx context.Context, filter listFilter) ([]UserControlItem, error) {
	query := `
		select
			au.id::text as user_id,
			coalesce(au.email, '') as email,
			coalesce(au.raw_user_meta_data->>'full_name', '') as full_name,
			coalesce(array_agg(distinct rl.name) filter (where rl.name is not null), '{}') as roles,
			ar.recipient_id,
			coalesce(ar.is_active, false) as recipient_active
		from auth.users au
		left join user_roles ur on ur.user_id = au.id
		left join roles rl on rl.id = ur.role_id
		left join affiliate_recipients ar on ar.user_id = au.id and ar.is_active = true
		where (
			$1 = '' or
			au.id::text ilike '%' || $1 || '%' or
			coalesce(au.email, '') ilike '%' || $1 || '%' or
			coalesce(au.raw_user_meta_data->>'full_name', '') ilike '%' || $1 || '%'
		)
		group by au.id, au.email, au.raw_user_meta_data, ar.recipient_id, ar.is_active
		order by au.created_at desc
		limit $2 offset $3`

	rows, err := h.pool.Query(ctx, query, filter.Search, filter.Limit, filter.Offset)
	if err != nil {
		if isUndefinedTable(err) {
			return []UserControlItem{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := make([]UserControlItem, 0, filter.Limit)
	for rows.Next() {
		var item UserControlItem
		if err := rows.Scan(&item.UserID, &item.Email, &item.FullName, &item.Roles, &item.RecipientID, &item.HasRecipient); err != nil {
			return nil, err
		}
		item.CanAccessSaldo = hasRole(item.Roles, "financeiro")
		item.IsActive = true
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h *Handler) userExists(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	schemaReady, err := h.hasUsersSchema(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	if schemaReady {
		err = tx.QueryRow(ctx, `select exists(select 1 from user_profiles where id = $1::uuid)`, userID).Scan(&exists)
		if err != nil && !isUndefinedTable(err) {
			return false, err
		}
		if err == nil && exists {
			return true, nil
		}
	}
	err = tx.QueryRow(ctx, `select exists(select 1 from auth.users where id = $1::uuid)`, userID).Scan(&exists)
	return exists, err
}

func hasRole(roles []string, role string) bool {
	for _, item := range roles {
		if strings.EqualFold(strings.TrimSpace(item), role) {
			return true
		}
	}
	return false
}

func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	msg := strings.ToUpper(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "SQLSTATE 42P01")
}

func isUndefinedColumn(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42703"
	}
	msg := strings.ToUpper(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "SQLSTATE 42703")
}
