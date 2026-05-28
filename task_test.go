package abyss

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── memTaskStore ──────────────────────────────────────────────────────────────

type memTaskStore struct {
	mu    sync.Mutex
	tasks map[string]*TaskInfo
}

func newMemTaskStore() *memTaskStore {
	return &memTaskStore{tasks: make(map[string]*TaskInfo)}
}

func (s *memTaskStore) Save(_ context.Context, t *TaskInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *memTaskStore) GetByID(_ context.Context, id string) (*TaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *memTaskStore) ListByUser(_ context.Context, userID uint64) ([]*TaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*TaskInfo
	for _, t := range s.tasks {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memTaskStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
	return nil
}

type cancelAwareTaskStore struct {
	*memTaskStore
}

func (s *cancelAwareTaskStore) Save(ctx context.Context, t *TaskInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.memTaskStore.Save(ctx, t)
}

// ── scheduler tests ───────────────────────────────────────────────────────────

func TestScheduler_TrackAndCancel(t *testing.T) {
	sch := newScheduler()
	cancelled := false
	sch.Track("t1", func() { cancelled = true })
	ok := sch.Cancel("t1")
	assert.True(t, ok)
	assert.True(t, cancelled)
}

func TestScheduler_Cancel_NotFound(t *testing.T) {
	sch := newScheduler()
	ok := sch.Cancel("nonexistent")
	assert.False(t, ok)
}

func TestScheduler_Cancel_Idempotent(t *testing.T) {
	sch := newScheduler()
	sch.Track("t2", func() {})
	sch.Cancel("t2")
	// second cancel should return false
	ok := sch.Cancel("t2")
	assert.False(t, ok)
}

func TestScheduler_Untrack(t *testing.T) {
	sch := newScheduler()
	sch.Track("t3", func() {})
	assert.Equal(t, 1, sch.Size())
	sch.Untrack("t3")
	assert.Equal(t, 0, sch.Size())
}

// ── taskService tests ─────────────────────────────────────────────────────────

func TestTaskService_Submit_Success(t *testing.T) {
	store := newMemTaskStore()
	sch := newScheduler()
	svc := newTaskService(store, sch)

	done := make(chan struct{})
	id, err := svc.Submit(context.Background(), "test-task", 1, func(_ context.Context) error {
		close(done)
		return nil
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not complete in time")
	}
	require.Eventually(t, func() bool { return sch.Size() == 0 }, time.Second, 10*time.Millisecond)
}

func TestTaskService_Submit_Failure(t *testing.T) {
	store := newMemTaskStore()
	svc := newTaskService(store, newScheduler())

	done := make(chan struct{})
	id, err := svc.Submit(context.Background(), "fail-task", 1, func(_ context.Context) error {
		defer close(done)
		return errors.New("task error")
	})
	require.NoError(t, err)
	<-done
	// Wait for status update
	time.Sleep(50 * time.Millisecond)

	info, err := store.GetByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, TaskFailed, info.Status)
	assert.Equal(t, "task error", info.Message)
}

func TestTaskService_Cancel(t *testing.T) {
	store := newMemTaskStore()
	sch := newScheduler()
	svc := newTaskService(store, sch)

	started := make(chan struct{})
	id, _ := svc.Submit(context.Background(), "long-task", 1, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started
	require.NoError(t, svc.Cancel(id))
	require.Eventually(t, func() bool { return sch.Size() == 0 }, time.Second, 10*time.Millisecond)
}

func TestTaskService_Cancel_PersistsFinalStatus(t *testing.T) {
	store := &cancelAwareTaskStore{memTaskStore: newMemTaskStore()}
	sch := newScheduler()
	svc := newTaskService(store, sch)

	started := make(chan struct{})
	id, err := svc.Submit(context.Background(), "cancel-persist", 1, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	require.NoError(t, err)
	<-started
	require.NoError(t, svc.Cancel(id))

	require.Eventually(t, func() bool {
		info, getErr := store.GetByID(context.Background(), id)
		return getErr == nil && info.Status == TaskCanceled
	}, time.Second, 10*time.Millisecond)
}

func TestTaskService_Cancel_NotFound(t *testing.T) {
	svc := newTaskService(newMemTaskStore(), newScheduler())
	err := svc.Cancel("unknown")
	assert.Error(t, err)
}

func TestTaskService_ListByUser(t *testing.T) {
	store := newMemTaskStore()
	svc := newTaskService(store, newScheduler())

	_, _ = svc.Submit(context.Background(), "t1", 1, func(_ context.Context) error { return nil })
	_, _ = svc.Submit(context.Background(), "t2", 1, func(_ context.Context) error { return nil })
	_, _ = svc.Submit(context.Background(), "t3", 2, func(_ context.Context) error { return nil })

	list, err := svc.ListByUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestTaskService_RegisterRunner_And_SubmitRegistered(t *testing.T) {
	store := newMemTaskStore()
	svc := newTaskService(store, newScheduler())

	done := make(chan struct{})
	svc.RegisterRunner("my-runner", func(_ context.Context) error {
		close(done)
		return nil
	})

	id, err := svc.SubmitRegistered(context.Background(), "my-runner", 1)
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	<-done
}

func TestTaskService_SubmitRegistered_UnknownRunner(t *testing.T) {
	svc := newTaskService(newMemTaskStore(), newScheduler())
	_, err := svc.SubmitRegistered(context.Background(), "nope", 1)
	assert.Error(t, err)
}

// ── memSettingsStore ──────────────────────────────────────────────────────────

type memSettingsStore struct {
	s *Settings
}

func (m *memSettingsStore) Get(_ context.Context) (*Settings, error) {
	if m.s == nil {
		return nil, ErrNotFound
	}
	cp := *m.s
	return &cp, nil
}

func (m *memSettingsStore) Save(_ context.Context, s *Settings) error {
	cp := *s
	m.s = &cp
	return nil
}

type countingSettingsStore struct {
	inner *memSettingsStore
	gets  int
	saves int
}

func (c *countingSettingsStore) Get(ctx context.Context) (*Settings, error) {
	c.gets++
	return c.inner.Get(ctx)
}

func (c *countingSettingsStore) Save(ctx context.Context, s *Settings) error {
	c.saves++
	return c.inner.Save(ctx, s)
}

// ── settingsService tests ─────────────────────────────────────────────────────

func TestSettingsService_GetAndSave(t *testing.T) {
	svc := newSettingsService(&memSettingsStore{})

	got, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, got)

	in := &Settings{
		Branding: SettingsBranding{Name: "My Abyss", Theme: "dark"},
	}
	require.NoError(t, svc.Save(context.Background(), in))

	got2, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "My Abyss", got2.Branding.Name)
	assert.Equal(t, "dark", got2.Branding.Theme)
}

func TestSettingsService_Get_UsesCache(t *testing.T) {
	store := &countingSettingsStore{inner: &memSettingsStore{}}
	svc := newSettingsService(store)

	_, err := svc.Get(context.Background())
	require.NoError(t, err)
	_, err = svc.Get(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, store.gets)

	require.NoError(t, svc.Save(context.Background(), &Settings{Branding: SettingsBranding{Name: "Cached"}}))
	assert.Equal(t, 1, store.saves)
	_, err = svc.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, store.gets)
}
