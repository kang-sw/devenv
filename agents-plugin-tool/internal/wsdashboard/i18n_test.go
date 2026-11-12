package wsdashboard

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wslive"
)

// getLang fetches url as a browser with the given gear-menu cookie value
// (empty sends none) and Accept-Language header.
func getLang(t *testing.T, url, cookie, accept string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: wsi18n.CookieName, Value: cookie})
	}
	if accept != "" {
		req.Header.Set("Accept-Language", accept)
	}
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestSessionPageFollowsTheLanguageChoice(t *testing.T) {
	f := startPageFixture(t)
	url := f.base + "/s/livesession"
	cases := []struct {
		name, cookie, accept string
		lang, pref           string
		has, hasNot          string
	}{
		{"default is English", "", "", "en", "", `>Conversation<`, "대화"},
		{"browser language Korean", "", "ko-KR,ko;q=0.9", "ko", "", "대화", ">Conversation<"},
		{"unsupported browser language", "", "nl-NL", "en", "", `>Conversation<`, "대화"},
		{"cookie beats the browser", "ko", "en-US", "ko", "ko", "대화", ">Conversation<"},
		{"cookie English beats a Korean browser", "en", "ko-KR", "en", "en", `>Conversation<`, "대화"},
	}
	for _, c := range cases {
		code, body := getLang(t, url, c.cookie, c.accept)
		if code != http.StatusOK {
			t.Fatalf("%s: GET = %d", c.name, code)
		}
		for _, want := range []string{`data-lang="` + c.lang + `"`, `data-langpref="` + c.pref + `"`, c.has} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: page lacks %q", c.name, want)
			}
		}
		if strings.Contains(body, c.hasNot) {
			t.Errorf("%s: page holds %q", c.name, c.hasNot)
		}
		// 261030: the pages carry no lang attribute; the language travels in
		// data-lang only.
		if strings.Contains(body, "<html lang=") || strings.Contains(body, " lang=\"") {
			t.Errorf("%s: the page sets a lang attribute", c.name)
		}
	}
}

// TestEveryPageHasSettingsWithTheLanguageItem pins where the settings are:
// the index page's gear menu, and a session page's drawer card, the gear
// gone from its top bar.
func TestEveryPageHasSettingsWithTheLanguageItem(t *testing.T) {
	f := startPageFixture(t)
	for _, c := range []struct {
		name, path string
		layout     bool
	}{
		{"session list", "/", false},
		{"board-first session", "/s/boardsession", false},
		{"adapter session", "/s/livesession", true},
	} {
		_, body := getLang(t, f.base+c.path, "ko", "")
		if c.path == "/" {
			if !strings.Contains(body, `id="gear"`) || !strings.Contains(body, `id="gmenu"`) {
				t.Errorf("%s: no gear", c.name)
			}
		} else {
			if strings.Contains(body, `id="gear"`) || strings.Contains(body, `id="gmenu"`) {
				t.Errorf("%s: the gear is still in the top bar", c.name)
			}
			if !strings.Contains(body, `<details class="panel drcard setcard" id="p-settings" open>`) || !strings.Contains(body, `<div class="gmenu gcard">`) {
				t.Errorf("%s: no settings card in the drawer", c.name)
			}
		}
		// The scripts call wsT on load; a page without the table breaks them.
		if !strings.Contains(body, "window.wsT") {
			t.Errorf("%s: no client string table", c.name)
		}
		// The follow-the-browser option and one option per embedded language.
		if !strings.Contains(body, `<option value="">`) {
			t.Errorf("%s: no follow-the-browser item", c.name)
		}
		for _, l := range wsi18n.Languages() {
			if !strings.Contains(body, `<option value="`+l.Code+`">`+l.Name+`</option>`) {
				t.Errorf("%s: no item for %s", c.name, l.Code)
			}
		}
		if has := strings.Contains(body, `<button data-layout="split"`) && strings.Contains(body, `<button data-input="mod"`); has != c.layout {
			t.Errorf("%s: layout and input items present = %v, want %v", c.name, has, c.layout)
		}
	}
}

// Viewers in two languages share one entry cache: a cached entry from one
// must never reach the other.
func TestEntryCacheKeepsLanguagesApart(t *testing.T) {
	f := startPageFixture(t)
	url := f.base + "/s/livesession"
	for i, lang := range []string{"ko", "en", "ko", "en"} {
		_, body := getLang(t, url, lang, "")
		en, ko := `<span class="src wb">whiteboard</span>`, `<span class="src wb">화이트보드</span>`
		want, not := en, ko
		if lang == "ko" {
			want, not = ko, en
		}
		// The board-origin user entry is in the entries region, rendered
		// through the cache.
		entries := body[strings.Index(body, `data-region="entries"`):]
		entries = entries[:strings.Index(entries, `data-region="convlive"`)]
		if !strings.Contains(entries, want) || strings.Contains(entries, not) {
			t.Fatalf("request %d (%s): entries region has the wrong language:\n%s", i, lang, entries)
		}
	}
}

func TestStreamAndHistoryFollowTheCookie(t *testing.T) {
	f := startPageFixture(t)
	req, _ := http.NewRequest(http.MethodGet, f.base+"/s/livesession/events", nil)
	req.AddCookie(&http.Cookie{Name: wsi18n.CookieName, Value: "ko"})
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	regions := map[string]string{}
	for len(regions) < 8 {
		ev, ok := nextRegion(t, events, time.Second)
		if !ok {
			break
		}
		regions[ev.Name] = ev.HTML
	}
	if !strings.Contains(regions["entries"], "대화") || !strings.Contains(regions["tree"], "에이전트") {
		t.Fatalf("the stream's regions are not Korean: entries=%.200q tree=%.200q", regions["entries"], regions["tree"])
	}

	// The history route renders its entries through the same cache.
	_, hist := getLang(t, f.base+"/s/livesession/history?n=10", "ko", "")
	var answer historyAnswer
	if err := json.Unmarshal([]byte(hist), &answer); err != nil {
		t.Fatalf("history: %v: %s", err, hist)
	}
	if !strings.Contains(answer.HTML, "화이트보드") || strings.Contains(answer.HTML, ">whiteboard<") {
		t.Fatalf("history entries are not Korean: %.300s", answer.HTML)
	}
}

func TestGoBuiltStringsFollowTheLanguage(t *testing.T) {
	usage := &wslive.TokenUsage{Input: 1200, Output: 300, CacheRead: 5000}
	if got := tokenText("en", 1, usage); got != "prompt 6.2k · output 300" {
		t.Fatalf("tokenText en = %q", got)
	}
	if got := tokenText("ko", 1, usage); got != "프롬프트 6.2k · 출력 300" {
		t.Fatalf("tokenText ko = %q", got)
	}
	if got := tokenText("xx", 1, usage); got != tokenText("en", 1, usage) {
		t.Fatalf("an unsupported language renders English, got %q", got)
	}
	if got := tokenText("en", 1, nil); got != "" {
		t.Fatalf("tokenText nil = %q", got)
	}
	if got := tokenTitle("en", 6, nil); got != "" {
		t.Fatalf("tokenTitle nil = %q", got)
	}
	// Several requests lead with their count; a cache write trails the
	// prompt as +N, and the title names every part with exact counts.
	written := &wslive.TokenUsage{Input: 14, Output: 1_402, CacheRead: 708_800, CacheWrite: 11_698}
	if got := tokenText("ko", 6, written); got != "호출 6 · 프롬프트 721k +11.7k · 출력 1.4k" {
		t.Fatalf("tokenText ko with calls = %q", got)
	}
	if got := tokenText("en", 6, written); got != "6 calls · prompt 721k +11.7k · output 1.4k" {
		t.Fatalf("tokenText en with calls = %q", got)
	}
	if got := tokenTitle("ko", 6, written); got != "모델 호출 6회 · 프롬프트 720,512 (입력 14 · 캐시 읽기 708,800 · 캐시 쓰기 11,698) · 출력 1,402" {
		t.Fatalf("tokenTitle ko = %q", got)
	}
	if got := tokenTitle("en", 1, written); got != "prompt 720,512 (14 input · 708,800 cache read · 11,698 cache write) · output 1,402" {
		t.Fatalf("tokenTitle en single = %q", got)
	}
	if got := summaryRequests(&wslive.ToolSummary{Batches: make([]wslive.ToolBatch, 2), BatchesOmitted: 3}); got != 5 {
		t.Fatalf("summaryRequests = %d, want the listed and omitted batches", got)
	}
	// A batch line's numbers carry no labels; each one's title names it.
	nums := func(lang string, b wslive.ToolBatch) (text, titles string) {
		var ts []string
		for _, n := range batchNums(lang, b) {
			text += n.Sep + n.Text
			ts = append(ts, n.Title)
		}
		return text, strings.Join(ts, " | ")
	}
	text, titles := nums("ko", wslive.ToolBatch{Calls: 1, Tools: []wslive.ToolTally{{Name: "Read", Calls: 1}}, DurationMs: 900, Usage: written})
	if text != "0.9s · 721k +11.7k · 1.4k" || titles != "소요 시간 900 ms | 프롬프트 720,512 (입력 14 · 캐시 읽기 708,800 · 캐시 쓰기 11,698) | 캐시 쓰기 11,698 | 출력 1,402" {
		t.Fatalf("batchNums ko with usage = %q / %q", text, titles)
	}
	b := wslive.ToolBatch{Calls: 3, Failed: 1, DurationMs: 1500}
	if text, titles := nums("en", b); text != "×3 · 1.5s · ✗1" || titles != "3 calls | duration 1,500 ms | 1 failed" {
		t.Fatalf("batchNums en = %q / %q", text, titles)
	}
	if text, titles := nums("ko", b); text != "×3 · 1.5s · ✗1" || titles != "호출 3회 | 소요 시간 1,500 ms | 실패 1" {
		t.Fatalf("batchNums ko = %q / %q", text, titles)
	}
	if got := phaseWord("ko", "thinking"); got != "생각 중" {
		t.Fatalf("phaseWord = %q", got)
	}
}

func TestFileViewPageFollowsTheLanguage(t *testing.T) {
	f := startPageFixture(t)
	if got := wsi18n.T("ko", "file.reason.large", 8); got != "8 MiB보다 큽니다" {
		t.Fatalf("file.reason.large ko = %q", got)
	}
	// The not-viewable page: a binary file in the project root.
	writeFile(t, f.project+"/blob.bin", "\x00\x01\x02")
	_, en := getLang(t, f.base+"/s/livesession/file?path=blob.bin", "", "")
	_, ko := getLang(t, f.base+"/s/livesession/file?path=blob.bin", "ko", "")
	if !strings.Contains(en, "it is not text") || !strings.Contains(en, "Copy relative path") {
		t.Fatalf("English file page: %s", en)
	}
	if !strings.Contains(ko, "텍스트가 아닙니다") || !strings.Contains(ko, "상대 경로 복사") || strings.Contains(ko, "Copy relative path") {
		t.Fatalf("Korean file page: %s", ko)
	}
}

// The page scripts get the client table in the page's language, English for
// a key the language lacks, and no prefix-less (server-only) key.
func TestPageCarriesTheClientTable(t *testing.T) {
	f := startPageFixture(t)
	_, body := getLang(t, f.base+"/s/livesession", "ko", "")
	if !strings.Contains(body, `lang: "ko"`) || !strings.Contains(body, `"js.ago":"{0} 전"`) {
		t.Fatal("the Korean page does not carry the Korean client table")
	}
	if strings.Contains(body, `"tab.conversation"`) {
		t.Fatal("a server-only key reached the client table")
	}
}
