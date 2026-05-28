package abyss

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	bbolt "go.etcd.io/bbolt"
)

// ── Plugin type / metadata ────────────────────────────────────────────────────

// PluginType defines the type of a plugin.
type PluginType string

const (
	TypeFree PluginType = "free"
	TypePaid PluginType = "paid"
)

// PluginInfo describes plugin metadata.
type PluginInfo struct {
	SlugName     string
	Name         string
	Description  string
	Version      string
	Type         PluginType
	Author       string
	Dependencies []string
	Category     string
	Tags         []string
	Capabilities []string
	Priority     int
}

// Base is the interface that all plugins must implement.
type Base interface {
	Info() PluginInfo
}

// OptionProvider is kept for compatibility with plugins that still register option UIs separately.
type OptionProvider interface {
	Base
}

// ── IStatusManager ────────────────────────────────────────────────────────────

// IStatusManager defines the interface for plugin status management.
type IStatusManager interface {
	IsEnabled(name string) bool
	Enable(name string, enabled bool) error
	SetStatuses(statuses map[string]bool)
	SetPersistenceHook(hook func(name string, enabled bool) error)
}

// StatusManager is the global plugin status manager.
var StatusManager IStatusManager

// ErrStop signals plugin iteration to stop.
var ErrStop = errors.New("stop iteration")

// ── statusManager ─────────────────────────────────────────────────────────────

type statusManager struct {
	mu              sync.RWMutex
	status          map[string]bool
	persistenceHook func(name string, enabled bool) error
}

func init() {
	StatusManager = &statusManager{status: make(map[string]bool)}
}

func (m *statusManager) Enable(name string, enabled bool) error {
	target := GetBase(name)
	if enabled && target != nil {
		if err := m.checkActivationRequirements(name, target); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.status[name] = enabled
	hook := m.persistenceHook
	m.mu.Unlock()
	if hook != nil {
		if err := hook(name, enabled); err != nil {
			return err
		}
	}
	return nil
}

func (m *statusManager) checkActivationRequirements(name string, target Base) error {
	info := target.Info()
	for _, dep := range info.Dependencies {
		if !m.IsEnabled(dep) {
			return errors.New("plugin " + name + " depends on " + dep + " which is disabled")
		}
	}
	return nil
}

func (m *statusManager) SetPersistenceHook(hook func(name string, enabled bool) error) {
	m.mu.Lock()
	m.persistenceHook = hook
	m.mu.Unlock()
}

func (m *statusManager) IsEnabled(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if status, ok := m.status[name]; ok {
		return status
	}

	return false
}

func (m *statusManager) SetStatuses(statuses map[string]bool) {
	m.mu.Lock()
	for name, enabled := range statuses {
		m.status[name] = enabled
	}
	m.mu.Unlock()
}

// ── Plugin registry ───────────────────────────────────────────────────────────

// RegisterFn is a function that registers a plugin of type T.
type RegisterFn[T Base] func(p T)

// Caller is a callback invoked for each plugin of type T.
type Caller[T Base] func(p T) error

// CallFn iterates over all registered plugins of type T.
type CallFn[T Base] func(fn Caller[T]) error

// GetFn returns a plugin by its slug name.
type GetFn[T Base] func(slug string) T

var allStacks []interface {
	Sort() error
}

// MakePlugin creates (call, callAll, get, register) for type T.
func MakePlugin[T Base](super bool) (call, callAll CallFn[T], get GetFn[T], register RegisterFn[T]) {
	stack := &Stack[T]{bySlug: make(map[string]T)}
	allStacks = append(allStacks, stack)

	call = func(fn Caller[T]) error {
		stack.mu.RLock()
		defer stack.mu.RUnlock()
		for _, p := range stack.plugins {
			if !super && StatusManager != nil && !StatusManager.IsEnabled(p.Info().SlugName) {
				continue
			}
			if err := fn(p); err != nil {
				return err
			}
		}
		return nil
	}
	callAll = func(fn Caller[T]) error {
		stack.mu.RLock()
		defer stack.mu.RUnlock()
		for _, p := range stack.plugins {
			if err := fn(p); err != nil {
				return err
			}
		}
		return nil
	}
	get = func(slug string) T {
		stack.mu.RLock()
		defer stack.mu.RUnlock()
		return stack.bySlug[slug]
	}
	register = func(p T) {
		stack.mu.Lock()
		defer stack.mu.Unlock()
		slug := p.Info().SlugName
		if _, exists := stack.bySlug[slug]; exists {
			panic("plugin " + slug + " is already registered")
		}
		stack.plugins = append(stack.plugins, p)
		stack.bySlug[slug] = p
	}
	return call, callAll, get, register
}

// Stack manages a collection of plugins of the same type.
type Stack[T Base] struct {
	mu      sync.RWMutex
	plugins []T
	bySlug  map[string]T
}

// Sort sorts the plugins based on dependencies.
func (s *Stack[T]) Sort() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bases := make([]Base, 0, len(s.plugins))
	for _, p := range s.plugins {
		bases = append(bases, p)
	}
	sortedBases, err := SortPlugins(bases)
	if err != nil {
		return err
	}
	sorted := make([]T, 0, len(sortedBases))
	for _, b := range sortedBases {
		if v, ok := b.(T); ok {
			sorted = append(sorted, v)
		}
	}
	s.plugins = sorted
	return nil
}

// SortAll sorts all registered plugin stacks.
func SortAll() error {
	for _, s := range allStacks {
		if err := s.Sort(); err != nil {
			return err
		}
	}
	return nil
}

// SortPlugins topologically sorts plugins based on dependencies.
func SortPlugins(plugins []Base) ([]Base, error) {
	var sorted []Base
	visited := make(map[string]bool)
	temp := make(map[string]bool)
	pluginMap := make(map[string]Base)
	for _, p := range plugins {
		pluginMap[p.Info().SlugName] = p
	}
	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if temp[name] {
			return fmt.Errorf("cycle detected for plugin %s", name)
		}
		temp[name] = true
		p, ok := pluginMap[name]
		if !ok {
			temp[name] = false
			return nil
		}
		for _, dep := range p.Info().Dependencies {
			if err := visit(dep); err != nil {
				return err
			}
		}
		delete(temp, name)
		visited[name] = true
		sorted = append(sorted, p)
		return nil
	}
	for _, p := range plugins {
		if err := visit(p.Info().SlugName); err != nil {
			return nil, err
		}
	}
	return sorted, nil
}

// GetAll returns metadata for all registered plugins.
func GetAllPlugins() []PluginInfo {
	var infos []PluginInfo
	_ = CallBaseAll(func(p Base) error {
		infos = append(infos, p.Info())
		return nil
	})
	return infos
}

var (
	CallBase,
	CallBaseAll,
	GetBase,
	RegisterBase = MakePlugin[Base](true)
)

// ── Lifecycle ─────────────────────────────────────────────────────────────────

// StartupContext provides access to core resources during plugin initialization.
type StartupContext struct {
	Files      Files
	Users      Users
	Logger     *slog.Logger
	BaseURL    string
	Handler    HandlerWrapper
	PluginSlug string
	// StoreFactory allows creating isolated PluginStore instances per plugin slug.
	StoreFactory func(slug string) PluginStore
}

func (c *StartupContext) Store() (PluginStore, error) {
	if c == nil {
		return nil, ErrPluginNotInitialized
	}
	if c.StoreFactory == nil {
		return nil, ErrPluginNotInitialized
	}
	if c.PluginSlug == "" {
		return nil, ErrPluginNotInitialized
	}
	store := c.StoreFactory(c.PluginSlug)
	if store == nil {
		return nil, ErrPluginNotInitialized
	}
	return store, nil
}

// Plugin is the unified plugin interface.
type Plugin interface {
	Base
	Init(ctx *StartupContext) error
	Stop(ctx context.Context) error
	ConfigFields() []ConfigField
	ConfigReceiver(config []byte) error
}

var (
	CallPlugin,
	CallPluginAll,
	GetPlugin,
	RegisterPlugin = MakePlugin[Plugin](true)
)

// Register registers a plugin (Base + Plugin).
func Register(p Plugin) {
	RegisterBase(p)
	RegisterPlugin(p)
}

// RegisterOptionProvider is a compatibility no-op for legacy plugins.
func RegisterOptionProvider(p OptionProvider) {
	_ = p
}

// ConfigType represents the type of a configuration field.
type ConfigType string

const (
	ConfigTypeInput    ConfigType = "input"
	ConfigTypePassword ConfigType = "password"
	ConfigTypeTextarea ConfigType = "textarea"
	ConfigTypeCheckbox ConfigType = "checkbox"
	ConfigTypeSwitch   ConfigType = "switch"
	ConfigTypeSelect   ConfigType = "select"
	ConfigTypeNumber   ConfigType = "number"
	ConfigTypeButton   ConfigType = "button"
)

// ConfigField describes a single configuration field.
type ConfigField struct {
	Name        string         `json:"name"`
	Type        ConfigType     `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required"`
	Value       interface{}    `json:"value"`
	Options     []ConfigOption `json:"options,omitempty"`
	ReadOnly    bool           `json:"readOnly,omitempty"`
	Copyable    bool           `json:"copyable,omitempty"`
	Action      string         `json:"action,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	IconClass   string         `json:"iconClass,omitempty"`
	Row         int            `json:"row,omitempty"`
	Group       string         `json:"group,omitempty"`
}

// ConfigOption is a selectable option for ConfigTypeSelect.
type ConfigOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// CallConfig iterates enabled plugins that have configuration.
func CallConfig(fn func(p Plugin) error) error {
	return CallPlugin(fn)
}

// GetPluginConfig returns a plugin by slug.
func GetPluginConfig(slug string) Plugin {
	return GetPlugin(slug)
}

// ── Auth plugin ───────────────────────────────────────────────────────────────

// PluginAuthMethod describes an authentication method provided by a plugin.
type PluginAuthMethod struct {
	SlugName    string `json:"slugName"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Type        string `json:"type"`
	RedirectURL string `json:"redirectUrl"`
}

// PluginAuthResult represents the result of an authentication hook.
type PluginAuthResult struct {
	NeedMFA   bool
	MFAToken  string
	MFAMethod string // e.g. "otp", "sms"
	ExtraData map[string]interface{}
}

// Authenticator extends the authentication flow.
type Authenticator interface {
	Base
	// AuthMethods returns the primary authentication methods this plugin supports.
	AuthMethods() []PluginAuthMethod

	// Authenticate handles a PRIMARY authentication request.
	// Returns UserID if successful.
	Authenticate(method string, data map[string]interface{}) (uint64, error)

	// OnLoginSuccess is called after ANY primary authentication succeeds.
	// Return NeedMFA=true to trigger the MFA flow.
	OnLoginSuccess(userID uint64, r *http.Request) (*PluginAuthResult, error)

	// VerifyMFA handles the completion of a SECONDARY authentication factor.
	VerifyMFA(userID uint64, method string, data map[string]interface{}) (bool, error)

	// OnRegisterSuccess is called after a new user is registered.
	OnRegisterSuccess(userID uint64, r *http.Request) error
}

var (
	CallAuthenticator,
	CallAuthenticatorAll,
	GetAuthenticator,
	RegisterAuthenticator = MakePlugin[Authenticator](false)
)

// GetAuthMethods returns all authentication methods from enabled plugins.
func GetAuthMethods() []PluginAuthMethod {
	var methods []PluginAuthMethod
	_ = CallAuthenticator(func(a Authenticator) error {
		methods = append(methods, a.AuthMethods()...)
		return nil
	})
	return methods
}

// ── Storage plugin ────────────────────────────────────────────────────────────

// StorageTypeInfo describes a storage engine type available from a plugin.
type StorageTypeInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

// VirtualPathInfo contains the resolved storage engine and relative path.
type VirtualPathInfo struct {
	Engine       StorageEngine
	Path         string
	TUSResumable bool
}

// StorageProvider allows plugins to extend the storage system.
type StorageProvider interface {
	Base
	AvailableStorageTypes() []StorageTypeInfo
	CreateUserEngine(userID uint64) (StorageEngine, error)
	OnUserEngineInit(userID uint64, setEngine func(name string, engine StorageEngine)) error
	ResolveVirtualPath(ctx context.Context, userID uint64, filePath string) (*VirtualPathInfo, error)
	GetVirtualEntries(ctx context.Context, userID uint64) ([]*EntryInfo, error)
	MigrateStorage(ctx context.Context, sourceType, targetType string, getEngine func(uint64) (StorageEngine, error)) error
	PreflightMigration(ctx context.Context, sourceType, targetType string) error
	TUSUploadComplete(ctx context.Context, userID uint64, virtualPath string, tempReader ReadSeekCloser) error
}

// StorageProviderBase provides default no-op implementations.
type StorageProviderBase struct{}

func (StorageProviderBase) AvailableStorageTypes() []StorageTypeInfo                   { return nil }
func (StorageProviderBase) CreateUserEngine(uint64) (StorageEngine, error)             { return nil, nil } //nolint:nilnil // Default base implementation intentionally returns no engine and no error.
func (StorageProviderBase) OnUserEngineInit(uint64, func(string, StorageEngine)) error { return nil }
func (StorageProviderBase) ResolveVirtualPath(context.Context, uint64, string) (*VirtualPathInfo, error) {
	return nil, nil //nolint:nilnil // Default base implementation intentionally returns no virtual path and no error.
}
func (StorageProviderBase) GetVirtualEntries(context.Context, uint64) ([]*EntryInfo, error) {
	return nil, nil //nolint:nilnil // Default base implementation intentionally returns no entries and no error.
}
func (StorageProviderBase) MigrateStorage(context.Context, string, string, func(uint64) (StorageEngine, error)) error {
	return nil
}
func (StorageProviderBase) PreflightMigration(context.Context, string, string) error { return nil }
func (StorageProviderBase) TUSUploadComplete(context.Context, uint64, string, ReadSeekCloser) error {
	return nil
}

var (
	CallStorageProvider,
	CallStorageProviderAll,
	GetStorageProvider,
	RegisterStorageProvider = MakePlugin[StorageProvider](false)
)

// GetAvailableStorageTypes returns all storage types from enabled plugins plus "path".
func GetAvailableStorageTypes() []StorageTypeInfo {
	types := []StorageTypeInfo{
		{Name: "path", DisplayName: "settings.pathStorage", Description: "settings.storageDescription"},
	}
	_ = CallStorageProvider(func(p StorageProvider) error {
		types = append(types, p.AvailableStorageTypes()...)
		return nil
	})
	return types
}

// ResolveVirtualPath attempts to resolve a virtual path by calling all registered providers.
func ResolveVirtualPath(ctx context.Context, userID uint64, filePath string) (*VirtualPathInfo, error) {
	var result *VirtualPathInfo
	err := CallStorageProvider(func(p StorageProvider) error {
		info, err := p.ResolveVirtualPath(ctx, userID, filePath)
		if err != nil {
			return err
		}
		if info != nil {
			result = info
			return ErrStop
		}
		return nil
	})
	if err == ErrStop {
		err = nil
	}
	return result, err
}

// GetAllVirtualEntries returns virtual entries from all enabled storage providers.
func GetAllVirtualEntries(ctx context.Context, userID uint64) []*EntryInfo {
	var entries []*EntryInfo
	_ = CallStorageProvider(func(p StorageProvider) error {
		e, err := p.GetVirtualEntries(ctx, userID)
		if err == nil {
			entries = append(entries, e...)
		}
		return nil
	})
	return entries
}

// TUSUploadComplete calls all registered storage providers for a finished TUS upload.
func TUSUploadComplete(ctx context.Context, userID uint64, virtualPath string, tempReader ReadSeekCloser) error {
	return CallStorageProvider(func(p StorageProvider) error {
		return p.TUSUploadComplete(ctx, userID, virtualPath, tempReader)
	})
}

// FileInfoHook allows plugins to inject extra data into FileInfo responses.
type FileInfoHook interface {
	Base
	CallFileInfoHook(ctx context.Context, userID uint64, info *FileInfo) error
}

var (
	CallFileInfoHook,
	CallFileInfoHookAll,
	GetFileInfoHook,
	RegisterFileInfoHook = MakePlugin[FileInfoHook](false)
)

// ── Hooks ─────────────────────────────────────────────────────────────────────

// DeletionHook allows plugins to intercept file deletions.
type DeletionHook interface {
	Base
	OnDelete(ctx context.Context, userID uint64, filePath string, isDir bool, size int64) (bool, error)
}

var (
	OnDelete,
	CallDeletionHookAll,
	GetDeletionHook,
	RegisterDeletionHook = MakePlugin[DeletionHook](true)
)

// DataExporter allows plugins to contribute data to user data exports.
type DataExporter interface {
	Base
	ExportUserData(userID uint64) (map[string]interface{}, error)
}

var (
	CallDataExporter,
	CallDataExporterAll,
	GetDataExporter,
	RegisterDataExporter = MakePlugin[DataExporter](true)
)

// GCHistory represents a single GC run record.
type GCHistory struct {
	Time         time.Time `json:"time"`
	Duration     string    `json:"duration"`
	DeletedSize  int64     `json:"deletedSize"`
	DeletedCount int       `json:"deletedCount"`
	MissingCount int       `json:"missingCount"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	DryRun       bool      `json:"dryRun"`
}

// GarbageCollector allows plugins to provide storage-specific garbage collection.
type GarbageCollector interface {
	Base
	RunGC(ctx context.Context, dryRun bool) (deletedSize int64, deletedCount int, err error)
	GetStatus() (running bool, history []GCHistory)
	StartBackgroundGC(ctx context.Context, interval time.Duration)
}

var (
	CallGarbageCollector,
	CallGarbageCollectorAll,
	GetGarbageCollector,
	RegisterGarbageCollector = MakePlugin[GarbageCollector](false)
)

// ── Notification ──────────────────────────────────────────────────────────────

// NotificationType represents the type of a notification event.
type NotificationType string

const (
	NotifyUserCreated   NotificationType = "user.created"
	NotifyPasswordReset NotificationType = "password.reset"
	NotifyShareCreated  NotificationType = "share.created"
	NotifyShareRemoved  NotificationType = "share.removed"
	NotifyCustom        NotificationType = "custom"
)

// NotificationMessage represents a notification to be sent.
type NotificationMessage struct {
	Type      NotificationType       `json:"type"`
	Recipient string                 `json:"recipient"`
	Subject   string                 `json:"subject"`
	Body      string                 `json:"body"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// Notification provides a notification channel.
type Notification interface {
	Base
	Send(msg NotificationMessage) error
}

var (
	CallNotification,
	CallNotificationAll,
	GetNotification,
	RegisterNotification = MakePlugin[Notification](false)
)

// SendNotification dispatches a message to all enabled notification plugins.
func SendNotification(msg NotificationMessage) {
	_ = CallNotification(func(n Notification) error {
		return n.Send(msg)
	})
}

// ── Protocol ──────────────────────────────────────────────────────────────────

// Protocol provides an additional access protocol.
type Protocol interface {
	Base
	ProtocolName() string
	ProtocolPrefix() string
	Handler() http.Handler
}

type ProtocolAuthMode string

const (
	ProtocolAuthRequired ProtocolAuthMode = "required"
	ProtocolAuthNone     ProtocolAuthMode = "none"
)

// ProtocolAuthProvider allows protocol plugins to override auth behavior.
// Protocol handlers are authenticated by default.
type ProtocolAuthProvider interface {
	ProtocolAuthMode() ProtocolAuthMode
}

var (
	CallProtocol,
	CallProtocolAll,
	GetProtocol,
	RegisterProtocol = MakePlugin[Protocol](false)
)

// ── Router ────────────────────────────────────────────────────────────────────

// RouterGroup abstracts HTTP route registration.
type RouterGroup interface {
	Handle(pattern string, handler http.Handler) RouterGroup
	HandleFunc(pattern string, handler http.HandlerFunc) RouterGroup
	Methods(methods ...string) RouterGroup
	Group(prefix string) RouterGroup
	Use(mw ...func(http.Handler) http.Handler) RouterGroup
}

// Router allows a plugin to register its own HTTP routes.
type Router interface {
	Base
	RegisterRoutes(api, global, users RouterGroup, auth func(http.Handler) http.Handler)
}

var (
	CallRouter,
	CallRouterAll,
	GetRouter,
	RegisterRouter = MakePlugin[Router](false)
)

// ── UI ────────────────────────────────────────────────────────────────────────

// UIPageType defines how a plugin page is rendered.
type UIPageType string

const (
	UIPageTypeFull   UIPageType = "full"
	UIPageTypeTab    UIPageType = "tab"
	UIPageTypeWidget UIPageType = "widget"
	UIPageTypeModal  UIPageType = "modal"
)

// UIPage describes a frontend page or component provided by a plugin.
type UIPage struct {
	SlugName         string     `json:"slugName"`
	Name             string     `json:"name"`
	Icon             string     `json:"icon"`
	Type             UIPageType `json:"type"`
	Route            string     `json:"route"`
	Component        string     `json:"component"`
	NavPosition      string     `json:"navPosition"`
	NavOrder         int        `json:"navOrder"`
	SidebarComponent string     `json:"sidebarComponent,omitempty"`
}

// UIProvider allows a plugin to provide frontend pages and static assets.
type UIProvider interface {
	Base
	UIPages() []UIPage
	UIAssets() fs.FS
}

var (
	CallUIProvider,
	CallUIProviderAll,
	GetUIProvider,
	RegisterUIProvider = MakePlugin[UIProvider](false)
)

// GetAllUIPages returns all UI page definitions from enabled plugins.
func GetAllUIPages() []UIPage {
	var pages []UIPage
	_ = CallUIProvider(func(u UIProvider) error {
		pages = append(pages, u.UIPages()...)
		return nil
	})
	return pages
}

func pluginDeepMerge(dst, src map[string]interface{}) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := dst[k].(map[string]interface{}); ok {
				pluginDeepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

// GetPluginI18n returns combined i18n data from all registered UI plugins (enabled or not).
func GetPluginI18n() map[string]interface{} {
	result := make(map[string]interface{})
	_ = CallUIProviderAll(func(u UIProvider) error {
		fsys := u.UIAssets()
		if fsys == nil {
			return nil
		}
		data, err := fsys.Open("i18n.json")
		if err != nil {
			return nil
		}
		defer func() { _ = data.Close() }()
		var m map[string]interface{}
		if err := json.NewDecoder(data).Decode(&m); err == nil {
			pluginDeepMerge(result, m)
		}
		return nil
	})
	return result
}

// ── Services ──────────────────────────────────────────────────────────────────

// Data provides request context for plugin handlers.
type Data struct {
	UserID   uint64
	User     *User
	IsAdmin  bool
	Engine   StorageEngine
	Request  *http.Request
	Response http.ResponseWriter
}

// Files provides file operations for plugins.
type Files interface {
	GetEngine(userID uint64) (StorageEngine, error)
	WriteFile(ctx context.Context, userID uint64, path string, r io.Reader) error
	CreateDir(ctx context.Context, userID uint64, path string) error
	GetFileByID(id uint64) (*EntryInfo, error)
	ListFiles(userID uint64) ([]*EntryInfo, error)
	ResizeImage(ctx context.Context, in io.Reader, w, h int, out io.Writer) error
}

// Users provides user and security operations for plugins.
type Users interface {
	GetByID(id uint64) (*User, error)
	GetByEmail(email string) (*User, error)
	GetByUsername(username string) (*User, error)
	GetAll() ([]*User, error)
	UserIDFromRequest(r *http.Request) uint64
	IssueToken(w http.ResponseWriter, r *http.Request, user *User, ttl time.Duration) (int, error)

	// Security & Auth
	IssueMFAToken(userID uint64, method string, ttl time.Duration) (string, error)
	Encrypt(data []byte) (encrypted []byte, nonce []byte, err error)
	Decrypt(encrypted, nonce []byte) (data []byte, err error)
}

// HandlerWrapper wraps a plugin HandleFunc into a standard http.Handler.
type HandlerWrapper func(fn HandleFunc) http.Handler

// HandleFunc is the signature for plugin HTTP handlers.
type HandleFunc func(d *Data) *Response

// Response represents a standard plugin API response.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Status  int         `json:"-"`
}

// OK returns a successful response with data.
func OK(data interface{}) *Response {
	return &Response{Success: true, Data: data, Status: http.StatusOK}
}

// Fail returns an error response with status.
func Fail(status int, err interface{}) *Response {
	var msg string
	switch v := err.(type) {
	case error:
		msg = v.Error()
	case string:
		msg = v
	default:
		msg = "unknown error"
	}
	return &Response{Success: false, Error: msg, Status: status}
}

// ErrToStatus maps errors to HTTP status codes.
func ErrToStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case "not_found":
			return http.StatusNotFound
		case "unauthorized":
			return http.StatusUnauthorized
		case "forbidden":
			return http.StatusForbidden
		case "conflict":
			return http.StatusConflict
		case "invalid_input":
			return http.StatusBadRequest
		}
	}

	switch {
	case os.IsPermission(err):
		return http.StatusForbidden
	case os.IsNotExist(err):
		return http.StatusNotFound
	case os.IsExist(err):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// PluginTaskService defines the interface for managing asynchronous tasks (accessible to plugins).
type PluginTaskService interface {
	Submit(t PluginTask) (string, error)
	Cancel(id string) error
	ListByUser(userID uint64) []*TaskInfo
}

// ── Schema/Migration ──────────────────────────────────────────────────────────

// PluginMigrationTx is the transaction interface for BoltDB migrations.
type PluginMigrationTx interface {
	CreateBucketIfNotExists(name string) error
	Get(bucket string, key []byte) ([]byte, error)
	Put(bucket string, key []byte, val []byte) error
	Delete(bucket string, key []byte) error
	ForEach(bucket string, fn func(k, v []byte) error) error
}

// PluginMigration represents a versioned schema migration step.
type PluginMigration struct {
	Version int
	Migrate func(tx PluginMigrationTx) error
}

// ── PluginStore ───────────────────────────────────────────────────────────────

// PluginStore provides data persistence for plugins.
type PluginStore interface {
	Migrate(migrations []PluginMigration) error
	Put(key string, value interface{}) error
	Get(key string, v interface{}) error
	Delete(key string) error
	GetConfig() ([]byte, error)
	SaveConfig(data []byte) error
	DataDir() string
	EnsureDataDir() string
	BoltDB() *bbolt.DB
}

// ── Task plugin types ─────────────────────────────────────────────────────────

const (
	TaskStatusPending   = TaskPending
	TaskStatusRunning   = TaskRunning
	TaskStatusFinished  = TaskSucceeded
	TaskStatusFailed    = TaskFailed
	TaskStatusCancelled = TaskCanceled
)

// TaskNotifier is used by tasks to report progress updates.
type TaskNotifier interface {
	UpdateProgress(progress float64, message string)
}

// PluginTask is the interface implemented by plugins for long-running operations.
type PluginTask interface {
	Execute(ctx context.Context, notifier TaskNotifier) error
	GetInfo() *TaskInfo
}

// TaskFactory is a function that creates a task from its info.
type TaskFactory func(info *TaskInfo) (PluginTask, error)

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
	if b.closed.Load() {
		return
	}
	b.mu.RLock()
	entries, ok := b.subscribers[topic]
	b.mu.RUnlock()
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
		b.closed.Store(true)
		close(b.jobs)
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
			routerGroupAdapter{scoped, nil},
			routerGroupAdapter{global, nil},
			routerGroupAdapter{users, nil},
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

type routerGroupAdapter struct {
	r     *mux.Router
	route *mux.Route
}

func (a routerGroupAdapter) Handle(pattern string, handler http.Handler) RouterGroup {
	route := a.r.Handle(pattern, handler)
	return routerGroupAdapter{a.r, route}
}

func (a routerGroupAdapter) HandleFunc(pattern string, handler http.HandlerFunc) RouterGroup {
	route := a.r.HandleFunc(pattern, handler)
	return routerGroupAdapter{a.r, route}
}

func (a routerGroupAdapter) Methods(methods ...string) RouterGroup {
	if a.route != nil {
		a.route.Methods(methods...)
	}
	return a
}

func (a routerGroupAdapter) Group(prefix string) RouterGroup {
	return routerGroupAdapter{a.r.PathPrefix(prefix).Subrouter(), nil}
}

func (a routerGroupAdapter) Use(mw ...func(http.Handler) http.Handler) RouterGroup {
	a.r.Use(mux.MiddlewareFunc(func(next http.Handler) http.Handler {
		h := next
		for i := len(mw) - 1; i >= 0; i-- {
			h = mw[i](h)
		}
		return h
	}))
	return a
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
