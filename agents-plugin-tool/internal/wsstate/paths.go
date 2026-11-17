package wsstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsexec"
)

const (
	// DefaultCacheDirName names the cache directory under the user's .cache.
	DefaultCacheDirName = "ws@kang-sw-devenv"
	// EnvCacheHome names the cache root override a caller process carries.
	EnvCacheHome  = "WS_CACHE_HOME"
	schemaVersion = 1
)

type Clock func() time.Time

type Options struct {
	CacheHome string
	Now       Clock
	// Env, when set, is the environment the manager's git runs under (and
	// resolves git against); nil means this process's own.
	Env []string
}

type Layout struct {
	CacheRoot   string
	ProjectKey  string
	WorktreeKey string

	ProjectDir  string
	ProjectMeta string
	SharedDir   string
	LocksDir    string

	WorktreesDir     string
	WorktreeDir      string
	WorktreeMeta     string
	AgentsDir        string
	ReviewDir        string
	PromptDir        string
	SessionsDir      string
	WorktreeLocksDir string
	TmpDir           string
}

type ProjectMetadata struct {
	SchemaVersion int    `json:"schema_version"`
	RootPath      string `json:"root_path"`
	RootID        string `json:"root_id"`
	RepoBasename  string `json:"repo_basename"`
	ProjectKey    string `json:"project_key"`
	CreatedAt     string `json:"created_at"`
	LastSeenAt    string `json:"last_seen_at"`
}

type WorktreeMetadata struct {
	SchemaVersion int    `json:"schema_version"`
	RootPath      string `json:"root_path"`
	RootID        string `json:"root_id"`
	ProjectKey    string `json:"project_key"`
	WorktreePath  string `json:"worktree_path"`
	WorktreeID    string `json:"worktree_id"`
	WorktreeName  string `json:"worktree_name"`
	WorktreeKey   string `json:"worktree_key"`
	CreatedAt     string `json:"created_at"`
	LastSeenAt    string `json:"last_seen_at"`
}

type Manager struct {
	opts Options
}

var metadataWriteMu sync.Mutex

func NewManager(opts Options) Manager {
	return Manager{opts: opts}
}

func CacheRoot(opts Options) (string, error) {
	if opts.CacheHome != "" {
		return canonicalCacheHome(opts.CacheHome)
	}
	if env := os.Getenv(EnvCacheHome); env != "" {
		return canonicalCacheHome(env)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cache", DefaultCacheDirName), nil
}

func (m Manager) Resolve(repoPath string) (Layout, ProjectMetadata, WorktreeMetadata, error) {
	root, commonRoot, err := gitIdentity(m.opts.Env, repoPath)
	if err != nil {
		return Layout{}, ProjectMetadata{}, WorktreeMetadata{}, err
	}
	cacheRoot, err := CacheRoot(m.opts)
	if err != nil {
		return Layout{}, ProjectMetadata{}, WorktreeMetadata{}, err
	}

	rootID := shortHash(commonRoot)
	worktreeID := shortHash(root)
	projectKey := rootID
	worktreeKey := projectKey
	if root != commonRoot {
		worktreeKey = projectKey + "@" + worktreeID
	}
	now := m.now().UTC().Format(time.RFC3339)

	layout := layoutFor(cacheRoot, projectKey, worktreeKey)
	project := ProjectMetadata{
		SchemaVersion: schemaVersion,
		RootPath:      commonRoot,
		RootID:        rootID,
		RepoBasename:  filepath.Base(commonRoot),
		ProjectKey:    projectKey,
		CreatedAt:     now,
		LastSeenAt:    now,
	}
	worktree := WorktreeMetadata{
		SchemaVersion: schemaVersion,
		RootPath:      commonRoot,
		RootID:        rootID,
		ProjectKey:    projectKey,
		WorktreePath:  root,
		WorktreeID:    worktreeID,
		WorktreeName:  filepath.Base(root),
		WorktreeKey:   worktreeKey,
		CreatedAt:     now,
		LastSeenAt:    now,
	}
	return layout, project, worktree, nil
}

func (m Manager) Ensure(repoPath string) (Layout, ProjectMetadata, WorktreeMetadata, error) {
	layout, project, worktree, err := m.Resolve(repoPath)
	if err != nil {
		return Layout{}, ProjectMetadata{}, WorktreeMetadata{}, err
	}
	for _, dir := range []string{
		layout.LocksDir,
		layout.AgentsDir,
		layout.ReviewDir,
		layout.PromptDir,
		layout.SessionsDir,
		layout.WorktreeLocksDir,
		layout.TmpDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Layout{}, ProjectMetadata{}, WorktreeMetadata{}, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := upsertJSON(layout.ProjectMeta, &project); err != nil {
		return Layout{}, ProjectMetadata{}, WorktreeMetadata{}, err
	}
	if err := upsertJSON(layout.WorktreeMeta, &worktree); err != nil {
		return Layout{}, ProjectMetadata{}, WorktreeMetadata{}, err
	}
	return layout, project, worktree, nil
}

func (m Manager) now() time.Time {
	if m.opts.Now != nil {
		return m.opts.Now()
	}
	return time.Now()
}

func layoutFor(cacheRoot, projectKey, worktreeKey string) Layout {
	projectDir := filepath.Join(cacheRoot, "proj", projectKey)
	sharedDir := filepath.Join(projectDir, "shared")
	worktreesDir := filepath.Join(cacheRoot, "proj")
	worktreeDir := filepath.Join(cacheRoot, "proj", worktreeKey)
	return Layout{
		CacheRoot:        cacheRoot,
		ProjectKey:       projectKey,
		WorktreeKey:      worktreeKey,
		ProjectDir:       projectDir,
		ProjectMeta:      filepath.Join(projectDir, "project.json"),
		SharedDir:        sharedDir,
		LocksDir:         filepath.Join(sharedDir, "locks"),
		WorktreesDir:     worktreesDir,
		WorktreeDir:      worktreeDir,
		WorktreeMeta:     filepath.Join(worktreeDir, "worktree.json"),
		AgentsDir:        filepath.Join(worktreeDir, "agents"),
		ReviewDir:        filepath.Join(worktreeDir, "review-paths"),
		PromptDir:        filepath.Join(worktreeDir, "prompt-paths"),
		SessionsDir:      filepath.Join(worktreeDir, "sessions"),
		WorktreeLocksDir: filepath.Join(worktreeDir, "locks"),
		TmpDir:           filepath.Join(worktreeDir, "tmp"),
	}
}

func LayoutForWorktreeKey(cacheRoot, worktreeKey string) (Layout, error) {
	if !validWorktreeKey(worktreeKey) {
		return Layout{}, fmt.Errorf("invalid worktree key %q", worktreeKey)
	}
	projectKey := worktreeKey
	if before, _, ok := strings.Cut(worktreeKey, "@"); ok {
		projectKey = before
	}
	return layoutFor(cacheRoot, projectKey, worktreeKey), nil
}

func validWorktreeKey(value string) bool {
	if len(value) != 8 && len(value) != 17 {
		return false
	}
	for i, r := range value {
		if i == 8 && len(value) == 17 {
			if r != '@' {
				return false
			}
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// gitIdentityEntry is one remembered gitIdentity answer for a worktree
// top level, with the stat of its .git entry when the answer was computed.
type gitIdentityEntry struct {
	dotGit                   os.FileInfo
	worktreeRoot, commonRoot string
}

// gitIdentities remembers gitIdentity per top level, so Ensure (called by
// note, mailbox, path and playbook tools) does not spawn two git processes per
// call. An answer is reused only while the top level's .git entry stats as the
// same entry: for a linked worktree's or submodule's .git file, the same file
// (mode, size, mtime, identity), so removing and re-adding a worktree at the
// path invalidates it; for a main checkout's .git directory, the same
// directory identity (a main checkout is its own common root, so only a
// symlinked path retargeted elsewhere could change the answer). Only a top
// level is remembered, never a subdirectory, so a repository later created
// between a subdirectory and its top level cannot be shadowed. The key carries
// the GIT_* environment, which can redirect git.
var gitIdentities sync.Map // gitIdentityKey -> gitIdentityEntry

func gitIdentityKey(env []string, abs string) string {
	if env == nil {
		env = os.Environ()
	}
	var b strings.Builder
	b.WriteString(abs)
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			b.WriteByte(0)
			b.WriteString(kv)
		}
	}
	return b.String()
}

func sameDotGit(a, b os.FileInfo) bool {
	if a.IsDir() || b.IsDir() {
		return a.IsDir() && b.IsDir() && sameFileIdentity(a, b)
	}
	return SameFileMetadata(a, b)
}

func gitIdentity(env []string, repoPath string) (worktreeRoot string, commonRoot string, err error) {
	var key string
	var dotGit os.FileInfo
	if abs, absErr := filepath.Abs(repoPath); absErr == nil && repoPath != "" {
		abs = filepath.Clean(abs)
		key = gitIdentityKey(env, abs)
		// Stat-ed before git runs, so a change racing the spawn is seen by
		// the next call's stat.
		dotGit, _ = Stat(filepath.Join(abs, ".git"))
		if cached, ok := gitIdentities.Load(key); ok {
			entry := cached.(gitIdentityEntry)
			if dotGit != nil && sameDotGit(entry.dotGit, dotGit) {
				return entry.worktreeRoot, entry.commonRoot, nil
			}
			gitIdentities.Delete(key)
		}
	}
	abs, err := canonicalPath(repoPath)
	if err != nil {
		return "", "", err
	}
	worktreeRoot, commonRoot, err = gitIdentityAt(env, abs)
	if err != nil {
		return "", "", err
	}
	// A .git file modified within the racy window could still be rewritten
	// inside its timestamp granule, so it is not remembered yet.
	if dotGit != nil && abs == worktreeRoot && (dotGit.IsDir() || time.Since(dotGit.ModTime()) >= readCacheRacyWindow) {
		gitIdentities.Store(key, gitIdentityEntry{dotGit: dotGit, worktreeRoot: worktreeRoot, commonRoot: commonRoot})
	}
	return worktreeRoot, commonRoot, nil
}

func gitIdentityAt(env []string, abs string) (worktreeRoot string, commonRoot string, err error) {
	root, err := git(env, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	root, err = canonicalPath(root)
	if err != nil {
		return "", "", err
	}
	commonGitDir, err := git(env, abs, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", err
	}
	commonRoot, err = commonRootFromGitDir(commonGitDir)
	if err == nil {
		return root, commonRoot, nil
	}
	// commonRootFromGitDir rejected the git-common-dir. An absorbed submodule's
	// common dir lives under the superproject's .git/modules tree, so it lands
	// here by design; probe for that shape and, when confirmed, treat the
	// submodule as an independent single-worktree project (root == commonRoot).
	// Probing only on this error path is deliberate: every layout that resolves
	// today still takes its original code path, so submodule support cannot
	// reclassify it, and ordinary roots pay no extra git process.
	superprojectRoot, probeErr := git(env, abs, "rev-parse", "--show-superproject-working-tree")
	if probeErr != nil || superprojectRoot == "" {
		// Not a submodule, or the probe itself failed: preserve the original
		// fail-loud error. A worktree created inside a submodule normally lands
		// here, because git reports no superproject for a checkout the parent
		// does not track. The exception is a submodule worktree placed at a path
		// the superproject records as a gitlink: git does report a superproject
		// for it, so it resolves as an independent project instead of failing
		// loud. That is accepted — the superproject genuinely tracks that
		// checkout — but it is not the blanket fail-loud the non-goal implies.
		return "", "", err
	}
	return root, root, nil
}

func commonRootFromGitDir(gitDir string) (string, error) {
	gitDir, err := canonicalPath(gitDir)
	if err != nil {
		return "", err
	}
	if filepath.Base(gitDir) != ".git" {
		return "", fmt.Errorf("unsupported git common dir %q: expected a non-bare .git directory", gitDir)
	}
	return canonicalPath(filepath.Dir(gitDir))
}

func git(env []string, dir string, args ...string) (string, error) {
	cmd := wsexec.Command(env, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func canonicalPath(path string) (string, error) {
	canonical, _, err := canonicalPathResolved(path)
	return canonical, err
}

// canonicalPathResolved is canonicalPath also reporting whether symlink
// evaluation succeeded (false when the path does not exist yet).
func canonicalPathResolved(path string) (string, bool, error) {
	if path == "" {
		return "", false, errors.New("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false, err
	}
	evaluated, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return filepath.Clean(abs), false, nil
	}
	return filepath.Clean(evaluated), true, nil
}

// cacheHomeCanonical memoizes the cache home's symlink resolution for the
// process: every keyed tool call resolves the cache root several times, and
// EvalSymlinks costs one lstat per path component. The cache home is a fixed
// path for the process's lifetime; a resolution is memoized only once the
// path exists, so a home created later is still evaluated.
var cacheHomeCanonical sync.Map // input path -> canonical path

func canonicalCacheHome(path string) (string, error) {
	if cached, ok := cacheHomeCanonical.Load(path); ok {
		return cached.(string), nil
	}
	canonical, resolved, err := canonicalPathResolved(path)
	if err != nil {
		return "", err
	}
	if resolved && filepath.IsAbs(path) {
		cacheHomeCanonical.Store(path, canonical)
	}
	return canonical, nil
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func upsertJSON(path string, next any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metadata dir %s: %w", filepath.Dir(path), err)
	}

	metadataWriteMu.Lock()
	defer metadataWriteMu.Unlock()

	now := ""
	switch v := next.(type) {
	case *ProjectMetadata:
		now = v.LastSeenAt
		if existing, ok := readProjectMetadata(path); ok && existing.CreatedAt != "" {
			v.CreatedAt = existing.CreatedAt
			probe := *v
			probe.LastSeenAt = existing.LastSeenAt
			if probe == existing && lastSeenFresh(existing.LastSeenAt, now) {
				v.LastSeenAt = existing.LastSeenAt
				return nil
			}
		}
	case *WorktreeMetadata:
		now = v.LastSeenAt
		if existing, ok := readWorktreeMetadata(path); ok && existing.CreatedAt != "" {
			v.CreatedAt = existing.CreatedAt
			probe := *v
			probe.LastSeenAt = existing.LastSeenAt
			if probe == existing && lastSeenFresh(existing.LastSeenAt, now) {
				v.LastSeenAt = existing.LastSeenAt
				return nil
			}
		}
	}
	if now == "" {
		return fmt.Errorf("metadata %s has empty last_seen_at", path)
	}

	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata %s: %w", path, err)
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create metadata temp %s: %w", path, err)
	}
	tmp := tmpFile.Name()
	if _, err := tmpFile.Write(append(data, '\n')); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write metadata %s: %w", tmp, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close metadata %s: %w", tmp, err)
	}
	if err := replaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace metadata %s: %w", path, err)
	}
	return nil
}

// lastSeenRefresh is how old a stored last_seen_at may be before an Ensure
// that changes nothing else rewrites the metadata file. Nothing reads
// last_seen_at back (upsertJSON reads only created_at), so the coarser value
// costs no reader, and an Ensure per tool call no longer rewrites two files.
const lastSeenRefresh = 5 * time.Minute

// lastSeenFresh reports whether stored is a parsable last_seen_at at most
// lastSeenRefresh older than now (and not after it).
func lastSeenFresh(stored, now string) bool {
	s, err := time.Parse(time.RFC3339, stored)
	if err != nil {
		return false
	}
	n, err := time.Parse(time.RFC3339, now)
	if err != nil {
		return false
	}
	d := n.Sub(s)
	return d >= 0 && d < lastSeenRefresh
}

func replaceFile(tmp, path string) error {
	return AtomicReplaceFile(tmp, path)
}

func readProjectMetadata(path string) (ProjectMetadata, bool) {
	var data ProjectMetadata
	raw, err := os.ReadFile(path)
	if err != nil {
		return data, false
	}
	return data, json.Unmarshal(raw, &data) == nil
}

func readWorktreeMetadata(path string) (WorktreeMetadata, bool) {
	var data WorktreeMetadata
	raw, err := os.ReadFile(path)
	if err != nil {
		return data, false
	}
	return data, json.Unmarshal(raw, &data) == nil
}
