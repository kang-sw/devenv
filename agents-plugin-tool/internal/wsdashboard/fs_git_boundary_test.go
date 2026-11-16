package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func assertNoListingGit(t *testing.T, a fsListAnswer) {
	t.Helper()
	if a.Repo {
		t.Error("outside listing reports a repository")
	}
	for _, e := range a.Entries {
		if e.Status != "" || e.Gone || (e.Ignored && e.Name != ".git") {
			t.Errorf("outside listing carries Git metadata: %+v", e)
		}
	}
}

func boundaryList(t *testing.T, f *pageFixture, v viewer, id, query string) fsListAnswer {
	t.Helper()
	rec := serve(f.d, v, http.MethodGet, "/s/"+id+"/fs/list?"+query, "")
	var a fsListAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &a); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("listing %s?%s = %d %s (%v)", id, query, rec.Code, rec.Body.String(), err)
	}
	return a
}

func TestFSListingGitBoundary(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	outside := t.TempDir()
	for _, root := range []string{f.project, outside} {
		gitRun(t, root, "init", "-q")
		writeFile(t, filepath.Join(root, ".gitignore"), "*.log\n")
		writeFile(t, filepath.Join(root, "noise.log"), "ignored")
		writeFile(t, filepath.Join(root, "new.txt"), "untracked")
	}
	relOutside, err := filepath.Rel(f.project, outside)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := "root=" + url.QueryEscape(f.project)
	for _, tc := range []struct{ id, query string }{
		{"livesession", "dir=" + url.QueryEscape(outside)},
		{"livesession", "dir=" + url.QueryEscape(relOutside)},
		{"livesession", projectRoot + "&dir=" + url.QueryEscape(outside)},
		{"livesession", projectRoot + "&dir=" + url.QueryEscape(relOutside)},
		{"livesession", "root=" + url.QueryEscape(outside)},
		{LocalBrowseAnchor, "root=" + url.QueryEscape(outside)},
		{LocalBrowseAnchor, projectRoot + "&dir=" + url.QueryEscape(outside)},
		{LocalBrowseAnchor, projectRoot + "&dir=" + url.QueryEscape(relOutside)},
	} {
		a := boundaryList(t, f, local, tc.id, tc.query)
		assertNoListingGit(t, a)
		if len(a.Entries) != 4 {
			t.Errorf("outside entries = %+v", a.Entries)
		}
	}
	// An override or local browse anchor is not inherently trusted or
	// untrusted: only the registered roots determine Git eligibility.
	for _, tc := range []struct {
		v         viewer
		id, query string
	}{
		{local, "livesession", ""},
		{lan, "livesession", ""},
		{local, "livesession", "root=" + url.QueryEscape(f.project)},
		{local, LocalBrowseAnchor, "root=" + url.QueryEscape(f.project)},
		{local, "livesession", "root=" + url.QueryEscape(outside) + "&dir=" + url.QueryEscape(f.project)},
		{local, LocalBrowseAnchor, "root=" + url.QueryEscape(filepath.Dir(f.project)) + "&dir=" + url.QueryEscape(f.project)},
	} {
		a := boundaryList(t, f, tc.v, tc.id, tc.query)
		marks := map[string]fsEntry{}
		for _, e := range a.Entries {
			marks[e.Name] = e
		}
		if !a.Repo || marks["new.txt"].Status != "U" || !marks["noise.log"].Ignored {
			t.Errorf("in-root Git metadata lost: %+v", a)
		}
	}
	t.Run("symlinks", func(t *testing.T) {
		escape := filepath.Join(f.project, "escape")
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(outside, escape); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if err := os.Symlink(f.project, alias); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ id, query string }{
			{"livesession", "dir=escape"},
			{"livesession", projectRoot + "&dir=escape"},
			{"livesession", projectRoot + "&dir=" + url.QueryEscape(escape)},
			{LocalBrowseAnchor, projectRoot + "&dir=escape"},
			{"livesession", "root=" + url.QueryEscape(escape)},
			{LocalBrowseAnchor, "root=" + url.QueryEscape(escape)},
			// Resolved containment alone is insufficient too.
			{LocalBrowseAnchor, "root=" + url.QueryEscape(alias)},
		} {
			assertNoListingGit(t, boundaryList(t, f, local, tc.id, tc.query))
		}
	})
}

// Catch every Git invocation, not only commands that happen to execute a
// configured filter. Empty folders must not run a repository probe either.
func TestOutsideListingDoesNotRunGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git sentinel is a shell script")
	}
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	bin := t.TempDir()
	marker := filepath.Join(bin, "executed")
	script := filepath.Join(bin, "git")
	writeFile(t, script, "#!/bin/sh\nprintf executed > '"+strings.ReplaceAll(marker, "'", "'\\''")+"'\nexit 1\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	outside := t.TempDir()
	for _, populated := range []bool{false, true} {
		if populated {
			writeFile(t, filepath.Join(outside, "a.txt"), "outside")
		}
		for _, tc := range []struct{ id, query string }{
			{"livesession", "dir=" + url.QueryEscape(outside)},
			{"livesession", "root=" + url.QueryEscape(outside)},
			{LocalBrowseAnchor, "root=" + url.QueryEscape(outside)},
		} {
			assertNoListingGit(t, boundaryList(t, f, local, tc.id, tc.query))
		}
	}
	// Name/content searches must not re-enter Git through the candidate
	// cache either. They use the bounded filesystem walk outside roots.
	query := "root=" + url.QueryEscape(outside)
	for _, id := range []string{"livesession", LocalBrowseAnchor} {
		if rec := serve(f.d, local, http.MethodGet, "/s/"+id+"/fs/find?"+query+"&q=a", ""); rec.Code != 200 {
			t.Errorf("outside find = %d %s", rec.Code, rec.Body.String())
		}
	}
	if code, a := fsSearch(t, f, query+"&q=outside"); code != 200 || len(a.Files) != 1 || a.Files[0].Path != "a.txt" {
		t.Errorf("outside content search = %d %+v", code, a)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("outside listing/search invoked Git (or stat failed): %v", err)
	}
}

func TestOutsideListingDoesNotExecuteGitFilters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hostile filters are shell scripts")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, driver := range []string{"clean", "process"} {
		t.Run(driver, func(t *testing.T) {
			f := startPageFixture(t, lanMode)
			local, _ := fsViewers(t, f)
			outside := t.TempDir()
			gitRun(t, outside, "init", "-q")
			writeFile(t, filepath.Join(outside, "a.txt"), "base")
			gitRun(t, outside, "add", "a.txt")
			// Same size: status must compare filtered contents.
			writeFile(t, filepath.Join(outside, "a.txt"), "evil")
			writeFile(t, filepath.Join(outside, ".gitattributes"), "*.txt filter=hostile\n")
			marker := filepath.Join(t.TempDir(), "executed")
			script := filepath.Join(outside, "filter")
			body := "#!/bin/sh\nprintf executed > '" + strings.ReplaceAll(marker, "'", "'\\''") + "'\n"
			if driver == "clean" {
				body += "cat\n"
			} else {
				body += "exit 1\n" // executing the process is already the exploit
			}
			writeFile(t, script, body)
			if err := os.Chmod(script, 0o755); err != nil {
				t.Fatal(err)
			}
			gitRun(t, outside, "config", "filter.hostile."+driver, "'"+strings.ReplaceAll(script, "'", "'\\''")+"'")
			// Positive control: fsmonitor-off status really executes this
			// filter. A broken hostile fixture must not silently pass.
			gitMarks(outside)
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("hostile %s filter did not execute in control: %v", driver, err)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct{ id, query string }{
				{"livesession", "dir=" + url.QueryEscape(outside)},
				{"livesession", "root=" + url.QueryEscape(outside)},
				{LocalBrowseAnchor, "root=" + url.QueryEscape(outside)},
			} {
				assertNoListingGit(t, boundaryList(t, f, local, tc.id, tc.query))
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatalf("configured %s filter executed (or stat failed): %v", driver, err)
				}
			}
		})
	}
}
