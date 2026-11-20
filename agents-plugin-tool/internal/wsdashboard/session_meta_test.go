package wsdashboard

import (
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
