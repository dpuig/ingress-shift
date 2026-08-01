package knowledgebase

import "testing"

func TestAllEntriesHaveRequiredFields(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("knowledge base is empty")
	}

	seen := make(map[string]bool, len(all))
	for _, e := range all {
		if e.Name == "" {
			t.Errorf("entry has empty name: %+v", e)
			continue
		}
		if seen[e.Name] {
			t.Errorf("duplicate entry name: %s", e.Name)
		}
		seen[e.Name] = true

		if e.Description == "" {
			t.Errorf("%s: missing description", e.Name)
		}
		if e.Category == "" {
			t.Errorf("%s: missing category", e.Name)
		}
		if e.GatewayAPINote == "" {
			t.Errorf("%s: missing gateway_api_note", e.Name)
		}

		switch e.Classification {
		case ClassificationDirect, ClassificationExtension, ClassificationNone:
		default:
			t.Errorf("%s: invalid classification %q", e.Name, e.Classification)
		}

		switch e.Effort {
		case EffortLow, EffortMedium, EffortHigh:
		default:
			t.Errorf("%s: invalid effort %q", e.Name, e.Effort)
		}
	}
}

func TestClassificationHelpersAreMutuallyExclusive(t *testing.T) {
	for _, e := range All() {
		count := 0
		if e.IsTranslatable() {
			count++
		}
		if e.RequiresExtension() {
			count++
		}
		if e.NoEquivalent() {
			count++
		}
		if count != 1 {
			t.Errorf("%s: expected exactly one classification helper to be true, got %d", e.Name, count)
		}
	}
}

func TestLookup(t *testing.T) {
	e, ok := Lookup("nginx.ingress.kubernetes.io/rewrite-target")
	if !ok {
		t.Fatal("expected rewrite-target to be found")
	}
	if !e.BreaksNaiveTranslation {
		t.Error("rewrite-target should be flagged as breaking naive translation")
	}

	_, ok = Lookup("nginx.ingress.kubernetes.io/does-not-exist")
	if ok {
		t.Error("expected unknown annotation to not be found")
	}
}

func TestUnknownDefaultsToNoEquivalent(t *testing.T) {
	e := Unknown("custom.example.com/whatever")
	if !e.NoEquivalent() {
		t.Error("unknown annotation should default to no-equivalent classification")
	}
	if e.Effort != EffortHigh {
		t.Error("unknown annotation should default to high effort")
	}
}

// The plan explicitly names six classes that break naive translation:
// auth snippets, custom NGINX config blocks, rewrite semantics, session
// affinity, rate limiting, and mTLS passthrough. Each must be represented
// by at least one flagged entry.
func TestBreaksNaiveTranslationCoversPlanClasses(t *testing.T) {
	required := map[string]bool{
		"auth":       false,
		"snippet":    false,
		"rewrite":    false,
		"affinity":   false,
		"rate-limit": false,
		"mtls":       false,
		"tls":        false, // ssl-passthrough
	}

	for _, e := range All() {
		if !e.BreaksNaiveTranslation {
			continue
		}
		if _, tracked := required[e.Category]; tracked {
			required[e.Category] = true
		}
	}

	for category, found := range required {
		if !found {
			t.Errorf("no breaks_naive_translation entry found for category %q", category)
		}
	}
}
