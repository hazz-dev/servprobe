package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hazz-dev/servprobe/internal/config"
	"github.com/hazz-dev/servprobe/internal/server"
	"github.com/hazz-dev/servprobe/internal/storage"
)

// mockStore implements server.ServerStore for testing.
type mockStore struct {
	checks    []storage.Check
	latest    map[string]*storage.Check
	history   map[string][]storage.Check
	totalHist map[string]int
	uptime    map[string]float64
	err       error
}

func (m *mockStore) AllLatest(_ context.Context) ([]storage.Check, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.checks, nil
}

func (m *mockStore) LatestCheck(_ context.Context, service string) (*storage.Check, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.latest != nil {
		return m.latest[service], nil
	}
	return nil, nil
}

func (m *mockStore) ServiceHistory(_ context.Context, service string, limit, offset int) ([]storage.Check, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	checks := m.history[service]
	total := m.totalHist[service]
	return checks, total, nil
}

func (m *mockStore) UptimePercent(_ context.Context, service string, last int) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.uptime[service], nil
}

func makeServices() []config.Service {
	return []config.Service{
		{
			Name:     "api",
			Type:     "http",
			Target:   "https://example.com",
			Interval: config.Duration{Duration: 30 * time.Second},
			Timeout:  config.Duration{Duration: 5 * time.Second},
		},
	}
}

func makeCheck(service, status string) storage.Check {
	return storage.Check{
		ID:         1,
		Service:    service,
		Status:     status,
		ResponseMs: 42,
		CheckedAt:  time.Now().UTC(),
	}
}

func doRequest(t *testing.T, router http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decoding JSON response: %v", err)
	}
}

func TestHealth(t *testing.T) {
	s := server.New(&mockStore{}, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/health")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	decodeJSON(t, w, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
}

func TestListServices_Empty(t *testing.T) {
	store := &mockStore{checks: []storage.Check{}}
	s := server.New(store, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Error string        `json:"error"`
	}
	decodeJSON(t, w, &resp)
	if resp.Error != "" {
		t.Errorf("expected no error, got %q", resp.Error)
	}
}

func TestListServices_WithStatus(t *testing.T) {
	store := &mockStore{
		checks: []storage.Check{makeCheck("api", "up")},
		latest: map[string]*storage.Check{"api": func() *storage.Check { c := makeCheck("api", "up"); return &c }()},
		uptime: map[string]float64{"api": 100.0},
	}
	s := server.New(store, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data  []map[string]interface{} `json:"data"`
		Error string                   `json:"error"`
	}
	decodeJSON(t, w, &resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 service, got %d", len(resp.Data))
	}
	if resp.Data[0]["name"] != "api" {
		t.Errorf("expected name 'api', got %v", resp.Data[0]["name"])
	}
}

func TestGetService_Found(t *testing.T) {
	c := makeCheck("api", "up")
	store := &mockStore{
		latest:    map[string]*storage.Check{"api": &c},
		history:   map[string][]storage.Check{"api": {c}},
		totalHist: map[string]int{"api": 1},
		uptime:    map[string]float64{"api": 99.5},
	}
	s := server.New(store, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services/api")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  map[string]interface{} `json:"data"`
		Error string                 `json:"error"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data["name"] != "api" {
		t.Errorf("expected name 'api', got %v", resp.Data["name"])
	}
}

func TestGetService_NotFound(t *testing.T) {
	s := server.New(&mockStore{}, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetServiceHistory_Pagination(t *testing.T) {
	checks := make([]storage.Check, 5)
	for i := range checks {
		checks[i] = makeCheck("api", "up")
	}
	store := &mockStore{
		history:   map[string][]storage.Check{"api": checks},
		totalHist: map[string]int{"api": 50},
	}
	s := server.New(store, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services/api/history?limit=5&offset=0")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Checks []interface{} `json:"checks"`
			Total  int           `json:"total"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.Total != 50 {
		t.Errorf("expected total 50, got %d", resp.Data.Total)
	}
	if len(resp.Data.Checks) != 5 {
		t.Errorf("expected 5 checks, got %d", len(resp.Data.Checks))
	}
}

func TestGetServiceHistory_NotFound(t *testing.T) {
	s := server.New(&mockStore{}, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services/nonexistent/history")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetServiceHistory_InvalidLimit(t *testing.T) {
	s := server.New(&mockStore{}, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services/api/history?limit=bad")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad limit, got %d", w.Code)
	}
}

func TestGetServiceHistory_InvalidOffset(t *testing.T) {
	s := server.New(&mockStore{}, makeServices(), nil)
	w := doRequest(t, s.Router(), "GET", "/api/services/api/history?offset=notanumber")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad offset, got %d", w.Code)
	}
}
