package wsdashboard

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func dashboardEffect(value string) wslive.RingEntry {
	entry, _ := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{
		{Kind: "panel", Topic: "plan", EntryID: "e1", PanelID: "p1", Field: "payload", Before: "old <b>value</b>", After: value},
	})
	return entry
}

func TestWhiteboardEffectsEscapedExpandableAndStable(t *testing.T) {
	entry := dashboardEffect(`<script>alert(1)</script><img src=x onerror="attack()"> **not markdown**`)
	entry.Truncated = true
	ring := []wslive.RingRecord{ringRecord(1, entry)}
	set, err := renderSessionRegions(historyView(ring, "", PageContent{}, sessionLookup{}), newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	html := string(set[regionEntries].HTML)
	for _, want := range []string{`<details class="wbeffects" data-entry="` + ring[0].Name + `" data-key="whiteboard-` + ring[0].Name + `">`, "Committed whiteboard update", "Before", "After", "old &lt;b&gt;value&lt;/b&gt;", "&lt;script&gt;alert(1)&lt;/script&gt;", "**not markdown**", "Some changes or values were omitted", "panel [plan] e1 p1 · payload"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q: %s", want, html)
		}
	}
	for _, bad := range []string{"<script", "<img", "<iframe", "<strong>not markdown", `class="wbeffects" open`} {
		if strings.Contains(html, bad) {
			t.Errorf("executable/expanded effects %q: %s", bad, html)
		}
	}
}

func TestWhiteboardEffectsVisibleBoundedFallback(t *testing.T) {
	for _, text := range []string{
		`<img src=x onerror="attack()">` + strings.Repeat("雪", 5000),
		`{"v":2,"effects":[{"kind":"item","id":1,"field":"title","after":"new"}]}`,
		`{"v":1,"effects":[{"kind":"future","field":"title","after":"invented"}]}`,
	} {
		ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingWhiteboardEffects, Text: text})}
		set, err := renderSessionRegions(historyView(ring, "", PageContent{}, sessionLookup{}), nil)
		if err != nil {
			t.Fatal(err)
		}
		html := string(set[regionEntries].HTML)
		if !strings.Contains(html, "Details unavailable or unsupported") || !strings.Contains(html, `class="wbeffect-fallback"`) || !strings.Contains(html, `data-entry="`+ring[0].Name) {
			t.Fatalf("missing visible fallback: %s", html)
		}
		if strings.Contains(html, "<img") || strings.Contains(html, `class="wbeffect-values"`) || len(html) > 20*1024 {
			t.Fatalf("unbounded/interpreted fallback: %.400s", html)
		}
	}
}

func TestWhiteboardEffectsSavedLeadModeWithoutLiveState(t *testing.T) {
	now := testNow
	stale := liveSnap(now.Add(-time.Hour), wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "stale streaming text"}})
	for _, snap := range []*wslive.Snapshot{nil, stale} {
		session := liveSession("self", "/proj", now.Add(-time.Hour))
		v := buildSessionView(pageInputs{Session: session, Snapshot: snap, Ring: []wslive.RingRecord{ringRecord(1, dashboardEffect("saved only"))}, Now: now})
		if v.Adapter || v.Live || v.ProcessRunning || !v.SavedConversation || v.Mode() != modeHistory || v.Conversation.Reply != nil || v.Conversation.Phase != "" || v.Host != nil || len(v.Commands) != 0 {
			t.Fatalf("fabricated live state: %+v", v)
		}
		regions, err := renderSessionRegions(v, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := regions[regionEntries]; !ok {
			t.Fatal("saved entries region omitted")
		}
		for _, absent := range []string{regionTree, regionConvLive, regionCtx, regionCmds, regionSLine} {
			if _, ok := regions[absent]; ok {
				t.Errorf("live region %s present", absent)
			}
		}
		page, err := renderSessionPage(v, regions, true, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`data-mode="history"`, `id="tabbar"`, `id="convo"`, "saved only"} {
			if !strings.Contains(page, want) {
				t.Errorf("saved page lacks %q", want)
			}
		}
		for _, absent := range []string{`id="composer"`, `id="ctext"`, `id="splitter"`, `id="apbox"`, "stale streaming text"} {
			if strings.Contains(page, absent) {
				t.Errorf("saved page has %q", absent)
			}
		}
		// An empty legacy board remains board-only, and a runner child is
		// never promoted to an adapter-less lead history page.
		if empty := buildSessionView(pageInputs{Session: session, Now: now}); empty.Mode() != modeBoard {
			t.Fatal("empty board changed mode")
		}
		session.Agent = true
		if child := buildSessionView(pageInputs{Session: session, Ring: []wslive.RingRecord{ringRecord(1, dashboardEffect("child"))}, Now: now}); child.SavedConversation {
			t.Fatal("child promoted")
		}
	}
}

func TestWhiteboardEffectsPinnedOldRendererOmitsOnlyNewKind(t *testing.T) {
	// Full unedited template at 4ddb1910537678e2e8125ee448a1d26d6f4010ec.
	old, err := os.ReadFile("testdata/pre_effects/dashboard.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(old)); got != "3d68bf0856ccfebf24cb2b063c3298dabefa31a208be8ca119fe2be3f182b504" {
		t.Fatal("old renderer fixture changed: " + got)
	}
	a, err := buildDashboardAssets(func(name string) ([]byte, error) {
		if name == "dashboard.tmpl" {
			return old, nil
		}
		return embeddedDashboardAsset(name)
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := []ConvEntry{
		{RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: "legacy user"}, Key: "user"},
		{RingEntry: dashboardEffect("must be omitted"), Key: "effect"},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingTools, Tools: toolsSummary()}, Key: "tools"},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCommand, Command: "/legacy", Text: "legacy output"}, Key: "command"},
	}
	var buf bytes.Buffer
	for _, entry := range entries {
		if err := a.tmpl.ExecuteTemplate(&buf, "entry", entry); err != nil {
			t.Fatal(err)
		}
	}
	html := buf.String()
	for _, want := range []string{"legacy user", "legacy output", `data-entry="tools"`, "whiteboard.update"} {
		if !strings.Contains(html, want) {
			t.Errorf("old renderer lost %q", want)
		}
	}
	if strings.Contains(html, "must be omitted") || strings.Contains(html, `data-entry="effect"`) {
		t.Fatal("old renderer interpreted new effects")
	}
}

func TestWhiteboardEffectsHistoryLayoutAndClientGuards(t *testing.T) {
	css, err := dashboardTestAsset("assets/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	js, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	// Static tripwires, not browser geometry/interaction acceptance.
	for _, want := range []string{`.dgrid.history[data-tab="board"] > .convo`, `.dgrid.history[data-tab] > .bcol`, `.wbeffect-values { grid-template-columns: minmax(0, 1fr); }`, `white-space: pre-wrap; overflow-wrap: anywhere`} {
		if !strings.Contains(string(css), want) {
			t.Errorf("missing responsive guard %q", want)
		}
	}
	for _, want := range []string{`mode !== "history" ? WsRail.drafts`, `var liveMode = mode === "live"`, `q("details[data-key]"`, `convo.addEventListener("scroll", nearTop)`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("missing client guard %q", want)
		}
	}
}
