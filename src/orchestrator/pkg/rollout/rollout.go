// Package rollout is the cutover control loop: it advances a staged,
// weighted traffic shift and watches SLO/health checks during each bake
// period, rolling back to 100% incumbent in a single patch the instant a
// breach is detected. This is the critical path PLAN.md calls out
// explicitly: "Rollback must be automatic and fast — minutes, not hours."
package rollout

import (
	"context"
	"fmt"
	"time"

	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/slo"
	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/traffic"
)

// ShiftSetter is the minimal traffic-shifting capability rollout needs,
// satisfied by *traffic.Shifter in production and a fake in tests.
type ShiftSetter interface {
	SetWeights(ctx context.Context, weights []traffic.BackendWeight) error
}

// Config describes one cutover run.
type Config struct {
	IncumbentName string
	CandidateName string
	// Stages is the candidate-weight progression, e.g. [1, 5, 25, 50, 100].
	// The last stage must be 100 — enforced by traffic.ParseStages.
	Stages []int32
	// BakeDuration is how long to hold and watch each stage before advancing.
	BakeDuration time.Duration
	// ConfirmDuration is how long to hold and watch 100% before declaring
	// the rollout complete.
	ConfirmDuration time.Duration
	// CheckInterval is how often Checkers are polled during a bake period.
	CheckInterval time.Duration
	Checkers      []slo.Checker
}

// StageOutcome records what happened at one stage of the rollout, for the
// audit trail embedded in the remediation certificate.
type StageOutcome struct {
	CandidateWeight int32     `json:"candidate_weight"`
	StartedAt       time.Time `json:"started_at"`
	Outcome         string    `json:"outcome"` // "advanced" | "rolled_back"
	BreachReason    string    `json:"breach_reason,omitempty"`
}

// Result is the full outcome of a Run.
type Result struct {
	Stages     []StageOutcome `json:"stages"`
	RolledBack bool           `json:"rolled_back"`
	Completed  bool           `json:"completed"`
}

// Run executes the staged rollout described by cfg against shifter,
// checking SLOs/health during each bake period. On any breach it issues a
// single immediate weight-revert patch (100% incumbent, 0% candidate) and
// stops — it does not attempt a multi-step unwind. If the rollback patch
// itself fails, Run returns an error clearly marked CRITICAL: the traffic
// split is left in an unknown, unsafe state and needs human intervention.
func Run(ctx context.Context, shifter ShiftSetter, cfg Config) (*Result, error) {
	result := &Result{}

	for _, weight := range cfg.Stages {
		stage := StageOutcome{CandidateWeight: weight, StartedAt: time.Now().UTC()}

		if err := shifter.SetWeights(ctx, []traffic.BackendWeight{
			{Name: cfg.IncumbentName, Weight: 100 - weight},
			{Name: cfg.CandidateName, Weight: weight},
		}); err != nil {
			return result, fmt.Errorf("failed to shift to %d%% candidate: %w", weight, err)
		}

		breached, reason, err := bakeAndWatch(ctx, cfg.BakeDuration, cfg.CheckInterval, cfg.Checkers)
		if err != nil {
			return result, fmt.Errorf("SLO check failed during %d%% bake: %w", weight, err)
		}

		if breached {
			stage.Outcome = "rolled_back"
			stage.BreachReason = reason
			result.Stages = append(result.Stages, stage)
			return rollback(ctx, shifter, cfg, result, reason)
		}

		stage.Outcome = "advanced"
		result.Stages = append(result.Stages, stage)
	}

	breached, reason, err := bakeAndWatch(ctx, cfg.ConfirmDuration, cfg.CheckInterval, cfg.Checkers)
	if err != nil {
		return result, fmt.Errorf("SLO check failed during confirmation window: %w", err)
	}
	if breached {
		result.Stages = append(result.Stages, StageOutcome{
			CandidateWeight: 100,
			StartedAt:       time.Now().UTC(),
			Outcome:         "rolled_back",
			BreachReason:    reason,
		})
		return rollback(ctx, shifter, cfg, result, reason)
	}

	result.Completed = true
	return result, nil
}

func rollback(ctx context.Context, shifter ShiftSetter, cfg Config, result *Result, reason string) (*Result, error) {
	if err := shifter.SetWeights(ctx, []traffic.BackendWeight{
		{Name: cfg.IncumbentName, Weight: 100},
		{Name: cfg.CandidateName, Weight: 0},
	}); err != nil {
		return result, fmt.Errorf("CRITICAL: rollback patch failed after breach (%s); traffic split is in an unknown state and needs immediate manual intervention: %w", reason, err)
	}
	result.RolledBack = true
	return result, nil
}

// bakeAndWatch waits for bake, polling checkers every interval (or once
// immediately if bake <= 0), and returns as soon as a breach is found or
// the bake period elapses cleanly.
func bakeAndWatch(ctx context.Context, bake, interval time.Duration, checkers []slo.Checker) (bool, string, error) {
	if bake <= 0 {
		if len(checkers) == 0 {
			return false, "", nil
		}
		return slo.CheckAll(ctx, checkers)
	}

	if interval <= 0 || interval > bake {
		interval = bake
	}

	deadline := time.Now().Add(bake)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if len(checkers) > 0 {
			breached, reason, err := slo.CheckAll(ctx, checkers)
			if err != nil {
				return false, "", err
			}
			if breached {
				return true, reason, nil
			}
		}

		if !time.Now().Before(deadline) {
			return false, "", nil
		}

		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		case <-ticker.C:
		}
	}
}
