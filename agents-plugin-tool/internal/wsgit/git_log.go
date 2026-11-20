// Git log and merge-base queries.
package wsgit

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
)

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
	args, limit, err := LogArgs(opts)
	if err != nil {
		return LogResult{}, err
	}
	out, err := c.runner().RunGit(ctx, root, args...)
	if err != nil {
		return LogResult{}, err
	}
	return LogResult{Range: opts.Range, Limit: limit, IncludeBody: opts.IncludeBody, Commits: ParseLog(out, opts.IncludeBody)}, nil
}

func LogArgs(opts LogOptions) ([]string, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	args := []string{"log", "-n", strconv.Itoa(limit), "--date=iso-strict", "--pretty=format:%H%x1f%an%x1f%aI%x1f%s%x1f%b%x1e"}
	if opts.Range != "" {
		if err := validateRevision("range", opts.Range); err != nil {
			return nil, 0, err
		}
		args = append(args, opts.Range)
	}
	return args, limit, nil
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
	if err := validateRevision("base", base); err != nil {
		return MergeBaseResult{}, err
	}
	if err := validateRevision("head", head); err != nil {
		return MergeBaseResult{}, err
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
