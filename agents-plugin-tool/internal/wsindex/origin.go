package wsindex

import (
	"context"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsreview"
)

const ticketsDir = "ai-docs/tickets"

// OriginTickets is a review-track tree's ticket inventory.
type OriginTickets struct {
	Open   map[string]bool // stems under idea/, todo/, ready/
	Closed map[string]bool // stems under .done/, .dropped/
}

// HasOrigin reports whether the clone has an origin remote (local config
// only).
func (c *Client) HasOrigin(ctx context.Context) bool { return c.hasOrigin(ctx) }

// OriginTrack resolves the review-track origin-first from local refs, with no
// network: the review-track declaration in AGENTS.md of origin's default
// branch (the local refs/remotes/origin/HEAD tree), then the git-default
// heuristic. The local checkout's AGENTS.md is never consulted, so a stale or
// diverged local declaration cannot redirect the index.
func (c *Client) OriginTrack(ctx context.Context) string {
	if track := c.declaredTrackAt(ctx, "refs/remotes/"+RemoteName+"/HEAD"); track != "" {
		return track
	}
	track, _ := wsreview.ResolveTrackFallback(ctx, c.root)
	return track
}

// onlineTrack is OriginTrack for paths that already go online (every index
// write, acquire, init). When the clone lacks a local
// refs/remotes/origin/HEAD, it asks origin for its default branch (the probe
// InitSource uses) and reads the review-track declared there, falling back to
// that branch as init does, so init, acquire, pruning, and GC resolve the
// same track. Nothing is cached: the probe runs only on such clones. A failed
// probe falls back to OriginTrack.
func (c *Client) onlineTrack(ctx context.Context) string {
	if _, ok, err := c.refOID(ctx, "refs/remotes/"+RemoteName+"/HEAD"); err == nil && ok {
		return c.OriginTrack(ctx)
	}
	defaultBranch, err := c.probeDefaultBranch(ctx)
	if err != nil || defaultBranch == "" {
		return c.OriginTrack(ctx)
	}
	return c.trackAtDefaultBranch(ctx, defaultBranch, false)
}

// trackAtDefaultBranch reads the review-track declared in AGENTS.md of
// origin's default branch, falling back to that branch itself. The branch's
// tracking ref is fetched first when alwaysFetch is set (init) or when it is
// missing; a failed fetch leaves the fallback.
func (c *Client) trackAtDefaultBranch(ctx context.Context, defaultBranch string, alwaysFetch bool) string {
	fetched := true
	if _, ok, err := c.refOID(ctx, trackingRef(defaultBranch)); alwaysFetch || err != nil || !ok {
		fetched = c.FetchTrack(ctx, defaultBranch) == nil
	}
	if fetched {
		if track := c.declaredTrackAt(ctx, trackingRef(defaultBranch)); track != "" {
			return track
		}
	}
	return defaultBranch
}

// probeDefaultBranch asks origin for its default branch with ls-remote
// --symref; "" when origin has none (an empty or unpushed origin).
func (c *Client) probeDefaultBranch(ctx context.Context) (string, error) {
	rctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()
	out, err := c.remote(rctx, "ls-remote", "--symref", RemoteName, "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			return strings.TrimSpace(strings.SplitN(rest, "\t", 2)[0]), nil
		}
	}
	return "", nil
}

func (c *Client) declaredTrackAt(ctx context.Context, rev string) string {
	out, err := c.runner.Run(ctx, c.root, Command{Args: []string{"show", rev + ":AGENTS.md"}, Env: localEnv})
	if err != nil {
		return ""
	}
	return wsreview.ParseAgentsReviewPolicy(string(out)).ReviewTrack
}

// trackingRef is the local remote-tracking ref of an origin branch.
func trackingRef(branch string) string { return "refs/remotes/" + RemoteName + "/" + branch }

// FetchTrack refreshes the local remote-tracking ref of the review-track so
// the origin-closed check reads a fresh tree. It is best-effort: a failure
// (a missing branch on an empty origin, or a transport error the index fetch
// will classify anyway) leaves the tracking ref as it was.
func (c *Client) FetchTrack(ctx context.Context, track string) error {
	if track == "" {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()
	_, err := c.remote(rctx, "fetch", "--quiet", "--no-tags", "--no-write-fetch-head",
		"--no-auto-gc", "--no-recurse-submodules", RemoteName, "+refs/heads/"+track+":"+trackingRef(track))
	return err
}

// OriginTicketsAt lists the ticket inventory of an origin branch from its
// local remote-tracking ref. A missing ref is an empty inventory.
func (c *Client) OriginTicketsAt(ctx context.Context, branch string) OriginTickets {
	inv := OriginTickets{Open: map[string]bool{}, Closed: map[string]bool{}}
	if branch == "" {
		return inv
	}
	out, err := c.git(ctx, nil, "ls-tree", "-r", "--name-only", trackingRef(branch), "--", ticketsDir)
	if err != nil {
		return inv
	}
	for _, line := range strings.Split(out, "\n") {
		status, stem, ok := wsdoc.ParseTicketPath(line)
		if !ok {
			continue
		}
		switch status {
		case "idea", "todo", "ready":
			inv.Open[stem] = true
		case ".done", ".dropped":
			inv.Closed[stem] = true
		}
	}
	return inv
}

// OpenOnAnyBranch returns a lazy predicate for GC: on first use it refreshes
// every origin branch's remote-tracking ref (bounded, best-effort) and
// collects every open ticket across them. When the refresh fails the
// predicate returns the error, so GC prunes nothing on a guess.
func (c *Client) OpenOnAnyBranch(ctx context.Context) func(stem string) (bool, error) {
	var open map[string]bool
	var failed error
	return func(stem string) (bool, error) {
		if open == nil && failed == nil {
			open = map[string]bool{}
			rctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
			_, err := c.remote(rctx, "fetch", "--quiet", "--no-tags", "--no-write-fetch-head",
				"--no-auto-gc", "--no-recurse-submodules", RemoteName, "+refs/heads/*:refs/remotes/"+RemoteName+"/*")
			cancel()
			if err != nil {
				failed = err
				return false, err
			}
			refs, err := c.git(ctx, nil, "for-each-ref", "--format=%(refname)", "refs/remotes/"+RemoteName+"/")
			if err != nil {
				failed = err
				return false, err
			}
			for _, ref := range strings.Split(refs, "\n") {
				branch := strings.TrimPrefix(strings.TrimSpace(ref), "refs/remotes/"+RemoteName+"/")
				if branch == "" || branch == "HEAD" {
					continue
				}
				for s := range c.OriginTicketsAt(ctx, branch).Open {
					open[s] = true
				}
			}
		}
		if failed != nil {
			return false, failed
		}
		return open[stem], nil
	}
}

// InitSource resolves, over the network, what init registers: origin's
// default branch (from ls-remote --symref), the review-track declared in its
// AGENTS.md (falling back to the git-default heuristic), and the open tickets
// on that track. An empty or unpushed origin yields an empty inventory.
func (c *Client) InitSource(ctx context.Context) (string, OriginTickets, error) {
	defaultBranch, err := c.probeDefaultBranch(ctx)
	if err != nil {
		return "", OriginTickets{}, err
	}
	track := ""
	if defaultBranch != "" {
		track = c.trackAtDefaultBranch(ctx, defaultBranch, true)
	}
	if track == "" {
		track, _ = wsreview.ResolveTrackFallback(ctx, c.root)
	}
	if track != "" && track != defaultBranch {
		_ = c.FetchTrack(ctx, track)
	}
	return track, c.OriginTicketsAt(ctx, track), nil
}

// LoadContext fills what one submission needs once the index is known to be
// in use: the recording owner's clone id (generated on first use), the
// origin-closed set of the review-track, and, online, the lazy GC predicate.
// Online, the review-track resolves through onlineTrack; offline, through
// OriginTrack. The review-track is fetched first when fetchTrack is set
// (acquire) or when the pending log holds an acquire entry the flush will
// replay; otherwise the local remote-tracking ref is read with no network.
func (c *Client) LoadContext(ctx context.Context, sub *Submission, online, fetchTrack bool) error {
	id, err := c.CloneID(ctx)
	if err != nil {
		return err
	}
	sub.Entry.Owner.CloneID = id
	var track string
	if online {
		track = c.onlineTrack(ctx)
	} else {
		track = c.OriginTrack(ctx)
	}
	if online && !fetchTrack {
		pending, err := c.Pending(ctx)
		if err != nil {
			return err
		}
		for _, e := range pending {
			if e.Op == OpAcquire {
				fetchTrack = true
				break
			}
		}
	}
	if online && fetchTrack {
		_ = c.FetchTrack(ctx, track)
	}
	sub.Applier.Closed = c.OriginTicketsAt(ctx, track).Closed
	if online {
		sub.Applier.OpenAnywhere = c.OpenOnAnyBranch(ctx)
	}
	return nil
}
