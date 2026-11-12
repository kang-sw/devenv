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

func TestCommandEntryStandsAloneAndRefusesSteer(t *testing.T) {
	f := startPageFixture(t)
	f.withCommands(t, wslive.SlashCommand{Name: "compact"})
	text, _ := f.send(t, `{"text":"before"}`)
	cmd, _ := f.send(t, `{"command":"compact"}`)
	after, merged := f.send(t, `{"text":"after"}`)
	if merged || after == text || after == cmd {
		t.Fatalf("text after a command merged=%v id=%s", merged, after)
	}
	if held := f.held(t); len(held) != 3 || held[0].Entry.Text() != "before" || held[1].Entry.Text() != "/compact" || held[2].Entry.Text() != "after" {
		t.Fatalf("held = %+v", held)
	}
	code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+cmd+`"}`)
	if code != http.StatusConflict || !strings.Contains(raw, "cannot be steered") {
		t.Fatalf("steer of a command = %d: %s", code, raw)
	}
	if held := f.held(t); held[1].ID() != cmd || held[1].Entry.Steer {
		t.Fatalf("held after the refused steer = %+v", held)
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

	if page := serve(f.d, local, http.MethodGet, "/s/livesession", "").Body.String(); !strings.Contains(page, `<i data-name="compact" data-src="builtin"></i>`) {
		t.Error("the local page does not carry the command list")
	}
	if page := serve(f.d, lan, http.MethodGet, "/s/livesession", "").Body.String(); strings.Contains(page, `data-region="cmds"`) {
		t.Error("the LAN page carries the command list region")
	}
}
