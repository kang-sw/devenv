package wsledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Attachments. An attachment is a content-addressed blob (its oid is the
// sha256 of its content) that a member's events name in their payloads. The
// content never enters an event commit: it lives at attachments/<sha256> in
// a chain of signed commits on the member's attachment ref
// (refs/heads/attach/m/<member>/<clone>, one per member ref), each commit
// adding paths and never changing or removing one. Attachment refs are
// fetched with --filter=blob:none, so a clone holds their commits and trees
// and fetches a blob only when ReadAttachment asks for it. The ledger stays
// payload-agnostic: it never reads which events name which blobs; the domain
// hands the newly added blobs to AppendWithBlobs, and closeBatch commits
// them to the attachment ref before the event commit, so an event commit is
// never pushed ahead of the blobs it names (the two refs go in one atomic
// push).
//
// Verification of an attachment ref reads no blob content (a partial clone
// lacks it): a commit is accepted when it is signed by a key its member held
// (live or since revoked; a re-keyed member's old attachments stay readable),
// has one parent at most, and adds only well-formed attachments/<sha256>
// paths. The first refused commit cuts the chain: it and the commits after
// it are not read, and a WarnBadAttachment names it. The content hash is
// checked by AppendWithBlobs before anything is written and by
// ReadAttachment every time it serves a blob.

var (
	// ErrAttachmentNotFound: no verified attachment tree, and no pending
	// write of this clone, holds a blob of that oid whose content hashes to
	// it.
	ErrAttachmentNotFound = errors.New("ledger: attachment not found")
	// ErrBlobMismatch: a blob handed to AppendWithBlobs does not hash to its
	// oid.
	ErrBlobMismatch = errors.New("ledger: blob content does not hash to its oid")
)

// GitEnv is the environment every ledger git run gets. GIT_NO_LAZY_FETCH
// keeps a partial clone from fetching a missing object implicitly: only
// ReadAttachment fetches a blob, explicitly, by its id.
var GitEnv = []string{"GIT_NO_LAZY_FETCH=1"}

// attachState is the parsed history of one attachment ref, kept between
// refreshes so a moved ref parses only its new commits.
type attachState struct {
	member, clone string
	tip           string
	commits       []*attachCommit
}

// attachCommit is one parsed commit of an attachment ref.
type attachCommit struct {
	OID string
	// Adds maps each added blob's sha256 oid to its git blob id.
	Adds map[string]string
	// Bad is a structural malformation ("" when the commit is a pure
	// addition of attachment paths).
	Bad string
}

// attachLoc is one verified copy of an attachment blob.
type attachLoc struct {
	member, clone string
	blob          string // git blob id
}

func (l *Ledger) ownAttachRef() string { return attachRef(l.member, l.clone) }

// blobOID is the attachment oid of data.
func blobOID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// AppendWithBlobs is Append with the content of the attachments the events
// newly name. Every blob must hash to its oid (ErrBlobMismatch otherwise,
// and nothing is written); the blobs' git objects are written now and
// recorded in the journal with the first event, so the batch close commits
// them to this clone's attachment ref whatever process closes it.
func (l *Ledger) AppendWithBlobs(ctx context.Context, events []NewEvent, blobs []Blob) ([]EventID, error) {
	if len(blobs) == 0 {
		return l.Append(ctx, events)
	}
	if len(events) == 0 {
		return nil, errors.New("ledger: blobs need an event that names them")
	}
	for _, b := range blobs {
		if !ValidAttachmentOID(b.OID) {
			return nil, fmt.Errorf("ledger: attachment oid %q is not a lowercase hex sha256", b.OID)
		}
		if got := blobOID(b.Data); got != b.OID {
			return nil, fmt.Errorf("%w: %s hashes to %s", ErrBlobMismatch, b.OID, got)
		}
	}
	if err := checkNewEvents(events); err != nil {
		return nil, err
	}
	var ids []EventID
	err := l.withLock(ctx, func() error {
		if err := l.refresh(ctx); err != nil {
			return err
		}
		if _, err := l.receiveClose(ctx); err != nil {
			return err
		}
		att := map[string]string{}
		for _, b := range blobs {
			oid, err := l.git.hashObject(ctx, "blob", b.Data)
			if err != nil {
				return err
			}
			att[b.OID] = oid
		}
		var err error
		if ids, err = l.appendLocked(events, nil, att); err != nil {
			return err
		}
		if l.hasRemote(ctx) {
			return l.extendDeadline(l.now())
		}
		return nil
	})
	return ids, err
}

// commitAttachments writes the attachment commit of the open batch: one
// commit on this clone's attachment ref adding every blob the pending
// events' journal records name that the ref does not hold yet. It runs
// before the event commit and is a no-op when there is nothing to add.
func (l *Ledger) commitAttachments(ctx context.Context, pend []journalEvent) error {
	adds := map[string]string{}
	for _, e := range pend {
		for oid, blob := range e.Att {
			adds[oid] = blob
		}
	}
	if len(adds) == 0 {
		return nil
	}
	ref := l.ownAttachRef()
	tip, held := "", map[string]bool{}
	if st := l.attach[ref]; st != nil {
		tip = st.tip
		for _, c := range st.commits {
			for oid := range c.Adds {
				held[oid] = true
			}
		}
	}
	oids := make([]string, 0, len(adds))
	for oid := range adds {
		if !held[oid] {
			oids = append(oids, oid)
		}
	}
	if len(oids) == 0 {
		return nil
	}
	sort.Strings(oids)
	tree := ""
	if tip != "" {
		var err error
		if tree, err = l.git.treeOf(ctx, tip); err != nil {
			return err
		}
	}
	for _, oid := range oids {
		var err error
		if tree, err = l.git.putPath(ctx, tree, []string{attachDir, oid}, adds[oid]); err != nil {
			return err
		}
	}
	hdr, err := json.Marshal(commitHeader{Stream: l.stream, TS: l.now()})
	if err != nil {
		return err
	}
	oid, err := l.git.commitTree(ctx, tree, tip, hdr, l.member, l.keyPath)
	if err != nil {
		return err
	}
	return l.git.updateRef(ctx, ref, oid, tip)
}

// loadAttachRefs parses what moved among the local attachment refs.
func (l *Ledger) loadAttachRefs(ctx context.Context) error {
	refs, err := l.git.listRefs(ctx, attachPrefix)
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for name, oid := range refs {
		member, clone, ok := parseAttachRef(name)
		if !ok {
			l.badRefs = append(l.badRefs, Warning{Ref: name, Kind: WarnBadRef, Detail: "not a ledger attachment ref name"})
			continue
		}
		live[name] = true
		st := l.attach[name]
		if st == nil {
			st = &attachState{member: member, clone: clone}
			l.attach[name] = st
		}
		if st.tip == oid {
			continue
		}
		from := ""
		if st.tip != "" && l.git.isAncestor(ctx, st.tip, oid) {
			from = st.tip
		} else {
			st.commits = nil
		}
		commits, err := l.git.attachCommits(ctx, oid, from)
		if err != nil {
			return err
		}
		st.commits = append(st.commits, commits...)
		st.tip = oid
	}
	for name := range l.attach {
		if !live[name] {
			delete(l.attach, name)
		}
	}
	return nil
}

// evalAttach judges every attachment ref against the signatures and the
// folded identity state and rebuilds the index of verified blobs. It runs
// after rebuild.
func (l *Ledger) evalAttach() {
	l.attachIndex = map[string][]attachLoc{}
	l.attachWarns = nil
	names := make([]string, 0, len(l.attach))
	for name := range l.attach {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		st := l.attach[name]
		var m *idMember
		if l.view != nil && l.view.ident != nil {
			m = l.view.ident.members[st.member]
		}
		for _, c := range st.commits {
			bad := c.Bad
			info := l.sigCache[c.OID]
			switch {
			case bad != "":
			case info.state == sigUnsigned:
				bad = "commit is unsigned"
			case info.state != sigGood || info.fp == "":
				bad = "commit signature does not verify"
			case m == nil || (!m.holds(info.fp) && m.revokedBy(info.fp) == nil):
				bad = fmt.Sprintf("signer key %s is not a key of %s", info.fp, st.member)
			}
			if bad != "" {
				short := c.OID
				if len(short) > 12 {
					short = short[:12]
				}
				l.attachWarns = append(l.attachWarns, Warning{Ref: name, Kind: WarnBadAttachment,
					Detail: short + ": " + bad + "; it and the commits after it are not read"})
				break
			}
			for oid, blob := range c.Adds {
				l.attachIndex[oid] = append(l.attachIndex[oid], attachLoc{member: st.member, clone: st.clone, blob: blob})
			}
		}
	}
}

// attachCommitOIDs lists every attachment commit for the signature check.
func (l *Ledger) attachCommitOIDs() []string {
	var out []string
	for _, st := range l.attach {
		for _, c := range st.commits {
			out = append(out, c.OID)
		}
	}
	return out
}

// HasAttachment reports whether a blob of oid is stored: in a verified
// attachment tree of some member (this clone need not hold its content) or
// written by a pending event of this clone.
func (l *Ledger) HasAttachment(oid string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.attachIndex[oid]) > 0 {
		return true
	}
	for _, e := range l.journal.pending(l.committedMax) {
		if _, ok := e.Att[oid]; ok {
			return true
		}
	}
	return false
}

// ReadAttachment returns the content of attachment oid. It tries every
// verified attachment tree holding the path, the trees of prefer (the
// member whose event names the blob) first, then the blobs of this clone's
// pending writes, fetching a blob this clone lacks from the remote by its
// id, and serves the first whose content hashes to oid. A blob that does
// not is never served; a copy another member holds at the same path cannot
// shadow the right one. ErrAttachmentNotFound when none matches; a fetch
// that fails is returned as its own error, since the remote may hold it.
func (l *Ledger) ReadAttachment(ctx context.Context, oid, prefer string) ([]byte, error) {
	if !ValidAttachmentOID(oid) {
		return nil, fmt.Errorf("%w: %q is not an attachment oid", ErrAttachmentNotFound, oid)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var blobs []string
	seen := map[string]bool{}
	add := func(blob string) {
		if !seen[blob] {
			seen[blob] = true
			blobs = append(blobs, blob)
		}
	}
	locs := l.attachIndex[oid]
	for _, loc := range locs {
		if loc.member == prefer {
			add(loc.blob)
		}
	}
	for _, loc := range locs {
		add(loc.blob)
	}
	for _, e := range l.journal.pending(l.committedMax) {
		if blob, ok := e.Att[oid]; ok {
			add(blob)
		}
	}
	if len(blobs) == 0 {
		return nil, ErrAttachmentNotFound
	}
	remote := l.hasRemote(ctx)
	var lastErr, fetchErr error
	for _, blob := range blobs {
		data, ok, err := l.git.readBlob(ctx, blob)
		if err != nil {
			lastErr = err
			continue
		}
		if !ok && remote {
			if err := l.git.fetchObject(ctx, blob); err != nil {
				fetchErr = err
				continue
			}
			data, ok, err = l.git.readBlob(ctx, blob)
			if err != nil {
				lastErr = err
				continue
			}
		}
		if !ok {
			lastErr = fmt.Errorf("blob %s is not in this clone", blob)
			continue
		}
		if blobOID(data) != oid {
			lastErr = fmt.Errorf("blob %s does not hash to %s", blob, oid)
			continue
		}
		return data, nil
	}
	if fetchErr != nil {
		// The remote may hold it: a transport or credential failure is not
		// a missing attachment.
		return nil, fmt.Errorf("ledger: fetch attachment %s: %w", oid, fetchErr)
	}
	return nil, fmt.Errorf("%w: %v", ErrAttachmentNotFound, lastErr)
}

// noFilterWarning is the warning a filtered fetch the server ignored
// produces; the fallback is reported once per store per process.
const noFilterMarker = "filtering not recognized by server"

func noFilterWarning(ref string) Warning {
	return Warning{Ref: ref, Kind: WarnNoFilter,
		Detail: "the remote does not honor partial-clone filters; attachment refs are fetched whole, so this clone holds every attachment"}
}
