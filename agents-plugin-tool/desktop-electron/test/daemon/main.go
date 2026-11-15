// This source-built fixture runs the real dashboard daemon and assets on an
// OS-assigned port in an explicit, disposable cache root. It never runs Ensure.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

type source struct{}

func (source) ReadSession(wsdashboard.Registration) (wsdashboard.SessionData, bool, error) {
	return wsdashboard.SessionData{Fingerprint: "electron-smoke"}, true, nil
}
func (source) ReadPage(wsdashboard.Registration) (wsdashboard.PageContent, bool, error) {
	return wsdashboard.PageContent{}, true, nil
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	root := os.Getenv("WS_CACHE_HOME")
	if root == "" || !filepath.IsAbs(root) {
		panic("an explicit absolute isolated WS_CACHE_HOME is required")
	}
	dir := filepath.Join(root, "dashboard")
	project := filepath.Join(root, "project")
	must(os.MkdirAll(project, 0700))
	must(os.WriteFile(filepath.Join(project, "fixture.md"), []byte("# Electron fixture\n"), 0600))
	now := time.Now()
	must(wsdashboard.WriteRegistration(dir, wsdashboard.Registration{
		SchemaVersion: 1, ReplyID: "electronfixture", Project: project, Harness: "pi",
		RegisteredAt: now.Format(time.RFC3339), HeartbeatAt: now.Format(time.RFC3339),
	}))
	snap := wslive.Snapshot{SchemaVersion: 1, ReplyID: "electronfixture", Adapter: "pi",
		UpdatedAt: now.UnixMilli(), HeartbeatMs: 300000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}},
	}
	raw, err := json.Marshal(snap)
	must(err)
	must(os.WriteFile(wslive.SnapshotPath(wsdashboard.SessionDir(dir, "electronfixture")), raw, 0600))
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{
		Dir: dir, Port: 0, Version: "electron-fixture", Sessions: source{}, Log: os.Stderr,
	})
	must(err)
	d.Index().Refresh()
	fmt.Printf("isolated dashboard pid=%d port=%d\n", os.Getpid(), d.Endpoint().Port)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	d.Close()
}
