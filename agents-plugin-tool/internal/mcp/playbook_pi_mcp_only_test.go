package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// A Pi session without the native ws Pi extension renders the daemon's
// agent tools (agent.spawn and mailbox.send) as its idioms; a native Pi
// session keeps the extension's tools. These
// tests pin both rows and drive the switch from the caller's session key
// through the daemon's conversation record to the rendered playbook.

const (
	piMCPOnlySpawn    = "`ws/agent.spawn` (tier: the resolved capability tier, not a model name; authority: at most your own)"
	piMCPOnlyExplore  = "a `ws/agent.spawn` subagent at tier small with read-only authority"
	piMCPOnlyContinue = "ws/mailbox.send to the address agent.spawn returned"
)

func TestPlaybookTerminologyPiMCPOnlyRow(t *testing.T) {
	want := map[string]string{
		"ExploreAgent":  piMCPOnlyExplore,
		"SpawnIdiom":    piMCPOnlySpawn,
		"ContinueIdiom": piMCPOnlyContinue,
	}
	// The MCP-only row wins whatever harness the connection was detected as.
	for _, harness := range []string{"", "pi", "claude", "codex"} {
		term := playbookTerminology(harness, true, "ws")
		for name, value := range want {
			if term[name] != value {
				t.Errorf("harness %q: MCP-only %s = %q, want %q", harness, name, term[name], value)
			}
		}
	}
	// Without the MCP-only signal the harness row is unchanged, the native
	// pi row included.
	for _, harness := range []string{"", "pi", "claude", "codex"} {
		got := playbookTerminology(harness, false, "ws")
		native := terminologyForHarness(harness, "ws")
		for name := range want {
			if got[name] != native[name] {
				t.Errorf("harness %q: %s = %q, want the harness row's %q", harness, name, got[name], native[name])
			}
		}
	}
	if term := playbookTerminology("", true, "wsflow"); term["SpawnIdiom"] != strings.ReplaceAll(piMCPOnlySpawn, "ws/", "wsflow/") {
		t.Errorf("wsflow MCP-only SpawnIdiom = %q, want the wsflow namespace", term["SpawnIdiom"])
	}
}

// piRecordFixture is a lead session on the shipped rsrc tree whose daemon
// conversation records the test writes.
type piRecordFixture struct {
	s       *Server
	root    string
	leadKey string
	dashDir string
}

func newPiRecordFixture(t *testing.T) *piRecordFixture {
	t.Helper()
	t.Setenv(wsagent.EnvPiToken, "test-token")
	t.Setenv(wsagent.EnvPiEndpoint, "http://127.0.0.1:1")
	s, root, key := setupPlanRenderServer(t, "ws")
	dir, err := s.dashboard.stateDir()
	if err != nil {
		t.Fatal(err)
	}
	return &piRecordFixture{s: s, root: root, leadKey: key, dashDir: dir}
}

// record writes the daemon's conversation record naming key.
func (f *piRecordFixture) record(t *testing.T, key string, rec wsagent.Record) {
	t.Helper()
	rec.ReplyID = wsmailbox.ReplyID(key)
	rec.WSKey = key
	rec.Root = f.root
	rec.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := wsagent.WriteRecordForTest(f.dashDir, rec); err != nil {
		t.Fatal(err)
	}
}

// spawnedKey mints a Pi subagent's key as the runner's key store does and
// records it as an agent.spawn child's conversation with profile def.
func (f *piRecordFixture) spawnedKey(t *testing.T, def *wsagent.Profile) string {
	t.Helper()
	key, err := NewAgentKeyStore().Mint(f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.record(t, key, wsagent.Record{Backend: wsagent.BackendPi, Profile: wsagent.AgentWorkerProfile(wsagent.BackendPi), ResolvedDefinition: def})
	return key
}

// renderedBody renders name through the playbook.render tool after completing
// the initialize requests given, and returns the rendered file's text.
func renderedBody(t *testing.T, s *Server, key, name string, initialize ...string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"name": name, "session_key": key})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// ServeStdio dispatches requests concurrently. Complete the MCP handshake
	// before rendering so the tool cannot race harness detection.
	for _, input := range initialize {
		if err := s.ServeStdio(context.Background(), strings.NewReader(input+"\n"), &out); err != nil {
			t.Fatalf("ServeStdio initialize: %v", err)
		}
		out.Reset()
	}
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":99,"method":"tools/call","params":{"name":"playbook.render","arguments":%s}}`+"\n", raw)
	if err := s.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	path := renderLines(t, lines[len(lines)-1])[0]
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook %s: %v", path, err)
	}
	return string(body)
}

func assertPiMCPOnlyBody(t *testing.T, body string) {
	t.Helper()
	for _, want := range []string{piMCPOnlySpawn, piMCPOnlyExplore, "`" + piMCPOnlyContinue + "`", "`ws/config.resolve_agent(tier)`"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q:\n%s", want, body)
		}
	}
	for _, banned := range []string{"ws-agent-spawn", "ws-agent-send", "the `explore` researcher", "Agent({", "pi.spawn", "pi.send"} {
		if strings.Contains(body, banned) {
			t.Errorf("rendered body names %q, a tool an MCP-only Pi session lacks", banned)
		}
	}
}

// TestPlaybookRenderPiSpawnedCallerUsesMCPOnlyIdioms: an agent.spawn Pi
// child (no extension, so its connection is not detected as pi) rendering
// the elevated worker playbook is told agent.spawn with the resolved tier.
func TestPlaybookRenderPiSpawnedCallerUsesMCPOnlyIdioms(t *testing.T) {
	f := newPiRecordFixture(t)
	key := f.spawnedKey(t, &wsagent.Profile{Backend: wsagent.BackendPi, WSTools: []string{"agent.spawn", "mailbox.send"}})
	assertPiMCPOnlyBody(t, renderedBody(t, f.s, key, "ticket-worker-elevated"))
}

// TestPlaybookRenderPiSpawnedCallerWithoutSnapshotResolvesProfile: a record
// written before the launched-profile snapshot falls back to its profile
// name, whose builtin Pi agent worker loads no extension.
func TestPlaybookRenderPiSpawnedCallerWithoutSnapshotResolvesProfile(t *testing.T) {
	f := newPiRecordFixture(t)
	key := f.spawnedKey(t, nil)
	assertPiMCPOnlyBody(t, renderedBody(t, f.s, key, "ticket-worker-elevated"))
}

// TestPlaybookRenderHostedPiLeadSwitchesOnBridgeFallback: a hosted Pi lead
// on the MCP bridge gets the MCP-only row, and the same lead on its bundled
// extension (connection detected as pi through the extension's clientInfo)
// keeps the native row.
func TestPlaybookRenderHostedPiLeadSwitchesOnBridgeFallback(t *testing.T) {
	f := newPiRecordFixture(t)
	f.record(t, f.leadKey, wsagent.Record{Backend: wsagent.BackendPi, Lead: true, PluginWarning: "Bundled Pi extension unavailable (missing); using the ws MCP bridge instead."})
	assertPiMCPOnlyBody(t, renderedBody(t, f.s, f.leadKey, "ticket-worker-elevated"))

	f.record(t, f.leadKey, wsagent.Record{Backend: wsagent.BackendPi, Lead: true})
	body := renderedBody(t, f.s, f.leadKey, "ticket-worker-elevated",
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ws-pi-bridge","version":"test"}}}`)
	native := terminologyForHarness("pi", "ws")
	for _, name := range []string{"SpawnIdiom", "ExploreAgent", "ContinueIdiom"} {
		if !strings.Contains(body, native[name]) {
			t.Errorf("native Pi lead body missing native %s %q", name, native[name])
		}
	}
	if strings.Contains(body, "agent.spawn") {
		t.Errorf("unmarked Pi lead body names agent.spawn:\n%s", body)
	}
}

// TestPlaybookRenderUnrecordedCallerKeepsHarnessRow: a key with no daemon
// record (a terminal session) renders its connection's harness row.
func TestPlaybookRenderUnrecordedCallerKeepsHarnessRow(t *testing.T) {
	f := newPiRecordFixture(t)
	body := renderedBody(t, f.s, f.leadKey, "ticket-worker-elevated")
	if strings.Contains(body, "agent.spawn") {
		t.Errorf("unrecorded caller body names agent.spawn:\n%s", body)
	}
	if !strings.Contains(body, terminologyForHarness("", "ws")["SpawnIdiom"]) {
		t.Errorf("unrecorded caller body missing the host-neutral SpawnIdiom")
	}
}

// TestPlaybookReadPiSpawnedCallerUsesMCPOnlyIdioms: playbook.read carries the
// caller key to the terminology switch as playbook.render does.
func TestPlaybookReadPiSpawnedCallerUsesMCPOnlyIdioms(t *testing.T) {
	f := newPiRecordFixture(t)
	key := f.spawnedKey(t, &wsagent.Profile{Backend: wsagent.BackendPi, WSTools: []string{"agent.spawn", "mailbox.send"}})
	resp := callToolOnce(t, f.s, 2, "playbook.read", map[string]any{"name": "ticket-worker-elevated", "session_key": key})
	if toolIsError(t, resp) {
		t.Fatalf("playbook.read returned error: %s", resp)
	}
	assertPiMCPOnlyBody(t, toolText(t, resp))
}
