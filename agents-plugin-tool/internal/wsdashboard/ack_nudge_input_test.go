package wsdashboard

import (
	"github.com/kang-sw/devenv/internal/wsinbox"
	"testing"
)

func TestAnswerCheckNudgeInputPredicate(t *testing.T) {
	text := ackNudgeText([]int{1, 2})
	plain := wsinbox.Entry{Parts: []string{text}}
	if !IsAnswerCheckNudge(plain) {
		t.Fatal("generator and predicate drifted")
	}
	for _, e := range []wsinbox.Entry{
		{Parts: []string{text, "owner text"}},
		{Parts: []string{text + " extra"}},
		{Parts: []string{text}, Chips: []wsinbox.Chip{{ID: "1", Answer: "ok"}}},
		{Parts: []string{"Dashboard answer check: owner-authored literal"}},
	} {
		if IsAnswerCheckNudge(e) {
			t.Fatal("owner content excluded", e)
		}
	}
	ids := make([]int, ackNudgeCap+1)
	for i := range ids {
		ids[i] = i + 1
	}
	if !IsAnswerCheckNudge(wsinbox.Entry{Parts: []string{ackNudgeText(ids)}}) {
		t.Fatal("capped nudge not recognized")
	}
}
