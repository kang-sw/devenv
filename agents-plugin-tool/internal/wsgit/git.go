package wsgit

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
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

type CommitOptions struct {
	Paths               []string `json:"paths"`
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	AIContext           []string `json:"ai_context"`
	MentalModelNotes    []string `json:"mental_model_notes,omitempty"`
	UpdatedTickets      []string `json:"updated_tickets,omitempty"`
	UpdatedSpecs        []string `json:"updated_specs,omitempty"`
	UpdatedMentalModels []string `json:"updated_mental_models,omitempty"`
}

type CommitResult struct {
	Hash          string         `json:"hash"`
	Paths         []string       `json:"paths"`
	Title         string         `json:"title"`
	TicketChanges []TicketChange `json:"ticket_changes,omitempty"`
}

type TicketChange struct {
	Stem          string `json:"stem"`
	Path          string `json:"path"`
	OldPath       string `json:"old_path,omitempty"`
	FromStatus    string `json:"from_status,omitempty"`
	ToStatus      string `json:"to_status,omitempty"`
	ResultAdded   bool   `json:"result_added,omitempty"`
	ResultHeading string `json:"result_heading,omitempty"`
}

func (c Client) Commit(ctx context.Context, root string, opts CommitOptions) (CommitResult, error) {
	opts, err := normalizeCommitOptions(opts)
	if err != nil {
		return CommitResult{}, err
	}
	runner := c.runner()
	preStatusOut, err := runner.RunGit(ctx, root, StatusArgs()...)
	if err != nil {
		return CommitResult{}, err
	}
	preStatus := ParseStatus(preStatusOut)
	opts.Paths = expandCommitPathsForTicketMoves(preStatus, opts.Paths)
	for _, args := range stagingCommandsForCommit(opts.Paths, preStatus) {
		if _, err := runner.RunGit(ctx, root, args...); err != nil {
			return CommitResult{}, err
		}
	}
	statusOut, err := runner.RunGit(ctx, root, StatusArgs()...)
	if err != nil {
		return CommitResult{}, err
	}
	status := ParseStatus(statusOut)
	if err := validateCommitStatus(status, opts.Paths); err != nil {
		return CommitResult{}, err
	}
	ticketChanges := detectTicketChanges(ctx, runner, root)
	if len(opts.UpdatedTickets) == 0 {
		opts.UpdatedTickets = ticketChangeSummaries(ticketChanges)
	}
	message := CommitMessage(opts)
	if _, err := runner.RunGit(ctx, root, "commit", "-m", message); err != nil {
		return CommitResult{}, err
	}
	hashOut, err := runner.RunGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Hash: strings.TrimSpace(string(hashOut)), Paths: opts.Paths, Title: opts.Title, TicketChanges: ticketChanges}, nil
}

func normalizeCommitOptions(opts CommitOptions) (CommitOptions, error) {
	opts.Title = strings.TrimSpace(opts.Title)
	opts.Description = strings.TrimSpace(opts.Description)
	opts.AIContext = trimStrings(opts.AIContext)
	opts.MentalModelNotes = trimStrings(opts.MentalModelNotes)
	opts.UpdatedTickets = trimStrings(opts.UpdatedTickets)
	opts.UpdatedSpecs = trimStrings(opts.UpdatedSpecs)
	opts.UpdatedMentalModels = trimStrings(opts.UpdatedMentalModels)
	if opts.Title == "" {
		return CommitOptions{}, fmt.Errorf("title is required")
	}
	if strings.ContainsAny(opts.Title, "\r\n") {
		return CommitOptions{}, fmt.Errorf("title must be a single line")
	}
	if len(opts.AIContext) == 0 {
		return CommitOptions{}, fmt.Errorf("ai_context requires at least one entry")
	}
	paths := trimStrings(opts.Paths)
	if len(paths) == 0 {
		return CommitOptions{}, fmt.Errorf("paths requires at least one path")
	}
	for _, path := range paths {
		if err := validateCommitPath(path); err != nil {
			return CommitOptions{}, err
		}
	}
	opts.Paths = paths
	return opts, nil
}

func stagingCommandsForCommit(paths []string, status StatusResult) [][]string {
	addPaths := []string{}
	rmPaths := []string{}
	seen := map[string]bool{}
	for _, path := range paths {
		deletedPaths := deletedPathsUnderCommitRoot(status, path)
		if len(deletedPaths) > 0 && !commitRootHasAddableStatus(status, path) {
			for _, deletedPath := range deletedPaths {
				if !seen["rm:"+deletedPath] {
					rmPaths = append(rmPaths, deletedPath)
					seen["rm:"+deletedPath] = true
				}
			}
			continue
		}
		if !seen["add:"+path] {
			addPaths = append(addPaths, path)
			seen["add:"+path] = true
		}
	}
	commands := [][]string{}
	if len(addPaths) > 0 {
		commands = append(commands, append([]string{"add", "-A", "--"}, addPaths...))
	}
	if len(rmPaths) > 0 {
		commands = append(commands, append([]string{"rm", "--cached", "--ignore-unmatch", "--"}, rmPaths...))
	}
	return commands
}

func deletedPathsUnderCommitRoot(status StatusResult, root string) []string {
	root = filepath.ToSlash(filepath.Clean(root))
	seen := map[string]bool{}
	var deleted []string
	for _, file := range status.ChangedFiles {
		if file.WorktreeStatus == "D" || file.IndexStatus == "D" {
			path := filepath.ToSlash(filepath.Clean(file.Path))
			if pathInCommitSet(path, []string{root}) && !seen[path] {
				deleted = append(deleted, path)
				seen[path] = true
			}
		}
		if file.OldPath != "" && (file.WorktreeStatus == "D" || file.IndexStatus == "D" || strings.HasPrefix(file.Status, "R")) {
			path := filepath.ToSlash(filepath.Clean(file.OldPath))
			if pathInCommitSet(path, []string{root}) && !seen[path] {
				deleted = append(deleted, path)
				seen[path] = true
			}
		}
	}
	return deleted
}

func commitRootHasAddableStatus(status StatusResult, root string) bool {
	root = filepath.ToSlash(filepath.Clean(root))
	for _, file := range status.ChangedFiles {
		path := filepath.ToSlash(filepath.Clean(file.Path))
		if !pathInCommitSet(path, []string{root}) {
			continue
		}
		if file.Status == "?" || file.Status == "!" {
			return true
		}
		if file.WorktreeStatus != "D" && file.IndexStatus != "D" {
			return true
		}
	}
	return false
}

func validateCommitPath(path string) error {
	if path == "" {
		return fmt.Errorf("paths may not contain empty entries")
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("path %q must not start with '-'", path)
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return fmt.Errorf("path %q must be relative", path)
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || strings.HasPrefix(filepath.ToSlash(cleaned), "../") {
		return fmt.Errorf("path %q must stay inside the repository", path)
	}
	return nil
}

func validateCommitStatus(status StatusResult, paths []string) error {
	staged := 0
	for _, file := range status.ChangedFiles {
		if strings.HasPrefix(file.Status, "U") {
			return fmt.Errorf("cannot commit while unmerged path %q is present", file.Path)
		}
		if file.IndexStatus == "" || file.IndexStatus == "." {
			continue
		}
		if !pathInCommitSet(file.Path, paths) && (file.OldPath == "" || !pathInCommitSet(file.OldPath, paths)) {
			return fmt.Errorf("refusing to commit unrelated staged path %q", file.Path)
		}
		staged++
	}
	if staged == 0 {
		return fmt.Errorf("no staged changes in requested paths")
	}
	return nil
}

func expandCommitPathsForTicketMoves(status StatusResult, paths []string) []string {
	stems := map[string]bool{}
	for _, path := range paths {
		if _, stem, ok := ticketStatusStem(path); ok {
			stems[stem] = true
		}
	}
	if len(stems) == 0 {
		return paths
	}
	expanded := append([]string(nil), paths...)
	seen := map[string]bool{}
	for _, path := range paths {
		seen[filepath.ToSlash(filepath.Clean(path))] = true
	}
	for _, file := range status.ChangedFiles {
		for _, path := range []string{file.Path, file.OldPath} {
			if path == "" {
				continue
			}
			_, stem, ok := ticketStatusStem(path)
			if !ok || !stems[stem] {
				continue
			}
			cleaned := filepath.ToSlash(filepath.Clean(path))
			if !seen[cleaned] {
				expanded = append(expanded, path)
				seen[cleaned] = true
			}
		}
	}
	return expanded
}

func pathInCommitSet(path string, roots []string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	for _, root := range roots {
		root = filepath.ToSlash(filepath.Clean(root))
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func CommitMessage(opts CommitOptions) string {
	var b strings.Builder
	b.WriteString(opts.Title)
	if opts.Description != "" {
		b.WriteString("\n\n")
		b.WriteString(opts.Description)
	}
	b.WriteString("\n\n## AI Context\n")
	for _, item := range opts.AIContext {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	writeCommitSubsection(&b, "### Mental Model Notes", opts.MentalModelNotes)
	writeCommitSection(&b, "## Updated Tickets", opts.UpdatedTickets)
	writeCommitSection(&b, "## Updated Specs", opts.UpdatedSpecs)
	writeCommitSection(&b, "## Updated Mental Models", opts.UpdatedMentalModels)
	return strings.TrimRight(b.String(), "\n")
}

func writeCommitSection(b *strings.Builder, heading string, values []string) {
	if len(values) == 0 {
		return
	}
	b.WriteString("\n\n")
	writeCommitHeading(b, heading, values)
}

func writeCommitSubsection(b *strings.Builder, heading string, values []string) {
	if len(values) == 0 {
		return
	}
	b.WriteByte('\n')
	writeCommitHeading(b, heading, values)
}

func writeCommitHeading(b *strings.Builder, heading string, values []string) {
	b.WriteString(heading)
	b.WriteByte('\n')
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func detectTicketChanges(ctx context.Context, runner Runner, root string) []TicketChange {
	var changes []TicketChange
	if out, err := runner.RunGit(ctx, root, "diff", "--cached", "--name-status", "--", "ai-docs/tickets"); err == nil {
		changes = append(changes, parseTicketNameStatus(out)...)
	}
	if out, err := runner.RunGit(ctx, root, "diff", "--cached", "--unified=0", "--", "ai-docs/tickets"); err == nil {
		for _, change := range parseTicketResultAdditions(out) {
			changes = mergeTicketResultAddition(changes, change)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		return ticketChangeSortKey(changes[i]) < ticketChangeSortKey(changes[j])
	})
	return changes
}

func mergeTicketResultAddition(changes []TicketChange, result TicketChange) []TicketChange {
	for i, change := range changes {
		if change.Stem == result.Stem && change.Path == result.Path {
			changes[i].ResultAdded = result.ResultAdded
			changes[i].ResultHeading = result.ResultHeading
			return changes
		}
	}
	return append(changes, result)
}

func parseTicketNameStatus(out []byte) []TicketChange {
	var changes []TicketChange
	addsByStem := map[string][]TicketChange{}
	deletesByStem := map[string][]TicketChange{}
	explicitRenameStems := map[string]bool{}
	var passthrough []TicketChange

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		code := fields[0]
		path := fields[1]
		oldPath := ""
		if strings.HasPrefix(code, "R") && len(fields) >= 3 {
			oldPath = fields[1]
			path = fields[2]
		}
		change, ok := ticketChangeForPath(path)
		if !ok {
			continue
		}
		if oldPath != "" {
			if oldChange, ok := ticketChangeForPath(oldPath); ok {
				change.OldPath = oldPath
				change.FromStatus = oldChange.ToStatus
				explicitRenameStems[change.Stem] = true
			}
			changes = append(changes, change)
			continue
		}

		switch code {
		case "A":
			addsByStem[change.Stem] = append(addsByStem[change.Stem], change)
		case "D":
			deletesByStem[change.Stem] = append(deletesByStem[change.Stem], change)
		default:
			passthrough = append(passthrough, change)
		}
	}

	pairedAdds := map[string]bool{}
	pairedDeletes := map[string]bool{}
	for stem, adds := range addsByStem {
		deletes := deletesByStem[stem]
		if explicitRenameStems[stem] || len(adds) != 1 || len(deletes) != 1 {
			continue
		}
		add := adds[0]
		deleteChange := deletes[0]
		if add.ToStatus == deleteChange.ToStatus {
			continue
		}
		add.OldPath = deleteChange.Path
		add.FromStatus = deleteChange.ToStatus
		changes = append(changes, add)
		pairedAdds[stem] = true
		pairedDeletes[stem] = true
	}

	for stem, adds := range addsByStem {
		if pairedAdds[stem] {
			continue
		}
		for _, change := range adds {
			changes = append(changes, change)
		}
	}
	for stem, deletes := range deletesByStem {
		if pairedDeletes[stem] {
			continue
		}
		for _, change := range deletes {
			changes = append(changes, change)
		}
	}
	for _, change := range passthrough {
		changes = append(changes, change)
	}
	return changes
}

func parseTicketResultAdditions(out []byte) []TicketChange {
	var changes []TicketChange
	var current TicketChange
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			current = TicketChange{}
			if change, ok := ticketChangeForPath(strings.TrimPrefix(line, "+++ b/")); ok {
				current = change
			}
			continue
		}
		if current.Stem == "" || !isAddedTicketResultHeading(line) {
			continue
		}
		change := current
		change.ResultAdded = true
		change.ResultHeading = strings.TrimPrefix(line, "+")
		changes = append(changes, change)
	}
	return changes
}

func isAddedTicketResultHeading(line string) bool {
	return strings.HasPrefix(line, "+### Result") || strings.HasPrefix(line, "+#### Edition")
}

func ticketChangeForPath(path string) (TicketChange, bool) {
	status, stem, ok := ticketStatusStem(path)
	if !ok {
		return TicketChange{}, false
	}
	return TicketChange{Stem: stem, Path: path, ToStatus: status}, true
}

func ticketStatusStem(path string) (string, string, bool) {
	normalized := filepath.ToSlash(filepath.Clean(path))
	const prefix = "ai-docs/tickets/"
	if !strings.HasPrefix(normalized, prefix) || !strings.HasSuffix(normalized, ".md") {
		return "", "", false
	}
	rest := strings.TrimPrefix(normalized, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	status := parts[0]
	switch status {
	case "idea", "todo", "ready", "wip", ".done", ".dropped":
	default:
		return "", "", false
	}
	stem := strings.TrimSuffix(parts[1], ".md")
	if stem == "" {
		return "", "", false
	}
	return status, stem, true
}

func ticketChangeSortKey(change TicketChange) string {
	return strings.Join([]string{change.Stem, change.Path, change.OldPath, change.FromStatus, change.ToStatus}, "\x00")
}

func ticketChangeSummaries(changes []TicketChange) []string {
	summaries := make([]string, 0, len(changes))
	for _, change := range changes {
		switch {
		case change.FromStatus != "" && change.FromStatus != change.ToStatus && change.ResultAdded:
			summaries = append(summaries, fmt.Sprintf("%s: moved %s -> %s and added %s", change.Stem, change.FromStatus, change.ToStatus, change.ResultHeading))
		case change.FromStatus != "" && change.FromStatus != change.ToStatus:
			summaries = append(summaries, fmt.Sprintf("%s: moved %s -> %s", change.Stem, change.FromStatus, change.ToStatus))
		case change.ResultAdded:
			summaries = append(summaries, fmt.Sprintf("%s: added %s", change.Stem, change.ResultHeading))
		}
	}
	return summaries
}
