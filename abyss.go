// Package abyss is the library entry point for abyss.
//
// External projects (e.g. the root abyss binary) call [Main] to start the server:
//
//	package main
//
//	import "github.com/nuln/abyss-core"
//
//	func main() {
//	    abyss.Main()
//	}
//
// Plugins are assembled at compile time via blank imports in a separate build-tag file.
package abyss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"

	"github.com/nuln/abyss-core/www"
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
		s, err := deps.settingsSvc.Get(ctx)
		if err != nil {
			return err
		}
		if s.PluginStatuses == nil {
			s.PluginStatuses = make(map[string]bool)
		}
		s.PluginStatuses[name] = enabled
		return deps.settingsSvc.Save(ctx, s)
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
	result, err := u.authSvc.issueAuthResult(r.Context(), user, r.UserAgent())
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
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(u.authSvc.jwtSecret)
}

func (u pluginUsers) VerifyMFAToken(tokenStr string) (userID uint64, method string, err error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
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
