package wsdoc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ticketScope is the single gate for every index-aware ticket path in this
// package. A nil *ticketScope means "no sparse-checkout scope is active", and
// every downstream branch on a nil scope is byte-for-byte today's code path —
// nothing else in wsdoc may test core.sparseCheckout.
//
// It follows gitIgnoreMatcher's posture (project_tree.go): exec git directly,
// memoize, and degrade rather than propagate. The {#260720-wsdoc-commit-boundary}
// rule forbids wsdoc *importing* wsgit (wsgit.Client.Verifier already calls
// into wsdoc.TicketVerify, so the reverse import would invert the dependency
// direction); it does not forbid wsdoc from running git, and gitIgnoreMatcher
// is the standing precedent for a wsdoc-local, exec-based, per-call accessor.
//
// tickets_mutate.go's injected GitRunner is a caller-injection convenience for
// the two mutation helpers, not a rule: threading a runner through TicketVerify
// and its 26 call sites would make the sparse-off path structurally different
// from today, which is exactly what must not change.
type ticketScope struct {
	root   string
	gitDir string

	pathsLoaded bool
	paths       []string
	pathSet     map[string]bool
	pathsErr    error

	bodyCache map[string]string
}

// ticketIndexPrefix bounds every index query to the ticket board; the scope
// never enumerates the rest of the index.
const ticketIndexPrefix = "ai-docs/tickets"

// newTicketScope runs the gate cheapest-first so a repository the user does not
// have scoped spawns zero git processes. Any negative or failed probe yields
// nil, i.e. today's behavior.
//
// Both filter-off shapes must stay free, not just the never-scoped one:
// `git sparse-checkout disable` leaves $GIT_DIR/info/sparse-checkout in place
// with its patterns intact and only writes core.sparseCheckout = false into
// $GIT_DIR/config.worktree (measured, git 2.43). A pattern-file stat alone
// would therefore pass forever after a disable and spawn a `git config` process
// on every gated call — per loadTicketGraph, per TicketCreate, per mutation,
// per discovery annotation — on a repository the user restored. So an
// explicitly false core.sparseCheckout that every config file agrees on is
// terminal, and git is consulted whenever the files leave the answer open.
func newTicketScope(root string) *ticketScope {
	gitDir := resolveGitDir(root)
	if gitDir == "" {
		return nil
	}
	// Verified: the active pattern file lives at
	// $(git rev-parse --absolute-git-dir)/info/sparse-checkout for both a plain
	// repository and a linked worktree.
	if info, err := os.Stat(filepath.Join(gitDir, "info", "sparse-checkout")); err != nil || info.IsDir() {
		return nil
	}
	if sparseCheckoutDisabledInConfigFiles(gitDir) {
		return nil
	}
	scope := &ticketScope{root: root, gitDir: gitDir}
	out, err := scope.run("", "config", "--type=bool", "--get", "core.sparseCheckout")
	if err != nil {
		// git exits 1 with no output when the key was never set.
		return nil
	}
	if strings.TrimSpace(string(out)) != "true" {
		return nil
	}
	return scope
}

// sparseCheckoutDisabledInConfigFiles reads core.sparseCheckout out of the
// config files git keeps for this worktree and reports a *negative* fast path
// only: it returns true solely when the files cannot disagree about the answer
// being false. Anything else — a `true` anywhere, an unparseable value, no
// value at all — reports false and defers to `git config`, which stays the
// authority for a positive answer (includes, global/system scope, and
// precedence are git's business, not ours).
//
// The agreement rule is what makes reading $GIT_DIR/config.worktree safe
// without resolving extensions.worktreeConfig: git honors that file only when
// the extension is set, so trusting it alone reports false where git resolves
// true (measured: unset the extension by hand, leave config.worktree's
// `sparseCheckout = false` in place, and `git config --get core.sparseCheckout`
// still answers true from the repository config). That inversion is the
// dangerous direction — the scope goes nil while git keeps filtering the
// worktree, and every resolution surface silently reverts to a partial board.
// Requiring agreement removes it: a `true` in either file defers to git, and a
// `false` with no competing `true` resolves to false under both readings.
func sparseCheckoutDisabledInConfigFiles(gitDir string) bool {
	sawFalse := false
	for _, path := range gitConfigFiles(gitDir) {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value, found := coreBoolFromGitConfig(string(raw), "sparsecheckout")
		if !found {
			continue
		}
		if value {
			return false
		}
		sawFalse = true
	}
	return sawFalse
}

// gitConfigFiles lists the on-disk config files that can carry
// core.sparseCheckout for this worktree, in no particular precedence order —
// sparseCheckoutDisabledInConfigFiles requires agreement rather than ranking
// them. A linked worktree's GIT_DIR holds only config.worktree, so the shared
// repository config is reached through the $GIT_DIR/commondir pointer; missing
// files are simply skipped by the caller.
func gitConfigFiles(gitDir string) []string {
	paths := []string{
		filepath.Join(gitDir, "config.worktree"),
		filepath.Join(gitDir, "config"),
	}
	raw, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return paths
	}
	common := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
	if common == "" {
		return paths
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	return append(paths, filepath.Join(common, "config"))
}

// coreBoolFromGitConfig finds `key` under a plain `[core]` section and decodes
// git's boolean spelling. Only the exact shapes git writes itself are
// recognized; a subsectioned header, a continuation, or an unrecognized value
// reports not-found so the caller falls through to git.
func coreBoolFromGitConfig(text, key string) (value bool, found bool) {
	inCore := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inCore = strings.EqualFold(trimmed, "[core]")
			continue
		}
		if !inCore {
			continue
		}
		name, raw, hasValue := strings.Cut(trimmed, "=")
		if !strings.EqualFold(strings.TrimSpace(name), key) {
			continue
		}
		if !hasValue {
			// A bare key is git's spelling of true.
			return true, true
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "yes", "on", "1":
			return true, true
		case "false", "no", "off", "0":
			return false, true
		default:
			return false, false
		}
	}
	return false, false
}

// resolveGitDir finds GIT_DIR with the filesystem only: <root>/.git is either
// the directory itself or a one-line `gitdir: <path>` pointer (linked worktree
// or submodule). Anything else means "not a repository we can reason about".
func resolveGitDir(root string) string {
	candidate := filepath.Join(root, ".git")
	info, err := os.Stat(candidate)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return candidate
	}
	raw, err := os.ReadFile(candidate)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	dir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if dir == "" {
		return ""
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	return dir
}

func (s *ticketScope) run(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", s.root}, args...)...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}

// indexPaths lists every board-relative ticket path the index carries,
// including the ones sparse-checkout removed from disk. Memoized for the life
// of one call. Once the gate has confirmed a scope is active a failure here
// means the board is known-partial, so the error propagates instead of
// degrading — emitting no advisories beats emitting false ones.
func (s *ticketScope) indexPaths() ([]string, error) {
	if s.pathsLoaded {
		return s.paths, s.pathsErr
	}
	s.pathsLoaded = true
	out, err := s.run("", "ls-files", "-z", "--", ticketIndexPrefix)
	if err != nil {
		s.pathsErr = fmt.Errorf("list sparse-checkout ticket index: %w", err)
		return nil, s.pathsErr
	}
	s.paths = splitNULPaths(out)
	s.pathSet = make(map[string]bool, len(s.paths))
	for _, path := range s.paths {
		s.pathSet[path] = true
	}
	return s.paths, nil
}

// hasIndexPath reports whether a board-relative path exists in the index,
// whether or not it is checked out.
func (s *ticketScope) hasIndexPath(rel string) (bool, error) {
	if _, err := s.indexPaths(); err != nil {
		return false, err
	}
	return s.pathSet[rel], nil
}

// bodies reads the index blob for each requested path with a single
// `git cat-file --batch` process, so the graph path never spawns one process
// per hidden ticket. Results are memoized; already-cached paths are not
// re-requested.
func (s *ticketScope) bodies(paths []string) (map[string]string, error) {
	if s.bodyCache == nil {
		s.bodyCache = map[string]string{}
	}
	var wanted []string
	for _, path := range paths {
		if _, ok := s.bodyCache[path]; ok {
			continue
		}
		wanted = append(wanted, path)
	}
	if len(wanted) > 0 {
		var stdin strings.Builder
		for _, path := range wanted {
			// Records are paired to requests positionally, so a path carrying a
			// newline would split into two stdin lines and shift every later
			// body onto the wrong ticket - silently handing one ticket's
			// `parent:` to another. Refuse instead: the caller's propagate path
			// yields no advisories, which is the safe direction.
			if strings.ContainsAny(path, "\n\r") {
				return nil, fmt.Errorf("read sparse-checkout ticket bodies: index path contains a newline: %q", path)
			}
			stdin.WriteString(":" + path + "\n")
		}
		out, err := s.run(stdin.String(), "cat-file", "--batch")
		if err != nil {
			return nil, fmt.Errorf("read sparse-checkout ticket bodies: %w", err)
		}
		decoded, err := parseCatFileBatch(out, wanted)
		if err != nil {
			return nil, fmt.Errorf("read sparse-checkout ticket bodies: %w", err)
		}
		for path, body := range decoded {
			s.bodyCache[path] = body
		}
	}
	result := make(map[string]string, len(paths))
	for _, path := range paths {
		if body, ok := s.bodyCache[path]; ok {
			result[path] = body
		}
	}
	return result, nil
}

// includes reports whether a path is inside the current sparsity rules, which
// is exact even for a path that does not exist yet — the property that makes
// it usable as a destination pre-flight. `git sparse-checkout check-rules`
// requires git >= 2.42, so any failure fails OPEN (returns true) and the
// post-hoc wrap in the mutation helpers is the backstop.
func (s *ticketScope) includes(rel string) bool {
	out, err := s.run(rel+"\n", "sparse-checkout", "check-rules")
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == rel {
			return true
		}
	}
	return false
}

// splitNULPaths mirrors wsgit's parseNULTerminatedPaths shape; it is
// reimplemented rather than imported because wsdoc must not import wsgit.
func splitNULPaths(out []byte) []string {
	if len(out) == 0 {
		return nil
	}
	parts := bytes.Split(out, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		paths = append(paths, string(part))
	}
	return paths
}

// parseCatFileBatch decodes `git cat-file --batch` output into a
// requested-path -> body map. Records arrive in request order as
// "<oid> blob <size>\n<content>\n"; an unknown path yields "<input> missing"
// and is skipped. The input order is the only link back to the path, since the
// header carries the object id rather than the request.
//
// A framing anomaly is an error, never a short read. Returning what was decoded
// so far would hand every later hidden ticket an empty body, and in resolveGraph
// mode that drops its `parent:` - so graph.children loses the edge and the
// all-children-closed ACTION can fire on an epic whose hidden children are still
// open. That inverted advisory is exactly what the index path exists to prevent,
// so a partial decode must take the propagate-then-swallow path instead of
// returning success.
func parseCatFileBatch(out []byte, requested []string) (map[string]string, error) {
	result := map[string]string{}
	offset, next := 0, 0
	for next < len(requested) {
		nl := bytes.IndexByte(out[offset:], '\n')
		if nl < 0 {
			return nil, fmt.Errorf("cat-file --batch ended after %d of %d records", next, len(requested))
		}
		header := string(out[offset : offset+nl])
		offset += nl + 1
		path := requested[next]
		next++
		if strings.HasSuffix(header, " missing") {
			continue
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[1] != "blob" {
			return nil, fmt.Errorf("cat-file --batch record %d for %s has an unexpected header: %q", next, path, header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || offset+size > len(out) {
			return nil, fmt.Errorf("cat-file --batch record %d for %s has an unreadable size: %q", next, path, header)
		}
		result[path] = string(out[offset : offset+size])
		offset += size
		if offset < len(out) && out[offset] == '\n' {
			offset++
		}
	}
	return result, nil
}

// ticketIndexPathParts splits a board-relative index path into its status
// directory and stem, rejecting anything that is not
// ai-docs/tickets/<status>/<stem>.md.
func ticketIndexPathParts(rel string) (status, stem string, ok bool) {
	const prefix = ticketIndexPrefix + "/"
	if !strings.HasPrefix(rel, prefix) || !strings.HasSuffix(rel, ".md") {
		return "", "", false
	}
	rest := strings.TrimPrefix(rel, prefix)
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "", "", false
	}
	status = rest[:slash]
	stem = strings.TrimSuffix(rest[slash+1:], ".md")
	if stem == "" || strings.Contains(stem, "/") {
		return "", "", false
	}
	return status, stem, true
}

// TicketScopeInfo reports whether this worktree hides part of the ticket board
// and how much. It is the path-only index enumeration the discovery surfaces
// (tickets.list, tickets.find's query form, project_tree) use for their
// hidden-count annotation: it reads no blob and never touches loadTicketGraph,
// so a discovery call never pays for .done/.dropped body reads.
type TicketScopeInfo struct {
	Active      bool     `json:"active"`
	Hidden      int      `json:"hidden"`
	HiddenStems []string `json:"hidden_stems,omitempty"`
}

// TicketScope counts the tickets under statuses that live in the index but not
// on disk. statuses is the effective status list (empty means the default open
// set ready/todo/idea). With core.sparseCheckout unset it returns
// {Active: false, Hidden: 0} after at most two os.Stat calls.
func TicketScope(root string, statuses []string) (TicketScopeInfo, error) {
	scope := newTicketScope(root)
	if scope == nil {
		return TicketScopeInfo{}, nil
	}
	wanted := map[string]bool{}
	for _, status := range ticketScopeStatuses(statuses) {
		wanted[status] = true
	}
	paths, err := scope.indexPaths()
	if err != nil {
		return TicketScopeInfo{}, err
	}
	info := TicketScopeInfo{Active: true}
	for _, rel := range paths {
		status, stem, ok := ticketIndexPathParts(rel)
		if !ok || !wanted[status] {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			continue
		}
		info.Hidden++
		info.HiddenStems = append(info.HiddenStems, stem)
	}
	return info, nil
}

func ticketScopeStatuses(statuses []string) []string {
	if len(statuses) == 0 {
		return []string{"ready", "todo", "idea"}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, status := range statuses {
		normalized := normalizeTicketStatus(status)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

// scopeBlockedMoveError is the caller-facing refusal for a mutation whose
// source or destination sits outside this worktree's sparse-checkout scope. It
// names the scope and the widen-then-retry remedy instead of relaying git's
// raw advice text, which is gettext-localized and therefore unparseable.
func scopeBlockedMoveError(role, relPath string) error {
	return fmt.Errorf(
		"%s %s is outside this worktree's sparse-checkout scope (core.sparseCheckout); "+
			"it stays in the index but is not checked out here. Widen the scope "+
			"(`git sparse-checkout add %s`, or `git sparse-checkout disable`) and retry",
		role, relPath, relPath)
}

// wrapScopeMoveError is the non-classifying backstop for a git failure that a
// pre-flight did not catch (notably git < 2.42, where check-rules is absent and
// includes fails open). It wraps rather than replaces the raw error so an
// unrelated git failure stays legible, and states the remedy conditionally so
// the message never claims a cause it did not verify.
func wrapScopeMoveError(err error, relPath string) error {
	return fmt.Errorf(
		"%w; a sparse-checkout scope is active in this worktree (core.sparseCheckout) - "+
			"if %s is outside it, widen the scope (`git sparse-checkout add %s`, or "+
			"`git sparse-checkout disable`) and retry",
		err, relPath, relPath)
}
