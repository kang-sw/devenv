package wsreview

import (
	"context"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// ResolveTrack resolves the review-track branch. Preference order: the
// `AGENTS.md` `### Review Policy` `review-track` declaration (④,
// 260824-feat-review-release-gate-policy) when present, else the git-default
// heuristic — `origin/HEAD`'s symbolic-ref target (stripped of its "origin/"
// prefix), then local `refs/heads/main`, then `refs/heads/master`.
//
// Fails open: returns ("", err) on any resolution failure (no AGENTS.md
// declaration, no origin remote, detached origin/HEAD, no main/master). The
// caller (CheckpointNudge) treats a resolution failure as "skip the nudge,"
// never as fatal — this is advisory-only best-effort resolution, not a hard
// dependency.
func ResolveTrack(ctx context.Context, root string) (string, error) {
	if policy := ReadAgentsReviewPolicy(root); policy.ReviewTrack != "" {
		return policy.ReviewTrack, nil
	}

	runner := wsgit.ExecRunner{}

	if out, err := runner.RunGit(ctx, root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/"); ref != "" {
			return ref, nil
		}
	}

	for _, candidate := range []string{"main", "master"} {
		if _, err := runner.RunGit(ctx, root, "rev-parse", "--verify", "--quiet", "refs/heads/"+candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("resolve review track: no origin/HEAD symbolic-ref and neither refs/heads/main nor refs/heads/master exists")
}
