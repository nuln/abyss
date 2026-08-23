package abyss

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
