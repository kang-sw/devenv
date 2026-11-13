package mcp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// TestBoardPerfServe serves a synthetic long board from an in-process
// dashboard daemon for the board performance harness (scripts/board-perf.mjs),
// which builds this test binary from a source tree, runs it, and drives the
// page with Playwright. It never touches the installed daemon: the daemon,
// its state directory, and the board's project live in temp directories.
//
// Protocol on stdout: one "BOARD-PERF READY <url>" line once the page is
// served. On stdin, each "log <n>" line appends n one-line log entries to
// the first open topic and nudges the daemon, answered by "BOARD-PERF OK";
// "quit" (or EOF) ends the test.
//
// The board's shape comes from WS_BOARD_PERF_SHAPE ("open,closed,panels,
// versions,lines", default "8,6,4,2,60"): open topics, closed topics, panels
// per open topic (closed topics get half), versions per panel, and log lines
// per open topic.
func TestBoardPerfServe(t *testing.T) {
	if os.Getenv("WS_BOARD_PERF_SERVE") == "" {
		t.Skip("set WS_BOARD_PERF_SERVE to serve the board performance harness's synthetic board")
	}
	shape := [5]int{8, 6, 4, 2, 60}
	if s := os.Getenv("WS_BOARD_PERF_SHAPE"); s != "" {
		parts := strings.Split(s, ",")
		if len(parts) != len(shape) {
			t.Fatalf("WS_BOARD_PERF_SHAPE: want 5 comma-separated numbers, got %q", s)
		}
		for i, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 {
				t.Fatalf("WS_BOARD_PERF_SHAPE: %q is not a count", p)
			}
			shape[i] = n
		}
	}
	open, closed, panels, versions, lines := shape[0], shape[1], shape[2], shape[3], shape[4]
	if open < 1 {
		t.Fatal("WS_BOARD_PERF_SHAPE: the board needs at least one open topic")
	}
	f := newWhiteboardFixture(t)
	// Panel IDs are numbered across the board in creation order.
	nextPanel := 0
	topic := func(name string, panels int, closeIt bool) {
		var b strings.Builder
		fmt.Fprintf(&b, `<wb:topic name="%s"><wb:body>`, name)
		fmt.Fprintf(&b, "## %s\n\nThe topic body: a decision summary with a list and a table.\n\n- first point about %s\n- second point\n- third point\n\n| key | value |\n|---|---|\n| a | 1 |\n| b | 2 |\n\n", name, name)
		for i := 0; i < 6; i++ {
			fmt.Fprintf(&b, "Paragraph %d of %s carries enough prose to wrap across a couple of lines in the board column, as a real body does.\n\n", i, name)
		}
		b.WriteString(`</wb:body>`)
		if name == "T00" {
			b.WriteString(`<wb:add class="policy"><wb:title>Pick the rollout</wb:title><wb:option>Gradual</wb:option><wb:option>Immediate</wb:option></wb:add><wb:add class="technical"><wb:title>Use the shared helper</wb:title></wb:add>`)
		}
		ids := []int{}
		for p := 0; p < panels; p++ {
			nextPanel++
			ids = append(ids, nextPanel)
			fmt.Fprintf(&b, `<wb:panel recap="%s panel %d">%s`, name, p, panelBody(name, p, 0))
			b.WriteString(`</wb:panel>`)
		}
		for i := 0; i < lines; i++ {
			fmt.Fprintf(&b, "<wb:log>%s line %03d: a compact **decision** note.</wb:log>", name, i)
		}
		for v := 1; v < versions; v++ {
			for p, id := range ids {
				fmt.Fprintf(&b, `<wb:panel id="p%d" recap="%s panel %d v%d">%s</wb:panel>`, id, name, p, v, panelBody(name, p, v))
			}
		}
		if closeIt {
			b.WriteString(`<wb:close result="Closed for the harness"/>`)
		}
		b.WriteString(`</wb:topic>`)
		f.update(t, b.String())
	}
	for i := closed - 1; i >= 0; i-- {
		topic(fmt.Sprintf("C%02d", i), (panels+1)/2, true)
	}
	for i := open - 1; i >= 0; i-- {
		topic(fmt.Sprintf("T%02d", i), panels, false)
	}
	dir := t.TempDir()
	register := func() {
		reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root, HeartbeatAt: time.Now().UTC().Format(time.RFC3339)}
		if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
			t.Error(err)
			return
		}
		snapshot := fmt.Sprintf(`{"schemaVersion":1,"replyId":%q,"adapter":"pi","updatedAt":%d,"heartbeatMs":30000,"lead":{"phase":"idle"}}`, f.replyID(), time.Now().UnixMilli())
		if err := os.WriteFile(filepath.Join(wsdashboard.SessionDir(dir, f.replyID()), "snapshot.json"), []byte(snapshot), 0o600); err != nil {
			t.Error(err)
		}
	}
	register()
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return dir, nil }}
	daemon := startWhiteboardDaemonForTest(t, dir, src)
	// The session stays live (heartbeat) for as long as the harness runs.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				register()
			}
		}
	}()
	fmt.Printf("BOARD-PERF READY http://127.0.0.1:%d/s/%s\n", daemon.Endpoint().Port, f.replyID())
	seq := 0
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		cmd := strings.Fields(in.Text())
		if len(cmd) == 0 {
			continue
		}
		switch cmd[0] {
		case "quit":
			return
		case "log":
			n := 1
			if len(cmd) > 1 {
				if v, err := strconv.Atoi(cmd[1]); err == nil && v > 0 {
					n = v
				}
			}
			var b strings.Builder
			b.WriteString(`<wb:topic name="T00">`)
			for i := 0; i < n; i++ {
				seq++
				fmt.Fprintf(&b, "<wb:log>harness update %04d: one more line.</wb:log>", seq)
			}
			b.WriteString(`</wb:topic>`)
			f.update(t, b.String())
			if !daemon.Index().Nudge(f.replyID(), f.root) {
				t.Error("board perf: nudge refused")
			}
			fmt.Println("BOARD-PERF OK")
		default:
			fmt.Printf("BOARD-PERF ERR unknown command %q\n", cmd[0])
		}
	}
}

// panelBody is a synthetic panel payload of about 1 KB of markdown.
func panelBody(topic string, panel, version int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s panel %d, version %d\n\n", topic, panel, version)
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&b, "Line %d: findings for this panel with `code`, a [link](https://example.com/%d) and **emphasis** to render.\n\n", i, i)
	}
	b.WriteString("```go\nfunc example() int { return 42 }\n```\n")
	return b.String()
}
