package mcp

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsreview"
)

type implMergeAcknowledgement struct {
	ReleaseTargetOverride bool
	ExpectedSourceOID     string
	ExpectedTargetOID     string
}

type implMergeDiagnostic struct {
	Code           string   `json:"code"`
	Classification string   `json:"classification"`
	Reason         string   `json:"reason"`
	Resolution     string   `json:"resolution"`
	Command        []string `json:"command,omitempty"`
	RawOutput      string   `json:"raw_output,omitempty"`
}

type implMergeReviewEvidence struct {
	Frontier       *wsreview.Entry `json:"frontier,omitempty"`
	CandidateRange string          `json:"candidate_range,omitempty"`
	UncoveredRange string          `json:"uncovered_range,omitempty"`
	UncoveredCount *uint64         `json:"uncovered_count,omitempty"`
	Note           string          `json:"note"`
}

type implMergeResult struct {
	Branch        string                   `json:"branch"`
	Target        string                   `json:"target"`
	Status        string                   `json:"status"`
	Commit        string                   `json:"commit,omitempty"`
	BranchDeleted bool                     `json:"branch_deleted"`
	Advisory      string                   `json:"advisory,omitempty"`
	SourceOID     string                   `json:"source_oid,omitempty"`
	TargetOID     string                   `json:"target_oid,omitempty"`
	Diagnostics   []implMergeDiagnostic    `json:"diagnostics,omitempty"`
	Review        *implMergeReviewEvidence `json:"review,omitempty"`
}

func (r implMergeResult) text() string {
	text := fmt.Sprintf("status: %s\nbranch: %s\ntarget: %s\ncommit: %s\nbranch_deleted: %t\nadvisory: %s\n", r.Status, r.Branch, r.Target, r.Commit, r.BranchDeleted, r.Advisory)
	if r.SourceOID != "" || r.TargetOID != "" {
		text += fmt.Sprintf("source_oid: %s\ntarget_oid: %s\n", r.SourceOID, r.TargetOID)
	}
	for _, d := range r.Diagnostics {
		text += fmt.Sprintf("diagnostic: %s [%s]: %s\nresolution: %s\n", d.Code, d.Classification, d.Reason, d.Resolution)
		if len(d.Command) > 0 {
			text += fmt.Sprintf("command: git %s\nraw_output: %q\n", strings.Join(d.Command, " "), d.RawOutput)
		}
	}
	if r.Review != nil {
		text += fmt.Sprintf("review: %s\ncandidate_range: %s\nuncovered_range: %s\n", r.Review.Note, r.Review.CandidateRange, r.Review.UncoveredRange)
		if r.Review.Frontier != nil {
			text += fmt.Sprintf("review_frontier: %s..%s: %s\n", r.Review.Frontier.Base, r.Review.Frontier.Head, r.Review.Frontier.Verdict)
		}
		if r.Review.UncoveredCount != nil {
			text += fmt.Sprintf("uncovered_count: %d\n", *r.Review.UncoveredCount)
		}
	}
	return text
}

func mergeImplBranch(ctx context.Context, root string, runner wsgit.Runner, branch, target string, message wsgit.CommitOptions, acknowledgement implMergeAcknowledgement) (implMergeResult, error) {
	result := implMergeResult{}
	run := func(args ...string) (string, error) {
		out, err := runner.RunGit(ctx, root, args...)
		return strings.TrimSpace(string(out)), err
	}
	message.Title = strings.TrimSpace(message.Title)
	if message.Title == "" || strings.ContainsAny(message.Title, "\r\n") {
		return result, fmt.Errorf("git.merge requires a single-line title")
	}
	var bullets []string
	for _, bullet := range message.AIContext {
		if bullet = strings.TrimSpace(bullet); bullet != "" {
			bullets = append(bullets, bullet)
		}
	}
	if len(bullets) == 0 {
		return result, fmt.Errorf("git.merge requires nonempty ai_context")
	}
	message.AIContext = bullets
	if branch == "" {
		var err error
		branch, err = run("symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return result, err
		}
	}
	mergeRoot, stem, ok := parseImplBranchRoot(branch)
	if !ok || mergeRoot == "" || stem == "" {
		return result, fmt.Errorf("git.merge requires impl/<root>/<stem>, got %q", branch)
	}
	// A refs/heads/- ref is valid, but switch interprets "-" as the previous
	// checkout even after "--". Never let branch shorthand select the target.
	if strings.HasPrefix(mergeRoot, "-") {
		return result, fmt.Errorf("invalid merge target %q", mergeRoot)
	}
	result.Branch, result.Target = branch, mergeRoot
	releaseTarget := mergeRoot == "main" || mergeRoot == "master"
	add := func(code, reason, resolution string) {
		result.Diagnostics = append(result.Diagnostics, implMergeDiagnostic{Code: code, Classification: "must_resolve", Reason: reason, Resolution: resolution})
	}
	gitFailure := func(code, reason, output string, err error, args ...string) {
		if err != nil {
			output += "\n" + err.Error()
		}
		if len(output) > 2048 {
			output = output[:2048] + " [truncated]"
		}
		add(code, reason, "Resolve the Git inspection failure, then retry git.merge to inspect a fresh candidate.")
		d := &result.Diagnostics[len(result.Diagnostics)-1]
		d.Command, d.RawOutput = args, output
	}
	if target != "" && target != mergeRoot {
		add("target_mismatch", fmt.Sprintf("target %q does not match encoded root %q", target, mergeRoot), "Use the encoded root as the target assertion, or select the intended impl branch.")
	}
	if releaseTarget && !acknowledgement.ReleaseTargetOverride {
		result.Diagnostics = append(result.Diagnostics, implMergeDiagnostic{Code: "release_target", Classification: "overrideable", Reason: "The encoded target is a release-class branch; inferred repository topology does not authorize this merge.", Resolution: "After explicit acknowledgement, retry the same git.merge call with release_target_override: true, expected_source_oid: source_oid, and expected_target_oid: target_oid returned here. Resolve all must_resolve findings first."})
	}
	if acknowledgement.ReleaseTargetOverride && !releaseTarget {
		add("override_scope", "release_target_override applies only to main or master", "Remove release_target_override for this non-release target.")
	}
	inspectOID := func(ref string) string {
		// rev-parse alone uses DWIM: a missing refs/heads/X can resolve to
		// refs/tags/refs/heads/X. Require the exact local ref on every recheck.
		if strings.HasPrefix(ref, "refs/heads/") {
			args := []string{"show-ref", "--verify", "--hash", ref}
			out, err := run(args...)
			if err != nil || !validMergeOID(out) {
				gitFailure("ref_inspection", "Cannot resolve the exact local branch "+ref, out, err, args...)
				return ""
			}
			ref = out
		}
		args := []string{"rev-parse", "--verify", ref + "^{commit}"}
		out, err := run(args...)
		if err != nil || !validMergeOID(out) {
			gitFailure("ref_inspection", "Cannot resolve a commit OID for "+ref, out, err, args...)
			return ""
		}
		return out
	}
	for i, ref := range []string{branch, mergeRoot} {
		args := []string{"check-ref-format", "refs/heads/" + ref}
		if out, err := run(args...); err != nil {
			gitFailure("invalid_ref", "Invalid local branch ref: "+ref, out, err, args...)
			continue
		}
		oid := inspectOID("refs/heads/" + ref)
		if i == 0 {
			result.SourceOID = oid
		} else {
			result.TargetOID = oid
		}
	}
	checkWorktree := func() {
		args := []string{"status", "--porcelain=v1", "--untracked-files=all"}
		status, err := run(args...)
		if err != nil {
			gitFailure("worktree_inspection", "Cannot inspect worktree and index", status, err, args...)
		} else if status != "" {
			add("dirty_worktree", "git.merge requires a clean worktree and index", "Commit or stash the worktree and index changes, then retry git.merge.")
		}
		args = []string{"rev-parse", "--verify", "--quiet", "MERGE_HEAD"}
		out, err := run(args...)
		var exitErr *exec.ExitError
		if err == nil {
			if validMergeOID(out) {
				add("merge_in_progress", "git.merge refuses an existing merge in progress", "Complete or abort the existing merge, then retry git.merge.")
			} else {
				gitFailure("merge_inspection", "Cannot parse existing merge state", out, nil, args...)
			}
		} else if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			gitFailure("merge_inspection", "Cannot inspect existing merge state", out, err, args...)
		}
	}
	checkWorktree()
	if acknowledgement.ReleaseTargetOverride {
		if !validMergeOID(acknowledgement.ExpectedSourceOID) || !validMergeOID(acknowledgement.ExpectedTargetOID) {
			add("expected_oids_required", "An override requires both full inspected commit OIDs", "Retry with expected_source_oid and expected_target_oid from the inspected refusal.")
		} else {
			if result.SourceOID != "" && result.SourceOID != acknowledgement.ExpectedSourceOID {
				add("source_changed", "Source tip changed since acknowledgement", "Inspect the returned source_oid and request fresh acknowledgement before retrying.")
			}
			if result.TargetOID != "" && result.TargetOID != acknowledgement.ExpectedTargetOID {
				add("target_changed", "Target tip changed since acknowledgement", "Inspect the returned target_oid and request fresh acknowledgement before retrying.")
			}
		}
	}
	if result.SourceOID != "" && result.TargetOID != "" {
		args := []string{"rev-list", "--count", result.TargetOID + ".." + result.SourceOID}
		out, err := run(args...)
		count, parseErr := strconv.ParseUint(out, 10, 64)
		if err != nil || parseErr != nil {
			gitFailure("containment_inspection", "Cannot parse candidate containment", out, err, args...)
		} else if count == 0 {
			add("already_contained", "impl branch is already contained in target; no merge performed", "Inspect the existing integration; no new merge is needed for this candidate.")
		}
		if releaseTarget {
			result.Review = &implMergeReviewEvidence{CandidateRange: result.TargetOID + ".." + result.SourceOID, Note: "Frontier reachability is evidence only; it does not establish review quality or absence of unresolved findings."}
			frontier, found, readErr := wsreview.Frontier(root)
			if readErr != nil {
				result.Review.Note += " Review frontier unavailable: " + readErr.Error()
			} else {
				revisions := []string{result.SourceOID, "^" + result.TargetOID}
				if found {
					result.Review.Frontier = &frontier
					if oid := inspectOID(frontier.Head); oid != "" {
						revisions = append(revisions, "^"+oid)
					} else {
						revisions = nil
					}
				} else {
					result.Review.Note += " No review frontier is available."
				}
				if revisions != nil {
					result.Review.UncoveredRange = strings.Join(revisions, " ")
					args := append([]string{"rev-list", "--count"}, revisions...)
					out, err := run(args...)
					count, parseErr := strconv.ParseUint(out, 10, 64)
					if err != nil || parseErr != nil {
						gitFailure("review_inspection", "Cannot parse uncovered candidate count", out, err, args...)
					} else {
						result.Review.UncoveredCount = &count
					}
				}
			}
		}
	}
	blocked := func() (implMergeResult, error) {
		result.Status = "policy_blocked"
		if releaseTarget {
			return result, nil
		}
		return result, fmt.Errorf("%s", result.Diagnostics[0].Reason)
	}
	if len(result.Diagnostics) > 0 {
		return blocked()
	}
	// Recheck the snapshot immediately before checkout and again before merge:
	// checkout hooks may move either branch while preserving symbolic HEAD.
	checkTips := func() {
		if oid := inspectOID("refs/heads/" + branch); oid != "" && oid != result.SourceOID {
			add("source_changed", "Source tip changed during inspection", "Retry git.merge for a fresh snapshot and acknowledgement.")
		}
		if oid := inspectOID("refs/heads/" + mergeRoot); oid != "" && oid != result.TargetOID {
			add("target_changed", "Target tip changed during inspection", "Retry git.merge for a fresh snapshot and acknowledgement.")
		}
	}
	checkTips()
	if len(result.Diagnostics) > 0 {
		return blocked()
	}
	if _, err := run("switch", "--no-guess", "--", mergeRoot); err != nil {
		return result, err
	}
	checkedOut, err := run("symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return result, err
	}
	if checkedOut != "refs/heads/"+mergeRoot {
		return result, fmt.Errorf("checkout target mismatch: got %q, want %q", checkedOut, mergeRoot)
	}
	checkWorktree()
	checkTips()
	if len(result.Diagnostics) > 0 {
		return blocked()
	}
	if _, err := run("merge", "--no-ff", "--no-squash", "--commit", "-m", wsgit.CommitMessage(message), result.SourceOID); err != nil {
		unmerged, checkErr := run("diff", "--name-only", "--diff-filter=U")
		if checkErr == nil && unmerged != "" {
			result.Status = "conflict"
			result.Advisory = "Merge left in progress on target; use lead-delegate to resolve conflicts and complete the merge record. The impl branch is retained."
			return result, nil
		}
		return result, err
	}
	result.Commit, err = run("rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	result.Status = "merged"
	if _, err := run("branch", "-d", "--", branch); err != nil {
		result.Advisory = "Merge succeeded but branch cleanup failed: " + err.Error()
	} else {
		result.BranchDeleted = true
	}
	return result, nil
}

func validMergeOID(oid string) bool {
	if len(oid) != 40 && len(oid) != 64 {
		return false
	}
	_, err := hex.DecodeString(oid)
	return err == nil
}
