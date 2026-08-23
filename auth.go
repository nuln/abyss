package abyss

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthMethod represents a method of authentication.
type AuthMethod string

const (
	AuthPassword AuthMethod = "password"
	AuthPasskey  AuthMethod = "passkey"
	AuthOTP      AuthMethod = "otp"
)

// RefreshToken represents a long-lived token used to obtain new access tokens.
type RefreshToken struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	UserID    uint64    `json:"userId"`
	UserAgent string    `json:"userAgent"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// AuthResult is returned after a successful login.
type AuthResult struct {
	UserID       uint64     `json:"userId"`
	Method       AuthMethod `json:"method"`
	AccessToken  string     `json:"accessToken"`
	Token        string     `json:"token"` // For compatibility
	RefreshToken string     `json:"refreshToken"`
	ExpiresAt    time.Time  `json:"expiresAt"`
}

// AccessTokenClaims holds the JWT claims embedded in an access token.
type AccessTokenClaims struct {
	UserID    uint64   `json:"uid"`
	Role      UserRole `json:"role"`
	IsAdmin   bool     `json:"admin"`
	Issuer    string   `json:"iss,omitempty"`
	Subject   string   `json:"sub,omitempty"`
	ExpiresAt int64    `json:"exp,omitempty"`
	IssuedAt  int64    `json:"iat,omitempty"`
}

// SessionStore defines refresh-token / session persistence.
type SessionStore interface {
	Save(ctx context.Context, token *RefreshToken) error
	GetByID(ctx context.Context, id string) (*RefreshToken, error)
	GetByHash(ctx context.Context, hash string) (*RefreshToken, error)
	GetByUser(ctx context.Context, userID uint64) ([]*RefreshToken, error)
	Revoke(ctx context.Context, id string) error
	RevokeAll(ctx context.Context, userID uint64) error
}

// ── authService ──────────────────────────────────────────────────────────────

type authService struct {
	users           UserStore
	sessions        SessionStore
	jwtSecret       []byte
	accessTTL       time.Duration
	refreshTTL      time.Duration
	allowQueryToken bool
}

func newAuthService(users UserStore, sessions SessionStore, jwtSecret []byte, accessTTL, refreshTTL time.Duration, allowQueryToken bool) *authService {
	return &authService{
		users:           users,
		sessions:        sessions,
		jwtSecret:       jwtSecret,
		accessTTL:       accessTTL,
		refreshTTL:      refreshTTL,
		allowQueryToken: allowQueryToken,
	}
}

func (s *authService) Login(ctx context.Context, email, password, userAgent, ip string) (*AuthResult, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Try by username
			u, err = s.users.GetByUsername(ctx, email)
		}
		if err != nil {
			return nil, ErrUnauthorized
		}
	}
	if !comparePassword(u.PasswordHash, password) {
		return nil, ErrUnauthorized
	}

	return s.issueAuthResult(ctx, u, userAgent)
}

func (s *authService) issueAuthResult(ctx context.Context, u *User, userAgent string) (*AuthResult, error) {
	now := time.Now().UTC()

	accessToken, err := s.signJWT(u)
	if err != nil {
		return nil, WrapError(ErrInternal, err, "sign jwt")
	}

	refreshRaw, err := randomToken(32)
	if err != nil {
		return nil, WrapError(ErrInternal, err, "random token")
	}
	rt := &RefreshToken{
		ID:        refreshRaw,
		UserID:    u.ID,
		UserAgent: userAgent,
		CreatedAt: now,
		ExpiresAt: now.Add(s.refreshTTL),
	}
	if err := s.sessions.Save(ctx, rt); err != nil {
		return nil, fmt.Errorf("save refresh token: %w", err)
	}

	return &AuthResult{
		UserID:       u.ID,
		AccessToken:  accessToken,
		Token:        accessToken,
		RefreshToken: refreshRaw,
		ExpiresAt:    now.Add(s.accessTTL),
	}, nil
}

func (s *authService) Refresh(ctx context.Context, rawRefreshToken string) (*AuthResult, error) {
	hash := sha256Hex(rawRefreshToken)
	rt, err := s.sessions.GetByHash(ctx, hash)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, ErrUnauthorized
	}
	u, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	accessToken, err := s.signJWT(u)
	if err != nil {
		return nil, WrapError(ErrInternal, err, "sign jwt")
	}
	return &AuthResult{
		UserID:       u.ID,
		Method:       AuthPassword,
		AccessToken:  accessToken,
		Token:        accessToken,
		RefreshToken: rawRefreshToken,
		ExpiresAt:    time.Now().UTC().Add(s.accessTTL),
	}, nil
}

func (s *authService) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := sha256Hex(rawRefreshToken)
	rt, err := s.sessions.GetByHash(ctx, hash)
	if err != nil {
		return nil // already gone
	}
	return s.sessions.Revoke(ctx, rt.ID)
}

func (s *authService) Me(ctx context.Context, id uint64) (*User, error) {
	return s.users.GetByID(ctx, id)
}

func (s *authService) ListSessions(ctx context.Context, userID uint64) ([]*RefreshToken, error) {
	return s.sessions.GetByUser(ctx, userID)
}

func (s *authService) RevokeSession(ctx context.Context, id string) error {
	return s.sessions.Revoke(ctx, id)
}

func (s *authService) RevokeAllSessions(ctx context.Context, userID uint64) error {
	return s.sessions.RevokeAll(ctx, userID)
}

func (s *authService) VerifyJWT(tokenStr string) (*AccessTokenClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrUnauthorized
	}
	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrUnauthorized
	}
	uid, _ := mapClaims["uid"].(float64)
	role, _ := mapClaims["role"].(string)
	isAdmin, _ := mapClaims["admin"].(bool)
	return &AccessTokenClaims{
		UserID:  uint64(uid),
		Role:    UserRole(role),
		IsAdmin: isAdmin,
	}, nil
}

func (s *authService) signJWT(u *User) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"uid":   u.ID,
		"role":  string(u.Role),
		"admin": u.Role == RoleAdmin,
		"iss":   "abyss",
		"sub":   fmt.Sprintf("%d", u.ID),
		"iat":   now.Unix(),
		"exp":   now.Add(s.accessTTL).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(s.jwtSecret)
}

// ── authMiddleware ────────────────────────────────────────────────────────────

func authMiddleware(svc *authService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := r.Header.Get("X-Auth")
			if tokenStr == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			if tokenStr == "" {
				if svc.allowQueryToken {
					tokenStr = r.URL.Query().Get("auth")
				}
			}
			if tokenStr == "" {
				cookie, err := r.Cookie("auth")
				if err == nil {
					tokenStr = cookie.Value
				}
			}

			if tokenStr == "" {
				WriteErr(w, ErrUnauthorized)
				return
			}
			claims, err := svc.VerifyJWT(tokenStr)
			if err != nil {
				WriteErr(w, ErrUnauthorized)
				return
			}
			ctx := withAuthUserID(r.Context(), claims.UserID)
			ctx = withAuthIsAdmin(ctx, claims.IsAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
