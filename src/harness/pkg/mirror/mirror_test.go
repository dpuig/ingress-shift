package mirror

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dpuig/ingress-shift/src/harness/pkg/diff"
	"github.com/dpuig/ingress-shift/src/harness/pkg/report"
)

func newTestServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDispatchReturnsBothResponses(t *testing.T) {
	incumbentSrv := newTestServer(t, 200, `{"result":"incumbent"}`)
	candidateSrv := newTestServer(t, 200, `{"result":"candidate"}`)

	d := NewDispatcher(
		Target{Name: "incumbent", BaseURL: incumbentSrv.URL, Timeout: time.Second},
		Target{Name: "candidate", BaseURL: candidateSrv.URL, Timeout: time.Second},
	)

	incumbentResp, candidateResp, err := d.Dispatch(context.Background(), http.MethodGet, "/foo", http.Header{}, nil)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if string(incumbentResp.Body) != `{"result":"incumbent"}` {
		t.Errorf("unexpected incumbent body: %s", incumbentResp.Body)
	}
	if string(candidateResp.Body) != `{"result":"candidate"}` {
		t.Errorf("unexpected candidate body: %s", candidateResp.Body)
	}
}

func TestDispatchFailsIfIncumbentUnreachable(t *testing.T) {
	candidateSrv := newTestServer(t, 200, `ok`)

	d := NewDispatcher(
		Target{Name: "incumbent", BaseURL: "http://127.0.0.1:1", Timeout: 200 * time.Millisecond},
		Target{Name: "candidate", BaseURL: candidateSrv.URL, Timeout: time.Second},
	)

	_, _, err := d.Dispatch(context.Background(), http.MethodGet, "/foo", http.Header{}, nil)
	if err == nil {
		t.Fatal("expected an error when the incumbent is unreachable")
	}
}

func TestListenerRecordsDivergence(t *testing.T) {
	incumbentSrv := newTestServer(t, 200, `{"result":"a"}`)
	candidateSrv := newTestServer(t, 500, `{"error":"boom"}`)

	agg := report.NewAggregator()
	listener := &Listener{
		Dispatcher: NewDispatcher(
			Target{Name: "incumbent", BaseURL: incumbentSrv.URL, Timeout: time.Second},
			Target{Name: "candidate", BaseURL: candidateSrv.URL, Timeout: time.Second},
		),
		DiffConfig: diff.DefaultConfig(),
		Aggregator: agg,
	}

	req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	rec := httptest.NewRecorder()
	listener.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected listener to always ack 200, got %d", rec.Code)
	}

	records := agg.Snapshot()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if !records[0].HasDivergence() {
		t.Error("expected the recorded request to have a divergence")
	}
	if records[0].HighestSeverity() != diff.SeverityBreaking {
		t.Errorf("expected breaking severity, got %s", records[0].HighestSeverity())
	}
}

func TestListenerAcksEvenWhenBackendUnreachable(t *testing.T) {
	candidateSrv := newTestServer(t, 200, `ok`)

	agg := report.NewAggregator()
	listener := &Listener{
		Dispatcher: NewDispatcher(
			Target{Name: "incumbent", BaseURL: "http://127.0.0.1:1", Timeout: 200 * time.Millisecond},
			Target{Name: "candidate", BaseURL: candidateSrv.URL, Timeout: time.Second},
		),
		DiffConfig: diff.DefaultConfig(),
		Aggregator: agg,
	}

	req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	rec := httptest.NewRecorder()
	listener.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected listener to ack 200 even on dispatch failure, got %d", rec.Code)
	}
	if len(agg.Snapshot()) != 0 {
		t.Error("expected no record when dispatch fails outright")
	}
}
