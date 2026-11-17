package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// countRunner counts the git runs of the runner it wraps.
type countRunner struct {
	inner wsgit.Runner
	n     atomic.Int64
}

func (r *countRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.n.Add(1)
	return r.inner.RunGit(ctx, root, args...)
}

// BenchmarkDashboardTrackerViewMiss times the parts of a tracker view
// cache miss (the view route's d.tracker.View) on a store whose refs and
// journal settled, so the host's reload runs no git: the host view, the
// whole dashboard view, and its markdown rendering, view JSON, view tag,
// and answer encoding apart; git-run is one git process, the unit the
// reload no longer pays. The store holds benchItems items, each with a
// markdown body and one comment.
func BenchmarkDashboardTrackerViewMiss(b *testing.B) {
	const benchItems = 120
	isolateGit(b)
	ctx := context.Background()
	runner := &countRunner{inner: offlineRunner{inner: wsgit.ExecRunner{}}}
	h, err := wstrackerhost.New(wstrackerhost.Options{Root: b.TempDir(), Runner: runner})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { h.Close() })
	dt := &dashboardTracker{h: h}
	if _, err := dt.Register(ctx, wsdashboard.TrackerRegisterRequest{ID: "team", Label: "Team", Member: "ada", Profile: &wsdashboard.TrackerProfile{Given: "Ada"}}); err != nil {
		b.Fatal(err)
	}
	body := strings.Repeat("Some **markdown** text with a [link](https://example.invalid/x) and `code`.\n\n- one\n- two\n\n", 7)
	var events []wsdashboard.TrackerEvent
	for i := range benchItems {
		create, _ := json.Marshal(map[string]any{"kind": "task", "title": fmt.Sprintf("item %d", i), "body": body})
		comment, _ := json.Marshal(map[string]any{"item": fmt.Sprintf("$%d", len(events)+1), "body": body[:300]})
		events = append(events, wsdashboard.TrackerEvent{Type: wstracker.TypeItemCreate, Payload: create}, wsdashboard.TrackerEvent{Type: wstracker.TypeCommentAdd, Payload: comment})
	}
	if _, err := dt.Dispatch(ctx, "team", events); err != nil {
		b.Fatal(err)
	}
	if _, err := h.Sync(ctx, "team"); err != nil {
		b.Fatal(err)
	}
	e, err := h.Entry("team")
	if err != nil {
		b.Fatal(err)
	}
	at := time.Now().Add(-time.Hour)
	if err := filepath.WalkDir(e.Dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, at, at)
	}); err != nil {
		b.Fatal(err)
	}
	v, err := h.View(ctx, "team", 0) // settles the stamps
	if err != nil {
		b.Fatal(err)
	}
	html := renderViewHTML("team", v)
	raw, err := json.Marshal(dashboardView{StoreView: v, HTML: html})
	if err != nil {
		b.Fatal(err)
	}
	tv, err := dt.View(ctx, "team")
	if err != nil {
		b.Fatal(err)
	}

	run := func(name string, f func() error) {
		b.Run(name, func(b *testing.B) {
			runner.n.Store(0)
			b.ResetTimer()
			for range b.N {
				if err := f(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(runner.n.Load())/float64(b.N), "git/op")
			b.ReportMetric(float64(len(raw)), "view-bytes")
		})
	}
	run("host-view", func() error { _, err := h.View(ctx, "team", 0); return err })
	// The refs' stamp moves on every run (a settled time, a new one each
	// run) though no ref did: the reload lists the refs and counts the
	// commits again, which is what every miss paid before the stamps.
	genesis := filepath.Join(e.Dir, "repo.git", "refs", "heads", "tracker", "genesis")
	moved := 0
	run("host-view-refs-moved", func() error {
		moved++
		t := at.Add(time.Duration(moved) * time.Millisecond)
		if err := os.Chtimes(genesis, t, t); err != nil {
			return err
		}
		_, err := h.View(ctx, "team", 0)
		return err
	})
	run("dashboard-view", func() error { _, err := dt.View(ctx, "team"); return err })
	run("render-html", func() error { renderViewHTML("team", v); return nil })
	run("view-json", func() error { _, err := json.Marshal(dashboardView{StoreView: v, HTML: html}); return err })
	run("view-tag", func() error { _, err := dashboardViewTag(v, html); return err })
	run("answer-encode", func() error { _, err := json.Marshal(tv); return err })
	run("git-run", func() error {
		_, err := runner.RunGit(ctx, filepath.Join(e.Dir, "repo.git"), "rev-list", "--count", "--glob=refs/heads/tracker/m/*")
		return err
	})
}

// TestDashboardTrackerViewRendersAgain: the view's rendered texts are
// reused between builds (viewRenders) yet follow every change: a view
// built again renders as renderViewHTML does, and an edited body shows its
// new rendering on the next view.
func TestDashboardTrackerViewRendersAgain(t *testing.T) {
	dt, _ := newTestTracker(t)
	ctx := context.Background()
	create, _ := json.Marshal(map[string]any{"kind": "task", "title": "md", "body": "first **body**"})
	other, _ := json.Marshal(map[string]any{"kind": "task", "title": "same", "body": "first **body**"})
	ids, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{{Type: wstracker.TypeItemCreate, Payload: create}, {Type: wstracker.TypeItemCreate, Payload: other}})
	if err != nil {
		t.Fatal(err)
	}
	html := func() map[string]string {
		t.Helper()
		v, err := dt.View(ctx, "team")
		if err != nil {
			t.Fatal(err)
		}
		var view struct {
			HTML struct {
				Items map[string]string `json:"items"`
			} `json:"html"`
		}
		if err := json.Unmarshal(v.View, &view); err != nil {
			t.Fatal(err)
		}
		return view.HTML.Items
	}
	first := html()
	if !strings.Contains(first[ids[0]], "<strong>body</strong>") || first[ids[1]] != first[ids[0]] {
		t.Fatalf("first view html = %q", first)
	}
	if again := html(); again[ids[0]] != first[ids[0]] || again[ids[1]] != first[ids[1]] {
		t.Fatalf("a view built again rendered %q, was %q", again, first)
	}
	edit, _ := json.Marshal(map[string]any{"item": ids[0], "body": "second *body*"})
	if _, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{{Type: wstracker.TypeItemEdit, Payload: edit}}); err != nil {
		t.Fatal(err)
	}
	edited := html()
	if !strings.Contains(edited[ids[0]], "<em>body</em>") || edited[ids[1]] != first[ids[1]] {
		t.Fatalf("after the edit: html = %q", edited)
	}
	v, err := dt.h.View(ctx, "team", 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := renderViewHTML("team", v); string(want.Items[ids[0]]) != edited[ids[0]] {
		t.Fatalf("cached rendering %q, renderViewHTML %q", edited[ids[0]], want.Items[ids[0]])
	}
}
