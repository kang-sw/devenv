// Package wsledger is a payload-agnostic, signed, multi-writer event log
// over git refs. Every clone of a store appends events to its own branch;
// every view is a client-side merge of all branches. The package owns refs,
// commits, signing and verification, membership and keys, causality,
// waiting and broken events, key revocation, sync, and rewind refusal. It
// never interprets event payloads except the identity events below; event
// types and their rules belong to the domain layer built on top.
//
// # Refs
//
//	refs/heads/tracker/genesis             one signed commit holding roles.json; never updated
//	refs/heads/tracker/id/<member>         claim ref: one commit carrying the member's join; never updated
//	refs/heads/tracker/m/<member>/<clone>  one clone's event stream; append-only
//
// Member and clone ids are lowercase letters, digits, '_' and '-' (see
// ValidID). A stream is "<member>.<clone>" and an event id
// "<member>.<clone>#<seq>"; seq is per stream, continuous across commits.
// Refs are branches so host push permissions and protection rules apply.
//
// # Commit format
//
// The commit message is a JSON header:
//
//	{"stream": "<member>.<clone>", "seen": {"<member>.<clone>": <seq>, ...}, "ts": "<RFC 3339>"}
//
// seen maps every other clone stream to the last seq the writer had
// received. The commit appends JSON lines to events/YYYY/MM/DD.jsonl, the
// file dated (UTC) by the header ts:
//
//	{"seq": 7, "ts": "<write time>", "type": "...", "origin": "...", "payload": {...}}
//
// origin is optional and carried for the domain. Each line's ts is its own
// write time, so a batch spanning days keeps each write's time. A commit
// must be a single pure append to that one file. Commits are SSH-signed
// (gpg.format=ssh) by the clone's key; events carry no signature.
//
// # Genesis roles.json
//
//	{"version": 1, "admins": ["ada"],
//	 "members": [{"id": "ada", "name": "Ada", "keys": ["ssh-ed25519 AAAA..."], "leads": []}]}
//
// The protocol reads admins and each member's id, name, and keys; leads and
// any other field are passed through (IdentityState.Genesis, Member.Leads).
// Create writes it with the creator as sole admin and a root (leads []).
// The genesis commit must be signed by an admin key it lists.
//
// # Identity events
//
// The protocol owns these types; every other type is a domain event.
//
//	member.join          {"member": "<id>", "name": "...", "key": "<ssh public key>"}
//	roles.add_admin      {"member": "<id>"}
//	roles.remove_admin   {"member": "<id>"}
//	roles.add_key        {"member": "<id>", "key": "<ssh public key>"}
//	roles.revoke_key     {"member": "<id>", "key": "<ssh public key>"}
//	roles.remove_member  {"member": "<id>"}
//
// member.join is a self-join by anyone with write access to the repository:
// it is written by Join (or Create) as the first event of a commit of its
// own, and the same commit becomes the member's create-once claim ref. A
// join counts only when the claim commit carries that join (same event id
// and key) signed by the declared key, and the id is free in its causal
// past; a join declaring a key the id already holds is a valid no-op. The
// roles.* events are admin-only. roles.remove_member also runs the domain
// veto hook (Options.Veto), as does every other identity event.
//
// # Verification
//
// Every event gets a Verdict:
//
//   - Broken (Flagged, outside causality): the commit sits in a ref its
//     author does not own, is unsigned or its signature does not verify, is
//     malformed, or the line's seq is a gap or duplicate. A broken event
//     consumes no seq and gets a synthetic id (EventID.Broken).
//   - Waiting: its causal past (its stream's earlier events plus what its
//     commit's seen names, closed transitively) names an event this client
//     has not received, or holds a join whose claim ref has not arrived.
//   - Flagged: it breaks a rule judged against the identity state folded
//     from its causal past only: the commit's signer must be a live key its
//     author held there (a commit opening with a valid member.join verifies
//     against the key the join declares); the author must be a member and
//     not removed; identity events follow their rules and the veto hook.
//   - Ignored: a valid roles.revoke_key cuts at what the revoking admin had
//     seen of each of the member's streams; events signed with the revoked
//     key above that cutoff are ignored whatever their seen claims. This is
//     the one rule that changes an already given verdict, and every client
//     reaches the same result. A revocation is judged against its own causal
//     past only, so a concurrent revocation never cuts it: two admins
//     revoking each other concurrently both take effect.
//
// Signatures are checked through the user's git with an allowed-signers
// file derived from the identity keys; the verdict uses the signer
// fingerprint git reports, so it is a pure function of the commit.
//
// # Journal and batches
//
// Append writes events durably to this clone's journal
// (journal.<member>.<clone>.jsonl in the store directory). The open batch is closed into one signed commit when the
// writer receives new events (its seen would change) or at Sync, so every
// commit's seen is exact. While this clone's key is not a live key of its
// member, the batch stays open (PendingReport.KeyNotRegistered); once an
// admin adds the key, the batch takes the current seen and closes. Pending
// events appear in Events with Pending set and a provisional verdict. When
// a revocation cut this clone's own events and a new key is registered,
// Sync re-issues their payloads as new events; only events this clone's
// journal recorded are re-issued.
//
// Seq allocation, journal writes, batch close, Fetch, and Sync are
// serialized across processes by a file lock in the store directory.
//
// # Remote
//
// Register clones a repository URL into the store's own bare repository.
// An empty repository gets a new store (genesis, then the creator's claim,
// then its ref); when another registration's genesis lands first, it fetches
// and joins instead. A repository with a store is joined (claim first; a
// lost claim is ErrIDTaken and nothing else is pushed), unless the identity
// state already holds this key for the member id, which writes no join.
//
// Sync closes the batch, pushes this clone's ref fast-forward only, and
// fetches refs/heads/tracker/* into a snapshot namespace; Fetch does the
// same without pushing. A fetched ref replaces the local copy only when the
// local copy is absent or, for a member ref, when the remote fast-forwards
// it. A deletion, a move to a non-descendant, or a moved create-once ref is
// refused with a Warning, and Restore pushes the local copy back. The
// remote copy of this clone's own ref is never taken over local commits:
// commits this clone did not write there set ForeignAdvance, and
// MoveToFreshClone moves the unpushed events to a fresh clone id.
//
// Every write moves the idle push deadline to at least now plus
// Options.PushDelay; the deadline is persisted per stream, so opening a
// store with unpushed writes restarts it, and a due deadline runs Sync.
//
// # Reading
//
// Events takes a Cursor: a per-stream watermark plus a verdict generation.
// Each call returns the events above the watermark and every event whose
// verdict or pending state changed since the generation, because a filled
// gap or a long-offline clone's events sort before events already
// delivered, and a revocation or a filled gap changes verdicts behind any
// position. Callers upsert by EventID.
package wsledger
