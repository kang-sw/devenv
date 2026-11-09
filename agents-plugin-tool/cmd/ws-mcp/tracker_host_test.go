package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// trackerSecretRemote is a remote URL whose userinfo carries a token.
const trackerSecretRemote = "https://user:secret-token@example.invalid/x.git"

// isolateGit points git at an empty global config, so a developer's
// signing defaults, hooks, or url rewrites do not reach the test's stores.
func isolateGit(t *testing.T) {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[init]\n\tdefaultBranch = main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// offlineRunner runs git, but every command that would reach a remote fails
// the way an unreachable https remote does, naming the URL with its
// credentials, so no test touches the network.
type offlineRunner struct{ inner wsgit.Runner }

func (r offlineRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	for _, a := range args {
		switch a {
		case "fetch", "push", "ls-remote", "clone":
			return nil, fmt.Errorf("git %s: exit status 128: fatal: unable to access '%s/': Could not resolve host: example.invalid", strings.Join(args, " "), trackerSecretRemote)
		}
	}
	return r.inner.RunGit(ctx, root, args...)
}

// newTestTracker returns a dashboardTracker over a real host in a temp
// root with one no-remote store "team" registered by member ada.
func newTestTracker(t *testing.T) (*dashboardTracker, string) {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	h, err := wstrackerhost.New(wstrackerhost.Options{Root: root, Runner: offlineRunner{inner: wsgit.ExecRunner{}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	dt := &dashboardTracker{h: h}
	if _, err := dt.Register(context.Background(), wsdashboard.TrackerRegisterRequest{ID: "team", Label: "Team", Member: "ada", Profile: &wsdashboard.TrackerProfile{Given: "Ada"}}); err != nil {
		t.Fatal(err)
	}
	return dt, root
}

func TestDashboardTrackerRoundTrip(t *testing.T) {
	dt, _ := newTestTracker(t)
	ctx := context.Background()

	stores, err := dt.Stores()
	if err != nil || !reflect.DeepEqual(stores, []wsdashboard.TrackerStore{{ID: "team", Label: "Team", Member: "ada"}}) {
		t.Fatalf("Stores = %+v, %v", stores, err)
	}

	create, _ := json.Marshal(map[string]any{"kind": "task", "title": "from the page"})
	comment, _ := json.Marshal(map[string]any{"item": "$1", "body": "on it"})
	ids, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{
		{Type: wstracker.TypeItemCreate, Payload: create, Origin: "agent"},
		{Type: wstracker.TypeCommentAdd, Payload: comment},
	})
	if err != nil || len(ids) != 2 {
		t.Fatalf("Dispatch = %v, %v", ids, err)
	}

	if err := dt.MarkSeen("team", wsdashboard.TrackerSeen{Badge: []string{ids[0]}, Read: []string{ids[1]}}); err != nil {
		t.Fatal(err)
	}
	v, err := dt.View(ctx, "team")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Seen, wsdashboard.TrackerSeen{Badge: []string{ids[0]}, Read: []string{ids[1]}}) {
		t.Fatalf("seen = %+v", v.Seen)
	}
	var view struct {
		Items    map[string]json.RawMessage `json:"items"`
		Comments map[string]struct {
			Item string `json:"item"`
		} `json:"comments"`
		Events []struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
		} `json:"events"`
	}
	if err := json.Unmarshal(v.View, &view); err != nil {
		t.Fatalf("view JSON: %v\n%s", err, v.View)
	}
	if _, ok := view.Items[ids[0]]; !ok {
		t.Fatalf("view lacks item %s: %s", ids[0], v.View)
	}
	if got := view.Comments[ids[1]].Item; got != ids[0] {
		t.Fatalf("the $1 comment is on %q, want %q", got, ids[0])
	}
	origins := map[string]string{}
	for _, ev := range view.Events {
		origins[ev.ID] = ev.Origin
	}
	if origins[ids[0]] != wstracker.OriginHuman || origins[ids[1]] != wstracker.OriginHuman {
		t.Fatalf("a page event must be human: origins %v", origins)
	}

	// A no-remote store's check and sync are reports, not errors.
	for name, run := range map[string]func(context.Context, string) (wsdashboard.TrackerSyncReport, error){"check": dt.Check, "sync": dt.Sync} {
		rep, err := run(ctx, "team")
		if err != nil || !rep.NoRemote || rep.Offline || rep.Error != "" {
			t.Fatalf("%s = %+v, %v", name, rep, err)
		}
	}

	profiles, err := dt.Profiles()
	if err != nil || len(profiles.Profiles) != 1 || profiles.Profiles[0].Given != "Ada" || profiles.Last != 0 {
		t.Fatalf("Profiles = %+v, %v", profiles, err)
	}
	if err := dt.SaveProfile(wsdashboard.TrackerProfile{Org: "Acme", Given: "Grace"}); err != nil {
		t.Fatal(err)
	}
	if profiles, _ = dt.Profiles(); len(profiles.Profiles) != 2 || profiles.Last != 1 || profiles.Profiles[1] != (wsdashboard.TrackerProfile{Org: "Acme", Given: "Grace"}) {
		t.Fatalf("after save, Profiles = %+v", profiles)
	}
	if err := dt.SaveProfile(wsdashboard.TrackerProfile{}); err == nil {
		t.Fatal("a profile with no given name must be refused")
	}

	if _, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{{Type: wstracker.TypeCommentAdd, Payload: json.RawMessage(`{"item":"nobody@pc:99","body":"x"}`)}}); err == nil {
		t.Fatal("a comment on a missing item must be refused")
	} else {
		var refusal *wsdashboard.TrackerRefusal
		if !errors.As(err, &refusal) || refusal.Type != wstracker.TypeCommentAdd || refusal.Reason == "" {
			t.Fatalf("refusal = %#v (%v)", err, err)
		}
	}
	if _, err := dt.View(ctx, "nope"); err == nil {
		t.Fatal("an unknown store's view must error")
	}
}

// TestDashboardTrackerRedactsAndReportsOffline: a store whose remote carries
// a token never shows the token, and its failed fetch is the offline report,
// not an error.
func TestDashboardTrackerRedactsAndReportsOffline(t *testing.T) {
	dt, root := newTestTracker(t)
	ctx := context.Background()

	// Give "team" a remote: in the registry, and as its repository's origin.
	path := filepath.Join(root, "stores.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	entry := doc["stores"].([]any)[0].(map[string]any)
	entry["remote"] = trackerSecretRemote
	raw, _ = json.Marshal(doc)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(entry["dir"].(string), "repo.git")
	if out, err := exec.Command("git", "-C", repo, "config", "remote.origin.url", trackerSecretRemote).CombinedOutput(); err != nil {
		t.Fatalf("set origin: %v: %s", err, out)
	}
	// A fresh host, as the daemon after a restart, reads the new entry.
	h, err := wstrackerhost.New(wstrackerhost.Options{Root: root, Runner: offlineRunner{inner: wsgit.ExecRunner{}}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	dt = &dashboardTracker{h: h}

	for name, run := range map[string]func(context.Context, string) (wsdashboard.TrackerSyncReport, error){"check": dt.Check, "sync": dt.Sync} {
		rep, err := run(ctx, "team")
		if err != nil || !rep.Offline || rep.NoRemote || rep.Error != "" {
			t.Fatalf("%s of an unreachable remote = %+v, %v; want the offline report with no error", name, rep, err)
		}
	}

	stores, err := dt.Stores()
	if err != nil || len(stores) != 1 || stores[0].Remote != "https://***@example.invalid/x.git" {
		t.Fatalf("Stores = %+v, %v", stores, err)
	}
	v, err := dt.View(ctx, "team")
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Store struct {
			Remote string `json:"remote"`
		} `json:"store"`
		Sync struct {
			Error string `json:"error"`
		} `json:"sync"`
	}
	if err := json.Unmarshal(v.View, &view); err != nil {
		t.Fatal(err)
	}
	if view.Store.Remote != "https://***@example.invalid/x.git" || !strings.Contains(view.Sync.Error, "https://***@example.invalid") {
		t.Fatalf("view store.remote %q, sync.error %q; want both redacted", view.Store.Remote, view.Sync.Error)
	}
	if strings.Contains(string(v.View), "secret-token") {
		t.Fatalf("the view JSON carries the token: %s", v.View)
	}

	// A registration whose clone fails reports the error without the token.
	_, err = dt.Register(ctx, wsdashboard.TrackerRegisterRequest{ID: "other", Remote: trackerSecretRemote, Member: "ada"})
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("Register error = %v; want an error without the token", err)
	}
}

func TestDashboardTrackerWithoutAHostReturnsItsError(t *testing.T) {
	dt := &dashboardTracker{err: errors.New("tracker root: cannot resolve " + trackerSecretRemote)}
	ctx := context.Background()
	check := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "cannot resolve https://***@example.invalid") || strings.Contains(err.Error(), "secret-token") {
			t.Errorf("%s = %v, want the creation error, redacted", name, err)
		}
	}
	_, err := dt.Stores()
	check("Stores", err)
	_, err = dt.View(ctx, "team")
	check("View", err)
	_, err = dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{{Type: "x"}})
	check("Dispatch", err)
	_, err = dt.Check(ctx, "team")
	check("Check", err)
	_, err = dt.Sync(ctx, "team")
	check("Sync", err)
	check("MarkSeen", dt.MarkSeen("team", wsdashboard.TrackerSeen{}))
	_, err = dt.Profiles()
	check("Profiles", err)
	check("SaveProfile", dt.SaveProfile(wsdashboard.TrackerProfile{Given: "Ada"}))
	_, err = dt.Register(ctx, wsdashboard.TrackerRegisterRequest{ID: "team"})
	check("Register", err)
	_, err = dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: "x"})
	check("Render", err)
}

// TestDashboardTrackerRenderMatchesTheView: an editor's preview renders
// through the renderer the view's html uses, safe mode included, so what
// the preview shows is what the saved text shows.
func TestDashboardTrackerRenderMatchesTheView(t *testing.T) {
	dt, _ := newTestTracker(t)
	ctx := context.Background()
	text := "# heading\n\n**bold** <script>alert(1)</script> ![alt](shot.png) [x](https://example.com/a)"
	create, _ := json.Marshal(map[string]any{"kind": "task", "title": "md", "body": text})
	ids, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{{Type: wstracker.TypeItemCreate, Payload: create}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
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
	if want := view.HTML.Items[ids[0]]; got != want || want == "" {
		t.Fatalf("Render = %q, want the view's html %q", got, want)
	}
	for _, absent := range []string{"<script", "<img"} {
		if strings.Contains(got, absent) {
			t.Errorf("Render = %q holds %q; safe mode lost", got, absent)
		}
	}
	if !strings.Contains(got, "<h1>heading</h1>") || !strings.Contains(got, `target="_blank"`) {
		t.Errorf("Render = %q, want the markdown rendered", got)
	}
	if empty, err := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{}); err != nil || empty != "" {
		t.Errorf("Render of no text = %q, %v; want empty", empty, err)
	}
}

// TestDashboardTrackerViewHTML: the view JSON carries each item body and
// each shown comment text rendered as markdown under html, beside the view
// state's own fields, which keep their plain text.
func TestDashboardTrackerViewHTML(t *testing.T) {
	dt, _ := newTestTracker(t)
	ctx := context.Background()

	create, _ := json.Marshal(map[string]any{"kind": "task", "title": "md", "body": "# heading\n\n- one\n- two"})
	bare, _ := json.Marshal(map[string]any{"kind": "task", "title": "no body"})
	comment, _ := json.Marshal(map[string]any{"item": "$1", "body": "**bold** <script>alert(1)</script>"})
	gone, _ := json.Marshal(map[string]any{"item": "$1", "body": "to hide"})
	ids, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{
		{Type: wstracker.TypeItemCreate, Payload: create},
		{Type: wstracker.TypeItemCreate, Payload: bare},
		{Type: wstracker.TypeCommentAdd, Payload: comment},
		{Type: wstracker.TypeCommentAdd, Payload: gone},
	})
	if err != nil {
		t.Fatal(err)
	}
	hide, _ := json.Marshal(map[string]any{"comment": ids[3]})
	if _, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{{Type: wstracker.TypeCommentHide, Payload: hide}}); err != nil {
		t.Fatal(err)
	}
	v, err := dt.View(ctx, "team")
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Items map[string]struct {
			Body string `json:"body"`
		} `json:"items"`
		Store struct {
			Name string `json:"name"`
		} `json:"store"`
		HTML struct {
			Items    map[string]string `json:"items"`
			Comments map[string]string `json:"comments"`
		} `json:"html"`
	}
	if err := json.Unmarshal(v.View, &view); err != nil {
		t.Fatalf("view JSON: %v\n%s", err, v.View)
	}
	if view.Store.Name != "team" || view.Items[ids[0]].Body != "# heading\n\n- one\n- two" {
		t.Fatalf("the view state's own fields changed: %s", v.View)
	}
	if got, want := view.HTML.Items[ids[0]], "<h1>heading</h1>\n<ul>\n<li>one</li>\n<li>two</li>\n</ul>"; got != want {
		t.Fatalf("item body html = %q, want %q", got, want)
	}
	if _, ok := view.HTML.Items[ids[1]]; ok {
		t.Fatalf("an item with no body has html: %v", view.HTML.Items)
	}
	if got, want := view.HTML.Comments[ids[2]], "<strong>bold</strong> &lt;script&gt;alert(1)&lt;/script&gt;"; got != want {
		t.Fatalf("comment html = %q, want %q", got, want)
	}
	if _, ok := view.HTML.Comments[ids[3]]; ok {
		t.Fatalf("a hidden comment has html: %v", view.HTML.Comments)
	}
}

// TestTrackerTextSafeMode: tracker text is other members' input, so typed
// HTML, script URLs, local files, and images never reach the page as
// markup; only an http(s) link is an anchor, opening in a new tab.
func TestTrackerTextSafeMode(t *testing.T) {
	inert := map[string]struct {
		in      string
		absent  []string
		present []string
	}{
		"script tag":       {"<script>alert(1)</script>", []string{"<script"}, []string{"&lt;script&gt;"}},
		"script block":     {"<script>\nalert(1)\n</script>", []string{"<script"}, []string{"&lt;script&gt;"}},
		"img onerror":      {"<img src=x onerror=alert(1)>", []string{"<img"}, []string{"&lt;img src=x onerror=alert(1)&gt;"}},
		"raw div":          {"<div>hi</div>", []string{"<div"}, []string{"&lt;div&gt;hi&lt;/div&gt;"}},
		"div block":        {"<div>\nhi\n</div>", []string{"<div"}, []string{"&lt;div&gt;"}},
		"javascript link":  {"[x](javascript:alert(1))", []string{"<a", "javascript:"}, []string{"x"}},
		"entity scheme":    {"[x](java&#x73;cript:alert(1))", []string{"<a"}, nil},
		"javascript auto":  {"<javascript:alert(1)>", []string{"<a"}, nil},
		"data link":        {"[x](data:text/html,<b>x</b>)", []string{"<a"}, nil},
		"file link":        {"[x](file:///etc/passwd)", []string{"<a", "href"}, []string{"/etc/passwd"}},
		"path link":        {"[x](/etc/passwd)", []string{"<a", "href"}, nil},
		"relative link":    {"[x](notes.md)", []string{"<a", "href"}, nil},
		"bare path":        {"see /etc/passwd", []string{"<a", "href"}, []string{"/etc/passwd"}},
		"network image":    {"![alt](https://example.com/x.png)", []string{"<img"}, []string{"alt"}},
		"local image":      {"![alt](shot.png)", []string{"<img"}, []string{"alt"}},
		"item reference":   {"see {{12}}", []string{"<a"}, []string{"{{12}}"}},
		"mermaid":          {"```mermaid\ngraph TD; a-->b\n```", []string{"class=\"mermaid\""}, []string{"<pre>"}},
		"onclick in title": {`[x](https://example.com "a\" onclick=\"alert(1)")`, []string{`" onclick`}, nil},
	}
	for name, c := range inert {
		got := string(trackerText(c.in))
		for _, s := range c.absent {
			if strings.Contains(got, s) {
				t.Errorf("%s: %q renders %q, which holds %q", name, c.in, got, s)
			}
		}
		for _, s := range c.present {
			if !strings.Contains(got, s) {
				t.Errorf("%s: %q renders %q, which lacks %q", name, c.in, got, s)
			}
		}
	}
	for _, in := range []string{"[x](https://example.com/a)", "https://example.com/a"} {
		got := string(trackerText(in))
		if !strings.Contains(got, `<a href="https://example.com/a" target="_blank" rel="noopener noreferrer">`) {
			t.Errorf("%q renders %q, want an anchor opening in a new tab", in, got)
		}
	}
}
