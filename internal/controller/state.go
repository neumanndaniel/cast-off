// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package controller

import (
	"encoding/json"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

type State struct {
	DeployedVersion          string
	DeployedReleaseRevision  int
	NodesToUpdate            []string
	CastOff                  bool
	ConfigMapChangesDetected bool
	HelmReleaseDetected      bool
	UpdateRunID              string
	CiliumConfigHash         string
	CiliumEnvoyConfigHash    string
}

func defaultState() State {
	return State{
		DeployedVersion:          "",
		DeployedReleaseRevision:  0,
		NodesToUpdate:            []string{},
		CastOff:                  true,
		ConfigMapChangesDetected: false,
		HelmReleaseDetected:      false,
		UpdateRunID:              "",
		CiliumConfigHash:         "",
		CiliumEnvoyConfigHash:    "",
	}
}

func stateFromConfigMap(cm *corev1.ConfigMap) (State, error) {
	s := defaultState()
	if cm == nil {
		return s, nil
	}

	if v, ok := cm.Data["deployed_version"]; ok {
		s.DeployedVersion = v
	}

	if v, ok := cm.Data["deployed_release_revision"]; ok && v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return State{}, fmt.Errorf("parse deployed_release_revision: %w", err)
		}
		s.DeployedReleaseRevision = parsed
	}

	if v, ok := cm.Data["cast_off"]; ok {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return State{}, fmt.Errorf("parse cast_off: %w", err)
		}
		s.CastOff = parsed
	}

	if v, ok := cm.Data["nodes_to_update"]; ok && v != "" {
		if err := json.Unmarshal([]byte(v), &s.NodesToUpdate); err != nil {
			return State{}, fmt.Errorf("parse nodes_to_update: %w", err)
		}
	}

	if v, ok := cm.Data["config_map_changes_detected"]; ok {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return State{}, fmt.Errorf("parse config_map_changes_detected: %w", err)
		}
		s.ConfigMapChangesDetected = parsed
	}

	if v, ok := cm.Data["helm_release_detected"]; ok {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return State{}, fmt.Errorf("parse helm_release_detected: %w", err)
		}
		s.HelmReleaseDetected = parsed
	}

	if v, ok := cm.Data["update_run_id"]; ok {
		s.UpdateRunID = v
	}

	if v, ok := cm.Data["cilium_config_hash"]; ok {
		s.CiliumConfigHash = v
	}

	if v, ok := cm.Data["cilium_envoy_config_hash"]; ok {
		s.CiliumEnvoyConfigHash = v
	}

	return s, nil
}

func (s State) toConfigMapData() (map[string]string, error) {
	nodes, err := json.Marshal(s.NodesToUpdate)
	if err != nil {
		return nil, fmt.Errorf("marshal nodes_to_update: %w", err)
	}

	return map[string]string{
		"deployed_version":            s.DeployedVersion,
		"deployed_release_revision":   strconv.Itoa(s.DeployedReleaseRevision),
		"nodes_to_update":             string(nodes),
		"cast_off":                    strconv.FormatBool(s.CastOff),
		"config_map_changes_detected": strconv.FormatBool(s.ConfigMapChangesDetected),
		"helm_release_detected":       strconv.FormatBool(s.HelmReleaseDetected),
		"update_run_id":               s.UpdateRunID,
		"cilium_config_hash":          s.CiliumConfigHash,
		"cilium_envoy_config_hash":    s.CiliumEnvoyConfigHash,
	}, nil
}
