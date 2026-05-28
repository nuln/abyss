package abyss

import (
	"context"
	"encoding/json"
	"sync"
)

// Settings holds global application settings.
type Settings struct {
	Signup                bool             `json:"signup"`
	CreateUserDir         bool             `json:"createUserDir"`
	HideLoginButton       bool             `json:"hideLoginButton"`
	MinimumPasswordLength int              `json:"minimumPasswordLength"`
	StorageType           string           `json:"storageType"`
	Defaults              SettingsDefaults `json:"defaults"`
	Branding              SettingsBranding `json:"branding"`
	Tus                   SettingsTus      `json:"tus"`
	Rules                 []any            `json:"rules"`
	PluginStatuses        map[string]bool  `json:"pluginStatuses,omitempty"`

	// Virtual fields (not persisted in DB but sent to frontend)
	AvailableStorageTypes []StorageTypeInfo `json:"availableStorageTypes"`
}

// SettingsDefaults holds default preferences for new users.
type SettingsDefaults struct {
	Scope          string      `json:"scope"`
	Locale         string      `json:"locale"`
	Theme          string      `json:"theme"`
	ViewMode       string      `json:"viewMode"`
	SingleClick    bool        `json:"singleClick"`
	DateFormat     bool        `json:"dateFormat"`
	Perm           Permissions `json:"perm"`
	Sorting        Sorting     `json:"sorting"`
	AceEditorTheme string      `json:"aceEditorTheme"`
}

// SettingsBranding holds site-wide branding settings.
type SettingsBranding struct {
	Name                  string `json:"name"`
	DisableExternal       bool   `json:"disableExternal"`
	DisableUsedPercentage bool   `json:"disableUsedPercentage"`
	Theme                 string `json:"theme"`
	Color                 string `json:"color"`
	Files                 string `json:"files"`
}

// SettingsTus holds Tus upload settings.
type SettingsTus struct {
	ChunkSize  int64 `json:"chunkSize"`
	RetryCount int   `json:"retryCount"`
}

// SettingsStore defines global settings persistence.
type SettingsStore interface {
	Get(ctx context.Context) (*Settings, error)
	Save(ctx context.Context, settings *Settings) error
}

// ── settingsService ───────────────────────────────────────────────────────────

type settingsService struct {
	store SettingsStore
	mu    sync.RWMutex
	cache *Settings
}

func newSettingsService(store SettingsStore) *settingsService {
	return &settingsService{store: store}
}

func (s *settingsService) Get(ctx context.Context) (*Settings, error) {
	s.mu.RLock()
	if s.cache != nil {
		cached := cloneSettings(s.cache)
		s.mu.RUnlock()
		return normalizeSettings(cached), nil
	}
	s.mu.RUnlock()

	st, err := s.store.Get(ctx)
	if err != nil {
		if err == ErrNotFound {
			st = &Settings{
				Signup:                false,
				CreateUserDir:         true,
				HideLoginButton:       false,
				MinimumPasswordLength: 8,
				StorageType:           "path",
				Defaults: SettingsDefaults{
					Scope:       ".",
					Locale:      "auto",
					Theme:       "auto",
					ViewMode:    "mosaic",
					SingleClick: false,
					Perm: Permissions{
						Create: true, Rename: true, Modify: true, Delete: true,
						Share: true, Download: true, Copy: true, Move: true, Upload: true,
					},
					Sorting:        Sorting{By: "name", Asc: true},
					AceEditorTheme: "github",
				},
				Branding: SettingsBranding{
					Name:  "Abyss",
					Theme: "auto",
					Files: "",
				},
				Rules:                 []any{},
			}
		} else {
			return nil, err
		}
	}
	normalized := normalizeSettings(st)
	s.mu.Lock()
	s.cache = cloneSettings(normalized)
	s.mu.Unlock()
	return cloneSettings(normalized), nil
}

func (s *settingsService) Save(ctx context.Context, in *Settings) error {
	if err := s.store.Save(ctx, in); err != nil {
		return err
	}
	s.mu.Lock()
	s.cache = cloneSettings(in)
	s.mu.Unlock()
	return nil
}

func cloneSettings(in *Settings) *Settings {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		cp := *in
		return &cp
	}
	var out Settings
	if err := json.Unmarshal(data, &out); err != nil {
		cp := *in
		return &cp
	}
	return &out
}

func normalizeSettings(st *Settings) *Settings {
	if st == nil {
		return nil
	}
	st = cloneSettings(st)
	st.AvailableStorageTypes = GetAvailableStorageTypes()
	if st.MinimumPasswordLength == 0 {
		st.MinimumPasswordLength = 8
	}
	if st.StorageType == "" {
		st.StorageType = "path"
	}
	if st.Defaults.Locale == "" {
		st.Defaults.Locale = "auto"
	}
	if st.Defaults.Theme == "" {
		st.Defaults.Theme = "auto"
	}
	if st.Branding.Theme == "" {
		st.Branding.Theme = "auto"
	}
	return st
}
