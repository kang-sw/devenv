package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// hostedEnvKeys are the variables a hosted test env sets itself; everything
// else is inherited from the test process (PATH, HOME) so git still runs.
var hostedEnvKeys = []string{
	envNamespace, envAllowedTools, envNoAgent, envDebugLog, envConfigManifests,
	envMailbox, envMailboxAuto, wsconfig.EnvConfigHome, wsstate.EnvCacheHome,
	wsrsrc.EnvRsrcRoot, "PATH",
}

// hostedEnv is the test process's environment with hostedEnvKeys replaced by
// set (a key absent from set is dropped).
func hostedEnv(set map[string]string) []string {
	drop := map[string]bool{}
	for _, key := range hostedEnvKeys {
		drop[key] = true
	}
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if drop[key] {
			continue
		}
		env = append(env, kv)
	}
	for key, value := range set {
		env = append(env, key+"="+value)
	}
	return env
}

// processEnvFixture sets the test process's own environment to a third set
// of values a hosted server must never read, and returns its cache home.
func processEnvFixture(t *testing.T) (cacheHome string) {
	t.Helper()
	cacheHome = filepath.Join(t.TempDir(), "proc-cache")
	t.Setenv(envNamespace, "nsproc")
	t.Setenv(envAllowedTools, "git.log")
	t.Setenv(envNoAgent, "1")
	t.Setenv(envDebugLog, filepath.Join(t.TempDir(), "proc.log"))
	t.Setenv(envConfigManifests, writeManifest(t, "proc.json", `{"schema_version":1,"namespace":"proc.","keys":[{"key":"proc.label","type":"string","default":"Proc","description":"d"}]}`))
	t.Setenv(envMailbox, "")
	t.Setenv(envMailboxAuto, "")
	t.Setenv(wsconfig.EnvConfigHome, filepath.Join(t.TempDir(), "proc-config"))
	t.Setenv(wsstate.EnvCacheHome, cacheHome)
	return cacheHome
}

func absRsrcRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestHostedServersReadOnlyTheirOwnEnvironment pins the per-connection
// seam: two servers built from different hello envs each see only their own
// namespace, allowlist, agentless mode, config and cache homes, config
// manifests, and debug ring, never the other's and never this process's.
func TestHostedServersReadOnlyTheirOwnEnvironment(t *testing.T) {
	procCache := processEnvFixture(t)
	procLog := os.Getenv(envDebugLog)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)

	type hosted struct {
		name, cacheHome, configHome, debugLog string
		server                                *Server
	}
	mk := func(name string, extra map[string]string, manifest string) hosted {
		h := hosted{
			name:       name,
			cacheHome:  filepath.Join(t.TempDir(), name+"-cache"),
			configHome: filepath.Join(t.TempDir(), name+"-config"),
			debugLog:   filepath.Join(t.TempDir(), name+".log"),
		}
		set := map[string]string{
			envNamespace:           "ns" + name,
			envDebugLog:            h.debugLog,
			envConfigManifests:     manifest,
			wsconfig.EnvConfigHome: h.configHome,
			wsstate.EnvCacheHome:   h.cacheHome,
			wsrsrc.EnvRsrcRoot:     absRsrcRoot(t),
			"PATH":                 os.Getenv("PATH"),
		}
		for key, value := range extra {
			set[key] = value
		}
		h.server = NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: hostedEnv(set), OwnerPID: os.Getppid()})
		return h
	}
	alpha := mk("alpha", map[string]string{
		envAllowedTools: "ferrule,workflow_manual,runtime.debug_events,config.get,config.tune,git.status",
		envMailbox:      "alpha@machine",
	}, writeManifest(t, "acme.json", testManifest))
	beta := mk("beta", map[string]string{
		envNoAgent: "1",
	}, writeManifest(t, "beta.json", `{"schema_version":1,"namespace":"beta.","keys":[{"key":"beta.label","type":"string","default":"World","description":"d"}]}`))

	// tools/list: allowlist and namespace come from each server's own env.
	alphaList := callToolsList(t, alpha.server)
	betaList := callToolsList(t, beta.server)
	if strings.Contains(alphaList, `"git.log"`) || !strings.Contains(alphaList, `"git.status"`) {
		t.Fatalf("alpha tools/list ignored its own allowlist: %s", alphaList)
	}
	if !strings.Contains(betaList, `"git.log"`) || !strings.Contains(betaList, `"tickets.query"`) {
		t.Fatalf("beta tools/list applied an allowlist it never declared: %s", betaList)
	}
	for _, c := range []struct {
		list, want string
		not        []string
	}{
		{alphaList, "nsalpha", []string{"nsbeta", "nsproc"}},
		{betaList, "nsbeta", []string{"nsalpha", "nsproc"}},
	} {
		if !strings.Contains(c.list, c.want) {
			t.Fatalf("tools/list missing own namespace %s: %s", c.want, c.list)
		}
		for _, other := range c.not {
			if strings.Contains(c.list, other) {
				t.Fatalf("tools/list (%s) carries foreign namespace %s", c.want, other)
			}
		}
	}

	// Agentless mode: only beta declared it.
	alphaExec := callToolOnce(t, alpha.server, 2, "exec.spawn", map[string]any{"cmd": "echo"})
	betaExec := callToolOnce(t, beta.server, 2, "exec.spawn", map[string]any{"cmd": "echo"})
	if strings.Contains(alphaExec, "agentless") || !strings.Contains(alphaExec, "tool not available in current nsalpha MCP profile") {
		t.Fatalf("alpha exec.spawn = %s", alphaExec)
	}
	if !strings.Contains(betaExec, "nsbeta agentless mode disables agent-backed tool: exec.spawn") {
		t.Fatalf("beta exec.spawn = %s", betaExec)
	}

	// Session key store: under alpha's cache home, not the process's.
	key, _ := parseLoginResponse(t, callLogin(t, alpha.server, 5, root, nil))
	if _, err := os.Stat(filepath.Join(alpha.cacheHome, sessionsDirName, key)); err != nil {
		t.Fatalf("alpha's session key not stored under its cache home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(procCache, sessionsDirName, key)); err == nil {
		t.Fatal("alpha's session key landed under the process cache home")
	}
	betaKey, _ := parseLoginResponse(t, callLogin(t, beta.server, 8, root, nil))

	// Debug ring and log: a precheck refusal (keyed, so the precheck runs)
	// lands in its own server only.
	keyedCall(t, alpha.server, 3, key, "git.status", map[string]any{"bogus_alpha": true})
	keyedCall(t, beta.server, 3, betaKey, "git.status", map[string]any{"bogus_beta": true})
	for _, c := range []struct {
		h         hosted
		want, not string
	}{{alpha, "bogus_alpha", "bogus_beta"}, {beta, "bogus_beta", "bogus_alpha"}} {
		ring := toolText(t, callToolOnce(t, c.h.server, 4, "runtime.debug_events", map[string]any{"limit": 256}))
		if !strings.Contains(ring, c.want) || strings.Contains(ring, c.not) {
			t.Fatalf("%s debug ring = %s", c.h.name, ring)
		}
		raw, err := os.ReadFile(c.h.debugLog)
		if err != nil || !strings.Contains(string(raw), c.want) || strings.Contains(string(raw), c.not) {
			t.Fatalf("%s debug log (err %v) = %s", c.h.name, err, raw)
		}
	}
	for _, event := range recentDebugEvents(maxDebugEvents) {
		if strings.Contains(strings.Join(nonNilStrings(anyStrings(event["unknown"])), ","), "bogus_") {
			t.Fatalf("a hosted server's event reached the process ring: %v", event)
		}
	}
	if raw, err := os.ReadFile(procLog); err == nil && strings.Contains(string(raw), "bogus_") {
		t.Fatalf("a hosted server's event reached the process debug log: %s", raw)
	}

	// Config manifests: alpha declares acme.*, beta does not, the process's
	// proc.* manifest reaches neither.
	if text := toolText(t, keyedCall(t, alpha.server, 6, key, "config.get", map[string]any{"key": "acme.label"})); !strings.Contains(text, "Hello") {
		t.Fatalf("alpha config.get acme.label = %s", text)
	}
	// An undeclared key reads <unset>; a declared one would read its default.
	if resp := keyedCall(t, alpha.server, 7, key, "config.get", map[string]any{"key": "proc.label"}); strings.Contains(resp, "Proc") {
		t.Fatalf("alpha resolved the process manifest's key: %s", resp)
	}
	if resp := keyedCall(t, beta.server, 9, betaKey, "config.get", map[string]any{"key": "acme.label"}); strings.Contains(resp, "Hello") {
		t.Fatalf("beta resolved alpha's manifest key: %s", resp)
	}
	if text := toolText(t, keyedCall(t, beta.server, 10, betaKey, "config.get", map[string]any{"key": "beta.label"})); !strings.Contains(text, "World") {
		t.Fatalf("beta config.get beta.label = %s", text)
	}

	// Config home: a global-scope write lands under alpha's WS_CONFIG_HOME.
	if resp := keyedCall(t, alpha.server, 11, key, "config.tune", map[string]any{"key": "acme.retention_days", "value": "7"}); toolIsError(t, resp) {
		t.Fatalf("alpha config.tune: %s", resp)
	}
	if raw, err := os.ReadFile(filepath.Join(alpha.configHome, "config.json")); err != nil || !strings.Contains(string(raw), "acme.retention_days") {
		t.Fatalf("alpha's global write missing from its config home (err %v): %s", err, raw)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv(wsconfig.EnvConfigHome), "config.json")); err == nil {
		t.Fatal("alpha's global write landed in the process config home")
	}

	// Mailbox presence records the owner PID, not this process's.
	path, err := wsmailbox.MachinePath()
	if err != nil {
		t.Fatal(err)
	}
	store, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	presence, ok := store.Presence["alpha"]
	if !ok {
		t.Fatalf("alpha@machine presence not registered: %#v", store.Presence)
	}
	if presence.PID != os.Getppid() || presence.PID == os.Getpid() {
		t.Fatalf("presence PID = %d, want owner PID %d (process %d)", presence.PID, os.Getppid(), os.Getpid())
	}
}

func anyStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// TestHostedServerSubprocessUsesHelloEnvironment pins that a git subprocess
// a hosted server starts resolves git from the hello env's PATH and runs
// under that env, not under this process's.
func TestHostedServerSubprocessUsesHelloEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git is a POSIX shell script")
	}
	processEnvFixture(t)
	t.Setenv(envAllowedTools, "")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)

	fakeDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do if [ \"$a\" = status ]; then echo \"FAKE-GIT-HOSTED $WS_HOSTED_MARK\" >&2; exit 3; fi; done\n" +
		"exec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(fakeDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	server := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: hostedEnv(map[string]string{
		"PATH":                 fakeDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"WS_HOSTED_MARK":       "hello-env",
		wsconfig.EnvConfigHome: filepath.Join(t.TempDir(), "config"),
		wsstate.EnvCacheHome:   filepath.Join(t.TempDir(), "cache"),
		wsrsrc.EnvRsrcRoot:     absRsrcRoot(t),
	})})
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	text := toolText(t, keyedCall(t, server, 2, key, "git.status", nil))
	if !strings.Contains(text, "FAKE-GIT-HOSTED hello-env") {
		t.Fatalf("hosted git.status did not run the hello env's git under the hello env: %s", text)
	}

	// The same call on a stdio server uses this process's PATH: real git.
	stdio := NewServer(root, "test")
	stdioKey, _ := parseLoginResponse(t, callLogin(t, stdio, 3, root, nil))
	if text := toolText(t, keyedCall(t, stdio, 4, stdioKey, "git.status", nil)); strings.Contains(text, "FAKE-GIT-HOSTED") {
		t.Fatalf("stdio git.status ran the hosted env's git: %s", text)
	}
}
