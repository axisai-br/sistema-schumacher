package users

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"schumacher-tur/api/internal/auth"
	httpx "schumacher-tur/api/internal/shared/http"
)

type Handler struct {
	pool *pgxpool.Pool
}

type MeResponse struct {
	UserID         string   `json:"user_id"`
	Roles          []string `json:"roles"`
	CanAccessSaldo bool     `json:"can_access_saldo"`
	HasRecipient   bool     `json:"has_recipient"`
}

func NewHandler(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool} }

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Get("/me", h.me)
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authenticated user", nil)
		return
	}

	roles, err := h.loadRoles(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "USER_ROLES_ERROR", "could not load user roles", err.Error())
		return
	}
	hasRecipient, err := h.hasActiveRecipient(r.Context(), userID)
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

	httpx.WriteJSON(w, http.StatusOK, MeResponse{
		UserID:         userID,
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
