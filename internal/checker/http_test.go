package checker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hazz-dev/svcmon/internal/checker"
	"github.com/hazz-dev/svcmon/internal/config"
)

func makeHTTPService(t *testing.T, url string, extras ...func(*config.Service)) config.Service {
	t.Helper()
	svc := config.Service{
		Name:           "test-http",
		Type:           "http",
		Target:         url,
		Timeout:        config.Duration{Duration: 5 * time.Second},
		ExpectedStatus: 200,
	}
	for _, fn := range extras {
		fn(&svc)
	}
	return svc
}

func TestHTTPChecker_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := checker.New(makeHTTPService(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusUp {
		t.Errorf("expected StatusUp, got %q: %s", result.Status, result.Error)
	}
	if result.ResponseTime <= 0 {
		t.Errorf("expected positive response time, got %v", result.ResponseTime)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
}

func TestHTTPChecker_WrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := checker.New(makeHTTPService(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for wrong status code")
	}
}

func TestHTTPChecker_NetworkError(t *testing.T) {
	// Use a server that we close immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c, err := checker.New(makeHTTPService(t, url))
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for network error")
	}
}

func TestHTTPChecker_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until request context is cancelled (client disconnects / timeout)
		<-r.Context().Done()
	}))
	defer srv.Close()

	svc := makeHTTPService(t, srv.URL, func(s *config.Service) {
		s.Timeout = config.Duration{Duration: 50 * time.Millisecond}
	})
	c, err := checker.New(svc)
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown on timeout, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for timeout")
	}
}

func TestHTTPChecker_CustomHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := makeHTTPService(t, srv.URL, func(s *config.Service) {
		s.Headers = map[string]string{"Authorization": "Bearer mytoken"}
	})
	c, err := checker.New(svc)
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusUp {
		t.Errorf("expected StatusUp, got %q: %s", result.Status, result.Error)
	}
	if gotAuth != "Bearer mytoken" {
		t.Errorf("expected Authorization header 'Bearer mytoken', got %q", gotAuth)
	}
}

func TestHTTPChecker_CustomExpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc := makeHTTPService(t, srv.URL, func(s *config.Service) {
		s.ExpectedStatus = http.StatusNoContent
	})
	c, err := checker.New(svc)
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusUp {
		t.Errorf("expected StatusUp for 204, got %q: %s", result.Status, result.Error)
	}
}
