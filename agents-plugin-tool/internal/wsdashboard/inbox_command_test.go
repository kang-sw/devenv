package wsdashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// withCommands rewrites livesession's snapshot with a command list, as an
// adapter that runs slash commands reports it.
func (f *pageFixture) withCommands(t *testing.T, commands ...wslive.SlashCommand) {
	t.Helper()
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "claude", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}, Commands: commands})
}

func TestCommandSendNeedsAnAdapterThatReportsCommands(t *testing.T) {
	f := startPageFixture(t)
	code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, `{"command":"compact"}`)
	if code != http.StatusConflict || !strings.Contains(raw, "runs no slash commands") {
		t.Fatalf("command to an adapter with no list = %d: %s", code, raw)
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("held = %+v", held)
	}
	f.withCommands(t, wslive.SlashCommand{Name: "compact", Source: "builtin"})
	id, merged := f.send(t, `{"command":"compact","args":"keep the plan"}`)
	if merged {
		t.Fatal("command merged")
	}
	held := f.held(t)
	if len(held) != 1 || held[0].ID() != id || held[0].Entry.Command == nil || *held[0].Entry.Command != (wsinbox.Command{Name: "compact", Args: "keep the plan"}) || len(held[0].Entry.Parts) != 0 {
		t.Fatalf("held = %+v", held)
	}
	for _, body := range []string{
		`{"command":"compact","text":"and text"}`,
		`{"command":"two words"}`,
		`{"command":"compact","refs":["0123456789abcdef-1.png"]}`,
	} {
		if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, body); code != http.StatusBadRequest {
			t.Errorf("send %s = %d: %s", body, code, raw)
		}
	}
}

func TestCommandSendRechecksAWarmCommandList(t *testing.T) {
	for _, change := range []string{"removed list", "removed command", "hosted list"} {
		t.Run(change, func(t *testing.T) {
			f := startPageFixture(t)
			f.withCommands(t, wslive.SlashCommand{Name: "compact"})
			// Warm a permissive presentation entry and freeze its clock. A
			// command refusal must not depend on expiry or an index refresh.
			clock := time.Now()
			f.d.snapshots.files.mu.Lock()
			f.d.snapshots.files.now = func() time.Time { return clock }
			f.d.snapshots.files.mu.Unlock()
			f.d.snapshots.Forget(f.dir, "livesession")
			warm := f.d.snapshots.Snapshot(f.dir, "livesession")
			if warm == nil || len(warm.Commands) != 1 || warm.Commands[0].Name != "compact" {
				t.Fatal("command-list presentation cache was not warmed")
			}
			want := http.StatusConflict
			switch change {
			case "removed list":
				f.withCommands(t)
			case "removed command":
				f.withCommands(t, wslive.SlashCommand{Name: "other"})
				want = http.StatusBadRequest
			case "hosted list":
				// Disk still permits compact, but authoritative hosted RAM does not.
				hosted := *warm
				hosted.Commands = nil
				f.d.snapshots.SetLive(f.dir, "livesession", hosted)
			}
			if change != "hosted list" {
				if cached := f.d.snapshots.Snapshot(f.dir, "livesession"); cached == nil || len(cached.Commands) != 1 || cached.Commands[0].Name != "compact" {
					t.Fatal("test lost its warm permissive presentation entry")
				}
			}
			if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, `{"command":"compact"}`); code != want {
				t.Fatalf("removed command admitted: %d %s, want %d", code, raw, want)
			}
			if held := f.held(t); len(held) != 0 {
				t.Fatalf("removed command was held: %+v", held)
			}
		})
	}
}

func TestCommandEntryStandsAloneAndSteersAsText(t *testing.T) {
	f := startPageFixture(t)
	f.withCommands(t, wslive.SlashCommand{Name: "compact"})
	text, _ := f.send(t, `{"text":"before"}`)
	cmd, _ := f.send(t, `{"command":"compact"}`)
	after, merged := f.send(t, `{"text":"after"}`)
	if merged || after == text || after == cmd {
		t.Fatalf("text after a command merged=%v id=%s", merged, after)
	}
	if held := f.held(t); len(held) != 3 || inboxBody(t, held[0].Entry) != "before" || inboxBody(t, held[1].Entry) != "/compact" || inboxBody(t, held[2].Entry) != "after" {
		t.Fatalf("held = %+v", held)
	}
	code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+cmd+`"}`)
	if code != http.StatusOK {
		t.Fatalf("steer of a command = %d: %s", code, raw)
	}
	// The steered command is a steered text entry holding its typed line,
	// ahead of the follow-ups; the command entry is gone.
	held := f.held(t)
	if len(held) != 3 || !held[0].Entry.Steer || held[0].Entry.Command != nil || inboxBody(t, held[0].Entry) != "/compact" {
		t.Fatalf("held after the steer = %+v", held)
	}
	for _, r := range held {
		if r.ID() == cmd {
			t.Fatalf("command entry %s still held after its steer", cmd)
		}
	}
}

func TestCancelAndInterruptReturnACommandAsACommand(t *testing.T) {
	f := startPageFixture(t)
	f.withCommands(t, wslive.SlashCommand{Name: "compact"})
	cmd, _ := f.send(t, `{"command":"compact","args":"focus"}`)
	code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+cmd+`"}`)
	if code != http.StatusOK {
		t.Fatalf("cancel = %d: %s", code, raw)
	}
	var one struct {
		Text    string           `json:"text"`
		Command *wsinbox.Command `json:"command"`
	}
	if err := json.Unmarshal([]byte(raw), &one); err != nil || one.Command == nil || *one.Command != (wsinbox.Command{Name: "compact", Args: "focus"}) || one.Text != "" {
		t.Fatalf("cancel answer = %s (%v)", raw, err)
	}

	f.send(t, `{"text":"plain"}`)
	f.send(t, `{"command":"compact"}`)
	code, _, raw = postJSON(t, f.base+"/s/livesession/interrupt", f.base, ``)
	if code != http.StatusOK {
		t.Fatalf("interrupt = %d: %s", code, raw)
	}
	var ans struct {
		Held []struct {
			Text    string           `json:"text"`
			Command *wsinbox.Command `json:"command"`
		} `json:"held"`
	}
	if err := json.Unmarshal([]byte(raw), &ans); err != nil || len(ans.Held) != 2 || ans.Held[0].Command != nil || ans.Held[0].Text != "plain" ||
		ans.Held[1].Command == nil || ans.Held[1].Command.Name != "compact" {
		t.Fatalf("interrupt answer = %s (%v)", raw, err)
	}
}

// TestCommandIsHeldOnlyFromThisMachineAndOnlyWhenListed pins the command
// gate: a LAN token viewer's command is refused, and so is a name the
// session did not report, while a browser on this machine sending a listed
// name is held. The LAN page carries no command list for the picker.
func TestCommandIsHeldOnlyFromThisMachineAndOnlyWhenListed(t *testing.T) {
	f := startPageFixture(t, lanMode)
	f.withCommands(t, wslive.SlashCommand{Name: "compact", Source: "builtin"})
	local, lan := fsViewers(t, f)

	rec := serve(f.d, lan, http.MethodPost, "/s/livesession/message", `{"command":"compact"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "browser on this machine") {
		t.Fatalf("LAN command = %d: %s", rec.Code, rec.Body.String())
	}
	rec = serve(f.d, local, http.MethodPost, "/s/livesession/message", `{"command":"deploy"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not a command this session runs") {
		t.Fatalf("unlisted command = %d: %s", rec.Code, rec.Body.String())
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("held after the refusals = %+v", held)
	}
	rec = serve(f.d, local, http.MethodPost, "/s/livesession/message", `{"command":"compact","args":"keep"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("local listed command = %d: %s", rec.Code, rec.Body.String())
	}
	if held := f.held(t); len(held) != 1 || held[0].Entry.Command == nil || *held[0].Entry.Command != (wsinbox.Command{Name: "compact", Args: "keep"}) {
		t.Fatalf("held = %+v", held)
	}
	// A LAN viewer's plain message is still held.
	if rec = serve(f.d, lan, http.MethodPost, "/s/livesession/message", `{"text":"from the phone"}`); rec.Code != http.StatusOK {
		t.Fatalf("LAN message = %d: %s", rec.Code, rec.Body.String())
	}
	// A LAN text starting with / would run as a command on a hosted
	// backend, so it is refused; a slash later in the line is not.
	for _, body := range []string{`{"text":"/compact"}`, `{"text":"  /x"}`, `{"text":"\n\t/compact now"}`} {
		rec = serve(f.d, lan, http.MethodPost, "/s/livesession/message", body)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "starting with /") {
			t.Fatalf("LAN %s = %d: %s", body, rec.Code, rec.Body.String())
		}
	}
	if held := f.held(t); len(held) != 2 || inboxBody(t, held[1].Entry) != "from the phone" {
		t.Fatalf("held after the LAN slash refusals = %+v", held)
	}
	if rec = serve(f.d, lan, http.MethodPost, "/s/livesession/message", `{"text":"see a/b or /tmp"}`); rec.Code != http.StatusOK {
		t.Fatalf("LAN text with a later slash = %d: %s", rec.Code, rec.Body.String())
	}
	if rec = serve(f.d, local, http.MethodPost, "/s/livesession/message", `{"text":"/compact"}`); rec.Code != http.StatusOK {
		t.Fatalf("local /compact text = %d: %s", rec.Code, rec.Body.String())
	}
	if held := f.held(t); !strings.Contains(inboxBody(t, held[len(held)-1].Entry), "/compact") {
		t.Fatalf("local /compact text not held: %+v", held)
	}

	// Page capabilities are presentation evidence: expire its startup snapshot.
	clock := time.Now().Add(fileCacheValidationInterval)
	f.d.snapshots.files.mu.Lock()
	f.d.snapshots.files.now = func() time.Time { return clock }
	f.d.snapshots.files.mu.Unlock()
	if page := serve(f.d, local, http.MethodGet, "/s/livesession?frame=1", "").Body.String(); !strings.Contains(page, `<i data-name="compact" data-src="builtin"></i>`) {
		t.Error("the local page does not carry the command list")
	}
	if page := serve(f.d, lan, http.MethodGet, "/s/livesession?frame=1", "").Body.String(); strings.Contains(page, `data-region="cmds"`) {
		t.Error("the LAN page carries the command list region")
	}
}
