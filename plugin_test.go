package abyss

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Shared stubs ──────────────────────────────────────────────────────────────

type stubPlugin struct {
	slug string
	deps []string
	typ  PluginType
}

func (s *stubPlugin) Info() PluginInfo {
	pt := s.typ
	if pt == "" {
		pt = TypeFree
	}
	return PluginInfo{
		SlugName:     s.slug,
		Type:         pt,
		Dependencies: s.deps,
	}
}

func newStub(slug string, deps ...string) Base {
	return &stubPlugin{slug: slug, deps: deps}
}

func newPaidStub(slug string, deps ...string) Base {
	return &stubPlugin{slug: slug, typ: TypePaid, deps: deps}
}

// ── interfaces_test ───────────────────────────────────────────────────────────

type stubAuthenticator struct {
	slug    string
	methods []PluginAuthMethod
	login   *PluginAuthResult
	err     error
}

func (s *stubAuthenticator) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubAuthenticator) AuthMethods() []PluginAuthMethod {
	return s.methods
}

func (s *stubAuthenticator) Authenticate(_ string, _ map[string]interface{}) (uint64, error) {
	if s.login != nil {
		return 1, s.err
	}
	return 0, s.err
}

func (s *stubAuthenticator) OnLoginSuccess(_ uint64, _ *http.Request) (*PluginAuthResult, error) {
	return s.login, s.err
}

func (s *stubAuthenticator) VerifyMFA(_ uint64, _ string, _ map[string]interface{}) (bool, error) {
	return s.err == nil, s.err
}

func (s *stubAuthenticator) OnRegisterSuccess(_ uint64, _ *http.Request) error {
	return s.err
}

type stubNotification struct {
	slug string
	sent *[]NotificationMessage
	err  error
}

func (s *stubNotification) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubNotification) Send(msg NotificationMessage) error {
	if s.sent != nil {
		*s.sent = append(*s.sent, msg)
	}
	return s.err
}

type stubDeletionHook struct {
	slug    string
	handled bool
	err     error
}

func (s *stubDeletionHook) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubDeletionHook) OnDelete(_ context.Context, _ uint64, _ string, _ bool, _ int64) (bool, error) {
	return s.handled, s.err
}

type stubConfigPlugin struct {
	slug   string
	config []byte
	inits  int
}

func (s *stubConfigPlugin) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubConfigPlugin) Init(_ *StartupContext) error {
	s.inits++
	return nil
}

func (s *stubConfigPlugin) Stop(_ context.Context) error { return nil }

func (s *stubConfigPlugin) ConfigFields() []ConfigField { return nil }

func (s *stubConfigPlugin) ConfigReceiver(config []byte) error {
	s.config = append([]byte(nil), config...)
	return nil
}

func TestGetAuthMethods_ReturnsEmptyWhenNothingRegistered(t *testing.T) {
	oldCall := CallAuthenticator
	t.Cleanup(func() {
		CallAuthenticator = oldCall
	})

	call, _, _, _ := MakePlugin[Authenticator](false)
	CallAuthenticator = call

	assert.Empty(t, GetAuthMethods())
}

func TestGetAuthMethods_AggregatesAcrossAuthenticators(t *testing.T) {
	oldCall := CallAuthenticator
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallAuthenticator = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Authenticator](false)
	register(&stubAuthenticator{slug: "passkey", methods: []PluginAuthMethod{{SlugName: "passkey", Name: "Passkey"}}})
	register(&stubAuthenticator{slug: "otp", methods: []PluginAuthMethod{{SlugName: "otp", Name: "OTP"}}})
	CallAuthenticator = call

	methods := GetAuthMethods()
	require.Len(t, methods, 2)
	assert.Equal(t, []string{"passkey", "otp"}, []string{methods[0].SlugName, methods[1].SlugName})
}

func TestSendNotification_DispatchesToAllRegisteredNotifications(t *testing.T) {
	oldCall := CallNotification
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallNotification = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Notification](false)
	var sentA []NotificationMessage
	var sentB []NotificationMessage
	register(&stubNotification{slug: "smtp", sent: &sentA})
	register(&stubNotification{slug: "webhook", sent: &sentB})
	CallNotification = call

	msg := NotificationMessage{Type: NotifyUserCreated, Recipient: "u1", Subject: "subject", Body: "body"}
	SendNotification(msg)

	require.Len(t, sentA, 1)
	require.Len(t, sentB, 1)
	assert.Equal(t, msg, sentA[0])
	assert.Equal(t, msg, sentB[0])
}

func TestSendNotification_SwallowsPluginError(t *testing.T) {
	oldCall := CallNotification
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallNotification = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Notification](false)
	var sent []NotificationMessage
	register(&stubNotification{slug: "smtp", sent: &sent, err: errors.New("send failed")})
	CallNotification = call

	assert.NotPanics(t, func() {
		SendNotification(NotificationMessage{Type: NotifyPasswordReset, Recipient: "u1"})
	})
	require.Len(t, sent, 1)
}

func TestDeletionHook_MakePluginStopsAfterHandled(t *testing.T) {
	call, _, _, register := MakePlugin[DeletionHook](true)
	seen := 0
	register(&stubDeletionHook{slug: "trash", handled: true})
	register(&stubDeletionHook{slug: "audit", handled: false})

	err := call(func(h DeletionHook) error {
		seen++
		handled, err := h.OnDelete(context.Background(), 1, "/tmp/file", false, 123)
		if err != nil {
			return err
		}
		if handled {
			return errors.New("handled")
		}
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, 1, seen)
}

func TestMemoryEventBus_Close(t *testing.T) {
	bus := NewMemoryEventBus()
	var calls atomic.Int32

	_ = bus.Subscribe("topic", func(_ interface{}) {
		calls.Add(1)
	})

	bus.Publish("topic", "first")
	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, time.Second, 10*time.Millisecond)

	bus.Close()
	bus.Publish("topic", "second")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), calls.Load())
}

// ── registry_test ─────────────────────────────────────────────────────────────

func TestPlugin_Info_Paid(t *testing.T) {
	p := newPaidStub("paid-plugin", "dep1")
	info := p.Info()
	assert.Equal(t, "paid-plugin", info.SlugName)
	assert.Equal(t, TypePaid, info.Type)
	assert.Contains(t, info.Dependencies, "dep1")
}

func TestSortPlugins_NoDependencies(t *testing.T) {
	plugins := []Base{
		newStub("a"),
		newStub("b"),
		newStub("c"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	assert.Len(t, sorted, 3)
	slugs := make(map[string]bool)
	for _, p := range sorted {
		slugs[p.Info().SlugName] = true
	}
	assert.True(t, slugs["a"])
	assert.True(t, slugs["b"])
	assert.True(t, slugs["c"])
}

func TestSortPlugins_Simple(t *testing.T) {
	plugins := []Base{
		newStub("b", "a"),
		newStub("a"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	require.Len(t, sorted, 2)

	indexA := -1
	indexB := -1
	for i, p := range sorted {
		switch p.Info().SlugName {
		case "a":
			indexA = i
		case "b":
			indexB = i
		}
	}
	assert.Less(t, indexA, indexB, "plugin a must come before plugin b")
}

func TestSortPlugins_MultiLevel(t *testing.T) {
	plugins := []Base{
		newStub("c", "b"),
		newStub("a"),
		newStub("b", "a"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	require.Len(t, sorted, 3)

	idx := map[string]int{}
	for i, p := range sorted {
		idx[p.Info().SlugName] = i
	}
	assert.Less(t, idx["a"], idx["b"])
	assert.Less(t, idx["b"], idx["c"])
}

func TestSortPlugins_CircularDependency(t *testing.T) {
	plugins := []Base{
		newStub("a", "b"),
		newStub("b", "a"),
	}
	_, err := SortPlugins(plugins)
	assert.Error(t, err, "circular dependency should be detected")
}

func TestSortPlugins_UnknownDependencyIsIgnored(t *testing.T) {
	plugins := []Base{
		newStub("a"),
		newStub("b", "external"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	assert.Len(t, sorted, 2)
}

func TestStack_Register_And_Get(t *testing.T) {
	call, _, get, register := MakePlugin[Base](true)

	pa := newStub("test-reg-a")
	pb := newStub("test-reg-b")
	register(pa)
	register(pb)

	retrieved := get("test-reg-a")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-reg-a", retrieved.Info().SlugName)

	count := 0
	err := call(func(_ Base) error {
		count++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestStack_DuplicateRegistrationPanics(t *testing.T) {
	_, _, _, register := MakePlugin[Base](true)

	register(newStub("dup-slug"))
	assert.Panics(t, func() {
		register(newStub("dup-slug"))
	}, "registering the same slug twice should panic")
}

func TestStack_ConcurrentRegister_Safe(t *testing.T) {
	_, _, _, register := MakePlugin[Base](true)

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		slug := "concurrent-" + string(rune('a'+i))
		go func(s string) {
			defer wg.Done()
			register(&stubPlugin{slug: s})
		}(slug)
	}
	wg.Wait()
}

func TestStack_GetNonExistent_ReturnsNil(t *testing.T) {
	_, _, get, _ := MakePlugin[Base](true)
	result := get("does-not-exist")
	var zero Base
	assert.Equal(t, zero, result)
}

// ── manager_test ──────────────────────────────────────────────────────────────

func newTestManager() *statusManager {
	m := &statusManager{status: make(map[string]bool)}
	StatusManager = m
	return m
}

func registerNewStub(slug string, deps ...string) {
	RegisterBase(&stubPlugin{slug: slug, typ: TypeFree, deps: deps})
}

func TestStatusManager_Plugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-free"
	registerNewStub(slug)

	assert.False(t, m.IsEnabled(slug), "plugin should be disabled by default")
}

func TestStatusManager_PaidPlugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-paid"
	RegisterBase(&stubPlugin{slug: slug, typ: TypePaid})

	assert.False(t, m.IsEnabled(slug), "paid (pro) plugin should be disabled by default")
}

func TestStatusManager_UnknownPlugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	assert.False(t, m.IsEnabled("totally-unknown"), "fallback for unknown plugin is false")
}

func TestStatusManager_Enable_FreePlugin(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-fp"
	registerNewStub(slug)

	require.NoError(t, m.Enable(slug, true))
	assert.True(t, m.IsEnabled(slug))

	require.NoError(t, m.Enable(slug, false))
	assert.False(t, m.IsEnabled(slug))
}

func TestStatusManager_Enable_DependencyNotActive(t *testing.T) {
	m := newTestManager()
	slugA := t.Name() + "-A"
	slugB := t.Name() + "-B"
	registerNewStub(slugA)
	registerNewStub(slugB, slugA)

	require.NoError(t, m.Enable(slugA, false))

	err := m.Enable(slugB, true)
	assert.Error(t, err, "should fail when dependency is disabled")
}

func TestStatusManager_SetStatuses(t *testing.T) {
	m := newTestManager()
	slugA := t.Name() + "-a"
	slugB := t.Name() + "-b"
	registerNewStub(slugA)
	registerNewStub(slugB)

	m.SetStatuses(map[string]bool{
		slugA: true,
		slugB: false,
	})

	assert.True(t, m.IsEnabled(slugA))
	assert.False(t, m.IsEnabled(slugB))
}

func TestStatusManager_PersistenceHook_Called(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-hook"
	registerNewStub(slug)

	called := false
	m.SetPersistenceHook(func(name string, enabled bool) error {
		if name == slug {
			called = true
		}
		return nil
	})

	require.NoError(t, m.Enable(slug, true))
	assert.True(t, called, "persistence hook should have been called")
}

func TestStatusManager_PersistenceHook_Error_PropagatesError(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-hookerr"
	registerNewStub(slug)

	m.SetPersistenceHook(func(_ string, _ bool) error {
		return errors.New("storage failure")
	})

	err := m.Enable(slug, true)
	assert.Error(t, err)
}

func TestStatusManager_IsEnabled_RaceCondition(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-race"
	registerNewStub(slug)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = m.IsEnabled(slug)
		}()
		go func() {
			defer wg.Done()
			_ = m.Enable(slug, true)
		}()
	}
	wg.Wait()
}

// ── skeleton_test ─────────────────────────────────────────────────────────────

func TestSkeleton_Init_SetsCtx(t *testing.T) {
	s := &Skeleton{}
	ctx := &StartupContext{}
	err := s.Init(ctx)
	require.NoError(t, err)
	assert.Equal(t, ctx, s.Ctx)
}

func TestSkeleton_Stop_IsNoop(t *testing.T) {
	s := &Skeleton{Ctx: &StartupContext{}}
	err := s.Stop(context.Background())
	assert.NoError(t, err)
}

func TestSkeleton_ConfigFields_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	assert.Nil(t, s.ConfigFields())
}

func TestSkeleton_ConfigReceiver_StoresBytes(t *testing.T) {
	s := &Skeleton{}
	data := []byte(`{"key":"value"}`)
	err := s.ConfigReceiver(data)
	require.NoError(t, err)
	assert.Equal(t, data, s.RawConfig)
}

func TestSkeleton_GetConfig_Empty(t *testing.T) {
	s := &Skeleton{}
	var cfg map[string]string
	err := s.GetConfig(&cfg)
	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestSkeleton_GetConfig_Unmarshal(t *testing.T) {
	s := &Skeleton{}
	raw, _ := json.Marshal(map[string]string{"foo": "bar"})
	s.RawConfig = raw

	var cfg map[string]string
	err := s.GetConfig(&cfg)
	require.NoError(t, err)
	assert.Equal(t, "bar", cfg["foo"])
}

func TestSkeleton_GetConfig_InvalidJSON(t *testing.T) {
	s := &Skeleton{RawConfig: []byte(`not-json`)}
	var cfg map[string]string
	err := s.GetConfig(&cfg)
	assert.Error(t, err)
}

func TestSkeleton_RegisterRoutes_IsNoop(t *testing.T) {
	s := &Skeleton{}
	assert.NotPanics(t, func() {
		s.RegisterRoutes(nil, nil, nil, nil)
	})
}

func TestSkeleton_AvailableStorageTypes_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	assert.Nil(t, s.AvailableStorageTypes())
}

func TestSkeleton_CreateUserEngine_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	engine, err := s.CreateUserEngine(1)
	assert.NoError(t, err)
	assert.Nil(t, engine)
}

func TestManagerInit_LoadsPersistedPluginConfig(t *testing.T) {
	oldCallPluginAll := CallPluginAll
	t.Cleanup(func() {
		CallPluginAll = oldCallPluginAll
	})

	call, _, _, register := MakePlugin[Plugin](true)
	CallPluginAll = call

	plugin := &stubConfigPlugin{slug: "config-plugin"}
	register(plugin)

	db := openTestDB(t)
	dataDir := t.TempDir()
	store := newBoltPluginStore(db, plugin.slug, dataDir)
	require.NoError(t, store.SaveConfig([]byte(`{"foo":"bar"}`)))

	mgr := NewManager()
	err := mgr.Init(&StartupContext{
		StoreFactory: func(slug string) PluginStore {
			return newBoltPluginStore(db, slug, dataDir)
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"foo":"bar"}`, string(plugin.config))
	assert.Equal(t, 1, plugin.inits)

	stored, ok := GetStoreTyped[PluginStore](plugin.slug)
	assert.True(t, ok)
	assert.NotNil(t, stored)
}
