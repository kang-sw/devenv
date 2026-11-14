package wswhiteboard

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This driver imports the archived package, not the current Entry/FolderStore.
const historicalDriver = `package main
import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "time"
 old "github.com/kang-sw/devenv/historyfixture/historical"
)
func main() {
 f:=old.FolderStore{Dir:os.Args[1]}
 action:=os.Args[2]
 var problems []string
 var err error
 switch action {
 case "open": _,err=f.Open(nil)
 case "legacy":
  data,e:=os.ReadFile(os.Args[3]); if e!=nil {panic(e)}
  var legacy old.State; if e=json.Unmarshal(data,&legacy);e!=nil {panic(e)}
  _,err=f.Open(&legacy)
 default:
  err=f.Update(nil,func(s *old.State) error {
   if action=="render" {
    _,e:=old.RenderPage(old.BuildView(s,old.ViewInput{Layout:old.LayoutFull}));if e!=nil{return e}
    s.BeginRender();return nil
   }
   _,problems=old.Apply(s,os.Args[3],old.Env{Now:time.Date(2026,11,15,1,5,0,0,time.UTC),IsAbs:filepath.IsAbs,IsDir:func(p string)bool{st,e:=os.Stat(p);return e==nil&&st.IsDir()}})
   if len(problems)>0{return fmt.Errorf("refused")};return nil
  })
 }
 result:=map[string]any{"problems":problems}
 if err!=nil {result["error"]=err.Error()}
 if e:=json.NewEncoder(os.Stdout).Encode(result);e!=nil {panic(e)}
}
`

func buildHistoricalWriter(t *testing.T) string {
	t.Helper()
	fixture := filepath.Join("testdata", "historical-schema2")
	var manifest struct {
		Commit     string `json:"commit"`
		ArchiveSHA string `json:"archive_sha256"`
		Files      []struct{ Path, SHA256 string }
	}
	if err := json.Unmarshal(folderRead(t, filepath.Join(fixture, "manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	archive := folderRead(t, filepath.Join(fixture, "source.tar.gz"))
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != manifest.ArchiveSHA {
		t.Fatal("historical archive hash changed")
	}
	want := map[string]string{}
	for _, file := range manifest.Files {
		want[file.Path] = file.SHA256
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	dir := t.TempDir()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg || !filepath.IsLocal(filepath.FromSlash(header.Name)) {
			t.Fatal("invalid historical fixture path")
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if want[header.Name] != hex.EncodeToString(sum[:]) {
			t.Fatalf("historical source changed: %s", header.Name)
		}
		delete(want, header.Name)
		path := filepath.Join(dir, "historical", filepath.FromSlash(header.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if len(want) != 0 {
		t.Fatal("missing historical source files")
	}
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod := fmt.Sprintf("module github.com/kang-sw/devenv/historyfixture\n\ngo 1.23\n\nrequire github.com/kang-sw/devenv v0.0.0\nreplace github.com/kang-sw/devenv => %s\n", strconvQuote(module))
	for path, data := range map[string][]byte{"go.mod": []byte(mod), "go.sum": folderRead(t, filepath.Join(module, "go.sum")), "main.go": []byte(historicalDriver)} {
		if err := os.WriteFile(filepath.Join(dir, path), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(dir, "historical-writer")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-p=1", "-mod=mod", "-o", binary, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
	start := time.Now()
	output, err := cmd.CombinedOutput()
	t.Logf("historical %s build: %s (timeout 60s)\n%s", manifest.Commit, time.Since(start), output)
	if err != nil {
		t.Fatalf("historical writer build: %v", err)
	}
	return binary
}

func strconvQuote(value string) string { data, _ := json.Marshal(value); return string(data) }

func historicalCall(t *testing.T, binary, dir, action, commands string, refused bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, dir, action, commands)
	start := time.Now()
	output, err := cmd.CombinedOutput()
	t.Logf("historical %s: %s (timeout 20s)\n%s", action, time.Since(start), output)
	if err != nil {
		t.Fatalf("historical %s: %v", action, err)
	}
	var result struct {
		Error    string
		Problems []string
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	if refused {
		if result.Error == "" || len(result.Problems) == 0 {
			t.Fatal("historical writer accepted new command")
		}
	} else if result.Error != "" {
		t.Fatal(result.Error)
	}
}

func TestTopicStateHistoricalSchema2Writer(t *testing.T) {
	binary := buildHistoricalWriter(t)
	t.Run("committed-prefix-and-lifecycle", func(t *testing.T) {
		dir := t.TempDir()
		f := FolderStore{Dir: dir}
		if err := f.Update(nil, func(s *State) error {
			stateApply(t, s, `<wb:topic name="Work" tickets="261114-feat-example"><wb:body>Original</wb:body><wb:log>Agent summary</wb:log><wb:state value="implementing" owner="executor" eta_minutes="40"/></wb:topic>`, stateNow)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		// Add noncanonical whitespace and an opaque future member to a real
		// committed line. This is fixture construction, not a caller API.
		path := filepath.Join(dir, "t1", "log.jsonl")
		log := folderRead(t, path)
		log = bytes.Replace(log, []byte(`"topic_state":{`), []byte(`"future_entry": { "opaque": [1, 2] }, "topic_state":{`), 1)
		if err := os.WriteFile(path, log, 0644); err != nil {
			t.Fatal(err)
		}
		var meta folderSession
		if _, err := folderDecode(filepath.Join(dir, "meta.json"), &meta); err != nil {
			t.Fatal(err)
		}
		meta.Topics[0].LogBytes = int64(len(log))
		if err := os.WriteFile(filepath.Join(dir, "meta.json"), folderJSON(meta), 0644); err != nil {
			t.Fatal(err)
		}
		start := folderOpen(t, dir, nil).TopicStates()["Work"].State
		steps := []struct {
			action, commands, key string
			active                bool
		}{
			{"update", `<wb:topic name="Work"><wb:body>Old content write</wb:body><wb:log>Old agent update</wb:log></wb:topic>`, "Work", true},
			{"update", `<wb:topic name="Work"><wb:add class="check"><wb:title>Old item</wb:title></wb:add></wb:topic>`, "Work", true},
			{"render", "", "Work", true},
			{"update", `<wb:topic name="work" label="Work"/>`, "work", true},
			{"update", `<wb:topic name="work"><wb:close result="Old close"/></wb:topic>`, "work", false},
			{"update", `<wb:topic name="work"><wb:log>Old reopen</wb:log></wb:topic>`, "work", false},
		}
		for _, step := range steps {
			historicalCall(t, binary, dir, step.action, step.commands, false)
			if !bytes.HasPrefix(folderRead(t, path), log) {
				t.Fatal("historical writer changed committed bytes")
			}
			got := folderOpen(t, dir, nil)
			view := got.TopicStates()[step.key]
			if view.Diagnostic != nil || (view.State != nil) != step.active {
				t.Fatalf("state after %s: %+v", step.commands, view)
			}
			if step.active && !reflect.DeepEqual(start, view.State) {
				t.Fatal("old write changed execution deadline")
			}
			if got.Topics[0].Entries[2].unknown["future_entry"] == nil {
				t.Fatal("new reader lost opaque Entry member")
			}
			if !reflect.DeepEqual(got.Topics[0].Tickets, []string{"261114-feat-example"}) {
				t.Fatal("old writer lost ticket semantics")
			}
		}
		before := folderTree(t, dir)
		historicalCall(t, binary, dir, "update", `<wb:topic name="work"><wb:log>must roll back</wb:log><wb:state value="waiting"/></wb:topic>`, true)
		if !reflect.DeepEqual(before, folderTree(t, dir)) {
			t.Fatal("new command on old writer mutated board")
		}
	})
	t.Run("opaque-pending-partial-recovery", func(t *testing.T) {
		dir := t.TempDir()
		f := FolderStore{Dir: dir}
		if err := f.Update(nil, func(s *State) error {
			stateApply(t, s, `<wb:topic name="Work"><wb:log>Legacy activity</wb:log></wb:topic>`, stateNow)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		prefix := folderRead(t, filepath.Join(dir, "t1", "log.jsonl"))
		f.Fault = func(stage string) error {
			if stage == "log-partial" {
				return folderCrash
			}
			return nil
		}
		if err := f.Update(nil, func(s *State) error {
			stateApply(t, s, stateCommand("implementing", `owner="executor" eta_minutes="40"`), stateNow)
			e := &s.Topics[0].Entries[len(s.Topics[0].Entries)-1]
			e.TopicState = bytes.Replace(e.TopicState, []byte(`"v":1`), []byte(`"future": { "bytes": [ 1, 2 ] }, "v":1`), 1)
			return nil
		}); err != folderCrash {
			t.Fatalf("expected partial append: %v", err)
		}
		var intent folderIntent
		if _, err := folderDecode(filepath.Join(dir, "pending.json"), &intent); err != nil {
			t.Fatal(err)
		}
		want := append(append([]byte(nil), prefix...), intent.Logs[0].Data...)
		historicalCall(t, binary, dir, "open", "", false)
		if !bytes.Equal(folderRead(t, filepath.Join(dir, "t1", "log.jsonl")), want) {
			t.Fatal("old recovery changed opaque append bytes")
		}
		got := folderOpen(t, dir, nil)
		if got.TopicStates()["Work"].State.Run.ETAMinutes != 40 {
			t.Fatal("recovered state missing")
		}
		before := folderTree(t, dir)
		historicalCall(t, binary, dir, "open", "", false)
		if !reflect.DeepEqual(before, folderTree(t, dir)) {
			t.Fatal("old recovery duplicated suffix")
		}
	})
	t.Run("typed-legacy-loss-is-separate", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "legacy.json")
		legacy := []byte(`{ "entry_seq":1,"topics":[{"name":"Work","body":"old","entries":[{"id":"e1","kind":"system","at":"2026-11-15T01:00:00Z","text":"opaque legacy","topic_state":{"v":1,"op":"set","value":"waiting"},"future_entry":{"bytes":true}}]}] }`)
		if err := os.WriteFile(path, legacy, 0644); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "board")
		historicalCall(t, binary, dir, "legacy", path, false)
		if !bytes.Equal(folderRead(t, path), legacy) {
			t.Fatal("legacy rollback snapshot rewritten")
		}
		got := folderOpen(t, dir, nil)
		if got.TopicStates()["Work"].State != nil || len(got.Topics[0].Entries[0].TopicState) != 0 {
			t.Fatal("claimed old typed decoder retained unknown legacy state")
		}
	})
}

func TestTopicStateCutoverBeforeMetadataAndCorruption(t *testing.T) {
	dir := t.TempDir()
	legacy := &State{Topics: []TopicDoc{{Name: "Work", Recap: "Historical summary", RecapAt: stateNow.Format(time.RFC3339Nano)}}}
	rollback := folderJSON(legacy)
	f := FolderStore{Dir: dir}
	if err := f.Update(legacy, func(s *State) error {
		if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
			t.Fatal("metadata emitted before folder cutover", err)
		}
		if strings.Contains(string(folderRead(t, filepath.Join(dir, "t1", "log.jsonl"))), "topic_state") {
			t.Fatal("backfilled legacy metadata")
		}
		if len(s.Topics[0].Entries) != 1 || !s.Topics[0].Entries[0].Imported {
			t.Fatal("conversion recap provenance")
		}
		stateApply(t, s, stateCommand("implementing", `owner="executor" eta_minutes="2"`), stateNow)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rollback, folderJSON(legacy)) {
		t.Fatal("new state dual-wrote legacy model")
	}
	path := filepath.Join(dir, "t1", "log.jsonl")
	data := folderRead(t, path)
	data[0] = '!'
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Open(nil); err == nil || !strings.Contains(err.Error(), "log:") {
		t.Fatalf("outer corruption masked as state diagnostic: %v", err)
	}
}
