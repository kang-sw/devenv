package wswhiteboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		}{p.Lang, p.ReplyID, p.FullBoardLink, p.Stored, p.onPage, p.StoredPanels, p.panelOnPage})
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
