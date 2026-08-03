// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neumanndaniel/cast-off/internal/config"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/drain"
)

type Controller struct {
	clientset     kubernetes.Interface
	cfg           config.Config
	logger        *slog.Logger
	updateBlocked bool
	startupDelay  time.Duration
}

const (
	ciliumConfigMapName      = "cilium-config"
	ciliumEnvoyConfigMapName = "cilium-envoy-config"
	resumeDelayAfterInit     = 10 * time.Second
)

var (
	ciliumAgentSelectors = []string{"app.kubernetes.io/name=cilium-agent", "k8s-app=cilium", "app.kubernetes.io/name=cilium"}
	ciliumEnvoySelectors = []string{"app.kubernetes.io/name=cilium-envoy", "k8s-app=cilium-envoy"}
)

func (c *Controller) manageEnvoySafeRolloutEnabled(ctx context.Context) (bool, error) {
	if c.cfg.ManageEnvoySafeRollout != nil {
		return *c.cfg.ManageEnvoySafeRollout, nil
	}

	cm, err := c.clientset.CoreV1().ConfigMaps(c.cfg.CiliumWorkloadNamespace).Get(ctx, ciliumConfigMapName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("get configmap %s in namespace %s: %w", ciliumConfigMapName, c.cfg.CiliumWorkloadNamespace, err)
	}

	raw, ok := cm.Data["enable-l7-proxy"]
	if !ok || raw == "" {
		c.logger.Info("cilium config map does not define enable-l7-proxy; envoy safe rollout disabled")
		return false, nil
	}

	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("parse enable-l7-proxy from configmap %s in namespace %s: %w", ciliumConfigMapName, c.cfg.CiliumWorkloadNamespace, err)
	}

	return enabled, nil
}

func New(clientset kubernetes.Interface, cfg config.Config, logger *slog.Logger) *Controller {
	return &Controller{clientset: clientset, cfg: cfg, logger: logger, startupDelay: resumeDelayAfterInit}
}

func (c *Controller) Run(ctx context.Context) error {
	c.logger.Info("initialization stage started")
	if err := c.stageInitialize(ctx); err != nil {
		return err
	}

	if err := c.resumeUpdateRunAfterStartupDelay(ctx); err != nil {
		return err
	}

	versionTicker := time.NewTicker(c.cfg.CheckInterval)
	defer versionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("controller shutdown requested")
			return nil
		case <-versionTicker.C:
			if err := c.reconcileWithChecks(ctx, true, true); err != nil {
				c.logger.Error("reconcile failed", "error", err)
			}
		}
	}
}

func (c *Controller) resumeUpdateRunAfterStartupDelay(ctx context.Context) error {
	state, err := c.getState(ctx)
	if err != nil {
		return err
	}

	if state.CastOff {
		return nil
	}

	delay := c.startupDelay
	c.logger.Info("state configmap indicates update run in progress; delaying resume", "update_run_id", state.UpdateRunID, "delay_seconds", int(delay/time.Second))

	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			c.logger.Info("controller shutdown requested before delayed update resume")
			return nil
		case <-timer.C:
		}
	}

	c.logger.Info("startup delay complete; resuming update run", "update_run_id", state.UpdateRunID)
	return c.stageUpdate(ctx)
}

func (c *Controller) reconcileWithChecks(ctx context.Context, checkVersion bool, checkConfigMaps bool) error {
	state, err := c.getState(ctx)
	if err != nil {
		return err
	}

	if c.updateBlocked {
		c.logger.Warn("update cycle is blocked after retry exhaustion; waiting for restart or leadership turnover")
		return nil
	}

	if state.CastOff {
		c.logger.Info("constant operational state", "cast_off", state.CastOff, "check_version", checkVersion, "check_config_maps", checkConfigMaps)

		targetVersion := state.DeployedVersion
		targetRevision := state.DeployedReleaseRevision
		versionChanged := false
		revisionChanged := false
		if checkVersion {
			currentVersion, currentRevision, err := c.getCiliumReleaseInfo(ctx)
			if err != nil {
				return err
			}

			c.logger.Info("checked cilium helm release", "current_version", currentVersion, "current_revision", currentRevision, "stored_version", state.DeployedVersion, "stored_revision", state.DeployedReleaseRevision)

			versionChanged = currentVersion != state.DeployedVersion
			revisionChanged = currentRevision != state.DeployedReleaseRevision
			targetVersion = currentVersion
			targetRevision = currentRevision

			if !versionChanged && !revisionChanged {
				c.logger.Info("no cilium helm release change detected", "version", currentVersion, "revision", currentRevision)
			} else {
				c.logger.Info("cilium helm release change detected", "from_version", state.DeployedVersion, "to_version", currentVersion, "from_revision", state.DeployedReleaseRevision, "to_revision", currentRevision)
			}
		}

		manageEnvoySafeRollout, err := c.manageEnvoySafeRolloutEnabled(ctx)
		if err != nil {
			return err
		}

		configMapChangesDetected := false
		helmReleaseDetected := false
		ciliumConfigHash := state.CiliumConfigHash
		ciliumEnvoyConfigHash := state.CiliumEnvoyConfigHash
		if checkConfigMaps {
			detected, nextState, err := c.detectCiliumConfigMapChanges(ctx, state, manageEnvoySafeRollout)
			if err != nil {
				return err
			}
			configMapChangesDetected = detected
			ciliumConfigHash = nextState.CiliumConfigHash
			ciliumEnvoyConfigHash = nextState.CiliumEnvoyConfigHash
			if !detected && (state.CiliumConfigHash == "" || (manageEnvoySafeRollout && state.CiliumEnvoyConfigHash == "")) {
				state.CiliumConfigHash = ciliumConfigHash
				state.CiliumEnvoyConfigHash = ciliumEnvoyConfigHash
				if err := c.updateState(ctx, state); err != nil {
					return err
				}
				c.logger.Info("initialized config map monitoring baseline", "cilium_config_hash", ciliumConfigHash, "cilium_envoy_config_hash", ciliumEnvoyConfigHash)
			}
		}

		if versionChanged || revisionChanged {
			rolloutDetected, rolloutDetails, err := c.ciliumWorkloadRolloutDetected(ctx, manageEnvoySafeRollout)
			if err != nil {
				return err
			}
			if !rolloutDetected {
				state.DeployedVersion = targetVersion
				state.DeployedReleaseRevision = targetRevision
				if err := c.updateState(ctx, state); err != nil {
					return err
				}
				versionChanged = false
				revisionChanged = false
				c.logger.Info("helm release version changed without cilium workload rollout; skipping update stage", "version", targetVersion, "cilium_status", rolloutDetails["cilium"], "cilium_envoy_status", rolloutDetails["cilium-envoy"])
			} else {
				helmReleaseDetected = true
				c.logger.Info("cilium workload rollout detected for helm release change", "cilium_status", rolloutDetails["cilium"], "cilium_envoy_status", rolloutDetails["cilium-envoy"])
			}
		}

		if !versionChanged && !revisionChanged && !configMapChangesDetected {
			return nil
		}

		if err := c.stageInitializeUpdate(ctx, targetVersion, targetRevision, configMapChangesDetected, helmReleaseDetected, ciliumConfigHash, ciliumEnvoyConfigHash); err != nil {
			return err
		}
	}

	return c.stageUpdate(ctx)
}

func (c *Controller) stageInitialize(ctx context.Context) error {
	state, err := c.getState(ctx)
	if err != nil {
		return err
	}

	version, revision, err := c.getCiliumReleaseInfo(ctx)
	if err != nil {
		return err
	}

	manageEnvoySafeRollout, err := c.manageEnvoySafeRolloutEnabled(ctx)
	if err != nil {
		return err
	}

	ciliumConfigHash, ciliumEnvoyConfigHash, err := c.currentCiliumConfigMapHashes(ctx, manageEnvoySafeRollout)
	if err != nil {
		return err
	}

	if state.DeployedVersion == "" {
		state = defaultState()
		state.DeployedVersion = version
		state.DeployedReleaseRevision = revision
		state.CiliumConfigHash = ciliumConfigHash
		state.CiliumEnvoyConfigHash = ciliumEnvoyConfigHash
		if err := c.updateState(ctx, state); err != nil {
			return err
		}
		c.logger.Info("created state configmap", "deployed_version", version, "deployed_release_revision", revision, "cilium_config_hash", ciliumConfigHash, "cilium_envoy_config_hash", ciliumEnvoyConfigHash, "manage_envoy_safe_rollout", manageEnvoySafeRollout)
		return nil
	}

	c.logger.Info("state configmap already exists", "deployed_version", state.DeployedVersion, "deployed_release_revision", state.DeployedReleaseRevision, "cast_off", state.CastOff, "config_map_changes_detected", state.ConfigMapChangesDetected)
	return nil
}

func (c *Controller) stageInitializeUpdate(ctx context.Context, version string, revision int, configMapChangesDetected bool, helmReleaseDetected bool, ciliumConfigHash string, ciliumEnvoyConfigHash string) error {
	c.logger.Info("initialize update stage started", "version", version, "config_map_changes_detected", configMapChangesDetected, "helm_release_detected", helmReleaseDetected)
	state, err := c.getState(ctx)
	if err != nil {
		return err
	}

	if !state.CastOff || len(state.NodesToUpdate) > 0 {
		c.logger.Info("update initialization skipped because another update run is already in progress", "update_run_id", state.UpdateRunID, "cast_off", state.CastOff, "remaining_nodes", len(state.NodesToUpdate))
		return nil
	}

	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	names := make([]string, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		names = append(names, n.Name)
	}
	sort.Strings(names)

	state.DeployedVersion = version
	state.DeployedReleaseRevision = revision
	state.NodesToUpdate = names
	state.CastOff = false
	state.ConfigMapChangesDetected = configMapChangesDetected
	state.HelmReleaseDetected = helmReleaseDetected
	state.UpdateRunID = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	state.CiliumConfigHash = ciliumConfigHash
	state.CiliumEnvoyConfigHash = ciliumEnvoyConfigHash

	if err := c.updateState(ctx, state); err != nil {
		return err
	}

	c.logger.Info("initialize update stage complete", "nodes_to_update", len(names), "version", version, "revision", revision, "config_map_changes_detected", configMapChangesDetected, "helm_release_detected", helmReleaseDetected, "update_run_id", state.UpdateRunID)
	return nil
}

func (c *Controller) stageUpdate(ctx context.Context) error {
	state, err := c.getState(ctx)
	if err != nil {
		return err
	}

	manageEnvoySafeRollout, err := c.manageEnvoySafeRolloutEnabled(ctx)
	if err != nil {
		return err
	}

	version := state.DeployedVersion
	c.logger.Info("update stage started", "version", version, "config_map_changes_detected", state.ConfigMapChangesDetected, "helm_release_detected", state.HelmReleaseDetected, "update_run_id", state.UpdateRunID)

	if len(state.NodesToUpdate) == 0 {
		if err := c.finalizeUpdateIfReady(ctx, &state); err != nil {
			return err
		}
		return nil
	}

	for len(state.NodesToUpdate) > 0 {
		nodeName := state.NodesToUpdate[0]
		wasLastNode := len(state.NodesToUpdate) == 1
		node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				c.logger.Warn("node from queue no longer exists; skipping", "node", nodeName)
				state.NodesToUpdate = state.NodesToUpdate[1:]
				state.CastOff = len(state.NodesToUpdate) == 0 && !state.ConfigMapChangesDetected && !state.HelmReleaseDetected
				if err := c.updateState(ctx, state); err != nil {
					return err
				}
				continue
			}
			return err
		}

		if err := withRetry(ctx, c.cfg.MaxRetries, c.cfg.RetryBackoff, func() error {
			return c.drainNode(ctx, node)
		}); err != nil {
			c.updateBlocked = true
			return fmt.Errorf("drain node %s failed: %w", nodeName, err)
		}

		if err := withRetry(ctx, c.cfg.MaxRetries, c.cfg.RetryBackoff, func() error {
			if err := c.recycleCiliumPodsOnNode(ctx, nodeName, manageEnvoySafeRollout); err != nil {
				return err
			}
			return c.waitForCiliumPodsReady(ctx, nodeName, manageEnvoySafeRollout)
		}); err != nil {
			c.updateBlocked = true
			return fmt.Errorf("recycle cilium pods on node %s failed: %w", nodeName, err)
		}

		if err := withRetry(ctx, c.cfg.MaxRetries, c.cfg.RetryBackoff, func() error {
			return c.uncordonNode(ctx, nodeName)
		}); err != nil {
			c.updateBlocked = true
			return fmt.Errorf("uncordon node %s failed: %w", nodeName, err)
		}

		state.NodesToUpdate = state.NodesToUpdate[1:]
		state.CastOff = len(state.NodesToUpdate) == 0 && !state.ConfigMapChangesDetected && !state.HelmReleaseDetected
		if err := c.updateState(ctx, state); err != nil {
			return err
		}

		if wasLastNode {
			c.logger.Info("final update state persisted to configmap", "node", nodeName, "remaining_nodes", len(state.NodesToUpdate), "cast_off", state.CastOff)
			c.logger.Info("update run finished", "update_run_id", state.UpdateRunID)
		}

		c.logger.Info("node update completed", "node", nodeName, "remaining_nodes", len(state.NodesToUpdate), "manage_envoy_safe_rollout", manageEnvoySafeRollout)
	}

	return c.finalizeUpdateIfReady(ctx, &state)
}

func (c *Controller) finalizeUpdateIfReady(ctx context.Context, state *State) error {
	state.CastOff = true
	state.ConfigMapChangesDetected = false
	state.HelmReleaseDetected = false
	state.UpdateRunID = ""
	if err := c.updateState(ctx, *state); err != nil {
		return err
	}

	c.logger.Info("update stage complete; returning to constant operational state", "version", state.DeployedVersion, "config_map_changes_detected", state.ConfigMapChangesDetected, "helm_release_detected", state.HelmReleaseDetected, "update_run_id", state.UpdateRunID)
	return nil
}

func (c *Controller) ciliumWorkloadRolloutDetected(ctx context.Context, manageEnvoySafeRollout bool) (bool, map[string]string, error) {
	workloads := []string{"cilium"}
	if manageEnvoySafeRollout {
		workloads = append(workloads, "cilium-envoy")
	}
	details := map[string]string{}
	rolloutDetected := false

	for _, daemonSetName := range workloads {
		detail, detected, err := c.daemonSetRolloutDetail(ctx, daemonSetName)
		if err != nil {
			return false, nil, err
		}

		details[daemonSetName] = detail
		if detected {
			rolloutDetected = true
		}
	}

	return rolloutDetected, details, nil
}

func (c *Controller) daemonSetRolloutDetail(ctx context.Context, daemonSetName string) (string, bool, error) {
	daemonSet, err := c.clientset.AppsV1().DaemonSets(c.cfg.CiliumWorkloadNamespace).Get(ctx, daemonSetName, metav1.GetOptions{})
	if err != nil {
		return "", false, fmt.Errorf("get daemonset %s in namespace %s: %w", daemonSetName, c.cfg.CiliumWorkloadNamespace, err)
	}

	if daemonSet.Generation != daemonSet.Status.ObservedGeneration {
		return fmt.Sprintf("daemonset %s is updated but rollout has not started", daemonSetName), true, nil
	}

	if daemonSet.Status.UpdatedNumberScheduled < daemonSet.Status.DesiredNumberScheduled {
		return fmt.Sprintf("daemonset %s is rolling out - %d out of %d pods updated", daemonSetName, daemonSet.Status.UpdatedNumberScheduled, daemonSet.Status.DesiredNumberScheduled), true, nil
	}

	return "no rollout detected", false, nil
}

func (c *Controller) detectCiliumConfigMapChanges(ctx context.Context, state State, manageEnvoySafeRollout bool) (bool, State, error) {
	next := state
	currentCiliumConfigHash, currentCiliumEnvoyConfigHash, err := c.currentCiliumConfigMapHashes(ctx, manageEnvoySafeRollout)
	if err != nil {
		return false, state, err
	}

	next.CiliumConfigHash = currentCiliumConfigHash
	next.CiliumEnvoyConfigHash = currentCiliumEnvoyConfigHash

	if state.CiliumConfigHash == "" || (manageEnvoySafeRollout && state.CiliumEnvoyConfigHash == "") {
		c.logger.Info("config map hash baseline captured", "cilium_config_hash", currentCiliumConfigHash, "cilium_envoy_config_hash", currentCiliumEnvoyConfigHash)
		return false, next, nil
	}

	if state.CiliumConfigHash != currentCiliumConfigHash || (manageEnvoySafeRollout && state.CiliumEnvoyConfigHash != currentCiliumEnvoyConfigHash) {
		c.logger.Info("cilium config map change detected", "cilium_config_hash_before", state.CiliumConfigHash, "cilium_config_hash_after", currentCiliumConfigHash, "cilium_envoy_config_hash_before", state.CiliumEnvoyConfigHash, "cilium_envoy_config_hash_after", currentCiliumEnvoyConfigHash)
		return true, next, nil
	}

	c.logger.Info("no cilium config map changes detected", "cilium_config_hash", currentCiliumConfigHash, "cilium_envoy_config_hash", currentCiliumEnvoyConfigHash)
	return false, next, nil
}

func (c *Controller) currentCiliumConfigMapHashes(ctx context.Context, manageEnvoySafeRollout bool) (string, string, error) {
	ciliumConfigMap, err := c.clientset.CoreV1().ConfigMaps(c.cfg.CiliumWorkloadNamespace).Get(ctx, ciliumConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("get configmap %s in namespace %s: %w", ciliumConfigMapName, c.cfg.CiliumWorkloadNamespace, err)
	}

	if !manageEnvoySafeRollout {
		return configMapContentHash(ciliumConfigMap), "", nil
	}

	ciliumEnvoyConfigMap, err := c.clientset.CoreV1().ConfigMaps(c.cfg.CiliumWorkloadNamespace).Get(ctx, ciliumEnvoyConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("get configmap %s in namespace %s: %w", ciliumEnvoyConfigMapName, c.cfg.CiliumWorkloadNamespace, err)
	}

	return configMapContentHash(ciliumConfigMap), configMapContentHash(ciliumEnvoyConfigMap), nil
}

func configMapContentHash(cm *corev1.ConfigMap) string {
	h := sha256.New()

	dataKeys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		dataKeys = append(dataKeys, k)
	}
	sort.Strings(dataKeys)
	for _, k := range dataKeys {
		_, _ = h.Write([]byte("data:"))
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(cm.Data[k]))
		_, _ = h.Write([]byte{0})
	}

	binaryDataKeys := make([]string, 0, len(cm.BinaryData))
	for k := range cm.BinaryData {
		binaryDataKeys = append(binaryDataKeys, k)
	}
	sort.Strings(binaryDataKeys)
	for _, k := range binaryDataKeys {
		_, _ = h.Write([]byte("binary:"))
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(cm.BinaryData[k])
		_, _ = h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))
}

func (c *Controller) getState(ctx context.Context) (State, error) {
	cm, err := c.clientset.CoreV1().ConfigMaps(c.cfg.Namespace).Get(ctx, c.cfg.StateConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return defaultState(), nil
		}
		return State{}, err
	}
	return stateFromConfigMap(cm)
}

func (c *Controller) updateState(ctx context.Context, state State) error {
	data, err := state.toConfigMapData()
	if err != nil {
		return err
	}

	existing, err := c.clientset.CoreV1().ConfigMaps(c.cfg.Namespace).Get(ctx, c.cfg.StateConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		_, createErr := c.clientset.CoreV1().ConfigMaps(c.cfg.Namespace).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.cfg.StateConfigMapName,
				Namespace: c.cfg.Namespace,
			},
			Data: data,
		}, metav1.CreateOptions{})
		return createErr
	}

	existing.Data = data
	_, err = c.clientset.CoreV1().ConfigMaps(c.cfg.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Controller) getCiliumReleaseInfo(ctx context.Context) (string, int, error) {
	store := driver.NewSecrets(c.clientset.CoreV1().Secrets(c.cfg.CiliumWorkloadNamespace))
	releases, err := store.List(func(rel *release.Release) bool {
		if rel == nil || rel.Info == nil {
			return false
		}
		return rel.Name == c.cfg.ReleaseName && rel.Info.Status == release.StatusDeployed
	})
	if err != nil {
		return "", 0, fmt.Errorf("list helm releases from secrets in namespace %s: %w", c.cfg.CiliumWorkloadNamespace, err)
	}

	version, revision, err := deployedReleaseInfo(releases, c.cfg.ReleaseName)
	if err != nil {
		return "", 0, err
	}
	return version, revision, nil
}

func deployedReleaseInfo(releases []*release.Release, releaseName string) (string, int, error) {
	if len(releases) == 0 {
		return "", 0, fmt.Errorf("helm release %s not found in deployed state", releaseName)
	}

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Version > releases[j].Version
	})

	latest := releases[0]
	if latest.Chart == nil || latest.Chart.Metadata == nil {
		return "", 0, fmt.Errorf("helm release %s has no chart metadata", releaseName)
	}

	appVersion := latest.Chart.Metadata.AppVersion
	if appVersion == "" {
		return "", 0, fmt.Errorf("helm release %s has empty chart appVersion", releaseName)
	}

	return appVersion, latest.Version, nil
}

func (c *Controller) drainNode(ctx context.Context, node *corev1.Node) error {
	c.logger.Info("node drain started", "node", node.Name)

	helper := &drain.Helper{
		Ctx:                 ctx,
		Client:              c.clientset,
		Force:               true,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  true,
		GracePeriodSeconds:  int(c.cfg.TerminationGracePeriod.Seconds()),
		Timeout:             c.cfg.DrainTimeout,
		Out:                 io.Discard,
		ErrOut:              io.Discard,
	}

	if err := drain.RunCordonOrUncordon(helper, node, true); err != nil {
		return err
	}
	c.logger.Info("node cordoned", "node", node.Name)

	if err := drain.RunNodeDrain(helper, node.Name); err != nil {
		return err
	}

	c.logger.Info("node drained", "node", node.Name)
	return nil
}

func (c *Controller) uncordonNode(ctx context.Context, nodeName string) error {
	c.logger.Info("node uncordon started", "node", nodeName)

	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get node %s: %w", nodeName, err)
	}
	helper := &drain.Helper{
		Ctx:    ctx,
		Client: c.clientset,
		Out:    io.Discard,
		ErrOut: io.Discard,
	}

	if err := drain.RunCordonOrUncordon(helper, node, false); err != nil {
		return err
	}

	c.logger.Info("node uncordoned", "node", nodeName)
	return nil
}

type ciliumWorkloadSelector struct {
	name      string
	selectors []string
}

func (c *Controller) recycleCiliumPodsOnNode(ctx context.Context, nodeName string, manageEnvoySafeRollout bool) error {
	workloads := []ciliumWorkloadSelector{
		{name: "cilium-agent", selectors: ciliumAgentSelectors},
	}
	if manageEnvoySafeRollout {
		workloads = append(workloads, ciliumWorkloadSelector{name: "cilium-envoy", selectors: ciliumEnvoySelectors})
	}

	for _, workload := range workloads {
		pods, err := c.listPodsOnNodeForSelectors(ctx, nodeName, workload.selectors)
		if err != nil {
			return err
		}

		for _, pod := range pods {
			if err := c.clientset.CoreV1().Pods(c.cfg.CiliumWorkloadNamespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}

		c.logger.Info("cilium workload pods recycled", "node", nodeName, "workload", workload.name, "deleted_pods", len(pods))
	}

	return nil
}

func (c *Controller) waitForCiliumPodsReady(ctx context.Context, nodeName string, manageEnvoySafeRollout bool) error {
	deadline := time.Now().Add(c.cfg.PodRestartTimeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for cilium pods to be ready on node %s", nodeName)
		}

		agentReady, agentCount, err := c.isWorkloadReadyOnNode(ctx, nodeName, ciliumAgentSelectors)
		if err != nil {
			return err
		}
		envoyReady := true
		envoyCount := 0
		if manageEnvoySafeRollout {
			envoyReady, envoyCount, err = c.isWorkloadReadyOnNode(ctx, nodeName, ciliumEnvoySelectors)
			if err != nil {
				return err
			}
		}

		if agentReady && envoyReady {
			c.logger.Info("cilium workloads are ready on node", "node", nodeName, "agent_pods", agentCount, "manage_envoy_safe_rollout", manageEnvoySafeRollout, "envoy_pods", envoyCount)
			return nil
		}

		c.logger.Info("waiting for cilium workloads to become ready", "node", nodeName, "agent_ready", agentReady, "agent_pods", agentCount, "manage_envoy_safe_rollout", manageEnvoySafeRollout, "envoy_ready", envoyReady, "envoy_pods", envoyCount)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *Controller) isWorkloadReadyOnNode(ctx context.Context, nodeName string, selectors []string) (bool, int, error) {
	pods, err := c.listPodsOnNodeForSelectors(ctx, nodeName, selectors)
	if err != nil {
		return false, 0, err
	}
	if len(pods) == 0 {
		return false, 0, nil
	}

	for _, p := range pods {
		if p.Status.Phase != corev1.PodRunning {
			return false, len(pods), nil
		}
		if !podReady(&p) {
			return false, len(pods), nil
		}
	}
	return true, len(pods), nil
}

func (c *Controller) listPodsOnNodeForSelectors(ctx context.Context, nodeName string, selectors []string) ([]corev1.Pod, error) {
	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", nodeName).String()
	seen := make(map[string]struct{})
	result := make([]corev1.Pod, 0)

	for _, selector := range selectors {
		pods, err := c.clientset.CoreV1().Pods(c.cfg.CiliumWorkloadNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
			FieldSelector: fieldSelector,
		})
		if err != nil {
			return nil, err
		}

		for _, pod := range pods.Items {
			if !strings.HasPrefix(pod.Name, "cilium") {
				continue
			}
			uid := string(pod.UID)
			if _, ok := seen[uid]; ok {
				continue
			}
			seen[uid] = struct{}{}
			result = append(result, pod)
		}
	}

	return result, nil
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
