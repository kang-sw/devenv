package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestDesktopPredecessor(t *testing.T) {
	identity := `{"port":1234,"pid":4321,"started_at":"time","version":"0.50.0","token_sha256":"` +
		strings.Repeat("a", 64) + `"}`
	if id, err := desktopPredecessor(strings.NewReader(identity)); err != nil || id.PID != 4321 {
		t.Fatalf("valid predecessor was refused: %v", err)
	}
	if id, err := desktopPredecessor(strings.NewReader("null")); err != nil || id != nil {
		t.Fatalf("missing-only start was refused: %v", err)
	}
	for _, input := range []string{"", "{}", identity + "null", strings.Repeat(" ", 4096) + "null"} {
		if _, err := desktopPredecessor(strings.NewReader(input)); err == nil {
			t.Fatal("accepted malformed or oversized handoff")
		}
	}
}

func TestDaemonStartReplaceFlagWiring(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WS_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(root, "config"))
	for _, replace := range []bool{false, true} {
		args := []string{"--build-time", "123", "--start-wait", "1s"}
		if replace {
			args = append(args, "--replace")
		}
		called := false
		daemonStartWithEnsure(args, func(_ context.Context, opts wsdashboard.EnsureOptions) (wsdashboard.Outcome, error) {
			called = true
			if opts.Replace != replace || opts.BuildTime != 123 || opts.StartWait != time.Second {
				t.Fatalf("starter options = %+v", opts)
			}
			return wsdashboard.OutcomeRunning, nil
		})
		if !called {
			t.Fatal("Ensure not called")
		}
	}
}

func TestDaemonStartStrictLifetimeFlagWiring(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only CLI opt-in; compile-only on remote Linux")
	}
	root := t.TempDir()
	t.Setenv("WS_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(root, "config"))
	daemonStartWithEnsure([]string{"--require-independent-lifetime", "--replace"}, func(_ context.Context, opts wsdashboard.EnsureOptions) (wsdashboard.Outcome, error) {
		if !opts.SpawnBeforeShutdown || !opts.PreserveRunningLAN || !opts.Replace {
			t.Fatalf("strict options = %+v", opts)
		}
		return wsdashboard.OutcomeRunning, nil
	})
}

// TestDaemonStartLANFollowsTheApplyRule pins daemonStartLAN: a starter the
// dashboard applies to decides from its settings; any other starter leaves
// a running daemon's mode alone and otherwise takes the global file's
// value, never its own settings' false.
func TestDaemonStartLANFollowsTheApplyRule(t *testing.T) {
	running := func(lan, ok bool) func() (bool, bool) { return func() (bool, bool) { return lan, ok } }
	global := func(v bool) func() bool { return func() bool { return v } }
	for _, c := range []struct {
		name              string
		applies, settings bool
		running           func() (bool, bool)
		global            func() bool
		want              bool
	}{
		{"applies: settings true", true, true, running(false, true), global(false), true},
		{"applies: settings false", true, false, running(true, true), global(true), false},
		{"not applying: running LAN daemon stays", false, false, running(true, true), global(false), true},
		{"not applying: running loopback daemon stays", false, false, running(false, true), global(true), false},
		{"not applying, none running: global true", false, false, running(false, false), global(true), true},
		{"not applying, none running: global false", false, false, running(false, false), global(false), false},
	} {
		if got := daemonStartLAN(c.applies, c.settings, c.running, c.global); got != c.want {
			t.Errorf("%s: daemonStartLAN = %v, want %v", c.name, got, c.want)
		}
	}
}
