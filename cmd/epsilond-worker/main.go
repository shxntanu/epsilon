package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shxntanu/epsilon/multiplayer/chat/googlechat"
	"github.com/shxntanu/epsilon/multiplayer/epsilond"
	"github.com/shxntanu/epsilon/multiplayer/store"
	"github.com/shxntanu/epsilon/multiplayer/temporal/activities"
	"github.com/shxntanu/epsilon/multiplayer/temporal/workflows"
	"github.com/shxntanu/epsilon/multiplayer/workspace"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "epsilond-worker:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := epsilond.LoadConfigFromEnv()
	if err != nil {
		return err
	}
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
		pgStore = gormStore
	}
	workspaceManager, err := workspace.NewManager(workspace.Config{
		WorkspaceRoot: cfg.WorkspaceRoot,
		RepoCacheRoot: cfg.RepoCacheRoot,
	})
	if err != nil {
		return err
	}
	var googleClient *googlechat.Client
	if cfg.GoogleChatAccessToken != "" {
		googleClient = &googlechat.Client{
			TokenProvider: googlechat.StaticTokenProvider{Token: cfg.GoogleChatAccessToken},
		}
	}
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		return fmt.Errorf("dial temporal: %w", err)
	}
	defer temporalClient.Close()

	w := worker.New(temporalClient, cfg.TemporalTaskQueue, worker.Options{})
	w.RegisterWorkflowWithOptions(workflows.ThreadWorkflow, workflowRegisterOptions(workflows.ThreadWorkflowName))
	activitySet := &activities.Activities{
		Store:            pgStore,
		WorkspaceManager: workspaceManager,
	}
	if googleClient != nil {
		activitySet.ChatClient = googleClient
		activitySet.AttachmentClient = googleClient
	}
	w.RegisterActivity(activitySet)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("epsilond-worker starting", "temporal_address", cfg.TemporalAddress, "namespace", cfg.TemporalNamespace, "task_queue", cfg.TemporalTaskQueue)
	if err := w.Start(); err != nil {
		return fmt.Errorf("start temporal worker: %w", err)
	}
	<-ctx.Done()
	logger.Info("epsilond-worker stopping")
	w.Stop()
	return nil
}

func workflowRegisterOptions(name string) workflow.RegisterOptions {
	return workflow.RegisterOptions{Name: name}
}
