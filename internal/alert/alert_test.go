package alert_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hazz-dev/servprobe/internal/alert"
	"github.com/hazz-dev/servprobe/internal/checker"
)

func statusPtr(s checker.Status) *checker.Status {
	return &s
}

func makeResult(service string, status checker.Status) checker.CheckResult {
	return checker.CheckResult{
		ServiceName:  service,
		Status:       status,
		ResponseTime: 10 * time.Millisecond,
		CheckedAt:    time.Now().UTC(),
	}
}

func TestAlerter_StateChange_UpToDown(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := alert.New(srv.URL, time.Hour, nil)
	a.Notify(makeResult("api", checker.StatusDown), statusPtr(checker.StatusUp))

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 webhook call for up→down, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestAlerter_StateChange_DownToUp(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := alert.New(srv.URL, time.Hour, nil)
	a.Notify(makeResult("api", checker.StatusUp), statusPtr(checker.StatusDown))

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 webhook call for down→up, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestAlerter_SameState_NoWebhook(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := alert.New(srv.URL, time.Hour, nil)
	a.Notify(makeResult("api", checker.StatusUp), statusPtr(checker.StatusUp))
	a.Notify(makeResult("api", checker.StatusDown), statusPtr(checker.StatusDown))

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&callCount) != 0 {
		t.Errorf("expected 0 webhook calls for same-state, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestAlerter_FirstCheck_NoWebhook(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := alert.New(srv.URL, time.Hour, nil)
	a.Notify(makeResult("api", checker.StatusDown), nil) // nil = first check

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&callCount) != 0 {
		t.Errorf("expected 0 webhook calls for first check, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestAlerter_Cooldown_SuppressesAlerts(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cooldown := time.Hour // long cooldown
	a := alert.New(srv.URL, cooldown, nil)

	// First state change — should send
	a.Notify(makeResult("api", checker.StatusDown), statusPtr(checker.StatusUp))
	time.Sleep(50 * time.Millisecond)

	// Second state change — within cooldown, should suppress
	a.Notify(makeResult("api", checker.StatusUp), statusPtr(checker.StatusDown))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 webhook call (cooldown suppressed second), got %d", atomic.LoadInt32(&callCount))
	}
}

func TestAlerter_Cooldown_PerService(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cooldown := time.Hour
	a := alert.New(srv.URL, cooldown, nil)

	// Alert for svc1 — triggers cooldown for svc1
	a.Notify(makeResult("svc1", checker.StatusDown), statusPtr(checker.StatusUp))
	time.Sleep(50 * time.Millisecond)

	// Alert for svc2 — different service, not affected by svc1's cooldown
	a.Notify(makeResult("svc2", checker.StatusDown), statusPtr(checker.StatusUp))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 webhook calls (one per service), got %d", atomic.LoadInt32(&callCount))
	}
}

func TestAlerter_WebhookPayload(t *testing.T) {
	var payload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := alert.New(srv.URL, time.Hour, nil)
	result := checker.CheckResult{
		ServiceName:  "api",
		Status:       checker.StatusDown,
		ResponseTime: 0,
		Error:        "connection refused",
		CheckedAt:    time.Now().UTC(),
	}
	a.Notify(result, statusPtr(checker.StatusUp))

	time.Sleep(100 * time.Millisecond)

	if payload["service"] != "api" {
		t.Errorf("expected service 'api', got %v", payload["service"])
	}
	if payload["status"] != "down" {
		t.Errorf("expected status 'down', got %v", payload["status"])
	}
	if payload["previous_status"] != "up" {
		t.Errorf("expected previous_status 'up', got %v", payload["previous_status"])
	}
	if payload["source"] != "servprobe" {
		t.Errorf("expected source 'servprobe', got %v", payload["source"])
	}
}

func TestAlerter_HTTPError_DoesNotCrash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := alert.New(srv.URL, time.Hour, nil)
	// Should not panic even on HTTP error
	a.Notify(makeResult("api", checker.StatusDown), statusPtr(checker.StatusUp))
	time.Sleep(100 * time.Millisecond)
}
