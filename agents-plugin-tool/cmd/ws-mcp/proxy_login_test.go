package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// Exercise the hook through Accept, not a second HTTP minting client: the
// hosted whiteboard tool prints a code the daemon exchanges exactly once.
func TestProxyWhiteboardLoginLink(t *testing.T) {
	cache, config, root := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("WS_CACHE_HOME", cache)
	t.Setenv("WS_CONFIG_HOME", config)
	if err := os.MkdirAll(filepath.Join(root, "ai-docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	rsrc, err := filepath.Abs(filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := wsdashboard.Dir(cache)
	if err != nil {
		t.Fatal(err)
	}
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: dir, Sessions: mcp.NewDashboardSessionSource(), PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	h := &proxyHost{logw: io.Discard}
	h.live.Store(d)
	session, err := h.Accept(wsdashboard.ProxyHello{
		Mode: wsdashboard.ProxyModeMCP, Cwd: root, ProjectRoot: root, OwnerPID: os.Getpid(),
		Env: map[string]string{
			"PATH": os.Getenv("PATH"), "SystemRoot": os.Getenv("SystemRoot"),
			"HOME": root, "USERPROFILE": root,
			"WS_CACHE_HOME": cache, "WS_CONFIG_HOME": config,
			"WS_RSRC_ROOT": rsrc, "WS_SKILLS_ROOT": filepath.Join(root, "missing-skills"),
			"WS_MCP_CONFIG_MANIFESTS": writeProxyTestManifest(t, true),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		session.Serve(context.Background(), server)
		_ = server.Close()
	}()
	defer func() { _ = client.Close(); <-done }()
	conn := &hostedConn{t: t, conn: client, r: bufio.NewReader(client)}
	key := conn.login(root)
	var link string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp := conn.tool("whiteboard.update", map[string]any{"session_key": key, "commands": ""})
		for _, line := range strings.Split(toolText(t, resp), "\n") {
			if strings.HasPrefix(line, "full: http://") {
				link = strings.TrimPrefix(line, "full: ")
			}
		}
		if strings.Contains(link, "login=") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	u, err := url.Parse(link)
	if err != nil || u.Query().Get("login") == "" || !strings.HasPrefix(u.Path, "/s/") {
		t.Fatalf("whiteboard full link = %q, err = %v", link, err)
	}
	httpClient := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for i, want := range []int{http.StatusSeeOther, http.StatusUnauthorized} {
		req, err := http.NewRequest(http.MethodGet, link, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "text/html")
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("exchange %d = %d, want %d", i+1, resp.StatusCode, want)
		}
		if i == 0 && (len(resp.Cookies()) == 0 || resp.Header.Get("Location") != u.Path) {
			t.Fatalf("exchange did not set cookie and clean URL: %v", resp.Header)
		}
	}
}
