package wsdashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// The head's parent jump: a session some other session owns shows a pill left
// of its identity that names the owner and links to its page.

func TestAgentRecordEndDecodesParentReplyID(t *testing.T) {
	dir := t.TempDir()
	sessionDir := SessionDir(dir, "kidsession")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"lead":false,"parent":{"kind":"conversation","reply_id":"ownersession","future":1}}`
	if err := os.WriteFile(filepath.Join(sessionDir, AgentRecordFileName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	record, hosted := readAgentRecordEnd(sessionDir)
	if !hosted || record.Parent == nil || record.Parent.Kind != "conversation" || record.Parent.ReplyID != "ownersession" {
		t.Fatalf("readAgentRecordEnd = %+v, %v; want the parent's kind and reply-id", record, hosted)
	}
	if !record.retired() && record.ClosedAt != "" {
		t.Fatalf("retirement reads the same record: %+v", record)
	}
}

func TestRegistrationCarriesParentReplyID(t *testing.T) {
	dir := t.TempDir()
	reg := Registration{ReplyID: "kidsession", Project: t.TempDir(), Harness: "claude", ParentReplyID: "ownersession"}
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRegistration(dir, "kidsession")
	if err != nil || got.ParentReplyID != "ownersession" {
		t.Fatalf("ReadRegistration = %+v, %v; want parent_reply_id kept", got, err)
	}
	raw, err := wsstate.ReadFileOnce(filepath.Join(SessionDir(dir, "kidsession"), registrationFileName))
	if err != nil || !strings.Contains(string(raw), `"parent_reply_id": "ownersession"`) {
		t.Fatalf("registration.json = %s, %v", raw, err)
	}
	// A row without an owner writes no member, so an older reader sees the
	// same bytes it always did.
	if err := WriteRegistration(dir, Registration{ReplyID: "rootsession", Project: reg.Project}); err != nil {
		t.Fatal(err)
	}
	raw, _ = wsstate.ReadFileOnce(filepath.Join(SessionDir(dir, "rootsession"), registrationFileName))
	if strings.Contains(string(raw), "parent_reply_id") {
		t.Fatalf("an unowned registration names a parent: %s", raw)
	}
}

type parentFixture struct {
	*pageFixture
	reader pageReader
}

// startParentFixture registers: rootsession ("Root Label") owns midsession
// ("Mid Label"), which owns leafsession; nestedkid is owned only by the rail
// layout (under rootsession); orphankid names a parent the registry no longer
// holds; cyclea and cycleb own each other.
func startParentFixture(t *testing.T) *parentFixture {
	t.Helper()
	f := startPageFixture(t)
	now := time.Now()
	add := func(id, parent, label string) {
		reg := register(t, f.dir, id, f.project, now)
		reg.ParentReplyID = parent
		if err := WriteRegistration(f.dir, reg); err != nil {
			t.Fatal(err)
		}
		if label != "" {
			if err := writeSessionLabel(SessionDir(f.dir, id), label); err != nil {
				t.Fatal(err)
			}
		}
		f.src.setPage(id, PageContent{})
	}
	add("rootsession", "", "Root Label")
	add("midsession", "rootsession", "Mid Label")
	add("leafsession", "midsession", "")
	add("nestedkid", "", "")
	add("orphankid", "gonesession", "")
	add("cyclea", "cycleb", "")
	add("cycleb", "cyclea", "")
	doc := &railLayoutDoc{SchemaVersion: railLayoutSchemaVersion, Workspaces: map[string]*railLayoutWS{
		"/w/repo": {Children: map[string][]string{"rootsession": {"nestedkid"}}},
	}}
	raw, err := doc.encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, railLayoutFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	f.d.Index().Refresh()
	for _, id := range []string{"rootsession", "midsession", "leafsession", "nestedkid", "orphankid", "cyclea", "cycleb"} {
		id := id
		waitFor(t, id+" to be indexed", func() bool { _, ok := f.d.Index().Session(id); return ok })
	}
	r := f.d.pages()
	r.foreign = nil // always look at the layout file, as a fresh render does
	return &parentFixture{pageFixture: f, reader: r}
}

func (f *parentFixture) head(t *testing.T, id string) (string, *ParentView) {
	t.Helper()
	view, ok := f.reader.session(id)
	if !ok {
		t.Fatalf("session %s is not served", id)
	}
	regions, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(regions[regionHead].HTML), view.Parent
}

func TestParentPillOmittedOnRootSession(t *testing.T) {
	f := startParentFixture(t)
	head, parent := f.head(t, "rootsession")
	if parent != nil || strings.Contains(head, "tph-parent") {
		t.Fatalf("a session nobody owns shows a parent: %+v\n%s", parent, head)
	}
	if !strings.HasPrefix(head, `<span class="tph tph-id">`) {
		t.Fatalf("the identity does not lead the head:\n%s", head)
	}
}

func TestParentPillLinksSpawnedChild(t *testing.T) {
	f := startParentFixture(t)
	head, parent := f.head(t, "midsession")
	want := `<a class="tph tph-parent" href="/s/rootsession" title="Go to parent session Root Label">`
	if parent == nil || !parent.Linked || parent.Path != "" || !strings.HasPrefix(head, want) {
		t.Fatalf("parent = %+v; head lacks %q:\n%s", parent, want, head)
	}
	for _, frag := range []string{`<span class="meta tph-r1">Parent</span><b class="tph-pn">Root Label</b>`, `</a><span class="tph tph-id">`} {
		if !strings.Contains(head, frag) {
			t.Errorf("head lacks %q:\n%s", frag, head)
		}
	}
	// Over HTTP the page's head carries the same pill.
	_, page := get(t, f.base+"/s/midsession")
	if !strings.Contains(page, `href="/s/rootsession"`) || !strings.Contains(page, `class="tph tph-parent"`) {
		t.Errorf("the shell page lacks the parent pill")
	}
}

func TestParentPillGrandchildNamesTheChain(t *testing.T) {
	f := startParentFixture(t)
	head, parent := f.head(t, "leafsession")
	if parent == nil || parent.ReplyID != "midsession" || parent.Name != "Mid Label" || parent.Path != "Root Label › Mid Label" {
		t.Fatalf("parent = %+v", parent)
	}
	want := `<a class="tph tph-parent" href="/s/midsession" title="Go to parent session Mid Label&#10;Root Label › Mid Label">`
	if !strings.HasPrefix(head, want) {
		t.Fatalf("head lacks %q:\n%s", want, head)
	}
}

func TestParentPillFallsBackToRailNesting(t *testing.T) {
	f := startParentFixture(t)
	head, parent := f.head(t, "nestedkid")
	if parent == nil || parent.ReplyID != "rootsession" || !parent.Linked {
		t.Fatalf("parent = %+v", parent)
	}
	if !strings.Contains(head, `href="/s/rootsession"`) || !strings.Contains(head, "Root Label") {
		t.Fatalf("head does not link the layout parent:\n%s", head)
	}
}

func TestParentPillPrunedParentIsInert(t *testing.T) {
	f := startParentFixture(t)
	head, parent := f.head(t, "orphankid")
	if parent == nil || parent.Linked || parent.Name != "gonesession" {
		t.Fatalf("parent = %+v", parent)
	}
	if !strings.HasPrefix(head, `<span class="tph tph-parent dead" title="Parent session gonesession is no longer available">`) || strings.Contains(head, "/s/gonesession") {
		t.Fatalf("a pruned parent is not an inert badge:\n%s", head)
	}
}

func TestParentPillCycleTerminates(t *testing.T) {
	f := startParentFixture(t)
	for _, id := range []string{"cyclea", "cycleb"} {
		_, parent := f.head(t, id)
		other := "cycleb"
		if id == "cycleb" {
			other = "cyclea"
		}
		if parent == nil || parent.ReplyID != other || parent.Path != "" {
			t.Fatalf("%s: parent = %+v", id, parent)
		}
	}
	// A forged chain longer than the cap stops at the cap.
	const n = parentChainCap + 8
	sessions := map[string]Session{}
	for i := 0; i < n; i++ {
		id := "chain" + string(rune('a'+i))
		next := "chain" + string(rune('a'+i+1))
		sessions[id] = Session{Registration: Registration{ReplyID: id, ParentReplyID: next}}
	}
	src := parentSource{session: func(id string) (Session, bool) { s, ok := sessions[id]; return s, ok }}
	v := resolveParent(sessions["chaina"], src)
	if v == nil || strings.Count(v.Path, "›")+1 != parentChainCap {
		t.Fatalf("chain = %+v; want %d ancestors", v, parentChainCap)
	}
}
