package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

	head := stringsTrim(runGitOutput(t, root, "rev-parse", "HEAD"))

	for _, tc := range []struct {
		name  string
		args  []string
		check func(t *testing.T, out []byte)
	}{
		{name: "status", args: []string{"git", "status", "--root", root}, check: func(t *testing.T, out []byte) {
			var got struct {
				Clean        bool `json:"clean"`
				ChangedFiles []struct {
					Path string `json:"path"`
				} `json:"changed_files"`
			}
			mustUnmarshalCLIJSON(t, out, &got)
			if got.Clean || len(got.ChangedFiles) != 1 || got.ChangedFiles[0].Path != "file.txt" {
				t.Fatalf("status JSON = %#v", got)
			}
		}},
		{name: "diff", args: []string{"git", "diff", "--root", root, "--mode", "name_only", "file.txt"}, check: func(t *testing.T, out []byte) {
			var got struct {
				Mode   string `json:"mode"`
				Output string `json:"output"`
			}
			mustUnmarshalCLIJSON(t, out, &got)
			if got.Mode != "name_only" || got.Output != "file.txt\n" {
				t.Fatalf("diff JSON = %#v", got)
			}
		}},
		{name: "log", args: []string{"git", "log", "--root", root, "--limit", "1"}, check: func(t *testing.T, out []byte) {
			var got struct {
				Limit   int `json:"limit"`
				Commits []struct {
					Subject string `json:"subject"`
				} `json:"commits"`
			}
			mustUnmarshalCLIJSON(t, out, &got)
			if got.Limit != 1 || len(got.Commits) != 1 || got.Commits[0].Subject != "initial" {
				t.Fatalf("log JSON = %#v", got)
			}
		}},
		{name: "merge-base", args: []string{"git", "merge-base", "--root", root, "HEAD", "HEAD"}, check: func(t *testing.T, out []byte) {
			var got struct {
				MergeBase string `json:"merge_base"`
			}
			mustUnmarshalCLIJSON(t, out, &got)
			if got.MergeBase != head {
				t.Fatalf("merge-base JSON = %#v, want %s", got, head)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ws-mcp %v failed: %v\n%s", tc.args, err, string(out))
			}
			tc.check(t, out)
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

func runGitOutput(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return out
}

func mustUnmarshalCLIJSON(t *testing.T, out []byte, value any) {
	t.Helper()
	if err := json.Unmarshal(out, value); err != nil {
		t.Fatalf("invalid CLI JSON: %v\n%s", err, string(out))
	}
}

func stringsTrim(out []byte) string {
	text := string(out)
	for len(text) > 0 && (text[len(text)-1] == '\n' || text[len(text)-1] == '\r') {
		text = text[:len(text)-1]
	}
	return text
}
