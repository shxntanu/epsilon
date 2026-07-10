package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shxntanu/epsilon/multiplayer/epsilond"
	"github.com/shxntanu/epsilon/multiplayer/temporal/activities"
	"github.com/shxntanu/epsilon/multiplayer/temporal/workflows"
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
	w.RegisterActivity(&activities.Activities{})

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
