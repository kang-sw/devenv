package wsledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// DefaultPushDelay is the idle delay after the last write before own writes
// are pushed.
const DefaultPushDelay = 5 * time.Minute

// Clock is the time source of a Ledger; tests inject a fake one.
type Clock interface {
	Now() time.Time
	// AfterFunc runs f once d has passed on this clock.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is a pending AfterFunc.
type Timer interface{ Stop() bool }

type systemClock struct{}

func (systemClock) Now() time.Time                            { return time.Now() }
func (systemClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// Options configures one Ledger instance.
type Options struct {
	// Dir is the store directory: journal, clone id, lock, and, unless
	// RepoDir is set, the bare repository at Dir/repo.git.
	Dir string
	// RepoDir overrides the bare repository location. Several writers (each
	// with its own Dir) may share one repository; tests use this to exercise
	// multi-writer verification without a remote.
	RepoDir string
	// Member is this writer's member id.
	Member string
	// Name is the display name Create and Join write.
	Name string
	// KeyDir holds the personal SSH key (id_ed25519), generated with
	// ssh-keygen when absent. Empty means ~/.ws/tracker/keys.
	KeyDir string
	// CloneID fixes the clone id; empty means one generated on first open
	// and kept in Dir.
	CloneID string
	// Clock is the time source; nil means the system clock.
	Clock Clock
	// PushDelay is the idle push delay; zero means DefaultPushDelay.
	PushDelay time.Duration
	// Veto is the optional domain veto hook for identity events.
	Veto VetoFunc
	// Runner runs git; nil means wsgit.ExecRunner.
	Runner wsgit.Runner
}

var (
	// ErrNoStore: the repository has no genesis ref.
	ErrNoStore = errors.New("ledger: no store in this repository (no genesis ref)")
	// ErrStoreExists: Create found a genesis ref.
	ErrStoreExists = errors.New("ledger: the repository already holds a store")
	// ErrIDTaken: another join claimed the member id first.
	ErrIDTaken = errors.New("ledger: the member id is already claimed by another join")
)

// Ledger is one writer's open store.
type Ledger struct {
	mu   sync.Mutex
	opts Options
	git  *gitRepo
	lock *flock.Flock

	member, clone, stream string
	keyPath, pubKey, fp   string
	epoch                 string

	journal  *journal
	genesis  *genesisState
	refs     map[string]*refState  // member ref -> parsed history
	claims   map[string]*commitRec // member id -> claim commit
	sigCache map[string]sigInfo
	badRefs  []Warning

	view          *view
	pendingCommit *commitRec
	committedMax  int
	gen           uint64
	marks         map[EventID]mark
	received      int // events new to this view during the current call
}

type mark struct {
	verdict Verdict
	reason  string
	pending bool
	gen     uint64
}

var _ Store = (*Ledger)(nil)

func (o Options) normalize() (Options, error) {
	if o.Dir == "" {
		return o, errors.New("ledger: Options.Dir required")
	}
	if !ValidID(o.Member) {
		return o, fmt.Errorf("ledger: member id %q is not valid (lowercase letters, digits, '_' and '-')", o.Member)
	}
	if o.CloneID != "" && !ValidID(o.CloneID) {
		return o, fmt.Errorf("ledger: clone id %q is not valid", o.CloneID)
	}
	if o.RepoDir == "" {
		o.RepoDir = filepath.Join(o.Dir, "repo.git")
	}
	if o.KeyDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return o, err
		}
		o.KeyDir = filepath.Join(home, ".ws", "tracker", "keys")
	}
	if o.Clock == nil {
		o.Clock = systemClock{}
	}
	if o.PushDelay <= 0 {
		o.PushDelay = DefaultPushDelay
	}
	if o.Runner == nil {
		o.Runner = wsgit.ExecRunner{}
	}
	return o, nil
}

func newLedger(ctx context.Context, opts Options) (*Ledger, error) {
	o, err := opts.normalize()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return nil, err
	}
	if err := checkTools(ctx, o.Runner, o.Dir, exec.LookPath); err != nil {
		return nil, err
	}
	keyPath, pub, fp, err := ensureKey(ctx, o.KeyDir)
	if err != nil {
		return nil, err
	}
	return &Ledger{
		opts:     o,
		git:      &gitRepo{runner: o.Runner, dir: o.RepoDir, tmp: filepath.Join(o.Dir, "tmp")},
		lock:     flock.New(filepath.Join(o.Dir, "lock")),
		member:   o.Member,
		keyPath:  keyPath,
		pubKey:   pub,
		fp:       fp,
		epoch:    randomID(),
		journal:  newJournal(filepath.Join(o.Dir, "journal.jsonl")),
		refs:     map[string]*refState{},
		claims:   map[string]*commitRec{},
		sigCache: map[string]sigInfo{},
		marks:    map[EventID]mark{},
	}, nil
}

// Create makes a new store in an empty repository (one with no genesis
// ref): the genesis roles.json lists the creator as sole admin and a root,
// and the creator's member.join and claim ref are written with it.
func Create(ctx context.Context, opts Options) (*Ledger, error) {
	l, err := newLedger(ctx, opts)
	if err != nil {
		return nil, err
	}
	err = l.withLock(ctx, func() error {
		if err := l.git.initBare(ctx); err != nil {
			return err
		}
		if err := l.initClone(); err != nil {
			return err
		}
		refs, err := l.git.listRefs(ctx, refPrefix)
		if err != nil {
			return err
		}
		if _, ok := refs[genesisRef]; ok {
			return ErrStoreExists
		}
		if err := l.writeGenesis(ctx); err != nil {
			return err
		}
		if err := l.refresh(ctx); err != nil {
			return err
		}
		return l.joinLocked(ctx, true)
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Join opens a store and joins it as a new member: a member.join in this
// clone's own ref plus the create-once claim ref of the member id. When the
// identity state already holds this key for the member id, it writes
// nothing and just opens. ErrIDTaken means another join claimed the id.
func Join(ctx context.Context, opts Options) (*Ledger, error) {
	l, err := newLedger(ctx, opts)
	if err != nil {
		return nil, err
	}
	err = l.withLock(ctx, func() error {
		if err := l.initClone(); err != nil {
			return err
		}
		if err := l.refresh(ctx); err != nil {
			return err
		}
		return l.joinLocked(ctx, false)
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Open opens an existing store.
func Open(ctx context.Context, opts Options) (*Ledger, error) {
	l, err := newLedger(ctx, opts)
	if err != nil {
		return nil, err
	}
	err = l.withLock(ctx, func() error {
		if err := l.initClone(); err != nil {
			return err
		}
		return l.refresh(ctx)
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Close releases the instance. The journal is durable; nothing is lost.
func (l *Ledger) Close() error { return nil }

// MemberID returns this writer's member id.
func (l *Ledger) MemberID() string { return l.member }

// CloneID returns this clone's id.
func (l *Ledger) CloneID() string { return l.clone }

// withLock serializes against this instance and, through a file lock in
// the store directory, against every other process using the directory.
func (l *Ledger) withLock(ctx context.Context, fn func() error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	locked, err := l.lock.TryLockContext(ctx, 20*time.Millisecond)
	if err != nil {
		return fmt.Errorf("ledger: store lock: %w", err)
	}
	if !locked {
		return errors.New("ledger: store lock not acquired")
	}
	defer l.lock.Unlock() //nolint:errcheck
	l.received = 0
	return fn()
}

func randomID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// initClone settles the clone id: the fixed one, else the one kept in Dir,
// else a new random one written there.
func (l *Ledger) initClone() error {
	id := l.opts.CloneID
	if id == "" {
		path := filepath.Join(l.opts.Dir, "clone-id")
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			id = strings.TrimSpace(string(data))
			if !ValidID(id) {
				return fmt.Errorf("ledger: %s holds an invalid clone id", path)
			}
		case errors.Is(err, os.ErrNotExist):
			id = randomID()
			if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
				return err
			}
		default:
			return err
		}
	}
	l.clone = id
	l.stream = streamKey(l.member, id)
	return nil
}

func (l *Ledger) ownRef() string { return memberRef(l.member, l.clone) }

func (l *Ledger) ownTip() string {
	if st := l.refs[l.ownRef()]; st != nil {
		return st.tip
	}
	return ""
}

func (l *Ledger) now() time.Time { return l.opts.Clock.Now().UTC() }

// ensureKey returns the personal key, generating it with ssh-keygen when
// absent. A file lock in the key directory keeps two processes from
// generating at once.
func ensureKey(ctx context.Context, dir string) (path, norm, fp string, err error) {
	path = filepath.Join(dir, "id_ed25519")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", "", err
	}
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		fl := flock.New(filepath.Join(dir, ".lock"))
		if _, err := fl.TryLockContext(ctx, 20*time.Millisecond); err != nil {
			return "", "", "", err
		}
		if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
			out, err := exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "ws-ledger", "-f", path).CombinedOutput()
			if err != nil {
				fl.Unlock() //nolint:errcheck
				return "", "", "", fmt.Errorf("ledger: ssh-keygen could not generate %s: %w: %s", path, err, strings.TrimSpace(string(out)))
			}
		}
		fl.Unlock() //nolint:errcheck
	}
	pub, err := os.ReadFile(path + ".pub")
	if err != nil {
		return "", "", "", fmt.Errorf("ledger: public key: %w", err)
	}
	norm, fp, err = parseKey(string(pub))
	if err != nil {
		return "", "", "", fmt.Errorf("ledger: %s.pub: %w", path, err)
	}
	return path, norm, fp, nil
}

func (l *Ledger) writeGenesis(ctx context.Context) error {
	roles := Roles{
		Version: 1,
		Admins:  []string{l.member},
		Members: []RoleMember{{ID: l.member, Name: l.opts.Name, Keys: []string{l.pubKey}, Leads: json.RawMessage("[]")}},
	}
	data, err := json.MarshalIndent(roles, "", "  ")
	if err != nil {
		return err
	}
	blob, err := l.git.hashObject(ctx, "blob", append(data, '\n'))
	if err != nil {
		return err
	}
	tree, err := l.git.writeTree(ctx, []treeEntry{{mode: "100644", name: rolesFile, oid: blob}})
	if err != nil {
		return err
	}
	msg, _ := json.Marshal(map[string]any{"genesis": 1, "ts": l.now()})
	oid, err := l.git.commitTree(ctx, tree, "", msg, l.member, l.keyPath)
	if err != nil {
		return err
	}
	if err := l.git.updateRef(ctx, genesisRef, oid, ""); err != nil {
		if refs, lerr := l.git.listRefs(ctx, genesisRef); lerr == nil && refs[genesisRef] != "" {
			return ErrStoreExists
		}
		return err
	}
	return nil
}

// joinLocked writes this clone's member.join and the claim ref, claim
// first, unless the identity state already holds this key for the member.
// The creator writes both although genesis holds its key, so every member
// id, the creator's included, has a claim ref; its join is a valid no-op.
func (l *Ledger) joinLocked(ctx context.Context, creator bool) error {
	if !creator && l.view.holdsKey(l.member, l.fp) {
		return nil
	}
	for _, n := range l.view.streams[l.stream] {
		if n.typ() == TypeMemberJoin {
			return nil // joined before; Pending reports the outcome
		}
	}
	if len(l.journal.pending(l.committedMax)) > 0 {
		return errors.New("ledger: cannot join with pending writes")
	}
	tip := l.ownTip()
	if tip != "" {
		// A claim commit must be the first commit of its stream.
		return errors.New("ledger: this clone already has commits but no membership; join from a fresh store directory")
	}
	payload, _ := json.Marshal(JoinPayload{Member: l.member, Name: l.opts.Name, Key: l.pubKey})
	ev := journalEvent{Seq: max(l.journal.maxSeq(), l.committedMax) + 1, TS: l.now(), Type: TypeMemberJoin, Payload: payload}
	oid, err := l.writeEventCommit(ctx, tip, l.seenNow(), ev.TS, []journalEvent{ev})
	if err != nil {
		return err
	}
	if err := l.git.updateRef(ctx, claimRef(l.member), oid, ""); err != nil {
		refs, lerr := l.git.listRefs(ctx, claimRef(l.member))
		if lerr != nil {
			return lerr
		}
		if cur := refs[claimRef(l.member)]; cur != oid {
			return ErrIDTaken
		}
	}
	if err := l.git.updateRef(ctx, l.ownRef(), oid, tip); err != nil {
		return err
	}
	ts := ev.TS
	if err := l.journal.write(journalRec{K: "ev", Seq: ev.Seq, TS: &ts, Type: ev.Type, Payload: ev.Payload}); err != nil {
		return err
	}
	return l.refresh(ctx)
}

// writeEventCommit appends events to the dated event file of parent's tree
// and writes one signed commit carrying the header.
func (l *Ledger) writeEventCommit(ctx context.Context, parent string, seen map[string]int, ts time.Time, events []journalEvent) (string, error) {
	if seen == nil {
		seen = map[string]int{}
	}
	hdr, err := json.Marshal(commitHeader{Stream: l.stream, Seen: seen, TS: ts.UTC()})
	if err != nil {
		return "", err
	}
	path := dayPath(ts)
	var content []byte
	base := ""
	if parent != "" {
		old, _, err := l.git.readBlob(ctx, parent+":"+path)
		if err != nil {
			return "", err
		}
		content = old
		if base, err = l.git.treeOf(ctx, parent); err != nil {
			return "", err
		}
	}
	for _, e := range events {
		line, err := encodeLine(e)
		if err != nil {
			return "", err
		}
		content = append(append(content, line...), '\n')
	}
	blob, err := l.git.hashObject(ctx, "blob", content)
	if err != nil {
		return "", err
	}
	tree, err := l.git.putPath(ctx, base, strings.Split(path, "/"), blob)
	if err != nil {
		return "", err
	}
	return l.git.commitTree(ctx, tree, parent, hdr, l.member, l.keyPath)
}

// refresh re-reads the journal and every local ledger ref, parses what
// moved, checks new signatures, and rebuilds the view.
func (l *Ledger) refresh(ctx context.Context) error {
	if err := l.journal.reload(); err != nil {
		return err
	}
	refs, err := l.git.listRefs(ctx, refPrefix)
	if err != nil {
		return err
	}
	gOID, ok := refs[genesisRef]
	if !ok {
		return ErrNoStore
	}
	if l.genesis == nil || l.genesis.oid != gOID {
		g, err := loadGenesis(ctx, l.git, gOID)
		if err != nil {
			return err
		}
		l.genesis = g
	}
	l.badRefs = nil
	live := map[string]bool{}
	liveClaims := map[string]bool{}
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		oid := refs[name]
		switch {
		case name == genesisRef:
		case strings.HasPrefix(name, memberPrefix):
			member, clone, ok := parseMemberRef(name)
			if !ok {
				l.badRefs = append(l.badRefs, Warning{Ref: name, Kind: WarnBadRef, Detail: "not a ledger member ref name"})
				continue
			}
			live[name] = true
			st := l.refs[name]
			if st == nil {
				st = &refState{owner: streamKey(member, clone)}
				l.refs[name] = st
			}
			if st.tip != oid {
				if err := l.loadRef(ctx, st, oid); err != nil {
					return err
				}
			}
		case strings.HasPrefix(name, claimPrefix):
			member, ok := parseClaimRef(name)
			if !ok {
				l.badRefs = append(l.badRefs, Warning{Ref: name, Kind: WarnBadRef, Detail: "not a ledger claim ref name"})
				continue
			}
			liveClaims[member] = true
			if c := l.claims[member]; c == nil || c.OID != oid {
				raws, err := l.git.logCommits(ctx, oid, "", 1)
				if err != nil {
					return err
				}
				c := &commitRec{OID: oid, Bad: "unreadable claim commit"}
				if len(raws) == 1 {
					c = parseCommit(raws[0])
					if c.Bad == "" && raws[0].PatchBad != "" {
						c.Bad = raws[0].PatchBad
					}
				}
				l.claims[member] = c
			}
		default:
			l.badRefs = append(l.badRefs, Warning{Ref: name, Kind: WarnBadRef, Detail: "not a ledger ref name"})
		}
	}
	for name := range l.refs {
		if !live[name] {
			delete(l.refs, name)
		}
	}
	for member := range l.claims {
		if !liveClaims[member] {
			delete(l.claims, member)
		}
	}
	if err := l.checkSignatures(ctx); err != nil {
		return err
	}
	l.committedMax = l.ownSeqThrough("")
	l.rebuild()
	return nil
}

// ownSeqThrough is the highest seq of this clone's ref that is inside
// causality, counted the way the envelope pass of verification counts it
// (well-formed commit, own stream, good signature, continuous seq), through
// the commit stop ("" = the tip). A writer's next seq follows it, so an
// event broken on its ref never drags the next valid one into a gap.
func (l *Ledger) ownSeqThrough(stop string) int {
	st := l.refs[l.ownRef()]
	if st == nil {
		return 0
	}
	expected := 1
	for _, c := range st.commits {
		info := l.sigCache[c.OID]
		if c.Bad == "" && c.Stream == st.owner && info.state == sigGood && info.fp != "" {
			for _, ln := range c.Lines {
				if ln.Bad == "" && ln.Seq == expected {
					expected++
				}
			}
		}
		if c.OID == stop {
			break
		}
	}
	return expected - 1
}

func (l *Ledger) loadRef(ctx context.Context, st *refState, oid string) error {
	from := ""
	if st.tip != "" && l.git.isAncestor(ctx, st.tip, oid) {
		from = st.tip
	} else {
		*st = refState{owner: st.owner}
	}
	raws, err := l.git.logCommits(ctx, oid, from, 0)
	if err != nil {
		return err
	}
	for _, raw := range raws {
		st.absorb(raw)
	}
	st.tip = oid
	return nil
}

// refCommitList returns every parsed member ref, sorted by owner stream.
func (l *Ledger) refCommitList() []refCommits {
	var out []refCommits
	for _, st := range l.refs {
		out = append(out, refCommits{Owner: st.owner, Commits: st.commits})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Owner < out[j].Owner })
	return out
}

// checkSignatures verifies, through git, every commit not yet in the cache.
// Results are a pure function of the commit, so the cache never goes stale.
func (l *Ledger) checkSignatures(ctx context.Context) error {
	var oids []string
	want := map[string]bool{}
	need := func(c *commitRec) {
		if _, ok := l.sigCache[c.OID]; !ok && !want[c.OID] && oidRe.MatchString(c.OID) {
			want[c.OID] = true
			oids = append(oids, c.OID)
		}
	}
	list := l.refCommitList()
	for _, r := range list {
		for _, c := range r.Commits {
			need(c)
		}
	}
	for _, c := range l.claims {
		need(c)
	}
	if len(oids) == 0 {
		return nil
	}
	res, err := l.git.verifySigs(ctx, oids, allowedSigners(l.genesis.roles, list, l.claims))
	if err != nil {
		return err
	}
	for oid, info := range res {
		l.sigCache[oid] = info
	}
	return nil
}

func (l *Ledger) applySig(c *commitRec) {
	if info, ok := l.sigCache[c.OID]; ok {
		c.Sig, c.FP = info.state, info.fp
	}
}

// rebuild recomputes the view from the parsed refs plus the open batch, and
// stamps a new generation on every event that is new or changed.
func (l *Ledger) rebuild() {
	in := &verifyInput{
		Roles:      l.genesis.roles,
		GenesisRaw: l.genesis.raw,
		Claims:     map[string]*commitRec{},
		Veto:       l.opts.Veto,
	}
	for m, c := range l.claims {
		l.applySig(c)
		in.Claims[m] = c
	}
	in.Refs = l.refCommitList()
	for _, r := range in.Refs {
		for _, c := range r.Commits {
			l.applySig(c)
		}
	}
	l.pendingCommit = nil
	if pend := l.journal.pending(l.committedMax); len(pend) > 0 {
		pc := &commitRec{Stream: l.stream, Seen: copySeen(l.journal.batchSeen), TS: pend[len(pend)-1].TS, Sig: sigGood, FP: l.fp, Pending: true}
		for _, e := range pend {
			pc.Lines = append(pc.Lines, lineRec{Seq: e.Seq, TS: e.TS, Type: e.Type, Origin: e.Origin, Payload: e.Payload})
		}
		l.pendingCommit = pc
		placed := false
		for i := range in.Refs {
			if in.Refs[i].Owner == l.stream {
				in.Refs[i].Commits = append(append([]*commitRec(nil), in.Refs[i].Commits...), pc)
				placed = true
			}
		}
		if !placed {
			in.Refs = append(in.Refs, refCommits{Owner: l.stream, Commits: []*commitRec{pc}})
			sort.Slice(in.Refs, func(i, j int) bool { return in.Refs[i].Owner < in.Refs[j].Owner })
		}
	}
	l.view = buildView(in)

	next := l.gen + 1
	changed := false
	for _, n := range l.view.nodes {
		m, ok := l.marks[n.id]
		if ok && m.verdict == n.verdict && m.reason == n.reason && m.pending == n.commit.Pending {
			continue
		}
		if !ok && n.stream != l.stream {
			l.received++
		}
		l.marks[n.id] = mark{verdict: n.verdict, reason: n.reason, pending: n.commit.Pending, gen: next}
		changed = true
	}
	if changed {
		l.gen = next
	}
}

func copySeen(s map[string]int) map[string]int {
	out := make(map[string]int, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

func equalSeen(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// seenNow is the seen a commit written now records: for every other clone
// stream, the last seq of its prefix that is resolved (not waiting), so
// this clone's own events never wait on its own view.
func (l *Ledger) seenNow() map[string]int {
	out := map[string]int{}
	for s, h := range l.view.heads {
		if s != l.stream {
			out[s] = h
		}
	}
	return out
}

// batchSignable reports whether the open batch would verify if closed now:
// this clone's key is a live key of its member in the batch's causal past.
func (l *Ledger) batchSignable() bool {
	if l.pendingCommit == nil {
		return true
	}
	r, ok := l.view.commitRes[l.pendingCommit]
	return !ok || r.verdict == Valid
}

// closeBatch closes the open batch into one signed commit on this clone's
// own ref. When this clone's key is not live in the batch's causal past the
// batch stays open; once the current identity state holds the key, the
// batch takes the current seen first, so its commit verifies.
func (l *Ledger) closeBatch(ctx context.Context) (bool, error) {
	pend := l.journal.pending(l.committedMax)
	if len(pend) == 0 {
		return false, nil
	}
	if !l.batchSignable() {
		now := l.seenNow()
		if !l.view.holdsKey(l.member, l.fp) || equalSeen(now, l.journal.batchSeen) {
			return false, nil
		}
		if err := l.journal.write(journalRec{K: "open", Seen: now}); err != nil {
			return false, err
		}
		l.rebuild()
		if !l.batchSignable() {
			return false, nil
		}
	}
	if pend[0].Seq != l.committedMax+1 {
		return false, errJournalGap
	}
	tip := l.ownTip()
	oid, err := l.writeEventCommit(ctx, tip, l.journal.batchSeen, l.now(), pend)
	if err != nil {
		return false, err
	}
	if err := l.git.updateRef(ctx, l.ownRef(), oid, tip); err != nil {
		return false, err
	}
	return true, l.refresh(ctx)
}

// receiveClose closes the open batch when what this clone has received
// moved past the batch's seen, so every commit's seen stays exact.
func (l *Ledger) receiveClose(ctx context.Context) (bool, error) {
	if l.pendingCommit == nil || equalSeen(l.seenNow(), l.journal.batchSeen) {
		return false, nil
	}
	return l.closeBatch(ctx)
}

// Append writes events durably to this clone's journal.
func (l *Ledger) Append(ctx context.Context, events []NewEvent) ([]EventID, error) {
	for i, e := range events {
		if e.Type == "" {
			return nil, fmt.Errorf("ledger: event %d: type required", i)
		}
		if e.Type == TypeMemberJoin {
			return nil, fmt.Errorf("ledger: event %d: %s is written by Join", i, TypeMemberJoin)
		}
		if len(e.Payload) > 0 && !json.Valid(e.Payload) {
			return nil, fmt.Errorf("ledger: event %d: payload is not valid JSON", i)
		}
	}
	var ids []EventID
	err := l.withLock(ctx, func() error {
		if err := l.refresh(ctx); err != nil {
			return err
		}
		if _, err := l.receiveClose(ctx); err != nil {
			return err
		}
		var err error
		ids, err = l.appendLocked(events, nil)
		return err
	})
	return ids, err
}

func (l *Ledger) appendLocked(events []NewEvent, of []int) ([]EventID, error) {
	if len(events) == 0 {
		return nil, nil
	}
	var recs []journalRec
	if len(l.journal.pending(l.committedMax)) == 0 {
		recs = append(recs, journalRec{K: "open", Seen: l.seenNow()})
	}
	seq := max(l.journal.maxSeq(), l.committedMax)
	ts := l.now()
	ids := make([]EventID, 0, len(events))
	for i, e := range events {
		seq++
		payload := e.Payload
		if len(payload) == 0 {
			payload = json.RawMessage("{}")
		}
		r := journalRec{K: "ev", Seq: seq, TS: &ts, Type: e.Type, Origin: e.Origin, Payload: payload}
		if of != nil {
			r.Of = of[i]
		}
		recs = append(recs, r)
		ids = append(ids, EventID{Member: l.member, Clone: l.clone, Seq: seq})
	}
	if err := l.journal.write(recs...); err != nil {
		return nil, err
	}
	l.rebuild()
	return ids, nil
}

// reissue re-appends this clone's own events that a revocation of its old
// key cut, once the identity state holds the current key. Only events this
// clone's journal recorded are re-issued, never events found on its ref.
func (l *Ledger) reissue() (int, error) {
	if !l.view.holdsKey(l.member, l.fp) {
		return 0, nil
	}
	var events []NewEvent
	var of []int
	for _, n := range l.view.streams[l.stream] {
		if n.verdict != Ignored || n.typ() == TypeMemberJoin || l.journal.reissued[n.id.Seq] {
			continue
		}
		je, ok := l.journal.event(n.id.Seq)
		if !ok || je.Type != n.line.Type {
			continue
		}
		events = append(events, NewEvent{Type: je.Type, Origin: je.Origin, Payload: je.Payload})
		of = append(of, n.id.Seq)
	}
	if len(events) == 0 {
		return 0, nil
	}
	if _, err := l.appendLocked(events, of); err != nil {
		return 0, err
	}
	return len(events), nil
}

// Sync closes the open batch, verifies, and re-issues cut events. With no
// remote that is the whole apply path.
func (l *Ledger) Sync(ctx context.Context) (SyncReport, error) {
	var rep SyncReport
	err := l.withLock(ctx, func() error {
		if err := l.refresh(ctx); err != nil {
			return err
		}
		closed, err := l.closeBatch(ctx)
		if err != nil {
			return err
		}
		rep.Closed = closed
		n, err := l.reissue()
		if err != nil {
			return err
		}
		rep.Reissued = n
		if n > 0 {
			if closed, err = l.closeBatch(ctx); err != nil {
				return err
			}
			rep.Closed = rep.Closed || closed
		}
		return nil
	})
	l.finishReport(&rep)
	return rep, err
}

// Fetch receives without pushing. With no remote it verifies local refs.
func (l *Ledger) Fetch(ctx context.Context) (SyncReport, error) {
	var rep SyncReport
	err := l.withLock(ctx, func() error {
		if err := l.refresh(ctx); err != nil {
			return err
		}
		closed, err := l.receiveClose(ctx)
		rep.Closed = closed
		return err
	})
	l.finishReport(&rep)
	return rep, err
}

func (l *Ledger) finishReport(rep *SyncReport) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rep.Received = l.received
	rep.Warnings = append(rep.Warnings, l.badRefs...)
	rep.Pending = l.pendingLocked()
}

// Pending reports this clone's own writes not yet pushed.
func (l *Ledger) Pending() PendingReport {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pendingLocked()
}

func (l *Ledger) pendingLocked() PendingReport {
	var rep PendingReport
	if l.view == nil {
		return rep
	}
	pend := l.journal.pending(l.committedMax)
	rep.Uncommitted = len(pend)
	rep.Count = len(pend)
	if len(pend) > 0 {
		rep.Oldest = pend[0].TS
	}
	rep.KeyNotRegistered = !l.view.holdsKey(l.member, l.fp)
	if c := l.claims[l.member]; c != nil {
		for _, n := range l.view.streams[l.stream] {
			if n.typ() == TypeMemberJoin && n.idp != nil && !claimMatches(c, n) && n.verdict == Flagged {
				rep.IDTaken = true
			}
		}
	}
	return rep
}

// Events returns the events new since after plus every event whose verdict
// changed since it, in display order.
func (l *Ledger) Events(after Cursor) ([]Event, Cursor) {
	l.mu.Lock()
	defer l.mu.Unlock()
	all := after.Epoch != l.epoch
	var out []Event
	for _, n := range l.view.nodes {
		if all || (n.state != stBroken && n.id.Seq > after.Watermark[n.stream]) || l.marks[n.id].gen > after.Gen {
			out = append(out, n.event())
		}
	}
	cur := Cursor{Watermark: map[string]int{}, Gen: l.gen, Epoch: l.epoch}
	for s, list := range l.view.streams {
		cur.Watermark[s] = len(list)
	}
	return out, cur
}

// Concurrent reports whether neither event is in the other's causal past.
func (l *Ledger) Concurrent(a, b EventID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	na, nb := l.view.byID[a], l.view.byID[b]
	if na == nil || nb == nil || na == nb || na.state != stResolved || nb.state != stResolved {
		return false
	}
	return !hb(na, nb) && !hb(nb, na)
}

// Identity returns the identity state folded from genesis and every valid
// identity event.
func (l *Ledger) Identity() IdentityState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.view.identity(l.genesis.raw)
}
