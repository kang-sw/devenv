package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixedContentView(t *testing.T, topic TopicView) EntryView {
	t.Helper()
	for _, e := range pageEntries(topic.Stream) {
		if e.LegacyContent && !e.Historical {
			return e
		}
	}
	t.Fatalf("topic %q has no latest content panel in view", topic.Name)
	return EntryView{}
}

func legacyVersions(t *testing.T, st *State, topic string) []Entry {
	t.Helper()
	d := topicEntry(t, st, topic)
	fixed, ok := d.legacyContentPanel()
	if !ok {
		t.Fatalf("topic %q has no fixed content panel", topic)
	}
	var out []Entry
	for _, e := range d.Entries {
		if e.Kind == EntryPanel && e.PanelID == fixed.PanelID {
			out = append(out, e)
		}
	}
	return out
}

// Check retained bytes independently of the decoded history, including the
// optional marker's equality in immutable panel and payload-free log records.
func assertLegacyFiles(t *testing.T, dir string, prefix map[string]string) {
	t.Helper()
	tree := folderTree(t, dir)
	for path, old := range prefix {
		if strings.Contains(path, "/panels/") && tree[path] != old || strings.HasSuffix(path, "/log.jsonl") && !strings.HasPrefix(tree[path], old) {
			t.Fatalf("immutable prefix changed: %s", path)
		}
	}
	for path, data := range tree {
		if !strings.HasSuffix(path, "/log.jsonl") {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
			if line == "" {
				continue
			}
			var log Entry
			if err := json.Unmarshal([]byte(line), &log); err != nil {
				t.Fatal(err)
			}
			if log.Kind != EntryPanel {
				continue
			}
			var panel Entry
			panelPath := filepath.ToSlash(filepath.Join(filepath.Dir(path), "panels", log.ID+".json"))
			if err := json.Unmarshal([]byte(tree[panelPath]), &panel); err != nil {
				t.Fatal(err)
			}
			panel.Text = ""
			if log.Text != "" || !reflect.DeepEqual(log, panel) {
				t.Fatalf("panel/log metadata mismatch: %s", panelPath)
			}
		}
	}
}

func assertLegacyExact(t *testing.T, st *State, versions []Entry) {
	t.Helper()
	for _, want := range versions {
		got := queryResult(t, st, fmt.Sprintf(`{"panel":%q}`, want.ID)).Direct.Entry
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exact version %s changed: got %+v, want %+v", want.ID, got, want)
		}
	}
	latest := versions[len(versions)-1]
	if got := queryResult(t, st, fmt.Sprintf(`{"panel":%q}`, latest.PanelID)).Direct.Entry; !reflect.DeepEqual(got, latest) {
		t.Fatalf("latest panel link did not resolve to %s: %+v", latest.ID, got)
	}
}

func legacyMigrationFixture() *State {
	return &State{Seq: 12, Counter: 7, EntrySeq: 8, PanelCounter: 3,
		Topics: []TopicDoc{{Name: "Old title", Body: "  雪 &amp; {{7}} [prior](#e7)\r\n\n<svg>raw</svg>  ", Changed: 9, ChangedAt: "2026-11-01T01:02:03Z",
			Entries: []Entry{
				{ID: "e7", Kind: EntryPanel, PanelID: "p3", Recap: "Keep this", Text: "original payload", At: "2026-10-31T00:00:00Z"},
				{ID: "e8", Kind: EntryAgent, Text: "retained {{7}} [panel](#p3)", At: "2026-11-02T00:00:00Z"},
			}}},
		Items: []Item{{ID: 7, Topic: "Old title", Class: ClassCheck, Status: StatusOpen, Title: "Keep reference"}},
	}
}

func TestLegacyContentInitialFolderConversion(t *testing.T) {
	legacy := legacyMigrationFixture()
	original := folderJSON(legacy)
	dir := t.TempDir()
	got := folderOpen(t, dir, legacy)
	if !bytes.Equal(original, folderJSON(legacy)) {
		t.Fatal("conversion mutated its legacy input")
	}
	versions := legacyVersions(t, got, "Old title")
	want := Entry{ID: "e9", Kind: EntryPanel, PanelID: "p4", LegacyContent: true, Recap: "Current content", Text: legacy.Topics[0].Body, At: legacy.Topics[0].ChangedAt, Imported: true}
	if !reflect.DeepEqual(versions, []Entry{want}) || got.EntrySeq != 9 || got.PanelCounter != 4 {
		t.Fatalf("migration lost payload/time/identity: %+v", versions)
	}
	if !reflect.DeepEqual(got.Topics[0].Entries[:2], legacy.Topics[0].Entries) || !reflect.DeepEqual(got.Items, legacy.Items) || got.Topics[0].ChangedAt != legacy.Topics[0].ChangedAt {
		t.Fatal("conversion changed old history, item references, or body time")
	}
	assertLegacyExact(t, got, versions)
	assertLegacyFiles(t, dir, nil)
	view := topicView(t, got, LayoutFull, "Old title")
	if view.Doc != "" {
		t.Fatal("migrated content rendered twice")
	}
	found := 0
	for _, e := range pageEntries(view.Stream) {
		if e.ID == want.ID && e.Text == want.Text {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("migrated snapshot appears %d times in stream", found)
	}
	before := folderTree(t, dir)
	if again := folderOpen(t, dir, legacy); !reflect.DeepEqual(got, again) || !reflect.DeepEqual(before, folderTree(t, dir)) {
		t.Fatal("reopen duplicated or republished migration")
	}
}

func TestLegacyContentRepeatedAliasesAndAdoption(t *testing.T) {
	dir := t.TempDir()
	f := FolderStore{Dir: dir}
	write := func(xml string, at time.Time) *State {
		t.Helper()
		if err := f.Update(nil, func(s *State) error { applyBodyTimeAt(t, s, xml, at); return nil }); err != nil {
			t.Fatal(err)
		}
		return folderOpen(t, dir, nil)
	}
	// A user-chosen name/recap is not authority for the body/edit alias.
	st := write(panelCommandXML("Old title", `name="current-content" recap="Current content"`, "authored panel")+topicXML("Old title", "alpha anchor"), testNow)
	fixed := legacyVersions(t, st, "Old title")[0]
	if fixed.PanelID != "p2" || fixed.PanelName != "" || st.PanelCounter != 2 {
		t.Fatalf("alias collided with authored panel: %+v", fixed)
	}
	prefix := folderTree(t, dir)
	edit := `<wb:topic name="Old title"><wb:edit><wb:old>alpha</wb:old><wb:new>beta</wb:new></wb:edit></wb:topic>`
	st = write(edit, testNow.Add(time.Minute))
	st = write(topicXML("Old title", "gamma anchor"), testNow.Add(2*time.Minute))
	versions := legacyVersions(t, st, "Old title")
	for i, body := range []string{"alpha anchor", "beta anchor", "gamma anchor"} {
		if versions[i].Text != body || versions[i].PanelID != fixed.PanelID || !versions[i].LegacyContent || versions[i].At != testNow.Add(time.Duration(i)*time.Minute).Format(time.RFC3339Nano) {
			t.Fatalf("version %d lost fixed identity/time: %+v", i, versions[i])
		}
	}
	if len(versions) != 3 || st.TopicBody("Old title") != "gamma anchor" {
		t.Fatal("body/edit/body did not append three snapshots")
	}
	assertBodyTime(t, st.Topics[0], testNow.Add(2*time.Minute).Format(time.RFC3339))
	// Identical body and identity edit must not publish a version or reset age.
	before := st.clone()
	tree := folderTree(t, dir)
	st = write(topicXML("Old title", "gamma anchor")+`<wb:topic name="Old title"><wb:edit><wb:old>gamma</wb:old><wb:new>gamma</wb:new></wb:edit></wb:topic>`, testNow.Add(time.Hour))
	before.Seq = st.Seq // Apply still advances the render batch on no-op calls.
	if !reflect.DeepEqual(before, st) {
		t.Fatal("no-op alias changed body, age, or history")
	}
	assertLegacyFiles(t, dir, tree)
	st = write(`<wb:topic name="adopted" label="Old title"/>`, testNow.Add(2*time.Hour))
	if st.topicKnown("Old title") || !reflect.DeepEqual(legacyVersions(t, st, "adopted"), versions) {
		t.Fatal("adoption lost the fixed panel/history")
	}
	// Direct pN reposts update the compatibility projection too, so the next
	// alias edits that payload rather than the last body-authored snapshot.
	st = write(panelCommandXML("adopted", `id="p2" recap="Direct"`, "direct anchor"), testNow.Add(3*time.Hour))
	if st.TopicBody("adopted") != "direct anchor" {
		t.Fatal("direct id repost did not update Body projection")
	}
	assertBodyTime(t, st.Topics[0], testNow.Add(3*time.Hour).Format(time.RFC3339))
	st = write(`<wb:topic name="Old title"><wb:edit><wb:old>direct</wb:old><wb:new>alias</wb:new></wb:edit><wb:body>final anchor</wb:body></wb:topic>`, testNow.Add(4*time.Hour))
	versions = legacyVersions(t, st, "adopted")
	if len(versions) != 6 || versions[3].Text != "direct anchor" || versions[4].Text != "alias anchor" || versions[5].Text != "final anchor" || st.PanelCounter != 2 {
		t.Fatalf("direct/aliases lost stable versions: %+v", versions)
	}
	if got := queryResult(t, st, `{"panel":"p1"}`).Direct.Entry; got.Text != "authored panel" || got.LegacyContent {
		t.Fatal("fixed panel writes changed the named authored panel")
	}
	assertLegacyExact(t, st, versions)
	assertLegacyFiles(t, dir, prefix)
	st = write(`<wb:topic name="adopted"><wb:close result="done"/></wb:topic>`, testNow.Add(5*time.Hour))
	st = write(topicXML("adopted", "final anchor"), testNow.Add(6*time.Hour))
	if !st.topicClosed("adopted") || len(legacyVersions(t, st, "adopted")) != 6 {
		t.Fatal("no-op body reopened a closed topic")
	}
	st = write(topicXML("adopted", "reopened"), testNow.Add(7*time.Hour))
	if st.topicClosed("adopted") || legacyVersions(t, st, "adopted")[6].Text != "reopened" {
		t.Fatal("changed body did not reopen the same fixed panel")
	}
}

func TestLegacyContentClearAndEmptyEdit(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", ""))
	if st.PanelCounter != 0 {
		t.Fatal("clearing absent content minted a panel")
	}
	mustApply(t, st, topicXML("t", "one one"))
	refused(t, st, `<wb:topic name="t"><wb:edit><wb:old>one</wb:old><wb:new/></wb:edit></wb:topic>`)
	mustApply(t, st, `<wb:topic name="t"><wb:edit replace_all="true"><wb:old>one</wb:old><wb:new/></wb:edit></wb:topic>`)
	versions := legacyVersions(t, st, "t")
	if len(versions) != 2 || versions[1].Text != "" || st.TopicBody("t") != "" || st.Topics[0].ChangedAt != "" || st.Topics[0].BodyTimeProof != nil {
		t.Fatalf("empty edit did not retain a clear snapshot: %+v", st.Topics[0])
	}
	refused(t, st, `<wb:topic name="t"><wb:edit><wb:old>one</wb:old><wb:new>two</wb:new></wb:edit></wb:topic>`)
	before := st.clone()
	mustApply(t, st, topicXML("t", "  "))
	before.Seq = st.Seq
	if !reflect.DeepEqual(before, st) {
		t.Fatal("repeated clear changed history")
	}
	mustApply(t, st, topicXML("t", "again")+topicXML("t", ""))
	versions = legacyVersions(t, st, "t")
	if len(versions) != 4 || versions[2].Text != "again" || versions[3].Text != "" || st.PanelCounter != 1 {
		t.Fatal("body clear did not version the same panel")
	}
	assertLegacyExact(t, st, versions)
}

// A new alias's own trailing Content event is not evidence of a distinct
// old Body mutation. An old id-only repost needs no extra import in these cases.
func TestLegacyContentOldIDAfterNewAlias(t *testing.T) {
	binary := buildPanelV050Writer(t)
	for _, tc := range []struct{ name, command string }{
		{"edit", `<wb:edit><wb:old>new</wb:old><wb:new>edited</wb:new></wb:edit>`},
		{"clear", `<wb:body/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			f := FolderStore{Dir: dir}
			if err := f.Update(nil, func(s *State) error {
				mustApply(t, s, topicXML("t", "new payload")+`<wb:topic name="t">`+tc.command+`</wb:topic>`)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := legacyVersions(t, folderOpen(t, dir, nil), "t")
			if len(before) != 2 {
				t.Fatal("fixture must have two new alias versions")
			}
			historicalCall(t, binary, dir, "update", panelCommandXML("t", `id="p1" recap="Old id"`, "old id wins"), false)
			st := folderOpen(t, dir, nil)
			got := legacyVersions(t, st, "t")
			if len(got) != 3 || !reflect.DeepEqual(got[:2], before) || got[2].LegacyContent || got[2].Text != "old id wins" || st.TopicBody("t") != "old id wins" {
				t.Fatalf("old id repost fabricated migration versions: %+v", got)
			}
			assertLegacyExact(t, st, got)
			tree := folderTree(t, dir)
			if again := folderOpen(t, dir, nil); !reflect.DeepEqual(st, again) || !reflect.DeepEqual(tree, folderTree(t, dir)) {
				t.Fatal("old id reconciliation did not stabilize")
			}
		})
	}
}

func TestLegacyContentThroughV050Writer(t *testing.T) {
	binary := buildPanelV050Writer(t)
	t.Run("existing-schema2-migration-and-recovery", func(t *testing.T) {
		for _, stage := range []string{"committed", "intent", "log-partial"} {
			t.Run(stage, func(t *testing.T) {
				dir := t.TempDir()
				legacy := legacyMigrationFixture()
				path := filepath.Join(t.TempDir(), "legacy.json")
				if err := os.WriteFile(path, folderJSON(legacy), 0644); err != nil {
					t.Fatal(err)
				}
				historicalCall(t, binary, dir, "legacy", path, false)
				prefix := folderTree(t, dir)
				f := FolderStore{Dir: dir}
				if stage != "committed" {
					f.Fault = func(hit string) error {
						if hit == stage {
							return folderCrash
						}
						return nil
					}
				}
				_, err := f.Open(nil)
				if stage == "committed" && err != nil || stage != "committed" && !errors.Is(err, folderCrash) {
					t.Fatalf("migration crash %s: %v", stage, err)
				}
				// The actual old writer must also recover a new migration intent.
				historicalCall(t, binary, dir, "open", "", false)
				got := folderOpen(t, dir, nil)
				versions := legacyVersions(t, got, "Old title")
				if len(versions) != 1 || versions[0].Text != legacy.Topics[0].Body || versions[0].At != legacy.Topics[0].ChangedAt || !versions[0].Imported || versions[0].ID != "e9" || versions[0].PanelID != "p4" || !versions[0].LegacyContent {
					t.Fatalf("existing schema-2 migration lost content/time/ref: %+v", versions)
				}
				if !reflect.DeepEqual(got.Topics[0].Entries[:2], legacy.Topics[0].Entries) || !reflect.DeepEqual(got.Items, legacy.Items) {
					t.Fatal("schema-2 migration changed prior references")
				}
				assertLegacyFiles(t, dir, prefix)
				assertLegacyExact(t, got, versions)
				tree := folderTree(t, dir)
				folderOpen(t, dir, nil)
				if !reflect.DeepEqual(tree, folderTree(t, dir)) {
					t.Fatal("migration/recovery repeated on reopen")
				}
			})
		}
	})
	for _, tc := range []struct{ name, commands, want string }{
		{"body", topicXML("t", "old body"), "old body"},
		{"edit", `<wb:topic name="t"><wb:edit><wb:old>new</wb:old><wb:new>old</wb:new></wb:edit></wb:topic>`, "old payload"},
		{"clear", topicXML("t", ""), ""},
		{"empty-edit", `<wb:topic name="t"><wb:edit><wb:old>new payload</wb:old><wb:new/></wb:edit></wb:topic>`, ""},
		{"id", panelCommandXML("t", `id="p1" recap="Old id"`, "old id payload"), "old id payload"},
		{"body-then-id", topicXML("t", "old body") + panelCommandXML("t", `id="p1" recap="Old id"`, "id wins"), "id wins"},
		{"body-then-id-adjacent", panelCommandXML("t", `id="p1" recap="Old id"`, "id wins"), "id wins"},
		{"edit-then-id", `<wb:topic name="t"><wb:edit><wb:old>new</wb:old><wb:new>old</wb:new></wb:edit></wb:topic>` + panelCommandXML("t", `id="p1" recap="Old id"`, "id wins"), "id wins"},
		{"id-then-edit", panelCommandXML("t", `id="p1" recap="Old id"`, "old id payload") + `<wb:topic name="t"><wb:edit><wb:old>new</wb:old><wb:new>old</wb:new></wb:edit></wb:topic>`, "old payload"},
		{"id-then-body", panelCommandXML("t", `id="p1" recap="Old id"`, "old id payload") + topicXML("t", "body wins"), "body wins"},
		{"id-then-clear", panelCommandXML("t", `id="p1" recap="Old id"`, "old id payload") + topicXML("t", ""), ""},
		{"unrelated", logXML("Other", "unrelated old write"), "new payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			f := FolderStore{Dir: dir}
			if err := f.Update(nil, func(s *State) error { mustApply(t, s, topicXML("t", "new payload")); return nil }); err != nil {
				t.Fatal(err)
			}
			original := legacyVersions(t, folderOpen(t, dir, nil), "t")[0]
			prefix := folderTree(t, dir)
			if tc.name == "body-then-id-adjacent" {
				historicalCall(t, binary, dir, "update", topicXML("t", "old body"), false)
			}
			historicalCall(t, binary, dir, "update", tc.commands+logXML("Other", "still writable"), false)
			oldTree := folderTree(t, dir)
			got := folderOpen(t, dir, nil)
			versions := legacyVersions(t, got, "t")
			retainedBody := tc.name == "body-then-id" || tc.name == "body-then-id-adjacent" || tc.name == "edit-then-id"
			idLast := tc.name == "id" || retainedBody
			wantCount := 2
			if tc.name == "unrelated" {
				wantCount = 1
			}
			if tc.name == "id-then-body" || tc.name == "id-then-clear" || tc.name == "id-then-edit" {
				wantCount = 3
			}
			// These Body bytes remain durable in old topic metadata even though
			// a later old id repost wins the current projection. Retain both.
			if retainedBody {
				wantCount = 4
				intermediate := "old body"
				if tc.name == "edit-then-id" {
					intermediate = "old payload"
				}
				found := false
				for _, e := range versions {
					if e.Text == intermediate {
						found = true
					}
				}
				if !found {
					t.Errorf("lost retained old Body snapshot %q after later id repost: %+v", intermediate, versions)
				}
			}
			latest := versions[len(versions)-1]
			if len(versions) != wantCount || latest.Text != tc.want || got.TopicBody("t") != tc.want || got.PanelCounter != 1 {
				t.Fatalf("old %s/new reopen: versions %+v, body %q", tc.name, versions, got.TopicBody("t"))
			}
			if tc.name != "unrelated" && latest.At != "2026-11-15T01:05:00Z" {
				t.Fatalf("old change time lost: %+v", latest)
			}
			if !idLast && tc.name != "unrelated" && (!latest.Imported || !latest.LegacyContent) {
				t.Fatal("old body/edit/clear did not migrate to marked snapshot")
			}
			if !reflect.DeepEqual(versions[0], original) {
				t.Fatal("old writer changed original fixed panel")
			}
			assertLegacyFiles(t, dir, prefix)
			assertLegacyFiles(t, dir, oldTree)
			assertLegacyExact(t, got, versions)
			tree := folderTree(t, dir)
			folderOpen(t, dir, nil)
			if !reflect.DeepEqual(tree, folderTree(t, dir)) {
				t.Fatal("old write migration duplicated on second reopen")
			}
			// Edit the new projection first: an old id-only repost must not
			// resurrect stale Body when the new alias next runs.
			if tc.want != "" {
				if err := f.Update(nil, func(s *State) error {
					mustApply(t, s, fmt.Sprintf(`<wb:topic name="t"><wb:edit><wb:old>%s</wb:old><wb:new>edited after old writer</wb:new></wb:edit></wb:topic>`, tc.want))
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				got = folderOpen(t, dir, nil)
				versions = legacyVersions(t, got, "t")
				if got.TopicBody("t") != "edited after old writer" {
					t.Fatal("edit resurrected stale Body")
				}
			}
			// Aliases must select the original marker through an unmarked old
			// id repost, including an empty compatibility projection.
			if err := f.Update(nil, func(s *State) error { mustApply(t, s, topicXML("t", "after old writer")); return nil }); err != nil {
				t.Fatal(err)
			}
			got = folderOpen(t, dir, nil)
			next := legacyVersions(t, got, "t")
			if len(next) != len(versions)+1 || next[len(next)-1].PanelID != "p1" || !next[len(next)-1].LegacyContent {
				t.Fatal("alias lost fixed identity after old writer")
			}
			assertLegacyExact(t, got, next)
			assertLegacyFiles(t, dir, tree)
		})
	}
}

// Render only: a caller may retain this production page for a separate local
// browser capture. This test never starts a browser or a ws executable.
