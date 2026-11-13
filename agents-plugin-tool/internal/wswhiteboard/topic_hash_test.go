package wswhiteboard

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"regexp"
	"strings"
	"testing"
)

var topicHash = regexp.MustCompile(` data-th="([0-9a-f]{16})"`)

// TestTopicHashCoversTheRenderedSection pins data-th (TopicHTML), the hash the
// board swap compares in place of a topic section's HTML (dashboard.js
// morphBoard): it is the hash of the section's rendered bytes without the
// attribute itself, so an unchanged topic keeps its hash, and a change to
// only a section class or only a frame's data-srcdoc changes it. The static
// page carries none.
func TestTopicHashCoversTheRenderedSection(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "one", "x")+topicXML("one", "first body"))
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	topic := view.Topics[0]
	hash := func(p *pageData, tv TopicView) string {
		t.Helper()
		h, err := p.TopicHTML(tv)
		if err != nil {
			t.Fatal(err)
		}
		s := string(h)
		m := topicHash.FindStringSubmatch(s)
		if m == nil || !strings.HasPrefix(s, "<section ") || strings.Index(s, m[0]) > strings.IndexByte(s, '>') {
			t.Fatalf("no data-th on the section's opening tag:\n%.300s", s)
		}
		sum := sha256.Sum256([]byte(strings.Replace(s, m[0], "", 1)))
		if want := hex.EncodeToString(sum[:8]); m[1] != want {
			t.Errorf("data-th = %s, want the hash of the bytes without it, %s", m[1], want)
		}
		return m[1]
	}
	base := hash(newPageData(view, a, true), topic)
	if again := hash(newPageData(view, a, true), topic); again != base {
		t.Errorf("an unchanged topic's data-th changed: %s, then %s", base, again)
	}
	live := topic
	live.Live = true
	if hash(newPageData(view, a, true), live) == base {
		t.Errorf("data-th unchanged when only the section's class (live) changed")
	}
	closed := topic
	closed.Closed = true
	if hash(newPageData(view, a, true), closed) == base {
		t.Errorf("data-th unchanged when the topic closed")
	}
	p := newPageData(view, a, true)
	p.docs[topic.Name] = template.HTML(strings.Replace(string(p.docs[topic.Name]), "first body", "first body!", 1))
	if !strings.Contains(string(p.docs[topic.Name]), "first body!") {
		t.Fatal("the frame's document did not change")
	}
	if hash(p, topic) == base {
		t.Errorf("data-th unchanged when only the frame's data-srcdoc changed")
	}

	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	if got := topicHash.FindAllString(string(regions.Board), -1); len(got) != len(view.Topics)+len(view.DeferredTopics) {
		t.Errorf("board carries %d data-th, want one per topic (%d)", len(got), len(view.Topics))
	}
	wantNone(t, render(t, view), "data-th=")
}
