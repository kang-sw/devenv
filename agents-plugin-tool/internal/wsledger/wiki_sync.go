// Wiki sync reconciles local and remote wiki branches.
package wsledger

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Fetch and absorb.

// fetchWikiRefs sets the wiki snapshot namespace to the remote's wiki
// branches, whole; the file comment says why the filter is off and the
// negotiation tips are the wiki refs alone.
func (g *gitRepo) fetchWikiRefs(ctx context.Context) error {
	_, err := g.fetchNamespace(ctx, wikiNamespace, "--no-filter",
		"--negotiation-tip="+wikiPrefix+"*", "--negotiation-tip="+snapPrefix+wikiNamespace+"*")
	return err
}

// fetchWiki fetches and absorbs the wiki refs alone (a save's fetch).
func (l *Ledger) fetchWiki(ctx context.Context) error {
	if err := l.git.fetchWikiRefs(ctx); err != nil {
		return fmt.Errorf("ledger: wiki fetch: %w", err)
	}
	return l.absorbWiki(ctx)
}

func (l *Ledger) wikiRefs(ctx context.Context) (local, snap, known map[string]string, err error) {
	if local, err = l.git.listRefs(ctx, wikiPrefix); err != nil {
		return nil, nil, nil, err
	}
	snapRaw, err := l.git.listRefs(ctx, snapPrefix+wikiNamespace)
	if err != nil {
		return nil, nil, nil, err
	}
	knownRaw, err := l.git.listRefs(ctx, knownPrefix+wikiNamespace)
	if err != nil {
		return nil, nil, nil, err
	}
	return local, rekey(snapRaw, snapPrefix), rekey(knownRaw, knownPrefix), nil
}

// absorbWiki applies the wiki snapshot to the local copies under the
// follow-the-remote rules and records the anomalies in l.wikiWarns.
func (l *Ledger) absorbWiki(ctx context.Context) error {
	local, snap, known, err := l.wikiRefs(ctx)
	if err != nil {
		return err
	}
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
	l.wikiWarns = nil
	warn := func(ref, kind, detail string) {
		l.wikiWarns = append(l.wikiWarns, Warning{Ref: ref, Kind: kind, Detail: detail})
	}
	recover := func(name, ref, oid string) error {
		// The recovery ref may exist from an earlier follow of the same tip.
		if err := l.git.updateRef(ctx, wikiRecoveryRef(name, oid), oid, ""); err != nil {
			if refs, lerr := l.git.listRefs(ctx, wikiRecoveryRef(name, oid)); lerr != nil || refs[wikiRecoveryRef(name, oid)] != oid {
				return err
			}
		}
		return nil
	}
	for _, ref := range sorted {
		L, S, K := local[ref], snap[ref], known[ref]
		name, ok := wikiNameOf(ref)
		if !ok {
			if S != "" && L == "" {
				warn(ref, WarnBadRef, "a remote ref under the wiki namespace that is not a wiki name; not taken")
			}
			continue
		}
		switch {
		case S == "":
			if L != "" && K != "" {
				if err := recover(name, ref, L); err != nil {
					return err
				}
				if err := l.git.deleteRef(ctx, ref, L); err != nil {
					return err
				}
				warn(ref, WarnDeleted, "deleted on the remote; followed, and the local tip is kept under "+wikiRecoveryRef(name, L))
			}
			// L != "" && K == "": a wiki this clone wrote before the store
			// had a remote; pushLocalWikis publishes it.
		case L == "":
			if err := l.git.updateRef(ctx, ref, S, ""); err != nil {
				return err
			}
			if err := l.setKnown(ctx, ref, S); err != nil {
				return err
			}
		case L == S:
			if K != S {
				if err := l.setKnown(ctx, ref, S); err != nil {
					return err
				}
			}
		case l.git.isAncestor(ctx, L, S):
			if err := l.git.updateRef(ctx, ref, S, L); err != nil {
				return err
			}
			if err := l.setKnown(ctx, ref, S); err != nil {
				return err
			}
		case K == "" && l.git.isAncestor(ctx, S, L):
			// Never pushed, and ahead of the remote: pushLocalWikis
			// fast-forwards the remote under a lease on S.
		default:
			if err := recover(name, ref, L); err != nil {
				return err
			}
			if err := l.git.updateRef(ctx, ref, S, L); err != nil {
				return err
			}
			if err := l.setKnown(ctx, ref, S); err != nil {
				return err
			}
			warn(ref, WarnRewound, "moved on the remote to a non-descendant of the local copy; followed, and the local tip is kept under "+wikiRecoveryRef(name, L))
		}
	}
	return nil
}

// pushLocalWikis pushes every wiki the remote is not known to hold (one
// written before the store had a remote) under a lease on the last
// snapshot of the remote ref: create-only when the remote has none, a
// fast-forward when the local copy descends from it (absorbWiki left that
// case alone). A rejection (the remote moved meanwhile) is left to the
// fetch that follows, which follows the remote and keeps the local tip as
// a recovery point. A push that fails (a remote refusal, a transport
// failure) is returned in failed, by ref, and the other wikis are still
// pushed: one wiki the remote refuses must not hold up the ledger's sync
// or saves to other wikis. err is a local failure. The refusals are kept
// in l.wikiPushErrs until the next call, as the reasons noteUnpublished
// gives.
func (l *Ledger) pushLocalWikis(ctx context.Context, step stepFn) (failed map[string]error, err error) {
	// A wiki ref moves locally only under the network lock, which the
	// caller holds, so each is pushed by name between the step that lists
	// them and the step that records what landed.
	var local, snap map[string]string
	var refs []string
	if err := step(func() error {
		var known map[string]string
		var err error
		if local, snap, known, err = l.wikiRefs(ctx); err != nil {
			return err
		}
		for ref := range local {
			if _, ok := wikiNameOf(ref); ok && known[ref] == "" {
				refs = append(refs, ref)
			}
		}
		if len(refs) == 0 {
			l.wikiPushErrs = nil
		}
		return nil
	}); err != nil || len(refs) == 0 {
		return nil, err
	}
	sort.Strings(refs)
	var landed []string
	for _, ref := range refs {
		lease := snap[ref]
		out, err := l.git.push(ctx, ref, &lease)
		if err != nil {
			if failed == nil {
				failed = map[string]error{}
			}
			failed[ref] = fmt.Errorf("ledger: wiki push: %w", err)
			continue
		}
		if out == pushOK {
			landed = append(landed, ref)
		}
	}
	return failed, step(func() error {
		for _, ref := range landed {
			if err := l.setKnown(ctx, ref, local[ref]); err != nil {
				return err
			}
		}
		l.wikiPushErrs = failed
		return nil
	})
}

// noteUnpublished re-derives l.wikiPushWarns from the refs: a local wiki
// the remote is not known to hold, on a store with a remote, is not
// published yet (its push was refused, or has not run), and stays so until
// a push of it lands from whichever process shares the store. The reason
// is this process's last refusal of it when it saw one. A failure to read
// the refs leaves the warnings as they were.
func (l *Ledger) noteUnpublished(ctx context.Context) {
	if !l.hasRemote(ctx) {
		l.wikiPushWarns = nil
		return
	}
	local, _, known, err := l.wikiRefs(ctx)
	if err != nil {
		return
	}
	unpublished := map[string]error{}
	for ref := range local {
		if _, ok := wikiNameOf(ref); ok && known[ref] == "" {
			if unpublished[ref] = l.wikiPushErrs[ref]; unpublished[ref] == nil {
				unpublished[ref] = errors.New("ledger: wiki not yet published to the remote")
			}
		}
	}
	l.wikiPushWarns = wikiPushWarnings(unpublished)
}

// wikiPushWarnings reports the failed pushes of pushLocalWikis, by ref.
func wikiPushWarnings(failed map[string]error) []Warning {
	refs := make([]string, 0, len(failed))
	for ref := range failed {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	out := make([]Warning, 0, len(refs))
	for _, ref := range refs {
		out = append(out, Warning{Ref: ref, Kind: WarnWikiSyncFailed,
			Detail: failed[ref].Error() + "; the wiki stays on this clone and the next sync retries"})
	}
	return out
}
