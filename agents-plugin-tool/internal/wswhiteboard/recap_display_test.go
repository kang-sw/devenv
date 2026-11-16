package wswhiteboard

import (
	"regexp"
	"strings"
	"testing"
)

func TestRecapDisplayClampAndContentSizedHeader(t *testing.T) {
	data, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	wantAll(t, css, `.trecap { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 4; line-clamp: 4; overflow: hidden; }`)
	slot := regexp.MustCompile(`(?m)^\.tsub > \.trecap \{[^}]+\}`).FindString(css)
	wantAll(t, slot, `max-height: 5.8em;`, `font-size: 12.5px;`, `line-height: 1.45;`, `min-width: 0;`, `background: none;`)
	if regexp.MustCompile(`(?:\{|;)\s*(?:height|min-height)\s*:`).MatchString(slot) {
		t.Fatal("header recap reserves blank lines instead of shrinking to content")
	}
	// The author badge must not enlarge the first line beyond the line grid.
	wantAll(t, css, `.tsub > .trecap .rat .evi { width: 14px; height: 14px; vertical-align: text-bottom; }`,
		`.evi.wb-evi-panel { background: color-mix(in srgb, var(--wb-fg) 11%, transparent); color: var(--wb-muted-fg); }`)
	// Sticking must not change the shared maximum or content-driven height.
	if regexp.MustCompile(`(?m)^[^\n{]*\.wb-stuck[^\n{]*\.trecap\s*\{`).MatchString(css) {
		t.Fatal("stuck recap overrides the shared clamp/height")
	}
}

func TestRecapDisplayPanelTitleIsNotClipped(t *testing.T) {
	data, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	wantAll(t, css, `.entry-panel > summary > .entry-recap { grid-column: 2; grid-row: 1; line-height: 24px; }`)
	rules := regexp.MustCompile(`(?m)^([^{}\n]+)\{([^{}]*)\}`).FindAllStringSubmatch(css, -1)
	clipping := regexp.MustCompile(`(?:^|;)\s*(?:(?:-webkit-)?line-clamp\s*:\s*[1-9]|overflow(?:-x|-y)?\s*:\s*(?:hidden|clip)|text-overflow\s*:\s*ellipsis|(?:max-)?height\s*:)`)
	for _, rule := range rules {
		if strings.Contains(rule[1], ".entry-recap") && clipping.MatchString(rule[2]) {
			t.Fatalf("panel title selector clips its full text: %s{%s}", rule[1], rule[2])
		}
	}
}

func TestRecapDisplayFullTextTitles(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="agent"><wb:log>agent "full" text</wb:log></wb:topic><wb:topic name="current"><wb:log>old log</wb:log><wb:panel recap="panel &quot;full&quot; &amp; text">body</wb:panel><wb:panel id="p1" recap="new &quot;panel&quot; &amp; text">new body</wb:panel></wb:topic>`)
	legacyRecap(st, "legacy", `legacy "full" & text`)
	for _, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		wantAll(t, out,
			`class="rtext md" title="agent &#34;full&#34; text"`,
			`class="rtext md" title="legacy &#34;full&#34; &amp; text"`,
			`class="entry-recap" title="new &#34;panel&#34; &amp; text"`,
			`class="rtext md" title="new &#34;panel&#34; &amp; text"`)
		if layout == LayoutFull {
			// Historical versions retain their own full recap, not the latest one's.
			wantAll(t, out, `class="entry-recap" title="panel &#34;full&#34; &amp; text"`)
		}
		if strings.Contains(out, `title="agent "full"`) {
			t.Fatal("recap title was not attribute-escaped")
		}
	}
}
