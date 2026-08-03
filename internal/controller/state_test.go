// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package controller

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestStateFromConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{Data: map[string]string{
		"deployed_version":            "1.16.2",
		"deployed_release_revision":   "2",
		"nodes_to_update":             "[\"node-a\",\"node-b\"]",
		"cast_off":                    "false",
		"config_map_changes_detected": "true",
		"helm_release_detected":       "true",
		"update_run_id":               "run-123",
		"cilium_config_hash":          "hash-a",
		"cilium_envoy_config_hash":    "hash-b",
	}}

	state, err := stateFromConfigMap(cm)
	if err != nil {
		t.Fatalf("stateFromConfigMap failed: %v", err)
	}

	if state.DeployedVersion != "1.16.2" {
		t.Fatalf("unexpected version: %s", state.DeployedVersion)
	}
	if state.DeployedReleaseRevision != 2 {
		t.Fatalf("unexpected deployed_release_revision: %d", state.DeployedReleaseRevision)
	}
	if state.CastOff {
		t.Fatalf("expected cast_off false")
	}
	if !state.ConfigMapChangesDetected {
		t.Fatalf("expected config_map_changes_detected true")
	}
	if !state.HelmReleaseDetected {
		t.Fatalf("expected helm_release_detected true")
	}
	if state.UpdateRunID != "run-123" {
		t.Fatalf("unexpected update_run_id: %s", state.UpdateRunID)
	}
	if state.CiliumConfigHash != "hash-a" {
		t.Fatalf("unexpected cilium_config_hash: %s", state.CiliumConfigHash)
	}
	if state.CiliumEnvoyConfigHash != "hash-b" {
		t.Fatalf("unexpected cilium_envoy_config_hash: %s", state.CiliumEnvoyConfigHash)
	}
	expectedNodes := []string{"node-a", "node-b"}
	if !reflect.DeepEqual(expectedNodes, state.NodesToUpdate) {
		t.Fatalf("unexpected nodes: %#v", state.NodesToUpdate)
	}
}

func TestStateRoundTrip(t *testing.T) {
	in := State{
		DeployedVersion:          "1.17.0",
		DeployedReleaseRevision:  7,
		NodesToUpdate:            []string{"node-x"},
		CastOff:                  true,
		ConfigMapChangesDetected: false,
		HelmReleaseDetected:      true,
		UpdateRunID:              "run-456",
		CiliumConfigHash:         "hash-c",
		CiliumEnvoyConfigHash:    "hash-d",
	}

	data, err := in.toConfigMapData()
	if err != nil {
		t.Fatalf("toConfigMapData failed: %v", err)
	}

	out, err := stateFromConfigMap(&corev1.ConfigMap{Data: data})
	if err != nil {
		t.Fatalf("stateFromConfigMap failed: %v", err)
	}

	if !reflect.DeepEqual(in, out) {
		t.Fatalf("roundtrip mismatch: in=%#v out=%#v", in, out)
	}
}

func TestStateFromConfigMapLegacyData(t *testing.T) {
	cm := &corev1.ConfigMap{Data: map[string]string{
		"deployed_version": "1.16.2",
		"nodes_to_update":  "[\"node-a\"]",
		"cast_off":         "true",
	}}

	state, err := stateFromConfigMap(cm)
	if err != nil {
		t.Fatalf("stateFromConfigMap failed: %v", err)
	}

	if state.ConfigMapChangesDetected {
		t.Fatalf("expected config_map_changes_detected false by default")
	}
	if state.DeployedReleaseRevision != 0 {
		t.Fatalf("expected deployed_release_revision default 0, got: %d", state.DeployedReleaseRevision)
	}
	if state.HelmReleaseDetected {
		t.Fatalf("expected helm_release_detected false by default")
	}
	if state.UpdateRunID != "" {
		t.Fatalf("expected empty update_run_id by default")
	}
	if state.CiliumConfigHash != "" {
		t.Fatalf("expected empty cilium_config_hash, got: %s", state.CiliumConfigHash)
	}
	if state.CiliumEnvoyConfigHash != "" {
		t.Fatalf("expected empty cilium_envoy_config_hash, got: %s", state.CiliumEnvoyConfigHash)
	}
}
