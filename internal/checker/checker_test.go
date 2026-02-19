package checker_test

import (
	"testing"

	"github.com/hazz-dev/servprobe/internal/checker"
	"github.com/hazz-dev/servprobe/internal/config"
)

func TestNew_UnknownType(t *testing.T) {
	svc := config.Service{
		Name:   "test",
		Type:   "ftp",
		Target: "ftp://example.com",
	}
	_, err := checker.New(svc)
	if err == nil {
		t.Fatal("expected error for unknown checker type, got nil")
	}
}

func TestStatusConstants(t *testing.T) {
	if checker.StatusUp != "up" {
		t.Errorf("StatusUp should be 'up', got %q", checker.StatusUp)
	}
	if checker.StatusDown != "down" {
		t.Errorf("StatusDown should be 'down', got %q", checker.StatusDown)
	}
}
