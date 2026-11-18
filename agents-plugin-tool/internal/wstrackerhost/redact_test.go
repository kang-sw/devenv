package wstrackerhost

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wstracker"
)

// TestViewWithinReadersRedactApart: concurrent ViewWithin readers each get
// their own view to Redact, both when they join one reload and when they
// are served the last folded state, so a page's redaction never writes a
// view another reader holds. The store has a credentialed remote, sync
// error, and wiki warning, so a shared view would show up as a redacted
// cached state even without go test -race.
func TestViewWithinReadersRedactApart(t *testing.T) {
	w := newWorld(t)
	h := w.machine("ada")
	res := w.register(h, "team", "ada", &wstracker.Profile{Given: "Ada"})
	_ = within(t, h, "team")

	// View reads the registry and host-sync.json and never calls the
	// remote, so an unreachable credentialed URL is safe here.
	const cred = "https://user:secret@example.com/r.git"
	if _, err := h.editEntry("team", func(e *StoreEntry) error { e.Remote = cred; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(res.Entry.Dir, syncStateFile), syncState{Error: "fetch " + cred, WikiWarnings: []string{"wiki/x: push " + cred}}); err != nil {
		t.Fatal(err)
	}
	st, err := h.store("team")
	if err != nil {
		t.Fatal(err)
	}
	// cached checks the last folded state still holds every credential: a
	// reader's Redact must never reach it.
	cached := func(round string) {
		t.Helper()
		st.viewMu.Lock()
		defer st.viewMu.Unlock()
		s := st.snap
		if s == nil || s.Store.Remote == nil || *s.Store.Remote != cred || s.Sync.Error != "fetch "+cred ||
			len(s.Sync.WikiWarnings) != 1 || s.Sync.WikiWarnings[0] != "wiki/x: push "+cred {
			t.Fatalf("%s: the cached state lost its credentials (a reader redacted it): %+v", round, s)
		}
	}

	readers := func(round string) {
		t.Helper()
		views := make([]*StoreView, 8)
		var wg sync.WaitGroup
		for i := range views {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, err := h.ViewWithin(context.Background(), "team", 0, pageWait)
				if err != nil {
					t.Errorf("%s: view within: %v", round, err)
					return
				}
				v.Redact()
				views[i] = v
			}()
		}
		wg.Wait()
		for i, v := range views {
			if v == nil {
				continue
			}
			text := v.Sync.Error + strings.Join(v.Sync.WikiWarnings, " ")
			if v.Store.Remote != nil {
				text += *v.Store.Remote
			}
			if strings.Contains(text, "secret") {
				t.Fatalf("%s: reader %d holds an unredacted view: %q", round, i, text)
			}
			for _, other := range views[:i] {
				if v == other {
					t.Fatalf("%s: two readers were handed one view", round)
				}
			}
		}
	}
	readers("joined reload")

	// With the store's lock held, every read stops waiting and is served
	// the last folded state. A reload the first round left running holds
	// that lock while it reads, so wait for it to land first.
	waitFor(t, 10*time.Second, "the first round's reloads to land", func() bool {
		st.viewMu.Lock()
		defer st.viewMu.Unlock()
		return st.cur == nil && st.next == nil
	})
	cached("joined reload")
	lock := flock.New(filepath.Join(res.Entry.Dir, "lock"))
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("take the store lock: %v %v", ok, err)
	}
	defer lock.Unlock()
	readers("last folded state")
	cached("last folded state")
}

func TestRedactsRemoteCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://x-access-token:ghp_secret@github.com/o/r.git": "https://***@github.com/o/r.git",
		"fatal: could not read from HTTP://tok@host/r":         "fatal: could not read from HTTP://***@host/r",
		"git@github.com:o/r.git":                               "git@github.com:o/r.git",
		"ssh://git@github.com/o/r.git":                         "ssh://git@github.com/o/r.git",
		"https://github.com/o/r.git":                           "https://github.com/o/r.git",
		"https://user:p@ss@host/x.git":                         "https://***@host/x.git",
		"clone https://u:a@b@c@host/r and https://h/p?q=a@b":   "clone https://***@host/r and https://h/p?q=***",
		"https://host/r.git?private_token=glpat-x&ref=main":    "https://host/r.git?private_token=***&ref=***",
		"fetch 'https://u:p@host/r?access_token=abc' failed":   "fetch 'https://***@host/r?access_token=***' failed",
		"https://host/r.git?sigsecret#frag":                    "https://host/r.git?***#frag",
		"see https://host/docs?q=1 and ssh://h/r?x=1":          "see https://host/docs?q=*** and ssh://h/r?x=1",
	} {
		if got := RedactCredentials(in); got != want {
			t.Fatalf("RedactCredentials(%q) = %q, want %q", in, got, want)
		}
	}
	remote := "https://tok@example.com/r.git"
	v := &StoreView{View: &wstracker.View{}, Store: StoreInfo{Name: "s", Remote: &remote}}
	v.Sync.Error = "push to https://tok@example.com/r.git failed"
	v.Redact()
	if strings.Contains(*v.Store.Remote+v.Sync.Error, "tok") || remote != "https://tok@example.com/r.git" {
		t.Fatalf("the view's remote and error must be redacted without touching the entry: %q %q", *v.Store.Remote, v.Sync.Error)
	}
	(*StoreView)(nil).Redact() // must not panic
	var none StoreView
	none.Redact() // a view with no remote
}
