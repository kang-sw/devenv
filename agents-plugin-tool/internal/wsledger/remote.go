package wsledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Remote sync. The store's bare repository has one remote, "origin". Three
// ref namespaces meet here:
//
//	refs/heads/tracker/...         local copies, the only refs verification reads
//	refs/heads/attach/...          local copies of the attachment refs (attach.go)
//	refs/ledger/remote/...         the remote as the last fetch saw it (pruned)
//	refs/ledger/known/...          the last value of each ref known to have been
//	                               on the remote (fetched or pushed); never
//	                               pruned, so a deletion stays visible
//
// A fetch moves a local copy only into existence or forward along a member
// or attachment ref; anything else is refused with a warning and the local
// copy is kept (rewind refusal). Only a ref value that was once pushed or
// fetched is protected: a create-once ref this clone wrote but never pushed
// is replaced by the remote's when its push loses.
//
// Fetch modes: attachment refs are fetched first, with --filter=blob:none
// (commits and trees only; git marks the remote as the promisor on the first
// filtered fetch), then event refs with --no-filter, so verification never
// lacks an event blob. Git also records the filter in
// remote.origin.partialclonefilter, which a later plain fetch would inherit,
// so it is unset after every filtered fetch; a binary that predates
// attachments on the same clone then fetches whole. When the attachment
// fetch fails the event fetch does not run, so an absorbed event never
// names a blob missing from every attachment tree this clone has. The wiki
// refs (refs/heads/wiki/<name>, wiki.go) are fetched last, whole, with
// their own follow-the-remote rules; they are not ledger refs, so a wiki
// fetch or push failure is a WarnWikiSyncFailed, never a failed sync.

const (
	remoteName  = "origin"
	snapPrefix  = "refs/ledger/remote/"
	knownPrefix = "refs/ledger/known/"
)

// ledgerNamespaces are the branch namespaces a store uses under
// refs/heads/, refs/ledger/remote/, and refs/ledger/known/.
var ledgerNamespaces = []string{"tracker/", "attach/"}

// listLedgerRefs lists every ref under prefix + each ledger namespace.
func (g *gitRepo) listLedgerRefs(ctx context.Context, prefix string) (map[string]string, error) {
	out := map[string]string{}
	for _, ns := range ledgerNamespaces {
		refs, err := g.listRefs(ctx, prefix+ns)
		if err != nil {
			return nil, err
		}
		for k, v := range refs {
			out[k] = v
		}
	}
	return out, nil
}

func snapRef(ref string) string  { return snapPrefix + strings.TrimPrefix(ref, "refs/heads/") }
func knownRef(ref string) string { return knownPrefix + strings.TrimPrefix(ref, "refs/heads/") }

// rekey maps refs listed under a private namespace back to their branch
// names.
func rekey(refs map[string]string, prefix string) map[string]string {
	out := make(map[string]string, len(refs))
	for name, oid := range refs {
		out["refs/heads/"+strings.TrimPrefix(name, prefix)] = oid
	}
	return out
}

type refKind int

const (
	kindBad refKind = iota
	kindGenesis
	kindClaim
	kindMember
	kindAttach
)

// appendOnly reports a ref that only ever moves forward (a member's event
// ref or attachment ref), as opposed to a create-once ref.
func (k refKind) appendOnly() bool { return k == kindMember || k == kindAttach }

func classifyRef(ref string) refKind {
	if ref == genesisRef {
		return kindGenesis
	}
	if _, ok := parseClaimRef(ref); ok {
		return kindClaim
	}
	if _, _, ok := parseMemberRef(ref); ok {
		return kindMember
	}
	if _, _, ok := parseAttachRef(ref); ok {
		return kindAttach
	}
	return kindBad
}

func (g *gitRepo) remoteURL(ctx context.Context) string {
	out, err := g.run(ctx, "config", "--get", "remote."+remoteName+".url")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (g *gitRepo) setRemoteURL(ctx context.Context, url string) error {
	_, err := g.run(ctx, "config", "remote."+remoteName+".url", url)
	return err
}

// remoteArgs disables credential prompting where git understands it, so a
// background sync never blocks on a prompt.
func remoteArgs(args ...string) []string {
	return append([]string{"-c", "credential.interactive=false"}, args...)
}

// fetchNamespace sets one snapshot namespace to exactly the remote's
// branches under it. A fetch with nothing new transfers no objects. The
// combined output is returned for the caller to read git's warnings.
func (g *gitRepo) fetchNamespace(ctx context.Context, ns string, mode ...string) ([]byte, error) {
	args := append([]string{"fetch", "--no-tags", "--no-write-fetch-head",
		"--prune", "--no-auto-gc", "--no-recurse-submodules"}, mode...)
	args = append(args, remoteName, "+refs/heads/"+ns+"*:"+snapPrefix+ns+"*")
	return g.run(ctx, remoteArgs(args...)...)
}

// fetchRefs fetches the remote's ledger refs into the snapshot namespace in
// the two modes the file comment describes: attachment refs filtered, then
// event refs whole. It reports whether the server ignored the filter. The
// wiki refs are fetchRemote's own step.
func (g *gitRepo) fetchRefs(ctx context.Context) (noFilter bool, err error) {
	out, err := g.fetchNamespace(ctx, "attach/", "--filter=blob:none")
	// Unset before judging the error: a failed filtered fetch may have
	// recorded the filter already.
	if _, uerr := g.run(ctx, "config", "--unset", "remote."+remoteName+".partialclonefilter"); uerr != nil && err == nil {
		// Exit 5 is "no such key"; anything else is a real failure.
		if !strings.Contains(uerr.Error(), "exit status 5") {
			return false, uerr
		}
	}
	if err != nil {
		return false, err
	}
	noFilter = strings.Contains(string(out), noFilterMarker)
	if _, err := g.fetchNamespace(ctx, "tracker/", "--no-filter"); err != nil {
		return noFilter, err
	}
	return noFilter, nil
}

type pushOutcome int

const (
	pushOK pushOutcome = iota
	pushRejected
)

// pushSpec is one ref of a push: pushed to its own name on the remote,
// fast-forward only, or, with Lease set, forced but only while the remote
// still holds *Lease ("" = must not exist).
type pushSpec struct {
	Ref   string
	Lease *string
	// Src is what to push to Ref: an object id, or "" for the local Ref
	// itself (a wiki save pushes a commit before it moves the local ref).
	Src string
}

func (s pushSpec) src() string {
	if s.Src != "" {
		return s.Src
	}
	return s.Ref
}

// push pushes one ref (see pushRefs).
func (g *gitRepo) push(ctx context.Context, ref string, lease *string) (pushOutcome, error) {
	return g.pushRefs(ctx, pushSpec{Ref: ref, Lease: lease})
}

// pushCommit pushes commit to ref under lease (see pushRefs).
func (g *gitRepo) pushCommit(ctx context.Context, ref, commit string, lease *string) (pushOutcome, error) {
	return g.pushRefs(ctx, pushSpec{Ref: ref, Lease: lease, Src: commit})
}

// pushRefs pushes the refs in one push, atomic when there are several (the
// remote takes all or none; a host without the atomic capability fails the
// push with an error naming it). A client-side "[rejected]" on any ref (the
// remote ref is not what this push expects: a lost race, a foreign advance,
// a deletion under a lease, or the atomic transaction failed on another
// ref) is pushRejected, which the fetch after the push explains. A "[remote
// rejected]" (a hook, branch protection, or missing permission) and a
// transport or credential failure are errors, so Sync reports them.
func (g *gitRepo) pushRefs(ctx context.Context, specs ...pushSpec) (pushOutcome, error) {
	args := []string{"push", "--porcelain", "--no-verify"}
	if len(specs) > 1 {
		args = append(args, "--atomic")
	}
	for _, s := range specs {
		if s.Lease != nil {
			// No "+" on the refspec: a forced refspec overrides the lease. A
			// matching lease alone permits a non-fast-forward update.
			args = append(args, "--force-with-lease="+s.Ref+":"+*s.Lease)
		}
	}
	args = append(args, remoteName)
	for _, s := range specs {
		args = append(args, s.src()+":"+s.Ref)
	}
	_, err := g.run(ctx, remoteArgs(args...)...)
	if err == nil {
		return pushOK, nil
	}
	for _, s := range specs {
		if strings.Contains(err.Error(), "!\t"+s.src()+":"+s.Ref+"\t[rejected]") {
			return pushRejected, nil
		}
	}
	return pushOK, err
}

func (l *Ledger) hasRemote(ctx context.Context) bool {
	if !l.remoteChecked {
		l.remote = l.git.remoteURL(ctx)
		l.remoteChecked = true
	}
	return l.remote != ""
}

func (l *Ledger) setKnown(ctx context.Context, ref, oid string) error {
	if err := l.git.updateRef(ctx, knownRef(ref), oid, ""); err != nil {
		// update-ref with an empty old value refuses an existing ref; move it.
		if _, err := l.git.run(ctx, "update-ref", "-m", "ledger", knownRef(ref), oid); err != nil {
			return err
		}
	}
	if ref == l.ownRef() {
		l.knownOwn = oid
	}
	return nil
}

func (l *Ledger) loadKnownOwn(ctx context.Context) error {
	refs, err := l.git.listRefs(ctx, knownRef(l.ownRef()))
	if err != nil {
		return err
	}
	l.knownOwn = refs[knownRef(l.ownRef())]
	return nil
}

// fetchRemote fetches and applies the snapshot to the local copies, then
// fetches and absorbs the wiki refs. A failed wiki fetch is a warning, not
// an error: the ledger refs are absorbed already, and the wikis stay as
// the last fetch left them.
func (l *Ledger) fetchRemote(ctx context.Context) error {
	noFilter, err := l.git.fetchRefs(ctx)
	if err != nil {
		return fmt.Errorf("ledger: fetch: %w", err)
	}
	if noFilter && !l.noFilterWarned {
		l.noFilterWarned = true
		l.pendingWarns = append(l.pendingWarns, noFilterWarning(attachPrefix+"*"))
	}
	if err := l.absorbRemote(ctx); err != nil {
		return err
	}
	if err := l.git.fetchWikiRefs(ctx); err != nil {
		// A Sync can fetch twice before its report drains this (a reissue
		// publishes again); the report carries the failure once.
		l.addPending(Warning{Ref: wikiPrefix + "*", Kind: WarnWikiSyncFailed,
			Detail: "wiki fetch failed: " + err.Error() + "; the wikis stay as the last fetch left them and the next sync retries"})
		return nil
	}
	return l.absorbWiki(ctx)
}

// addPending queues w for the next report, replacing a queued warning of
// the same ref and kind.
func (l *Ledger) addPending(w Warning) {
	for i, p := range l.pendingWarns {
		if p.Ref == w.Ref && p.Kind == w.Kind {
			l.pendingWarns[i] = w
			return
		}
	}
	l.pendingWarns = append(l.pendingWarns, w)
}

// absorbRemote applies the fetched snapshot to the local copies under the
// rewind-refusal rules and records the anomalies in l.remoteWarns.
func (l *Ledger) absorbRemote(ctx context.Context) error {
	local, err := l.git.listLedgerRefs(ctx, "refs/heads/")
	if err != nil {
		return err
	}
	snapRaw, err := l.git.listLedgerRefs(ctx, snapPrefix)
	if err != nil {
		return err
	}
	knownRaw, err := l.git.listLedgerRefs(ctx, knownPrefix)
	if err != nil {
		return err
	}
	snap, known := rekey(snapRaw, snapPrefix), rekey(knownRaw, knownPrefix)
	names := map[string]bool{}
	for n := range local {
		names[n] = true
	}
	for n := range snap {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	l.remoteWarns = nil
	l.foreign, l.ownBlocked = false, false
	warn := func(ref, kind, detail string) {
		l.remoteWarns = append(l.remoteWarns, Warning{Ref: ref, Kind: kind, Detail: detail})
	}
	take := func(ref, s, old string) error {
		if err := l.git.updateRef(ctx, ref, s, old); err != nil {
			return err
		}
		return l.setKnown(ctx, ref, s)
	}
	own, ownAttach := l.ownRef(), l.ownAttachRef()
	for _, name := range sorted {
		L, S, K := local[name], snap[name], known[name]
		kind := classifyRef(name)
		if kind == kindBad {
			if S != "" && L == "" {
				warn(name, WarnBadRef, "a remote ref under the ledger namespace that is not a ledger ref; not taken")
			}
			continue
		}
		// This clone's attachment ref follows the same rules as its event
		// ref: the two move together in one atomic push.
		isOwn := name == own || name == ownAttach
		switch {
		case S == "":
			if L != "" && K != "" {
				warn(name, WarnDeleted, "deleted on the remote; the local copy is kept, and Restore pushes it back")
				if isOwn {
					l.ownBlocked = true
				}
			}
		case L == "":
			if err := take(name, S, ""); err != nil {
				return err
			}
		case L == S:
			if K != S {
				if err := l.setKnown(ctx, name, S); err != nil {
					return err
				}
			}
		case isOwn && l.git.isAncestor(ctx, S, L):
			// The remote is behind this clone: ordinary unpushed commits,
			// unless it moved back behind what was last pushed.
			if K != "" && K != S && l.git.isAncestor(ctx, S, K) {
				warn(name, WarnRewound, "the remote moved this clone's ref back behind what it pushed; the local copy is kept, and Restore pushes it back")
				l.ownBlocked = true
			} else if K != S {
				// The remote holds more of this clone's ref than recorded (a
				// push whose record was lost); the push lease needs the
				// actual value.
				if err := l.setKnown(ctx, name, S); err != nil {
					return err
				}
			}
		case isOwn:
			warn(name, WarnForeignAdvance, "the remote copy of this clone's ref holds commits this clone did not write; not taken. MoveToFreshClone moves the unpushed events to a fresh clone id")
			l.foreign = true
		case kind.appendOnly() && l.git.isAncestor(ctx, L, S):
			if err := take(name, S, L); err != nil {
				return err
			}
		case !kind.appendOnly() && K == "":
			// A create-once ref this clone wrote and never pushed lost to
			// the remote's.
			if err := take(name, S, L); err != nil {
				return err
			}
		case !kind.appendOnly():
			warn(name, WarnRewound, "a create-once ref moved on the remote; the local copy is kept")
		default:
			warn(name, WarnRewound, "moved on the remote to a non-descendant of the local copy; the local copy is kept, and Restore pushes it back")
		}
	}
	return nil
}

// ownJoined reports whether this clone's stream opens with its member.join.
func (l *Ledger) ownJoined() bool {
	for _, n := range l.view.streams[l.stream] {
		if n.typ() == TypeMemberJoin {
			return true
		}
	}
	return false
}

// pushAll publishes what this clone wrote: a genesis it created, then the
// claim of its join, then its own ref, each only after the one before is on
// the remote. A lost genesis or claim race fetches, so the remote's ref
// replaces the local one and the view converges with everyone's.
func (l *Ledger) pushAll(ctx context.Context) (bool, error) {
	local, err := l.git.listLedgerRefs(ctx, "refs/heads/")
	if err != nil {
		return false, err
	}
	knownRaw, err := l.git.listLedgerRefs(ctx, knownPrefix)
	if err != nil {
		return false, err
	}
	known := rekey(knownRaw, knownPrefix)
	lost := func() error {
		if err := l.fetchRemote(ctx); err != nil {
			return err
		}
		return l.refresh(ctx)
	}
	if g := local[genesisRef]; g != "" && known[genesisRef] == "" {
		out, err := l.git.push(ctx, genesisRef, nil)
		if err != nil {
			return false, fmt.Errorf("ledger: push: %w", err)
		}
		if out == pushRejected {
			if err := lost(); err != nil {
				return false, err
			}
		} else if err := l.setKnown(ctx, genesisRef, g); err != nil {
			return false, err
		}
	}
	own := l.ownRef()
	if c := claimRef(l.member); local[own] != "" && local[c] != "" && known[c] == "" && l.ownJoined() {
		out, err := l.git.push(ctx, c, nil)
		if err != nil {
			return false, fmt.Errorf("ledger: push: %w", err)
		}
		if out == pushRejected {
			return false, lost()
		}
		if err := l.setKnown(ctx, c, local[c]); err != nil {
			return false, err
		}
		known[c] = local[c]
	}
	if l.pendingLocked().IDTaken {
		return false, nil
	}
	attach := l.ownAttachRef()
	tip, atip := local[own], local[attach]
	if tip == "" || l.foreign || l.ownBlocked {
		return false, nil
	}
	if tip == known[own] && (atip == "" || atip == known[attach]) {
		return false, nil
	}
	if l.ownJoined() && known[claimRef(l.member)] == "" && local[claimRef(l.member)] != "" {
		return false, nil // the claim did not reach the remote
	}
	// Leased on the value last known on the remote rather than a plain
	// fast-forward, so a Sync that runs before any fetch noticed a deletion
	// or a rewind (the push-delay timer runs Sync unattended) cannot push
	// the ref back; only Restore does. The local tip always descends from
	// that value, which the guard keeps true for this forced push.
	lease := known[own]
	if lease != "" && !l.git.isAncestor(ctx, lease, tip) {
		return false, nil
	}
	// The attachment ref goes in the same atomic push, first in the refspec
	// list as in the local commit order, so the remote never holds an event
	// commit without the blobs it names.
	var specs []pushSpec
	if atip != "" && atip != known[attach] {
		alease := known[attach]
		if alease != "" && !l.git.isAncestor(ctx, alease, atip) {
			return false, nil
		}
		specs = append(specs, pushSpec{Ref: attach, Lease: &alease})
	}
	specs = append(specs, pushSpec{Ref: own, Lease: &lease})
	out, err := l.git.pushRefs(ctx, specs...)
	if err != nil {
		return false, fmt.Errorf("ledger: push: %w", err)
	}
	if out == pushRejected {
		// Deleted, rewound, or advanced by another writer: the fetch that
		// follows every push tells which and warns.
		return false, nil
	}
	if len(specs) > 1 {
		if err := l.setKnown(ctx, attach, atip); err != nil {
			return false, err
		}
	}
	return true, l.setKnown(ctx, own, tip)
}

// knownOwnMaxSeq is the highest seq of this clone's ref the remote is known
// to hold.
func (l *Ledger) knownOwnMaxSeq() int {
	if l.knownOwn == "" {
		return 0
	}
	return l.ownSeqThrough(l.knownOwn)
}

// Push delay. The deadline is persisted in the store directory so every
// process sharing it agrees; a write or an open moves it later, never
// earlier.

type pushState struct {
	Deadline time.Time `json:"deadline"`
}

func (l *Ledger) pushStatePath() string {
	return filepath.Join(l.opts.Dir, "push-state."+l.stream+".json")
}

func (l *Ledger) readPushState() pushState {
	var st pushState
	if data, err := os.ReadFile(l.pushStatePath()); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func (l *Ledger) writePushState(st pushState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := l.pushStatePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.pushStatePath())
}

// extendDeadline moves the persisted deadline to at least from+delay and
// arms the timer.
func (l *Ledger) extendDeadline(from time.Time) error {
	st := l.readPushState()
	if d := from.Add(l.opts.PushDelay); d.After(st.Deadline) {
		st.Deadline = d
		if err := l.writePushState(st); err != nil {
			return err
		}
	}
	l.armTimer(st.Deadline)
	return nil
}

func (l *Ledger) clearDeadline() error {
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	if l.readPushState().Deadline.IsZero() {
		return nil
	}
	return l.writePushState(pushState{})
}

func (l *Ledger) armTimer(deadline time.Time) {
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	if deadline.IsZero() || l.closed {
		return
	}
	l.timer = l.opts.Clock.AfterFunc(deadline.Sub(l.now()), l.onDeadline)
}

// onDeadline runs the apply path when the persisted deadline has passed; an
// open or a write in another process may have moved it later.
func (l *Ledger) onDeadline() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	st := l.readPushState()
	if st.Deadline.IsZero() {
		l.mu.Unlock()
		return
	}
	if l.now().Before(st.Deadline) {
		l.armTimer(st.Deadline)
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := l.Sync(ctx); err != nil {
		l.mu.Lock()
		_ = l.extendDeadline(l.now())
		l.mu.Unlock()
	}
}

// Register clones the store at url into this store directory. A repository
// with no genesis ref gets a new store with this writer as sole admin; when
// another registration's genesis reaches the remote first, this one falls
// back to joining. A repository with a store is joined, unless the identity
// state already holds this key for the member id (another PC with the
// copied key, a re-clone, or a key an admin added), which writes no join
// and no claim. ErrIDTaken means another join claimed the member id first;
// any other failure after the push landed is a *PushedError.
func Register(ctx context.Context, opts Options, url string) (*Ledger, error) {
	l, err := newLedger(ctx, opts)
	if err != nil {
		return nil, err
	}
	// pushed is the store's genesis once the push landed, nil before.
	var pushed json.RawMessage
	err = l.withLock(ctx, func() error {
		if err := l.git.initBare(ctx); err != nil {
			return err
		}
		switch cur := l.git.remoteURL(ctx); {
		case cur == "":
			if err := l.git.setRemoteURL(ctx, url); err != nil {
				return err
			}
		case cur != url:
			return fmt.Errorf("ledger: the store directory is registered to %s", cur)
		}
		l.remote, l.remoteChecked = url, true
		if err := l.initClone(); err != nil {
			return err
		}
		if err := l.fetchRemote(ctx); err != nil {
			return err
		}
		refs, err := l.git.listRefs(ctx, genesisRef)
		if err != nil {
			return err
		}
		created := refs[genesisRef] == ""
		if created {
			if err := l.writeGenesis(ctx); err != nil {
				return err
			}
		}
		if err := l.refresh(ctx); err != nil {
			return err
		}
		if err := l.loadKnownOwn(ctx); err != nil {
			return err
		}
		if err := l.joinLocked(ctx, created); err != nil {
			return err
		}
		landed, err := l.pushAll(ctx)
		if err != nil {
			return err
		}
		// pushAll reports false when its own ref did not land: nothing new
		// (a key the store already holds), a lost or unpublished claim, or a
		// taken id. Only a landed push leaves the remote holding this
		// registration.
		if landed {
			pushed = append(json.RawMessage(nil), l.genesis.raw...)
		}
		// A store that had no remote until now publishes its wikis too; a
		// wiki the remote refuses is reported (wikiPushWarns), and the next
		// sync retries it.
		if _, err := l.pushLocalWikis(ctx); err != nil {
			return err
		}
		if err := l.fetchRemote(ctx); err != nil {
			return err
		}
		if err := l.refresh(ctx); err != nil {
			return err
		}
		if l.pendingLocked().IDTaken {
			return ErrIDTaken
		}
		return l.afterOpen(ctx)
	})
	if err != nil {
		l.Close()
		if pushed != nil && !errors.Is(err, ErrIDTaken) {
			err = &PushedError{Err: err, Genesis: pushed}
		}
		return nil, err
	}
	return l, nil
}

// PushedError is a Register failure that came after its push landed: the
// remote holds the genesis, or this member's join, though Register opened
// nothing here. Registering again with the same key finishes it without a
// new join.
type PushedError struct {
	Err error
	// Genesis is the store's genesis payload, which names its first admins.
	Genesis json.RawMessage
}

func (e *PushedError) Error() string { return e.Err.Error() }
func (e *PushedError) Unwrap() error { return e.Err }

// Restore pushes the local copy of ref back to the remote, replacing a
// deletion or a rewind there. It is the manual action rewind refusal leaves
// to a person, who judges the cause first; signed events verify whoever
// pushes them. Restoring a member's event ref also restores its attachment
// ref when that one is deleted or rewound (with or without the event ref),
// in the same atomic push, so the remote never holds an event commit
// without the blobs it names, provided this clone holds every blob the
// push needs. Another member's attachment ref was fetched filtered, so its
// blobs are usually absent here: then the event ref goes back alone and a
// WarnNotRestored in the next report names the attachment ref, which a
// clone holding the blobs (the member's own) restores.
func (l *Ledger) Restore(ctx context.Context, ref string) error {
	return l.withLock(ctx, func() error {
		if !l.hasRemote(ctx) {
			return errors.New("ledger: the store has no remote")
		}
		if classifyRef(ref) == kindBad {
			return fmt.Errorf("ledger: %s is not a ledger ref", ref)
		}
		local, err := l.git.listLedgerRefs(ctx, "refs/heads/")
		if err != nil {
			return err
		}
		tip := local[ref]
		if tip == "" {
			return fmt.Errorf("ledger: no local copy of %s", ref)
		}
		if _, err := l.git.fetchRefs(ctx); err != nil {
			return fmt.Errorf("ledger: fetch: %w", err)
		}
		snapRaw, err := l.git.listLedgerRefs(ctx, snapPrefix)
		if err != nil {
			return err
		}
		snap := rekey(snapRaw, snapPrefix)
		// behind reports whether the remote's copy is a deletion or a
		// rewind of the local one (restorable), or already it, or ahead.
		behind := func(ref, tip string) (restorable bool, err error) {
			lease := snap[ref]
			if lease == tip {
				return false, nil
			}
			if lease != "" && l.git.isAncestor(ctx, tip, lease) {
				// Not a deletion or a rewind: the remote moved ahead, and
				// pushing the local copy would discard its newer commits.
				return false, fmt.Errorf("ledger: the remote %s is ahead of the local copy; fetch instead of restoring", ref)
			}
			return true, nil
		}
		restorable, err := behind(ref, tip)
		if err != nil {
			return err
		}
		var specs []pushSpec
		var pushed [][2]string
		var notRestored *Warning
		if member, clone, ok := parseMemberRef(ref); ok {
			// The attachment ref alone may be the one deleted or rewound.
			if a := attachRef(member, clone); local[a] != "" {
				if r, err := behind(a, local[a]); err == nil && r {
					missing, err := l.git.missingObjects(ctx, local[a])
					if err != nil {
						return err
					}
					if missing {
						notRestored = &Warning{Ref: a, Kind: WarnNotRestored,
							Detail: "not restored with " + ref + ": this clone lacks blobs the push needs; restore it from a clone that holds them (the member's own)"}
					} else {
						lease := snap[a]
						specs = append(specs, pushSpec{Ref: a, Lease: &lease})
						pushed = append(pushed, [2]string{a, local[a]})
					}
				}
			}
		}
		if restorable {
			lease := snap[ref]
			specs = append(specs, pushSpec{Ref: ref, Lease: &lease})
			pushed = append(pushed, [2]string{ref, tip})
		}
		warnNotRestored := func() {
			if notRestored != nil {
				l.pendingWarns = append(l.pendingWarns, *notRestored)
			}
		}
		if len(specs) == 0 {
			warnNotRestored()
			return nil // the remote already holds the local copies it can take
		}
		out, err := l.git.pushRefs(ctx, specs...)
		if err != nil {
			return fmt.Errorf("ledger: push: %w", err)
		}
		if out == pushRejected {
			return fmt.Errorf("ledger: the remote %s moved while restoring; fetch and retry", ref)
		}
		warnNotRestored()
		for _, p := range pushed {
			if err := l.setKnown(ctx, p[0], p[1]); err != nil {
				return err
			}
		}
		if err := l.fetchRemote(ctx); err != nil {
			return err
		}
		return l.refresh(ctx)
	})
}

// MoveToFreshClone moves this clone's unpushed events to a fresh clone id
// and ref. It is the way out when the remote copy of this clone's ref holds
// commits this clone did not write (a copied clone id, or a writer with a
// stolen key), so the old ref can no longer be pushed. The old ref's local
// copy goes back to what the remote last knew of it; its unpushed commits
// were never published. It returns the new clone id.
func (l *Ledger) MoveToFreshClone(ctx context.Context) (string, error) {
	var fresh string
	err := l.withLock(ctx, func() error {
		if err := l.refresh(ctx); err != nil {
			return err
		}
		knownMax := l.knownOwnMaxSeq()
		var moved []journalEvent
		for _, e := range l.journal.events {
			if e.Seq > knownMax && e.Type != TypeMemberJoin {
				moved = append(moved, e)
			}
		}
		old := l.ownRef()
		if l.knownOwn != "" {
			if _, err := l.git.run(ctx, "update-ref", "-m", "ledger", old, l.knownOwn); err != nil {
				return err
			}
		} else if l.ownTip() != "" {
			if err := l.git.deleteRef(ctx, old, l.ownTip()); err != nil {
				return err
			}
		}
		// The old attachment ref goes back the same way; the moved events'
		// journal records still name their blobs' git objects, so the fresh
		// clone's first batch close commits them to its own attachment ref.
		if oldAttach := l.ownAttachRef(); l.attach[oldAttach] != nil {
			knownRaw, err := l.git.listRefs(ctx, knownRef(oldAttach))
			if err != nil {
				return err
			}
			if k := knownRaw[knownRef(oldAttach)]; k != "" {
				if _, err := l.git.run(ctx, "update-ref", "-m", "ledger", oldAttach, k); err != nil {
					return err
				}
			} else if err := l.git.deleteRef(ctx, oldAttach, l.attach[oldAttach].tip); err != nil {
				return err
			}
		}
		// The old stream's journal and push deadline retire with it, so
		// reopening the old clone id cannot publish the moved events twice.
		if err := os.Rename(l.journal.path, l.journal.path+".moved"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Remove(l.pushStatePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if l.timer != nil {
			l.timer.Stop()
			l.timer = nil
		}
		fresh = randomID()
		if l.cloneFile {
			if err := os.WriteFile(filepath.Join(l.opts.Dir, "clone-id"), []byte(fresh+"\n"), 0o600); err != nil {
				return err
			}
		} else {
			l.opts.CloneID = fresh
		}
		if err := l.initClone(); err != nil {
			return err
		}
		l.foreign, l.ownBlocked, l.knownOwn = false, false, ""
		if err := l.refresh(ctx); err != nil {
			return err
		}
		if len(moved) > 0 {
			if _, err := l.appendEvents(moved); err != nil {
				return err
			}
			return l.extendDeadline(l.now())
		}
		return nil
	})
	return fresh, err
}
