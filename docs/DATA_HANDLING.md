# Data Handling Policy

*Ingress Shift — Analyzer, Shadow & Diff Harness, Cutover Orchestrator*

This page is written to be pasted directly into a customer security questionnaire. It covers all three tools in this repository.

## Summary

All three tools run entirely on-premises, inside the customer's own network. None of them phone home, send telemetry, or make any outbound network call other than the ones the customer explicitly configures (the Kubernetes API server, the customer's own Prometheus, the customer's own ingress/gateway backends). No data — configuration, traffic, credentials, or reports — leaves the customer's network as part of normal operation. All three ship as single static binaries with no bundled runtime or phone-home library, so this can be verified by binary inspection or network policy enforcement (e.g., a default-deny egress NetworkPolicy plus explicit allows for the API server, Prometheus, and backend services).

## Annotation Coverage Analyzer (open source)

| | |
|---|---|
| **Reads** | `Ingress` resource metadata (name, namespace, annotations) via the Kubernetes API, read-only. Uses `list` on `networking.k8s.io/v1 Ingress` only — no `get`/`watch` on Secrets, ConfigMaps, or any other resource type. |
| **Writes** | Nothing to the cluster. The only output is a report file (or stdout) the operator chooses to save locally. |
| **Credentials required** | Read-only kubeconfig with `list` on `Ingress` across the namespaces/contexts being analyzed. No write, no cluster-admin. |
| **Network egress** | Kubernetes API server(s) named in the kubeconfig. Nothing else. |
| **Data retained** | None by the tool itself. The knowledge base (`src/analyzer/pkg/knowledgebase/annotations.yaml`) is a static, versioned file shipped with the binary — it is not customer data and contains no customer-specific content. |

## Shadow & Diff Harness

| | |
|---|---|
| **Reads** | HTTP requests mirrored to it by the customer's own ingress controller / Gateway API `RequestMirror` filter (a copy the customer's infrastructure sends, not traffic the tool intercepts). Response bodies/headers/status from the incumbent and candidate backends it's configured to compare. |
| **Writes** | A local parity report (JSON + Markdown), signed with a locally generated ed25519 key. No cluster writes. |
| **Data retention** | Normalization rules should be configured to strip dynamic content (timestamps, request IDs, tokens) before any body is retained in a report — see `src/harness/pkg/diff`'s normalization rules. Raw mirrored traffic is processed in memory per-request and is not persisted beyond what's needed to compute the diff for that request. |
| **Network egress** | Only the incumbent and candidate backend URLs the operator configures. |
| **Credentials required** | None beyond network reachability to the two backend URLs; no Kubernetes API access needed to run (it operates on HTTP traffic, not cluster objects). |

## Cutover Orchestrator

| | |
|---|---|
| **Reads** | The target `HTTPRoute` object's current weights; PromQL query results from the customer's own Prometheus endpoint; HTTP health check responses from customer-specified endpoints. |
| **Writes** | `Patch` calls to a single, operator-specified `HTTPRoute`'s `backendRefs[].weight` fields only. No other resource type, no other object, is ever written. |
| **Credentials required** | `get`/`patch` on the one named `HTTPRoute` (or, more permissively, on `HTTPRoute` in the target namespace). Not cluster-admin. |
| **Network egress** | The Kubernetes API server, and the customer-configured Prometheus URL. Nothing else. |
| **Data retained** | A local, signed remediation certificate on successful completion, and a local incident record if a rollback was triggered. No traffic bodies are read or stored — only aggregate SLO query results and health-check pass/fail. |

## Signing

Both the harness's parity report and the orchestrator's remediation certificate are signed with `crypto/ed25519` (Go standard library) using a key generated and held locally by the operator — no external KMS or SaaS signing service is contacted. This keeps signing available in air-gapped environments.

## What we explicitly do not do

- No usage analytics, crash reporting, or update-check network calls
- No credentials, cluster contents, or traffic bodies are ever transmitted outside the customer's network
- No cloud dependency for any of the three tools to function
