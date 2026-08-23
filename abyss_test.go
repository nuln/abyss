package abyss

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── config ──
func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig([]string{"-config", filepath.Join(dir, "nonexistent.toml")})
	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.Server.Addr)
	assert.Equal(t, "data", cfg.Data.Dir)
	assert.Equal(t, 24*time.Hour, cfg.Auth.AccessTokenTTL)
	assert.Equal(t, 30*24*time.Hour, cfg.Auth.RefreshTokenTTL)
	assert.True(t, cfg.Auth.AllowQueryToken)
}

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[server]
addr = ":9090"

[data]
dir = "mydata"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o600))
	cfg, err := LoadConfig([]string{"-config", cfgPath})
	require.NoError(t, err)
	assert.Equal(t, ":9090", cfg.Server.Addr)
	assert.Equal(t, "mydata", cfg.Data.Dir)
}

func TestLoadConfig_DisableQueryToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[auth]
allowQueryToken = false
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o600))
	cfg, err := LoadConfig([]string{"-config", cfgPath})
	require.NoError(t, err)
	assert.False(t, cfg.Auth.AllowQueryToken)
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("[[[[bad toml"), 0o600))
	_, err := LoadConfig([]string{"-config", cfgPath})
	assert.Error(t, err)
}

func TestConfig_DBPath(t *testing.T) {
	cfg := Config{Data: DataConfig{Dir: "/var/data"}}
	assert.Equal(t, "/var/data/abyss.db", cfg.DBPath())
}

func TestConfig_DBPath_ExplicitPath(t *testing.T) {
	cfg := Config{Data: DataConfig{Dir: "/var/data"}, Database: DatabaseConfig{Path: "/tmp/custom.db"}}
	assert.Equal(t, "/tmp/custom.db", cfg.DBPath())
}

func TestConfig_JWTSecretBytes_Valid(t *testing.T) {
	cfg := Config{Auth: AuthConfig{JWTSecret: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"}}
	b, err := cfg.JWTSecretBytes()
	require.NoError(t, err)
	assert.Len(t, b, 32)
}

func TestConfig_JWTSecretBytes_Invalid(t *testing.T) {
	cfg := Config{Auth: AuthConfig{JWTSecret: "nothex!"}}
	_, err := cfg.JWTSecretBytes()
	assert.Error(t, err)
}

func TestApplyDefaults_FilledConfig(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{Addr: ":1234"},
		Data:     DataConfig{Dir: "custom"},
		Auth:     AuthConfig{AccessTokenTTL: time.Hour, RefreshTokenTTL: 2 * time.Hour},
		Database: DatabaseConfig{Timeout: 5 * time.Second},
	}
	applyDefaults(&cfg)
	assert.Equal(t, ":1234", cfg.Server.Addr)
	assert.Equal(t, "custom", cfg.Data.Dir)
	assert.Equal(t, time.Hour, cfg.Auth.AccessTokenTTL)
	assert.Equal(t, 5*time.Second, cfg.Database.Timeout)
}

func TestEnsureJWTSecret(t *testing.T) {
	cfg := &Config{}
	generated, err := ensureJWTSecret(cfg)
	require.NoError(t, err)
	assert.True(t, generated)
	secret1 := cfg.Auth.JWTSecret
	assert.NotEmpty(t, secret1)

	// Second call should return false and keep the same secret
	generated, err = ensureJWTSecret(cfg)
	require.NoError(t, err)
	assert.False(t, generated)
	assert.Equal(t, secret1, cfg.Auth.JWTSecret)
}

func TestEnsureJWTSecret_AlreadySet(t *testing.T) {
	cfg := &Config{Auth: AuthConfig{JWTSecret: "deadbeef"}}
	generated, err := ensureJWTSecret(cfg)
	require.NoError(t, err)
	assert.False(t, generated)
	assert.Equal(t, "deadbeef", cfg.Auth.JWTSecret)
}

func TestValidateConfig_MissingAddr(t *testing.T) {
	cfg := Config{Data: DataConfig{Dir: "data"}}
	assert.Error(t, validateConfig(&cfg))
}

func TestValidateConfig_MissingDir(t *testing.T) {
	cfg := Config{Server: ServerConfig{Addr: ":8080"}}
	assert.Error(t, validateConfig(&cfg))
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Addr: ":8080"},
		Data:   DataConfig{Dir: "data"},
	}
	assert.NoError(t, validateConfig(&cfg))
}

func TestLoadConfig_LegacyFlags(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	cfg, err := LoadConfig([]string{
		"-config", filepath.Join(dir, "missing.toml"),
		"-address", "127.0.0.1",
		"-port", "19090",
		"-root", filepath.Join(dir, "data"),
		"-tokenExpirationTime", "2h",
		"-db-path", dbPath,
		"-db-timeout", "5s",
		"-db-nosync",
	})
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:19090", cfg.Server.Addr)
	assert.Equal(t, filepath.Join(dir, "data"), cfg.Data.Dir)
	assert.Equal(t, 2*time.Hour, cfg.Auth.AccessTokenTTL)
	assert.Equal(t, dbPath, cfg.Database.Path)
	assert.Equal(t, 5*time.Second, cfg.Database.Timeout)
	assert.True(t, cfg.Database.NoSync)
}

func TestLoadConfig_LegacyFlatFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "legacy.toml")
	content := `
address = "0.0.0.0"
port = "18080"
root = "legacydata"
tokenExpirationTime = "3h"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o600))

	cfg, err := LoadConfig([]string{"-config", cfgPath})
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:18080", cfg.Server.Addr)
	assert.Equal(t, "legacydata", cfg.Data.Dir)
	assert.Equal(t, 3*time.Hour, cfg.Auth.AccessTokenTTL)
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"/abyss", "/abyss"},
		{"abyss", "/abyss"},
		{"/abyss/", "/abyss"},
		{"  /abyss  ", "/abyss"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, normalizeBaseURL(tt.in), "input: %s", tt.in)
	}
}

// ── identity ──
// ── stubs ─────────────────────────────────────────────────────────────────────

type memUserStore struct {
	users      map[uint64]*User
	byEmail    map[string]uint64
	byUsername map[string]uint64
	nextID     uint64
}

func newMemUserStore() *memUserStore {
	return &memUserStore{
		users:      make(map[uint64]*User),
		byEmail:    make(map[string]uint64),
		byUsername: make(map[string]uint64),
	}
}

func (s *memUserStore) Create(_ context.Context, u *User) error {
	if _, ok := s.byEmail[u.Email]; ok {
		return WrapError(ErrConflict, nil, "email taken")
	}
	s.nextID++
	u.ID = s.nextID
	uCopy := *u
	s.users[u.ID] = &uCopy
	s.byEmail[u.Email] = u.ID
	s.byUsername[u.Username] = u.ID
	return nil
}

func (s *memUserStore) GetByID(_ context.Context, id uint64) (*User, error) {
	u, ok := s.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (s *memUserStore) GetByEmail(_ context.Context, email string) (*User, error) {
	id, ok := s.byEmail[email]
	if !ok {
		return nil, ErrNotFound
	}
	return s.users[id], nil
}

func (s *memUserStore) GetByUsername(_ context.Context, username string) (*User, error) {
	id, ok := s.byUsername[username]
	if !ok {
		return nil, ErrNotFound
	}
	return s.users[id], nil
}

func (s *memUserStore) Update(_ context.Context, u *User) error {
	if _, ok := s.users[u.ID]; !ok {
		return ErrNotFound
	}
	uCopy := *u
	s.users[u.ID] = &uCopy
	return nil
}

func (s *memUserStore) Delete(_ context.Context, id uint64) error {
	u, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.byEmail, u.Email)
	delete(s.byUsername, u.Username)
	delete(s.users, id)
	return nil
}

func (s *memUserStore) List(_ context.Context) ([]*User, error) {
	out := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out, nil
}

type memSessionStore struct {
	sessions map[string]*RefreshToken // keyed by ID
	byHash   map[string]string        // hash → ID
}

func newMemSessionStore() *memSessionStore {
	return &memSessionStore{
		sessions: make(map[string]*RefreshToken),
		byHash:   make(map[string]string),
	}
}

func (s *memSessionStore) Save(_ context.Context, t *RefreshToken) error {
	t.Hash = sha256Hex(t.ID)
	s.sessions[t.ID] = t
	s.byHash[t.Hash] = t.ID
	return nil
}

func (s *memSessionStore) GetByID(_ context.Context, id string) (*RefreshToken, error) {
	t, ok := s.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *memSessionStore) GetByHash(_ context.Context, hash string) (*RefreshToken, error) {
	id, ok := s.byHash[hash]
	if !ok {
		return nil, ErrNotFound
	}
	return s.sessions[id], nil
}

func (s *memSessionStore) GetByUser(_ context.Context, userID uint64) ([]*RefreshToken, error) {
	var out []*RefreshToken
	for _, t := range s.sessions {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memSessionStore) Revoke(_ context.Context, id string) error {
	t, ok := s.sessions[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.byHash, t.Hash)
	delete(s.sessions, id)
	return nil
}

func (s *memSessionStore) RevokeAll(_ context.Context, userID uint64) error {
	for id, t := range s.sessions {
		if t.UserID == userID {
			delete(s.byHash, t.Hash)
			delete(s.sessions, id)
		}
	}
	return nil
}

// ── userService tests ─────────────────────────────────────────────────────────

func TestUserService_Register(t *testing.T) {
	svc := &userService{store: newMemUserStore()}
	u, err := svc.Register(context.Background(), "a@example.com", "alice", "pass1234", "Alice")
	require.NoError(t, err)
	assert.NotZero(t, u.ID)
	assert.Equal(t, "alice", u.Username)
	assert.Equal(t, RoleUser, u.Role)
	assert.NotEmpty(t, u.PasswordHash)
}

func TestUserService_Register_DuplicateEmail(t *testing.T) {
	svc := &userService{store: newMemUserStore()}
	_, err := svc.Register(context.Background(), "dup@example.com", "u1", "pass", "A")
	require.NoError(t, err)
	_, err = svc.Register(context.Background(), "dup@example.com", "u2", "pass", "B")
	require.Error(t, err)
}

func TestUserService_GetByID(t *testing.T) {
	svc := &userService{store: newMemUserStore()}
	created, _ := svc.Register(context.Background(), "b@example.com", "bob", "pass", "Bob")
	got, err := svc.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}

func TestUserService_GetByUsername(t *testing.T) {
	svc := &userService{store: newMemUserStore()}
	created, _ := svc.Register(context.Background(), "u@example.com", "username123", "pass", "User")
	got, err := svc.GetByUsername(context.Background(), "username123")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}

func TestUserService_GetByID_NotFound(t *testing.T) {
	svc := &userService{store: newMemUserStore()}
	_, err := svc.GetByID(context.Background(), 999)
	assert.Error(t, err)
}

func TestUserService_UpdateUser(t *testing.T) {
	svc := &userService{store: newMemUserStore()}
	u, _ := svc.Register(context.Background(), "c@example.com", "carol", "pass", "Carol")
	u.DisplayName = "Carol Updated"
	require.NoError(t, svc.UpdateUser(context.Background(), u))
	got, _ := svc.GetByID(context.Background(), u.ID)
	assert.Equal(t, "Carol Updated", got.DisplayName)
}

func TestUserService_DeleteUser_RevokesSessionsAndDeletes(t *testing.T) {
	store := newMemUserStore()
	sessions := newMemSessionStore()
	svc := &userService{store: store}

	u, _ := svc.Register(context.Background(), "d@example.com", "dave", "pass", "Dave")
	_ = sessions.Save(context.Background(), &RefreshToken{ID: "tok1", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})

	require.NoError(t, svc.DeleteUser(context.Background(), u.ID, sessions, nil))
	assert.Empty(t, sessions.sessions)
	_, err := svc.GetByID(context.Background(), u.ID)
	assert.Error(t, err)
}

func TestUserService_DeleteUser_RunsCleanup(t *testing.T) {
	store := newMemUserStore()
	sessions := newMemSessionStore()
	svc := &userService{store: store}

	u, _ := svc.Register(context.Background(), "cleanup@example.com", "cleanup", "pass", "Cleanup")
	called := false

	err := svc.DeleteUser(context.Background(), u.ID, sessions, func(_ context.Context, user *User) error {
		called = true
		require.NotNil(t, user)
		assert.Equal(t, u.ID, user.ID)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestUserService_List(t *testing.T) {
	svc := &userService{store: newMemUserStore()}
	_, _ = svc.Register(context.Background(), "e@example.com", "e1", "pass", "E1")
	_, _ = svc.Register(context.Background(), "f@example.com", "e2", "pass", "E2")
	list, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

// ── authService tests ─────────────────────────────────────────────────────────

func makeAuthSvc(t *testing.T) (*authService, *memUserStore, *memSessionStore) {
	t.Helper()
	secret := make([]byte, 32)
	users := newMemUserStore()
	sessions := newMemSessionStore()
	svc := newAuthService(users, sessions, secret, time.Hour, 7*24*time.Hour, true)
	return svc, users, sessions
}

func registerTestUser(t *testing.T, users *memUserStore) *User {
	t.Helper()
	us := &userService{store: users}
	u, err := us.Register(context.Background(), "test@example.com", "testuser", "password", "Test")
	require.NoError(t, err)
	return u
}

func TestAuthService_Login_Success(t *testing.T) {
	svc, users, _ := makeAuthSvc(t)
	registerTestUser(t, users)

	result, err := svc.Login(context.Background(), "test@example.com", "password", "agent", "127.0.0.1")
	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, users, _ := makeAuthSvc(t)
	registerTestUser(t, users)

	_, err := svc.Login(context.Background(), "test@example.com", "wrong", "agent", "127.0.0.1")
	require.Error(t, err)
	var appErr *Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, ErrUnauthorized.Code, appErr.Code)
}

func TestAuthService_Login_Username(t *testing.T) {
	svc, users, _ := makeAuthSvc(t)
	registerTestUser(t, users)

	// Login with username
	result, err := svc.Login(context.Background(), "testuser", "password", "agent", "127.0.0.1")
	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
}

func TestAuthService_Login_UnknownIdentifier(t *testing.T) {
	svc, _, _ := makeAuthSvc(t)
	// Both email and username don't exist
	_, err := svc.Login(context.Background(), "nope@example.com", "pass", "agent", "127.0.0.1")
	assert.Error(t, err)
}

func TestAuthService_Refresh_Valid(t *testing.T) {
	svc, users, _ := makeAuthSvc(t)
	registerTestUser(t, users)
	res, _ := svc.Login(context.Background(), "test@example.com", "password", "ua", "")

	result, err := svc.Refresh(context.Background(), res.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
}

func TestAuthService_Refresh_Invalid(t *testing.T) {
	svc, _, _ := makeAuthSvc(t)
	_, err := svc.Refresh(context.Background(), "bogustoken")
	assert.Error(t, err)
}

func TestAuthService_ListSessions(t *testing.T) {
	svc, users, _ := makeAuthSvc(t)
	u := registerTestUser(t, users)

	_, err := svc.Login(context.Background(), "test@example.com", "password", "ua1", "127.0.0.1")
	require.NoError(t, err)
	_, err = svc.Login(context.Background(), "test@example.com", "password", "ua2", "127.0.0.2")
	require.NoError(t, err)

	sessions, err := svc.ListSessions(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestAuthService_Logout(t *testing.T) {
	svc, users, sessions := makeAuthSvc(t)
	registerTestUser(t, users)
	res, _ := svc.Login(context.Background(), "test@example.com", "password", "ua", "")

	require.NoError(t, svc.Logout(context.Background(), res.RefreshToken))
	// token should be gone
	hash := sha256Hex(res.RefreshToken)
	_, err := sessions.GetByHash(context.Background(), hash)
	assert.Error(t, err)
}

func TestAuthService_Logout_Unknown_IsNoop(t *testing.T) {
	svc, _, _ := makeAuthSvc(t)
	// logout of unknown token must not error
	assert.NoError(t, svc.Logout(context.Background(), "nonexistent"))
}

func TestAuthService_Me(t *testing.T) {
	svc, users, _ := makeAuthSvc(t)
	u := registerTestUser(t, users)

	got, err := svc.Me(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestAuthService_VerifyJWT_ValidToken(t *testing.T) {
	svc, users, _ := makeAuthSvc(t)
	u := registerTestUser(t, users)
	token, err := svc.signJWT(u)
	require.NoError(t, err)

	claims, err := svc.VerifyJWT(token)
	require.NoError(t, err)
	assert.Equal(t, u.ID, claims.UserID)
}

func TestAuthService_VerifyJWT_Invalid(t *testing.T) {
	svc, _, _ := makeAuthSvc(t)
	_, err := svc.VerifyJWT("not.a.jwt")
	assert.Error(t, err)
}

func TestAuthService_VerifyJWT_WrongSecret(t *testing.T) {
	svc1, users, _ := makeAuthSvc(t)
	svc2, _, _ := makeAuthSvc(t) // different secret (zero bytes, same but let's swap)
	svc2.jwtSecret = []byte("different_secret_different_length_xx")

	u := registerTestUser(t, users)
	token, _ := svc1.signJWT(u)

	_, err := svc2.VerifyJWT(token)
	assert.Error(t, err)
}

// ── Crypto ────────────────────────────────────────────────────────────────────

func TestHashPassword_Roundtrip(t *testing.T) {
	hash, err := hashPassword("hunter2")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.True(t, comparePassword(hash, "hunter2"))
	assert.False(t, comparePassword(hash, "wrong"))
}

func TestHashPassword_DifferentHash(t *testing.T) {
	h1, _ := hashPassword("secret")
	h2, _ := hashPassword("secret")
	// bcrypt always produces a different hash because of random salt
	assert.NotEqual(t, h1, h2)
}

func TestRandomToken_Length(t *testing.T) {
	tok, err := randomToken(32)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
	// base64url-encoded 32 bytes → 43 characters (without padding)
	assert.GreaterOrEqual(t, len(tok), 40)
}

func TestRandomToken_Unique(t *testing.T) {
	t1, _ := randomToken(32)
	t2, _ := randomToken(32)
	assert.NotEqual(t, t1, t2)
}

func TestSha256Hex(t *testing.T) {
	h := sha256Hex("hello")
	assert.Len(t, h, 64)
	assert.Equal(t, sha256Hex("hello"), h) // deterministic
	assert.NotEqual(t, sha256Hex("hello"), sha256Hex("world"))
}

// ── AES ──────────────────────────────────────────────────────────────────────

func TestAesEncryptDecrypt_Roundtrip(t *testing.T) {
	key := bytes.Repeat([]byte("k"), 32)
	plain := []byte("secret data")
	enc, nonce, err := aesEncrypt(plain, key)
	require.NoError(t, err)
	dec, err := aesDecrypt(enc, nonce, key)
	require.NoError(t, err)
	assert.Equal(t, plain, dec)
}

func TestAesDecrypt_BackwardCompatWithRawKey(t *testing.T) {
	// Simulate data encrypted with the raw key (pre-HKDF migration).
	key := bytes.Repeat([]byte("k"), 32)
	plain := []byte("legacy secret")

	// Encrypt using raw key directly (old behavior).
	enc, nonce, err := aesEncryptRaw(plain, key)
	require.NoError(t, err)

	// aesDecrypt should fall back to raw key and succeed.
	dec, err := aesDecrypt(enc, nonce, key)
	require.NoError(t, err)
	assert.Equal(t, plain, dec)
}

// aesEncryptRaw encrypts with a raw key (simulating pre-HKDF behavior).
func aesEncryptRaw(data, key []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	var gcm cipher.AEAD
	gcm, err = cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, data, nil), nonce, nil
}

// ── task ──
// ── memTaskStore ──────────────────────────────────────────────────────────────

type memTaskStore struct {
	mu    sync.Mutex
	tasks map[string]*TaskInfo
}

func newMemTaskStore() *memTaskStore {
	return &memTaskStore{tasks: make(map[string]*TaskInfo)}
}

func (s *memTaskStore) Save(_ context.Context, t *TaskInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *memTaskStore) GetByID(_ context.Context, id string) (*TaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *memTaskStore) ListByUser(_ context.Context, userID uint64) ([]*TaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*TaskInfo
	for _, t := range s.tasks {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memTaskStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
	return nil
}

type cancelAwareTaskStore struct {
	*memTaskStore
}

func (s *cancelAwareTaskStore) Save(ctx context.Context, t *TaskInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.memTaskStore.Save(ctx, t)
}

// ── scheduler tests ───────────────────────────────────────────────────────────

func TestScheduler_TrackAndCancel(t *testing.T) {
	sch := newScheduler()
	cancelled := false
	sch.Track("t1", func() { cancelled = true })
	ok := sch.Cancel("t1")
	assert.True(t, ok)
	assert.True(t, cancelled)
}

func TestScheduler_Cancel_NotFound(t *testing.T) {
	sch := newScheduler()
	ok := sch.Cancel("nonexistent")
	assert.False(t, ok)
}

func TestScheduler_Cancel_Idempotent(t *testing.T) {
	sch := newScheduler()
	sch.Track("t2", func() {})
	sch.Cancel("t2")
	// second cancel should return false
	ok := sch.Cancel("t2")
	assert.False(t, ok)
}

func TestScheduler_Untrack(t *testing.T) {
	sch := newScheduler()
	sch.Track("t3", func() {})
	assert.Equal(t, 1, sch.Size())
	sch.Untrack("t3")
	assert.Equal(t, 0, sch.Size())
}

// ── taskService tests ─────────────────────────────────────────────────────────

func TestTaskService_Submit_Success(t *testing.T) {
	store := newMemTaskStore()
	sch := newScheduler()
	svc := newTaskService(store, sch)

	done := make(chan struct{})
	id, err := svc.Submit(context.Background(), "test-task", 1, func(_ context.Context) error {
		close(done)
		return nil
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not complete in time")
	}
	require.Eventually(t, func() bool { return sch.Size() == 0 }, time.Second, 10*time.Millisecond)
}

func TestTaskService_Submit_Failure(t *testing.T) {
	store := newMemTaskStore()
	svc := newTaskService(store, newScheduler())

	done := make(chan struct{})
	id, err := svc.Submit(context.Background(), "fail-task", 1, func(_ context.Context) error {
		defer close(done)
		return errors.New("task error")
	})
	require.NoError(t, err)
	<-done
	// Wait for status update
	time.Sleep(50 * time.Millisecond)

	info, err := store.GetByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, TaskFailed, info.Status)
	assert.Equal(t, "task error", info.Message)
}

func TestTaskService_Cancel(t *testing.T) {
	store := newMemTaskStore()
	sch := newScheduler()
	svc := newTaskService(store, sch)

	started := make(chan struct{})
	id, _ := svc.Submit(context.Background(), "long-task", 1, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started
	require.NoError(t, svc.Cancel(id))
	require.Eventually(t, func() bool { return sch.Size() == 0 }, time.Second, 10*time.Millisecond)
}

func TestTaskService_Cancel_PersistsFinalStatus(t *testing.T) {
	store := &cancelAwareTaskStore{memTaskStore: newMemTaskStore()}
	sch := newScheduler()
	svc := newTaskService(store, sch)

	started := make(chan struct{})
	id, err := svc.Submit(context.Background(), "cancel-persist", 1, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	require.NoError(t, err)
	<-started
	require.NoError(t, svc.Cancel(id))

	require.Eventually(t, func() bool {
		info, getErr := store.GetByID(context.Background(), id)
		return getErr == nil && info.Status == TaskCanceled
	}, time.Second, 10*time.Millisecond)
}

func TestTaskService_Cancel_NotFound(t *testing.T) {
	svc := newTaskService(newMemTaskStore(), newScheduler())
	err := svc.Cancel("unknown")
	assert.Error(t, err)
}

func TestTaskService_ListByUser(t *testing.T) {
	store := newMemTaskStore()
	svc := newTaskService(store, newScheduler())

	_, _ = svc.Submit(context.Background(), "t1", 1, func(_ context.Context) error { return nil })
	_, _ = svc.Submit(context.Background(), "t2", 1, func(_ context.Context) error { return nil })
	_, _ = svc.Submit(context.Background(), "t3", 2, func(_ context.Context) error { return nil })

	list, err := svc.ListByUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestTaskService_RegisterRunner_And_SubmitRegistered(t *testing.T) {
	store := newMemTaskStore()
	svc := newTaskService(store, newScheduler())

	done := make(chan struct{})
	svc.RegisterRunner("my-runner", func(_ context.Context) error {
		close(done)
		return nil
	})

	id, err := svc.SubmitRegistered(context.Background(), "my-runner", 1)
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	<-done
}

func TestTaskService_SubmitRegistered_UnknownRunner(t *testing.T) {
	svc := newTaskService(newMemTaskStore(), newScheduler())
	_, err := svc.SubmitRegistered(context.Background(), "nope", 1)
	assert.Error(t, err)
}

// ── memSettingsStore ──────────────────────────────────────────────────────────

type memSettingsStore struct {
	s *Settings
}

func (m *memSettingsStore) Get(_ context.Context) (*Settings, error) {
	if m.s == nil {
		return nil, ErrNotFound
	}
	cp := *m.s
	return &cp, nil
}

func (m *memSettingsStore) Save(_ context.Context, s *Settings) error {
	cp := *s
	m.s = &cp
	return nil
}

type countingSettingsStore struct {
	inner *memSettingsStore
	gets  int
	saves int
}

func (c *countingSettingsStore) Get(ctx context.Context) (*Settings, error) {
	c.gets++
	return c.inner.Get(ctx)
}

func (c *countingSettingsStore) Save(ctx context.Context, s *Settings) error {
	c.saves++
	return c.inner.Save(ctx, s)
}

// ── settingsService tests ─────────────────────────────────────────────────────

func TestSettingsService_GetAndSave(t *testing.T) {
	svc := newSettingsService(&memSettingsStore{})

	got, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, got)

	in := &Settings{
		Branding: SettingsBranding{Name: "My Abyss", Theme: "dark"},
	}
	require.NoError(t, svc.Save(context.Background(), in))

	got2, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "My Abyss", got2.Branding.Name)
	assert.Equal(t, "dark", got2.Branding.Theme)
}

func TestSettingsService_Get_UsesCache(t *testing.T) {
	store := &countingSettingsStore{inner: &memSettingsStore{}}
	svc := newSettingsService(store)

	_, err := svc.Get(context.Background())
	require.NoError(t, err)
	_, err = svc.Get(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, store.gets)

	require.NoError(t, svc.Save(context.Background(), &Settings{Branding: SettingsBranding{Name: "Cached"}}))
	assert.Equal(t, 1, store.saves)
	_, err = svc.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, store.gets)
}

// ── boltdb ──
// ── helpers ───────────────────────────────────────────────────────────────────

func openTestDB(t *testing.T) *boltDB {
	t.Helper()
	dir := t.TempDir()
	db, err := openBoltDB(boltConfig{
		Path:    filepath.Join(dir, "test.db"),
		Timeout: 3 * time.Second,
		Mode:    0o600,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ── boltUserStore ─────────────────────────────────────────────────────────────

func TestBoltUserStore_CreateAndGetByID(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u := &User{Email: "test@example.com", Username: "testuser"}
	require.NoError(t, store.Create(context.Background(), u))
	assert.NotZero(t, u.ID)
	assert.NotZero(t, u.CreatedAt)

	got, err := store.GetByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, "test@example.com", got.Email)
}

func TestBoltUserStore_GetByEmail(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u := &User{Email: "Email@Example.COM", Username: "emailuser"}
	require.NoError(t, store.Create(context.Background(), u))

	got, err := store.GetByEmail(context.Background(), "email@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestBoltUserStore_GetByUsername(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u := &User{Email: "user@example.com", Username: "MyUser"}
	require.NoError(t, store.Create(context.Background(), u))

	got, err := store.GetByUsername(context.Background(), "myuser")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestBoltUserStore_DuplicateEmail(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u1 := &User{Email: "dup@example.com", Username: "u1"}
	u2 := &User{Email: "dup@example.com", Username: "u2"}
	require.NoError(t, store.Create(context.Background(), u1))
	err := store.Create(context.Background(), u2)
	assert.ErrorIs(t, err, ErrConflict)
}

func TestBoltUserStore_GetByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}
	_, err := store.GetByID(context.Background(), 9999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBoltUserStore_Update(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u := &User{Email: "update@example.com", Username: "updater"}
	require.NoError(t, store.Create(context.Background(), u))

	u.DisplayName = "Updated Name"
	require.NoError(t, store.Update(context.Background(), u))

	got, _ := store.GetByID(context.Background(), u.ID)
	assert.Equal(t, "Updated Name", got.DisplayName)
	assert.True(t, got.UpdatedAt.After(got.CreatedAt) || got.UpdatedAt.Equal(got.CreatedAt))
}

func TestBoltUserStore_Update_EmailConflict(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u1 := &User{Email: "a@example.com", Username: "a"}
	u2 := &User{Email: "b@example.com", Username: "b"}
	require.NoError(t, store.Create(context.Background(), u1))
	require.NoError(t, store.Create(context.Background(), u2))

	u2.Email = u1.Email
	err := store.Update(context.Background(), u2)
	assert.ErrorIs(t, err, ErrConflict)
}

func TestBoltUserStore_Update_UsernameConflict(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u1 := &User{Email: "a1@example.com", Username: "a1"}
	u2 := &User{Email: "b1@example.com", Username: "b1"}
	require.NoError(t, store.Create(context.Background(), u1))
	require.NoError(t, store.Create(context.Background(), u2))

	u2.Username = u1.Username
	err := store.Update(context.Background(), u2)
	assert.ErrorIs(t, err, ErrConflict)
}

func TestBoltUserStore_Delete(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	u := &User{Email: "del@example.com", Username: "deleter"}
	require.NoError(t, store.Create(context.Background(), u))
	require.NoError(t, store.Delete(context.Background(), u.ID))

	_, err := store.GetByID(context.Background(), u.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	// Email index should also be cleared
	_, err = store.GetByEmail(context.Background(), "del@example.com")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBoltUserStore_List(t *testing.T) {
	db := openTestDB(t)
	store := &boltUserStore{db: db}

	require.NoError(t, store.Create(context.Background(), &User{Email: "a@e.com", Username: "a"}))
	require.NoError(t, store.Create(context.Background(), &User{Email: "b@e.com", Username: "b"}))

	list, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

// ── boltSessionStore ──────────────────────────────────────────────────────────

func TestBoltSessionStore_SaveAndGetByHash(t *testing.T) {
	db := openTestDB(t)
	store := &boltSessionStore{db: db}

	token := &RefreshToken{
		ID:        "rawtoken",
		UserID:    1,
		UserAgent: "test-agent",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, store.Save(context.Background(), token))
	assert.NotEmpty(t, token.Hash)

	got, err := store.GetByHash(context.Background(), token.Hash)
	require.NoError(t, err)
	assert.Equal(t, token.ID, got.ID)
}

func TestBoltSessionStore_GetByID(t *testing.T) {
	db := openTestDB(t)
	store := &boltSessionStore{db: db}

	token := &RefreshToken{ID: "sess-1", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, store.Save(context.Background(), token))

	got, err := store.GetByID(context.Background(), token.ID)
	require.NoError(t, err)
	assert.Equal(t, token.ID, got.ID)
}

func TestBoltSessionStore_GetByHash_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := &boltSessionStore{db: db}
	_, err := store.GetByHash(context.Background(), "nohash")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBoltSessionStore_GetByUser(t *testing.T) {
	db := openTestDB(t)
	store := &boltSessionStore{db: db}

	_ = store.Save(context.Background(), &RefreshToken{ID: "t1", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)})
	_ = store.Save(context.Background(), &RefreshToken{ID: "t2", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)})
	_ = store.Save(context.Background(), &RefreshToken{ID: "t3", UserID: 2, ExpiresAt: time.Now().Add(time.Hour)})

	tokens, err := store.GetByUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, tokens, 2)
}

func TestBoltSessionStore_Revoke(t *testing.T) {
	db := openTestDB(t)
	store := &boltSessionStore{db: db}

	tok := &RefreshToken{ID: "revoke-me", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, store.Save(context.Background(), tok))
	require.NoError(t, store.Revoke(context.Background(), tok.ID))

	_, err := store.GetByHash(context.Background(), tok.Hash)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBoltSessionStore_RevokeAll(t *testing.T) {
	db := openTestDB(t)
	store := &boltSessionStore{db: db}

	_ = store.Save(context.Background(), &RefreshToken{ID: "ta", UserID: 5, ExpiresAt: time.Now().Add(time.Hour)})
	_ = store.Save(context.Background(), &RefreshToken{ID: "tb", UserID: 5, ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, store.RevokeAll(context.Background(), 5))

	tokens, _ := store.GetByUser(context.Background(), 5)
	assert.Empty(t, tokens)
}

// ── boltFileStore ─────────────────────────────────────────────────────────────

func TestBoltFileStore_SaveAndGetByID(t *testing.T) {
	db := openTestDB(t)
	store := &boltFileStore{db: db}

	f := &File{UserID: 1, Path: "/test.txt", Name: "test.txt", Type: EntryFile, Size: 100}
	require.NoError(t, store.Save(context.Background(), f))
	assert.NotZero(t, f.ID)

	got, err := store.GetByID(context.Background(), f.ID)
	require.NoError(t, err)
	assert.Equal(t, "/test.txt", got.Path)
	assert.Equal(t, uint64(1), got.UserID)
}

func TestBoltFileStore_GetByUserPath(t *testing.T) {
	db := openTestDB(t)
	store := &boltFileStore{db: db}

	f := &File{UserID: 2, Path: "/docs/readme.md", Name: "readme.md", Type: EntryFile}
	require.NoError(t, store.Save(context.Background(), f))

	got, err := store.GetByUserPath(context.Background(), 2, "/docs/readme.md")
	require.NoError(t, err)
	assert.Equal(t, f.ID, got.ID)
}

func TestBoltFileStore_GetByUserPath_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := &boltFileStore{db: db}
	_, err := store.GetByUserPath(context.Background(), 1, "/nope.txt")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBoltFileStore_ListByUser(t *testing.T) {
	db := openTestDB(t)
	store := &boltFileStore{db: db}

	_ = store.Save(context.Background(), &File{UserID: 10, Path: "/a.txt", Name: "a.txt", Type: EntryFile})
	_ = store.Save(context.Background(), &File{UserID: 10, Path: "/b.txt", Name: "b.txt", Type: EntryFile})
	_ = store.Save(context.Background(), &File{UserID: 20, Path: "/c.txt", Name: "c.txt", Type: EntryFile})

	files, err := store.ListByUser(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestBoltFileStore_ListByUserAndParent(t *testing.T) {
	db := openTestDB(t)
	store := &boltFileStore{db: db}

	_ = store.Save(context.Background(), &File{UserID: 1, Path: "/docs/a.txt", Name: "a.txt", Type: EntryFile})
	_ = store.Save(context.Background(), &File{UserID: 1, Path: "/docs/b.txt", Name: "b.txt", Type: EntryFile})
	_ = store.Save(context.Background(), &File{UserID: 1, Path: "/other.txt", Name: "other.txt", Type: EntryFile})

	files, err := store.ListByUserAndParent(context.Background(), 1, "/docs")
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestBoltFileStore_Delete(t *testing.T) {
	db := openTestDB(t)
	store := &boltFileStore{db: db}

	f := &File{UserID: 1, Path: "/del.txt", Name: "del.txt", Type: EntryFile}
	require.NoError(t, store.Save(context.Background(), f))
	require.NoError(t, store.Delete(context.Background(), f.ID))

	_, err := store.GetByID(context.Background(), f.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

// ── boltTaskStore ─────────────────────────────────────────────────────────────

func TestBoltTaskStore_SaveAndGetByID(t *testing.T) {
	db := openTestDB(t)
	store := &boltTaskStore{db: db}

	info := &TaskInfo{ID: "task-001", Name: "test", UserID: 1, Status: TaskPending}
	require.NoError(t, store.Save(context.Background(), info))

	got, err := store.GetByID(context.Background(), "task-001")
	require.NoError(t, err)
	assert.Equal(t, "task-001", got.ID)
	assert.Equal(t, uint64(1), got.UserID)
}

func TestBoltTaskStore_GetByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := &boltTaskStore{db: db}
	_, err := store.GetByID(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBoltTaskStore_ListByUser(t *testing.T) {
	db := openTestDB(t)
	store := &boltTaskStore{db: db}

	_ = store.Save(context.Background(), &TaskInfo{ID: "t1", Name: "n1", UserID: 7, Status: TaskPending})
	_ = store.Save(context.Background(), &TaskInfo{ID: "t2", Name: "n2", UserID: 7, Status: TaskRunning})
	_ = store.Save(context.Background(), &TaskInfo{ID: "t3", Name: "n3", UserID: 8, Status: TaskPending})

	tasks, err := store.ListByUser(context.Background(), 7)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestBoltTaskStore_Delete(t *testing.T) {
	db := openTestDB(t)
	store := &boltTaskStore{db: db}

	_ = store.Save(context.Background(), &TaskInfo{ID: "del-task", Name: "n", UserID: 1, Status: TaskPending})
	require.NoError(t, store.Delete(context.Background(), "del-task"))

	_, err := store.GetByID(context.Background(), "del-task")
	assert.ErrorIs(t, err, ErrNotFound)
}

// ── boltSettingsStore ─────────────────────────────────────────────────────────

func TestBoltSettingsStore_GetDefault(t *testing.T) {
	db := openTestDB(t)
	store := &boltSettingsStore{db: db}

	// Before any save, Get returns ErrNotFound
	_, err := store.Get(context.Background())
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBoltSettingsStore_Save(t *testing.T) {
	db := openTestDB(t)
	store := &boltSettingsStore{db: db}

	in := &Settings{Branding: SettingsBranding{Name: "Test Abyss"}}
	require.NoError(t, store.Save(context.Background(), in))

	got, err := store.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Test Abyss", got.Branding.Name)
}

func TestBoltSettingsStore_SaveOverwrite(t *testing.T) {
	db := openTestDB(t)
	store := &boltSettingsStore{db: db}

	_ = store.Save(context.Background(), &Settings{Branding: SettingsBranding{Name: "First"}})
	_ = store.Save(context.Background(), &Settings{Branding: SettingsBranding{Name: "Second"}})

	got, _ := store.Get(context.Background())
	assert.Equal(t, "Second", got.Branding.Name)
}

// ── boltPluginStore ───────────────────────────────────────────────────────────

func TestBoltPluginStore_PutAndGet(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	store := newBoltPluginStore(db, "test-plugin", dir)

	require.NoError(t, store.Put("key1", "value1"))

	var out string
	require.NoError(t, store.Get("key1", &out))
	assert.Equal(t, "value1", out)
}

func TestBoltPluginStore_Get_Missing(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	store := newBoltPluginStore(db, "test-plugin", dir)

	var out string
	err := store.Get("missing", &out)
	// Missing key should return ErrNotFound or similar
	assert.Error(t, err)
}

func TestBoltPluginStore_Delete(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	store := newBoltPluginStore(db, "test-plugin", dir)

	_ = store.Put("k", "v")
	require.NoError(t, store.Delete("k"))

	var out string
	err := store.Get("k", &out)
	assert.Error(t, err)
}

func TestBoltPluginStore_SaveAndGetConfig(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	store := newBoltPluginStore(db, "test-plugin", dir)

	data := []byte(`{"setting":"value"}`)
	require.NoError(t, store.SaveConfig(data))

	got, err := store.GetConfig()
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestBoltPluginStore_IsolatedBySlugs(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	s1 := newBoltPluginStore(db, "plugin-a", dir)
	s2 := newBoltPluginStore(db, "plugin-b", dir)

	_ = s1.Put("key", "plugin-a-value")
	_ = s2.Put("key", "plugin-b-value")

	var v1, v2 string
	_ = s1.Get("key", &v1)
	_ = s2.Get("key", &v2)
	assert.Equal(t, "plugin-a-value", v1)
	assert.Equal(t, "plugin-b-value", v2)
}

func TestBoltPluginStore_DataDir(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	store := newBoltPluginStore(db, "test-plugin", dir)
	assert.NotEmpty(t, store.DataDir())
}

// ── storage ──
type stubUserStore struct {
	mu    sync.RWMutex
	users map[uint64]*User
}

func (s *stubUserStore) Create(_ context.Context, u *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	return nil
}
func (s *stubUserStore) GetByID(_ context.Context, id uint64) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}
func (s *stubUserStore) GetByEmail(_ context.Context, _ string) (*User, error) {
	return nil, ErrNotFound
}
func (s *stubUserStore) GetByUsername(_ context.Context, _ string) (*User, error) {
	return nil, ErrNotFound
}
func (s *stubUserStore) Update(_ context.Context, _ *User) error  { return nil }
func (s *stubUserStore) Delete(_ context.Context, _ uint64) error { return nil }
func (s *stubUserStore) List(_ context.Context) ([]*User, error)  { return nil, nil }

// ── memFileStore ──────────────────────────────────────────────────────────────

type memFileStore struct {
	files  map[uint64]*File
	nextID uint64
}

func newMemFileStore() *memFileStore {
	return &memFileStore{files: make(map[uint64]*File)}
}

func (s *memFileStore) Save(_ context.Context, f *File) error {
	if f.ID == 0 {
		s.nextID++
		f.ID = s.nextID
	}
	cp := *f
	s.files[f.ID] = &cp
	return nil
}

func (s *memFileStore) GetByID(_ context.Context, id uint64) (*File, error) {
	f, ok := s.files[id]
	if !ok {
		return nil, ErrNotFound
	}
	return f, nil
}

func (s *memFileStore) GetByUserPath(_ context.Context, userID uint64, p string) (*File, error) {
	for _, f := range s.files {
		if f.UserID == userID && f.Path == p {
			return f, nil
		}
	}
	return nil, ErrNotFound
}

func (s *memFileStore) ListByUser(_ context.Context, userID uint64) ([]*File, error) {
	var out []*File
	for _, f := range s.files {
		if f.UserID == userID {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s *memFileStore) ListByUserAndParent(_ context.Context, userID uint64, parent string) ([]*File, error) {
	var out []*File
	for _, f := range s.files {
		if f.UserID == userID {
			dir := normalizePath(f.Path)
			// parent of file is the directory containing it
			parentDir := normalizePath(dirOf(dir))
			if parentDir == parent {
				out = append(out, f)
			}
		}
	}
	return out, nil
}

func (s *memFileStore) Delete(_ context.Context, id uint64) error {
	if _, ok := s.files[id]; !ok {
		return ErrNotFound
	}
	delete(s.files, id)
	return nil
}

// dirOf returns the directory containing the given path.
func dirOf(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/"
	}
	return p[:idx]
}

// ── memStorageEngine ──────────────────────────────────────────────────────────

type memEngine struct {
	data map[string][]byte
}

type spyStreamEngine struct {
	writeCalled bool
	written     int64
}

func (e *spyStreamEngine) Name() string { return "spy" }

func (e *spyStreamEngine) Write(_ context.Context, _ string, r io.Reader) error {
	e.writeCalled = true
	n, err := io.Copy(io.Discard, r)
	e.written = n
	return err
}

func (e *spyStreamEngine) Read(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, ErrNotFound
}

func (e *spyStreamEngine) Stat(_ context.Context, p string) (*FileStat, error) {
	return nil, ErrNotFound
}

func (e *spyStreamEngine) Move(_ context.Context, _, _ string) error {
	return nil
}

func (e *spyStreamEngine) Mkdir(_ context.Context, _ string) error {
	return nil
}

func (e *spyStreamEngine) Copy(_ context.Context, _, _ string) error {
	return nil
}

func (e *spyStreamEngine) Delete(_ context.Context, _ string) error {
	return nil
}

func (e *spyStreamEngine) List(_ context.Context, _ string) ([]EntryInfo, error) {
	return nil, nil
}

type errAfterReader struct {
	remaining int
	failErr   error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, r.failErr
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	if r.remaining == 0 {
		return n, r.failErr
	}
	return n, nil
}

func newMemEngine() *memEngine {
	return &memEngine{data: make(map[string][]byte)}
}

func (e *memEngine) Name() string { return "mem" }

func (e *memEngine) Write(_ context.Context, p string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	e.data[p] = b
	return nil
}

func (e *memEngine) Read(_ context.Context, p string) (io.ReadCloser, error) {
	b, ok := e.data[p]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(strings.NewReader(string(b))), nil
}

func (e *memEngine) Stat(_ context.Context, p string) (*FileStat, error) {
	b, ok := e.data[p]
	if !ok {
		return nil, ErrNotFound
	}
	return &FileStat{Path: p, Size: int64(len(b)), IsDir: false}, nil
}

func (e *memEngine) Move(_ context.Context, oldPath, newPath string) error {
	b, ok := e.data[oldPath]
	if !ok {
		return ErrNotFound
	}
	e.data[newPath] = b
	delete(e.data, oldPath)
	return nil
}

func (e *memEngine) Mkdir(_ context.Context, _ string) error {
	return nil
}

func (e *memEngine) Copy(_ context.Context, src, dst string) error {
	b, ok := e.data[src]
	if !ok {
		return ErrNotFound
	}
	e.data[dst] = b
	return nil
}

func (e *memEngine) Delete(_ context.Context, p string) error {
	delete(e.data, p)
	return nil
}

func (e *memEngine) List(_ context.Context, dirPath string) ([]EntryInfo, error) {
	dir := normalizePath(dirPath)
	seen := make(map[string]EntryInfo)

	for fp := range e.data {
		norm := normalizePath(fp)
		if dir == "/" {
			if !strings.HasPrefix(norm, "/") {
				continue
			}
		} else {
			prefix := dir + "/"
			if !strings.HasPrefix(norm, prefix) {
				continue
			}
		}

		rel := strings.TrimPrefix(norm, dir)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		parts := strings.Split(rel, "/")
		name := parts[0]
		if _, ok := seen[name]; ok {
			continue
		}
		typ := EntryFile
		if len(parts) > 1 {
			typ = EntryDir
		}
		seen[name] = EntryInfo{
			File:   File{Name: name, Path: path.Join(dir, name), Type: typ},
			Name:   name,
			Path:   path.Join(dir, name),
			IsDir:  typ == EntryDir,
			Type:   typ,
			Engine: e.Name(),
		}
	}

	out := make([]EntryInfo, 0, len(seen))
	for name := range seen {
		entry := seen[name]
		out = append(out, entry)
	}
	return out, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeStorageSvc(t *testing.T) *storageService {
	t.Helper()
	store := newMemFileStore()
	userStore := &stubUserStore{users: make(map[uint64]*User)}
	userStore.users[1] = &User{ID: 1, UUID: "user-1-uuid"}
	engine := newMemEngine()
	settingsSvc := newSettingsService(&memSettingsStore{})
	svc := newStorageService(store, userStore, settingsSvc, t.TempDir())
	svc.engines[1] = engine // user 1 → in-memory engine
	return svc
}

// ── normalizePath tests ───────────────────────────────────────────────────────

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"":           "/",
		"/":          "/",
		"foo":        "/foo",
		"/foo":       "/foo",
		"foo/bar":    "/foo/bar",
		"foo/../bar": "/bar",
		"//foo":      "/foo",
	}
	for input, want := range cases {
		assert.Equal(t, want, normalizePath(input), "input: %q", input)
	}
}

// ── storageService tests ──────────────────────────────────────────────────────

func TestStorageService_WriteAndGetByID(t *testing.T) {
	svc := makeStorageSvc(t)
	_, err := svc.WriteFile(context.Background(), 1, "/hello.txt", strings.NewReader("hello world"))
	require.NoError(t, err)

	// find the saved file
	files, err := svc.store.ListByUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, files, 1)

	entry, err := svc.GetFileByID(context.Background(), files[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "/hello.txt", entry.Path)
	assert.Equal(t, uint64(1), entry.UserID)
}

func TestStorageService_WriteFile_StreamErrorStillInvokesEngineWrite(t *testing.T) {
	store := newMemFileStore()
	userStore := &stubUserStore{users: make(map[uint64]*User)}
	userStore.users[1] = &User{ID: 1, UUID: "user-1-uuid"}
	engine := &spyStreamEngine{}
	settingsSvc := newSettingsService(&memSettingsStore{})
	svc := newStorageService(store, userStore, settingsSvc, t.TempDir())
	svc.engines[1] = engine

	r := &errAfterReader{remaining: 64, failErr: errors.New("stream read failed")}
	_, err := svc.WriteFile(context.Background(), 1, "/stream.bin", r)
	require.Error(t, err)
	assert.True(t, engine.writeCalled)
	assert.Contains(t, err.Error(), "stream read failed")
}

func TestStorageService_CreateDir(t *testing.T) {
	svc := makeStorageSvc(t)
	d, err := svc.CreateDir(context.Background(), 1, "/new_folder")
	require.NoError(t, err)
	assert.Equal(t, EntryDir, d.Type)
	assert.Equal(t, "/new_folder", d.Path)

	// Verify it's in the store
	f, err := svc.store.GetByUserPath(context.Background(), 1, "/new_folder")
	require.NoError(t, err)
	assert.Equal(t, EntryDir, f.Type)
}

func TestStorageService_ListByPath(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/a.txt", strings.NewReader("a"))
	_, _ = svc.WriteFile(context.Background(), 1, "/b.txt", strings.NewReader("b"))

	items, err := svc.ListByPath(context.Background(), 1, "/")
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestStorageService_ListByPath_Subdirectory(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/docs/readme.txt", strings.NewReader("r"))
	_, _ = svc.WriteFile(context.Background(), 1, "/other.txt", strings.NewReader("o"))

	items, err := svc.ListByPath(context.Background(), 1, "/docs")
	require.NoError(t, err)

	// should contain /docs/readme.txt
	var found bool
	for _, item := range items {
		if item.Path == "/docs/readme.txt" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestStorageService_DeleteFile(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/delete_me.txt", strings.NewReader("data"))
	files, _ := svc.store.ListByUser(context.Background(), 1)
	require.Len(t, files, 1)

	err := svc.DeleteFile(context.Background(), 1, files[0].ID)
	require.NoError(t, err)

	files2, _ := svc.store.ListByUser(context.Background(), 1)
	assert.Empty(t, files2)
}

func TestStorageService_DeleteFile_WrongUser(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/file.txt", strings.NewReader("data"))
	files, _ := svc.store.ListByUser(context.Background(), 1)

	// user 2 tries to delete user 1's file
	err := svc.DeleteFile(context.Background(), 2, files[0].ID)
	assert.Error(t, err)
}

func TestStorageService_RenameFile(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/old.txt", strings.NewReader("content"))
	files, _ := svc.store.ListByUser(context.Background(), 1)

	f, err := svc.RenameFile(context.Background(), 1, files[0].ID, "/new.txt")
	require.NoError(t, err)
	assert.Equal(t, "/new.txt", f.Path)
	assert.Equal(t, "new.txt", f.Name)
}

func TestStorageService_CopyFile(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/original.txt", strings.NewReader("content"))
	files, _ := svc.store.ListByUser(context.Background(), 1)

	f, err := svc.CopyFile(context.Background(), 1, files[0].ID, "/copy.txt")
	require.NoError(t, err)
	assert.Equal(t, "/copy.txt", f.Path)

	// Verify both exist
	files2, _ := svc.store.ListByUser(context.Background(), 1)
	assert.Len(t, files2, 2)
}

func TestStorageService_OpenFile(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/read.txt", strings.NewReader("hello"))
	files, _ := svc.store.ListByUser(context.Background(), 1)

	rc, f, err := svc.OpenFile(context.Background(), 1, files[0].ID)
	require.NoError(t, err)
	defer rc.Close()
	assert.Equal(t, "/read.txt", f.Path)

	content, _ := io.ReadAll(rc)
	assert.Equal(t, "hello", string(content))
}

func TestStorageService_OpenFile_WrongUser(t *testing.T) {
	svc := makeStorageSvc(t)
	_, _ = svc.WriteFile(context.Background(), 1, "/secret.txt", strings.NewReader("secret"))
	files, _ := svc.store.ListByUser(context.Background(), 1)

	_, _, err := svc.OpenFile(context.Background(), 2, files[0].ID)
	assert.Error(t, err)
}

func TestStorageService_GetEngine_DefaultsToPath(t *testing.T) {
	store := newMemFileStore()
	userStore := &stubUserStore{users: make(map[uint64]*User)}
	userStore.users[42] = &User{ID: 42, UUID: "user-42-uuid"}
	settingsSvc := newSettingsService(&memSettingsStore{})
	svc := newStorageService(store, userStore, settingsSvc, t.TempDir())
	engine, err := svc.GetEngine(42)
	require.NoError(t, err)
	assert.Equal(t, "path", engine.Name())
}

func TestStorageService_GetEngine_Concurrent(t *testing.T) {
	store := newMemFileStore()
	userStore := &stubUserStore{users: make(map[uint64]*User)}
	settingsSvc := newSettingsService(&memSettingsStore{})
	svc := newStorageService(store, userStore, settingsSvc, t.TempDir())

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(uid uint64) {
			defer wg.Done()
			_ = userStore.Create(context.Background(), &User{ID: uid % 8, UUID: "uuid"})
			_, err := svc.GetEngine(uid % 8)
			require.NoError(t, err)
		}(uint64(i))
	}
	wg.Wait()
}

func TestStorageService_RepairConsistency_DryRun(t *testing.T) {
	store := newMemFileStore()
	userStore := &stubUserStore{users: make(map[uint64]*User)}
	userStore.users[1] = &User{ID: 1, UUID: "user-1-uuid"}
	engine := newMemEngine()
	settingsSvc := newSettingsService(&memSettingsStore{})
	svc := newStorageService(store, userStore, settingsSvc, t.TempDir())
	svc.engines[1] = engine

	require.NoError(t, store.Save(context.Background(), &File{UserID: 1, Path: "/missing.txt", Name: "missing.txt", Type: EntryFile}))
	require.NoError(t, engine.Write(context.Background(), "/orphan.txt", strings.NewReader("orphan")))

	report, err := svc.RepairConsistency(context.Background(), 1, false, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, report.OrphanMeta)
	assert.Equal(t, 1, report.OrphanFile)
	assert.Equal(t, 0, report.FixedCount)

	_, err = store.GetByUserPath(context.Background(), 1, "/missing.txt")
	require.NoError(t, err)
	_, err = store.GetByUserPath(context.Background(), 1, "/orphan.txt")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStorageService_RepairConsistency_AutoFix(t *testing.T) {
	store := newMemFileStore()
	userStore := &stubUserStore{users: make(map[uint64]*User)}
	userStore.users[1] = &User{ID: 1, UUID: "user-1-uuid"}
	engine := newMemEngine()
	settingsSvc := newSettingsService(&memSettingsStore{})
	svc := newStorageService(store, userStore, settingsSvc, t.TempDir())
	svc.engines[1] = engine

	require.NoError(t, store.Save(context.Background(), &File{UserID: 1, Path: "/missing.txt", Name: "missing.txt", Type: EntryFile}))
	require.NoError(t, engine.Write(context.Background(), "/orphan.txt", strings.NewReader("orphan")))

	report, err := svc.RepairConsistency(context.Background(), 1, true, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, report.OrphanMeta)
	assert.Equal(t, 1, report.OrphanFile)
	assert.Equal(t, 2, report.FixedCount)
	assert.Equal(t, 0, report.FailedCount)

	_, err = store.GetByUserPath(context.Background(), 1, "/missing.txt")
	assert.ErrorIs(t, err, ErrNotFound)
	orphan, err := store.GetByUserPath(context.Background(), 1, "/orphan.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(6), orphan.Size)
	assert.Equal(t, "orphan.txt", orphan.Name)
}

// ── pathEngine tests ──────────────────────────────────────────────────────────

func TestPathEngine_WriteRead(t *testing.T) {
	dir := t.TempDir()
	e := newPathEngine(dir)
	err := e.Write(context.Background(), "/test.txt", strings.NewReader("content"))
	require.NoError(t, err)

	rc, err := e.Read(context.Background(), "/test.txt")
	require.NoError(t, err)
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	assert.Equal(t, "content", string(b))
}

func TestPathEngine_Move(t *testing.T) {
	dir := t.TempDir()
	e := newPathEngine(dir)
	_ = e.Write(context.Background(), "/source.txt", strings.NewReader("data"))
	require.NoError(t, e.Move(context.Background(), "/source.txt", "/dest.txt"))

	rc, err := e.Read(context.Background(), "/dest.txt")
	require.NoError(t, err)
	rc.Close()

	_, err = e.Read(context.Background(), "/source.txt")
	assert.Error(t, err)
}

func TestPathEngine_Move_EXDEVFallback(t *testing.T) {
	dir := t.TempDir()
	e := newPathEngine(dir)
	require.NoError(t, e.Write(context.Background(), "/source.txt", strings.NewReader("data")))

	orig := renameFile
	renameFile = func(_, _ string) error { return syscall.EXDEV }
	t.Cleanup(func() { renameFile = orig })

	require.NoError(t, e.Move(context.Background(), "/source.txt", "/dest.txt"))
	rc, err := e.Read(context.Background(), "/dest.txt")
	require.NoError(t, err)
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	assert.Equal(t, "data", string(b))
	_, err = e.Read(context.Background(), "/source.txt")
	assert.Error(t, err)
}

func TestPathEngine_Delete(t *testing.T) {
	dir := t.TempDir()
	e := newPathEngine(dir)
	_ = e.Write(context.Background(), "/tmp.txt", strings.NewReader("x"))
	require.NoError(t, e.Delete(context.Background(), "/tmp.txt"))
	_, err := e.Read(context.Background(), "/tmp.txt")
	assert.Error(t, err)
}

func TestPathEngine_List(t *testing.T) {
	dir := t.TempDir()
	e := newPathEngine(dir)
	_ = e.Write(context.Background(), "/a.txt", strings.NewReader("a"))
	_ = e.Write(context.Background(), "/b.txt", strings.NewReader("b"))

	entries, err := e.List(context.Background(), "/")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestPathEngine_Copy_FileIntegrity(t *testing.T) {
	dir := t.TempDir()
	e := newPathEngine(dir)
	payload := strings.Repeat("abc123", 2048)
	require.NoError(t, e.Write(context.Background(), "/src.txt", strings.NewReader(payload)))

	require.NoError(t, e.Copy(context.Background(), "/src.txt", "/dst.txt"))

	rc, err := e.Read(context.Background(), "/dst.txt")
	require.NoError(t, err)
	defer rc.Close()
	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, payload, string(b))
}

// ── virtualEngine tests ───────────────────────────────────────────────────────

func TestVirtualEngine(t *testing.T) {
	e := newVirtualEngine()
	assert.Equal(t, "virtual", e.Name())

	// Write is a no-op
	assert.NoError(t, e.Write(context.Background(), "/x", strings.NewReader("y")))
	// Read errors
	_, err := e.Read(context.Background(), "/x")
	assert.Error(t, err)
	// Move, Delete, List are noops
	assert.NoError(t, e.Move(context.Background(), "/a", "/b"))
	assert.NoError(t, e.Delete(context.Background(), "/a"))
	entries, err := e.List(context.Background(), "/")
	assert.NoError(t, err)
	assert.Empty(t, entries)
}

// ── MIME ──────────────────────────────────────────────────────────────────────

func TestDetectMIME_BySniff(t *testing.T) {
	// PNG magic bytes
	pngHeader := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20))
	mime := detectMIME("unknown", pngHeader)
	assert.Contains(t, mime, "png")
}

func TestDetectMIME_ByExtension(t *testing.T) {
	cases := map[string]string{
		"photo.jpg":  "image/jpeg",
		"photo.jpeg": "image/jpeg",
		"icon.png":   "image/png",
		"anim.gif":   "image/gif",
		"img.webp":   "image/webp",
		"doc.pdf":    "application/pdf",
		"data.json":  "application/json",
		"read.txt":   "text/plain; charset=utf-8",
	}
	for name, want := range cases {
		mime := detectMIME(name, nil)
		assert.Equal(t, want, mime, "file: %s", name)
	}
}

func TestDetectMIME_Unknown(t *testing.T) {
	mime := detectMIME("file.unknownxyz", nil)
	assert.Equal(t, "application/octet-stream", mime)
}

// ── Image ─────────────────────────────────────────────────────────────────────

func TestResizeToFit_InvalidReader(t *testing.T) {
	err := resizeToFit(bytes.NewReader([]byte("not an image")), 100, 100, new(bytes.Buffer))
	assert.Error(t, err)
}

func TestDecodeImage_InvalidData(t *testing.T) {
	_, err := decodeImage(bytes.NewReader([]byte("garbage")))
	assert.Error(t, err)
}

// ── api ──
// ── test app factory ──────────────────────────────────────────────────────────

func newTestApp(t *testing.T) *App {
	t.Helper()
	jwtSecret := make([]byte, 32)
	users := newMemUserStore()
	sessions := newMemSessionStore()
	fileStore := newMemFileStore()
	settingsSvc := newSettingsService(&memSettingsStore{})

	engine := newMemEngine()
	// Pre-populate test user 1
	_ = users.Create(t.Context(), &User{ID: 1, UUID: "test-user-1-uuid", Email: "system-test@example.com"})
	storageSvc := newStorageService(fileStore, users, settingsSvc, t.TempDir())
	storageSvc.engines[1] = engine // map user 1 to mem engine

	userSvc := &userService{store: users}
	authSvc := newAuthService(users, sessions, jwtSecret, time.Hour, 7*24*time.Hour, true)
	taskSvc := newTaskService(newMemTaskStore(), newScheduler())
	app := &App{
		userSvc:      userSvc,
		authSvc:      authSvc,
		storageSvc:   storageSvc,
		taskSvc:      taskSvc,
		settingsSvc:  settingsSvc,
		sessionStore: sessions,
		Router:       mux.NewRouter(),
	}

	// For testing, enable signup and set a short minimum password length
	_ = settingsSvc.Save(t.Context(), &Settings{
		Signup:                true,
		MinimumPasswordLength: 1,
		Defaults: SettingsDefaults{
			Locale: "auto",
			Theme:  "auto",
		},
		Branding: SettingsBranding{
			Theme: "auto",
		},
		StorageType: "path",
	})

	registerAllRoutes(app.Router, app)
	return app
}

func doPostRequest(t *testing.T, app *App, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	return rr
}

func doAuthRequest(t *testing.T, app *App, method, path string, body any, userID uint64, isAdmin bool) *httptest.ResponseRecorder {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	// Inject a valid JWT
	u := &User{ID: userID, Role: RoleUser}
	if isAdmin {
		u.Role = RoleAdmin
	}
	token, err := app.authSvc.signJWT(u)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	return rr
}

// ── context helpers tests ─────────────────────────────────────────────────────

func TestWithAuthContext(t *testing.T) {
	ctx := withAuthUserID(t.Context(), 42)
	ctx = withAuthIsAdmin(ctx, true)
	assert.Equal(t, uint64(42), AuthUserIDFromContext(ctx))
	assert.True(t, AuthIsAdminFromContext(ctx))
}

func TestAuthContext_Defaults(t *testing.T) {
	assert.Zero(t, AuthUserIDFromContext(t.Context()))
	assert.False(t, AuthIsAdminFromContext(t.Context()))
}

// ── ErrorResponse / WriteErr ──────────────────────────────────────────────────

func TestErrorResponse(t *testing.T) {
	e := ErrorResponse("something broke")
	assert.Equal(t, "something broke", e["error"])
}

func TestWriteErr_AppError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteErr(rr, ErrNotFound)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestWriteErr_GenericError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteErr(rr, ErrInternal)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestWriteErr_EmptyGenericError_DoesNotPanic(t *testing.T) {
	rr := httptest.NewRecorder()
	assert.NotPanics(t, func() {
		WriteErr(rr, errors.New(""))
	})
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ── Identity HTTP handlers ────────────────────────────────────────────────────

func TestHandleRegister_Success(t *testing.T) {
	app := newTestApp(t)
	rr := doPostRequest(t, app, "/api/signup", map[string]any{
		"email":    "new@example.com",
		"username": "newuser",
		"password": "secret",
	})
	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestHandleSetupStatus(t *testing.T) {
	app := newTestApp(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", http.NoBody)
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"initialized":true`)
}

func TestHandleRegister_DuplicateEmail(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "dup@example.com", "username": "u1", "password": "pass",
	})
	rr := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "dup@example.com", "username": "u2", "password": "pass",
	})
	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestHandleRegister_BadJSON(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/signup", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleLogin_Success(t *testing.T) {
	app := newTestApp(t)
	// register first
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "test@example.com", "username": "testuser", "password": "password",
	})
	require.Equal(t, http.StatusCreated, rrReg.Code)
	rr := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "test@example.com", "password": "password",
	})
	assert.Equal(t, http.StatusOK, rr.Code)
	var res struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.NotEmpty(t, res.Data.AccessToken)
	assert.NotEmpty(t, res.Data.RefreshToken)
}

func TestHandleLogin_Username(t *testing.T) {
	app := newTestApp(t)
	// register first
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "test@example.com", "username": "testuser", "password": "password",
	})
	require.Equal(t, http.StatusCreated, rrReg.Code)

	// Login with username instead of email
	rr := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "testuser", "password": "password",
	})
	assert.Equal(t, http.StatusOK, rr.Code)
	var res struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.NotEmpty(t, res.Data.AccessToken)
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "wp@example.com", "username": "wpu", "password": "correct",
	})
	rr := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "wp@example.com", "password": "wrong",
	})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleRefresh(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "ref@example.com", "username": "refu", "password": "pw",
	})
	assert.Equal(t, http.StatusCreated, rrReg.Code)
	loginRR := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "ref@example.com", "password": "pw",
	})
	var loginRes struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(loginRR.Body.Bytes(), &loginRes))

	rr := doPostRequest(t, app, "/api/auth/refresh", map[string]any{
		"refreshToken": loginRes.Data.RefreshToken,
	})
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleRefresh_InvalidToken(t *testing.T) {
	app := newTestApp(t)
	rr := doPostRequest(t, app, "/api/auth/refresh", map[string]any{
		"refreshToken": "bogus",
	})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleLogout(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "logout@example.com", "username": "lo", "password": "pw",
	})
	loginRR := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "logout@example.com", "password": "pw",
	})
	var loginRes struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(loginRR.Body.Bytes(), &loginRes))

	rr := doAuthRequest(t, app, http.MethodPost, "/api/auth/logout", map[string]any{
		"refreshToken": loginRes.Data.RefreshToken,
	}, loginRes.Data.UserID, false)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleRevokeSession_RejectsOtherUsersSession(t *testing.T) {
	app := newTestApp(t)
	require.NoError(t, app.sessionStore.(*memSessionStore).Save(t.Context(), &RefreshToken{
		ID:        "other-session",
		UserID:    2,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	rr := doAuthRequest(t, app, http.MethodDelete, "/api/auth/sessions/other-session", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleRevokeSession_AdminCanRevokeOtherUsersSession(t *testing.T) {
	app := newTestApp(t)
	require.NoError(t, app.sessionStore.(*memSessionStore).Save(t.Context(), &RefreshToken{
		ID:        "other-session",
		UserID:    2,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	rr := doAuthRequest(t, app, http.MethodDelete, "/api/auth/sessions/other-session", nil, 1, true)
	assert.Equal(t, http.StatusOK, rr.Code)
	_, err := app.sessionStore.GetByID(t.Context(), "other-session")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestHandleMe_Authenticated(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "me@example.com", "username": "meuser", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	rr := doAuthRequest(t, app, http.MethodGet, "/api/me", nil, regRes.Data.ID, false)
	assert.Equal(t, http.StatusOK, rr.Code)
	var got struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, regRes.Data.ID, uint64(got.Data["id"].(float64)))
}

func TestHandleMe_Unauthenticated(t *testing.T) {
	app := newTestApp(t)
	// Make request without Auth header — middleware should block
	req := httptest.NewRequest(http.MethodGet, "/api/me", http.NoBody)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAuthMiddleware_QueryTokenDisabled(t *testing.T) {
	app := newTestApp(t)
	app.authSvc.allowQueryToken = false

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/me?auth="+tok, http.NoBody)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleUpdateMe(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "upd@example.com", "username": "updu", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	rr := doAuthRequest(t, app, http.MethodPut, "/api/me", map[string]any{
		"displayName": "Updated",
	}, regRes.Data.ID, false)
	assert.Equal(t, http.StatusOK, rr.Code)
	var got struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "Updated", got.Data["displayName"])
}

func TestHandleListUsers_AdminOnly(t *testing.T) {
	app := newTestApp(t)

	// non-admin gets 403
	rr := doAuthRequest(t, app, http.MethodGet, "/api/users", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	// admin gets 200
	rr = doAuthRequest(t, app, http.MethodGet, "/api/users", nil, 1, true)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleGetUser_AdminOnly(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "gu@example.com", "username": "guu", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	rr := doAuthRequest(t, app, http.MethodGet, "/api/users/999", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	rr = doAuthRequest(t, app, http.MethodGet, "/api/users/999", nil, 1, true)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleDeleteUser_AdminOnly(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "du@example.com", "username": "duu", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	loginRR := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "du@example.com", "password": "pw",
	})
	assert.Equal(t, http.StatusOK, loginRR.Code)
	sessions := app.sessionStore.(*memSessionStore)
	assert.Len(t, sessions.sessions, 1)

	// non-admin
	rr := doAuthRequest(t, app, http.MethodDelete, "/api/users/1", nil, 99, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	// admin deletes existing user
	deletePath := "/api/users/" + uintToStr(regRes.Data.ID)
	rr = doAuthRequest(t, app, http.MethodDelete, deletePath, nil, 99, true)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, sessions.sessions)
}

// ── Settings HTTP handlers ────────────────────────────────────────────────────

func TestHandleSettingsGet_Admin(t *testing.T) {
	app := newTestApp(t)
	// First save some settings via admin
	rr := doAuthRequest(t, app, http.MethodPut, "/api/settings", &Settings{
		Branding: SettingsBranding{Name: "TestApp"},
	}, 1, true)
	assert.Equal(t, http.StatusNoContent, rr.Code)

	rr2 := doAuthRequest(t, app, http.MethodGet, "/api/settings", nil, 1, true)
	assert.Equal(t, http.StatusOK, rr2.Code)
	var s struct {
		Data Settings `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &s))
	assert.Equal(t, "TestApp", s.Data.Branding.Name)
}

func TestHandleSettingsGet_NonAdmin_Forbidden(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/settings", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleSettingsSave_Admin(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPut, "/api/settings", map[string]any{
		"branding": map[string]string{"name": "My Abyss", "theme": "dark"},
	}, 1, true)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandleSettingsSave_NonAdmin_Forbidden(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPut, "/api/settings", map[string]any{}, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// ── Task HTTP handlers ────────────────────────────────────────────────────────

func TestHandleTaskList(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/tasks", nil, 1, false)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleTaskSubmit_UnregisteredRunner(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPost, "/api/tasks", map[string]any{
		"name": "no-such-runner",
	}, 1, false)
	// Runner not registered → should fail with some error (500 or 400)
	assert.GreaterOrEqual(t, rr.Code, 400)
}

func TestHandleTaskCancel_NotFound(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPost, "/api/tasks/nonexistent-id/cancel", nil, 1, false)
	// Cancel of unknown task returns an error (scheduler doesn't know it)
	assert.GreaterOrEqual(t, rr.Code, 400)
}

// ── Storage HTTP handlers ─────────────────────────────────────────────────────

func TestHandleFileList(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/resources/?path=/", nil, 1, false)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleFileUpload(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "uploader@example.com", "username": "uploader", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))
	app.storageSvc.engines[regRes.Data.ID] = newMemEngine()

	req := httptest.NewRequest(http.MethodPost, "/api/resources/?path=/hello.txt", bytes.NewBufferString("file contents"))
	req.Header.Set("Content-Type", "application/octet-stream")
	tok, _ := app.authSvc.signJWT(&User{ID: regRes.Data.ID, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)
	var f struct {
		Data File `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &f))
	assert.Equal(t, "/hello.txt", f.Data.Path)
}

func TestHandleMkdir(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "mkdir@example.com", "username": "mkdiruser", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	// POST to path with trailing slash
	req := httptest.NewRequest(http.MethodPost, "/api/resources/my_folder/", http.NoBody)
	tok, _ := app.authSvc.signJWT(&User{ID: regRes.Data.ID, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var res struct {
		Data File `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.Equal(t, EntryDir, res.Data.Type)
	assert.Equal(t, "/my_folder", res.Data.Path)
}

func TestHandleFileUpload_MissingPath(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/resources/", bytes.NewBufferString("data"))
	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleFileGetByID_NotFound(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/resources/9999", nil, 1, false)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func uintToStr(n uint64) string {
	return strconv.FormatUint(n, 10)
}
func TestHandleFileMove(t *testing.T) {
	app := newTestApp(t)
	// Create file
	doAuthRequest(t, app, http.MethodPost, "/api/resources/old.txt", bytes.NewBufferString("data"), 1, false)

	// PATCH with query params
	req := httptest.NewRequest(http.MethodPatch, "/api/resources/old.txt?action=rename&destination=/new.txt", http.NoBody)
	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var res struct {
		Data File `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.Equal(t, "/new.txt", res.Data.Path)
}

func TestHandleFileCopy(t *testing.T) {
	app := newTestApp(t)
	doAuthRequest(t, app, http.MethodPost, "/api/resources/src.txt", bytes.NewBufferString("data"), 1, false)

	req := httptest.NewRequest(http.MethodPatch, "/api/resources/src.txt?action=copy&destination=/dst.txt", http.NoBody)
	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleFileDelete_NormalizesPath(t *testing.T) {
	app := newTestApp(t)
	app.storageSvc.engines[1] = newMemEngine()
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/docs/a.txt", bytes.NewBufferString("x"))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodDelete, "/api/resources/?path=/docs/../docs//a.txt", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandleFileDownload_ContentDispositionEscaped(t *testing.T) {
	app := newTestApp(t)
	engine := newMemEngine()
	app.storageSvc.engines[1] = engine
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/evil\"name.txt", bytes.NewBufferString("x"))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/raw/evil\"name.txt", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	cd := rr.Header().Get("Content-Disposition")
	assert.Contains(t, cd, "inline")
	assert.NotContains(t, cd, "\n")
	assert.NotContains(t, cd, "\r")
	assert.Equal(t, "x", rr.Body.String())
}

func TestHandleFileDownload_ContentMatchesUploadedFile(t *testing.T) {
	app := newTestApp(t)
	engine := newMemEngine()
	app.storageSvc.engines[1] = engine
	payload := "download-payload"
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/dl.txt", bytes.NewBufferString(payload))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/raw/dl.txt", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, payload, rr.Body.String())
	assert.Contains(t, rr.Header().Get("Content-Disposition"), "filename")
}

func TestHandleFilePreview_ReturnsJPEG(t *testing.T) {
	app := newTestApp(t)
	engine := newMemEngine()
	app.storageSvc.engines[1] = engine

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	var src bytes.Buffer
	require.NoError(t, png.Encode(&src, img))
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/preview.png", bytes.NewReader(src.Bytes()))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/preview/small/preview.png", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
}

func TestHandleUpdateUser(t *testing.T) {
	app := newTestApp(t)
	// Signup user 1
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "u1@e.com", "username": "u1", "password": "pw",
	})
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "u2@e.com", "username": "u2", "password": "pw",
	})

	// admin updates user 2
	rrAdmin := doAuthRequest(t, app, http.MethodPut, "/api/users/2", map[string]any{
		"data": map[string]any{"displayName": "Admin Updated"},
	}, 99, true)
	assert.Equal(t, http.StatusOK, rrAdmin.Code)

	// Update user 1 as user 1
	rr := doAuthRequest(t, app, http.MethodPut, "/api/users/1", map[string]any{
		"data": map[string]any{"displayName": "New Name"},
	}, 1, false)
	assert.Equal(t, http.StatusOK, rr.Code)

	var res struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.Equal(t, "New Name", res.Data["displayName"])
}

func TestHandleUpdateUser_SelfCannotEscalateRoleOrEditEmail(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "self@e.com", "username": "self", "password": "pw",
	})

	rr := doAuthRequest(t, app, http.MethodPut, "/api/users/2", map[string]any{
		"data": map[string]any{
			"displayName": "Safe Name",
			"email":       "attacker@example.com",
			"role":        "admin",
			"permissions": map[string]any{"admin": true},
		},
	}, 2, false)
	assert.Equal(t, http.StatusOK, rr.Code)

	u, err := app.userSvc.GetByID(t.Context(), 2)
	require.NoError(t, err)
	assert.Equal(t, "Safe Name", u.DisplayName)
	assert.Equal(t, "self@e.com", u.Email)
	assert.Equal(t, RoleUser, u.Role)
	assert.False(t, u.Permissions.Admin)
}

func TestHandleUpdateUser_AdminCanUpdateRoleAndPermissions(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "managed@e.com", "username": "managed", "password": "pw",
	})

	rr := doAuthRequest(t, app, http.MethodPut, "/api/users/2", map[string]any{
		"data": map[string]any{
			"role": "admin",
			"permissions": map[string]any{
				"admin":  true,
				"create": true,
				"rename": true,
			},
		},
	}, 1, true)
	assert.Equal(t, http.StatusOK, rr.Code)

	u, err := app.userSvc.GetByID(t.Context(), 2)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, u.Role)
	assert.True(t, u.Permissions.Admin)
	assert.True(t, u.Permissions.Create)
	assert.True(t, u.Permissions.Rename)
}

func TestDecodeJSON_RejectsOversizedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(bytes.Repeat([]byte("a"), int(maxJSONBodySize+1))))
	var payload map[string]any
	err := DecodeJSON(req, &payload)
	assert.Error(t, err)
}

// ── plugin ──
// ── Shared stubs ──────────────────────────────────────────────────────────────

type stubPlugin struct {
	slug string
	deps []string
	typ  PluginType
}

func (s *stubPlugin) Info() PluginInfo {
	pt := s.typ
	if pt == "" {
		pt = TypeFree
	}
	return PluginInfo{
		SlugName:     s.slug,
		Type:         pt,
		Dependencies: s.deps,
	}
}

func newStub(slug string, deps ...string) Base {
	return &stubPlugin{slug: slug, deps: deps}
}

func newPaidStub(slug string, deps ...string) Base {
	return &stubPlugin{slug: slug, typ: TypePaid, deps: deps}
}

// ── interfaces_test ───────────────────────────────────────────────────────────

type stubAuthenticator struct {
	slug    string
	methods []PluginAuthMethod
	login   *PluginAuthResult
	err     error
}

func (s *stubAuthenticator) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubAuthenticator) AuthMethods() []PluginAuthMethod {
	return s.methods
}

func (s *stubAuthenticator) Authenticate(_ string, _ map[string]interface{}) (uint64, error) {
	if s.login != nil {
		return 1, s.err
	}
	return 0, s.err
}

func (s *stubAuthenticator) OnLoginSuccess(_ uint64, _ *http.Request) (*PluginAuthResult, error) {
	return s.login, s.err
}

func (s *stubAuthenticator) VerifyMFA(_ uint64, _ string, _ map[string]interface{}) (bool, error) {
	return s.err == nil, s.err
}

func (s *stubAuthenticator) OnRegisterSuccess(_ uint64, _ *http.Request) error {
	return s.err
}

type stubNotification struct {
	slug string
	sent *[]NotificationMessage
	err  error
}

func (s *stubNotification) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubNotification) Send(msg NotificationMessage) error {
	if s.sent != nil {
		*s.sent = append(*s.sent, msg)
	}
	return s.err
}

type stubDeletionHook struct {
	slug    string
	handled bool
	err     error
}

func (s *stubDeletionHook) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubDeletionHook) OnDelete(_ context.Context, _ uint64, _ string, _ bool, _ int64) (bool, error) {
	return s.handled, s.err
}

type stubConfigPlugin struct {
	slug   string
	config []byte
	inits  int
}

func (s *stubConfigPlugin) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubConfigPlugin) Init(_ *StartupContext) error {
	s.inits++
	return nil
}

func (s *stubConfigPlugin) Stop(_ context.Context) error { return nil }

func (s *stubConfigPlugin) ConfigFields() []ConfigField { return nil }

func (s *stubConfigPlugin) ConfigReceiver(config []byte) error {
	s.config = append([]byte(nil), config...)
	return nil
}

func TestGetAuthMethods_ReturnsEmptyWhenNothingRegistered(t *testing.T) {
	oldCall := CallAuthenticator
	t.Cleanup(func() {
		CallAuthenticator = oldCall
	})

	call, _, _, _ := MakePlugin[Authenticator](false)
	CallAuthenticator = call

	assert.Empty(t, GetAuthMethods())
}

func TestGetAuthMethods_AggregatesAcrossAuthenticators(t *testing.T) {
	oldCall := CallAuthenticator
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallAuthenticator = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Authenticator](false)
	register(&stubAuthenticator{slug: "passkey", methods: []PluginAuthMethod{{SlugName: "passkey", Name: "Passkey"}}})
	register(&stubAuthenticator{slug: "otp", methods: []PluginAuthMethod{{SlugName: "otp", Name: "OTP"}}})
	CallAuthenticator = call

	methods := GetAuthMethods()
	require.Len(t, methods, 2)
	assert.Equal(t, []string{"passkey", "otp"}, []string{methods[0].SlugName, methods[1].SlugName})
}

func TestSendNotification_DispatchesToAllRegisteredNotifications(t *testing.T) {
	oldCall := CallNotification
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallNotification = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Notification](false)
	var sentA []NotificationMessage
	var sentB []NotificationMessage
	register(&stubNotification{slug: "smtp", sent: &sentA})
	register(&stubNotification{slug: "webhook", sent: &sentB})
	CallNotification = call

	msg := NotificationMessage{Type: NotifyUserCreated, Recipient: "u1", Subject: "subject", Body: "body"}
	SendNotification(msg)

	require.Len(t, sentA, 1)
	require.Len(t, sentB, 1)
	assert.Equal(t, msg, sentA[0])
	assert.Equal(t, msg, sentB[0])
}

func TestSendNotification_SwallowsPluginError(t *testing.T) {
	oldCall := CallNotification
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallNotification = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Notification](false)
	var sent []NotificationMessage
	register(&stubNotification{slug: "smtp", sent: &sent, err: errors.New("send failed")})
	CallNotification = call

	assert.NotPanics(t, func() {
		SendNotification(NotificationMessage{Type: NotifyPasswordReset, Recipient: "u1"})
	})
	require.Len(t, sent, 1)
}

func TestDeletionHook_MakePluginStopsAfterHandled(t *testing.T) {
	call, _, _, register := MakePlugin[DeletionHook](true)
	seen := 0
	register(&stubDeletionHook{slug: "trash", handled: true})
	register(&stubDeletionHook{slug: "audit", handled: false})

	err := call(func(h DeletionHook) error {
		seen++
		handled, err := h.OnDelete(context.Background(), 1, "/tmp/file", false, 123)
		if err != nil {
			return err
		}
		if handled {
			return errors.New("handled")
		}
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, 1, seen)
}

func TestMemoryEventBus_Close(t *testing.T) {
	bus := NewMemoryEventBus()
	var calls atomic.Int32

	_ = bus.Subscribe("topic", func(_ interface{}) {
		calls.Add(1)
	})

	bus.Publish("topic", "first")
	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, time.Second, 10*time.Millisecond)

	bus.Close()
	bus.Publish("topic", "second")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), calls.Load())
}

// ── registry_test ─────────────────────────────────────────────────────────────

func TestPlugin_Info_Paid(t *testing.T) {
	p := newPaidStub("paid-plugin", "dep1")
	info := p.Info()
	assert.Equal(t, "paid-plugin", info.SlugName)
	assert.Equal(t, TypePaid, info.Type)
	assert.Contains(t, info.Dependencies, "dep1")
}

func TestSortPlugins_NoDependencies(t *testing.T) {
	plugins := []Base{
		newStub("a"),
		newStub("b"),
		newStub("c"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	assert.Len(t, sorted, 3)
	slugs := make(map[string]bool)
	for _, p := range sorted {
		slugs[p.Info().SlugName] = true
	}
	assert.True(t, slugs["a"])
	assert.True(t, slugs["b"])
	assert.True(t, slugs["c"])
}

func TestSortPlugins_Simple(t *testing.T) {
	plugins := []Base{
		newStub("b", "a"),
		newStub("a"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	require.Len(t, sorted, 2)

	indexA := -1
	indexB := -1
	for i, p := range sorted {
		switch p.Info().SlugName {
		case "a":
			indexA = i
		case "b":
			indexB = i
		}
	}
	assert.Less(t, indexA, indexB, "plugin a must come before plugin b")
}

func TestSortPlugins_MultiLevel(t *testing.T) {
	plugins := []Base{
		newStub("c", "b"),
		newStub("a"),
		newStub("b", "a"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	require.Len(t, sorted, 3)

	idx := map[string]int{}
	for i, p := range sorted {
		idx[p.Info().SlugName] = i
	}
	assert.Less(t, idx["a"], idx["b"])
	assert.Less(t, idx["b"], idx["c"])
}

func TestSortPlugins_CircularDependency(t *testing.T) {
	plugins := []Base{
		newStub("a", "b"),
		newStub("b", "a"),
	}
	_, err := SortPlugins(plugins)
	assert.Error(t, err, "circular dependency should be detected")
}

func TestSortPlugins_UnknownDependencyIsIgnored(t *testing.T) {
	plugins := []Base{
		newStub("a"),
		newStub("b", "external"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	assert.Len(t, sorted, 2)
}

func TestStack_Register_And_Get(t *testing.T) {
	call, _, get, register := MakePlugin[Base](true)

	pa := newStub("test-reg-a")
	pb := newStub("test-reg-b")
	register(pa)
	register(pb)

	retrieved := get("test-reg-a")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-reg-a", retrieved.Info().SlugName)

	count := 0
	err := call(func(_ Base) error {
		count++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestStack_DuplicateRegistrationPanics(t *testing.T) {
	_, _, _, register := MakePlugin[Base](true)

	register(newStub("dup-slug"))
	assert.Panics(t, func() {
		register(newStub("dup-slug"))
	}, "registering the same slug twice should panic")
}

func TestStack_ConcurrentRegister_Safe(t *testing.T) {
	_, _, _, register := MakePlugin[Base](true)

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		slug := "concurrent-" + string(rune('a'+i))
		go func(s string) {
			defer wg.Done()
			register(&stubPlugin{slug: s})
		}(slug)
	}
	wg.Wait()
}

func TestStack_GetNonExistent_ReturnsNil(t *testing.T) {
	_, _, get, _ := MakePlugin[Base](true)
	result := get("does-not-exist")
	var zero Base
	assert.Equal(t, zero, result)
}

// ── manager_test ──────────────────────────────────────────────────────────────

func newTestManager() *statusManager {
	m := &statusManager{status: make(map[string]bool)}
	StatusManager = m
	return m
}

func registerNewStub(slug string, deps ...string) {
	RegisterBase(&stubPlugin{slug: slug, typ: TypeFree, deps: deps})
}

func TestStatusManager_Plugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-free"
	registerNewStub(slug)

	assert.False(t, m.IsEnabled(slug), "plugin should be disabled by default")
}

func TestStatusManager_PaidPlugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-paid"
	RegisterBase(&stubPlugin{slug: slug, typ: TypePaid})

	assert.False(t, m.IsEnabled(slug), "paid (pro) plugin should be disabled by default")
}

func TestStatusManager_UnknownPlugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	assert.False(t, m.IsEnabled("totally-unknown"), "fallback for unknown plugin is false")
}

func TestStatusManager_Enable_FreePlugin(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-fp"
	registerNewStub(slug)

	require.NoError(t, m.Enable(slug, true))
	assert.True(t, m.IsEnabled(slug))

	require.NoError(t, m.Enable(slug, false))
	assert.False(t, m.IsEnabled(slug))
}

func TestStatusManager_Enable_DependencyNotActive(t *testing.T) {
	m := newTestManager()
	slugA := t.Name() + "-A"
	slugB := t.Name() + "-B"
	registerNewStub(slugA)
	registerNewStub(slugB, slugA)

	require.NoError(t, m.Enable(slugA, false))

	err := m.Enable(slugB, true)
	assert.Error(t, err, "should fail when dependency is disabled")
}

func TestStatusManager_SetStatuses(t *testing.T) {
	m := newTestManager()
	slugA := t.Name() + "-a"
	slugB := t.Name() + "-b"
	registerNewStub(slugA)
	registerNewStub(slugB)

	m.SetStatuses(map[string]bool{
		slugA: true,
		slugB: false,
	})

	assert.True(t, m.IsEnabled(slugA))
	assert.False(t, m.IsEnabled(slugB))
}

func TestStatusManager_PersistenceHook_Called(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-hook"
	registerNewStub(slug)

	called := false
	m.SetPersistenceHook(func(name string, enabled bool) error {
		if name == slug {
			called = true
		}
		return nil
	})

	require.NoError(t, m.Enable(slug, true))
	assert.True(t, called, "persistence hook should have been called")
}

func TestStatusManager_PersistenceHook_Error_PropagatesError(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-hookerr"
	registerNewStub(slug)

	m.SetPersistenceHook(func(_ string, _ bool) error {
		return errors.New("storage failure")
	})

	err := m.Enable(slug, true)
	assert.Error(t, err)
}

func TestStatusManager_IsEnabled_RaceCondition(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-race"
	registerNewStub(slug)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = m.IsEnabled(slug)
		}()
		go func() {
			defer wg.Done()
			_ = m.Enable(slug, true)
		}()
	}
	wg.Wait()
}

// ── skeleton_test ─────────────────────────────────────────────────────────────

func TestSkeleton_Init_SetsCtx(t *testing.T) {
	s := &Skeleton{}
	ctx := &StartupContext{}
	err := s.Init(ctx)
	require.NoError(t, err)
	assert.Equal(t, ctx, s.Ctx)
}

func TestSkeleton_Stop_IsNoop(t *testing.T) {
	s := &Skeleton{Ctx: &StartupContext{}}
	err := s.Stop(context.Background())
	assert.NoError(t, err)
}

func TestSkeleton_ConfigFields_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	assert.Nil(t, s.ConfigFields())
}

func TestSkeleton_ConfigReceiver_StoresBytes(t *testing.T) {
	s := &Skeleton{}
	data := []byte(`{"key":"value"}`)
	err := s.ConfigReceiver(data)
	require.NoError(t, err)
	assert.Equal(t, data, s.RawConfig)
}

func TestSkeleton_GetConfig_Empty(t *testing.T) {
	s := &Skeleton{}
	var cfg map[string]string
	err := s.GetConfig(&cfg)
	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestSkeleton_GetConfig_Unmarshal(t *testing.T) {
	s := &Skeleton{}
	raw, _ := json.Marshal(map[string]string{"foo": "bar"})
	s.RawConfig = raw

	var cfg map[string]string
	err := s.GetConfig(&cfg)
	require.NoError(t, err)
	assert.Equal(t, "bar", cfg["foo"])
}

func TestSkeleton_GetConfig_InvalidJSON(t *testing.T) {
	s := &Skeleton{RawConfig: []byte(`not-json`)}
	var cfg map[string]string
	err := s.GetConfig(&cfg)
	assert.Error(t, err)
}

func TestSkeleton_RegisterRoutes_IsNoop(t *testing.T) {
	s := &Skeleton{}
	assert.NotPanics(t, func() {
		s.RegisterRoutes(nil, nil, nil, nil)
	})
}

func TestSkeleton_AvailableStorageTypes_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	assert.Nil(t, s.AvailableStorageTypes())
}

func TestSkeleton_CreateUserEngine_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	engine, err := s.CreateUserEngine(1)
	assert.NoError(t, err)
	assert.Nil(t, engine)
}

func TestManagerInit_LoadsPersistedPluginConfig(t *testing.T) {
	oldCallPluginAll := CallPluginAll
	t.Cleanup(func() {
		CallPluginAll = oldCallPluginAll
	})

	call, _, _, register := MakePlugin[Plugin](true)
	CallPluginAll = call

	plugin := &stubConfigPlugin{slug: "config-plugin"}
	register(plugin)

	db := openTestDB(t)
	dataDir := t.TempDir()
	store := newBoltPluginStore(db, plugin.slug, dataDir)
	require.NoError(t, store.SaveConfig([]byte(`{"foo":"bar"}`)))

	mgr := NewManager()
	err := mgr.Init(&StartupContext{
		StoreFactory: func(slug string) PluginStore {
			return newBoltPluginStore(db, slug, dataDir)
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"foo":"bar"}`, string(plugin.config))
	assert.Equal(t, 1, plugin.inits)

	stored, ok := GetStoreTyped[PluginStore](plugin.slug)
	assert.True(t, ok)
	assert.NotNil(t, stored)
}

// ── plugin_storage_provider ──
type testShardedProvider struct{}

func (testShardedProvider) Info() PluginInfo {
	return PluginInfo{SlugName: "sharded-test-provider", Name: "sharded-test-provider"}
}

func (testShardedProvider) AvailableStorageTypes() []StorageTypeInfo {
	return []StorageTypeInfo{{Name: "sharded", DisplayName: "sharded", Description: "test sharded"}}
}

func (testShardedProvider) CreateUserEngine(uint64) (StorageEngine, error) {
	return testShardedEngine{}, nil
}

func (testShardedProvider) OnUserEngineInit(uint64, func(string, StorageEngine)) error {
	return nil
}

func (testShardedProvider) ResolveVirtualPath(context.Context, uint64, string) (*VirtualPathInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no virtual path.
}

func (testShardedProvider) GetVirtualEntries(context.Context, uint64) ([]*EntryInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no virtual entries.
}

func (testShardedProvider) MigrateStorage(context.Context, string, string, func(uint64) (StorageEngine, error)) error {
	return nil
}

func (testShardedProvider) PreflightMigration(context.Context, string, string) error {
	return nil
}

func (testShardedProvider) TUSUploadComplete(context.Context, uint64, string, ReadSeekCloser) error {
	return nil
}

type testShardedEngine struct{}

func (testShardedEngine) Name() string { return "sharded" }

func (testShardedEngine) Read(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrNotFound
}

func (testShardedEngine) Stat(context.Context, string) (*FileStat, error) {
	return nil, ErrNotFound
}

func (testShardedEngine) Write(context.Context, string, io.Reader) error { return nil }
func (testShardedEngine) Mkdir(context.Context, string) error            { return nil }
func (testShardedEngine) Copy(context.Context, string, string) error     { return nil }
func (testShardedEngine) Move(context.Context, string, string) error     { return nil }
func (testShardedEngine) Delete(context.Context, string) error           { return nil }

func (testShardedEngine) List(context.Context, string) ([]EntryInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no entries.
}

func TestStorageSwitchToSharded(t *testing.T) {
	RegisterStorageProvider(testShardedProvider{})
	_ = StatusManager.Enable("sharded-test-provider", true)

	db := openTestDB(t)

	userStore := &boltUserStore{db: db}
	settingsStore := &boltSettingsStore{db: db}
	fileStore := &boltFileStore{db: db}
	settingsSvc := newSettingsService(settingsStore)

	dataDir, err := os.MkdirTemp("", "abyss-storage-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dataDir)

	storageSvc := newStorageService(fileStore, userStore, settingsSvc, dataDir)

	user := &User{Email: "test@example.com", Username: "sharded_test_user", UUID: "sharded-test-uuid"}
	err = userStore.Create(context.Background(), user)
	require.NoError(t, err)
	require.NotZero(t, user.ID)

	engine, err := storageSvc.GetEngine(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "path", engine.Name())

	settings, err := settingsSvc.Get(context.Background())
	require.NoError(t, err)
	settings.StorageType = "sharded"
	err = settingsSvc.Save(context.Background(), settings)
	require.NoError(t, err)

	storageSvc.engines = make(map[uint64]StorageEngine)

	engine, err = storageSvc.GetEngine(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "sharded", engine.Name())
}

// ── errors ──
func TestError_Error_WithoutCause(t *testing.T) {
	e := &Error{Code: "not_found", Message: "resource not found"}
	assert.Equal(t, "not_found: resource not found", e.Error())
}

func TestError_Error_WithCause(t *testing.T) {
	cause := errors.New("db timeout")
	e := &Error{Code: "internal", Message: "internal server error", Cause: cause}
	assert.Contains(t, e.Error(), "internal")
	assert.Contains(t, e.Error(), "db timeout")
}

func TestError_Error_Nil(t *testing.T) {
	var e *Error
	assert.Equal(t, "", e.Error())
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("original")
	e := &Error{Code: "internal", Message: "wrapped", Cause: cause}
	assert.Equal(t, cause, errors.Unwrap(e))
}

func TestError_Unwrap_Nil(t *testing.T) {
	var e *Error
	assert.Nil(t, e.Unwrap())
}

func TestError_Unwrap_NoCause(t *testing.T) {
	e := &Error{Code: "not_found", Message: "not found"}
	assert.Nil(t, e.Unwrap())
}

func TestSentinels_Defined(t *testing.T) {
	require.NotNil(t, ErrNotFound)
	require.NotNil(t, ErrUnauthorized)
	require.NotNil(t, ErrForbidden)
	require.NotNil(t, ErrInvalidInput)
	require.NotNil(t, ErrConflict)
	require.NotNil(t, ErrInternal)

	assert.Equal(t, "not_found", ErrNotFound.Code)
	assert.Equal(t, "unauthorized", ErrUnauthorized.Code)
	assert.Equal(t, "forbidden", ErrForbidden.Code)
	assert.Equal(t, "invalid_input", ErrInvalidInput.Code)
	assert.Equal(t, "conflict", ErrConflict.Code)
	assert.Equal(t, "internal", ErrInternal.Code)
}

func TestWrapError_OverridesMessage(t *testing.T) {
	cause := errors.New("low-level")
	wrapped := WrapError(ErrNotFound, cause, "custom message")
	assert.Equal(t, "not_found", wrapped.Code)
	assert.Equal(t, "custom message", wrapped.Message)
	assert.Equal(t, cause, wrapped.Cause)
}

func TestWrapError_PreservesBaseMessage(t *testing.T) {
	cause := errors.New("low-level")
	wrapped := WrapError(ErrInternal, cause, "")
	assert.Equal(t, "internal", wrapped.Code)
	assert.Equal(t, ErrInternal.Message, wrapped.Message)
}

func TestWrapError_NilBase_UsesErrInternal(t *testing.T) {
	wrapped := WrapError(nil, errors.New("x"), "msg")
	assert.Equal(t, ErrInternal.Code, wrapped.Code)
}

func TestError_ErrorsAs(t *testing.T) {
	e := WrapError(ErrNotFound, nil, "item missing")
	var target *Error
	require.True(t, errors.As(e, &target))
	assert.Equal(t, "not_found", target.Code)
}
