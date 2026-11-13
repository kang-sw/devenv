package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestLauncherMailboxStreamEndsWhenPOSIXParentIsKilled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX reparenting test")
	}
	bin := buildDashboardTestBin(t, "")
	launcher, env, cache, cwd := launcherTestPlugin(t, bin)
	const key = "amber-tide-fox"
	seedReplyMail(t, env, key, "one")
	// Like Node/Bun spawn: stdin is ignored, and killing the parent sends no
	// signal to the launcher. Only its parent watch can end the daemon wait.
	parentScript := filepath.Join(t.TempDir(), "parent.py")
	if err := os.WriteFile(parentScript, []byte("import subprocess, sys, time\nsubprocess.Popen([sys.executable, *sys.argv[1:]], stdin=subprocess.DEVNULL)\ntime.sleep(60)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parent := startLauncher(t, parentScript, cwd, env, launcher, "mailbox", "wait", "--session-key", key, "--stream")
	first := make(chan error, 1)
	go func() {
		line, err := parent.out.ReadBytes('\n')
		if err == nil {
			var arrival map[string]any
			err = json.Unmarshal(line, &arrival)
		}
		first <- err
	}()
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("first arrival: %v; stderr %s", err, parent.stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("no stream arrival; stderr %s", parent.stderr.String())
	}
	paths := wsmailbox.Paths{State: wsstate.Options{CacheHome: cache}}
	marker, ok, err := paths.ReadListeningMarker(key)
	if err != nil || !ok || marker.PID == parent.cmd.Process.Pid {
		t.Fatalf("launcher marker = %+v, %v, %v", marker, ok, err)
	}
	t.Cleanup(func() {
		if _, ok, _ := paths.ReadListeningMarker(key); ok {
			if process, err := os.FindProcess(marker.PID); err == nil {
				_ = process.Kill()
			}
		}
	})
	if err := parent.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// The launcher inherited both output pipes. EOF proves it exited too,
	// rather than just leaving a disconnected, orphaned Python process.
	ended := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, parent.out)
		ended <- err
	}()
	select {
	case err := <-ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("launcher survived its parent's SIGKILL with ignored stdin")
	}
	_ = waitExit(t, parent.cmd, 5*time.Second)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, ok, err := paths.ReadListeningMarker(key)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("parent death left the daemon wait's marker alive")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLauncherMailboxStreamSurvivesArrivalsAndTimeoutThenSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows termination is covered by launcher parent-watch tests; Go cannot send SIGTERM there")
	}
	bin := buildDashboardTestBin(t, "")
	launcher, env, cache, cwd := launcherTestPlugin(t, bin)
	const key = "amber-tide-fox"
	seedReplyMail(t, env, key, "one")
	client := startLauncher(t, launcher, cwd, env, "mailbox", "wait", "--session-key", key, "--stream", "--timeout", "50ms")
	paths := wsmailbox.Paths{State: wsstate.Options{CacheHome: cache}}
	read := func(want string) string {
		t.Helper()
		type result struct {
			line []byte
			err  error
		}
		got := make(chan result, 1)
		go func() {
			line, err := client.out.ReadBytes('\n')
			got <- result{line, err}
		}()
		select {
		case r := <-got:
			var arrival map[string]any
			if r.err != nil || json.Unmarshal(r.line, &arrival) != nil || !strings.Contains(string(r.line), want) || arrival["timed_out"] != false || arrival["rearm"] != nil {
				t.Fatalf("arrival %q, err %v, stderr %s", r.line, r.err, client.stderr.String())
			}
			return string(r.line)
		case <-time.After(10 * time.Second):
			t.Fatalf("no stream arrival; stderr %s", client.stderr.String())
		}
		return ""
	}
	read("one")
	read("one") // The unchanged pending mail is retried, not a timeout exit.
	seedReplyMail(t, env, key, "two")
	// An already-in-flight reminder can precede the newly written mail.
	found := false
	for i := 0; i < 3; i++ {
		if strings.Contains(read(""), "two") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("stream did not report mail added after the reminder")
	}
	marker, ok, err := paths.ReadListeningMarker(key)
	if err != nil || !ok || marker.PID != client.cmd.Process.Pid || marker.TimeoutAt != "" {
		t.Fatalf("marker while streaming: %+v, %v, %v", marker, ok, err)
	}
	if err := client.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = waitExit(t, client.cmd, 5*time.Second)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, ok, err := paths.ReadListeningMarker(key)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("launcher signal left the daemon wait and marker alive")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
