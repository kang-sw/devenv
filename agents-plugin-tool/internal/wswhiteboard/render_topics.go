package wswhiteboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
)

// topicRevision hashes the complete projected topic, including derived
// reverse waiters and the private full-history selector. Page-wide reference
// membership changes affect markdown links even when a topic itself did not
// change. Function-backed external inputs are covered by the source's key.
func (p *pageData) topicRevision(t TopicView) string {
	if p.topicContext == "" {
		raw, _ := json.Marshal(struct {
			Lang, ReplyID, FullBoardLink string
			Stored, OnPage               map[int]bool
			StoredPanels, PanelOnPage    map[string]bool
			LazyHistory                  bool
		}{p.Lang, p.ReplyID, p.FullBoardLink, p.Stored, p.onPage, p.StoredPanels, p.panelOnPage, p.LazyHistory})
		sum := sha256.Sum256(raw)
		p.topicContext = hex.EncodeToString(sum[:])
	}
	raw, _ := json.Marshal(struct {
		Topic       TopicView
		FullHistory bool
	}{t, t.fullHistory})
	h := sha256.New()
	h.Write([]byte(p.topicContext))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

// collectTopicRegions verifies the embedded template's contiguous topic run
// without tokenizing HTML. A dev template with an unfamiliar layout offers no
// units; the dashboard then keeps its existing parser/full-region fallback.
func collectTopicRegions(board template.HTML, topics []TopicView, rendered map[string]TopicUnit) *TopicRegions {
	units := make([]TopicUnit, 0, len(topics))
	var shell strings.Builder
	rest := board
	for _, t := range topics {
		u, ok := rendered[t.Name]
		if !ok || u.HTML == "" || !strings.HasPrefix(string(rest), string(u.HTML)) {
			return nil
		}
		rest = rest[len(u.HTML):]
		if !strings.HasPrefix(string(rest), string(u.after)) {
			return nil
		}
		rest = rest[len(u.after):]
		units = append(units, u)
		shell.WriteString(string(u.after))
	}
	shell.WriteString(string(rest))
	return &TopicRegions{Board: board, Topics: units, Shell: template.HTML(shell.String())}
}

func topicHTMLHash(html template.HTML) string {
	_, rest, _ := strings.Cut(string(html), ` data-th="`)
	hash, _, _ := strings.Cut(rest, `"`)
	return hash
}

// lazyLiveEntries is the set of entry IDs a lazy render shows in its live
// timeline rows, the only rows whose panel bodies it renders; nil when the
// render is not lazy, which renders every body.
func lazyLiveEntries(t TopicView, lazy bool) map[string]bool {
	if !lazy {
		return nil
	}
	live := map[string]bool{}
	for _, part := range t.Timeline {
		if part.Gap {
			continue
		}
		for _, row := range part.Rows {
			for _, e := range timelineEntries(row.Page) {
				live[e.ID] = true
			}
		}
	}
	return live
}

// TopicFragment is one topic section rendered in full for the served page's
// lazy history (RenderTopicFragments).
type TopicFragment struct {
	Name string
	HTML template.HTML
}

// RenderTopicFragments renders the named topics of view (shown or capped
// closed, in the order given; unknown names are skipped) as framed sections
// with their complete history: the inert gap payloads, earlier panel
// versions and capped closed topics a LazyHistory render leaves out.
// Only the named topics' bodies and panels render.
func RenderTopicFragments(view View, names []string) ([]TopicFragment, error) {
	a, err := loadAssets()
	if err != nil {
		return nil, fmt.Errorf("whiteboard page assets: %w", err)
	}
	view.Layout = LayoutFull
	view.StampFile = ""
	view.LazyHistory = false
	p := pageDataBase(view, a, true)
	byName := map[string]TopicView{}
	for _, t := range append(append([]TopicView(nil), view.Topics...), view.DeferredTopics...) {
		byName[t.Name] = t
	}
	var out []TopicFragment
	seen := map[string]bool{}
	for _, name := range names {
		t, ok := byName[name]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		p.prepareTopic(t, a)
		html, err := p.renderTopicHTML(t)
		if err != nil {
			return nil, fmt.Errorf("whiteboard topic %s: %w", name, err)
		}
		out = append(out, TopicFragment{Name: name, HTML: template.HTML(strings.TrimRight(string(html), " \t\r\n\f"))})
	}
	return out, nil
}
