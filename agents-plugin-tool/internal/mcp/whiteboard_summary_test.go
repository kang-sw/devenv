package mcp

import (
	"reflect"
	"strings"
	"testing"
)

func whiteboardSummaryLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "topic added: ") || strings.HasPrefix(line, "topic updated: ") {
			out = append(out, line)
		}
	}
	return out
}

func TestWhiteboardUpdateSummariesPreservePrefixLinesAndEffectiveState(t *testing.T) {
	f := newWhiteboardFixture(t)
	text := f.update(t, `<wb:topic name="rollout"><wb:add class="check"><wb:title>Verify</wb:title></wb:add></wb:topic>`)
	full, embed := resultLine(t, text, "full: "), resultLine(t, text, "embed: ")
	oldLines := "ids: (01)\nfull: " + full + "\nembed: " + embed + "\n"
	if !strings.HasPrefix(text, oldLines) || full != "ai-docs/whiteboards/"+f.replyID()+"/index.html" || !strings.HasPrefix(embed, "ai-docs/whiteboards/"+f.replyID()+"-") || !strings.HasSuffix(text, "topic added: rollout (state: discussing, default)\n") {
		t.Fatalf("prefix/value/summary compatibility: %s", text)
	}
	for _, tc := range []struct{ commands, want string }{
		{`<wb:topic name="explicit"><wb:log>Start</wb:log><wb:state value="waiting"/></wb:topic>`, "topic added: explicit (state: waiting, explicit)"},
		{`<wb:topic name="run"><wb:log>Start</wb:log><wb:state value="implementing" owner="executor" eta_minutes="40"/></wb:topic>`, "topic added: run (state: implementing, explicit)"},
		{`<wb:topic name="run"><wb:eta minutes="50" reason="More work"/></wb:topic>`, "topic updated: run (eta 50m)"},
		{`<wb:topic name="explicit"><wb:state value="awaiting-confirmation"/></wb:topic>`, "topic updated: explicit (state: awaiting-confirmation)"},
	} {
		text = f.update(t, tc.commands)
		if got := whiteboardSummaryLines(text); !reflect.DeepEqual(got, []string{tc.want}) || strings.Contains(text, "ids: ") {
			t.Fatalf("summary/conditional IDs: %q", text)
		}
		if !strings.HasPrefix(text, "full: "+full+"\nembed: ") {
			t.Fatalf("old result lines changed: %s", text)
		}
	}
	text = f.update(t, `<wb:topic name="content"><wb:body>No ask</wb:body></wb:topic>`)
	if !strings.Contains(text, `info: discussing_without_open_items:`) || !strings.Contains(text, `topic added: content (state: discussing, default)`) {
		t.Fatal(text)
	}
}

func TestWhiteboardUpdateSummariesNetMixedTopicsAnswersAndLifecycle(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="a" label="A"><wb:add class="check"><wb:title>One</wb:title></wb:add><wb:add class="check"><wb:title>Two</wb:title></wb:add><wb:body>Original</wb:body></wb:topic><wb:topic name="b"><wb:add class="check"><wb:title>Three</wb:title></wb:add></wb:topic>`)
	for _, tc := range []struct {
		commands string
		want     []string
	}{
		{`<wb:topic name="b"><wb:body/></wb:topic><wb:answer ids="1 3 2" value="yes">Yes.</wb:answer><wb:topic name="a"><wb:close result="Done"/></wb:topic>`, []string{"topic updated: b (answered 1)", "topic updated: a (answered 2, closed)"}},
		{`<wb:topic name="new"><wb:add class="check"><wb:title>New</wb:title></wb:add></wb:topic><wb:answer id="4" value="yes">Yes.</wb:answer><wb:topic name="new"><wb:body>More</wb:body></wb:topic>`, []string{"topic added: new (state: discussing, default)"}},
		{`<wb:patch id="3"><wb:title>Changed</wb:title></wb:patch><wb:topic name="b"><wb:body>Body</wb:body></wb:topic>`, []string{"topic updated: b (patched 1, body)"}},
		{`<wb:topic name="b"><wb:body>Interim</wb:body><wb:body>Body</wb:body></wb:topic><wb:topic name="a" label="Temporary"/><wb:topic name="a" label="A"/>`, nil},
		{`<wb:topic name="b"><wb:state value="waiting"/><wb:state value="discussing"/></wb:topic>`, nil},
		{`<wb:answer id="3" value="yes">Yes.</wb:answer>`, nil},
		{``, nil},
		{`<wb:topic name="adopted" label="b"/>`, []string{"topic updated: adopted (renamed from b)"}},
		{`<wb:topic name="a"><wb:log>Reopened</wb:log></wb:topic>`, []string{"topic updated: a (logged 1, reopened)"}},
	} {
		text := f.update(t, tc.commands)
		if got := whiteboardSummaryLines(text); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: got %q, want %q", tc.commands, got, tc.want)
		}
	}
}

func TestWhiteboardUpdateSummaryClosedAdoptionKeepsNetLifecycle(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="Old"><wb:log>History</wb:log><wb:close result="Done"/></wb:topic>`)
	text := f.update(t, `<wb:topic name="new"><wb:body/></wb:topic><wb:topic name="Old"><wb:body/></wb:topic><wb:topic name="new" label="Old"/>`)
	if got := whiteboardSummaryLines(text); !reflect.DeepEqual(got, []string{"topic updated: new (renamed from Old)"}) {
		t.Fatalf("adoption invented a close: %s", text)
	}
}

func TestWhiteboardUpdateSummaryRefusalIsAtomic(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, "")
	before, err := f.s.sessions.readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	resp := f.call(t, "whiteboard.update", map[string]any{"commands": `<wb:topic name="fresh"><wb:state value="waiting"/><wb:log>Too late</wb:log></wb:topic>`})
	text := toolText(t, resp)
	if !isToolError(resp) || !strings.Contains(text, "state requires a known open topic") || len(whiteboardSummaryLines(text)) != 0 || strings.Contains(text, "\nfull: ") {
		t.Fatal(text)
	}
	after, err := f.s.sessions.readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Whiteboard, after.Whiteboard) {
		t.Fatal("refusal persisted a default")
	}
}
