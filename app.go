// Package abyss is the library entry point for abyss.
//
// External projects (e.g. the root abyss binary) call [Run] to start the server:
//
//	package main
//
//	import "github.com/nuln/abyss"
//
//	func main() {
//	    abyss.Main()
//	}
//
// Plugins are assembled at compile time via blank imports in a separate build-tag file.
package abyss

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/pelletier/go-toml/v2"

	"github.com/nuln/abyss/www"
)

// App is the running application.
type App struct {
	Config Config
	DB     *boltDB
	Router *mux.Router
	Server *http.Server

	userSvc      *userService
	authSvc      *authService
	storageSvc   *storageService
	taskSvc      *taskService
	settingsSvc  *settingsService
	sessionStore SessionStore
	pluginMgr    *Manager
	users        pluginUsers
}

// Run is the library entry point.
// It bootstraps the server, runs it until SIGINT/SIGTERM, then shuts down gracefully.
func Run() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	app, err := Bootstrap(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("abyss starting", "addr", app.Config.Server.Addr)
	if err := app.Run(ctx); err != nil {
		return err
	}

	slog.Info("abyss stopped")
	return nil
}

// Bootstrap assembles all dependencies and returns a ready-to-run App.
//
//nolint:gocyclo // Bootstrap wires many startup branches in one place by design.
func Bootstrap(args []string) (*App, error) {
	cfg, err := LoadConfig(args)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Use Info level in production; set LOG_LEVEL=debug env var to enable debug output.
	logLevel := slog.LevelInfo
	if v := os.Getenv("LOG_LEVEL"); strings.EqualFold(v, "debug") {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	// Ensure data directory exists.
	if mkdirErr := os.MkdirAll(cfg.Data.Dir, 0o750); mkdirErr != nil {
		return nil, fmt.Errorf("create data dir: %w", mkdirErr)
	}

	db, err := openAndInitDB(&cfg)
	if err != nil {
		return nil, err
	}

	deps, err := newAppDependencies(&cfg, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	// ── Plugin status persistence ─────────────────────────────────────────────
	// Restore plugin enabled/disabled state from settings so it survives restarts.
	if savedSettings, settingsErr := deps.settingsSvc.Get(context.Background()); settingsErr == nil && savedSettings != nil {
		if len(savedSettings.PluginStatuses) > 0 {
			StatusManager.SetStatuses(savedSettings.PluginStatuses)
		}
	}
	// Persist plugin status changes back to settings.
	StatusManager.SetPersistenceHook(func(name string, enabled bool) error {
		ctx := context.Background()
		return deps.settingsSvc.Update(ctx, func(s *Settings) {
			if s.PluginStatuses == nil {
				s.PluginStatuses = make(map[string]bool)
			}
			s.PluginStatuses[name] = enabled
		})
	})

	// Create demo user if enabled.
	if cfg.Demo {
		u, err := deps.userSvc.store.GetByEmail(context.Background(), cfg.DemoEmail)
		if err != nil && errors.Is(err, ErrNotFound) {
			slog.Info("creating demo user", "email", cfg.DemoEmail)
			u, err = deps.userSvc.Register(context.Background(), cfg.DemoEmail, "demo", cfg.DemoPassword, "Demo User")
			if err != nil {
				slog.Error("failed to create demo user", "err", err)
			} else {
				u.Role = RoleAdmin
				u.Permissions.Admin = true

				// Apply default settings
				if settings, settingsErr := deps.settingsSvc.Get(context.Background()); settingsErr == nil && settings != nil {
					u.Preferences.Locale = settings.Defaults.Locale
					u.Preferences.Theme = settings.Defaults.Theme
					u.Preferences.Scope = settings.Defaults.Scope
					u.Preferences.SingleClick = settings.Defaults.SingleClick
					u.Preferences.Sorting = settings.Defaults.Sorting
				}

				_ = deps.userSvc.UpdateUser(context.Background(), u)
			}
		} else if err == nil {
			slog.Info("demo user already exists, ensuring password is up to date", "email", u.Email)
			hash, hashErr := hashPassword(cfg.DemoPassword)
			if hashErr != nil {
				slog.Error("failed to hash demo user password", "err", hashErr)
			} else {
				u.PasswordHash = hash
				u.Role = RoleAdmin
				u.Permissions.Admin = true
				_ = deps.userSvc.UpdateUser(context.Background(), u)
			}
		}
	}

	// ── HTTP Router ──────────────────────────────────────────────────────────
	r := mux.NewRouter()
	root := r
	if cfg.Server.BaseURL != "" {
		root = r.PathPrefix(cfg.Server.BaseURL).Subrouter()
	}

	app := &App{
		Config:       cfg,
		DB:           db,
		Router:       r,
		userSvc:      deps.userSvc,
		authSvc:      deps.authSvc,
		storageSvc:   deps.storageSvc,
		taskSvc:      deps.taskSvc,
		settingsSvc:  deps.settingsSvc,
		sessionStore: deps.sessionStore,
		users:        pluginUsers{userSvc: deps.userSvc, authSvc: deps.authSvc},
	}

	// Connect task service for plugins.
	mountPluginTaskService(deps.taskSvc)

	// Initialize all registered plugins with per-plugin stores.
	pluginMgr := NewManager()
	startupCtx := &StartupContext{
		Files:   pluginFiles{storage: deps.storageSvc},
		Users:   pluginUsers{userSvc: deps.userSvc, authSvc: deps.authSvc},
		Logger:  slog.Default(),
		BaseURL: cfg.Server.BaseURL,
		Handler: func(fn HandleFunc) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				uid := AuthUserIDFromContext(r.Context())
				isAdmin := AuthIsAdminFromContext(r.Context())

				var user *User
				if uid != 0 {
					u, err := deps.userSvc.GetByID(r.Context(), uid)
					if err == nil {
						user = u
					}
				}

				var engine StorageEngine
				if uid != 0 {
					e, err := deps.storageSvc.GetEngine(uid)
					if err == nil {
						engine = e
					}
				}

				resp := fn(&Data{
					UserID:   uid,
					User:     user,
					IsAdmin:  isAdmin,
					Engine:   engine,
					Request:  r,
					Response: w,
				})
				if resp == nil {
					return
				}

				status := resp.Status
				if status == 0 {
					status = http.StatusOK
				}
				if resp.Success {
					WriteJSON(w, status, resp.Data)
				} else {
					WriteJSON(w, status, ErrorResponse(resp.Error))
				}
			})
		},
		StoreFactory: func(slug string) PluginStore {
			return newBoltPluginStore(db, slug, cfg.Data.Dir)
		},
	}
	if err := pluginMgr.Init(startupCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init plugins: %w", err)
	}

	registerAllRoutes(root, app)
	app.pluginMgr = pluginMgr

	// Serve embedded web static files.
	if www.PublicFS != nil {
		dist := www.PublicFS
		assetsHandler := http.FileServer(http.FS(dist))
		imgHandler := http.FileServer(http.FS(dist))
		if cfg.Server.BaseURL != "" {
			// Strip the BaseURL so the FileServer sees paths starting with /assets or /img.
			assetsHandler = http.StripPrefix(cfg.Server.BaseURL, assetsHandler)
			imgHandler = http.StripPrefix(cfg.Server.BaseURL, imgHandler)
		}

		root.PathPrefix("/assets/").Handler(assetsHandler)
		root.PathPrefix("/img/").Handler(imgHandler)

		// Also register on the main router 'r' as fallback for absolute paths
		// often found in compiled CSS files (e.g. url(/assets/font.woff)).
		if cfg.Server.BaseURL != "" {
			r.PathPrefix("/assets/").Handler(http.FileServer(http.FS(dist)))
			r.PathPrefix("/img/").Handler(http.FileServer(http.FS(dist)))
		}

		root.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, dist, "img/icons/favicon.ico")
		})
		root.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, dist, "manifest.json")
		})
	}

	// Everything else served by the index handler (SPA).
	root.PathPrefix("/").HandlerFunc(app.handleIndex)

	// ── HTTP Server ──────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
	}
	app.Server = srv

	return app, nil
}

type appDependencies struct {
	userSvc      *userService
	authSvc      *authService
	storageSvc   *storageService
	taskSvc      *taskService
	settingsSvc  *settingsService
	sessionStore SessionStore
}

func openAndInitDB(cfg *Config) (*boltDB, error) {
	timeout := cfg.Database.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	db, err := openBoltDB(boltConfig{Path: cfg.DBPath(), Timeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := boltEnsureSchema(db,
		usersBucket,
		usersEmailIdx,
		usersUsernameIdx,
		sessionsBucket,
		sessionsHashIdx,
		sessionsUserIdx,
		filesBucket,
		filesByUserPathIdx,
		filesByUserParentIdx,
		tasksBucket,
		tasksByUserIdx,
		settingsBucket,
	); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	return db, nil
}

func newAppDependencies(cfg *Config, db *boltDB) (*appDependencies, error) {
	jwtSecret, err := cfg.JWTSecretBytes()
	if err != nil {
		return nil, fmt.Errorf("jwt secret: %w", err)
	}
	userStore := &boltUserStore{db: db}
	sessionStore := &boltSessionStore{db: db}
	fileStore := &boltFileStore{db: db}
	taskStore := &boltTaskStore{db: db}
	settingsStore := &boltSettingsStore{db: db}

	deps := &appDependencies{
		userSvc:      &userService{store: userStore},
		authSvc:      newAuthService(userStore, sessionStore, jwtSecret, cfg.Auth.AccessTokenTTL, cfg.Auth.RefreshTokenTTL, cfg.Auth.AllowQueryToken),
		settingsSvc:  newSettingsService(settingsStore),
		sessionStore: sessionStore,
	}
	deps.storageSvc = newStorageService(fileStore, userStore, deps.settingsSvc, cfg.Data.Dir)
	deps.taskSvc = newTaskService(taskStore, newScheduler())
	registerStorageRepairRunners(deps.taskSvc, deps.storageSvc)
	return deps, nil
}

func registerStorageRepairRunners(taskSvc *taskService, storageSvc *storageService) {
	taskSvc.RegisterRunner("storage_consistency_repair", makeStorageRepairRunner(storageSvc, false, 0, "dry_run"))
	taskSvc.RegisterRunner("storage_consistency_repair_auto_fix", makeStorageRepairRunner(storageSvc, true, 1000, "auto_fix"))
}

func makeStorageRepairRunner(storageSvc *storageService, autoFix bool, maxOps int, mode string) func(context.Context) error {
	return func(ctx context.Context) error {
		uid := taskUserIDFromContext(ctx)
		if uid == 0 {
			return ErrUnauthorized
		}
		report, err := storageSvc.RepairConsistency(ctx, uid, autoFix, maxOps)
		if err != nil {
			return err
		}
		slog.Info("storage consistency repair completed",
			"mode", mode,
			"userID", uid,
			"scannedMeta", report.ScannedMeta,
			"scannedFS", report.ScannedFS,
			"orphanMeta", report.OrphanMeta,
			"orphanFile", report.OrphanFile,
			"fixedCount", report.FixedCount,
			"failedCount", report.FailedCount,
		)
		return nil
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	if a.pluginMgr != nil {
		if err := a.pluginMgr.StartAll(ctx); err != nil {
			return fmt.Errorf("start plugins: %w", err)
		}
	}
	go func() {
		<-ctx.Done()
		_ = a.Shutdown(context.Background())
	}()
	if err := a.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server and closes the database.
func (a *App) Shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var errs []error
	if a.Server != nil {
		if err := a.Server.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("http shutdown: %w", err))
		}
	}
	if a.pluginMgr != nil {
		if err := a.pluginMgr.StopAll(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("plugin stop: %w", err))
		}
	}
	if a.DB != nil {
		if err := a.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("db close: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (a *App) cleanupUserData(ctx context.Context, u *User) error {
	if a == nil || u == nil {
		return nil
	}
	userID := u.ID
	if a.storageSvc != nil {
		// Cleanup plugin storage if any
		_ = CallStorageProvider(func(p StorageProvider) error {
			_, _ = p.CreateUserEngine(userID)
			// TODO: Add a proper CleanupUser method to StorageProvider interface
			return nil
		})

		// Cleanup path storage
		if a.storageSvc.dataDir != "" && u.UUID != "" {
			rootDir := filepath.Join(a.storageSvc.dataDir, "files", u.UUID)
			if err := os.RemoveAll(rootDir); err != nil {
				return err
			}
		}
		a.storageSvc.RemoveUserCache(userID)
	}

	if a.taskSvc != nil {
		tasks, err := a.taskSvc.ListByUser(ctx, userID)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			if task == nil {
				continue
			}
			if err := a.taskSvc.store.Delete(ctx, task.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

type pluginFiles struct {
	storage *storageService
}

func (f pluginFiles) GetEngine(userID uint64) (StorageEngine, error) {
	return f.storage.GetEngine(userID)
}

func (f pluginFiles) WriteFile(ctx context.Context, userID uint64, path string, r io.Reader) error {
	_, err := f.storage.WriteFile(ctx, userID, path, r)
	return err
}

func (f pluginFiles) CreateDir(ctx context.Context, userID uint64, path string) error {
	_, err := f.storage.CreateDir(ctx, userID, path)
	return err
}

func (f pluginFiles) GetFileByID(id uint64) (*EntryInfo, error) {
	return f.storage.GetFileByID(context.Background(), id)
}

func (f pluginFiles) ListFiles(userID uint64) ([]*EntryInfo, error) {
	return f.storage.ListByPath(context.Background(), userID, "/")
}

func (f pluginFiles) ResizeImage(ctx context.Context, in io.Reader, w, h int, out io.Writer) error {
	if w <= 0 {
		w = 512
	}
	if h <= 0 {
		h = 512
	}
	return resizeToFit(in, w, h, out)
}

type pluginUsers struct {
	userSvc *userService
	authSvc *authService
}

func (u pluginUsers) GetByID(id uint64) (*User, error) {
	return u.userSvc.GetByID(context.Background(), id)
}

func (u pluginUsers) GetByEmail(email string) (*User, error) {
	return u.userSvc.store.GetByEmail(context.Background(), email)
}

func (u pluginUsers) GetByUsername(username string) (*User, error) {
	return u.userSvc.store.GetByUsername(context.Background(), username)
}

func (u pluginUsers) GetAll() ([]*User, error) {
	return u.userSvc.List(context.Background())
}

func (u pluginUsers) UserIDFromRequest(r *http.Request) uint64 {
	return AuthUserIDFromContext(r.Context())
}

func (u pluginUsers) IssueToken(w http.ResponseWriter, r *http.Request, user *User, ttl time.Duration) (int, error) {
	if u.authSvc == nil {
		return http.StatusInternalServerError, ErrInternal
	}
	result, err := u.authSvc.issueAuthResultWithTTL(r.Context(), user, r.UserAgent(), ttl)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	WriteJSON(w, http.StatusOK, result)
	return http.StatusOK, nil
}

func (u pluginUsers) IssueMFAToken(userID uint64, method string, ttl time.Duration) (string, error) {
	// Generate a temporary token with MFA pending claim and bound method
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"uid": userID,
		"mfa": true,
		"met": method, // Bound method
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
		"typ": "mfa",
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(u.authSvc.jwtSecret)
}

func (u pluginUsers) VerifyMFAToken(tokenStr string) (userID uint64, method string, err error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return u.authSvc.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, "", fmt.Errorf("invalid mfa token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", fmt.Errorf("invalid claims")
	}
	if mfa, _ := claims["mfa"].(bool); !mfa {
		return 0, "", fmt.Errorf("not an mfa token")
	}
	if typ, _ := claims["typ"].(string); typ != "mfa" {
		return 0, "", fmt.Errorf("not an mfa token")
	}
	uid, _ := claims["uid"].(float64)
	met, _ := claims["met"].(string)
	return uint64(uid), met, nil
}

func (u pluginUsers) Encrypt(data []byte) (encrypted, nonce []byte, err error) {
	return aesEncrypt(data, u.authSvc.jwtSecret)
}

func (u pluginUsers) Decrypt(encrypted, nonce []byte) ([]byte, error) {
	return aesDecrypt(encrypted, nonce, u.authSvc.jwtSecret)
}

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
	// AllowQueryToken defaults to off: JWTs in URLs leak into proxy and
	// access logs. Enable explicitly in config.toml for debugging only.
	cfg := Config{Auth: AuthConfig{AllowQueryToken: false}}
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
				Rules: []any{},
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

// saveMu serialises read-modify-write cycles (plugin status hooks vs.
// admin saves) so concurrent writers cannot silently overwrite each other
// with a stale full-settings snapshot.
var settingsSaveMu sync.Mutex

// Update atomically reads the latest settings, applies mutate, and persists
// the result under the same lock as Save.
func (s *settingsService) Update(ctx context.Context, mutate func(*Settings)) error {
	settingsSaveMu.Lock()
	defer settingsSaveMu.Unlock()
	cur, err := s.store.Get(ctx)
	if err != nil {
		return err
	}
	if cur == nil {
		cur = &Settings{}
	}
	mutate(cur)
	if err := s.store.Save(ctx, cur); err != nil {
		return err
	}
	s.mu.Lock()
	s.cache = cloneSettings(cur)
	s.mu.Unlock()
	return nil
}

func (s *settingsService) Save(ctx context.Context, in *Settings) error {
	settingsSaveMu.Lock()
	defer settingsSaveMu.Unlock()
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

var (
	ErrNotFound         = newError("not_found", "resource not found")
	ErrUnauthorized     = newError("unauthorized", "unauthorized")
	ErrForbidden        = newError("forbidden", "forbidden")
	ErrPermissionDenied = ErrForbidden
	ErrInvalidInput     = newError("invalid_input", "invalid input")
	ErrConflict         = newError("conflict", "resource conflict")
	ErrInternal         = newError("internal", "internal server error")
	ErrEmptyField       = newError("empty_field", "field cannot be empty")
)

// Error is the canonical application error type.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func newError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WrapError wraps a base error with a cause and optional message override.
func WrapError(base *Error, cause error, message string) *Error {
	if base == nil {
		base = ErrInternal
	}
	m := base.Message
	if message != "" {
		m = message
	}
	return &Error{Code: base.Code, Message: m, Cause: cause}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
