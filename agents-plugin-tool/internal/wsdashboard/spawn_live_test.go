package wsdashboard

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestPiSpawnDisplayStatus(t *testing.T) {
	for _, tc := range []struct {
		name, record, file, want string
	}{
		{"closed retained", `{"backend":"pi","closed_at":"2026-11-18"}`, "regular", "sleep"},
		{"closed missing", `{"backend":"pi","closed_at":"2026-11-18"}`, "", "deleted"},
		{"closed nonregular", `{"backend":"pi","closed_at":"2026-11-18"}`, "directory", "deleted"},
		{"open before first file", `{"backend":"pi"}`, "", "sleep"},
		{"unreadable record", `{`, "", "sleep"},
		{"removed directory", "", "", "deleted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			childDir := SessionDir(dir, "child")
			if tc.record != "" {
				if err := os.MkdirAll(childDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(childDir, AgentRecordFileName), []byte(tc.record), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			file := filepath.Join(childDir, "pi", "session.jsonl")
			if tc.file != "" {
				if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
					t.Fatal(err)
				}
				var err error
				if tc.file == "directory" {
					err = os.Mkdir(file, 0o700)
				} else {
					err = os.WriteFile(file, []byte("session\n"), 0o600)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			lookup := (pageReader{dir: dir}).lookup()
			row := wslive.Agent{ID: "pi:child", ReplyID: "child", AgentKind: wslive.AgentKindPi, Status: wslive.StatusDone}
			node := agentTree([]wslive.Agent{row}, lookup)[0]
			if node.EndedStatus() != tc.want || node.Status != wslive.StatusDone || node.IsRunning() {
				t.Fatalf("node = %+v, display = %q, want %q with legacy done status", node, node.EndedStatus(), tc.want)
			}
			html := renderNamed(t, "agent", node)
			if !strings.Contains(html, `class="agent done"`) || !strings.Contains(html, `class="chip astatus">`+tc.want+`</span>`) || !strings.Contains(html, `<span class="chip">`+tc.want+`</span>`) {
				t.Fatalf("row/hover lost presentation or legacy CSS: %s", html)
			}
			// Presentation is never written back to the shared live schema.
			raw, err := json.Marshal(node.Agent)
			if err != nil || strings.Contains(string(raw), tc.want) || row.Status != wslive.StatusDone {
				t.Fatalf("presentation leaked into snapshot: %s, %v", raw, err)
			}
		})
	}
}

func TestPiSpawnDisplayRefreshesAfterSessionRemoval(t *testing.T) {
	r, cache := overlayConsumerFixture(t)
	childDir := SessionDir(r.dir, "child")
	file := filepath.Join(childDir, "pi", "session.jsonl")
	record := filepath.Join(childDir, AgentRecordFileName)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte(`{"backend":"pi","closed_at":"2026-11-18"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("session\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap := liveSnap(testNow, wslive.Lead{}, wslive.Agent{ID: "pi:child", ReplyID: "child", AgentKind: wslive.AgentKindPi, Status: wslive.StatusDone})
	cache.SetLive(r.dir, "peer", *snap)
	view, inputs, ok := r.readSession("peer")
	if !ok || len(view.Tree.Agents) != 1 || view.Tree.Agents[0].EndedStatus() != "sleep" {
		t.Fatalf("initial view = %+v, ok = %v", view.Tree.Agents, ok)
	}
	if !slices.Contains(inputs.watch, file) || !slices.Contains(inputs.watch, record) {
		t.Fatalf("revival files absent from refresh inputs: %v", inputs.watch)
	}
	var before, after bytes.Buffer
	stampWatchedSince(&before, inputs.watch, time.Now())
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	stampWatchedSince(&after, inputs.watch, time.Now())
	if before.String() == after.String() {
		t.Fatal("session-file deletion did not change the refresh stamp")
	}
	view, _, ok = r.readSession("peer")
	if !ok || view.Tree.Agents[0].EndedStatus() != "deleted" || snap.Agents[0].Status != wslive.StatusDone {
		t.Fatalf("refreshed view = %+v, ok = %v", view.Tree.Agents, ok)
	}
}

func TestPiSpawnDisplayLeavesNativeAndActiveRowsAlone(t *testing.T) {
	lookup := sessionLookup{piDeleted: func(string) bool {
		t.Fatal("revival check for a native, failed, or active row")
		return true
	}}
	for _, tc := range []struct {
		kind, status, want string
	}{
		{"", wslive.StatusDone, "done"}, // Claude/Codex
		{"", wslive.StatusFailed, "failed"},
		{wslive.AgentKindPi, wslive.StatusFailed, "failed"},
		{wslive.AgentKindPi, wslive.StatusRunning, ""},
	} {
		node := agentTree([]wslive.Agent{{ID: "child", ReplyID: "child", AgentKind: tc.kind, Status: tc.status}}, lookup)[0]
		if tc.want != "" && node.EndedStatus() != tc.want {
			t.Fatalf("kind %q status %q: %s", tc.kind, tc.status, node.EndedStatus())
		}
		if tc.kind == "" && strings.Contains(renderNamed(t, "agent", node), "astatus") {
			t.Fatal("native row acquired a Pi lifecycle chip")
		}
	}
	if piChildDeleted(t.TempDir(), "../outside") {
		t.Fatal("unsafe reply id accepted")
	}
}
