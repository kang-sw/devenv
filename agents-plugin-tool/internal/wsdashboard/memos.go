package wsdashboard

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Memos are optional daemon-local state, one file per rail workspace (never
// per pooled worktree). Unknown fields survive read/modify/write.
const memoSchemaVersion = 1

type memo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type memoDoc struct {
	fields map[string]json.RawMessage
	items  []map[string]json.RawMessage
}

func memoFile(dir, workspace string) string {
	return filepath.Join(dir, "memos", fmt.Sprintf("%x.json", sha256.Sum256([]byte(workspace))))
}

func readMemoDoc(file string) (memoDoc, error) {
	doc := memoDoc{fields: map[string]json.RawMessage{}, items: []map[string]json.RawMessage{}}
	raw, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return doc, err
	}
	if err = json.Unmarshal(raw, &doc.fields); err != nil || doc.fields == nil {
		return doc, fmt.Errorf("invalid memo document")
	}
	var version int
	if err = json.Unmarshal(doc.fields["schema_version"], &version); err != nil {
		return doc, fmt.Errorf("invalid memo schema_version")
	}
	if raw := doc.fields["memos"]; len(raw) != 0 {
		err = json.Unmarshal(raw, &doc.items)
	}
	return doc, err
}

func (doc memoDoc) memos() []memo {
	out := []memo{}
	for _, item := range doc.items {
		var m memo
		_ = json.Unmarshal(item["id"], &m.ID)
		_ = json.Unmarshal(item["text"], &m.Text)
		if m.ID != "" && m.Text != "" {
			out = append(out, m)
		}
	}
	return out
}

func (doc memoDoc) save(file string) error {
	var version int
	_ = json.Unmarshal(doc.fields["schema_version"], &version)
	if version > memoSchemaVersion {
		return fmt.Errorf("memo schema_version %d is newer than supported %d", version, memoSchemaVersion)
	}
	doc.fields["schema_version"], _ = json.Marshal(memoSchemaVersion)
	doc.fields["memos"], _ = json.Marshal(doc.items)
	raw, err := json.MarshalIndent(doc.fields, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writePrivateFile(dir, filepath.Base(file), append(raw, '\n'))
}

// memoWorkspace accepts exactly the identities the rail renders, including
// plain folders. A caller cannot turn this route into an arbitrary file store.
func (d *Daemon) memoWorkspace(path string) bool {
	stored := d.workspaces.workspaces()
	for _, ws := range stored {
		if ws.Path == path {
			return true
		}
	}
	if d.index != nil {
		for _, s := range d.index.Sessions() {
			if s.Unlisted {
				continue
			}
			root := railRepoRoot(s.Registration)
			if root == "" {
				root = filepath.Clean(s.Registration.Project)
				for _, ws := range stored {
					if !ws.Repo && railPathUnder(ws.Path, root) {
						root = ws.Path
						break // the stored workspace was already admitted above
					}
				}
			}
			if root == path {
				return true
			}
		}
	}
	return false
}

func (d *Daemon) handleMemos(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Workspace string `json:"workspace"`
		Action    string `json:"action"`
		ID        string `json:"id"`
		Text      string `json:"text"`
	}
	if r.Method == http.MethodPost {
		if !localViewer(r) {
			http.NotFound(w, r)
			return
		}
		if !workspaceBody(w, r, &req) {
			return
		}
	} else {
		req.Workspace = r.URL.Query().Get("workspace")
	}
	if !d.memoWorkspace(req.Workspace) {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	d.memoMu.Lock()
	defer d.memoMu.Unlock()
	file := memoFile(d.dir, req.Workspace)
	doc, err := readMemoDoc(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if r.Method == http.MethodPost {
		req.Text = strings.TrimSpace(req.Text)
		if req.Action != "delete" && (req.Text == "" || len(req.Text) > 8000) {
			http.Error(w, "memo text must contain 1–8000 bytes", http.StatusBadRequest)
			return
		}
		idx := -1
		for i, item := range doc.items {
			var id string
			_ = json.Unmarshal(item["id"], &id)
			if id == req.ID {
				idx = i
				break
			}
		}
		switch req.Action {
		case "add":
			req.ID, err = newToken()
			if err != nil {
				http.Error(w, "cannot create memo", http.StatusInternalServerError)
				return
			}
			id, _ := json.Marshal(req.ID)
			text, _ := json.Marshal(req.Text)
			doc.items = append([]map[string]json.RawMessage{{"id": id, "text": text}}, doc.items...)
		case "edit":
			if idx < 0 {
				http.NotFound(w, r)
				return
			}
			doc.items[idx]["text"], _ = json.Marshal(req.Text)
		case "delete":
			if idx < 0 {
				http.NotFound(w, r)
				return
			}
			doc.items = append(doc.items[:idx], doc.items[idx+1:]...)
		default:
			http.Error(w, "unknown memo action", http.StatusBadRequest)
			return
		}
		if err := doc.save(file); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Memos    []memo `json:"memos"`
		Writable bool   `json:"writable"`
	}{doc.memos(), localViewer(r)})
}
