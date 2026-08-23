package abyss

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level application configuration loaded from config.toml.
type Config struct {
	Server       ServerConfig   `toml:"server"`
	Data         DataConfig     `toml:"data"`
	Auth         AuthConfig     `toml:"auth"`
	Database     DatabaseConfig `toml:"database"`
	Demo         bool           `toml:"demo"`
	DemoEmail    string         `toml:"demoEmail"`
	DemoPassword string         `toml:"demoPassword"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr    string `toml:"addr"`
	BaseURL string `toml:"baseURL"`
}

// DataConfig holds data directory configuration.
type DataConfig struct {
	Dir string `toml:"dir"`
}

// AuthConfig holds JWT signing configuration.
type AuthConfig struct {
	// JWTSecret is a hex-encoded 32-byte secret. Generated randomly if empty.
	JWTSecret       string        `toml:"jwtSecret"`
	AccessTokenTTL  time.Duration `toml:"accessTokenTTL"`
	RefreshTokenTTL time.Duration `toml:"refreshTokenTTL"`
	AllowQueryToken bool          `toml:"allowQueryToken"`
}

// DatabaseConfig controls BoltDB behaviour.
type DatabaseConfig struct {
	// Path is an optional explicit database path. If empty, Data.Dir/abyss.db is used.
	Path string `toml:"path"`
	// Timeout is how long to wait for the database lock (default 3s).
	Timeout time.Duration `toml:"timeout"`
	// NoSync disables fsync – for testing only.
	NoSync bool `toml:"noSync"`
}

type legacyCompatConfig struct {
	Address             string `toml:"address"`
	Port                string `toml:"port"`
	Root                string `toml:"root"`
	TokenExpirationTime string `toml:"tokenExpirationTime"`
	BaseURL             string `toml:"baseURL"`
}

type configFlagValues struct {
	configPath          string
	demo                bool
	demoEmail           string
	demoPassword        string
	address             string
	port                string
	root                string
	tokenExpirationTime string
	dbPath              string
	databasePath        string
	dbTimeout           string
	dbNoSync            bool
	addr                string
	baseURL             string
}

// LoadConfig parses command-line flags and config.toml.
func LoadConfig(args []string) (Config, error) {
	flags, setFlags, err := parseConfigFlags(args)
	if err != nil {
		return Config{}, err
	}

	cfg, legacyCfg, fileMissing, err := loadConfigFile(flags.configPath)
	if err != nil {
		return Config{}, err
	}

	applyLegacyConfigCompat(&cfg, &legacyCfg)
	applyDefaults(&cfg)

	secretGenerated, err := ensureJWTSecret(&cfg)
	if err != nil {
		return Config{}, err
	}

	if fileMissing || secretGenerated {
		if saveErr := SaveConfig(flags.configPath, &cfg); saveErr != nil {
			slog.Warn("failed to auto-save config", "path", flags.configPath, "err", saveErr)
		}
	}

	if err := validateConfig(&cfg); err != nil {
		return Config{}, err
	}
	if err := applyFlagOverrides(&cfg, &flags, setFlags); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func parseConfigFlags(args []string) (configFlagValues, map[string]bool, error) {
	fs := flag.NewFlagSet("abyss", flag.ContinueOnError)
	configPath := fs.String("config", "config.toml", "path to config.toml")
	demo := fs.Bool("demo", false, "enable demo mode")
	demoEmail := fs.String("demo-email", "", "demo account email (used with -demo)")
	demoPassword := fs.String("demo-password", "", "demo account password (used with -demo)")

	// Legacy compatibility flags from pre-refactor binaries.
	address := fs.String("address", "", "legacy: server listen address")
	port := fs.String("port", "", "legacy: server listen port")
	root := fs.String("root", "", "legacy: data root directory")
	tokenExpirationTime := fs.String("tokenExpirationTime", "", "legacy: access token TTL, e.g. 24h or 7d")
	dbPath := fs.String("db-path", "", "database path override")
	databasePath := fs.String("database", "", "legacy alias of -db-path")
	dbTimeout := fs.String("db-timeout", "", "database lock timeout, e.g. 3s")
	dbNoSync := fs.Bool("db-nosync", false, "disable fsync (testing only)")
	addr := fs.String("addr", "", "server listen address, e.g. :8080")
	baseURL := fs.String("baseurl", "", "server base URL prefix, e.g. /abyss")
	if err := fs.Parse(args); err != nil {
		return configFlagValues{}, nil, fmt.Errorf("parse flags: %w", err)
	}

	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	return configFlagValues{
		configPath:          *configPath,
		demo:                *demo,
		demoEmail:           *demoEmail,
		demoPassword:        *demoPassword,
		address:             *address,
		port:                *port,
		root:                *root,
		tokenExpirationTime: *tokenExpirationTime,
		dbPath:              *dbPath,
		databasePath:        *databasePath,
		dbTimeout:           *dbTimeout,
		dbNoSync:            *dbNoSync,
		addr:                *addr,
		baseURL:             *baseURL,
	}, setFlags, nil
}

func loadConfigFile(configPath string) (Config, legacyCompatConfig, bool, error) {
	cfg := Config{Auth: AuthConfig{AllowQueryToken: true}}
	legacyCfg := legacyCompatConfig{}
	cleanPath := filepath.Clean(configPath)
	data, err := os.ReadFile(cleanPath)
	if err != nil && !os.IsNotExist(err) {
		return Config{}, legacyCompatConfig{}, false, fmt.Errorf("read config: %w", err)
	}
	if len(data) > 0 {
		if unmarshalErr := toml.Unmarshal(data, &cfg); unmarshalErr != nil {
			return Config{}, legacyCompatConfig{}, false, fmt.Errorf("parse toml: %w", unmarshalErr)
		}
		_ = toml.Unmarshal(data, &legacyCfg)
	}
	return cfg, legacyCfg, os.IsNotExist(err), nil
}

func applyFlagOverrides(cfg *Config, flags *configFlagValues, setFlags map[string]bool) error {
	if flags.demo {
		cfg.Demo = true
	}
	if setFlags["demo-email"] {
		cfg.DemoEmail = flags.demoEmail
	}
	if setFlags["demo-password"] {
		cfg.DemoPassword = flags.demoPassword
	}
	if setFlags["root"] {
		cfg.Data.Dir = flags.root
	}
	if setFlags["address"] || setFlags["port"] {
		cfg.Server.Addr = composeLegacyAddr(flags.address, flags.port)
	}
	if setFlags["addr"] {
		cfg.Server.Addr = flags.addr
	}
	if setFlags["tokenExpirationTime"] {
		d, err := parseFlexibleDuration(flags.tokenExpirationTime)
		if err != nil {
			return fmt.Errorf("invalid -tokenExpirationTime: %w", err)
		}
		cfg.Auth.AccessTokenTTL = d
	}
	if setFlags["db-path"] {
		cfg.Database.Path = flags.dbPath
	}
	if setFlags["database"] {
		cfg.Database.Path = flags.databasePath
	}
	if setFlags["db-timeout"] {
		d, err := parseFlexibleDuration(flags.dbTimeout)
		if err != nil {
			return fmt.Errorf("invalid -db-timeout: %w", err)
		}
		cfg.Database.Timeout = d
	}
	if setFlags["db-nosync"] {
		cfg.Database.NoSync = flags.dbNoSync
	}
	if setFlags["baseurl"] {
		cfg.Server.BaseURL = normalizeBaseURL(flags.baseURL)
	}
	return nil
}

// SaveConfig writes the configuration to a TOML file.
func SaveConfig(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// DBPath returns the absolute path to the BoltDB database file.
func (c *Config) DBPath() string {
	if strings.TrimSpace(c.Database.Path) != "" {
		return c.Database.Path
	}
	return filepath.Join(c.Data.Dir, "abyss.db")
}

// JWTSecretBytes decodes the hex JWT secret into raw bytes.
func (c *Config) JWTSecretBytes() ([]byte, error) {
	b, err := hex.DecodeString(c.Auth.JWTSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid jwtSecret: %w", err)
	}
	return b, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	cfg.Server.BaseURL = normalizeBaseURL(cfg.Server.BaseURL)
	if cfg.Data.Dir == "" {
		cfg.Data.Dir = "data"
	}
	if cfg.Auth.AccessTokenTTL == 0 {
		cfg.Auth.AccessTokenTTL = 24 * time.Hour
	}
	if cfg.Auth.RefreshTokenTTL == 0 {
		cfg.Auth.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if cfg.Database.Timeout == 0 {
		cfg.Database.Timeout = 3 * time.Second
	}
	if cfg.DemoEmail == "" {
		cfg.DemoEmail = "demo@abyss.local"
	}
	if cfg.DemoPassword == "" {
		cfg.DemoPassword = "demo123456"
	}
}

func normalizeBaseURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || u == "/" {
		return ""
	}
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	return strings.TrimSuffix(u, "/")
}

func applyLegacyConfigCompat(cfg *Config, legacy *legacyCompatConfig) {
	if cfg.Server.Addr == "" && (strings.TrimSpace(legacy.Address) != "" || strings.TrimSpace(legacy.Port) != "") {
		cfg.Server.Addr = composeLegacyAddr(legacy.Address, legacy.Port)
	}
	if cfg.Server.BaseURL == "" && strings.TrimSpace(legacy.BaseURL) != "" {
		cfg.Server.BaseURL = legacy.BaseURL
	}
	if cfg.Data.Dir == "" && strings.TrimSpace(legacy.Root) != "" {
		cfg.Data.Dir = legacy.Root
	}
	if cfg.Auth.AccessTokenTTL == 0 && strings.TrimSpace(legacy.TokenExpirationTime) != "" {
		if d, err := parseFlexibleDuration(legacy.TokenExpirationTime); err == nil {
			cfg.Auth.AccessTokenTTL = d
		}
	}
}

func composeLegacyAddr(address, port string) string {
	address = strings.TrimSpace(address)
	port = strings.TrimSpace(port)
	if port == "" {
		if address == "" {
			return ":8080"
		}
		return address
	}
	if strings.HasPrefix(port, ":") {
		if address == "" {
			return port
		}
		return address + port
	}
	if address == "" {
		return ":" + port
	}
	if strings.Contains(address, ":") {
		return address
	}
	return address + ":" + port
}

func parseFlexibleDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("empty duration")
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	if strings.HasSuffix(raw, "d") {
		n := strings.TrimSuffix(raw, "d")
		v, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(v * float64(24*time.Hour)), nil
	}
	return 0, fmt.Errorf("unsupported duration format: %s", raw)
}

func ensureJWTSecret(cfg *Config) (bool, error) {
	if strings.TrimSpace(cfg.Auth.JWTSecret) != "" {
		return false, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return false, fmt.Errorf("generate jwt secret: %w", err)
	}
	cfg.Auth.JWTSecret = hex.EncodeToString(b)
	return true, nil
}

func validateConfig(cfg *Config) error {
	if cfg.Server.Addr == "" {
		return errors.New("server.addr is required")
	}
	if cfg.Data.Dir == "" {
		return errors.New("data.dir is required")
	}
	return nil
}
