// Tests for Git ticket change detection and inference.
package wsgit

import (
	"context"
	"strings"
	"testing"
)

func TestParseTicketNameStatusDetectsMoves(t *testing.T) {
	changes := parseTicketNameStatus([]byte("R100\tai-docs/tickets/todo/260503-feat-demo.md\tai-docs/tickets/.done/260503-feat-demo.md\n"))
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	change := changes[0]
	if change.Stem != "260503-feat-demo" || change.FromStatus != "todo" || change.ToStatus != ".done" || change.OldPath == "" {
		t.Fatalf("change = %#v", change)
	}
}

func TestParseTicketNameStatusDetectsReadyMoves(t *testing.T) {
	changes := parseTicketNameStatus([]byte("R100\tai-docs/tickets/todo/260503-feat-demo.md\tai-docs/tickets/ready/260503-feat-demo.md\n"))
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	change := changes[0]
	if change.Stem != "260503-feat-demo" || change.FromStatus != "todo" || change.ToStatus != "ready" || change.OldPath == "" {
		t.Fatalf("change = %#v", change)
	}
}

func TestParseTicketNameStatusReconstructsAddDeleteMove(t *testing.T) {
	changes := parseTicketNameStatus([]byte("D\tai-docs/tickets/todo/260503-feat-demo.md\nA\tai-docs/tickets/.done/260503-feat-demo.md\n"))
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	change := changes[0]
	if change.Stem != "260503-feat-demo" || change.FromStatus != "todo" || change.ToStatus != ".done" {
		t.Fatalf("change statuses = %#v", change)
	}
	if change.Path != "ai-docs/tickets/.done/260503-feat-demo.md" || change.OldPath != "ai-docs/tickets/todo/260503-feat-demo.md" {
		t.Fatalf("change paths = %#v", change)
	}
}

func TestParseTicketNameStatusDoesNotReconstructAmbiguousAddDeleteMove(t *testing.T) {
	changes := parseTicketNameStatus([]byte(strings.Join([]string{
		"D\tai-docs/tickets/todo/260503-feat-demo.md",
		"D\tai-docs/tickets/ready/260503-feat-demo.md",
		"A\tai-docs/tickets/.done/260503-feat-demo.md",
		"",
	}, "\n")))
	if len(changes) != 3 {
		t.Fatalf("changes = %#v", changes)
	}
	for _, change := range changes {
		if change.FromStatus != "" || change.OldPath != "" {
			t.Fatalf("ambiguous change reconstructed a move: %#v", change)
		}
	}
}

func TestCommitMergesResultHeadingIntoReconstructedAddDeleteMove(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},
		{},
		[]byte("1 A. N... 100644 100644 100644 aaa bbb ai-docs/tickets/.done/260503-feat-demo.md\n1 D. N... 100644 000000 000000 aaa 0000000000000000000000000000000000000000 ai-docs/tickets/todo/260503-feat-demo.md\n"),
		[]byte("D\tai-docs/tickets/todo/260503-feat-demo.md\nA\tai-docs/tickets/.done/260503-feat-demo.md\n"),
		[]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/.done/260503-feat-demo.md\n+++ b/ai-docs/tickets/.done/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n"),
		{},
		[]byte("abc123\n"),
	}}
	result, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:          []string{"ai-docs/tickets"},
		Title:          "docs(ticket): close demo ticket",
		AIContext:      []string{"User intent: close the workflow ticket."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TicketChanges) != 1 {
		t.Fatalf("ticket changes = %#v", result.TicketChanges)
	}
	change := result.TicketChanges[0]
	if change.FromStatus != "todo" || change.ToStatus != ".done" || !change.ResultAdded || change.ResultHeading != "### Result (abc123) - 2026-05-04" {
		t.Fatalf("ticket change = %#v", change)
	}
	message := runner.calls[6].args[2]
	if !strings.Contains(message, "260503-feat-demo: moved todo -> .done and added ### Result") {
		t.Fatalf("message missing reconstructed move summary:\n%s", message)
	}
}

func TestCommitKeepsAmbiguousAddDeleteResultHeadingNonMove(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},
		{},
		[]byte("1 A. N... 100644 100644 100644 aaa bbb ai-docs/tickets/.done/260503-feat-demo.md\n1 D. N... 100644 000000 000000 aaa 0000000000000000000000000000000000000000 ai-docs/tickets/todo/260503-feat-demo.md\n1 D. N... 100644 000000 000000 aaa 0000000000000000000000000000000000000000 ai-docs/tickets/ready/260503-feat-demo.md\n"),
		[]byte(strings.Join([]string{
			"D\tai-docs/tickets/todo/260503-feat-demo.md",
			"D\tai-docs/tickets/ready/260503-feat-demo.md",
			"A\tai-docs/tickets/.done/260503-feat-demo.md",
			"",
		}, "\n")),
		[]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/.done/260503-feat-demo.md\n+++ b/ai-docs/tickets/.done/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n"),
		{},
		[]byte("abc123\n"),
	}}
	_, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:          []string{"ai-docs/tickets"},
		Title:          "docs(ticket): update ambiguous ticket records",
		AIContext:      []string{"User intent: preserve ambiguous ticket summary behavior."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := runner.calls[6].args[2]
	if strings.Contains(message, "moved") {
		t.Fatalf("ambiguous add/delete emitted move summary:\n%s", message)
	}
	if !strings.Contains(message, "260503-feat-demo: added ### Result") {
		t.Fatalf("message missing non-move result summary:\n%s", message)
	}
}

func TestDetectTicketChangesDoesNotOverwriteAmbiguousSameStemRecords(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte(strings.Join([]string{
			"D\tai-docs/tickets/todo/260503-feat-demo.md",
			"D\tai-docs/tickets/ready/260503-feat-demo.md",
			"A\tai-docs/tickets/.done/260503-feat-demo.md",
			"",
		}, "\n")),
		[]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/.done/260503-feat-demo.md\n+++ b/ai-docs/tickets/.done/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n"),
	}}
	changes := detectTicketChanges(context.Background(), runner, "/repo")
	if len(changes) != 3 {
		t.Fatalf("changes = %#v", changes)
	}
	for _, change := range changes {
		if change.FromStatus != "" || change.OldPath != "" {
			t.Fatalf("ambiguous change reconstructed a move after detect merge: %#v", change)
		}
	}
}

func TestDetectTicketChangesDoesNotOverwriteExplicitRenameWithSameStemAddDelete(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte(strings.Join([]string{
			"R100\tai-docs/tickets/todo/260503-feat-demo.md\tai-docs/tickets/.done/260503-feat-demo.md",
			"A\tai-docs/tickets/ready/260503-feat-demo.md",
			"D\tai-docs/tickets/idea/260503-feat-demo.md",
			"",
		}, "\n")),
		{},
	}}
	changes := detectTicketChanges(context.Background(), runner, "/repo")
	var foundRename bool
	for _, change := range changes {
		if change.Path == "ai-docs/tickets/.done/260503-feat-demo.md" && change.OldPath == "ai-docs/tickets/todo/260503-feat-demo.md" && change.FromStatus == "todo" && change.ToStatus == ".done" {
			foundRename = true
		}
	}
	if !foundRename {
		t.Fatalf("explicit rename was overwritten: %#v", changes)
	}
}

func TestParseTicketResultAdditions(t *testing.T) {
	changes := parseTicketResultAdditions([]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/todo/260503-feat-demo.md\n+++ b/ai-docs/tickets/todo/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n+body\n"))
	if len(changes) != 1 || changes[0].Stem != "260503-feat-demo" || changes[0].ResultHeading != "### Result (abc123) - 2026-05-04" {
		t.Fatalf("changes = %#v", changes)
	}
}

func TestParseTicketEditionAdditions(t *testing.T) {
	changes := parseTicketResultAdditions([]byte("diff --git a/ai-docs/tickets/ready/260503-feat-demo.md b/ai-docs/tickets/ready/260503-feat-demo.md\n+++ b/ai-docs/tickets/ready/260503-feat-demo.md\n+#### Edition (def456) - 2026-05-05\n+Follow-up tweak.\n"))
	if len(changes) != 1 || changes[0].Stem != "260503-feat-demo" || changes[0].ResultHeading != "#### Edition (def456) - 2026-05-05" {
		t.Fatalf("changes = %#v", changes)
	}
}

// TestCommitBlockedByVerifierNeverReachesCommit exercises the commit-gate
// backstop: a Verifier that vetoes the staged ticket path must stop Commit
// before it stages further ticket-change detection or issues `git commit`,
// mirroring TestCommitRefusesUnrelatedStagedPaths's assert-on-call-count
// style. Uses a real t.TempDir() root (unlike the fake "/repo" used
// elsewhere in this file) with an actual invalid-shaped ticket file on disk,
// since the commit-gate's whole point is vetoing real staged ticket content.
