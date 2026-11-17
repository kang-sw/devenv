package wsdashboard

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// topicHistoryMax bounds the topics one history fetch may name.
const topicHistoryMax = 512

// topicTarget is a page target the history fetch locates: an entry, a
// panel, or an item.
var topicTarget = regexp.MustCompile(`^(?:[pe][1-9][0-9]{0,9}|item-[1-9][0-9]{0,9})$`)

// handleTopicHistory serves /s/<reply-id>/topic?name=<topic>[&name=...] or
// ?target=<eN|pN|item-N>: the named topics, or the one topic holding the
// target, rendered with the complete history the served board leaves out
// (TopicHistorySource), as {"topics":[{"name","html"}]} with each body
// frame's theme pinned like the board's. The page fetches it when the
// viewer expands lazy history. It is read-only and serves what the session
// page shows, so it takes the page's own access rule.
func (d *Daemon) handleTopicHistory(w http.ResponseWriter, r *http.Request, replyID string) {
	query := r.URL.Query()
	names, target := query["name"], query.Get("target")
	if (len(names) == 0) == (target == "") || len(names) > topicHistoryMax || target != "" && !topicTarget.MatchString(target) {
		http.Error(w, "name or target required", http.StatusBadRequest)
		return
	}
	src, has := d.source.(TopicHistorySource)
	var s Session
	ok := has && d.index != nil && ValidReplyID(replyID)
	if ok {
		s, ok = d.index.Session(replyID)
	}
	type fragment struct {
		Name string `json:"name"`
		HTML string `json:"html"`
	}
	out := struct {
		Topics []fragment `json:"topics"`
	}{Topics: []fragment{}}
	if ok {
		lang, _ := requestLang(r)
		topics, read := src.ReadTopicHistory(s.Registration, lang, names, target)
		ok = read
		theme := requestTheme(r)
		for _, t := range topics {
			out.Topics = append(out.Topics, fragment{Name: t.Name, HTML: string(wswhiteboard.ThemeFrames(t.HTML, theme))})
		}
	}
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	raw, _ := json.Marshal(out)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}
