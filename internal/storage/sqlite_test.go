package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/hazz-dev/svcmon/internal/checker"
	"github.com/hazz-dev/svcmon/internal/storage"
)

func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("opening in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func makeResult(service string, status checker.Status, responseMs int64) checker.CheckResult {
	return checker.CheckResult{
		ServiceName:  service,
		Status:       status,
		ResponseTime: time.Duration(responseMs) * time.Millisecond,
		Error:        "",
		CheckedAt:    time.Now().UTC(),
	}
}

func TestOpen_CreatesSchema(t *testing.T) {
	db := openTestDB(t)
	// If we can insert, schema is correct.
	err := db.InsertCheck(context.Background(), makeResult("api", checker.StatusUp, 42))
	if err != nil {
		t.Fatalf("InsertCheck after Open: %v", err)
	}
}

func TestInsertCheck_And_LatestCheck(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	r := makeResult("api", checker.StatusUp, 42)
	if err := db.InsertCheck(ctx, r); err != nil {
		t.Fatalf("InsertCheck: %v", err)
	}

	got, err := db.LatestCheck(ctx, "api")
	if err != nil {
		t.Fatalf("LatestCheck: %v", err)
	}
	if got == nil {
		t.Fatal("expected a check, got nil")
	}
	if got.Service != "api" {
		t.Errorf("expected service 'api', got %q", got.Service)
	}
	if got.Status != "up" {
		t.Errorf("expected status 'up', got %q", got.Status)
	}
	if got.ResponseMs != 42 {
		t.Errorf("expected 42ms, got %d", got.ResponseMs)
	}
}

func TestLatestCheck_ReturnsNilWhenEmpty(t *testing.T) {
	db := openTestDB(t)
	got, err := db.LatestCheck(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("LatestCheck: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown service, got %+v", got)
	}
}

func TestLatestCheck_ReturnsMostRecent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	r1 := makeResult("api", checker.StatusDown, 10)
	r1.CheckedAt = time.Now().Add(-2 * time.Minute).UTC()
	r2 := makeResult("api", checker.StatusUp, 20)
	r2.CheckedAt = time.Now().Add(-1 * time.Minute).UTC()

	if err := db.InsertCheck(ctx, r1); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertCheck(ctx, r2); err != nil {
		t.Fatal(err)
	}

	got, err := db.LatestCheck(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "up" {
		t.Errorf("expected latest to be 'up', got %q", got.Status)
	}
}

func TestServiceHistory_Pagination(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		r := makeResult("api", checker.StatusUp, int64(i))
		r.CheckedAt = time.Now().Add(time.Duration(i) * time.Second).UTC()
		if err := db.InsertCheck(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	checks, total, err := db.ServiceHistory(ctx, "api", 5, 0)
	if err != nil {
		t.Fatalf("ServiceHistory: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total 10, got %d", total)
	}
	if len(checks) != 5 {
		t.Errorf("expected 5 results, got %d", len(checks))
	}

	// Second page
	checks2, total2, err := db.ServiceHistory(ctx, "api", 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if total2 != 10 {
		t.Errorf("expected total 10 on page 2, got %d", total2)
	}
	if len(checks2) != 5 {
		t.Errorf("expected 5 results on page 2, got %d", len(checks2))
	}
}

func TestServiceHistory_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	checks, total, err := db.ServiceHistory(context.Background(), "api", 10, 0)
	if err != nil {
		t.Fatalf("ServiceHistory: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(checks) != 0 {
		t.Errorf("expected 0 results, got %d", len(checks))
	}
}

func TestAllLatest_ReturnsOnePerService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Insert for two services
	for i := 0; i < 3; i++ {
		r := makeResult("api", checker.StatusUp, int64(i))
		r.CheckedAt = time.Now().Add(time.Duration(i) * time.Second).UTC()
		if err := db.InsertCheck(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		r := makeResult("db", checker.StatusDown, int64(i))
		r.CheckedAt = time.Now().Add(time.Duration(i) * time.Second).UTC()
		if err := db.InsertCheck(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	all, err := db.AllLatest(ctx)
	if err != nil {
		t.Fatalf("AllLatest: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 services, got %d", len(all))
	}

	byService := make(map[string]storage.Check)
	for _, c := range all {
		byService[c.Service] = c
	}
	if byService["api"].Status != "up" {
		t.Errorf("expected api status 'up', got %q", byService["api"].Status)
	}
	if byService["db"].Status != "down" {
		t.Errorf("expected db status 'down', got %q", byService["db"].Status)
	}
}

func TestAllLatest_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	all, err := db.AllLatest(context.Background())
	if err != nil {
		t.Fatalf("AllLatest: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 results, got %d", len(all))
	}
}

func TestUptimePercent_AllUp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if err := db.InsertCheck(ctx, makeResult("api", checker.StatusUp, 10)); err != nil {
			t.Fatal(err)
		}
	}

	pct, err := db.UptimePercent(ctx, "api", 10)
	if err != nil {
		t.Fatalf("UptimePercent: %v", err)
	}
	if pct != 100.0 {
		t.Errorf("expected 100%%, got %.2f", pct)
	}
}

func TestUptimePercent_HalfUp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := db.InsertCheck(ctx, makeResult("api", checker.StatusUp, 10)); err != nil {
			t.Fatal(err)
		}
		if err := db.InsertCheck(ctx, makeResult("api", checker.StatusDown, 10)); err != nil {
			t.Fatal(err)
		}
	}

	pct, err := db.UptimePercent(ctx, "api", 10)
	if err != nil {
		t.Fatalf("UptimePercent: %v", err)
	}
	if pct != 50.0 {
		t.Errorf("expected 50%%, got %.2f", pct)
	}
}

func TestUptimePercent_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	pct, err := db.UptimePercent(context.Background(), "api", 100)
	if err != nil {
		t.Fatalf("UptimePercent: %v", err)
	}
	if pct != 0.0 {
		t.Errorf("expected 0%%, got %.2f", pct)
	}
}

func TestClose(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
