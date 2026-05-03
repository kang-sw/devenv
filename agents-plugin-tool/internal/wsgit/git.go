package wsgit

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	DiffModeFull     = "full"
	DiffModeStat     = "stat"
	DiffModeNameOnly = "name_only"
)

type Runner interface {
	RunGit(ctx context.Context, root string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	argv := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

type Client struct {
	Runner Runner
}

func NewClient() Client { return Client{Runner: ExecRunner{}} }

func (c Client) runner() Runner {
	if c.Runner == nil {
		return ExecRunner{}
	}
	return c.Runner
}

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

type DiffOptions struct {
	Range string
	Paths []string
	Mode  string
}

type DiffResult struct {
	Mode   string   `json:"mode"`
	Range  string   `json:"range,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Output string   `json:"output"`
}

func (c Client) Diff(ctx context.Context, root string, opts DiffOptions) (DiffResult, error) {
	args, mode, err := DiffArgs(opts)
	if err != nil {
		return DiffResult{}, err
	}
	out, err := c.runner().RunGit(ctx, root, args...)
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{Mode: mode, Range: opts.Range, Paths: opts.Paths, Output: string(out)}, nil
}

func DiffArgs(opts DiffOptions) ([]string, string, error) {
	mode := opts.Mode
	if mode == "" {
		mode = DiffModeFull
	}
	args := []string{"diff"}
	switch mode {
	case DiffModeFull:
	case DiffModeStat:
		args = append(args, "--stat")
	case DiffModeNameOnly:
		args = append(args, "--name-only")
	default:
		return nil, "", fmt.Errorf("unsupported diff mode %q", opts.Mode)
	}
	if opts.Range != "" {
		args = append(args, opts.Range)
	}
	if len(opts.Paths) > 0 {
		args = append(args, "--")
		args = append(args, opts.Paths...)
	}
	return args, mode, nil
}

type LogOptions struct {
	Range       string
	Limit       int
	IncludeBody bool
}

type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Body    string `json:"body,omitempty"`
}

type LogResult struct {
	Range       string   `json:"range,omitempty"`
	Limit       int      `json:"limit"`
	IncludeBody bool     `json:"include_body"`
	Commits     []Commit `json:"commits"`
}

func (c Client) Log(ctx context.Context, root string, opts LogOptions) (LogResult, error) {
	args, limit := LogArgs(opts)
	out, err := c.runner().RunGit(ctx, root, args...)
	if err != nil {
		return LogResult{}, err
	}
	return LogResult{Range: opts.Range, Limit: limit, IncludeBody: opts.IncludeBody, Commits: ParseLog(out, opts.IncludeBody)}, nil
}

func LogArgs(opts LogOptions) ([]string, int) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	args := []string{"log", "-n", strconv.Itoa(limit), "--date=iso-strict", "--pretty=format:%H%x1f%an%x1f%aI%x1f%s%x1f%b%x1e"}
	if opts.Range != "" {
		args = append(args, opts.Range)
	}
	return args, limit
}

func ParseLog(out []byte, includeBody bool) []Commit {
	trimmed := bytes.TrimRight(out, "\x1e\n")
	if len(trimmed) == 0 {
		return nil
	}
	records := bytes.Split(trimmed, []byte("\x1e"))
	commits := make([]Commit, 0, len(records))
	for _, record := range records {
		parts := strings.SplitN(strings.TrimLeft(string(record), "\n"), "\x1f", 5)
		if len(parts) < 5 {
			continue
		}
		commit := Commit{Hash: parts[0], Author: parts[1], Date: parts[2], Subject: parts[3]}
		if includeBody {
			commit.Body = strings.TrimSpace(parts[4])
		}
		commits = append(commits, commit)
	}
	return commits
}

type MergeBaseResult struct {
	Base      string `json:"base"`
	Head      string `json:"head"`
	MergeBase string `json:"merge_base"`
}

func (c Client) MergeBase(ctx context.Context, root, base, head string) (MergeBaseResult, error) {
	if base == "" || head == "" {
		return MergeBaseResult{}, fmt.Errorf("base and head are required")
	}
	out, err := c.runner().RunGit(ctx, root, MergeBaseArgs(base, head)...)
	if err != nil {
		return MergeBaseResult{}, err
	}
	return MergeBaseResult{Base: base, Head: head, MergeBase: strings.TrimSpace(string(out))}, nil
}

func MergeBaseArgs(base, head string) []string {
	return []string{"merge-base", base, head}
}
