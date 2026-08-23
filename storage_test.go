package abyss

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
