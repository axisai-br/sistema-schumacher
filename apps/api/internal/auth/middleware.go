package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc"
	"github.com/golang-jwt/jwt/v4"
)

type contextKey string

const userIDKey contextKey = "user_id"
const authUserKey contextKey = "auth_user"
const authBypassKey contextKey = "auth_bypass"

// Config controls API authentication against Supabase Auth.
type Config struct {
	JWKSURL            string
	JWTSecret          string
	Issuer             string
	Audience           string
	AllowMissingIssuer bool
	ServiceTokens      []string
	Skip               bool
}

type AuthUser struct {
	ID      string
	Email   string
	Name    string
	Service bool
}

// Authenticator validates Supabase JWTs and service tokens.
type Authenticator struct {
	jwks               *keyfunc.JWKS
	jwtSecret          []byte
	issuer             string
	audience           string
	allowMissingIssuer bool
	services           map[string]string
	debugUID           string
	skip               bool
}

func NewAuthenticator(cfg Config) (*Authenticator, error) {
	if cfg.Skip {
		debugUID := strings.TrimSpace(os.Getenv("AUTH_DEBUG_USER_ID"))
		if debugUID == "" {
			debugUID = "00000000-0000-0000-0000-000000000001"
		}
		return &Authenticator{skip: true, debugUID: debugUID}, nil
	}

	services := make(map[string]string, len(cfg.ServiceTokens))
	for _, token := range cfg.ServiceTokens {
		trimmed := strings.TrimSpace(token)
		if trimmed == "" {
			continue
		}
		services[trimmed] = serviceSubject(trimmed)
	}

	auth := &Authenticator{
		jwtSecret:          []byte(strings.TrimSpace(cfg.JWTSecret)),
		issuer:             strings.TrimSpace(cfg.Issuer),
		audience:           strings.TrimSpace(cfg.Audience),
		allowMissingIssuer: cfg.AllowMissingIssuer,
		services:           services,
	}
	if len(auth.jwtSecret) > 0 {
		return auth, nil
	}

	options := keyfunc.Options{
		RefreshInterval:     time.Hour,
		RefreshErrorHandler: func(err error) {},
		RefreshTimeout:      10 * time.Second,
	}
	jwks, err := keyfunc.Get(cfg.JWKSURL, options)
	if err != nil {
		return nil, err
	}
	auth.jwks = jwks
	return auth, nil
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.skip {
			uid := strings.TrimSpace(r.Header.Get("X-Debug-User-Id"))
			if uid == "" {
				uid = a.debugUID
			}
			ctx := WithUser(r.Context(), AuthUser{ID: uid, Service: false})
			ctx = context.WithValue(ctx, authBypassKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		authHeader := r.Header.Get("Authorization")
		logFields := authLogFields{
			hasAuthorizationHeader: authHeader != "",
			expectedIssuer:         a.issuer,
		}
		if authHeader == "" {
			logAuthFailure(logFields, "missing authorization")
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) > 0 {
			logFields.authScheme = parts[0]
		}
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			logAuthFailure(logFields, "invalid authorization")
			http.Error(w, "invalid authorization", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		if subject, ok := a.services[tokenString]; ok {
			ctx := WithUser(r.Context(), AuthUser{ID: subject, Service: true})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		logFields = inspectJWT(tokenString, logFields)

		token, err := a.parseToken(tokenString)
		if err != nil || !token.Valid {
			logAuthFailure(logFields, summarizeJWTError(err, token))
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			logAuthFailure(logFields, "invalid claims type")
			http.Error(w, "invalid claims", http.StatusUnauthorized)
			return
		}

		iss, _ := claims["iss"].(string)
		if a.issuer != "" && iss != a.issuer {
			if iss == "" && a.allowMissingIssuer && a.compatibleMissingIssuerClaims(claims) {
				log.Printf("auth compatibility warning has_authorization_header=%t auth_scheme=%q jwt_alg=%q jwt_iss=%q expected_iss=%q role=%q sub_present=%t error=%q",
					logFields.hasAuthorizationHeader,
					logFields.authScheme,
					logFields.jwtAlg,
					logFields.jwtIssuer,
					logFields.expectedIssuer,
					logFields.role,
					logFields.subPresent,
					"missing issuer accepted temporarily",
				)
			} else {
				logAuthFailure(logFields, issuerError(iss))
				http.Error(w, "invalid issuer", http.StatusUnauthorized)
				return
			}
		}

		if a.audience != "" {
			aud := claims["aud"]
			if !audMatches(a.audience, aud) {
				logAuthFailure(logFields, "invalid audience")
				http.Error(w, "invalid audience", http.StatusUnauthorized)
				return
			}
		}

		sub, _ := claims["sub"].(string)
		if sub == "" {
			logAuthFailure(logFields, "invalid subject")
			http.Error(w, "invalid subject", http.StatusUnauthorized)
			return
		}
		ctx := WithUser(r.Context(), authUserFromClaims(sub, claims))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Authenticator) parseToken(tokenString string) (*jwt.Token, error) {
	if len(a.jwtSecret) > 0 {
		return jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Header["alg"])
			}
			return a.jwtSecret, nil
		})
	}
	return jwt.Parse(tokenString, a.jwks.Keyfunc)
}

func (a *Authenticator) compatibleMissingIssuerClaims(claims jwt.MapClaims) bool {
	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	return sub != "" && role == "authenticated" && audMatches("authenticated", claims["aud"])
}

func WithUser(ctx context.Context, user AuthUser) context.Context {
	ctx = context.WithValue(ctx, userIDKey, user.ID)
	return context.WithValue(ctx, authUserKey, user)
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(userIDKey)
	if v == nil {
		return "", false
	}
	id, ok := v.(string)
	return id, ok
}

func UserFromContext(ctx context.Context) (AuthUser, bool) {
	v := ctx.Value(authUserKey)
	if v == nil {
		if id, ok := UserIDFromContext(ctx); ok {
			return AuthUser{ID: id}, true
		}
		return AuthUser{}, false
	}
	user, ok := v.(AuthUser)
	return user, ok
}

func IsAuthBypass(ctx context.Context) bool {
	v := ctx.Value(authBypassKey)
	b, ok := v.(bool)
	return ok && b
}

func audMatches(expected string, aud interface{}) bool {
	switch v := aud.(type) {
	case string:
		return v == expected
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	}
	return false
}

func serviceSubject(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "service:" + hex.EncodeToString(sum[:6])
}

type authLogFields struct {
	hasAuthorizationHeader bool
	authScheme             string
	jwtAlg                 string
	jwtIssuer              string
	expectedIssuer         string
	role                   string
	subPresent             bool
}

func inspectJWT(tokenString string, fields authLogFields) authLogFields {
	claims := jwt.MapClaims{}
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, claims)
	if err != nil {
		return fields
	}
	if token != nil && token.Method != nil {
		fields.jwtAlg = token.Method.Alg()
	}
	fields.jwtIssuer, _ = claims["iss"].(string)
	fields.role, _ = claims["role"].(string)
	sub, _ := claims["sub"].(string)
	fields.subPresent = sub != ""
	return fields
}

func logAuthFailure(fields authLogFields, reason string) {
	log.Printf("auth rejected has_authorization_header=%t auth_scheme=%q jwt_alg=%q jwt_iss=%q expected_iss=%q role=%q sub_present=%t error=%q",
		fields.hasAuthorizationHeader,
		fields.authScheme,
		fields.jwtAlg,
		fields.jwtIssuer,
		fields.expectedIssuer,
		fields.role,
		fields.subPresent,
		reason,
	)
}

func summarizeJWTError(err error, token *jwt.Token) string {
	if err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "expired"):
			return "token expired"
		case strings.Contains(msg, "signature"):
			return "invalid signature"
		case strings.Contains(msg, "signing method") || strings.Contains(msg, "alg"):
			return "invalid signing method"
		default:
			return "jwt parse failed"
		}
	}
	if token == nil || !token.Valid {
		return "invalid token"
	}
	return "invalid token"
}

func issuerError(iss string) string {
	if iss == "" {
		return "missing issuer"
	}
	return "invalid issuer"
}

func authUserFromClaims(sub string, claims jwt.MapClaims) AuthUser {
	user := AuthUser{
		ID:    strings.TrimSpace(sub),
		Email: strings.TrimSpace(asClaimString(claims["email"])),
	}
	if metadata, ok := claims["user_metadata"].(map[string]interface{}); ok {
		user.Name = firstNonEmptyClaim(
			asClaimString(metadata["full_name"]),
			asClaimString(metadata["name"]),
			asClaimString(metadata["display_name"]),
		)
	}
	if user.Name == "" {
		user.Name = firstNonEmptyClaim(
			asClaimString(claims["name"]),
			asClaimString(claims["full_name"]),
			user.Email,
		)
	}
	return user
}

func asClaimString(value interface{}) string {
	if value == nil {
		return ""
	}
	s, _ := value.(string)
	return strings.TrimSpace(s)
}

func firstNonEmptyClaim(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
