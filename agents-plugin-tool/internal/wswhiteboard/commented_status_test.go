package wswhiteboard

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCommentedStatusRenderingAndReask(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "Choose", "One", "Two"))
	mustApply(t, st, `<wb:answer id="1" value="">original &amp; note</wb:answer>`)
	for _, layout := range []Layout{LayoutFull} {
		page := markup(render(t, BuildView(st, ViewInput{Layout: layout, Location: time.UTC})))
		wantAll(t, page, `class="cmchip"`, `Lead&#39;s move`, `original &amp; note`, `class="cmnopick"`, `cm-none`)
		if strings.Contains(page, `user commented, lead`) {
			t.Fatal("raw event leaked into display")
		}
	}
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	view := BuildView(st, ViewInput{Layout: LayoutFull})
	data := pageDataBase(view, a)
	var settled bytes.Buffer
	if err := a.templateFor(view.Lang).ExecuteTemplate(&settled, "settled", data.TopicOf(view.Topics[0])); err != nil {
		t.Fatal(err)
	}
	wantAll(t, settled.String(), `class="srow commented"`, `class="cmchip"`, `class="quote cmq"`, `class="cmnopick"`)
	koView := BuildView(st, ViewInput{Layout: LayoutFull})
	koView.Lang = "ko"
	ko := markup(render(t, koView))
	wantAll(t, ko, `의견 남김`, `리드 차례`, `고른 선택지 없음`)
	mustApply(t, st, `<wb:patch id="1" status="open"/>`)
	page := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC})))
	wantAll(t, page, `class="cmchip"`, `class="cmquote"`, `class="cmprev"`, `Previous comment`, `Asked again`, `original &amp; note`)
	if strings.Contains(page, `user commented [`) {
		t.Fatal("raw comment event leaked after re-ask")
	}
	mustApply(t, st, `<wb:answer id="1" value="b">later choice</wb:answer>`)
	page = markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC})))
	wantAll(t, page, `class="cmquote"`, `original &amp; note`, `later choice`)

}
