# Cast Off Roadmap

## Table of Contents

- [Parallel Node Updates](#parallel-node-updates)
- [Prometheus Metrics](#prometheus-metrics)
- [Cilium ztunnel Rollout Management](#cilium-ztunnel-rollout-management)
- [Artifact Hub Publication](#artifact-hub-publication)

## Parallel Node Updates

### Objective

Allow multiple nodes to be updated in parallel with a configurable concurrency limit.

### Scope

- Introduce a `maxConcurrentNodeUpdates` (name can be adjusted to existing config style) setting.
- Update reconcile/update stage logic to process up to the configured limit concurrently.
- Preserve ordering and safety constraints where needed (for example, avoid control-plane disruption patterns if applicable).
- Ensure state persistence remains consistent in ConfigMap when multiple nodes are in-flight.

### Deliverables

- Concurrency-aware scheduler/worker logic in controller update stage.
- Validation for invalid values (`0`, negative values, or very high unsafe values).
- Unit tests for:
  - limit enforcement,
  - completion handling,
  - retry/failure behavior with multiple in-flight nodes,
  - state recovery after restart.

### Success Criteria

- Throughput improves predictably versus single-node updates.
- No regressions in rollout correctness.
- Configurable limit works at runtime through configuration.

## Prometheus Metrics

### Objective

Expose meaningful metrics for rollout progress and configuration-driven events.

### Scope

- Add metrics endpoint and instrumentation for:
  - node update progress (queued, in-flight, succeeded, failed),
  - per-stage reconcile durations,
  - retry counts and terminal failures,
  - ConfigMap change detection events.
- Define metric names, labels, and cardinality guardrails.
- Add documentation with sample PromQL queries and alerts.

### Deliverables

- Metrics package/instrumentation integrated into reconcile flow.
- `/metrics` exposure in deployment and chart values configuration.
- Test coverage for metric updates on key transitions.
- Ops documentation section in README.

### Success Criteria

- Operators can track rollout status without parsing logs.
- Metric cardinality remains bounded in realistic cluster sizes.
- Basic alerting can detect stalled or failing rollouts.

## Cilium ztunnel Rollout Management

### Objective

Extend Cast Off to manage Cilium ztunnel rollout workflows.

### Scope

- Add ztunnel-specific rollout detection and status checks.
- Integrate ztunnel into upgrade decision/state model.
- Define safety gates for ztunnel progression and rollback behavior.
- Ensure compatibility with existing Cilium upgrade orchestration.

### Deliverables

- Controller logic for ztunnel lifecycle monitoring and coordination.
- Configuration options to enable/disable ztunnel management.
- Integration tests for mixed scenarios (Cilium-only, ztunnel-only, combined).
- Documentation updates covering ztunnel prerequisites and behavior.

### Success Criteria

- ztunnel rollouts are orchestrated with the same safety guarantees as node updates.
- Combined Cilium + ztunnel operations are deterministic and recoverable.

## Artifact Hub Publication

### Objective

Publish and maintain the Helm chart in Artifact Hub.

### Scope

- Prepare chart metadata and quality requirements for Artifact Hub.
- Add Artifact Hub annotations, maintainers, links, and security metadata.
- Set up release automation to package/publish chart artifacts.
- Verify discoverability and installability from Artifact Hub.

### Deliverables

- Updated chart metadata and docs for Artifact Hub compliance.
- CI/CD workflow for chart release publishing.
- Submission completed and listing verified.

### Success Criteria

- Chart appears in Artifact Hub with complete metadata.
- Releases are reproducible and automated.
- Users can install from published chart without manual steps.
