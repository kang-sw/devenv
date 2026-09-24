package wsindex

import (
	"context"
	"regexp"
	"strings"

	"github.com/kang-sw/devenv/internal/wsreview"
)

const ticketsDir = "ai-docs/tickets"

var ticketStemRE = regexp.MustCompile(`^\d{6}-[\w-]+$`)

// ValidStem reports whether stem has the ticket stem shape.
func ValidStem(stem string) bool { return ticketStemRE.MatchString(stem) }

// OriginTickets is a review-track tree's ticket inventory.
type OriginTickets struct {
	Open   map[string]bool // stems under idea/, todo/, ready/
	Closed map[string]bool // stems under .done/, .dropped/
}

// HasOrigin reports whether the clone has an origin remote (local config
// only).
func (c *Client) HasOrigin(ctx context.Context) bool { return c.hasOrigin(ctx) }

// AbsenceCached reports whether a never-seen clone is inside the absence TTL.
func (c *Client) AbsenceCached(ctx context.Context) bool {
	seen, err := c.Seen(ctx)
	return err == nil && !seen && c.absenceCached()
}

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
		status, stem, ok := ticketPath(line)
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

func ticketPath(path string) (status, stem string, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(path), ticketsDir+"/")
	if !found {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || !strings.HasSuffix(parts[1], ".md") {
		return "", "", false
	}
	stem = strings.TrimSuffix(parts[1], ".md")
	if !ticketStemRE.MatchString(stem) {
		return "", "", false
	}
	return parts[0], stem, true
}

// OpenOnAnyBranch returns a lazy predicate for GC: on first use it refreshes
// every origin branch's remote-tracking ref (bounded, best-effort) and
// collects every open ticket across them. When the refresh fails the
// predicate answers true for every stem, so GC prunes nothing on a guess.
func (c *Client) OpenOnAnyBranch(ctx context.Context) func(stem string) bool {
	var open map[string]bool
	failed := false
	return func(stem string) bool {
		if open == nil && !failed {
			open = map[string]bool{}
			rctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
			_, err := c.remote(rctx, "fetch", "--quiet", "--no-tags", "--no-write-fetch-head",
				"--no-auto-gc", "--no-recurse-submodules", RemoteName, "+refs/heads/*:refs/remotes/"+RemoteName+"/*")
			cancel()
			if err != nil {
				failed = true
				return true
			}
			refs, err := c.git(ctx, nil, "for-each-ref", "--format=%(refname)", "refs/remotes/"+RemoteName+"/")
			if err != nil {
				failed = true
				return true
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
		if failed {
			return true
		}
		return open[stem]
	}
}

// InitSource resolves, over the network, what init registers: origin's
// default branch (from ls-remote --symref), the review-track declared in its
// AGENTS.md (falling back to the git-default heuristic), and the open tickets
// on that track. An empty or unpushed origin yields an empty inventory.
func (c *Client) InitSource(ctx context.Context) (string, OriginTickets, error) {
	rctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()
	out, err := c.remote(rctx, "ls-remote", "--symref", RemoteName, "HEAD")
	if err != nil {
		return "", OriginTickets{}, err
	}
	defaultBranch := ""
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			defaultBranch = strings.TrimSpace(strings.SplitN(rest, "\t", 2)[0])
		}
	}
	track := ""
	if defaultBranch != "" {
		if err := c.FetchTrack(ctx, defaultBranch); err == nil {
			track = c.declaredTrackAt(ctx, trackingRef(defaultBranch))
		}
		if track == "" {
			track = defaultBranch
		}
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
// The review-track is fetched first when fetchTrack is set (acquire) or when
// the pending log holds an acquire entry the flush will replay; otherwise the
// local remote-tracking ref is read with no network.
func (c *Client) LoadContext(ctx context.Context, sub *Submission, online, fetchTrack bool) error {
	id, err := c.CloneID(ctx)
	if err != nil {
		return err
	}
	sub.Entry.Owner.CloneID = id
	track := c.OriginTrack(ctx)
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
