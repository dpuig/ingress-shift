package certificate

import (
	"strings"
	"testing"
	"time"

	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/rollout"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

func TestBuildDecommissionChecklistIncludesControllerName(t *testing.T) {
	checklist := BuildDecommissionChecklist("ingress-nginx")

	if checklist.OldController != "ingress-nginx" {
		t.Errorf("expected old controller to be recorded, got %s", checklist.OldController)
	}
	if len(checklist.Items) == 0 {
		t.Fatal("expected a non-empty checklist")
	}

	found := false
	for _, item := range checklist.Items {
		if strings.Contains(item, "ingress-nginx") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one checklist item to reference the controller by name")
	}
}

func TestDecommissionChecklistToMarkdownIsATaskList(t *testing.T) {
	checklist := BuildDecommissionChecklist("ingress-nginx")
	md := checklist.ToMarkdown()

	if !strings.Contains(md, "- [ ]") {
		t.Error("expected markdown to render as an unchecked task list")
	}
	if !strings.Contains(md, "Decommission Checklist: ingress-nginx") {
		t.Error("expected markdown to include a title referencing the controller")
	}
}

func TestBuildRemediationCertificateFromCompletedRollout(t *testing.T) {
	result := &rollout.Result{
		Completed: true,
		Stages: []rollout.StageOutcome{
			{CandidateWeight: 1, StartedAt: time.Now(), Outcome: "advanced"},
			{CandidateWeight: 100, StartedAt: time.Now(), Outcome: "advanced"},
		},
	}

	cert := BuildRemediationCertificate("checkout-route", "incumbent-svc", "candidate-svc", result)

	if !cert.Completed {
		t.Error("expected certificate to reflect a completed migration")
	}
	if len(cert.Stages) != 2 {
		t.Errorf("expected 2 stages in the audit trail, got %d", len(cert.Stages))
	}
	if cert.HTTPRoute != "checkout-route" {
		t.Errorf("expected HTTPRoute to be recorded, got %s", cert.HTTPRoute)
	}
}

func TestRemediationCertificateToMarkdownContainsStageHistory(t *testing.T) {
	result := &rollout.Result{
		Completed: true,
		Stages: []rollout.StageOutcome{
			{CandidateWeight: 50, StartedAt: time.Now(), Outcome: "advanced"},
		},
	}
	cert := BuildRemediationCertificate("checkout-route", "incumbent-svc", "candidate-svc", result)

	md := cert.ToMarkdown()
	for _, want := range []string{"Remediation Certificate", "checkout-route", "50%", "advanced"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected markdown to contain %q, got:\n%s", want, md)
		}
	}
}

func TestSignAndVerifyRemediationCertificate(t *testing.T) {
	kp, err := sign.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	result := &rollout.Result{Completed: true}
	cert := BuildRemediationCertificate("checkout-route", "incumbent-svc", "candidate-svc", result)

	doc, err := Sign(cert, kp.PrivateKey)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	verified, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if verified.HTTPRoute != cert.HTTPRoute {
		t.Errorf("expected round-tripped certificate to match, got %s want %s", verified.HTTPRoute, cert.HTTPRoute)
	}
}

func TestVerifyFailsOnTamperedCertificate(t *testing.T) {
	kp, err := sign.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	result := &rollout.Result{Completed: true}
	cert := BuildRemediationCertificate("checkout-route", "incumbent-svc", "candidate-svc", result)

	doc, err := Sign(cert, kp.PrivateKey)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Tamper: try to certify a different, unauthorized route.
	doc.Payload = []byte(`{"http_route":"admin-route","completed":true}`)

	if _, err := Verify(doc); err == nil {
		t.Error("expected verification to fail on a tampered certificate")
	}
}
