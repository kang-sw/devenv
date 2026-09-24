package wsindex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

// RemoteState is what one remote query established. Absence is only ever a
// successful remote answer with no matching ref; any transport, timeout, or
// credential failure is Unreachable, never Absent.
type RemoteState int

const (
	RemoteUnreachable RemoteState = iota
	RemoteAbsent
	RemotePresent
)

func (s RemoteState) String() string {
	switch s {
	case RemoteAbsent:
		return "absent"
	case RemotePresent:
		return "present"
	default:
		return "unreachable"
	}
}

// hasOrigin reports whether the clone has an origin remote, from local config
// only. A repository without one is silently index-absent.
func (c *Client) hasOrigin(ctx context.Context) bool {
	out, err := c.git(ctx, nil, "config", "--get", "remote."+RemoteName+".url")
	return err == nil && strings.TrimSpace(out) != ""
}

// lsRemote is discovery: one ls-remote against the ordered candidate list,
// returning the first candidate present.
func (c *Client) lsRemote(ctx context.Context) (string, RemoteState, error) {
	args := append([]string{"ls-remote", RemoteName}, discoveryCandidates...)
	out, err := c.remote(ctx, args...)
	if err != nil {
		return "", RemoteUnreachable, err
	}
	found := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			found[fields[1]] = fields[0]
		}
	}
	for _, ref := range discoveryCandidates {
		if oid, ok := found[ref]; ok {
			return oid, RemotePresent, nil
		}
	}
	return "", RemoteAbsent, nil
}

// fetchTip fetches the remote index into a scratch ref and returns its tip.
// A failed fetch is classified through ls-remote so a deleted ref reads as
// absence rather than as an unreachable remote. rctx bounds the remote
// commands; local ref reads run on ctx.
func (c *Client) fetchTip(ctx, rctx context.Context) (string, RemoteState, error) {
	temp := fetchTempPrefix + randomHex(8)
	_, err := c.remote(rctx, "fetch", "--quiet", "--no-tags", "--no-write-fetch-head",
		"--no-auto-gc", "--no-recurse-submodules", RemoteName, "+"+RemoteRef+":"+temp)
	if err != nil {
		_, state, lsErr := c.lsRemote(rctx)
		if state == RemoteAbsent {
			return "", RemoteAbsent, nil
		}
		if lsErr != nil {
			return "", RemoteUnreachable, lsErr
		}
		// The ref exists but the fetch still failed: a transport problem.
		return "", RemoteUnreachable, err
	}
	defer func() {
		// Best-effort scratch cleanup on a fresh context: the caller's may be
		// near its deadline.
		_, _ = c.git(context.Background(), nil, "update-ref", "-d", temp)
	}()
	oid, ok, err := c.refOID(ctx, temp)
	if err != nil {
		return "", RemoteUnreachable, err
	}
	if !ok {
		return "", RemoteAbsent, nil
	}
	return oid, RemotePresent, nil
}

// pushOutcome classifies one CAS push.
type pushOutcome int

const (
	pushAccepted pushOutcome = iota
	// pushLost is a compare-and-swap loss: the remote tip moved since the
	// caller read it. The caller re-fetches and re-applies.
	pushLost
	// pushRefused is any other remote refusal (a hook, a permission denial).
	pushRefused
	// pushFailed is a transport, timeout, or credential failure.
	pushFailed
)

// pushCAS pushes commit to the remote index ref only if the remote still
// holds expected ("" = the ref must not exist). Hooks are bypassed: the index
// ref is not code and a local pre-push hook must not veto or slow it.
func (c *Client) pushCAS(ctx context.Context, commit, expected string) (pushOutcome, string, error) {
	out, err := c.remote(ctx, "push", "--porcelain", "--no-verify",
		"--force-with-lease="+RemoteRef+":"+expected, RemoteName, commit+":"+RemoteRef)
	if err == nil {
		return pushAccepted, "", nil
	}
	detail := out
	var gitErr *GitError
	if errors.As(err, &gitErr) {
		detail = gitErr.Stdout + "\n" + gitErr.Stderr
	}
	status, ok := porcelainStatus(detail)
	if !ok {
		return pushFailed, strings.TrimSpace(detail), err
	}
	if isCASLoss(status) {
		return pushLost, status, nil
	}
	return pushRefused, status, err
}

// casLossPhrases are the rejection reasons that mean the remote tip moved
// between the caller's read and its push. Matching is on the porcelain
// status text, which differs across git versions:
//   - client-side lease check: "stale info"; plain ref races: "non-fast-forward",
//     "fetch first", "already exists".
//   - receive-pack before git 2.51 updates each ref on its own and reports a
//     lost race as "failed to update ref" (with "cannot lock ref ... but
//     expected ..." in the detail).
//   - receive-pack from git 2.51 batches ref updates and reports the ref
//     transaction error itself (ref_transaction_error_msg in refs.c):
//     "incorrect old value provided" when the tip moved, "reference does not
//     exist" when it was deleted meanwhile, "reference already exists" when a
//     create raced another create.
//
// A concurrent receive holding the ref lock surfaces as a generic update
// failure ("failed to update ref(s)", "failed to lock"); the bounded retry
// absorbs a misclassified permanent refusal.
var casLossPhrases = []string{
	"stale info", "non-fast-forward", "fetch first", "cannot lock ref",
	"but expected", "already exists", "incorrect old value", "reference does not exist",
	"failed to update ref", "failed to lock",
}

// isCASLoss reports whether a porcelain rejection status is a lost
// compare-and-swap race rather than a permanent refusal.
func isCASLoss(status string) bool {
	lower := strings.ToLower(status)
	for _, phrase := range casLossPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// porcelainStatus extracts the rejection summary of the index ref from push
// --porcelain output ("!\t<src>:<dst>\t[rejected] (reason)").
func porcelainStatus(out string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "!\t") {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) == 3 && strings.HasSuffix(parts[1], ":"+RemoteRef) {
			return parts[2], true
		}
	}
	return "", false
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
