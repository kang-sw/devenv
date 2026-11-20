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

func mustAsset(t *testing.T, name string) string {
	t.Helper()
	raw, err := dashboardTestAsset(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestBandContextStates pins the redesigned context bar across native fallback
// and ws-compaction-scaled threshold states (normal, soft info, 1/3 bell,
// 2/3 bell-ring, overflow crit zap, hard==native, and unknown "?").
func TestBandContextStates(t *testing.T) {
	comp := &wslive.UsageCompaction{
		Soft:            110_000,
		Hard:            180_000,
		Milestones:      []int64{133_333, 156_667},
		AdvisoryEnabled: true,
	}

	// Normal (28%, no floating icon, ticks present, native parenthetical shown).
	ctx, _ := bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: i64p(51_293), Window: 1_000_000, CacheHit: f64p(92.4), Compaction: comp,
	}}, nil)
	contains(t, "normal compaction", ctx,
		`class="sb-ctx"`, `<span class="sb-pct">28%</span>`, `width: 28.5%`, `aria-valuenow="28"`,
		"51,293 / 180K", `<span class="sb-native"> (1.0M)</span>`,
		`<u class="m-soft" style="left: 61.1%">`,
		`<u class="m-tick" style="left: 74.1%">`,
		`<u class="m-tick" style="left: 87.0%">`,
		`class="ctx-tip"`, `CH 92%`)
	lacks(t, "normal compaction", ctx, "sb-float", "sb-flag", "t80", "t95", "warn", "d-hot", "crit")

	// Soft crossed (info icon over Soft).
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: i64p(118_800), Window: 1_000_000, Compaction: comp,
	}}, nil)
	contains(t, "soft crossed", ctx, `class="sb-ctx warn"`, `<span class="sb-pct">66%</span>`, `#l-info`, `left:61.1%`)
	lacks(t, "soft crossed", ctx, "#l-bell", "#l-zap", "sb-cache")

	// Nudge 1/3 crossed (bell icon over 1/3 tick).
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: i64p(140_400), Window: 1_000_000, Compaction: comp,
	}}, nil)
	contains(t, "m1 crossed", ctx, `class="sb-ctx warn"`, `<span class="sb-pct">78%</span>`, `#l-bell"`, `left:74.1%`)
	lacks(t, "m1 crossed", ctx, "#l-info", "#l-bell-ring", "#l-zap")

	// Nudge 2/3 crossed (bell-ring icon over 2/3 tick, hot tone).
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: i64p(163_800), Window: 1_000_000, Compaction: comp,
	}}, nil)
	contains(t, "m2 crossed", ctx, `class="sb-ctx d-hot"`, `<span class="sb-pct">91%</span>`, `#l-bell-ring`, `left:87.0%`)

	// Overflow >= hard (unclamped %, 100% clamped fill, zap right-aligned).
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: i64p(189_000), Window: 1_000_000, Compaction: comp,
	}}, nil)
	contains(t, "overflow", ctx, `class="sb-ctx crit"`, `<span class="sb-pct">105%</span>`, `width: 100.0%`, `class="sb-float at-hard"`, `#l-zap`)

	// Hard == Native omits parenthetical.
	sameComp := &wslive.UsageCompaction{Soft: 120_000, Hard: 200_000, Milestones: []int64{146_667, 173_333}, AdvisoryEnabled: true}
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: i64p(90_000), Window: 200_000, Compaction: sameComp,
	}}, nil)
	contains(t, "hard==native", ctx, `<span class="sb-pct">45%</span>`, "90,000 / 200K")
	lacks(t, "hard==native", ctx, "sb-native")

	// Unknown context retains ticks when Compaction is present and shows no floating icon.
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: nil, Window: 1_000_000, Compaction: comp,
	}}, nil)
	contains(t, "unknown compaction", ctx, `class="sb-ctx d-unknown"`, `<span class="sb-pct">?</span>`, `width: 0.0%`, "? / 180K", `<span class="sb-native"> (1.0M)</span>`, `m-soft`, `m-tick`)
	lacks(t, "unknown compaction", ctx, "aria-valuenow", "sb-float")

	// Non-hosted / native fallback (Compaction == nil) shows no compaction ticks and no floating icon.
	ctx, _ = bandRegions(t, &wslive.Usage{Context: &wslive.UsageContext{
		Tokens: i64p(1_920_000), Window: 2_000_000, Percent: f64p(96),
	}}, nil)
	contains(t, "native fallback", ctx, `class="sb-ctx crit"`, `<span class="sb-pct">96%</span>`, "1,920,000 / 2.0M")
	lacks(t, "native fallback", ctx, "m-soft", "m-tick", "sb-float", "sb-native", "sb-flag")

	ctx, sline := bandRegions(t, nil, nil)
	if ctx != "" || sline != "" {
		t.Errorf("a band with no figures rendered:\nctx %q\nsline %q", ctx, sline)
	}
}

// TestSLineRetiresFolderLimitsAndCost pins that r-sline emits no folder,
// usage limit, or cost chips in the composer footer while BandView.Cost remains
// populated for the rail row hover card.
func TestSLineRetiresFolderLimitsAndCost(t *testing.T) {
	five := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC).UnixMilli()
	week := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC).UnixMilli()
	snap := liveSnap(testNow, wslive.Lead{})
	snap.Repo = &wslive.Repo{Folder: "devenv", Branch: "develop"}
	snap.Usage = &wslive.Usage{
		Limits: []wslive.UsageLimit{{Kind: wslive.LimitFiveHour, Percent: 97, ResetsAt: five}, {Kind: wslive.LimitSevenDay, Percent: 81.4, ResetsAt: week}},
		Cost:   &wslive.UsageCost{USD: 1.84, Estimate: true},
	}
	if bv := bandView(snap); bv.Cost != "~$1.84" {
		t.Fatalf("BandView.Cost = %q, want ~$1.84 for rail hover", bv.Cost)
	}
	_, sline := bandRegions(t, snap.Usage, snap.Repo)
	if sline != "" {
		t.Fatalf("r-sline should be empty, got %q", sline)
	}
}

// TestBandRegionsSitAroundTheMessageBox pins the placement of ctx above and
// sline below the message box on the live page, and the 2-control composer
// footer strip visibility across hosted lead, non-local, child, and external.
func TestBandRegionsSitAroundTheMessageBox(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{})
	snap.Usage = &wslive.Usage{Context: &wslive.UsageContext{Tokens: i64p(1), Window: 10}}

	render := func(hosted, isChild, canEdit bool, model, effort string) string {
		sess := liveSession("self", "/p", now)
		sess.Registration.Hosted = hosted
		view := buildSessionView(pageInputs{Session: sess, Snapshot: snap, Now: now, IsChild: isChild, Model: model, Effort: effort})
		view.CanEditSettings = canEdit
		regions, err := renderSessionRegions(view, nil)
		if err != nil {
			t.Fatal(err)
		}
		page, err := renderSessionPage(view, regions, false, "")
		if err != nil {
			t.Fatal(err)
		}
		return page
	}

	// Check only the rendered DOM (before inline <script> i18n/JS bundles) for pill markup.
	domOnly := func(page string) string {
		if i := strings.Index(page, "<script>"); i >= 0 {
			return page[:i]
		}
		return page
	}

	// External session omits #composer-pills while keeping ctx/sline anchors.
	extPage := render(false, false, false, "", "")
	composer, ctx, text, sline := strings.Index(extPage, `id="composer"`), strings.Index(extPage, `data-region="ctx"`), strings.Index(extPage, `id="ctext"`), strings.Index(extPage, `data-region="sline"`)
	if composer < 0 || !(composer < ctx && ctx < text && text < sline) {
		t.Fatalf("band regions out of place: composer %d ctx %d textarea %d sline %d", composer, ctx, text, sline)
	}
	lacks(t, "external session", domOnly(extPage), `id="composer-pills"`, `data-pop="perm"`, `shield`)

	// Hosted local lead shows exactly Model + Effort triggers (no Permission).
	leadDOM := domOnly(render(true, false, true, "opus-4.6", "high"))
	contains(t, "hosted local lead", leadDOM, `id="composer-pills"`, `data-pop="model"`, `data-pop="effort"`, `opus-4.6`, `high`, `#l-cpu`, `#l-gauge`, `#l-chevron-down`)
	lacks(t, "hosted local lead", leadDOM, `data-pop="perm"`, `#l-lock`, `shield`)

	// Non-local hosted lead disables triggers with host-machine tooltip (no lock icon).
	remoteDOM := domOnly(render(true, false, false, "opus-4.6", "high"))
	contains(t, "non-local hosted lead", remoteDOM, `id="composer-pills"`, `Controls are available on the host machine`, `disabled`)
	lacks(t, "non-local hosted lead", remoteDOM, `#l-lock`, `fixed at spawn`)

	// Child session (both worker and card-bound) renders locked read-only with lock icon.
	for _, hosted := range []bool{false, true} {
		childDOM := domOnly(render(hosted, true, false, "sonnet", "medium"))
		contains(t, "child session", childDOM, `id="composer-pills"`, `data-child="1"`, `#l-lock`, `fixed at spawn`, `sonnet`, `medium`, `disabled`)
		lacks(t, "child session", childDOM, `#l-chevron-down`, `Controls are available on the host machine`)
	}

	board := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Now: now})
	if regions, err := renderSessionRegions(board, nil); err != nil {
		t.Fatal(err)
	} else if _, ok := regions[regionCtx]; ok {
		t.Error("board page rendered a context bar")
	} else if _, ok := regions[regionSLine]; ok {
		t.Error("board page rendered a status line")
	}
}
