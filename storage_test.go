package abyss

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
