package mcp

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// terminalHarnessPID stands for the harness process of a terminal session
// in these tests.
const terminalHarnessPID = 9001

// stubHostedProcessAlive makes the login guard read exactly the given
// processes as alive.
func stubHostedProcessAlive(t *testing.T, alive ...int) {
	t.Helper()
	prev := hostedProcessAlive
	hostedProcessAlive = func(pid int) bool {
		for _, p := range alive {
			if p == pid {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { hostedProcessAlive = prev })
}

// keyRecordForTest reads key's stored record.
func keyRecordForTest(t *testing.T, s *Server, key string) sessionRecord {
	t.Helper()
	d, err := s.sessions.dirs()
	if err != nil {
		t.Fatal(err)
	}
	record, ok := s.sessions.readRecord(d, key, partKey)
	if !ok {
		t.Fatalf("key %q has no record", key)
	}
	return record
}

// terminalServer is an ordinary server under a terminal harness whose
// launcher reported terminalHarnessPID.
func terminalServer(t *testing.T, root string) *Server {
	t.Helper()
	t.Setenv(envHostedSession, "")
	t.Setenv(envHostedSessionKey, "")
	t.Setenv(envHarnessPID, strconv.Itoa(terminalHarnessPID))
	s := NewServer(root, "test")
	s.sessionHarness = "pi"
	return s
}

func relogin(t *testing.T, s *Server, root, key string) string {
	t.Helper()
	return dashCall(t, s, "ferrule", map[string]any{"root": root, "relogin_session_key": key})
}

func TestLoginRecordsTheHarnessSessionApartFromTheRoot(t *testing.T) {
	root, server := hostedLoginEnv(t)
	server.sessionHarness = "claude"
	sub := filepath.Join(root, "pkg", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	key, loginRoot := parseLoginResponse(t, callLogin(t, server, 1, sub, map[string]any{"harness_session": "claude-session-1", "harness_cwd": sub + string(filepath.Separator)}))
	rec := keyRecordForTest(t, server, key)
	if rec.Harness != "claude" || rec.HarnessSession != "claude-session-1" || rec.HarnessCwd != sub {
		t.Fatalf("record = harness %q session %q cwd %q, want claude, claude-session-1, %q", rec.Harness, rec.HarnessSession, rec.HarnessCwd, sub)
	}
	if rec.Root != loginRoot || rec.Root == rec.HarnessCwd {
		t.Fatalf("the record's root %q stays the canonical root %q, apart from the harness directory %q", rec.Root, loginRoot, rec.HarnessCwd)
	}

	// A re-login reports the conversation it now belongs to.
	if got, _ := parseLoginResponse(t, callLogin(t, server, 2, root, map[string]any{"relogin_session_key": key, "harness_session": "claude-session-2", "harness_cwd": root})); got != key {
		t.Fatalf("re-login returned %q, want %q", got, key)
	}
	if rec := keyRecordForTest(t, server, key); rec.HarnessSession != "claude-session-2" || rec.HarnessCwd != root {
		t.Fatalf("after the re-login: session %q cwd %q", rec.HarnessSession, rec.HarnessCwd)
	}

	// A report that cannot resume anything is ignored, and the login holds.
	if got, _ := parseLoginResponse(t, callLogin(t, server, 3, root, map[string]any{"relogin_session_key": key, "harness_session": "claude-session-3", "harness_cwd": "relative/dir"})); got != key {
		t.Fatalf("re-login with an unusable report returned %q, want %q", got, key)
	}
	if rec := keyRecordForTest(t, server, key); rec.HarnessSession != "claude-session-2" || rec.HarnessCwd != root {
		t.Fatalf("an unusable report changed the record: session %q cwd %q", rec.HarnessSession, rec.HarnessCwd)
	}

	// A login that reports nothing keeps what an earlier one recorded.
	plain, _ := parseLoginResponse(t, callLogin(t, server, 4, root, nil))
	if rec := keyRecordForTest(t, server, plain); rec.HarnessSession != "" || rec.HarnessCwd != "" || rec.Harness != "" {
		t.Fatalf("a login that reported no harness session recorded one: %+v", rec.sessionKeyRecord)
	}
}

func TestTerminalRegistrationRecordsTheHarnessProcess(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	t.Setenv(envHarnessPID, strconv.Itoa(terminalHarnessPID))
	s, key, _ := dashboardSessionServer(t, root, true)
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	row, ok := readRegistration(t, dir, key)
	if !ok {
		t.Fatal("the keyed call registered no row")
	}
	if row.HarnessPID != terminalHarnessPID || row.Hosted || row.HostPID != 0 {
		t.Fatalf("row = %+v, want an unhosted row naming harness process %d", row, terminalHarnessPID)
	}
	// The process never makes the session live: the heartbeat alone does.
	row.HeartbeatAt = dashboardStamp(time.Now().Add(-2 * wsdashboard.LiveWindow))
	if row.Live(time.Now()) {
		t.Fatal("a row with a stale heartbeat reads live because it names a process")
	}
}

func TestReloginToASessionTheDashboardRunsIsRefused(t *testing.T) {
	cases := map[string]wsdashboard.Registration{
		"a live process":     {Hosted: true, HostPID: hostedHarnessPID},
		"a spawn under way":  {Hosted: true},
		"a row of old shape": {Hosted: true, HostPID: 0, HeartbeatAt: dashboardStamp(time.Now().Add(-time.Hour))},
	}
	for name, row := range cases {
		t.Run(name, func(t *testing.T) {
			dir := dashboardSessionEnv(t)
			root := dashboardGitRoot(t)
			_, key, loginRoot := dashboardSessionServer(t, root, true)
			stubHostedProcessAlive(t, hostedHarnessPID)
			row.SchemaVersion, row.ReplyID, row.Project, row.Harness = 1, wsmailbox.ReplyID(key), loginRoot, "claude"
			if err := wsdashboard.WriteRegistration(dir, row); err != nil {
				t.Fatal(err)
			}
			s := terminalServer(t, root)
			keys := sessionKeyFileCount(t, s)
			resp := relogin(t, s, root, key)
			if !toolIsError(t, resp) {
				t.Fatalf("the re-login was accepted: %s", resp)
			}
			text := toolText(t, resp)
			if !strings.Contains(text, HostedLoginRefusalMarker) || !strings.Contains(text, "Hand the session to the terminal") {
				t.Fatalf("the refusal names neither its marker nor the way out: %s", text)
			}
			if got := sessionKeyFileCount(t, s); got != keys {
				t.Fatalf("a refused re-login minted a key: %d records, want %d", got, keys)
			}
			if got, _ := readRegistration(t, dir, key); got != row {
				t.Fatalf("a refused re-login rewrote the row:\n got %+v\nwant %+v", got, row)
			}
			// Nothing of the refused server's writes the row afterwards.
			dashCall(t, s, "todo.list", map[string]any{"session_key": key})
			s.dashboardHeartbeat()
			if got, _ := readRegistration(t, dir, key); got != row {
				t.Fatalf("the refused server rewrote the row:\n got %+v\nwant %+v", got, row)
			}
		})
	}
}

func TestTheHostedSessionsOwnLoginIsAccepted(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	_, key, loginRoot := dashboardSessionServer(t, root, true)
	stubHostedProcessAlive(t, hostedHarnessPID)
	row := wsdashboard.Registration{SchemaVersion: 1, ReplyID: wsmailbox.ReplyID(key), Project: loginRoot, Harness: "claude", Hosted: true, HostPID: hostedHarnessPID}
	if err := wsdashboard.WriteRegistration(dir, row); err != nil {
		t.Fatal(err)
	}

	t.Run("by its hosted flag", func(t *testing.T) {
		hostedSessionEnv(t, key)
		s := NewServer(root, "test")
		s.sessionHarness = "claude"
		resp := relogin(t, s, root, key)
		if toolIsError(t, resp) {
			t.Fatalf("the hosted session's own re-login was refused: %s", resp)
		}
		if got, _ := parseLoginResponse(t, resp); got != key {
			t.Fatalf("re-login returned %q, want %q", got, key)
		}
		if got, _ := readRegistration(t, dir, key); got != row {
			t.Fatalf("the row changed:\n got %+v\nwant %+v", got, row)
		}
	})

	t.Run("by the process the row names", func(t *testing.T) {
		// No hosted flag: the harness is still the one process the row
		// names (a ws-mcp the harness started outside the hosted launch).
		t.Setenv(envHostedSession, "")
		t.Setenv(envHostedSessionKey, "")
		t.Setenv(envHarnessPID, strconv.Itoa(hostedHarnessPID))
		s := NewServer(root, "test")
		s.sessionHarness = "claude"
		resp := relogin(t, s, root, key)
		if toolIsError(t, resp) {
			t.Fatalf("the re-login of the process the row names was refused: %s", resp)
		}
		if got, _ := readRegistration(t, dir, key); got != row {
			t.Fatalf("the row changed:\n got %+v\nwant %+v", got, row)
		}
	})
}

func TestReloginToAHostedSessionWithNoProcessTakesItsRow(t *testing.T) {
	cases := map[string]wsdashboard.Registration{
		"stopped":             {Hosted: true, HostStopped: true},
		"closed":              {Hosted: true, EndedAt: dashboardStamp(time.Now().Add(-time.Hour))},
		"its process is gone": {Hosted: true, HostPID: hostedHarnessPID},
	}
	for name, row := range cases {
		t.Run(name, func(t *testing.T) {
			dir := dashboardSessionEnv(t)
			root := dashboardGitRoot(t)
			_, key, loginRoot := dashboardSessionServer(t, root, true)
			stubHostedProcessAlive(t) // nothing runs
			row.SchemaVersion, row.ReplyID, row.Project, row.Harness = 1, wsmailbox.ReplyID(key), loginRoot, "claude"
			row.RegisteredAt = dashboardStamp(time.Now().Add(-2 * time.Hour))
			row.HeartbeatAt = dashboardStamp(time.Now().Add(-time.Hour))
			if err := wsdashboard.WriteRegistration(dir, row); err != nil {
				t.Fatal(err)
			}
			// The runner's record stays in the session directory.
			record := filepath.Join(wsdashboard.SessionDir(dir, row.ReplyID), wsdashboard.AgentRecordFileName)
			if err := os.WriteFile(record, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}

			s := terminalServer(t, root)
			resp := relogin(t, s, root, key)
			if toolIsError(t, resp) {
				t.Fatalf("the re-login was refused: %s", resp)
			}
			if got, _ := parseLoginResponse(t, resp); got != key {
				t.Fatalf("re-login returned %q, want %q", got, key)
			}
			got, ok := readRegistration(t, dir, key)
			if !ok {
				t.Fatal("the row is gone")
			}
			if got.Hosted || got.HostPID != 0 || got.HostStopped || got.EndedAt != "" {
				t.Fatalf("the row is still a hosted one: %+v", got)
			}
			if got.HarnessPID != terminalHarnessPID || got.Harness != "pi" || got.RegisteredAt != row.RegisteredAt || got.Project != loginRoot {
				t.Fatalf("the row is not the terminal session's: %+v", got)
			}
			if !got.Live(time.Now()) {
				t.Fatalf("the row's heartbeat was not refreshed: %+v", got)
			}

			// The row stays the terminal's through its own later writes,
			// and stays in place when its dashboard key turns off: the
			// daemon reads it to know the session is no longer its own.
			dashCall(t, s, "todo.list", map[string]any{"session_key": key})
			s.dashboardHeartbeat()
			if got, _ := readRegistration(t, dir, key); got.Hosted || got.HarnessPID != terminalHarnessPID {
				t.Fatalf("after the terminal's own writes: %+v", got)
			}
			if resp := dashCall(t, s, "config.tune", map[string]any{"session_key": key, "key": DashboardEnabledKey, "value": "false", "scope": "session"}); toolIsError(t, resp) {
				t.Fatalf("config.tune: %s", resp)
			}
			dashCall(t, s, "todo.list", map[string]any{"session_key": key})
			s.dashboardHeartbeat()
			if got, ok := readRegistration(t, dir, key); !ok || got.Hosted {
				t.Fatalf("with the dashboard key off the handed-off row must stay an unhosted one: present %v, %+v", ok, got)
			}
		})
	}
}

func TestParseMailTextReadsBackWhatTheBatchFormatWrote(t *testing.T) {
	mail := []wsmailbox.Envelope{
		{From: "peer@repo", Content: "first: line\n\nmail from nobody (never):\nstill the body", SentAt: "2026-01-02T03:04:05Z"},
		{ReplyTo: wsmailbox.ReplyIDPrefix + "abc123", Content: "second", SentAt: "2026-01-02T03:04:06Z"},
		{Content: "third, from no one", SentAt: "2026-01-02T03:04:07Z"},
	}
	texts := formatMailBatch(leadMailLabel, leadMailRecvTool, "some-key", mail)
	want := []wsmailbox.Envelope{
		mail[0],
		{From: mail[1].ReplyTo, Content: "second", SentAt: mail[1].SentAt},
		mail[2],
	}
	for i, text := range texts {
		if got := parseMailText(text); got != want[i] {
			t.Fatalf("text %d read back as %+v, want %+v", i, got, want[i])
		}
	}
	// Read back and formatted again, a batch is the same texts.
	back := make([]wsmailbox.Envelope, 0, len(texts))
	for _, text := range texts {
		back = append(back, parseMailText(text))
	}
	again := formatMailBatch(leadMailLabel, leadMailRecvTool, "some-key", back)
	for i := range texts {
		if again[i] != texts[i] {
			t.Fatalf("text %d changed on the way back:\n got %q\nwant %q", i, again[i], texts[i])
		}
	}
	// A text of another shape goes back whole.
	if got := parseMailText("not a mail text"); got.Content != "not a mail text" || got.From != "" || got.SentAt == "" {
		t.Fatalf("a text of another shape read back as %+v", got)
	}
}

func TestLeadMailboxReturnsClaimedMailToTheInbox(t *testing.T) {
	setupMailboxTestEnv(t)
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	initGit(t, root)
	leadKey, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	leadReply := wsmailbox.ReplyID(leadKey)
	sender := NewServer(root, "test")
	senderKey := mailboxLogin(t, sender, 1, root)
	mail := NewLeadMailbox(stateDir)

	mustSend(t, sender, 2, senderKey, wsmailbox.ReplyIDPrefix+leadReply, "claimed first")
	mustSend(t, sender, 3, senderKey, wsmailbox.ReplyIDPrefix+leadReply, "claimed second")
	claimed, err := mail.Drain(root, leadKey)
	if err != nil || len(claimed) != 2 {
		t.Fatalf("Drain = %q, %v, want two texts", claimed, err)
	}
	// Mail that arrived after the claim stays behind what goes back.
	mustSend(t, sender, 4, senderKey, wsmailbox.ReplyIDPrefix+leadReply, "arrived later")
	if err := mail.Return(root, leadKey, claimed); err != nil {
		t.Fatalf("Return: %v", err)
	}

	// The session's own claim (a terminal harness's mailbox.recv) now
	// delivers all three, the returned ones first, from their sender.
	out := callToolWithKey(t, NewServer(root, "test"), 5, leadKey, "mailbox.recv", map[string]any{"format": "json"})
	first, second, later := strings.Index(out, "claimed first"), strings.Index(out, "claimed second"), strings.Index(out, "arrived later")
	if first < 0 || second < first || later < second {
		t.Fatalf("recv after the return, want the three messages in order: %s", out)
	}
	if handle := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(senderKey); strings.Count(out, handle) < 3 {
		t.Fatalf("each returned message keeps its sender %s: %s", handle, out)
	}
	if again, err := mail.Drain(root, leadKey); err != nil || len(again) != 0 {
		t.Fatalf("nothing is left after the session's claim, got %q, %v", again, err)
	}
	if err := mail.Return(root, leadKey, nil); err != nil {
		t.Fatalf("Return of nothing: %v", err)
	}
}
