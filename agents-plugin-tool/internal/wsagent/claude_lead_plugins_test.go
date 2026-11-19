package wsagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// claudePluginLeadSpec is a hosted Claude lead's start with the daemon's
// ws-mcp over stdio and empty settings sources.
func claudePluginLeadSpec(t *testing.T) StartSpec {
	t.Helper()
	return StartSpec{
		Root:  t.TempDir(),
		Env:   []string{"CLAUDE_CONFIG_DIR=" + t.TempDir()},
		WSMCP: &MCPCommand{Command: "/bin/ws-mcp"},
		Lead:  &LeadSpec{DaemonVersion: hostedPluginTestVersion},
	}
}

func claudePluginOverrides(t *testing.T, args []string) map[string]bool {
	t.Helper()
	raw, ok := flagValue(args, "--settings")
	if !ok {
		t.Fatalf("missing --settings: %v", args)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 {
		t.Fatalf("lead override must touch only enabledPlugins: %s", raw)
	}
	var plugins map[string]bool
	if err := json.Unmarshal(settings["enabledPlugins"], &plugins); err != nil {
		t.Fatal(err)
	}
	return plugins
}

// TestClaudeLeadPluginArgsModes: a hosted Claude lead loads no ws package
// in any isolation mode: every installed ws and wsflow copy the merged
// settings enable is turned off, unrelated plugins (a ws-prefixed one
// included) stay as the user set them, and no bundle directory is loaded.
// The daemon's ws-mcp is the lead's ws server instead, allowed last.
func TestClaudeLeadPluginArgsModes(t *testing.T) {
	spec := claudePluginLeadSpec(t)
	configDir := strings.TrimPrefix(spec.Env[0], "CLAUDE_CONFIG_DIR=")
	writeLeadPluginFixture(t, filepath.Join(configDir, "settings.json"), `{
		"enabledPlugins":{"ws@user-market":true,"wsflow@old-market":true,"ws@off":false,"clangd@tools":true,"ws-extra@tools":true},
		"hooks":{"SessionStart":[]},"permissions":{"defaultMode":"plan"},"env":{"UNCHANGED":"yes"}
	}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Root, ".claude", "settings.json"), `{
		"enabledPlugins":{"ws@user-market":false,"wsflow@project-market":true,"clangd@tools":false,"other@project":true}
	}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Root, ".claude", "settings.local.json"), `{
		"enabledPlugins":{"wsflow@old-market":false,"ws@local-market":true,"clangd@tools":true,"other@project":false}
	}`)
	spec.Lead.PermissionMode = "plan"
	spec.Model, spec.Effort, spec.SessionID = "opus", "high", "resume-id"
	spec.Instructions = "/sess/instructions.md"
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp", Args: []string{"serve", "--stdio"}, Env: map[string]string{"WS_MCP_PROJECT_ROOT": spec.Root}}
	spec.RemoveSubagents = RemovesSubagents(Record{Backend: BackendClaude, Lead: true})
	// The record's isolation no longer applies to a Claude lead, so not
	// even an unknown value changes or refuses its launch.
	for _, mode := range []string{"", "default", "isolated", "unknown"} {
		t.Run("mode="+mode, func(t *testing.T) {
			spec.Lead.PluginIsolation = mode
			args, err := claudeArgs(spec)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]bool{"wsflow@project-market": false, "ws@local-market": false}
			if got := claudePluginOverrides(t, args); !reflect.DeepEqual(got, want) {
				t.Fatalf("plugin overrides = %v, want %v", got, want)
			}
			for flag, value := range map[string]string{
				"--permission-mode": "plan", "--permission-prompt-tool": "stdio",
				"--model": "opus", "--effort": "high", "--resume": "resume-id",
				"--input-format": "stream-json", "--output-format": "stream-json",
				// Every isolation mode removes the native subagent tool alone.
				"--disallowedTools":           "Agent,Task",
				"--append-system-prompt-file": "/sess/instructions.md",
			} {
				if got, ok := flagValue(args, flag); !ok || got != value {
					t.Errorf("%s = %q (present %v), want %q", flag, got, ok, value)
				}
			}
			server := leadMCPServer(t, args)
			if server["type"] != "stdio" || server["command"] != "/bin/ws-mcp" || !reflect.DeepEqual(server["args"], []any{"serve", "--stdio"}) ||
				!reflect.DeepEqual(server["env"], map[string]any{"WS_MCP_PROJECT_ROOT": spec.Root}) {
				t.Fatalf("the ws server is the daemon's stdio ws-mcp: %v", server)
			}
			// The option takes every following argument: the ws rule is
			// its only one, and nothing follows it.
			if n := len(args); n < 2 || !slices.Equal(args[n-2:], []string{"--allowedTools", "mcp__ws"}) || slices.Index(args, "--allowedTools") != n-2 {
				t.Fatalf("the ws allow rule must close the command line, alone: %v", args)
			}
			for _, banned := range []string{"--plugin-dir", "--setting-sources", "--strict-mcp-config", "--tools", "--disable-slash-commands", "--append-system-prompt", "--system-prompt", "--bare"} {
				if slices.Contains(args, banned) {
					t.Errorf("lead must load no bundle and preserve settings, hooks, memory, and MCP: found %s", banned)
				}
			}
		})
	}
}

// TestClaudeLeadMCPArgs: a hosted Claude lead's ws server is the daemon's
// HTTP route when the start has a session (over the stdio command it also
// carries), the daemon's stdio ws-mcp without one; with neither, its
// command line is refused, naming the missing ws-mcp.
func TestClaudeLeadMCPArgs(t *testing.T) {
	spec := claudePluginLeadSpec(t)
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp", Args: []string{"serve", "--stdio"}}
	spec.MCPSession = &MCPSession{URL: "http://127.0.0.1:1/mcp", Token: "secret-token"}
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if server := leadMCPServer(t, args); !reflect.DeepEqual(server, map[string]any{"type": "http", "url": "http://127.0.0.1:1/mcp",
		"headers": map[string]any{"Authorization": "Bearer ${" + EnvMCPSessionToken + "}"}}) {
		t.Fatalf("with a session the ws server is the HTTP route: %v", server)
	}
	if strings.Contains(strings.Join(args, " "), "secret-token") {
		t.Fatalf("the token stays out of argv: %v", args)
	}

	spec.MCPSession = nil
	args, err = claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if server := leadMCPServer(t, args); server["type"] != "stdio" || server["command"] != "/bin/ws-mcp" {
		t.Fatalf("without a session the ws server is the stdio ws-mcp: %v", server)
	}

	// Start's refusal: TestClaudeLeadStartRefusesPluginResolutionErrors.
	spec.WSMCP = nil
	if args, err := claudeArgs(spec); err == nil || args != nil || !strings.Contains(err.Error(), "no ws-mcp to attach") {
		t.Fatalf("a lead with no ws-mcp must be refused: %v, %v", args, err)
	}
}

func TestClaudeLeadPluginSettingsLocations(t *testing.T) {
	leadBundledFixture(t, "ws")
	spec := claudePluginLeadSpec(t)
	home := t.TempDir()
	homeKey := "HOME"
	if runtime.GOOS == "windows" {
		homeKey = "USERPROFILE"
	}
	writeLeadPluginFixture(t, filepath.Join(home, ".claude", "settings.json"), `{"enabledPlugins":{"ws@home":true}}`)
	spec.Env = []string{homeKey + "=" + home}
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := claudePluginOverrides(t, args); !reflect.DeepEqual(got, map[string]bool{"ws@home": false}) {
		t.Fatalf("home settings not read: %v", got)
	}

	// An explicit config directory replaces ~/.claude, and the lead's cwd
	// rather than the daemon/root selects project and local settings.
	spec.Cwd = t.TempDir()
	spec.Lead.Env = map[string]string{"CLAUDE_CONFIG_DIR": "custom-config"}
	writeLeadPluginFixture(t, filepath.Join(spec.Cwd, "custom-config", "settings.json"), `{"enabledPlugins":{"wsflow@custom":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Root, ".claude", "settings.json"), `{"enabledPlugins":{"ws@wrong-root":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Cwd, ".claude", "settings.json"), `{"enabledPlugins":{"ws@cwd":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Cwd, ".claude", "settings.local.json"), `{"enabledPlugins":{"wsflow@local":true}}`)
	args, err = claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"wsflow@custom": false, "ws@cwd": false, "wsflow@local": false}
	if got := claudePluginOverrides(t, args); !reflect.DeepEqual(got, want) {
		t.Fatalf("custom/cwd settings = %v, want %v", got, want)
	}
}

func TestClaudeLeadPluginSettingsNestedCwd(t *testing.T) {
	leadBundledFixture(t, "ws")
	spec := claudePluginLeadSpec(t)
	spec.Cwd = filepath.Join(spec.Root, "src", "nested")
	if err := os.MkdirAll(filepath.Join(spec.Root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeLeadPluginFixture(t, filepath.Join(spec.Root, ".claude", "settings.json"), `{"enabledPlugins":{"ws@root-project-not-loaded":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Root, ".claude", "settings.local.json"), `{"enabledPlugins":{"ws@root-local":true,"ws@overridden":false}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Root, "src", ".claude", "settings.local.json"), `{"enabledPlugins":{"ws@intermediate-not-loaded":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Cwd, ".claude", "settings.json"), `{"enabledPlugins":{"ws@cwd-project":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Cwd, ".claude", "settings.local.json"), `{"enabledPlugins":{"ws@cwd-local":true,"ws@overridden":true}}`)
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"ws@cwd-project": false, "ws@cwd-local": false}
	if runtime.GOOS == "windows" {
		want["ws@overridden"] = false
	} else {
		want["ws@root-local"] = false
	}
	if got := claudePluginOverrides(t, args); !reflect.DeepEqual(got, want) {
		t.Fatalf("nested cwd settings = %v, want %v", got, want)
	}
}

func TestClaudeLeadLocalSettingsCanonicalWorktree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Claude does not canonicalize the local store on Windows")
	}
	mainRoot, worktree, home := t.TempDir(), t.TempDir(), t.TempDir()
	gitDir := filepath.Join(mainRoot, ".git", "worktrees", "linked")
	writeLeadPluginFixture(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")
	writeLeadPluginFixture(t, filepath.Join(gitDir, "commondir"), "../..\n")
	writeLeadPluginFixture(t, filepath.Join(gitDir, "gitdir"), filepath.Join(worktree, ".git")+"\n")
	cwd := filepath.Join(worktree, "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := claudeLeadLocalSettingsRoot(cwd, home); got != mainRoot {
		t.Fatalf("canonical local store = %q, want %q", got, mainRoot)
	}
	if got := claudeLeadLocalSettingsRoot(cwd, mainRoot); got != "" {
		t.Fatalf("home repository must not become the local store: %s", got)
	}
}

func TestClaudeLeadLocalSettingsSymlinkedRootAndHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Claude does not canonicalize the local store on Windows")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{root, alias} {
		for _, home := range []string{root, alias} {
			if got := claudeLeadLocalSettingsRoot(filepath.Join(repo, "nested"), home); got != "" {
				t.Fatalf("home repository %q with home %q must not become the local store: %s", repo, home, got)
			}
		}
	}
}

func TestClaudeLeadPluginSettingsDoNotWalkNonGitAncestors(t *testing.T) {
	leadBundledFixture(t, "ws")
	spec := claudePluginLeadSpec(t)
	spec.Cwd = filepath.Join(spec.Root, "nested")
	writeLeadPluginFixture(t, filepath.Join(spec.Root, ".claude", "settings.json"), `{"enabledPlugins":{"ws@ancestor-project":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Root, ".claude", "settings.local.json"), `{"enabledPlugins":{"ws@ancestor-local":true}}`)
	writeLeadPluginFixture(t, filepath.Join(spec.Cwd, ".claude", "settings.json"), `{"enabledPlugins":{"ws@cwd":true}}`)
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := claudePluginOverrides(t, args); !reflect.DeepEqual(got, map[string]bool{"ws@cwd": false}) {
		t.Fatalf("non-git ancestor settings must not load: %v", got)
	}
}

func TestClaudeLeadPluginArgsEmptySettings(t *testing.T) {
	spec := claudePluginLeadSpec(t)
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := claudePluginOverrides(t, args); len(got) != 0 {
		t.Fatalf("a lead disables no builtins or unrelated plugins: %v", got)
	}
	if slices.Contains(args, "--plugin-dir") {
		t.Fatalf("a lead loads no bundle with empty settings: %v", args)
	}
}

func TestClaudeLeadPluginSettingsRefuseUnreadableOrMalformed(t *testing.T) {
	leadBundledFixture(t, "ws")
	for _, source := range []string{"user", "project", "local"} {
		for _, contents := range []string{"directory", `{`, `{"enabledPlugins":{"ws@market":"yes"}}`} {
			t.Run(source+"/"+contents, func(t *testing.T) {
				spec := claudePluginLeadSpec(t)
				path := filepath.Join(strings.TrimPrefix(spec.Env[0], "CLAUDE_CONFIG_DIR="), "settings.json")
				if source == "project" {
					path = filepath.Join(spec.Root, ".claude", "settings.json")
				} else if source == "local" {
					path = filepath.Join(spec.Root, ".claude", "settings.local.json")
				}
				if contents == "directory" {
					if err := os.MkdirAll(path, 0o755); err != nil {
						t.Fatal(err)
					}
				} else {
					writeLeadPluginFixture(t, path, contents)
				}
				if args, err := claudeArgs(spec); err == nil || args != nil || !strings.Contains(err.Error(), path) {
					t.Fatalf("bad plugin settings must refuse launch naming source: %v, %v", args, err)
				}
			})
		}
	}
}
