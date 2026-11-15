package wsdashboard

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRailFourStateLifecycle pins how the rail places the four session
// states: an active or asleep hosted lead stays among the live rows, an
// archived one (the dashboard's visibility sidecar) leaves them for the
// archived fold and never counts as live, and an ended row shows when the
// existing registry prune deletes it, unless the prune exempts it.
func TestRailFourStateLifecycle(t *testing.T) {
	now := testNow
	beat := stampFor(now)
	old := stampFor(now.Add(-36 * time.Hour))
	in := func(reg Registration, hidden bool) railInput {
		reg.Project, reg.Harness, reg.RegisteredAt = "/w/repo", "claude", old
		return railInput{Session: Session{Registration: reg, Hidden: hidden}, Repo: "/w/repo", CanRevive: true}
	}
	sessions := []railInput{
		in(Registration{ReplyID: "activeon", Hosted: true, HeartbeatAt: beat}, false),
		in(Registration{ReplyID: "asleep", Hosted: true, HeartbeatAt: beat, HostStopped: true, HostExitReason: "stop"}, false),
		in(Registration{ReplyID: "archived", Hosted: true, HeartbeatAt: beat, HostStopped: true, HostExitReason: "stop"}, true),
		// A stale archived lead stays archived; its open record exempts it
		// from the prune.
		in(Registration{ReplyID: "stalearch", Hosted: true, HeartbeatAt: old}, true),
		in(Registration{ReplyID: "closed", Hosted: true, HeartbeatAt: old, EndedAt: old}, true),
		in(Registration{ReplyID: "terminal", HeartbeatAt: old}, false),
		// An open hosted lead with a stale row is the runner's: no prune time.
		in(Registration{ReplyID: "openstale", Hosted: true, HeartbeatAt: old}, false),
	}
	repos := map[string]railRepo{"/w/repo": {Read: true, Entries: []railWorktreeEntry{{Path: "/w/repo", Branch: "main"}}}}
	view := buildRailView(sessions, repos, nil, now)
	splitRailArchived(&view)
	wt := view.Workspaces[0].Worktrees[0]
	ids := func(rows []RailRow) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.ReplyID)
		}
		return out
	}
	if got := strings.Join(ids(wt.Sessions), ","); got != "activeon,asleep" {
		t.Errorf("live rows = %s, want activeon,asleep", got)
	}
	if got := strings.Join(ids(wt.Archived), ","); got != "archived,stalearch" {
		t.Errorf("archived rows = %s, want archived,stalearch", got)
	}
	if got := strings.Join(ids(wt.Ended), ","); got != "closed,terminal,openstale" && got != "closed,openstale,terminal" && got != "terminal,closed,openstale" {
		t.Errorf("ended rows = %s, want closed, terminal and openstale", got)
	}
	if view.Live != 2 || view.Workspaces[0].Live != 2 {
		t.Errorf("live counts = %d/%d, want 2: an archived lead is not live", view.Live, view.Workspaces[0].Live)
	}
	byID := map[string]RailRow{}
	for _, list := range [][]RailRow{wt.Sessions, wt.Archived, wt.Ended} {
		for _, r := range list {
			byID[r.ReplyID] = r
		}
	}
	if r := byID["archived"]; !r.Archived || r.Live || !r.Stopped() || r.Revive != "" || r.DeleteAt != 0 {
		t.Errorf("archived row = %+v, want archived, asleep, not live, no revive, no prune time", r)
	}
	if r := byID["asleep"]; r.Archived || !r.Stopped() {
		t.Errorf("asleep row = %+v, want stopped and not archived", r)
	}
	wantDel := now.Add(-36 * time.Hour).Add(RegistryRetention).UnixMilli()
	for _, id := range []string{"closed", "terminal"} {
		if r := byID[id]; r.Archived || r.DeleteAt != wantDel {
			t.Errorf("%s DeleteAt = %d (archived %v), want %d: the existing 7-day window from its last sighting", id, r.DeleteAt, r.Archived, wantDel)
		}
	}
	if r := byID["openstale"]; r.DeleteAt != 0 {
		t.Errorf("an open hosted lead's stale row shows a prune time %d; the prune exempts it", r.DeleteAt)
	}

	region, err := renderRailRegion(view, "ko")
	if err != nil {
		t.Fatal(err)
	}
	html := string(region.HTML)
	for _, want := range []string{
		`<div class="rfold" role="group" aria-label="접힌 세션" data-key="/w/repo">`,
		`class="rfh" data-k="arch" aria-expanded="false" title="보관된 세션 2개"><svg class="i" aria-hidden="true"><use href="#i-archive"/></svg>보관 <span class="d-n">2</span></button>`,
		`class="rfh" data-k="del" aria-expanded="false" title="종료된 세션 3개"><svg class="i" aria-hidden="true"><use href="#i-clock"/></svg>종료됨 <span class="d-n">3</span></button>`,
		`<div class="rended" data-k="arch"><div class="rsessions"><div class="rs ended stopped archived hosted-row" data-rid="archived"`,
		`<div class="rended" data-k="del">`,
		`class="drarch unarch" data-act="unhide" title="보관 해제 · 잠듦으로 돌아감, 쌓인 메일 전달"`,
		`<time class="when del" data-ms="` + strconv.FormatInt(wantDel, 10) + `" data-f="del"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rail lacks %q", want)
		}
	}
	activeRow, _, _ := strings.Cut(strings.SplitN(html, `data-rid="activeon"`, 2)[1], `data-rid="asleep"`)
	if !strings.Contains(activeRow, `class="drarch" data-act="hide" title="보관 · 프로세스를 끝내고 레일에서 숨김"`) {
		t.Errorf("an active hosted row lacks its archive button:\n%s", activeRow)
	}
	termRow := strings.SplitN(html, `data-rid="terminal"`, 2)[1]
	termRow, _, _ = strings.Cut(termRow, `<div class="rs`)
	if strings.Contains(termRow, "drarch") {
		t.Errorf("a terminal row offers archive:\n%s", termRow)
	}

	// No archived rows: the archived half is disabled, the ended one is not.
	plain := RailView{Workspaces: []RailWorkspace{{Name: "repo", Path: "/w/repo", Repo: true, Worktrees: []RailWorktree{{Path: "/w/repo", Ended: []RailRow{{ReplyID: "gone"}}}}}}}
	region, err = renderRailRegion(plain, "en")
	if err != nil {
		t.Fatal(err)
	}
	if html := string(region.HTML); !strings.Contains(html, `data-k="arch" aria-expanded="false" title="0 archived" disabled>`) || strings.Contains(html, `data-k="del" aria-expanded="false" title="1 ended" disabled`) {
		t.Errorf("an empty archived half must be disabled and the ended half enabled:\n%s", html)
	}
}
