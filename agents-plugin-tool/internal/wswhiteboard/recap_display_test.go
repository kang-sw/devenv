package wswhiteboard

import (
	"regexp"
	"strings"
	"testing"
)

func TestRecapDisplayClampAndContentSizedHeader(t *testing.T) {
	data, err := embeddedAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	wantAll(t, css, `.trecap, .entry-recap { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 4; line-clamp: 4; overflow: hidden; }`)
	slot := regexp.MustCompile(`(?m)^\.tsub > \.trecap \{[^}]+\}`).FindString(css)
	wantAll(t, slot, `max-height: 5.8em;`, `font-size: 12.5px;`, `line-height: 1.45;`, `min-width: 0;`, `background: none;`)
	if regexp.MustCompile(`(?:\{|;)\s*(?:height|min-height)\s*:`).MatchString(slot) {
		t.Fatal("header recap reserves blank lines instead of shrinking to content")
	}
	// The author badge must not enlarge the first line beyond the line grid.
	wantAll(t, css, `.tsub > .trecap .rat .evi { width: 14px; height: 14px; vertical-align: text-bottom; }`)
	// Sticking must not change the shared maximum or content-driven height.
	if regexp.MustCompile(`(?m)^[^\n{]*\.wb-stuck[^\n{]*\.trecap\s*\{`).MatchString(css) {
		t.Fatal("stuck recap overrides the shared clamp/height")
	}
}

func TestRecapDisplayFullTextTitles(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="current"><wb:log>agent "full" text</wb:log><wb:panel recap="panel &quot;full&quot; &amp; text">body</wb:panel><wb:panel id="p1" recap="new panel">new body</wb:panel></wb:topic>`)
	legacyRecap(st, "legacy", `legacy "full" & text`)
	for _, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		wantAll(t, out,
			`class="rtext md" title="agent &#34;full&#34; text"`,
			`class="rtext md" title="legacy &#34;full&#34; &amp; text"`,
			`class="entry-recap" title="new panel"`)
		if layout == LayoutFull {
			// Historical versions retain their own full recap, not the latest one's.
			wantAll(t, out, `class="entry-recap" title="panel &#34;full&#34; &amp; text"`)
		}
		if strings.Contains(out, `title="agent "full"`) {
			t.Fatal("recap title was not attribute-escaped")
		}
	}
}
