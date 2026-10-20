package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// Hook points of a worktree profile's lifecycle.
const (
	worktreeHookPostProvision = "post_provision"
	worktreeHookPostAcquire   = "post_acquire"
	worktreeHookPreRelease    = "pre_release"
	worktreeHookPreRemove     = "pre_remove"
)

// worktreeHookTimeout bounds each hook; a hook still running at the deadline
// is killed and counts as failed. A variable only so tests can shorten it.
var worktreeHookTimeout = 10 * time.Minute

// worktreeHookOutputTail caps the captured output a failure reports.
const worktreeHookOutputTail = 4096

// worktreeHookRun is one executed hook's provenance, reported in the
// acquire/release/remove result.
type worktreeHookRun struct {
	Point    string   `json:"point"`
	Argv     []string `json:"argv"`
	Cwd      string   `json:"cwd,omitempty"`
	Scope    string   `json:"scope"`
	ExitCode int      `json:"exit_code"`
}

func (r worktreeHookRun) text() string {
	line := fmt.Sprintf("hook: %s [%s] exit %d: %s", r.Point, r.Scope, r.ExitCode, strings.Join(r.Argv, " "))
	if r.Cwd != "" {
		line += " (cwd " + r.Cwd + ")"
	}
	return line + "\n"
}

// worktreeHookFailure is a failed hook: a non-zero exit, a start failure, or
// the timeout. ExitCode is -1 when the process did not exit on its own.
type worktreeHookFailure struct {
	Run        worktreeHookRun
	OutputTail string
	Cause      error
}

func (f *worktreeHookFailure) Error() string {
	msg := fmt.Sprintf("%s hook %q (%s scope) failed: %v", f.Run.Point, strings.Join(f.Run.Argv, " "), f.Run.Scope, f.Cause)
	if tail := strings.TrimSpace(f.OutputTail); tail != "" {
		msg += "; output tail:\n" + tail
	}
	return msg
}

// worktreeHookEnv is what a hook receives through its environment.
type worktreeHookEnv struct {
	WorktreeDir  string
	Base         string
	TargetBranch string
}

// runWorktreeHooks runs one point's hooks in order, in the worktree (or its
// relative cwd), without a shell, stopping at the first failure. It returns
// every executed hook, the failed one last.
func runWorktreeHooks(ctx context.Context, point string, hooks []worktreeProfileHook, scope wsconfig.Scope, env worktreeHookEnv) ([]worktreeHookRun, *worktreeHookFailure) {
	var runs []worktreeHookRun
	for _, hook := range hooks {
		run := worktreeHookRun{Point: point, Argv: append([]string(nil), hook.Argv...), Scope: string(scope)}
		dir := env.WorktreeDir
		if hook.Cwd != nil && *hook.Cwd != "" {
			run.Cwd = *hook.Cwd
			dir = filepath.Join(env.WorktreeDir, filepath.FromSlash(*hook.Cwd))
		}
		output, cause := execWorktreeHook(ctx, dir, hook.Argv, env, &run)
		runs = append(runs, run)
		if cause != nil {
			return runs, &worktreeHookFailure{Run: run, OutputTail: tailString(output, worktreeHookOutputTail), Cause: cause}
		}
	}
	return runs, nil
}

func execWorktreeHook(ctx context.Context, dir string, argv []string, env worktreeHookEnv, run *worktreeHookRun) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, worktreeHookTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"WS_WORKTREE_DIR="+env.WorktreeDir,
		"WS_BASE="+env.Base,
		"WS_TARGET_BRANCH="+env.TargetBranch,
	)
	// The hook's output is captured, never inherited: this process's stdout
	// is the MCP transport. WaitDelay keeps a grandchild that holds the pipe
	// open from outliving the timeout.
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	run.ExitCode = -1
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		run.ExitCode = cmd.ProcessState.ExitCode()
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		run.ExitCode = -1
		return out.String(), fmt.Errorf("timed out after %s", worktreeHookTimeout)
	case err != nil:
		return out.String(), err
	}
	return out.String(), nil
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// worktreeQuarantine records why a worktree was quarantined. A quarantined
// worktree is never reused; worktree.remove is what clears it.
type worktreeQuarantine struct {
	Point      string   `json:"point"`
	Argv       []string `json:"argv"`
	Scope      string   `json:"scope"`
	ExitCode   int      `json:"exit_code"`
	OutputTail string   `json:"output_tail,omitempty"`
	At         string   `json:"at"`
}

// quarantineWorktree handles a failed post_provision or post_acquire hook:
// release-equivalent hygiene without pre_release (so the target branch is
// freed), the quarantine recorded in the marker, and an error counting the
// profile's quarantined worktrees in the pool.
func quarantineWorktree(ctx context.Context, runner wsgit.Runner, root, poolRoot, wtPath string, profile resolvedWorktreeProfile, failure *worktreeHookFailure) error {
	var notes []string
	if err := releaseHygiene(ctx, runner, wtPath, profile.Definition.Preserve); err != nil {
		notes = append(notes, fmt.Sprintf("returning it to a detached state also failed: %v", err))
	}
	marker, err := readWorktreeMarker(ctx, runner, wtPath)
	if err != nil || marker == nil {
		marker = &worktreeMarker{Profile: profile.Name}
	}
	marker.Quarantine = &worktreeQuarantine{
		Point:      failure.Run.Point,
		Argv:       failure.Run.Argv,
		Scope:      failure.Run.Scope,
		ExitCode:   failure.Run.ExitCode,
		OutputTail: failure.OutputTail,
		At:         time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeWorktreeMarker(ctx, runner, wtPath, *marker); err != nil {
		notes = append(notes, fmt.Sprintf("recording the quarantine also failed: %v", err))
	}
	count := countQuarantined(ctx, runner, root, poolRoot, profile.Name)
	msg := fmt.Sprintf("worktree.acquire: profile %q: %v. Worktree %s is quarantined: it is never reused, and only worktree.remove clears it. %d worktree(s) of profile %q are quarantined", profile.Name, failure, wtPath, count, profile.Name)
	if count >= 2 {
		msg += "; more than one points to a defect in the profile or its hook, not in this worktree"
	}
	msg += ". Fix the profile (config.tune) or the command its hook runs before acquiring it again"
	for _, n := range notes {
		msg += "; " + n
	}
	return errors.New(msg)
}

// countQuarantined counts the pool's quarantined worktrees of one profile.
func countQuarantined(ctx context.Context, runner wsgit.Runner, root, poolRoot, profileName string) int {
	entries, err := listWorktrees(ctx, runner, root)
	if err != nil {
		return 0
	}
	count := 0
	for i, e := range entries {
		if i == 0 || e.Prunable || !pathUnder(poolRoot, e.Path) {
			continue
		}
		if m, err := readWorktreeMarker(ctx, runner, e.Path); err == nil && m != nil && m.Profile == profileName && m.Quarantine != nil {
			count++
		}
	}
	return count
}

// unpreservedIgnoredPaths lists ignored paths git clean would delete despite
// preserve: what `git clean -ndx` would remove minus what `git clean -nd`
// (untracked, not ignored) would. Called right after post_provision, whose
// hygiene has just run, so every path it reports is one the hook created.
func unpreservedIgnoredPaths(ctx context.Context, runner wsgit.Runner, wtPath string, preserve []string) ([]string, error) {
	list := func(withIgnored bool) (map[string]bool, []string, error) {
		args := []string{"clean", "-nd"}
		if withIgnored {
			args = []string{"clean", "-ndx"}
		}
		for _, p := range preserve {
			args = append(args, "-e", p)
		}
		out, err := wtRun(ctx, runner, wtPath, args...)
		if err != nil {
			return nil, nil, err
		}
		set := map[string]bool{}
		var order []string
		for _, line := range strings.Split(out, "\n") {
			if p, ok := strings.CutPrefix(strings.TrimSpace(line), "Would remove "); ok {
				set[p] = true
				order = append(order, p)
			}
		}
		return set, order, nil
	}
	_, all, err := list(true)
	if err != nil {
		return nil, err
	}
	untracked, _, err := list(false)
	if err != nil {
		return nil, err
	}
	var ignored []string
	for _, p := range all {
		if !untracked[p] {
			ignored = append(ignored, p)
		}
	}
	return ignored, nil
}
