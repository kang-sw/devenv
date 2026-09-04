package wsreview

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// DefaultStalenessCommits is the checkpoint-nudge staleness knob's built-in
// default, per the 2026-08-30 lead adjudication: deliberately below
// SizeThresholdCommits (20) so the two thresholds stay non-degenerate —
// quiet below 10 commits, a gentle FYI at 10-19, a stronger "large
// accumulation" nudge at >=20.
const DefaultStalenessCommits = 10

// checkpointNudgeSectionRE isolates a `## Checkpoint Nudge` section's body
// out of ai-docs/_review.local.md, stopping at the next top-level heading or
// end of file.
var checkpointNudgeSectionRE = regexp.MustCompile(`(?ms)^## Checkpoint Nudge\s*\n(.*?)(?:\n## |\z)`)

// stalenessLineRE matches a `staleness: <N> commits` line inside the
// Checkpoint Nudge section.
var stalenessLineRE = regexp.MustCompile(`(?m)^staleness:\s*(\d+)\s*commits?\s*$`)

// StalenessKnob reads the checkpoint-nudge staleness commit-count knob from
// ai-docs/_review.local.md's `## Checkpoint Nudge` section, e.g.:
//
//	## Checkpoint Nudge
//	staleness: 15 commits
//
// Never errors: a missing file, a missing section, or a malformed value all
// fail open to DefaultStalenessCommits — mirroring wsreview.Read's
// "no file yet" contract and lead-review.md's documented "config load is
// always optional" invariant. No Go-side parser existed for this file prior
// to this function; this is new, narrowly-scoped code, not a shared config
// loader.
func StalenessKnob(root string) int {
	raw, err := os.ReadFile(filepath.Join(root, "ai-docs", "_review.local.md"))
	if err != nil {
		return DefaultStalenessCommits
	}

	section := checkpointNudgeSectionRE.FindStringSubmatch(string(raw))
	if section == nil {
		return DefaultStalenessCommits
	}

	m := stalenessLineRE.FindStringSubmatch(section[1])
	if m == nil {
		return DefaultStalenessCommits
	}

	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return DefaultStalenessCommits
	}
	return n
}
