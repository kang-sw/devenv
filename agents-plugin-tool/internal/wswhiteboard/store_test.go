package wswhiteboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)

func testEnv() Env {
	return Env{
		Now:      testNow,
		ReadFile: func(string) ([]byte, error) { return nil, errors.New("no such file") },
		IsDir:    func(string) bool { return false },
		IsAbs:    filepath.IsAbs,
	}
}

// payload decodes a JSON literal the way the MCP layer receives it.
func payload(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("bad test payload %s: %v", raw, err)
	}
	return v
}

func mustApply(t *testing.T, st *State, raw string) Outcome {
	t.Helper()
	out, problems := Apply(st, payload(t, raw), testEnv())
	if len(problems) > 0 {
		t.Fatalf("Apply(%s) refused: %v", raw, problems)
	}
	return out
}

func refused(t *testing.T, st *State, raw string) []string {
	t.Helper()
	before, _ := json.Marshal(st)
	_, problems := Apply(st, payload(t, raw), testEnv())
	if len(problems) == 0 {
		t.Fatalf("Apply(%s) must refuse", raw)
	}
	after, _ := json.Marshal(st)
	if string(before) != string(after) {
		t.Fatalf("a refused batch must leave the state untouched:\nbefore %s\nafter  %s", before, after)
	}
	return problems
}

func hasProblem(problems []string, substr string) bool {
	for _, p := range problems {
		if strings.Contains(p, substr) {
			return true
		}
	}
	return false
}

func item(t *testing.T, st *State, id int) Item {
	t.Helper()
	idx := st.index(id)
	if idx < 0 {
		t.Fatalf("item %d not in store", id)
	}
	return st.Items[idx]
}

func TestApplyBatchInOrderReturnsIDsInAddOrder(t *testing.T) {
	st := &State{}
	out := mustApply(t, st, `{"add":[
		{"class":"policy","topic":"t","title":"first","options":["x","y"],"recommendation":"b: safer"},
		{"class":"technical","topic":"t","title":"second","rationale":"because"},
		{"class":"announced","topic":"u","title":"third"}],
		"answer":[{"id":2,"answer":"ok","answer_quote":"fine"}]}`)
	if fmt.Sprint(out.IDs) != "[1 2 3]" {
		t.Fatalf("minted IDs = %v, want [1 2 3]", out.IDs)
	}
	// The answer applied after the add in the same batch.
	if got := item(t, st, 2); got.Status != StatusAnswered || got.AnswerQuote != "fine" {
		t.Fatalf("item 2 = %+v, want answered with quote", got)
	}
	if st.Seq != 1 {
		t.Fatalf("one update advances the render sequence once; Seq = %d", st.Seq)
	}
}

func TestApplyExplicitIDAdoptsAndRaisesCounter(t *testing.T) {
	st := &State{}
	out := mustApply(t, st, `{"add":[{"id":40,"class":"policy","topic":"q","title":"adopted"},{"class":"technical","topic":"q","title":"minted"}]}`)
	if fmt.Sprint(out.IDs) != "[40 41]" {
		t.Fatalf("IDs = %v, want [40 41]", out.IDs)
	}
	// A lower explicit ID not yet in the store is adopted without lowering the counter.
	out = mustApply(t, st, `{"add":[{"id":7,"class":"policy","topic":"q","title":"older"},{"class":"policy","topic":"q","title":"next"}]}`)
	if fmt.Sprint(out.IDs) != "[7 42]" {
		t.Fatalf("IDs = %v, want [7 42]", out.IDs)
	}
	problems := refused(t, st, `{"add":[{"id":40,"class":"policy","topic":"q","title":"dup"}]}`)
	if !hasProblem(problems, "(40) is already in the store") {
		t.Fatalf("duplicate explicit id problems = %v", problems)
	}
	problems = refused(t, st, `{"add":[{"id":"(43)","class":"policy","topic":"q","title":"string id"}]}`)
	if !hasProblem(problems, "payload.add[0].id: must be a number; got string") {
		t.Fatalf("string id problems = %v", problems)
	}
}

func TestApplyRefusedBatchAppliesNothingAndReportsEveryProblem(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[{"class":"policy","topic":"t","title":"one"}]}`)
	problems := refused(t, st, `{
		"add":[{"class":"wrong","topic":"t","title":"x"},{"class":"policy","title":"no topic"}],
		"answer":[{"id":99,"answer":"a","answer_quote":"q"}],
		"patch":[{"id":1,"status":"answered"}],
		"bogus":1}`)
	for _, want := range []string{
		"payload.add[0].class: must be one of policy, technical, announced",
		"payload.add[1].topic: required",
		"payload.answer[0].id: (99) is not in the store",
		"payload.patch[0].status: must be one of open, withdrawn, superseded",
		"payload.bogus: unknown field",
	} {
		if !hasProblem(problems, want) {
			t.Fatalf("problems missing %q: %v", want, problems)
		}
	}
	if st.Counter != 1 || len(st.Items) != 1 {
		t.Fatalf("refused batch changed the store: %+v", st)
	}
}

func TestApplyPatchOptionsRefusedOnNonOpenItem(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[{"class":"policy","topic":"t","title":"p","options":["x","y"]}],
		"answer":[{"id":1,"answer":"b","answer_quote":"b please"}]}`)
	problems := refused(t, st, `{"patch":[{"id":1,"options":["z","x","y"]}]}`)
	if !hasProblem(problems, "options change only on an open item") {
		t.Fatalf("problems = %v", problems)
	}
	// Re-opening first, as its own patch entry, then changing options is accepted.
	mustApply(t, st, `{"patch":[{"id":1,"status":"open"},{"id":1,"options":["z","x","y"]}]}`)
	if got := item(t, st, 1); got.Status != StatusOpen || len(got.Options) != 3 || got.Settled != 0 {
		t.Fatalf("item = %+v", got)
	}
	// Reopening drops the earlier answer; withdrawing keeps it as history.
	if got := item(t, st, 1); got.Answer != "" || got.AnswerQuote != "" || got.AnsweredAt != "" {
		t.Fatalf("a reopened item must carry no answer: %+v", got)
	}
	mustApply(t, st, `{"answer":[{"id":1,"answer":"a","answer_quote":"a then"}]}`)
	mustApply(t, st, `{"patch":[{"id":1,"status":"withdrawn"}]}`)
	if got := item(t, st, 1); got.Status != StatusWithdrawn || got.Answer != "a" || got.AnswerQuote != "a then" {
		t.Fatalf("a withdrawn item keeps its answer: %+v", got)
	}
}

func TestApplyAnswerRequiresQuote(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[{"class":"technical","topic":"t","title":"x"}]}`)
	problems := refused(t, st, `{"answer":[{"id":1,"answer":"ok"}]}`)
	if !hasProblem(problems, "payload.answer[0].answer_quote: required") {
		t.Fatalf("problems = %v", problems)
	}
	problems = refused(t, st, `{"answer":[{"id":1,"answer":"ok","answer_quote":"  "}]}`)
	if !hasProblem(problems, "answer_quote: must be non-empty") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestApplyPolicyAnswerIsOptionLetter(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[{"class":"policy","topic":"t","title":"p","options":["x","y"]}]}`)
	problems := refused(t, st, `{"answer":[{"id":1,"answer":"yes","answer_quote":"yes"}]}`)
	if !hasProblem(problems, "starts with its option letter (a-b)") {
		t.Fatalf("problems = %v", problems)
	}
	mustApply(t, st, `{"answer":[{"id":1,"answer":"b, keep it small","answer_quote":"b but small"}]}`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	iv := view.Topics[0].Expanded[0]
	if iv.AnswerLetter != "b" || iv.AnswerText != "keep it small" || !iv.Options[1].Picked {
		t.Fatalf("answer split = %+v", iv)
	}
}

func TestApplyAssumedThenConfirmedMovesToAnswered(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[{"class":"policy","topic":"t","title":"p","options":["x","y"]}]}`)
	mustApply(t, st, `{"answer":[{"id":1,"answer":"a","answer_quote":"sounds right","assumed":true}]}`)
	got := item(t, st, 1)
	if got.Status != StatusAssumed || got.Settled != 0 {
		t.Fatalf("assumed item = %+v", got)
	}
	later := testEnv()
	later.Now = testNow.Add(time.Hour)
	if _, problems := Apply(st, payload(t, `{"answer":[{"id":1,"answer":"a","answer_quote":"(01) ok"}]}`), later); len(problems) > 0 {
		t.Fatalf("confirm refused: %v", problems)
	}
	got = item(t, st, 1)
	if got.Status != StatusAnswered || got.AnswerQuote != "(01) ok" || got.AnsweredAt != testNow.Add(time.Hour).Format(time.RFC3339) || got.Settled == 0 {
		t.Fatalf("confirmed item = %+v, want answered with the new quote and time", got)
	}
}

func TestApplyJSONStringPayloadAndArrayFieldsRefusedTypeAccurately(t *testing.T) {
	st := &State{}
	_, problems := Apply(st, `{"add":[]}`, testEnv())
	if !hasProblem(problems, "payload: must be an object; got string") {
		t.Fatalf("string payload problems = %v", problems)
	}
	problems = refused(t, st, `{"add":"[{\"class\":\"policy\"}]","close_topics":"t","patch":{"id":1}}`)
	for _, want := range []string{
		"payload.add: must be an array of objects; got string (pass the value itself, not a JSON-encoded string)",
		"payload.close_topics: must be an array of strings; got string",
		"payload.patch: must be an array of objects; got object",
	} {
		if !hasProblem(problems, want) {
			t.Fatalf("problems missing %q: %v", want, problems)
		}
	}
	problems = refused(t, st, `{"add":[{"class":"policy","topic":"t","title":"x","options":"[\"a\"]"}]}`)
	if !hasProblem(problems, "payload.add[0].options: must be an array of strings; got string") {
		t.Fatalf("options problems = %v", problems)
	}
}

func TestApplyIDFloor(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[{"class":"policy","topic":"t","title":"one"}]}`)
	out := mustApply(t, st, `{"id_floor":30,"add":[{"class":"policy","topic":"t","title":"after floor"}]}`)
	if fmt.Sprint(out.IDs) != "[31]" {
		t.Fatalf("IDs after floor 30 = %v, want [31]", out.IDs)
	}
	// A floor at or below the counter is ignored without error.
	out = mustApply(t, st, `{"id_floor":5,"add":[{"class":"policy","topic":"t","title":"low floor"}]}`)
	if fmt.Sprint(out.IDs) != "[32]" {
		t.Fatalf("IDs after a low floor = %v, want [32]", out.IDs)
	}
	mustApply(t, st, `{"id_floor":32}`)
	if st.Counter != 32 {
		t.Fatalf("counter = %d, want 32", st.Counter)
	}
}

func settleItems(t *testing.T, st *State, topic string, n int) []int {
	t.Helper()
	var adds []string
	for i := 0; i < n; i++ {
		adds = append(adds, fmt.Sprintf(`{"class":"technical","topic":%q,"title":"item %d"}`, topic, i))
	}
	out := mustApply(t, st, `{"add":[`+strings.Join(adds, ",")+`]}`)
	var answers []string
	for _, id := range out.IDs {
		answers = append(answers, fmt.Sprintf(`{"id":%d,"answer":"ok","answer_quote":"ok"}`, id))
	}
	mustApply(t, st, `{"answer":[`+strings.Join(answers, ",")+`]}`)
	return out.IDs
}

func TestRotationKeepsTenMostRecentSettledBoardWide(t *testing.T) {
	st := &State{}
	older := settleItems(t, st, "a", 8)
	newer := settleItems(t, st, "b", 6)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	expanded := map[int]bool{}
	collapsed := 0
	for _, topic := range view.Topics {
		for _, iv := range topic.Expanded {
			expanded[iv.ID] = true
		}
		collapsed += len(topic.Collapsed)
	}
	if len(expanded) != ExpandedSettled || collapsed != 4 {
		t.Fatalf("expanded %d collapsed %d, want 10 and 4", len(expanded), collapsed)
	}
	for _, id := range newer {
		if !expanded[id] {
			t.Fatalf("newest item %d must be expanded", id)
		}
	}
	for _, id := range older[:4] {
		if expanded[id] {
			t.Fatalf("oldest item %d must be collapsed", id)
		}
	}
	// Closing a topic collapses it at once; the freed slots go to no one
	// else here because topic b already shows all of its items.
	mustApply(t, st, `{"close_topics":["a"]}`)
	view = BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	for _, topic := range view.Topics {
		if topic.Name == "a" && (len(topic.Expanded) != 0 || len(topic.Collapsed) != 8 || !topic.Quiet || !topic.Closed) {
			t.Fatalf("closed topic a = %+v", topic)
		}
	}
	problems := refused(t, st, `{"close_topics":["missing"]}`)
	if !hasProblem(problems, `no item has topic "missing"`) {
		t.Fatalf("problems = %v", problems)
	}
}

func TestArchiveCapEvictsOldestSettled(t *testing.T) {
	st := &State{}
	first := settleItems(t, st, "a", 300)
	settleItems(t, st, "b", 210)
	mustApply(t, st, `{"add":[{"class":"policy","topic":"c","title":"still open"}]}`)
	settled := 0
	for _, it := range st.Items {
		if IsSettled(it.Status) {
			settled++
		}
	}
	if settled != ArchiveCap {
		t.Fatalf("settled kept = %d, want %d", settled, ArchiveCap)
	}
	for _, id := range first[:10] {
		if st.index(id) >= 0 {
			t.Fatalf("oldest settled item %d must be evicted", id)
		}
	}
	if st.index(first[10]) < 0 {
		t.Fatalf("item %d must survive the cap", first[10])
	}
	// Eviction never lowers the counter, so a minted ID never repeats.
	out := mustApply(t, st, `{"add":[{"class":"policy","topic":"c","title":"next"}]}`)
	if out.IDs[0] != 512 {
		t.Fatalf("next minted ID = %d, want 512", out.IDs[0])
	}
}

func TestQueryReturnsArchivedAndFilters(t *testing.T) {
	st := &State{}
	settleItems(t, st, "a", 12)
	mustApply(t, st, `{"add":[{"class":"policy","topic":"b","title":"open one"}]}`)
	all, problems := Query(st, nil)
	if len(problems) > 0 || len(all) != 13 {
		t.Fatalf("query all = %d items, problems %v", len(all), problems)
	}
	// Item 1 is collapsed on the page but still returned.
	byID, _ := Query(st, payload(t, `{"id":1}`))
	if len(byID) != 1 || byID[0].Status != StatusAnswered {
		t.Fatalf("query id 1 = %+v", byID)
	}
	open, _ := Query(st, payload(t, `{"status":"open","topic":"b"}`))
	if len(open) != 1 || open[0].ID != 13 {
		t.Fatalf("query open b = %+v", open)
	}
	if _, problems := Query(st, payload(t, `{"status":"done"}`)); !hasProblem(problems, "payload.status: must be one of") {
		t.Fatalf("bad status problems = %v", problems)
	}
	if _, problems := Query(st, `{}`); !hasProblem(problems, "payload: must be an object; got string") {
		t.Fatalf("string query payload problems = %v", problems)
	}
	if !strings.Contains(FormatItems(byID), "(01) [answered] [technical] a: item 0") {
		t.Fatalf("formatted = %s", FormatItems(byID))
	}
}

func TestNowFreshReadsStoreWithoutUpdateSeq(t *testing.T) {
	var st State
	if err := json.Unmarshal([]byte(`{"seq":4,"now":"<p>x</p>","now_seq":4}`), &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !st.NowFresh() {
		t.Fatalf("a store without update_seq must judge Now against the current render")
	}
	st.BeginRender()
	if st.NowFresh() {
		t.Fatalf("a store without update_seq keeps the old per-render judgement")
	}
}

func TestApplyNowRules(t *testing.T) {
	st := &State{}
	env := testEnv()
	content := map[string]string{"/tmp/now.html": "<p>from file</p>", "/tmp/now.md": "# Status\n\n- <b>green</b>\n"}
	env.ReadFile = func(path string) ([]byte, error) {
		if v, ok := content[path]; ok {
			return []byte(v), nil
		}
		return nil, errors.New("missing")
	}
	if _, problems := Apply(st, payload(t, `{"now_path":"/tmp/now.html"}`), env); len(problems) > 0 {
		t.Fatalf("now_path refused: %v", problems)
	}
	delete(content, "/tmp/now.html")
	if st.Now != "<p>from file</p>" || !st.NowFresh() {
		t.Fatalf("now_path content must be stored once: %q", st.Now)
	}
	// An implicit render keeps the stored content although the file is gone,
	// and keeps Now fresh: freshness follows the latest update only.
	st.BeginRender()
	if st.Now != "<p>from file</p>" || !st.NowFresh() {
		t.Fatalf("Now after an implicit render: %q fresh=%v", st.Now, st.NowFresh())
	}
	mustApply(t, st, `{"id_floor":1}`)
	if st.NowFresh() {
		t.Fatalf("an update without now_path must leave Now stale")
	}
	// The inline field is gone: refused with a pointer at now_path, and
	// nothing else in the batch applies.
	for _, raw := range []string{`{"now":"<p>x</p>"}`, `{"now":"<p>x</p>","now_path":"/tmp/now.md"}`, `{"now":""}`} {
		before := st.Now
		_, problems := Apply(st, payload(t, raw), env)
		if !hasProblem(problems, "payload.now: no longer accepted") || !hasProblem(problems, "now_path") {
			t.Fatalf("%s problems = %v", raw, problems)
		}
		if hasProblem(problems, "unknown field") {
			t.Fatalf("%s: the removed field must not also read as an unknown field: %v", raw, problems)
		}
		if st.Now != before {
			t.Fatalf("%s: a refused batch changed Now to %q", raw, st.Now)
		}
	}
	for _, doc := range []string{`<!DOCTYPE html><p>x</p>`, `<html><body>x</body></html>`, `<HTML lang=en>`,
		"# Title\n\n<!DOCTYPE html>\n<p>x</p>", "<!doctype html>\n<html><body>x</body></html>", "Some text <html lang=en> inline."} {
		content["/tmp/doc.md"] = doc
		if _, problems := Apply(st, payload(t, `{"now_path":"/tmp/doc.md"}`), env); !hasProblem(problems, "markdown with inline HTML, not a full document") {
			t.Fatalf("full document %q problems = %v", doc, problems)
		}
	}
	// A Now that only mentions a document tag in code (or in text goldmark
	// escapes, such as a lowercase doctype alone) is markdown, not a full
	// document.
	for _, doc := range []string{"Wrap it in `<html>` and start with `<!DOCTYPE html>`.",
		"Example:\n\n```html\n<!DOCTYPE html>\n<html lang=en><body></body></html>\n```\n",
		"Indented:\n\n    <html>\n", "Text <!doctype html> escaped."} {
		other := &State{}
		content["/tmp/doc.md"] = doc
		if _, problems := Apply(other, payload(t, `{"now_path":"/tmp/doc.md"}`), env); len(problems) > 0 || other.Now != doc {
			t.Fatalf("code-only mention %q: problems %v, Now %q", doc, problems, other.Now)
		}
	}
	// now_path names a markdown file, stored verbatim like any Now text.
	if _, problems := Apply(st, payload(t, `{"now_path":"/tmp/now.md"}`), env); len(problems) > 0 || st.Now != "# Status\n\n- <b>green</b>\n" {
		t.Fatalf("now_path .md file: %v %q", problems, st.Now)
	}
	// Reading the same text again is not a change.
	mustApply(t, st, `{"id_floor":2}`)
	if _, problems := Apply(st, payload(t, `{"now_path":"/tmp/now.md"}`), env); len(problems) > 0 {
		t.Fatalf("now_path refused: %v", problems)
	}
	if st.NowFresh() {
		t.Fatalf("identical Now must not count as changed")
	}
	// An empty now_path clears the slot, without reading any file.
	mustApply(t, st, `{"now_path":""}`)
	if st.Now != "" || st.NowSeq != 0 {
		t.Fatalf("empty now_path must clear the slot: %q seq %d", st.Now, st.NowSeq)
	}
	if _, problems := Apply(st, payload(t, `{"now_path":"/tmp/absent.md"}`), env); !hasProblem(problems, "payload.now_path: cannot read /tmp/absent.md") {
		t.Fatalf("unreadable now_path problems = %v", problems)
	}
}

func TestApplyEmbedDir(t *testing.T) {
	st := &State{}
	// Absolute paths come from t.TempDir so the test holds on Windows, where
	// a slash-rooted path such as /host/scratch is not absolute.
	base := t.TempDir()
	scratch := filepath.Join(base, "scratch")
	missing := filepath.Join(base, "missing")
	embedDir := func(dir string) string { return fmt.Sprintf(`{"embed_dir":%q}`, dir) }
	dirs := map[string]bool{scratch: true}
	env := testEnv()
	env.IsDir = func(path string) bool { return dirs[path] }
	if _, problems := Apply(st, payload(t, `{"embed_dir":"relative/dir"}`), env); !hasProblem(problems, "must be an absolute path") {
		t.Fatalf("relative embed_dir problems = %v", problems)
	}
	if _, problems := Apply(st, payload(t, embedDir(missing)), env); !hasProblem(problems, "not an existing directory") {
		t.Fatalf("missing embed_dir problems = %v", problems)
	}
	if _, problems := Apply(st, payload(t, embedDir(scratch)), env); len(problems) > 0 {
		t.Fatalf("embed_dir refused: %v", problems)
	}
	if st.EmbedDir != scratch {
		t.Fatalf("embed_dir must be remembered: %q", st.EmbedDir)
	}
	// The stored folder is re-checked by every later update.
	delete(dirs, scratch)
	if _, problems := Apply(st, payload(t, `{}`), env); !hasProblem(problems, scratch+" (stored for this session) is no longer an existing directory") {
		t.Fatalf("vanished embed_dir problems = %v", problems)
	}
	// Clearing it in the same update is accepted.
	if _, problems := Apply(st, payload(t, `{"embed_dir":""}`), env); len(problems) > 0 || st.EmbedDir != "" {
		t.Fatalf("clearing embed_dir: problems %v dir %q", problems, st.EmbedDir)
	}
}

func TestApplyAcceptsAnyItemReference(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[{"class":"policy","topic":"t","title":"old"}]}`)
	mustApply(t, st, `{"add":[{"class":"policy","topic":"t","title":"new","links":["supersedes {{01}}","261030-feat-x"]}],
		"patch":[{"id":1,"status":"superseded","links":["superseded by {{02}}"]}]}`)
	// References are display only: an ID the store never held, prose
	// parentheses, and non-digit brace tokens are all accepted.
	mustApply(t, st, `{"add":[{"class":"technical","topic":"t","title":"see {{77}} after step (2)",
		"rationale":"step (3) then {{my-tag}}","links":["see {{77}}","step (2)"]}]}`)
	if got := st.Items[2].Links; len(got) != 2 || got[0] != "see {{77}}" || got[1] != "step (2)" {
		t.Fatalf("links stored as %q", got)
	}
}

func TestApplyEmptyPayloadOnlyAdvancesRender(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{}`)
	if _, problems := Apply(st, nil, testEnv()); len(problems) > 0 {
		t.Fatalf("absent payload refused: %v", problems)
	}
	if st.Seq != 2 || len(st.Items) != 0 {
		t.Fatalf("state after empty updates = %+v", st)
	}
}

func TestBuildViewTopicOrderAndEmbedFreshness(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[
		{"class":"technical","topic":"settled","title":"s"},
		{"class":"technical","topic":"acks","title":"k"},
		{"class":"policy","topic":"assumed","title":"m","options":["x","y"]},
		{"class":"policy","topic":"policy","title":"p","options":["x","y"]}],
		"answer":[{"id":1,"answer":"ok","answer_quote":"ok"},{"id":3,"answer":"a","answer_quote":"maybe","assumed":true}]}`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	var names []string
	for _, topic := range view.Topics {
		names = append(names, topic.Name)
	}
	if strings.Join(names, ",") != "policy,assumed,acks,settled" {
		t.Fatalf("topic order = %v", names)
	}
	if !view.BulkAck || !view.Answerable || view.StampFile != "r.stamp.js" {
		t.Fatalf("view flags = %+v", view)
	}
	// Mark the first embed, then change one item: only it is new.
	st.LastEmbedSeq = st.Seq
	mustApply(t, st, `{"answer":[{"id":2,"answer":"ok","answer_quote":"ok"}]}`)
	embed := BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"})
	fresh := 0
	for _, topic := range embed.Topics {
		fresh += topic.Fresh
		for _, iv := range topic.Expanded {
			if iv.ID != 2 || !iv.New {
				t.Fatalf("embed expanded only the new settled item: %+v", iv)
			}
		}
		if topic.Name == "settled" && (!topic.Quiet || len(topic.Collapsed) != 1) {
			t.Fatalf("unchanged settled topic must be quiet in the embed: %+v", topic)
		}
	}
	if fresh != 1 || embed.StampFile != "" {
		t.Fatalf("embed fresh = %d stamp %q", fresh, embed.StampFile)
	}
}
