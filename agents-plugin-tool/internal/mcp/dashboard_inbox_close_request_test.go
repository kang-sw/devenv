package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

func TestDashboardInboxDrainCloseRequestsInsideEnvelope(t *testing.T) {
	f := newLiveFixture(t, true)
	_, _, err := wsinbox.Send(f.inboxDir(f.replyA), wsinbox.Entry{CloseRequests: []wsinbox.CloseRequest{{Topic: "a", Reason: "done <now>"}, {Topic: "gone"}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	out, isError := directDrain(t, f.server, f.keyA, nil)
	if isError || !strings.Contains(out, `<wb:close-request topic="a">done &lt;now&gt;</wb:close-request>`+"\n"+`<wb:close-request topic="gone"></wb:close-request>`) || strings.Index(out, "<ws:user at=") > strings.Index(out, "<wb:close-request") || strings.Index(out, "</ws:user>") < strings.LastIndex(out, "</wb:close-request>") {
		t.Fatalf("drain: %s", out)
	}
}

func TestDashboardInboxCloseRequestPlaybookModes(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv(envNamespace, namespace)
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
			} else {
				t.Setenv(envNoAgent, "")
			}
			f := newWhiteboardFixture(t)
			body := toolText(t, f.call(t, "playbook.read", map[string]any{"name": "whiteboard"}))
			if strings.TrimSpace(body) == "" {
				t.Fatal("whiteboard playbook is empty")
			}
		})
	}
}
