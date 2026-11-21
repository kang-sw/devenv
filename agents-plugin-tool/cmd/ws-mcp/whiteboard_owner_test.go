package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardConstructionBeforeDashboardBind(t *testing.T) {
	for _, mode := range []string{"proxy", "hosted-cli", "raw-cli"} {
		t.Run(mode, func(t *testing.T) {
			hello := cliHello(t)
			for _, name := range []string{"WS_CACHE_HOME", "WS_CONFIG_HOME", "WS_MCP_PROJECT_ROOT", "WS_RSRC_ROOT", "WS_MCP_NO_DASHBOARD", "WS_MCP_CONFIG_MANIFESTS"} {
				t.Setenv(name, hello.Env[name])
			}
			host := &proxyHost{logw: io.Discard}
			var stdout, stderr strings.Builder
			cli := &hostedCLI{hello: hello, host: host, ctx: context.Background(), env: mcp.EnvFromMap(hello.Env), stdout: &stdout, stderr: &stderr}
			if mode == "raw-cli" {
				cli.host = nil
			}
			var server *mcp.Server
			if mode == "proxy" {
				hello.Mode = wsdashboard.ProxyModeMCP
				session, err := host.Accept(hello)
				if err != nil {
					t.Fatal(err)
				}
				server = session.(*proxyMCPSession).server
			} else {
				server = cli.server(hello.Cwd)
			}
			defer server.Close()
			client, pipe := net.Pipe()
			done := make(chan struct{})
			go func() { defer close(done); defer pipe.Close(); _ = server.ServeStdio(context.Background(), pipe, pipe) }()
			defer func() { _ = client.Close(); <-done }()
			conn := &hostedConn{t: t, conn: client, r: bufio.NewReader(client)}
			key := conn.login(hello.Cwd)
			args := map[string]any{"session_key": key, "commands": `<wb:topic name="owner"><wb:log>committed</wb:log></wb:topic>`}
			var text string
			if mode == "proxy" {
				text = toolText(t, conn.tool("whiteboard.update", args))
			} else {
				data, _ := json.Marshal(args)
				if code := cli.mcpCommand("call", []string{"whiteboard.update", string(data)}); code != 0 {
					t.Fatalf("code=%d, stderr=%s", code, &stderr)
				}
				text = stdout.String()
			}
			want := "full: page not yet served\n"
			if mode == "raw-cli" {
				want = "full: page unavailable\n"
			}
			if !strings.Contains(text, want) || strings.Contains(text, "index.html") || strings.Contains(text, "start the daemon") {
				t.Fatal(text)
			}
			board, err := (wswhiteboard.FolderStore{Dir: filepath.Join(hello.Cwd, "ai-docs", "whiteboards", wsmailbox.ReplyID(key))}).Open(nil)
			if err != nil || board == nil || board.Seq != 1 {
				t.Fatalf("board did not commit once: %+v, %v", board, err)
			}

		})
	}
}
