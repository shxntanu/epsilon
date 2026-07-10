package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shxntanu/epsilon/multiplayer/epsilond"
	"github.com/shxntanu/epsilon/multiplayer/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "epsilond:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := epsilond.LoadConfigFromEnv()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	checker := epsilond.ConfigReadiness{Config: cfg}
	var pgStore store.Store
	if cfg.PostgresDSN != "" {
		gormStore, err := store.OpenGORM(cfg.PostgresDSN)
		if err != nil {
			return err
		}
		if cfg.AutoMigrate {
			migrateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := gormStore.AutoMigrate(migrateCtx); err != nil {
				cancel()
				return err
			}
			cancel()
		}
		checker.Store = gormStore
		pgStore = gormStore
	}
	service := epsilond.NewServer(cfg,
		epsilond.WithLogger(logger),
		epsilond.WithDependencyChecker(checker),
		epsilond.WithIngestor(epsilond.BootstrapIngestor{
			Orchestrator: epsilond.BootstrapOrchestrator{},
			Store:        pgStore,
		}),
	)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: service.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("epsilond listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	return nil
}
