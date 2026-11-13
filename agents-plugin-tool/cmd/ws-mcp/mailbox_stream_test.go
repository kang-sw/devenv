package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswake"
)

func mailboxStreamGetenv(env []string) func(string) string {
	values := map[string]string{}
	for _, kv := range env {
		key, value, _ := strings.Cut(kv, "=")
		values[key] = value
	}
	return func(key string) string { return values[key] }
}

type mailboxStreamWriter func([]byte) (int, error)

func (f mailboxStreamWriter) Write(p []byte) (int, error) { return f(p) }

func TestRunMailboxWaitStreamJSONAndLifetimeMarker(t *testing.T) {
	env := mailboxTestEnv(t)
	getenv := mailboxStreamGetenv(env)
	paths := wsmailbox.Paths{State: wsstate.Options{CacheHome: getenv("WS_CACHE_HOME")}}
	const key = "amber-tide-fox"
	seedReplyMail(t, env, key, "one")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := &wswake.Hub{}
	var stdout, stderr bytes.Buffer
	lines := 0
	writer := mailboxStreamWriter(func(p []byte) (int, error) {
		lines++
		var arrival map[string]any
		if err := json.Unmarshal(p, &arrival); err != nil {
			t.Fatalf("not one JSON line: %q: %v", p, err)
		}
		if arrival["timed_out"] != false || arrival["unread"] != float64(1) || arrival["rearm"] != nil {
			t.Fatalf("arrival = %s", p)
		}
		marker, ok, err := paths.ReadListeningMarker(key)
		if err != nil || !ok || marker.PID != os.Getpid() || marker.TimeoutAt != "" {
			t.Fatalf("lifetime marker = %+v, %v, %v", marker, ok, err)
		}
		if lines == 1 {
			seedReplyMail(t, env, key, "two")
			hub.Wake()
		} else {
			cancel()
		}
		return len(p), nil
	})
	code := runMailboxWait(ctx, []string{"--session-key", key, "--stream", "--format", "text", "--timeout", "1h"}, &stdout, &stderr, "", os.Getpid(), env, getenv, hub, writer)
	if code != mailboxWaitExitInterrupted || lines != 2 || stdout.Len() != 0 {
		t.Fatalf("code %d, lines %d, buffered stdout %q, stderr %q", code, lines, stdout.String(), stderr.String())
	}
	if _, ok, err := paths.ReadListeningMarker(key); ok || err != nil {
		t.Fatalf("marker survived cancellation: %v, %v", ok, err)
	}
}

func TestRunMailboxWaitStreamDoesNotExitOrEmitOnEmptyTimeout(t *testing.T) {
	env := mailboxTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := runMailboxWait(ctx, []string{"--session-key", "amber-tide-fox", "--stream", "--timeout", "1ms"}, &stdout, &stderr, "", os.Getpid(), env, mailboxStreamGetenv(env), &wswake.Hub{}, nil)
	if code != mailboxWaitExitInterrupted || stdout.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestProxyMailboxWaitStreamFramesBeforeExitAndDisconnectClearsMarker(t *testing.T) {
	env := mailboxTestEnv(t)
	const key = "amber-tide-fox"
	seedReplyMail(t, env, key, "one")
	hello := wsdashboard.ProxyHello{Argv: []string{"mailbox", "wait", "--session-key", key, "--stream"}, Cwd: t.TempDir(), OwnerPID: os.Getpid(), Env: map[string]string{}}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		hello.Env[k] = v
	}
	server, client := net.Pipe()
	defer client.Close()
	session := &proxyCLISession{hello: hello}
	done := make(chan struct{})
	go func() {
		session.Serve(context.Background(), server)
		server.Close()
		close(done)
	}()
	reader := bufio.NewReader(client)
	readArrival := func(want string) {
		t.Helper()
		_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var frame wsdashboard.ProxyCLIFrame
		if err := json.Unmarshal(line, &frame); err != nil || frame.Stream != "stdout" || frame.Final || !strings.Contains(frame.Data, want) {
			t.Fatalf("live stdout frame = %s, err %v", line, err)
		}
	}
	readArrival("one")
	seedReplyMail(t, env, key, "two")
	wswake.Mailbox.Wake()
	readArrival("two")
	client.Close() // A terminated launcher (signal/parent death) closes this socket.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnected stream still running")
	}
	paths := wsmailbox.Paths{State: wsstate.Options{CacheHome: mailboxStreamGetenv(env)("WS_CACHE_HOME")}}
	if _, ok, err := paths.ReadListeningMarker(key); err != nil || ok {
		t.Fatalf("marker after disconnect: %v, %v", ok, err)
	}
}

func TestRunMailboxWaitStreamOutputFailureClearsMarker(t *testing.T) {
	env := mailboxTestEnv(t)
	const key = "amber-tide-fox"
	seedReplyMail(t, env, key, "one")
	var stdout, stderr bytes.Buffer
	writer := mailboxStreamWriter(func([]byte) (int, error) { return 0, io.ErrClosedPipe })
	code := runMailboxWait(context.Background(), []string{"--session-key", key, "--stream"}, &stdout, &stderr, "", os.Getpid(), env, mailboxStreamGetenv(env), nil, writer)
	if code != 1 || !strings.Contains(stderr.String(), "closed pipe") {
		t.Fatalf("code %d, stderr %q", code, stderr.String())
	}
	paths := wsmailbox.Paths{State: wsstate.Options{CacheHome: mailboxStreamGetenv(env)("WS_CACHE_HOME")}}
	if _, ok, err := paths.ReadListeningMarker(key); err != nil || ok {
		t.Fatalf("marker after write failure: %v, %v", ok, err)
	}
}
