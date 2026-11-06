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

// TestPanelRowPathText pins a linkless row's path: shown after its text.
func TestPanelRowPathText(t *testing.T) {
	panels := []Panel{{ID: "tickets", Title: "Tickets", Icon: "board", Rows: []PanelRow{
		{Label: "ready", Text: "A ticket", Path: "ai-docs/tickets/ready/x.md"},
	}}}
	out := render(t, BuildView(&State{}, ViewInput{Layout: LayoutFull, ReplyID: "r", Panels: panels}))
	wantAll(t, out, `<span>A ticket <code class="fpath">ai-docs/tickets/ready/x.md</code></span>`)
}
