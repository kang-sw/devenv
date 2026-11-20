// Git status types, branch status, and working tree status parsing.
package wsgit

import (
	"context"
	"strconv"
	"strings"
)

type StatusResult struct {
	Branch       BranchStatus `json:"branch"`
	Clean        bool         `json:"clean"`
	ChangedFiles []FileStatus `json:"changed_files"`
}

type BranchStatus struct {
	Head     string `json:"head,omitempty"`
	OID      string `json:"oid,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Ahead    int    `json:"ahead,omitempty"`
	Behind   int    `json:"behind,omitempty"`
}

// BranchSync reads root's branch and its standing against its upstream with
// cheap ref reads only (rev-parse, symbolic-ref, and rev-list, never a working-tree scan or a
// fetch, so Behind reflects the last fetch). Head is the branch name, or ""
// with OID set to the short commit id on a detached HEAD. Upstream is empty
// when the branch tracks none. ok is false when root is not a repository or
// HEAD names no commit yet.
func BranchSync(ctx context.Context, runner Runner, root string) (status BranchStatus, ok bool) {
	if runner == nil {
		runner = ExecRunner{}
	}
	short, err := runner.RunGit(ctx, root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return BranchStatus{}, false
	}
	oid := strings.TrimSpace(string(short))
	if oid == "" {
		return BranchStatus{}, false
	}
	// The full ref name, trimmed here: both `rev-parse --abbrev-ref HEAD` and
	// `symbolic-ref --short HEAD` print "heads/main" when a tag shares the
	// branch's name. A detached HEAD fails symbolic-ref.
	ref, err := runner.RunGit(ctx, root, "symbolic-ref", "-q", "HEAD")
	head := strings.TrimPrefix(strings.TrimSpace(string(ref)), "refs/heads/")
	if err != nil || head == "" {
		return BranchStatus{OID: oid}, true
	}
	status.Head = head
	up, err := runner.RunGit(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return status, true
	}
	status.Upstream = strings.TrimSpace(string(up))
	counts, err := runner.RunGit(ctx, root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return status, true
	}
	if fields := strings.Fields(string(counts)); len(fields) == 2 {
		status.Ahead, _ = strconv.Atoi(fields[0])
		status.Behind, _ = strconv.Atoi(fields[1])
	}
	return status, true
}

// TreeCounts counts a working tree's entries by kind. A file changed in both
// the index and the working tree counts as Staged and as Modified; an
// unmerged path counts as Modified alone; an untracked directory counts once.
type TreeCounts struct {
	Modified  int
	Staged    int
	Untracked int
}

// WorkingTreeCounts reads root's working-tree standing with one `git status
// --porcelain`, untracked files included (one entry per untracked directory,
// whatever the user's status.showUntrackedFiles says) and ignored ones not.
// It takes no optional lock, so a periodic read never makes a concurrent git
// command of the user's fail on index.lock. ok is false when git fails (not a
// repository, a canceled or timed-out read).
func WorkingTreeCounts(ctx context.Context, runner Runner, root string) (counts TreeCounts, ok bool) {
	if runner == nil {
		runner = ExecRunner{}
	}
	out, err := runner.RunGit(ctx, root, "--no-optional-locks", "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return TreeCounts{}, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		switch {
		case x == '?' && y == '?':
			counts.Untracked++
		case x == '!' && y == '!':
		case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
			counts.Modified++
		default:
			if x != ' ' {
				counts.Staged++
			}
			if y != ' ' {
				counts.Modified++
			}
		}
	}
	return counts, true
}

type FileStatus struct {
	Path           string `json:"path"`
	OldPath        string `json:"old_path,omitempty"`
	Status         string `json:"status"`
	IndexStatus    string `json:"index_status,omitempty"`
	WorktreeStatus string `json:"worktree_status,omitempty"`
}

func (c Client) Status(ctx context.Context, root string) (StatusResult, error) {
	out, err := c.runner().RunGit(ctx, root, StatusArgs()...)
	if err != nil {
		return StatusResult{}, err
	}
	return ParseStatus(out), nil
}

func StatusArgs() []string {
	return []string{"status", "--porcelain=v2", "--branch"}
}

func ParseStatus(out []byte) StatusResult {
	result := StatusResult{Clean: true}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			parseBranchLine(&result.Branch, strings.TrimPrefix(line, "# "))
			continue
		}
		if file, ok := parseFileLine(line); ok {
			result.ChangedFiles = append(result.ChangedFiles, file)
		}
	}
	result.Clean = len(result.ChangedFiles) == 0
	return result
}

func parseBranchLine(branch *BranchStatus, line string) {
	key, value, ok := strings.Cut(line, " ")
	if !ok {
		return
	}
	switch key {
	case "branch.oid":
		branch.OID = value
	case "branch.head":
		branch.Head = value
	case "branch.upstream":
		branch.Upstream = value
	case "branch.ab":
		fields := strings.Fields(value)
		if len(fields) == 2 {
			branch.Ahead = parseAheadBehind(fields[0], "+")
			branch.Behind = parseAheadBehind(fields[1], "-")
		}
	}
}

func parseAheadBehind(value, prefix string) int {
	trimmed := strings.TrimPrefix(value, prefix)
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	return parsed
}

func parseFileLine(line string) (FileStatus, bool) {
	typeCode, rest, ok := strings.Cut(line, " ")
	if !ok {
		return FileStatus{}, false
	}
	switch typeCode {
	case "1":
		fields := strings.SplitN(rest, " ", 8)
		if len(fields) < 8 {
			return FileStatus{}, false
		}
		return FileStatus{Path: cleanPath(fields[7]), Status: fields[0], IndexStatus: string(fields[0][0]), WorktreeStatus: string(fields[0][1])}, true
	case "2":
		fields := strings.SplitN(rest, " ", 9)
		if len(fields) < 9 {
			return FileStatus{}, false
		}
		path := fields[8]
		oldPath := ""
		if current, previous, ok := strings.Cut(path, "\t"); ok {
			path = current
			oldPath = previous
		}
		return FileStatus{Path: cleanPath(path), OldPath: cleanPath(oldPath), Status: fields[0], IndexStatus: string(fields[0][0]), WorktreeStatus: string(fields[0][1])}, true
	case "u":
		fields := strings.SplitN(rest, " ", 10)
		if len(fields) < 10 {
			return FileStatus{}, false
		}
		return FileStatus{Path: cleanPath(fields[9]), Status: fields[0], IndexStatus: string(fields[0][0]), WorktreeStatus: string(fields[0][1])}, true
	case "?", "!":
		return FileStatus{Path: cleanPath(rest), Status: typeCode}, true
	default:
		return FileStatus{}, false
	}
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	unquoted, err := strconv.Unquote(path)
	if err == nil {
		return unquoted
	}
	return path
}
