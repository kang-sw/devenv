package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

// manualsAuthoringGuidance is the fixed authoring-convention paragraph rendered
// under the "# Manuals" header on every lead workflow_manual. It teaches where
// shared project procedures live (tracked, with a `summary:` line) and the
// local/tracked split (machine-local details go to a gitignored `*.local.md`).
// It is deliberately always present — unlike the presence-gated `# Notes` block
// — so the convention anchors even for a project that has not authored any
// manual yet.
const manualsAuthoringGuidance = "Shared project procedures (build, deploy, env setup, …) live here: one markdown\n" +
	"file per procedure under `ai-docs/manuals/`, each opening with a one-line\n" +
	"`summary:` frontmatter. Keep machine-local details (credentials, IPs, hostnames)\n" +
	"out of tracked manuals — write them to a sibling `*.local.md` (gitignored)."

// computeManuals computes the always-on ambient "# Manuals" block: the header,
// the fixed manualsAuthoringGuidance paragraph, then one line per manual under
// ai-docs/manuals/ (or a "- (none yet)" placeholder when none exist yet). The
// block is an ever-present authoring anchor for the lead session, not a
// presence-gated memory dump like `# Notes`.
//
// It returns "" only on a genuine resolution error (a non-NotExist ReadDir
// failure), preserving the scopeAnnouncement-style "silent, never blocks
// workflow_manual" doctrine for the error path; a missing/empty
// ai-docs/manuals/ directory is the common steady state and still renders the
// anchor with the "(none yet)" placeholder.
//
// Unlike scopeAnnouncement there is no applicability predicate — every tracked
// manual's path + summary is injected unconditionally (260807 Phase 1).
//
// A tracked manual with no `summary:` frontmatter is still listed, with an
// explicit no-summary marker rather than a silently blank or omitted line, so
// an author notices the gap. A `*.local.md` manual is listed as a bare `- <path>`
// line with no summary rendered and no no-summary nag: the `.local.md` suffix
// already marks it machine-local, and nagging a gitignored creds/IP file to add
// frontmatter would be wrong.
func computeManuals(root string) string {
	manuals, err := wsdoc.ManualsList(root)
	if err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Manuals\n")
	sb.WriteString(manualsAuthoringGuidance)
	sb.WriteString("\n")
	if len(manuals) == 0 {
		sb.WriteString("- (none yet)\n")
		return strings.TrimRight(sb.String(), "\n")
	}
	for _, manual := range manuals {
		if strings.HasSuffix(manual.Path, ".local.md") {
			fmt.Fprintf(&sb, "- %s\n", manual.Path)
			continue
		}
		if manual.Summary == "" {
			fmt.Fprintf(&sb, "- %s — (no summary: add a `summary:` frontmatter line)\n", manual.Path)
			continue
		}
		fmt.Fprintf(&sb, "- %s — %s\n", manual.Path, manual.Summary)
	}
	return strings.TrimRight(sb.String(), "\n")
}
