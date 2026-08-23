// Identity domain: user accounts, authentication (JWT sessions, MFA)
// and the crypto helpers they rely on.

package abyss

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
)

// UserRole represents a user's role in the system.
type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

// Permissions defines per-user capability flags.
type Permissions struct {
	Admin    bool `json:"admin"`
	Execute  bool `json:"execute"`
	Create   bool `json:"create"`
	Rename   bool `json:"rename"`
	Modify   bool `json:"modify"`
	Delete   bool `json:"delete"`
	Share    bool `json:"share"`
	Download bool `json:"download"`
	Copy     bool `json:"copy"`
	Move     bool `json:"move"`
	Shell    bool `json:"shell"`
	Upload   bool `json:"upload"`
}

// Sorting defines how lists are ordered in the UI.
type Sorting struct {
	By  string `json:"by"`
	Asc bool   `json:"asc"`
}

// Preferences holds strongly-typed UI and locale settings.
type Preferences struct {
	Locale      string  `json:"locale,omitempty"`
	Theme       string  `json:"theme,omitempty"`
	Scope       string  `json:"scope,omitempty"`
	SingleClick bool    `json:"singleClick,omitempty"`
	DarkMode    bool    `json:"darkMode,omitempty"`
	Sorting     Sorting `json:"sorting,omitempty"`
}

// User is the core user entity.
type User struct {
	ID           uint64      `json:"id"`
	UUID         string      `json:"uuid"`
	Email        string      `json:"email"`
	Username     string      `json:"username"`
	DisplayName  string      `json:"displayName"`
	PasswordHash string      `json:"passwordHash"`
	Role         UserRole    `json:"role"`
	Permissions  Permissions `json:"permissions"`
	Preferences  Preferences `json:"preferences"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Sanitize clears sensitive fields for API responses.
func (u *User) Sanitize() {
	if u != nil {
		u.PasswordHash = ""
	}
}

// ToFrontend returns a map that matches the frontend's IUser interface.
func (u *User) ToFrontend() map[string]any {
	if u == nil {
		return nil
	}
	locale := u.Preferences.Locale
	if locale == "" {
		locale = "auto"
	}
	theme := u.Preferences.Theme
	if theme == "" {
		theme = "auto"
	}
	res := map[string]any{
		"id":          u.ID,
		"uuid":        u.UUID,
		"email":       u.Email,
		"username":    u.Username,
		"displayName": u.DisplayName,
		"scope":       u.Preferences.Scope,
		"locale":      locale,
		"singleClick": u.Preferences.SingleClick,
		"theme":       theme,
		"perm": map[string]any{
			"admin":    u.Permissions.Admin,
			"execute":  u.Permissions.Execute,
			"create":   u.Permissions.Create,
			"rename":   u.Permissions.Rename,
			"modify":   u.Permissions.Modify,
			"delete":   u.Permissions.Delete,
			"share":    u.Permissions.Share,
			"download": u.Permissions.Download,
			"copy":     u.Permissions.Copy || u.Permissions.Create,
			"move":     u.Permissions.Move || u.Permissions.Rename,
			"shell":    u.Permissions.Shell || u.Permissions.Execute,
			"upload":   u.Permissions.Upload || u.Permissions.Create,
		},
		"sorting": map[string]any{
			"by":  u.Preferences.Sorting.By,
			"asc": u.Preferences.Sorting.Asc,
		},
	}
	if s, ok := res["sorting"].(map[string]any); ok {
		if s["by"] == "" {
			s["by"] = "name"
			s["asc"] = true
		}
	}
	if u.Preferences.DarkMode {
		res["theme"] = "dark"
	}
	if res["scope"] == "" {
		res["scope"] = "."
	}
	return res
}

// UserStore defines all user persistence operations.
type UserStore interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uint64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context) ([]*User, error)
}

// ── userService ──────────────────────────────────────────────────────────────

type userService struct {
	store UserStore
}

func (s *userService) Register(ctx context.Context, email, username, password, displayName string) (*User, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, WrapError(ErrInternal, err, "cannot hash password")
	}
	u := &User{
		UUID:         uuid.New().String(),
		Email:        email,
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: hash,
		Role:         RoleUser,
		Permissions: Permissions{
			Create: true, Rename: true, Modify: true, Delete: true,
			Share: true, Download: true,
		},
	}
	if err := s.store.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *userService) GetByID(ctx context.Context, id uint64) (*User, error) {
	return s.store.GetByID(ctx, id)
}

func (s *userService) GetByEmail(ctx context.Context, email string) (*User, error) {
	return s.store.GetByEmail(ctx, email)
}

func (s *userService) GetByUsername(ctx context.Context, username string) (*User, error) {
	return s.store.GetByUsername(ctx, username)
}

func (s *userService) UpdateUser(ctx context.Context, user *User) error {
	return s.store.Update(ctx, user)
}

func (s *userService) DeleteUser(ctx context.Context, id uint64, sessionStore SessionStore, cleanupFn func(context.Context, *User) error) error {
	u, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := sessionStore.RevokeAll(ctx, id); err != nil {
		return err
	}
	if cleanupFn != nil {
		if err := cleanupFn(ctx, u); err != nil {
			return err
		}
	}
	return s.store.Delete(ctx, id)
}

func (s *userService) List(ctx context.Context) ([]*User, error) {
	return s.store.List(ctx)
}

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

// ── Crypto ──────────────────────────────────────────────────────────────────

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func comparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func randomToken(n int) (string, error) { //nolint:unparam // Kept configurable for future callers/tests.
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func aesEncrypt(data, key []byte) (encrypted, nonce []byte, err error) {
	key, err = deriveEncryptionKey(key)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	encrypted = gcm.Seal(nil, nonce, data, nil)
	return encrypted, nonce, nil
}

func aesDecrypt(encrypted, nonce, key []byte) ([]byte, error) {
	// Try derived key first (current scheme).
	derived, err := deriveEncryptionKey(key)
	if err == nil {
		if plain, decErr := aesDecryptRaw(encrypted, nonce, derived); decErr == nil {
			return plain, nil
		}
	}
	// Fallback: try the raw key for data encrypted before the HKDF migration.
	return aesDecryptRaw(encrypted, nonce, key)
}

func aesDecryptRaw(encrypted, nonce, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, encrypted, nil)
}

func deriveEncryptionKey(key []byte) ([]byte, error) {
	reader := hkdf.New(sha256.New, key, nil, []byte("abyss/aes-gcm"))
	derived := make([]byte, 32)
	if _, err := io.ReadFull(reader, derived); err != nil {
		return nil, err
	}
	return derived, nil
}
