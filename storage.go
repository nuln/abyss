package abyss

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/disintegration/imaging"
)

// EntryType distinguishes files from directories.
type EntryType string

const (
	EntryFile EntryType = "file"
	EntryDir  EntryType = "dir"
)

// FileMedia holds media-related metadata for a file.
type FileMedia struct {
	MIME      string `json:"mime"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  int64  `json:"duration,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

// File is the core file entity.
type File struct {
	ID         uint64    `json:"id"`
	UserID     uint64    `json:"userId"`
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Type       EntryType `json:"type"`
	Size       int64     `json:"size"`
	Media      FileMedia `json:"media,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
	ModifiedAt time.Time `json:"modifiedAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (f *File) ToFrontend() map[string]any {
	if f == nil {
		return nil
	}
	res := map[string]any{
		"id":        f.ID,
		"ownerId":   f.UserID,
		"path":      f.Path,
		"name":      f.Name,
		"size":      f.Size,
		"isDir":     f.Type == EntryDir,
		"modified":  f.ModifiedAt.Format(time.RFC3339),
		"extension": path.Ext(f.Name),
		"mode":      0,
		"isSymlink": false,
		"type":      "blob",
	}
	if f.Type == EntryDir {
		res["type"] = "dir"
	} else if f.Media.MIME != "" {
		mime := f.Media.MIME
		switch {
		case strings.HasPrefix(mime, "image/"):
			res["type"] = "image"
		case strings.HasPrefix(mime, "video/"):
			res["type"] = "video"
		case strings.HasPrefix(mime, "audio/"):
			res["type"] = "audio"
		case strings.HasPrefix(mime, "text/") || mime == "application/json" || mime == "application/javascript":
			res["type"] = "text"
		case mime == "application/pdf":
			res["type"] = "pdf"
		}
	}
	return res
}

// EntryInfo augments a File with its storage engine name.
type EntryInfo struct {
	File
	ID            uint64    `json:"id,omitempty"`
	OwnerID       uint64    `json:"ownerId,omitempty"`
	Name          string    `json:"name,omitempty"`
	Path          string    `json:"path,omitempty"`
	Size          int64     `json:"size,omitempty"`
	IsDir         bool      `json:"isDir,omitempty"`
	ModTime       time.Time `json:"modTime,omitempty"`
	Mode          uint32    `json:"mode,omitempty"`
	Type          EntryType `json:"type,omitempty"`
	Shared        bool      `json:"shared,omitempty"`
	ThumbnailHash string    `json:"thumbnailHash,omitempty"`
	Engine        string    `json:"engine"`
}

// ReadSeekCloser combines io.Reader, io.Seeker, and io.Closer.
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// WriteCloser combines io.Writer and io.Closer.
type WriteCloser interface {
	io.Writer
	io.Closer
}

// WriteSeekCloser combines io.Writer, io.Seeker, and io.Closer.
type WriteSeekCloser interface {
	io.Writer
	io.Seeker
	io.Closer
}

// MetadataWriter is an optional interface for storage engines that support writing custom metadata.
type MetadataWriter interface {
	GetWriteMetadata(w interface{}) ([]byte, bool)
}

// MetadataOpener is an optional interface for storage engines that support opening files with metadata.
type MetadataOpener interface {
	OpenWithMetadata(metadata []byte) (ReadSeekCloser, error)
}

// ImageSize holds image dimensions.
type ImageSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// FileInfo describes common file metadata used in plugin events and hooks.
type FileInfo struct {
	ID            uint64    `json:"id"`
	OwnerID       uint64    `json:"ownerId,omitempty"`
	UserID        uint64    `json:"userId"`
	Path          string    `json:"path"`
	Name          string    `json:"name"`
	Size          int64     `json:"size"`
	MimeType      string    `json:"mimeType"`
	IsDir         bool      `json:"isDir"`
	ModTime       time.Time `json:"modTime,omitempty"`
	Mode          uint32    `json:"mode,omitempty"`
	Type          EntryType `json:"type,omitempty"`
	Shared        bool      `json:"shared,omitempty"`
	ThumbnailHash string    `json:"thumbnailHash,omitempty"`
	Listing       *Listing  `json:"listing,omitempty"`
}

// Listing holds a paged collection of shared file infos.
type Listing struct {
	Items    []*FileInfo `json:"items"`
	NumDirs  int         `json:"numDirs"`
	NumFiles int         `json:"numFiles"`
}

// FileStat describes stat information returned by storage engines.
type FileStat struct {
	ID            uint64
	OwnerID       uint64
	Path          string
	Name          string
	Size          int64
	IsDir         bool
	ModTime       time.Time
	Mode          uint32
	Type          EntryType
	Shared        bool
	ThumbnailHash string
}

func (s *FileStat) ToFileInfo() os.FileInfo {
	return &fileInfo{
		name:    path.Base(s.Path),
		size:    s.Size,
		mode:    0,
		modTime: s.ModTime,
		isDir:   s.IsDir,
	}
}

type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (f *fileInfo) Name() string       { return f.name }
func (f *fileInfo) Size() int64        { return f.size }
func (f *fileInfo) Mode() os.FileMode  { return f.mode }
func (f *fileInfo) ModTime() time.Time { return f.modTime }
func (f *fileInfo) IsDir() bool        { return f.isDir }
func (f *fileInfo) Sys() interface{}   { return nil }

func (f *File) ToFileInfo() os.FileInfo {
	return &fileInfo{
		name:    f.Name,
		size:    f.Size,
		mode:    0,
		modTime: f.ModifiedAt,
		isDir:   f.Type == EntryDir,
	}
}

func (e *EntryInfo) ToFileInfo() os.FileInfo {
	return e.File.ToFileInfo()
}

// StorageEngine is the abstraction for a user's backing storage.
type StorageEngine interface {
	Name() string
	Read(ctx context.Context, path string) (io.ReadCloser, error)
	Stat(ctx context.Context, path string) (*FileStat, error)
	Write(ctx context.Context, path string, r io.Reader) error
	Mkdir(ctx context.Context, path string) error
	Copy(ctx context.Context, src, dst string) error
	Move(ctx context.Context, oldPath, newPath string) error
	Delete(ctx context.Context, path string) error
	List(ctx context.Context, path string) ([]EntryInfo, error)
}

// FileStore defines file metadata persistence.
type FileStore interface {
	Save(ctx context.Context, file *File) error
	GetByID(ctx context.Context, id uint64) (*File, error)
	GetByUserPath(ctx context.Context, userID uint64, path string) (*File, error)
	ListByUser(ctx context.Context, userID uint64) ([]*File, error)
	ListByUserAndParent(ctx context.Context, userID uint64, parent string) ([]*File, error)
	Delete(ctx context.Context, id uint64) error
}

// ── pathEngine ────────────────────────────────────────────────────────────────

type pathEngine struct {
	baseDir string
}

var renameFile = os.Rename

func newPathEngine(baseDir string) *pathEngine {
	return &pathEngine{baseDir: baseDir}
}

func (e *pathEngine) Name() string { return "path" }

func (e *pathEngine) Read(ctx context.Context, p string) (io.ReadCloser, error) {
	_ = ctx
	return os.Open(e.abs(p))
}

func (e *pathEngine) Stat(ctx context.Context, p string) (*FileStat, error) {
	_ = ctx
	info, err := os.Stat(e.abs(p))
	if err != nil {
		return nil, err
	}
	return &FileStat{Path: p, Size: info.Size(), IsDir: info.IsDir(), ModTime: info.ModTime()}, nil
}

func (e *pathEngine) Write(ctx context.Context, p string, r io.Reader) error {
	_ = ctx
	abs := e.abs(p)
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return err
	}
	abs = filepath.Clean(abs) //nolint:gosec // abs is produced by e.abs and constrained under baseDir.
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".tmp-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (e *pathEngine) Mkdir(ctx context.Context, p string) error {
	_ = ctx
	return os.MkdirAll(e.abs(p), 0o750)
}

func (e *pathEngine) Copy(ctx context.Context, src, dst string) error {
	_ = ctx
	srcAbs := e.abs(src)
	dstAbs := e.abs(dst)
	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o750); err != nil {
		return err
	}
	s, err := os.Open(srcAbs) //nolint:gosec // srcAbs is normalized by e.abs under baseDir.
	if err != nil {
		return err
	}
	defer func() {
		_ = s.Close()
	}()
	d, err := os.Create(dstAbs) //nolint:gosec // dstAbs is normalized by e.abs under baseDir.
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		_ = d.Close()
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

func (e *pathEngine) Move(ctx context.Context, oldPath, newPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	oldAbs := e.abs(oldPath)
	newAbs := e.abs(newPath)
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o750); err != nil {
		return err
	}
	if err := renameFile(oldAbs, newAbs); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	s, err := os.Open(oldAbs) //nolint:gosec // oldAbs is normalized by e.abs under baseDir.
	if err != nil {
		return err
	}
	defer func() {
		_ = s.Close()
	}()

	d, err := os.Create(newAbs) //nolint:gosec // newAbs is normalized by e.abs under baseDir.
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		_ = d.Close()
		_ = os.Remove(newAbs)
		return err
	}
	if err := d.Close(); err != nil {
		_ = os.Remove(newAbs)
		return err
	}
	return os.Remove(oldAbs)
}

func (e *pathEngine) Delete(ctx context.Context, p string) error {
	_ = ctx
	return os.Remove(e.abs(p))
}

func (e *pathEngine) List(ctx context.Context, p string) ([]EntryInfo, error) {
	_ = ctx
	entries, err := os.ReadDir(e.abs(p))
	if err != nil {
		return nil, err
	}
	out := make([]EntryInfo, 0, len(entries))
	for _, item := range entries {
		t := EntryFile
		if item.IsDir() {
			t = EntryDir
		}
		out = append(out, EntryInfo{
			File:   File{Name: item.Name(), Type: t, Path: item.Name(), Size: 0, ModifiedAt: time.Time{}},
			Name:   item.Name(),
			Path:   item.Name(),
			IsDir:  item.IsDir(),
			Engine: e.Name(),
		})
	}
	return out, nil
}

func (e *pathEngine) abs(p string) string {
	clean := strings.TrimPrefix(filepath.Clean("/"+p), "/")
	return filepath.Join(e.baseDir, clean)
}

// ── virtualEngine ─────────────────────────────────────────────────────────────

type virtualEngine struct{}

func newVirtualEngine() *virtualEngine { return &virtualEngine{} }

func (e *virtualEngine) Name() string { return "virtual" }

func (e *virtualEngine) Read(ctx context.Context, p string) (io.ReadCloser, error) {
	_, _ = ctx, p
	return nil, errors.New("virtual engine does not persist files")
}

func (e *virtualEngine) Stat(ctx context.Context, p string) (*FileStat, error) {
	_, _ = ctx, p
	return nil, ErrNotFound
}

func (e *virtualEngine) Write(ctx context.Context, p string, r io.Reader) error {
	_, _, _ = ctx, p, r
	return nil
}

func (e *virtualEngine) Mkdir(ctx context.Context, p string) error {
	_, _ = ctx, p
	return nil
}

func (e *virtualEngine) Copy(ctx context.Context, src, dst string) error {
	_, _, _ = ctx, src, dst
	return nil
}

func (e *virtualEngine) Move(ctx context.Context, oldPath, newPath string) error {
	_, _, _ = ctx, oldPath, newPath
	return nil
}

func (e *virtualEngine) Delete(ctx context.Context, p string) error {
	_, _ = ctx, p
	return nil
}

func (e *virtualEngine) List(ctx context.Context, p string) ([]EntryInfo, error) {
	_, _ = ctx, p
	return []EntryInfo{}, nil
}

// ── storageService ────────────────────────────────────────────────────────────

type storageService struct {
	store       FileStore
	userStore   UserStore
	settingsSvc *settingsService
	uuidCache   map[uint64]string
	engines     map[uint64]StorageEngine
	dataDir     string
	mu          sync.RWMutex
}

const maxStorageCacheEntries = 1024

// ConsistencyRepairReport captures scan/fix summary for storage metadata consistency checks.
type ConsistencyRepairReport struct {
	ScannedMeta int           `json:"scannedMeta"`
	ScannedFS   int           `json:"scannedFS"`
	OrphanMeta  int           `json:"orphanMeta"`
	OrphanFile  int           `json:"orphanFile"`
	FixedCount  int           `json:"fixedCount"`
	FailedCount int           `json:"failedCount"`
	Duration    time.Duration `json:"duration"`
}

type sniffingCounterReader struct {
	r        io.Reader
	sniffBuf [512]byte
	sniffLen int
	n        int64
}

func newSniffingCounterReader(r io.Reader) *sniffingCounterReader {
	return &sniffingCounterReader{r: r}
}

func (s *sniffingCounterReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.n += int64(n)
		if s.sniffLen < len(s.sniffBuf) {
			copied := copy(s.sniffBuf[s.sniffLen:], p[:n])
			s.sniffLen += copied
		}
	}
	return n, err
}

func (s *sniffingCounterReader) SniffData() []byte {
	return s.sniffBuf[:s.sniffLen]
}

func (s *sniffingCounterReader) Size() int64 {
	return s.n
}

func newStorageService(store FileStore, userStore UserStore, settingsSvc *settingsService, dataDir string) *storageService {
	return &storageService{
		store:       store,
		userStore:   userStore,
		settingsSvc: settingsSvc,
		uuidCache:   make(map[uint64]string),
		engines:     make(map[uint64]StorageEngine),
		dataDir:     dataDir,
	}
}

func (s *storageService) SetEngine(userID uint64, engine StorageEngine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enforceStorageCacheLimit(s.engines)
	s.engines[userID] = engine
}

func (s *storageService) GetEngine(userID uint64) (StorageEngine, error) {
	s.mu.RLock()
	engine, ok := s.engines[userID]
	s.mu.RUnlock()
	if ok {
		return engine, nil
	}

	if s.dataDir == "" {
		return nil, fmt.Errorf("no engine configured for user %d", userID)
	}

	// Determine which storage type to use outside lock
	storageType := "path"
	if s.settingsSvc != nil {
		if settings, err := s.settingsSvc.Get(context.Background()); err == nil && settings != nil {
			storageType = settings.StorageType
		}
	}

	// If storage type is "path", we skip plugins and go straight to local path engine
	if storageType != "path" {
		// Try to get engine from plugins first
		var pluginEngine StorageEngine
		_ = CallStorageProvider(func(p StorageProvider) error {
			// Check if this provider supports the requested storage type
			supported := false
			for _, t := range p.AvailableStorageTypes() {
				if t.Name == storageType {
					supported = true
					break
				}
			}
			if !supported {
				return nil
			}

			e, err := p.CreateUserEngine(userID)
			if err != nil {
				return err
			}
			if e != nil {
				pluginEngine = e
				return ErrStop
			}
			return nil
		})

		if pluginEngine != nil {
			s.mu.Lock()
			enforceStorageCacheLimit(s.engines)
			s.engines[userID] = pluginEngine
			s.mu.Unlock()
			return pluginEngine, nil
		}
	}

	// Fallback to local path engine
	if s.dataDir == "" {
		return nil, fmt.Errorf("no data directory configured for path storage")
	}

	// Get UUID from cache or store
	s.mu.RLock()
	uuidStr, ok := s.uuidCache[userID]
	s.mu.RUnlock()

	if !ok {
		u, err := s.userStore.GetByID(context.Background(), userID)
		if err != nil {
			return nil, fmt.Errorf("lookup user %d: %w", userID, err)
		}
		if u.UUID == "" {
			return nil, fmt.Errorf("user %d has no uuid", userID)
		}
		uuidStr = u.UUID
		s.mu.Lock()
		enforceStorageCacheLimit(s.uuidCache)
		s.uuidCache[userID] = uuidStr
		s.mu.Unlock()
	}

	dir := path.Join(s.dataDir, "files", uuidStr)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create user dir: %w", err)
	}

	engine = newPathEngine(dir)

	s.mu.Lock()
	if existing, ok := s.engines[userID]; ok {
		s.mu.Unlock()
		return existing, nil
	}
	enforceStorageCacheLimit(s.engines)
	s.engines[userID] = engine
	s.mu.Unlock()

	return engine, nil
}

func (s *storageService) RemoveUserCache(userID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.engines, userID)
	delete(s.uuidCache, userID)
}

func enforceStorageCacheLimit[V any](m map[uint64]V) {
	if len(m) < maxStorageCacheEntries {
		return
	}
	// Evict the numerically oldest key as a cheap stand-in for LRU: user
	// ids are roughly allocation-ordered, so this approximates evicting
	// the least-recently-created entry without extra bookkeeping.
	var oldest uint64
	first := true
	for key := range m {
		if first || key < oldest {
			oldest = key
			first = false
		}
	}
	delete(m, oldest)
}

func (s *storageService) WriteFile(ctx context.Context, userID uint64, filePath string, r io.Reader) (*File, error) {
	engine, err := s.GetEngine(userID)
	if err != nil {
		return nil, err
	}
	// Resolve the existing record (if any) BEFORE writing so an overwrite
	// updates metadata in place instead of colliding after the disk write.
	existing, lookupErr := s.store.GetByUserPath(ctx, userID, filePath)
	if lookupErr != nil && !errors.Is(lookupErr, ErrNotFound) {
		return nil, lookupErr
	}
	stream := newSniffingCounterReader(r)
	if err := engine.Write(ctx, filePath, stream); err != nil {
		return nil, err
	}
	sniffData := stream.SniffData()
	meta := &File{
		UserID: userID,
		Path:   filePath,
		Name:   path.Base(filePath),
		Type:   EntryFile,
		Size:   stream.Size(),
		Media: FileMedia{
			MIME: detectMIME(filePath, sniffData),
		},
	}
	if existing != nil {
		// Overwrite: keep identity and creation time, refresh content fields.
		meta.ID = existing.ID
		meta.CreatedAt = existing.CreatedAt
		meta.Checksum = existing.Checksum
	}
	meta.ModifiedAt = time.Now().UTC()
	if err := s.store.Save(ctx, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *storageService) CreateDir(ctx context.Context, userID uint64, filePath string) (*File, error) {
	engine, err := s.GetEngine(userID)
	if err != nil {
		return nil, err
	}
	if err := engine.Mkdir(ctx, filePath); err != nil {
		return nil, err
	}
	meta := &File{
		UserID: userID,
		Path:   filePath,
		Name:   path.Base(filePath),
		Type:   EntryDir,
	}
	if err := s.store.Save(ctx, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *storageService) GetFileByID(ctx context.Context, id uint64) (*EntryInfo, error) {
	f, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &EntryInfo{
		File:    *f,
		ID:      f.ID,
		OwnerID: f.UserID,
		Name:    f.Name,
		Path:    f.Path,
		Size:    f.Size,
		IsDir:   f.Type == EntryDir,
		ModTime: f.ModifiedAt,
		Type:    f.Type,
		Engine:  "path",
	}, nil
}

func (s *storageService) GetFileByPath(ctx context.Context, userID uint64, filePath string) (*File, error) {
	return s.store.GetByUserPath(ctx, userID, normalizePath(filePath))
}

func (s *storageService) ListByPath(ctx context.Context, userID uint64, dirPath string) ([]*EntryInfo, error) {
	dir := normalizePath(dirPath)

	directFiles, err := s.store.ListByUserAndParent(ctx, userID, dir)
	if err != nil {
		return nil, err
	}
	out := make([]*EntryInfo, 0, len(directFiles))

	for _, f := range directFiles {
		out = append(out, &EntryInfo{
			File:    *f,
			ID:      f.ID,
			OwnerID: f.UserID,
			Name:    f.Name,
			Path:    f.Path,
			Size:    f.Size,
			IsDir:   f.Type == EntryDir,
			ModTime: f.ModifiedAt,
			Type:    f.Type,
			Engine:  "path",
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *storageService) DeleteFile(ctx context.Context, userID, id uint64) error {
	f, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if f.UserID != userID {
		return ErrForbidden
	}
	engine, err := s.GetEngine(userID)
	if err != nil {
		return err
	}
	if err := engine.Delete(ctx, f.Path); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

func (s *storageService) DeleteFileRecord(ctx context.Context, userID, id uint64) error {
	f, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if f.UserID != userID {
		return ErrForbidden
	}
	return s.store.Delete(ctx, id)
}

func (s *storageService) CopyFile(ctx context.Context, userID, id uint64, newPath string) (*File, error) {
	f, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.UserID != userID {
		return nil, ErrForbidden
	}
	engine, err := s.GetEngine(userID)
	if err != nil {
		return nil, err
	}
	np := normalizePath(newPath)
	if err := engine.Copy(ctx, f.Path, np); err != nil {
		return nil, err
	}
	newFile := *f
	newFile.ID = 0 // Will be set by Save
	newFile.Path = np
	newFile.Name = path.Base(np)
	if err := s.store.Save(ctx, &newFile); err != nil {
		return nil, err
	}
	return &newFile, nil
}

func (s *storageService) RenameFile(ctx context.Context, userID, id uint64, newPath string) (*File, error) {
	f, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.UserID != userID {
		return nil, ErrForbidden
	}
	engine, err := s.GetEngine(userID)
	if err != nil {
		return nil, err
	}
	np := normalizePath(newPath)
	// Reject moves onto an existing target path before touching the disk.
	if other, lookupErr := s.store.GetByUserPath(ctx, userID, np); lookupErr == nil && other.ID != f.ID {
		return nil, ErrConflict
	}
	isDir := f.Type == EntryDir
	oldPath := f.Path
	if err := engine.Move(ctx, f.Path, np); err != nil {
		return nil, err
	}
	f.Path = np
	f.Name = path.Base(f.Path)
	if err := s.store.Save(ctx, f); err != nil {
		return nil, err
	}
	// Cascade the rename to every descendant record so metadata stays in
	// sync with the physical tree after moving a directory.
	if isDir && oldPath != "/" {
		prefix := strings.TrimSuffix(oldPath, "/") + "/"
		newPrefix := strings.TrimSuffix(np, "/") + "/"
		all, listErr := s.store.ListByUser(ctx, userID)
		if listErr == nil {
			for _, child := range all {
				if child.ID == f.ID || !strings.HasPrefix(child.Path, prefix) {
					continue
				}
				child.Path = newPrefix + strings.TrimPrefix(child.Path, prefix)
				child.Name = path.Base(child.Path)
				if err := s.store.Save(ctx, child); err != nil {
					slog.Error("cascade rename", "path", child.Path, "error", err)
				}
			}
		} else {
			slog.Error("list for cascade rename", "error", listErr)
		}
	}
	return f, nil
}

func (s *storageService) OpenFile(ctx context.Context, userID, id uint64) (io.ReadCloser, *File, error) {
	f, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if f.UserID != userID {
		return nil, nil, ErrForbidden
	}
	engine, err := s.GetEngine(userID)
	if err != nil {
		return nil, nil, err
	}
	rc, err := engine.Read(ctx, f.Path)
	if err != nil {
		return nil, nil, err
	}
	return rc, f, nil
}

func (s *storageService) RenderPreview(ctx context.Context, userID, id uint64, width, height int) (preview []byte, mime string, err error) {
	rc, f, err := s.OpenFile(ctx, userID, id)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rc.Close() }()
	const maxPreviewSourceSize int64 = 20 << 20
	if f != nil && f.Size > maxPreviewSourceSize {
		return nil, "", WrapError(ErrInvalidInput, nil, "file too large for preview")
	}
	if width <= 0 {
		width = 320
	}
	if height <= 0 {
		height = 320
	}
	buf := new(bytes.Buffer)
	if err := resizeToFit(rc, width, height, buf); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

func (s *storageService) RepairConsistency(ctx context.Context, userID uint64, autoFix bool, maxOps int) (*ConsistencyRepairReport, error) {
	start := time.Now()
	report := &ConsistencyRepairReport{}
	engine, err := s.GetEngine(userID)
	if err != nil {
		return nil, err
	}

	metaFiles, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	report.ScannedMeta = len(metaFiles)

	metaByPath := make(map[string]*File, len(metaFiles))
	budget := newRepairBudget(autoFix, maxOps)

	if reconcileErr := s.reconcileOrphanMetadata(ctx, engine, metaFiles, metaByPath, budget, report); reconcileErr != nil {
		return nil, reconcileErr
	}
	err = s.rebuildMissingMetadata(ctx, engine, userID, metaByPath, budget, report)
	if err != nil {
		return nil, err
	}

	report.Duration = time.Since(start)
	return report, nil
}

type repairBudget struct {
	autoFix bool
	maxOps  int
	ops     int
}

func newRepairBudget(autoFix bool, maxOps int) *repairBudget {
	return &repairBudget{autoFix: autoFix, maxOps: maxOps}
}

func (b *repairBudget) canFix() bool {
	if !b.autoFix {
		return false
	}
	if b.maxOps <= 0 {
		return true
	}
	return b.ops < b.maxOps
}

func (b *repairBudget) consume() {
	b.ops++
}

func (s *storageService) reconcileOrphanMetadata(ctx context.Context, engine StorageEngine, metaFiles []*File, metaByPath map[string]*File, budget *repairBudget, report *ConsistencyRepairReport) error {
	for _, f := range metaFiles {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		norm := normalizePath(f.Path)
		metaByPath[norm] = f
		exists, err := storagePathExists(ctx, engine, f)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		report.OrphanMeta++
		if !budget.canFix() {
			continue
		}
		budget.consume()
		if err := s.store.Delete(ctx, f.ID); err != nil {
			report.FailedCount++
			continue
		}
		report.FixedCount++
	}
	return nil
}

func (s *storageService) rebuildMissingMetadata(ctx context.Context, engine StorageEngine, userID uint64, metaByPath map[string]*File, budget *repairBudget, report *ConsistencyRepairReport) error {
	return storageWalkFiles(ctx, engine, "/", func(fullPath string) error {
		report.ScannedFS++
		norm := normalizePath(fullPath)
		if _, ok := metaByPath[norm]; ok {
			return nil
		}
		report.OrphanFile++
		if !budget.canFix() {
			return nil
		}
		budget.consume()
		return s.restoreMetadataFromFile(ctx, engine, userID, norm, report)
	})
}

func (s *storageService) restoreMetadataFromFile(ctx context.Context, engine StorageEngine, userID uint64, filePath string, report *ConsistencyRepairReport) error {
	st, err := engine.Stat(ctx, filePath)
	if err != nil {
		report.FailedCount++
		return nil
	}
	sniffData, err := readSniffData(ctx, engine, filePath)
	if err != nil {
		report.FailedCount++
		return nil
	}
	meta := &File{
		UserID: userID,
		Path:   filePath,
		Name:   path.Base(filePath),
		Type:   EntryFile,
		Size:   st.Size,
		Media: FileMedia{
			MIME: detectMIME(filePath, sniffData),
		},
	}
	if err := s.store.Save(ctx, meta); err != nil {
		report.FailedCount++
		return nil
	}
	report.FixedCount++
	return nil
}

func readSniffData(ctx context.Context, engine StorageEngine, filePath string) ([]byte, error) {
	rc, err := engine.Read(ctx, filePath)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 512)
	n, readErr := io.ReadFull(rc, buf)
	closeErr := rc.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		if closeErr != nil {
			return nil, closeErr
		}
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return buf[:n], nil
}

func storagePathExists(ctx context.Context, engine StorageEngine, f *File) (bool, error) {
	if f == nil {
		return false, nil
	}
	if f.Type == EntryDir {
		_, err := engine.List(ctx, f.Path)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	rc, err := engine.Read(ctx, f.Path)
	if err == nil {
		_ = rc.Close()
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

// WalkFunc is the type of the function called for each file or directory
// visited by Walk.
type WalkFunc func(path string, info *FileInfo, err error) error

// Walk traverses the storage engine starting from the given root.
func Walk(ctx context.Context, engine StorageEngine, root string, fn WalkFunc) error {
	st, err := engine.Stat(ctx, root)
	if err != nil {
		return fn(root, nil, err)
	}
	info := &FileInfo{
		Path:    st.Path,
		Name:    st.Name,
		Size:    st.Size,
		IsDir:   st.IsDir,
		ModTime: st.ModTime,
	}
	return walk(ctx, engine, root, info, fn)
}

func walk(ctx context.Context, engine StorageEngine, p string, info *FileInfo, fn WalkFunc) error {
	if err := fn(p, info, nil); err != nil {
		if info.IsDir && errors.Is(err, filepath.SkipDir) {
			return nil
		}
		return err
	}

	if !info.IsDir {
		return nil
	}

	list, err := engine.List(ctx, p)
	if err != nil {
		return fn(p, info, err)
	}

	for i := range list {
		ei := &list[i]
		fi := &FileInfo{
			Path:    path.Join(p, ei.Name),
			Name:    ei.Name,
			Size:    ei.Size,
			IsDir:   ei.IsDir,
			ModTime: ei.ModTime,
		}
		if err := walk(ctx, engine, fi.Path, fi, fn); err != nil {
			if errors.Is(err, filepath.SkipDir) {
				continue
			}
			return err
		}
	}
	return nil
}

func storageWalkFiles(ctx context.Context, engine StorageEngine, dir string, fn func(fullPath string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	items, err := engine.List(ctx, dir)
	if err != nil {
		return err
	}
	for i := range items {
		item := items[i]
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.Name == "" {
			continue
		}
		fullPath := normalizePath(path.Join(dir, item.Name))
		if item.Type == EntryDir {
			if err := storageWalkFiles(ctx, engine, fullPath, fn); err != nil {
				return err
			}
			continue
		}
		if err := fn(fullPath); err != nil {
			return err
		}
	}
	return nil
}

func normalizePath(p string) string {
	clean := path.Clean("/" + p)
	if clean == "." {
		return "/"
	}
	return clean
}

// HashPath returns a sharded path for a given hash (e.g. "abc..." -> "a/b/c/abc...").
func HashPath(hash string) string {
	if len(hash) < 3 {
		return hash
	}
	return filepath.Join(hash[0:1], hash[1:2], hash[2:3], hash)
}

// ── MIME ─────────────────────────────────────────────────────────────────────

func detectMIME(filename string, data []byte) string {
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "application/octet-stream"
	}
	if t := mimeByExt(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func mimeByExt(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	case ".pdf":
		return "application/pdf"
	default:
		return ""
	}
}

// ── Image ────────────────────────────────────────────────────────────────────

func decodeImage(r io.Reader) (image.Image, error) {
	img, err := imaging.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func resizeToFit(in io.Reader, width, height int, out io.Writer) error {
	img, err := decodeImage(in)
	if err != nil {
		return err
	}
	resized := imaging.Fit(img, width, height, imaging.Lanczos)
	if err := imaging.Encode(out, resized, imaging.JPEG); err != nil {
		return fmt.Errorf("encode image: %w", err)
	}
	return nil
}
