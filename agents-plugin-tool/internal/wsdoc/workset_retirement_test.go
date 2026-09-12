package wsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorksetAuthoringRetired(t *testing.T) {
	const accepted = "accepted: feat, bug, refactor, chore, research, epic"
	check := func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !strings.HasSuffix(err.Error(), accepted) {
			t.Fatalf("retired category error = %v, want accepted categories without workset", err)
		}
	}
	_, err := TicketTemplate("workset")
	check(t, err)
	for _, phase := range []string{"content", "intent"} {
		_, err := TicketChecklist("workset", phase)
		check(t, err)
	}
	for _, state := range []string{"idea", "todo", "ready"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			_, err := TicketCreate(root, TicketCreateOptions{Stem: "workset-board", InitialState: state, Today: "260101"})
			if state == "ready" {
				if err == nil || !strings.Contains(err.Error(), "workset tickets never enter ready/") {
					t.Fatalf("ready category error = %v", err)
				}
			} else {
				check(t, err)
			}
			if _, err := os.Stat(filepath.Join(root, "ai-docs")); !os.IsNotExist(err) {
				t.Fatalf("rejected creation changed the filesystem: %v", err)
			}
		})
	}
}

func TestLegacyWorksetsRemainReadableAndClosable(t *testing.T) {
	for _, closeStatus := range []string{"done", "dropped"} {
		t.Run(closeStatus, func(t *testing.T) {
			root := t.TempDir()
			const stem = "260101-workset-board"
			const listed = "260101-feat-independent"
			const listedPath = "ai-docs/tickets/todo/" + listed + ".md"
			mustWrite(t, root, "ai-docs/tickets/todo/"+stem+".md", "---\ntitle: Legacy board\nrelated:\n  "+listed+": focus\n---\n\n## Tickets\n\n- "+listed+"\n")
			mustWrite(t, root, listedPath, sampleTicket)
			assertReadable := func(wantStatus string) {
				t.Helper()
				matches, err := TicketsFind(root, TicketFindOptions{TicketStem: stem, IncludeDone: true, IncludeDropped: true})
				if err != nil || len(matches) != 1 || matches[0].Status != wantStatus {
					t.Fatalf("legacy query = %+v, %v", matches, err)
				}
				info, err := TicketsStatus(root, TicketStatusOptions{TicketStem: stem, IncludeDone: true, IncludeDropped: true})
				if err != nil || info.Status != wantStatus {
					t.Fatalf("legacy status = %+v, %v", info, err)
				}
				graph, err := loadTicketGraph(root)
				if err != nil {
					t.Fatal(err)
				}
				if graph.byStem[stem].Status != wantStatus || graph.byStem[stem].Related[listed] != "focus" {
					t.Fatalf("legacy graph entry = %+v", graph.byStem[stem])
				}
				if len(graph.children[stem]) != 0 || graph.byStem[listed].Parent != "" {
					t.Fatal("workset listing became a child relationship")
				}
			}
			assertReadable("todo")
			_, err := TicketsClose(root, &mockGitRunner{}, TicketCloseOptions{TicketStem: stem, Status: closeStatus, Today: "2026-09-10"})
			if err != nil {
				t.Fatal(err)
			}
			assertReadable("." + closeStatus)
			if got := readFileString(t, filepath.Join(root, listedPath)); got != sampleTicket {
				t.Fatalf("closing workset changed listed ticket: %s", got)
			}
		})
	}
}
