package wslive

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This is a real historical package build, not a newly simplified 'old'
// struct standing in for an older writer. The fixed revision predates the
// optional fields; its ordinary immutable writer must leave prior bytes alone.
const summaryHistoricalRevision = "9854ff350"

func TestSummaryHistoricalReaderWriterPreservesOriginalBytes(t *testing.T) {
	dir := t.TempDir()
	store := &SummaryCommits{closed: map[string]bool{}, write: writeFrozenRing}
	if err := store.Register(summaryTurnFixture()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Retry(RingDir(dir)); err != nil {
		t.Fatal(err)
	}
	names, _ := RingNames(RingDir(dir))
	original := filepath.Join(RingDir(dir), names[0])
	before, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	top, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	archiveCmd := exec.CommandContext(ctx, "git", "archive", summaryHistoricalRevision, "agents-plugin-tool")
	archiveCmd.Dir = strings.TrimSpace(string(top))
	archive, err := archiveCmd.Output()
	if err != nil {
		t.Fatalf("load pinned historical source: %v", err)
	}
	root := t.TempDir()
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		clean := filepath.Clean(h.Name)
		if !strings.HasPrefix(filepath.ToSlash(clean), "agents-plugin-tool/") {
			continue
		}
		path := filepath.Join(root, clean)
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0o755)
		case tar.TypeReg:
			err = os.MkdirAll(filepath.Dir(path), 0o755)
			if err == nil {
				var raw []byte
				raw, err = io.ReadAll(tr)
				if err == nil {
					err = os.WriteFile(path, raw, 0o600)
				}
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := `package wslive
import("os";"testing")
func TestSummaryHistoricalWriterFixture(t *testing.T){
 dir:=os.Getenv("WS_SUMMARY_FIXTURE_DIR")
 rows,err:=ReadRing(RingDir(dir));if err!=nil||len(rows)!=1||rows[0].Kind!=RingTurn||rows[0].Done!=2||rows[0].Failed!=1||rows[0].DurationMs!=73||rows[0].Reason!="error"{t.Fatalf("old reader: %+v %v",rows,err)}
 if err:=AppendRing(RingDir(dir),[]RingEntry{{Kind:RingReply,Text:"historical writer append"},{Kind:RingTurn,Done:1,DurationMs:11}});err!=nil{t.Fatal(err)}
}
`
	module := filepath.Join(root, "agents-plugin-tool")
	if err := os.WriteFile(filepath.Join(module, "internal", "wslive", "summary_historical_fixture_test.go"), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "./internal/wslive", "-run", "^TestSummaryHistoricalWriterFixture$", "-count=1", "-vet=off", "-p=1", "-parallel=1", "-timeout=90s")
	cmd.Dir = module
	cmd.Env = summaryHistoricalEnv(dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("historical writer fixture: %v\n%s", err, out)
	} else {
		t.Logf("historical writer fixture output:\n%s", out)
	}
	// Exercise the actual approved pre-feature daemon mapper with the new
	// bundle's additive widget shape, not a newly invented tolerant struct.
	mapperFixture := `package wsagent
import("testing";"github.com/kang-sw/devenv/internal/wslive")
func TestSummaryOldDaemonNewBundleWidget(t *testing.T) {
 for _,nativeAbort:=range []bool{false,true} { for _,stop:=range []string{"aborted","error","length"} {
  name:=stop; if nativeAbort { name+="-native-abort" }; t.Run(name,func(t *testing.T){
  h:=newLeadHarness(t).start(); id:=h.lead(BackendPi,LeadOptions{}).ID; h.send(id,"new bundle on old daemon"); holder:=h.holder(id)
  hello:=string(piWidgetLine(piEventsWidgetKey,"{\"kind\":\"status\",\"background\":0,\"monitors\":0,\"capabilities\":[\"reasoning_summary_v1\"]}"))
  summary:=string(piWidgetLine(piEventsWidgetKey,__SUMMARY_WIDGET_JSON__))
  settled:="{\"type\":\"agent_settled\",\"aborted\":false}"; if nativeAbort { settled="{\"type\":\"agent_settled\",\"aborted\":true}" }
  h.holders.out(holder,"{\"type\":\"agent_start\"}",hello,summary,"{\"type\":\"message_end\",\"message\":{\"role\":\"assistant\",\"content\":[],\"stopReason\":\""+stop+"\",\"errorMessage\":\"actual failure\"}}",settled)
  h.idle(id); h.quiesce(id); turns:=0
  for _,row:=range h.ring(id) { if row.Kind==wslive.RingTurn { turns++; if row.Reason!="" { t.Fatalf("old daemon reason changed: %+v",row) } } }
  wantError:=""; if stop=="error" { wantError="actual failure" }
  if turns!=1 || h.state(id).BackgroundTasks!=0 || h.state(id).Error!=wantError { t.Fatalf("old daemon lifecycle/counts/diagnostic changed: turns=%d state=%+v",turns,h.state(id)) }
 }) } }
}
`
	projection, err := json.Marshal(map[string]any{"kind": KindAgent, "reasoningSummary": summaryFixture()})
	if err != nil {
		t.Fatal(err)
	}
	mapperFixture = strings.ReplaceAll(mapperFixture, "__SUMMARY_WIDGET_JSON__", strconv.Quote(string(projection)))
	// This source pin is the exact integrated pre-feature develop gate, not a
	// newer moving branch. It contains the real hosted test harness.
	mapperRevision := "54b252036f430ddd0886af90ba8de767bc4b1a6a"
	mapperArchive := exec.CommandContext(ctx, "git", "archive", mapperRevision, "agents-plugin-tool")
	mapperArchive.Dir = strings.TrimSpace(string(top))
	mapperRaw, err := mapperArchive.Output()
	if err != nil {
		t.Fatalf("load pinned legacy mapper: %v", err)
	}
	mapperRoot := t.TempDir()
	// Build that complete historical module; do not mix mapper and dependency
	// revisions into a synthetic daemon.
	mapperReader := tar.NewReader(bytes.NewReader(mapperRaw))
	for {
		header, err := mapperReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		clean := filepath.Clean(header.Name)
		if !strings.HasPrefix(filepath.ToSlash(clean), "agents-plugin-tool/") {
			continue
		}
		path := filepath.Join(mapperRoot, clean)
		if header.Typeflag == tar.TypeDir {
			err = os.MkdirAll(path, 0o755)
		} else if header.Typeflag == tar.TypeReg {
			err = os.MkdirAll(filepath.Dir(path), 0o755)
			if err == nil {
				var data []byte
				data, err = io.ReadAll(mapperReader)
				if err == nil {
					err = os.WriteFile(path, data, 0o600)
				}
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	mapperModule := filepath.Join(mapperRoot, "agents-plugin-tool")
	if err := os.WriteFile(filepath.Join(mapperModule, "internal", "wsagent", "summary_historical_mapper_test.go"), []byte(mapperFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	mapperCommand := exec.CommandContext(ctx, "go", "test", "./internal/wsagent", "-run", "^TestSummaryOldDaemonNewBundleWidget$", "-count=1", "-vet=off", "-p=1", "-parallel=1", "-timeout=90s", "-v")
	mapperCommand.Dir, mapperCommand.Env = mapperModule, summaryHistoricalEnv(dir)
	if out, err := mapperCommand.CombinedOutput(); err != nil {
		t.Fatalf("historical mapper fixture: %v\n%s", err, out)
	} else {
		t.Logf("historical mapper fixture output:\n%s", out)
	}
	after, err := os.ReadFile(original)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("old writer changed summary-bearing immutable bytes", err)
	}
	rows, err := ReadRing(RingDir(dir))
	if err != nil || len(rows) != 3 || rows[0].SummaryHistory == nil || rows[0].SummaryHistory.Requests[0].Attempt != 1 {
		t.Fatalf("new reader after old append: %+v %v", rows, err)
	}
	// Decision967 keeps live state out of both production disk writers; their
	// tests own that invariant. This real historical build proves only the
	// separate immutable-history boundary, never recovery of in-flight data.
	if !store.Has(rows[0].SummaryHistory.Commit) {
		t.Fatal("immutable summary commit was not retained")
	}
}

// Keep the historical child independent of developer credentials, host session
// keys, installed harness launchers and test-binary stand-in switches.
func summaryHistoricalEnv(dir string) []string {
	var env []string
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL", "GOROOT", "GOCACHE", "GOMODCACHE", "GOOS", "GOARCH", "GOMAXPROCS", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_EXEC_PATH", "GIT_TEMPLATE_DIR", "GIT_NO_REPLACE_OBJECTS", "GIT_ATTR_NOSYSTEM", "GIT_TERMINAL_PROMPT", "GIT_ASKPASS", "GIT_SSH_COMMAND", "SSH_ASKPASS"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return append(env, "WS_SUMMARY_FIXTURE_DIR="+dir, "GOTOOLCHAIN=local", "GOENV=off", "GOWORK=off", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off", "GOVCS=*:off", "GOTELEMETRY=off", "GOFLAGS=-mod=readonly -buildvcs=false")
}

func TestSummaryPagingPruningAndNoResurrection(t *testing.T) {
	dir := t.TempDir()
	store := &SummaryCommits{closed: map[string]bool{}, write: writeFrozenRing}
	entry := summaryTurnFixture()
	if err := store.Register(entry); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Retry(dir); err != nil {
		t.Fatal(err)
	}
	entries := make([]RingEntry, RingHistoryCap)
	for i := range entries {
		entries[i] = RingEntry{Kind: RingTurn, DurationMs: int64(i + 1)}
	}
	if err := AppendRing(dir, entries); err != nil {
		t.Fatal(err)
	}
	names, err := RingNames(dir)
	if err != nil || len(names) != RingHistoryCap {
		t.Fatalf("disk cap: %d %v", len(names), err)
	}
	page, err := ReadRingPage(dir, "", RingCap)
	if err != nil || len(page.Records) != 50 || page.End {
		t.Fatalf("live paging: %+v %v", page, err)
	}
	older, err := ReadRingPage(dir, page.Before, RingCap)
	if err != nil || len(older.Records) != 50 || older.Records[49].Name >= page.Before {
		t.Fatalf("older paging: %+v %v", older, err)
	}
	if err := store.Register(entry); err != nil {
		t.Fatal(err)
	}
	if n, err := store.Retry(dir); n != 0 || err != nil {
		t.Fatalf("pruned record resurrected: %d %v", n, err)
	}
}
