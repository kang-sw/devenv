package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

// computeManuals computes the ambient "# Manuals" block: one path + summary
// line per manual under ai-docs/manuals/, or "" when no manuals exist or on
// any resolution error. Modeled on scopeAnnouncement: silent-by-design, never
// blocks workflow_manual from rendering.
//
// Unlike scopeAnnouncement there is no applicability predicate — every
// manual's path + summary is injected unconditionally (260807 Phase 1).
//
// A manual with no `summary:` frontmatter is still listed, with an explicit
// no-summary marker rather than a silently blank or omitted line, so an
// author notices the gap instead of the manual quietly vanishing from the
// ambient block.
func computeManuals(root string) string {
	manuals, err := wsdoc.ManualsList(root)
	if err != nil || len(manuals) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Manuals\n")
	for _, manual := range manuals {
		if manual.Summary == "" {
			fmt.Fprintf(&sb, "- %s — (no summary: add a `summary:` frontmatter line)\n", manual.Path)
			continue
		}
		fmt.Fprintf(&sb, "- %s — %s\n", manual.Path, manual.Summary)
	}
	return strings.TrimRight(sb.String(), "\n")
}
