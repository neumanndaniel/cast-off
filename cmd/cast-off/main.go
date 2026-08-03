// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/neumanndaniel/cast-off/internal/config"
	"github.com/neumanndaniel/cast-off/internal/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func main() {
	cfg := config.MustLoad()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	kubeconfig := flag.String("kubeconfig", "", "Path to kubeconfig file (optional when running in-cluster)")
	flag.Parse()

	restCfg, err := buildRESTConfig(*kubeconfig)
	if err != nil {
		logger.Error("failed to build Kubernetes config", "error", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logger.Error("failed to build Kubernetes client", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	id, err := os.Hostname()
	if err != nil {
		logger.Error("failed to get hostname", "error", err)
		os.Exit(1)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{Name: cfg.LeaseName, Namespace: cfg.Namespace},
		Client:    clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leaderCtx, leaderCancel := context.WithCancel(ctx)
	defer leaderCancel()

	ctrl := controller.New(clientset, cfg, logger)

	leaderConfig := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   30 * time.Second,
		RenewDeadline:   20 * time.Second,
		RetryPeriod:     5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(c context.Context) {
				logger.Info("leader acquired")
				if err := ctrl.Run(c); err != nil {
					logger.Error("controller exited with error", "error", err)
				}
			},
			OnStoppedLeading: func() {
				logger.Warn("leader lost")
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					logger.Info("leader unchanged", "identity", identity)
					return
				}
				logger.Info("leader changed", "identity", identity)
			},
		},
	}

	elector, err := leaderelection.NewLeaderElector(leaderConfig)
	if err != nil {
		logger.Error("failed to create leader elector", "error", err)
		os.Exit(1)
	}

	elector.Run(leaderCtx)
}

func buildRESTConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	return clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
}
