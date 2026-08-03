# Release Runbook

This project currently uses a Makefile-only release flow for Helm chart artifacts.

## Prerequisites

- Helm installed and available on PATH.
- Access to the OCI registry (Quay) if you want to push.
- Updated CHANGELOG.md with a section for the target version.

## Versioning

- Use semantic versions like 0.0.1.
- Pass the version via RELEASE_VERSION.
- The release flow updates charts/cast-off/Chart.yaml version and appVersion.

## Local Artifact Release (No Push)

1. Update CHANGELOG.md with a section header like `## [0.0.1] - YYYY-MM-DD`.
2. Run:

```bash
make chart-release-local RELEASE_VERSION=0.0.1
```

1. Verify artifacts in dist/:

- cast-off-0.0.1.tgz
- release-notes-0.0.1.md
- release-0.0.1/

## Push Release to OCI

1. Authenticate:

```bash
helm registry login quay.io -u <username>
```

1. Run:

```bash
make chart-release RELEASE_VERSION=0.0.1
```

1. Validate published chart:

```bash
helm pull oci://quay.io/neumanndaniel/cast-off --version 0.0.1
```

## Useful Targets

- make chart-lint
- make chart-version RELEASE_VERSION=<x.y.z>
- make chart-package RELEASE_VERSION=<x.y.z>
- make chart-push RELEASE_VERSION=<x.y.z>
- make release-notes RELEASE_VERSION=<x.y.z>
- make release-bundle RELEASE_VERSION=<x.y.z>
