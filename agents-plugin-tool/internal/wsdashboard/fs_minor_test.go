package wsdashboard

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Session classification must not decide CORP: a local absolute path can
// escape even with no root override, or with root explicitly set to project.
func TestLocalMediaAlwaysSameOrigin(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	outside := t.TempDir()
	for _, name := range []string{"a.png", "a.svg", "a.mp4", "a.mp3", "a.pdf"} {
		writeFile(t, filepath.Join(f.project, name), "INSIDE")
		writeFile(t, filepath.Join(outside, name), "OUTSIDE")
		for _, root := range []string{"", "&root=" + url.QueryEscape(f.project), "&root=" + url.QueryEscape(outside)} {
			for _, path := range []string{name, filepath.Join(outside, name)} {
				want := "INSIDE"
				if filepath.IsAbs(path) || root == "&root="+url.QueryEscape(outside) {
					want = "OUTSIDE"
				}
				rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/raw?path="+url.QueryEscape(path)+root, "")
				if rec.Code != http.StatusOK || rec.Body.String() != want || rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
					t.Errorf("raw %q root %q = %d %v", path, root, rec.Code, rec.Header())
				}
			}
		}
		if name != "a.png" && name != "a.svg" {
			continue
		}
		for _, path := range []string{name, filepath.Join(outside, name)} {
			rec := serve(f.d, local, http.MethodGet, "/s/livesession/file?path="+url.QueryEscape(path), "")
			if rec.Code != http.StatusOK || rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
				t.Errorf("file image %q = %d %v", path, rec.Code, rec.Header())
			}
		}
		for _, route := range []string{"file", "fs/raw"} {
			rec := serve(f.d, lan, http.MethodGet, "/s/livesession/"+route+"?path="+name, "")
			if rec.Code != http.StatusOK || rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
				t.Errorf("LAN %s = %d %v", route, rec.Code, rec.Header())
			}
		}
	}
}

func TestBadgeGitCommandDisablesFSMonitor(t *testing.T) {
	root := t.TempDir()
	cmd := badgeGitCommand(context.Background(), root, "status", "--porcelain=v1")
	want := []string{cmd.Args[0], "-C", root, "-c", "core.fsmonitor=false", "status", "--porcelain=v1"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("argv = %q, want %q", cmd.Args, want)
	}
}

func TestOutsideListingDoesNotExecuteFSMonitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hostile monitor is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	outside := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", outside}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %q: %v: %s", args, err, out)
		}
	}
	git("init")
	writeFile(t, filepath.Join(outside, "a.txt"), "outside")
	git("add", "a.txt")
	marker := filepath.Join(t.TempDir(), "executed")
	monitor := filepath.Join(outside, "monitor")
	writeFile(t, monitor, "#!/bin/sh\nprintf executed > '"+marker+"'\nprintf '\\0'\n")
	if err := os.Chmod(monitor, 0o755); err != nil {
		t.Fatal(err)
	}
	git("config", "core.fsmonitor", monitor)
	code, _ := fsList(t, f, local, "dir="+url.QueryEscape(outside))
	if code != http.StatusOK {
		t.Fatalf("outside listing = %d", code)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("configured fsmonitor executed (or stat failed): %v", err)
	}
}
