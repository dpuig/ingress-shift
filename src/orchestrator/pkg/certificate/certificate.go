// Package certificate produces the two artifacts PLAN.md calls for at the
// end of a successful cutover: a decommission checklist for the old ingress
// controller, and a signed remediation certificate documenting the
// completed migration as a compliance artifact.
package certificate

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/rollout"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

// DecommissionChecklist is a plain (unsigned) operational checklist — it
// guides a human through retiring the old controller, it isn't compliance
// evidence itself.
type DecommissionChecklist struct {
	GeneratedAt   time.Time `json:"generated_at"`
	OldController string    `json:"old_controller"`
	Items         []string  `json:"items"`
}

// BuildDecommissionChecklist returns the standard checklist for retiring
// oldController once a cutover has completed and held at 100% for the
// confirmation window.
func BuildDecommissionChecklist(oldController string) *DecommissionChecklist {
	return &DecommissionChecklist{
		GeneratedAt:   time.Now().UTC(),
		OldController: oldController,
		Items: []string{
			"Confirm 100% traffic has been on the candidate controller for the full confirmation window with no rollback",
			"Remove DNS / LoadBalancer records that still point at " + oldController,
			"Scale the " + oldController + " Deployment/DaemonSet to zero replicas (don't delete yet — keep a fast revert path for one release cycle)",
			"Revoke " + oldController + "'s RBAC permissions beyond what's needed for a clean uninstall",
			"Archive " + oldController + "'s configuration (Ingress resources, ConfigMaps, Secrets it used) for audit purposes",
			"Remove the " + oldController + " IngressClass if no other workload still references it",
			"Update runbooks, dashboards, and on-call documentation to reference the new Gateway API controller",
			"After the revert-path window has passed with no issues, uninstall " + oldController + " and delete its remaining resources",
		},
	}
}

// ToMarkdown renders the checklist as a Markdown task list.
func (c *DecommissionChecklist) ToMarkdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Decommission Checklist: %s\n\n", c.OldController)
	fmt.Fprintf(&b, "Generated: %s\n\n", c.GeneratedAt.Format(time.RFC3339))
	for _, item := range c.Items {
		fmt.Fprintf(&b, "- [ ] %s\n", item)
	}
	return b.String()
}

// RemediationCertificate is the audit-facing compliance artifact
// documenting a completed migration, including the full stage-by-stage
// audit trail from the rollout.
type RemediationCertificate struct {
	GeneratedAt   time.Time              `json:"generated_at"`
	HTTPRoute     string                 `json:"http_route"`
	IncumbentName string                 `json:"incumbent_name"`
	CandidateName string                 `json:"candidate_name"`
	Stages        []rollout.StageOutcome `json:"stages"`
	Completed     bool                   `json:"completed"`
}

// BuildRemediationCertificate summarizes a completed rollout.Result into a certificate.
// It should only be called when result.Completed is true — the CLI layer is
// responsible for that check, since an incomplete/rolled-back run has no
// migration to certify.
func BuildRemediationCertificate(httpRoute, incumbentName, candidateName string, result *rollout.Result) *RemediationCertificate {
	return &RemediationCertificate{
		GeneratedAt:   time.Now().UTC(),
		HTTPRoute:     httpRoute,
		IncumbentName: incumbentName,
		CandidateName: candidateName,
		Stages:        result.Stages,
		Completed:     result.Completed,
	}
}

// ToMarkdown renders the certificate as a Markdown document suitable for
// attaching to a change record.
func (c *RemediationCertificate) ToMarkdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Remediation Certificate\n\n")
	fmt.Fprintf(&b, "- Generated: %s\n", c.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- HTTPRoute: %s\n", c.HTTPRoute)
	fmt.Fprintf(&b, "- Incumbent: %s\n", c.IncumbentName)
	fmt.Fprintf(&b, "- Candidate: %s\n", c.CandidateName)
	fmt.Fprintf(&b, "- Migration completed: %v\n\n", c.Completed)

	fmt.Fprintf(&b, "## Stage history\n\n")
	fmt.Fprintf(&b, "| Time | Candidate Weight | Outcome |\n|---|---|---|\n")
	for _, s := range c.Stages {
		fmt.Fprintf(&b, "| %s | %d%% | %s |\n", s.StartedAt.Format(time.RFC3339), s.CandidateWeight, s.Outcome)
	}

	return b.String()
}

// Sign produces a signed envelope around the certificate.
func Sign(c *RemediationCertificate, priv ed25519.PrivateKey) (*sign.Document, error) {
	return sign.SignJSON(priv, c)
}

// Verify checks a signed envelope's signature and unmarshals the enclosed certificate.
func Verify(doc *sign.Document) (*RemediationCertificate, error) {
	if err := sign.Verify(doc); err != nil {
		return nil, err
	}
	var c RemediationCertificate
	if err := json.Unmarshal(doc.Payload, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal remediation certificate: %w", err)
	}
	return &c, nil
}
