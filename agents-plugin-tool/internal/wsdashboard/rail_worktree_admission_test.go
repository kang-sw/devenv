package wsdashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func admissionFixture(t *testing.T) (string, string, *railAdmissionTracker, func(string) (RepoWorktrees, error)) {
	t.Helper()
	base, root := worktreeRepo(t)
	path := filepath.Join(base, "detected")
	gitRun(t, root, "worktree", "add", "-q", "-b", "detected", path)
	tracker := newRailAdmissionTracker(openRailLayoutStore(t.TempDir()))
	read := func(root string) (RepoWorktrees, error) { return (gitWorktreeSource{}).ReadRepoWorktrees(root, nil) }
	return root, path, tracker, read
}
func observeAdmissions(t *testing.T, a *railAdmissionTracker, root string, got RepoWorktrees) map[string]admissionObservation {
	t.Helper()
	observed, err := a.observe(root, got)
	if err != nil {
		t.Fatal(err)
	}
	return observed
}

func admissionSelection(t *testing.T, a *railAdmissionTracker, read func(string) (RepoWorktrees, error), root, path, action string) railAdmissionSelection {
	t.Helper()
	got, err := read(root)
	if err != nil {
		t.Fatal(err)
	}
	obs := observeAdmissions(t, a, root, got)[path]
	if obs.revision == "" {
		t.Fatalf("no observation of %s", path)
	}
	return railAdmissionSelection{Repository: root, Path: path, Revision: obs.revision, Action: action}
}
func admissionRecords(t *testing.T, s *railLayoutStore) []railAdmissionRecord {
	t.Helper()
	doc, _ := s.current()
	records, err := railAdmissions(doc)
	if err != nil {
		t.Fatal(err)
	}
	return records
}
func TestRailAdmissionPersistenceAndAtomicBatch(t *testing.T) {
	root, path, a, read := admissionFixture(t)
	selection := admissionSelection(t, a, read, root, path, "reject")
	before := a.store.gen
	reply, err := a.apply([]railAdmissionSelection{selection}, read)
	if err != nil || !reply.Applied || reply.Gen <= before || !strings.Contains(reply.Prompt, path) {
		t.Fatalf("reject: %+v %v", reply, err)
	}
	if _, err := os.Stat(filepath.Join(path, "a")); err != nil {
		t.Fatalf("rejection touched worktree: %v", err)
	}
	reopened := openRailLayoutStore(filepath.Dir(a.store.file))
	if r := admissionAt(admissionRecords(t, reopened), root, path); r.State != "rejected" || r.Origin != "detected" {
		t.Fatalf("reopen: %+v", r)
	}
	// Another browser cannot overwrite an already applied classification.
	if v, e := a.apply([]railAdmissionSelection{selection}, read); !errors.Is(e, errAdmissionStale) || v.Applied {
		t.Fatalf("repeat: %+v %v", v, e)
	}
	selection.Action = "undo"
	if v, e := a.apply([]railAdmissionSelection{selection}, read); e != nil || !v.Applied {
		t.Fatalf("undo: %+v %v", v, e)
	}
	if admissionAt(admissionRecords(t, a.store), root, path).State != "pending" {
		t.Fatal("undo did not return pending")
	}
	other := filepath.Join(filepath.Dir(root), "other")
	gitRun(t, root, "worktree", "add", "-q", "-b", "other", other)
	one := admissionSelection(t, a, read, root, path, "approve")
	two := admissionSelection(t, a, read, root, other, "reject")
	two.Revision = "old-daemon-revision"
	raw, _ := os.ReadFile(a.store.file)
	if v, e := a.apply([]railAdmissionSelection{one, two}, read); !errors.Is(e, errAdmissionStale) || len(v.Stale) != 1 {
		t.Fatalf("batch: %+v %v", v, e)
	}
	after, _ := os.ReadFile(a.store.file)
	if string(raw) != string(after) {
		t.Fatal("stale batch partially wrote admission")
	}
	two = admissionSelection(t, a, read, root, other, "reject")
	if _, e := a.apply([]railAdmissionSelection{one, two}, read); e != nil {
		t.Fatal(e)
	}
	// Later rejection prompt includes the cumulative list, not just this batch.
	third := filepath.Join(filepath.Dir(root), "third")
	gitRun(t, root, "worktree", "add", "-q", "-b", "third", third)
	three := admissionSelection(t, a, read, root, third, "reject")
	v, e := a.apply([]railAdmissionSelection{three}, read)
	if e != nil || !strings.Contains(v.Prompt, other) || !strings.Contains(v.Prompt, third) {
		t.Fatalf("cumulative: %+v %v", v, e)
	}
}
func TestRailAdmissionIdentityFailureAndPruning(t *testing.T) {
	root, path, a, read := admissionFixture(t)
	selection := admissionSelection(t, a, read, root, path, "approve")
	if _, err := a.apply([]railAdmissionSelection{selection}, read); err != nil {
		t.Fatal(err)
	}
	// A failed repository list cannot prune or invent fresh observations.
	list, _ := read(root)
	l := newRailLister(func(string, []Registration) (RepoWorktrees, error) { return RepoWorktrees{}, errors.New("unreadable") }, nil, make(chan struct{}))
	l.admission = a
	if l.readRepo(root, nil).Read {
		t.Fatal("failed read was successful")
	}
	if len(admissionRecords(t, a.store)) != 1 {
		t.Fatal("failed read pruned admission")
	}
	// Broken association is unverifiable, not evidence that the directory is gone.
	marker := filepath.Join(path, ".git")
	original, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(marker, []byte("gitdir: /missing-admin\n"), 0600); err != nil {
		t.Fatal(err)
	}
	observeAdmissions(t, a, root, list)
	if len(admissionRecords(t, a.store)) != 1 {
		t.Fatal("unreadable association pruned admission")
	}
	selection.Action = "reject"
	if v, e := a.apply([]railAdmissionSelection{selection}, read); !errors.Is(e, errAdmissionStale) || v.Applied {
		t.Fatalf("association: %+v %v", v, e)
	}
	if err = os.WriteFile(marker, original, 0600); err != nil {
		t.Fatal(err)
	}
	// Existing handle evidence survives a temporary probe failure, but its old
	// revision cannot silently become fresh proof after recovery.
	got := observeAdmissions(t, a, root, list)[path]
	if got.revision == selection.Revision || got.revision == "" {
		t.Fatal("failed probe reused old revision")
	}
	if len(admissionRecords(t, a.store)) != 1 {
		t.Fatal("restored association lost admission")
	}
	// Replace the directory while retaining the old one so filesystem IDs cannot
	// immediately be reused. Git association bytes alone cannot hide replacement.
	old := path + "-old"
	if err = os.Rename(path, old); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(path, ".git"), original, 0600); err != nil {
		t.Fatal(err)
	}
	selection.Revision = got.revision
	if v, e := a.apply([]railAdmissionSelection{selection}, read); !errors.Is(e, errAdmissionStale) || v.Applied {
		t.Fatalf("replacement: %+v %v", v, e)
	}
	observeAdmissions(t, a, root, list)
	if len(admissionRecords(t, a.store)) != 0 {
		t.Fatal("observed replacement retained old admission")
	}
	// Confirmed disappearance prunes admission; a recreation is pending.
	fresh := admissionSelection(t, a, read, root, path, "approve")
	if _, err = a.apply([]railAdmissionSelection{fresh}, read); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	observeAdmissions(t, a, root, list)
	if len(admissionRecords(t, a.store)) != 0 {
		t.Fatal("confirmed missing path not pruned")
	}
	if err = os.Rename(old, path); err != nil {
		t.Fatal(err)
	}
	observeAdmissions(t, a, root, list)
	if admissionAt(admissionRecords(t, a.store), root, path).State != "pending" {
		t.Fatal("recreated path not pending")
	}
	restarted := newRailAdmissionTracker(a.store)
	if v, e := restarted.apply([]railAdmissionSelection{fresh}, read); !errors.Is(e, errAdmissionStale) || v.Applied {
		t.Fatalf("restart: %+v %v", v, e)
	}
}
func TestRailAdmissionRetriesInvalidationAfterWriteFailure(t *testing.T) {
	root, path, a, read := admissionFixture(t)
	selection := admissionSelection(t, a, read, root, path, "reject")
	if _, err := a.apply([]railAdmissionSelection{selection}, read); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(path, ".git"), marker, 0600); err != nil {
		t.Fatal(err)
	}
	l := newRailLister(func(root string, _ []Registration) (RepoWorktrees, error) { return read(root) }, nil, make(chan struct{}))
	l.admission = a
	dir := filepath.Dir(a.store.file)
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if err = os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	failed := l.readRepo(root, nil)
	if admissionAt(admissionRecords(t, a.store), root, path).State != "rejected" {
		t.Fatal("fixture did not inject a durable write failure")
	}
	t.Logf("injected prune write failure retained the durable rejection; reported Read=%v", failed.Read)
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	recovered := l.readRepo(root, nil)
	if !recovered.Read {
		t.Fatal("read did not recover after writes were restored")
	}
	if record := admissionAt(admissionRecords(t, a.store), root, path); record.State != "pending" {
		t.Fatalf("old rejection survived recreated-path recovery after failed prune: %+v", record)
	}
	if failed.Read {
		t.Fatal("prune write failure was not propagated as an unavailable read")
	}
}

func TestRailAdmissionUnknownFieldsAndClassification(t *testing.T) {
	root, path, a, read := admissionFixture(t)
	selectOne := admissionSelection(t, a, read, root, path, "approve")
	if _, err := a.apply([]railAdmissionSelection{selectOne}, read); err != nil {
		t.Fatal(err)
	}
	doc, err := readRailLayout(a.store.file)
	if err != nil {
		t.Fatal(err)
	}
	doc.extra["future_root"] = json.RawMessage(`{"kept":true}`)
	records, _ := railAdmissions(doc)
	records[0].extra = map[string]json.RawMessage{"future_record": json.RawMessage(`42`)}
	doc.extra[railAdmissionField], _ = json.Marshal(records)
	raw, _ := doc.encode()
	if err = os.WriteFile(a.store.file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// Simulate a schema-1 writer that knows only the existing layout operations.
	if _, err = a.store.apply(railLayoutRequest{Op: "star", RID: "a1"}, railFactsOf(layoutTestView()), true); err != nil {
		t.Fatal(err)
	}
	reopened, err := readRailLayout(a.store.file)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := reopened.encode()
	if !strings.Contains(string(encoded), "future_record") || !strings.Contains(string(encoded), "future_root") || reopened.SchemaVersion != 1 {
		t.Fatalf("unknown field loss: %s", encoded)
	}
	pool := filepath.Join(root, ".ws-worktrees", "pooled")
	view := buildRailView(nil, map[string]railRepo{root: {Read: true, Entries: []railWorktreeEntry{{Path: root}, {Path: path}, {Path: pool}}}}, []storedWorkspace{{Path: root, Repo: true}}, time.Now())
	classifyRailAdmissions(&view, reopened)
	if len(view.Workspaces) != 1 || len(view.Workspaces[0].Worktrees) != 2 {
		t.Fatalf("pool leaked: %+v", view)
	}
	for _, w := range view.Workspaces[0].Worktrees {
		if w.Admission != "admitted" {
			t.Fatalf("classification: %+v", w)
		}
	}
	empty := &railLayoutDoc{}
	classifyRailAdmissions(&view, empty)
	if view.Workspaces[0].Worktrees[1].Admission != "pending" || view.Workspaces[0].Worktrees[1].Origin != "detected" {
		t.Fatal("unrecorded discovery not pending")
	}
}

// A successful fresh Git list can omit a removed worktree entirely. This is
// different from the retained/prunable entry exercised by the earlier test.
func TestRailAdmissionRecreatedUserAddKeepsAdmission(t *testing.T) {
	f := newDeleteFixture(t)
	path := filepath.Join(f.tmp, "recreated-user")
	add := func(branch string) {
		t.Helper()
		raw, _ := json.Marshal(worktreeAddRequest{Workspace: f.root, Branch: branch, New: true, Folder: path})
		code, _, body := postJSON(t, f.base+worktreeAddRoute, f.base, string(raw))
		if code != http.StatusOK {
			t.Fatalf("dashboard Add %s: %d %s", branch, code, body)
		}
	}
	add("old-user")
	f.d.railBaseView(f.d.pages())
	f.d.rail.readRepo(f.root, nil)
	old, err := probeAdmission(f.root, path)
	if err != nil {
		t.Fatal(err)
	}
	// Retain the old directory at another path so the recreation cannot
	// indistinguishably reuse its filesystem identity during this regression.
	if err = os.Rename(path, path+"-retained-old"); err != nil {
		t.Fatal(err)
	}
	gitRun(t, f.root, "worktree", "remove", "--force", path)
	fresh, err := f.d.rail.read(f.root, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range fresh.Worktrees {
		if samePath(w.Path, path) {
			t.Fatal("fresh post-removal Git list still includes old path")
		}
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old path not confirmed absent: %v", err)
	}
	f.d.rail.readRepo(f.root, nil)
	if admissionAt(admissionRecords(t, f.d.layout), f.root, path).State != "pending" {
		t.Fatal("confirmed disappearance did not prune old admission")
	}
	t.Log("fresh Git list omitted the confirmed-absent old path; old admission was pruned")
	add("recreated-user")
	record := admissionAt(admissionRecords(t, f.d.layout), f.root, path)
	if record.Origin != "user" || record.State != "admitted" {
		t.Fatalf("actual dashboard Add did not save user/admitted: %+v", record)
	}
	current, err := probeAdmission(f.root, path)
	if err != nil {
		t.Fatal(err)
	}
	if sameAdmissionIdentity(old, current) {
		t.Fatal("fixture did not produce a distinguishable new incarnation")
	}
	t.Log("actual dashboard Add saved user/admitted for a distinguishable recreation")
	f.d.rail.readRepo(f.root, nil)
	record = admissionAt(admissionRecords(t, f.d.layout), f.root, path)
	if record.Origin != "user" || record.State != "admitted" {
		t.Fatalf("Add-triggered reread downgraded the recreated user worktree: %+v", record)
	}
	reopened := openRailLayoutStore(f.dir)
	record = admissionAt(admissionRecords(t, reopened), f.root, path)
	if record.Origin != "user" || record.State != "admitted" {
		t.Fatalf("reopened recreation admission: %+v", record)
	}
}

func TestRailAdmissionPromptMatchesVisibleRejectedWorktrees(t *testing.T) {
	f := newDeleteFixture(t)
	baseB, rootB := worktreeRepo(t)
	if _, err := f.d.workspaces.add(rootB, false); err != nil {
		t.Fatal(err)
	}
	forgotten := filepath.Join(f.tmp, "forgotten-rejection")
	pooled := filepath.Join(baseB, "pooled-rejection")
	visible := filepath.Join(baseB, "visible-rejection")
	newest := filepath.Join(baseB, "new-rejection")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "forgotten-rejection", forgotten)
	for name, path := range map[string]string{"pooled-rejection": pooled, "visible-rejection": visible, "new-rejection": newest} {
		gitRun(t, rootB, "worktree", "add", "-q", "-b", name, path)
	}
	refresh := func() {
		f.d.railBaseView(f.d.pages())
		for _, root := range []string{f.root, rootB} {
			list := f.d.rail.readRepo(root, nil)
			f.d.rail.mu.Lock()
			f.d.rail.repos[root] = list
			f.d.rail.mu.Unlock()
		}
	}
	refresh()
	reject := func(root, path string) railAdmissionReply {
		t.Helper()
		selection := admissionSelection(t, f.d.rail.admission, func(root string) (RepoWorktrees, error) { return f.d.rail.read(root, nil) }, root, path, "reject")
		raw, _ := json.Marshal(railAdmissionRequest{Selections: []railAdmissionSelection{selection}})
		code, _, body := postJSON(t, f.base+railAdmissionRoute, f.base, string(raw))
		var reply railAdmissionReply
		if err := json.Unmarshal([]byte(body), &reply); err != nil || code != 200 || !reply.Applied {
			t.Fatalf("reject: %d %s %v", code, body, err)
		}
		return reply
	}
	reject(f.root, forgotten)
	reject(rootB, pooled)
	reject(rootB, visible)
	forgetRaw, _ := json.Marshal(workspacePathRequest{Path: f.root})
	if code, _, body := postJSON(t, f.base+workspaceForgetRoute, f.base, string(forgetRaw)); code != http.StatusNoContent {
		t.Fatalf("forget repository: %d %s", code, body)
	}
	originalRead := f.d.rail.read
	f.d.rail.read = func(root string, regs []Registration) (RepoWorktrees, error) {
		got, err := originalRead(root, regs)
		if samePath(root, rootB) {
			got.Pools = append(got.Pools, pooled)
		}
		return got, err
	}
	refresh()
	reply := reject(rootB, newest)
	view := f.d.railView(f.d.pages())
	want := map[string]bool{}
	for _, ws := range view.Workspaces {
		for _, wt := range ws.RejectedWorktrees() {
			want[wt.Path] = true
		}
	}
	if len(want) != 2 || !want[visible] || !want[newest] {
		t.Fatalf("fixture visible rejected list: %v", want)
	}
	if len(admissionRecords(t, f.d.layout)) != 4 {
		t.Fatal("hidden rejection records must remain durable")
	}
	_, raw, ok := strings.Cut(reply.Prompt, "\n\n")
	var prompt []railAdmissionRecord
	if !ok {
		t.Fatalf("prompt has no JSON list: %s", reply.Prompt)
	}
	if err := json.Unmarshal([]byte(raw), &prompt); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range prompt {
		got[r.Path] = true
	}
	if len(got) != len(want) || got[forgotten] || got[pooled] || !got[visible] || !got[newest] {
		t.Fatalf("prompt does not match visible collapsed rejections: got=%v want=%v", got, want)
	}
}

func TestRailAdmissionRouteAndUserAdd(t *testing.T) {
	f := newDeleteFixture(t)
	path := filepath.Join(f.tmp, "admission-route")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "admission-route", path)
	f.d.railBaseView(f.d.pages())
	list := f.d.rail.readRepo(f.root, nil)
	f.d.rail.mu.Lock()
	f.d.rail.repos[f.root] = list
	f.d.rail.mu.Unlock()
	selection := admissionSelection(t, f.d.rail.admission, func(root string) (RepoWorktrees, error) { return f.d.rail.read(root, nil) }, f.root, path, "reject")
	raw, _ := json.Marshal(railAdmissionRequest{Selections: []railAdmissionSelection{selection}})
	if code, _, _ := postJSON(t, f.base+railAdmissionRoute, "http://evil.example", string(raw)); code != http.StatusForbidden {
		t.Fatalf("origin: %d", code)
	}
	if code, _ := get(t, f.base+railAdmissionRoute); code != http.StatusMethodNotAllowed {
		t.Fatalf("method: %d", code)
	}
	code, _, body := postJSON(t, f.base+railAdmissionRoute, f.base, string(raw))
	var reply railAdmissionReply
	if err := json.Unmarshal([]byte(body), &reply); err != nil || code != 200 || !reply.Applied || reply.Gen == 0 || reply.Prompt == "" {
		t.Fatalf("route: %d %s %v", code, body, err)
	}
	if !pathExists(path) {
		t.Fatal("route removed rejected worktree")
	}
	added := filepath.Join(f.tmp, "user-added")
	addRaw, _ := json.Marshal(worktreeAddRequest{Workspace: f.root, Branch: "user-added", New: true, Folder: added})
	code, _, body = postJSON(t, f.base+worktreeAddRoute, f.base, string(addRaw))
	if code != 200 {
		t.Fatalf("add: %d %s", code, body)
	}
	record := admissionAt(admissionRecords(t, f.d.layout), f.root, added)
	if record.Origin != "user" || record.State != "admitted" {
		t.Fatalf("user add: %+v", record)
	}
}
