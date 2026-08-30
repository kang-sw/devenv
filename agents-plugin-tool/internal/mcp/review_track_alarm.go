package mcp

import (
	"github.com/kang-sw/devenv/internal/wsreview"
)

// reviewTrackNudge computes the review-track config advisory, or "" when no
// nudge should be surfaced. Silent case (by design, not a bug):
//   - root's AGENTS.md `### Review Policy` section declares a non-empty
//     `review-track` field.
//
// Firing case:
//   - The `review-track` field is unset (missing file, missing section, or
//     missing field — wsreview.ReadAgentsReviewPolicy's fail-open contract),
//     so wsreview.ResolveTrack falls back to the git-default heuristic
//     instead of an explicit declaration.
//
// This function is stateless and always safe to call — like
// docCoverageWarning, it is not itself the once-per-session gate. The two
// workflow_manual.go wiring sites own the once-per-session suppression via
// sessionRecord.ReviewTrackNudgeShown / setReviewTrackNudgeShown.
func reviewTrackNudge(root string) string {
	if wsreview.ReadAgentsReviewPolicy(root).ReviewTrack != "" {
		return ""
	}

	return "> **Review-track branch is not configured.** This project's AGENTS.md has no `### Review Policy` `review-track` field, so the review sweep falls back to the git-default branch heuristic. Declare `review-track: <branch>` (plus `release-boundary`, `rendezvous-backend`, `release-tag-glob`) under a `### Review Policy` subsection to make the declaration explicit."
}

// injectReviewTrackNudge prepends nudge to body, delegating to the existing
// generic prepend-if-nonempty helper rather than duplicating it.
func injectReviewTrackNudge(body, nudge string) string {
	return injectBootstrapStalenessWarning(body, nudge)
}
