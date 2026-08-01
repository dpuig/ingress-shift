package rollout

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/slo"
	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/traffic"
)

// fakeShifter records every SetWeights call for assertions and can be
// configured to fail on a specific call.
type fakeShifter struct {
	mu       sync.Mutex
	calls    [][]traffic.BackendWeight
	failCall int // 1-indexed; 0 means never fail
	failErr  error
}

func (f *fakeShifter) SetWeights(ctx context.Context, weights []traffic.BackendWeight) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, weights)
	if f.failCall != 0 && len(f.calls) == f.failCall {
		return f.failErr
	}
	return nil
}

func (f *fakeShifter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeShifter) lastCall() []traffic.BackendWeight {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// fakeChecker breaches on a specific invocation number (1-indexed), never if breachOn is 0.
type fakeChecker struct {
	mu       sync.Mutex
	calls    int
	breachOn int
	reason   string
}

func (f *fakeChecker) Name() string { return "fake-checker" }

func (f *fakeChecker) Check(ctx context.Context) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.breachOn != 0 && f.calls >= f.breachOn {
		return true, f.reason, nil
	}
	return false, "", nil
}

func weightFor(weights []traffic.BackendWeight, name string) int32 {
	for _, w := range weights {
		if w.Name == name {
			return w.Weight
		}
	}
	return -1
}

func TestRunCompletesAllStagesWithoutBreach(t *testing.T) {
	shifter := &fakeShifter{}
	cfg := Config{
		IncumbentName: "incumbent",
		CandidateName: "candidate",
		Stages:        []int32{1, 5, 25, 50, 100},
	}

	result, err := Run(context.Background(), shifter, cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !result.Completed {
		t.Error("expected rollout to complete")
	}
	if result.RolledBack {
		t.Error("expected no rollback")
	}
	if len(result.Stages) != 5 {
		t.Errorf("expected 5 stage outcomes, got %d", len(result.Stages))
	}
	for _, s := range result.Stages {
		if s.Outcome != "advanced" {
			t.Errorf("expected all stages to advance, got %s at weight %d", s.Outcome, s.CandidateWeight)
		}
	}

	if shifter.callCount() != 5 {
		t.Errorf("expected 5 SetWeights calls, got %d", shifter.callCount())
	}
	last := shifter.lastCall()
	if weightFor(last, "candidate") != 100 || weightFor(last, "incumbent") != 0 {
		t.Errorf("expected final call to be 100%% candidate, got %+v", last)
	}
}

func TestRunRollsBackImmediatelyOnBreachMidStage(t *testing.T) {
	shifter := &fakeShifter{}
	checker := &fakeChecker{breachOn: 1, reason: "error rate exceeded SLO"}

	cfg := Config{
		IncumbentName: "incumbent",
		CandidateName: "candidate",
		Stages:        []int32{1, 5, 25, 50, 100},
		BakeDuration:  0, // check once, no waiting — deterministic test
		Checkers:      []slo.Checker{checker},
	}

	result, err := Run(context.Background(), shifter, cfg)
	if err != nil {
		t.Fatalf("Run should not error on a clean rollback, got: %v", err)
	}

	if !result.RolledBack {
		t.Fatal("expected rollout to roll back")
	}
	if result.Completed {
		t.Error("expected rollout not to be marked completed")
	}

	// Breach happens on the very first stage (weight=1): one SetWeights call
	// to enter that stage, then one rollback call. Nothing beyond that.
	if shifter.callCount() != 2 {
		t.Fatalf("expected exactly 2 SetWeights calls (enter stage + rollback), got %d: %+v", shifter.callCount(), shifter.calls)
	}

	rollbackCall := shifter.lastCall()
	if weightFor(rollbackCall, "incumbent") != 100 || weightFor(rollbackCall, "candidate") != 0 {
		t.Errorf("expected rollback to be a single 100%%/0%% patch, got %+v", rollbackCall)
	}

	if len(result.Stages) != 1 {
		t.Fatalf("expected 1 stage outcome, got %d", len(result.Stages))
	}
	if result.Stages[0].Outcome != "rolled_back" {
		t.Errorf("expected stage outcome 'rolled_back', got %s", result.Stages[0].Outcome)
	}
	if result.Stages[0].BreachReason != "error rate exceeded SLO" {
		t.Errorf("expected breach reason to be recorded, got %q", result.Stages[0].BreachReason)
	}
}

func TestRunRollsBackDuringLaterStage(t *testing.T) {
	shifter := &fakeShifter{}
	// Breach on the 3rd checker invocation — one call per stage since BakeDuration=0.
	checker := &fakeChecker{breachOn: 3, reason: "latency SLO breached"}

	cfg := Config{
		IncumbentName: "incumbent",
		CandidateName: "candidate",
		Stages:        []int32{1, 5, 25, 50, 100},
		BakeDuration:  0,
		Checkers:      []slo.Checker{checker},
	}

	result, err := Run(context.Background(), shifter, cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !result.RolledBack {
		t.Fatal("expected rollback")
	}
	// Stages 1 and 5 advance, stage 25 triggers the breach: 3 stage-entry
	// calls + 1 rollback call = 4.
	if shifter.callCount() != 4 {
		t.Fatalf("expected 4 SetWeights calls, got %d: %+v", shifter.callCount(), shifter.calls)
	}
	if len(result.Stages) != 3 {
		t.Fatalf("expected 3 stage outcomes (2 advanced + 1 rolled back), got %d", len(result.Stages))
	}
	if result.Stages[0].Outcome != "advanced" || result.Stages[1].Outcome != "advanced" {
		t.Errorf("expected first two stages to advance, got %+v", result.Stages[:2])
	}
	if result.Stages[2].Outcome != "rolled_back" || result.Stages[2].CandidateWeight != 25 {
		t.Errorf("expected rollback at weight 25, got %+v", result.Stages[2])
	}
}

func TestRunRollsBackDuringConfirmationWindow(t *testing.T) {
	shifter := &fakeShifter{}
	// Never breach during the 1 stage's bake check, but breach on the 2nd
	// call (the confirmation-window check).
	checker := &fakeChecker{breachOn: 2, reason: "regression detected during confirmation"}

	cfg := Config{
		IncumbentName:   "incumbent",
		CandidateName:   "candidate",
		Stages:          []int32{100},
		BakeDuration:    0,
		ConfirmDuration: 0,
		Checkers:        []slo.Checker{checker},
	}

	result, err := Run(context.Background(), shifter, cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !result.RolledBack {
		t.Fatal("expected rollback during confirmation window")
	}
	if result.Completed {
		t.Error("expected rollout not to be marked completed")
	}
}

func TestRunReturnsCriticalErrorWhenRollbackPatchFails(t *testing.T) {
	rollbackErr := errors.New("API server unreachable")
	shifter := &fakeShifter{failCall: 2, failErr: rollbackErr} // 2nd call = the rollback patch
	checker := &fakeChecker{breachOn: 1, reason: "breach"}

	cfg := Config{
		IncumbentName: "incumbent",
		CandidateName: "candidate",
		Stages:        []int32{1, 100},
		BakeDuration:  0,
		Checkers:      []slo.Checker{checker},
	}

	_, err := Run(context.Background(), shifter, cfg)
	if err == nil {
		t.Fatal("expected an error when the rollback patch itself fails")
	}
	if !errors.Is(err, rollbackErr) {
		t.Errorf("expected error to wrap the underlying rollback failure, got: %v", err)
	}
}

func TestRunPropagatesShiftFailureOnEntry(t *testing.T) {
	entryErr := errors.New("HTTPRoute not found")
	shifter := &fakeShifter{failCall: 1, failErr: entryErr}

	cfg := Config{
		IncumbentName: "incumbent",
		CandidateName: "candidate",
		Stages:        []int32{1, 100},
	}

	_, err := Run(context.Background(), shifter, cfg)
	if err == nil {
		t.Fatal("expected an error when entering the first stage fails")
	}
	if !errors.Is(err, entryErr) {
		t.Errorf("expected wrapped entry error, got: %v", err)
	}
}

func TestBakeAndWatchRespectsRealDuration(t *testing.T) {
	checker := &fakeChecker{breachOn: 0}
	start := time.Now()

	breached, _, err := bakeAndWatch(context.Background(), 40*time.Millisecond, 10*time.Millisecond, []slo.Checker{checker})
	if err != nil {
		t.Fatalf("bakeAndWatch failed: %v", err)
	}
	if breached {
		t.Error("expected no breach")
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("expected bakeAndWatch to wait at least 40ms, took %v", elapsed)
	}
}
