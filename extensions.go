package abyss

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"time"

	bbolt "go.etcd.io/bbolt"
)

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
