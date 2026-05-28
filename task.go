package abyss

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskStatus represents the current state of an asynchronous task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskSucceeded TaskStatus = "succeeded"
	TaskFailed    TaskStatus = "failed"
	TaskCanceled  TaskStatus = "canceled"
)

// TaskInfo contains metadata and status for a task.
type TaskInfo struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	UserID    uint64            `json:"userId"`
	Status    TaskStatus        `json:"status"`
	Progress  float64           `json:"progress"`
	Message   string            `json:"message,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// TaskStore defines task persistence.
type TaskStore interface {
	Save(ctx context.Context, task *TaskInfo) error
	GetByID(ctx context.Context, id string) (*TaskInfo, error)
	ListByUser(ctx context.Context, userID uint64) ([]*TaskInfo, error)
	Delete(ctx context.Context, id string) error
}

// ── scheduler ─────────────────────────────────────────────────────────────────

type scheduler struct {
	mu     sync.Mutex
	cancel map[string]context.CancelFunc
}

func newScheduler() *scheduler {
	return &scheduler{cancel: make(map[string]context.CancelFunc)}
}

func (s *scheduler) Track(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel[id] = cancel
}

func (s *scheduler) Untrack(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancel, id)
}

func (s *scheduler) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cancel)
}

func (s *scheduler) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cancel[id]
	if !ok {
		return false
	}
	c()
	delete(s.cancel, id)
	return true
}

// ── taskService ───────────────────────────────────────────────────────────────

type taskRunner func(context.Context) error

type taskUserIDCtxKey struct{}

func withTaskUserID(ctx context.Context, userID uint64) context.Context {
	return context.WithValue(ctx, taskUserIDCtxKey{}, userID)
}

func taskUserIDFromContext(ctx context.Context) uint64 {
	v := ctx.Value(taskUserIDCtxKey{})
	uid, _ := v.(uint64)
	return uid
}

type taskService struct {
	store       TaskStore
	sch         *scheduler
	runners     map[string]taskRunner
	subscribers map[chan *TaskInfo]uint64
	mu          sync.RWMutex
}

func newTaskService(store TaskStore, sch *scheduler) *taskService {
	return &taskService{
		store:       store,
		sch:         sch,
		runners:     make(map[string]taskRunner),
		subscribers: make(map[chan *TaskInfo]uint64),
	}
}

func (s *taskService) RegisterRunner(name string, runner taskRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runners[name] = runner
}

func (s *taskService) SubmitRegistered(ctx context.Context, name string, userID uint64) (string, error) {
	s.mu.RLock()
	runner, ok := s.runners[name]
	s.mu.RUnlock()
	if !ok {
		return "", WrapError(ErrInvalidInput, nil, "task runner not registered")
	}
	return s.Submit(ctx, name, userID, runner)
}

func (s *taskService) Submit(ctx context.Context, name string, userID uint64, run func(context.Context) error) (string, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	t := &TaskInfo{ID: id, Name: name, UserID: userID, Status: TaskPending, Progress: 0, CreatedAt: now, UpdatedAt: now}
	if err := s.store.Save(ctx, t); err != nil {
		return "", err
	}
	s.broadcast(t)

	if ctx == nil {
		ctx = context.Background()
	}
	ctx = withTaskUserID(ctx, userID)
	runCtx, cancel := context.WithCancel(ctx)
	s.sch.Track(id, cancel)
	go func() {
		defer s.sch.Untrack(id)
		t.Status = TaskRunning
		t.UpdatedAt = time.Now().UTC()
		_ = s.store.Save(runCtx, t)
		s.broadcast(t)
		err := run(runCtx)
		t.UpdatedAt = time.Now().UTC()
		switch {
		case errors.Is(err, context.Canceled):
			t.Status = TaskCanceled
			t.Message = err.Error()
		case err != nil:
			t.Status = TaskFailed
			t.Message = err.Error()
		default:
			t.Status = TaskSucceeded
			t.Progress = 100
		}
		_ = s.store.Save(context.Background(), t)
		s.broadcast(t)
	}()
	return id, nil
}

func (s *taskService) broadcast(t *TaskInfo) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch, uid := range s.subscribers {
		if uid == 0 || uid == t.UserID {
			select {
			case ch <- t:
			default:
			}
		}
	}
}

func (s *taskService) Subscribe(userID uint64) chan *TaskInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan *TaskInfo, 10)
	s.subscribers[ch] = userID
	return ch
}

func (s *taskService) Unsubscribe(ch chan *TaskInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscribers, ch)
	close(ch)
}

func (s *taskService) Cancel(id string) error {
	if !s.sch.Cancel(id) {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

func (s *taskService) GetByID(ctx context.Context, id string) (*TaskInfo, error) {
	return s.store.GetByID(ctx, id)
}

func (s *taskService) ListByUser(ctx context.Context, userID uint64) ([]*TaskInfo, error) {
	return s.store.ListByUser(ctx, userID)
}
