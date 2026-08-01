// Package knowledgebase loads the versioned annotation knowledge base described
// in PLAN.md: "Treat as a versioned data file, not embedded code." It is the
// compounding asset the analyzer sells against — every customer engagement adds
// entries to annotations.yaml, independent of the analyzer binary's own releases.
package knowledgebase

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v2"
)

//go:embed annotations.yaml
var annotationsYAML []byte

// Classification describes how well an annotation maps onto Gateway API.
type Classification string

const (
	ClassificationDirect    Classification = "direct"
	ClassificationExtension Classification = "extension"
	ClassificationNone      Classification = "none"
)

// Effort is a coarse sizing for the manual work an entry requires.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// Entry is a single annotation's knowledge base record.
type Entry struct {
	Name                   string         `yaml:"name"`
	Description            string         `yaml:"description"`
	Category               string         `yaml:"category"`
	Classification         Classification `yaml:"classification"`
	GatewayAPINote         string         `yaml:"gateway_api_note"`
	Effort                 Effort         `yaml:"effort"`
	BreaksNaiveTranslation bool           `yaml:"breaks_naive_translation"`
}

// IsTranslatable reports whether a core Gateway API field covers this annotation directly.
func (e Entry) IsTranslatable() bool { return e.Classification == ClassificationDirect }

// RequiresExtension reports whether an implementation-specific policy CRD is needed.
func (e Entry) RequiresExtension() bool { return e.Classification == ClassificationExtension }

// NoEquivalent reports whether no Gateway API mapping exists at all.
func (e Entry) NoEquivalent() bool { return e.Classification == ClassificationNone }

var (
	entries []Entry
	byName  map[string]Entry
)

func init() {
	var parsed []Entry
	if err := yaml.Unmarshal(annotationsYAML, &parsed); err != nil {
		panic(fmt.Sprintf("knowledgebase: invalid annotations.yaml: %v", err))
	}

	entries = parsed
	byName = make(map[string]Entry, len(parsed))
	for _, e := range parsed {
		byName[e.Name] = e
	}
}

// Lookup returns the knowledge base entry for an annotation name, and whether it was found.
func Lookup(name string) (Entry, bool) {
	e, ok := byName[name]
	return e, ok
}

// All returns every entry in the knowledge base, in file order.
func All() []Entry {
	return entries
}

// Unknown returns a synthetic entry for an annotation absent from the knowledge base.
// Absence means no mapping has been researched yet, so it defaults to the most
// conservative classification (no equivalent, high effort) rather than implying
// a known extension path.
func Unknown(name string) Entry {
	return Entry{
		Name:           name,
		Description:    "Unknown annotation",
		Category:       "unknown",
		Classification: ClassificationNone,
		GatewayAPINote: "Not present in the knowledge base; requires manual research.",
		Effort:         EffortHigh,
	}
}
