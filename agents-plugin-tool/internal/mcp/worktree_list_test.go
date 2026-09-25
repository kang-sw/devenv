package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// worktreeListServer builds a fixture repo, an isolated key cache, a server,
// and a lead key bound to the repo root.
func worktreeListServer(t *testing.T) (s *Server, root, base, leadKey string) {
	t.Helper()
	root, base = worktreeFixture(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s = NewServer(root, "test")
	leadKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, root, base, leadKey
}

func acquireForTest(t *testing.T, s *Server, id int, leadKey, base, branch string) worktreeAcquireResult {
	t.Helper()
	resp := callToolOnce(t, s, id, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": base, "target_branch": branch, "format": "json",
	})
	if toolIsError(t, resp) {
		t.Fatalf("acquire %s failed: %s", branch, resp)
	}
	var acq worktreeAcquireResult
	if err := json.Unmarshal([]byte(toolText(t, resp)), &acq); err != nil {
		t.Fatalf("unmarshal acquire: %v\n%s", err, resp)
	}
	return acq
}

func listForTest(t *testing.T, s *Server, id int, key string) worktreeListResult {
	t.Helper()
	resp := callToolOnce(t, s, id, "worktree.list", map[string]any{"session_key": key, "format": "json"})
	if toolIsError(t, resp) {
		t.Fatalf("worktree.list failed: %s", resp)
	}
	var res worktreeListResult
	if err := json.Unmarshal([]byte(toolText(t, resp)), &res); err != nil {
		t.Fatalf("unmarshal list: %v\n%s", err, resp)
	}
	return res
}

func listEntryByPath(t *testing.T, res worktreeListResult, path string) worktreeListEntry {
	t.Helper()
	for _, e := range res.Worktrees {
		if e.Path == path {
			return e
		}
	}
	t.Fatalf("worktree %q not listed: %+v", path, res.Worktrees)
	return worktreeListEntry{}
}

func keyRecordPath(t *testing.T, s *Server, key string) string {
	t.Helper()
	dir, err := s.sessions.keysDir()
	if err != nil {
		t.Fatal(err)
	}
	return s.sessions.keyPath(dir, key)
}

// TestWorktreeListDispatch covers the per-entry facts over a mixed pool: a held
// clean worktree, a held dirty one, a released (detached, lease-less) one, and
// one from before worktree leases existed; the primary root and a foreign
// linked worktree are excluded.
func TestWorktreeListDispatch(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	ctx := context.Background()

	// Pre-change worktree: provisioned without a lease, still branch-held.
	legacy, err := provisionWorktree(ctx, wsgit.ExecRunner{}, root, base, "impl/test/legacy", nil, "")
	if err != nil {
		t.Fatalf("provision legacy: %v", err)
	}
	clean := acquireForTest(t, s, 1, leadKey, base, "impl/test/clean")
	dirty := acquireForTest(t, s, 2, leadKey, base, "impl/test/dirty")
	released := acquireForTest(t, s, 3, leadKey, base, "impl/test/released")
	untracked := acquireForTest(t, s, 8, leadKey, base, "impl/test/untracked")
	handDetached := acquireForTest(t, s, 9, leadKey, base, "impl/test/hand-detached")
	runGit(t, handDetached.Path, "checkout", "--detach")
	if rel := callToolOnce(t, s, 4, "worktree.release", map[string]any{"session_key": leadKey, "path": released.Path}); toolIsError(t, rel) {
		t.Fatalf("release failed: %s", rel)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	runGit(t, root, "worktree", "add", "--detach", foreign, base)

	// Dirty the second worktree: a staged rename (new path newest), a
	// collapsed untracked directory, and a deletion (skipped for mtime).
	runGit(t, dirty.Path, "mv", "f.txt", "renamed.txt")
	if err := os.MkdirAll(filepath.Join(dirty.Path, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirty.Path, "newdir", "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dirty.Path, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	renamedTime := time.Date(2022, 3, 4, 5, 6, 7, 0, time.UTC)
	dirTime := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dirty.Path, "renamed.txt"), renamedTime, renamedTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dirty.Path, "newdir"), dirTime, dirTime); err != nil {
		t.Fatal(err)
	}

	// Untracked-only dirt: a collapsed untracked directory, the only (and so
	// the newest) dirty path. release's clean -ffdx would delete it, so the
	// entry must count as dirty and carry no release nudge.
	if err := os.MkdirAll(filepath.Join(untracked.Path, "scratch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(untracked.Path, "scratch", "notes.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scratchTime := time.Date(2023, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(untracked.Path, "scratch"), scratchTime, scratchTime); err != nil {
		t.Fatal(err)
	}

	res := listForTest(t, s, 5, leadKey)
	if len(res.Worktrees) != 6 {
		t.Fatalf("want exactly the 6 pool worktrees, got %+v", res.Worktrees)
	}
	for i := 1; i < len(res.Worktrees); i++ {
		if res.Worktrees[i-1].Path >= res.Worktrees[i].Path {
			t.Fatalf("entries not ordered by path: %+v", res.Worktrees)
		}
	}
	for _, e := range res.Worktrees {
		if e.Path == canonicalRootForTest(t, root) || e.Path == canonicalRootForTest(t, foreign) {
			t.Fatalf("primary or foreign worktree must be excluded: %q", e.Path)
		}
		if e.HeadCommitTime == "" {
			t.Fatalf("entry %q lacks head_commit_time", e.Path)
		}
	}

	c := listEntryByPath(t, res, clean.Path)
	if c.Branch == nil || *c.Branch != "impl/test/clean" || c.Dirty == nil || *c.Dirty || c.NewestDirtyMtime != "" {
		t.Fatalf("held clean entry facts wrong: %+v", c)
	}
	if c.WorktreeLease == nil || c.WorktreeLease.WorkerKey != clean.WorkerKey || c.WorktreeLease.ParentKey != leadKey || c.WorktreeLease.AcquiredAt == "" {
		t.Fatalf("held clean entry lease wrong: %+v", c.WorktreeLease)
	}
	if c.WorktreeLease.WorkerKeyRecordMtime == nil || c.WorktreeLease.ParentKeyRecordMtime == nil {
		t.Fatalf("live key records must report an mtime: %+v", c.WorktreeLease)
	}
	// The nudge quotes the path as a string literal, so a Windows path's
	// backslashes appear escaped; match the quoted form, not the raw path.
	if !strings.Contains(c.Release, "worktree.release(path: "+strconv.Quote(c.Path)+")") || !strings.Contains(c.Release, "discards uncommitted changes") {
		t.Fatalf("clean entry must carry the release nudge and discard warning: %q", c.Release)
	}

	d := listEntryByPath(t, res, dirty.Path)
	if d.Dirty == nil || !*d.Dirty || d.Release != "" {
		t.Fatalf("dirty entry must be dirty with no release nudge: %+v", d)
	}
	if d.NewestDirtyMtime != renamedTime.Format(time.RFC3339) {
		t.Fatalf("newest_dirty_mtime = %q, want the renamed file's %q", d.NewestDirtyMtime, renamedTime.Format(time.RFC3339))
	}
	if d.WorktreeLease == nil || d.WorktreeLease.WorkerKey != dirty.WorkerKey {
		t.Fatalf("dirty entry lease wrong: %+v", d.WorktreeLease)
	}

	r := listEntryByPath(t, res, released.Path)
	if r.Branch != nil || r.Dirty == nil || *r.Dirty || r.WorktreeLease != nil {
		t.Fatalf("released entry must be detached, clean, with no lease record: %+v", r)
	}

	u := listEntryByPath(t, res, untracked.Path)
	if u.Dirty == nil || !*u.Dirty || u.Release != "" {
		t.Fatalf("untracked-only entry must be dirty with no release nudge: %+v", u)
	}
	if u.NewestDirtyMtime != scratchTime.Format(time.RFC3339) {
		t.Fatalf("newest_dirty_mtime = %q, want the collapsed untracked dir's own %q", u.NewestDirtyMtime, scratchTime.Format(time.RFC3339))
	}

	// Detached by hand while still leased: both facts are shown.
	h := listEntryByPath(t, res, handDetached.Path)
	if h.Branch != nil || h.WorktreeLease == nil || h.WorktreeLease.WorkerKey != handDetached.WorkerKey {
		t.Fatalf("hand-detached entry must show detached and its lease: %+v", h)
	}

	l := listEntryByPath(t, res, legacy.Path)
	if l.Branch == nil || *l.Branch != "impl/test/legacy" || l.WorktreeLease != nil {
		t.Fatalf("pre-lease entry must show its branch and no lease record: %+v", l)
	}

	// JSON encodes the absent facts as null, not omitted.
	raw := toolText(t, callToolOnce(t, s, 6, "worktree.list", map[string]any{"session_key": leadKey, "format": "json"}))
	if !strings.Contains(raw, `"branch":null`) || !strings.Contains(raw, `"worktree_lease":null`) {
		t.Fatalf("json must encode detached branch and missing lease as null: %s", raw)
	}

	// Text form is the default and states the same facts.
	text := toolText(t, callToolOnce(t, s, 7, "worktree.list", map[string]any{"session_key": leadKey}))
	for _, want := range []string{"worktree: " + clean.Path, "branch: (detached)", "state: dirty", "worktree_lease: no lease record", "worker_key " + clean.WorkerKey + " (record mtime ", "release: releasable via worktree.release"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text output lacks %q:\n%s", want, text)
		}
	}
}

func TestWorktreeListDoesNotTouchKeyRecords(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	acq := acquireForTest(t, s, 1, leadKey, base, "impl/test/alpha")
	// Age both reported records past the touch guard; otherwise a touch would
	// be throttled and the assertion would pass trivially.
	workerPath := keyRecordPath(t, s, acq.WorkerKey)
	parentPath := keyRecordPath(t, s, leadKey)
	backdateMtime(t, workerPath, touchGuardWindow+time.Hour)
	backdateMtime(t, parentPath, touchGuardWindow+time.Hour)
	workerBefore, parentBefore := mtimeOf(t, workerPath), mtimeOf(t, parentPath)

	// A second lead key calls list: its own auth lookup legitimately touches
	// the caller's record, but not the records the list reports.
	otherLead, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	res := listForTest(t, s, 2, otherLead)
	e := listEntryByPath(t, res, acq.Path)
	if e.WorktreeLease == nil || e.WorktreeLease.WorkerKeyRecordMtime == nil || *e.WorktreeLease.WorkerKeyRecordMtime != formatFactTime(workerBefore) {
		t.Fatalf("reported worker key mtime must be the aged record's: %+v", e.WorktreeLease)
	}
	if !mtimeOf(t, workerPath).Equal(workerBefore) || !mtimeOf(t, parentPath).Equal(parentBefore) {
		t.Fatal("worktree.list must not refresh the mtime of the key records it reports")
	}
}

func TestWorktreeListReportsPrunedKeyRecordMissing(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	acq := acquireForTest(t, s, 1, leadKey, base, "impl/test/alpha")
	if err := os.Remove(keyRecordPath(t, s, acq.WorkerKey)); err != nil {
		t.Fatal(err)
	}
	otherLead, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	e := listEntryByPath(t, listForTest(t, s, 2, otherLead), acq.Path)
	if e.WorktreeLease == nil || e.WorktreeLease.WorkerKey != acq.WorkerKey || e.WorktreeLease.WorkerKeyRecordMtime != nil {
		t.Fatalf("pruned worker key must be listed with a null record mtime: %+v", e.WorktreeLease)
	}
	if e.WorktreeLease.ParentKeyRecordMtime == nil {
		t.Fatalf("the live parent key must still report an mtime: %+v", e.WorktreeLease)
	}
	text := toolText(t, callToolOnce(t, s, 3, "worktree.list", map[string]any{"session_key": otherLead}))
	if !strings.Contains(text, "worker_key "+acq.WorkerKey+" (record missing)") {
		t.Fatalf("text must report the pruned key as record missing:\n%s", text)
	}
}

func TestWorktreeListIncludesLegacyInTreePoolUnderDefaultConfig(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	legacyPath := filepath.Join(root, ".ws-worktrees", "legacy")
	runGit(t, root, "worktree", "add", "--detach", legacyPath, base)
	res := listForTest(t, s, 1, leadKey)
	e := listEntryByPath(t, res, canonicalRootForTest(t, legacyPath))
	if e.Branch != nil || e.WorktreeLease != nil {
		t.Fatalf("legacy pool entry facts wrong: %+v", e)
	}
	// Symmetric with release: what list shows, release accepts.
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, e.Path, defaultWorktreePoolTemplate); err != nil {
		t.Fatalf("a listed legacy pool worktree must be releasable: %v", err)
	}
}

func TestWorktreeListRejectsNonLead(t *testing.T) {
	s, root, _, leadKey := worktreeListServer(t)
	delegateKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleDelegate, leadKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := callToolOnce(t, s, 1, "worktree.list", map[string]any{"session_key": delegateKey}); !jsonrpcHasError(t, got) {
		t.Fatalf("non-lead worktree.list not rejected: %s", got)
	}
}

func TestPorcelainPathsParsesRenamesAndCollapsedDirs(t *testing.T) {
	root, _ := worktreeFixture(t)
	runGit(t, root, "mv", "f.txt", "with space.txt")
	if err := os.MkdirAll(filepath.Join(root, "untracked", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked", "deep", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := porcelainPaths(context.Background(), wsgit.ExecRunner{}, root)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(paths, "|")
	if got != "with space.txt|untracked/" {
		t.Fatalf("porcelain paths = %q, want the rename's new path and the collapsed untracked dir", got)
	}
}

func TestWorktreeListSchemaRequiresSessionKey(t *testing.T) {
	if !toolSchemaRequiresSessionKey("worktree.list") {
		t.Fatal("toolSchemaRequiresSessionKey(\"worktree.list\") = false, want true")
	}
	s, _, _, _ := worktreeListServer(t)
	listResp := callToolsList(t, s)
	if !toolNameListed(t, listResp, "worktree.list") {
		t.Fatalf("tools/list missing worktree.list: %s", listResp)
	}
	if _, ok := toolPropertiesByName(t, listResp, "worktree.list")["session_key"]; !ok {
		t.Fatal("worktree.list schema missing session_key property")
	}
}
