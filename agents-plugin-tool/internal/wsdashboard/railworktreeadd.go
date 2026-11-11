package wsdashboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Adding a worktree from the session rail: git worktree add in a workspace
// the rail shows, on a new branch (from a local branch of the repository)
// or on an existing local branch no worktree has checked out. The folder is
// the default one beside the root or one the user names, which must be
// absent or empty. The daemon adds and nothing else: it never removes or
// prunes a worktree, and never deletes or renames a branch. Every name a
// request carries is checked before it reaches git's command line, and none
// can read as an option: a branch is a valid branch name that does not
// start with "-", a base goes in as refs/heads/<base>, and the folder
// follows "--".

const (
	// worktreeAddDir is the folder beside a root that holds its default
	// worktree folders: <root>/../.ws-dashboard-worktrees/<root-dir-name>/.
	worktreeAddDir = ".ws-dashboard-worktrees"
	// worktreeBranchMax bounds a branch name; worktreeBranchList bounds the
	// branches an options read answers.
	worktreeBranchMax  = 255
	worktreeBranchList = 500
)

// worktreeRefusal is why the daemon refused to add a worktree before git
// ran; empty when it did not.
type worktreeRefusal string

const (
	worktreeBadBranch   worktreeRefusal = "badbranch"   // not a valid branch name
	worktreeBranchTaken worktreeRefusal = "exists"      // the new branch exists already
	worktreeNoBranch    worktreeRefusal = "nobranch"    // no such local branch
	worktreeCheckedOut  worktreeRefusal = "checkedout"  // a worktree has the branch checked out
	worktreeNoBase      worktreeRefusal = "nobase"      // the base is no local branch
	worktreeBadFolder   worktreeRefusal = "badfolder"   // the folder was refused (see pathRefusal)
	worktreeFolderInUse worktreeRefusal = "folderinuse" // the folder exists and is not empty
)

// worktreeOptions is what the add-worktree form offers.
type worktreeOptions struct {
	// Current is the root's checked-out branch, empty on a detached HEAD.
	Current string `json:"current"`
	// Bases are the local branches a new branch can start from, the current
	// one first; Free are those no worktree has checked out.
	Bases []string `json:"bases"`
	Free  []string `json:"free"`
	// Folder is the default folder a worktree added now would take.
	Folder string `json:"folder"`
}

// worktreeRequest is one add.
type worktreeRequest struct {
	// Branch is the branch the worktree checks out; New creates it from
	// Base (the root's current branch when empty).
	Branch string
	New    bool
	Base   string
	// Folder is the worktree's folder, already read as a path; empty takes
	// the default.
	Folder string
}

// gitOutput runs a git read of an add in root and returns its trimmed
// output.
func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := gitActionCommand(ctx, root, args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("timed out after %s", gitActionTimeout)
		}
		stderr := ""
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			stderr = string(exit.Stderr)
		}
		return "", fmt.Errorf("%s", gitActionErrText(stderr, err))
	}
	return strings.TrimSpace(string(out)), nil
}

// readWorktreeOptions reads root's branches for the add-worktree form.
func readWorktreeOptions(ctx context.Context, root string, now time.Time) (worktreeOptions, error) {
	opts := worktreeOptions{Bases: []string{}, Free: []string{}, Folder: defaultWorktreeFolder(root, now, pathExists)}
	// A detached HEAD fails the read and leaves no current branch.
	opts.Current, _ = gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	refs, err := gitOutput(ctx, root, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return opts, err
	}
	held, err := checkedOutBranches(ctx, root)
	if err != nil {
		return opts, err
	}
	if opts.Current != "" {
		opts.Bases = append(opts.Bases, opts.Current)
	}
	for _, b := range strings.Split(refs, "\n") {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if b != opts.Current && len(opts.Bases) < worktreeBranchList {
			opts.Bases = append(opts.Bases, b)
		}
		if !held[b] && len(opts.Free) < worktreeBranchList {
			opts.Free = append(opts.Free, b)
		}
	}
	return opts, nil
}

// checkedOutBranches are the branches root's worktrees have checked out,
// the root's own among them.
func checkedOutBranches(ctx context.Context, root string) (map[string]bool, error) {
	out, err := gitOutput(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if b, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "branch refs/heads/"); ok {
			held[strings.TrimSpace(b)] = true
		}
	}
	return held, nil
}

// branchExists reports a local branch of root.
func branchExists(ctx context.Context, root, branch string) bool {
	_, err := gitOutput(ctx, root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
	return err == nil
}

// validBranchName reports a name git takes as a branch name and that
// cannot read as an option.
func validBranchName(ctx context.Context, root, name string) bool {
	if name == "" || len(name) > worktreeBranchMax || strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\x00\r\n") {
		return false
	}
	out, err := gitOutput(ctx, root, "check-ref-format", "--branch", name)
	// git expands a previous-checkout form (@{-1}) to another name; only a
	// name it echoes unchanged is the branch asked for.
	return err == nil && out == name
}

// defaultWorktreeFolder is the folder a worktree added to root at now
// takes: <root>/../.ws-dashboard-worktrees/<root-dir-name>/<YYMMDD-HHMM>,
// with -2, -3, ... appended while a folder of that name exists.
func defaultWorktreeFolder(root string, now time.Time, exists func(string) bool) string {
	root = filepath.Clean(root)
	base := filepath.Join(filepath.Dir(root), worktreeAddDir, filepath.Base(root), now.Format("060102-1504"))
	path := base
	for n := 2; exists(path); n++ {
		path = base + "-" + strconv.Itoa(n)
	}
	return path
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// emptyOrAbsent reports a path a new worktree can take: nothing there, or
// an empty folder.
func emptyOrAbsent(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return !pathExists(path)
	}
	return len(entries) == 0
}

// addWorktree adds a worktree to root as req asks, at now. A refusal comes
// with a nil error and names the reason; err is git's failure, its message
// git's first meaningful line. On success it returns the worktree's folder.
func addWorktree(ctx context.Context, root string, req worktreeRequest, now time.Time) (folder string, refusal worktreeRefusal, err error) {
	if !validBranchName(ctx, root, req.Branch) {
		return "", worktreeBadBranch, nil
	}
	base := "HEAD"
	if req.New {
		if branchExists(ctx, root, req.Branch) {
			return "", worktreeBranchTaken, nil
		}
		if req.Base != "" {
			if !validBranchName(ctx, root, req.Base) || !branchExists(ctx, root, req.Base) {
				return "", worktreeNoBase, nil
			}
			base = "refs/heads/" + req.Base
		}
	} else {
		if !branchExists(ctx, root, req.Branch) {
			return "", worktreeNoBranch, nil
		}
		held, err := checkedOutBranches(ctx, root)
		if err != nil {
			return "", "", err
		}
		if held[req.Branch] {
			return "", worktreeCheckedOut, nil
		}
	}
	folder = req.Folder
	if folder == "" {
		folder = defaultWorktreeFolder(root, now, pathExists)
	} else if !filepath.IsAbs(folder) {
		return "", worktreeBadFolder, nil
	} else if folder = filepath.Clean(folder); !emptyOrAbsent(folder) {
		return "", worktreeFolderInUse, nil
	}
	// An existing branch goes in by its short name: git checks out a branch
	// named so, and detaches HEAD at a full ref.
	args := []string{"worktree", "add", "--", folder, req.Branch}
	if req.New {
		args = []string{"worktree", "add", "-b", req.Branch, "--", folder, base}
	}
	if err := runGitAction(ctx, root, args...); err != nil {
		return "", "", err
	}
	return folder, "", nil
}
