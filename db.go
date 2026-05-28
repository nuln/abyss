package abyss

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	bbolt "go.etcd.io/bbolt"
)

// ── boltDB ────────────────────────────────────────────────────────────────────

type boltDB struct {
	inner *bbolt.DB
}

func (db *boltDB) Raw() *bbolt.DB {
	if db == nil {
		return nil
	}
	return db.inner
}

type boltConfig struct {
	Path    string
	Timeout time.Duration
	Mode    os.FileMode
	NoSync  bool
}

func openBoltDB(cfg boltConfig) (*boltDB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("bolt path is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.Mode == 0 {
		cfg.Mode = 0o600
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o750); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	opts := &bbolt.Options{Timeout: cfg.Timeout, NoSync: cfg.NoSync}
	raw, err := bbolt.Open(cfg.Path, cfg.Mode, opts)
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}
	return &boltDB{inner: raw}, nil
}

func (d *boltDB) Close() error {
	if d == nil || d.inner == nil {
		return nil
	}
	return d.inner.Close()
}

func boltView(db *boltDB, fn func(tx *bbolt.Tx) error) error {
	return db.inner.View(fn)
}

func boltUpdate(db *boltDB, fn func(tx *bbolt.Tx) error) error {
	return db.inner.Update(fn)
}

func boltMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func boltUnmarshal(data []byte, out any) error {
	return json.Unmarshal(data, out)
}

func boltNextID(bucket *bbolt.Bucket) (uint64, error) {
	id, err := bucket.NextSequence()
	if err != nil {
		return 0, fmt.Errorf("next sequence: %w", err)
	}
	return id, nil
}

func boltUint64Key(v uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return buf
}

// boltEnsureSchema creates the top-level schema buckets.
func boltEnsureSchema(db *boltDB, buckets ...[]byte) error {
	return boltUpdate(db, func(tx *bbolt.Tx) error {
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("create bucket %s: %w", b, err)
			}
		}
		return nil
	})
}

type boltSessionStore struct {
	db *boltDB
}

func (r *boltSessionStore) GetByID(ctx context.Context, id string) (*RefreshToken, error) {
	_ = ctx
	var out *RefreshToken
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(sessionsBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get([]byte(id))
		if v == nil {
			return ErrNotFound
		}
		t := new(RefreshToken)
		if err := boltUnmarshal(v, t); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

// ── boltUserStore ─────────────────────────────────────────────────────────────

var (
	usersBucket      = []byte("identity_users")
	usersEmailIdx    = []byte("identity_users_by_email")
	usersUsernameIdx = []byte("identity_users_by_username")
)

type boltUserStore struct {
	db *boltDB
}

func (r *boltUserStore) Create(ctx context.Context, user *User) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(usersBucket)
		if err != nil {
			return fmt.Errorf("create users bucket: %w", err)
		}
		emailIdx, err := tx.CreateBucketIfNotExists(usersEmailIdx)
		if err != nil {
			return fmt.Errorf("create email index: %w", err)
		}
		usernameIdx, err := tx.CreateBucketIfNotExists(usersUsernameIdx)
		if err != nil {
			return fmt.Errorf("create username index: %w", err)
		}

		emailKey := []byte(strings.ToLower(strings.TrimSpace(user.Email)))
		if emailIdx.Get(emailKey) != nil {
			return ErrConflict
		}
		usernameKey := []byte(strings.ToLower(strings.TrimSpace(user.Username)))
		if len(usernameKey) > 0 && usernameIdx.Get(usernameKey) != nil {
			return ErrConflict
		}

		id, err := boltNextID(b)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		user.ID = id
		user.Email = string(emailKey)
		if len(usernameKey) > 0 {
			user.Username = string(usernameKey)
		}
		user.CreatedAt = now
		user.UpdatedAt = now

		encoded, err := boltMarshal(user)
		if err != nil {
			return err
		}
		pk := boltUint64Key(id)
		if err := b.Put(pk, encoded); err != nil {
			return err
		}
		if err := emailIdx.Put(emailKey, pk); err != nil {
			return err
		}
		if len(usernameKey) > 0 {
			if err := usernameIdx.Put(usernameKey, pk); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *boltUserStore) GetByID(ctx context.Context, id uint64) (*User, error) {
	_ = ctx
	var out *User
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(usersBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get(boltUint64Key(id))
		if v == nil {
			return ErrNotFound
		}
		u := new(User)
		if err := boltUnmarshal(v, u); err != nil {
			return err
		}
		out = u
		return nil
	})
	return out, err
}

func (r *boltUserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	_ = ctx
	emailKey := []byte(strings.ToLower(strings.TrimSpace(email)))
	return r.lookupByIndex(usersEmailIdx, emailKey)
}

func (r *boltUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	_ = ctx
	key := []byte(strings.ToLower(strings.TrimSpace(username)))
	return r.lookupByIndex(usersUsernameIdx, key)
}

func (r *boltUserStore) lookupByIndex(idxBucket, indexKey []byte) (*User, error) {
	var out *User
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		idx := tx.Bucket(idxBucket)
		if idx == nil {
			return ErrNotFound
		}
		pk := idx.Get(indexKey)
		if pk == nil {
			return ErrNotFound
		}
		b := tx.Bucket(usersBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get(pk)
		if v == nil {
			return ErrNotFound
		}
		u := new(User)
		if err := boltUnmarshal(v, u); err != nil {
			return err
		}
		out = u
		return nil
	})
	return out, err
}

func (r *boltUserStore) Update(ctx context.Context, user *User) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(usersBucket)
		if b == nil {
			return ErrNotFound
		}
		pk := boltUint64Key(user.ID)
		existing := b.Get(pk)
		if existing == nil {
			return ErrNotFound
		}
		old := new(User)
		if err := boltUnmarshal(existing, old); err != nil {
			return err
		}
		emailIdx, _ := tx.CreateBucketIfNotExists(usersEmailIdx)
		usernameIdx, _ := tx.CreateBucketIfNotExists(usersUsernameIdx)

		newEmail := strings.ToLower(strings.TrimSpace(user.Email))
		oldEmail := strings.ToLower(strings.TrimSpace(old.Email))
		if newEmail != oldEmail {
			if existingPK := emailIdx.Get([]byte(newEmail)); existingPK != nil && binary.BigEndian.Uint64(existingPK) != user.ID {
				return ErrConflict
			}
			_ = emailIdx.Delete([]byte(oldEmail))
			if err := emailIdx.Put([]byte(newEmail), pk); err != nil {
				return err
			}
		}
		newUsername := strings.ToLower(strings.TrimSpace(user.Username))
		oldUsername := strings.ToLower(strings.TrimSpace(old.Username))
		if newUsername != oldUsername {
			if newUsername != "" {
				if existingPK := usernameIdx.Get([]byte(newUsername)); existingPK != nil && binary.BigEndian.Uint64(existingPK) != user.ID {
					return ErrConflict
				}
			}
			if oldUsername != "" {
				_ = usernameIdx.Delete([]byte(oldUsername))
			}
			if newUsername != "" {
				if err := usernameIdx.Put([]byte(newUsername), pk); err != nil {
					return err
				}
			}
		}

		user.UpdatedAt = time.Now().UTC()
		encoded, err := boltMarshal(user)
		if err != nil {
			return err
		}
		return b.Put(pk, encoded)
	})
}

func (r *boltUserStore) Delete(ctx context.Context, id uint64) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(usersBucket)
		if b == nil {
			return nil
		}
		pk := boltUint64Key(id)
		v := b.Get(pk)
		if v != nil {
			u := new(User)
			if err := boltUnmarshal(v, u); err == nil {
				if emailIdx := tx.Bucket(usersEmailIdx); emailIdx != nil {
					_ = emailIdx.Delete([]byte(strings.ToLower(u.Email)))
				}
				if usernameIdx := tx.Bucket(usersUsernameIdx); usernameIdx != nil {
					_ = usernameIdx.Delete([]byte(strings.ToLower(u.Username)))
				}
			}
		}
		return b.Delete(pk)
	})
}

func (r *boltUserStore) List(ctx context.Context) ([]*User, error) {
	_ = ctx
	var out []*User
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(usersBucket)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			u := new(User)
			if err := boltUnmarshal(v, u); err != nil {
				return err
			}
			out = append(out, u)
			return nil
		})
	})
	return out, err
}

// ── boltSessionStore ──────────────────────────────────────────────────────────

var (
	sessionsBucket  = []byte("identity_sessions")
	sessionsHashIdx = []byte("identity_sessions_by_hash")
	sessionsUserIdx = []byte("identity_sessions_by_user")
)

func (r *boltSessionStore) Save(ctx context.Context, token *RefreshToken) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(sessionsBucket)
		if err != nil {
			return err
		}
		hashIdx, err := tx.CreateBucketIfNotExists(sessionsHashIdx)
		if err != nil {
			return err
		}
		userIdx, err := tx.CreateBucketIfNotExists(sessionsUserIdx)
		if err != nil {
			return err
		}
		if token.ID == "" {
			token.ID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
		}
		if token.Hash == "" {
			h := sha256.Sum256([]byte(token.ID))
			token.Hash = fmt.Sprintf("%x", h)
		}
		encoded, err := boltMarshal(token)
		if err != nil {
			return err
		}
		pk := []byte(token.ID)
		if err := b.Put(pk, encoded); err != nil {
			return err
		}
		if err := hashIdx.Put([]byte(token.Hash), pk); err != nil {
			return err
		}
		userKey := append(boltUint64Key(token.UserID), []byte("_"+token.ID)...)
		return userIdx.Put(userKey, pk)
	})
}

func (r *boltSessionStore) GetByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	_ = ctx
	var out *RefreshToken
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		hashIdx := tx.Bucket(sessionsHashIdx)
		if hashIdx == nil {
			return ErrNotFound
		}
		pk := hashIdx.Get([]byte(hash))
		if pk == nil {
			return ErrNotFound
		}
		b := tx.Bucket(sessionsBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get(pk)
		if v == nil {
			return ErrNotFound
		}
		t := new(RefreshToken)
		if err := boltUnmarshal(v, t); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

func (r *boltSessionStore) GetByUser(ctx context.Context, userID uint64) ([]*RefreshToken, error) {
	_ = ctx
	prefix := boltUint64Key(userID)
	var out []*RefreshToken
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		userIdx := tx.Bucket(sessionsUserIdx)
		if userIdx == nil {
			return nil
		}
		b := tx.Bucket(sessionsBucket)
		if b == nil {
			return nil
		}
		c := userIdx.Cursor()
		for k, pk := c.Seek(prefix); len(k) >= 8; k, pk = c.Next() {
			matched := true
			for i := 0; i < 8; i++ {
				if k[i] != prefix[i] {
					matched = false
					break
				}
			}
			if !matched {
				break
			}
			v := b.Get(pk)
			if v == nil {
				continue
			}
			t := new(RefreshToken)
			if err := boltUnmarshal(v, t); err != nil {
				continue
			}
			out = append(out, t)
		}
		return nil
	})
	return out, err
}

func (r *boltSessionStore) Revoke(ctx context.Context, id string) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(sessionsBucket)
		if b == nil {
			return nil
		}
		pk := []byte(id)
		v := b.Get(pk)
		if v != nil {
			t := new(RefreshToken)
			if err := boltUnmarshal(v, t); err == nil {
				if hashIdx := tx.Bucket(sessionsHashIdx); hashIdx != nil {
					_ = hashIdx.Delete([]byte(t.Hash))
				}
				if userIdx := tx.Bucket(sessionsUserIdx); userIdx != nil {
					userKey := append(boltUint64Key(t.UserID), []byte("_"+id)...)
					_ = userIdx.Delete(userKey)
				}
			}
		}
		return b.Delete(pk)
	})
}

func (r *boltSessionStore) RevokeAll(ctx context.Context, userID uint64) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(sessionsBucket)
		if b == nil {
			return nil
		}
		hashIdx := tx.Bucket(sessionsHashIdx)
		userIdx := tx.Bucket(sessionsUserIdx)
		if userIdx == nil {
			return nil
		}
		prefix := boltUint64Key(userID)
		keys := make([][]byte, 0)
		pks := make([][]byte, 0)
		c := userIdx.Cursor()
		for k, pk := c.Seek(prefix); len(k) >= 8; k, pk = c.Next() {
			matched := true
			for i := 0; i < 8; i++ {
				if k[i] != prefix[i] {
					matched = false
					break
				}
			}
			if !matched {
				break
			}
			keys = append(keys, append([]byte(nil), k...))
			pks = append(pks, append([]byte(nil), pk...))
		}
		for i := range pks {
			v := b.Get(pks[i])
			if v != nil {
				t := new(RefreshToken)
				if err := boltUnmarshal(v, t); err == nil && hashIdx != nil {
					_ = hashIdx.Delete([]byte(t.Hash))
				}
			}
			_ = b.Delete(pks[i])
			_ = userIdx.Delete(keys[i])
		}
		return nil
	})
}

// ── boltFileStore ─────────────────────────────────────────────────────────────

var (
	filesBucket          = []byte("storage_files")
	filesByUserPathIdx   = []byte("storage_files_by_user")
	filesByUserParentIdx = []byte("storage_files_by_user_parent")
)

type boltFileStore struct {
	db *boltDB
}

func (r *boltFileStore) Save(ctx context.Context, file *File) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(filesBucket)
		if err != nil {
			return err
		}
		idx, err := tx.CreateBucketIfNotExists(filesByUserPathIdx)
		if err != nil {
			return err
		}
		pidx, err := tx.CreateBucketIfNotExists(filesByUserParentIdx)
		if err != nil {
			return err
		}

		var old *File
		if file.ID != 0 {
			if v := b.Get(boltUint64Key(file.ID)); v != nil {
				old = new(File)
				if unmarshalErr := boltUnmarshal(v, old); unmarshalErr != nil {
					return unmarshalErr
				}
			}
		}

		if file.ID == 0 {
			id, nextIDErr := boltNextID(b)
			if nextIDErr != nil {
				return nextIDErr
			}
			file.ID = id
		}

		idxKey := fileUserPathKey(file.UserID, file.Path)
		if pk := idx.Get(idxKey); pk != nil {
			if len(pk) != 8 || binary.BigEndian.Uint64(pk) != file.ID {
				return ErrConflict
			}
		}

		if file.CreatedAt.IsZero() {
			file.CreatedAt = time.Now().UTC()
		}
		file.ModifiedAt = time.Now().UTC()
		enc, err := boltMarshal(file)
		if err != nil {
			return err
		}
		pk := boltUint64Key(file.ID)
		if err := b.Put(pk, enc); err != nil {
			return err
		}
		if old != nil {
			if oldKey := fileUserPathKey(old.UserID, old.Path); !bytesEqual(oldKey, idxKey) {
				_ = idx.Delete(oldKey)
			}
			if oldParent := fileParentPath(old.Path); oldParent != fileParentPath(file.Path) {
				_ = pidx.Delete(fileUserParentFileKey(old.UserID, oldParent, old.ID))
			}
		}
		if err := idx.Put(idxKey, pk); err != nil {
			return err
		}
		return pidx.Put(fileUserParentFileKey(file.UserID, fileParentPath(file.Path), file.ID), pk)
	})
}

func (r *boltFileStore) GetByID(ctx context.Context, id uint64) (*File, error) {
	_ = ctx
	var out *File
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(filesBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get(boltUint64Key(id))
		if v == nil {
			return ErrNotFound
		}
		f := new(File)
		if err := boltUnmarshal(v, f); err != nil {
			return err
		}
		out = f
		return nil
	})
	return out, err
}

func (r *boltFileStore) GetByUserPath(ctx context.Context, userID uint64, filePath string) (*File, error) {
	_ = ctx
	var out *File
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		idx := tx.Bucket(filesByUserPathIdx)
		if idx == nil {
			return ErrNotFound
		}
		pk := idx.Get(fileUserPathKey(userID, filePath))
		if pk == nil {
			return ErrNotFound
		}
		b := tx.Bucket(filesBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get(pk)
		if v == nil {
			return ErrNotFound
		}
		f := new(File)
		if err := boltUnmarshal(v, f); err != nil {
			return err
		}
		out = f
		return nil
	})
	return out, err
}

func (r *boltFileStore) ListByUser(ctx context.Context, userID uint64) ([]*File, error) {
	out := make([]*File, 0)
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		idx := tx.Bucket(filesByUserPathIdx)
		if idx == nil {
			return nil
		}
		b := tx.Bucket(filesBucket)
		if b == nil {
			return nil
		}
		prefix := boltUint64Key(userID)
		c := idx.Cursor()
		for k, pk := c.Seek(prefix); k != nil && hasPrefixBytes(k, prefix); k, pk = c.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			v := b.Get(pk)
			if v == nil {
				continue
			}
			f := new(File)
			if err := boltUnmarshal(v, f); err != nil {
				return fmt.Errorf("unmarshal file: %w", err)
			}
			out = append(out, f)
		}
		return nil
	})
	return out, err
}

func (r *boltFileStore) ListByUserAndParent(ctx context.Context, userID uint64, parentDir string) ([]*File, error) {
	out := make([]*File, 0)
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		pidx := tx.Bucket(filesByUserParentIdx)
		if pidx == nil {
			return nil
		}
		b := tx.Bucket(filesBucket)
		if b == nil {
			return nil
		}
		prefix := fileUserParentPrefix(userID, parentDir)
		c := pidx.Cursor()
		for k, pk := c.Seek(prefix); k != nil && hasPrefixBytes(k, prefix); k, pk = c.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			v := b.Get(pk)
			if v == nil {
				continue
			}
			f := new(File)
			if err := boltUnmarshal(v, f); err != nil {
				return fmt.Errorf("unmarshal file: %w", err)
			}
			out = append(out, f)
		}
		return nil
	})
	return out, err
}

func (r *boltFileStore) Delete(ctx context.Context, id uint64) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(filesBucket)
		if b == nil {
			return nil
		}
		pk := boltUint64Key(id)
		v := b.Get(pk)
		if v == nil {
			return ErrNotFound
		}
		f := new(File)
		if err := boltUnmarshal(v, f); err != nil {
			return err
		}
		if idx := tx.Bucket(filesByUserPathIdx); idx != nil {
			_ = idx.Delete(fileUserPathKey(f.UserID, f.Path))
		}
		if pidx := tx.Bucket(filesByUserParentIdx); pidx != nil {
			_ = pidx.Delete(fileUserParentFileKey(f.UserID, fileParentPath(f.Path), f.ID))
		}
		return b.Delete(pk)
	})
}

// file index helpers

func fileUserPathKey(userID uint64, filePath string) []byte {
	clean := normalizePath(filePath)
	key := make([]byte, 0, 8+1+len(clean))
	key = append(key, boltUint64Key(userID)...)
	key = append(key, 0)
	key = append(key, []byte(clean)...)
	return key
}

func fileParentPath(filePath string) string {
	clean := normalizePath(filePath)
	dir := path.Dir(clean)
	if dir == "" || dir == "." {
		return "/"
	}
	return dir
}

func fileUserParentFileKey(userID uint64, parent string, fileID uint64) []byte {
	clean := parent
	if clean == "" || clean == "." {
		clean = "/"
	}
	key := make([]byte, 0, 8+1+len(clean)+1+8)
	key = append(key, boltUint64Key(userID)...)
	key = append(key, 0)
	key = append(key, []byte(clean)...)
	key = append(key, 0)
	key = append(key, boltUint64Key(fileID)...)
	return key
}

func fileUserParentPrefix(userID uint64, parent string) []byte {
	clean := parent
	if clean == "" || clean == "." {
		clean = "/"
	}
	key := make([]byte, 0, 8+1+len(clean)+1)
	key = append(key, boltUint64Key(userID)...)
	key = append(key, 0)
	key = append(key, []byte(clean)...)
	key = append(key, 0)
	return key
}

func hasPrefixBytes(key, prefix []byte) bool {
	if len(key) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if key[i] != prefix[i] {
			return false
		}
	}
	return true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── boltTaskStore ─────────────────────────────────────────────────────────────

var (
	tasksBucket    = []byte("tasking_tasks")
	tasksByUserIdx = []byte("tasking_tasks_by_user")
)

type boltTaskStore struct {
	db *boltDB
}

func (r *boltTaskStore) Save(ctx context.Context, task *TaskInfo) error {
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := tx.CreateBucketIfNotExists(tasksBucket)
		if err != nil {
			return err
		}
		idx, err := tx.CreateBucketIfNotExists(tasksByUserIdx)
		if err != nil {
			return err
		}

		if existing := b.Get([]byte(task.ID)); existing != nil {
			old := new(TaskInfo)
			if unmarshalErr := boltUnmarshal(existing, old); unmarshalErr == nil {
				_ = idx.Delete(taskUserKey(old.UserID, old.ID))
			}
		}

		enc, err := boltMarshal(task)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(task.ID), enc); err != nil {
			return err
		}
		return idx.Put(taskUserKey(task.UserID, task.ID), []byte(task.ID))
	})
}

func (r *boltTaskStore) GetByID(ctx context.Context, id string) (*TaskInfo, error) {
	_ = ctx
	var out *TaskInfo
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(tasksBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get([]byte(id))
		if v == nil {
			return ErrNotFound
		}
		t := new(TaskInfo)
		if err := boltUnmarshal(v, t); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

func (r *boltTaskStore) ListByUser(ctx context.Context, userID uint64) ([]*TaskInfo, error) {
	out := make([]*TaskInfo, 0)
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		idx := tx.Bucket(tasksByUserIdx)
		b := tx.Bucket(tasksBucket)
		if b == nil || idx == nil {
			return nil
		}
		prefix := boltUint64Key(userID)
		c := idx.Cursor()
		for k, taskID := c.Seek(prefix); k != nil && hasPrefixBytes(k, prefix); k, taskID = c.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			v := b.Get(taskID)
			if v == nil {
				continue
			}
			t := new(TaskInfo)
			if err := boltUnmarshal(v, t); err != nil {
				return err
			}
			out = append(out, t)
		}
		return nil
	})
	return out, err
}

func (r *boltTaskStore) Delete(ctx context.Context, id string) error {
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b := tx.Bucket(tasksBucket)
		if b == nil {
			return nil
		}
		if v := b.Get([]byte(id)); v != nil {
			t := new(TaskInfo)
			if err := boltUnmarshal(v, t); err == nil {
				if idx := tx.Bucket(tasksByUserIdx); idx != nil {
					_ = idx.Delete(taskUserKey(t.UserID, t.ID))
				}
			}
		}
		return b.Delete([]byte(id))
	})
}

func taskUserKey(userID uint64, taskID string) []byte {
	key := make([]byte, 0, 8+1+len(taskID))
	key = append(key, boltUint64Key(userID)...)
	key = append(key, 0)
	key = append(key, []byte(taskID)...)
	return key
}

// ── boltSettingsStore ─────────────────────────────────────────────────────────

var (
	settingsBucket = []byte("settings")
	settingsKey    = []byte("global")
)

type boltSettingsStore struct {
	db *boltDB
}

func (r *boltSettingsStore) Get(ctx context.Context) (*Settings, error) {
	_ = ctx
	var out *Settings
	err := boltView(r.db, func(tx *bbolt.Tx) error {
		b := tx.Bucket(settingsBucket)
		if b == nil {
			return ErrNotFound
		}
		v := b.Get(settingsKey)
		if v == nil {
			return ErrNotFound
		}
		s := new(Settings)
		if err := boltUnmarshal(v, s); err != nil {
			return err
		}
		out = s
		return nil
	})
	return out, err
}

func (r *boltSettingsStore) Save(ctx context.Context, settings *Settings) error {
	_ = ctx
	return boltUpdate(r.db, func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(settingsBucket)
		if err != nil {
			return err
		}
		enc, err := boltMarshal(settings)
		if err != nil {
			return err
		}
		return b.Put(settingsKey, enc)
	})
}

// ── boltPluginStore ───────────────────────────────────────────────────────────

type boltPluginStore struct {
	db      *boltDB
	slug    string
	dataDir string
}

var pluginsBucket = []byte("plugins") //nolint:unused //nolint:unused

func newBoltPluginStore(db *boltDB, slug, dataDir string) *boltPluginStore {
	return &boltPluginStore{db: db, slug: slug, dataDir: dataDir}
}

func (s *boltPluginStore) bucket() []byte {
	return []byte("plugin_" + s.slug)
}

func (s *boltPluginStore) Migrate(migrations []PluginMigration) error {
	return boltUpdate(s.db, func(tx *bbolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists(s.bucket())
		if err != nil {
			return fmt.Errorf("create plugin bucket: %w", err)
		}
		schemaBucket, err := root.CreateBucketIfNotExists([]byte("__schema"))
		if err != nil {
			return err
		}
		versionKey := []byte("version")
		current := 0
		if raw := schemaBucket.Get(versionKey); len(raw) == 4 {
			// Backward compatibility: previous schema version was uint32-encoded.
			current = int(binary.BigEndian.Uint32(raw))
		} else if len(raw) > 0 {
			parsed, parseErr := strconv.Atoi(string(raw))
			if parseErr == nil {
				current = parsed
			}
		}
		for _, m := range migrations {
			if m.Version <= current {
				continue
			}
			if m.Migrate == nil {
				continue
			}
			tx_ := &boltPluginMigrationTx{root: root}
			if err := m.Migrate(tx_); err != nil {
				return fmt.Errorf("plugin migration %d: %w", m.Version, err)
			}
			if m.Version < 0 {
				return fmt.Errorf("invalid migration version: %d", m.Version)
			}
			if err := schemaBucket.Put(versionKey, []byte(strconv.Itoa(m.Version))); err != nil {
				return err
			}
			current = m.Version
		}
		return nil
	})
}

func (s *boltPluginStore) BoltDB() *bbolt.DB {
	return s.db.Raw()
}

func (s *boltPluginStore) Put(key string, value interface{}) error {
	return boltUpdate(s.db, func(tx *bbolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists(s.bucket())
		if err != nil {
			return err
		}
		kv, err := root.CreateBucketIfNotExists([]byte("kv"))
		if err != nil {
			return err
		}
		enc, err := boltMarshal(value)
		if err != nil {
			return err
		}
		return kv.Put([]byte(key), enc)
	})
}

func (s *boltPluginStore) Get(key string, v interface{}) error {
	return boltView(s.db, func(tx *bbolt.Tx) error {
		root := tx.Bucket(s.bucket())
		if root == nil {
			return ErrNotFound
		}
		kv := root.Bucket([]byte("kv"))
		if kv == nil {
			return ErrNotFound
		}
		data := kv.Get([]byte(key))
		if data == nil {
			return ErrNotFound
		}
		return boltUnmarshal(data, v)
	})
}

func (s *boltPluginStore) Delete(key string) error {
	return boltUpdate(s.db, func(tx *bbolt.Tx) error {
		root := tx.Bucket(s.bucket())
		if root == nil {
			return nil
		}
		kv := root.Bucket([]byte("kv"))
		if kv == nil {
			return nil
		}
		return kv.Delete([]byte(key))
	})
}

func (s *boltPluginStore) GetConfig() ([]byte, error) {
	var out []byte
	err := boltView(s.db, func(tx *bbolt.Tx) error {
		root := tx.Bucket(s.bucket())
		if root == nil {
			return nil
		}
		v := root.Get([]byte("__config"))
		if v != nil {
			out = make([]byte, len(v))
			copy(out, v)
		}
		return nil
	})
	return out, err
}

func (s *boltPluginStore) SaveConfig(data []byte) error {
	return boltUpdate(s.db, func(tx *bbolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists(s.bucket())
		if err != nil {
			return err
		}
		return root.Put([]byte("__config"), data)
	})
}

func (s *boltPluginStore) DataDir() string {
	return filepath.Join(s.dataDir, "plugins", s.slug)
}

func (s *boltPluginStore) EnsureDataDir() string {
	dir := s.DataDir()
	_ = os.MkdirAll(dir, 0o750)
	return dir
}

// boltPluginMigrationTx adapts bbolt.Bucket to PluginMigrationTx.
type boltPluginMigrationTx struct {
	root *bbolt.Bucket
}

func (t *boltPluginMigrationTx) CreateBucketIfNotExists(name string) error {
	_, err := t.root.CreateBucketIfNotExists([]byte(name))
	return err
}

func (t *boltPluginMigrationTx) Get(bucket string, key []byte) ([]byte, error) {
	b := t.root.Bucket([]byte(bucket))
	if b == nil {
		return nil, nil
	}
	v := b.Get(key)
	if v == nil {
		return nil, nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (t *boltPluginMigrationTx) Put(bucket string, key, val []byte) error {
	b, err := t.root.CreateBucketIfNotExists([]byte(bucket))
	if err != nil {
		return err
	}
	return b.Put(key, val)
}

func (t *boltPluginMigrationTx) Delete(bucket string, key []byte) error {
	b := t.root.Bucket([]byte(bucket))
	if b == nil {
		return nil
	}
	return b.Delete(key)
}

func (t *boltPluginMigrationTx) ForEach(bucket string, fn func(k, v []byte) error) error {
	b := t.root.Bucket([]byte(bucket))
	if b == nil {
		return nil
	}
	return b.ForEach(fn)
}
