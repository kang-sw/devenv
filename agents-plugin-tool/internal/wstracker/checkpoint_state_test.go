package wstracker

import (
	"reflect"
	"testing"
	"time"
)

func checkpointRichState() *state {
	n := &node{id: "a.pc#1", member: "a", origin: OriginHuman}
	due, start := "2026-12-01", "2026-11-01"
	r := &rec{id: "r", item: "i", to: "a", state: "live", accepted: n, ended: n,
		restored: n, last: n, start: n, writes: []*node{n}}
	ru := &rule{id: "rule", item: "i", excluded: map[string]bool{"b": true}}
	return &state{
		hb: func(x, y *node) bool { return x == n && y == n }, marks: true,
		members: map[string]*member{"a": {id: "a", profile: Profile{Org: "org", Family: "Family", Given: "Given"},
			keys: []memberKey{{"key", "cmp"}}, revoked: []revokedKey{{memberKey{"old", "old"}, n}},
			leads: []string{"b"}, removed: n, joined: n}},
		morder: []string{"a"}, admins: map[string]bool{"a": true}, aorder: []string{"a"},
		items: map[string]*item{"i": {id: "i", due: &due, start: &start, placedBy: n, parentBy: n,
			edited: n, node: n, mentionsAt: n, mentions: []string{"a"}, refs: []string{"ref"},
			attachments: []attachment{{oid: "oid", name: "file", size: 42}}, labels: labelSet{"l"},
			open:       &opening{node: n, by: "a", scope: ScopeStore, ended: n},
			ccExcluded: map[string]bool{"b": true},
			cc:         []ccEntry{{member: "a", since: n, personal: true, orgs: map[string]bool{"o": true}}}}},
		iorder: []string{"i"}, place: []placement{{id: "i", afterSet: true}},
		asg: map[string]*rec{"r": r}, recs: []*rec{r}, byItem: map[string][]*rec{"i": {r}},
		rules: []*rule{ru}, ruleByID: map[string]*rule{"rule": ru},
		comments: map[string]*comment{"c": {id: "c", hiddenBy: n, edited: n, mentions: []string{"a"},
			attachments: []attachment{{oid: "comment-oid"}}, refs: []string{"comment-ref"}}},
		corder: []string{"c"}, reqs: map[string]*takeReq{"q": {id: "q", node: n, ended: n}}, rorder: []string{"q"},
		reg: map[string][]head{
			"nil": {{n, nil}}, "string": {{n, "text"}}, "bool": {{n, true}}, "int": {{n, 2}},
			"float": {{n, float64(42)}}, "cc": {{n, ccVal(true)}}, "cc-org": {{n, ccOrgVal(true)}},
			"status": {{n, statusVal{"doing", "note", "a", n}}}, "place": {{n, placeVal{parent: "i"}}},
			"leads": {{n, []string{"a"}}}, "labels": {{n, labelSet{"l"}}},
			"priority": {{n, priority{names: []string{"Low", "High"}, def: 1}}},
		},
		bodies: map[string]*bodyFold{"i": {writes: []*node{n},
			val:  bodyVal{lines: []bline{{"text", n}}, atts: []attachment{{oid: "body-oid"}}},
			base: map[string]bodyVal{"base": {lines: []bline{{"base", n}}, atts: []attachment{{oid: "base-oid"}}}}}},
		labels: map[string]*label{"l": {id: "l", name: "Label", removed: n}}, lorder: []string{"l"},
		orgs: map[string]*org{"o": {id: "o", name: "Org", head: "a", removed: n}}, oorder: []string{"o"},
		ccOrgs: map[string]*ccOrgReg{"i:o": {item: "i", org: "o", active: true, since: n,
			added: map[string]bool{"a": true}, excluded: map[string]bool{"b": true}}}, ccOrder: []string{"i:o"},
		prio: priority{names: []string{"Low", "High"}, def: 1}, attMax: 1234,
	}
}

func checkpointEqual(t *testing.T, a, b *state) {
	t.Helper()
	x, y := *a, *b
	x.hb, y.hb = nil, nil // Non-nil functions are never reflect.DeepEqual.
	if !reflect.DeepEqual(x, y) {
		t.Fatal("checkpoint state differs")
	}
}

// Inspect all nested containers, including unexported fields and interface
// values. A newly added mutable field is covered without extending a list of
// field names. Only replay-owned nodes and immutable time values may alias.
func checkpointDisjoint(t *testing.T, a, b reflect.Value, path string) {
	t.Helper()
	if a.Type() == reflect.TypeOf((*node)(nil)) {
		if a.Pointer() != b.Pointer() {
			t.Fatalf("%s: event identity changed", path)
		}
		return
	}
	if a.Type() == reflect.TypeOf(time.Time{}) {
		return
	}
	switch a.Kind() {
	case reflect.Interface:
		if !a.IsNil() {
			checkpointDisjoint(t, a.Elem(), b.Elem(), path)
		}
	case reflect.Pointer:
		if !a.IsNil() {
			if a.Pointer() == b.Pointer() {
				t.Fatalf("%s: borrowed pointer", path)
			}
			checkpointDisjoint(t, a.Elem(), b.Elem(), path)
		}
	case reflect.Map:
		if !a.IsNil() {
			if a.UnsafePointer() == b.UnsafePointer() {
				t.Fatalf("%s: borrowed map", path)
			}
			for _, k := range a.MapKeys() {
				checkpointDisjoint(t, a.MapIndex(k), b.MapIndex(k), path+"["+k.String()+"]")
			}
		}
	case reflect.Slice:
		if a.Len() > 0 && a.Pointer() == b.Pointer() {
			t.Fatalf("%s: borrowed slice", path)
		}
		for i := 0; i < a.Len(); i++ {
			checkpointDisjoint(t, a.Index(i), b.Index(i), path+"[]")
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			checkpointDisjoint(t, a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name)
		}
	}
}

func TestStateCheckpointOwnership(t *testing.T) {
	s := checkpointRichState()
	cp := s.checkpoint()
	checkpointEqual(t, s, cp)
	checkpointDisjoint(t, reflect.ValueOf(s), reflect.ValueOf(cp), "state")
	if cp.asg["r"] != cp.recs[0] || cp.asg["r"] != cp.byItem["i"][0] {
		t.Fatal("record indexes lost shared identity")
	}
	if cp.rules[0] != cp.ruleByID["rule"] {
		t.Fatal("rule indexes lost shared identity")
	}
	n := s.items["i"].node
	if !cp.hb(n, n) || cp.hb(nil, n) {
		t.Fatal("causal closure was not retained")
	}
}

func TestStateCheckpointMutationIsolation(t *testing.T) {
	mutations := map[string]func(*state){
		"member": func(s *state) {
			m := s.members["a"]
			m.profile.Given = "changed"
			m.keys[0].text = "changed"
			m.revoked[0].key.text = "changed"
			m.leads[0] = "changed"
		},
		"item": func(s *state) {
			it := s.items["i"]
			*it.due, *it.start = "changed", "changed"
			it.open.by = "changed"
			it.mentions[0], it.refs[0], it.labels[0], it.cc[0].member = "changed", "changed", "changed", "changed"
			it.ccExcluded["b"] = false
			it.cc[0].orgs["o"] = false
			it.attachments[0].name = "changed"
		},
		"record": func(s *state) { s.recs[0].status = "changed"; s.byItem["i"][0].writes[0] = nil },
		"rule":   func(s *state) { s.rules[0].state = "changed"; s.ruleByID["rule"].excluded["b"] = false },
		"comment": func(s *state) {
			c := s.comments["c"]
			c.body, c.mentions[0], c.refs[0], c.attachments[0].oid = "changed", "changed", "changed", "changed"
		},
		"request": func(s *state) { s.reqs["q"].state = "changed" },
		"org state": func(s *state) {
			s.orgs["o"].name = "changed"
			s.ccOrgs["i:o"].added["a"] = false
			s.ccOrgs["i:o"].excluded["b"] = false
		},
		"register": func(s *state) {
			s.reg["leads"][0].v.([]string)[0] = "changed"
			s.reg["labels"][0].v.(labelSet)[0] = "changed"
			s.reg["priority"][0].v.(priority).names[0] = "changed"
			s.reg["string"][0].v = "changed"
		},
		"body": func(s *state) {
			f := s.bodies["i"]
			f.writes[0] = nil
			f.val.lines[0].text, f.val.atts[0].oid = "changed", "changed"
			b := f.base["base"]
			b.lines[0].text, b.atts[0].oid = "changed", "changed"
			f.base["new"] = bodyVal{}
		},
		"orders and settings": func(s *state) {
			s.morder[0], s.aorder[0], s.iorder[0], s.corder[0], s.rorder[0], s.lorder[0] = "x", "x", "x", "x", "x", "x"
			s.oorder[0], s.ccOrder[0] = "x", "x"
			s.place[0].id, s.prio.names[0], s.labels["l"].name = "x", "x", "x"
			s.admins["a"] = false
		},
		"map membership": func(s *state) {
			delete(s.members, "a")
			delete(s.items, "i")
			delete(s.asg, "r")
			delete(s.byItem, "i")
			delete(s.ruleByID, "rule")
			delete(s.comments, "c")
			delete(s.reqs, "q")
			delete(s.reg, "nil")
			delete(s.bodies, "i")
			delete(s.labels, "l")
			delete(s.orgs, "o")
			delete(s.ccOrgs, "i:o")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			for _, mutateOriginal := range []bool{false, true} {
				s := checkpointRichState()
				cp := s.checkpoint()
				if mutateOriginal {
					mutate(s)
					checkpointEqual(t, cp, checkpointRichState())
				} else {
					mutate(cp)
					checkpointEqual(t, s, checkpointRichState())
				}
			}
		})
	}
}

func TestStateCheckpointEmpty(t *testing.T) {
	for _, s := range []*state{{}, newState(&genesis{}, nil, false)} {
		cp := s.checkpoint()
		checkpointEqual(t, s, cp)
		checkpointDisjoint(t, reflect.ValueOf(s), reflect.ValueOf(cp), "state")
	}
}

func TestStateCheckpointFoldSuffix(t *testing.T) {
	g := &genesis{members: []*member{{id: "a"}, {id: "b"}, {id: "c"}}, admins: []string{"a"}, prio: defaultPriority}
	// The two body edits are concurrent, both based on create. Folding the
	// second populates a nested base cache, which later checkpoints must own.
	events := []*node{
		{id: "a.pc#1", member: "a", typ: TypeItemCreate, p: obj{"kind": "task", "title": "Doc", "body": "one\ntwo\nthree"}},
		{id: "b.pc#1", member: "b", typ: TypeItemEdit, p: obj{"item": "a.pc#1", "body": "ONE\ntwo\nthree", "base": []any{"a.pc#1"}}},
		{id: "c.pc#1", member: "c", typ: TypeItemEdit, p: obj{"item": "a.pc#1", "body": "one\ntwo\nTHREE", "base": []any{"a.pc#1"}}},
		{id: "a.pc#2", member: "a", typ: TypeItemOpen, p: obj{"item": "a.pc#1", "scope": ScopeStore}},
		{id: "b.pc#2", member: "b", typ: TypeItemClaim, p: obj{"item": "a.pc#1"}},
		{id: "b.pc#3", member: "b", typ: TypeItemStatus, p: obj{"item": "a.pc#1", "status": "doing", "note": "working"}},
		{id: "c.pc#2", member: "c", typ: TypeItemRequest, p: obj{"item": "a.pc#1", "assignee": "b"}},
		{id: "b.pc#4", member: "b", typ: TypeItemApprove, p: obj{"item": "a.pc#1", "request": "c.pc#2"}},
		{id: "c.pc#3", member: "c", typ: TypeCommentAdd, p: obj{"item": "a.pc#1", "body": "comment", "mentions": []any{"b"}}},
		{id: "c.pc#4", member: "c", typ: TypeCommentEdit, p: obj{"comment": "c.pc#3", "body": "edited"}},
	}
	for i, n := range events {
		n.pos, n.origin, n.ts = i, OriginHuman, time.Unix(int64(i), 0).UTC()
	}
	hb := func(x, y *node) bool {
		return x.pos < y.pos && (x.pos == 0 || x.member == y.member)
	}
	fold := func(s *state, ns []*node) {
		for _, n := range ns {
			s.apply(n)
		}
	}
	want := newState(g, hb, false)
	fold(want, events)
	if len(want.bodies["a.pc#1"].base) == 0 || want.reqs["c.pc#2"].state != "approved" {
		t.Fatal("suffix fixture did not exercise body cache and request transitions")
	}
	for cut := 0; cut <= len(events); cut++ {
		prefix := newState(g, hb, false)
		fold(prefix, events[:cut])
		saved := prefix.checkpoint()
		resume := saved.checkpoint()
		fold(resume, events[cut:])
		checkpointEqual(t, resume, want)
		checkpointEqual(t, prefix, saved)
		// Continuing the source also cannot mutate the saved checkpoint.
		fold(prefix, events[cut:])
		independent := newState(g, hb, false)
		fold(independent, events[:cut])
		checkpointEqual(t, saved, independent)
	}
}
