package abyss

import (
	"context"
	"time"

	"github.com/google/uuid"
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
