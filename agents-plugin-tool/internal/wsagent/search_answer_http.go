package wsagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// SearchAnswerRoutePrefix is private to local session pages; no answer is
// part of the shared page-region cache or the generic /agents surface.
const SearchAnswerRoutePrefix = "/search-answers/"

// ServeHTTP serves POST /search-answers/<session> {question,context?}, GET
// /search-answers/<session>/<answer> (current state), GET .../events (SSE
// region {name,hash,html}), and POST .../cancel. A new POST supersedes the
// session's current question. History is the browser's, not this service's.
func (s *SearchAnswers) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	local, own := wsdashboard.TrustedWrite(req)
	if !local {
		http.NotFound(w, req)
		return
	}
	// Same-origin EventSource GETs may omit Origin; a supplied wrong Origin
	// or a cross-site fetch is still refused. Every write requires Origin.
	if (!own && (req.Method == http.MethodPost || req.Header.Get("Origin") != "")) || req.Header.Get("Sec-Fetch-Site") == "cross-site" || req.Header.Get("Sec-Fetch-Site") == "same-site" {
		http.Error(w, "the dashboard's own Origin is required", 403)
		return
	}
	path, ok := strings.CutPrefix(req.URL.Path, SearchAnswerRoutePrefix)
	parts := strings.Split(path, "/")
	if !ok || len(parts) < 1 || len(parts) > 3 || !wsdashboard.ValidReplyID(parts[0]) {
		http.NotFound(w, req)
		return
	}
	if len(parts) == 1 {
		if req.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var body struct {
			Question string `json:"question"`
			Context  string `json:"context"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 32<<10)).Decode(&body); err != nil {
			http.Error(w, "invalid question body", 400)
			return
		}
		a, err := s.start(parts[0], body.Question, body.Context)
		if err != nil {
			writeAnswer(w, 0, nil, err)
			return
		}
		writeAnswer(w, 201, a.snapshot(), nil)
		return
	}
	s.mu.Lock()
	a := s.bySession[parts[0]]
	s.mu.Unlock()
	if a == nil || a.snapshot().ID != parts[1] {
		http.NotFound(w, req)
		return
	}
	if len(parts) == 2 && req.Method == http.MethodGet {
		writeAnswer(w, 200, a.snapshot(), nil)
		return
	}
	if len(parts) == 3 && parts[2] == "cancel" && req.Method == http.MethodPost {
		a.cancel()
		<-a.done
		writeAnswer(w, 200, a.snapshot(), nil)
		return
	}
	if len(parts) == 3 && parts[2] == "events" && req.Method == http.MethodGet {
		s.stream(w, req, a)
		return
	}
	http.NotFound(w, req)
}

func answerRegion(v answerView) map[string]string {
	// Escaped text preserves citation markers for host-side validation in
	// the drawer; raw model HTML and links never become executable markup.
	markup := `<section data-answer-id="` + v.ID + `" data-answer-status="` + v.Status + `"><pre>` + html.EscapeString(v.Text) + `</pre>`
	if v.Error != "" {
		markup += `<p role="alert">` + html.EscapeString(v.Error) + `</p>`
	}
	markup += `</section>`
	region := map[string]string{"name": "answer", "html": markup}
	if v.Status == TurnDone && v.Error == "" && v.Context != "" {
		region["context"] = capSearchContext(v.Context)
	}
	hashInput := markup
	if region["context"] != "" {
		hashInput += "\x00" + region["context"]
	}
	h := sha256.Sum256([]byte(hashInput))
	region["hash"] = hex.EncodeToString(h[:16])
	return region
}

func (s *SearchAnswers) stream(w http.ResponseWriter, req *http.Request, a *searchAnswer) {
	flush, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	last := ""
	for {
		v := a.snapshot()
		region := answerRegion(v)
		if region["hash"] != last {
			raw, _ := json.Marshal(region)
			// Bound a stalled client's write, so daemon shutdown cannot be
			// held hostage by a reader that stopped consuming the stream.
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprintf(w, "event: region\ndata: %s\n\n", raw); err != nil {
				return
			}
			flush.Flush()
			last = region["hash"]
		}
		if v.Status != "running" {
			return
		}
		select {
		case <-req.Context().Done():
			return
		case <-tick.C:
		}
	}
}
