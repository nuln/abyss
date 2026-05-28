package abyss

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
