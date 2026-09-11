package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

type implMergeResult struct {
	Branch        string `json:"branch"`
	Target        string `json:"target"`
	Status        string `json:"status"`
	Commit        string `json:"commit,omitempty"`
	BranchDeleted bool   `json:"branch_deleted"`
	Advisory      string `json:"advisory,omitempty"`
}

func (r implMergeResult) text() string {
	return fmt.Sprintf("status: %s\nbranch: %s\ntarget: %s\ncommit: %s\nbranch_deleted: %t\nadvisory: %s\n", r.Status, r.Branch, r.Target, r.Commit, r.BranchDeleted, r.Advisory)
}

func mergeImplBranch(ctx context.Context, root string, runner wsgit.Runner, branch, target string, message wsgit.CommitOptions) (implMergeResult, error) {
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
	if target != "" && target != mergeRoot {
		return result, fmt.Errorf("target %q does not match encoded root %q", target, mergeRoot)
	}
	if mergeRoot == "main" || mergeRoot == "master" {
		return result, fmt.Errorf("forbidden merge target %q", mergeRoot)
	}
	result.Branch, result.Target = branch, mergeRoot
	for _, ref := range []string{branch, mergeRoot} {
		if _, err := run("check-ref-format", "refs/heads/"+ref); err != nil {
			return result, err
		}
		if _, err := run("show-ref", "--verify", "refs/heads/"+ref); err != nil {
			return result, err
		}
	}
	status, err := run("status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return result, err
	}
	if status != "" {
		return result, fmt.Errorf("git.merge requires a clean worktree and index")
	}
	if _, err := run("rev-parse", "--verify", "MERGE_HEAD"); err == nil {
		return result, fmt.Errorf("git.merge refuses an existing merge in progress")
	}
	source, err := run("rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		return result, err
	}
	// An already-contained branch cannot produce a merge commit. Refuse it
	// before checkout and retain its ref for the lead to inspect.
	count, err := run("rev-list", "--count", "refs/heads/"+mergeRoot+".."+source)
	if err != nil {
		return result, err
	}
	if count == "0" {
		return result, fmt.Errorf("impl branch is already contained in target; no merge performed")
	}
	if _, err := run("switch", "--no-guess", mergeRoot); err != nil {
		return result, err
	}
	if _, err := run("merge", "--no-ff", "--no-squash", "--commit", "-m", wsgit.CommitMessage(message), source); err != nil {
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
