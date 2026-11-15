package wsdashboard

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestCloseRequestComposerLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "testdata/close-request.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("close request fixture: %v\n%s", err, out)
	}
	t.Log(string(out))
}

func TestCloseRequestRouteLifecycleAndCapsules(t *testing.T) {
	f := startPageFixture(t)
	want := []wsinbox.CloseRequest{{Topic: "unknown-or-closed", Reason: ""}, {Topic: "plan", Reason: "done <safely> & now"}}
	body, err := json.Marshal(map[string]any{"closeRequests": want})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := f.send(t, string(body))
	if held := f.held(t); len(held) != 1 || !reflect.DeepEqual(held[0].Entry.CloseRequests, want) {
		t.Fatalf("held: %+v", held)
	}
	// Each envelope gets its own delivered capsules, alongside the held ones.
	delivered := (wsinbox.Entry{CloseRequests: want[:1]}).Text() + "\n\n" + (wsinbox.Entry{CloseRequests: want[1:]}).Text()
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: delivered}}); err != nil {
		t.Fatal(err)
	}
	_, page := get(t, f.base+"/s/livesession?frame=1")
	for _, capsule := range []string{
		`<span class="achip close-request-chip"><b>Close [unknown-or-closed]</b></span>`,
		`<span class="achip close-request-chip"><b>Close [plan]</b><span class="anote" title="done &lt;safely&gt; &amp; now">done &lt;safely&gt; &amp; now</span></span>`,
	} {
		if strings.Count(page, capsule) != 2 {
			t.Errorf("held and delivered capsule missing: %s", capsule)
		}
	}
	code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`)
	var back struct {
		CloseRequests []wsinbox.CloseRequest `json:"closeRequests"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(raw), &back) != nil || !reflect.DeepEqual(back.CloseRequests, want) {
		t.Fatalf("cancel: %d %s", code, raw)
	}
	id, _ = f.send(t, string(body))
	if code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+id+`"}`); code != http.StatusOK {
		t.Fatalf("steer: %d %s", code, raw)
	}
	if held := f.held(t); len(held) != 1 || !held[0].Entry.Steer || !reflect.DeepEqual(held[0].Entry.CloseRequests, want) {
		t.Fatalf("steered: %+v", held)
	}
	code, _, raw = postJSON(t, f.base+"/s/livesession/interrupt", f.base, ``)
	var interrupted struct {
		Held []struct {
			CloseRequests []wsinbox.CloseRequest `json:"closeRequests"`
		} `json:"held"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(raw), &interrupted) != nil || len(interrupted.Held) != 1 || !reflect.DeepEqual(interrupted.Held[0].CloseRequests, want) {
		t.Fatalf("interrupt: %d %s", code, raw)
	}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"closeRequests":[{"topic":""}]}`, http.StatusBadRequest},
		// This fixture's adapter rejects all command sends before normalization.
		{`{"command":"compact","closeRequests":[{"topic":"plan"}]}`, http.StatusConflict},
	} {
		if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, tc.body); code != tc.status {
			t.Errorf("invalid request status: %d %s", code, raw)
		}
	}
}

func TestCloseRequestUserBlocks(t *testing.T) {
	first := (wsinbox.Entry{CloseRequests: []wsinbox.CloseRequest{{Topic: "one"}}}).Text()
	second := (wsinbox.Entry{CloseRequests: []wsinbox.CloseRequest{{Topic: "two", Reason: "why"}}, Parts: []string{"words"}}).Text()
	entries := []ConvEntry{{RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: first + "\n\n" + second}, Key: "batch"}}
	splitUserEntries(entries, "", "")
	blocks := entries[0].UserBlocks
	if len(blocks) != 2 || len(blocks[0].CloseRequests) != 1 || blocks[0].CloseRequests[0].Topic != "one" || blocks[0].Text != "" || len(blocks[1].CloseRequests) != 1 || blocks[1].CloseRequests[0].Topic != "two" || blocks[1].Text != "words" {
		t.Fatalf("blocks: %+v", blocks)
	}
}
