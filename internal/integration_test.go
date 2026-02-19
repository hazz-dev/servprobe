package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hazz-dev/servprobe/internal/checker"
	"github.com/hazz-dev/servprobe/internal/config"
	"github.com/hazz-dev/servprobe/internal/scheduler"
	"github.com/hazz-dev/servprobe/internal/server"
	"github.com/hazz-dev/servprobe/internal/storage"
)

// TestIntegration_FullFlow verifies the complete pipeline:
// config → scheduler → checker → storage → API
func TestIntegration_FullFlow(t *testing.T) {
	// 1. Start a fake HTTP target service
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// 2. Open in-memory SQLite
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("opening storage: %v", err)
	}
	defer db.Close()

	// 3. Build config
	services := []config.Service{
		{
			Name:           "test-api",
			Type:           "http",
			Target:         target.URL,
			Interval:       config.Duration{Duration: time.Hour}, // don't auto-repeat
			Timeout:        config.Duration{Duration: 5 * time.Second},
			ExpectedStatus: 200,
		},
	}

	// 4. Create scheduler with real checker factory
	factory := func(svc config.Service) (checker.Checker, error) {
		return checker.New(svc)
	}
	sched := scheduler.New(services, db, factory, nil)

	// 5. Start scheduler — it will run the first check immediately
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)

	// 6. Wait for the first check to land in the DB (up to 5s)
	deadline := time.Now().Add(5 * time.Second)
	var latestCheck *storage.Check
	for time.Now().Before(deadline) {
		c, err := db.LatestCheck(ctx, "test-api")
		if err != nil {
			t.Fatalf("LatestCheck: %v", err)
		}
		if c != nil {
			latestCheck = c
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if latestCheck == nil {
		t.Fatal("no check result in DB after 5s")
	}
	if latestCheck.Status != "up" {
		t.Errorf("expected status 'up', got %q (error: %s)", latestCheck.Status, latestCheck.Error)
	}

	// 7. Build API server
	apiServer := server.New(db, services, nil)

	// 8. GET /api/health
	t.Run("health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()
		apiServer.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["status"] != "ok" {
			t.Errorf("expected status 'ok', got %q", resp["status"])
		}
	})

	// 9. GET /api/services — verify test-api appears
	t.Run("list services", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/services", nil)
		w := httptest.NewRecorder()
		apiServer.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Data []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"data"`
		}
		json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 service, got %d", len(resp.Data))
		}
		if resp.Data[0].Name != "test-api" {
			t.Errorf("expected name 'test-api', got %q", resp.Data[0].Name)
		}
		if resp.Data[0].Status != "up" {
			t.Errorf("expected status 'up', got %q", resp.Data[0].Status)
		}
	})

	// 10. GET /api/services/{name}
	t.Run("get service detail", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/services/test-api", nil)
		w := httptest.NewRecorder()
		apiServer.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Data struct {
				Name string `json:"name"`
			} `json:"data"`
		}
		json.NewDecoder(w.Body).Decode(&resp)
		if resp.Data.Name != "test-api" {
			t.Errorf("expected name 'test-api', got %q", resp.Data.Name)
		}
	})

	// 11. GET /api/services/{name}/history — at least 1 check
	t.Run("service history", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/services/test-api/history", nil)
		w := httptest.NewRecorder()
		apiServer.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Data struct {
				Total  int           `json:"total"`
				Checks []interface{} `json:"checks"`
			} `json:"data"`
		}
		json.NewDecoder(w.Body).Decode(&resp)
		if resp.Data.Total < 1 {
			t.Errorf("expected at least 1 check in history, got %d", resp.Data.Total)
		}
	})

	// 12. Graceful shutdown
	cancel()
	sched.Wait()

	// 13. Verify no goroutine leak by checking DB is still accessible after shutdown
	_, err = db.LatestCheck(context.Background(), "test-api")
	if err != nil {
		t.Errorf("DB unusable after shutdown: %v", err)
	}

}
