package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hazz-dev/servprobe/internal/checker"
	"github.com/hazz-dev/servprobe/internal/config"
	"github.com/hazz-dev/servprobe/internal/storage"
)

// Store defines the storage operations required by the scheduler.
type Store interface {
	InsertCheck(ctx context.Context, r checker.CheckResult) error
	LatestCheck(ctx context.Context, service string) (*storage.Check, error)
}

// CheckerFactory creates a Checker for a given service config.
type CheckerFactory func(config.Service) (checker.Checker, error)

// Scheduler runs health checks for each service in its own goroutine.
type Scheduler struct {
	services []config.Service
	store    Store
	factory  CheckerFactory
	onResult func(checker.CheckResult, *checker.Status)
	logger   *slog.Logger
	wg       sync.WaitGroup
}

// New creates a new Scheduler. Pass nil logger to discard logs.
func New(services []config.Service, store Store, factory CheckerFactory, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		services: services,
		store:    store,
		factory:  factory,
		logger:   logger,
	}
}

// SetOnResult sets the callback invoked after each check.
// result is the current check result; prev is the previous status (nil on first check).
func (s *Scheduler) SetOnResult(fn func(checker.CheckResult, *checker.Status)) {
	s.onResult = fn
}

// Start spawns one goroutine per service. It is non-blocking.
func (s *Scheduler) Start(ctx context.Context) {
	for _, svc := range s.services {
		svc := svc // capture loop var
		c, err := s.factory(svc)
		if err != nil {
			s.logger.Error("creating checker", "service", svc.Name, "error", err)
			continue
		}
		s.wg.Add(1)
		go s.runService(ctx, svc, c)
	}
}

// Wait blocks until all service goroutines have exited.
func (s *Scheduler) Wait() {
	s.wg.Wait()
}

func (s *Scheduler) runService(ctx context.Context, svc config.Service, c checker.Checker) {
	defer s.wg.Done()

	// Run immediately.
	s.runCheck(ctx, svc, c)

	ticker := time.NewTicker(svc.Interval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCheck(ctx, svc, c)
		}
	}
}

func (s *Scheduler) runCheck(ctx context.Context, svc config.Service, c checker.Checker) {
	// Fetch previous status before running the check.
	prev, err := s.store.LatestCheck(ctx, svc.Name)
	if err != nil {
		s.logger.Warn("fetching previous check", "service", svc.Name, "error", err)
	}

	result := c.Check(ctx)

	s.logger.Info("check result",
		"service", svc.Name,
		"status", result.Status,
		"response_time", result.ResponseTime,
		"error", result.Error,
	)

	if err := s.store.InsertCheck(ctx, result); err != nil {
		s.logger.Error("storing check result", "service", svc.Name, "error", err)
	}

	if s.onResult != nil {
		var prevStatus *checker.Status
		if prev != nil {
			st := checker.Status(prev.Status)
			prevStatus = &st
		}
		s.onResult(result, prevStatus)
	}
}
