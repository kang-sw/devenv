package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
)

// TestMain defaults WS_RSRC_ROOT to the shipped rsrc tree so `mercenary register`
// can load delegate-orientation (260611 Phase 6b moved it off the wsprompt
// go:embed bundle).
func TestMain(m *testing.M) {
	if os.Getenv("WS_RSRC_ROOT") == "" {
		_ = os.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	}
	os.Exit(m.Run())
}

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
	cmd.Env = append(os.Environ(),
		"WS_CONFIG_HOME="+t.TempDir(),
		"WS_MCP_TOOL_PROFILE=leaf",
		"WS_MCP_ALLOWED_TOOLS=project_tree",
	)
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
		Version      string   `json:"version"`
		SourceCommit string   `json:"source_commit"`
		MCPProtocol  string   `json:"mcp_protocol"`
		Tools        []string `json:"tools"`
		Commands     []string `json:"commands"`
	}
	mustUnmarshalCLIJSON(t, out, &got)
	if got.Version == "" || got.SourceCommit == "" {
		t.Fatalf("runtime capabilities missing version/source_commit: %#v", got)
	}
	if got.MCPProtocol != contract.MCPProtocol {
		t.Fatalf("mcp_protocol = %q, want %q", got.MCPProtocol, contract.MCPProtocol)
	}
	wantTools := sortedMapKeys(contract.Tools)
	slices.Sort(got.Tools)
	if !slices.Equal(got.Tools, wantTools) {
		t.Fatalf("tools = %v, want full lead runtime contract tools %v", got.Tools, wantTools)
	}
	for _, hidden := range []string{"ws.lead.prefer_mercenary", "ws.mercenary.call", "ws.mercenary.register"} {
		if slices.Contains(got.Tools, hidden) {
			t.Fatalf("runtime capabilities exposed hidden mercenary tool %s in %v", hidden, got.Tools)
		}
	}
	wantCommands := sortedMapKeys(contract.Commands)
	slices.Sort(got.Commands)
	if !slices.Equal(got.Commands, wantCommands) {
		t.Fatalf("commands = %v, want runtime contract commands %v", got.Commands, wantCommands)
	}
}

func TestRuntimeCapabilitiesCommandReportsNoAgentSurface(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	cmd := exec.Command(bin, "runtime", "capabilities")
	cmd.Env = append(os.Environ(), "WS_MCP_NO_AGENT=1", "WS_MCP_NAMESPACE=wsflow", "WS_MCP_SETUP_TOOL=setup")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ws-mcp runtime capabilities failed: %v", err)
	}
	var got struct {
		Tools    []string `json:"tools"`
		Commands []string `json:"commands"`
	}
	mustUnmarshalCLIJSON(t, out, &got)
	for _, hidden := range []string{"ws.lead.prefer_mercenary", "ws.mercenary.call", "ws.mercenary.register", "ws.mercenary.debug.tail", "subquery", "api.ask", "api.ask_async", "api.status", "api.result", "api.cancel", "ws.setup", "exec.spawn", "exec.shell", "exec.status", "exec.result", "exec.abort", "exec.raw.tail", "exec.raw.read", "exec.raw.grep"} {
		if slices.Contains(got.Tools, hidden) {
			t.Fatalf("no-agent capabilities exposed hidden tool %s in %v", hidden, got.Tools)
		}
	}
	for _, visible := range []string{"api.list", "config.show", "config.tuning", "config.agents_tier", "tickets.list"} {
		if !slices.Contains(got.Tools, visible) {
			t.Fatalf("no-agent capabilities missing visible tool %s in %v", visible, got.Tools)
		}
	}
	for _, hidden := range []string{"mercenary.call", "mercenary.cancel", "mercenary.run-current", "subquery", "config.agents-tier"} {
		if slices.Contains(got.Commands, hidden) {
			t.Fatalf("no-agent capabilities exposed hidden command %s in %v", hidden, got.Commands)
		}
	}
	for _, visible := range []string{"config.show", "tickets.list", "runtime.capabilities"} {
		if !slices.Contains(got.Commands, visible) {
			t.Fatalf("no-agent capabilities missing visible command %s in %v", visible, got.Commands)
		}
	}
}

// TestRuntimeCapabilitiesCommandReportsWsflowContractSurface is the agentless
// analogue of the full-surface contract test above: it asserts the live wsflow
// no-agent tool/command set equals agents-plugin-wsflow/runtime.json exactly.
// The wsflow launcher checks this manifest with runtime_capabilities.match
// "exact", so without this test the hand-maintained wsflow manifest can drift
// silently and only fail at launcher runtime for users while CI stays green.
func TestRuntimeCapabilitiesCommandReportsWsflowContractSurface(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	contract := readRuntimeContractAtTest(t, filepath.Join("..", "..", "..", "agents-plugin-wsflow", "runtime.json"))
	cmd := exec.Command(bin, "runtime", "capabilities")
	cmd.Env = append(os.Environ(), "WS_MCP_NO_AGENT=1", "WS_MCP_NAMESPACE=wsflow", "WS_MCP_SETUP_TOOL=setup")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ws-mcp runtime capabilities (wsflow) failed: %v", err)
	}

	var got struct {
		MCPProtocol string   `json:"mcp_protocol"`
		Tools       []string `json:"tools"`
		Commands    []string `json:"commands"`
	}
	mustUnmarshalCLIJSON(t, out, &got)
	if got.MCPProtocol != contract.MCPProtocol {
		t.Fatalf("wsflow mcp_protocol = %q, want %q", got.MCPProtocol, contract.MCPProtocol)
	}
	wantTools := sortedMapKeys(contract.Tools)
	slices.Sort(got.Tools)
	if !slices.Equal(got.Tools, wantTools) {
		t.Fatalf("wsflow tools = %v, want wsflow runtime contract tools %v", got.Tools, wantTools)
	}
	wantCommands := sortedMapKeys(contract.Commands)
	slices.Sort(got.Commands)
	if !slices.Equal(got.Commands, wantCommands) {
		t.Fatalf("wsflow commands = %v, want wsflow runtime contract commands %v", got.Commands, wantCommands)
	}
}

func TestNoAgentCLICommandsReturnDisabledErrors(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "mercenary", args: []string{"mercenary", "status", "--name", "impl"}, want: "wsflow agentless mode disables agent-backed command: mercenary"},
		{name: "config agents-tier", args: []string{"config", "agents-tier", "--tier", "core"}, want: "wsflow agentless mode disables agent-backed command: config agents-tier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Env = append(os.Environ(), "WS_MCP_NO_AGENT=1", "WS_MCP_NAMESPACE=wsflow")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("ws-mcp %v unexpectedly succeeded: %s", tc.args, string(out))
			}
			if !strings.Contains(string(out), tc.want) {
				t.Fatalf("ws-mcp %v error missing %q:\n%s", tc.args, tc.want, string(out))
			}
		})
	}
}

func TestSmokeCommandRunsExecutableChecksInOneProcess(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "smoke", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp smoke failed: %v\n%s", err, string(out))
	}
	text := string(out)
	for _, want := range []string{"version:", "ok repo root:", "ok stdio smoke:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("smoke output missing %q:\n%s", want, text)
		}
	}
}

func TestGitCLICommandsDefaultToTextAndKeepJSONFormat(t *testing.T) {
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
		name      string
		args      []string
		jsonArgs  []string
		checkText func(t *testing.T, out []byte)
		checkJSON func(t *testing.T, out []byte)
	}{
		{name: "status", args: []string{"git", "status", "--root", root}, jsonArgs: []string{"git", "status", "--root", root, "--format", "json"}, checkText: func(t *testing.T, out []byte) {
			text := string(out)
			if !strings.Contains(text, "file.txt") || !strings.Contains(text, "## ") || strings.HasPrefix(strings.TrimSpace(text), "{") {
				t.Fatalf("status text = %q", text)
			}
		}, checkJSON: func(t *testing.T, out []byte) {
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
		{name: "diff", args: []string{"git", "diff", "--root", root, "--mode", "name_only", "file.txt"}, jsonArgs: []string{"git", "diff", "--root", root, "--mode", "name_only", "--format", "json", "file.txt"}, checkText: func(t *testing.T, out []byte) {
			if string(out) != "file.txt\n" {
				t.Fatalf("diff text = %q", out)
			}
		}, checkJSON: func(t *testing.T, out []byte) {
			var got struct {
				Mode   string `json:"mode"`
				Output string `json:"output"`
			}
			mustUnmarshalCLIJSON(t, out, &got)
			if got.Mode != "name_only" || got.Output != "file.txt\n" {
				t.Fatalf("diff JSON = %#v", got)
			}
		}},
		{name: "log", args: []string{"git", "log", "--root", root, "--limit", "1"}, jsonArgs: []string{"git", "log", "--root", root, "--limit", "1", "--format", "json"}, checkText: func(t *testing.T, out []byte) {
			text := string(out)
			if !strings.Contains(text, "commit "+head) || !strings.Contains(text, "initial") || strings.Contains(text, `"commits"`) {
				t.Fatalf("log text = %q", text)
			}
		}, checkJSON: func(t *testing.T, out []byte) {
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
		{name: "merge-base", args: []string{"git", "merge-base", "--root", root, "HEAD", "HEAD"}, jsonArgs: []string{"git", "merge-base", "--root", root, "--format", "json", "HEAD", "HEAD"}, checkText: func(t *testing.T, out []byte) {
			if string(out) != head+"\n" {
				t.Fatalf("merge-base text = %q", out)
			}
		}, checkJSON: func(t *testing.T, out []byte) {
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
			tc.checkText(t, out)

			cmd = exec.Command(bin, tc.jsonArgs...)
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ws-mcp %v failed: %v\n%s", tc.jsonArgs, err, string(out))
			}
			tc.checkJSON(t, out)
		})
	}
}

func TestGitCommitCLIRendersMentalModelNotes(t *testing.T) {
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

	cmd := exec.Command(bin,
		"git", "commit",
		"--root", root,
		"--path", "file.txt",
		"--title", "test: cli mental model notes",
		"--ai-context", "User intent: verify CLI commit notes.",
		"--mental-model-note", "CLI forwards structured Mental Model Notes.",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp git commit failed: %v\n%s", err, string(out))
	}
	if text := string(out); !strings.Contains(text, "commit: ") || !strings.Contains(text, "title: test: cli mental model notes") {
		t.Fatalf("git commit text response = %q", text)
	}

	commitBody := string(runGitOutput(t, root, "log", "-1", "--format=%B"))
	if !strings.Contains(commitBody, "## AI Context\n- User intent: verify CLI commit notes.\n\n### Mental Model Notes\n- CLI forwards structured Mental Model Notes.") {
		t.Fatalf("commit body missing CLI Mental Model Notes subsection:\n%s", commitBody)
	}
}

func TestDocumentationCLICommandsDefaultToTextAndKeepJSONFormat(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root := t.TempDir()
	mustWriteCLITest(t, filepath.Join(root, "ai-docs/tickets/todo/260504-demo-ticket.md"), "---\ntitle: Demo Ticket\nspec:\n  - 260504-demo-spec\n---\n# Demo Ticket\n")
	mustWriteCLITest(t, filepath.Join(root, "ai-docs/spec/demo.md"), "---\ntitle: Demo Spec\nsummary: Demo summary\n---\n# Demo\n\n## Feature {#260504-demo-spec}\n\nDemo installer marketplace release packaging behavior.\n")
	mustWriteCLITest(t, filepath.Join(root, "ai-docs/mental-model/demo.md"), "---\ndomain: demo\ndescription: Demo model\nsources:\n  - ai-docs/spec/demo.md#260504-demo-spec\n---\n# Demo\n\nRuntime readable CLI mirror behavior.\n")
	runGit(t, root, "init")
	runGit(t, root, "config", "core.autocrlf", "false")

	for _, tc := range []struct {
		name     string
		textArgs []string
		jsonArgs []string
		wantText string
	}{
		{name: "tickets list", textArgs: []string{"tickets", "list", "--root", root}, jsonArgs: []string{"tickets", "list", "--root", root, "--format", "json"}, wantText: "[todo] 260504-demo-ticket"},
		{name: "specs status", textArgs: []string{"specs", "status", "--root", root, "260504-demo-spec"}, jsonArgs: []string{"specs", "status", "--root", root, "--format", "json", "260504-demo-spec"}, wantText: "spec_stem: 260504-demo-spec"},
		{name: "mental-models find", textArgs: []string{"mental-models", "find", "--root", root, "--domain", "demo"}, jsonArgs: []string{"mental-models", "find", "--root", root, "--domain", "demo", "--format", "json"}, wantText: "ai-docs/mental-model/demo.md"},
		{name: "references trace", textArgs: []string{"references", "trace", "--root", root, "--ticket-stem", "260504-demo-ticket"}, jsonArgs: []string{"references", "trace", "--root", root, "--ticket-stem", "260504-demo-ticket", "--format", "json"}, wantText: "input: ticket 260504-demo-ticket"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.textArgs...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ws-mcp %v failed: %v\n%s", tc.textArgs, err, string(out))
			}
			text := string(out)
			if !strings.Contains(text, tc.wantText) || strings.HasPrefix(strings.TrimSpace(text), "{") || strings.HasPrefix(strings.TrimSpace(text), "[") && !strings.HasPrefix(tc.wantText, "[") {
				t.Fatalf("%s text = %q", tc.name, text)
			}

			cmd = exec.Command(bin, tc.jsonArgs...)
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ws-mcp %v failed: %v\n%s", tc.jsonArgs, err, string(out))
			}
			var value any
			mustUnmarshalCLIJSON(t, out, &value)
		})
	}

	cmd := exec.Command(bin, "specs", "find", "--root", root, "--query", "installer marketplace release packaging")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp specs query failed: %v\n%s", err, string(out))
	}
	if text := string(out); !strings.Contains(text, "candidate spec for query=\"installer marketplace release packaging\"") || !strings.Contains(text, "ai-docs/spec/demo.md\tscore=") || !strings.Contains(text, "  ") || strings.Contains(text, "matched:") {
		t.Fatalf("specs query text = %q", text)
	}
	cmd = exec.Command(bin, "specs", "find", "--root", root, "--query", "installer marketplace release packaging", "--format", "json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp specs query json failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "\"matches\"") || !strings.Contains(string(out), "\"matched_terms\"") {
		t.Fatalf("specs query json missing evidence: %s", string(out))
	}

	cmd = exec.Command(bin, "mental-models", "find", "--root", root, "--query", "runtime readable CLI mirror")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp mental-models query failed: %v\n%s", err, string(out))
	}
	if text := string(out); !strings.Contains(text, "candidate mental model for query=\"runtime readable CLI mirror\"") || !strings.Contains(text, "ai-docs/mental-model/demo.md\tscore=") || strings.Contains(text, "matched:") {
		t.Fatalf("mental-models query text = %q", text)
	}
	cmd = exec.Command(bin, "mental-models", "find", "--root", root, "--query", "runtime readable CLI mirror", "--format", "json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp mental-models query json failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "\"matches\"") || !strings.Contains(string(out), "\"matched_terms\"") {
		t.Fatalf("mental-models query json missing evidence: %s", string(out))
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
		{name: "stdout", args: []string{"mercenary", "debug", "stdout", "--root", root, "--name", "impl", "--lines", "1"}, want: "stdout new\n"},
		{name: "stderr", args: []string{"mercenary", "debug", "stderr", "--root", root, "--name", "impl", "--lines", "1"}, want: "stderr new\n"},
		{name: "runtime-log", args: []string{"mercenary", "debug", "runtime-log", "--root", root, "--name", "impl", "--lines", "1"}, want: "runtime new\n"},
		{name: "events", args: []string{"mercenary", "debug", "events", "--root", root, "--name", "impl", "--lines", "1"}, want: "event new\n"},
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

	cmd := exec.Command(bin, "mercenary", "debug", "tail", "--root", root, "--name", "impl", "--lines", "1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp mercenary debug tail failed: %v\n%s", err, string(out))
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
					Effort  string `json:"effort"`
				} `json:"tiers"`
				ModelAliases map[string]map[string]struct {
					Backend string `json:"backend"`
					Model   string `json:"model"`
					Effort  string `json:"effort"`
				} `json:"model_aliases"`
			} `json:"agents"`
		} `json:"config"`
	}
	beforeText := string(show("config", "show"))
	if !strings.Contains(beforeText, "path: "+wantConfigPath()) || !strings.Contains(beforeText, "model_aliases:") || strings.HasPrefix(strings.TrimSpace(beforeText), "{") {
		t.Fatalf("config show text = %q", beforeText)
	}
	mustUnmarshalCLIJSON(t, show("config", "show", "--format", "json"), &before)
	if before.Path != wantConfigPath() {
		t.Fatalf("config show path = %q", before.Path)
	}
	if before.Config.SchemaVersion != 1 || len(before.Config.Agents.Tiers) != 4 {
		t.Fatalf("default config show = %#v", before.Config)
	}
	if small := before.Config.Agents.Tiers["small"]; small.Backend != "codex" || small.Model != "gpt-5.6-luna" {
		t.Fatalf("default small tier = %#v", small)
	}
	if medium := before.Config.Agents.Tiers["medium"]; medium.Backend != "codex" || medium.Model != "gpt-5.6-terra" {
		t.Fatalf("default medium tier = %#v", medium)
	}
	if large := before.Config.Agents.Tiers["large"]; large.Backend != "codex" || large.Model != "gpt-5.6-sol" {
		t.Fatalf("default large tier = %#v", large)
	}
	if xlarge := before.Config.Agents.Tiers["xlarge"]; xlarge.Backend != "codex" || xlarge.Model != "gpt-5.6-sol" {
		t.Fatalf("default xlarge tier = %#v", xlarge)
	}

	show("config", "agents-tier", "--tier", "light", "--model", "claude-sonnet-4")

	var after struct {
		Path   string `json:"path"`
		Config struct {
			Agents struct {
				Tiers map[string]struct {
					Backend string `json:"backend"`
					Model   string `json:"model"`
					Effort  string `json:"effort"`
				} `json:"tiers"`
				ModelAliases map[string]map[string]struct {
					Backend string `json:"backend"`
					Model   string `json:"model"`
					Effort  string `json:"effort"`
				} `json:"model_aliases"`
			} `json:"agents"`
		} `json:"config"`
	}
	mustUnmarshalCLIJSON(t, show("config", "show", "--format", "json"), &after)
	small := after.Config.Agents.Tiers["small"]
	if after.Path != wantConfigPath() || small.Backend != "claude" || small.Model != "claude-sonnet-4" {
		t.Fatalf("configured config show = path %q small %#v", after.Path, small)
	}

	show("config", "agents-tier", "--tier", "core", "--harness", "claude", "--backend", "codex", "--model", "gpt-5.4", "--effort", "medium")
	var harnessAfter struct {
		Config struct {
			Agents struct {
				ModelAliases map[string]map[string]struct {
					Backend string `json:"backend"`
					Model   string `json:"model"`
					Effort  string `json:"effort"`
				} `json:"model_aliases"`
			} `json:"agents"`
		} `json:"config"`
	}
	mustUnmarshalCLIJSON(t, show("config", "show", "--format", "json"), &harnessAfter)
	claudeMedium := harnessAfter.Config.Agents.ModelAliases["medium"]["claude"]
	if claudeMedium.Backend != "codex" || claudeMedium.Model != "gpt-5.4" || claudeMedium.Effort != "medium" {
		t.Fatalf("claude medium alias = %#v", claudeMedium)
	}

	show("config", "agents-tier", "--tier", "core", "--harness", "claude", "--backend", "codex", "--model", "gpt-5.5")
	mustUnmarshalCLIJSON(t, show("config", "show", "--format", "json"), &harnessAfter)
	claudeMedium = harnessAfter.Config.Agents.ModelAliases["medium"]["claude"]
	if claudeMedium.Backend != "codex" || claudeMedium.Model != "gpt-5.5" || claudeMedium.Effort != "" {
		t.Fatalf("claude medium alias after omitted effort update = %#v", claudeMedium)
	}
}

type runtimeContractTest struct {
	MCPProtocol string            `json:"mcp_protocol"`
	Tools       map[string]string `json:"tools"`
	Commands    map[string]string `json:"commands"`
}

func readRuntimeContractTest(t *testing.T) runtimeContractTest {
	t.Helper()
	return readRuntimeContractAtTest(t, filepath.Join("..", "..", "..", "agents-plugin", "runtime.json"))
}

func readRuntimeContractAtTest(t *testing.T, path string) runtimeContractTest {
	t.Helper()
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

// --- tools / call passthrough (Phase 1) ---

// inProcessToolsList drives a bare tools/list request directly through
// mcp.NewServer(...).ServeStdio, mirroring the CLI's own runMCPLine helper,
// so tests can assert the CLI's `tools` output against the same profile
// filtering (filteredTools) without duplicating it.
func inProcessToolsList(t *testing.T) []map[string]any {
	t.Helper()
	server := mcp.NewServer(".", "test", "test")
	var buf bytes.Buffer
	line := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	if err := server.ServeStdio(context.Background(), strings.NewReader(line), &buf); err != nil {
		t.Fatalf("in-process tools/list failed: %v", err)
	}
	var resp struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("decode in-process tools/list: %v\n%s", err, buf.String())
	}
	return resp.Result.Tools
}

// parseSessionKeyLine extracts the `session_key: <value>` line ferrule's
// text response always leads with.
func parseSessionKeyLine(t *testing.T, text string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if name, value, ok := strings.Cut(line, ": "); ok && name == "session_key" {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("session_key not found in output: %q", text)
	return ""
}

func TestToolsCommandBareListMatchesToolsList(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{name: "full", env: nil},
		{name: "agentless", env: map[string]string{"WS_MCP_NO_AGENT": "1", "WS_MCP_NAMESPACE": "wsflow"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			wantTools := inProcessToolsList(t)
			wantNames := make([]string, 0, len(wantTools))
			for _, tool := range wantTools {
				name, _ := tool["name"].(string)
				wantNames = append(wantNames, name)
			}
			slices.Sort(wantNames)

			cmd := exec.Command(bin, "tools")
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ws-mcp tools failed: %v\n%s", err, string(out))
			}
			text := string(out)
			if !strings.Contains(text, toolsMappingRule) {
				t.Fatalf("tools output missing mapping rule: %q", text)
			}
			if strings.Contains(text, "inputSchema") {
				t.Fatalf("bare tools output leaked inputSchema: %q", text)
			}
			lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
			if len(lines) == 0 || lines[0] != toolsMappingRule {
				t.Fatalf("tools output first line = %q, want mapping rule", lines[0])
			}
			gotNames := make([]string, 0, len(lines)-1)
			for _, line := range lines[1:] {
				name, _, ok := strings.Cut(line, ": ")
				if !ok {
					t.Fatalf("tools line missing name/description separator: %q", line)
				}
				gotNames = append(gotNames, name)
			}
			slices.Sort(gotNames)
			if !slices.Equal(gotNames, wantNames) {
				t.Fatalf("tools names = %v, want %v", gotNames, wantNames)
			}

			if tc.name == "agentless" {
				for _, hidden := range []string{"mercenary.call", "mercenary.register", "mercenary.debug.tail"} {
					if slices.Contains(gotNames, hidden) {
						t.Fatalf("agentless tools output exposed hidden tool %s in %v", hidden, gotNames)
					}
				}
				if !slices.Contains(gotNames, "config.agents_tier") {
					t.Fatalf("agentless tools output missing shared tool config.agents_tier in %v", gotNames)
				}
			}
		})
	}
}

func TestToolsCommandShowMatchesInputSchema(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	wantTools := inProcessToolsList(t)
	var wantSchema map[string]any
	for _, tool := range wantTools {
		if name, _ := tool["name"].(string); name == "runtime.info" {
			wantSchema, _ = tool["inputSchema"].(map[string]any)
			break
		}
	}
	if wantSchema == nil {
		t.Fatal("runtime.info missing inputSchema in in-process tools/list")
	}

	cmd := exec.Command(bin, "tools", "runtime.info")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp tools runtime.info failed: %v\n%s", err, string(out))
	}
	var gotSchema map[string]any
	if err := json.Unmarshal(out, &gotSchema); err != nil {
		t.Fatalf("invalid schema JSON: %v\n%s", err, string(out))
	}
	wantJSON, _ := json.Marshal(wantSchema)
	gotJSON, _ := json.Marshal(gotSchema)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("tools runtime.info schema = %s, want %s", gotJSON, wantJSON)
	}
}

func TestToolsCommandShowUnknownToolExitsNonZero(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	cmd := exec.Command(bin, "tools", "nonexistent-tool")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown tool, got success: %s", out)
	}
	if !strings.Contains(string(out), "tool not found") {
		t.Fatalf("unexpected message: %s", out)
	}
}

func TestCallCommandMalformedJSONExitsNonZero(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	cmd := exec.Command(bin, "call", "runtime.info", "not-json")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for malformed JSON, got success: %s", out)
	}
	if !strings.Contains(string(out), "malformed JSON") {
		t.Fatalf("unexpected message: %s", out)
	}
}

func TestCallCommandUnknownToolExitsNonZero(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	cmd := exec.Command(bin, "call", "nonexistent-tool", "{}")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown tool, got success: %s", out)
	}
	if !strings.Contains(string(out), "unknown tool") {
		t.Fatalf("unexpected message: %s", out)
	}
}

func TestCallCommandCrossProcessSessionKeyRoundTrip(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root := t.TempDir()
	runGit(t, root, "init")
	cache := filepath.Join(t.TempDir(), "cache")
	env := append(os.Environ(), "WS_CACHE_HOME="+cache)

	mint := exec.Command(bin, "call", "ferrule", fmt.Sprintf(`{"root":%q}`, root))
	mint.Env = env
	mintOut, err := mint.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp call ferrule failed: %v\n%s", err, string(mintOut))
	}
	key := parseSessionKeyLine(t, string(mintOut))

	// Fresh process, same WS_CACHE_HOME: the minted key must resolve here too.
	callCmd := exec.Command(bin, "call", "git.status", fmt.Sprintf(`{"session_key":%q}`, key))
	callCmd.Env = env
	gotOut, err := callCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp call git.status failed: %v\n%s", err, string(gotOut))
	}

	// In-process equivalent for the same key, sharing WS_CACHE_HOME via t.Setenv.
	t.Setenv("WS_CACHE_HOME", cache)
	server := mcp.NewServer(".", "test", "test")
	var buf bytes.Buffer
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.status","arguments":{"session_key":%q}}}`, key) + "\n"
	if err := server.ServeStdio(context.Background(), strings.NewReader(line), &buf); err != nil {
		t.Fatalf("in-process tools/call failed: %v", err)
	}
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("decode in-process tools/call: %v\n%s", err, buf.String())
	}
	var wantText string
	if len(resp.Result.Content) > 0 {
		wantText = resp.Result.Content[0].Text
	}
	if string(gotOut) != wantText {
		t.Fatalf("ws-mcp call git.status = %q, want %q (in-process equivalent)", gotOut, wantText)
	}
}

func TestCallCommandRejectsLeadOnlyToolForNonLeadSessionKey(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root := t.TempDir()
	runGit(t, root, "init")
	cache := filepath.Join(t.TempDir(), "cache")
	env := append(os.Environ(), "WS_CACHE_HOME="+cache)

	// Keyless mint requesting a non-lead ("delegate") capability succeeds —
	// only the keyed gate inside callTool rejects a non-lead key from
	// reaching a lead-only tool, not the mint call itself.
	mint := exec.Command(bin, "call", "ferrule", fmt.Sprintf(`{"root":%q,"capability":"delegate"}`, root))
	mint.Env = env
	mintOut, err := mint.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp call ferrule (delegate mint) failed: %v\n%s", err, string(mintOut))
	}
	delegateKey := parseSessionKeyLine(t, string(mintOut))

	for _, tool := range []string{"ferrule", "workflow_manual"} {
		t.Run(tool, func(t *testing.T) {
			args := fmt.Sprintf(`{"session_key":%q,"root":%q}`, delegateKey, root)
			callCmd := exec.Command(bin, "call", tool, args)
			callCmd.Env = env
			out, err := callCmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected non-zero exit for lead-only tool %s via non-lead session_key, got success: %s", tool, out)
			}
			if !strings.Contains(string(out), "not available") {
				t.Fatalf("unexpected rejection message for %s: %s", tool, out)
			}
		})
	}
}

func TestCallCommandColdStartWorkflowManualMintsLeadKey(t *testing.T) {
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	root := t.TempDir()
	runGit(t, root, "init")
	cache := filepath.Join(t.TempDir(), "cache")

	cmd := exec.Command(bin, "call", "workflow_manual", fmt.Sprintf(`{"session_key":"obsidian-latch","root":%q}`, root))
	cmd.Env = append(os.Environ(), "WS_CACHE_HOME="+cache)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ws-mcp call workflow_manual (cold start) failed: %v\n%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "## Session Key") {
		t.Fatalf("cold-start workflow_manual output missing session key section: %q", text)
	}
	if strings.Contains(text, "obsidian-latch") {
		t.Fatalf("cold-start workflow_manual echoed the freshBootstrapKey sentinel instead of minting a fresh key: %q", text)
	}
}
