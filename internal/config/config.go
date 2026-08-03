// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package config

import (
	"log"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Namespace               string
	ReleaseName             string
	CheckInterval           time.Duration
	TerminationGracePeriod  time.Duration
	DrainTimeout            time.Duration
	PodRestartTimeout       time.Duration
	RetryBackoff            time.Duration
	MaxRetries              int
	LeaseName               string
	StateConfigMapName      string
	CiliumWorkloadNamespace string
	ManageEnvoySafeRollout  *bool
}

func MustLoad() Config {
	cfg, err := Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	return cfg
}

func Load() (Config, error) {
	checkIntervalMinutes, err := intFromEnv("CHECK_INTERVAL_MINUTES", 5)
	if err != nil {
		return Config{}, err
	}
	termGraceSeconds, err := intFromEnv("TERMINATION_GRACE_PERIOD_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	drainTimeoutSeconds, err := intFromEnv("DRAIN_TIMEOUT_SECONDS", 600)
	if err != nil {
		return Config{}, err
	}
	podRestartTimeoutSeconds, err := intFromEnv("POD_RESTART_TIMEOUT_SECONDS", 300)
	if err != nil {
		return Config{}, err
	}
	retryBackoffSeconds, err := intFromEnv("RETRY_BACKOFF_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}
	maxRetries, err := intFromEnv("MAX_RETRIES", 3)
	if err != nil {
		return Config{}, err
	}
	envoySafeRollout, err := optionalBoolFromEnv("MANAGE_ENVOY_SAFE_ROLLOUT")
	if err != nil {
		return Config{}, err
	}

	namespace := envOrDefault("NAMESPACE", "kube-system")
	releaseName := envOrDefault("HELM_RELEASE_NAME", "cilium")

	return Config{
		Namespace:               namespace,
		ReleaseName:             releaseName,
		CheckInterval:           time.Duration(checkIntervalMinutes) * time.Minute,
		TerminationGracePeriod:  time.Duration(termGraceSeconds) * time.Second,
		DrainTimeout:            time.Duration(drainTimeoutSeconds) * time.Second,
		PodRestartTimeout:       time.Duration(podRestartTimeoutSeconds) * time.Second,
		RetryBackoff:            time.Duration(retryBackoffSeconds) * time.Second,
		MaxRetries:              maxRetries,
		LeaseName:               envOrDefault("LEASE_NAME", "cast-off-leader-election"),
		StateConfigMapName:      envOrDefault("STATE_CONFIGMAP_NAME", "cast-off-cilium"),
		CiliumWorkloadNamespace: envOrDefault("CILIUM_NAMESPACE", namespace),
		ManageEnvoySafeRollout:  envoySafeRollout,
	}, nil
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func intFromEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}

func optionalBoolFromEnv(key string) (*bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
