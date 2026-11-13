package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// TestWhiteboardBrowserFixture exports actual file and served renderings for
// an opt-in static browser gate. It never runs a local daemon or Go binary.
// The remote test runner fetches only this directory into its scratch folder.
func TestWhiteboardBrowserFixture(t *testing.T) {
	dest := os.Getenv("WS_WHITEBOARD_BROWSER_FIXTURE")
	if dest == "" {
		t.Skip("set WS_WHITEBOARD_BROWSER_FIXTURE to export browser fixtures")
	}
	f := newWhiteboardFixture(t)
	picture := image.NewRGBA(image.Rect(0, 0, 1200, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 1200; x++ {
			picture.SetRGBA(x, y, color.RGBA{R: uint8(40 + x/8%160), G: uint8(60 + y/2), B: 180, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, f.root, "shots/panel.png", encoded.String())
	var commands strings.Builder
	commands.WriteString(`<wb:topic name="Browser gate"><wb:body><h2>Current content</h2><p>Start with <a id="jump-old" href="#p1">the older panel</a>, the latest {{p2}}, <a id="jump-closed" href="#p7">the archived panel</a>, or item {{1}}.</p></wb:body><wb:add class="policy"><wb:title>Choose the rollout</wb:title><wb:option>Gradual</wb:option><wb:option>Immediate</wb:option></wb:add><wb:panel recap="Old target outside the first history page">old-target-payload</wb:panel><wb:panel recap="Original version of p2">original-version-payload</wb:panel>`)
	for i := 1; i <= 110; i++ {
		fmt.Fprintf(&commands, "<wb:log>Agent line %03d: a compact **decision** about item {{1}}.</wb:log>", i)
	}
	commands.WriteString(`<wb:panel id="p2" recap="Latest version of p2">latest-version-payload</wb:panel><wb:panel recap="Keyboard disclosure">keyboard-panel-payload</wb:panel><wb:panel recap="Mockup card"><div style="padding:12px;border:2px solid #5982bd">A sandboxed mockup with an <a href="#p1">older panel link</a>.</div></wb:panel><wb:panel recap="Image card">![Local panel image](shots/panel.png)</wb:panel><wb:panel recap="Long panel and image">## Long panel gate

`)
	for i := 0; i < 55; i++ {
		fmt.Fprintf(&commands, "Long payload paragraph %02d: substantive material stays inside this full-width card rather than a side rail.\n\n", i)
	}
	commands.WriteString(`![Long panel image](shots/panel.png)</wb:panel><wb:log>Latest agent line references {{p1}} and {{p2}}.</wb:log></wb:topic>`)
	// The oldest closed topic falls beyond the normal 30-topic window. Its
	// panel must remain navigable through the inert history-topic template.
	for i := 0; i < 33; i++ {
		fmt.Fprintf(&commands, `<wb:topic name="Archived %02d"><wb:body>Archived content %02d</wb:body>`, i, i)
		if i == 0 {
			commands.WriteString(`<wb:panel recap="Beyond closed cap">closed-target-payload</wb:panel>`)
		}
		commands.WriteString(`<wb:close result="Archived for the browser gate"/></wb:topic>`)
	}
	f.call(t, "todo.add", map[string]any{"key": "browser", "title": "Auxiliary sections stay below the full-width board"})
	f.update(t, commands.String())
	dir := t.TempDir()
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root, HeartbeatAt: time.Now().UTC().Format(time.RFC3339)}
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	snapshot := fmt.Sprintf(`{"schemaVersion":1,"replyId":%q,"adapter":"pi","updatedAt":%d,"heartbeatMs":30000,"lead":{"phase":"idle"}}`, f.replyID(), time.Now().UnixMilli())
	if err := os.WriteFile(filepath.Join(wsdashboard.SessionDir(dir, f.replyID()), "snapshot.json"), []byte(snapshot), 0o600); err != nil {
		t.Fatal(err)
	}
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return dir, nil }}
	daemon := startWhiteboardDaemonForTest(t, dir, src)
	base := fmt.Sprintf("http://127.0.0.1:%d", daemon.Endpoint().Port)
	resp, err := http.Get(base + "/s/" + f.replyID())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	served, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("render served fixture: %s, %v", resp.Status, err)
	}
	files := map[string][]byte{
		"served/index.html": served,
		"shots/panel.png":   encoded.Bytes(),
	}
	// The full file page resolves image paths from its real nested output
	// directory. Copy that layout, rather than rewriting generated HTML.
	for name, data := range files {
		path := filepath.Join(dest, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fileDir := filepath.Join(dest, "ai-docs", "whiteboards", f.replyID())
	if err := os.MkdirAll(fileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "index.stamp.js"} {
		if err := os.WriteFile(filepath.Join(fileDir, name), readFileForTest(t, filepath.Join(f.pageDir(), name)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Capture the actual SSE region for one later committed update. The
	// browser gate injects this packet into the real page's transport handler
	// to exercise disclosure, loaded-history, and viewport restoration.
	f.update(t, `<wb:topic name="Browser gate"><wb:log>SSE-agent-line-marker</wb:log><wb:panel recap="SSE-panel-marker">SSE panel payload</wb:panel></wb:topic>`)
	if !daemon.Index().Nudge(f.replyID(), f.root) {
		t.Fatal("fixture update nudge refused")
	}
	daemon.Index().Settle()
	update := waitWhiteboardRegionForTest(t, openWhiteboardStreamForTest(t, daemon, f.replyID()), "board", "SSE-panel-marker")
	packet, _ := json.Marshal(update)
	if err := os.WriteFile(filepath.Join(dest, "update.json"), packet, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.MarshalIndent(map[string]any{
		"file":              filepath.ToSlash(filepath.Join("ai-docs", "whiteboards", f.replyID(), "index.html")),
		"served":            "served/index.html",
		"reply_id":          f.replyID(),
		"topic":             "Browser gate",
		"history_page_size": 50,
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(dest, "manifest.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
}
