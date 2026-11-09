package wsledger

import (
	"context"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// TestWikiPushRefusalStandsAcrossFetches: a local-only wiki the remote
// refuses is reported by every report, a Fetch's (which does not push)
// included, until a push lands; a background poll's Fetch must not be the
// one report that carried it.
func TestWikiPushRefusalStandsAcrossFetches(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	oa := w.remoteOpts("ada", "c1")
	ada, err := Create(context.Background(), oa)
	if err != nil {
		t.Fatal(err)
	}
	saveOK(t, ada, wikiCreate("stuck", "p", "1\n"))
	ada.Close()

	fr := &wikiFaultRunner{inner: wsgit.ExecRunner{Env: GitEnv}, pushRef: wikiRef("stuck")}
	oa.Runner = fr
	ada = w.register(oa, url)
	for i := 1; i <= 2; i++ {
		if rep := fetchOK(t, ada); !hasWarning(rep.Warnings, wikiRef("stuck"), WarnWikiSyncFailed) {
			t.Fatalf("fetch %d after register = %+v, want the refused wiki's warning", i, rep.Warnings)
		}
	}
	rep := syncOK(t, ada)
	if n := countWarnings(rep.Warnings, wikiRef("stuck"), WarnWikiSyncFailed); n != 1 {
		t.Fatalf("sync carries the refusal %d times, want once: %+v", n, rep.Warnings)
	}
	if rep := fetchOK(t, ada); !hasWarning(rep.Warnings, wikiRef("stuck"), WarnWikiSyncFailed) {
		t.Fatalf("fetch after a refused sync = %+v, want the refused wiki's warning", rep.Warnings)
	}

	fr.set("", false)
	syncOK(t, ada)
	if rep := fetchOK(t, ada); hasWarning(rep.Warnings, wikiRef("stuck"), WarnWikiSyncFailed) {
		t.Fatalf("fetch after the push landed = %+v, want no warning", rep.Warnings)
	}
}

func countWarnings(ws []Warning, ref, kind string) int {
	n := 0
	for _, w := range ws {
		if w.Ref == ref && w.Kind == kind {
			n++
		}
	}
	return n
}
