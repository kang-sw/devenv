package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/mcpframe"
)

// TestServeStdioFramingHandlesLargeToolAndSurvivesOversize exercises the real
// ServeStdio loop over pipes: a >64 KiB whiteboard.update succeeds, requests
// above mcpframe.MaxRequestBytes receive per-request JSON-RPC -32600 errors
// (recovering id when parseable from the bounded prefix, else null) without
// ending the loop, and a subsequent ordinary request on the same session
// succeeds.
func TestServeStdioFramingHandlesLargeToolAndSurvivesOversize(t *testing.T) {
	f := newWhiteboardFixture(t)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- f.s.ServeStdio(context.Background(), inR, outW)
		_ = outW.Close()
	}()
	waited := false
	t.Cleanup(func() {
		_ = inW.Close()
		_ = outR.Close()
		if !waited {
			<-serveDone
		}
	})

	reader := bufio.NewReader(outR)
	sendAndRead := func(line string) string {
		t.Helper()
		if _, err := io.WriteString(inW, line+"\n"); err != nil {
			t.Fatalf("write request: %v", err)
		}
		resp, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		return strings.TrimSpace(resp)
	}

	// 1. Representative >64 KiB whiteboard.update with SVG content succeeds.
	svgBody := `<wb:topic name="diagram"><wb:body><svg>` + strings.Repeat(`<rect width="10" height="10"/>`, 3000) + `</svg></wb:body></wb:topic>`
	largeReq, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      101,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "whiteboard.update",
			"arguments": map[string]any{
				"session_key": f.key,
				"commands":    svgBody,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(largeReq) <= 64<<10 {
		t.Fatalf("largeReq len = %d, want > 64 KiB", len(largeReq))
	}
	resp := sendAndRead(string(largeReq))
	if isToolError(resp) {
		t.Fatalf("large whiteboard.update failed: %s", resp)
	}
	var largeResp struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp), &largeResp); err != nil || string(largeResp.ID) != "101" {
		t.Fatalf("large whiteboard.update response id = %s, err = %v", resp, err)
	}
	if !strings.Contains(toolText(t, resp), "diagram") {
		t.Fatalf("large whiteboard.update did not record diagram topic: %s", toolText(t, resp))
	}

	// 2. Oversize request with id in prefix gets -32600 with recovered id.
	oversizeWithID := `{"jsonrpc":"2.0","id":"oversize-1","method":"tools/call","params":{"name":"whiteboard.update","arguments":{"session_key":` +
		`"unused","commands":"` + strings.Repeat("x", mcpframe.MaxRequestBytes) + `"}}}`
	resp = sendAndRead(oversizeWithID)
	var errResp struct {
		ID    json.RawMessage `json:"id"`
		Error *rpcError       `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &errResp); err != nil {
		t.Fatalf("decode oversize-with-id response: %v (%s)", err, resp)
	}
	if string(errResp.ID) != `"oversize-1"` || errResp.Error == nil || errResp.Error.Code != -32600 || errResp.Error.Message != mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes) {
		t.Fatalf("oversize-with-id = %s, want id=\"oversize-1\" -32600 %q", resp, mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes))
	}

	// 3. Oversize request with id past the bounded prefix gets -32600 with id: null.
	oversizeTailID := `{"jsonrpc":"2.0","method":"ping","params":{"pad":"` + strings.Repeat("y", mcpframe.MaxRequestBytes) + `"},"id":999}`
	resp = sendAndRead(oversizeTailID)
	errResp = struct {
		ID    json.RawMessage `json:"id"`
		Error *rpcError       `json:"error"`
	}{}
	if err := json.Unmarshal([]byte(resp), &errResp); err != nil {
		t.Fatalf("decode oversize-tail-id response: %v (%s)", err, resp)
	}
	if string(errResp.ID) != "null" || errResp.Error == nil || errResp.Error.Code != -32600 || errResp.Error.Message != mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes) {
		t.Fatalf("oversize-tail-id = %s, want id=null -32600 %q", resp, mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes))
	}

	// 4. Subsequent ordinary request on the same ServeStdio session succeeds.
	resp = sendAndRead(`{"jsonrpc":"2.0","id":102,"method":"ping"}`)
	var pingResp struct {
		ID     json.RawMessage `json:"id"`
		Result map[string]any  `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &pingResp); err != nil {
		t.Fatalf("decode follow-up ping: %v (%s)", err, resp)
	}
	if string(pingResp.ID) != "102" || pingResp.Error != nil || pingResp.Result == nil {
		t.Fatalf("follow-up ping = %s, want id=102 result", resp)
	}

	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	waited = true
	if err := <-serveDone; err != nil {
		t.Fatalf("ServeStdio exited with error: %v", err)
	}
}
