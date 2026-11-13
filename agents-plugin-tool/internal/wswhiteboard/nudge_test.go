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
		sb.WriteString(`<wb:topic name="` + name + `"><wb:body>body of ` + name + "</wb:body></wb:topic>\n")
	}
	return sb.String()
}

// staleBoard holds seven open topics at testNow+30m: A, B, D, E carry only a
// body set at testNow; W has an open item and S an assumed one, both from
// testNow; F's body changed at testNow+30m. X is closed and is not counted.
// An assumed item reads as settled, so S goes stale like a topic with no
// item waiting.
func staleBoard(t *testing.T) *State {
	t.Helper()
	st := &State{}
	applyAt(t, st, `<wb:topic name="W"><wb:add class="technical"><wb:title>waits</wb:title></wb:add></wb:topic>
<wb:topic name="S"><wb:add class="technical"><wb:title>assumed</wb:title></wb:add></wb:topic>`+
		bodies("A", "B", "D", "E", "F", "X")+`<wb:topic name="X"><wb:close result="done"/></wb:topic>`, testNow)
	applyAt(t, st, `<wb:answer id="2" value="ok" assumed="true">fine</wb:answer>`, testNow)
	applyAt(t, st, `<wb:topic name="F"><wb:body>new body of F</wb:body></wb:topic>`, testNow.Add(30*time.Minute))
	return st
}

const wantStaleInfo = "5 of 7 open topics unchanged for 60+ min (S, A, B, D, E); if finished, close them with wb:close and a one-line result."

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
	applyAt(t, st, bodies("A", "B", "C", "D", "E")+`<wb:topic name="E"><wb:close result="done"/></wb:topic>`, testNow)
	if out := applyAt(t, st, ``, testNow.Add(2*time.Hour)); out.Info != "" {
		t.Fatalf("four open topics must not nudge; info = %q", out.Info)
	}
	// Reopening the fifth makes it fresh, but the four stale ones now nudge.
	out := applyAt(t, st, `<wb:topic name="E"><wb:body>reopened</wb:body></wb:topic>`, testNow.Add(2*time.Hour))
	if want := "4 of 5 open topics unchanged for 60+ min (A, B, C, D); if finished, close them with wb:close and a one-line result."; out.Info != want {
		t.Fatalf("info = %q\nwant  %q", out.Info, want)
	}
}

func TestStaleNudgeCapsListedNames(t *testing.T) {
	st := &State{}
	applyAt(t, st, bodies("A", "B", "C", "D", "E", "F", "G"), testNow)
	out := applyAt(t, st, ``, testNow.Add(time.Hour))
	if want := "7 of 7 open topics unchanged for 60+ min (A, B, C, D, E, ...); if finished, close them with wb:close and a one-line result."; out.Info != want {
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

func TestStaleNudgeUsesLogAndPanelActivity(t *testing.T) {
	for _, tc := range []struct {
		name, command string
	}{
		{"log-only", `<wb:log>Current state</wb:log>`},
		{"panel-only", `<wb:panel recap="Current panel">payload</wb:panel>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commands strings.Builder
			for _, name := range []string{"A", "B", "C", "D", "E"} {
				commands.WriteString(`<wb:topic name="` + name + `">` + tc.command + `</wb:topic>`)
			}
			st := &State{}
			if out := applyAt(t, st, commands.String(), testNow); out.Info != "" {
				t.Fatalf("fresh entry-only topics must not nudge: %s", out.Info)
			}
			if out := applyAt(t, st, "", testNow.Add(59*time.Minute)); out.Info != "" {
				t.Fatalf("entry activity is not stale before an hour: %s", out.Info)
			}
			// Refreshing only the stream must reset a topic's stale age just as
			// changing the retired recap did, without needing content or items.
			if out := applyAt(t, st, commands.String(), testNow.Add(61*time.Minute)); out.Info != "" {
				t.Fatalf("new entries must refresh their topics: %s", out.Info)
			}
			if out := applyAt(t, st, "", testNow.Add(120*time.Minute)); out.Info != "" {
				t.Fatalf("refreshed entries are not an hour old: %s", out.Info)
			}
			if out := applyAt(t, st, "", testNow.Add(121*time.Minute)); !strings.HasPrefix(out.Info, "5 of 5 open topics unchanged for 60+ min (A, B, C, D, E)") {
				t.Fatalf("entry-only topics must eventually become stale: %q", out.Info)
			}
		})
	}
}

func TestStaleNudgeUsesConvertedRecapActivity(t *testing.T) {
	legacy := &State{Seq: 1}
	at := testNow.Format(time.RFC3339Nano)
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		legacy.Topics = append(legacy.Topics, TopicDoc{Name: name, Recap: "Current legacy state", RecapChanged: 1, RecapAt: at})
	}
	converted, err := (FolderStore{Dir: t.TempDir()}).Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range converted.Topics {
		if doc.RecapAt != "" || len(doc.Entries) != 1 || !doc.Entries[0].Imported || doc.Entries[0].At != at {
			t.Fatalf("fixture must carry its original recap time only in the imported entry: %+v", doc)
		}
	}
	if out := applyAt(t, converted, "", testNow.Add(59*time.Minute)); out.Info != "" {
		t.Fatalf("conversion must preserve recap freshness: %s", out.Info)
	}
	if out := applyAt(t, converted, "", testNow.Add(61*time.Minute)); !strings.HasPrefix(out.Info, "5 of 5 open topics unchanged for 60+ min (A, B, C, D, E)") {
		t.Fatalf("converted recap topics must age from their recorded time: %q", out.Info)
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
