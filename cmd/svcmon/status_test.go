package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/hazz-dev/svcmon/internal/storage"
)

type mockStatusStore struct {
	checks []storage.Check
	err    error
}

func (m *mockStatusStore) AllLatest(_ context.Context) ([]storage.Check, error) {
	return m.checks, m.err
}

func TestExecuteStatus_EmptyDB(t *testing.T) {
	store := &mockStatusStore{checks: []storage.Check{}}
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := executeStatus(cmd, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No check history") {
		t.Errorf("expected 'No check history' message, got:\n%s", output)
	}
}

func TestExecuteStatus_WithChecks(t *testing.T) {
	checks := []storage.Check{
		{ID: 1, Service: "api", Status: "up", ResponseMs: 42, CheckedAt: time.Now()},
		{ID: 2, Service: "db", Status: "down", ResponseMs: 0, Error: "timeout", CheckedAt: time.Now()},
	}
	store := &mockStatusStore{checks: checks}

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := executeStatus(cmd, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "api") {
		t.Errorf("expected 'api' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "db") {
		t.Errorf("expected 'db' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "up") {
		t.Errorf("expected 'up' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "down") {
		t.Errorf("expected 'down' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "timeout") {
		t.Errorf("expected 'timeout' error in output, got:\n%s", output)
	}
}
