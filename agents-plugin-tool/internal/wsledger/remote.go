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
//	refs/ledger/remote/tracker/... the remote as the last fetch saw it (pruned)
//	refs/ledger/known/tracker/...  the last value of each ref known to have been
//	                               on the remote (fetched or pushed); never
//	                               pruned, so a deletion stays visible
//
// A fetch moves a local copy only into existence or forward along a member
// ref; anything else is refused with a warning and the local copy is kept
// (rewind refusal). Only a ref value that was once pushed or fetched is
// protected: a create-once ref this clone wrote but never pushed is replaced
// by the remote's when its push loses.

const (
	remoteName  = "origin"
	snapPrefix  = "refs/ledger/remote/"
	knownPrefix = "refs/ledger/known/"
)

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
)

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

// fetchTracker sets the snapshot namespace to exactly the remote's tracker
// branches. A fetch with nothing new transfers no objects.
func (g *gitRepo) fetchTracker(ctx context.Context) error {
	_, err := g.run(ctx, remoteArgs("fetch", "--quiet", "--no-tags", "--no-write-fetch-head",
		"--prune", "--no-auto-gc", "--no-recurse-submodules",
		remoteName, "+"+refPrefix+"*:"+snapPrefix+"tracker/*")...)
	return err
}

type pushOutcome int

const (
	pushOK pushOutcome = iota
	pushRejected
)

// push pushes the local ref to the same name on the remote, fast-forward
// only, or, with lease set, forced but only while the remote still holds
// *lease ("" = must not exist). A client-side "[rejected]" (the remote ref
// is not what this push expects: a lost race, a foreign advance, a deletion
// under a lease) is pushRejected, which the fetch after the push explains.
// A "[remote rejected]" (a hook, branch protection, or missing permission)
// and a transport or credential failure are errors, so Sync reports them.
func (g *gitRepo) push(ctx context.Context, ref string, lease *string) (pushOutcome, error) {
	args := []string{"push", "--porcelain", "--no-verify"}
	if lease != nil {
		// No "+" on the refspec: a forced refspec overrides the lease. A
		// matching lease alone permits a non-fast-forward update.
		args = append(args, "--force-with-lease="+ref+":"+*lease)
	}
	args = append(args, remoteName, ref+":"+ref)
	_, err := g.run(ctx, remoteArgs(args...)...)
	if err == nil {
		return pushOK, nil
	}
	if strings.Contains(err.Error(), "!\t"+ref+":"+ref+"\t[rejected]") {
		return pushRejected, nil
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

// fetchRemote fetches and applies the snapshot to the local copies.
func (l *Ledger) fetchRemote(ctx context.Context) error {
	if err := l.git.fetchTracker(ctx); err != nil {
		return fmt.Errorf("ledger: fetch: %w", err)
	}
	return l.absorbRemote(ctx)
}

// absorbRemote applies the fetched snapshot to the local copies under the
// rewind-refusal rules and records the anomalies in l.remoteWarns.
func (l *Ledger) absorbRemote(ctx context.Context) error {
	local, err := l.git.listRefs(ctx, refPrefix)
	if err != nil {
		return err
	}
	snapRaw, err := l.git.listRefs(ctx, snapPrefix+"tracker/")
	if err != nil {
		return err
	}
	knownRaw, err := l.git.listRefs(ctx, knownPrefix+"tracker/")
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
	own := l.ownRef()
	for _, name := range sorted {
		L, S, K := local[name], snap[name], known[name]
		kind := classifyRef(name)
		if kind == kindBad {
			if S != "" && L == "" {
				warn(name, WarnBadRef, "a remote ref under the ledger namespace that is not a ledger ref; not taken")
			}
			continue
		}
		isOwn := name == own
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
		case kind == kindMember && l.git.isAncestor(ctx, L, S):
			if err := take(name, S, L); err != nil {
				return err
			}
		case kind != kindMember && K == "":
			// A create-once ref this clone wrote and never pushed lost to
			// the remote's.
			if err := take(name, S, L); err != nil {
				return err
			}
		case kind != kindMember:
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
	local, err := l.git.listRefs(ctx, refPrefix)
	if err != nil {
		return false, err
	}
	knownRaw, err := l.git.listRefs(ctx, knownPrefix+"tracker/")
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
	tip := local[own]
	if tip == "" || tip == known[own] || l.foreign || l.ownBlocked {
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
	out, err := l.git.push(ctx, own, &lease)
	if err != nil {
		return false, fmt.Errorf("ledger: push: %w", err)
	}
	if out == pushRejected {
		// Deleted, rewound, or advanced by another writer: the fetch that
		// follows every push tells which and warns.
		return false, nil
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
// and no claim. ErrIDTaken means another join claimed the member id first.
func Register(ctx context.Context, opts Options, url string) (*Ledger, error) {
	l, err := newLedger(ctx, opts)
	if err != nil {
		return nil, err
	}
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
		if _, err := l.pushAll(ctx); err != nil {
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
		return nil, err
	}
	return l, nil
}

// Restore pushes the local copy of ref back to the remote, replacing a
// deletion or a rewind there. It is the manual action rewind refusal leaves
// to a person, who judges the cause first; signed events verify whoever
// pushes them.
func (l *Ledger) Restore(ctx context.Context, ref string) error {
	return l.withLock(ctx, func() error {
		if !l.hasRemote(ctx) {
			return errors.New("ledger: the store has no remote")
		}
		if classifyRef(ref) == kindBad {
			return fmt.Errorf("ledger: %s is not a ledger ref", ref)
		}
		local, err := l.git.listRefs(ctx, ref)
		if err != nil {
			return err
		}
		tip := local[ref]
		if tip == "" {
			return fmt.Errorf("ledger: no local copy of %s", ref)
		}
		if err := l.git.fetchTracker(ctx); err != nil {
			return fmt.Errorf("ledger: fetch: %w", err)
		}
		snap, err := l.git.listRefs(ctx, snapRef(ref))
		if err != nil {
			return err
		}
		lease := snap[snapRef(ref)]
		if lease == tip {
			return nil // the remote already holds the local copy
		}
		if lease != "" && l.git.isAncestor(ctx, tip, lease) {
			// Not a deletion or a rewind: the remote moved ahead, and
			// pushing the local copy would discard its newer commits.
			return fmt.Errorf("ledger: the remote %s is ahead of the local copy; fetch instead of restoring", ref)
		}
		out, err := l.git.push(ctx, ref, &lease)
		if err != nil {
			return fmt.Errorf("ledger: push: %w", err)
		}
		if out == pushRejected {
			return fmt.Errorf("ledger: the remote %s moved while restoring; fetch and retry", ref)
		}
		if err := l.setKnown(ctx, ref, tip); err != nil {
			return err
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
