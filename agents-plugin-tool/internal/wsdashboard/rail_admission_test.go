package wsdashboard

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Exercise both sorting layers on each admission, not just synthetic facts
// whose positions already happen to match the desired result.
func TestRailSequentialAdmission(t *testing.T) {
	for _, group := range []string{"plain", "root", "linked"} {
		for _, mode := range []string{"empty", "tree", "saved"} {
			t.Run(group+"/"+mode, func(t *testing.T) {
				now := time.Now()
				ws, project, repo := filepath.Clean("/w/plain"), filepath.Clean("/w/plain"), ""
				var repos map[string]railRepo
				if group != "plain" {
					ws, repo = filepath.Clean("/w/repo"), filepath.Clean("/w/repo")
					project = repo
					if group == "linked" {
						project = filepath.Clean("/w/feature")
					}
					repos = map[string]railRepo{repo: {Read: true, Entries: []railWorktreeEntry{{Path: repo}, {Path: filepath.Clean("/w/feature")}}}}
				}
				var inputs []railInput
				add := func(id string, ended time.Time) {
					registered := now.Add(-24 * time.Hour).Add(time.Duration(len(inputs)) * time.Minute)
					inputs = append(inputs, railInput{Session: railSession(id, project, registered, ended), Repo: repo})
				}
				for _, id := range []string{"first", "second", "parent", "child-one", "child-two", "star-old", "star-parent", "star-child"} {
					add(id, time.Time{})
				}
				// Registration and ended timestamp order intentionally disagree.
				add("ended-old", now.Add(-time.Hour))
				add("ended-new", now.Add(-2*time.Hour))
				doc := emptyLayout()
				if mode != "empty" {
					doc.Workspaces[ws] = &railLayoutWS{
						Stars:    []string{"star-old", "star-parent", "child-one", "new-star-z", "new-star-a"},
						Children: map[string][]string{"parent": {"child-two", "child-one"}, "star-parent": {"star-child"}},
					}
					if mode == "saved" {
						doc.Workspaces[ws].Order = []string{"second", "parent", "first", "star-parent", "star-old", "ended-old"}
					}
				}
				dir := t.TempDir()
				raw, err := doc.encode()
				if err != nil {
					t.Fatal(err)
				}
				writeLayoutFile(t, dir, string(raw))
				rowsOf := func(v RailView) ([]RailRow, []RailRow) {
					t.Helper()
					for _, w := range v.Workspaces {
						if w.Path != ws {
							continue
						}
						if group == "plain" {
							return w.Sessions, w.Ended
						}
						for _, wt := range w.Worktrees {
							if wt.Path == project {
								return wt.Sessions, wt.Ended
							}
						}
					}
					t.Fatal("admission group missing")
					return nil, nil
				}
				var ordinary, stars []string
				for _, newcomer := range []string{"", "new-z", "new-star-z", "new-a", "new-star-a"} {
					if newcomer != "" {
						add(newcomer, time.Time{})
						if strings.HasPrefix(newcomer, "new-star") {
							stars = append(stars, newcomer)
						} else {
							ordinary = append(ordinary, newcomer)
						}
					}
					// Registry enumeration order must not determine admission order.
					shuffled := slices.Clone(inputs)
					slices.Reverse(shuffled)
					for _, phase := range []string{"admission", "swap", "reload"} {
						current := doc
						if phase == "reload" {
							current, _ = openRailLayoutStore(dir).current()
						}
						v := buildRailView(shuffled, repos, nil, now)
						base, ended := rowsOf(v)
						wantBase := []string{"first", "second", "parent", "child-one", "child-two", "star-old", "star-parent", "star-child"}
						for _, in := range inputs[10:] {
							wantBase = append(wantBase, in.Session.Registration.ReplyID)
						}
						if !slices.Equal(railIDs(base), wantBase) || !slices.Equal(railIDs(ended), []string{"ended-old", "ended-new"}) {
							t.Fatalf("%s %s: base live %v, ended %v", newcomer, phase, railIDs(base), railIDs(ended))
						}
						before, _ := current.encode()
						orderRailView(&v, current, railFactsOf(v))
						live, ended := rowsOf(v)
						want := slices.Clone(wantBase)
						wantEnded := []string{"ended-old", "ended-new"}
						if mode != "empty" {
							prefix := []string{"star-old", "star-parent"}
							tail := []string{"first", "second", "parent"}
							if mode == "saved" {
								prefix = []string{"star-parent", "star-old"}
								tail = []string{"second", "parent", "first"}
								// Ended display order ignores manual order, and their
								// ranks used by old operations also remain unchanged.
								scope := current.scope(ws, railFactsOf(buildRailView(shuffled, repos, nil, now)), false)
								if scope.rank("ended-new") != -1 || scope.rank("ended-old") != 5 {
									t.Fatal("admission changed ended ranks")
								}
							}
							want = slices.Concat(prefix, stars, tail, ordinary)
							for _, r := range live {
								if r.ReplyID == "parent" && (!slices.Equal(railIDs(r.Children), []string{"child-two", "child-one"}) || !r.HasChildren) {
									t.Fatalf("parent unit changed: %+v", r)
								}
								if r.ReplyID == "star-parent" && (!r.Starred || !slices.Equal(railIDs(r.Children), []string{"star-child"})) {
									t.Fatalf("starred unit changed: %+v", r)
								}
								for _, child := range r.Children {
									if !child.Child || child.Starred || !child.StarHidden {
										t.Fatalf("child star/placement changed: %+v", child)
									}
								}
							}
						}
						if !slices.Equal(railIDs(live), want) || !slices.Equal(railIDs(ended), wantEnded) {
							t.Fatalf("%s %s: live %v, want %v; ended %v, want %v", newcomer, phase, railIDs(live), want, railIDs(ended), wantEnded)
						}
						if after, _ := current.encode(); !bytes.Equal(before, after) {
							t.Fatal("admission render mutated saved layout")
						}
					}
				}
			})
		}
	}
}

func TestRailAdmissionPreservesRevival(t *testing.T) {
	now := time.Now()
	path := filepath.Clean("/w/plain")
	doc := emptyLayout()
	doc.Workspaces[path] = &railLayoutWS{Order: []string{"first", "parent", "last"}, Children: map[string][]string{"parent": {"child"}}}
	before, err := doc.encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, ended := range []time.Time{now.Add(-time.Minute), {}} {
		v := buildRailView([]railInput{
			{Session: railSession("new", path, now, time.Time{})},
			{Session: railSession("child", path, now.Add(-time.Hour), time.Time{})},
			{Session: railSession("last", path, now.Add(-2*time.Hour), time.Time{})},
			{Session: railSession("parent", path, now.Add(-3*time.Hour), ended)},
			{Session: railSession("first", path, now.Add(-4*time.Hour), time.Time{})},
		}, nil, nil, now)
		orderRailView(&v, doc, railFactsOf(v))
		rows := v.Workspaces[0].Sessions
		want := []string{"first", "child", "last", "new"}
		if ended.IsZero() {
			want = []string{"first", "parent", "last", "new"}
			if !slices.Equal(railIDs(rows[1].Children), []string{"child"}) {
				t.Fatal("revival failed to restore dormant child")
			}
		}
		if !slices.Equal(railIDs(rows), want) {
			t.Fatalf("ended=%v: %v, want %v", ended, railIDs(rows), want)
		}
		if after, _ := doc.encode(); !bytes.Equal(before, after) {
			t.Fatal("orphan/admission/revival dropped dormant link")
		}
	}
}
