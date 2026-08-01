// Package report builds and signs the parity report — the artifact PLAN.md
// describes as "the artifact that lets someone approve the production
// cutover", intended to be presented to a change advisory board.
package report

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dpuig/ingress-shift/src/harness/pkg/diff"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

// Record is one mirrored request's outcome: which divergences, if any, were
// found between the incumbent and candidate responses.
type Record struct {
	Timestamp   time.Time         `json:"timestamp"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Divergences []diff.Divergence `json:"divergences,omitempty"`
}

// HasDivergence reports whether this record found any difference at all.
func (r Record) HasDivergence() bool {
	return len(r.Divergences) > 0
}

// HighestSeverity returns the most severe divergence on this record, in
// order breaking > degraded > cosmetic > expected, or "" if there were none.
func (r Record) HighestSeverity() diff.Severity {
	rank := map[diff.Severity]int{
		diff.SeverityBreaking: 4,
		diff.SeverityDegraded: 3,
		diff.SeverityCosmetic: 2,
		diff.SeverityExpected: 1,
	}
	var highest diff.Severity
	best := 0
	for _, d := range r.Divergences {
		if rank[d.Severity] > best {
			best = rank[d.Severity]
			highest = d.Severity
		}
	}
	return highest
}

// Aggregator collects Records concurrently as mirrored requests are processed.
type Aggregator struct {
	mu      sync.Mutex
	records []Record
}

// NewAggregator creates an empty, ready-to-use Aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{}
}

// Add records one request's outcome. Safe for concurrent use.
func (a *Aggregator) Add(rec Record) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = append(a.records, rec)
}

// Snapshot returns a copy of every record collected so far.
func (a *Aggregator) Snapshot() []Record {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Record, len(a.records))
	copy(out, a.records)
	return out
}

// ParityReport is the soak-window summary PLAN.md calls for: parity
// percentage, divergences classified by severity, over the configured window.
type ParityReport struct {
	GeneratedAt       time.Time      `json:"generated_at"`
	SoakWindow        string         `json:"soak_window"`
	IncumbentURL      string         `json:"incumbent_url"`
	CandidateURL      string         `json:"candidate_url"`
	TotalRequests     int            `json:"total_requests"`
	DivergentRequests int            `json:"divergent_requests"`
	ParityPercent     float64        `json:"parity_percent"`
	BySeverity        map[string]int `json:"by_severity"`
	Divergences       []Divergence   `json:"divergences"`
}

// Divergence is a single flattened divergence entry in the report, with
// enough request context to investigate it.
type Divergence struct {
	Timestamp time.Time     `json:"timestamp"`
	Method    string        `json:"method"`
	Path      string        `json:"path"`
	Field     string        `json:"field"`
	Severity  diff.Severity `json:"severity"`
	Detail    string        `json:"detail"`
}

// Build summarizes records collected over soakWindow into a ParityReport.
func Build(records []Record, soakWindow time.Duration, incumbentURL, candidateURL string) *ParityReport {
	r := &ParityReport{
		GeneratedAt:   time.Now().UTC(),
		SoakWindow:    soakWindow.String(),
		IncumbentURL:  incumbentURL,
		CandidateURL:  candidateURL,
		TotalRequests: len(records),
		BySeverity:    map[string]int{},
		Divergences:   []Divergence{},
	}

	for _, rec := range records {
		if rec.HasDivergence() {
			r.DivergentRequests++
		}
		for _, d := range rec.Divergences {
			r.BySeverity[string(d.Severity)]++
			r.Divergences = append(r.Divergences, Divergence{
				Timestamp: rec.Timestamp,
				Method:    rec.Method,
				Path:      rec.Path,
				Field:     d.Field,
				Severity:  d.Severity,
				Detail:    d.Detail,
			})
		}
	}

	sort.Slice(r.Divergences, func(i, j int) bool {
		return r.Divergences[i].Timestamp.Before(r.Divergences[j].Timestamp)
	})

	if r.TotalRequests == 0 {
		r.ParityPercent = 100
	} else {
		r.ParityPercent = float64(r.TotalRequests-r.DivergentRequests) / float64(r.TotalRequests) * 100
	}

	return r
}

// ToMarkdown renders a human-readable summary suitable for presenting to a
// change advisory board, per PLAN.md's design notes for Workstream B.
func (r *ParityReport) ToMarkdown() string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Shadow & Diff Parity Report\n\n")
	fmt.Fprintf(&b, "- Generated: %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Soak window: %s\n", r.SoakWindow)
	fmt.Fprintf(&b, "- Incumbent: %s\n", r.IncumbentURL)
	fmt.Fprintf(&b, "- Candidate: %s\n\n", r.CandidateURL)

	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "- Total requests mirrored: %d\n", r.TotalRequests)
	fmt.Fprintf(&b, "- Requests with divergence: %d\n", r.DivergentRequests)
	fmt.Fprintf(&b, "- **Parity: %.2f%%**\n\n", r.ParityPercent)

	fmt.Fprintf(&b, "## Divergences by severity\n\n")
	fmt.Fprintf(&b, "| Severity | Count |\n|---|---|\n")
	for _, sev := range []diff.Severity{diff.SeverityBreaking, diff.SeverityDegraded, diff.SeverityCosmetic, diff.SeverityExpected} {
		fmt.Fprintf(&b, "| %s | %d |\n", sev, r.BySeverity[string(sev)])
	}
	fmt.Fprintf(&b, "\n")

	if len(r.Divergences) > 0 {
		fmt.Fprintf(&b, "## Divergence detail\n\n")
		fmt.Fprintf(&b, "| Time | Method | Path | Field | Severity | Detail |\n|---|---|---|---|---|---|\n")
		for _, d := range r.Divergences {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
				d.Timestamp.Format(time.RFC3339), d.Method, d.Path, d.Field, d.Severity, d.Detail)
		}
	}

	return b.String()
}

// SignedReport is a ParityReport wrapped in a signed envelope for distribution.
type SignedReport struct {
	*sign.Document
}

// Sign produces a signed envelope around the report.
func Sign(r *ParityReport, priv ed25519.PrivateKey) (*sign.Document, error) {
	return sign.SignJSON(priv, r)
}

// Verify checks a signed envelope's signature and unmarshals the enclosed report.
func Verify(doc *sign.Document) (*ParityReport, error) {
	if err := sign.Verify(doc); err != nil {
		return nil, err
	}
	var r ParityReport
	if err := json.Unmarshal(doc.Payload, &r); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parity report: %w", err)
	}
	return &r, nil
}
