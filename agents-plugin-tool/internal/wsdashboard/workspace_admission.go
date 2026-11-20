package wsdashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

const workspaceAdmissionSchemaVersion = 1

var errWorkspaceAdmissionNewer = errors.New("workspace admission file has a newer version")

// WorkspaceAdmissionFilePath returns the workspace admission store path beside
// workspacesFile (<dir>/workspace-admission.json), or empty when workspacesFile
// is empty (store disabled).
func WorkspaceAdmissionFilePath(workspacesFile string) string {
	if workspacesFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workspacesFile), "workspace-admission.json")
}

type workspaceAdmissionDoc struct {
	Version  int                        `json:"version"`
	Rejected []workspaceAdmissionRecord `json:"rejected"`
	extra    map[string]json.RawMessage
}

type workspaceAdmissionRecord struct {
	Path  string `json:"path"`
	extra map[string]json.RawMessage
}

func (d *workspaceAdmissionDoc) UnmarshalJSON(raw []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if m == nil {
		return errors.New("workspace admission must be a JSON object")
	}
	*d = workspaceAdmissionDoc{}
	if err := takeJSON(m, "version", &d.Version); err != nil {
		return err
	}
	if err := takeJSON(m, "rejected", &d.Rejected); err != nil {
		return err
	}
	if len(m) > 0 {
		d.extra = m
	}
	return nil
}

func (d workspaceAdmissionDoc) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range d.extra {
		m[k] = v
	}
	m["version"] = workspaceAdmissionSchemaVersion
	rej := d.Rejected
	if rej == nil {
		rej = []workspaceAdmissionRecord{}
	}
	m["rejected"] = rej
	return json.Marshal(m)
}

func (r *workspaceAdmissionRecord) UnmarshalJSON(raw []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if m == nil {
		return errors.New("workspace admission record must be a JSON object")
	}
	*r = workspaceAdmissionRecord{}
	if err := takeJSON(m, "path", &r.Path); err != nil {
		return err
	}
	if len(m) > 0 {
		r.extra = m
	}
	return nil
}

func (r workspaceAdmissionRecord) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range r.extra {
		m[k] = v
	}
	m["path"] = r.Path
	return json.Marshal(m)
}

func admissionPathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

func (d *workspaceAdmissionDoc) normalize() {
	out := make([]workspaceAdmissionRecord, 0, len(d.Rejected))
	seen := make(map[string]bool, len(d.Rejected))
	for _, r := range d.Rejected {
		if r.Path == "" || !filepath.IsAbs(r.Path) {
			continue
		}
		r.Path = filepath.Clean(r.Path)
		k := admissionPathKey(r.Path)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	d.Rejected = out
}

func (d *workspaceAdmissionDoc) clone() *workspaceAdmissionDoc {
	c := &workspaceAdmissionDoc{
		Version:  d.Version,
		Rejected: make([]workspaceAdmissionRecord, len(d.Rejected)),
		extra:    maps.Clone(d.extra),
	}
	for i, r := range d.Rejected {
		c.Rejected[i] = workspaceAdmissionRecord{Path: r.Path, extra: maps.Clone(r.extra)}
	}
	return c
}

func (d *workspaceAdmissionDoc) encode() ([]byte, error) {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func readWorkspaceAdmission(file string) (*workspaceAdmissionDoc, error) {
	raw, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return &workspaceAdmissionDoc{Version: workspaceAdmissionSchemaVersion, Rejected: []workspaceAdmissionRecord{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc workspaceAdmissionDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Version > workspaceAdmissionSchemaVersion {
		return nil, fmt.Errorf("%w (%d, this daemon writes %d)", errWorkspaceAdmissionNewer, doc.Version, workspaceAdmissionSchemaVersion)
	}
	doc.normalize()
	return &doc, nil
}

type workspaceAdmissionStore struct {
	file string

	mu          sync.Mutex
	doc         *workspaceAdmissionDoc
	seen        railFileStamp
	rejectedSet map[string]bool
	gen         uint64
}

func openWorkspaceAdmissionStore(file string) *workspaceAdmissionStore {
	if file == "" {
		return nil
	}
	s := &workspaceAdmissionStore{
		file:        file,
		doc:         &workspaceAdmissionDoc{Version: workspaceAdmissionSchemaVersion, Rejected: []workspaceAdmissionRecord{}},
		rejectedSet: map[string]bool{},
		gen:         uint64(time.Now().UnixMicro()),
	}
	s.mu.Lock()
	st := statRailFile(file)
	if doc, err := readWorkspaceAdmission(file); err == nil {
		s.setDocLocked(doc, st, false)
	}
	s.mu.Unlock()
	return s
}

func buildRejectedSet(doc *workspaceAdmissionDoc) map[string]bool {
	set := make(map[string]bool, len(doc.Rejected))
	for _, r := range doc.Rejected {
		set[admissionPathKey(r.Path)] = true
	}
	return set
}

func (s *workspaceAdmissionStore) setDocLocked(doc *workspaceAdmissionDoc, st railFileStamp, bumpGen bool) {
	s.doc = doc
	s.seen = st
	s.rejectedSet = buildRejectedSet(doc)
	if bumpGen {
		s.gen++
	}
}

func (s *workspaceAdmissionStore) rejected() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc == nil || len(s.doc.Rejected) == 0 {
		return nil
	}
	out := make([]string, len(s.doc.Rejected))
	for i, r := range s.doc.Rejected {
		out[i] = r.Path
	}
	return out
}

func (s *workspaceAdmissionStore) isRejected(path string) bool {
	if s == nil || path == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rejectedSet[admissionPathKey(path)]
}

func (s *workspaceAdmissionStore) generation() uint64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// preflight refuses malformed or newer on-disk state before a route writes
// workspaces.json. The no-op mutation also refreshes a foreign valid change.
func (s *workspaceAdmissionStore) preflight() error {
	_, err := s.mutate(func(*workspaceAdmissionDoc) error { return nil })
	return err
}

func (s *workspaceAdmissionStore) mutate(fn func(*workspaceAdmissionDoc) error) (uint64, error) {
	if s == nil {
		return 0, errors.New("workspace admission store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := statRailFile(s.file)
	doc := s.doc
	if st != s.seen {
		var err error
		doc, err = readWorkspaceAdmission(s.file)
		if err != nil {
			return s.gen, err
		}
	}
	before, err := doc.encode()
	if err != nil {
		return s.gen, err
	}
	next := doc.clone()
	if err := fn(next); err != nil {
		return s.gen, err
	}
	next.normalize()
	after, err := next.encode()
	if err != nil {
		return s.gen, err
	}
	if bytes.Equal(before, after) {
		if !maps.Equal(s.rejectedSet, buildRejectedSet(doc)) {
			s.setDocLocked(doc, st, true)
		} else if st != s.seen {
			s.setDocLocked(doc, st, false)
		}
		return s.gen, nil
	}
	dir := filepath.Dir(s.file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return s.gen, err
	}
	if err := writePrivateFile(dir, filepath.Base(s.file), after); err != nil {
		return s.gen, err
	}
	s.setDocLocked(next, statRailFile(s.file), true)
	return s.gen, nil
}

func (s *workspaceAdmissionStore) apply(rejectPaths, removePaths []string) (uint64, error) {
	if s == nil {
		return 0, nil
	}
	drop := make(map[string]bool, len(removePaths))
	for _, p := range removePaths {
		if p != "" {
			drop[admissionPathKey(p)] = true
		}
	}
	return s.mutate(func(doc *workspaceAdmissionDoc) error {
		if len(drop) > 0 {
			doc.Rejected = slices.DeleteFunc(doc.Rejected, func(r workspaceAdmissionRecord) bool {
				return drop[admissionPathKey(r.Path)]
			})
		}
		if len(rejectPaths) > 0 {
			have := buildRejectedSet(doc)
			for _, p := range rejectPaths {
				p = filepath.Clean(p)
				k := admissionPathKey(p)
				if p == "" || !filepath.IsAbs(p) || have[k] {
					continue
				}
				have[k] = true
				doc.Rejected = append(doc.Rejected, workspaceAdmissionRecord{Path: p})
			}
		}
		return nil
	})
}

func (s *workspaceAdmissionStore) reject(paths []string) (uint64, error) {
	if s == nil {
		return 0, errors.New("workspace admission store unavailable")
	}
	return s.apply(paths, nil)
}

func (s *workspaceAdmissionStore) undo(paths []string) (uint64, error) {
	if s == nil {
		return 0, errors.New("workspace admission store unavailable")
	}
	return s.apply(nil, paths)
}

func (s *workspaceAdmissionStore) clearRejected(paths []string) (uint64, error) {
	return s.apply(nil, paths)
}

// sweep reloads from disk if the file changed, prunes rejected entries whose
// directory no longer exists (fs.ErrNotExist only), writes only when entries
// were removed, and reports whether rejectedSet changed.
func (s *workspaceAdmissionStore) sweep() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	beforeSet := maps.Clone(s.rejectedSet)
	st := statRailFile(s.file)
	if st != s.seen {
		doc, err := readWorkspaceAdmission(s.file)
		if err != nil {
			return false
		}
		s.setDocLocked(doc, st, false)
	}
	if s.doc != nil && len(s.doc.Rejected) > 0 {
		kept := make([]workspaceAdmissionRecord, 0, len(s.doc.Rejected))
		for _, r := range s.doc.Rejected {
			if _, err := os.Lstat(r.Path); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			kept = append(kept, r)
		}
		if len(kept) != len(s.doc.Rejected) {
			next := s.doc.clone()
			next.Rejected = kept
			if after, err := next.encode(); err == nil {
				dir := filepath.Dir(s.file)
				if err := os.MkdirAll(dir, 0o700); err == nil {
					if err := writePrivateFile(dir, filepath.Base(s.file), after); err == nil {
						s.setDocLocked(next, statRailFile(s.file), false)
					}
				}
			}
		}
	}
	if !maps.Equal(beforeSet, s.rejectedSet) {
		s.gen++
		return true
	}
	return false
}
