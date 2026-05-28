package abyss

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
