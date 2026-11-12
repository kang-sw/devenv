package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

// fakeTokenRoute is the dashboard daemon's token route as a Pi worker's
// ws-mcp reaches it: it records each call and answers per op.
type fakeTokenRoute struct {
	mu    sync.Mutex
	calls []fakeTokenCall
}

type fakeTokenCall struct {
	path, auth string
	body       map[string]any
}

func (f *fakeTokenRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.calls = append(f.calls, fakeTokenCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body})
	f.mu.Unlock()
	if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer tok123" {
		http.Error(w, "the capability token is not valid", http.StatusUnauthorized)
		return
	}
	answer := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	kid := wsagent.State{ID: "kid1", Status: wsagent.StatusRunning, Root: "/repo", Model: "prov/small", Effort: "low", Tier: "small", Label: "sub"}
	switch strings.TrimPrefix(r.URL.Path, wsagent.RoutePrefix+"/by-token/") {
	case "spawn":
		if body["tier"] == "large" {
			http.Error(w, `pi.spawn: tier "large" is above this conversation's own tier "medium", which caps its subagents' tier; pick "medium" or lower`, http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusCreated)
		answer(kid)
	case "list":
		answer(map[string]any{"conversations": []wsagent.State{kid}})
	case "send", "stop":
		answer(kid)
	case "transcript":
		answer(map[string]any{"records": []any{map[string]any{"kind": "reply", "text": "kid says hi", "at": "2026-11-13T00:00:00Z"}}})
	case "wait":
		answer(wsagent.TurnResult{ID: "kid1", Status: wsagent.TurnDone, Message: "kid finished"})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeTokenRoute) last() fakeTokenCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// TestPiWorkerForwardsPiToolsOnItsToken: a Pi worker's attached ws-mcp
// (no runner, a delegate key) lists the pi.* tools with the worker's
// descriptions and forwards each call to the daemon's token route on its
// token; every other lead-only tool stays refused to its key, and without
// a token pi.* is refused like any lead-only tool.
func TestPiWorkerForwardsPiToolsOnItsToken(t *testing.T) {
	// The worker's server checks neither the knob nor the Pi version: the
	// daemon does.
	setPiVersion(t, "", errors.New("no pi here"))
	setupMailboxTestEnv(t)
	route := &fakeTokenRoute{}
	daemon := httptest.NewServer(route)
	defer daemon.Close()
	t.Setenv(envAllowedTools, "git.status,workflow_state,pi.spawn,pi.send,pi.list,pi.stop,pi.transcript,pi.wait")
	root := t.TempDir()
	initGit(t, root)

	// Without a token: pi.* is a lead-only tool to a delegate key.
	plain := NewServer(root, "test")
	defer plain.Close()
	plainKey, err := plain.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := listedPiTools(plain); len(got) != 0 {
		t.Fatalf("no token lists no pi.* tool: %v", got)
	}
	if raw := callToolOnce(t, plain, 2, "pi.list", map[string]any{"session_key": plainKey}); !strings.Contains(raw, "tool not available in current") {
		t.Fatalf("no token refuses pi.*: %s", raw)
	}

	t.Setenv(wsagent.EnvPiToken, "tok123")
	t.Setenv(wsagent.EnvPiEndpoint, daemon.URL+"/")
	s := NewServer(root, "test")
	defer s.Close()
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := listedPiTools(s); strings.Join(got, ",") != strings.Join(piToolNames, ",") {
		t.Fatalf("listed = %v", got)
	}
	for _, tool := range s.filteredTools() {
		if name, _ := tool["name"].(string); name == "pi.spawn" {
			if desc, _ := tool["description"].(string); !strings.Contains(desc, "Spawn a Pi subagent of your own") || strings.Contains(desc, "mailbox") {
				t.Fatalf("pi.spawn reads as a worker's: %q", desc)
			}
			props, _ := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
			if cwd := props["cwd"].(map[string]string)["description"]; !strings.Contains(cwd, "relative to your own working directory") || strings.Contains(cwd, "root") {
				t.Fatalf("pi.spawn's cwd reads as a worker's: %q", cwd)
			}
		}
	}

	out := callToolWithKey(t, s, 3, key, "pi.spawn", map[string]any{"prompt": "sub task", "tier": "small", "cwd": "sub", "label": "sub"})
	if !strings.Contains(out, "spawned Pi subagent kid1 (tier small: prov/small, effort low)") || !strings.Contains(out, "steered into your running turn") {
		t.Fatalf("pi.spawn = %q", out)
	}
	call := route.last()
	if call.path != "/agents/by-token/spawn" || call.auth != "Bearer tok123" || call.body["prompt"] != "sub task" || call.body["tier"] != "small" || call.body["cwd"] != "sub" || call.body["label"] != "sub" {
		t.Fatalf("forwarded spawn = %+v", call)
	}
	if out := callToolWithKey(t, s, 4, key, "pi.spawn", map[string]any{"prompt": "x", "tier": "large"}); !strings.Contains(out, `tier "large" is above this conversation's own tier "medium"`) {
		t.Fatalf("a daemon refusal reaches the worker: %q", out)
	}
	if out := callToolWithKey(t, s, 5, key, "pi.list", nil); !strings.Contains(out, "kid1  running  prov/small/low") {
		t.Fatalf("pi.list = %q", out)
	}
	if out := callToolWithKey(t, s, 6, key, "pi.send", map[string]any{"id": "kid1", "message": "more"}); !strings.Contains(out, "sent to Pi subagent kid1") || route.last().body["message"] != "more" {
		t.Fatalf("pi.send = %q", out)
	}
	if out := callToolWithKey(t, s, 7, key, "pi.transcript", map[string]any{"id": "kid1"}); !strings.Contains(out, "kid says hi") {
		t.Fatalf("pi.transcript = %q", out)
	}
	if out := callToolWithKey(t, s, 8, key, "pi.wait", map[string]any{"id": "kid1", "timeout_s": 5}); !strings.Contains(out, "Pi subagent kid1: turn done") || !strings.Contains(out, "kid finished") || route.last().body["timeout_s"] != float64(5) {
		t.Fatalf("pi.wait = %q (%+v)", out, route.last())
	}
	if out := callToolWithKey(t, s, 9, key, "pi.stop", map[string]any{"id": "kid1"}); !strings.Contains(out, "closed Pi subagent kid1") || route.last().path != "/agents/by-token/stop" {
		t.Fatalf("pi.stop = %q", out)
	}

	// The token authorizes pi.* alone: another lead-only tool stays
	// refused to the worker's delegate key.
	if raw := callToolOnce(t, s, 10, "workflow_state", map[string]any{"session_key": key}); !strings.Contains(raw, "tool not available in current") {
		t.Fatalf("a lead-only tool with a token: %s", raw)
	}

	// An unreachable daemon is named.
	daemon.Close()
	if out := callToolWithKey(t, s, 11, key, "pi.list", nil); !strings.Contains(out, "the dashboard daemon") || !strings.Contains(out, "did not answer") {
		t.Fatalf("an unreachable daemon = %q", out)
	}
}
