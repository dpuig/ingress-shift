package report

import (
	"strings"
	"testing"
	"time"

	"github.com/dpuig/ingress-shift/src/harness/pkg/diff"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

func TestAggregatorIsConcurrencySafe(t *testing.T) {
	agg := NewAggregator()
	done := make(chan struct{})

	for i := 0; i < 50; i++ {
		go func(i int) {
			agg.Add(Record{Method: "GET", Path: "/x", Timestamp: time.Now()})
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}

	if len(agg.Snapshot()) != 50 {
		t.Errorf("expected 50 records, got %d", len(agg.Snapshot()))
	}
}

func TestBuildComputesParityPercent(t *testing.T) {
	records := []Record{
		{Method: "GET", Path: "/a", Timestamp: time.Now()},
		{Method: "GET", Path: "/b", Timestamp: time.Now(), Divergences: []diff.Divergence{
			{Field: "status_code", Severity: diff.SeverityBreaking, Detail: "500 vs 200"},
		}},
		{Method: "GET", Path: "/c", Timestamp: time.Now()},
		{Method: "GET", Path: "/d", Timestamp: time.Now()},
	}

	r := Build(records, time.Hour, "https://incumbent", "https://candidate")

	if r.TotalRequests != 4 {
		t.Errorf("expected 4 total requests, got %d", r.TotalRequests)
	}
	if r.DivergentRequests != 1 {
		t.Errorf("expected 1 divergent request, got %d", r.DivergentRequests)
	}
	if r.ParityPercent != 75.0 {
		t.Errorf("expected 75%% parity, got %.2f", r.ParityPercent)
	}
	if r.BySeverity[string(diff.SeverityBreaking)] != 1 {
		t.Errorf("expected 1 breaking divergence, got %d", r.BySeverity[string(diff.SeverityBreaking)])
	}
}

func TestBuildWithNoRequestsIsFullParity(t *testing.T) {
	r := Build(nil, time.Hour, "https://incumbent", "https://candidate")
	if r.ParityPercent != 100 {
		t.Errorf("expected 100%% parity for zero requests, got %.2f", r.ParityPercent)
	}
}

func TestToMarkdownContainsKeyFields(t *testing.T) {
	records := []Record{
		{Method: "GET", Path: "/a", Timestamp: time.Now(), Divergences: []diff.Divergence{
			{Field: "body", Severity: diff.SeverityBreaking, Detail: "differs"},
		}},
	}
	r := Build(records, 24*time.Hour, "https://incumbent", "https://candidate")

	md := r.ToMarkdown()
	for _, want := range []string{"Parity Report", "Parity: 0.00%", "breaking", "https://incumbent", "https://candidate"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected markdown to contain %q, got:\n%s", want, md)
		}
	}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	kp, err := sign.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	r := Build(nil, time.Hour, "https://incumbent", "https://candidate")

	doc, err := Sign(r, kp.PrivateKey)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	verified, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if verified.ParityPercent != r.ParityPercent {
		t.Errorf("expected round-tripped report to match, got %.2f want %.2f", verified.ParityPercent, r.ParityPercent)
	}
}

func TestVerifyFailsOnTamperedReport(t *testing.T) {
	kp, err := sign.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	r := Build([]Record{
		{Method: "GET", Path: "/a", Timestamp: time.Now(), Divergences: []diff.Divergence{
			{Field: "status_code", Severity: diff.SeverityBreaking, Detail: "500 vs 200"},
		}},
	}, time.Hour, "https://incumbent", "https://candidate")

	doc, err := Sign(r, kp.PrivateKey)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Tamper: try to hide the breaking divergence by rewriting the payload.
	doc.Payload = []byte(`{"parity_percent":100,"total_requests":1,"divergent_requests":0}`)

	if _, err := Verify(doc); err == nil {
		t.Error("expected verification to fail on tampered report, got nil error")
	}
}

func TestHighestSeverityPrefersBreaking(t *testing.T) {
	rec := Record{
		Divergences: []diff.Divergence{
			{Field: "header:X-Cache", Severity: diff.SeverityCosmetic},
			{Field: "status_code", Severity: diff.SeverityBreaking},
			{Field: "header:X-Foo", Severity: diff.SeverityDegraded},
		},
	}
	if got := rec.HighestSeverity(); got != diff.SeverityBreaking {
		t.Errorf("expected breaking, got %s", got)
	}
}
