package wsdashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestEnsureDesktopStartGuardsTheObservedPredecessor(t *testing.T) {
	// The gate is identical in either Windows process creation mode; this
	// fixture can run in a test runner's job even if breakaway is forbidden.
	t.Setenv("WS_MCP_DAEMON_JOB_LIFETIME", "1")
	t.Run("missing-only start leaves a raced older daemon alone", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.49.0")
		opts := ensureOpts(dir, "0.50.0", func([]string) error { t.Fatal("unexpected spawn"); return nil })
		opts.DesktopStart, opts.SpawnBeforeShutdown, opts.PreserveRunningLAN = true, true, true
		if outcome, err := Ensure(context.Background(), opts); outcome != OutcomeRunning || err != nil {
			t.Fatalf("Ensure = %s, %v", outcome, err)
		}
		if _, err := Health(context.Background(), testClient(), old.Endpoint()); err != nil {
			t.Fatal(err)
		}
	})

	for _, mode := range []string{"before spawn", "after spawn", "successful handoff"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			old := startTestDaemon(t, dir, "0.49.0")
			ep := old.Endpoint()
			info, err := Health(context.Background(), testClient(), ep)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte(ep.Token))
			expected := &DaemonIdentity{
				Port: ep.Port, PID: ep.PID, StartedAt: info.StartedAt, Version: info.Version,
				TokenHash: hex.EncodeToString(sum[:]),
			}
			if mode == "before spawn" {
				expected.TokenHash = strings.Repeat("0", 64)
			}
			sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"0.50.0"}}
			defer sp.closeAll()
			opts := ensureOpts(dir, "0.50.0", func([]string) error { t.Fatal("unexpected direct spawn"); return nil })
			opts.PrepareSuccessor = func(args []string) (func(bool) error, error) {
				switch mode {
				case "before spawn":
					t.Fatal("unexpected spawn")
				case "after spawn":
					admit, err := PrepareDetachedSuccessor(os.Args[0],
						[]string{"-test.run=^TestDesktopGatedSuccessorChild$", "--", "desktop-preflight-child"}, dir)
					if err != nil {
						return nil, err
					}
					waitForPreflightFile(t, dir, "waiting")
					// Same PID, port and health, but the file no longer
					// authorizes the predecessor that the shell observed.
					ep.Token = strings.Repeat("0", 64)
					if err := writeEndpoint(dir, ep); err != nil {
						_ = admit(false)
						return nil, err
					}
					return admit, nil
				}
				return func(allow bool) error {
					if allow {
						return sp.spawn(args)
					}
					return nil
				}, nil
			}
			opts.DesktopStart, opts.SpawnBeforeShutdown, opts.PreserveRunningLAN = true, true, true
			opts.Predecessor = expected
			outcome, err := Ensure(context.Background(), opts)
			if mode == "successful handoff" {
				if outcome != OutcomeReplaced || err != nil || len(sp.calls) != 1 {
					t.Fatalf("Ensure = %s, %v, %d spawns", outcome, err, len(sp.calls))
				}
				successor, err := ReadEndpoint(dir)
				if err != nil || successor.Port != ep.Port {
					t.Fatalf("replacement moved the dashboard origin: port %d, want %d (%v)", successor.Port, ep.Port, err)
				}
				waitDone(t, old)
				return
			}
			if mode == "before spawn" && (outcome != OutcomeRunning || err != nil) ||
				mode == "after spawn" && (outcome != OutcomeUnavailable || err == nil) {
				t.Fatalf("Ensure = %s, %v", outcome, err)
			}
			if _, err := Health(context.Background(), testClient(), old.Endpoint()); err != nil {
				t.Fatalf("predecessor was drained: %v", err)
			}
			if mode == "after spawn" {
				if result := waitForPreflightFile(t, dir, "admission"); result != "denied" {
					t.Fatalf("rejected successor was allowed to start: %q", result)
				}
			}
		})
	}
}

func waitForPreflightFile(t *testing.T, dir, name string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if bytes, err := os.ReadFile(filepath.Join(dir, name)); err == nil && len(bytes) > 0 {
			return string(bytes)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("gated child did not write %s", name)
	return ""
}

func TestDesktopGatedSuccessorChild(t *testing.T) {
	if os.Args[len(os.Args)-1] != "desktop-preflight-child" {
		t.Skip("subprocess fixture")
	}
	if err := os.WriteFile("waiting", []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	admission := "denied"
	if AwaitDesktopPreflight(os.Stdin) {
		admission = "admitted"
	}
	if err := os.WriteFile("admission", []byte(admission), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureElectronPreflightAndFreshHealth(t *testing.T) {
	t.Run("strict preflight failure leaves old serving", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.48.0")
		opts := ensureOpts(dir, "0.48.0", func([]string) error { return errors.New("breakaway refused") })
		opts.Replace, opts.SpawnBeforeShutdown = true, true
		outcome, err := Ensure(context.Background(), opts)
		if outcome != OutcomeUnavailable || err == nil {
			t.Fatalf("Ensure = %s, %v", outcome, err)
		}
		if _, err := Health(context.Background(), testClient(), old.Endpoint()); err != nil {
			t.Fatalf("predecessor was drained: %v", err)
		}
	})
	t.Run("accepted successor precedes authenticated drain and explicit swap is one shot", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.48.0")
		sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"0.48.0"}}
		defer sp.closeAll()
		opts := ensureOpts(dir, "0.48.0", func(args []string) error {
			if _, err := Health(context.Background(), testClient(), old.Endpoint()); err != nil {
				t.Fatalf("predecessor stopped before independent creation: %v", err)
			}
			return sp.spawn(args)
		})
		opts.Replace, opts.SpawnBeforeShutdown = true, true
		outcome, err := Ensure(context.Background(), opts)
		if err != nil || outcome != OutcomeReplaced || len(sp.calls) != 1 {
			t.Fatalf("Ensure = %s, %v, %d spawns", outcome, err, len(sp.calls))
		}
		waitDone(t, old)
	})
	t.Run("accepted creation with no healthy successor is unavailable", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.48.0")
		opts := ensureOpts(dir, "0.48.0", func([]string) error { return nil })
		opts.Replace, opts.SpawnBeforeShutdown = true, true
		opts.StartWait = 100 * time.Millisecond
		outcome, err := Ensure(context.Background(), opts)
		if err != nil || outcome != OutcomeUnavailable {
			t.Fatalf("Ensure = %s, %v", outcome, err)
		}
		waitDone(t, old)
	})
	t.Run("preserve raced LAN mode inside Ensure", func(t *testing.T) {
		dir := t.TempDir()
		old, err := StartDaemon(DaemonOptions{Dir: dir, Version: "0.48.0", LAN: true, bindHost: "127.0.0.1", LockWait: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(old.Close)
		opts := ensureOpts(dir, "0.48.0", func([]string) error { t.Fatal("unexpected spawn"); return nil })
		opts.SpawnBeforeShutdown, opts.PreserveRunningLAN = true, true
		if outcome, err := Ensure(context.Background(), opts); outcome != OutcomeRunning || err != nil {
			t.Fatalf("Ensure = %s, %v", outcome, err)
		}
		if _, err := Health(context.Background(), testClient(), old.Endpoint()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("older-minor daemon is replaced without an explicit request", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.49.3")
		sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"0.50.1"}}
		defer sp.closeAll()
		opts := ensureOpts(dir, "0.50.1", sp.spawn)
		opts.SpawnBeforeShutdown, opts.PreserveRunningLAN = true, true
		outcome, err := Ensure(context.Background(), opts)
		if err != nil || outcome != OutcomeReplaced || len(sp.calls) != 1 {
			t.Fatalf("Ensure = %s, %v, %d spawns", outcome, err, len(sp.calls))
		}
		waitDone(t, old)
	})
	t.Run("raced newer-minor winner is not drained, even on explicit replace", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.49.0")
		opts := ensureOpts(dir, "0.48.1", func([]string) error { t.Fatal("unexpected spawn"); return nil })
		opts.Replace, opts.SpawnBeforeShutdown = true, true
		if outcome, err := Ensure(context.Background(), opts); outcome != OutcomeRunning || err != nil {
			t.Fatalf("Ensure = %s, %v", outcome, err)
		}
		if _, err := Health(context.Background(), testClient(), old.Endpoint()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("strict starter waits for launcher lock before observing health", func(t *testing.T) {
		dir := t.TempDir()
		other := flock.New(filepath.Join(dir, "launcher-start.lock"))
		if ok, err := other.TryLock(); err != nil || !ok {
			t.Fatalf("lock: %v, %v", ok, err)
		}
		defer other.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		opts := ensureOpts(dir, "0.48.0", func([]string) error { t.Fatal("unexpected spawn"); return nil })
		opts.SpawnBeforeShutdown = true
		outcome, err := Ensure(ctx, opts)
		if outcome != OutcomeUnavailable || err == nil {
			t.Fatalf("Ensure = %s, %v", outcome, err)
		}
		if !strings.Contains(err.Error(), "launcher start lock:") || strings.Contains(err.Error(), "<nil>") {
			t.Fatalf("unexpected launcher lock diagnostic: %v", err)
		}
	})
}
