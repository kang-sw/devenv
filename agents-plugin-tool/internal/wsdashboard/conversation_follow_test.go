package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

func oneLineDashboardFunc(t *testing.T, src, name string) string {
	t.Helper()
	block := regexp.MustCompile(`(?m)^  function ` + name + `\(\) \{[^\n]*\}\n`).FindString(src)
	if block == "" {
		t.Fatalf("dashboard.js holds no one-line function %s", name)
	}
	return block
}

// TestConversationSendRearmsBottomFollow pins the send-owned conversation
// follow: a valid send from an upward-scrolled conversation moves to the
// bottom, the next live-region update stays there, and a later user scroll
// cancels follow. Empty and invalid sends do not arm it. The button and Enter
// paths are also pinned to the shared send function.
func TestConversationSendRearmsBottomFollow(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	if !strings.Contains(src, `if (csend) csend.addEventListener("click", function () { sendMessage(); ctext.focus(); });`) {
		t.Fatal("the send button no longer routes through sendMessage")
	}
	if !strings.Contains(src, `if (inputMode === "enter" ? !e.shiftKey && !e.altKey : mod) { e.preventDefault(); sendMessage(); }`) {
		t.Fatal("Enter no longer routes through sendMessage")
	}
	fns := strings.Join([]string{
		oneLineDashboardFunc(t, src, "atBottom"),
		oneLineDashboardFunc(t, src, "pinned"),
		oneLineDashboardFunc(t, src, "pinConvo"),
		oneLineDashboardFunc(t, src, "followConvo"),
		composerFunc(t, src, "sendCommand"),
		composerFunc(t, src, "sendMessage"),
	}, "\n")
	runNode(t, "conversation send follow", selectionCodecSource(t)+composerToastPrelude+`var sending = false;
function noteWarn(t) { toasts.push([t, "warn"]); }
var convo = { scrollTop: 700, clientHeight: 100, scrollHeight: 1000 };
function visible(el) { return el === convo; }
`+fns+`
function clearCommand() { cmdSel = null; }
function incomingReply(height) { var pin = pinned(); convo.scrollHeight = height; if (pin) pinConvo(); }
function userScroll(top) { convo.scrollTop = top; stick = atBottom(); }
function state() { return [convo.scrollTop, stick]; }

// A button send starts from above the bottom and follows its reply.
ctext.value = "button"; convo.scrollTop = 700; stick = false; answer = [true, {}];
sendMessage();
check("button send reaches bottom", state(), [1000, true]);
incomingReply(1400);
check("button send follows incoming reply", state(), [1400, true]);

// Enter uses the same send path after the user has cancelled follow again.
userScroll(400); check("user scroll cancels follow", state(), [400, false]);
ctext.value = "enter"; convo.scrollHeight = 1400; sendMessage();
check("Enter send re-arms after upward scroll", state(), [1400, true]);
incomingReply(1800);
check("Enter send follows incoming reply", state(), [1800, true]);
userScroll(500); incomingReply(2200);
check("later user scroll still cancels follow", state(), [500, false]);

// Guards do not re-arm follow.
convo.scrollHeight = 2200; convo.scrollTop = 500; stick = false; ctext.value = ""; sendMessage();
check("empty send does not arm follow", state(), [500, false]);
ctext.value = "invalid"; imgs = [{ ref: "", data: null }]; sendMessage();
check("invalid image send does not arm follow", state(), [500, false]);
imgs = [];

// A valid slash-command send follows too; its image guard does not.
cmdSel = { name: "compact", args: null }; ctext.value = "args"; convo.scrollHeight = 2400; sendCommand();
check("command send reaches bottom", state(), [2400, true]);
userScroll(600); cmdSel = { name: "compact", args: null }; imgs = [{ ref: "", data: null }]; sendCommand();
check("command image guard does not re-arm", state(), [600, false]);
process.stdout.write(bad.join("\n"));
`)
}
