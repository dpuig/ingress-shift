package diff

import "testing"

func TestCompareIdenticalResponsesYieldsNoDivergence(t *testing.T) {
	resp := Response{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		Body:       []byte(`{"ok":true}`),
	}

	got := Compare(resp, resp, DefaultConfig())
	if len(got) != 0 {
		t.Errorf("expected no divergences for identical responses, got %+v", got)
	}
}

func TestCompareStatusCodeSameClassIsDegraded(t *testing.T) {
	incumbent := Response{StatusCode: 200, Headers: map[string][]string{}, Body: []byte("ok")}
	candidate := Response{StatusCode: 204, Headers: map[string][]string{}, Body: []byte("ok")}

	got := Compare(incumbent, candidate, DefaultConfig())
	div := findDivergence(t, got, "status_code")
	if div.Severity != SeverityDegraded {
		t.Errorf("expected degraded, got %s", div.Severity)
	}
}

func TestCompareStatusCodeDifferentClassIsBreaking(t *testing.T) {
	incumbent := Response{StatusCode: 200, Headers: map[string][]string{}, Body: []byte("ok")}
	candidate := Response{StatusCode: 500, Headers: map[string][]string{}, Body: []byte("error")}

	got := Compare(incumbent, candidate, DefaultConfig())
	div := findDivergence(t, got, "status_code")
	if div.Severity != SeverityBreaking {
		t.Errorf("expected breaking, got %s", div.Severity)
	}
}

func TestCompareIgnoredHeaderProducesNoDivergence(t *testing.T) {
	incumbent := Response{StatusCode: 200, Headers: map[string][]string{"Date": {"Mon, 01 Jan 2026 00:00:00 GMT"}}, Body: []byte("ok")}
	candidate := Response{StatusCode: 200, Headers: map[string][]string{"Date": {"Tue, 02 Jan 2026 00:00:00 GMT"}}, Body: []byte("ok")}

	got := Compare(incumbent, candidate, DefaultConfig())
	if len(got) != 0 {
		t.Errorf("expected Date header mismatch to be ignored, got %+v", got)
	}
}

func TestCompareSignificantHeaderMismatchIsBreaking(t *testing.T) {
	incumbent := Response{StatusCode: 200, Headers: map[string][]string{"Content-Type": {"application/json"}}, Body: []byte("ok")}
	candidate := Response{StatusCode: 200, Headers: map[string][]string{"Content-Type": {"text/plain"}}, Body: []byte("ok")}

	got := Compare(incumbent, candidate, DefaultConfig())
	div := findDivergence(t, got, "header:Content-Type")
	if div.Severity != SeverityBreaking {
		t.Errorf("expected breaking, got %s", div.Severity)
	}
}

func TestCompareUnlistedHeaderMismatchIsCosmetic(t *testing.T) {
	incumbent := Response{StatusCode: 200, Headers: map[string][]string{"X-Cache": {"HIT"}}, Body: []byte("ok")}
	candidate := Response{StatusCode: 200, Headers: map[string][]string{"X-Cache": {"MISS"}}, Body: []byte("ok")}

	got := Compare(incumbent, candidate, DefaultConfig())
	div := findDivergence(t, got, "header:X-Cache")
	if div.Severity != SeverityCosmetic {
		t.Errorf("expected cosmetic, got %s", div.Severity)
	}
}

func TestCompareExpectedDiffHeaderIsExpected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExpectedDiffHeaders = []string{"X-Backend-Version"}

	incumbent := Response{StatusCode: 200, Headers: map[string][]string{"X-Backend-Version": {"nginx-1.2"}}, Body: []byte("ok")}
	candidate := Response{StatusCode: 200, Headers: map[string][]string{"X-Backend-Version": {"envoy-gw-2.0"}}, Body: []byte("ok")}

	got := Compare(incumbent, candidate, cfg)
	div := findDivergence(t, got, "header:X-Backend-Version")
	if div.Severity != SeverityExpected {
		t.Errorf("expected 'expected' severity, got %s", div.Severity)
	}
}

func TestCompareBodyDivergenceOnSuccessIsBreaking(t *testing.T) {
	incumbent := Response{StatusCode: 200, Headers: map[string][]string{}, Body: []byte(`{"result":"a"}`)}
	candidate := Response{StatusCode: 200, Headers: map[string][]string{}, Body: []byte(`{"result":"b"}`)}

	got := Compare(incumbent, candidate, DefaultConfig())
	div := findDivergence(t, got, "body")
	if div.Severity != SeverityBreaking {
		t.Errorf("expected breaking, got %s", div.Severity)
	}
}

func TestCompareBodyDivergenceOnErrorIsDegraded(t *testing.T) {
	incumbent := Response{StatusCode: 500, Headers: map[string][]string{}, Body: []byte(`{"error":"upstream timeout"}`)}
	candidate := Response{StatusCode: 500, Headers: map[string][]string{}, Body: []byte(`{"error":"backend unavailable"}`)}

	got := Compare(incumbent, candidate, DefaultConfig())
	div := findDivergence(t, got, "body")
	if div.Severity != SeverityDegraded {
		t.Errorf("expected degraded, got %s", div.Severity)
	}
}

func TestCompareBodyNormalizesTimestampsAndRequestIDs(t *testing.T) {
	incumbent := Response{
		StatusCode: 200,
		Headers:    map[string][]string{},
		Body:       []byte(`{"created_at":"2026-01-01T00:00:00Z","request_id":"abc-123","data":"same"}`),
	}
	candidate := Response{
		StatusCode: 200,
		Headers:    map[string][]string{},
		Body:       []byte(`{"created_at":"2026-06-15T12:30:00Z","request_id":"xyz-999","data":"same"}`),
	}

	got := Compare(incumbent, candidate, DefaultConfig())
	if len(got) != 0 {
		t.Errorf("expected timestamp/request-id differences to normalize away, got %+v", got)
	}
}

func TestNormalizeCSRFToken(t *testing.T) {
	body := []byte(`{"_csrf":"abcdef123456","data":"x"}`)
	got := Normalize(body, DefaultNormalizeRules())
	want := `{"_csrf":"<token>","data":"x"}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func findDivergence(t *testing.T, divergences []Divergence, field string) Divergence {
	t.Helper()
	for _, d := range divergences {
		if d.Field == field {
			return d
		}
	}
	t.Fatalf("expected a divergence for field %q, got %+v", field, divergences)
	return Divergence{}
}
