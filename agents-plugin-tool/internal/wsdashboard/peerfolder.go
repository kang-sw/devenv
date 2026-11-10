package wsdashboard

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// worktreeMains memoizes worktreeMain per peer path: whether a folder is a
// linked worktree does not change while the daemon runs, and the peer group
// is rebuilt on every render, so each distinct path reads its .git once.
// Bounded by the number of distinct peer paths the daemon sees.
var worktreeMains sync.Map // path -> string

// worktreeMain is the main repository root of path when path is a git linked
// worktree (its .git is a file whose gitdir is <root>/.git/worktrees/<name>),
// else "". A plain repository (.git directory), a submodule (gitdir under
// .git/modules), a bare-repository worktree, and a missing or unreadable path
// (a Windows path seen from another OS included) all yield "". Memoized; git
// itself is never run.
func worktreeMain(path string) string {
	if path == "" {
		return ""
	}
	if v, ok := worktreeMains.Load(path); ok {
		return v.(string)
	}
	root := readWorktreeMain(path)
	worktreeMains.Store(path, root)
	return root
}

func readWorktreeMain(path string) string {
	dir := filepath.Clean(path)
	data, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil { // a .git directory reads as an error too: a plain repo
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return ""
	}
	gitdir = strings.TrimSpace(gitdir)
	if gitdir == "" {
		return ""
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}
	gitdir = filepath.Clean(gitdir)
	if filepath.Base(filepath.Dir(gitdir)) != "worktrees" {
		return "" // a submodule's gitdir lives under .git/modules
	}
	common := filepath.Dir(filepath.Dir(gitdir))
	if b, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
		if c := strings.TrimSpace(string(b)); c != "" {
			if !filepath.IsAbs(c) {
				c = filepath.Join(gitdir, c)
			}
			common = filepath.Clean(c)
		}
	}
	if filepath.Base(common) != ".git" {
		return "" // a bare repository or a submodule's own worktree
	}
	return filepath.Dir(common)
}
