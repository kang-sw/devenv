// Git diff options, execution, and output formatting.
package wsgit

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
)

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
	output := string(out)
	if opts.Range == "" {
		untracked, err := c.untrackedFiles(ctx, root, opts.Paths)
		if err != nil {
			return DiffResult{}, err
		}
		output = appendUntrackedDiffOutput(output, mode, untracked)
	}
	return DiffResult{Mode: mode, Range: opts.Range, Paths: opts.Paths, Output: output}, nil
}

func (c Client) untrackedFiles(ctx context.Context, root string, paths []string) ([]string, error) {
	args := []string{"ls-files", "--others", "--exclude-standard", "-z"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := c.runner().RunGit(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	return parseNULTerminatedPaths(out), nil
}

func DiffArgs(opts DiffOptions) ([]string, string, error) {
	mode := opts.Mode
	if mode == "" {
		mode = DiffModeStat
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
		if err := validateRevision("range", opts.Range); err != nil {
			return nil, "", err
		}
		args = append(args, opts.Range)
	}
	if len(opts.Paths) > 0 {
		args = append(args, "--")
		args = append(args, opts.Paths...)
	}
	return args, mode, nil
}

func validateRevision(name, value string) error {
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s must be a revision or range, not a git option", name)
	}
	return nil
}

func appendUntrackedDiffOutput(output, mode string, untracked []string) string {
	if len(untracked) == 0 {
		return output
	}
	sort.Strings(untracked)
	var b strings.Builder
	b.WriteString(output)
	if b.Len() > 0 && !strings.HasSuffix(output, "\n") {
		b.WriteString("\n")
	}
	switch mode {
	case DiffModeNameOnly:
		for _, path := range untracked {
			b.WriteString(path)
			b.WriteString("\n")
		}
	default:
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Untracked files:\n")
		for _, path := range untracked {
			b.WriteString("  ")
			b.WriteString(path)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func parseNULTerminatedPaths(out []byte) []string {
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
