package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// setupMailboxTestEnv isolates WS_CACHE_HOME/WS_CONFIG_HOME/WS_RSRC_ROOT per
// test, mirroring setupNoteTestEnv/setupWorkflowManualNoteEnv, so mailbox
// storage and the workflow_manual render path never touch the real user
// home directory and self-address-surface tests can render the manual body.
func setupMailboxTestEnv(t *testing.T) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
}

// mailboxLogin binds a lead session_key to an existing root via ws.ferrule,
// unlike mintRootKey (which always mints its own fresh root): mailbox scope
// tests need several servers/keys to share ONE root so their worktree/clone
// storage paths actually coincide.
func mailboxLogin(t *testing.T, s *Server, id int, root string) string {
	t.Helper()
	key, _ := parseLoginResponse(t, callLogin(t, s, id, root, nil))
	return key
}

// TestMailboxInertByDefault verifies Decision 1's inert-by-default contract:
// with neither WS_MAILBOX nor WS_MAILBOX_AUTO set, and the session having
// neither sent nor self-looked-up, ordinary tool traffic gets no piggyback
// badge and touches no mailbox storage at all (no presence file, no
// machine secret).
func TestMailboxInertByDefault(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	key := mailboxLogin(t, s, 1, root)

	resp := callToolWithKey(t, s, 2, key, "runtime.read", nil)
	if strings.Contains(resp, "unread") {
		t.Fatalf("inert session unexpectedly got a mailbox badge: %s", resp)
	}

	machinePath, err := wsmailbox.MachinePath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(machinePath); !os.IsNotExist(err) {
		t.Fatalf("machine mailbox store was created for a fully inert session: %s", machinePath)
	}
	worktreePath, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree mailbox store was created for a fully inert session: %s", worktreePath)
	}
	secretPath, err := wsmailbox.MachineSecretPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
		t.Fatalf("machine secret was minted for a fully inert session: %s", secretPath)
	}
}

// TestMailboxUniversalSendEnvLessReturnPathAndPiggyback verifies universal
// send (Decision 11) from an env-less session, that the reply-id it
// publishes is the HMAC of its session_key (never the raw key), that the
// named-inbox owner can recv and reply to that reply-id, and that the
// env-less sender can then recv its own reply and gets a piggyback badge for
// it beforehand — even though it holds no owner pointer at all.
func TestMailboxUniversalSendEnvLessReturnPathAndPiggyback(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)

	t.Setenv(envMailbox, "")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)

	sendResp := callToolWithKey(t, serverB, 2, keyB, "mailbox.send", map[string]any{
		"to": "alice@worktree", "content": "hi from env-less",
	})
	if strings.Contains(sendResp, keyB) {
		t.Fatalf("send response leaked the raw session_key: %s", sendResp)
	}
	if !strings.Contains(sendResp, "your_reply_id: id:") {
		t.Fatalf("send response missing your_reply_id handle: %s", sendResp)
	}

	secret, err := wsmailbox.EnsureMachineSecret()
	if err != nil {
		t.Fatal(err)
	}
	wantReplyID := wsmailbox.ReplyID(secret, keyB)
	if !strings.Contains(sendResp, wantReplyID) {
		t.Fatalf("published reply-id does not match HMAC(machine_secret, session_key): resp=%s want=%s", sendResp, wantReplyID)
	}

	recvResp := callToolWithKey(t, serverA, 3, keyA, "mailbox.recv", nil)
	if !strings.Contains(recvResp, "hi from env-less") || !strings.Contains(recvResp, "id:"+wantReplyID) {
		t.Fatalf("owner recv did not surface the message with the sender's reply-id handle: %s", recvResp)
	}

	callToolWithKey(t, serverA, 4, keyA, "mailbox.send", map[string]any{
		"to": "id:" + wantReplyID, "content": "welcome back",
	})

	badgeResp := callToolWithKey(t, serverB, 5, keyB, "runtime.read", nil)
	if !strings.Contains(badgeResp, "unread 1") {
		t.Fatalf("env-less sender got no piggyback badge for its own reply-id queue: %s", badgeResp)
	}

	drainResp := callToolWithKey(t, serverB, 6, keyB, "mailbox.recv", nil)
	if !strings.Contains(drainResp, "welcome back") {
		t.Fatalf("env-less sender could not recv its own reply: %s", drainResp)
	}

	afterDrain := callToolWithKey(t, serverB, 7, keyB, "runtime.read", nil)
	if strings.Contains(afterDrain, "unread") {
		t.Fatalf("piggyback badge still present after the reply was drained: %s", afterDrain)
	}
}

// TestMailboxReplyIDStableAcrossRestartAndDiesAtNewFerrule verifies Decision
// 11/13's restart-stability contract: the reply-id is a pure function of
// (machine_secret, session_key), so it survives a simulated MCP-process
// restart (a brand new Server instance) while the session_key stays live,
// but a parent-less ferrule's freshly minted session_key computes a
// different reply-id, orphaning the old one.
func TestMailboxReplyIDStableAcrossRestartAndDiesAtNewFerrule(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	server1 := NewServer(root, "test")
	key1 := mailboxLogin(t, server1, 1, root)

	replyID1, err := server1.mailboxReplyID(key1)
	if err != nil {
		t.Fatal(err)
	}
	if !wsmailbox.IsValidReplyID(replyID1) {
		t.Fatalf("reply-id %q does not match the expected HMAC digest shape", replyID1)
	}

	// Simulated restart: a brand new process-lifetime Server, same storage
	// roots (same WS_CACHE_HOME), same still-live session_key.
	server2 := NewServer(root, "test")
	replyID2, err := server2.mailboxReplyID(key1)
	if err != nil {
		t.Fatal(err)
	}
	if replyID1 != replyID2 {
		t.Fatalf("reply-id not stable across a simulated MCP-process restart: %s vs %s", replyID1, replyID2)
	}

	// A parent-less ferrule mints a brand new session_key (revive/re-login);
	// the reply-id computed from it must differ from the old one.
	key2 := mailboxLogin(t, server2, 2, root)
	replyID3, err := server2.mailboxReplyID(key2)
	if err != nil {
		t.Fatal(err)
	}
	if replyID3 == replyID1 {
		t.Fatalf("a new parent-less ferrule's session_key produced the same reply-id as the old one")
	}

	// Prove delivery, not just digest inequality: mail addressed to the OLD
	// reply-id must land in the old reply-id's own queue and be drainable
	// only by the OLD session_key (key1) — never by the new one (key2),
	// which computes and drains a different queue entirely.
	t.Setenv(envMailbox, "")
	serverSender := NewServer(root, "test")
	keySender := mailboxLogin(t, serverSender, 3, root)
	callToolWithKey(t, serverSender, 4, keySender, "mailbox.send", map[string]any{
		"to": "id:" + replyID1, "content": "for the old reply-id",
	})

	newKeyRecv := callToolWithKey(t, server2, 5, key2, "mailbox.recv", nil)
	if strings.Contains(newKeyRecv, "for the old reply-id") {
		t.Fatalf("the new ferrule's session_key drained mail addressed to the orphaned old reply-id: %s", newKeyRecv)
	}

	oldKeyRecv := callToolWithKey(t, server2, 6, key1, "mailbox.recv", nil)
	if !strings.Contains(oldKeyRecv, "for the old reply-id") {
		t.Fatalf("the old session_key could not drain mail addressed to its own still-registered reply-id: %s", oldKeyRecv)
	}
}

// TestMailboxEnvelopeStampingSameTier verifies Decision 12: mail from a
// slug'd sender to a recipient sharing that slug's tier arrives stamped
// from: <slug>, and recv surfaces that handle.
func TestMailboxEnvelopeStampingSameTier(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)

	t.Setenv(envMailbox, "bob@worktree")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)

	sendResp := callToolWithKey(t, serverA, 2, keyA, "mailbox.send", map[string]any{
		"to": "bob@worktree", "content": "same tier",
	})
	if !strings.Contains(sendResp, "from: alice@worktree") {
		t.Fatalf("same-tier send not stamped from: <slug>: %s", sendResp)
	}

	recvResp := callToolWithKey(t, serverB, 3, keyB, "mailbox.recv", nil)
	if !strings.Contains(recvResp, "alice@worktree: same tier") {
		t.Fatalf("recv did not surface the sender's durable from: handle: %s", recvResp)
	}
}

// TestMailboxAutoIdentityCollisionFreeAndExplicitOverride verifies Decision
// 10: WS_MAILBOX_AUTO mints a random 3-word stem, two independent
// resolutions never collide, and an explicit WS_MAILBOX overrides
// WS_MAILBOX_AUTO when both are set.
func TestMailboxAutoIdentityCollisionFreeAndExplicitOverride(t *testing.T) {
	t.Setenv(envMailbox, "")
	t.Setenv(envMailboxAuto, "worktree")

	id1 := computeMailboxIdentity()
	id2 := computeMailboxIdentity()
	if !id1.Active || !id2.Active || !id1.Auto || !id2.Auto {
		t.Fatalf("WS_MAILBOX_AUTO identity not active/auto: %#v %#v", id1, id2)
	}
	if id1.Scope != wsmailbox.ScopeWorktree || id2.Scope != wsmailbox.ScopeWorktree {
		t.Fatalf("auto identity scope mismatch: %#v %#v", id1, id2)
	}
	if id1.Name == id2.Name {
		t.Fatalf("two independent WS_MAILBOX_AUTO resolutions minted the same stem: %s", id1.Name)
	}

	t.Setenv(envMailbox, "carol@machine")
	id3 := computeMailboxIdentity()
	if id3.Auto || id3.Name != "carol" || id3.Scope != wsmailbox.ScopeMachine {
		t.Fatalf("explicit WS_MAILBOX did not override WS_MAILBOX_AUTO when both are set: %#v", id3)
	}
}

// TestMailboxSelfAddressSurface verifies Decision 10's self-address surface:
// the workflow_manual ambient block exposes the caller's own resolved
// <stem>@<scope>, mailbox.lookup_peers lists it as a self entry, an
// env-less caller's self entry is its own reply-id (not an unreachable
// address), and a peer's presence record carries descriptive metadata.
func TestMailboxSelfAddressSurface(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailboxAuto, "worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)

	identity := serverA.mailboxIdentityResolved()
	if !identity.Active || !identity.Auto {
		t.Fatalf("WS_MAILBOX_AUTO identity not active: %#v", identity)
	}

	manualResp := callToolWithKey(t, serverA, 2, keyA, "workflow_manual", nil)
	if !strings.Contains(manualResp, "Mailbox address:") || !strings.Contains(manualResp, identity.Name+"@worktree") {
		t.Fatalf("workflow_manual ambient block missing the resolved self mailbox address: %s", manualResp)
	}

	t.Setenv(envMailboxAuto, "")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)

	lookupResp := callToolWithKey(t, serverB, 2, keyB, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if !strings.Contains(lookupResp, identity.Name+"@worktree") {
		t.Fatalf("lookup_peers did not list the auto-identity peer: %s", lookupResp)
	}
	if !strings.Contains(lookupResp, "cwd=") || !strings.Contains(lookupResp, "started_at=") {
		t.Fatalf("lookup_peers missing peer descriptive metadata (cwd/started_at): %s", lookupResp)
	}
	if !strings.Contains(lookupResp, "self: ") || !strings.Contains(lookupResp, "reply-id only") {
		t.Fatalf("env-less caller's lookup_peers self entry is not its own reply-id: %s", lookupResp)
	}
}

// TestMailboxOwnerGate verifies Decision 3: only the caller_session_key ==
// owner_key gate is authorized against the named inbox (recv, and claiming
// the slug's from: identity when sending, and the piggyback badge); a
// non-owner session_key sharing the same process is inert with respect to
// that named inbox, though it may still send/recv on its own reply-id.
func TestMailboxOwnerGate(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)

	// A parent-carrying (delegate) mint sharing serverA's process/identity,
	// but never the owner.
	delegateResp := callLogin(t, serverA, 2, root, map[string]any{
		"parent_session_key": keyA,
		"capability":         "delegate",
	})
	keyA2, _ := parseLoginResponse(t, delegateResp)

	t.Setenv(envMailbox, "bob@worktree")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)

	// A third party's mail lands in alice's named inbox before either A key
	// touches it.
	callToolWithKey(t, serverB, 2, keyB, "mailbox.send", map[string]any{
		"to": "alice@worktree", "content": "for the owner",
	})

	// The non-owner delegate key must not be able to claim alice@worktree's
	// from: identity when sending.
	delegateSend := callToolWithKey(t, serverA, 3, keyA2, "mailbox.send", map[string]any{
		"to": "bob@worktree", "content": "from delegate",
	})
	if strings.Contains(delegateSend, "from: alice@worktree") || !strings.Contains(delegateSend, "reply_to: id:") {
		t.Fatalf("non-owner session_key was able to claim the named inbox's from: identity: %s", delegateSend)
	}

	// The non-owner delegate key must not be able to drain the named inbox.
	delegateRecv := callToolWithKey(t, serverA, 4, keyA2, "mailbox.recv", nil)
	if strings.Contains(delegateRecv, "for the owner") {
		t.Fatalf("non-owner session_key drained the named inbox: %s", delegateRecv)
	}

	// Nor does it get a piggyback badge for the named inbox's unread mail.
	delegateBadge := callToolWithKey(t, serverA, 5, keyA2, "runtime.read", nil)
	if strings.Contains(delegateBadge, "unread") {
		t.Fatalf("non-owner session_key got a piggyback badge for the named inbox: %s", delegateBadge)
	}

	// The owner key, by contrast, gets the badge and can drain the mail.
	ownerBadge := callToolWithKey(t, serverA, 6, keyA, "runtime.read", nil)
	if !strings.Contains(ownerBadge, "unread 1") {
		t.Fatalf("owner session_key got no piggyback badge for its own named inbox: %s", ownerBadge)
	}
	ownerRecv := callToolWithKey(t, serverA, 7, keyA, "mailbox.recv", nil)
	if !strings.Contains(ownerRecv, "for the owner") {
		t.Fatalf("owner session_key could not drain its own named inbox: %s", ownerRecv)
	}
}

// TestMailboxFerruleRebindPreservesAddressAndQueuedMail verifies Decision 3:
// a parent-carrying ferrule never rebinds the owner pointer; a parent-less
// ferrule does, without re-minting the address; and mail queued before the
// rebind stays drainable by the new owner while the stale old owner key
// loses access.
func TestMailboxFerruleRebindPreservesAddressAndQueuedMail(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA1 := mailboxLogin(t, serverA, 1, root)

	// A parent-carrying ferrule must NOT rebind ownership.
	callLogin(t, serverA, 2, root, map[string]any{"parent_session_key": keyA1})

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Presence["alice"].Owner != keyA1 {
		t.Fatalf("a parent-carrying ferrule rebound ownership: owner = %q, want unchanged %q", store.Presence["alice"].Owner, keyA1)
	}

	// Queue mail for alice while keyA1 still owns the name.
	t.Setenv(envMailbox, "")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)
	callToolWithKey(t, serverB, 2, keyB, "mailbox.send", map[string]any{
		"to": "alice@worktree", "content": "queued before rebind",
	})

	// A parent-less ferrule (revive) rebinds the owner pointer; the address
	// itself must not change.
	keyA2 := mailboxLogin(t, serverA, 3, root)
	if keyA2 == keyA1 {
		t.Fatalf("expected a fresh session_key from the parent-less ferrule")
	}

	identity := serverA.mailboxIdentityResolved()
	if identity.Name != "alice" || identity.Scope != wsmailbox.ScopeWorktree {
		t.Fatalf("mailbox address changed across a ferrule owner rebind: %#v", identity)
	}

	store2, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if store2.Presence["alice"].Owner != keyA2 {
		t.Fatalf("parent-less ferrule did not rebind the owner pointer: owner = %q, want %q", store2.Presence["alice"].Owner, keyA2)
	}

	recvResp := callToolWithKey(t, serverA, 4, keyA2, "mailbox.recv", nil)
	if !strings.Contains(recvResp, "queued before rebind") {
		t.Fatalf("mail queued before an owner-key rebind was lost: %s", recvResp)
	}

	// The named inbox is now empty (just drained by the new owner). Queue a
	// SECOND, post-rebind message and check the STALE key FIRST, before the
	// legitimate new owner ever touches this second message — draining with
	// the new owner first would empty the queue and make the stale-key
	// check pass vacuously regardless of whether the gate actually works.
	callToolWithKey(t, serverB, 6, keyB, "mailbox.send", map[string]any{
		"to": "alice@worktree", "content": "queued after rebind",
	})

	oldOwnerResp := callToolWithKey(t, serverA, 5, keyA1, "mailbox.recv", nil)
	if strings.Contains(oldOwnerResp, "queued before rebind") || strings.Contains(oldOwnerResp, "queued after rebind") {
		t.Fatalf("stale pre-rebind owner key could still drain the named inbox after rebind: %s", oldOwnerResp)
	}

	newOwnerResp := callToolWithKey(t, serverA, 7, keyA2, "mailbox.recv", nil)
	if !strings.Contains(newOwnerResp, "queued after rebind") {
		t.Fatalf("new owner key could not drain mail queued after the rebind: %s", newOwnerResp)
	}
}

// TestMailboxTieredSlugDurabilityAcrossScopeAndFerrule verifies Decision 13:
// a machine-scope slug is reachable (and stamped from:) from a cross-scope
// peer and keeps working across that peer's own ferrule churn; a
// worktree-scope slug reaches a cross-scope peer only as a reply-to: id:
// handle.
func TestMailboxTieredSlugDurabilityAcrossScopeAndFerrule(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "carol@machine")
	serverC := NewServer(root, "test")
	keyC := mailboxLogin(t, serverC, 1, root)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA1 := mailboxLogin(t, serverA, 1, root)

	sendFromCarol := callToolWithKey(t, serverC, 2, keyC, "mailbox.send", map[string]any{
		"to": "alice@worktree", "content": "from machine tier",
	})
	if !strings.Contains(sendFromCarol, "from: carol@machine") {
		t.Fatalf("machine-scope sender not stamped from: <slug> across scopes: %s", sendFromCarol)
	}

	sendFromAlice := callToolWithKey(t, serverA, 3, keyA1, "mailbox.send", map[string]any{
		"to": "carol@machine", "content": "from worktree tier",
	})
	if strings.Contains(sendFromAlice, "from: alice@worktree") || !strings.Contains(sendFromAlice, "reply_to: id:") {
		t.Fatalf("worktree-scope sender wrongly claimed a durable from: handle across scopes: %s", sendFromAlice)
	}

	// alice's own ferrule churns; carol's machine-scope address and
	// ownership must be completely unaffected.
	keyA2 := mailboxLogin(t, serverA, 4, root)
	sendFromAlice2 := callToolWithKey(t, serverA, 5, keyA2, "mailbox.send", map[string]any{
		"to": "carol@machine", "content": "after churn",
	})
	if !strings.Contains(sendFromAlice2, "sent to carol@machine") {
		t.Fatalf("carol's machine-scope slug stopped working after an unrelated peer's ferrule churn: %s", sendFromAlice2)
	}

	carolPath, err := wsmailbox.MachinePath()
	if err != nil {
		t.Fatal(err)
	}
	carolStore, err := wsmailbox.Load(carolPath)
	if err != nil {
		t.Fatal(err)
	}
	if carolStore.Presence["carol"].Owner != keyC {
		t.Fatalf("carol's ownership was disturbed by an unrelated peer's ferrule churn: %#v", carolStore.Presence["carol"])
	}
}

// TestMailboxLookupPeersFiltersDeadPeersAndSurfacesConflict verifies
// Decision 2: lookup_peers hides a peer once its heartbeat is older than
// the liveness threshold, and a duplicate live-name registration never
// clobbers the first process's presence record but flags it Conflict,
// surfaced through lookup_peers.
func TestMailboxLookupPeersFiltersDeadPeersAndSurfacesConflict(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "dora@worktree")
	serverD := NewServer(root, "test")
	mailboxLogin(t, serverD, 1, root)

	t.Setenv(envMailbox, "")
	serverLooker := NewServer(root, "test")
	keyLooker := mailboxLogin(t, serverLooker, 1, root)

	liveResp := callToolWithKey(t, serverLooker, 2, keyLooker, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if !strings.Contains(liveResp, "dora@worktree") {
		t.Fatalf("lookup_peers did not list a live peer: %s", liveResp)
	}

	original := mailboxNow
	t.Cleanup(func() { mailboxNow = original })
	mailboxNow = func() time.Time { return original().Add(mailboxLivenessThreshold + time.Minute) }

	deadResp := callToolWithKey(t, serverLooker, 3, keyLooker, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if strings.Contains(deadResp, "dora@worktree") {
		t.Fatalf("lookup_peers listed a peer with no heartbeat inside the liveness window: %s", deadResp)
	}
	mailboxNow = original

	// Duplicate-live-name conflict: fabricate an existing live presence
	// record under a different, but genuinely still-running, PID (the test
	// binary's own parent process — guaranteed alive for the test's
	// duration, unlike an arbitrary os.Getpid()+1 which mailboxPresenceLive's
	// real PID-liveness check would otherwise treat as already dead), then
	// self-register over it.
	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	otherLivePID := os.Getppid()
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		store.Presence["eve"] = wsmailbox.Presence{
			Name: "eve", Scope: wsmailbox.ScopeWorktree, PID: otherLivePID, LastSeen: mailboxNowString(),
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envMailbox, "eve@worktree")
	serverE := NewServer(root, "test")
	serverE.ensureMailboxRegistered(root)

	afterConflict, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !afterConflict.Presence["eve"].Conflict {
		t.Fatalf("duplicate live-name registration did not flag Conflict on the existing record: %#v", afterConflict.Presence["eve"])
	}
	if afterConflict.Presence["eve"].PID != otherLivePID {
		t.Fatalf("duplicate live-name registration clobbered the first process's presence record: %#v", afterConflict.Presence["eve"])
	}

	conflictResp := callToolWithKey(t, serverLooker, 4, keyLooker, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if !strings.Contains(conflictResp, "CONFLICT") {
		t.Fatalf("lookup_peers did not surface the duplicate-live-name conflict: %s", conflictResp)
	}
}

// TestRebindMailboxOwnerRefusesLiveDifferentPIDHolder covers the
// security-sensitive in-lock refusal branch of rebindMailboxOwnerAtFerrule
// (Critical: owner-rebind ignoring conflict state): when a DIFFERENT, genuinely
// live process legitimately holds the presence record at the moment of a
// parent-less ferrule login — a race that resolved against this process AFTER
// its own registration — the rebind must refuse to steal the ownership pointer
// rather than hijack the live holder's identity.
//
// The existing conflict tests only exercise the registration-time flagging path
// (a duplicate detected at self-register, via s.mailbox.conflict). This drives
// the distinct post-registration re-verify under the write lock: registration
// succeeds cleanly (empty store, so conflict stays false), then a live different
// PID takes over the record before rebind, so the rebind reaches the in-lock
// `p.PID != pid && mailboxPresenceLive` guard rather than the conflict-flag
// early return.
func TestRebindMailboxOwnerRefusesLiveDifferentPIDHolder(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "frank@worktree")
	s := NewServer(root, "test")
	// Register under this process's own PID against an empty store: no conflict
	// is flagged, so registerOnce is spent and the later rebind is routed past
	// the conflict-flag early return into the in-lock re-verify branch.
	s.ensureMailboxRegistered(root)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	// A different, genuinely-live PID (the test binary's own parent, guaranteed
	// alive for the test's duration — the same live-PID choice the existing
	// conflict fixture makes) takes over the presence record after our own
	// registration.
	otherLivePID := os.Getppid()
	if otherLivePID == os.Getpid() || otherLivePID <= 0 {
		t.Skipf("cannot obtain a distinct live parent PID (ppid=%d)", otherLivePID)
	}
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p := store.Presence["frank"]
		p.PID = otherLivePID
		p.Owner = ""
		p.LastSeen = mailboxNowString()
		store.Presence["frank"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Parent-less ferrule login: the only path that ever rebinds ownership.
	s.rebindMailboxOwnerAtFerrule("intruder-session-key", "", roleLead, root)

	after, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Presence["frank"].Owner; got != "" {
		t.Fatalf("rebind hijacked ownership of a name held by a live different PID: Owner = %q, want empty", got)
	}
	if got := after.Presence["frank"].PID; got != otherLivePID {
		t.Fatalf("rebind clobbered the live holder's presence record: PID = %d, want the live holder's %d", got, otherLivePID)
	}
}

// TestMailboxFerruleRebindRequiresLeadCapability verifies that only a
// parent-less lead-capability ferrule rebinds the named-inbox owner: a
// parent-less mint whose capability is leaf or delegate (a child launched
// without its lead's key, sharing the lead's WS_MAILBOX identity) mints its
// key but leaves the owner pointer alone, while an explicit "lead" capability
// still rebinds, as an omitted capability does (see
// TestMailboxFerruleRebindPreservesAddressAndQueuedMail).
func TestMailboxFerruleRebindRequiresLeadCapability(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "grace@worktree")
	s := NewServer(root, "test")
	leadKey := mailboxLogin(t, s, 1, root)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	owner := func() string {
		t.Helper()
		store, err := wsmailbox.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return store.Presence["grace"].Owner
	}
	if got := owner(); got != leadKey {
		t.Fatalf("parent-less ferrule with omitted capability did not bind: owner = %q, want %q", got, leadKey)
	}

	for i, capability := range []string{"leaf", "delegate"} {
		childKey, _ := parseLoginResponse(t, callLogin(t, s, 2+i, root, map[string]any{"capability": capability}))
		if childKey == "" || childKey == leadKey {
			t.Fatalf("parent-less %s ferrule did not mint its own key: %q", capability, childKey)
		}
		if got := owner(); got != leadKey {
			t.Fatalf("parent-less %s ferrule rebound ownership: owner = %q, want unchanged %q", capability, got, leadKey)
		}
	}

	reloginKey, _ := parseLoginResponse(t, callLogin(t, s, 4, root, map[string]any{"capability": "lead"}))
	if got := owner(); got != reloginKey {
		t.Fatalf("parent-less lead ferrule did not rebind: owner = %q, want %q", got, reloginKey)
	}
}

// TestRebindMailboxOwnerReclaimsDeadPIDRecord verifies that a lead rebind
// over a presence record left by a different, exited process (a child that
// overwrote the record and then quit) takes the record's PID for this
// process, so the ordinary heartbeat refresh — which only touches a record
// carrying this process's PID — keeps the reclaimed inbox fresh.
func TestRebindMailboxOwnerReclaimsDeadPIDRecord(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "heidi@worktree")
	s := NewServer(root, "test")
	s.ensureMailboxRegistered(root)

	deadPID := exitedChildPID(t)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	stale := mailboxNow().Add(-(mailboxLivenessThreshold + time.Minute)).Format(time.RFC3339)
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p := store.Presence["heidi"]
		p.PID = deadPID
		p.Owner = "departed-child-key"
		p.LastSeen = stale
		store.Presence["heidi"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	s.rebindMailboxOwnerAtFerrule("lead-key", "", roleLead, root)

	after, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Presence["heidi"]; got.Owner != "lead-key" || got.PID != os.Getpid() {
		t.Fatalf("lead rebind over a dead-PID record: owner = %q, PID = %d; want owner %q, PID %d", got.Owner, got.PID, "lead-key", os.Getpid())
	}

	later := advanceMailboxClock(t, mailboxHeartbeatThrottle+time.Minute)
	s.refreshMailboxPresenceHeartbeat(root)

	refreshed, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := refreshed.Presence["heidi"].LastSeen, later.Format(time.RFC3339); got != want {
		t.Fatalf("heartbeat refresh skipped the reclaimed record: LastSeen = %q, want %q", got, want)
	}
}

// TestRebindMailboxOwnerRebuildsReclaimedRecord verifies a lead rebind that
// reclaims a dead-PID record rebuilds it as this process's record: the
// previous holder's Conflict flag and descriptive fields would otherwise make
// lookup_peers report a false conflict and the dead process's harness/cwd on
// the live owner's record.
func TestRebindMailboxOwnerRebuildsReclaimedRecord(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "ivan@worktree")
	s := NewServer(root, "test")
	s.observeHarness("test", "pi")
	s.ensureMailboxRegistered(root)

	deadPID := exitedChildPID(t)
	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	stale := mailboxNow().Add(-(mailboxLivenessThreshold + time.Minute)).Format(time.RFC3339)
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		store.Presence["ivan"] = wsmailbox.Presence{
			Name: "ivan", Scope: wsmailbox.ScopeWorktree,
			Harness: "codex", Cwd: "/elsewhere", StartedAt: stale, LastSeen: stale,
			PID: deadPID, Owner: "departed-child-key", Auto: true, Conflict: true,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	rebindAt := advanceMailboxClock(t, time.Minute).Format(time.RFC3339)
	s.rebindMailboxOwnerAtFerrule("lead-key", "", roleLead, root)

	got := loadPresence(t, path, "ivan")
	want := wsmailbox.Presence{
		Name: "ivan", Scope: wsmailbox.ScopeWorktree,
		Harness: "pi", Cwd: root, StartedAt: rebindAt, LastSeen: rebindAt,
		PID: os.Getpid(), Owner: "lead-key",
	}
	if got != want {
		t.Fatalf("reclaimed record not rebuilt as this process's record:\ngot  %#v\nwant %#v", got, want)
	}
}

// TestMailboxToolBoundaryErrors verifies each of the three MCP handlers'
// argument/session validation surfaces a clear error rather than a panic,
// a silently-wrong result, or a generic message indistinguishable from an
// unrelated failure: an unknown session_key, a malformed "to" address, an
// invalid lookup_peers scope, and a missing required "content"/"to"/"scope"
// argument.
func TestMailboxToolBoundaryErrors(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "")
	s := NewServer(root, "test")
	key := mailboxLogin(t, s, 1, root)

	for _, tool := range []string{"mailbox.send", "mailbox.recv", "mailbox.lookup_peers"} {
		resp := callToolWithKey(t, s, 2, "not-a-real-session-key", tool, map[string]any{
			"to": "x@worktree", "content": "hi", "scope": "worktree",
		})
		if !strings.Contains(resp, "unknown_session") {
			t.Fatalf("%s with an unknown session_key did not report unknown_session: %s", tool, resp)
		}
	}

	sendMalformedTo := callToolWithKey(t, s, 3, key, "mailbox.send", map[string]any{
		"to": "not a valid address", "content": "hi",
	})
	if !strings.Contains(sendMalformedTo, "mailbox.send") || !strings.Contains(sendMalformedTo, "name@scope") {
		t.Fatalf("mailbox.send with a malformed to did not report the specific address-parse failure: %s", sendMalformedTo)
	}

	sendMissingContent := callToolWithKey(t, s, 4, key, "mailbox.send", map[string]any{
		"to": "x@worktree",
	})
	if !strings.Contains(sendMissingContent, "content") {
		t.Fatalf("mailbox.send with missing content did not name the missing argument: %s", sendMissingContent)
	}

	sendMissingTo := callToolWithKey(t, s, 5, key, "mailbox.send", map[string]any{
		"content": "hi",
	})
	if !strings.Contains(sendMissingTo, "to") {
		t.Fatalf("mailbox.send with missing to did not name the missing argument: %s", sendMissingTo)
	}

	lookupInvalidScope := callToolWithKey(t, s, 6, key, "mailbox.lookup_peers", map[string]any{"scope": "planet"})
	if !strings.Contains(lookupInvalidScope, "scope") {
		t.Fatalf("mailbox.lookup_peers with an invalid scope did not name the scope problem: %s", lookupInvalidScope)
	}

	lookupMissingScope := callToolWithKey(t, s, 7, key, "mailbox.lookup_peers", nil)
	if !strings.Contains(lookupMissingScope, "scope") {
		t.Fatalf("mailbox.lookup_peers with a missing scope did not name the missing argument: %s", lookupMissingScope)
	}
}

// TestMailboxPiggybackSuppressedForJSONFormat verifies the central piggyback
// wrapper never concatenates badge text onto a format:"json" response: doing
// so would corrupt the marshalled JSON for any caller parsing it
// structurally. A text-format call to the same underlying tool, from the
// same session with the same unread mail, still gets the badge.
func TestMailboxPiggybackSuppressedForJSONFormat(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)

	t.Setenv(envMailbox, "")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)
	callToolWithKey(t, serverB, 2, keyB, "mailbox.send", map[string]any{
		"to": "alice@worktree", "content": "for the json check",
	})

	jsonResp := callToolWithKey(t, serverA, 3, keyA, "runtime.read", map[string]any{"format": "json"})
	if strings.Contains(jsonResp, "unread") {
		t.Fatalf("format:\"json\" response was corrupted with a piggyback badge: %s", jsonResp)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonResp), &parsed); err != nil {
		t.Fatalf("format:\"json\" response with unread mail pending did not parse as JSON: %v\nresp=%s", err, jsonResp)
	}

	textResp := callToolWithKey(t, serverA, 4, keyA, "runtime.read", nil)
	if !strings.Contains(textResp, "unread 1") {
		t.Fatalf("text-format response for the same session lost its piggyback badge: %s", textResp)
	}
}

// TestMailboxLookupPeersExcludesSelfFromPeers is the regression test for
// 260913-bug-mailbox-lookup-peers-self-leak: the caller's own bound named
// inbox must appear only under self, never inside peers[], even though it
// lives in the very same Presence map every other peer is enumerated from.
// A genuinely distinct second live holder of a different name — including
// one flagged Conflict — must still surface as a peer: the fix is a
// self-only exclusion, not a "hide everything in my scope" shortcut.
func TestMailboxLookupPeersExcludesSelfFromPeers(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	s := NewServer(root, "test")
	key := mailboxLogin(t, s, 1, root)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	// A distinct, genuinely-live second holder under a different name (the
	// test binary's own parent PID, guaranteed alive for the test's
	// duration, matching the existing conflict fixture's PID choice), with
	// the conflict marker already set, so the fix cannot suppress a real
	// peer or its conflict marker along with the self entry.
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		store.Presence["bob"] = wsmailbox.Presence{
			Name: "bob", Scope: wsmailbox.ScopeWorktree, PID: os.Getppid(),
			LastSeen: mailboxNowString(), Conflict: true,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	resp := callToolWithKey(t, s, 2, key, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if !strings.Contains(resp, "self: alice@worktree") {
		t.Fatalf("owner session's self entry is not its own address: %s", resp)
	}
	for _, line := range strings.Split(resp, "\n") {
		if strings.HasPrefix(line, "alice@worktree") {
			t.Fatalf("lookup_peers leaked the caller's own bound named inbox into peers[]: %s", resp)
		}
	}
	if !strings.Contains(resp, "bob@worktree") || !strings.Contains(resp, "CONFLICT") {
		t.Fatalf("lookup_peers suppressed a genuinely distinct peer or its conflict marker: %s", resp)
	}
}

// TestMailboxLookupPeersSurfacesSelfConflict is the regression test for the
// self-leak hotfix's follow-up repair: Presence is name-keyed, so a
// same-name contest (a second live process also claiming this owner's own
// WS_MAILBOX name) flags Conflict on the OWNER's own record rather than
// creating a second entry (mirroring ensureMailboxRegistered's real
// duplicate-registration path). The self-exclusion fix must not swallow
// that record's Conflict signal along with dropping it from peers[] — it
// must relocate the signal into self, since the owner is the party that
// most needs the warning that another process claims its name.
func TestMailboxLookupPeersSurfacesSelfConflict(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	s := NewServer(root, "test")
	key := mailboxLogin(t, s, 1, root)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	// Flag Conflict directly on the caller's own already-registered record,
	// exactly as ensureMailboxRegistered would when a second live process
	// contests the same name — no second Presence entry is ever created for
	// a name-keyed store.
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p := store.Presence["alice"]
		p.Conflict = true
		store.Presence["alice"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	resp := callToolWithKey(t, s, 2, key, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if !strings.Contains(resp, "self: alice@worktree") || !strings.Contains(resp, "CONFLICT") {
		t.Fatalf("owner session's self entry did not surface its own name's conflict: %s", resp)
	}
	for _, line := range strings.Split(resp, "\n") {
		if strings.HasPrefix(line, "alice@worktree") {
			t.Fatalf("owner's own conflicted entry still leaked into peers[]: %s", resp)
		}
	}

	jsonResp := callToolWithKey(t, s, 3, key, "mailbox.lookup_peers", map[string]any{"scope": "worktree", "format": "json"})
	var parsed struct {
		Self map[string]any `json:"self"`
	}
	if err := json.Unmarshal([]byte(jsonResp), &parsed); err != nil {
		t.Fatalf("json lookup_peers response did not parse: %v\nresp=%s", err, jsonResp)
	}
	if v, _ := parsed.Self["conflict"].(bool); !v {
		t.Fatalf("json self object did not carry conflict:true: %s", jsonResp)
	}
	if addr, _ := parsed.Self["address"].(string); addr != "alice@worktree" {
		t.Fatalf("json self object lost its address alongside the conflict flag: %s", jsonResp)
	}
}

// TestMailboxLookupPeersSelfAddressGatedByOwnership verifies the self-address
// surface never hands a non-owner session an address it cannot actually
// recv from: a parent-carrying delegate session sharing the owner's process
// identity gets a reply-id self entry, exactly like an env-less caller,
// while the owner itself still gets its address.
func TestMailboxLookupPeersSelfAddressGatedByOwnership(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)

	delegateResp := callLogin(t, serverA, 2, root, map[string]any{
		"parent_session_key": keyA,
		"capability":         "delegate",
	})
	keyA2, _ := parseLoginResponse(t, delegateResp)

	ownerLookup := callToolWithKey(t, serverA, 3, keyA, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if !strings.Contains(ownerLookup, "self: alice@worktree") {
		t.Fatalf("owner session's lookup_peers self entry is not its own address: %s", ownerLookup)
	}

	delegateLookup := callToolWithKey(t, serverA, 4, keyA2, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if strings.Contains(delegateLookup, "self: alice@worktree") {
		t.Fatalf("non-owner delegate session was handed the owner's unusable named-inbox address: %s", delegateLookup)
	}
	if !strings.Contains(delegateLookup, "reply-id only") {
		t.Fatalf("non-owner delegate session's self entry is not a reply-id fallback: %s", delegateLookup)
	}
}

// TestMailboxWaitCommandRenderVariable verifies Phase 2's render-time
// {{.MailboxWaitCommand}} injection across the four enumerated scenarios: a
// key + explicit WS_MAILBOX, a key + WS_MAILBOX_AUTO (the emitted --slug must
// be the SERVER-registered minted stem, not an env re-derivation), a key +
// env-less session (reply-id-only: --session-key alone), and no key (generic
// degrade). It asserts both the resolver output and that the playbook.read
// dispatch actually substitutes the value into the shipped mailbox body.
func TestMailboxWaitCommandRenderVariable(t *testing.T) {
	// Scenario 1: key + explicit WS_MAILBOX → concrete command with the exact
	// registered "name@scope" slug.
	t.Run("explicit_slug", func(t *testing.T) {
		setupMailboxTestEnv(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv(envMailbox, "alice@worktree")
		s := NewServer(root, "test")
		key := mailboxLogin(t, s, 1, root)

		cmd := mailboxWaitCommandVar(s, key)
		if cmd == mailboxWaitCommandGeneric {
			t.Fatalf("owned explicit-slug session got the generic fallback: %s", cmd)
		}
		if !strings.Contains(cmd, "mailbox wait --session-key "+key) {
			t.Fatalf("command missing the resolved --session-key: %s", cmd)
		}
		if !strings.Contains(cmd, "--slug alice@worktree") {
			t.Fatalf("command missing the registered --slug: %s", cmd)
		}

		// The playbook.read dispatch substitutes it into the shipped body.
		body := callToolWithKey(t, s, 2, key, "playbook.read", map[string]any{"name": "lead-use-mailbox"})
		if !strings.Contains(body, "--slug alice@worktree") {
			t.Fatalf("playbook.read body did not carry the concrete wait command: %s", body)
		}
		if strings.Contains(body, "{{.MailboxWaitCommand}}") {
			t.Fatalf("playbook.read body left the render variable unsubstituted: %s", body)
		}
	})

	// Scenario 2: key + WS_MAILBOX_AUTO → the --slug is the server-minted stem
	// (AUTO-safe), never an env re-derivation. WS_MAILBOX_AUTO carries only the
	// scope, so a correct implementation cannot reconstruct the address from the
	// environment; it must read the registered identity.
	t.Run("auto_slug_is_registered_not_env", func(t *testing.T) {
		setupMailboxTestEnv(t)
		root := t.TempDir()
		initGit(t, root)
		// Clear WS_MAILBOX so computeMailboxIdentity (which checks WS_MAILBOX
		// first) cannot be routed down the explicit branch by an ambient value
		// in the run environment.
		t.Setenv(envMailbox, "")
		t.Setenv(envMailboxAuto, "worktree")
		s := NewServer(root, "test")
		key := mailboxLogin(t, s, 1, root)

		identity := s.mailboxIdentityResolved()
		if !identity.Active || !identity.Auto {
			t.Fatalf("WS_MAILBOX_AUTO identity not active/auto: %#v", identity)
		}
		wantSlug := "--slug " + identity.Name + "@worktree"
		cmd := mailboxWaitCommandVar(s, key)
		if !strings.Contains(cmd, wantSlug) {
			t.Fatalf("AUTO command did not emit the registered self address %q: %s", wantSlug, cmd)
		}
		// End-to-end: the playbook.read substitution carries the registered slug.
		body := callToolWithKey(t, s, 2, key, "playbook.read", map[string]any{"name": "lead-use-mailbox"})
		if !strings.Contains(body, wantSlug) {
			t.Fatalf("playbook.read body did not carry the AUTO registered slug: %s", body)
		}
		if strings.Contains(body, "{{.MailboxWaitCommand}}") {
			t.Fatalf("playbook.read body left the render variable unsubstituted: %s", body)
		}
	})

	// Scenario 3: key + env-less session → no registered address, so the
	// reply-id-only form (--session-key alone, no --slug).
	t.Run("env_less_reply_id_only", func(t *testing.T) {
		setupMailboxTestEnv(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv(envMailbox, "")
		t.Setenv(envMailboxAuto, "")
		s := NewServer(root, "test")
		key := mailboxLogin(t, s, 1, root)

		cmd := mailboxWaitCommandVar(s, key)
		if !strings.Contains(cmd, "mailbox wait --session-key "+key) {
			t.Fatalf("env-less command missing the resolved --session-key: %s", cmd)
		}
		if strings.Contains(cmd, "--slug") {
			t.Fatalf("env-less session emitted a --slug it cannot own: %s", cmd)
		}
		// End-to-end: the substituted body's command ends at --session-key <key>
		// (backtick-terminated), proving no --slug was appended.
		body := callToolWithKey(t, s, 2, key, "playbook.read", map[string]any{"name": "lead-use-mailbox"})
		if !strings.Contains(body, "mailbox wait --session-key "+key+"`") {
			t.Fatalf("playbook.read body did not carry the reply-id-only command: %s", body)
		}
	})

	// Scenario 3b: an owned identity but a NON-OWNER session (a parent-carrying
	// delegate sharing the process) → reply-id-only, never the owner's --slug.
	// This exercises the isOwner gate in mailboxWaitCommandVar directly: without
	// it a non-owner would be handed a --slug it cannot recv from.
	t.Run("non_owner_delegate_reply_id_only", func(t *testing.T) {
		setupMailboxTestEnv(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv(envMailbox, "alice@worktree")
		s := NewServer(root, "test")
		keyOwner := mailboxLogin(t, s, 1, root)

		delegateResp := callLogin(t, s, 2, root, map[string]any{
			"parent_session_key": keyOwner,
			"capability":         "delegate",
		})
		keyDelegate, _ := parseLoginResponse(t, delegateResp)

		// The owner still gets the concrete --slug.
		if got := mailboxWaitCommandVar(s, keyOwner); !strings.Contains(got, "--slug alice@worktree") {
			t.Fatalf("owner session lost its registered --slug: %s", got)
		}
		// The non-owner delegate, sharing the same process identity, must not.
		gotDelegate := mailboxWaitCommandVar(s, keyDelegate)
		if !strings.Contains(gotDelegate, "mailbox wait --session-key "+keyDelegate) {
			t.Fatalf("delegate command missing its own --session-key: %s", gotDelegate)
		}
		if strings.Contains(gotDelegate, "--slug") {
			t.Fatalf("non-owner delegate was handed the owner's --slug: %s", gotDelegate)
		}
	})

	// Scenario 4: no key (fresh session) → today's generic guidance, never a
	// broken command.
	t.Run("no_key_generic_degrade", func(t *testing.T) {
		setupMailboxTestEnv(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv(envMailbox, "alice@worktree")
		s := NewServer(root, "test")
		// A syntactically valid but unregistered key also degrades (no live
		// record), same as an empty key.
		if got := mailboxWaitCommandVar(s, ""); got != mailboxWaitCommandGeneric {
			t.Fatalf("empty key did not degrade to the generic command: %s", got)
		}
		if got := mailboxWaitCommandVar(s, "not-a-real-session-key"); got != mailboxWaitCommandGeneric {
			t.Fatalf("unresolvable key did not degrade to the generic command: %s", got)
		}
		// End-to-end: an unregistered key still renders a body carrying today's
		// generic guidance (the `[--timeout <duration>]` marker appears only in
		// the generic fallback, never in a concrete command), never a broken one.
		body := callToolWithKey(t, s, 2, "not-a-real-session-key", "playbook.read", map[string]any{"name": "lead-use-mailbox"})
		if !strings.Contains(body, mailboxWaitCommandGeneric) {
			t.Fatalf("playbook.read body for an unresolvable key did not degrade to the generic command: %s", body)
		}
		if strings.Contains(body, "{{.MailboxWaitCommand}}") {
			t.Fatalf("playbook.read body left the render variable unsubstituted: %s", body)
		}
	})

	// The injected value overrides any caller-supplied context value, so a
	// concrete command cannot be spoofed through render context.
	t.Run("caller_context_cannot_spoof", func(t *testing.T) {
		setupMailboxTestEnv(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv(envMailbox, "alice@worktree")
		s := NewServer(root, "test")
		key := mailboxLogin(t, s, 1, root)
		got := s.injectMailboxWaitCommand(map[string]string{"MailboxWaitCommand": "evil --slug attacker@worktree"}, key)
		if got["MailboxWaitCommand"] == "evil --slug attacker@worktree" {
			t.Fatalf("caller-supplied MailboxWaitCommand survived injection: %s", got["MailboxWaitCommand"])
		}
		if !strings.Contains(got["MailboxWaitCommand"], "--slug alice@worktree") {
			t.Fatalf("injection did not resolve the real registered slug: %s", got["MailboxWaitCommand"])
		}
	})

	// Regression: {{.MailboxWaitCommand}} is a reserved implicit var, so any
	// path that substitutes the body must supply it or substitution fails with
	// ErrUnprovidedVar. The stem is kind:print and routes to playbook.read, but
	// playbook.render is an exposed tool, so an off-contract render of this stem
	// must still resolve the variable rather than fail closed.
	t.Run("render_path_resolves_variable", func(t *testing.T) {
		setupMailboxTestEnv(t)
		root := t.TempDir()
		initGit(t, root)
		t.Setenv(envMailbox, "alice@worktree")
		s := NewServer(root, "test")
		key := mailboxLogin(t, s, 1, root)

		resp := callToolOnce(t, s, 2, "playbook.render", map[string]any{
			"session_key": key, "name": "lead-use-mailbox",
		})
		if toolIsError(t, resp) {
			t.Fatalf("playbook.render of lead-use-mailbox failed closed (ErrUnprovidedVar regression): %s", resp)
		}
		renderedPath := strings.TrimSpace(toolText(t, resp))
		bodyBytes, err := os.ReadFile(renderedPath)
		if err != nil {
			t.Fatalf("read rendered playbook %q: %v", renderedPath, err)
		}
		body := string(bodyBytes)
		if !strings.Contains(body, "--slug alice@worktree") {
			t.Fatalf("rendered body did not carry the concrete wait command: %s", body)
		}
		if strings.Contains(body, "{{.MailboxWaitCommand}}") {
			t.Fatalf("rendered body left the render variable unsubstituted: %s", body)
		}
	})
}
