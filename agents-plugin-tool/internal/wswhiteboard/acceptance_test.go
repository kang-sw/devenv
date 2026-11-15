package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Compare the entire resulting board and effects with an independently applied
// accepted-only batch. This includes events, IDs, defaults, marks and counters.
func mixed(t *testing.T, s *State, input, accepted string) []string {
	t.Helper()
	return mixedAt(t, s, input, accepted, testNow)
}

func mixedAt(t *testing.T, s *State, input, accepted string, now time.Time) []string {
	t.Helper()
	expected := s.clone()
	env := testEnv()
	env.Now = now
	want, wp := Apply(expected, accepted, env)
	if len(wp) != 0 {
		t.Fatalf("invalid accepted-only fixture: %v", wp)
	}
	out, p := Apply(s, input, env)
	if len(p) == 0 || out.Unpartitioned || out.Failed == "" {
		t.Fatalf("not a partial result: %+v %v", out, p)
	}
	a, _ := json.Marshal(s)
	b, _ := json.Marshal(expected)
	if string(a) != string(b) || !reflect.DeepEqual(out.Effects, want.Effects) || !reflect.DeepEqual(out.Summaries, want.Summaries) || !reflect.DeepEqual(out.IDs, want.IDs) || !reflect.DeepEqual(out.Withdrawn, want.Withdrawn) {
		t.Fatalf("accepted-only mismatch:\n got %s\nwant %s\n effects %+v / %+v\n summaries %v / %v", a, b, out.Effects, want.Effects, out.Summaries, want.Summaries)
	}
	return p
}

func TestCommandAcceptanceRollbackOrderAndCompleteRetry(t *testing.T) {
	s := &State{}
	mustApply(t, s, addXML(ClassPolicy, "a", "Pick", "One", "Two")+addXML(ClassPolicy, "b", "Pick", "One", "Two")+topicXML("a", "old\n\nkeep\n\ntail"))
	body := "\n  <b>raw &amp; whitespace</b>\n<wb:made-up>inside raw body</wb:made-up>\n"
	bad := `<wb:answer ids="1 2 999" value="a">full & <b>verbatim</b></wb:answer>`
	good := `<wb:answer id="1" value="b">corrected</wb:answer>`
	panel := panelXML("a", "Accepted panel", "large accepted payload")
	log := logXML("b", "Accepted log")
	edit := `<wb:topic name="a"><wb:edit><wb:old>old</wb:old><wb:new>new</wb:new></wb:edit></wb:topic>`
	badEdit := `<wb:topic name="a"><wb:edit replace_all="false"><wb:old>absent</wb:old><wb:new>` + body + `</wb:new></wb:edit></wb:topic>`
	closeA := `<wb:topic name="a"><wb:close result="do not withdraw"/></wb:topic>`
	closeB := `<wb:topic name="b"><wb:close result="also blocked"/></wb:topic>`
	mixed(t, s, panel+bad+good+edit+badEdit+log+closeA+closeB, panel+good+edit+log)
	if s.SettleSeq != 1 || s.AnswerSeq != s.Seq || s.PanelCounter != 1 || item(t, s, 2).Status != StatusOpen {
		t.Fatalf("failed multi-ID bookkeeping leaked: %+v", s)
	}
	out, p := Apply(s, panel+badEdit+`<wb:patch id="2" mystery="kept"><wb:why>full fields &amp; text</wb:why></wb:patch>`, testEnv())
	if len(p) != 2 || !strings.Contains(out.Failed, body) || !strings.Contains(out.Failed, `mystery="kept"`) || strings.Contains(out.Failed, "large accepted payload") {
		t.Fatalf("incomplete retry: %s %v", out.Failed, p)
	}
	retry := strings.TrimSuffix(strings.TrimPrefix(out.Failed, "<wb:failed>\n"), "\n</wb:failed>")
	retry = strings.ReplaceAll(retry, "absent", "keep")
	retry = strings.ReplaceAll(retry, ` mystery="kept"`, "")
	panels, ids := s.PanelCounter, s.Counter
	mustApply(t, s, retry)
	if s.PanelCounter != panels || s.Counter != ids || item(t, s, 2).Rationale != "full fields & text" {
		t.Fatal("retry duplicated accepted commands or lost fields")
	}
}

func TestCommandAcceptanceMetadataDefaultsAndGuardIdentity(t *testing.T) {
	s := &State{}
	mustApply(t, s, addXML(ClassPolicy, "Old title", "Pick", "a", "b")+addXML(ClassPolicy, "other", "Pick", "a", "b"))
	badGroup := `<wb:topic name="adopt" label="Old title" tickets="bad"><wb:body>blocked body</wb:body></wb:topic>`
	good := topicXML("other", "accepted")
	close := `<wb:topic name="Old title"><wb:close result="blocked"/></wb:topic>`
	mixed(t, s, badGroup+good+close, good)
	if s.stemmed("adopt") || item(t, s, 1).Topic != "Old title" {
		t.Fatal("failed adoption escaped rollback")
	}
	// Guard follows a later accepted adoption, including a correction in between.
	badAnswer := `<wb:answer id="1" value="z">invalid option</wb:answer>`
	adopt := `<wb:topic name="adopt" label="Old title"/>`
	correct := `<wb:answer id="1" value="a">correct</wb:answer>`
	otherClose := `<wb:topic name="other"><wb:close result="allowed"/></wb:topic>`
	mixed(t, s, badAnswer+correct+adopt+`<wb:topic name="adopt"><wb:close result="blocked"/></wb:topic>`+otherClose, correct+adopt+otherClose)
	fresh := &State{}
	log := logXML("fresh", "created")
	mixed(t, fresh, `<wb:topic name="fresh"><wb:state value="waiting"/></wb:topic>`+log, log)
	if fresh.TopicStates()["fresh"].State.Value != StateDiscussing {
		t.Fatal("failed explicit state suppressed default")
	}
	// Failed creations are invisible to later commands; IDs are not consumed.
	fresh = &State{}
	accepted := addXML(ClassTechnical, "fresh", "minted")
	mixed(t, fresh, `<wb:topic name="fresh"><wb:add id="99" class="bad"><wb:title>bad</wb:title></wb:add></wb:topic><wb:answer id="99" value="ok">missing</wb:answer>`+accepted, accepted)
	if fresh.Counter != 1 {
		t.Fatal("failed explicit ID advanced counter")
	}
	// An unresolvable ID does not poison an unrelated close.
	mixed(t, fresh, `<wb:patch id="999"/>`+`<wb:topic name="fresh"><wb:close result="allowed"/></wb:topic>`, `<wb:topic name="fresh"><wb:close result="allowed"/></wb:topic>`)
}

func TestCommandAcceptanceParserPayloadsAndScopes(t *testing.T) {
	s := &State{}
	body := "\n\t<div>raw &amp; <wb:like>HTML</wb:like></div>\n<![CDATA[own </wb:panel> closing text]]>\n"
	badGroup := `<wb:topic name='g' label='Title &amp; More' tickets='bad'>`
	unknown := `<wb:unknown recap='Rejected'>` + body + `</wb:unknown>`
	badAttr := `<wb:panel recap="Also rejected" extra="keep">` + body + `</wb:panel>`
	accepted := topicXML("large", strings.Repeat("accepted only ", 100))
	input := accepted + badGroup + unknown + badAttr + `</wb:topic>`
	out, p := Apply(s, input, testEnv())
	if len(p) < 3 || !strings.Contains(out.Failed, badGroup) || !strings.Contains(out.Failed, unknown) || !strings.Contains(out.Failed, badAttr) || strings.Contains(out.Failed, "accepted only") || strings.Index(out.Failed, unknown) > strings.Index(out.Failed, badAttr) {
		t.Fatalf("scope/payload/order lost: %s %v", out.Failed, p)
	}
	if s.topicKnown("g") || s.PanelCounter != 0 {
		t.Fatal("failed scope children mutated board")
	}
	retry := strings.TrimSuffix(strings.TrimPrefix(out.Failed, "<wb:failed>\n"), "\n</wb:failed>")
	retry = strings.ReplaceAll(retry, "tickets='bad'", "tickets='261115-feat-example'")
	retry = strings.ReplaceAll(retry, "wb:unknown", "wb:panel")
	retry = strings.ReplaceAll(retry, ` extra="keep"`, "")
	mustApply(t, s, retry)
	var payload string
	for _, entry := range s.Topics[1].Entries {
		if entry.Kind == EntryPanel {
			payload = entry.Text
			break
		}
	}
	if s.PanelCounter != 2 || s.TopicLabel("g") != "Title & More" || payload != "\t<div>raw &amp; <wb:like>HTML</wb:like></div>\nown </wb:panel> closing text" {
		t.Fatal("corrected source payload changed", s.Topics[1])
	}
	// A recoverable parse error is local even without failed metadata.
	fresh := &State{}
	mixed(t, fresh, `<wb:topic name="t"><wb:panel recap="bad" typo="x">kept raw</wb:panel><wb:log>valid</wb:log></wb:topic><wb:unknown>raw <b>payload</b></wb:unknown>`, logXML("t", "valid"))
}

func TestCommandAcceptanceStrayFragments(t *testing.T) {
	for name, tc := range map[string]struct {
		input, failed, label string
		accepted             int
	}{
		"failed-empty-group": {
			input:    `<wb:topic name="a"><wb:body>x</wb:body></wb:topic><wb:topic name="b"></wb:oops></wb:topic>`,
			failed:   "<wb:failed>\n<wb:topic name=\"b\"></wb:oops></wb:topic>\n</wb:failed>",
			label:    `stray fragment inside <wb:topic name="b"> (line 1)`,
			accepted: 1,
		},
		"accepted-group": {
			input:    `<wb:topic name="b"><wb:body>x</wb:body></wb:oops></wb:topic>`,
			failed:   "<wb:failed>\n<wb:topic name=\"b\">\n</wb:oops>\n</wb:topic>\n</wb:failed>",
			label:    `stray fragment inside <wb:topic name="b"> (line 1)`,
			accepted: 1,
		},
		"root": {
			input:    `<wb:floor id="1"/></wb:oops>`,
			failed:   "<wb:failed>\n</wb:oops>\n</wb:failed>",
			label:    `stray fragment (line 1)`,
			accepted: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &State{}
			out, p := Apply(s, tc.input, testEnv())
			if out.Unpartitioned || out.Accepted != tc.accepted || out.Failed != tc.failed {
				t.Fatalf("fragment retry: %+v %v; want %s", out, p, tc.failed)
			}
			diagnostics := strings.Join(p, "\n")
			if !strings.Contains(diagnostics, tc.label+`: </wb:oops> closes nothing open`) || strings.Contains(diagnostics, "command -1 <>") {
				t.Fatalf("fragment diagnostics: %s", diagnostics)
			}
		})
	}
}

func TestCommandAcceptanceRejectedCDATABoundaries(t *testing.T) {
	for _, rejected := range []string{
		`<wb:unknown><![CDATA[</wb:unknown><wb:floor id="999"/>]]></wb:unknown>`,
		`<wb:patch id="1"><wb:bad><![CDATA[</wb:bad></wb:patch><wb:floor id="999"/>]]></wb:bad></wb:patch>`,
		`<wb:topic name="bad">not commands <![CDATA[</wb:topic><wb:floor id="999"/>]]></wb:topic>`,
	} {
		s := &State{}
		accepted := topicXML("valid", "accepted")
		mixed(t, s, rejected+accepted, accepted)
		if s.Counter != 0 {
			t.Fatal("rejected CDATA exposed payload as a command")
		}
	}
}

func TestCommandAcceptanceRejectedGroupFieldBoundaries(t *testing.T) {
	for name, payload := range map[string]string{
		"review-repro":       `<wb:add class="technical"><wb:title></wb:add><wb:topic name="literal"></wb:title></wb:add>`,
		"title":              `<wb:add class="technical"><wb:title></wb:add></wb:topic><wb:topic name="literal"><wb:floor id="999"/></wb:title></wb:add>`,
		"patch-title":        `<wb:patch id="1"><wb:title></wb:patch></wb:topic><wb:topic name="literal"></wb:title><wb:why>kept</wb:why></wb:patch>`,
		"body":               `<wb:body></wb:topic><wb:topic name="literal"><wb:floor id="999"/></wb:body>`,
		"unknown-body-field": `<wb:add class="technical"><wb:body></wb:add></wb:topic><wb:topic name="literal"></wb:body></wb:add>`,
		"old":                `<wb:edit><wb:old></wb:edit></wb:topic><wb:topic name="literal"></wb:old><wb:new>replacement</wb:new></wb:edit>`,
		"new":                `<wb:edit><wb:old>original</wb:old><wb:new></wb:edit></wb:topic><wb:topic name="literal"></wb:new></wb:edit>`,
		"answer":             `<wb:answer id="1" value="a"></wb:topic><wb:topic name="literal"><wb:floor id="999"/></wb:answer>`,
		"title-cdata":        `<wb:add class="technical"><wb:title><![CDATA[</wb:title></wb:add></wb:topic><wb:topic name="literal">]]></wb:add><wb:topic name="also-literal"></wb:title></wb:add>`,
		"old-new-cdata":      `<wb:edit><wb:old><![CDATA[</wb:old></wb:edit></wb:topic><wb:topic name="literal">]]></wb:edit><wb:topic name="also-literal"></wb:old><wb:new><![CDATA[</wb:new></wb:edit><wb:topic name="literal">]]></wb:edit><wb:topic name="also-literal"></wb:new></wb:edit>`,
	} {
		t.Run(name, func(t *testing.T) {
			prefix := topicXML("prefix", "keep")
			rejected := `<wb:topic name="outer">junk` + payload + `</wb:topic>`
			expected := &State{}
			want := mustApply(t, expected, prefix)
			s := &State{}
			out, p := Apply(s, prefix+rejected, testEnv())
			if len(p) == 0 || !strings.Contains(strings.Join(p, " "), "text before its first command") || out.Unpartitioned || out.Accepted != want.Accepted {
				t.Fatalf("rejected group discarded prefix: %+v %v", out, p)
			}
			gotState, _ := json.Marshal(s)
			wantState, _ := json.Marshal(expected)
			if string(gotState) != string(wantState) || !reflect.DeepEqual(out.Effects, want.Effects) {
				t.Fatalf("rejected payload changed accepted state/effects: %s / %s; %+v / %+v", gotState, wantState, out.Effects, want.Effects)
			}
			if want := "<wb:failed>\n" + rejected + "\n</wb:failed>"; out.Failed != want {
				t.Fatalf("incomplete failed tree:\n got %s\nwant %s", out.Failed, want)
			}
		})
	}
}

func TestCommandAcceptanceRejectedGroupFieldDiagnostic(t *testing.T) {
	prefix := `<wb:topic name="prefix"><wb:body>keep</wb:body></wb:topic>`
	rejected := `<wb:topic name="outer">junk<wb:add class="technical"><wb:title typo="x">T</wb:title></wb:add></wb:topic>`
	expected := &State{}
	want := mustApply(t, expected, prefix)
	s := &State{}
	out, p := Apply(s, prefix+rejected, testEnv())
	gotState, _ := json.Marshal(s)
	wantState, _ := json.Marshal(expected)
	if out.Unpartitioned || out.Accepted != want.Accepted || string(gotState) != string(wantState) || !reflect.DeepEqual(out.Effects, want.Effects) {
		t.Fatalf("rejected group changed prefix acceptance: %+v %v", out, p)
	}
	if want := "<wb:failed>\n" + rejected + "\n</wb:failed>"; out.Failed != want {
		t.Fatalf("failed tree: %s; want %s", out.Failed, want)
	}
	diagnostics := strings.Join(p, "\n")
	if !strings.Contains(diagnostics, `<wb:topic> (line 1) skipped <wb:add> field <wb:title>: unknown attribute typo (it takes none)`) || strings.Contains(diagnostics, "command 0") {
		t.Fatalf("skipped field diagnostic: %s", diagnostics)
	}
}

func TestCommandAcceptanceUnsplittableNeverAppliesPrefix(t *testing.T) {
	for _, suffix := range []string{`<wb:topic name="t"><wb:body>unterminated`, `<wb:topic name="t"><wb:body>closed</wb:body>`, `<wb:answer id="1" value="broken>`, `<wb:topic name="t"><wb:edit><wb:old>no end`, `<wb:topic name="a"><wb:topic name="b"><wb:topic name="c"/></wb:topic><wb:floor id="999"/></wb:topic>`} {
		s := &State{}
		before, _ := json.Marshal(s)
		out, p := Apply(s, topicXML("prefix", "valid prefix")+suffix, testEnv())
		after, _ := json.Marshal(s)
		if len(p) == 0 || !out.Unpartitioned || out.Accepted != 0 || string(before) != string(after) {
			t.Fatalf("unsafe prefix: %+v %v", out, p)
		}
	}
}

func TestCommandAcceptanceNestedGroupBoundaries(t *testing.T) {
	for name, beforeNested := range map[string]string{
		"text":                `junk`,
		"whitespace":          " \n\t",
		"cdata":               `<![CDATA[not a command </wb:topic>]]>`,
		"comment":             `<!-- not a command </wb:topic> -->`,
		"text-after-child":    `<wb:log>first</wb:log>junk`,
		"cdata-after-child":   `<wb:log>first</wb:log><![CDATA[junk]]>`,
		"comment-after-child": `<wb:log>first</wb:log><!-- junk -->`,
		"quoted-attribute":    `<div title='<wb:topic name="literal">'>junk</div>`,
		"unquoted-boundary":   `junk<div attr=`,
		"raw-child":           `junk<wb:log><wb:topic name="literal"></wb:topic></wb:log>`,
		"unclosed-comment":    `<!--`,
		"unclosed-cdata":      `<![CDATA[`,
		"unclosed-attribute":  `<div title='`,
	} {
		t.Run(name, func(t *testing.T) {
			s := &State{}
			before, _ := json.Marshal(s)
			input := topicXML("prefix", "valid prefix") + `<wb:topic name="outer">` + beforeNested + `<wb:topic name="inner"><wb:log>inside</wb:log></wb:topic><wb:floor id="999"/></wb:topic>`
			out, p := Apply(s, input, testEnv())
			after, _ := json.Marshal(s)
			if len(p) == 0 || !out.Unpartitioned || out.Accepted != 0 || string(before) != string(after) {
				t.Fatalf("nested group applied a prefix/counter: %+v %v; state %s", out, p, after)
			}
		})
	}
	// Group-like text in attributes, comments, CDATA or raw command bodies
	// is not a nested group and must not expose its quoted closing tag.
	for name, rejected := range map[string]string{
		"attribute":        `<div title='<wb:topic name="literal"></wb:topic><wb:floor id="999"/>'>junk</div>`,
		"comment":          `<!-- <wb:topic name="literal"></wb:topic><wb:floor id="999"/> -->`,
		"cdata":            `<![CDATA[<wb:topic name="literal"></wb:topic><wb:floor id="999"/>]]>`,
		"double-attribute": `<div title="<wb:topic name='literal'></wb:topic><wb:floor id='999'/>">junk</div>`,
		"raw-body":         `junk<wb:log><wb:topic name="literal"></wb:topic><wb:floor id="999"/></wb:log>`,
		"unknown-body":     `junk<wb:unknown><wb:topic name="literal"></wb:topic><wb:floor id="999"/></wb:unknown>`,
		"name-prefix":      `junk<wb:topic-suffix/>`,
	} {
		t.Run("literal-"+name, func(t *testing.T) {
			s := &State{}
			accepted := topicXML("prefix", "valid prefix")
			mixed(t, s, accepted+`<wb:topic name="outer">`+rejected+`</wb:topic>`, accepted)
		})
	}
	s := &State{}
	mustApply(t, s, `<wb:topic name="t"><wb:log><![CDATA[<wb:topic name="literal"></wb:topic>]]></wb:log><wb:panel recap='<wb:topic'>raw <wb:topic name="literal"></wb:topic></wb:panel></wb:topic>`)
}

func TestCommandAcceptanceDuplicateTargetCloseGuard(t *testing.T) {
	for name, tc := range map[string]struct {
		attrs  string
		blockT bool
		blockU bool
	}{
		"answer-id":      {`id="999" id="1"`, true, false},
		"answer-id-many": {`id="999" id="1" id="2"`, true, true},
		"answer-ids":     {`ids="999" ids="1 2"`, true, true},
		"answer-mixed":   {`id="999" ids="998" id="1" ids="bad 2 997"`, true, true},
		"patch-id":       {`id="999" id="1"`, true, false},
		"patch-id-many":  {`id="999" id="1" id="2"`, true, true},
		"patch-ids":      {`id="999" ids="998" ids="bad 1 2 997"`, true, true},
		"unknown-only":   {`id="999" id="998" ids="997"`, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			s := &State{}
			mustApply(t, s, addXML(ClassPolicy, "t", "Pick", "One", "Two")+addXML(ClassPolicy, "u", "Pick", "One", "Two")+addXML(ClassPolicy, "other", "Pick", "One", "Two"))
			bad := `<wb:answer ` + tc.attrs + ` value="a">quote</wb:answer>`
			if strings.HasPrefix(name, "patch") {
				bad = `<wb:patch ` + tc.attrs + ` status="answered"/>`
			}
			closeT := `<wb:topic name="t"><wb:close result="done"/></wb:topic>`
			closeU := `<wb:topic name="u"><wb:close result="done"/></wb:topic>`
			closeOther := `<wb:topic name="other"><wb:close result="allowed"/></wb:topic>`
			accepted := closeOther
			if !tc.blockT {
				accepted += closeT
			}
			if !tc.blockU {
				accepted += closeU
			}
			p := mixed(t, s, bad+closeOther+closeT+closeU, accepted)
			if !strings.Contains(strings.Join(p, " "), "given twice") {
				t.Fatal("fixture did not reject duplicate attributes", p)
			}
		})
	}
}

func TestPolicyCommentLifecycleBookkeepingAndViews(t *testing.T) {
	s := &State{}
	mustApply(t, s, addXML(ClassPolicy, "t", "Pick", "One", "Two"))
	comment := `<wb:answer id="1" value="" assumed="true">  note &amp; <b>literal</b>  </wb:answer>`
	out := mustApply(t, s, comment)
	it := item(t, s, 1)
	if it.Status != StatusCommented || it.Answer != "" || it.AnswerQuote != "  note &amp; <b>literal</b>  " || it.AnsweredAt == "" || it.Changed != s.Seq || it.ChangedAt == "" || it.Settled != 1 || s.AnswerSeq != s.Seq || s.OpenAskCount() != 0 || IsSettled(it.Status) {
		t.Fatalf("comment mapping: %+v %+v", it, s)
	}
	if strings.Contains(fmt.Sprint(out.Summaries), "answered") || !strings.Contains(fmt.Sprint(out.Summaries), "commented 1") {
		t.Fatal(out.Summaries)
	}
	e := s.Topics[0].Entries[len(s.Topics[0].Entries)-1]
	if !strings.Contains(e.Text, "user commented") || strings.Contains(e.Text, "assumed") || !(EntryView{Entry: e}).OwnerAnswer() {
		t.Fatal(e)
	}
	before := s.clone()
	env := testEnv()
	env.Now = testNow.Add(time.Hour)
	repeated, p := Apply(s, comment, env)
	if len(p) > 0 || len(repeated.Effects) > 0 || s.EntrySeq != before.EntrySeq || s.SettleSeq != before.SettleSeq || s.AnswerSeq != before.AnswerSeq || !reflect.DeepEqual(s.Items, before.Items) {
		t.Fatal("identical comment is not a no-op", p)
	}
	s.AnswerSeq = 0
	if s.LastAnswerSeq() != it.Changed {
		t.Fatal("legacy fallback omitted comment")
	}
	s.AnswerSeq = before.AnswerSeq
	for _, layout := range []Layout{LayoutFull, LayoutEmbed} {
		view := BuildView(s, ViewInput{Layout: layout})
		html, err := RenderPage(view)
		if err != nil || !strings.Contains(html, "user commented") || !strings.Contains(html, "literal") {
			t.Fatalf("comment hidden in %v: %v", layout, err)
		}
		if iv := itemView(it); iv.AnswerLetter != "" || iv.Options[0].Picked || iv.Options[1].Picked {
			t.Fatal("comment chose an option")
		}
	}
	q, p := Query(s, queryPayload(t, `{"status":["commented"],"depth":"summary"}`))
	if len(p) > 0 || !strings.Contains(FormatQuery(q), "user commented, lead's move") || !strings.Contains(FormatQuery(q), "commented 1") {
		t.Fatal(FormatQuery(q), p)
	}
	q, p = Query(s, queryPayload(t, `{"ids":[1],"depth":"full"}`))
	if len(p) > 0 || !strings.Contains(FormatQuery(q), "answer_quote:") || !strings.Contains(FormatQuery(q), "answered_at:") || !strings.Contains(FormatQuery(q), "One") {
		t.Fatal(FormatQuery(q), p)
	}
	mustApply(t, s, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	if item(t, s, 1).Status != StatusCommented {
		t.Fatal("topic close withdrew comment")
	}
	mustApply(t, s, `<wb:patch id="1" status="open"/>`)
	it = item(t, s, 1)
	if it.Status != StatusOpen || it.AnswerQuote != "" || it.AnsweredAt != "" || it.Settled != 0 || it.Asked != s.Seq || it.ReaskedAt == "" {
		t.Fatal("reask failed", it)
	}
	mustApply(t, s, `<wb:answer id="1" value="b">Two</wb:answer>`)
	if item(t, s, 1).Status != StatusAnswered {
		t.Fatal("letter did not replace comment")
	}
	for _, status := range []string{StatusWithdrawn, StatusSuperseded} {
		mustApply(t, s, `<wb:answer id="1" value="">history note</wb:answer>`)
		mustApply(t, s, `<wb:patch id="1" status="`+status+`"/>`)
		if item(t, s, 1).AnswerQuote != "history note" {
			t.Fatal("withdrawal lost note")
		}
		mustApply(t, s, `<wb:patch id="1" status="open"/>`)
	}
	mustApply(t, s, comment)
	for n := 0; n < ArchiveCap+1; n++ {
		s.Counter++
		s.SettleSeq++
		s.Items = append(s.Items, Item{ID: s.Counter, Topic: "archive", Status: StatusAnswered, Settled: s.SettleSeq})
	}
	s.evictArchive()
	if item(t, s, 1).Status != StatusCommented {
		t.Fatal("archive evicted comment")
	}
}

func TestPolicyCommentInvalidAndMultiIDRollback(t *testing.T) {
	s := &State{}
	mustApply(t, s, addXML(ClassPolicy, "t", "Pick", "One", "Two")+addXML(ClassTechnical, "other", "Not a policy"))
	for _, cmd := range []string{`<wb:answer id="1">note</wb:answer>`, `<wb:answer id="1" value=""/>`, `<wb:answer id="1" value="">  </wb:answer>`, `<wb:answer id="1" value=" ">note</wb:answer>`, `<wb:answer id="2" value="">note</wb:answer>`, `<wb:answer id="1" value="z">note</wb:answer>`, `<wb:answer id="1" value="" assumed="yes">note</wb:answer>`, `<wb:answer ids="1 2" value="">note</wb:answer>`} {
		refused(t, s, cmd)
	}
}
