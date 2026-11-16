package wswhiteboard

import (
	"encoding/json"
	"html"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPanelDiffBlocks(t *testing.T) {
	for _, tc := range []struct {
		name, old, cur string
		counts         [3]int
	}{
		{"unchanged", "one\n\ntwo", "one\n\ntwo", [3]int{}},
		{"whitespace", "one two", "one  two", [3]int{}},
		{"added", "one", "one\n\ntwo", [3]int{1, 0, 0}},
		{"modified", "one two", "one three", [3]int{0, 1, 0}},
		{"removed", "one\n\ntwo\n\nthree", "one", [3]int{0, 0, 2}},
		{"empty", "one", "", [3]int{0, 0, 1}},
		{"from empty", "", "one", [3]int{1, 0, 0}},
		{"all kinds", "intro\n\n- alpha beta\n- fixed\n- removed\n- end", "intro\n\n- alpha gamma\n- fixed\n- end\n\nadded", [3]int{1, 1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := diffPanel(tc.old, tc.cur)
			var counts [3]int
			if d != nil {
				counts = [3]int{d.Added, d.Modified, d.Removed}
				var got []BlockMark
				for _, e := range d.edits {
					got = append(got, e.BlockMark)
				}
				if want := diffBodies(tc.old, tc.cur); !reflect.DeepEqual(got, want) {
					t.Fatalf("panel pairing %v differs from topic pairing %v", got, want)
				}
			}
			if counts != tc.counts {
				t.Fatalf("counts %v, want %v", counts, tc.counts)
			}
		})
	}
}

func TestPanelDiffWords(t *testing.T) {
	for _, tc := range []struct {
		name, old, cur string
		want           []wordEdit
	}{
		{"empty", "", "", nil},
		{"unchanged", "same words", "same words", []wordEdit{{"equal", "same words"}}},
		{"added", "", "new", []wordEdit{{MarkNew, "new"}}},
		{"removed", "old", "", []wordEdit{{MarkDel, "old"}}},
		{"replacement", "after 1 hour", "after 30 minutes", []wordEdit{{"equal", "after "}, {MarkDel, "1"}, {MarkNew, "30"}, {"equal", " "}, {MarkDel, "hour"}, {MarkNew, "minutes"}}},
		{"unicode and punctuation", "로그 시각, 1시간", "로그·패널 시각, 30분", []wordEdit{{"equal", "로그"}, {MarkNew, "·패널"}, {"equal", " 시각, "}, {MarkDel, "1시간"}, {MarkNew, "30분"}}},
		{"spaces", "a  b", "a b", []wordEdit{{"equal", "a"}, {MarkDel, "  "}, {MarkNew, " "}, {"equal", "b"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := diffWords(tc.old, tc.cur)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("diffWords = %#v, want %#v", got, tc.want)
			}
		})
	}
	old, cur := strings.Repeat("old ", 600), strings.Repeat("new ", 600)
	if got := diffWords(old, cur); !reflect.DeepEqual(got, []wordEdit{{MarkDel, old}, {MarkNew, cur}}) {
		t.Fatal("large word diff did not degrade to a bounded replacement")
	}
}

const panelDiffV2 = "### 정렬 기준 설계안\n\n**결론** — 기본 정렬은 **최근 활동순**, 고정한 세션은 항상 맨 위에 둡니다.\n\n- **최근 활동**: 마지막 로그 시각 기준, 1시간 넘게 조용하면 “유휴”로 내림\n- **고정 세션**: 정렬과 무관하게 상단 고정, 고정끼리는 고정한 순서 유지\n- **이름순**: 보조 정렬 옵션으로 드롭다운에 제공\n- **닫힌 세션**: 목록 하단 “닫힘” 접힘 영역으로 이동\n\n#### 남은 질문\n\n고정 세션 수에 상한을 둘지 — 많아지면 최근 활동 영역이 화면 밖으로 밀립니다."

func panelDiffFixture(t *testing.T, live bool) View {
	t.Helper()
	v1 := strings.ReplaceAll(panelDiffV2, "1시간", "2시간")
	v3 := strings.ReplaceAll(panelDiffV2, "로그 시각 기준, 1시간", "로그·패널 갱신 시각 기준, 30분")
	v3 = strings.ReplaceAll(v3, "- **이름순**: 보조 정렬 옵션으로 드롭다운에 제공\n", "")
	v3 = strings.ReplaceAll(v3, "#### 남은 질문", "**재정렬 지연** — 사용자가 스크롤 중이거나 포인터가 목록 위에 있으면 재정렬을 3초 미룹니다. 손 아래에서 목록이 튀지 않게 하기 위함입니다.\n\n#### 남은 질문")
	at := func(h, m int) string { return time.Date(2026, 11, 16, h, m, 0, 0, time.UTC).Format(time.RFC3339) }
	entries := []Entry{
		{ID: "e1", Kind: EntryAgent, At: at(9, 40), Text: "정렬 기준 초안 작성 시작"},
		{ID: "e2", Kind: EntryPanel, PanelID: "p1", PanelName: "design", At: at(9, 48), Recap: "정렬 기준 초안", Text: v1},
		{ID: "e3", Kind: EntryAgent, At: at(9, 52), Text: "보조 정렬(이름순) 추가 검토"},
		{ID: "e4", Kind: EntryPanel, PanelID: "p1", At: at(9, 55), Recap: "보조 정렬 추가", Text: panelDiffV2},
		{ID: "e5", Kind: EntryAgent, At: at(10, 12), Text: "이름순은 사용 빈도가 낮아 빼기로 함, 유휴 기준 재조정"},
		{ID: "e6", Kind: EntryPanel, PanelID: "p1", At: at(10, 20), Recap: "유휴 기준 30분, 재정렬 지연 추가", Text: v3},
		{ID: "e7", Kind: EntryAgent, At: at(10, 24), Text: "유휴 기준 30분 확인 대기"},
	}
	if live {
		entries = append(entries, Entry{ID: "e8", Kind: EntryPanel, PanelID: "p1", At: at(10, 30), Recap: "유휴 기준 15분", Text: strings.ReplaceAll(v3, "30분", "15분")})
	}
	st := &State{Seq: 3, EntrySeq: len(entries), PanelCounter: 1, Labels: map[string]string{"session-sort": "세션 목록 정렬"}, Topics: []TopicDoc{{Name: "session-sort", Entries: entries}}}
	v := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "panel-diff", Location: time.UTC})
	v.Lang = "ko"
	return v
}

var panelDiffPayload = regexp.MustCompile(`<!--wbpdg-data:(.*?)-->`)

func TestPanelDiffRenderLatestOnly(t *testing.T) {
	v := panelDiffFixture(t, false)
	out := render(t, v)
	matches := panelDiffPayload.FindAllStringSubmatch(out, -1)
	if len(matches) != 1 || strings.Count(out, `class="pdg-chip"`) != 1 {
		t.Fatalf("want one latest diff, got %d payloads", len(matches))
	}
	var data panelDiffData
	if err := json.Unmarshal([]byte(html.UnescapeString(matches[0][1])), &data); err != nil {
		t.Fatal(err)
	}
	if data.Prior.ID != "e4" || data.Prior.Version != 2 || data.Current.ID != "e6" || data.Current.Version != 3 || len(data.Changes) != 3 {
		t.Fatalf("not latest vs immediate prior: %+v", data)
	}
	wantAll(t, out, `<span>v2 대비</span><span class="n a">+1</span><span class="n m">~1</span><span class="n d">−1</span>`, `v2 · 09:55 대비 변경`, `<!--wbpdg:0-->`)
	for _, c := range data.Changes {
		if strings.Contains(c.Old, "2시간") {
			t.Fatal("diff compared to v1, not v2")
		}
		if c.Kind == MarkMod && (!strings.Contains(c.Old, "<strong>최근 활동</strong>") || !strings.Contains(c.Old, "1시간") || len(c.Words) == 0) {
			t.Fatalf("old formatting or word diff lost: %+v", c)
		}
	}
	live := render(t, panelDiffFixture(t, true))
	wantAll(t, live, `<span>v3 대비</span><span class="n a">+0</span><span class="n m">~1</span><span class="n d">−0</span>`)
	if strings.Count(live, `<!--wbpdg-data:`) != 1 {
		t.Fatal("older version retained its marks after republish")
	}
	singleState := &State{Topics: []TopicDoc{{Name: "t", Entries: []Entry{{ID: "e1", Kind: EntryPanel, PanelID: "p1", Recap: "only version", Text: panelDiffV2}}}}}
	single := render(t, BuildView(singleState, ViewInput{Layout: LayoutFull}))
	if panelDiffPayload.MatchString(single) || strings.Contains(single, `class="pdg-chip"`) {
		t.Fatal("single version has a comparison")
	}
}

func TestPanelDiffSnapshotSanitizesOldHTMLAndNestedLists(t *testing.T) {
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	p := newPageData(View{Layout: LayoutFull, Lang: "en"}, a, false)
	e := EntryView{Entry: Entry{ID: "e2", Text: "- **parent** changed\n  - child\n"}, Diff: diffPanel("- **parent** old\n  - child\n", "- **parent** changed\n  - child\n")}
	out := string(p.renderPanel(e))
	var data panelDiffData
	m := panelDiffPayload.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatal(out)
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data.Changes[0].Old, "child") || !strings.Contains(data.Changes[0].Old, "<strong>parent</strong>") {
		t.Fatalf("nested units duplicated: %+v", data)
	}
	e.Text = "one new"
	e.Diff = diffPanel("one old", e.Text)
	out = string(p.renderPanel(e))
	m = panelDiffPayload.FindStringSubmatch(out)
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &data); err != nil {
		t.Fatal(err)
	}
	if data.Changes[0].Old != "<p>one old</p>" || !reflect.DeepEqual(data.Changes[0].Words, []wordEdit{{"equal", "one "}, {MarkDel, "old"}, {MarkNew, "new"}}) {
		t.Fatalf("single-paragraph snapshot lost its block or text: %+v", data)
	}
	e.Text = "<div><ul><li>new</li></ul></div>"
	e.Diff = diffPanel("<div><ul><li>old</li></ul></div>", e.Text)
	out = string(p.renderPanel(e))
	m = panelDiffPayload.FindStringSubmatch(out)
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data.Changes[0].Old, "<li>old</li>") || !reflect.DeepEqual(data.Changes[0].Words, []wordEdit{{MarkDel, "old"}, {MarkNew, "new"}}) {
		t.Fatalf("raw HTML list removed from its block: %+v", data)
	}
	e.Text = "<div>safe new</div>"
	e.Diff = diffPanel("<div onclick=\"alert(1)\">safe old<script>alert(1)</script></div>", e.Text)
	out = string(p.renderPanel(e))
	m = panelDiffPayload.FindStringSubmatch(out)
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data.Changes[0].Old, "onclick") || strings.Contains(data.Changes[0].Old, "<script") {
		t.Fatalf("active old HTML: %+v", data)
	}
}

func TestPanelDiffEmptyAndFramedRender(t *testing.T) {
	st := &State{Topics: []TopicDoc{{Name: "t", Entries: []Entry{
		{ID: "e1", Kind: EntryPanel, PanelID: "p1", Recap: "old", Text: "old body"},
		{ID: "e2", Kind: EntryPanel, PanelID: "p1", Recap: "empty"},
	}}}}
	v := BuildView(st, ViewInput{Layout: LayoutFull})
	out := render(t, v)
	wantAll(t, out, `class="pdg-chip"`, `<!--wbpdg:0-->`, `<div class="doc-body entry-body md">`)
	regions, err := RenderRegions(panelDiffFixture(t, false))
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(regions.Board), `data-sandbox="`+frameSandbox+`"`, `&lt;!--wbpdg-data:`, `class="pdg-chip"`)
}
