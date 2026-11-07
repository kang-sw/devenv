package wsdashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func i64p(v int64) *int64     { return &v }
func f64p(v float64) *float64 { return &v }

// bandRegions renders the band's two regions for a snapshot carrying usage
// and repo.
func bandRegions(t *testing.T, usage *wslive.Usage, repo *wslive.Repo) (ctx, sline string) {
	t.Helper()
	now := testNow
	snap := liveSnap(now, wslive.Lead{})
	snap.Usage, snap.Repo = usage, repo
	set, err := renderSessionRegions(buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now}), nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(set[regionCtx].HTML), string(set[regionSLine].HTML)
}

func contains(t *testing.T, what, html string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(html, want) {
			t.Errorf("%s lacks %q:\n%s", what, want, html)
		}
	}
}

func lacks(t *testing.T, what, html string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(html, u) {
			t.Errorf("%s has %q:\n%s", what, u, html)
		}
	}
}

// TestBandContextStates pins the context bar: normal, warning at 80%, and
// critical at 95%, each with its percent, fill, and token figures; the cache
// chip; and "?" when the host does not know its context size (after a
// compaction), which shows no percent, no fill, and no warning.
func TestBandContextStates(t *testing.T) {
	ctx, _ := bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{Tokens: i64p(137_609), Window: 300_000, Percent: f64p(45.87), CacheHit: f64p(92.4)}}, nil)
	contains(t, "normal context", ctx, `class="sb-ctx"`, `<span class="sb-pct">46%</span>`, `width: 46%`, `aria-valuenow="46"`, "137,609 / 300K", `CH 92%`)
	lacks(t, "normal context", ctx, "sb-flag", "warn", "crit")

	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{Tokens: i64p(249_100), Window: 300_000, Percent: f64p(83)}}, nil)
	contains(t, "warning context", ctx, `class="sb-ctx warn"`, `<span class="sb-flag">high</span>`, "249,100 / 300K")
	lacks(t, "warning context", ctx, "sb-cache")

	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{Tokens: i64p(1_920_000), Window: 2_000_000, Percent: f64p(96)}}, nil)
	contains(t, "critical context", ctx, `class="sb-ctx crit"`, `<span class="sb-flag">near limit</span>`, "1,920,000 / 2M")

	// Past 100% the label keeps the figure and the fill stops at the end.
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{Tokens: i64p(210_000), Window: 200_000, Percent: f64p(105)}}, nil)
	contains(t, "overfull context", ctx, `<span class="sb-pct">105%</span>`, `width: 100%`)

	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{Tokens: nil, Window: 200_000}}, nil)
	contains(t, "unknown context", ctx, `<span class="sb-pct">?</span>`, `width: 0%`, "? / 200K", "unknown until the next response")
	lacks(t, "unknown context", ctx, "aria-valuenow", "sb-flag")

	ctx, sline := bandRegions(t, nil, nil)
	if ctx != "" || sline != "" {
		t.Errorf("a band with no figures rendered:\nctx %q\nsline %q", ctx, sline)
	}
}

// TestBandRepoStates pins the line's git parts: no upstream, in sync, ahead
// and behind, ahead only, a detached HEAD (no sync chip), and a root that is
// not a repository (the folder alone). They show with no usage figures at
// all, since ws-mcp reads them itself.
func TestBandRepoStates(t *testing.T) {
	_, sline := bandRegions(t, nil, &wslive.Repo{Folder: "devenv", Branch: "topic/band"})
	contains(t, "no upstream", sline, `<span>devenv</span>`, `<span>topic/band</span>`, `class="sb-sync none"`, "no upstream")
	lacks(t, "no upstream", sline, "sb-tail")

	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "devenv", Branch: "develop", Upstream: "origin/develop"})
	contains(t, "in sync", sline, `class="sb-sync ok"`, "synced", "in sync with origin/develop")
	lacks(t, "in sync", sline, "no upstream", `class="up"`, `class="dn"`)

	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "devenv", Branch: "develop", Upstream: "origin/develop", Ahead: 2, Behind: 1})
	contains(t, "ahead and behind", sline, `<span class="up">↑2</span>`, `<span class="dn">↓1</span>`, "2 to push", "1 to pull")
	lacks(t, "ahead and behind", sline, "synced")

	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "devenv", Branch: "develop", Upstream: "origin/develop", Ahead: 3})
	contains(t, "ahead only", sline, `<span class="up">↑3</span>`)
	lacks(t, "ahead only", sline, `class="dn"`)

	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "devenv", Branch: "1a2b3c4", Detached: true})
	contains(t, "detached", sline, "detached HEAD at 1a2b3c4")
	lacks(t, "detached", sline, "sb-sync")

	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "scratch"})
	contains(t, "not a repository", sline, `<span>scratch</span>`)
	lacks(t, "not a repository", sline, "sb-br", "sb-sync")

	// The working-tree chip: each non-zero count with its glyph and a title
	// spelling the counts; a clean tree shows no chip.
	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "d", Branch: "b", Upstream: "origin/b", Modified: 3, Staged: 1, Untracked: 2})
	contains(t, "dirty chip", sline, `class="sb-diff"`, `<b class="mod">~3</b>`, `<b class="add">+1</b>`, `<b class="unt">?2</b>`, "working tree: 3 modified 1 staged 2 untracked")
	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "d", Branch: "b", Untracked: 4})
	contains(t, "untracked only", sline, `<b class="unt">?4</b>`, "working tree: 4 untracked")
	lacks(t, "untracked only", sline, `class="mod"`, `class="add"`)
	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "d", Branch: "1a2b3c4", Detached: true, Modified: 1})
	contains(t, "detached dirty", sline, `<b class="mod">~1</b>`)
	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "d", Branch: "b", Upstream: "origin/b"})
	lacks(t, "clean tree", sline, "sb-diff")

	// Branch names reach the page escaped.
	_, sline = bandRegions(t, nil, &wslive.Repo{Folder: "d", Branch: "<b>x"})
	contains(t, "escaped branch", sline, "&lt;b&gt;x")
}

// TestBandLimitAndCostChips pins the line's tail: the 5-hour chip first and
// the weekly one second, each with its reset for the page to format (a clock
// time, a weekday) and its pace inputs; warning and critical levels; a chip
// only for a reported limit; Claude's exact cost bare and Pi's estimate as
// ~$X or ~$X+; and the narrow-band rule's markers: with both chips shown,
// both carry "both" and the higher percent "hi" (a tie keeps the 5-hour
// limit), which the stylesheet keeps alone below a 360px band (340px of the
// line inside the composer's padding).
func TestBandLimitAndCostChips(t *testing.T) {
	five := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC).UnixMilli()
	week := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC).UnixMilli() // a Sunday
	_, sline := bandRegions(t, &wslive.Usage{
		Limits: []wslive.UsageLimit{{Kind: wslive.LimitFiveHour, Percent: 97, ResetsAt: five}, {Kind: wslive.LimitSevenDay, Percent: 81.4, ResetsAt: week}},
		Cost:   &wslive.UsageCost{USD: 99.314},
	}, nil)
	i5, i7 := strings.Index(sline, `data-limit="five_hour"`), strings.Index(sline, `data-limit="seven_day"`)
	if i5 < 0 || i7 < 0 || i5 > i7 {
		t.Fatalf("limit chips missing or out of order:\n%s", sline)
	}
	contains(t, "both limits", sline,
		`class="sb-use crit both hi" data-limit="five_hour"`, `class="sb-use warn both" data-limit="seven_day"`,
		`<span class="u-pct">97%</span>`, `<span class="u-pct">81%</span>`,
		`data-f="hm">18:00</time>`, `data-f="wd">Sun</time>`,
		`data-win="18000" data-pct="97"`, `data-win="604800" data-pct="81"`,
		`<span class="sb-cost" title="session cost">$99.31</span>`)
	lacks(t, "both limits", sline, "~$", "wk")

	// A tie keeps the 5-hour limit as the one that stays.
	_, sline = bandRegions(t, &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: wslive.LimitFiveHour, Percent: 40}, {Kind: wslive.LimitSevenDay, Percent: 40}}}, nil)
	contains(t, "tied limits", sline, `class="sb-use both hi" data-limit="five_hour"`, `class="sb-use both" data-limit="seven_day"`)
	lacks(t, "tied limits", sline, "u-pace", "<time")

	_, sline = bandRegions(t, &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: wslive.LimitSevenDay, Percent: 17, ResetsAt: week}}}, nil)
	contains(t, "weekly only", sline, `class="sb-use" data-limit="seven_day"`)
	lacks(t, "weekly only", sline, "five_hour", "both", "sb-cost")

	// Pi: no limits, the estimate's forms.
	_, sline = bandRegions(t, &wslive.Usage{Cost: &wslive.UsageCost{USD: 1.234, Estimate: true}}, &wslive.Repo{Folder: "p"})
	contains(t, "pi estimate", sline, `<span class="sb-cost" title="session cost">~$1.23</span>`)
	lacks(t, "pi estimate", sline, "sb-use", "&#43;")
	_, sline = bandRegions(t, &wslive.Usage{Cost: &wslive.UsageCost{USD: 1.234, Estimate: true, Incomplete: true}}, nil)
	// html/template writes the "+" as &#43;.
	contains(t, "pi incomplete estimate", sline, ">~$1.23&#43;</span>")

	css := mustAsset(t, "dashboard.css")
	contains(t, "stylesheet", css, ".r-sline { container-type: inline-size; }", "@container (max-width: 340px) { .sb-use.both:not(.hi) { display: none; } }")
}

func mustAsset(t *testing.T, name string) string {
	t.Helper()
	raw, err := dashboardFS.ReadFile("assets/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestBandRegionsSitAroundTheMessageBox pins the placement (variant C): the
// context bar region opens the composer above the message box and the line
// region follows the message box, both only on the live page.
func TestBandRegionsSitAroundTheMessageBox(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{})
	snap.Usage = &wslive.Usage{Context: &wslive.UsageContext{Tokens: i64p(1), Window: 10}}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now})
	regions, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := renderSessionPage(view, regions, false)
	if err != nil {
		t.Fatal(err)
	}
	composer, ctx, text, sline := strings.Index(page, `id="composer"`), strings.Index(page, `data-region="ctx"`), strings.Index(page, `id="ctext"`), strings.Index(page, `data-region="sline"`)
	if composer < 0 || !(composer < ctx && ctx < text && text < sline) {
		t.Fatalf("band regions out of place: composer %d ctx %d textarea %d sline %d", composer, ctx, text, sline)
	}

	board := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Now: now})
	regions, err = renderSessionRegions(board, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := regions[regionCtx]; ok {
		t.Error("the board-first page renders the band")
	}
}
