package wsdashboard

import (
	"net/http"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

type agentLiveProbe struct {
	host         LiveHost
	dir          string
	slot         HostedSlot
	boundAtStart bool
}

func (p *agentLiveProbe) Handler() http.Handler     { return http.NotFoundHandler() }
func (p *agentLiveProbe) InboxChanged(string)       {}
func (p *agentLiveProbe) SetLiveHost(host LiveHost) { p.host = host }
func (p *agentLiveProbe) Start(func(string, string)) {
	p.boundAtStart = p.host != nil
	if p.host != nil {
		p.slot, _ = p.host.OpenHostedSlot(p.dir)
	}
}
func (p *agentLiveProbe) Close() {
	if p.slot != nil {
		p.slot.Close()
	}
}

func TestAgentLiveHostIsBoundBeforeRunnerStarts(t *testing.T) {
	probe := &agentLiveProbe{}
	f := startPageFixture(t, func(o *DaemonOptions) {
		probe.dir = SessionDir(o.Dir, "livesession")
		o.Agents = probe
	})
	if !probe.boundAtStart || probe.slot == nil {
		t.Fatal("daemon did not bind its RAM projection before agent recovery")
	}
	summary := rendererSummary()
	probe.slot.Publish(wslive.Snapshot{SchemaVersion: wslive.SchemaVersion, Lead: wslive.Lead{ReasoningSummary: summary}})
	if snap := f.d.snapshots.Snapshot(f.dir, "livesession"); snap == nil || snap.Lead.ReasoningSummary == nil {
		t.Fatal("native projection did not reach the actual daemon snapshot cache")
	}
	if snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(probe.dir)); err != nil || snap.Lead.ReasoningSummary != nil {
		t.Fatal("RAM publication wrote optional live data to disk", err)
	}
	probe.slot.Close()
	if snap := f.d.snapshots.Snapshot(f.dir, "livesession"); snap == nil || snap.Lead.ReasoningSummary != nil {
		t.Fatal("slot close did not fall back to the summary-free disk projection")
	}
}
