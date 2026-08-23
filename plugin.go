package abyss

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
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
	// Persist first, then flip the in-memory flag: if persistence fails the
	// runtime state stays consistent with what will be loaded after restart.
	hook := m.persistenceHook
	if hook != nil {
		if err := hook(name, enabled); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.status[name] = enabled
	m.mu.Unlock()
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
