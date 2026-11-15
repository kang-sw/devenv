package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardCommandAcceptanceFirstTextRetryAndSequence(t *testing.T) {
	live := newLiveFixture(t, true)
	f := effectsWhiteboardFixture(live)
	f.update(t, `<wb:topic name="t"><wb:add class="policy"><wb:title>Pick</wb:title><wb:option>One</wb:option><wb:option>Two</wb:option></wb:add></wb:topic>`)
	read := func() *wswhiteboard.State {
		t.Helper()
		rec, err := f.s.sessions.readWhiteboard(f.key)
		if err != nil {
			t.Fatal(err)
		}
		return rec.Whiteboard
	}
	before := read()
	original := whiteboardRename
	publications := 0
	whiteboardRename = func(a, b string) error {
		if strings.HasSuffix(b, ".html") || strings.HasSuffix(b, ".stamp.js") {
			publications++
		}
		return original(a, b)
	}
	t.Cleanup(func() { whiteboardRename = original })
	large := strings.Repeat("large accepted body\n\n", 400)
	badEdit := `<wb:edit replace_all="false"><wb:old>missing anchor</wb:old><wb:new>  full &amp; <b>replacement</b>  </wb:new></wb:edit>`
	badPatch := `<wb:patch id="999"><wb:why>  full rationale &amp; text  </wb:why></wb:patch>`
	commands := `<wb:topic name="t"><wb:body>` + large + `</wb:body><wb:panel recap="Keep panel">accepted panel</wb:panel>` + badEdit + `<wb:log>accepted log</wb:log><wb:add class="check"><wb:title>accepted ask</wb:title></wb:add></wb:topic><wb:answer id="1" value="">comment, not a choice</wb:answer>` + badPatch
	resp := f.call(t, "whiteboard.update", map[string]any{"commands": commands})
	text := toolText(t, resp)
	var envelope struct {
		Result struct {
			Content []struct{ Text string }
			IsError bool `json:"isError"`
		}
	}
	if err := json.Unmarshal([]byte(resp), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Result.IsError || len(envelope.Result.Content) != 1 || envelope.Result.Content[0].Text != text || !strings.HasPrefix(text, "applied: 5 accepted commands committed") {
		t.Fatalf("mixed first-text envelope: %s", resp)
	}
	start := strings.Index(text, "<wb:failed>")
	end := strings.Index(text, "</wb:failed>")
	if start < 0 || end < start {
		t.Fatal("missing retry tree", text)
	}
	tree := text[start : end+len("</wb:failed>")]
	if !strings.Contains(tree, badEdit) || !strings.Contains(tree, badPatch) || strings.Contains(tree, "large accepted body") || strings.Contains(tree, "accepted panel") || strings.Contains(text, "nothing was applied") {
		t.Fatal("retry payload inaccurate", tree)
	}
	after := read()
	if after.Seq != before.Seq+1 || after.LastEmbedSeq != after.Seq || after.Items[0].Changed != after.Items[1].Changed || after.AnswerSeq != after.Seq || after.Items[0].Status != wswhiteboard.StatusCommented || publications != 3 || len(effectsRecords(t, live, live.replyA)) != 2 {
		t.Fatal("mixed call not one durable/publication unit", publications, after)
	}
	publications = 0
	retry := strings.TrimSuffix(strings.TrimPrefix(tree, "<wb:failed>\n"), "\n</wb:failed>")
	retry = strings.ReplaceAll(retry, "missing anchor", large[:len("large accepted body")])
	retry = strings.ReplaceAll(retry, `replace_all="false"`, `replace_all="true"`)
	retry = strings.ReplaceAll(retry, `id="999"`, `id="1"`)
	f.update(t, retry)
	retried := read()
	if retried.Counter != after.Counter || retried.PanelCounter != after.PanelCounter || publications != 3 {
		t.Fatal("corrected retry duplicated accepted work")
	}
	if retried.Items[0].Rationale != "  full rationale & text  " || !strings.Contains(retried.TopicBody("t"), "  full &amp; <b>replacement</b>  ") {
		t.Fatal("retry lost raw fields/body")
	}
	// Refusals (including an unpartitionable valid prefix) preserve all bytes,
	// the numeric stamp, embed baseline and conversation projection.
	for _, bad := range []string{
		`<wb:patch id="999"/>`,
		`<wb:topic name="t"><wb:panel recap="never">prefix</wb:panel></wb:topic><wb:topic name="t"><wb:body>unterminated`,
		`<wb:topic name="t"><wb:panel recap="never">prefix</wb:panel></wb:topic><wb:topic name="outer">junk<wb:topic name="inner"><wb:log>inside</wb:log></wb:topic><wb:floor id="999"/></wb:topic>`,
		`<wb:topic name="t"><wb:panel recap="never">prefix</wb:panel></wb:topic><wb:topic name="outer"><!-- </wb:topic> --><wb:topic name="inner"><wb:log>inside</wb:log></wb:topic><wb:floor id="999"/></wb:topic>`,
	} {
		files := whiteboardTreeForTest(t, f.dir())
		records := len(effectsRecords(t, live, live.replyA))
		publications = 0
		r := f.call(t, "whiteboard.update", map[string]any{"commands": bad})
		if !isToolError(r) || !strings.Contains(toolText(t, r), "nothing was applied") || publications != 0 || len(effectsRecords(t, live, live.replyA)) != records {
			t.Fatal("failed-only call changed effects/publication", r)
		}
		assertWhiteboardTreeForTest(t, f.dir(), files)
	}
	for _, noop := range []string{`<wb:floor id="1"/>`, ``} {
		before := read()
		publications = 0
		records := len(effectsRecords(t, live, live.replyA))
		f.update(t, noop)
		after := read()
		if after.Seq != before.Seq+1 || after.LastEmbedSeq != after.Seq || after.EntrySeq != before.EntrySeq || publications != 3 || len(effectsRecords(t, live, live.replyA)) != records {
			t.Fatal("accepted no-op/empty sequence changed", noop)
		}
	}
	query := toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"status": "commented", "depth": "full"}}))
	if !strings.Contains(query, "commented 1") || !strings.Contains(query, "comment, not a choice") || !strings.Contains(query, "answered_at:") {
		t.Fatal(query)
	}
	// The last embed and file page expose the same status without a UI redesign.
	st := read()
	for _, path := range []string{f.pagePath(), filepath.Join(f.dir(), st.Embeds[len(st.Embeds)-1].Name)} {
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "user commented") {
			t.Fatal("comment label missing", path, err)
		}
	}
	if !reflect.DeepEqual(st.Items[0].Options, []string{"One", "Two"}) {
		t.Fatal("comment lost choices")
	}
}
