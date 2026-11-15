package wsdashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func rendererSummary() *wslive.ReasoningSummary {
	text := "Existing conversation and activity stay separate.\n**plain text** <script>alert(1)</script>"
	return &wslive.ReasoningSummary{V: 1, Producer: "12345678-1234-4234-8234-123456789abc", Turn: 1, Revision: 1, ActiveRequest: &wslive.SummaryPair{Request: 1, Attempt: 1},
		Requests: []wslive.SummaryAttempt{{Request: 1, Attempt: 1, Provider: "openai", API: "openai-responses", Model: "gpt-example", Status: wslive.SummaryStreaming,
			Parts: []wslive.SummaryPart{{Text: text, ObservedBytes: int64(len(text)), RetainedBytes: int64(len(text)), Status: wslive.SummaryStreaming}}}}}
}

func renderSummaryRegions(t *testing.T, s *wslive.ReasoningSummary, ring []wslive.RingRecord, cache *entryCache) regionSet {
	t.Helper()
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}, ReasoningSummary: s, Reply: &wslive.Reply{Text: "Ordinary answer"}}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, lead), Ring: ring, Now: testNow})
	set, err := renderSessionRegions(v, cache)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestReasoningSummaryLivePlacementPlainTextAndStableKey(t *testing.T) {
	s := rendererSummary()
	s.Requests[0].Model = `<img src=x onerror=alert(1)>`
	a := renderSummaryRegions(t, s, nil, newEntryCache())
	live := string(a[regionConvLive].HTML)
	for _, want := range []string{`class="d-summary live"`, `data-key="summary-live-` + s.Producer + `-1" open`, `</div></div><div class="activity">`, `</div><details class="d-summary live"`, `**plain text** &lt;script&gt;alert(1)&lt;/script&gt;`, `&lt;img src=x onerror=alert(1)&gt;`, "Streaming · observed prefix"} {
		if !strings.Contains(live, want) {
			t.Errorf("missing %q: %s", want, live)
		}
	}
	if strings.Contains(live, "<script>") || strings.Contains(live, "<strong>plain text") || strings.Contains(string(a[regionEntries].HTML), "d-summary") {
		t.Fatal("summary interpreted as markup or leaked into entries")
	}
	s.ActiveRequest = nil
	paused := string(renderSummaryRegions(t, s, nil, nil)[regionConvLive].HTML)
	if strings.Contains(paused, `class="d-updating"`) {
		t.Fatal("a cleared active attempt still claimed to be updating")
	}
	s.ActiveRequest = &wslive.SummaryPair{Request: 1, Attempt: 1}
	s.Revision++
	s.Requests[0].Parts[0].Text += " more"
	s.Requests[0].Parts[0].ObservedBytes += 5
	s.Requests[0].Parts[0].RetainedBytes += 5
	b := renderSummaryRegions(t, s, nil, newEntryCache())
	if a[regionEntries].Hash != b[regionEntries].Hash || a[regionConvLive].Hash == b[regionConvLive].Hash || !strings.Contains(string(b[regionConvLive].HTML), `data-key="summary-live-`+s.Producer+`-1"`) {
		t.Fatal("live update changed entry or details identity")
	}
}

func TestReasoningSummaryFallbackExactLegacyAndInvalidHistory(t *testing.T) {
	baseline := string(renderSummaryRegions(t, nil, nil, nil)[regionConvLive].HTML)
	cases := map[string]func(*wslive.ReasoningSummary){
		"version": func(s *wslive.ReasoningSummary) { s.V = 2 },
		"source":  func(s *wslive.ReasoningSummary) { s.Requests[0].API = "private-thinking" },
		"no-text": func(s *wslive.ReasoningSummary) { s.Requests[0].Parts = nil },
		"bytes":   func(s *wslive.ReasoningSummary) { s.Requests[0].Parts[0].RetainedBytes++ },
		"order":   func(s *wslive.ReasoningSummary) { s.Requests = append(s.Requests, s.Requests[0]) },
		"status":  func(s *wslive.ReasoningSummary) { s.Requests[0].Status = "invented" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := rendererSummary()
			mutate(s)
			if got := string(renderSummaryRegions(t, s, nil, nil)[regionConvLive].HTML); got != baseline {
				t.Fatalf("fallback differs: %s", got)
			}
			h := &wslive.SummaryHistory{V: s.V, Commit: strings.Repeat("a", 64), Requests: s.Requests}
			ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingTurn, SummaryHistory: h})}
			got := string(renderSummaryRegions(t, nil, ring, nil)[regionEntries].HTML)
			ring[0].SummaryHistory = nil
			want := string(renderSummaryRegions(t, nil, ring, nil)[regionEntries].HTML)
			if got != want {
				t.Fatalf("invalid/empty history changed legacy turn: %s", got)
			}
		})
	}
	badIdentity := rendererSummary()
	badIdentity.Producer = ""
	if got := string(renderSummaryRegions(t, badIdentity, nil, nil)[regionConvLive].HTML); got != baseline {
		t.Fatal("invalid live identity changed fallback")
	}
	for _, commit := range []string{"", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		e := ConvEntry{RingEntry: wslive.RingEntry{Kind: wslive.RingTurn, SummaryHistory: &wslive.SummaryHistory{V: 1, Commit: commit, Requests: rendererSummary().Requests}}}
		if e.Summary() != nil {
			t.Fatal("invalid commit displayed")
		}
	}
	// A payload on any other history kind cannot create a summary surface.
	e := ConvEntry{RingEntry: wslive.RingEntry{Kind: wslive.RingReply, SummaryHistory: &wslive.SummaryHistory{V: 1, Commit: strings.Repeat("a", 64), Requests: rendererSummary().Requests}}}
	if e.Summary() != nil {
		t.Fatal("non-turn summary displayed")
	}
}

func TestReasoningSummaryHistoryImmutableTurnAssociationAndAttempts(t *testing.T) {
	cache := newEntryCache()
	ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingReply, Text: "Previous answer"}), ringRecord(2, wslive.RingEntry{Kind: wslive.RingTurn}), ringRecord(3, wslive.RingEntry{Kind: wslive.RingReply, Text: "Own answer"})}
	before := renderSummaryRegions(t, nil, ring, cache)
	s := rendererSummary()
	first := &s.Requests[0]
	first.Status = wslive.SummaryFailed
	first.Truncated = true
	first.Parts[0].Status = wslive.SummaryFailed
	first.Parts[0].Truncated = true
	first.Parts[0].ObservedBytes += 10
	second := s.Requests[0]
	second.Attempt = 2
	second.Status = wslive.SummaryComplete
	second.Truncated = false
	second.Parts = []wslive.SummaryPart{{Text: "retry exact", ObservedBytes: 11, RetainedBytes: 11, Status: wslive.SummaryComplete}, {Output: 1, Part: 2, Text: "next", ObservedBytes: 4, RetainedBytes: 4, Status: wslive.SummaryIncomplete}}
	third := second
	third.Request = 2
	third.Attempt = 1
	third.Status = wslive.SummaryStreaming
	fourth := second
	fourth.Request = 3
	fourth.Attempt = 1
	fourth.Status = wslive.SummaryAborted
	s.Requests = append(s.Requests, second, third, fourth)
	h := &wslive.SummaryHistory{V: 1, Commit: strings.Repeat("a", 64), Requests: s.Requests}
	ring = append(ring, ringRecord(4, wslive.RingEntry{Kind: wslive.RingTurn, Done: 2, DurationMs: 42000, SummaryHistory: h}))
	after := renderSummaryRegions(t, nil, ring, cache)
	html := string(after[regionEntries].HTML)
	for _, want := range []string{`<div class="turn-with-summary" data-entry="` + ring[3].Name + `"><details class="d-summary saved" data-key="summary-turn-` + ring[3].Name + `">`, "Request 1 · attempt 1", "Request 1 · attempt 2", "Request 2 · attempt 1", "Output 1 · part 2", "Failed · observed prefix", "Aborted · observed prefix", "Locally clipped · retained prefix", "Incomplete · observed prefix", "Streaming · observed prefix", "full text not guaranteed", `</details><div class="turnline">`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q: %s", want, html)
		}
	}
	if strings.Count(html, `data-entry="`+ring[3].Name+`"`) != 1 || strings.Contains(html, `data-key="summary-turn-`+ring[3].Name+`" open`) {
		t.Fatal("duplicate turn key or expanded retained default")
	}
	// Earlier reply/turn markup is byte-identical after the late natural commit.
	a := conversation(wslive.Lead{}, ring[:3], nil, nil, sessionLookup{}, replyMarkdown("/proj", "self"))
	a.Lang = "en"
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	if err := renderEntries(assets, cache, "self", &a); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before[regionEntries].HTML), renderedEntries(a.Entries)) || !strings.Contains(html, renderedEntries(a.Entries)) {
		t.Fatal("late history rewrote earlier cached entries")
	}
	paged := renderSummaryRegions(t, nil, ring[3:], newEntryCache())
	if !strings.Contains(string(paged[regionEntries].HTML), `data-key="summary-turn-`+ring[3].Name+`"`) {
		t.Fatal("paging changed details key")
	}
}

// Optional offline visual fixture: actual renderer regions, sprite and styles.
// No listener, runtime session, provider, or copied reference component markup.
func TestReasoningSummaryVisualFixture(t *testing.T) {
	dir := os.Getenv("WS_SUMMARY_RENDER_OUTPUT")
	if dir == "" {
		t.Skip("offline capture output not requested")
	}
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	renderVisual := func(s *wslive.ReasoningSummary, ring []wslive.RingRecord) regionSet {
		lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}, ReasoningSummary: s}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, lead), Ring: ring, Now: testNow})
		v.Lang = "ko"
		set, err := renderSessionRegions(v, nil)
		if err != nil {
			t.Fatal(err)
		}
		return set
	}
	s := rendererSummary()
	p := &s.Requests[0].Parts[0]
	p.Text = "기존 대화 화면과 활동 표시를 확인했습니다. 지원 모드의 요약 표시와 미지원 모드의 기본 활동 표시를 구분하는 구성을 검토하고 있습니다."
	p.RetainedBytes = int64(len(p.Text))
	p.ObservedBytes = p.RetainedBytes
	user := ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "기존 대화 화면에 진행 요약을 어떻게 보여줄지 정리해 주세요."})
	live := renderVisual(s, []wslive.RingRecord{user})
	fallback := renderVisual(nil, []wslive.RingRecord{user})
	s.Requests[0].Status = wslive.SummaryComplete
	p.Status = wslive.SummaryComplete
	history := renderVisual(nil, []wslive.RingRecord{user, ringRecord(2, wslive.RingEntry{Kind: wslive.RingReply, Text: "요약은 기존 활동 표시 아래에 배치하고, 완료 후에는 이 답변 아래에 접어서 보관하는 구성을 제안합니다.\n\n미지원 모드에서는 기존 활동 표시를 그대로 유지합니다."}), ringRecord(3, wslive.RingEntry{Kind: wslive.RingTurn, Done: 2, DurationMs: 42000, SummaryHistory: &wslive.SummaryHistory{V: 1, Commit: strings.Repeat("a", 64), Requests: s.Requests}})})
	sets := []regionSet{live, fallback, history, history}
	names := []string{"live", "fallback", "history-collapsed", "history-expanded"}
	var body strings.Builder
	fragments := map[string]string{}
	for i, set := range sets {
		entries := string(set[regionEntries].HTML)
		trailing := ""
		if i < 2 {
			trailing = string(set[regionConvLive].HTML)
		}
		body.WriteString(`<section><h2>` + names[i] + `</h2><div class="convo" id="` + names[i] + `">` + entries + trailing + `</div></section>`)
		fragments[names[i]] = entries + trailing
	}
	html := `<!doctype html><html lang="en" data-theme="light"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>` + string(assets.data.WhiteboardCSS) + string(assets.data.CSS) + `</style><style>body{padding:24px 16px;display:block}main{max-width:1216px;margin:auto;display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:24px 20px}section{min-width:0}h2{font-size:14px}.convo{position:static;display:block}.centries{max-height:none} @media(max-width:760px){body{padding:16px 10px}main{grid-template-columns:minmax(0,1fr)}}</style><body>` + string(assets.data.WhiteboardSprite) + string(assets.data.Sprite) + `<main>` + body.String() + `</main></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "actual-renderer.html"), []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(fragments)
	if err := os.WriteFile(filepath.Join(dir, "fragments.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
