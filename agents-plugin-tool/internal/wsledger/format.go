package wsledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Commit format. The commit message is a JSON header; the tree holds event
// lines appended to the file dated by the header's ts.

type commitHeader struct {
	Stream string         `json:"stream"`
	Seen   map[string]int `json:"seen"`
	TS     time.Time      `json:"ts"`
}

type eventLine struct {
	Seq     int             `json:"seq"`
	TS      time.Time       `json:"ts"`
	Type    string          `json:"type"`
	Origin  string          `json:"origin,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

var dayPathRe = regexp.MustCompile(`^events/\d{4}/\d{2}/\d{2}\.jsonl$`)

// dayPath is the event file a commit written at ts appends to.
func dayPath(ts time.Time) string {
	u := ts.UTC()
	return fmt.Sprintf("events/%04d/%02d/%02d.jsonl", u.Year(), int(u.Month()), u.Day())
}

func encodeLine(e journalEvent) ([]byte, error) {
	payload := e.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return json.Marshal(eventLine{Seq: e.Seq, TS: e.TS.UTC(), Type: e.Type, Origin: e.Origin, Payload: payload})
}

func decodeLine(s string) lineRec {
	var l eventLine
	d := json.NewDecoder(strings.NewReader(s))
	if err := d.Decode(&l); err != nil {
		return lineRec{Bad: "malformed event line: " + err.Error()}
	}
	if d.More() {
		return lineRec{Bad: "malformed event line: trailing data"}
	}
	rec := lineRec{Seq: l.Seq, TS: l.TS, Type: l.Type, Origin: l.Origin, Payload: l.Payload}
	switch {
	case l.Seq < 1:
		rec.Bad = "malformed event line: seq must be at least 1"
	case l.Type == "":
		rec.Bad = "malformed event line: type required"
	case l.TS.IsZero():
		rec.Bad = "malformed event line: ts required"
	case len(l.Payload) == 0 || !json.Valid(l.Payload):
		rec.Bad = "malformed event line: payload required"
	}
	return rec
}

// refState is the parsed history of one member ref, kept between refreshes
// so a moved ref parses only its new commits.
type refState struct {
	owner   string
	tip     string
	commits []*commitRec
	lines   map[string]int // event file -> line count at tip
	// poisoned is set once a commit is not a pure append: later commits'
	// diffs no longer prove what each one added.
	poisoned string
}

func (st *refState) absorb(raw rawCommit) {
	if st.lines == nil {
		st.lines = map[string]int{}
	}
	c := parseCommit(raw)
	switch {
	case st.poisoned != "":
		c.Bad = "follows a malformed commit (" + st.poisoned + ")"
	case raw.PatchBad != "":
		c.Bad = raw.PatchBad
		st.poisoned = raw.PatchBad
	default:
		prev, known := st.lines[raw.Path]
		if raw.NewFile == known || raw.OldStart != prev {
			c.Bad = "event file does not continue from the parent commit"
			st.poisoned = c.Bad
		} else {
			st.lines[raw.Path] = prev + len(raw.Added)
			if c.Bad == "" && raw.Path != dayPath(c.TS) {
				c.Bad = fmt.Sprintf("event file %s does not match the commit's date", raw.Path)
			}
		}
	}
	st.commits = append(st.commits, c)
}

// parseCommit reads the header and event lines of one raw commit. Ref-level
// rules (pure append, continuity) are the caller's.
func parseCommit(raw rawCommit) *commitRec {
	c := &commitRec{OID: raw.OID, Root: len(raw.Parents) == 0}
	var h commitHeader
	if err := json.Unmarshal(bytes.TrimSpace(raw.Message), &h); err != nil {
		c.Bad = "malformed commit header: " + err.Error()
	} else if h.Stream == "" || h.TS.IsZero() {
		c.Bad = "malformed commit header: stream and ts required"
	}
	c.Stream, c.Seen, c.TS = h.Stream, h.Seen, h.TS
	if c.Bad == "" && len(raw.Parents) > 1 {
		c.Bad = "merge commit in a ledger ref"
	}
	if c.Bad == "" && raw.PatchBad == "" && !dayPathRe.MatchString(raw.Path) {
		c.Bad = fmt.Sprintf("commit writes %s, not an event file", raw.Path)
	}
	for _, a := range raw.Added {
		c.Lines = append(c.Lines, decodeLine(a))
	}
	return c
}

// genesisState is the parsed genesis ref.
type genesisState struct {
	oid   string
	raw   json.RawMessage
	roles Roles
}

// ErrBadGenesis wraps every reason the genesis ref is unusable.
var ErrBadGenesis = errors.New("ledger: invalid genesis")

func loadGenesis(ctx context.Context, g *gitRepo, oid string) (*genesisState, error) {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrBadGenesis, fmt.Sprintf(format, a...))
	}
	out, err := g.run(ctx, "rev-list", "--parents", "--max-count=2", oid)
	if err != nil {
		return nil, err
	}
	var heads [][]string
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) > 0 && oidRe.MatchString(f[0]) {
			heads = append(heads, f)
		}
	}
	if len(heads) != 1 || len(heads[0]) != 1 {
		return nil, bad("the genesis ref must hold exactly one root commit")
	}
	raw, ok, err := g.readBlob(ctx, oid+":"+rolesFile)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, bad("no %s", rolesFile)
	}
	var roles Roles
	if err := json.Unmarshal(raw, &roles); err != nil {
		return nil, bad("%s: %v", rolesFile, err)
	}
	if err := validateRoles(roles); err != nil {
		return nil, bad("%s: %v", rolesFile, err)
	}
	sigs, err := g.verifySigs(ctx, []string{oid}, allowedSigners(roles, nil))
	if err != nil {
		return nil, err
	}
	sig := sigs[oid]
	if sig.state != sigGood {
		return nil, bad("the genesis commit is not signed")
	}
	st := initialIdentity(roles)
	signed := false
	for a := range st.admins {
		signed = signed || st.members[a].holds(sig.fp)
	}
	if !signed {
		return nil, bad("the genesis commit is not signed by an admin key it lists")
	}
	return &genesisState{oid: oid, raw: json.RawMessage(raw), roles: roles}, nil
}

func validateRoles(r Roles) error {
	ids := map[string]bool{}
	for _, m := range r.Members {
		if !ValidID(m.ID) {
			return fmt.Errorf("member id %q is not valid", m.ID)
		}
		if ids[m.ID] {
			return fmt.Errorf("member %q listed twice", m.ID)
		}
		ids[m.ID] = true
		if len(m.Keys) == 0 {
			return fmt.Errorf("member %q has no keys", m.ID)
		}
		for _, k := range m.Keys {
			if _, _, err := parseKey(k); err != nil {
				return fmt.Errorf("member %q: %v", m.ID, err)
			}
		}
	}
	if len(r.Admins) == 0 {
		return errors.New("no admins")
	}
	for _, a := range r.Admins {
		if !ids[a] {
			return fmt.Errorf("admin %q is not a member", a)
		}
	}
	return nil
}

// allowedSigners renders an allowed-signers file from the genesis keys and
// every key an identity event declares. The principal is the member id; the
// verdict itself uses the signer fingerprint git reports.
func allowedSigners(roles Roles, refs []refCommits, claims ...map[string]*commitRec) []byte {
	var buf bytes.Buffer
	seen := map[string]bool{}
	add := func(member, key string) {
		norm, _, err := parseKey(key)
		if err != nil || !ValidID(member) || seen[member+" "+norm] {
			return
		}
		seen[member+" "+norm] = true
		buf.WriteString(member + " " + norm + "\n")
	}
	for _, m := range roles.Members {
		for _, k := range m.Keys {
			add(m.ID, k)
		}
	}
	scan := func(c *commitRec) {
		for _, l := range c.Lines {
			if l.Bad != "" || (l.Type != TypeMemberJoin && l.Type != TypeAddKey) {
				continue
			}
			var p KeyPayload
			if json.Unmarshal(l.Payload, &p) == nil {
				add(p.Member, p.Key)
			}
		}
	}
	for _, r := range refs {
		for _, c := range r.Commits {
			scan(c)
		}
	}
	for _, cl := range claims {
		for _, c := range cl {
			scan(c)
		}
	}
	return buf.Bytes()
}
