package wsindex

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// refOID resolves ref to its commit, reporting absence as ("", false, nil).
func (c *Client) refOID(ctx context.Context, ref string) (string, bool, error) {
	out, err := c.git(ctx, nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		var gitErr *GitError
		var exitErr *exec.ExitError
		if errors.As(err, &gitErr) && errors.As(gitErr.Err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	if out == "" {
		return "", false, nil
	}
	return out, true, nil
}

// readBlobAt reads path from commit's tree.
func (c *Client) readBlobAt(ctx context.Context, commit, path string) ([]byte, error) {
	out, err := c.runner.Run(ctx, c.root, Command{Args: []string{"cat-file", "blob", commit + ":" + path}, Env: localEnv})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readIndexAt decodes the index stored in commit.
func (c *Client) readIndexAt(ctx context.Context, commit string) (*Index, error) {
	data, err := c.readBlobAt(ctx, commit, indexFile)
	if err != nil {
		return nil, fmt.Errorf("read ticket index at %s: %w", commit, err)
	}
	return DecodeIndex(data)
}

// writeSingleFileCommit stores data as the only file of a new tree and
// commits it with the given parent ("" for a root commit). Plumbing only: no
// checkout, no worktree, no index file.
func (c *Client) writeSingleFileCommit(ctx context.Context, name string, data []byte, parent, message string) (string, error) {
	blob, err := c.git(ctx, data, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	tree, err := c.git(ctx, []byte(fmt.Sprintf("100644 blob %s\t%s\n", blob, name)), "mktree")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	return c.git(ctx, []byte(message), args...)
}

// writeIndexCommit stores idx as a new index version whose parent is the
// previous tip, preserving the audit chain.
func (c *Client) writeIndexCommit(ctx context.Context, idx *Index, parent, message string) (string, error) {
	data, err := EncodeIndex(idx)
	if err != nil {
		return "", err
	}
	return c.writeSingleFileCommit(ctx, indexFile, data, parent, message)
}

// casRef applies one local compare-and-swap ref update. old "" means the ref
// must not exist; next "" deletes it. A false return is a CAS miss: the ref
// changed under the caller, who re-reads and retries.
func (c *Client) casRef(ctx context.Context, ref, next, old string) (bool, error) {
	var line string
	switch {
	case next == "" && old == "":
		return true, nil
	case next == "":
		line = fmt.Sprintf("delete %s %s\n", ref, old)
	case old == "":
		line = fmt.Sprintf("create %s %s\n", ref, next)
	default:
		line = fmt.Sprintf("update %s %s %s\n", ref, next, old)
	}
	if _, err := c.git(ctx, []byte(line), "update-ref", "--stdin"); err != nil {
		var gitErr *GitError
		if errors.As(err, &gitErr) && isRefLockMiss(gitErr.Stderr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isRefLockMiss(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "cannot lock ref") ||
		strings.Contains(s, "reference already exists") ||
		strings.Contains(s, "but expected") ||
		strings.Contains(s, "unable to resolve reference") ||
		strings.Contains(s, "is at")
}

// isAncestor reports whether a is an ancestor of (or equal to) b.
func (c *Client) isAncestor(ctx context.Context, a, b string) (bool, error) {
	_, err := c.git(ctx, nil, "merge-base", "--is-ancestor", a, b)
	if err == nil {
		return true, nil
	}
	var gitErr *GitError
	var exitErr *exec.ExitError
	if errors.As(err, &gitErr) && errors.As(gitErr.Err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}
