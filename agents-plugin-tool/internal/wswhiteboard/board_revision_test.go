package wswhiteboard

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBoardRevisionIgnoresSystemAndSurvivesReload(t *testing.T) {
	s := &State{}
	applyAt(t, s, bodies("A", "B"), testNow)
	revision := s.boardRevision()
	s.appendEntry("A", Entry{Kind: EntrySystem, Text: "reconciliation"}, testNow.Add(time.Minute))
	if got := s.boardRevision(); got != revision {
		t.Fatal("system event changed board revision")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var reopened State
	if err := json.Unmarshal(data, &reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.boardRevision() != revision {
		t.Fatal("revision changed across reload")
	}
	applyAt(t, &reopened, `<wb:topic name="A"><wb:log>progress</wb:log></wb:topic>`, testNow.Add(2*time.Minute))
	if reopened.boardRevision() == revision {
		t.Fatal("lead activity did not change revision")
	}
}

func TestBoardRevisionMetadataRevertAndNoop(t *testing.T) {
	s := &State{}
	applyAt(t, s, bodies("A"), testNow)
	initial := s.boardRevision()
	applyAt(t, s, `<wb:topic name="A" tickets="261112-feat-a"/>`, testNow.Add(time.Minute))
	changed := s.boardRevision()
	if changed == initial {
		t.Fatal("ticket change did not change revision")
	}
	applyAt(t, s, `<wb:topic name="A" tickets="261112-feat-a"/>`, testNow.Add(2*time.Minute))
	if s.boardRevision() != changed {
		t.Fatal("no-op changed revision")
	}
	applyAt(t, s, `<wb:topic name="A" tickets=""/>`, testNow.Add(3*time.Minute))
	reverted := s.boardRevision()
	if reverted == initial || reverted == changed {
		t.Fatal("metadata revert reused an earlier revision")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var reopened State
	if err := json.Unmarshal(data, &reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.boardRevision() != reverted {
		t.Fatal("metadata revision did not survive reload")
	}
	applyAt(t, &reopened, `<wb:topic name="A" tickets="261112-feat-a"/><wb:topic name="A" tickets=""/>`, testNow.Add(4*time.Minute))
	if reopened.boardRevision() == reverted {
		t.Fatal("same-batch metadata edit and revert reused revision")
	}
}
