package wswhiteboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func applyBodyTimeAt(t *testing.T, st *State, commands string, at time.Time) {
	t.Helper()
	env := testEnv()
	env.Now = at
	if _, problems := Apply(st, commands, env); len(problems) > 0 {
		t.Fatalf("Apply(%s): %v", commands, problems)
	}
}

func assertBodyTime(t *testing.T, d TopicDoc, want string) {
	t.Helper()
	if got := d.validatedBodyTime(); got != want {
		t.Fatalf("validated body time = %q, want %q; doc %+v", got, want, d)
	}
	view := BuildView(&State{Topics: []TopicDoc{d}}, ViewInput{Layout: LayoutFull})
	if got := view.Topics[0].BodyUpdatedAt; got != want {
		t.Fatalf("view body time = %q, want %q", got, want)
	}
}

func TestBodyTimeDigestEncoding(t *testing.T) {
	d := TopicDoc{Body: "雪\r\none  two", Changed: 7, ChangedAt: "2026-11-14T13:00:00Z"}
	// An independently written vector pins the domain, field order, exact
	// whitespace and [] normalization rather than hashing the helper's output.
	encoded := `["wb-body-time-v1",7,"2026-11-14T13:00:00Z","雪\r\none  two",[]]`
	hash := sha256.Sum256([]byte(encoded))
	want := hex.EncodeToString(hash[:])
	if got := d.bodyTimeDigest(); got != want {
		t.Fatalf("digest %s, want %s for %s", got, want, encoded)
	}
	d.Marks = []BlockMark{}
	if d.bodyTimeDigest() != want {
		t.Fatal("nil and empty marks differ")
	}
	d.Marks = marks(1, MarkDel, 1, MarkMod)
	encoded = `["wb-body-time-v1",7,"2026-11-14T13:00:00Z","雪\r\none  two",[[1,"del"],[1,"mod"]]]`
	hash = sha256.Sum256([]byte(encoded))
	if d.bodyTimeDigest() != hex.EncodeToString(hash[:]) {
		t.Fatal("mark pair encoding changed")
	}
	before := d.bodyTimeDigest()
	d.Name, d.Tickets = "renamed", []string{"261114-feat-example"}
	if d.bodyTimeDigest() != before {
		t.Fatal("topic metadata entered the portable digest")
	}
	d.Marks[0], d.Marks[1] = d.Marks[1], d.Marks[0]
	if d.bodyTimeDigest() == before {
		t.Fatal("mark order is not bound")
	}
}

func TestBodyTimeProofValidation(t *testing.T) {
	valid := TopicDoc{Name: "t", Body: "one\n\ntwo", Changed: 7, ChangedAt: "2026-11-14T13:00:00Z", Marks: marks(1, MarkNew)}
	valid.mintBodyTimeProof()
	assertBodyTime(t, valid, valid.ChangedAt)
	cases := []struct {
		name       string
		breakProof func(*TopicDoc)
	}{
		{"missing", func(d *TopicDoc) { d.BodyTimeProof = nil }},
		{"unsupported", func(d *TopicDoc) { d.BodyTimeProof.Version = 2 }},
		{"empty body", func(d *TopicDoc) { d.Body = "" }},
		{"zero batch", func(d *TopicDoc) { d.Changed = 0; d.BodyTimeProof.Changed = 0 }},
		{"different batch", func(d *TopicDoc) { d.Changed++ }},
		{"different time", func(d *TopicDoc) { d.ChangedAt = "2026-11-14T13:00:01Z" }},
		{"missing time", func(d *TopicDoc) { d.ChangedAt = ""; d.BodyTimeProof.At = "" }},
		{"invalid time", func(d *TopicDoc) { d.ChangedAt = "not a time"; d.mintBodyTimeProof() }},
		{"non UTC", func(d *TopicDoc) { d.ChangedAt = "2026-11-14T14:00:00+01:00"; d.mintBodyTimeProof() }},
		{"wrong digest", func(d *TopicDoc) { d.BodyTimeProof.Digest = strings.Repeat("0", 64) }},
		{"uppercase digest", func(d *TopicDoc) { d.BodyTimeProof.Digest = strings.ToUpper(d.BodyTimeProof.Digest) }},
		{"exact body bytes", func(d *TopicDoc) { d.Body = "one \n\ntwo" }},
		{"body neutral marks", func(d *TopicDoc) { d.Marks = marks(1, MarkMod) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid
			proof := *valid.BodyTimeProof
			d.BodyTimeProof = &proof
			tc.breakProof(&d)
			assertBodyTime(t, d, "")
		})
	}
}

func TestBodyTimeMalformedAndFutureProofRoundTrip(t *testing.T) {
	for _, raw := range []string{
		`"bad shape"`, `[]`, `123`, `{"v":"1","at":false}`, `{"v":1,"changed":{}}`,
		`{"v":2,"at":"2026-11-14T13:00:00Z","future":{"nested":[1,2]}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			data := `{"seq":9,"topics":[{"name":"t","body":"legacy","changed":2,"changed_at":"2026-11-14T13:00:00Z","body_time_proof":` + raw + `}]}`
			var st State
			if err := json.Unmarshal([]byte(data), &st); err != nil {
				t.Fatalf("optional bad evidence made board unreadable: %v", err)
			}
			assertBodyTime(t, st.Topics[0], "")
			mustApply(t, &st, "")
			assertBodyTime(t, st.Topics[0], "")
			got, err := json.Marshal(st.Topics[0].BodyTimeProof)
			if err != nil || string(got) != raw {
				t.Fatalf("no-op changed unsupported/malformed proof: %s, %v", got, err)
			}
			mustApply(t, &st, topicXML("t", "new body"))
			assertBodyTime(t, st.Topics[0], testNow.Format(time.RFC3339))
		})
	}
}

func TestBodyTimeSupportedVersionMalformedShapeStaysUnknown(t *testing.T) {
	d := TopicDoc{Name: "t", Body: "actual body", Changed: 7, ChangedAt: "2026-11-14T13:00:00Z"}
	d.mintBodyTimeProof()
	data, err := json.Marshal(d.BodyTimeProof)
	if err != nil {
		t.Fatal(err)
	}
	// Go's case-insensitive field decoder can fill At from AT after an
	// explicit null at. It must not bypass strict proof-shape rejection.
	malformed := strings.Replace(string(data), `"at":`, `"at":null,"AT":`, 1)
	if err := json.Unmarshal([]byte(malformed), &d.BodyTimeProof); err != nil {
		t.Fatal(err)
	}
	if d.BodyTimeProof.At != d.ChangedAt || d.BodyTimeProof.raw == nil {
		t.Fatal("fixture must retain matching decoded fields but reject the proof shape")
	}
	assertBodyTime(t, d, "")
	st := &State{Seq: 7, Topics: []TopicDoc{d}}
	var loaded State
	if err := json.Unmarshal(mustMarshal(t, st), &loaded); err != nil {
		t.Fatal(err)
	}
	mustApply(t, &loaded, "")
	assertBodyTime(t, loaded.Topics[0], "")
	got, err := json.Marshal(loaded.Topics[0].BodyTimeProof)
	if err != nil || string(got) != malformed {
		t.Fatalf("no-op changed rejected evidence: %s, %v", got, err)
	}
}

func TestBodyTimeActualBatchAndNeutralUpdates(t *testing.T) {
	st := &State{}
	applyBodyTimeAt(t, st, topicXML("t", "one\n\ntwo"), testNow)
	first := topicEntry(t, st, "t")
	assertBodyTime(t, first, testNow.Format(time.RFC3339))
	if len(first.Marks) != 0 {
		t.Fatal("initial body unexpectedly marked")
	}
	applyBodyTimeAt(t, st, topicXML("t", "one\n\ntwo\n\nthree"), testNow.Add(time.Minute))
	before := topicEntry(t, st, "t")
	for _, commands := range []string{
		"", topicXML("t", before.Body), addXML("technical", "t", "Another activity"),
		`<wb:topic name="t"><wb:log>unrelated log</wb:log></wb:topic>`,
		`<wb:topic name="t"><wb:close result="done"/></wb:topic>`,
	} {
		applyBodyTimeAt(t, st, commands, testNow.Add(time.Hour))
		got := topicEntry(t, st, "t")
		if got.ChangedAt != before.ChangedAt || !reflect.DeepEqual(got.BodyTimeProof, before.BodyTimeProof) {
			t.Fatalf("neutral update renewed body age: %s", commands)
		}
		assertBodyTime(t, got, before.ChangedAt)
	}
	// Legacy recap metadata still participates in overall recency, not body age.
	st.Topics[0].Recap, st.Topics[0].RecapAt = "legacy recap", testNow.Add(2*time.Hour).Format(time.RFC3339)
	st.Topics[0].RecapChanged = st.Seq
	applyBodyTimeAt(t, st, "", testNow.Add(3*time.Hour))
	assertBodyTime(t, st.Topics[0], before.ChangedAt)
	view := BuildView(st, ViewInput{Layout: LayoutFull})
	if view.Topics[0].UpdatedAt == view.Topics[0].BodyUpdatedAt {
		t.Fatal("overall activity substituted for body time")
	}
	refused(t, st, topicXML("t", "<html>refused mutation</html>")+`<wb:answer id="999" value="ok"/>`)
	assertBodyTime(t, st.Topics[0], before.ChangedAt)
}

func TestBodyTimeLegacyBackfillNeverMintsProof(t *testing.T) {
	for _, commands := range []string{"", topicXML("t", "legacy"), addXML("technical", "t", "Activity")} {
		st := &State{Seq: 9, Topics: []TopicDoc{{Name: "t", Body: "legacy", Changed: 2}}}
		mustApply(t, st, commands)
		if st.Topics[0].ChangedAt == "" || st.Topics[0].BodyTimeProof != nil {
			t.Fatalf("backfill semantics/proof changed: %+v", st.Topics[0])
		}
		assertBodyTime(t, st.Topics[0], "")
	}
}

func TestBodyTimeReturnToLogicalOriginAcrossAdoption(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, commands := range []string{
			topicXML("Legacy", "away") + topicXML("Legacy", "one\n\ntwo"),
			`<wb:topic name="adopted" label="Legacy"><wb:body>away</wb:body><wb:body>one

two</wb:body></wb:topic>`,
			topicXML("Legacy", "away") + `<wb:topic name="adopted" label="Legacy"><wb:body>one

two</wb:body></wb:topic>`,
			`<wb:topic name="adopted" label="Legacy"><wb:body></wb:body><wb:body>one

two</wb:body></wb:topic>`,
		} {
			st := &State{}
			mustApply(t, st, topicXML("Legacy", "one\n\ntwo"))
			if legacy {
				st.Topics[0].BodyTimeProof = nil
			}
			before := st.Topics[0]
			applyBodyTimeAt(t, st, commands, testNow.Add(time.Hour))
			got := st.Topics[0]
			if got.Body != before.Body || got.Changed != before.Changed || got.ChangedAt != before.ChangedAt || !reflect.DeepEqual(got.Marks, before.Marks) || !reflect.DeepEqual(got.BodyTimeProof, before.BodyTimeProof) {
				t.Fatalf("adoption/return lost logical origin (legacy %v): %s\nbefore %+v\nafter %+v", legacy, commands, before, got)
			}
			want := before.ChangedAt
			if legacy {
				want = ""
			}
			assertBodyTime(t, got, want)
		}
	}
	st := &State{}
	mustApply(t, st, topicXML("Legacy", "one\n\ntwo"))
	applyBodyTimeAt(t, st, `<wb:topic name="adopted" label="Legacy"><wb:body>one

two changed</wb:body></wb:topic>`, testNow.Add(time.Hour))
	assertBodyTime(t, st.Topics[0], testNow.Add(time.Hour).Format(time.RFC3339))
	if !reflect.DeepEqual(st.Topics[0].Marks, marks(1, MarkMod)) {
		t.Fatal("adoption plus net edit did not diff the logical origin")
	}
}

func TestBodyTimeSameSecondDifferentBatchesAndClear(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "one"))
	original := st.Topics[0]
	mustApply(t, st, topicXML("t", "two"))
	mustApply(t, st, topicXML("t", "one"))
	current := st.Topics[0]
	if current.ChangedAt != original.ChangedAt || current.Changed == original.Changed || current.BodyTimeProof.Digest == original.BodyTimeProof.Digest {
		t.Fatal("fixture must distinguish same-second edit-away/edit-back batches")
	}
	assertBodyTime(t, current, current.ChangedAt)
	current.BodyTimeProof = original.BodyTimeProof
	assertBodyTime(t, current, "")
	// Even without block marks, a real byte mutation earns fresh proof.
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old>one</wb:old><wb:new>one </wb:new></wb:edit></wb:topic>`)
	assertBodyTime(t, st.Topics[0], testNow.Format(time.RFC3339))
	if len(st.Topics[0].Marks) != 0 {
		t.Fatal("block-neutral edit should have no marks")
	}
	mustApply(t, st, `<wb:topic name="t"><wb:body></wb:body></wb:topic>`)
	if got := topicEntry(t, st, "t"); got.BodyTimeProof != nil || got.Changed != 0 || got.ChangedAt != "" {
		t.Fatal("clear retained proof or body metadata")
	}
	mustApply(t, st, topicXML("t", "one"))
	assertBodyTime(t, st.Topics[0], testNow.Format(time.RFC3339))
}
