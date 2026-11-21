package wsdashboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestSessionViewChildAndCardBoundMetadata(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{
		Observed: &wslive.ObservedModel{Current: wslive.ModelObservation{Model: "obs-model", Effort: "high"}},
	})

	// Hosted lead (non-child) is editable by default.
	leadSess := liveSession("lead1", "/p", now)
	leadSess.Registration.Hosted = true
	leadView := buildSessionView(pageInputs{Session: leadSess, Snapshot: snap, Now: now, Model: "opus-4.6", Effort: "medium"})
	if leadView.IsChild || !leadView.CanEditSettings || leadView.Model != "opus-4.6" || leadView.Effort != "medium" {
		t.Fatalf("leadView = %+v", leadView)
	}

	// Parented worker renders as child read-only with fallback observed model/effort.
	childSess := liveSession("child1", "/p", now)
	childSess.Registration.Agent = true
	childSess.Registration.ParentReplyID = "lead1"
	childView := buildSessionView(pageInputs{Session: childSess, Snapshot: snap, Now: now})
	if !childView.IsChild || childView.CanEditSettings || childView.Model != "obs-model" || childView.Effort != "high" {
		t.Fatalf("childView = %+v", childView)
	}

	// Card-bound hosted lead keeps ParentReplyID empty while rendering IsChild=true and CanEditSettings=false.
	cardSess := liveSession("cardlead1", "/p", now)
	cardSess.Registration.Hosted = true
	cardView := buildSessionView(pageInputs{Session: cardSess, Snapshot: snap, Now: now, IsChild: true, Model: "sonnet", Effort: "low"})
	if !cardView.IsChild || cardView.CanEditSettings || cardView.Model != "sonnet" || cardView.Effort != "low" {
		t.Fatalf("cardView = %+v", cardView)
	}
}

func TestChildCloseHeaderLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		known, closed, active bool
		phase, want           string
	}{
		{"unknown", false, false, false, "", ""},
		{"closed", true, true, false, "", ""},
		{"idle", true, false, false, "thinking", "idle"},
		{"active without snapshot", true, false, true, "", "thinking"},
		{"permission", true, false, true, "permission", "permission"},
		{"tool", true, false, true, "tool", "tool"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := pageInputs{Session: liveSession("child", "/p", testNow), Now: testNow, IsChild: true, ChildCloseKnown: tc.known, ChildClosed: tc.closed, ChildActive: tc.active}
			if tc.phase != "" {
				in.Snapshot = liveSnap(testNow, wslive.Lead{Activity: wslive.Activity{Phase: tc.phase}})
			}
			regions, err := renderSessionRegions(buildSessionView(in), nil)
			if err != nil {
				t.Fatal(err)
			}
			head := string(regions[regionHead].HTML)
			if tc.want == "" {
				if strings.Contains(head, "data-child-close") {
					t.Fatalf("unexpected close control: %s", head)
				}
			} else if !strings.Contains(head, `data-state="`+tc.want+`"`) {
				t.Fatalf("missing state %q: %s", tc.want, head)
			}
		})
	}
}

func TestChildCloseCapabilityIsLocal(t *testing.T) {
	for _, local := range []bool{false, true} {
		html, err := renderShellPage(shellPageData{Lang: "en", SysOpen: local, Src: "/?frame=1"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(html, `data-can-control="1"`); got != local {
			t.Fatalf("local=%v: data-can-control present=%v", local, got)
		}
	}
}
