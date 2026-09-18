package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// implRootRepo builds a repo with a base commit, a `develop` branch, and leaves
// HEAD on `impl/develop/foo` — the occupied-root state a lead sees while a
// worker holds the impl branch in the shared root.
func implRootRepo(t *testing.T, headBranch string) string {
	t.Helper()
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "f.txt", "base\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	runGit(t, root, "branch", "develop")
	runGit(t, root, "switch", "-c", headBranch)
	return root
}

func TestOccupiedRootAnnouncementTextVariants(t *testing.T) {
	// impl/<parent>/<stem>: names the parent as a quoted base.
	withParent := implRootRepo(t, "impl/develop/foo")
	got := occupiedRootAnnouncement(withParent)
	if !strings.Contains(got, "Occupied root.") {
		t.Fatalf("impl/develop/foo must produce the banner: %q", got)
	}
	if !strings.Contains(got, `base: "develop"`) {
		t.Fatalf("banner must name the parent as base: \"develop\": %q", got)
	}
	if !strings.Contains(got, "impl/develop/foo") {
		t.Fatalf("banner must name the current branch: %q", got)
	}

	// Rootless impl/<stem>: no parent segment, so no quoted base.
	rootless := implRootRepo(t, "impl/foo")
	gotRootless := occupiedRootAnnouncement(rootless)
	if !strings.Contains(gotRootless, "Occupied root.") {
		t.Fatalf("impl/foo must produce the banner: %q", gotRootless)
	}
	if strings.Contains(gotRootless, `base: "`) {
		t.Fatalf("rootless banner must not name a quoted base: %q", gotRootless)
	}
	if !strings.Contains(gotRootless, "the branch this worker was spawned from") {
		t.Fatalf("rootless banner must fall back to the spawned-from phrasing: %q", gotRootless)
	}

	// A non-impl branch is not an occupied root.
	onDevelop := implRootRepo(t, "impl/develop/foo")
	runGit(t, onDevelop, "switch", "develop")
	if got := occupiedRootAnnouncement(onDevelop); got != "" {
		t.Fatalf("non-impl branch must yield no banner: %q", got)
	}

	// Detached HEAD yields no banner (git symbolic-ref --quiet exits non-zero).
	detached := implRootRepo(t, "impl/develop/foo")
	runGit(t, detached, "checkout", "--detach")
	if got := occupiedRootAnnouncement(detached); got != "" {
		t.Fatalf("detached HEAD must yield no banner: %q", got)
	}
}

// TestOccupiedRootAnnouncementInjection covers the two workflow_manual paths and
// the parent-gating: a top-level lead key (empty parent) gets the banner on both
// FRESH-with-root and CONTINUE, while a key with a non-empty parent (a worker's
// own key) is silent even though the root's HEAD is on impl/*.
func TestOccupiedRootAnnouncementInjection(t *testing.T) {
	useLeadProfile(t)
	root := implRootRepo(t, "impl/develop/foo")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{"root": root})
	if !strings.Contains(freshResp, "Occupied root.") || !strings.Contains(freshResp, `base: "develop"`) {
		t.Fatalf("FRESH-with-root must carry the occupied-root banner naming base develop: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueResp, "Occupied root.") || !strings.Contains(continueResp, `base: "develop"`) {
		t.Fatalf("CONTINUE with a top-level lead key must carry the occupied-root banner: %s", continueResp)
	}

	// A key with a non-empty parent (a worker's own key) must be silent even
	// though the root's HEAD is on impl/*: the branch's owner would otherwise be
	// told "do not commit here" about its own branch.
	canonRoot := canonicalRootForTest(t, root)
	parentKey, err := s.sessions.mint(canonRoot, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	childKey, err := s.sessions.mint(canonRoot, roleLead, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	childResp := callToolWithKey(t, s, 4, childKey, "workflow_manual", nil)
	if strings.Contains(childResp, "Occupied root.") {
		t.Fatalf("CONTINUE with a non-empty-parent key must stay silent: %s", childResp)
	}
}
