package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// TestUnifiedHistoryBrowser is an opt-in, one-case fixture. It serves real
// embedded pages over fake session data, with no harness, holder or provider.
// The same test-only helper can run in a retained baseline source archive;
// none of its production renderer or assets are replaced by current code.
func TestUnifiedHistoryBrowser(t *testing.T) {
	raw := os.Getenv("WS_BOARD_ACCEPTANCE_CASE")
	if raw == "" {
		t.Skip("set WS_BOARD_ACCEPTANCE_CASE for one owned acceptance case")
	}
	var c struct {
		Scenario string `json:"scenario"`
		Host     string `json:"host"`
		Lang     string `json:"lang"`
		Theme    string `json:"theme"`
		Width    int    `json:"width"`
		Before   bool   `json:"before"`
		Fixtures string `json:"fixtures"`
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if c.Host != "standalone" && c.Host != "dashboard" {
		t.Fatal("host must be standalone or dashboard")
	}
	if c.Lang != "en" && c.Lang != "ko" {
		t.Fatal("locale must be explicit en or ko")
	}
	allowed := map[string]bool{"history-unified": true, "history": true, "history-pinned-arrival": true, "history-fit": true, "history-regressions": true, "panel-header": true, "recap-jitter": true}
	if !allowed[c.Scenario] || c.Width < 320 || c.Width > 1900 {
		t.Fatal("unapproved scenario or viewport")
	}
	artifacts := os.Getenv("WS_BOARD_ACCEPTANCE_OUTPUT")
	harness := os.Getenv("WS_BOARD_ACCEPTANCE_HARNESS")
	gate := os.Getenv("WS_BOARD_ACCEPTANCE_GATE")
	for _, dir := range []string{artifacts, harness, gate} {
		if !filepath.IsAbs(dir) {
			t.Fatal("absolute output, harness and process gate paths required")
		}
	}
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")
	var board wswhiteboard.Regions
	var standalone string
	if c.Scenario == "history-unified" {
		var manifest struct {
			Stages []struct {
				Languages map[string]struct {
					Standalone string `json:"standalone"`
					Board      string `json:"board"`
				} `json:"languages"`
			} `json:"stages"`
		}
		data, err := os.ReadFile(filepath.Join(c.Fixtures, "fixtures.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &manifest); err != nil || len(manifest.Stages) == 0 {
			t.Fatalf("renderer manifest: %v", err)
		}
		first := manifest.Stages[0].Languages[c.Lang]
		// Preserve the generator's specimen; only this disposable live copy is
		// replaced during standalone stage/reload verification.
		data, err = os.ReadFile(filepath.Join(c.Fixtures, first.Standalone))
		if err != nil {
			t.Fatal(err)
		}
		standalone = filepath.Join(c.Fixtures, "live-"+c.Lang+".html")
		if err := os.WriteFile(standalone, data, 0600); err != nil {
			t.Fatal(err)
		}
		board.Board = template.HTML(first.Board)
		board.CountPolicy = 2
	} else {
		state := unifiedHistoryMotionState(t, c.Scenario == "recap-jitter")
		view := wswhiteboard.BuildView(state, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "historymotion", Location: time.UTC})
		view.Lang, view.StampFile = c.Lang, ""
		var err error
		board, err = wswhiteboard.RenderRegions(view)
		if err != nil {
			t.Fatal(err)
		}
		page, err := wswhiteboard.RenderPage(view)
		if err != nil {
			t.Fatal(err)
		}
		standalone = filepath.Join(artifacts, "motion.html")
		if err := os.WriteFile(standalone, []byte(page), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var origins []string
	address := (&url.URL{Scheme: "file", Path: standalone}).String()
	if c.Host == "dashboard" {
		dir, project, src := t.TempDir(), t.TempDir(), newPageFake()
		register(t, dir, "historysession", project, time.Now())
		src.setPage("historysession", PageContent{HasBoard: true, Seq: 1, Board: board})
		writeSnapshot(t, dir, "historysession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}})
		d := startIndexedDaemon(t, dir, src, time.Hour, func(o *DaemonOptions) {
			o.noGitBadge = true
			o.Tracker = &fakeTracker{}
		})
		base := endpointURL(d.Endpoint(), "")
		u, err := url.Parse(base)
		if err != nil || u.Port() == "29417" || u.Port() == "" {
			t.Fatal("fixture requires its own ephemeral port")
		}
		waitFor(t, "history fixture indexed", func() bool { _, ok := d.Index().Session("historysession"); return ok })
		origins = []string{base}
		address = base + "/s/historysession"
	}
	files := []string{standalone,
		filepath.Join(harness, "ai-docs/tickets/assets/261114-feat-ws-whiteboard-history-centered-header/approved-centered-header.fragment.html"),
		filepath.Join(harness, "ai-docs/tickets/assets/261114-feat-ws-whiteboard-history-unified-decisions/approved-density.fragment.html")}
	encodedOrigins, _ := json.Marshal(origins)
	encodedFiles, _ := json.Marshal(files)
	t.Setenv("WS_BOARD_ACCEPTANCE_ORIGINS", string(encodedOrigins))
	t.Setenv("WS_BOARD_ACCEPTANCE_FILES", string(encodedFiles))
	t.Setenv("NODE_OPTIONS", "--require="+filepath.Join(harness, "scripts/board-acceptance-safety.cjs"))
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{filepath.Join(harness, "scripts/board-headless.mjs"), c.Scenario, "--url", address, "--width", fmt.Sprint(c.Width), "--height", "900", "--lang", c.Lang, "--artifacts", artifacts, "--capture-dir", artifacts}
	if c.Fixtures != "" {
		args = append(args, "--fixtures", c.Fixtures)
	}
	if c.Before {
		args = append(args, "--before")
	}
	if c.Theme != "" {
		args = append(args, "--theme", c.Theme)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, args...)
	cmd.Dir = harness
	cmd.WaitDelay = 5 * time.Second
	t.Logf("case=%s; node=%s %q; fixture=%s", raw, node, args, address)
	out, err := cmd.CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(artifacts, "browser-process.log"), out, 0600); writeErr != nil {
		t.Error(writeErr)
	}
	t.Log(string(out))
	if err != nil {
		t.Fatalf("one-case unified history browser: %v", err)
	}
}

// Same command order as the existing --topics 8 --history-entries 180
// --shared-actions fixture, without executing a CLI or registering a real key.
// Stable e320/p156/e519 identities are consequences of the renderer's events.
func unifiedHistoryMotionState(t *testing.T, recapPeers bool) *wswhiteboard.State {
	t.Helper()
	state := &wswhiteboard.State{}
	clock := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	apply := func(commands string) {
		t.Helper()
		_, problems := wswhiteboard.Apply(state, commands, wswhiteboard.Env{Now: clock})
		if len(problems) != 0 {
			t.Fatal(problems)
		}
		clock = clock.Add(time.Second)
	}
	topic := func(i int) string {
		name := fmt.Sprintf("T%02d", i)
		body := "<h3>" + name + "</h3>"
		for p := 1; p <= 9; p++ {
			body += fmt.Sprintf("<p>%s paragraph %d: board motion fixture text that wraps over a couple of lines so the topic body frame stands well above a few hundred pixels tall, which is what makes the board overflow the viewport several times over.</p>", name, p)
		}
		return `<wb:topic name="` + name + `"><wb:body>` + body + `</wb:body></wb:topic>`
	}
	for i := 1; i <= 8; i++ {
		apply(topic(i))
	}
	var panels strings.Builder
	for i := 1; i <= 180; i++ {
		fmt.Fprintf(&panels, `<wb:panel recap="History panel %d"><p>History payload %d: delayed content height fixture.</p></wb:panel>`, i, i)
	}
	apply(`<wb:topic name="HISTORY">` + panels.String() + `</wb:topic>`)
	for _, spec := range []struct {
		name  string
		count int
	}{{"LOGCHAIN", 200}, {"LOGCANCEL", 600}} {
		var commands strings.Builder
		fmt.Fprintf(&commands, `<wb:topic name="%s"><wb:log>Opening history line</wb:log>`, spec.name)
		for i := 0; i < spec.count-4; i++ {
			value := "waiting"
			if i%2 != 0 {
				value = "discussing"
			}
			fmt.Fprintf(&commands, `<wb:state value="%s"/>`, value)
		}
		for i := 1; i <= 3; i++ {
			fmt.Fprintf(&commands, `<wb:log>Visible tail line %d</wb:log>`, i)
		}
		commands.WriteString(`</wb:topic>`)
		apply(commands.String())
	}
	apply(topic(9))
	apply(`<wb:topic name="SHARED"><wb:body><p>Shared action fixture</p></wb:body><wb:panel recap="Expanded panel"><p>Panel payload</p></wb:panel><wb:panel recap="Closed panel"><p>Second payload</p></wb:panel><wb:log>History fixture</wb:log><wb:add class="technical"><wb:title>Open question fixture</wb:title><wb:option>Yes</wb:option><wb:option>No</wb:option></wb:add></wb:topic>`)
	if recapPeers {
		// Recap scroll probes need these real body peers below the pinned log.
		apply(topic(17))
		apply(topic(18))
	}
	return state
}
