// Package mirror receives duplicated production traffic (fed by the
// customer's own ingress controller or Gateway API RequestMirror filter),
// replays it against the incumbent and candidate backends concurrently, and
// hands the pair of responses to the diff engine. This keeps the harness
// non-intrusive to production: it never sits inline, it only ever receives
// a copy, per PLAN.md's design notes.
package mirror

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dpuig/ingress-shift/src/harness/pkg/diff"
	"github.com/dpuig/ingress-shift/src/harness/pkg/report"
)

// Target is one backend the harness dispatches mirrored requests to.
type Target struct {
	Name    string
	BaseURL string
	Timeout time.Duration
}

// Dispatcher sends a single incoming request to both the incumbent and
// candidate targets concurrently and captures both responses.
type Dispatcher struct {
	Incumbent Target
	Candidate Target
	client    *http.Client
}

// NewDispatcher builds a Dispatcher. Each target's own Timeout bounds its
// individual request; the shared client has no default timeout so slow
// targets don't get killed by an unrelated target's deadline.
func NewDispatcher(incumbent, candidate Target) *Dispatcher {
	return &Dispatcher{
		Incumbent: incumbent,
		Candidate: candidate,
		client:    &http.Client{},
	}
}

// dispatchResult carries one target's outcome back over a channel.
type dispatchResult struct {
	resp diff.Response
	err  error
}

// Dispatch replays method/path/headers/body against both targets
// concurrently and returns both responses. If either dispatch fails
// (network error, timeout), it returns an error naming which target failed
// rather than silently producing a partial diff.
func (d *Dispatcher) Dispatch(ctx context.Context, method, path string, headers http.Header, body []byte) (incumbent, candidate diff.Response, err error) {
	incumbentCh := make(chan dispatchResult, 1)
	candidateCh := make(chan dispatchResult, 1)

	go func() { incumbentCh <- d.send(ctx, d.Incumbent, method, path, headers, body) }()
	go func() { candidateCh <- d.send(ctx, d.Candidate, method, path, headers, body) }()

	incumbentResult := <-incumbentCh
	candidateResult := <-candidateCh

	if incumbentResult.err != nil {
		return diff.Response{}, diff.Response{}, fmt.Errorf("incumbent %s dispatch failed: %w", d.Incumbent.Name, incumbentResult.err)
	}
	if candidateResult.err != nil {
		return diff.Response{}, diff.Response{}, fmt.Errorf("candidate %s dispatch failed: %w", d.Candidate.Name, candidateResult.err)
	}

	return incumbentResult.resp, candidateResult.resp, nil
}

func (d *Dispatcher) send(ctx context.Context, target Target, method, path string, headers http.Header, body []byte) dispatchResult {
	timeout := target.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, target.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return dispatchResult{err: fmt.Errorf("failed to build request: %w", err)}
	}
	req.Header = headers.Clone()

	resp, err := d.client.Do(req)
	if err != nil {
		return dispatchResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return dispatchResult{err: fmt.Errorf("failed to read response body: %w", err)}
	}

	return dispatchResult{resp: diff.Response{
		StatusCode: resp.StatusCode,
		Headers:    map[string][]string(resp.Header),
		Body:       respBody,
	}}
}

// Listener is an http.Handler that receives mirrored requests, dispatches
// them to both backends, diffs the results, and records the outcome.
// It always responds 200 to the mirror sender regardless of diff outcome —
// the mirror is fire-and-forget infrastructure, not a request the harness
// is actually serving.
type Listener struct {
	Dispatcher *Dispatcher
	DiffConfig diff.Config
	Aggregator *report.Aggregator
}

func (l *Listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusOK) // still ack the mirror; log server-side if needed
		return
	}
	defer func() { _ = r.Body.Close() }()

	incumbentResp, candidateResp, err := l.Dispatcher.Dispatch(r.Context(), r.Method, r.URL.RequestURI(), r.Header, body)
	if err != nil {
		// Both backends must be reachable to produce a meaningful diff;
		// record nothing rather than a misleading partial comparison.
		w.WriteHeader(http.StatusOK)
		return
	}

	divergences := diff.Compare(incumbentResp, candidateResp, l.DiffConfig)

	l.Aggregator.Add(report.Record{
		Timestamp:   time.Now().UTC(),
		Method:      r.Method,
		Path:        r.URL.Path,
		Divergences: divergences,
	})

	w.WriteHeader(http.StatusOK)
}
