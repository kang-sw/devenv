package wswhiteboard

import (
	"strings"
	"testing"
)

// TestRenderRegionsMatchThePage pins the dashboard's reuse contract: the
// board and panel fragments are exactly what the full page renders for the
// same view, so a served page cannot drift from the static one.
func TestRenderRegionsMatchThePage(t *testing.T) {
	view := sampleView(LayoutFull)
	page := render(t, view)
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	if regions.Board == "" || regions.Panels == "" {
		t.Fatalf("empty regions: %+v", regions)
	}
	if !strings.Contains(page, string(regions.Board)) {
		t.Errorf("the page does not hold the board fragment:\n%.400s", regions.Board)
	}
	if !strings.Contains(page, string(regions.Panels)) {
		t.Errorf("the page does not hold the panels fragment:\n%.400s", regions.Panels)
	}
	if regions.CountPolicy == 0 {
		t.Errorf("counts not carried: %+v", regions)
	}
	assets, err := PageAssets()
	if err != nil || assets.CSS == "" || assets.Sprite == "" || assets.FormScript == "" || assets.MermaidInit == "" {
		t.Fatalf("shared assets missing: %v", err)
	}
}

// TestRenderRegionsMermaidFlag pins the mermaid flag: set only when the board
// holds a mermaid block.
func TestRenderRegionsMermaidFlag(t *testing.T) {
	st := &State{Seq: 1, Items: []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "x", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "```mermaid\ngraph TD; A-->B\n```"}}}
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	if !regions.Mermaid || !strings.Contains(string(regions.Board), `<pre class="mermaid">`) {
		t.Fatalf("mermaid flag not set: %+v", regions.Mermaid)
	}
	plain, _ := RenderRegions(sampleView(LayoutFull))
	if plain.Mermaid {
		t.Fatalf("mermaid flag set on a board without a mermaid block")
	}
}

// TestFileTextRendersFileTargetsAsPathText pins the served-page link rule:
// with FileText set, a file link keeps its label and shows the path as text,
// a file image shows its alt text and path, a file autolink shows its path,
// and none of them yields an href or src; http links and in-page anchors are
// unaffected.
func TestFileTextRendersFileTargetsAsPathText(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "files", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "see [the plan](ai-docs/plan.md) and [ai-docs/x.md](ai-docs/x.md)\n\n" +
			"![diagram](shots/a.png)\n\n" +
			"<file:///abs/notes.md> and [abs](/abs/b.md) and [web](https://example.com/) and [anchor](#item-1)"}}
	text := func(p string) string { return "REL:" + strings.TrimPrefix(p, "/") }
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", FileText: text,
		FileLink: func(p string) string { return "MUST-NOT-USE/" + p }}))
	wantAll(t, out,
		`the plan <code class="fpath">REL:ai-docs/plan.md</code>`,
		`<code class="fpath">REL:ai-docs/x.md</code>`,
		`<span class="img-text">diagram <code class="fpath">REL:shots/a.png</code></span>`,
		`<code class="fpath">REL:abs/notes.md</code>`,
		`abs <code class="fpath">REL:abs/b.md</code>`,
		`<a href="https://example.com/"`,
		`<a href="#item-1">anchor</a>`,
	)
	wantNone(t, out, "MUST-NOT-USE", "file:", `src="shots`, `href="ai-docs`, "<img")
	// A label equal to the path is not repeated.
	if strings.Count(out, "ai-docs/x.md") != 2 { // the label and the code span
		t.Errorf("a label equal to its path must show once beside the path text")
	}
}

// TestImageSrcServesFileImages pins the served-image rule: with FileText and
// ImageSrc set, a file image renders as <img src> carrying the ImageSrc
// result (its query and fragment survive escaping, the destination's own
// suffix is dropped), an image whose ImageSrc result is empty keeps the
// path text, and a file link still renders path text.
func TestImageSrcServesFileImages(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "files", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "![diagram](shots/a.png?x=1#frag)\n\n![outside](/abs/b.png)\n\n[the plan](shots/a.png)"}}
	imageSrc := func(p string) string {
		if p == "shots/a.png" {
			return "/s/r/image?path=shots%2Fa.png"
		}
		return ""
	}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r",
		FileText: func(p string) string { return "REL:" + p }, ImageSrc: imageSrc}))
	wantAll(t, out,
		`<img src="/s/r/image?path=shots%2Fa.png" alt="diagram">`,
		`<span class="img-text">outside <code class="fpath">REL:/abs/b.png</code></span>`,
		`the plan <code class="fpath">REL:shots/a.png</code>`,
	)
	wantNone(t, out, "x=1", "#frag", `<a href="/s/`)
	if strings.Count(out, "<img") != 1 {
		t.Errorf("only the served image renders as <img>:\n%s", out)
	}
}

// TestPanelRowPathText pins a linkless row's path: shown after its text.
func TestPanelRowPathText(t *testing.T) {
	panels := []Panel{{ID: "tickets", Title: "Tickets", Icon: "board", Rows: []PanelRow{
		{Label: "ready", Text: "A ticket", Path: "ai-docs/tickets/ready/x.md"},
	}}}
	out := render(t, BuildView(&State{}, ViewInput{Layout: LayoutFull, ReplyID: "r", Panels: panels}))
	wantAll(t, out, `<span>A ticket <code class="fpath">ai-docs/tickets/ready/x.md</code></span>`)
}

// TestPanelLanesRenderCards pins the kanban lane markup: a lane header with
// name, count, and note; a card with a category chip, a title that links or
// carries its path as a tooltip, and a stem line; and an empty lane saying so.
func TestPanelLanesRenderCards(t *testing.T) {
	panels := []Panel{{ID: "tickets", Title: "Tickets", Icon: "board", Lanes: []PanelLane{
		{Name: "ready", Count: "2", Rows: []PanelRow{
			{Label: "epic", Text: "An epic", Sub: "261101-epic-x", Link: "../t/x.md"},
			{Label: "feat", Text: "Served", Sub: "261102-feat-y", Path: "ai-docs/tickets/ready/y.md"},
		}},
		{Name: "todo", Count: "10", Note: "latest 10 of 12"},
	}}}
	out := render(t, BuildView(&State{}, ViewInput{Layout: LayoutFull, ReplyID: "r", Panels: panels}))
	wantAll(t, out,
		`<span class="lname">ready</span><span class="chip">2</span>`,
		`<span class="lnote">latest 10 of 12</span>`,
		`<li class="kcard"><span class="ctitle"><span class="chip cat epic">epic</span><a href="../t/x.md">An epic</a></span><span class="csub" title="261101-epic-x">261101-epic-x</span></li>`,
		`<span class="ctitle" title="ai-docs/tickets/ready/y.md"><span class="chip cat feat">feat</span>Served</span>`,
		`<li class="none">none</li>`,
	)
	wantNone(t, out, `<code class="fpath">ai-docs/tickets/ready/y.md</code>`)
}
