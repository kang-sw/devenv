package wsdashboard

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestUserMessageRouteCapturesAndValidatesOffset(t *testing.T) {
	f := startPageFixture(t)
	for _, offset := range []int{-720, 0, 540, 840} {
		id, _ := f.send(t, fmt.Sprintf(`{"text":"hello","utcOffsetMinutes":%d}`, offset))
		held := f.held(t)
		e := held[len(held)-1].Entry
		if e.UTCOffsetMinutes == nil || *e.UTCOffsetMinutes != offset || e.SentAt == 0 {
			t.Fatalf("stored = %+v", e)
		}
		at := time.UnixMilli(e.SentAt).In(time.FixedZone("", offset*60)).Format("2006-01-02T15:04-07:00")
		if !strings.HasPrefix(e.Text(), `<ws:user at="`+at+`">`) {
			t.Fatal(e.Text())
		}
		code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+id+`"}`)
		if code != http.StatusOK {
			t.Fatalf("steer = %d %s", code, raw)
		}
	}
	before := len(f.held(t))
	for _, raw := range []string{`{"text":"bad","utcOffsetMinutes":-721}`, `{"text":"bad","utcOffsetMinutes":841}`, `{"text":"bad","utcOffsetMinutes":1.5}`, `{"text":"bad","utcOffsetMinutes":"540"}`} {
		code, _, _ := postJSON(t, f.base+"/s/livesession/message", f.base, raw)
		if code != http.StatusBadRequest {
			t.Fatalf("accepted %s: %d", raw, code)
		}
	}
	if len(f.held(t)) != before {
		t.Fatal("invalid request wrote an entry")
	}
	f.send(t, `{"text":"legacy request"}`)
	held := f.held(t)
	if held[len(held)-1].Entry.UTCOffsetMinutes != nil {
		t.Fatal("absent offset became UTC")
	}
}

func TestUserMessageConversationBundlesKeepCapsulesAndImageOwnership(t *testing.T) {
	dir := t.TempDir()
	var texts []string
	offset := 540
	for i := 1; i <= 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("0123456789abcdef-%d.png", i))
		if err := os.WriteFile(path, []byte(pngBytes), 0600); err != nil {
			t.Fatal(err)
		}
		e := wsinbox.Entry{SentAt: testNow.Add(time.Duration(i) * time.Minute).UnixMilli(), UTCOffsetMinutes: &offset,
			Parts:    []string{fmt.Sprintf("item %d <b>literal</b> [Image #1] &amp;</ws:user>", i)},
			Chips:    []wsinbox.Chip{{ID: fmt.Sprint(i), Answer: "ok"}},
			Mentions: []wsinbox.Mention{{Topic: "plan", Text: fmt.Sprintf("mention %d", i)}},
			Picks:    []wsinbox.Pick{{URL: "http://localhost:5173", Selector: fmt.Sprintf("#pick%d", i)}},
			Consoles: []wsinbox.Console{{URL: "http://localhost:5173", Lines: []string{"error"}}},
			Images:   []string{path}, ImageNums: []int{1}}
		texts = append(texts, e.Text())
	}
	ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: strings.Join(texts, "\n\n")})}
	original, _ := json.Marshal(ring)
	cache := newEntryCache()
	for n := 0; n < 2; n++ {
		v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, wslive.Lead{}), Ring: ring, Now: testNow, AttachDir: dir})
		blocks := v.Conversation.Entries[0].UserBlocks
		if len(blocks) != 3 {
			t.Fatalf("blocks = %+v", blocks)
		}
		for i, b := range blocks {
			if len(b.Answers) != 1 || b.Answers[0].IDs != fmt.Sprint(i+1) || len(b.Mentions) != 1 || len(b.Picks) != 1 || len(b.Consoles) != 1 || len(b.Images) != 1 || b.Images[0].Num != 1 || !strings.Contains(b.Images[0].Path, fmt.Sprintf("-%d.png", i+1)) || !strings.Contains(b.Text, "&amp;</ws:user>") {
				t.Fatalf("block %d = %+v", i, b)
			}
		}
		set, err := renderSessionRegions(v, cache)
		if err != nil {
			t.Fatal(err)
		}
		markup := string(set[regionEntries].HTML)
		if strings.Count(markup, `class="msg user"`) != 3 || strings.Count(markup, `data-entry="`+ring[0].Name+`"`) != 1 || strings.Count(markup, `data-f="user"`) != 3 || strings.Contains(markup, "<b>literal</b>") {
			t.Fatalf("bad batch markup: %s", markup)
		}
		for i := 1; i <= 3; i++ {
			if !strings.Contains(markup, fmt.Sprintf(`src="/s/self/attachment/0123456789abcdef-%d.png"`, i)) {
				t.Fatal(markup)
			}
		}
	}
	after, _ := json.Marshal(ring)
	if string(original) != string(after) || cache.buildCount() != 1 {
		t.Fatal("mutated source or bypassed cache")
	}
	if err := os.Remove(filepath.Join(dir, "0123456789abcdef-1.png")); err != nil {
		t.Fatal(err)
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, wslive.Lead{}), Ring: ring, Now: testNow, AttachDir: dir})
	set, err := renderSessionRegions(v, cache)
	if err != nil {
		t.Fatal(err)
	}
	markup := string(set[regionEntries].HTML)
	if cache.buildCount() != 2 || strings.Contains(markup, `src="/s/self/attachment/0123456789abcdef-1.png"`) || !strings.Contains(markup, "Image #1: "+filepath.Join(dir, "0123456789abcdef-1.png")) {
		t.Fatalf("stale batch attachment cache: %s", markup)
	}
	if !strings.Contains(mustAsset(t, "dashboard.css"), ".user-batch { display: flex; flex-direction: column; gap: 10px; }") {
		t.Fatal("batch lost user-bubble flex layout")
	}
}

func TestUserMessageConversationMixedLegacyAndMalformed(t *testing.T) {
	env := wsinbox.Entry{SentAt: 1, Parts: []string{"inside"}, Chips: []wsinbox.Chip{{ID: "1", Answer: "ok"}}}.Text()
	outside := "<wb:answers>\n2=no\n</wb:answers>\n\nreport"
	entries := []ConvEntry{{RingEntry: wslive.RingEntry{Kind: "user", Text: outside + "\n\n" + env + "\n\nmail"}, Key: "mixed"}}
	splitUserEntries(entries, "", "self")
	blocks := entries[0].UserBlocks
	if len(blocks) != 3 || blocks[0].Text != outside || len(blocks[0].Answers) != 0 || len(blocks[1].Answers) != 1 || blocks[2].Text != "mail" {
		t.Fatalf("mixed = %+v", blocks)
	}
	for _, text := range []string{env + "\n\n<ws:user", `<ws:user at="bad">text</ws:user>`} {
		entries = []ConvEntry{{RingEntry: wslive.RingEntry{Kind: "user", Text: text}, Key: "bad"}}
		splitUserEntries(entries, "", "self")
		if entries[0].Text != text || len(entries[0].UserBlocks) != 0 || len(entries[0].Answers) != 0 {
			t.Fatal("malformed envelope lost")
		}
		markup := leadMarkdownView(t, []wslive.RingEntry{entries[0].RingEntry}, "")
		if !strings.Contains(string(markup), html.EscapeString(text)) {
			t.Fatalf("not plain: %s", markup)
		}
	}
	entries = []ConvEntry{{RingEntry: wslive.RingEntry{Kind: "user", Text: outside}, Key: "legacy"}}
	splitUserEntries(entries, "", "self")
	if len(entries[0].Answers) != 1 || entries[0].Text != "report" || entries[0].EnvelopeAt != "" {
		t.Fatal("legacy parsing changed")
	}
}

func TestUserMessageClockAndMinuteRefresh(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	fns := composerFunc(t, src, "fmtTime") + composerFunc(t, src, "tickTimes")
	runNode(t, "user message time", `
var bad=[]; function check(name,got,want){if(got!==want)bad.push(name+": "+got+" != "+want);}
var wsI18n={lang:"ko"}; function wsT(k,n){return n+"분 전";}
var clock=Date.parse("2026-11-15T11:42:00+09:00"), doc={hidden:false},shHidden=false,window={MutationObserver:function(){}},tickSets=null;
var attrs={"data-ms":clock,"data-f":"user","datetime":"2026-11-15T11:42+09:00"};
var t={getAttribute:function(k){return attrs[k];},textContent:""};
function q(sel){ if(sel.indexOf("time")===0 && sel.indexOf('data-f="user"')<0)bad.push("tick does not select user clocks"); return sel.indexOf("time")===0?[t]:[]; }
function pace(){}
`+fns+`
fmtTime(t,clock+59*60000);check("59 minutes",t.textContent,"59분 전");check("hover",t.title,"2026-11-15T11:42+09:00");
fmtTime(t,clock+60*60000);check("60 minutes",t.textContent,"11:42");
fmtTime(t,Date.parse("2026-11-16T00:00:00+09:00"));check("other day",t.textContent,"11/15 11:42");
Date.now=function(){return clock+2*60000;};tickTimes();check("tick",t.textContent,"2분 전");
Date.now=function(){return clock+3*60000;};tickTimes();check("minute advances",t.textContent,"3분 전");
attrs.datetime="2026-11-15T23:50-03:30";attrs["data-ms"]=Date.parse(attrs.datetime);fmtTime(t,Date.parse("2026-11-16T01:00-03:30"));check("negative offset midnight",t.textContent,"11/15 23:50");
process.stdout.write(bad.join("\n"));`)
}

func TestUserMessageComposerSendsBrowserOffset(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	runNode(t, "browser offset", selectionCodecSource(t)+composerToastPrelude+composerFunc(t, src, "sendMessage")+`
var sending=false, captured=null;
Date.prototype.getTimezoneOffset=function(){return -345;};
post=function(route,body,done){captured=body;done(true,{});};
ctext.value="hello";sendMessage();check("offset east of UTC",captured.utcOffsetMinutes,345);
process.stdout.write(bad.join("\n"));`)
}
