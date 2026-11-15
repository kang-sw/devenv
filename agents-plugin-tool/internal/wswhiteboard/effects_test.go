package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func wantEffect(t *testing.T, effects []Effect, kind, field, before, after string) Effect {
	t.Helper()
	for _, e := range effects {
		if e.Kind == kind && e.Field == field && e.Before == before && e.After == after {
			return e
		}
	}
	t.Fatalf("missing %s/%s %q -> %q in %+v", kind, field, before, after, effects)
	return Effect{}
}

func TestSemanticEffectsResolvedIdentitiesAndCommandOrder(t *testing.T) {
	s := &State{}
	out := mustApply(t, s, `<wb:floor id="40"/>
<wb:topic name="design" label="Design" tickets="261115-feat-example">
<wb:add class="policy"><wb:title>Choose</wb:title><wb:option>First</wb:option><wb:option>Second</wb:option><wb:recommend>a</wb:recommend><wb:why>Reason</wb:why><wb:link>docs/a.md</wb:link></wb:add>
<wb:body>old body</wb:body><wb:edit><wb:old>old</wb:old><wb:new>new</wb:new></wb:edit>
<wb:panel recap="first">payload one</wb:panel><wb:panel id="p1" recap="second">payload two</wb:panel><wb:log>Ready</wb:log>
</wb:topic>
<wb:answer id="41" value="b">Second, please.</wb:answer>
<wb:topic name="other"><wb:add id="60" class="check"><wb:title>Check</wb:title></wb:add></wb:topic>`)
	if !slices.Equal(out.IDs, []int{41, 60}) {
		t.Fatalf("resolved IDs: %+v", out.IDs)
	}
	for _, e := range out.Effects {
		if !slices.Contains(EffectFields(e.Kind), e.Field) {
			t.Fatalf("unknown effect: %+v", e)
		}
	}
	wantEffect(t, out.Effects, "setting", "id-floor", "0", "40")
	wantEffect(t, out.Effects, "topic", "label", "design", "Design")
	wantEffect(t, out.Effects, "topic", "tickets", "", `["261115-feat-example"]`)
	for field, value := range map[string]string{"class": "policy", "title": "Choose", "options": `["First","Second"]`, "recommendation": "a", "rationale": "Reason", "links": `["docs/a.md"]`} {
		e := wantEffect(t, out.Effects, "item", field, "", value)
		if e.ID != 41 || e.Topic != "design" {
			t.Fatalf("attribution: %+v", e)
		}
	}
	wantEffect(t, out.Effects, "topic", "body", "", "old body")
	wantEffect(t, out.Effects, "topic", "body", "old body", "new body")
	first := wantEffect(t, out.Effects, "panel", "payload", "", "payload one")
	second := wantEffect(t, out.Effects, "panel", "payload", "", "payload two")
	if first.PanelID != "p1" || second.PanelID != "p1" || first.EntryID == second.EntryID || first.EntryID == "" {
		t.Fatalf("panel versions: %+v %+v", first, second)
	}
	wantEffect(t, out.Effects, "panel", "recap", "", "second")
	log := wantEffect(t, out.Effects, "log", "text", "", "Ready")
	if log.EntryID == "" || log.Topic != "design" {
		t.Fatalf("log: %+v", log)
	}
	wantEffect(t, out.Effects, "item", "status", StatusOpen, StatusAnswered)
	wantEffect(t, out.Effects, "item", "answer-quote", "", "Second, please.")
	// All timestamps are identical, yet typed IDs and command order remain exact.
	var bodies []string
	for _, e := range out.Effects {
		if e.Kind == "topic" && e.Field == "body" {
			bodies = append(bodies, e.After)
		}
	}
	if !slices.Equal(bodies, []string{"old body", "new body"}) {
		t.Fatalf("body order: %v", bodies)
	}
}

func TestSemanticEffectsImplicitWithdrawalReopenAdoptionAndEviction(t *testing.T) {
	s := &State{}
	mustApply(t, s, `<wb:topic name="Old"><wb:add class="check"><wb:title>Confirm</wb:title></wb:add><wb:body>Content</wb:body><wb:panel recap="keep">Panel</wb:panel></wb:topic>`)
	out := mustApply(t, s, `<wb:topic name="Old"><wb:close result="Finished"/></wb:topic>`)
	wantEffect(t, out.Effects, "item", "status", StatusOpen, StatusWithdrawn)
	wantEffect(t, out.Effects, "topic", "closed", "", "closed")
	wantEffect(t, out.Effects, "topic", "result", "", "Finished")
	out = mustApply(t, s, `<wb:topic name="new" label="Old"><wb:log>Reopened</wb:log></wb:topic>`)
	wantEffect(t, out.Effects, "item", "topic", "Old", "new")
	wantEffect(t, out.Effects, "topic", "closed", "closed", "")
	wantEffect(t, out.Effects, "log", "text", "", "Reopened")
	for _, e := range out.Effects {
		if e.Kind == "panel" {
			t.Fatalf("adoption re-emitted an old panel: %+v", e)
		}
	}
	out = mustApply(t, s, `<wb:patch id="1" status="open" class="technical" clear="links"><wb:title>Again</wb:title><wb:why>New reason</wb:why></wb:patch>`)
	wantEffect(t, out.Effects, "item", "status", StatusWithdrawn, StatusOpen)
	wantEffect(t, out.Effects, "item", "class", ClassCheck, ClassTechnical)
	wantEffect(t, out.Effects, "item", "title", "Confirm", "Again")
	out = mustApply(t, s, `<wb:answer id="1" value="proceed" assumed="true">I think proceed.</wb:answer>`)
	wantEffect(t, out.Effects, "item", "status", StatusOpen, StatusAssumed)
	out = mustApply(t, s, `<wb:patch id="1" status="open"/>`)
	wantEffect(t, out.Effects, "item", "answer", "proceed", "")
	wantEffect(t, out.Effects, "item", "answer-quote", "I think proceed.", "")

	archive := &State{Counter: ArchiveCap + 1, SettleSeq: ArchiveCap + 1}
	for i := 1; i <= ArchiveCap+1; i++ {
		archive.Items = append(archive.Items, Item{ID: i, Topic: "archive", Class: ClassCheck, Status: StatusAnswered, Settled: i})
	}
	out = mustApply(t, archive, ``)
	if len(out.Effects) != 0 || archive.index(1) >= 0 || len(archive.Items) != ArchiveCap {
		t.Fatalf("archive maintenance emitted command effects or stopped evicting: %+v, items %d", out.Effects, len(archive.Items))
	}
}

func TestSemanticEffectsNoopsRefusalAndSettings(t *testing.T) {
	s := &State{}
	mustApply(t, s, `<wb:topic name="same"><wb:add class="check"><wb:title>Check</wb:title></wb:add><wb:body>Same</wb:body></wb:topic><wb:answer id="1" value="ok">Yes.</wb:answer>`)
	for _, commands := range []string{``, `<wb:topic name="same"><wb:body>Same</wb:body></wb:topic>`, `<wb:patch id="1"><wb:title>Check</wb:title></wb:patch>`, `<wb:answer id="1" value="ok">Yes.</wb:answer>`, `<wb:floor id="1"/>`, `<wb:topic name="unused" label="Unused"><wb:body/></wb:topic>`} {
		out := mustApply(t, s, commands)
		if len(out.Effects) != 0 {
			t.Fatalf("semantic noop %q: %+v", commands, out.Effects)
		}
	}
	before := s.clone()
	out, problems := Apply(s, `<wb:topic name="same"><wb:log>This must not commit</wb:log></wb:topic><wb:patch id="999"/>`, testEnv())
	if len(problems) == 0 || len(out.Effects) != 0 || !reflect.DeepEqual(before, s) {
		t.Fatalf("refusal leaked effects/state: %+v %v", out, problems)
	}
	env := testEnv()
	env.IsDir = func(string) bool { return true }
	env.IsAbs = func(string) bool { return true }
	out, problems = Apply(s, `<wb:embed-dir path="/external"/>`, env)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	wantEffect(t, out.Effects, "setting", "embed-dir", "", "/external")
	out, problems = Apply(s, `<wb:embed-dir path=""/>`, env)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	wantEffect(t, out.Effects, "setting", "embed-dir", "/external", "")
	// Effects are not a persisted State field or a new store shape.
	raw, _ := json.Marshal(s)
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(raw, &keys)
	if _, exists := keys["effects"]; exists {
		t.Fatalf("effects persisted: %s", raw)
	}
}

func TestSemanticEffectsFreshLabelWithoutTicketsPersistsWithFirstContent(t *testing.T) {
	for _, tc := range []struct {
		name, command string
	}{
		{"body", `<wb:body>First content</wb:body>`},
		{"item", `<wb:add class="check"><wb:title>First item</wb:title></wb:add>`},
		{"log", `<wb:log>First log</wb:log>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &State{}
			out := mustApply(t, s, `<wb:topic name="fresh" label="Fresh topic">`+tc.command+`</wb:topic>`)
			var labels []Effect
			for _, e := range out.Effects {
				if e.Kind == "topic" && e.Field == "label" {
					labels = append(labels, e)
				}
				if e.Kind == "topic" && e.Field == "tickets" {
					t.Fatalf("fresh topic unexpectedly persisted through tickets: %+v", e)
				}
			}
			want := []Effect{{Kind: "topic", Topic: "fresh", Field: "label", Before: "fresh", After: "Fresh topic"}}
			if !reflect.DeepEqual(labels, want) {
				t.Fatalf("first content lost or duplicated the provisional label: got %+v, want %+v", labels, want)
			}
			if e := wantEffect(t, out.Effects, "topic", "presence", "", "present"); e.Topic != "fresh" {
				t.Fatalf("first content persisted the wrong topic: %+v", e)
			}
			if !s.topicKnown("fresh") || s.TopicLabel("fresh") != "Fresh topic" {
				t.Fatalf("first content did not persist its labeled topic: %+v", s)
			}
		})
	}
}

func TestSemanticEffectsConsecutiveGroupMetadataReversions(t *testing.T) {
	s := &State{}
	mustApply(t, s, `<wb:topic name="t" label="Original" tickets="261115-feat-first"><wb:body>Keep</wb:body></wb:topic>`)
	out := mustApply(t, s, `<wb:topic name="t" label="Interim"/><wb:topic name="t" label="Original"/><wb:topic name="t" tickets="261115-feat-second"/><wb:topic name="t" tickets="261115-feat-first"/>`)
	want := []Effect{
		{Kind: "topic", Topic: "t", Field: "label", Before: "Original", After: "Interim"},
		{Kind: "topic", Topic: "t", Field: "label", Before: "Interim", After: "Original"},
		{Kind: "topic", Topic: "t", Field: "tickets", Before: `["261115-feat-first"]`, After: `["261115-feat-second"]`},
		{Kind: "topic", Topic: "t", Field: "tickets", Before: `["261115-feat-second"]`, After: `["261115-feat-first"]`},
	}
	if !reflect.DeepEqual(out.Effects, want) {
		t.Fatalf("group transitions collapsed: got %+v, want %+v", out.Effects, want)
	}
	if got := mustApply(t, s, `<wb:topic name="t" label="Original" tickets="261115-feat-first"/>`); len(got.Effects) != 0 {
		t.Fatalf("identical group metadata produced effects: %+v", got.Effects)
	}
}

func TestSemanticEffectsArchiveMaintenanceKeepsCommandEffectsAndPersistence(t *testing.T) {
	seed := &State{Counter: ArchiveCap + 2, SettleSeq: ArchiveCap}
	for id := 1; id <= ArchiveCap; id++ {
		seed.Items = append(seed.Items, Item{ID: id, Topic: "archive", Class: ClassTechnical, Title: "old", Status: StatusAnswered, Settled: id})
	}
	for _, id := range []int{ArchiveCap + 1, ArchiveCap + 2} {
		seed.Items = append(seed.Items, Item{ID: id, Topic: "current", Class: ClassTechnical, Title: "answer me", Status: StatusOpen})
	}
	store := FolderStore{Dir: t.TempDir()}
	var out Outcome
	if err := store.Update(seed, func(work *State) error {
		out = mustApply(t, work, fmt.Sprintf(`<wb:answer id="%d" value="ok">First.</wb:answer><wb:answer id="%d" value="ok">Second.</wb:answer>`, ArchiveCap+1, ArchiveCap+2))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, e := range out.Effects {
		if e.Kind == "item" {
			if e.ID != ArchiveCap+1 && e.ID != ArchiveCap+2 {
				t.Fatalf("untouched archive item entered command effects: %+v", e)
			}
			seen[e.ID] = true
		}
	}
	if len(seen) != 2 {
		t.Fatalf("lost target effects: %+v", out.Effects)
	}
	wantEffect(t, out.Effects, "item", "status", StatusOpen, StatusAnswered)
	wantEffect(t, out.Effects, "item", "answer-quote", "", "First.")
	wantEffect(t, out.Effects, "item", "answer-quote", "", "Second.")
	reloaded := folderOpen(t, store.Dir, nil)
	if len(reloaded.Items) != ArchiveCap || reloaded.index(1) >= 0 || reloaded.index(2) >= 0 || reloaded.index(3) < 0 || reloaded.Counter != ArchiveCap+2 {
		t.Fatalf("archive cap/order/counter changed after reload: %+v", reloaded.Items)
	}
	for _, id := range []int{ArchiveCap + 1, ArchiveCap + 2} {
		if got := item(t, reloaded, id); got.Status != StatusAnswered || got.Answer != "ok" || got.Settled <= ArchiveCap {
			t.Fatalf("newly settled target lost after reload: %+v", got)
		}
	}
}

func TestSemanticEffectsEveryPatchFieldAndTopicClear(t *testing.T) {
	s := &State{}
	mustApply(t, s, addXML(ClassPolicy, "choice", "Old", "A", "B"))
	out := mustApply(t, s, `<wb:patch id="1"><wb:title>New</wb:title><wb:option>C</wb:option><wb:option>D</wb:option><wb:recommend>b</wb:recommend><wb:why>Why</wb:why><wb:link>docs/new.md</wb:link></wb:patch>`)
	for field, pair := range map[string][2]string{"title": {"Old", "New"}, "options": {`["A","B"]`, `["C","D"]`}, "recommendation": {"", "b"}, "rationale": {"", "Why"}, "links": {"", `["docs/new.md"]`}} {
		wantEffect(t, out.Effects, "item", field, pair[0], pair[1])
	}
	mustApply(t, s, `<wb:topic name="choice" tickets="261115-feat-example"><wb:body>Content</wb:body></wb:topic>`)
	out = mustApply(t, s, `<wb:topic name="choice" tickets=""><wb:body></wb:body></wb:topic><wb:patch id="1" clear="options links"><wb:recommend></wb:recommend></wb:patch>`)
	wantEffect(t, out.Effects, "topic", "body", "Content", "")
	wantEffect(t, out.Effects, "topic", "tickets", `["261115-feat-example"]`, "")
	wantEffect(t, out.Effects, "item", "options", `["C","D"]`, "")
	wantEffect(t, out.Effects, "item", "links", `["docs/new.md"]`, "")
}
