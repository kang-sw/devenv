package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
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

func TestRuntimeCapabilitiesCommandReportsLauncherContractSurface(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	contract := readRuntimeContractTest(t)
	cmd := exec.Command(bin, "runtime", "capabilities")
	cmd.Env = append(os.Environ(), "WS_MCP_TOOL_PROFILE=leaf", "WS_MCP_ALLOWED_TOOLS=project_tree")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ws-mcp runtime capabilities failed: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runtime capabilities wrote diagnostics on success: %q", stderr.String())
	}

	var got struct {
		Version      string `json:"version"`
		SourceCommit string `json:"source_commit"`
		MCPProtocol  string `json:"mcp_protocol"`
		PromptBundle struct {
			SourceCommit  string   `json:"source_commit"`
			ContentSHA256 string   `json:"content_sha256"`
			Prompts       []string `json:"prompts"`
		} `json:"prompt_bundle"`
		Tools    []string `json:"tools"`
		Commands []string `json:"commands"`
	}
	mustUnmarshalCLIJSON(t, out, &got)
	if got.Version == "" || got.SourceCommit == "" {
		t.Fatalf("runtime capabilities missing version/source_commit: %#v", got)
	}
	if got.MCPProtocol != contract.MCPProtocol {
		t.Fatalf("mcp_protocol = %q, want %q", got.MCPProtocol, contract.MCPProtocol)
	}
	if got.PromptBundle.ContentSHA256 != contract.PromptBundle.ContentSHA256 || len(got.PromptBundle.Prompts) == 0 {
		t.Fatalf("prompt bundle = %#v, want hash %q with prompt list", got.PromptBundle, contract.PromptBundle.ContentSHA256)
	}
	wantTools := sortedMapKeys(contract.Tools)
	slices.Sort(got.Tools)
	if !slices.Equal(got.Tools, wantTools) {
		t.Fatalf("tools = %v, want full lead runtime contract tools %v", got.Tools, wantTools)
	}
	wantCommands := sortedMapKeys(contract.Commands)
	slices.Sort(got.Commands)
	if !slices.Equal(got.Commands, wantCommands) {
		t.Fatalf("commands = %v, want runtime contract commands %v", got.Commands, wantCommands)
	}
}

func TestGitCLICommandsReturnJSON(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "core.autocrlf", "false")
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

func TestAgentsDebugCLICommandsReturnDiagnostics(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root := t.TempDir()
	runGit(t, root, "init")
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)
	_, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteCLITest(t, layout.CurrentStdout, "stdout old\nstdout new\n")
	mustWriteCLITest(t, layout.CurrentStderr, "stderr old\nstderr new\n")
	mustWriteCLITest(t, layout.CurrentRuntimeLog, "runtime old\nruntime new\n")
	mustWriteCLITest(t, layout.EventsFile, "event old\nevent new\n")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "stdout", args: []string{"agents", "debug", "stdout", "--root", root, "--name", "impl", "--lines", "1"}, want: "stdout new\n"},
		{name: "stderr", args: []string{"agents", "debug", "stderr", "--root", root, "--name", "impl", "--lines", "1"}, want: "stderr new\n"},
		{name: "runtime-log", args: []string{"agents", "debug", "runtime-log", "--root", root, "--name", "impl", "--lines", "1"}, want: "runtime new\n"},
		{name: "events", args: []string{"agents", "debug", "events", "--root", root, "--name", "impl", "--lines", "1"}, want: "event new\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ws-mcp %v failed: %v\n%s", tc.args, err, string(out))
			}
			if string(out) != tc.want {
				t.Fatalf("ws-mcp %v = %q, want %q", tc.args, out, tc.want)
			}
		})
	}

	cmd := exec.Command(bin, "agents", "debug", "tail", "--root", root, "--name", "impl", "--lines", "1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp agents debug tail failed: %v\n%s", err, string(out))
	}
	if text := string(out); !strings.Contains(text, "== events ==") || !strings.Contains(text, "event new") || !strings.Contains(text, "== stdout ==") {
		t.Fatalf("debug tail output mismatch: %q", text)
	}
}

func TestConfigCLICommandsReturnConfigView(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	cache := filepath.Join(t.TempDir(), "cache")
	wantConfigPath := func() string {
		t.Helper()
		abs, err := filepath.Abs(cache)
		if err != nil {
			t.Fatal(err)
		}
		if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
			abs = evaluated
		}
		return filepath.Join(abs, "config.json")
	}
	show := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "WS_CACHE_HOME="+cache)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ws-mcp %v failed: %v\n%s", args, err, string(out))
		}
		return out
	}

	var before struct {
		Path   string `json:"path"`
		Config struct {
			SchemaVersion int `json:"schema_version"`
			Agents        struct {
				Tiers map[string]struct {
					Backend string `json:"backend"`
					Model   string `json:"model"`
				} `json:"tiers"`
			} `json:"agents"`
		} `json:"config"`
	}
	mustUnmarshalCLIJSON(t, show("config", "show"), &before)
	if before.Path != wantConfigPath() {
		t.Fatalf("config show path = %q", before.Path)
	}
	if before.Config.SchemaVersion != 1 || len(before.Config.Agents.Tiers) != 3 {
		t.Fatalf("default config show = %#v", before.Config)
	}
	if light := before.Config.Agents.Tiers["light"]; light.Backend != "codex" || light.Model != "gpt-5.4-mini" {
		t.Fatalf("default light tier = %#v", light)
	}
	if core := before.Config.Agents.Tiers["core"]; core.Backend != "codex" || core.Model != "gpt-5.5" {
		t.Fatalf("default core tier = %#v", core)
	}
	if deep := before.Config.Agents.Tiers["deep"]; deep.Backend != "codex" || deep.Model != "gpt-5.5" {
		t.Fatalf("default deep tier = %#v", deep)
	}

	show("config", "agents-tier", "--tier", "light", "--model", "gemini-3-1-pro")

	var after struct {
		Path   string `json:"path"`
		Config struct {
			Agents struct {
				Tiers map[string]struct {
					Backend string `json:"backend"`
					Model   string `json:"model"`
				} `json:"tiers"`
			} `json:"agents"`
		} `json:"config"`
	}
	mustUnmarshalCLIJSON(t, show("config", "show"), &after)
	light := after.Config.Agents.Tiers["light"]
	if after.Path != wantConfigPath() || light.Backend != "gemini" || light.Model != "gemini-3-1-pro" {
		t.Fatalf("configured config show = path %q light %#v", after.Path, light)
	}
}

type runtimeContractTest struct {
	MCPProtocol  string `json:"mcp_protocol"`
	PromptBundle struct {
		ContentSHA256 string `json:"content_sha256"`
	} `json:"prompt_bundle"`
	Tools    map[string]string `json:"tools"`
	Commands map[string]string `json:"commands"`
}

func readRuntimeContractTest(t *testing.T) runtimeContractTest {
	t.Helper()
	path := filepath.Join("..", "..", "..", "agents-plugin", "runtime.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var contract runtimeContractTest
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	return contract
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func wsMCPTestBin(t *testing.T) string {
	t.Helper()
	name := "ws-mcp"
	if goruntime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(t.TempDir(), name)
}

func mustWriteCLITest(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
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
