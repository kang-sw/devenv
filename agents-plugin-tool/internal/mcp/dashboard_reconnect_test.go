package mcp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// initializeDashboardProxy models the launcher's reconnect replay: initialize
// only, with no login or keyed tool call. Keep the stream open so EOF does not
// immediately end the restored rows again.
func initializeDashboardProxy(t *testing.T, s *Server) {
	t.Helper()
	in, client := io.Pipe()
	responses, out := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- s.ServeStdio(context.Background(), in, out)
	}()
	t.Cleanup(func() {
		_ = client.Close()
		_ = responses.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve loop: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("serve loop did not stop")
		}
		_ = in.Close()
		_ = out.Close()
	})
	if _, err := io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"claude-code","version":"0"}}}`+"\n"); err != nil {
		t.Fatal(err)
	}
	var reply response
	if err := json.NewDecoder(responses).Decode(&reply); err != nil || reply.Error != nil {
		t.Fatalf("initialize: %+v, %v", reply, err)
	}
}

func TestDashboardProxyReconnectRestoresLiveSessionsAfterDaemonRestart(t *testing.T) {
	for _, clean := range []bool{true, false} {
		t.Run(map[bool]string{true: "drained", false: "crashed"}[clean], func(t *testing.T) {
			dir := dashboardSessionEnv(t)
			t.Setenv(envHarnessPID, strconv.Itoa(os.Getppid()))
			root := dashboardGitRoot(t)
			old, key, _ := dashboardSessionServer(t, root, true)
			t.Cleanup(old.Close)
			old.ownerPID = 4242 // the launcher survives the daemon
			now := time.Now().UTC().Truncate(time.Second)
			old.dashboard.now = func() time.Time { return now }
			dashCall(t, old, "todo.list", map[string]any{"session_key": key})
			before, ok := readRegistration(t, dir, key)
			if !ok || before.MCPOwnerPID != old.ownerPID {
				t.Fatalf("registration did not persist its launcher: %+v", before)
			}
			if clean {
				old.dashboardEnd()
				ended, _ := readRegistration(t, dir, key)
				if ended.Live(now) || ended.EndedAt == "" {
					t.Fatalf("old daemon did not end its row: %+v", ended)
				}
			}

			now = now.Add(time.Minute)
			next := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: os.Environ(), OwnerPID: old.ownerPID, Harness: "claude"})
			t.Cleanup(next.Close)
			next.dashboard.now = func() time.Time { return now }
			initializeDashboardProxy(t, next)
			reg, _ := readRegistration(t, dir, key)
			if !reg.Live(now) || reg.EndedAt != "" || reg.HeartbeatAt != dashboardStamp(now) || reg.RegisteredAt != before.RegisteredAt || reg.LastCallAt != before.LastCallAt {
				t.Fatalf("reconnected row = %+v, before = %+v", reg, before)
			}

			// The successor's normal ticker must keep the row live beyond
			// the old heartbeat's window, still without a keyed call.
			now = now.Add(wsdashboard.LiveWindow + time.Minute)
			next.dashboardHeartbeat()
			reg, _ = readRegistration(t, dir, key)
			if !reg.Live(now) || reg.HeartbeatAt != dashboardStamp(now) {
				t.Fatalf("successor did not resume heartbeats: %+v", reg)
			}
			idx := wsdashboard.NewIndex(wsdashboard.IndexOptions{Dir: dir, Source: NewDashboardSessionSource(), Now: func() time.Time { return now }})
			idx.Refresh()
			row, found := idx.Session(wsmailbox.ReplyID(key))
			if !found || !row.Live {
				t.Fatalf("successor index still considers the CLI session ended: %+v, found=%v", row, found)
			}
			// Reconnect is not keyed activity: the original idle deadline
			// still stops the heartbeat, rather than gaining another 12h.
			lastCall, _ := time.Parse(time.RFC3339, before.LastCallAt)
			now = lastCall.Add(dashboardIdleCutoff)
			next.dashboardHeartbeat()
			idle, _ := readRegistration(t, dir, key)
			if idle.HeartbeatAt != reg.HeartbeatAt || idle.Live(now) {
				t.Fatalf("restart extended the idle budget: %+v", idle)
			}
		})
	}
}

func TestDashboardProxyReconnectDoesNotAdoptUnrelatedRows(t *testing.T) {
	for _, kind := range []string{"other launcher", "other harness", "hosted", "agent", "expired", "legacy", "wrong project"} {
		t.Run(kind, func(t *testing.T) {
			dir := dashboardSessionEnv(t)
			t.Setenv(envHarnessPID, strconv.Itoa(os.Getppid()))
			root := dashboardGitRoot(t)
			old, key, _ := dashboardSessionServer(t, root, true)
			t.Cleanup(old.Close)
			old.ownerPID = 4242
			dashCall(t, old, "todo.list", map[string]any{"session_key": key})
			old.dashboardEnd()
			reg, _ := readRegistration(t, dir, key)
			switch kind {
			case "other launcher":
				reg.MCPOwnerPID++
			case "other harness":
				reg.HarnessPID++
			case "hosted":
				reg.Hosted = true
			case "agent":
				reg.Agent = true
			case "expired":
				// A recent heartbeat is not recent keyed activity: a row
				// whose idle cutoff just fired must not be revived.
				reg.LastCallAt = dashboardStamp(time.Now().Add(-dashboardIdleCutoff - time.Minute))
			case "legacy":
				reg.MCPOwnerPID = 0
			case "wrong project":
				reg.Project = t.TempDir()
			}
			if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
				t.Fatal(err)
			}
			next := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: os.Environ(), OwnerPID: old.ownerPID, Harness: "claude"})
			t.Cleanup(next.Close)
			initializeDashboardProxy(t, next)
			after, _ := readRegistration(t, dir, key)
			if after != reg {
				t.Fatalf("reconnect adopted %s row: before=%+v after=%+v", kind, reg, after)
			}
		})
	}
}

func TestDashboardProxyReconnectRespectsDashboardKeyOff(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	old, key, _ := dashboardSessionServer(t, root, true)
	t.Cleanup(old.Close)
	old.ownerPID = 4242
	dashCall(t, old, "todo.list", map[string]any{"session_key": key})
	old.dashboardEnd()
	// A replacement can read a new manifest/config; reconnect is not a
	// reason to ignore the session's resolved dashboard.enabled value.
	t.Setenv(envConfigManifests, claudeManifestPath+";"+dashboardOffManifest(t))
	next := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: os.Environ(), OwnerPID: old.ownerPID, Harness: "claude"})
	t.Cleanup(next.Close)
	initializeDashboardProxy(t, next)
	if reg, ok := readRegistration(t, dir, key); ok {
		t.Fatalf("disabled session remained registered after reconnect: %+v", reg)
	}
}
