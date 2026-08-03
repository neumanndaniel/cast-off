// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package controller

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/neumanndaniel/cast-off/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResumeUpdateRunAfterStartupDelay(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		Namespace:              "cast-off",
		StateConfigMapName:     "cast-off-cilium",
		ManageEnvoySafeRollout: boolPtr(false),
	}

	state := State{
		DeployedVersion:          "1.19.5",
		DeployedReleaseRevision:  2,
		NodesToUpdate:            []string{},
		CastOff:                  false,
		ConfigMapChangesDetected: false,
		HelmReleaseDetected:      false,
		UpdateRunID:              "1784924151151261375",
	}

	data, err := state.toConfigMapData()
	if err != nil {
		t.Fatalf("toConfigMapData failed: %v", err)
	}

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.StateConfigMapName,
			Namespace: cfg.Namespace,
		},
		Data: data,
	})

	controller := &Controller{
		clientset:    client,
		cfg:          cfg,
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		startupDelay: 0,
	}

	if err := controller.resumeUpdateRunAfterStartupDelay(ctx); err != nil {
		t.Fatalf("resumeUpdateRunAfterStartupDelay failed: %v", err)
	}

	updated, err := controller.getState(ctx)
	if err != nil {
		t.Fatalf("getState failed: %v", err)
	}

	if !updated.CastOff {
		t.Fatalf("expected cast_off to be true after resume, got false")
	}
	if updated.UpdateRunID != "" {
		t.Fatalf("expected update_run_id to be cleared, got: %s", updated.UpdateRunID)
	}
}
