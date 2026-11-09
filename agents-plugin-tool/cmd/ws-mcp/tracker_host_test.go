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
		got := string(trackerText(c.in, nil))
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
		got := string(trackerText(in, nil))
		if !strings.Contains(got, `<a href="https://example.com/a" target="_blank" rel="noopener noreferrer">`) {
			t.Errorf("%q renders %q, want an anchor opening in a new tab", in, got)
		}
	}
	// The one opening: an attachment image naming the text's own
	// attachment. A foreign oid, a local path, a network image, and a link
	// to the attachment stay text even beside it.
	own, foreign := strings.Repeat("a", 64), strings.Repeat("b", 64)
	src := ownAttachments("team", []string{own})
	got := string(trackerText("![shot](attachment:"+own+")", src))
	if want := `<img src="/tracker/stores/team/attachments/` + own + `" alt="shot">`; !strings.Contains(got, want) {
		t.Errorf("own attachment renders %q, want %q", got, want)
	}
	for _, in := range []string{"![x](attachment:" + foreign + ")", "![x](shot.png)", "![x](https://example.com/x.png)", "[x](attachment:" + own + ")", "<img src=\"attachment:" + own + "\">"} {
		if got := string(trackerText(in, src)); strings.Contains(got, "<img") || strings.Contains(got, "href") {
			t.Errorf("%q with an own attachment list renders %q; want text", in, got)
		}
	}
	if got := string(trackerText("![x](attachment:"+own+")", ownAttachments("team", []string{"not-an-oid", ""}))); strings.Contains(got, "<img") {
		t.Errorf("an invalid own oid opened an image: %q", got)
	}
}

// TestDashboardTrackerAttachmentImages: a staged upload renders inline in
// a preview whose pending list names it, an unstaged oid in that list does
// not; the write commits the staged blob, after which the view renders the
// image in the body or comment that lists it and not in one that only
// names its oid.
func TestDashboardTrackerAttachmentImages(t *testing.T) {
	dt, _ := newTestTracker(t)
	ctx := context.Background()
	data := append([]byte("\x89PNG\r\n\x1a\n"), "shot"...)
	att, err := dt.StageAttachment(ctx, "team", "shot.png", data)
	if err != nil {
		t.Fatal(err)
	}
	ghost := wsdashboard.TrackerAttachment{OID: strings.Repeat("c", 64), Name: "ghost.png", Size: 1, Mime: "image/png"}
	img := `<img src="/tracker/stores/team/attachments/` + att.OID + `"`
	text := "look ![shot](attachment:" + att.OID + ") and ![ghost](attachment:" + ghost.OID + ")"

	got, err := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: text, Attachments: []wsdashboard.TrackerAttachment{att, ghost}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, img) || strings.Contains(got, ghost.OID) {
		t.Fatalf("preview = %q, want the staged image and not the unstaged one", got)
	}
	if got, _ := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: text}); strings.Contains(got, "<img") {
		t.Fatalf("preview without a pending list = %q, want no image", got)
	}

	meta := []map[string]any{{"oid": att.OID, "name": att.Name, "size": att.Size, "mime": att.Mime}}
	create, _ := json.Marshal(map[string]any{"kind": "task", "title": "img", "body": text, "attachments": meta})
	other, _ := json.Marshal(map[string]any{"kind": "task", "title": "borrower", "body": "![x](attachment:" + att.OID + ")"})
	comment, _ := json.Marshal(map[string]any{"item": "$1", "body": "again ![shot](attachment:" + att.OID + ")", "attachments": meta})
	ids, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{
		{Type: wstracker.TypeItemCreate, Payload: create},
		{Type: wstracker.TypeItemCreate, Payload: other},
		{Type: wstracker.TypeCommentAdd, Payload: comment},
	})
	if err != nil {
		t.Fatal(err)
	}
	if read, err := dt.ReadAttachment(ctx, "team", att.OID); err != nil || string(read) != string(data) {
		t.Fatalf("read after the write = %q, %v", read, err)
	}
	if got, _ := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: text, Attachments: []wsdashboard.TrackerAttachment{att}}); !strings.Contains(got, img) {
		t.Fatalf("preview of a stored oid = %q, want the image", got)
	}
	v, err := dt.View(ctx, "team")
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		HTML struct {
			Items    map[string]string `json:"items"`
			Comments map[string]string `json:"comments"`
		} `json:"html"`
	}
	if err := json.Unmarshal(v.View, &view); err != nil {
		t.Fatal(err)
	}
	if h := view.HTML.Items[ids[0]]; !strings.Contains(h, img) || strings.Contains(h, ghost.OID) {
		t.Errorf("item html = %q, want its own image only", h)
	}
	if h := view.HTML.Items[ids[1]]; strings.Contains(h, "<img") {
		t.Errorf("borrowing item html = %q, want no image", h)
	}
	if h := view.HTML.Comments[ids[2]]; !strings.Contains(h, img) {
		t.Errorf("comment html = %q, want its own image", h)
	}
}

// TestDashboardTrackerWiki: the wiki methods over a real host. A save
// creating a wiki names a staged upload, which lands in the wiki tree and
// shows in the page's render from the wiki image route; a staged but
// unsaved image shows in a wiki-mode preview from the attachment route, an
// unknown oid never, and a wiki's image not in an item's own list never
// in an item render; a wiki: link to a well-formed page is an anchor the
// page keys on, in a wiki page and in an item body alike, and a malformed
// one stays text. A stale base is the typed conflict, an expired image
// and a repeated new-wiki form are refusals, and an unknown wiki or page
// is not found.
func TestDashboardTrackerWiki(t *testing.T) {
	dt, _ := newTestTracker(t)
	ctx := context.Background()
	if list, err := dt.WikiList(ctx, "team"); err != nil || len(list) != 0 {
		t.Fatalf("list = %v, %v", list, err)
	}
	if _, err := dt.WikiPages(ctx, "team", "docs"); !errors.Is(err, wsdashboard.ErrTrackerWikiNotFound) {
		t.Fatalf("pages of an absent wiki = %v", err)
	}
	if _, err := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: "x", Wiki: "Docs"}); !errors.Is(err, wsdashboard.ErrTrackerWikiNotFound) {
		t.Fatalf("render of a malformed wiki name = %v", err)
	}

	data := append([]byte("\x89PNG\r\n\x1a\n"), "wiki"...)
	att, err := dt.StageAttachment(ctx, "team", "shot.png", data)
	if err != nil {
		t.Fatal(err)
	}
	ghost := wsdashboard.TrackerAttachment{OID: strings.Repeat("d", 64), Name: "ghost.png", Size: 1, Mime: "image/png"}
	text := "see [home](wiki:docs/guide/home) and [bad](wiki:Docs/x) ![shot](attachment:" + att.OID + ") ![ghost](attachment:" + ghost.OID + ")"
	link := `<a href="#" data-wiki="docs" data-page="guide/home">home</a>`
	staged := `<img src="/tracker/stores/team/attachments/` + att.OID + `"`
	inWiki := `<img src="/tracker/stores/team/wiki/docs/attachments/` + att.OID + `"`

	// The preview before the wiki exists: the staged image from the
	// attachment route, the link an anchor, the bad link text.
	got, err := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: text, Wiki: "docs", Attachments: []wsdashboard.TrackerAttachment{att, ghost}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, staged) || !strings.Contains(got, link) || strings.Contains(got, ghost.OID) || strings.Contains(got, `data-wiki="Docs"`) {
		t.Fatalf("preview = %q", got)
	}

	if _, err := dt.WikiSave(ctx, "team", "docs", wsdashboard.TrackerWikiSave{NewWiki: true, Op: "write", Path: "guide/home", Content: text, Attachments: []wsdashboard.TrackerAttachment{att, ghost}}); !errors.Is(err, wsdashboard.ErrTrackerWikiRefused) || !strings.Contains(err.Error(), ghost.OID) {
		t.Fatalf("save naming an expired image = %v, want a refusal naming it", err)
	}
	res, err := dt.WikiSave(ctx, "team", "docs", wsdashboard.TrackerWikiSave{NewWiki: true, Op: "write", Path: "guide/home", Content: text, Attachments: []wsdashboard.TrackerAttachment{att}})
	if err != nil || !res.Created || res.Commit == "" || res.Merged {
		t.Fatalf("save = %+v, %v", res, err)
	}
	if list, err := dt.WikiList(ctx, "team"); err != nil || len(list) != 1 || list[0].Name != "docs" || list[0].Tip != res.Commit || list[0].Unpushed {
		t.Fatalf("list = %+v, %v", list, err)
	}
	pages, err := dt.WikiPages(ctx, "team", "docs")
	if err != nil || len(pages.Pages) != 1 || pages.Pages[0] != "guide/home" || pages.Commit != res.Commit {
		t.Fatalf("pages = %+v, %v", pages, err)
	}
	page, err := dt.WikiRead(ctx, "team", "docs", "guide/home")
	if err != nil || page.Content != text || page.Commit != res.Commit || page.Path != "guide/home" {
		t.Fatalf("read = %+v, %v", page, err)
	}
	// the read renders the page in wiki mode: the tree's image shows
	if !strings.Contains(page.HTML, `<img src="/tracker/stores/team/wiki/docs/attachments/`+att.OID) {
		t.Fatalf("read html = %q", page.HTML)
	}
	if _, err := dt.WikiRead(ctx, "team", "docs", "nope"); !errors.Is(err, wsdashboard.ErrTrackerWikiNotFound) {
		t.Fatalf("read of an absent page = %v", err)
	}
	if img, err := dt.ReadWikiImage(ctx, "team", "docs", att.OID); err != nil || string(img) != string(data) {
		t.Fatalf("wiki image = %d bytes, %v", len(img), err)
	}

	// The saved page renders its image from the wiki, with no pending list.
	got, err = dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: page.Content, Wiki: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, inWiki) || strings.Contains(got, staged) || !strings.Contains(got, link) {
		t.Fatalf("page render = %q", got)
	}
	// An item preview does not reach into the wiki's tree, but links pages.
	if got, _ := dt.Render(ctx, "team", wsdashboard.TrackerRenderRequest{Text: page.Content}); strings.Contains(got, inWiki) || !strings.Contains(got, link) {
		t.Fatalf("item render = %q, want no wiki image and the link", got)
	}
	create, _ := json.Marshal(map[string]any{"kind": "task", "title": "linker", "body": "read [home](wiki:docs/guide/home)"})
	ids, err := dt.Dispatch(ctx, "team", []wsdashboard.TrackerEvent{{Type: wstracker.TypeItemCreate, Payload: create}})
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
	if h := view.HTML.Items[ids[0]]; !strings.Contains(h, link) {
		t.Errorf("item html = %q, want the wiki link", h)
	}

	// A second writer moved the tip; a save on the old base over the same
	// lines is the typed conflict with both texts, and nothing moved.
	if _, err := dt.WikiSave(ctx, "team", "docs", wsdashboard.TrackerWikiSave{Base: res.Commit, Op: "write", Path: "guide/home", Content: "theirs\n"}); err != nil {
		t.Fatal(err)
	}
	_, err = dt.WikiSave(ctx, "team", "docs", wsdashboard.TrackerWikiSave{Base: res.Commit, Op: "write", Path: "guide/home", Content: "ours\n"})
	var conflict *wsdashboard.TrackerWikiConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("stale save = %v, want a conflict", err)
	}
	if conflict.Path != "guide/home" || conflict.Base != res.Commit || conflict.Theirs == "" || conflict.OursText != "ours\n" || conflict.TheirsText != "theirs\n" || conflict.Kind == "" {
		t.Errorf("conflict = %+v", conflict)
	}
	if page, _ := dt.WikiRead(ctx, "team", "docs", "guide/home"); page.Content != "theirs\n" || page.Commit != conflict.Theirs {
		t.Errorf("page after the conflict = %+v", page)
	}
	for name, save := range map[string]wsdashboard.TrackerWikiSave{
		"new wiki twice": {NewWiki: true, Op: "write", Path: "x", Content: "x"},
		"no base":        {Op: "write", Path: "x", Content: "x"},
		"bad op":         {Base: conflict.Theirs, Op: "move", Path: "x"},
		"bad path":       {Base: conflict.Theirs, Op: "write", Path: "../x", Content: "x"},
		"no change":      {Base: conflict.Theirs, Op: "write", Path: "guide/home", Content: "theirs\n"},
	} {
		_, err := dt.WikiSave(ctx, "team", "docs", save)
		if errors.As(err, &conflict) && name == "no base" {
			continue // a missing base reads as stale
		}
		if !errors.Is(err, wsdashboard.ErrTrackerWikiRefused) {
			t.Errorf("%s = %v, want a refusal", name, err)
		}
	}
}
