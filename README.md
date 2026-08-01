# Ingress Shift Analyzer

> Ingress → Gateway API migration tooling — analyzer, validation harness, and cutover orchestrator.

**Timeline:** Weeks 1–10 (August–October 2026)

**Goal:** Three paid migrations delivered and a shipped analyzer by end of Q4 2026.

## Workstream A — Annotation Coverage Analyzer (Weeks 1–5)

Read-only `kubectl` plugin via krew, free and open source.

- Enumerate Ingress resources across all contexts and namespaces
- Map every annotation against a maintained knowledge base
- Flag classes that break naive translation (auth snippets, custom NGINX config, rewrite semantics, session affinity, rate limiting, mTLS passthrough)
- Emit a scored report: percentage translatable, list of manual interventions with effort estimate
- Recommend a target controller with stated reasoning

## Installation

### Using Krew

```bash
kubectl krew install ingress-shift-analyzer
```

### Manual Installation

Download the appropriate binary for your platform from the [releases page](https://github.com/duhostack/ingress-shift/releases) and place it in your PATH.

## Usage

```bash
# Analyze all Ingress resources in the current namespace
kubectl ingress-shift-analyzer analyze

# Analyze all Ingress resources across all namespaces
kubectl ingress-shift-analyzer analyze -A

# Analyze Ingress resources in a specific namespace
kubectl ingress-shift-analyzer analyze -n <namespace-name>

# Output in JSON format
kubectl ingress-shift-analyzer analyze -o json

# Output in YAML format
kubectl ingress-shift-analyzer analyze -o yaml

# Enable verbose output
kubectl ingress-shift-analyzer analyze -v
```

## Features

- **Comprehensive Analysis**: Enumerates all Ingress resources and their annotations
- **Knowledge Base Mapping**: Maps NGINX Ingress annotations to Gateway API equivalents
- **Complexity Scoring**: Calculates migration complexity score based on annotation usage
- **Detailed Reporting**: Provides detailed analysis with recommendations
- **Multiple Output Formats**: Table, JSON, and YAML output formats

## Annotation Coverage

The analyzer currently supports the following annotation classes:

| Annotation | Description | Translatable |
|------------|-------------|--------------|
| nginx.ingress.kubernetes.io/auth-secret | Basic auth secret reference | Yes |
| nginx.ingress.kubernetes.io/auth-realm | Basic auth realm | Yes |
| nginx.ingress.kubernetes.io/rewrite-target | Rewrite target for URL rewriting | Yes |
| nginx.ingress.kubernetes.io/ssl-redirect | SSL redirect configuration | Yes |
| nginx.ingress.kubernetes.io/proxy-body-size | Proxy body size limit | Yes |
| nginx.ingress.kubernetes.io/limit-rps | Rate limiting per second | No (requires extension) |
| nginx.ingress.kubernetes.io/configuration-snippet | Custom NGINX configuration snippet | No (requires extension) |
| nginx.ingress.kubernetes.io/session-affinity | Session affinity configuration | No (requires extension) |
| nginx.ingress.kubernetes.io/ssl-passthrough | SSL passthrough configuration | No (requires extension) |

## Output Format

The analyzer provides a comprehensive report with:

1. **Summary Statistics**: Total resources, translatable, manual intervention needed
2. **Complexity Score**: Percentage score indicating migration complexity
3. **Recommendations**: Actionable advice based on analysis
4. **Annotation Classes**: Detailed breakdown of annotation usage

## Development

### Building

```bash
make build
```

### Building for Specific Platforms

```bash
make build-linux
make build-macos
make build-windows
```

### Testing

```bash
make test
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request on GitHub.

## License

MIT License - see the LICENSE file for details.