package wsgit

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func validFollowupCommitOptions(followups []Followup) CommitOptions {
	return CommitOptions{Paths: []string{"src"}, Title: "chore(review): stamp", AIContext: []string{"context"}, ExpectedBranch: "main", Followups: followups}
}

func TestNormalizeCommitOptionsValidatesFollowups(t *testing.T) {
	cases := []struct {
		name string
		f    Followup
		want string
	}{
		{"unknown level", Followup{Level: "major", Category: "review", Content: "x"}, `level "major" is not one of minor, important, critical`},
		{"note level rejected", Followup{Level: "note", Category: "review", Content: "x"}, "is not one of minor, important, critical"},
		{"unknown category", Followup{Level: "minor", Category: "perf", Content: "x"}, `category "perf" is not one of review`},
		{"empty content", Followup{Level: "minor", Category: "review", Content: "  "}, "content is required"},
		{"multi-line content", Followup{Level: "minor", Category: "review", Content: "a.go:1 first\nsecond"}, "must be a single line"},
		{"carriage return content", Followup{Level: "minor", Category: "review", Content: "a.go:1 first\rsecond"}, "must be a single line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeCommitOptions(validFollowupCommitOptions([]Followup{tc.f}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("normalize error = %v, want %q", err, tc.want)
			}
		})
	}
	for _, level := range FollowupLevels {
		opts, err := normalizeCommitOptions(validFollowupCommitOptions([]Followup{{Level: level, Category: "review", Content: " a.go:3 summary "}}))
		if err != nil {
			t.Fatalf("level %s rejected: %v", level, err)
		}
		if opts.Followups[0].Content != "a.go:3 summary" {
			t.Fatalf("content not trimmed: %q", opts.Followups[0].Content)
		}
	}
	if _, err := normalizeCommitOptions(CommitOptions{Paths: []string{"src"}, Title: "fix: x", AIContext: []string{"c"}, ExpectedBranch: "main", Resolves: []string{"two words"}}); err == nil || !strings.Contains(err.Error(), "single follow-up id") {
		t.Fatalf("resolves with whitespace error = %v", err)
	}
}

func TestFollowupLevelRankIsOrdered(t *testing.T) {
	if !(FollowupLevelRank("minor") < FollowupLevelRank("important") && FollowupLevelRank("important") < FollowupLevelRank("critical")) {
		t.Fatal("levels are not ordered minor < important < critical")
	}
	if FollowupLevelRank("warn") != -1 {
		t.Fatal("unknown level must rank -1")
	}
}

func TestCommitMintsFollowupIDsInInputOrderAndWritesSections(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},               // pre-status
		{},               // add
		[]byte("1 A. N... 100644 100644 100644 aaa bbb src/file.go\n"),
		{}, // ticket name-status
		{}, // ticket diff
		{}, // commit
		[]byte("abc123\n"),
	}}
	ids := []string{"amber-latch-dwelled", "amber-latch-dwelled", "cobalt-rift-oaken"}
	next := 0
	mint := func() (string, error) {
		id := ids[next]
		next++
		return id, nil
	}
	opts := validFollowupCommitOptions([]Followup{
		{Level: "minor", Category: "review", Content: "src/a.go:12 duplicated literal"},
		{Level: "important", Category: "review", Content: "src/b.go:40 untested edge"},
	})
	opts.Resolves = []string{"older-fixed-item"}
	result, err := (Client{Runner: runner, MintID: mint}).Commit(context.Background(), "/repo", opts)
	if err != nil {
		t.Fatal(err)
	}
	// The duplicate draw is re-rolled so ids stay distinct within the commit.
	if len(result.Followups) != 2 || result.Followups[0].ID != "amber-latch-dwelled" || result.Followups[1].ID != "cobalt-rift-oaken" {
		t.Fatalf("minted followups = %#v", result.Followups)
	}
	if result.Followups[0].Content != "src/a.go:12 duplicated literal" || result.Followups[1].Level != "important" {
		t.Fatalf("followups lost input order: %#v", result.Followups)
	}
	message := runner.calls[6].args[2]
	for _, want := range []string{
		"## Follow-ups\n- minor/review amber-latch-dwelled: src/a.go:12 duplicated literal\n- important/review cobalt-rift-oaken: src/b.go:40 untested edge",
		"## Resolves\n- older-fixed-item",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
	if strings.Index(message, "## AI Context") > strings.Index(message, "## Follow-ups") {
		t.Fatalf("follow-ups must follow AI Context:\n%s", message)
	}
}

func TestCommitMintFailureCommitsNothing(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"),
		{},
		{},
		[]byte("1 A. N... 100644 100644 100644 aaa bbb src/file.go\n"),
		{},
		{},
	}}
	opts := validFollowupCommitOptions([]Followup{{Level: "minor", Category: "review", Content: "x"}})
	_, err := (Client{Runner: runner, MintID: func() (string, error) { return "", fmt.Errorf("no entropy") }}).Commit(context.Background(), "/repo", opts)
	if err == nil || !strings.Contains(err.Error(), "no entropy") {
		t.Fatalf("err = %v", err)
	}
	for _, call := range runner.calls {
		if len(call.args) > 0 && call.args[0] == "commit" {
			t.Fatalf("commit ran despite mint failure: %#v", runner.calls)
		}
	}
}
