package wsdashboard

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestEnsureElectronPreflightAndFreshHealth(t *testing.T) {
	t.Run("strict preflight failure leaves old serving", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.48.0")
		opts := ensureOpts(dir, "0.48.0", func([]string) error { return errors.New("breakaway refused") })
		opts.Replace, opts.SpawnBeforeShutdown, opts.RefuseCrossMinor = true, true, true
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
		opts.Replace, opts.SpawnBeforeShutdown, opts.RefuseCrossMinor = true, true, true
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
	t.Run("raced cross-minor winner is not drained", func(t *testing.T) {
		dir := t.TempDir()
		old := startTestDaemon(t, dir, "0.49.0")
		opts := ensureOpts(dir, "0.48.1", func([]string) error { t.Fatal("unexpected spawn"); return nil })
		opts.Replace, opts.SpawnBeforeShutdown, opts.RefuseCrossMinor = true, true, true
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
		if outcome, err := Ensure(ctx, opts); outcome != OutcomeUnavailable || err == nil {
			t.Fatalf("Ensure = %s, %v", outcome, err)
		}
	})
}
