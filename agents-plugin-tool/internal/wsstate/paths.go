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
	"time"
)

const (
	defaultCacheDirName = "ws@kang-sw-devenv"
	envCacheHome        = "WS_CACHE_HOME"
	schemaVersion       = 1
)

type Clock func() time.Time

type Options struct {
	CacheHome string
	Now       Clock
}

type Layout struct {
	CacheRoot   string
	ProjectKey  string
	WorktreeKey string

	ProjectDir  string
	ProjectMeta string
	SharedDir   string
	LocksDir    string

	WorktreesDir string
	WorktreeDir  string
	WorktreeMeta string
	AgentsDir    string
	ReviewDir    string
	SessionsDir  string
	TmpDir       string
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

func NewManager(opts Options) Manager {
	return Manager{opts: opts}
}

func CacheRoot(opts Options) (string, error) {
	if opts.CacheHome != "" {
		return canonicalPath(opts.CacheHome)
	}
	if env := os.Getenv(envCacheHome); env != "" {
		return canonicalPath(env)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cache", defaultCacheDirName), nil
}

func (m Manager) Resolve(repoPath string) (Layout, ProjectMetadata, WorktreeMetadata, error) {
	root, commonRoot, err := gitIdentity(repoPath)
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
		layout.SessionsDir,
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
		CacheRoot:    cacheRoot,
		ProjectKey:   projectKey,
		WorktreeKey:  worktreeKey,
		ProjectDir:   projectDir,
		ProjectMeta:  filepath.Join(projectDir, "project.json"),
		SharedDir:    sharedDir,
		LocksDir:     filepath.Join(sharedDir, "locks"),
		WorktreesDir: worktreesDir,
		WorktreeDir:  worktreeDir,
		WorktreeMeta: filepath.Join(worktreeDir, "worktree.json"),
		AgentsDir:    filepath.Join(worktreeDir, "agents"),
		ReviewDir:    filepath.Join(worktreeDir, "review-paths"),
		SessionsDir:  filepath.Join(worktreeDir, "sessions"),
		TmpDir:       filepath.Join(worktreeDir, "tmp"),
	}
}

func gitIdentity(repoPath string) (worktreeRoot string, commonRoot string, err error) {
	abs, err := canonicalPath(repoPath)
	if err != nil {
		return "", "", err
	}
	root, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	commonGitDir, err := git(abs, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", err
	}
	root, err = canonicalPath(root)
	if err != nil {
		return "", "", err
	}
	commonRoot, err = commonRootFromGitDir(commonGitDir)
	if err != nil {
		return "", "", err
	}
	return root, commonRoot, nil
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

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
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
	if path == "" {
		return "", errors.New("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
		abs = evaluated
	}
	return filepath.Clean(abs), nil
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func upsertJSON(path string, next any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metadata dir %s: %w", filepath.Dir(path), err)
	}

	now := ""
	switch v := next.(type) {
	case *ProjectMetadata:
		now = v.LastSeenAt
		if existing, ok := readProjectMetadata(path); ok && existing.CreatedAt != "" {
			v.CreatedAt = existing.CreatedAt
		}
	case *WorktreeMetadata:
		now = v.LastSeenAt
		if existing, ok := readWorktreeMetadata(path); ok && existing.CreatedAt != "" {
			v.CreatedAt = existing.CreatedAt
		}
	}
	if now == "" {
		return fmt.Errorf("metadata %s has empty last_seen_at", path)
	}

	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata %s: %w", path, err)
	}
	tmp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write metadata %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace metadata %s: %w", path, err)
	}
	return nil
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
