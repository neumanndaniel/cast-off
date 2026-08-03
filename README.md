# Cast Off for Cilium

[![Docker Repository on Quay](https://quay.io/repository/neumanndaniel/cast-off/status "Docker Repository on Quay")](https://quay.io/repository/neumanndaniel/cast-off) [![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Cast Off is a Go-based Kubernetes controller that orchestrates node-by-node Cilium pod refreshes after a detected Cilium version change or Cilium ConfigMap change.

> Warning: Cast Off is currently an early release. Interfaces and behavior may change as the project matures.
>
> Issues can be raised in the Cast Off GitHub repository. External contributions are not expected at this time, and will be considered and welcomed in the future once the project is more mature and stable.

The controller:

- Uses leader election so only one pod is active at a time.
- Stores rollout state in a ConfigMap.
- Processes nodes sequentially: drain node, recycle Cilium pods, wait for readiness, uncordon.
- Monitors `cilium-config` and `cilium-envoy-config` changes and triggers the same rollout flow when changes are detected.
- Tracks config-change rollout state with `config_map_changes_detected`, `cilium_config_hash`, and `cilium_envoy_config_hash` in the state ConfigMap.
- Automatically manages Cilium Envoy safe rollout behavior when `enable-l7-proxy=true`, with an explicit override available via configuration.
- Resumes in-progress update runs after restart with a 10-second startup delay when `cast_off=false`.

## Repository Structure

- `cmd/cast-off` - Application entrypoint.
- `internal/config` - Runtime configuration parsing.
- `internal/controller` - Controller state machine and Kubernetes operations.
- `charts/cast-off` - Helm chart for deployment.

## Prerequisites

- Go 1.26+
- Kubernetes cluster access (for runtime execution)
- Helm 3+ (for chart deployment)
- Docker (for container image builds)

## Build

Use the Makefile targets from repository root:

```bash
make help
make tidy
make test
make build
```

This produces a local development binary at:

- `bin/cast-off`

## Build Linux Binary for Docker

To build a Linux binary suitable for use in a Docker image:

```bash
make build-linux
```

This produces:

- `bin/cast-off-linux-amd64`

To build for a different architecture:

```bash
make build-linux GOARCH=arm64
```

## Build Container Image

Build using the Dockerfile with Makefile defaults:

```bash
make docker-build
```

By default this creates:

- `quay.io/neumanndaniel/cast-off:dev`

Override image coordinates:

```bash
make docker-build REGISTRY=docker.io REPOSITORY=your-org IMAGE_NAME=cast-off IMAGE_TAG=v1.0.0
```

Push the built image:

```bash
make docker-push REGISTRY=docker.io REPOSITORY=your-org IMAGE_NAME=cast-off IMAGE_TAG=v1.0.0
```

Build and push in one step:

```bash
make docker-release-multi REGISTRY=docker.io REPOSITORY=your-org IMAGE_NAME=cast-off IMAGE_TAG=v1.0.0
```

## Vulnerability Check

```bash
make govulncheck
```

## Local Run

```bash
make run
```

Optional kubeconfig path:

```bash
go run ./cmd/cast-off --kubeconfig ~/.kube/config
```

## Container Integration

This repository includes a multi-stage Dockerfile at `Dockerfile` that:

- Builds a static Linux binary with Go 1.26.
- Uses a Chainguard distroless runtime image (`cgr.dev/chainguard/static:latest`).
- Runs as a non-root user (`65532:65532`).

Build directly with Docker:

```bash
docker build -t cast-off:latest .
```

Run locally:

```bash
docker run --rm cast-off:latest
```

## Helm Deployment

Chart location:

- `charts/cast-off`

Lint chart:

```bash
helm lint charts/cast-off
```

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
helm upgrade --install cast-off charts/cast-off -n cast-off --create-namespace
```

## Manual Chart Release

This project currently uses a Makefile-only release flow for Helm chart artifacts.

Prerequisites:

- Helm 3.8+ with OCI support
- Registry authentication if pushing (`helm registry login`)

Release commands:

```bash
# Lint and template validation
make chart-lint

# Local artifacts only (no push)
make chart-release-local RELEASE_VERSION=0.0.1

# Full release including OCI push
make chart-release RELEASE_VERSION=0.0.1
```

Artifacts are created under `dist/`:

- `cast-off-<version>.tgz`
- `release-notes-<version>.md`
- `release-<version>/` bundle directory

See `RELEASE.md` for the full release checklist and runbook.

## State ConfigMap Fields

Cast Off persists runtime state in the ConfigMap defined by `STATE_CONFIGMAP_NAME` (default: `cast-off-cilium`).

The following list is the complete set of keys used for state management:

- `deployed_version`: Last observed deployed Cilium app version.
- `deployed_release_revision`: Last observed deployed Helm release revision.
- `nodes_to_update`: JSON array of node names queued for the current update run.
- `cast_off`: `true` when idle; `false` when an update run is active or pending resume.
- `config_map_changes_detected`: `true` when the current run was triggered by Cilium ConfigMap changes.
- `helm_release_detected`: `true` when the current run was triggered by a Helm release rollout change.
- `update_run_id`: Identifier for the current update run; cleared when the run is fully finalized.
- `cilium_config_hash`: Last persisted content hash of `cilium-config`.
- `cilium_envoy_config_hash`: Last persisted content hash of `cilium-envoy-config`.

## License

Apache License 2.0
