package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func TestWhiteboardCLIAuthority(t *testing.T) {
	hello := cliHello(t)
	t.Setenv("WS_CACHE_HOME", hello.Env["WS_CACHE_HOME"])
	t.Setenv("WS_CONFIG_HOME", hello.Env["WS_CONFIG_HOME"])
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "")
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	key, err := mcp.NewAgentKeyStore().MintLead(hello.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(hello.Cwd, "ai-docs", "whiteboards", wsmailbox.ReplyID(key))
	commands := `<wb:topic name="owned"><wb:log>daemon committed</wb:log></wb:topic>`
	update, _ := json.Marshal(map[string]string{"session_key": key, "commands": commands})
	query, _ := json.Marshal(map[string]string{"session_key": key})
	load, _ := json.Marshal(map[string]string{"session_key": key, "path": "missing.zip"})

	for _, tc := range []struct {
		tool string
		args []byte
	}{
		{"whiteboard.update", update},
		{"whiteboard.query", query},
		{"whiteboard.load", load},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			var diag bytes.Buffer
			raw := &hostedCLI{hello: hello, ctx: context.Background(), stdout: io.Discard, stderr: &diag}
			if exit := raw.mcpCommand("call", []string{tc.tool, string(tc.args)}); exit == 0 || !strings.Contains(diag.String(), "dashboard daemon, which owns the board") {
				t.Fatalf("raw call exit=%d diag=%q", exit, &diag)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("raw call touched board storage: %v", err)
			}
		})
	}

	boards := mcp.NewWhiteboardStores(io.Discard)
	t.Cleanup(func() {
		if err := boards.Close(); err != nil {
			t.Error(err)
		}
	})
	host := &proxyHost{boards: boards, logw: io.Discard}
	if out, diag, exit := hostedCLIResultWithHost(t, host, hello, "call", "whiteboard.update", string(update)); exit != 0 || !strings.Contains(out, "full: page not yet served") {
		t.Fatalf("daemon call exit=%d out=%q diag=%q", exit, out, diag)
	}
	if out, diag, exit := hostedCLIResultWithHost(t, host, hello, "call", "whiteboard.query", string(query)); exit != 0 || !strings.Contains(out, "owned") {
		t.Fatalf("daemon query exit=%d out=%q diag=%q", exit, out, diag)
	}
	if _, err := os.Stat(filepath.Join(dir, ".board.sqlite")); err != nil {
		t.Fatalf("daemon did not create board DB: %v", err)
	}
}
