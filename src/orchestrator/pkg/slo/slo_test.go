package slo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newPrometheusStub(t *testing.T, value string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"value":[1700000000,%q]}]}}`, value)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPromQLCheckBreachesOnGreaterThan(t *testing.T) {
	srv := newPrometheusStub(t, "0.08")

	check := &PromQLCheck{
		CheckName:     "error-rate",
		PrometheusURL: srv.URL,
		Query:         "rate(errors[5m])",
		Threshold:     0.05,
		Comparison:    ComparisonGreaterThan,
	}

	breached, reason, err := check.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !breached {
		t.Errorf("expected breach (0.08 > 0.05), reason: %s", reason)
	}
}

func TestPromQLCheckDoesNotBreachWhenUnderThreshold(t *testing.T) {
	srv := newPrometheusStub(t, "0.01")

	check := &PromQLCheck{
		CheckName:     "error-rate",
		PrometheusURL: srv.URL,
		Query:         "rate(errors[5m])",
		Threshold:     0.05,
		Comparison:    ComparisonGreaterThan,
	}

	breached, _, err := check.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if breached {
		t.Error("expected no breach (0.01 < 0.05)")
	}
}

func TestPromQLCheckLessThanComparison(t *testing.T) {
	srv := newPrometheusStub(t, "0.90")

	check := &PromQLCheck{
		CheckName:     "success-rate",
		PrometheusURL: srv.URL,
		Query:         "success_ratio",
		Threshold:     0.95,
		Comparison:    ComparisonLessThan,
	}

	breached, _, err := check.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !breached {
		t.Error("expected breach (0.90 < 0.95)")
	}
}

func TestPromQLCheckEmptyResultDoesNotBreach(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	t.Cleanup(srv.Close)

	check := &PromQLCheck{
		CheckName:     "no-data",
		PrometheusURL: srv.URL,
		Query:         "nonexistent_metric",
		Threshold:     0.05,
		Comparison:    ComparisonGreaterThan,
	}

	breached, _, err := check.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if breached {
		t.Error("expected no breach when Prometheus returns no data")
	}
}

func TestHTTPHealthCheckBreachesOnUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	check := &HTTPHealthCheck{CheckName: "backend-health", URL: srv.URL, ExpectedStatus: http.StatusOK}

	breached, reason, err := check.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !breached {
		t.Errorf("expected breach on 503, reason: %s", reason)
	}
}

func TestHTTPHealthCheckPassesOnExpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	check := &HTTPHealthCheck{CheckName: "backend-health", URL: srv.URL, ExpectedStatus: http.StatusOK}

	breached, _, err := check.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if breached {
		t.Error("expected no breach on 200")
	}
}

func TestHTTPHealthCheckBreachesOnUnreachable(t *testing.T) {
	check := &HTTPHealthCheck{CheckName: "backend-health", URL: "http://127.0.0.1:1", ExpectedStatus: http.StatusOK}

	breached, _, err := check.Check(context.Background())
	if err != nil {
		t.Fatalf("Check should not error on connection failure, got: %v", err)
	}
	if !breached {
		t.Error("expected breach when the health check endpoint is unreachable")
	}
}

func TestCheckAllReturnsFirstBreachButRunsEveryChecker(t *testing.T) {
	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthySrv.Close)

	unhealthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(unhealthySrv.Close)

	checkers := []Checker{
		&HTTPHealthCheck{CheckName: "ok-check", URL: healthySrv.URL, ExpectedStatus: http.StatusOK},
		&HTTPHealthCheck{CheckName: "bad-check", URL: unhealthySrv.URL, ExpectedStatus: http.StatusOK},
	}

	breached, reason, err := CheckAll(context.Background(), checkers)
	if err != nil {
		t.Fatalf("CheckAll failed: %v", err)
	}
	if !breached {
		t.Fatal("expected CheckAll to report a breach")
	}
	if reason == "" {
		t.Error("expected a non-empty breach reason")
	}
}

func TestCheckAllNoBreachWhenAllHealthy(t *testing.T) {
	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthySrv.Close)

	checkers := []Checker{
		&HTTPHealthCheck{CheckName: "ok-check", URL: healthySrv.URL, ExpectedStatus: http.StatusOK},
	}

	breached, _, err := CheckAll(context.Background(), checkers)
	if err != nil {
		t.Fatalf("CheckAll failed: %v", err)
	}
	if breached {
		t.Error("expected no breach when all checkers are healthy")
	}
}
