# Cast Off Helm Chart

This chart deploys the Cast Off controller.

Cast Off runs in its own namespace (`cast-off` by default) and manages Cilium workloads in `kube-system` by default.

## Install

Install from OCI registry (recommended):

```bash
helm upgrade --install cast-off oci://quay.io/neumanndaniel/cast-off --version 0.0.1 \
  --namespace cast-off \
  --create-namespace
```

Pull first, then install:

```bash
helm pull oci://quay.io/neumanndaniel/cast-off --version 0.0.1
helm upgrade --install cast-off ./cast-off-0.0.1.tgz \
  --namespace cast-off \
  --create-namespace
```

Note: Registry login is only required for private registries.

Install from local chart source:

```bash
helm upgrade --install cast-off ./charts/cast-off \
  --namespace cast-off \
  --create-namespace
```

## Release Artifacts

Chart release artifacts are produced from repository root with Makefile targets:

```bash
make chart-release-local RELEASE_VERSION=0.0.1
```

To push to OCI registry:

```bash
make chart-release RELEASE_VERSION=0.0.1
```

Pull/install from OCI (example):

```bash
helm pull oci://quay.io/neumanndaniel/cast-off --version 0.0.1
helm upgrade --install cast-off oci://quay.io/neumanndaniel/cast-off --version 0.0.1 -n cast-off --create-namespace
```

## Values Reference

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `replicaCount` | int | `2` | Number of Cast Off pods. Leader election ensures only one active leader. |
| `image.repository` | string | `quay.io/neumanndaniel/cast-off` | Container image repository. |
| `image.pullPolicy` | string | `IfNotPresent` | Kubernetes image pull policy. |
| `image.tag` | string | `v0.0.1` | Container image tag. |
| `nameOverride` | string | `""` | Override chart name portion of generated resource names. |
| `fullnameOverride` | string | `""` | Override full generated resource name. |
| `serviceAccount.create` | bool | `true` | Create a dedicated ServiceAccount for Cast Off. |
| `serviceAccount.name` | string | `""` | Existing ServiceAccount name to use (or created name when empty). |
| `podAnnotations` | map | `{}` | Extra pod annotations for the Deployment template. |
| `podLabels` | map | `{}` | Extra pod labels for the Deployment template. |
| `priorityClassName` | string | `system-cluster-critical` | PriorityClass assigned to Cast Off pods. |
| `podSecurityContext` | map | `{"runAsNonRoot":true,"runAsUser":65532,"runAsGroup":65532,"fsGroup":65532}` | Pod-level security context. |
| `containerSecurityContext` | map | `{"allowPrivilegeEscalation":false,"readOnlyRootFilesystem":true,"capabilities":{"drop":["ALL"]}}` | Container-level security context. |
| `resources` | map | `{"requests":{"cpu":"100m","memory":"64Mi"},"limits":{"cpu":"200m","memory":"128Mi"}}` | CPU/memory requests and limits for Cast Off container. |
| `nodeSelector` | map | `{}` | Node selector for scheduling Cast Off pods. |
| `tolerations` | list | `[{"key":"node-role.kubernetes.io/control-plane","operator":"Exists","effect":"NoSchedule"},{"key":"node-role.kubernetes.io/master","operator":"Exists","effect":"NoSchedule"},{"key":"CriticalAddonsOnly","operator":"Exists"}]` | Pod tolerations for scheduling Cast Off pods. |
| `affinity` | map | `{}` | Affinity/anti-affinity rules for Cast Off pods. |
| `topologySpreadConstraints` | list | `[{"maxSkew":1,"topologyKey":"topology.kubernetes.io/zone","whenUnsatisfiable":"DoNotSchedule"}]` | Topology spread rule that keeps replicas distributed across availability zones. |
| `podDisruptionBudget.minAvailable` | int | `1` | Minimum number of Cast Off pods that must stay available during voluntary disruptions. The PodDisruptionBudget is created only when `replicaCount >= 2`. |
| `podDisruptionBudget.unhealthyPodEvictionPolicy` | string | `AlwaysAllow` | PDB unhealthy pod eviction policy. |
| `config.helmReleaseName` | string | `cilium` | Cilium Helm release name used for version/workload targeting logic. |
| `config.namespace` | string | `cast-off` | Namespace where Cast Off resources run (Deployment, ServiceAccount, state ConfigMap, leader-election Lease). |
| `config.checkIntervalMinutes` | int | `5` | Reconciliation interval for periodic version and ConfigMap checks. |
| `config.terminationGracePeriodSeconds` | int | `30` | Grace period used for pod termination and node drain behavior. |
| `config.drainTimeoutSeconds` | int | `600` | Timeout for draining a node during update stage. |
| `config.podRestartTimeoutSeconds` | int | `300` | Timeout waiting for Cilium pods to become ready after recycle. |
| `config.retryBackoffSeconds` | int | `10` | Delay between retry attempts for node operations. |
| `config.maxRetries` | int | `3` | Maximum retry attempts before blocking/aborting update progression. |
| `config.leaseName` | string | `cast-off-leader-election` | Name of the leader election Lease resource in `config.namespace`. |
| `config.stateConfigMapName` | string | `cast-off-cilium` | Name of ConfigMap used to persist Cast Off state. |
| `config.ciliumNamespace` | string | `kube-system` | Namespace where Cilium pods are managed and Cilium config/release metadata is read. |
| `config.manageEnvoySafeRollout` | bool or null | `null` | Controls whether Cast Off manages Cilium Envoy safe rollouts. When unset, Cast Off auto-detects from `cilium-config` and only enables the behavior when `enable-l7-proxy` is `true`. |

## Namespace and RBAC Scope

This chart splits permissions across one ClusterRole and two namespace-scoped Roles.

| Scope | API Group | Resource | Verbs | Why it is needed |
| --- | --- | --- | --- | --- |
| ClusterRole | `""` | `nodes` | `get`, `list`, `watch`, `patch` | List/update nodes and cordon/uncordon during rollout. |
| ClusterRole | `""` | `pods` | `get`, `list`, `watch` | Read pod state during drain and rollout checks. |
| ClusterRole | `""` | `pods/eviction` | `create` | Evict pods as part of node drain. |
| ClusterRole | `apps` | `daemonsets` | `get`, `list`, `watch` | Validate rollout state for `cilium` and `cilium-envoy` DaemonSets. |
| Role in `config.ciliumNamespace` | `""` | `pods` | `get`, `list`, `watch`, `delete` | Delete/recycle Cilium pods and verify readiness. |
| Role in `config.ciliumNamespace` | `""` | `configmaps` | `get`, `list`, `watch` | Detect changes in `cilium-config` and `cilium-envoy-config`. |
| Role in `config.ciliumNamespace` | `""` | `secrets` | `get`, `list`, `watch` | Read Helm release metadata stored in Kubernetes secrets. |
| Role in `config.namespace` | `""` | `configmaps` | `get`, `list`, `watch`, `create`, `update`, `patch` | Persist and update controller state ConfigMap. |
| Role in `config.namespace` | `coordination.k8s.io` | `leases` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` | Leader election lifecycle management. |

## State ConfigMap Fields

Cast Off persists runtime state in the ConfigMap defined by `config.stateConfigMapName` (default: `cast-off-cilium`).

- `deployed_version`: Last observed deployed Cilium app version.
- `deployed_release_revision`: Last observed deployed Helm release revision.
- `nodes_to_update`: JSON array of node names queued for the current update run.
- `cast_off`: `true` when idle; `false` when an update run is active or pending resume.
- `config_map_changes_detected`: `true` when the current run was triggered by Cilium ConfigMap changes.
- `helm_release_detected`: `true` when the current run was triggered by a Helm release rollout change.
- `update_run_id`: Identifier for the current update run; cleared when the run is fully finalized.
- `cilium_config_hash`: Last persisted content hash of `cilium-config`.
- `cilium_envoy_config_hash`: Last persisted content hash of `cilium-envoy-config`.

When ConfigMap changes are detected, Cast Off runs the same node-by-node update flow. It always waits for the `cilium` DaemonSet and waits for `cilium-envoy` as well only when Envoy safe rollout management is enabled.

On startup, if persisted state indicates an update run is already in progress (`cast_off=false`), Cast Off waits 10 seconds before resuming the update flow.

Default scheduling behavior also enforces zone spreading for Cast Off pods with `topology.kubernetes.io/zone` and `whenUnsatisfiable: DoNotSchedule`, which requires at least two schedulable zones when `replicaCount` is 2.

## Example Overrides

### Run Cast Off in a custom namespace and target a custom Cilium namespace

```yaml
config:
  namespace: platform-cast-off
  ciliumNamespace: networking-system
```

### Use a pinned image and tune operation timings

```yaml
image:
  repository: ghcr.io/your-org/cast-off
  tag: v1.2.3

config:
  checkIntervalMinutes: 3
  drainTimeoutSeconds: 900
  podRestartTimeoutSeconds: 420
  retryBackoffSeconds: 15
  maxRetries: 5
```

### Set resource requests and limits

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```
