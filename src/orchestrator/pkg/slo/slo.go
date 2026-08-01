// Package slo evaluates the conditions that trigger automated rollback
// during a cutover: PromQL-based error-rate/latency SLOs against a customer
// Prometheus endpoint, and custom HTTP health checks. Thresholds and
// queries are always customer-supplied config, never hardcoded defaults,
// per SPEC.md — every environment's acceptable error rate and latency
// budget is different.
package slo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Checker evaluates one SLO or health condition. A true return means the
// condition has BREACHED (rollback should trigger), not that it passed.
type Checker interface {
	Name() string
	Check(ctx context.Context) (breached bool, reason string, err error)
}

// Comparison is how a PromQL result is compared against a threshold.
type Comparison string

const (
	ComparisonGreaterThan Comparison = "gt"
	ComparisonLessThan    Comparison = "lt"
)

// PromQLCheck evaluates a PromQL query against a Prometheus HTTP API and
// breaches when the resulting scalar crosses a threshold.
type PromQLCheck struct {
	CheckName     string
	PrometheusURL string
	Query         string
	Threshold     float64
	Comparison    Comparison
	HTTPClient    *http.Client
}

func (c *PromQLCheck) Name() string { return c.CheckName }

// prometheusQueryResponse mirrors the subset of Prometheus's instant-query
// API response shape this checker needs.
type prometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (c *PromQLCheck) Check(ctx context.Context) (bool, string, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	queryURL := fmt.Sprintf("%s/api/v1/query?query=%s", c.PrometheusURL, url.QueryEscape(c.Query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return false, "", fmt.Errorf("failed to build Prometheus query request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", fmt.Errorf("failed to read Prometheus response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("prometheus query returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed prometheusQueryResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, "", fmt.Errorf("failed to parse Prometheus response: %w", err)
	}

	if parsed.Status != "success" {
		return false, "", fmt.Errorf("prometheus query was not successful: status=%s", parsed.Status)
	}

	if len(parsed.Data.Result) == 0 {
		// No time series matched — nothing to compare, treat as not breached
		// rather than erroring out the whole rollout over an empty metric.
		return false, fmt.Sprintf("%s: no data returned for query", c.CheckName), nil
	}

	rawValue, ok := parsed.Data.Result[0].Value[1].(string)
	if !ok {
		return false, "", fmt.Errorf("unexpected Prometheus value type for query %q", c.Query)
	}

	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return false, "", fmt.Errorf("failed to parse Prometheus value %q: %w", rawValue, err)
	}

	var breached bool
	switch c.Comparison {
	case ComparisonGreaterThan:
		breached = value > c.Threshold
	case ComparisonLessThan:
		breached = value < c.Threshold
	default:
		return false, "", fmt.Errorf("unknown comparison %q", c.Comparison)
	}

	reason := fmt.Sprintf("%s: value=%.4f threshold=%.4f (%s)", c.CheckName, value, c.Threshold, c.Comparison)
	return breached, reason, nil
}

// HTTPHealthCheck polls an HTTP endpoint and breaches if the response
// status doesn't match ExpectedStatus (or the request fails outright).
type HTTPHealthCheck struct {
	CheckName      string
	URL            string
	ExpectedStatus int
	HTTPClient     *http.Client
}

func (c *HTTPHealthCheck) Name() string { return c.CheckName }

func (c *HTTPHealthCheck) Check(ctx context.Context) (bool, string, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	expected := c.ExpectedStatus
	if expected == 0 {
		expected = http.StatusOK
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return false, "", fmt.Errorf("failed to build health check request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return true, fmt.Sprintf("%s: request failed: %v", c.CheckName, err), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != expected {
		return true, fmt.Sprintf("%s: expected status %d, got %d", c.CheckName, expected, resp.StatusCode), nil
	}

	return false, fmt.Sprintf("%s: healthy (status %d)", c.CheckName, resp.StatusCode), nil
}

// CheckAll runs every checker and returns the first breach found, if any.
// It always runs every checker (rather than stopping at the first breach)
// so a single Check call's log/reason set is complete for the caller to record.
func CheckAll(ctx context.Context, checkers []Checker) (breached bool, reason string, err error) {
	for _, c := range checkers {
		b, r, checkErr := c.Check(ctx)
		if checkErr != nil {
			return false, "", fmt.Errorf("checker %q failed: %w", c.Name(), checkErr)
		}
		if b && !breached {
			breached = true
			reason = r
		}
	}
	return breached, reason, nil
}
