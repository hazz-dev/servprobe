package scheduler_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hazz-dev/svcmon/internal/checker"
	"github.com/hazz-dev/svcmon/internal/config"
	"github.com/hazz-dev/svcmon/internal/scheduler"
	"github.com/hazz-dev/svcmon/internal/storage"
)

// mockChecker always returns a fixed result.
type mockChecker struct {
	result checker.CheckResult
}

func (m *mockChecker) Check(ctx context.Context) checker.CheckResult {
	return m.result
}

// mockStore records inserted checks.
type mockStore struct {
	mu     sync.Mutex
	checks []checker.CheckResult
	latest map[string]*storage.Check
	err    error
}

func (m *mockStore) InsertCheck(_ context.Context, r checker.CheckResult) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	m.checks = append(m.checks, r)
	m.mu.Unlock()
	return nil
}

func (m *mockStore) LatestCheck(_ context.Context, service string) (*storage.Check, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.latest != nil {
		return m.latest[service], nil
	}
	return nil, nil
}

func makeServices(interval time.Duration) []config.Service {
	return []config.Service{
		{
			Name:     "api",
			Type:     "http",
			Target:   "http://example.com",
			Interval: config.Duration{Duration: interval},
			Timeout:  config.Duration{Duration: time.Second},
		},
	}
}

func makeFactory(c checker.Checker) scheduler.CheckerFactory {
	return func(svc config.Service) (checker.Checker, error) {
		return c, nil
	}
}

func TestScheduler_RunsCheckImmediately(t *testing.T) {
	store := &mockStore{}
	mc := &mockChecker{
		result: checker.CheckResult{ServiceName: "api", Status: checker.StatusUp},
	}
	sched := scheduler.New(makeServices(time.Hour), store, makeFactory(mc), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched.Start(ctx)

	// Wait for first check
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := len(store.checks)
		store.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	store.mu.Lock()
	n := len(store.checks)
	store.mu.Unlock()
	if n < 1 {
		t.Error("expected at least one check to run immediately")
	}
}

func TestScheduler_RunsPeriodicChecks(t *testing.T) {
	store := &mockStore{}
	mc := &mockChecker{
		result: checker.CheckResult{ServiceName: "api", Status: checker.StatusUp},
	}
	interval := 50 * time.Millisecond
	sched := scheduler.New(makeServices(interval), store, makeFactory(mc), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	sched.Start(ctx)
	<-ctx.Done()
	sched.Wait()

	store.mu.Lock()
	n := len(store.checks)
	store.mu.Unlock()

	// Should have at least 3 checks in 300ms with 50ms interval (1 immediate + ~5)
	if n < 3 {
		t.Errorf("expected at least 3 checks in 300ms, got %d", n)
	}
}

func TestScheduler_ContextCancellation(t *testing.T) {
	store := &mockStore{}
	mc := &mockChecker{
		result: checker.CheckResult{ServiceName: "api", Status: checker.StatusUp},
	}
	sched := scheduler.New(makeServices(time.Hour), store, makeFactory(mc), nil)

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		sched.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good — Wait() returned
	case <-time.After(2 * time.Second):
		t.Error("Wait() did not return within 2s after context cancel")
	}
}

func TestScheduler_OnResultCallback(t *testing.T) {
	store := &mockStore{}
	mc := &mockChecker{
		result: checker.CheckResult{ServiceName: "api", Status: checker.StatusUp},
	}

	var callCount int32
	sched := scheduler.New(makeServices(time.Hour), store, makeFactory(mc), nil)
	sched.SetOnResult(func(r checker.CheckResult, prev *checker.Status) {
		atomic.AddInt32(&callCount, 1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)

	// Wait for first check
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&callCount) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	sched.Wait()

	if atomic.LoadInt32(&callCount) < 1 {
		t.Error("expected onResult callback to be called at least once")
	}
}

func TestScheduler_StoreErrorDoesNotCrash(t *testing.T) {
	store := &mockStore{err: context.DeadlineExceeded}
	mc := &mockChecker{
		result: checker.CheckResult{ServiceName: "api", Status: checker.StatusUp},
	}

	sched := scheduler.New(makeServices(time.Hour), store, makeFactory(mc), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not panic
	sched.Start(ctx)
	<-ctx.Done()
	sched.Wait()
}

func TestScheduler_MultipleServices(t *testing.T) {
	store := &mockStore{}
	services := []config.Service{
		{Name: "svc1", Type: "http", Target: "http://a.com", Interval: config.Duration{Duration: time.Hour}, Timeout: config.Duration{Duration: time.Second}},
		{Name: "svc2", Type: "tcp", Target: "b.com:80", Interval: config.Duration{Duration: time.Hour}, Timeout: config.Duration{Duration: time.Second}},
	}
	factory := func(svc config.Service) (checker.Checker, error) {
		return &mockChecker{result: checker.CheckResult{ServiceName: svc.Name, Status: checker.StatusUp}}, nil
	}

	sched := scheduler.New(services, store, factory, nil)
	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := len(store.checks)
		store.mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	sched.Wait()

	store.mu.Lock()
	n := len(store.checks)
	store.mu.Unlock()
	if n < 2 {
		t.Errorf("expected at least 2 checks (one per service), got %d", n)
	}
}
