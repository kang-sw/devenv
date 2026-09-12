package wsreview

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// bindingAnchorSectionRE isolates the `### Binding Anchor` section's body out
// of a project's root AGENTS.md, stopping at the next heading of level 1-3 (so
// nested content under an unrelated subsection never leaks in) or end of file.
// `### Binding Anchor` sits under `## Workflow` as a sibling of `### Review
// Policy`, so it uses the same wider `#{1,3} ` stop pattern as
// reviewPolicySectionRE.
var bindingAnchorSectionRE = regexp.MustCompile(`(?ms)^### Binding Anchor\s*\n(.*?)(?:\n#{1,3} |\z)`)

// bindingAnchorLineRE matches an `anchor: <path>` line.
var bindingAnchorLineRE = regexp.MustCompile(`(?m)^anchor:\s*(.+?)\s*$`)

// bindingTopicsLineRE matches a `topics: <comma-separated phrases>` line.
var bindingTopicsLineRE = regexp.MustCompile(`(?m)^topics:\s*(.+?)\s*$`)

// BindingAnchor is the parsed `### Binding Anchor` config surface read from a
// project's root AGENTS.md. It is the generic hook a project uses to declare
// the ticket/artifact a lead must read before answering or editing when a
// target touches one of the declared topics; the shipped playbooks and resolver
// interpolate the declared values rather than naming any project's anchor.
type BindingAnchor struct {
	// Anchor is the declared anchor path, or "" when unset.
	Anchor string
	// Topics is the declared comma-separated topics phrase, or "" when unset.
	Topics string
	// Declared is true only when both anchor and topics parse non-empty.
	Declared bool
}

// ReadAgentsBindingAnchor reads the binding-anchor fields from root's AGENTS.md
// `### Binding Anchor` section, e.g.:
//
//	### Binding Anchor
//	anchor: ai-docs/tickets/idea/000000-example-anchor.md
//	topics: plugin architecture, adapter boundaries
//
// Never errors: a missing file, a missing section, or a missing/empty field all
// fail open to an undeclared BindingAnchor (Declared: false) — mirroring
// ReadAgentsReviewPolicy's fail-open contract. Both keys must parse non-empty
// for Declared to be true; a project that declares only one key renders no
// clause.
func ReadAgentsBindingAnchor(root string) BindingAnchor {
	raw, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return BindingAnchor{}
	}

	section := bindingAnchorSectionRE.FindStringSubmatch(string(raw))
	if section == nil {
		return BindingAnchor{}
	}
	body := section[1]

	var out BindingAnchor
	if m := bindingAnchorLineRE.FindStringSubmatch(body); m != nil {
		out.Anchor = strings.TrimSpace(m[1])
	}
	if m := bindingTopicsLineRE.FindStringSubmatch(body); m != nil {
		out.Topics = strings.TrimSpace(m[1])
	}
	out.Declared = out.Anchor != "" && out.Topics != ""
	return out
}

// PrepClause renders the Prep-guardrail anchor clause for an implement verdict:
// an empty string when the project declares no binding anchor, otherwise a
// "read <anchor> when target touches <topics>, " fragment (trailing comma-space
// so it splices cleanly between the ancestor-reads and impl-playbook clauses).
func (b BindingAnchor) PrepClause() string {
	if !b.Declared {
		return ""
	}
	return "read " + b.Anchor + " when target touches " + b.Topics + ", "
}
