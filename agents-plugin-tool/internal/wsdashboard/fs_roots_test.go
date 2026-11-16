package wsdashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

type fsRootsSource struct {
	calls atomic.Int32
	read  func(string, []Registration) (RepoWorktrees, error)
}

func (s *fsRootsSource) ReadWorktrees(Registration) (WorktreePool, error) {
	return WorktreePool{}, nil
}

func (s *fsRootsSource) ReadRepoWorktrees(repo string, regs []Registration) (RepoWorktrees, error) {
	s.calls.Add(1)
	return s.read(repo, regs)
}

func TestFSRootsSessionFirstAndWorktreeFields(t *testing.T) {
	src := &fsRootsSource{}
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) { o.Worktrees = src })
	local, _ := fsViewers(t, f)
	primary, detached, prunable := t.TempDir(), t.TempDir(), t.TempDir()
	missing := filepath.Join(t.TempDir(), "removed")
	src.read = func(repo string, regs []Registration) (RepoWorktrees, error) {
		if repo != f.project || len(regs) != 1 || regs[0].ReplyID != "livesession" || regs[0].Project != f.project {
			t.Errorf("enumeration identity = %q %+v", repo, regs)
		}
		return RepoWorktrees{Pools: []string{"POOL-MUST-NOT-LEAK"}, Worktrees: []RepoWorktree{
			{Path: primary, Branch: "main"},
			{Path: f.project, Branch: "feature"},
			{Path: detached, Detached: true},
			{Path: filepath.Join(primary, "."), Branch: "duplicate"},
			{Path: missing}, {Path: prunable, Prunable: true}, {Path: "relative/path"},
		}}, nil
	}
	// No rail stream has ever opened. An alternate root parameter must not
	// change which session repository supplies the suggestions.
	rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/worktrees?root="+url.QueryEscape(detached), "")
	var a fsRootsAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &a); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("roots = %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	want := []fsRootChoice{
		{Path: filepath.ToSlash(f.project), Branch: "feature"},
		{Path: filepath.ToSlash(primary), Branch: "main"},
		{Path: filepath.ToSlash(detached), Detached: true},
	}
	if !reflect.DeepEqual(a.Roots, want) || src.calls.Load() != 1 {
		t.Fatalf("roots = %+v (%d reads), want %+v", a.Roots, src.calls.Load(), want)
	}
	if strings.Contains(rec.Body.String(), "POOL-MUST-NOT-LEAK") || !strings.Contains(rec.Body.String(), `"detached":false`) ||
		rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Errorf("unexpected JSON/headers: %s %v", rec.Body.String(), rec.Header())
	}
}

func TestFSRootsPooledField(t *testing.T) {
	for _, poolName := range []string{"configured-pool", filepath.Join(".ws-worktrees", "repo")} {
		t.Run(poolName, func(t *testing.T) {
			src := &fsRootsSource{}
			f := startPageFixture(t, lanMode, func(o *DaemonOptions) { o.Worktrees = src })
			local, _ := fsViewers(t, f)
			pool := filepath.Join(t.TempDir(), poolName)
			pooled := filepath.Join(pool, "worker")
			detached := filepath.Join(pool, "nested", "detached")
			outside := filepath.Join(pool+"-other", "worker")
			for _, path := range []string{pooled, detached, outside} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			src.read = func(string, []Registration) (RepoWorktrees, error) {
				return RepoWorktrees{Pools: []string{pool}, Worktrees: []RepoWorktree{
					{Path: f.project, Branch: "main"},
					{Path: pooled, Branch: "topic"},
					{Path: outside, Branch: "outside"},
					{Path: detached, Detached: true},
				}}, nil
			}
			rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/worktrees", "")
			var a fsRootsAnswer
			if err := json.Unmarshal(rec.Body.Bytes(), &a); rec.Code != http.StatusOK || err != nil {
				t.Fatalf("roots = %d %s (%v)", rec.Code, rec.Body.String(), err)
			}
			want := []fsRootChoice{
				{Path: filepath.ToSlash(f.project), Branch: "main"},
				{Path: filepath.ToSlash(pooled), Branch: "topic", Pooled: true},
				{Path: filepath.ToSlash(outside), Branch: "outside"},
				{Path: filepath.ToSlash(detached), Detached: true, Pooled: true},
			}
			if !reflect.DeepEqual(a.Roots, want) || strings.Count(rec.Body.String(), `"pooled":true`) != 2 {
				t.Fatalf("pooled roots = %s, want %+v", rec.Body.String(), want)
			}
			// The session root may itself be in the pool, even if Git omits it.
			src.read = func(string, []Registration) (RepoWorktrees, error) {
				return RepoWorktrees{Pools: []string{filepath.Dir(f.project)}}, nil
			}
			rec = serve(f.d, local, http.MethodGet, "/s/livesession/fs/worktrees", "")
			if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil || len(a.Roots) != 1 || !a.Roots[0].Pooled {
				t.Fatalf("pooled session root = %s (%v)", rec.Body.String(), err)
			}
		})
	}
}

func TestFSRootsRefuseBeforeEnumeration(t *testing.T) {
	src := &fsRootsSource{read: func(string, []Registration) (RepoWorktrees, error) {
		return RepoWorktrees{}, errors.New("must not enumerate")
	}}
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) { o.Worktrees = src })
	local, lan := fsViewers(t, f)
	for _, tc := range []struct {
		name, peer, host, site, path string
		cookie                       string
	}{
		{"LAN token", lan.peer, lan.host, "", "/s/livesession/fs/worktrees", lan.cookie},
		{"loopback peer with LAN host", local.peer, lan.host, "", "/s/livesession/fs/worktrees", lan.cookie},
		{"LAN peer with loopback host", lan.peer, local.host, "", "/s/livesession/fs/worktrees", lan.cookie},
		{"cross-site", local.peer, local.host, "cross-site", "/s/livesession/fs/worktrees", ""},
		{"same-site", local.peer, local.host, "same-site", "/s/livesession/fs/worktrees", ""},
		{"unknown session", local.peer, local.host, "", "/s/nosuchsession/fs/worktrees", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.RemoteAddr, req.Host = tc.peer, tc.host
			req.Header.Set("Sec-Fetch-Site", tc.site)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			localhostTestHandler(t, f.d).ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), f.project) || src.calls.Load() != 0 {
				t.Errorf("refusal = %d %s, reads %d", rec.Code, rec.Body.String(), src.calls.Load())
			}
		})
	}
}

func TestFSRootsFallbackAndDirectoryValidity(t *testing.T) {
	for _, withSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "no source", true: "failed source"}[withSource], func(t *testing.T) {
			src := &fsRootsSource{read: func(string, []Registration) (RepoWorktrees, error) {
				return RepoWorktrees{}, errors.New("Git unavailable")
			}}
			f := startPageFixture(t, lanMode, func(o *DaemonOptions) {
				if withSource {
					o.Worktrees = src
				}
			})
			local, _ := fsViewers(t, f)
			rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/worktrees", "")
			var a fsRootsAnswer
			if err := json.Unmarshal(rec.Body.Bytes(), &a); rec.Code != http.StatusOK || err != nil ||
				!reflect.DeepEqual(a.Roots, []fsRootChoice{{Path: filepath.ToSlash(f.project)}}) {
				t.Fatalf("fallback roots = %d %s (%v)", rec.Code, rec.Body.String(), err)
			}
			// Git membership is only a suggestion. Existing arbitrary folders
			// remain browsable even when the source no longer lists them.
			other := t.TempDir()
			query := "root=" + url.QueryEscape(other)
			if code, _ := fsList(t, f, local, query); code != http.StatusOK {
				t.Fatalf("unlisted existing folder = %d", code)
			}
			if err := os.Remove(other); err != nil {
				t.Fatal(err)
			}
			if code, _ := fsList(t, f, local, query); code != http.StatusNotFound {
				t.Errorf("removed folder = %d, want 404", code)
			}
			if code, a := fsList(t, f, local, ""); code != http.StatusOK || !a.Session {
				t.Errorf("session fallback = %d %+v", code, a)
			}
		})
	}
}
