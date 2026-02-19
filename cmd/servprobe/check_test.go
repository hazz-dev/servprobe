package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hazz-dev/servprobe/internal/config"
)

func TestRunChecks_AllUp_OutputFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.Config{
		Services: []config.Service{
			{
				Name:           "myapi",
				Type:           "http",
				Target:         srv.URL,
				Timeout:        config.Duration{Duration: 5 * time.Second},
				ExpectedStatus: 200,
			},
		},
	}

	var buf bytes.Buffer
	err := runChecks(&buf, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "myapi") {
		t.Errorf("expected output to contain 'myapi', got:\n%s", output)
	}
	if !strings.Contains(output, "http") {
		t.Errorf("expected output to contain 'http', got:\n%s", output)
	}
	if !strings.Contains(output, "up") {
		t.Errorf("expected output to contain 'up', got:\n%s", output)
	}
	if !strings.Contains(output, "SERVICE") {
		t.Errorf("expected header row with 'SERVICE', got:\n%s", output)
	}
}

func TestRunChecks_MultipleServices(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv2.Close()

	cfg := &config.Config{
		Services: []config.Service{
			{Name: "svc1", Type: "http", Target: srv1.URL, Timeout: config.Duration{Duration: 5 * time.Second}, ExpectedStatus: 200},
			{Name: "svc2", Type: "http", Target: srv2.URL, Timeout: config.Duration{Duration: 5 * time.Second}, ExpectedStatus: 200},
		},
	}

	var buf bytes.Buffer
	err := runChecks(&buf, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "svc1") {
		t.Errorf("expected 'svc1' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "svc2") {
		t.Errorf("expected 'svc2' in output, got:\n%s", output)
	}
}
