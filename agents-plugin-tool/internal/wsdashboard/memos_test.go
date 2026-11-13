package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoStoreOptionalRoundTripAndSchema(t *testing.T) {
	file := memoFile(t.TempDir(), "/workspace")
	doc, err := readMemoDoc(file)
	if err != nil || len(doc.memos()) != 0 {
		t.Fatalf("missing optional file: %+v, %v", doc, err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	original := `{"schema_version":1,"future":{"keep":true},"memos":[{"id":"a","text":"line 1\nline 2","color":"gold"}]}`
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err = readMemoDoc(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.memos(); len(got) != 1 || got[0].Text != "line 1\nline 2" {
		t.Fatalf("memos = %+v", got)
	}
	doc.items[0]["text"] = json.RawMessage(`"edited"`)
	if err := doc.save(file); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	for _, want := range []string{`"schema_version": 1`, `"future"`, `"color": "gold"`, `"text": "edited"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("saved document lacks %s: %s", want, raw)
		}
	}
	newer := strings.Replace(original, `"schema_version":1`, `"schema_version":2`, 1)
	if err := os.WriteFile(file, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err = readMemoDoc(file)
	if err != nil || len(doc.memos()) != 1 {
		t.Fatalf("newer schema tolerant read: %v", err)
	}
	if err := doc.save(file); err == nil {
		t.Fatal("newer schema accepted a write")
	}
	raw, _ = os.ReadFile(file)
	if string(raw) != newer {
		t.Fatal("newer schema was changed")
	}
	if err := os.WriteFile(file, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMemoDoc(file); err == nil {
		t.Fatal("malformed store accepted")
	}
}

func TestMemoRoutesAndRailBadge(t *testing.T) {
	f := startPageFixture(t)
	var data struct {
		Memos    []memo `json:"memos"`
		Writable bool   `json:"writable"`
	}
	body := func(action, id, text string) string {
		raw, _ := json.Marshal(map[string]string{"workspace": f.project, "action": action, "id": id, "text": text})
		return string(raw)
	}
	post := func(action, id, text string) {
		t.Helper()
		code, _, raw := postJSON(t, f.base+"/rail/memos", f.base, body(action, id, text))
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", action, code, raw)
		}
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			t.Fatal(err)
		}
	}
	for _, peer := range []string{"192.168.0.2:1234", "127.0.0.1:1234"} {
		r := httptest.NewRequest(http.MethodPost, "http://localhost/rail/memos", strings.NewReader(body("add", "", "blocked")))
		r.RemoteAddr = peer
		if strings.HasPrefix(peer, "127.") {
			r.Host = "192.168.0.10"
		}
		r.Header.Set("Origin", "http://"+r.Host)
		w := httptest.NewRecorder()
		f.d.handleMemos(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("non-local write = %d", w.Code)
		}
	}
	if code, _, _ := postJSON(t, f.base+"/rail/memos", "http://evil.example", body("add", "", "bad")); code != http.StatusForbidden {
		t.Fatalf("foreign Origin = %d", code)
	}
	post("add", "", "first")
	post("add", "", "second")
	if len(data.Memos) != 2 || data.Memos[0].Text != "second" || !data.Writable {
		t.Fatalf("add order: %+v", data)
	}
	id := data.Memos[0].ID
	post("edit", id, "edited\nhttps://example.com")
	if data.Memos[0].Text != "edited\nhttps://example.com" {
		t.Fatalf("edit: %+v", data)
	}
	view := f.d.railView(f.d.pagesIn("en"))
	if len(view.Workspaces) != 1 || view.Workspaces[0].MemoCount != 2 {
		t.Fatalf("rail count: %+v", view)
	}
	code, page := get(t, f.base+"/s/livesession")
	if code != http.StatusOK || !strings.Contains(page, `aria-label="Workspace memos">▤ <span>2</span>`) {
		t.Fatal("rail render lacks memo count badge")
	}
	post("delete", id, "")
	if len(data.Memos) != 1 || data.Memos[0].Text != "first" {
		t.Fatalf("delete: %+v", data)
	}
	// LAN reads are admitted, but never advertise editing controls.
	r := httptest.NewRequest(http.MethodGet, "http://localhost/rail/memos?workspace="+f.project, nil)
	r.RemoteAddr = "192.168.0.2:1234"
	w := httptest.NewRecorder()
	f.d.handleMemos(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"writable":false`) {
		t.Fatalf("LAN read: %d %s", w.Code, w.Body)
	}
	if code, _ := get(t, f.base+"/rail/memos?workspace=unknown"); code != http.StatusNotFound {
		t.Errorf("unknown workspace: %d", code)
	}
	if err := os.WriteFile(memoFile(f.dir, f.project), []byte(`broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := renderRailRegion(f.d.railView(f.d.pagesIn("en")), "en"); err != nil {
		t.Fatalf("optional broken store broke rail: %v", err)
	}
}

func TestMemoScript(t *testing.T) {
	harness, err := os.ReadFile("testdata/memo_test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "memo panel", string(harness))
}
