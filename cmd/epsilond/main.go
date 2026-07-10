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

	"github.com/shxntanu/epsilon/multiplayer/chat"
	"github.com/shxntanu/epsilon/multiplayer/chat/googlechat"
	"github.com/shxntanu/epsilon/multiplayer/epsilond"
	"github.com/shxntanu/epsilon/multiplayer/store"
	temporalorchestrator "github.com/shxntanu/epsilon/multiplayer/temporal"
	"go.temporal.io/sdk/client"
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
	options := []epsilond.ServerOption{
		epsilond.WithLogger(logger),
		epsilond.WithDependencyChecker(checker),
	}
	if cfg.GoogleChatAudience != "" {
		options = append(options, epsilond.WithRequestVerifier(googlechat.IDTokenVerifier{
			Audience: cfg.GoogleChatAudience,
		}))
	}
	var replyClient chat.Client
	if cfg.GoogleChatAccessToken != "" {
		replyClient = &googlechat.Client{
			TokenProvider: googlechat.StaticTokenProvider{Token: cfg.GoogleChatAccessToken},
		}
	}
	chatOrchestrator := epsilond.ChatOrchestrator(epsilond.BootstrapOrchestrator{})
	var orchestratorClient *temporalorchestrator.Client
	var temporalClient client.Client
	if cfg.TemporalAddress != "" {
		temporalClient, err = client.Dial(client.Options{
			HostPort:  cfg.TemporalAddress,
			Namespace: cfg.TemporalNamespace,
		})
		if err != nil {
			return fmt.Errorf("dial temporal: %w", err)
		}
		defer temporalClient.Close()
		orchestratorClient, err = temporalorchestrator.NewClient(temporalClient, cfg.TemporalTaskQueue)
		if err != nil {
			return err
		}
		chatOrchestrator = epsilond.TemporalOrchestrator{Client: orchestratorClient}
	}
	if orchestratorClient != nil {
		options = append(options, epsilond.WithOrchestratorClient(orchestratorClient))
	}
	options = append(options, epsilond.WithIngestor(epsilond.BootstrapIngestor{
		Orchestrator: chatOrchestrator,
		ReplyClient:  replyClient,
		Store:        pgStore,
	}))
	service := epsilond.NewServer(cfg, options...)
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
