package wsdashboard

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// The stored workspaces: the folders a user added to the session rail, kept
// whether or not a session runs in them. A workspace is a repository's root
// worktree or a plain folder that is in no repository; a linked worktree is
// never one, it shows under its root. The list is user-curated, so it lives
// in the config home (DaemonOptions.WorkspacesFile), where clearing the
// cache does not lose it, as one JSON file replaced atomically. The daemon
// only ever adds an entry, rewrites one to its repository root, or removes
// one; it never touches a workspace's files.

const workspacesSchemaVersion = 1

// WorkspacesFilePath is the stored workspace list's file for the global
// config file at configFile: dashboard/workspaces.json beside it. Empty for
// an empty configFile, which keeps no stored workspace.
func WorkspacesFilePath(configFile string) string {
	if configFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(configFile), "dashboard", "workspaces.json")
}

type workspacesDoc struct {
	Version    int               `json:"version"`
	Workspaces []workspaceRecord `json:"workspaces"`
}

type workspaceRecord struct {
	Path string `json:"path"`
}

// storedWorkspace is one stored workspace as the rail reads it.
type storedWorkspace struct {
	Path string
	// Repo reports a repository root (Path holds a .git directory), as of
	// the store's last look.
	Repo bool
}

// workspaceStore holds the stored workspaces. Every change re-reads the
// file first, so a list another process wrote is not lost; the rail reads
// the copy in memory.
type workspaceStore struct {
	file string

	mu   sync.Mutex
	list []storedWorkspace
}

// openWorkspaceStore reads the stored list at file; nil for an empty file
// path, which serves no stored workspace. A missing or unreadable file
// reads as an empty list.
func openWorkspaceStore(file string) *workspaceStore {
	if file == "" {
		return nil
	}
	s := &workspaceStore{file: file}
	s.mu.Lock()
	_ = s.reload()
	s.mu.Unlock()
	return s
}

// reload reads the file into memory; mu is held. A missing file is an empty
// list; a malformed one is an error and leaves the list in memory as it
// was, so a change is refused rather than written over it.
func (s *workspaceStore) reload() error {
	raw, err := os.ReadFile(s.file)
	if errors.Is(err, fs.ErrNotExist) {
		s.list = nil
		return nil
	}
	if err != nil {
		return err
	}
	var doc workspacesDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	var list []storedWorkspace
	seen := map[string]bool{}
	for _, rec := range doc.Workspaces {
		if rec.Path == "" || !filepath.IsAbs(rec.Path) {
			continue
		}
		p := filepath.Clean(rec.Path)
		if seen[p] {
			continue
		}
		seen[p] = true
		list = append(list, storedWorkspace{Path: p, Repo: holdsGitDir(p)})
	}
	s.list = list
	return nil
}

// save writes the list in memory; mu is held.
func (s *workspaceStore) save() error {
	doc := workspacesDoc{Version: workspacesSchemaVersion, Workspaces: []workspaceRecord{}}
	for _, w := range s.list {
		doc.Workspaces = append(doc.Workspaces, workspaceRecord{Path: w.Path})
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writePrivateFile(dir, filepath.Base(s.file), append(raw, '\n'))
}

// workspaces returns the stored list as last read.
func (s *workspaceStore) workspaces() []storedWorkspace {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]storedWorkspace(nil), s.list...)
}

func (s *workspaceStore) index(path string) int {
	for i, w := range s.list {
		if w.Path == path {
			return i
		}
	}
	return -1
}

// workspaceResult is how an add ended.
type workspaceResult string

const (
	// workspaceAdded: the root was added to the list.
	workspaceAdded workspaceResult = "added"
	// workspaceKnown: the root is in the list already, so nothing changed.
	workspaceKnown workspaceResult = "noop"
	// workspaceMerged: the pasted folder was in the list and is a linked
	// worktree of the root; its entry now names the root.
	workspaceMerged workspaceResult = "merged"
)

// workspaceAdd is an add's outcome, or a preview's prediction of it.
type workspaceAdd struct {
	Result workspaceResult
	// Path is the folder asked for; Root the workspace it resolved to, which
	// differs when Path is inside a repository and is not its root.
	Path, Root string
	// Repo reports a repository root.
	Repo bool
	// Worktree is the linked worktree Path sits in, empty when it sits in
	// none: Root is then that worktree's repository.
	Worktree string
}

// add adds the workspace dir resolves to. With dry set it changes nothing
// and reports what an add would do.
func (s *workspaceStore) add(dir string, dry bool) (workspaceAdd, error) {
	loc := locateWorkspace(dir)
	out := workspaceAdd{Path: filepath.Clean(dir), Root: loc.Root, Repo: loc.Repo, Worktree: loc.Worktree}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reload(); err != nil {
		return out, err
	}
	changed := s.mergeLinkedLocked(loc.Worktree)
	switch {
	case loc.Worktree != "" && s.index(loc.Worktree) >= 0:
		out.Result = workspaceMerged
		s.rewriteLocked(loc.Worktree, loc.Root)
		changed = true
	case s.index(loc.Root) >= 0:
		out.Result = workspaceKnown
	default:
		out.Result = workspaceAdded
		s.list = append(s.list, storedWorkspace{Path: loc.Root, Repo: loc.Repo})
		changed = true
	}
	if dry {
		// The list in memory took the changes; read the file's back.
		return out, s.reload()
	}
	if !changed {
		return out, nil
	}
	if err := s.save(); err != nil {
		_ = s.reload()
		return out, err
	}
	return out, nil
}

// forget removes path from the list; false when it is not in it.
func (s *workspaceStore) forget(path string) (bool, error) {
	path = filepath.Clean(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reload(); err != nil {
		return false, err
	}
	i := s.index(path)
	if i < 0 {
		return false, nil
	}
	s.list = append(s.list[:i], s.list[i+1:]...)
	if err := s.save(); err != nil {
		_ = s.reload()
		return false, err
	}
	return true, nil
}

// sweep looks at every stored workspace again: one that became a linked
// worktree of another root is rewritten to that root, and each entry's
// repository mark follows its folder. It reports whether anything the rail
// shows changed.
func (s *workspaceStore) sweep() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := append([]storedWorkspace(nil), s.list...)
	if err := s.reload(); err != nil {
		return false
	}
	if s.mergeLinkedLocked("") {
		if err := s.save(); err != nil {
			_ = s.reload()
		}
	}
	if len(before) != len(s.list) {
		return true
	}
	for i := range before {
		if before[i] != s.list[i] {
			return true
		}
	}
	return false
}

// mergeLinkedLocked rewrites each stored entry that is a linked worktree to
// its root, except the entry skip (an add reports that one itself); mu is
// held. It reports whether the list changed.
func (s *workspaceStore) mergeLinkedLocked(skip string) bool {
	changed := false
	for _, w := range append([]storedWorkspace(nil), s.list...) {
		if w.Path == skip {
			continue
		}
		if root := readWorktreeMain(w.Path); root != "" && root != w.Path {
			s.rewriteLocked(w.Path, root)
			changed = true
		}
	}
	return changed
}

// rewriteLocked makes the entry from name root to, in from's place; when to
// is in the list already, from is only removed. mu is held.
func (s *workspaceStore) rewriteLocked(from, to string) {
	i := s.index(from)
	if i < 0 {
		return
	}
	if s.index(to) >= 0 {
		s.list = append(s.list[:i], s.list[i+1:]...)
		return
	}
	s.list[i] = storedWorkspace{Path: to, Repo: holdsGitDir(to)}
}

// workspaceLocation is the workspace a folder belongs to.
type workspaceLocation struct {
	// Root is the workspace: the repository's root worktree when the folder
	// is inside a repository, else the folder itself.
	Root string
	Repo bool
	// Worktree is the linked worktree the folder sits in, empty for none.
	Worktree string
}

// locateWorkspace finds the workspace of dir without running git, by the
// nearest .git at or above it: a .git directory marks a repository's root
// worktree; a .git file whose gitdir is a linked worktree's
// (readWorktreeMain) leads to that worktree's root. Any other .git (a
// submodule's, a bare repository's worktree) leaves dir a plain folder, as
// the rail shows a session rooted there (railRepoRoot).
func locateWorkspace(dir string) workspaceLocation {
	dir = filepath.Clean(dir)
	for d := dir; ; {
		if info, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			if info.IsDir() {
				return workspaceLocation{Root: d, Repo: true}
			}
			if root := readWorktreeMain(d); root != "" {
				return workspaceLocation{Root: root, Repo: true, Worktree: d}
			}
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return workspaceLocation{Root: dir}
}

// holdsGitDir reports a folder with a .git directory: a repository's root
// worktree.
func holdsGitDir(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}
