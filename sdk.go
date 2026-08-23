package abyss

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

// ── Skeleton ──────────────────────────────────────────────────────────────────

var pluginValidate *validator.Validate

func init() {
	pluginValidate = validator.New()
}

// ErrPluginNotInitialized is returned when a plugin method is called before Init.
var ErrPluginNotInitialized = errors.New("plugin not initialized")

// ErrNoPluginConfig is returned by ExtractAndValidateConfig when no nested config section exists.
var ErrNoPluginConfig = errors.New("no plugin config section found")

// Logger is an alias for the global logger interface.
type Logger = *slog.Logger

// Skeleton provides a base implementation for plugins.
type Skeleton struct {
	Ctx       *StartupContext
	RawConfig []byte
}

func (s *Skeleton) Init(ctx *StartupContext) error {
	s.Ctx = ctx
	return nil
}

func (s *Skeleton) Stop(_ context.Context) error {
	return nil
}

func (s *Skeleton) ConfigFields() []ConfigField {
	return nil
}

func (s *Skeleton) ConfigReceiver(cfg []byte) error {
	s.RawConfig = cfg
	return nil
}

func (s *Skeleton) GetConfig(v interface{}) error {
	if len(s.RawConfig) == 0 {
		return nil
	}
	if err := json.Unmarshal(s.RawConfig, v); err != nil {
		return err
	}
	return s.Validate(v)
}

func (s *Skeleton) Validate(v interface{}) error {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	return pluginValidate.Struct(v)
}

func (s *Skeleton) RegisterRoutes(api, global, users RouterGroup, auth func(http.Handler) http.Handler) {
}

func (s *Skeleton) AvailableStorageTypes() []StorageTypeInfo {
	return nil
}

func (s *Skeleton) CreateUserEngine(_ uint64) (StorageEngine, error) {
	return nil, nil //nolint:nilnil // Skeleton plugin does not provide storage engine by default.
}

func (s *Skeleton) OnUserEngineInit(_ uint64, _ func(name string, engine StorageEngine)) error {
	return nil
}

func (s *Skeleton) Put(key string, value interface{}) error {
	if s.Ctx == nil {
		return ErrPluginNotInitialized
	}
	store, err := s.Ctx.Store()
	if err != nil {
		return err
	}
	return store.Put(key, value)
}

func (s *Skeleton) Get(key string, v interface{}) error {
	if s.Ctx == nil {
		return ErrPluginNotInitialized
	}
	store, err := s.Ctx.Store()
	if err != nil {
		return err
	}
	return store.Get(key, v)
}

func (s *Skeleton) Delete(key string) error {
	if s.Ctx == nil {
		return ErrPluginNotInitialized
	}
	store, err := s.Ctx.Store()
	if err != nil {
		return err
	}
	return store.Delete(key)
}

func (s *Skeleton) GetDataPath(_ string) (string, error) {
	if s.Ctx == nil {
		return "", ErrPluginNotInitialized
	}
	store, err := s.Ctx.Store()
	if err != nil {
		return "", err
	}
	return store.DataDir(), nil
}

func (s *Skeleton) EnsureDataPath() (string, error) {
	if s.Ctx == nil {
		return "", ErrPluginNotInitialized
	}
	store, err := s.Ctx.Store()
	if err != nil {
		return "", err
	}
	return store.EnsureDataDir(), nil
}

// GetConfigTyped reads the plugin's configuration and unmarshals it into type T.
func GetConfigTyped[T any](s *Skeleton) (*T, error) {
	if s.Ctx == nil {
		return nil, ErrPluginNotInitialized
	}
	store, err := s.Ctx.Store()
	if err != nil {
		return nil, err
	}
	data, err := store.GetConfig()
	if err != nil {
		return nil, err
	}
	var cfg T
	if len(data) == 0 {
		return &cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ExtractPluginConfig extracts plugin-specific configuration from a raw config map.
func ExtractPluginConfig(cfg map[string]interface{}, slug string) map[string]interface{} {
	if plugins, ok := cfg["plugins"].(map[string]interface{}); ok {
		if pluginCfg, ok := plugins[slug].(map[string]interface{}); ok {
			return pluginCfg
		}
	}
	return nil
}

// ExtractAndValidateConfig handles extraction, unmarshaling, and validation in one go.
func ExtractAndValidateConfig(cfg map[string]interface{}, slug string, v interface{}) error {
	pluginCfg := ExtractPluginConfig(cfg, slug)
	if pluginCfg == nil {
		return ErrNoPluginConfig
	}
	data, err := json.Marshal(pluginCfg)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return pluginValidate.Struct(v)
}

// ── Service/Store registries ──────────────────────────────────────────────────

var pluginServices = struct {
	mu sync.RWMutex
	m  map[string]interface{}
}{m: make(map[string]interface{})}

func SetService(name string, svc interface{}) {
	pluginServices.mu.Lock()
	defer pluginServices.mu.Unlock()
	pluginServices.m[name] = svc
}

func GetService(name string) interface{} {
	pluginServices.mu.RLock()
	defer pluginServices.mu.RUnlock()
	return pluginServices.m[name]
}

var pluginStores = struct {
	mu sync.RWMutex
	m  map[string]interface{}
}{m: make(map[string]interface{})}

func SetStore(slug string, store interface{}) {
	pluginStores.mu.Lock()
	defer pluginStores.mu.Unlock()
	pluginStores.m[slug] = store
}

func GetStore(slug string) interface{} {
	pluginStores.mu.RLock()
	defer pluginStores.mu.RUnlock()
	return pluginStores.m[slug]
}

func GetServiceTyped[T any](name string) (T, bool) {
	svc := GetService(name)
	v, ok := svc.(T)
	return v, ok
}

func GetStoreTyped[T any](slug string) (T, bool) {
	s := GetStore(slug)
	v, ok := s.(T)
	return v, ok
}

// ── EventBus ──────────────────────────────────────────────────────────────────

const (
	TopicUserCreated = "user:created"
	TopicUserUpdated = "user:updated"
	TopicUserDeleted = "user:deleted"
	TopicFileCreated = "file:created"
	TopicFileUpdated = "file:updated"
	TopicFileDeleted = "file:deleted"
)

// EventHandler is a function that handles an event.
type EventHandler func(data interface{})

// Subscription represents an active event subscription.
type Subscription struct {
	topic string
	id    uint64
}

// EventBus defines the interface for an event-driven plugin system.
type EventBus interface {
	Publish(topic string, data interface{})
	Subscribe(topic string, handler EventHandler) Subscription
	Unsubscribe(sub Subscription)
}

type subscriberEntry struct {
	id      uint64
	handler EventHandler
}

// MemoryEventBus is a simple in-memory implementation of EventBus.
type MemoryEventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]subscriberEntry
	nextID      atomic.Uint64
	jobs        chan eventJob
	wg          sync.WaitGroup
	closeOnce   sync.Once
	closed      atomic.Bool
}

type eventJob struct {
	handler EventHandler
	data    interface{}
	topic   string
}

// NewMemoryEventBus creates a new in-memory event bus.
func NewMemoryEventBus() *MemoryEventBus {
	b := &MemoryEventBus{
		subscribers: make(map[string][]subscriberEntry),
		jobs:        make(chan eventJob, 1024),
	}
	const workers = 8
	for i := 0; i < workers; i++ {
		b.wg.Add(1)
		go b.worker()
	}
	return b
}

func (b *MemoryEventBus) Publish(topic string, data interface{}) {
	// Hold the read lock while sending so Close cannot close(b.jobs)
	// concurrently with a send (which would panic the process).
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed.Load() {
		return
	}
	entries, ok := b.subscribers[topic]
	if !ok {
		return
	}
	entriesSnapshot := append([]subscriberEntry(nil), entries...)
	for _, entry := range entriesSnapshot {
		select {
		case b.jobs <- eventJob{handler: entry.handler, data: data, topic: topic}:
		default:
			slog.Warn("event queue full, dropping event", "topic", topic)
		}
	}
}

func (b *MemoryEventBus) worker() {
	defer b.wg.Done()
	for job := range b.jobs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("event handler panicked", "topic", job.topic, "error", r)
				}
			}()
			job.handler(job.data)
		}()
	}
}

func (b *MemoryEventBus) Close() {
	b.closeOnce.Do(func() {
		// Take the write lock so a concurrent Publish cannot be mid-send.
		b.mu.Lock()
		if !b.closed.Swap(true) {
			close(b.jobs)
		}
		b.mu.Unlock()
		b.wg.Wait()
	})
}

func (b *MemoryEventBus) Subscribe(topic string, handler EventHandler) Subscription {
	id := b.nextID.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[topic] = append(b.subscribers[topic], subscriberEntry{id: id, handler: handler})
	return Subscription{topic: topic, id: id}
}

func (b *MemoryEventBus) Unsubscribe(sub Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries, ok := b.subscribers[sub.topic]
	if !ok {
		return
	}
	for i, e := range entries {
		if e.id == sub.id {
			b.subscribers[sub.topic] = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

// ── Bridge: HTTP ──────────────────────────────────────────────────────────────

// mountPluginHTTP wires all HTTP-capable plugins into the provided mux.Router.
func mountPluginHTTP(r *mux.Router, authMW func(http.Handler) http.Handler) {
	api := r.PathPrefix("/api").Subrouter()
	global := api
	users := api.PathPrefix("/users").Subrouter()

	// Use CallRouterAll so that routes are registered for ALL plugins regardless of enabled status.
	// This ensures that enabling a plugin via the admin UI takes effect immediately without a restart.
	_ = CallRouterAll(func(p Router) error {
		slug := p.Info().SlugName
		scoped := api.PathPrefix("/" + slug).Subrouter()
		p.RegisterRoutes(
			routerGroupAdapter{r: scoped},
			routerGroupAdapter{r: global},
			routerGroupAdapter{r: users},
			authMW,
		)
		return nil
	})

	_ = CallUIProviderAll(func(p UIProvider) error {
		mountPluginUIAssets(r, p)
		return nil
	})

	api.HandleFunc("/plugins/ui", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GetAllUIPages())
	}).Methods(http.MethodGet)

	api.HandleFunc("/plugins/i18n", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GetPluginI18n())
	}).Methods(http.MethodGet)

	api.HandleFunc("/plugins/list", func(w http.ResponseWriter, _ *http.Request) {
		slog.Debug("fetching plugin list")
		type entry struct {
			SlugName      string `json:"slugName"`
			Name          string `json:"name"`
			Description   string `json:"description"`
			Version       string `json:"version"`
			Type          string `json:"type"`
			Enabled       bool   `json:"enabled"`
			HasUI         bool   `json:"hasUI"`
			HasConfig     bool   `json:"hasConfig"`
			RequireConfig bool   `json:"requireConfig"`
		}
		var list []entry
		_ = CallBaseAll(func(p Base) error {
			info := p.Info()
			slug := info.SlugName
			_, hasUI := p.(UIProvider)
			hasConfig := false
			if pi, ok := p.(Plugin); ok {
				fields := pi.ConfigFields()
				hasConfig = len(fields) > 0
			}
			e := entry{
				SlugName:      slug,
				Name:          info.Name,
				Description:   info.Description,
				Version:       info.Version,
				Type:          string(info.Type),
				Enabled:       StatusManager.IsEnabled(slug),
				HasUI:         hasUI,
				HasConfig:     hasConfig,
				RequireConfig: false,
			}
			slog.Debug("plugin entry", "slug", slug, "enabled", e.Enabled, "hasUI", e.HasUI)
			list = append(list, e)
			return nil
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	}).Methods(http.MethodGet)

	api.Handle("/settings/plugins/{slug}/enable", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !AuthIsAdminFromContext(r.Context()) {
			WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
			return
		}
		slug := mux.Vars(r)["slug"]
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := DecodeJSON(r, &req); err != nil {
			WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
			return
		}
		if err := StatusManager.Enable(slug, req.Enabled); err != nil {
			WriteJSON(w, http.StatusBadRequest, ErrorResponse(err.Error()))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))).Methods(http.MethodPost)

	api.Handle("/settings/plugins/{slug}/config", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !AuthIsAdminFromContext(r.Context()) {
			WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
			return
		}
		slug := mux.Vars(r)["slug"]
		p := GetPlugin(slug)
		if p == nil {
			WriteJSON(w, http.StatusNotFound, ErrorResponse("plugin not found"))
			return
		}
		fields := p.ConfigFields()
		WriteJSON(w, http.StatusOK, fields)
	}))).Methods(http.MethodGet)

	api.Handle("/settings/plugins/{slug}/config", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !AuthIsAdminFromContext(r.Context()) {
			WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
			return
		}
		slug := mux.Vars(r)["slug"]
		p := GetPlugin(slug)
		if p == nil {
			WriteJSON(w, http.StatusNotFound, ErrorResponse("plugin not found"))
			return
		}
		var config map[string]interface{}
		if err := DecodeJSON(r, &config); err != nil {
			WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
			return
		}
		data, _ := json.Marshal(config)
		if err := p.ConfigReceiver(data); err != nil {
			WriteJSON(w, http.StatusBadRequest, ErrorResponse(err.Error()))
			return
		}
		if store, ok := GetStoreTyped[PluginStore](slug); ok && store != nil {
			if err := store.SaveConfig(data); err != nil {
				WriteJSON(w, http.StatusInternalServerError, ErrorResponse("failed to persist plugin config"))
				return
			}
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))).Methods(http.MethodPut)

	api.HandleFunc("/auth/methods", func(w http.ResponseWriter, _ *http.Request) {
		var methods []PluginAuthMethod
		_ = CallAuthenticator(func(p Authenticator) error {
			methods = append(methods, p.AuthMethods()...)
			return nil
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(methods)
	}).Methods(http.MethodGet)
}

// mountPluginProtocols wires all Protocol plugins into the router.
func mountPluginProtocols(r *mux.Router, authMW func(http.Handler) http.Handler) {
	_ = CallProtocolAll(func(p Protocol) error {
		prefix := p.ProtocolPrefix()
		handler := p.Handler()
		if prefix == "" || handler == nil {
			return nil
		}
		mode := ProtocolAuthRequired
		if ap, ok := p.(ProtocolAuthProvider); ok {
			if ap.ProtocolAuthMode() == ProtocolAuthNone {
				mode = ProtocolAuthNone
			}
		}

		// Capture slug once at registration time for the dynamic check closure.
		slug := p.Info().SlugName

		// Wrap with a dynamic enabled check so plugins can be toggled without restart.
		dynamicHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !StatusManager.IsEnabled(slug) {
				http.Error(w, "Plugin disabled", http.StatusForbidden)
				return
			}
			handler.ServeHTTP(w, req)
		})

		// r is already the BaseURL subrouter, so StripPrefix only needs the protocol prefix.
		wrapped := http.StripPrefix(prefix, dynamicHandler)
		if mode == ProtocolAuthRequired {
			wrapped = authMW(wrapped)
		}
		r.PathPrefix(prefix).Handler(wrapped)
		return nil
	})
}

// mountPluginTaskService registers the taskService as a PluginTaskService.
func mountPluginTaskService(svc *taskService) {
	SetService("task", &corePluginTaskService{svc: svc})
}

// corePluginTaskService adapts *taskService to PluginTaskService.
type corePluginTaskService struct {
	svc *taskService
}

func (c *corePluginTaskService) Submit(t PluginTask) (string, error) {
	info := t.GetInfo()
	if info == nil {
		return "", errors.New("task info is nil")
	}
	return c.svc.Submit(context.Background(), info.Name, info.UserID, func(ctx context.Context) error {
		return t.Execute(ctx, &noopTaskNotifier{})
	})
}

func (c *corePluginTaskService) Cancel(id string) error {
	return c.svc.Cancel(id)
}

func (c *corePluginTaskService) ListByUser(userID uint64) []*TaskInfo {
	tasks, _ := c.svc.ListByUser(context.Background(), userID)
	return tasks
}

type noopTaskNotifier struct{}

func (*noopTaskNotifier) UpdateProgress(_ float64, _ string) {}

// ── Bridge: data export functions (exported) ──────────────────────────────────

// RunDeletionHooks calls all DeletionHook plugins for a given file.
func RunDeletionHooks(ctx context.Context, userID uint64, filePath string, isDir bool, size int64) (bool, error) {
	var handled bool
	err := OnDelete(func(p DeletionHook) error {
		ok, err := p.OnDelete(ctx, userID, filePath, isDir, size)
		if err != nil {
			return err
		}
		if ok {
			handled = true
			return ErrStop
		}
		return nil
	})
	if err == ErrStop {
		err = nil
	}
	return handled, err
}

// RunGarbageCollectors invokes all GarbageCollector plugins sequentially.
func RunGarbageCollectors(ctx context.Context, dryRun bool) {
	_ = CallGarbageCollector(func(p GarbageCollector) error {
		_, _, _ = p.RunGC(ctx, dryRun)
		return nil
	})
}

// ExportUserData aggregates data exports from all DataExporter plugins for a user.
func ExportUserData(userID uint64) map[string]interface{} {
	result := make(map[string]interface{})
	_ = CallDataExporter(func(p DataExporter) error {
		data, err := p.ExportUserData(userID)
		if err != nil {
			return nil
		}
		result[p.Info().SlugName] = data
		return nil
	})
	return result
}

// ── routerGroupAdapter ────────────────────────────────────────────────────────

// routerGroupAdapter adapts mux routers to the plugin RouterGroup contract.
// Unlike gorilla's Subrouter.Use (which affects the whole subtree), the
// middleware chain is group-local: Use only wraps routes registered through
// this adapter or one of its Group() children.
type routerGroupAdapter struct {
	r     *mux.Router
	route *mux.Route
	mws   []func(http.Handler) http.Handler
}

func (a routerGroupAdapter) withMiddlewares(handler http.Handler) http.Handler {
	for i := len(a.mws) - 1; i >= 0; i-- {
		handler = a.mws[i](handler)
	}
	return handler
}

func (a routerGroupAdapter) Handle(pattern string, handler http.Handler) RouterGroup {
	route := a.r.Handle(pattern, a.withMiddlewares(handler))
	return routerGroupAdapter{a.r, route, a.mws}
}

func (a routerGroupAdapter) HandleFunc(pattern string, handler http.HandlerFunc) RouterGroup {
	route := a.r.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		a.withMiddlewares(handler).ServeHTTP(w, r)
	})
	return routerGroupAdapter{a.r, route, a.mws}
}

func (a routerGroupAdapter) Methods(methods ...string) RouterGroup {
	if a.route != nil {
		a.route.Methods(methods...)
	}
	return a
}

func (a routerGroupAdapter) Group(prefix string) RouterGroup {
	return routerGroupAdapter{a.r.PathPrefix(prefix).Subrouter(), nil, a.mws}
}

func (a routerGroupAdapter) Use(mw ...func(http.Handler) http.Handler) RouterGroup {
	chain := make([]func(http.Handler) http.Handler, 0, len(a.mws)+len(mw))
	chain = append(chain, a.mws...)
	chain = append(chain, mw...)
	return routerGroupAdapter{a.r, a.route, chain}
}

// ── Manager ───────────────────────────────────────────────────────────────────

// Manager manages the lifecycle of all registered plugins.
type Manager struct{}

// NewManager creates a new plugin Manager.
func NewManager() *Manager {
	return &Manager{}
}

// Init initializes all registered plugins with the provided startup context.
func (m *Manager) Init(ctx *StartupContext) error {
	return CallPluginAll(func(p Plugin) error {
		slug := p.Info().SlugName
		pluginCtx := *ctx
		pluginCtx.PluginSlug = slug
		if store, err := pluginCtx.Store(); err == nil && store != nil {
			SetStore(slug, store)
			if data, err := store.GetConfig(); err == nil && len(data) > 0 {
				if err := p.ConfigReceiver(data); err != nil {
					return fmt.Errorf("plugin %s load config: %w", slug, err)
				}
			}
		}
		if err := p.Init(&pluginCtx); err != nil {
			return fmt.Errorf("plugin %s init: %w", slug, err)
		}
		return nil
	})
}

// StartAll starts all enabled plugins.
func (m *Manager) StartAll(_ context.Context) error {
	return nil
}

// StopAll stops all registered plugins.
func (m *Manager) StopAll(ctx context.Context) error {
	return CallPluginAll(func(p Plugin) error {
		_ = p.Stop(ctx)
		return nil
	})
}

// ResetPluginGlobalsForTests clears mutable plugin globals for test isolation.
func ResetPluginGlobalsForTests() {
	pluginServices.mu.Lock()
	pluginServices.m = make(map[string]interface{})
	pluginServices.mu.Unlock()

	pluginStores.mu.Lock()
	pluginStores.m = make(map[string]interface{})
	pluginStores.mu.Unlock()
}

func mountPluginUIAssets(r *mux.Router, p UIProvider) {
	slug := p.Info().SlugName
	assets := p.UIAssets()
	if assets == nil {
		return
	}
	sub, err := fs.Sub(assets, ".")
	if err != nil {
		return
	}
	prefix := "/static/plugins/" + slug
	fsServer := http.FileServer(http.FS(sub))
	r.PathPrefix(prefix + "/").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		}

		// Robustly strip the prefix including any parent router prefixes (like BaseURL)
		if i := strings.Index(r.URL.Path, prefix+"/"); i != -1 {
			r.URL.Path = r.URL.Path[i+len(prefix):]
		}

		fsServer.ServeHTTP(w, r)
	}))
}
