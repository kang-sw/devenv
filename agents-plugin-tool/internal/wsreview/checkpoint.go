package wsreview

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// SizeThresholdCommits mirrors the lead-review "Deep Review" / is-large-diff
// numeric magnitude (20, agents-plugin/rsrc/lead-review/lead-review.md) as a
// commit-count analog for the cheap checkpoint's *size* dimension. This is a
// deliberate, documented unit reinterpretation, not a shared source-of-truth
// constant: the skill-side threshold measures diff footprint (files/lines),
// while this cheap Go-side check must stay a `git rev-list --count` call and
// never compute a diff stat — per the 2026-08-30 lead adjudication.
const SizeThresholdCommits = 20

// CheckpointNudge computes the cheap review-watermark checkpoint advisory.
//
// It is pure, root-in/string-out, and fail-open — mirroring
// doc_coverage_alarm.go's docCoverageWarning shape — and it NEVER spawns a
// review and NEVER appends to the ledger; it only reads (wsreview.Read, not
// wsreview.Bootstrap). This is the ledger-honesty guard: a checkpoint
// recompute/nudge must never grow the ledger file. Only the explicit,
// caller-opted-in review.marker(bootstrap: true) tool (invoked by the
// separately-run sweep) may bootstrap.
//
// Composition:
//  1. resolve the pre-④ review-track branch (ResolveTrack); any failure
//     silently skips the nudge ("").
//  2. Read (never Bootstrap) the ledger's latest entry; no entry found
//     returns a short baseline-missing advisory, with no write.
//  3. entry found: compute commits ahead of the marker over
//     marker.Head..track-tip, reusing aheadOfMergeRootCount's rev-list
//     --count-via-merge-base shape (implement_resolver.go:420-436) instead
//     of inventing a new git call pattern.
//  4. scale the advisory by SizeThresholdCommits and the staleness knob
//     (StalenessKnob), returning "" when the range is small and fresh
//     (quiet).
func CheckpointNudge(ctx context.Context, root string) string {
	track, err := ResolveTrack(ctx, root)
	if err != nil {
		return ""
	}

	entry, found, err := Read(root)
	if err != nil {
		return ""
	}
	if !found {
		return "no review ledger yet for this project; run a sweep (lead-review range: <base>..<head>) to establish a baseline"
	}

	count := commitsAheadOfMarker(ctx, root, entry.Head, track)
	staleness := StalenessKnob(root)

	switch {
	case count >= SizeThresholdCommits:
		return fmt.Sprintf(
			"review watermark is %d commits behind %s (>= the %d-commit large-accumulation threshold); consider running a sweep (lead-review range: %s..%s) soon.",
			count, track, SizeThresholdCommits, entry.Head, track,
		)
	case count >= staleness:
		return fmt.Sprintf(
			"review watermark is %d commits behind %s (>= the %d-commit staleness threshold); a sweep (lead-review range: %s..%s) would refresh it.",
			count, track, staleness, entry.Head, track,
		)
	default:
		return ""
	}
}

// commitsAheadOfMarker mirrors aheadOfMergeRootCount's shape
// (implement_resolver.go:420-436): merge-base(marker, track) then
// `rev-list --count <merge-base>..<track>`. Fails open to 0 on any git
// error (unresolvable ref, unrelated histories) — consistent with that
// function's own fail-open posture.
func commitsAheadOfMarker(ctx context.Context, root, marker, track string) int {
	result, err := wsgit.NewClient().MergeBase(ctx, root, marker, track)
	if err != nil {
		return 0
	}
	out, err := (wsgit.ExecRunner{}).RunGit(ctx, root, "rev-list", "--count", result.MergeBase+".."+track)
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return count
}
