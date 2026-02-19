package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/hazz-dev/svcmon/internal/alert"
	"github.com/hazz-dev/svcmon/internal/checker"
	"github.com/hazz-dev/svcmon/internal/config"
	"github.com/hazz-dev/svcmon/internal/dashboard"
	"github.com/hazz-dev/svcmon/internal/scheduler"
	"github.com/hazz-dev/svcmon/internal/server"
	"github.com/hazz-dev/svcmon/internal/storage"
	"github.com/hazz-dev/svcmon/internal/version"
)

var cfgFile string

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "svcmon",
		Short:        "Self-hosted service health monitor",
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&cfgFile, "config", "config.yml", "config file path")

	root.AddCommand(versionCmd())
	root.AddCommand(serveCmd())
	root.AddCommand(checkCmd())
	root.AddCommand(statusCmd())

	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("svcmon %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
		},
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the service monitor",
		RunE:  runServe,
	}
}

func runServe(cmd *cobra.Command, _ []string) error {
	logger := slog.Default()

	// 1. Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	logger.Info("config loaded", "services", len(cfg.Services))

	// 2. Open SQLite
	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// 3. Build alerter (if configured)
	var alerter *alert.Alerter
	if cfg.Alerts.Webhook.URL != "" {
		alerter = alert.New(cfg.Alerts.Webhook.URL, cfg.Alerts.Webhook.Cooldown.Duration, logger)
	}

	// 4. Build scheduler
	factory := func(svc config.Service) (checker.Checker, error) {
		return checker.New(svc)
	}
	sched := scheduler.New(cfg.Services, db, factory, logger)
	if alerter != nil {
		sched.SetOnResult(alerter.Notify)
	}

	// 5. Build API server
	apiServer := server.New(db, cfg.Services, logger)

	// 6. Mount routes on a single mux
	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Router())
	mux.Handle("/", dashboard.Handler())

	httpServer := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: mux,
	}

	// 7. Signal context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// 8. Start scheduler
	sched.Start(ctx)
	logger.Info("scheduler started", "services", len(cfg.Services))

	// 9. Start HTTP server in background
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "address", cfg.Server.Address)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// 10. Wait for signal or server error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		return fmt.Errorf("HTTP server: %w", err)
	}

	// 11. Graceful shutdown
	sched.Wait()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown", "error", err)
	}

	logger.Info("shutdown complete")
	return nil
}

func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run a one-off check of all configured services",
		RunE:  runCheck,
	}
}

func runCheck(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return executeCheck(cmd, cfg)
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print current service status from database",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	return executeStatus(cmd, db)
}
