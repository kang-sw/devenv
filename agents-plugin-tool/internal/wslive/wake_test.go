package wslive

import (
	"slices"
	"strings"
	"testing"
)

func TestWakeKindsAndTurn(t *testing.T) {
	if !slices.Contains(OptionalKinds, KindWake) {
		t.Fatal("wake must be negotiated")
	}
	for _, cause := range []string{"subagent_report", "subagent_done", "subagent_failed", "background_exit", "monitor", "mail", "message", "scheduled"} {
		st := NewState("secret-key")
		caption := "caption secret-key\nnext"
		res, err := st.Apply(Event{Kind: KindWake, Cause: cause, Source: "secret-key", Caption: &caption, Text: strings.Repeat("x", TextBytes+1) + " secret-key"}, 1)
		if err != nil || len(res.Ring) != 1 {
			t.Fatalf("%s: %+v %v", cause, res, err)
		}
		e := res.Ring[0]
		if e.Kind != RingWake || e.Cause != cause || !e.Truncated || strings.Contains(e.Caption+e.Source+e.Text, "secret-key") {
			t.Fatalf("bad wake: %+v", e)
		}
		st.Apply(Event{Kind: KindTurnStart}, 2)
		res, err = st.Apply(Event{Kind: KindTurnEnd}, 3)
		if err != nil || len(res.Ring) != 1 || !res.Ring[0].WakeTurn {
			t.Fatalf("wake turn: %+v %v", res, err)
		}
		st.Apply(Event{Kind: KindTurnStart}, 4)
		res, _ = st.Apply(Event{Kind: KindTurnEnd}, 5)
		if res.Ring[0].WakeTurn {
			t.Fatal("wake reused")
		}
	}
}

func TestWakeAfterOriginlessStartBeforeWork(t *testing.T) {
	st := NewState("key")
	st.Apply(Event{Kind: KindTurnStart}, 1)
	st.Apply(Event{Kind: KindWake, Cause: "subagent_report"}, 2)
	res, _ := st.Apply(Event{Kind: KindTurnEnd}, 3)
	if !res.Ring[0].WakeTurn {
		t.Fatal("Pi input after agent_start did not mark its turn")
	}
}

func TestWakeDoesNotClaimPersonOrRunningTurn(t *testing.T) {
	for _, person := range []bool{false, true} {
		st := NewState("key")
		if !person {
			st.Apply(Event{Kind: KindTurnStart, Origin: OriginTerminal}, 1)
		}
		st.Apply(Event{Kind: KindWake, Cause: "monitor"}, 2)
		if person {
			st.Apply(Event{Kind: KindTurnStart, Origin: OriginTerminal}, 3)
		}
		res, _ := st.Apply(Event{Kind: KindTurnEnd}, 4)
		if res.Ring[0].WakeTurn {
			t.Fatal("claimed a turn wake did not start")
		}
	}
	st := NewState("key")
	if _, err := st.Apply(Event{Kind: KindWake, Cause: "unknown"}, 1); err == nil {
		t.Fatal("unknown cause accepted")
	}
	st.Apply(Event{Kind: KindWake, Cause: "scheduled"}, 1)
	st.Apply(Event{Kind: KindReset}, 2)
	st.Apply(Event{Kind: KindTurnStart}, 3)
	res, _ := st.Apply(Event{Kind: KindTurnEnd}, 4)
	if res.Ring[0].WakeTurn {
		t.Fatal("reset kept wake")
	}
}
