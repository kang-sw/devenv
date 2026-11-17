package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

func projectionString(s string) *string { return &s }

func projectionFixture() *wstrackerhost.StoreView {
	const oid = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	att := []wstracker.AttachmentView{{OID: oid, Name: "secret-attachment.png", Mime: "image/png", Size: 42}}
	return &wstrackerhost.StoreView{
		Store: wstrackerhost.StoreInfo{Name: "store", Remote: projectionString("https://user:password@example.test/repo?token=secret")},
		Sync:  wstrackerhost.SyncInfo{Error: "https://user:password@example.test/?token=secret", WikiWarnings: []string{"https://user:password@example.test/?token=secret"}},
		View: &wstracker.View{
			Viewer: wstracker.Viewer{Member: "me", Stream: "me.pc"}, Now: "2026-11-18T07:18:00Z",
			Items: map[string]wstracker.ItemView{
				"i": {ID: "i", Alias: "#1", Kind: "task", Title: "Summary title", Author: "other", TS: "2026-11-17T01:00:00Z", Status: projectionString("todo"),
					Body: "**body-secret** <script>alert(1)</script> ![own](attachment:" + oid + ") ![remote](https://example.test/x.png)", BodyRev: []string{"body-revision-secret"},
					Attachments: att, Mentions: []string{"me"}, Thread: []string{"e", "c"}, Due: projectionString("2026-11-17")},
				"other-item": {ID: "other-item", Kind: "topic", Title: "Other", Body: "not-requested-secret"},
			},
			ItemOrder: []string{"i", "other-item"},
			Comments: map[string]wstracker.CommentView{
				"mine": {ID: "mine", Item: "i", Author: "me", Body: "my text"},
				"c":    {ID: "c", Item: "i", Author: "other", Body: "**comment-secret**", ReplyTo: projectionString("mine"), TS: "2026-11-18T01:00:00Z", Mentions: []string{"me"}, Attachments: att},
			},
			Events: []wstracker.EventView{
				{ID: "e", Type: "item.status", Status: "applied", Member: "other", TS: "2026-11-18T00:00:00Z", Thread: projectionString("i"), Payload: json.RawMessage(`{"item":"i","note":"event-payload-secret","status":"done"}`), Reason: projectionString("verdict-secret")},
				{ID: "c", Type: "comment.add", Status: "applied", Payload: json.RawMessage(`{"item":"i","body":"event-comment-secret"}`)},
			},
			Assignments: map[string]wstracker.Assignment{
				"a": {ID: "a", Item: "i", To: "me", By: "other", Sender: "other", Via: "e", State: "live", Forced: true, Status: "todo", TS: "2026-11-17T01:00:00Z", LastUpdate: "2026-11-18T00:00:00Z"},
			},
			Requests: map[string]wstracker.RequestView{"q": {ID: "q", Item: "i", By: "other", Assignee: "me", Note: projectionString("request note"), TS: "2026-11-18T01:00:00Z"}},
			Rules:    map[string]wstracker.RuleView{"rule": {ID: "rule", Item: "i", State: "live"}},
			Notices:  []wstracker.Notice{{Key: "w:a:done:now", Kind: "done", Assignment: "a", Item: "i", To: "me", TS: "2026-11-18T00:00:00Z"}},
			My: wstracker.My{Assignments: []string{"a"}, Received: wstracker.Received{Pending: []string{"a"}},
				Today: wstracker.Today{Confirms: []string{"a"}}, Requests: wstracker.MyReqs{Incoming: []string{"q"}},
				Badge:     []string{"a:a", "p:a", "q:q", "w:a:done:now"},
				CCNotices: []wstracker.CCNotice{{Key: "cc:e", Type: "item.status", Item: "i", Event: "e", By: "other", To: projectionString("me"), Status: projectionString("done"), TS: "2026-11-18T00:00:00Z"}}},
		},
	}
}

func mustProjectTracker(t *testing.T, v *wstrackerhost.StoreView) wsdashboard.TrackerProjection {
	t.Helper()
	p, err := projectTracker("store", v, wsdashboard.TrackerSeen{Badge: []string{"a:a"}, Read: []string{"r:c"}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTrackerProjectionSummaryContract(t *testing.T) {
	v := projectionFixture()
	p := mustProjectTracker(t, v)
	var row trackerItemProjection
	if err := json.Unmarshal(p.Items["i"].Data, &row); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"body", "bodyRev", "attachments"} {
		if _, ok := row.Item[key]; ok {
			t.Errorf("summary contains %s", key)
		}
	}
	// Every other ItemView field, including thread ids, remains available to
	// list/handle/palette consumers. No manually curated title/status subset.
	rv := reflect.TypeOf(wstracker.ItemView{})
	for i := 0; i < rv.NumField(); i++ {
		key := strings.Split(rv.Field(i).Tag.Get("json"), ",")[0]
		if key == "body" || key == "bodyRev" || key == "attachments" {
			continue
		}
		if _, ok := row.Item[key]; !ok {
			t.Errorf("missing ItemView field %s", key)
		}
	}
	if len(row.Assignments) != 1 || len(row.Requests) != 1 || len(row.Rules) != 1 || len(row.Notices) != 1 || len(row.CCNotices) != 1 {
		t.Fatalf("missing indirect records: %+v", row)
	}
	all, err := json.Marshal(struct {
		Items   any
		Globals any
	}{p.Items, p.Globals})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"body-secret", "body-revision-secret", "comment-secret", "event-payload-secret", "verdict-secret", "secret-attachment.png", "aaaaaaaaaaaaaaaa", `"html"`, `"payload"`} {
		if bytes.Contains(all, []byte(forbidden)) {
			t.Errorf("summary leaks %q", forbidden)
		}
	}
	for _, key := range []string{"items", "comments", "events", "assignments", "requests", "rules", "notices"} {
		if _, ok := p.Globals[key]; ok {
			t.Errorf("per-item/detail global %s", key)
		}
	}
	if string(p.Globals["now"]) != `"2026-11-18T07:00:00Z"` {
		t.Errorf("now = %s", p.Globals["now"])
	}
	for _, key := range []string{"store", "sync", "clones", "viewer", "priority", "attachments", "labels", "members", "admins", "rolesProblems", "itemOrder", "my", "stats", "heads", "now"} {
		if _, ok := p.Globals[key]; !ok {
			t.Errorf("missing global %s", key)
		}
	}
	if bytes.Contains(all, []byte("password")) || bytes.Contains(all, []byte("token=secret")) {
		t.Fatal("credential leak")
	}
	if !strings.Contains(*v.Store.Remote, "password") || !strings.Contains(v.Sync.WikiWarnings[0], "password") {
		t.Fatal("projection mutated source while redacting")
	}
	if !reflect.DeepEqual(p.Seen, wsdashboard.TrackerSeen{Badge: []string{"a:a"}, Read: []string{"r:c"}}) {
		t.Fatalf("seen = %+v", p.Seen)
	}
	if len(p.Items["i"].Seen.Badge) != 0 || len(p.Items["i"].Seen.Read) != 0 {
		t.Fatal("seen overlay belongs to dashboard")
	}
}

func TestTrackerProjectionDetailVersions(t *testing.T) {
	base := mustProjectTracker(t, projectionFixture())
	cases := map[string]func(*wstrackerhost.StoreView){
		"reason only": func(v *wstrackerhost.StoreView) { v.Events[0].Reason = projectionString("new reason") },
		"markers": func(v *wstrackerhost.StoreView) {
			v.Events[0].Markers = []wstracker.Marker{{Kind: "field", Mine: projectionString("changed")}}
		},
		"supersession": func(v *wstrackerhost.StoreView) {
			v.Events[0].Superseded = []wstracker.Superseded{{Field: "body", By: "later", Why: "human"}}
		},
		"payload": func(v *wstrackerhost.StoreView) {
			v.Events[0].Payload = json.RawMessage(`{"item":"i","note":"changed"}`)
		},
		"body":          func(v *wstrackerhost.StoreView) { it := v.Items["i"]; it.Body += "changed"; v.Items["i"] = it },
		"body revision": func(v *wstrackerhost.StoreView) { it := v.Items["i"]; it.BodyRev = []string{"new"}; v.Items["i"] = it },
		"item attachment": func(v *wstrackerhost.StoreView) {
			it := v.Items["i"]
			it.Attachments[0].Name = "renamed"
			v.Items["i"] = it
		},
		"comment": func(v *wstrackerhost.StoreView) { c := v.Comments["c"]; c.Body += "changed"; v.Comments["c"] = c },
		"comment attachment": func(v *wstrackerhost.StoreView) {
			c := v.Comments["c"]
			c.Attachments[0].Mime = "text/plain"
			v.Comments["c"] = c
		},
		"comment hidden": func(v *wstrackerhost.StoreView) {
			c := v.Comments["c"]
			c.Hidden = projectionString("hide")
			v.Comments["c"] = c
		},
		"member display": func(v *wstrackerhost.StoreView) {
			v.Members = map[string]wstracker.MemberView{"other": {Name: "Renamed"}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			v := projectionFixture()
			change(v)
			p := mustProjectTracker(t, v)
			if p.Items["i"].DetailVersion == base.Items["i"].DetailVersion {
				t.Fatal("detail version did not change")
			}
			if name == "reason only" && !bytes.Equal(p.Items["i"].Data, base.Items["i"].Data) {
				t.Fatal("reason-only change altered summary data")
			}
		})
	}
	if again := mustProjectTracker(t, projectionFixture()); !reflect.DeepEqual(base.Items, again.Items) {
		t.Fatal("identical inputs unstable")
	}
}

func TestTrackerProjectionIndirectDiffAndSplitGlobals(t *testing.T) {
	base := mustProjectTracker(t, projectionFixture())
	for name, change := range map[string]func(*wstrackerhost.StoreView){
		"assignment stale": func(v *wstrackerhost.StoreView) { a := v.Assignments["a"]; a.Stale = true; v.Assignments["a"] = a },
		"request":          func(v *wstrackerhost.StoreView) { q := v.Requests["q"]; q.State = "denied"; v.Requests["q"] = q },
		"rule":             func(v *wstrackerhost.StoreView) { r := v.Rules["rule"]; r.State = "ended"; v.Rules["rule"] = r },
		"notice":           func(v *wstrackerhost.StoreView) { v.Notices[0].Kind = "blocked" },
		"CC notice":        func(v *wstrackerhost.StoreView) { v.My.CCNotices[0].Status = projectionString("blocked") },
	} {
		t.Run(name, func(t *testing.T) {
			v := projectionFixture()
			change(v)
			p := mustProjectTracker(t, v)
			if bytes.Equal(base.Items["i"].Data, p.Items["i"].Data) {
				t.Fatal("indirect change missed row diff")
			}
			if !reflect.DeepEqual(base.Items["other-item"], p.Items["other-item"]) {
				t.Fatal("unrelated item changed")
			}
		})
	}
	v := projectionFixture()
	v.Stats.Commits++
	v.Now = "2026-11-18T07:59:00Z"
	p := mustProjectTracker(t, v)
	if bytes.Equal(base.Globals["stats"], p.Globals["stats"]) {
		t.Fatal("stats did not change")
	}
	for k := range base.Globals {
		if k != "stats" && !bytes.Equal(base.Globals[k], p.Globals[k]) {
			t.Errorf("stats/within-hour clock changed global %s", k)
		}
	}
	if !reflect.DeepEqual(base.Items, p.Items) {
		t.Fatal("stats/clock invalidated item detail")
	}
}

func TestTrackerProjectionTimeRelativeDiff(t *testing.T) {
	v := projectionFixture()
	it := v.Items["i"]
	it.Due = projectionString("2026-11-18")
	it.Status = projectionString("done")
	v.Items["i"] = it
	a := v.Assignments["a"]
	a.Status = "done"
	a.LastUpdate = "2026-11-04T07:30:00Z"
	v.Assignments["a"] = a
	before := mustProjectTracker(t, v)
	// Only the clock moves, crossing both the due day and the closed window.
	v.Now = "2026-11-19T07:18:00Z"
	after := mustProjectTracker(t, v)
	if bytes.Equal(before.Items["i"].Data, after.Items["i"].Data) {
		t.Fatal("time-relative transition missed item diff")
	}
	var oldRow, newRow trackerItemProjection
	if err := json.Unmarshal(before.Items["i"].Data, &oldRow); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after.Items["i"].Data, &newRow); err != nil {
		t.Fatal(err)
	}
	if oldRow.Time.DueState != "today" || newRow.Time.DueState != "over" || !oldRow.Time.RecentlyClosed || newRow.Time.RecentlyClosed || !oldRow.Time.RecentAssignments["a"] || newRow.Time.RecentAssignments["a"] {
		t.Fatalf("time states: %+v -> %+v", oldRow.Time, newRow.Time)
	}
	if !reflect.DeepEqual(before.Items["other-item"], after.Items["other-item"]) {
		t.Fatal("unaffected topic changed on clock tick")
	}
}

func TestTrackerProjectionDefersDetailEncoding(t *testing.T) {
	v := projectionFixture()
	// A deliberately invalid event payload is hashable but not JSON encodable.
	// A summary and an unrelated detail must not try encoding it eagerly.
	v.Events[0].Payload = json.RawMessage(`not-json`)
	p := mustProjectTracker(t, v)
	if _, err := p.Detail(context.Background(), []string{"other-item"}); err != nil {
		t.Fatal("encoded unrequested detail", err)
	}
	if _, err := p.Detail(context.Background(), []string{"i"}); err == nil {
		t.Fatal("requested invalid payload unexpectedly encoded")
	}
}

func TestTrackerProjectionLazySafeDetail(t *testing.T) {
	v := projectionFixture()
	p := mustProjectTracker(t, v)
	bundles, err := p.Detail(context.Background(), []string{"i", "i", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("requested-only bundles = %v", bundles)
	}
	var data trackerDetailData
	if err := json.Unmarshal(bundles["i"], &data); err != nil {
		t.Fatal(err)
	}
	if data.Body != v.Items["i"].Body || !reflect.DeepEqual(data.BodyRev, v.Items["i"].BodyRev) || len(data.Comments) != 2 || len(data.Events) != 2 || len(data.Attachments) != 1 {
		t.Fatalf("incomplete bundle: %+v", data)
	}
	if trackerString(data.Events[0].Reason) != "verdict-secret" {
		t.Fatal("bundle lost verdict reason")
	}
	if !reflect.DeepEqual(data.HTML, renderViewHTML("store", &wstrackerhost.StoreView{View: &wstracker.View{Items: map[string]wstracker.ItemView{"i": v.Items["i"]}, Comments: v.Comments}})) {
		t.Fatal("detail renderer differs from full view")
	}
	html := string(data.HTML.Items["i"])
	if strings.Contains(html, "<script>") || strings.Contains(html, `src="https://example.test`) || !strings.Contains(html, "/tracker/stores/store/attachments/") || !strings.Contains(html, "<strong>body-secret</strong>") {
		t.Fatalf("unsafe/wrong HTML: %s", html)
	}
	if bytes.Contains(bundles["i"], []byte("not-requested-secret")) {
		t.Fatal("rendered unrelated item")
	}
	// Replacing the caller's variable with a newer immutable view cannot
	// affect this closure, as a later host publication must not affect it.
	newer := projectionFixture()
	newer.Events[0].Reason = projectionString("newer reason")
	newP := mustProjectTracker(t, newer)
	again, err := p.Detail(context.Background(), []string{"i"})
	if err != nil || !bytes.Equal(bundles["i"], again["i"]) {
		t.Fatal("captured bundle drifted", err)
	}
	if newP.Items["i"].DetailVersion == p.Items["i"].DetailVersion {
		t.Fatal("new bundle not version-separated")
	}
	// This is the dashboard's publication rule: the bundle is paired with
	// the row version from its projection, not a separately reread view.
	wrapped, err := json.Marshal(wsdashboard.TrackerDetailBundle{DetailVersion: p.Items["i"].DetailVersion, Data: bundles["i"]})
	if err != nil {
		t.Fatal(err)
	}
	var bundle wsdashboard.TrackerDetailBundle
	if err := json.Unmarshal(wrapped, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.DetailVersion != p.Items["i"].DetailVersion {
		t.Fatal("bundle version mismatch")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Detail(ctx, []string{"i"}); err != context.Canceled {
		t.Fatalf("cancel = %v", err)
	}
}

func TestTrackerProjectionHiddenAndForeignAttachments(t *testing.T) {
	v := projectionFixture()
	c := v.Comments["c"]
	c.Hidden = projectionString("hide")
	v.Comments["c"] = c
	it := v.Items["i"]
	it.Body += " ![foreign](attachment:" + strings.Repeat("b", 64) + ")"
	v.Items["i"] = it
	p := mustProjectTracker(t, v)
	bundles, err := p.Detail(context.Background(), []string{"i"})
	if err != nil {
		t.Fatal(err)
	}
	var data trackerDetailData
	if err := json.Unmarshal(bundles["i"], &data); err != nil {
		t.Fatal(err)
	}
	if _, ok := data.HTML.Comments["c"]; ok {
		t.Fatal("hidden comment rendered")
	}
	if data.Comments["c"].Hidden == nil {
		t.Fatal("hidden comment record lost")
	}
	if strings.Contains(string(data.HTML.Items["i"]), "/attachments/"+strings.Repeat("b", 64)) {
		t.Fatal("foreign attachment rendered")
	}
}

func TestTrackerProjectionHashBoundaries(t *testing.T) {
	a, err := trackerProjectionHash(map[string]any{"b": []string{"ab", "c"}, "a": projectionString("value")})
	if err != nil {
		t.Fatal(err)
	}
	b, err := trackerProjectionHash(map[string]any{"a": projectionString("value"), "b": []string{"ab", "c"}})
	if err != nil || a != b {
		t.Fatal("map/pointer identity affects hash", err)
	}
	c, err := trackerProjectionHash(map[string]any{"a": projectionString("value"), "b": []string{"a", "bc"}})
	if err != nil || a == c {
		t.Fatal("hash lost field boundaries", err)
	}
	if _, err := trackerProjectionHash(func() {}); err == nil {
		t.Fatal("unsupported hash input silently accepted")
	}
}

func TestTrackerProjectionFeedKeysAndBound(t *testing.T) {
	v := projectionFixture()
	p := mustProjectTracker(t, v)
	var feed []trackerFeedEntry
	if err := json.Unmarshal(p.Feed, &feed); err != nil {
		t.Fatal(err)
	}
	entries := map[string]trackerFeedEntry{}
	for _, e := range feed {
		entries[e.Key] = e
	}
	for _, key := range []string{"a:a", "p:a", "q:q", "d:a:over", "w:a:done:now", "c:a:2026-11-18T00:00:00Z", "r:c", "m:c:2026-11-18T01:00:00Z", "m:i:2026-11-17T01:00:00Z", "cc:e"} {
		if _, ok := entries[key]; !ok {
			t.Errorf("missing feed key %s", key)
		}
		if !projectionHas(p.Keys["i"].Read, key) {
			t.Errorf("missing read mapping %s", key)
		}
	}
	if !reflect.DeepEqual(p.Keys["i"].Badge, []string{"a:a", "p:a", "q:q", "w:a:done:now"}) {
		t.Fatalf("badge mappings = %+v", p.Keys["i"])
	}
	if trackerString(entries["r:c"].Event) != "c" || trackerString(entries["w:a:done:now"].Event) != "e" || entries["cc:e"].Snippet != "event-payload-secret" || !entries["m:i:2026-11-17T01:00:00Z"].BodyMention {
		t.Fatal("lost feed rendering/focus inputs")
	}
	// Every older reply/mention still has seen candidates when dropped from
	// the latest-200 feed; a comment may have BOTH notification keys.
	for i := 0; i < 220; i++ {
		id := fmt.Sprintf("many-%03d", i)
		v.Comments[id] = wstracker.CommentView{ID: id, Item: "i", Author: "other", Body: strings.Repeat("界", 100), ReplyTo: projectionString("mine"), Mentions: []string{"me"}, TS: "2026-11-18T06:00:00Z"}
	}
	p = mustProjectTracker(t, v)
	if err := json.Unmarshal(p.Feed, &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed) != 200 {
		t.Fatalf("feed length = %d", len(feed))
	}
	for i, e := range feed {
		if !utf8.ValidString(e.Snippet) || utf8.RuneCountInString(e.Snippet) > 80 {
			t.Fatalf("invalid/unbounded snippet: %q", e.Snippet)
		}
		if i > 0 && (feed[i-1].TS < e.TS || (feed[i-1].TS == e.TS && feed[i-1].Key > e.Key)) {
			t.Fatal("feed not newest/key sorted")
		}
	}
	if len(p.Keys["i"].Read) != 450 {
		t.Fatalf("read keys truncated: %d", len(p.Keys["i"].Read))
	}
	for _, key := range []string{"r:c", "m:c:2026-11-18T01:00:00Z", "r:many-219", "m:many-219:2026-11-18T06:00:00Z", "cc:e", "d:a:over"} {
		if !projectionHas(p.Keys["i"].Read, key) {
			t.Errorf("older mapping missing: %s", key)
		}
	}
	if !projectionHas(p.Keys["i"].Badge, "a:a") {
		t.Fatal("feed bound lost badge")
	}
}

func TestTrackerProjectionNoticeAndMentionVariants(t *testing.T) {
	v := projectionFixture()
	a := v.Assignments["a"]
	a.HandoffOf = projectionString("prior")
	a.StatusBy = projectionString("superior")
	a.Note = projectionString("status note")
	a.Accepted = &wstracker.Ref{ID: "accept"}
	a.Ended = &wstracker.Ended{ID: "end"}
	v.Assignments["a"] = a
	v.Notices = nil
	v.My.Badge = nil
	for _, kind := range []string{"done", "blocked", "dropped", "declined", "returned", "accepted", "handoff"} {
		key := "notice:" + kind
		v.Notices = append(v.Notices, wstracker.Notice{Key: key, Kind: kind, Assignment: "a", Item: "i", To: "me", By: "other", From: projectionString("from"), HasFrom: true, EndedBy: "ender", Note: projectionString("answer note"), Answered: true})
		v.My.Badge = append(v.My.Badge, key)
	}
	c := v.Comments["c"]
	c.Edited = &wstracker.Ref{ID: "edit", TS: "2026-11-18T03:00:00Z"}
	v.Comments["c"] = c
	p := mustProjectTracker(t, v)
	var feed []trackerFeedEntry
	if err := json.Unmarshal(p.Feed, &feed); err != nil {
		t.Fatal(err)
	}
	entries := map[string]trackerFeedEntry{}
	for _, e := range feed {
		entries[e.Key] = e
	}
	for _, kind := range []string{"done", "blocked", "dropped", "declined", "returned", "accepted", "handoff"} {
		e, ok := entries["notice:"+kind]
		if !ok || !e.Handoff || e.From != "from" || e.EndedBy != "ender" {
			t.Errorf("notice variant missing: %s: %+v", kind, e)
		}
		wantEvent, wantSnippet := "e", "status note"
		if kind == "declined" || kind == "returned" {
			wantEvent, wantSnippet = "end", "answer note"
			if e.Kind != "answer" {
				t.Error("wrong answer kind")
			}
		}
		if kind == "accepted" || kind == "handoff" {
			wantEvent, wantSnippet = "accept", "answer note"
			if e.Kind != "reference" {
				t.Error("wrong reference kind")
			}
		}
		if trackerString(e.Event) != wantEvent || e.Snippet != wantSnippet {
			t.Errorf("wrong notice inputs: %+v", e)
		}
		if !projectionHas(p.Keys["i"].Badge, e.Key) || !projectionHas(p.Keys["i"].Read, e.Key) {
			t.Error("missing notice seen mapping")
		}
	}
	if !projectionHas(p.Keys["i"].Read, "m:c:2026-11-18T03:00:00Z") || projectionHas(p.Keys["i"].Read, "m:c:2026-11-18T01:00:00Z") {
		t.Fatal("edited mention timestamp incorrect")
	}
	// Hidden and self-authored comments generate neither reply nor mention.
	for _, hide := range []bool{true, false} {
		v := projectionFixture()
		c := v.Comments["c"]
		if hide {
			c.Hidden = projectionString("hide")
		} else {
			c.Author = "me"
		}
		v.Comments["c"] = c
		p := mustProjectTracker(t, v)
		if projectionHas(p.Keys["i"].Read, "r:c") || projectionHas(p.Keys["i"].Read, "m:c:2026-11-18T01:00:00Z") {
			t.Fatal("hidden/self comment notified")
		}
	}
}

func projectionHas(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}
