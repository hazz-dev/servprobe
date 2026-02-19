package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hazz-dev/svcmon/internal/config"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "api"
    type: "http"
    target: "https://example.com/health"
    interval: "30s"
    timeout: "5s"
    expected_status: 200
    headers:
      Authorization: "Bearer token"
  - name: "db"
    type: "tcp"
    target: "db.example.com:5432"
    interval: "15s"
    timeout: "3s"
alerts:
  webhook:
    url: "https://hooks.example.com/alert"
    cooldown: "5m"
server:
  address: ":9090"
storage:
  path: "test.db"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "api" {
		t.Errorf("expected service name 'api', got %q", cfg.Services[0].Name)
	}
	if cfg.Services[0].Type != "http" {
		t.Errorf("expected type 'http', got %q", cfg.Services[0].Type)
	}
	if cfg.Services[0].Headers["Authorization"] != "Bearer token" {
		t.Errorf("expected Authorization header, got %v", cfg.Services[0].Headers)
	}
	if cfg.Alerts.Webhook.URL != "https://hooks.example.com/alert" {
		t.Errorf("unexpected webhook url: %q", cfg.Alerts.Webhook.URL)
	}
	if cfg.Server.Address != ":9090" {
		t.Errorf("unexpected address: %q", cfg.Server.Address)
	}
	if cfg.Storage.Path != "test.db" {
		t.Errorf("unexpected storage path: %q", cfg.Storage.Path)
	}
}

func TestLoad_Defaults(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "api"
    type: "http"
    target: "https://example.com/health"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := cfg.Services[0]
	if svc.Interval.String() != "30s" {
		t.Errorf("expected default interval 30s, got %v", svc.Interval)
	}
	if svc.Timeout.String() != "5s" {
		t.Errorf("expected default timeout 5s, got %v", svc.Timeout)
	}
	if svc.ExpectedStatus != 200 {
		t.Errorf("expected default expected_status 200, got %d", svc.ExpectedStatus)
	}
	if cfg.Server.Address != ":8080" {
		t.Errorf("expected default address :8080, got %q", cfg.Server.Address)
	}
	if cfg.Storage.Path != "svcmon.db" {
		t.Errorf("expected default storage path svcmon.db, got %q", cfg.Storage.Path)
	}
}

func TestLoad_MissingName(t *testing.T) {
	path := writeTemp(t, `
services:
  - type: "http"
    target: "https://example.com"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention 'name': %v", err)
	}
}

func TestLoad_MissingTarget(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "api"
    type: "http"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing target, got nil")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("error should mention 'target': %v", err)
	}
}

func TestLoad_InvalidType(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "api"
    type: "ftp"
    target: "ftp://example.com"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid type, got nil")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error should mention 'type': %v", err)
	}
}

func TestLoad_InvalidInterval(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "api"
    type: "http"
    target: "https://example.com"
    interval: "not-a-duration"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid interval, got nil")
	}
	if !strings.Contains(err.Error(), "interval") {
		t.Errorf("error should mention 'interval': %v", err)
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "api"
    type: "http"
    target: "https://example.com"
    timeout: "bad"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid timeout, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error should mention 'timeout': %v", err)
	}
}

func TestLoad_EmptyServices(t *testing.T) {
	path := writeTemp(t, `
services: []
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for empty services, got nil")
	}
	if !strings.Contains(err.Error(), "service") {
		t.Errorf("error should mention 'service': %v", err)
	}
}

func TestLoad_DuplicateServiceNames(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "api"
    type: "http"
    target: "https://example.com"
  - name: "api"
    type: "tcp"
    target: "example.com:80"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for duplicate names, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention 'duplicate': %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_AllServiceTypes(t *testing.T) {
	path := writeTemp(t, `
services:
  - name: "http-svc"
    type: "http"
    target: "https://example.com"
  - name: "tcp-svc"
    type: "tcp"
    target: "example.com:80"
  - name: "ping-svc"
    type: "ping"
    target: "8.8.8.8"
  - name: "docker-svc"
    type: "docker"
    target: "my-container"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Services) != 4 {
		t.Fatalf("expected 4 services, got %d", len(cfg.Services))
	}
}
