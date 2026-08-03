// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package controller

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/neumanndaniel/cast-off/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestManageEnvoySafeRolloutExplicitOverrideWins(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		Namespace:               "cast-off",
		CiliumWorkloadNamespace: "kube-system",
		ManageEnvoySafeRollout:  boolPtr(false),
	}

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ciliumConfigMapName, Namespace: cfg.CiliumWorkloadNamespace},
		Data: map[string]string{
			"enable-l7-proxy": "true",
		},
	})

	controller := &Controller{clientset: client, cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	enabled, err := controller.manageEnvoySafeRolloutEnabled(ctx)
	if err != nil {
		t.Fatalf("manageEnvoySafeRolloutEnabled failed: %v", err)
	}
	if enabled {
		t.Fatalf("expected explicit override false to win")
	}
}

func TestManageEnvoySafeRolloutAutoDetectsFromConfigMap(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		Namespace:               "cast-off",
		CiliumWorkloadNamespace: "kube-system",
	}

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ciliumConfigMapName, Namespace: cfg.CiliumWorkloadNamespace},
		Data: map[string]string{
			"enable-l7-proxy": "true",
		},
	})

	controller := &Controller{clientset: client, cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	enabled, err := controller.manageEnvoySafeRolloutEnabled(ctx)
	if err != nil {
		t.Fatalf("manageEnvoySafeRolloutEnabled failed: %v", err)
	}
	if !enabled {
		t.Fatalf("expected auto-detection to enable Envoy safe rollout when enable-l7-proxy is true")
	}
}

func TestRecycleCiliumPodsOnNodeSkipsEnvoyWhenDisabled(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		Namespace:               "cast-off",
		CiliumWorkloadNamespace: "kube-system",
	}

	client := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-agent-node-a",
				Namespace: cfg.CiliumWorkloadNamespace,
				UID:       "agent-uid",
				Labels: map[string]string{
					"app.kubernetes.io/name": "cilium-agent",
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-a"},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-envoy-node-a",
				Namespace: cfg.CiliumWorkloadNamespace,
				UID:       "envoy-uid",
				Labels: map[string]string{
					"app.kubernetes.io/name": "cilium-envoy",
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-a"},
		},
	)

	controller := &Controller{clientset: client, cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	if err := controller.recycleCiliumPodsOnNode(ctx, "node-a", false); err != nil {
		t.Fatalf("recycleCiliumPodsOnNode failed: %v", err)
	}

	remaining, err := client.CoreV1().Pods(cfg.CiliumWorkloadNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pods failed: %v", err)
	}

	if len(remaining.Items) != 1 {
		t.Fatalf("expected 1 remaining pod, got %d", len(remaining.Items))
	}
	if remaining.Items[0].Name != "cilium-envoy-node-a" {
		t.Fatalf("expected envoy pod to remain, got %s", remaining.Items[0].Name)
	}
}

func TestWaitForCiliumPodsReadySkipsEnvoyWhenDisabled(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		Namespace:               "cast-off",
		CiliumWorkloadNamespace: "kube-system",
		PodRestartTimeout:       50 * time.Millisecond,
	}

	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cilium-agent-node-a",
			Namespace: cfg.CiliumWorkloadNamespace,
			UID:       "agent-uid",
			Labels: map[string]string{
				"app.kubernetes.io/name": "cilium-agent",
			},
		},
		Spec: corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})

	controller := &Controller{clientset: client, cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	if err := controller.waitForCiliumPodsReady(ctx, "node-a", false); err != nil {
		t.Fatalf("waitForCiliumPodsReady failed: %v", err)
	}
}

func TestCiliumWorkloadRolloutDetectedSkipsEnvoyWhenDisabled(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		Namespace:               "cast-off",
		CiliumWorkloadNamespace: "kube-system",
	}

	client := fake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cilium",
			Namespace:  cfg.CiliumWorkloadNamespace,
			Generation: 2,
		},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     2,
			DesiredNumberScheduled: 1,
			UpdatedNumberScheduled: 1,
		},
	})

	controller := &Controller{clientset: client, cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	detected, details, err := controller.ciliumWorkloadRolloutDetected(ctx, false)
	if err != nil {
		t.Fatalf("ciliumWorkloadRolloutDetected failed: %v", err)
	}
	if detected {
		t.Fatalf("expected no rollout to be detected")
	}
	if details["cilium"] != "no rollout detected" {
		t.Fatalf("unexpected cilium detail: %v", details["cilium"])
	}
	if _, ok := details["cilium-envoy"]; ok {
		t.Fatalf("did not expect envoy rollout details when disabled")
	}
}

func TestCiliumWorkloadRolloutDetectedIncludesEnvoyStatusWhenEnabled(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		Namespace:               "cast-off",
		CiliumWorkloadNamespace: "kube-system",
	}

	client := fake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "cilium",
				Namespace:  cfg.CiliumWorkloadNamespace,
				Generation: 2,
			},
			Status: appsv1.DaemonSetStatus{
				ObservedGeneration:     2,
				DesiredNumberScheduled: 4,
				UpdatedNumberScheduled: 0,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "cilium-envoy",
				Namespace:  cfg.CiliumWorkloadNamespace,
				Generation: 1,
			},
			Status: appsv1.DaemonSetStatus{
				ObservedGeneration:     1,
				DesiredNumberScheduled: 4,
				UpdatedNumberScheduled: 4,
			},
		},
	)

	controller := &Controller{clientset: client, cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	detected, details, err := controller.ciliumWorkloadRolloutDetected(ctx, true)
	if err != nil {
		t.Fatalf("ciliumWorkloadRolloutDetected failed: %v", err)
	}
	if !detected {
		t.Fatalf("expected rollout to be detected")
	}
	if details["cilium"] == "" {
		t.Fatalf("expected cilium rollout detail")
	}
	if details["cilium-envoy"] != "no rollout detected" {
		t.Fatalf("expected envoy rollout detail to be populated, got %q", details["cilium-envoy"])
	}
}
