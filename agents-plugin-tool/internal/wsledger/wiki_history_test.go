// Wiki history tests exercise verification and recovery points.
package wsledger

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestWikiHistoryReportsUnverifiedCommits: a commit signed by no member key
// and an unsigned commit are listed with the reason, not refused; the
// member's commits are verified.
func TestWikiHistoryReportsUnverifiedCommits(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("h", "p", "1\n"))
	tree, err := ada.git.treeOf(ctx, r0.Commit)
	if err != nil {
		t.Fatal(err)
	}
	foreignKey, _, _, err := ensureKey(ctx, filepath.Join(w.root, "keys", "mallory"), nil)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := ada.git.commitTree(ctx, tree, r0.Commit, []byte("wiki(h): forged\n"), "ada", foreignKey)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ada.git.run(ctx, "-c", "user.name=ada", "-c", "user.email=ada@ws-ledger.invalid", "commit-tree", "-m", "wiki(h): unsigned", "-p", foreign, tree)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := lastOID(out)
	if err != nil {
		t.Fatal(err)
	}
	setRef(t, ada, wikiRef("h"), unsigned, r0.Commit)

	hist, err := ada.WikiHistory(ctx, "h", "", 0)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history = %+v, %v", hist, err)
	}
	if hist[0].Verified || hist[0].Unverified != "unsigned" {
		t.Fatalf("unsigned commit = %+v", hist[0])
	}
	if hist[1].Verified || !strings.Contains(hist[1].Unverified, "not a key of any member") || hist[1].Author != "ada" {
		t.Fatalf("foreign-key commit = %+v", hist[1])
	}
	if !hist[2].Verified || hist[2].Signer != "ada" {
		t.Fatalf("own commit = %+v", hist[2])
	}
	// The branch stays usable: a save on the tip lands.
	saveOK(t, ada, wikiWrite("h", unsigned, "p", "2\n"))
}

// TestWikiRecoveryListsSeveralPoints: two recovery points of one wiki and
// one of another all carry their time and subject, newest first.
func TestWikiRecoveryListsSeveralPoints(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	a0 := saveOK(t, ada, wikiCreate("one", "p", "1\n"))
	a1 := saveOK(t, ada, wikiWrite("one", a0.Commit, "p", "2\n"))
	b0 := saveOK(t, ada, wikiCreate("two", "p", "1\n"))
	b1 := saveOK(t, ada, wikiWrite("two", b0.Commit, "p", "2\n"))
	fetchOK(t, bo)
	remoteGit(t, url, "update-ref", wikiRef("one"), a0.Commit, a1.Commit)
	remoteGit(t, url, "update-ref", wikiRef("two"), b0.Commit, b1.Commit)
	fetchOK(t, bo)
	a2 := saveOK(t, ada, wikiWrite("one", a0.Commit, "p", "3\n"))
	fetchOK(t, bo)
	remoteGit(t, url, "update-ref", wikiRef("one"), a0.Commit, a2.Commit)
	fetchOK(t, bo)

	points, err := bo.WikiRecovery(ctx, "")
	if err != nil || len(points) != 3 {
		t.Fatalf("recovery = %+v, %v", points, err)
	}
	for _, p := range points {
		if p.Time.IsZero() || !strings.HasPrefix(p.Subject, "wiki("+p.Wiki+"): write p") {
			t.Fatalf("point without its commit data: %+v", p)
		}
	}
	one, err := bo.WikiRecovery(ctx, "one")
	if err != nil || len(one) != 2 || one[0].OID == one[1].OID {
		t.Fatalf("recovery of one = %+v, %v", one, err)
	}
}
