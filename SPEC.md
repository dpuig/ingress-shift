# Spec: Ingress Shift

## Objective
Ingress Shift is a suite of tools designed to help organizations migrate from Ingress-NGINX to Gateway API. The project consists of three interconnected workstreams:
1. Workstream A - Annotation Coverage Analyzer (Weeks 1–5) - A free, open-source `kubectl` plugin via krew that analyzes Ingress resources and annotations to determine migration complexity.
2. Workstream B - Shadow & Diff Harness (Weeks 4–8) - A commercial tool that validates the behavior of Gateway API controllers against production traffic.
3. Workstream C - Cutover Orchestrator (Weeks 8–10) - A commercial tool for safely migrating traffic with automated rollback.

The primary goal is to deliver three paid migrations and ship a working analyzer by end of Q4 2026, with the analyzer serving as a lead magnet to generate demand for the commercial tools.

## Tech Stack
- Language: Go (single static binary, no runtime dependencies)
- Distribution: `kubectl` plugin via krew (analyzer); standalone binary (harness + orchestrator)
- Execution model: On-premises, no phone-home, no telemetry, air-gap capable
- Licensing: Analyzer is open source (lead generation). Shadow harness and cutover orchestrator are commercial (revenue).

## Commands
Build: go build -o ingress-shift
Test: go test ./...
Lint: golangci-lint run
Dev: go run main.go

## Project Structure
```
src/
├── analyzer/           → Annotation coverage analyzer implementation
│   ├── cmd/            → CLI commands
│   ├── pkg/            → Core logic for annotation analysis
│   └── krew/           → Krew plugin packaging
├── harness/            → Shadow & diff harness implementation
│   └── pkg/
└── orchestrator/       → Cutover orchestrator implementation
    └── pkg/
tests/                  → Unit and integration tests
e2e/                    → End-to-end tests
docs/                   → Documentation
```

## Code Style
- Go 1.21+ with idiomatic Go conventions
- Clear, descriptive variable and function names
- Comprehensive error handling with meaningful error messages
- Well-documented public APIs using godoc
- Consistent structuring of packages and modules

Example:
```go
// AnalyzeIngressResources enumerates Ingress resources across all contexts and namespaces
func AnalyzeIngressResources(ctx context.Context, client kubernetes.Interface) (*AnalysisReport, error) {
    // Implementation here
}
```

## Testing Strategy
- Unit tests for core logic using Go's built-in testing framework
- Integration tests for Kubernetes API interactions
- End-to-end tests for full workflow execution
- Test coverage targets: 80% for core logic, 90% for critical paths
- Mocking of external dependencies (Kubernetes API, file I/O)

## Boundaries
- Always: Run tests before commits, follow Go naming conventions, validate inputs, ensure security-first design
- Ask first: Database schema changes, adding dependencies, changing CI config, modifying licensing model
- Never: Commit secrets, edit vendor directories, remove failing tests without approval, ship untested code

## Success Criteria
- Analyzer runs against a real cluster in < 60 seconds
- Analyzer installable via `kubectl krew install`
- Analyzer handles multi-context, multi-namespace clusters
- Analyzer output formats: human-readable terminal, JSON, and machine-parseable report
- All three paid migrations delivered by end of Q4 2026
- First case study published

## Open Questions
1. What specific annotations from Ingress-NGINX need to be covered in the knowledge base?
2. What are the exact SLOs that will trigger automated rollback in the orchestrator?
3. How should the signed parity report be formatted for compliance purposes?
4. What is the expected scale of clusters this tool needs to handle (number of namespaces, resources)?