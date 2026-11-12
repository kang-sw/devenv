package wswhiteboard

import (
	"regexp"
	"strings"
	"testing"
)

// checkBoard holds one topic with a policy item, an ack, and four checks:
// (03) open, (04) answered ok, (05) answered no, (06) assumed ok, and a
// withdrawn check (07).
func checkBoard(t *testing.T) *State {
	t.Helper()
	st := &State{}
	mustApply(t, st, `<wb:topic name="t">
<wb:recap>status only</wb:recap>
<wb:add class="policy"><wb:title>pick</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>a</wb:recommend></wb:add>
<wb:add class="announced"><wb:title>told</wb:title></wb:add>
<wb:add class="check"><wb:title>look at the draft</wb:title><wb:why>the second section</wb:why><wb:link>draft.md</wb:link></wb:add>
<wb:add class="check"><wb:title>release notes</wb:title></wb:add>
<wb:add class="check"><wb:title>icon B</wb:title></wb:add>
<wb:add class="check"><wb:title>run it locally</wb:title></wb:add>
<wb:add class="check"><wb:title>gone</wb:title></wb:add>
</wb:topic>
<wb:answer id="4" value="ok">4=ok</wb:answer>
<wb:answer id="5" value="no">B is too dark; go with A</wb:answer>
<wb:answer id="6" value="ok" assumed="true">sounds fine</wb:answer>
<wb:patch id="7" status="withdrawn"/>`)
	return st
}

func viewIDs(list []ItemView) []int {
	var ids []int
	for _, iv := range list {
		ids = append(ids, iv.ID)
	}
	return ids
}

func TestCheckClassTakesAckAnswersAndWarnsOnOptions(t *testing.T) {
	st := &State{}
	out := mustApply(t, st, addXML(ClassCheck, "t", "verify", "x", "y"))
	if !strings.Contains(out.Info, "Options on a non-policy item: (01) [check]") {
		t.Fatalf("options on a check: info = %q", out.Info)
	}
	mustApply(t, st, addXML(ClassCheck, "t", "second")+addXML(ClassCheck, "t", "third"))
	mustApply(t, st, `<wb:answer id="1" value="ok">1=ok</wb:answer><wb:answer id="2" value="no">2=no^not yet</wb:answer><wb:answer id="3" value="looks right, ship it">fine</wb:answer>`)
	for id, want := range map[int]string{1: "ok", 2: "no", 3: "looks right, ship it"} {
		if got := item(t, st, id); got.Status != StatusAnswered || got.Answer != want || got.Class != ClassCheck {
			t.Fatalf("(%02d) = %+v, want answered %q", id, got, want)
		}
	}
	mustApply(t, st, `<wb:patch id="3" status="open" class="check"/>`)
	if got := item(t, st, 3); got.Status != StatusOpen || got.Class != ClassCheck {
		t.Fatalf("reopened check = %+v", got)
	}
	wantProblems(t, refused(t, st, `<wb:topic name="t"><wb:add class="gate"><wb:title>x</wb:title></wb:add></wb:topic>`), "class")
}

func TestQueryListsACheckLikeAnyItem(t *testing.T) {
	st := checkBoard(t)
	text := queryText(t, st, `{"topic":"t"}`)
	wantText(t, "check items", text, []string{
		"(03) [open] [check] look at the draft",
		"(04) [answered] [check] release notes",
		"answer: no",
	}, nil)
}

func TestCheckListHoldsOpenAnsweredAndAssumedChecks(t *testing.T) {
	st := checkBoard(t)
	for name, layout := range layouts() {
		topic := topicView(t, st, layout, "t")
		if got := viewIDs(topic.Checks); len(got) != 4 || got[0] != 3 || got[1] != 4 || got[2] != 5 || got[3] != 6 {
			t.Fatalf("%s: Checks = %v, want [3 4 5 6] in add order", name, got)
		}
		if got := viewIDs(topic.Acks); len(got) != 1 || got[0] != 2 {
			t.Fatalf("%s: Acks = %v, want only the announced item", name, got)
		}
		settled := append(viewIDs(topic.Expanded), viewIDs(topic.Collapsed)...)
		if len(settled) != 1 || settled[0] != 7 {
			t.Fatalf("%s: settled items = %v, want only the withdrawn check", name, settled)
		}
		if topic.OpenChecks() != 1 || topic.AnsweredChecks() != 3 || topic.AskCount() != 2 {
			t.Fatalf("%s: open %d, answered %d, ask count %d; want 1, 3, 2", name, topic.OpenChecks(), topic.AnsweredChecks(), topic.AskCount())
		}
		if !topic.ChecksShown() || topic.Quiet {
			t.Fatalf("%s: an open topic with checks shows the list and is not quiet", name)
		}
	}
	// An answered check takes no place among the board's expanded settled
	// items.
	for _, id := range recentSettled(st) {
		if id >= 4 && id <= 6 {
			t.Fatalf("recentSettled holds check (%02d)", id)
		}
	}
}

func TestOpenChecksCountAsAsksNotAcks(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassCheck, "c", "only checks")+addXML(ClassCheck, "c", "another"))
	if n := st.OpenAskCount(); n != 2 {
		t.Fatalf("OpenAskCount = %d, want 2 open checks", n)
	}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if !view.Answerable || view.BulkAck {
		t.Fatalf("a board of open checks is answerable with no bulk ack box: %+v", view)
	}
	page := render(t, view)
	if !strings.Contains(page, `<span class="chip policy" title="to answer">`) || strings.Contains(page, `class="chip ok"`) || strings.Contains(page, `id="ack-all"`) {
		t.Fatalf("open checks count in the amber chip only:\n%s", page)
	}
	mustApply(t, st, `<wb:answer id="1" value="ok">ok</wb:answer>`)
	if n := st.OpenAskCount(); n != 1 {
		t.Fatalf("an answered check leaves the count: OpenAskCount = %d", n)
	}

	// The mixed board: one policy item and one open check make the header
	// and topic chip 2, and the bulk ack box answers only the ack.
	page = render(t, BuildView(checkBoard(t), ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	header := regexp.MustCompile(`<span class="counts">.*?</div>`).FindString(page)
	if !strings.Contains(header, `</svg>2</span>`) || !strings.Contains(header, `class="chip ok"`) {
		t.Fatalf("header counts: %s", header)
	}
	if !regexp.MustCompile(`id="ack-all"><svg[^>]*><use href="#i-check"/></svg>1</label>`).MatchString(page) {
		t.Fatalf("the bulk ack box counts the ack alone:\n%s", page)
	}
}

func TestChecksOnlyTopicChipIsAPlainCount(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassCheck, "c", "only checks"))
	mustApply(t, st, addXML(ClassPolicy, "p", "choose", "x", "y")+addXML(ClassCheck, "p", "and look"))
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	topic := func(name string) string {
		return regexp.MustCompile(`(?s)<section class="topic[^"]*" data-topic="` + name + `">.*?</section>`).FindString(board)
	}
	if c := topic("c"); strings.Contains(c, "data-bulk") || !strings.Contains(c, `<span class="chip policy"`) {
		t.Fatalf("a checks-only topic's amber chip is a plain count:\n%s", c)
	}
	if p := topic("p"); !regexp.MustCompile(`data-bulk="policy"[^>]*>.*?</svg>2</button>`).MatchString(p) {
		t.Fatalf("a topic with a policy item keeps the bulk button, counting its check too:\n%s", p)
	}
	if regions.CountPolicy != 3 {
		t.Fatalf("Regions.CountPolicy = %d, want 3", regions.CountPolicy)
	}
}

func TestCheckRowsRenderTheirStates(t *testing.T) {
	page := render(t, BuildView(checkBoard(t), ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	row := func(id string) string {
		return regexp.MustCompile(`<div class="crow[^"]*" id="item-` + id + `".*?</div>`).FindString(page)
	}
	open := row("3")
	for _, want := range []string{`class="crow" id="item-3" data-id="03"`, `<label class="ck"><input type="checkbox" data-id="03">`, `class="btn mini no-toggle" aria-pressed="false"`, `<span class="why">the second section</span>`, "draft.md"} {
		if !strings.Contains(open, want) {
			t.Fatalf("open check row missing %q:\n%s", want, open)
		}
	}
	if ok := row("4"); !strings.Contains(ok, `class="crow done"`) || !strings.Contains(ok, `checked disabled`) || !strings.Contains(ok, `<span class="stamp">ok</span>`) || strings.Contains(ok, "data-id") {
		t.Fatalf("answered ok row:\n%s", ok)
	}
	if no := row("5"); !strings.Contains(no, `class="crow no"`) || !strings.Contains(no, `class="cbox"`) || !strings.Contains(no, `<span class="stamp">no</span>`) || !strings.Contains(no, `B is too dark; go with A</span>`) {
		t.Fatalf("answered no row:\n%s", no)
	}
	if as := row("6"); !strings.Contains(as, `data-id="06" data-assumed`) || !strings.Contains(as, `class="amark"`) || !strings.Contains(as, "note-toggle") {
		t.Fatalf("assumed row:\n%s", as)
	}
	if strings.Contains(page, `label class="ack"><input type="checkbox" data-id="03"`) {
		t.Fatal("a check row reuses label.ack")
	}
	if !strings.Contains(page, `answered 3 / 4`) {
		t.Fatal("the check list header carries its progress")
	}
	// Placement: recap, check list, then the policy and ack items.
	recap, list, policy := strings.Index(page, `class="trecap"`), strings.Index(page, `class="checks"`), strings.Index(page, `class="card policy"`)
	if !(recap >= 0 && recap < list && list < policy) {
		t.Fatalf("placement recap %d, checks %d, policy %d", recap, list, policy)
	}
}

func TestClosedTopicFoldsItsChecksWithTheSettledItems(t *testing.T) {
	st := checkBoard(t)
	mustApply(t, st, `<wb:answer id="1" value="a">a</wb:answer><wb:answer id="2" value="ok">ok</wb:answer><wb:answer id="3" value="ok">ok</wb:answer><wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	topic := topicView(t, st, LayoutFull, "t")
	if topic.ChecksShown() || len(topic.Checks) != 4 || topic.SettledCount() != 3+4 {
		t.Fatalf("closed topic: shown %v, checks %d, settled %d", topic.ChecksShown(), len(topic.Checks), topic.SettledCount())
	}
	page := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	fold := regexp.MustCompile(`(?s)<details class="settled".*?</details>`).FindString(page)
	if !strings.Contains(fold, `class="checks"`) {
		t.Fatalf("the check list folds into the settled details:\n%s", fold)
	}
}

func TestOpenCheckRanksAsAnAskNotAnAck(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassAnnounced, "ack", "fyi")+addXML(ClassCheck, "chk", "look"))
	rank := bumpRank(st, map[string]int{"ack": st.Items[0].Changed, "chk": st.Items[1].Changed})
	if rank["chk"] != bumpPolicy || rank["ack"] != bumpAck {
		t.Fatalf("ranks = %v", rank)
	}
}
