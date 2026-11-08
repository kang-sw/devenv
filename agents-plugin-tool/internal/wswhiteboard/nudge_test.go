package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func applyAt(t *testing.T, st *State, commands string, now time.Time) Outcome {
	t.Helper()
	env := testEnv()
	env.Now = now
	out, problems := Apply(st, commands, env)
	if len(problems) > 0 {
		t.Fatalf("Apply(%s) refused: %v", commands, problems)
	}
	return out
}

func bodies(names ...string) string {
	var sb strings.Builder
	for _, name := range names {
		sb.WriteString(`<wb:topic name="` + name + `">body of ` + name + "</wb:topic>\n")
	}
	return sb.String()
}

// staleBoard holds seven open topics at testNow+30m: A, B, D, E carry only a
// body set at testNow; W has an open item and S an assumed one, both from
// testNow; F's body changed at testNow+30m. X is closed and is not counted.
func staleBoard(t *testing.T) *State {
	t.Helper()
	st := &State{}
	applyAt(t, st, `<wb:add class="technical" topic="W"><wb:title>waits</wb:title></wb:add>
<wb:add class="technical" topic="S"><wb:title>assumed</wb:title></wb:add>`+
		bodies("A", "B", "D", "E", "F", "X")+`<wb:close topic="X"/>`, testNow)
	applyAt(t, st, `<wb:answer id="2" value="ok" assumed="true">fine</wb:answer>`, testNow)
	applyAt(t, st, `<wb:topic name="F">new body of F</wb:topic>`, testNow.Add(30*time.Minute))
	return st
}

const wantStaleInfo = "4 of 7 open topics unchanged for 60+ min (A, B, D, E); if finished, close them with wb:close."

func TestStaleNudgeNamesStaleTopicsOnly(t *testing.T) {
	st := staleBoard(t)
	if out := applyAt(t, st, ``, testNow.Add(59*time.Minute)); out.Info != "" {
		t.Fatalf("no topic is 60 min old yet; info = %q", out.Info)
	}
	out := applyAt(t, st, ``, testNow.Add(61*time.Minute))
	if out.Info != wantStaleInfo {
		t.Fatalf("info = %q\nwant  %q", out.Info, wantStaleInfo)
	}
}

func TestStaleNudgeNeedsFiveOpenTopics(t *testing.T) {
	st := &State{}
	applyAt(t, st, bodies("A", "B", "C", "D", "E")+`<wb:close topic="E"/>`, testNow)
	if out := applyAt(t, st, ``, testNow.Add(2*time.Hour)); out.Info != "" {
		t.Fatalf("four open topics must not nudge; info = %q", out.Info)
	}
	// Reopening the fifth makes it fresh, but the four stale ones now nudge.
	out := applyAt(t, st, `<wb:topic name="E">reopened</wb:topic>`, testNow.Add(2*time.Hour))
	if want := "4 of 5 open topics unchanged for 60+ min (A, B, C, D); if finished, close them with wb:close."; out.Info != want {
		t.Fatalf("info = %q\nwant  %q", out.Info, want)
	}
}

func TestStaleNudgeCapsListedNames(t *testing.T) {
	st := &State{}
	applyAt(t, st, bodies("A", "B", "C", "D", "E", "F", "G"), testNow)
	out := applyAt(t, st, ``, testNow.Add(time.Hour))
	if want := "7 of 7 open topics unchanged for 60+ min (A, B, C, D, E, ...); if finished, close them with wb:close."; out.Info != want {
		t.Fatalf("info = %q\nwant  %q", out.Info, want)
	}
}

func TestStaleNudgeAtMostOncePerHour(t *testing.T) {
	st := staleBoard(t)
	first := testNow.Add(61 * time.Minute)
	if out := applyAt(t, st, ``, first); out.Info == "" {
		t.Fatalf("first stale update must nudge")
	}
	if out := applyAt(t, st, ``, first.Add(59*time.Minute)); out.Info != "" {
		t.Fatalf("a nudge within the hour must not repeat; info = %q", out.Info)
	}
	if out := applyAt(t, st, ``, first.Add(60*time.Minute)); out.Info == "" {
		t.Fatalf("after the hour the nudge must return")
	}
}

func TestStaleNudgeTimeSurvivesReload(t *testing.T) {
	st := staleBoard(t)
	first := testNow.Add(61 * time.Minute)
	if out := applyAt(t, st, ``, first); out.Info == "" {
		t.Fatalf("first stale update must nudge")
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded State
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if out := applyAt(t, &reloaded, ``, first.Add(30*time.Minute)); out.Info != "" {
		t.Fatalf("the last-nudge time must survive a reload; info = %q", out.Info)
	}
}

// A store written before the change times existed has no change time on any
// item or body and no last-nudge time: it reads as never nudged, and its
// topics count as changed at the first update that sees them.
func TestStaleNudgeLegacyStore(t *testing.T) {
	var st State
	legacy := `{"counter":1,"seq":3,"items":[{"id":1,"class":"technical","topic":"A","title":"t","status":"answered","changed":1,"settled":1}],` +
		`"topics":[{"name":"B","body":"b","changed":1},{"name":"C","body":"c","changed":1},{"name":"D","body":"d","changed":2},{"name":"E","body":"e","changed":3}]}`
	if err := json.Unmarshal([]byte(legacy), &st); err != nil {
		t.Fatal(err)
	}
	if out := applyAt(t, &st, ``, testNow); out.Info != "" {
		t.Fatalf("a legacy board's topics have no known age yet; info = %q", out.Info)
	}
	if out := applyAt(t, &st, ``, testNow.Add(time.Hour)); out.Info == "" {
		t.Fatalf("a legacy board must nudge once its topics are an hour past first sight")
	}
}

func TestStaleNudgeNeverRenders(t *testing.T) {
	st := staleBoard(t)
	if out := applyAt(t, st, ``, testNow.Add(61*time.Minute)); out.Info == "" {
		t.Fatalf("stale update must nudge")
	}
	for _, layout := range []Layout{LayoutFull, LayoutEmbed} {
		html, err := RenderPage(BuildView(st, ViewInput{Layout: layout}))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(html, "unchanged for") || strings.Contains(html, "close them with") {
			t.Fatalf("layout %v renders the nudge", layout)
		}
	}
}
