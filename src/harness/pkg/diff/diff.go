// Package diff compares HTTP responses from the incumbent ingress controller
// and the candidate Gateway API controller, classifying any divergence by
// severity as required by PLAN.md Workstream B: breaking, degraded,
// cosmetic, or expected.
package diff

import (
	"bytes"
	"fmt"
	"regexp"
)

// Severity is how much a divergence matters for the cutover decision.
type Severity string

const (
	SeverityBreaking Severity = "breaking"
	SeverityDegraded Severity = "degraded"
	SeverityCosmetic Severity = "cosmetic"
	SeverityExpected Severity = "expected"
)

// Response is the normalized shape of an HTTP response captured from either backend.
type Response struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

// Divergence is a single difference found between the incumbent and candidate responses.
type Divergence struct {
	Field    string   `json:"field"`
	Severity Severity `json:"severity"`
	Detail   string   `json:"detail"`
}

// NormalizeRule blanks out dynamic content (timestamps, request IDs, CSRF
// tokens) from response bodies before comparison, so it doesn't register as
// a false divergence.
type NormalizeRule struct {
	Name    string
	Pattern *regexp.Regexp
	Replace string
}

// DefaultNormalizeRules covers the dynamic-content classes PLAN.md calls out
// explicitly: timestamps, request IDs, CSRF tokens.
func DefaultNormalizeRules() []NormalizeRule {
	return []NormalizeRule{
		{Name: "iso8601-timestamp", Pattern: regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?`), Replace: "<timestamp>"},
		{Name: "unix-timestamp-field", Pattern: regexp.MustCompile(`"(timestamp|created_at|updated_at)"\s*:\s*\d+`), Replace: `"$1":"<timestamp>"`},
		{Name: "request-id", Pattern: regexp.MustCompile(`(?i)"(request[_-]?id|x-request-id|trace[_-]?id)"\s*:\s*"[^"]*"`), Replace: `"$1":"<id>"`},
		{Name: "csrf-token", Pattern: regexp.MustCompile(`(?i)"(csrf[_-]?token|_csrf)"\s*:\s*"[^"]*"`), Replace: `"$1":"<token>"`},
	}
}

// Normalize applies every rule to body in order.
func Normalize(body []byte, rules []NormalizeRule) []byte {
	out := body
	for _, rule := range rules {
		out = rule.Pattern.ReplaceAll(out, []byte(rule.Replace))
	}
	return out
}

// Config controls how Compare classifies header-level divergences.
// Headers not listed anywhere default to "cosmetic" if they mismatch.
type Config struct {
	// IgnoredHeaders never produce a divergence at all — values expected to
	// vary between any two backends (Date, Server, request IDs, ...).
	IgnoredHeaders []string
	// SignificantHeaders produce a "breaking" divergence on mismatch —
	// headers the client's behavior depends on (Content-Type, Location, ...).
	SignificantHeaders []string
	// ExpectedDiffHeaders produce an "expected" divergence on mismatch —
	// headers the operator has explicitly allowlisted as known-different.
	ExpectedDiffHeaders []string
	NormalizeRules      []NormalizeRule
}

// DefaultConfig returns sensible defaults: common infra-noise headers
// ignored, Content-Type and Location treated as significant, and the
// standard dynamic-content normalization rules applied to bodies.
func DefaultConfig() Config {
	return Config{
		IgnoredHeaders:      []string{"Date", "Server", "X-Request-Id", "Age", "Via", "X-Envoy-Upstream-Service-Time"},
		SignificantHeaders:  []string{"Content-Type", "Location"},
		ExpectedDiffHeaders: nil,
		NormalizeRules:      DefaultNormalizeRules(),
	}
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if equalFoldHeader(item, target) {
			return true
		}
	}
	return false
}

func equalFoldHeader(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// Compare diffs the incumbent and candidate responses under cfg, returning
// every divergence found. An empty slice means full parity for this request.
func Compare(incumbent, candidate Response, cfg Config) []Divergence {
	var divergences []Divergence

	if d := compareStatusCode(incumbent.StatusCode, candidate.StatusCode); d != nil {
		divergences = append(divergences, *d)
	}

	divergences = append(divergences, compareHeaders(incumbent.Headers, candidate.Headers, cfg)...)

	if d := compareBody(incumbent, candidate, cfg); d != nil {
		divergences = append(divergences, *d)
	}

	return divergences
}

func statusClass(code int) int {
	return code / 100
}

func compareStatusCode(incumbent, candidate int) *Divergence {
	if incumbent == candidate {
		return nil
	}

	detail := fmt.Sprintf("incumbent=%d candidate=%d", incumbent, candidate)

	if statusClass(incumbent) != statusClass(candidate) {
		return &Divergence{Field: "status_code", Severity: SeverityBreaking, Detail: detail}
	}
	return &Divergence{Field: "status_code", Severity: SeverityDegraded, Detail: detail}
}

func compareHeaders(incumbent, candidate map[string][]string, cfg Config) []Divergence {
	var divergences []Divergence

	seen := make(map[string]bool)
	for name := range incumbent {
		seen[name] = true
	}
	for name := range candidate {
		seen[name] = true
	}

	for name := range seen {
		if contains(cfg.IgnoredHeaders, name) {
			continue
		}

		incumbentVal := firstOrEmpty(incumbent[name])
		candidateVal := firstOrEmpty(candidate[name])
		if incumbentVal == candidateVal {
			continue
		}

		detail := fmt.Sprintf("%s: incumbent=%q candidate=%q", name, incumbentVal, candidateVal)

		switch {
		case contains(cfg.ExpectedDiffHeaders, name):
			divergences = append(divergences, Divergence{Field: "header:" + name, Severity: SeverityExpected, Detail: detail})
		case contains(cfg.SignificantHeaders, name):
			divergences = append(divergences, Divergence{Field: "header:" + name, Severity: SeverityBreaking, Detail: detail})
		default:
			divergences = append(divergences, Divergence{Field: "header:" + name, Severity: SeverityCosmetic, Detail: detail})
		}
	}

	return divergences
}

func firstOrEmpty(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func compareBody(incumbent, candidate Response, cfg Config) *Divergence {
	normIncumbent := Normalize(incumbent.Body, cfg.NormalizeRules)
	normCandidate := Normalize(candidate.Body, cfg.NormalizeRules)

	if bytes.Equal(normIncumbent, normCandidate) {
		return nil
	}

	detail := fmt.Sprintf("normalized body differs (%d vs %d bytes)", len(normIncumbent), len(normCandidate))

	// A success/redirect body diverging is a contract break; an error body
	// diverging is usually just differently-worded error text.
	class := statusClass(incumbent.StatusCode)
	if class == 2 || class == 3 {
		return &Divergence{Field: "body", Severity: SeverityBreaking, Detail: detail}
	}
	return &Divergence{Field: "body", Severity: SeverityDegraded, Detail: detail}
}
