package wsdashboard

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Admission is additive schema-1 data. Keeping it in extra also exercises the
// same raw-field preservation path used by older layout writers.
const railAdmissionField = "worktree_admission"

type railAdmissionRecord struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Origin     string `json:"origin"`
	State      string `json:"state"`
	Branch     string `json:"branch,omitempty"`
	extra      map[string]json.RawMessage
}

func (a *railAdmissionRecord) UnmarshalJSON(b []byte) error {
	type known railAdmissionRecord
	var k known
	if err := json.Unmarshal(b, &k); err != nil {
		return err
	}
	*a = railAdmissionRecord(k)
	if err := json.Unmarshal(b, &a.extra); err != nil {
		return err
	}
	for _, key := range []string{"repository", "path", "origin", "state", "branch"} {
		delete(a.extra, key)
	}
	return nil
}
func (a railAdmissionRecord) MarshalJSON() ([]byte, error) {
	type known railAdmissionRecord
	b, err := json.Marshal(known(a))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range a.extra {
		m[k] = v
	}
	return json.Marshal(m)
}
func railAdmissions(doc *railLayoutDoc) ([]railAdmissionRecord, error) {
	var records []railAdmissionRecord
	if doc != nil && doc.extra[railAdmissionField] != nil {
		if err := json.Unmarshal(doc.extra[railAdmissionField], &records); err != nil {
			return nil, err
		}
	}
	return records, nil
}
func admissionKey(root, path string) string { return root + "\x00" + path }
func admissionAt(records []railAdmissionRecord, root, path string) railAdmissionRecord {
	for _, a := range records {
		if samePath(a.Repository, root) && samePath(a.Path, path) {
			return a
		}
	}
	return railAdmissionRecord{Repository: root, Path: path, Origin: "detected", State: "pending"}
}

// mutateAdmissions rereads under the layout mutex, preserves unknown fields,
// and commits the entire batch in one private atomic replacement.
func (s *railLayoutStore) mutateAdmissions(change func([]railAdmissionRecord) ([]railAdmissionRecord, error)) (railLayoutAnswer, error) {
	if s == nil {
		return railLayoutAnswer{Reason: layoutStore}, errors.New("admission store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := readRailLayout(s.file)
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore}, err
	}
	records, err := railAdmissions(doc)
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore}, err
	}
	records, err = change(records)
	if err != nil {
		return railLayoutAnswer{Gen: s.gen}, err
	}
	next := doc.clone()
	next.extra = make(map[string]json.RawMessage, len(doc.extra)+1)
	for k, v := range doc.extra {
		next.extra[k] = v
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore}, err
	}
	if len(records) > 0 {
		next.extra[railAdmissionField] = raw
	} else {
		delete(next.extra, railAdmissionField)
	}
	before, err := doc.encode()
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore}, err
	}
	after, err := next.encode()
	if err != nil {
		return railLayoutAnswer{Reason: layoutStore}, err
	}
	if bytes.Equal(before, after) {
		return railLayoutAnswer{Gen: s.gen}, nil
	}
	if err = writePrivateFile(filepath.Dir(s.file), filepath.Base(s.file), after); err != nil {
		return railLayoutAnswer{Reason: layoutStore}, err
	}
	s.doc, s.seen = next, statRailFile(s.file)
	s.gen++
	return railLayoutAnswer{Changed: true, Gen: s.gen}, nil
}

func canonicalAdmissionPath(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(p), nil
}

type admissionIdentity struct {
	root, path, admin, common, back string
	infos                           []os.FileInfo
}

// Open then Stat captures identity from the handle, including on Windows.
// No working-tree status or per-worktree Git process is run here.
func admissionHandle(path string, dir bool) (os.FileInfo, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() != dir {
		return nil, nil, fmt.Errorf("unexpected file type: %s", path)
	}
	if dir {
		return info, nil, nil
	}
	if info.Size() > 64<<10 {
		return nil, nil, errors.New("oversized Git association")
	}
	b, err := io.ReadAll(io.LimitReader(f, 64<<10+1))
	if err != nil || len(b) > 64<<10 {
		return nil, nil, errors.New("unreadable Git association")
	}
	return info, b, nil
}
func probeAdmission(root, path string) (admissionIdentity, error) {
	var id admissionIdentity
	var err error
	if id.root, err = canonicalAdmissionPath(root); err != nil {
		return id, err
	}
	if id.path, err = canonicalAdmissionPath(path); err != nil {
		return id, err
	}
	info, _, err := admissionHandle(id.path, true)
	if err != nil {
		return id, err
	}
	id.infos = append(id.infos, info)
	ri, _, err := admissionHandle(id.root, true)
	if err != nil {
		return id, err
	}
	id.infos = append(id.infos, ri)
	common, err := canonicalAdmissionPath(filepath.Join(id.root, ".git"))
	if err != nil {
		return id, err
	}
	ciRoot, _, err := admissionHandle(common, true)
	if err != nil {
		return id, err
	}
	id.infos = append(id.infos, ciRoot)
	marker := filepath.Join(id.path, ".git")
	mi, raw, err := admissionHandle(marker, false)
	if err != nil {
		return id, err
	}
	id.infos = append(id.infos, mi)
	text := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(text, "gitdir: ") {
		return id, errors.New("unverifiable worktree Git link")
	}
	admin := strings.TrimSpace(strings.TrimPrefix(text, "gitdir: "))
	if !filepath.IsAbs(admin) {
		admin = filepath.Join(id.path, admin)
	}
	if id.admin, err = canonicalAdmissionPath(admin); err != nil {
		return id, err
	}
	ai, _, err := admissionHandle(id.admin, true)
	if err != nil {
		return id, err
	}
	id.infos = append(id.infos, ai)
	ci, craw, err := admissionHandle(filepath.Join(id.admin, "commondir"), false)
	if err != nil {
		return id, err
	}
	id.infos = append(id.infos, ci)
	cp := strings.TrimSpace(string(craw))
	if !filepath.IsAbs(cp) {
		cp = filepath.Join(id.admin, cp)
	}
	if id.common, err = canonicalAdmissionPath(cp); err != nil {
		return id, err
	}
	if !samePath(common, id.common) {
		return id, errors.New("worktree repository association changed")
	}
	bi, braw, err := admissionHandle(filepath.Join(id.admin, "gitdir"), false)
	if err != nil {
		return id, err
	}
	id.infos = append(id.infos, bi)
	bp := strings.TrimSpace(string(braw))
	if !filepath.IsAbs(bp) {
		bp = filepath.Join(id.admin, bp)
	}
	if id.back, err = canonicalAdmissionPath(bp); err != nil {
		return id, err
	}
	realMarker, err := canonicalAdmissionPath(marker)
	if err != nil || !samePath(realMarker, id.back) {
		return id, errors.New("worktree backlink changed")
	}
	return id, nil
}
func sameAdmissionIdentity(a, b admissionIdentity) bool {
	if a.root != b.root || a.path != b.path || a.admin != b.admin || a.common != b.common || a.back != b.back || len(a.infos) != len(b.infos) {
		return false
	}
	for i := range a.infos {
		if !os.SameFile(a.infos[i], b.infos[i]) {
			return false
		}
	}
	return true
}

type admissionObservation struct {
	identity admissionIdentity
	revision string
}
type railAdmissionTracker struct {
	mu           sync.Mutex
	observations map[string]admissionObservation
	store        *railLayoutStore
	epoch        string
	seq          uint64
}

func newRailAdmissionTracker(store *railLayoutStore) *railAdmissionTracker {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return &railAdmissionTracker{store: store, epoch: hex.EncodeToString(b[:]), observations: map[string]admissionObservation{}}
}

// observe runs only after successful existing viewer-gated list reads, after
// pool exclusion. Failed reads never prove absence or prune persisted records.
func (a *railAdmissionTracker) observe(root string, got RepoWorktrees) (map[string]admissionObservation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	revisions := map[string]admissionObservation{}
	canonicalRoot, err := canonicalAdmissionPath(root)
	if err != nil {
		return nil, err
	}
	// Stage identity changes until their corresponding durable invalidations
	// commit. A failed write must leave the old evidence for the next scan.
	next := make(map[string]admissionObservation, len(a.observations))
	for key, old := range a.observations {
		next[key] = old
	}
	nextSeq := a.seq
	remove := map[string]bool{}
	present := map[string]bool{}
	for _, w := range got.Worktrees {
		if samePath(root, w.Path) || railPooled(railRepo{Pools: got.Pools}, root, w.Path) {
			continue
		}
		present[filepath.Clean(w.Path)] = true
		canonicalPath, e := canonicalAdmissionPath(w.Path)
		if e != nil {
			canonicalPath = filepath.Clean(w.Path)
		}
		key := admissionKey(canonicalRoot, canonicalPath)
		old, had := next[key]
		id, err := probeAdmission(root, w.Path)
		if err != nil {
			old.revision = ""
			if had {
				next[key] = old
			}
			if _, e := os.Lstat(w.Path); errors.Is(e, fs.ErrNotExist) {
				remove[filepath.Clean(w.Path)] = true
				delete(next, key)
			}
			continue
		}
		if !had || old.revision == "" || !sameAdmissionIdentity(old.identity, id) {
			if had && !sameAdmissionIdentity(old.identity, id) {
				remove[id.path] = true
			}
			nextSeq++
			old = admissionObservation{identity: id, revision: fmt.Sprintf("%s-%d", a.epoch, nextSeq)}
		}
		next[key] = old
		revisions[filepath.Clean(w.Path)] = old
	}
	// Git may omit a removed worktree entirely. Retire its observation as well
	// as its admission once the directory is confirmed absent, so a later Add
	// cannot be compared with the previous incarnation's retained identity.
	for key, old := range next {
		if samePath(old.identity.root, canonicalRoot) && !present[old.identity.path] {
			if _, e := os.Lstat(old.identity.path); errors.Is(e, fs.ErrNotExist) {
				delete(next, key)
			}
		}
	}
	// Only a positively missing directory, not an unreadable association, proves
	// removal. Keep ordering retention separate from admission pruning.
	_, err = a.store.mutateAdmissions(func(records []railAdmissionRecord) ([]railAdmissionRecord, error) {
		out := make([]railAdmissionRecord, 0, len(records))
		for _, r := range records {
			gone := false
			if samePath(r.Repository, canonicalRoot) {
				gone = remove[r.Path]
				if !present[r.Path] {
					if _, e := os.Lstat(r.Path); errors.Is(e, fs.ErrNotExist) {
						gone = true
					}
				}
			}
			if !gone {
				out = append(out, r)
			}
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	a.observations, a.seq = next, nextSeq
	return revisions, nil
}

func classifyRailAdmissions(view *RailView, doc *railLayoutDoc) {
	records, _ := railAdmissions(doc)
	for i := range view.Workspaces {
		ws := &view.Workspaces[i]
		for j := range ws.Worktrees {
			wt := &ws.Worktrees[j]
			if wt.Root {
				wt.Origin = "user"
				wt.Admission = "admitted"
				continue
			}
			root, path := wt.AdmissionRepository, wt.AdmissionPath
			if root == "" {
				root = ws.Path
			}
			if path == "" {
				path = wt.Path
			}
			r := admissionAt(records, root, path)
			wt.Origin, wt.Admission = r.Origin, r.State
		}
	}
}
