package wstrackerhost

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// StoreView is a store's view state as the mockup README "View state"
// defines it: wstracker's derived View plus the store, sync, and clones
// fields and stats.commits, which only the caller knows.
type StoreView struct {
	*wstracker.View
	Store  StoreInfo              `json:"store"`
	Sync   SyncInfo               `json:"sync"`
	Clones map[string][]CloneInfo `json:"clones"`
}

// StoreInfo is the view state's store field.
type StoreInfo struct {
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Remote *string `json:"remote"` // nil for a store with no remote
}

// SyncInfo is the viewer clone's sync state.
type SyncInfo struct {
	// Online: the last check or sync reached the remote (true before the
	// first one); always false with no remote, the same path as offline.
	Online bool `json:"online"`
	// SyncedAt is the last check or sync that reached the remote.
	SyncedAt *string `json:"syncedAt"`
	// Upstream counts remote commits not yet fetched. A check takes all of
	// them, and nothing is known between checks, so it is always 0.
	Upstream int `json:"upstream"`
	// Unpushed lists this clone's commits not yet on the remote, oldest
	// first; the open batch, when there is one, is the last.
	Unpushed []UnpushedCommit `json:"unpushed"`
	// Error is the last attempt's error when it did not reach the remote.
	Error string `json:"error,omitempty"`
}

// UnpushedCommit is one unpushed commit, or the open batch (Open).
type UnpushedCommit struct {
	Open   bool     `json:"open"`
	TS     string   `json:"ts"`
	Events []string `json:"events"`
}

// CloneInfo is one clone stream of a member.
type CloneInfo struct {
	Stream string `json:"stream"`
	// Online is known only for the viewer's own clone; nil for the others.
	Online *bool `json:"online"`
	// Unpushed counts the viewer clone's unpushed commits; 0 for others,
	// whose unpushed writes this clone cannot see.
	Unpushed int `json:"unpushed"`
}

func tsString(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// fill completes v for the viewer clone stream: evs are the store's
// events (wsledger Events), p its PendingReport, s its recorded sync
// state, and commits its commit count.
func fill(v *wstracker.View, e StoreEntry, stream string, evs []wsledger.Event, p wsledger.PendingReport, s syncState, commits int) *StoreView {
	out := &StoreView{View: v, Store: StoreInfo{Name: e.ID, Label: e.DisplayLabel()}}
	if e.Remote != "" {
		remote := e.Remote
		out.Store.Remote = &remote
		out.Sync.Online = s.Error == ""
		out.Sync.Error = s.Error
	}
	if !s.SyncedAt.IsZero() {
		at := tsString(s.SyncedAt)
		out.Sync.SyncedAt = &at
	}
	out.Sync.Unpushed = unpushedCommits(evs, stream, p)
	if out.Sync.Unpushed == nil {
		out.Sync.Unpushed = []UnpushedCommit{}
	}
	out.View.Stats.Commits = commits
	if p.Uncommitted > 0 {
		out.View.Stats.Commits++ // the open batch counts as a commit, as in the mockup
	}

	streams := map[string]bool{stream: true}
	for s := range v.Heads {
		streams[s] = true
	}
	out.Clones = map[string][]CloneInfo{}
	names := make([]string, 0, len(streams))
	for s := range streams {
		names = append(names, s)
	}
	sort.Strings(names)
	for _, s := range names {
		member, _, _ := strings.Cut(s, ".")
		c := CloneInfo{Stream: s}
		if s == stream {
			online := out.Sync.Online
			c.Online, c.Unpushed = &online, len(out.Sync.Unpushed)
		}
		out.Clones[member] = append(out.Clones[member], c)
	}
	return out
}

// unpushedCommits groups this clone's last p.Count events (the unpushed
// ones) into commits: the last p.Uncommitted are the open batch, and the
// closed ones split where the commit header's seen changes. Two closed
// commits with the same seen read as one; the list is for display.
func unpushedCommits(evs []wsledger.Event, stream string, p wsledger.PendingReport) []UnpushedCommit {
	var own []wsledger.Event
	for _, e := range evs {
		if e.ID.Stream() == stream && e.ID.Broken == "" {
			own = append(own, e)
		}
	}
	sort.Slice(own, func(i, j int) bool { return own[i].ID.Seq < own[j].ID.Seq })
	if p.Count < len(own) {
		own = own[len(own)-p.Count:]
	}
	var out []UnpushedCommit
	var seen map[string]int
	for _, e := range own {
		open := e.Pending
		last := len(out) - 1
		if last < 0 || out[last].Open != open || (!open && !reflect.DeepEqual(seen, e.Seen)) {
			out = append(out, UnpushedCommit{Open: open})
			last++
		}
		seen = e.Seen
		out[last].Events = append(out[last].Events, e.ID.String())
		out[last].TS = tsString(e.TS)
	}
	return out
}

// countCommits counts the commits of every clone stream in the store's
// repository (wsledger's documented layout: the bare repository at
// <dir>/repo.git, streams under refs/heads/tracker/m/).
func countCommits(ctx context.Context, runner wsgit.Runner, dir string) (int, error) {
	out, err := runner.RunGit(ctx, filepath.Join(dir, "repo.git"), "rev-list", "--count", "--glob=refs/heads/tracker/m/*")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
