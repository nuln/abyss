package abyss

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
