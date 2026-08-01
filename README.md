# Ingress Shift

[![CI](https://github.com/dpuig/ingress-shift/actions/workflows/ci.yml/badge.svg)](https://github.com/dpuig/ingress-shift/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/dpuig/ingress-shift.svg)](https://pkg.go.dev/github.com/dpuig/ingress-shift)
[![Go Report Card](https://goreportcard.com/badge/github.com/dpuig/ingress-shift)](https://goreportcard.com/report/github.com/dpuig/ingress-shift)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/v/release/dpuig/ingress-shift?include_prereleases)](https://github.com/dpuig/ingress-shift/releases)

**Analyze, validate, and safely cut over from Ingress-NGINX to Gateway API — three free, open-source tools, one migration.**

Ingress-NGINX reached end-of-life in March 2026. `ingress2gateway` already handles the easy 80% of translation. Ingress Shift covers the hard 20%: knowing exactly what will break, proving a candidate Gateway API controller behaves identically to production traffic, and cutting over without an outage.

## Contents

- [Why](#why)
- [What's here](#whats-here)
- [Quick start](#quick-start)
  - [Analyzer](#1-annotation-coverage-analyzer)
  - [Harness](#2-shadow--diff-harness)
  - [Orchestrator](#3-cutover-orchestrator)
- [How it works](#how-it-works)
- [Data handling & security](#data-handling--security)
- [Development](#development)
- [Project structure](#project-structure)
- [Status & roadmap](#status--roadmap)
- [Contributing](#contributing)
- [License](#license)

## Why

Translating an `Ingress` object to an `HTTPRoute` is the solved part of this migration. What actually blocks teams is everything translation tools can't see: annotations with no Gateway API equivalent, whether the new controller's response bytes actually match the old one under real traffic, and how to move production over without a rollback nightmare. These three tools are built in that order — analyze, validate, cut over.

## What's here

| Tool | Binary | Purpose |
|---|---|---|
| **Annotation Coverage Analyzer** | `ingress-shift-analyzer` | Read-only `kubectl` plugin. Scores every `Ingress` annotation in your cluster(s) against a versioned knowledge base and tells you what will and won't translate automatically. |
| **Shadow & Diff Harness** | `ingress-shift-harness` | Mirrors live production traffic to both the old and new controller, diffs the responses, and produces a signed parity report. |
| **Cutover Orchestrator** | `ingress-shift-orchestrator` | Staged, weighted traffic shift via native Gateway API `HTTPRoute` weights, with automated SLO-driven rollback. |

All three are Go, compile to a single static binary, run entirely on-premises, and are licensed MIT.

## Quick start

### 1. Annotation Coverage Analyzer

```bash
# Install directly from this repo's own manifest:
kubectl krew install --manifest-url=https://raw.githubusercontent.com/dpuig/ingress-shift/main/krew.yaml

# Or build from source
go install github.com/dpuig/ingress-shift/src/analyzer@latest
```

```bash
# Analyze all namespaces across every kubeconfig context
kubectl ingress-shift-analyzer -A

# A single namespace, JSON out
kubectl ingress-shift-analyzer -n my-namespace -o json

# Restrict to specific contexts
kubectl ingress-shift-analyzer -A --context prod-us --context prod-eu
```

Sample output includes a complexity score, a percentage of annotations directly translatable, a per-item effort estimate for everything that isn't, and a named Gateway API controller recommendation with reasoning.

### 2. Shadow & Diff Harness

```bash
go install github.com/dpuig/ingress-shift/src/harness@latest

ingress-shift-harness keygen --output-prefix ./harness-key

ingress-shift-harness serve \
  --incumbent-url https://old-ingress.internal \
  --candidate-url https://new-gateway.internal \
  --soak-window 24h \
  --signing-key ./harness-key.priv
```

Point your ingress controller's or Gateway API's request-mirror feature at the harness's `--listen` address (default `:8443`). It only ever receives a copy of traffic — it never sits inline with production.

```bash
ingress-shift-harness verify --report parity-report.json --public-key ./harness-key.pub
```

### 3. Cutover Orchestrator

```bash
go install github.com/dpuig/ingress-shift/src/orchestrator@latest

ingress-shift-orchestrator run \
  --httproute checkout --namespace default \
  --incumbent-service checkout-nginx --candidate-service checkout-gateway \
  --stages 1,5,25,50,100 \
  --bake-duration 15m --confirm-duration 1h \
  --prometheus-url http://prometheus.monitoring:9090 \
  --error-rate-query 'sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))' \
  --error-rate-threshold 0.02 \
  --signing-key ./harness-key.priv
```

Any SLO or health-check breach during a bake period triggers an immediate single-patch rollback to 100% incumbent — seconds, not minutes. On success, it writes a signed remediation certificate and a decommission checklist for the old controller.

## How it works

```
                 ┌─────────────────────┐
   kubeconfig →  │  Analyzer            │ → scored report (JSON/YAML/table)
                 │  (read-only)         │
                 └─────────────────────┘

   prod traffic  ┌─────────────────────┐
   (mirrored)  → │  Harness             │ → signed parity report
                 │  incumbent ⇄ candidate│
                 └─────────────────────┘

   HTTPRoute     ┌─────────────────────┐
   weights     ↔ │  Orchestrator        │ → signed remediation certificate
                 │  + Prometheus SLOs   │    + decommission checklist
                 └─────────────────────┘
```

Each tool is independently useful and independently runnable — the harness and orchestrator don't require the analyzer, and none of them require each other's output as input. They're meant to be run in the order above during a real migration.

## Data handling & security

- No phone-home, no telemetry, no external dependency required to run any of the three tools.
- The analyzer is strictly read-only against the Kubernetes API (`list` on `Ingress` only).
- The orchestrator's only cluster write is a `patch` of `backendRefs[].weight` on one named `HTTPRoute`.
- Signing (parity reports, remediation certificates) uses `crypto/ed25519` from the Go standard library — no external KMS, works air-gapped.

Full details, written to be pasted directly into a security questionnaire: [docs/DATA_HANDLING.md](docs/DATA_HANDLING.md).

## Development

```bash
make build              # analyzer binary
make build-harness
make build-orchestrator
make build-all          # all three

make test
make lint
```

Docker (any tool, via build arg):

```bash
docker build --build-arg TOOL=analyzer -t ingress-shift-analyzer .
docker build --build-arg TOOL=harness -t ingress-shift-harness .
docker build --build-arg TOOL=orchestrator -t ingress-shift-orchestrator .
```

Requires Go 1.21+. `go test ./...` and `golangci-lint run ./...` must be clean before opening a PR.

## Project structure

```
src/
├── analyzer/        kubectl plugin: cmd/ (CLI), pkg/ (analysis + knowledgebase/)
├── harness/          cmd/ (serve, keygen, verify), pkg/mirror, pkg/diff, pkg/report
├── orchestrator/     cmd/ (run), pkg/traffic, pkg/slo, pkg/rollout, pkg/certificate
└── shared/sign/      ed25519 signing used by the harness and orchestrator
tools/krew-manifest/  regenerates krew.yaml with real checksums on each tagged release
docs/                 DATA_HANDLING.md and other reference docs
```

## Status & roadmap

Pre-release — the analyzer, harness, and orchestrator all build and pass their test suites, and the analyzer's krew packaging (manifest, LICENSE bundling, `-n`/`-A`/`--context` flag parity, cloud-auth plugin support) has been validated with a real local `kubectl krew install --manifest=... --archive=...` round-trip. What's left is entirely release mechanics, not code: tag a `v*` release to trigger the signed cross-platform build (`.github/workflows/release.yml`), then submit `krew.yaml` as a PR to [krew-index](https://github.com/kubernetes-sigs/krew-index) for `kubectl krew install ingress-shift-analyzer` to resolve without `--manifest-url`.

## Contributing

Issues and PRs are welcome on all three tools. The annotation knowledge base (`src/analyzer/pkg/knowledgebase/annotations.yaml`) is the highest-value place to contribute — every real-world annotation mapping added there benefits every future user. Each entry needs a classification (`direct`/`extension`/`none`), a Gateway API note, and an effort estimate; see the schema test in the same package for the required fields.

Before opening a PR: `make test` and `make lint` should both be clean.

## License

[MIT](LICENSE) — the entire repository, all three tools.
