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

func claudePluginLeadSpec(t *testing.T) StartSpec {
	t.Helper()
	return StartSpec{
		Root: t.TempDir(),
		Env:  []string{"CLAUDE_CONFIG_DIR=" + t.TempDir()},
		Lead: &LeadSpec{},
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

func TestClaudeLeadPluginArgsModes(t *testing.T) {
	bundle := leadBundledFixture(t, "wsflow")
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
	spec.Instructions = "/must/not/append"
	spec.WSMCP = &MCPCommand{Command: "/must/not/replace/user/mcp"}
	for _, mode := range []string{"", "default", "isolated"} {
		t.Run("mode="+mode, func(t *testing.T) {
			spec.Lead.PluginIsolation = mode
			args, err := claudeArgs(spec)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]bool{"wsflow@project-market": false, "ws@local-market": false}
			if mode == "isolated" {
				want["clangd@tools"], want["ws-extra@tools"] = false, false
				for _, id := range claudeLeadBuiltinPlugins {
					want[id] = false
				}
			}
			if got := claudePluginOverrides(t, args); !reflect.DeepEqual(got, want) {
				t.Fatalf("plugin overrides = %v, want %v", got, want)
			}
			for flag, value := range map[string]string{
				"--plugin-dir": bundle, "--permission-mode": "plan", "--permission-prompt-tool": "stdio",
				"--model": "opus", "--effort": "high", "--resume": "resume-id",
				"--input-format": "stream-json", "--output-format": "stream-json",
			} {
				if got, ok := flagValue(args, flag); !ok || got != value {
					t.Errorf("%s = %q (present %v), want %q", flag, got, ok, value)
				}
			}
			if n := slices.Index(args, "--plugin-dir"); n < 0 || slices.Contains(args[n+1:], "--plugin-dir") {
				t.Fatalf("want one bundled plugin directory: %v", args)
			}
			for _, banned := range []string{"--setting-sources", "--strict-mcp-config", "--mcp-config", "--tools", "--allowedTools", "--disable-slash-commands", "--append-system-prompt-file", "--bare"} {
				if slices.Contains(args, banned) {
					t.Errorf("lead must preserve settings, hooks, memory, and MCP: found %s", banned)
				}
			}
		})
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
	bundle := leadBundledFixture(t, "ws")
	spec := claudePluginLeadSpec(t)
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := claudePluginOverrides(t, args); len(got) != 0 {
		t.Fatalf("default mode disables no builtins or unrelated plugins: %v", got)
	}
	if got, _ := flagValue(args, "--plugin-dir"); got != bundle {
		t.Fatalf("bundled plugin missing with empty settings: %v", args)
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
