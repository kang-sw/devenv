package wsreview

import (
	"os"
	"path/filepath"
	"regexp"
)

// Enum values for AgentsReviewPolicy.ReleaseBoundary.
const (
	ReleaseBoundaryPresent = "present"
	ReleaseBoundaryAbsent  = "absent"
)

// Enum values for AgentsReviewPolicy.RendezvousBackend.
const (
	RendezvousBackendPlatform = "platform"
	RendezvousBackendCanary   = "canary"
)

// reviewPolicySectionRE isolates the `### Review Policy` section's body out
// of a project's root AGENTS.md, stopping at the next heading of level 1-3
// (so nested content under an unrelated subsection never leaks in) or end of
// file. `### Review Policy` sits under `## Workflow` per this ticket's
// documented placement, one level deeper than config.go's `## Checkpoint
// Nudge` top-level section, hence the wider `#{1,3} ` stop pattern.
var reviewPolicySectionRE = regexp.MustCompile(`(?ms)^### Review Policy\s*\n(.*?)(?:\n#{1,3} |\z)`)

// reviewTrackLineRE matches a `review-track: <branch>` line.
var reviewTrackLineRE = regexp.MustCompile(`(?m)^review-track:\s*(\S+)\s*$`)

// releaseBoundaryLineRE matches a `release-boundary: present|absent` line.
var releaseBoundaryLineRE = regexp.MustCompile(`(?m)^release-boundary:\s*(\S+)\s*$`)

// rendezvousBackendLineRE matches a `rendezvous-backend: platform|canary` line.
var rendezvousBackendLineRE = regexp.MustCompile(`(?m)^rendezvous-backend:\s*(\S+)\s*$`)

// AgentsReviewPolicy is the parsed `### Review Policy` config surface read
// from a project's root AGENTS.md: the tracked, per-track structural facts
// config home (as opposed to the `ai-docs/` ledger's marker+verdict state or
// `_review.local.md`'s machine-local mechanics — see ReadAgentsReviewPolicy).
type AgentsReviewPolicy struct {
	// ReviewTrack is the declared review-track branch, or "" when unset.
	ReviewTrack string
	// ReleaseBoundary is ReleaseBoundaryPresent or ReleaseBoundaryAbsent,
	// defaulting to ReleaseBoundaryAbsent.
	ReleaseBoundary string
	// RendezvousBackend is RendezvousBackendPlatform or
	// RendezvousBackendCanary, defaulting to RendezvousBackendCanary (needs
	// no GitHub branch-protection config).
	RendezvousBackend string
}

// ReadAgentsReviewPolicy reads the review-policy fields from root's
// AGENTS.md `### Review Policy` section, e.g.:
//
//	### Review Policy
//	review-track: develop
//	release-boundary: present
//	rendezvous-backend: canary
//
// Never errors: a missing file, a missing section, or a missing/malformed
// field all fail open to the zero-value/documented default for that field
// alone — mirroring StalenessKnob's (config.go) fail-open contract. This is
// the `AGENTS.md` config home; it does not read or write
// `ai-docs/_review.local.md` (machine-local mechanics) or the `ai-docs/`
// review ledger (marker+verdict state) — those are separate, non-overlapping
// config homes.
func ReadAgentsReviewPolicy(root string) AgentsReviewPolicy {
	policy := AgentsReviewPolicy{
		ReleaseBoundary:   ReleaseBoundaryAbsent,
		RendezvousBackend: RendezvousBackendCanary,
	}

	raw, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return policy
	}

	section := reviewPolicySectionRE.FindStringSubmatch(string(raw))
	if section == nil {
		return policy
	}
	body := section[1]

	if m := reviewTrackLineRE.FindStringSubmatch(body); m != nil {
		policy.ReviewTrack = m[1]
	}
	if m := releaseBoundaryLineRE.FindStringSubmatch(body); m != nil {
		switch m[1] {
		case ReleaseBoundaryPresent, ReleaseBoundaryAbsent:
			policy.ReleaseBoundary = m[1]
		}
		// Any other value fails open to the built-in ReleaseBoundaryAbsent
		// default set above.
	}
	if m := rendezvousBackendLineRE.FindStringSubmatch(body); m != nil {
		switch m[1] {
		case RendezvousBackendPlatform, RendezvousBackendCanary:
			policy.RendezvousBackend = m[1]
		}
		// Any other value fails open to the built-in RendezvousBackendCanary
		// default set above.
	}

	return policy
}
