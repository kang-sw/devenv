package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRootUsesExplicitRoot(t *testing.T) {
	t.Setenv("WS_MCP_PROJECT_ROOT", "/env/root")
	if got := defaultRoot("/explicit/root"); got != "/explicit/root" {
		t.Fatalf("defaultRoot explicit = %q", got)
	}
}

func TestDefaultRootUsesProjectEnvForDot(t *testing.T) {
	t.Setenv("WS_MCP_PROJECT_ROOT", "/env/root")
	if got := defaultRoot("."); got != "/env/root" {
		t.Fatalf("defaultRoot dot = %q", got)
	}
}

func TestDefaultRootPreservesDotWithoutProjectEnv(t *testing.T) {
	t.Setenv("WS_MCP_PROJECT_ROOT", "")
	if got := defaultRoot("."); got != "." {
		t.Fatalf("defaultRoot without env = %q", got)
	}
}

func TestGitCLICommandsReturnJSON(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ws-mcp")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "status", args: []string{"git", "status", "--root", root}, want: "changed_files"},
		{name: "diff", args: []string{"git", "diff", "--root", root, "--mode", "name_only", "file.txt"}, want: "file.txt"},
		{name: "log", args: []string{"git", "log", "--root", root, "--limit", "1"}, want: "initial"},
		{name: "merge-base", args: []string{"git", "merge-base", "--root", root, "HEAD", "HEAD"}, want: "merge_base"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ws-mcp %v failed: %v\n%s", tc.args, err, string(out))
			}
			if !strings.Contains(string(out), tc.want) {
				t.Fatalf("output missing %q: %s", tc.want, string(out))
			}
		})
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
