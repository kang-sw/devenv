package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswake"
)

// hostedCLI is the shared one-shot CLI core, not a process. Its flags, input,
// output, git environment and MCP options never use daemon globals. Only the
// manual CLI's lifecycle commands remain process entry points in main.go.
type hostedCLI struct {
	hello          wsdashboard.ProxyHello
	host           *proxyHost
	ctx            context.Context
	stdout, stderr io.Writer
	streamout      io.Writer // live stdout frames, only for an explicitly streaming command
	stdin          io.Reader
	env            []string
	serverReady    func(*mcp.Server)
}

func (c *hostedCLI) getenv(key string) string { return c.hello.Env[key] }
func (c *hostedCLI) input() io.Reader {
	if c.stdin != nil {
		return c.stdin
	}
	return strings.NewReader(c.hello.Stdin)
}
func (c *hostedCLI) absolute(path string) string {
	if !filepath.IsAbs(path) {
		return filepath.Join(c.hello.Cwd, path)
	}
	return path
}
func (c *hostedCLI) root(root string) string {
	if root == "" || root == "." {
		root = c.hello.ProjectRoot
		if root == "" {
			root = defaultRootFrom(".", c.getenv)
		}
	}
	return c.absolute(root)
}
func (c *hostedCLI) config() wsconfig.Options {
	return wsconfig.Options{ConfigHome: c.getenv(wsconfig.EnvConfigHome), CacheHome: c.getenv(wsstate.EnvCacheHome), RepoRoot: c.root(".")}
}
func (c *hostedCLI) state() wsstate.Options {
	return wsstate.Options{CacheHome: c.getenv(wsstate.EnvCacheHome), Env: c.env}
}
func (c *hostedCLI) paths() wsmailbox.Paths {
	return wsmailbox.Paths{Config: c.config(), State: c.state()}
}
func (c *hostedCLI) runner() wsgit.ExecRunner {
	return wsgit.ExecRunner{Environ: func() []string { return c.env }}
}
func (c *hostedCLI) server(root string) *mcp.Server {
	opts := mcp.ServerOptions{Root: root, Version: version, SourceCommit: sourceCommit,
		Env: c.env, OwnerPID: c.hello.OwnerPID, Harness: c.hello.Harness, Diag: c.stderr}
	if c.host != nil {
		if c.host.agents != nil && strings.TrimSpace(c.getenv(wsagent.EnvPiToken)) == "" {
			opts.Agents = c.host.agents
		}
		if d := c.host.live.Load(); d != nil {
			opts.Live = d
		}
	} else {
		// A manually invoked CLI retains executable-relative resource lookup.
		opts.Env = nil
	}
	server := mcp.NewServerWithOptions(opts)
	if c.serverReady != nil {
		c.serverReady(server)
	}
	return server
}
func (c *hostedCLI) failed(command string, err error) int {
	fmt.Fprintf(c.stderr, "ws-mcp %s: %v\n", command, err)
	return 1
}
func (c *hostedCLI) output(command, format string, value any, text string, err error) int {
	if err != nil {
		return c.failed(command, err)
	}
	if outputJSON(format) {
		if err := json.NewEncoder(c.stdout).Encode(value); err != nil {
			return c.failed(command, err)
		}
	} else {
		fmt.Fprint(c.stdout, text)
	}
	return 0
}
func (c *hostedCLI) flags(command string) *flag.FlagSet {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	return fs
}

func (c *hostedCLI) run() int {
	args := c.hello.Argv
	if len(args) == 0 {
		return c.usage()
	}
	if c.ctx.Err() != nil {
		return mailboxWaitExitInterrupted
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(c.stdout, version)
		return 0
	case "mailbox":
		if len(args) < 2 {
			return c.usage()
		}
		switch args[1] {
		case "wait":
			getenv := func(key string) string {
				if key == "WS_MCP_PROJECT_ROOT" {
					return c.root(".")
				}
				return c.getenv(key)
			}
			return runMailboxWait(c.ctx, args[2:], c.stdout, c.stderr, c.hello.Cwd, c.hello.OwnerPID, c.env, getenv, wswake.Mailbox, c.streamout)
		case "codex-stop-hook":
			getenv := func(key string) string {
				if key == "WS_MCP_PROJECT_ROOT" {
					return c.root(".")
				}
				return c.getenv(key)
			}
			return runCodexStopHook(args[2:], c.input(), c.stdout, c.stderr, c.hello.Cwd, getenv, c.paths())
		}
	case "tools", "call":
		return c.mcpCommand(args[0], args[1:])
	case "runtime":
		if len(args) < 2 {
			return c.usage()
		}
		fs := c.flags("runtime " + args[1])
		if err := fs.Parse(args[2:]); err != nil {
			return 2
		}
		switch args[1] {
		case "info":
			return c.output("runtime info", "json", map[string]string{"version": version, "source_commit": sourceCommit}, "", nil)
		case "capabilities":
			return c.output("runtime capabilities", "json", runtimeCapabilitiesPayload{Version: version, SourceCommit: sourceCommit, MCPProtocol: mcp.ProtocolVersion, Tools: mcp.LeadToolNamesFromEnv(c.env), Commands: runtimeCapabilityCommandNames()}, "", nil)
		}
	case "doctor", "smoke", "serve":
		fs := c.flags(args[0])
		root := fs.String("root", ".", "repository root")
		var stdio bool
		if args[0] == "serve" {
			fs.BoolVar(&stdio, "stdio", false, "serve MCP over stdin/stdout")
		}
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if args[0] == "serve" {
			if !stdio {
				return c.failed("serve", fmt.Errorf("only --stdio is supported"))
			}
			server := c.server(c.root(*root))
			defer server.Close()
			if err := server.ServeStdio(c.ctx, c.input(), c.stdout); err != nil {
				return c.failed("serve", err)
			}
			return 0
		}
		if args[0] == "smoke" {
			fmt.Fprintf(c.stdout, "version: %s\n", version)
		}
		report := wsdoc.Doctor(c.root(*root))
		for _, line := range report.Lines {
			fmt.Fprintln(c.stdout, line)
		}
		if !report.OK {
			return 1
		}
		if args[0] == "doctor" {
			return 0
		}
		server := c.server(c.root(*root))
		defer server.Close()
		var responses bytes.Buffer
		input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"runtime.read","arguments":{}}}` + "\n" +
			`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"project_tree","arguments":{}}}` + "\n"
		if err := server.ServeStdio(c.ctx, strings.NewReader(input), &responses); err != nil {
			return c.failed("smoke", err)
		}
		for _, want := range []string{"runtime.read", "project_tree", "ai-docs/"} {
			if !strings.Contains(responses.String(), want) {
				return c.failed("smoke", fmt.Errorf("stdio smoke response missing %q", want))
			}
		}
		fmt.Fprintf(c.stdout, "ok stdio smoke: %d bytes\n", responses.Len())
		return 0
	case "config":
		return c.configCommand(args[1:])
	case "path":
		if len(args) < 2 || args[1] != "generate" {
			return c.usage()
		}
		fs := c.flags("path generate")
		root := fs.String("root", ".", "repository root")
		kind := fs.String("kind", "review", "generated path kind")
		if err := fs.Parse(args[2:]); err != nil {
			return 2
		}
		paths, err := wsstate.NewManager(c.state()).GeneratePaths(c.root(*root), *kind, fs.Args())
		if err != nil {
			return c.failed("path generate", err)
		}
		for _, path := range paths {
			fmt.Fprintln(c.stdout, path.Path)
		}
		return 0
	case "git":
		return c.gitCommand(args[1:])
	case "tickets":
		return c.ticketsCommand(args[1:])
	}
	return c.usage()
}

func (c *hostedCLI) usage() int {
	fmt.Fprintln(c.stderr, "usage: ws-mcp <version|doctor|runtime|serve|smoke|config|path|git|tickets|mailbox|dashboard|daemon-start>")
	return 2
}

func (c *hostedCLI) configCommand(args []string) int {
	if len(args) == 0 {
		return c.usage()
	}
	fs := c.flags("config " + args[0])
	switch args[0] {
	case "list":
		format := fs.String("format", "", "output format: text or json")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		view, err := wsconfig.ShowResolved(c.config())
		return c.output("config list", *format, view, mcp.FormatConfigView(view), err)
	case "tune":
		tier := fs.String("tier", "", "capability tier")
		backend := fs.String("backend", "", "backend name")
		model := fs.String("model", "", "concrete model")
		effort := fs.String("effort", "", "reasoning effort")
		harness := fs.String("harness", "", "harness alias key")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		var efforts []string
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "effort" {
				efforts = append(efforts, *effort)
			}
		})
		cfg, err := wsconfig.SetAgentsTierForHarness(c.config(), *tier, *backend, *model, *harness, efforts...)
		return c.output("config tune", "json", cfg, "", err)
	}
	return c.usage()
}

func (c *hostedCLI) gitCommand(args []string) int {
	if len(args) == 0 {
		return c.usage()
	}
	command := "git " + args[0]
	fs := c.flags(command)
	root := fs.String("root", ".", "repository root")
	format := fs.String("format", "", "output format: text or json")
	client := wsgit.Client{Runner: c.runner(), Verifier: mcp.VerifyAdapter}
	switch args[0] {
	case "status":
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if outputJSON(*format) {
			result, err := client.Status(c.ctx, c.root(*root))
			return c.output(command, *format, result, "", err)
		}
		out, err := c.runner().RunGit(c.ctx, c.root(*root), "status", "--short", "--branch")
		return c.output(command, "", nil, string(out), err)
	case "diff":
		rangeValue := fs.String("range", "", "revision range")
		mode := fs.String("mode", wsgit.DiffModeStat, "diff mode")
		var paths multiFlag
		fs.Var(&paths, "path", "path filter; may be repeated")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		paths = append(paths, fs.Args()...)
		result, err := client.Diff(c.ctx, c.root(*root), wsgit.DiffOptions{Range: *rangeValue, Mode: *mode, Paths: paths})
		return c.output(command, *format, result, result.Output, err)
	case "log":
		rangeValue := fs.String("range", "", "revision range")
		limit := fs.Int("limit", 20, "maximum commits")
		body := fs.Bool("include-body", false, "include commit body")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		result, err := client.Log(c.ctx, c.root(*root), wsgit.LogOptions{Range: *rangeValue, Limit: *limit, IncludeBody: *body})
		if outputJSON(*format) || err != nil {
			return c.output(command, *format, result, "", err)
		}
		if strings.HasPrefix(*rangeValue, "-") {
			return c.failed(command, fmt.Errorf("range must be a revision or range, not a git option"))
		}
		if *limit <= 0 {
			*limit = 20
		}
		if *limit > 100 {
			*limit = 100
		}
		gitArgs := []string{"log", "-n", fmt.Sprint(*limit), "--date=iso-strict"}
		if *rangeValue != "" {
			gitArgs = append(gitArgs, *rangeValue)
		}
		out, err := c.runner().RunGit(c.ctx, c.root(*root), gitArgs...)
		return c.output(command, "", nil, string(out), err)
	case "merge-base":
		base := fs.String("base", "", "base revision")
		head := fs.String("head", "", "head revision")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *base == "" && fs.NArg() > 0 {
			*base = fs.Arg(0)
		}
		if *head == "" && fs.NArg() > 1 {
			*head = fs.Arg(1)
		}
		result, err := client.MergeBase(c.ctx, c.root(*root), *base, *head)
		if outputJSON(*format) || err != nil {
			return c.output(command, *format, result, "", err)
		}
		out, err := c.runner().RunGit(c.ctx, c.root(*root), "merge-base", *base, *head)
		return c.output(command, "", nil, string(out), err)
	case "commit":
		title := fs.String("title", "", "single-line commit title")
		description := fs.String("description", "", "commit description body")
		file := fs.String("description-file", "", "commit description file; - for stdin")
		branch := fs.String("expected-branch", "", "expected checkout branch")
		var paths, aiContext, tickets multiFlag
		fs.Var(&paths, "path", "path to stage and commit; may be repeated")
		fs.Var(&aiContext, "ai-context", "AI Context bullet; may be repeated")
		fs.Var(&tickets, "updated-ticket", "ticket update summary; may be repeated")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		paths = append(paths, fs.Args()...)
		if *file == "-" {
			raw, err := io.ReadAll(c.input())
			if err != nil {
				return c.failed(command, fmt.Errorf("read stdin description: %w", err))
			}
			*description = string(raw)
		} else if *file != "" {
			raw, err := os.ReadFile(c.absolute(*file))
			if err != nil {
				return c.failed(command, fmt.Errorf("read description file: %w", err))
			}
			*description = string(raw)
		}
		result, err := client.Commit(c.ctx, c.root(*root), wsgit.CommitOptions{Paths: paths, Title: *title, Description: *description, ExpectedBranch: *branch, AIContext: aiContext, UpdatedTickets: tickets})
		return c.output(command, *format, result, mcp.FormatGitCommit(result), err)
	}
	return c.usage()
}

func (c *hostedCLI) ticketsCommand(args []string) int {
	if len(args) == 0 {
		return c.usage()
	}
	command := "tickets " + args[0]
	fs := c.flags(command)
	root := fs.String("root", ".", "repository root")
	switch args[0] {
	case "list", "find", "status":
		format := fs.String("format", "", "output format: text or json")
		done := fs.Bool("include-done", false, "include archived done tickets")
		dropped := fs.Bool("include-dropped", false, "include archived dropped tickets")
		var statuses multiFlag
		var query, stem, mentions string
		if args[0] != "status" {
			fs.Var(&statuses, "status", "ticket status; may be repeated")
		}
		if args[0] != "list" {
			fs.StringVar(&stem, "ticket-stem", "", "ticket stem")
		}
		if args[0] == "find" {
			fs.StringVar(&query, "query", "", "case-insensitive text query")
			fs.StringVar(&mentions, "mentions-ticket-stem", "", "mentioned ticket stem")
		}
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		var result []wsdoc.TicketInfo
		var err error
		switch args[0] {
		case "list":
			result, err = wsdoc.TicketsList(c.root(*root), wsdoc.TicketListOptions{Statuses: statuses, IncludeDone: *done, IncludeDropped: *dropped, Env: c.env})
		case "find":
			result, err = wsdoc.TicketsFind(c.root(*root), wsdoc.TicketFindOptions{Statuses: statuses, IncludeDone: *done, IncludeDropped: *dropped, Query: query, TicketStem: stem, MentionsTicketStem: mentions, Resolve: strings.TrimSpace(stem) != "", Env: c.env})
		case "status":
			if stem == "" && fs.NArg() > 0 {
				stem = fs.Arg(0)
			}
			item, statusErr := wsdoc.TicketsStatus(c.root(*root), wsdoc.TicketStatusOptions{TicketStem: stem, IncludeDone: *done, IncludeDropped: *dropped, Resolve: true, Env: c.env})
			if outputJSON(*format) {
				return c.output(command, *format, item, "", statusErr)
			}
			err = statusErr
			if item != nil {
				result = append(result, *item)
			}
		}
		return c.output(command, *format, result, mcp.FormatTickets(result), err)
	case "close", "move":
		stem := fs.String("stem", "", "ticket stem")
		var status, resolution, to string
		if args[0] == "close" {
			fs.StringVar(&status, "status", "", "close target: done or dropped")
			fs.StringVar(&resolution, "resolution", "", "optional resolution")
		} else {
			fs.StringVar(&to, "to", "", "target status: idea, todo or ready")
		}
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *stem == "" && fs.NArg() > 0 {
			*stem = fs.Arg(0)
		}
		var result wsdoc.TicketMutateResult
		var err error
		verb := "closed"
		if args[0] == "close" {
			result, err = wsdoc.TicketsClose(c.root(*root), c.runner(), wsdoc.TicketCloseOptions{TicketStem: *stem, Status: status, Resolution: resolution, Today: time.Now().Format("2006-01-02"), Env: c.env})
		} else {
			verb = "moved"
			opts := c.config()
			opts.RepoRoot = c.root(*root)
			resolver := wsconfig.NewResolver(opts, nil, nil, nil)
			design, _ := resolver.Get("", wsconfig.ItemSageReviewDesign)
			completeness, _ := resolver.Get("", wsconfig.ItemSageReview)
			result, err = wsdoc.TicketsMove(c.root(*root), c.runner(), wsdoc.TicketMoveOptions{TicketStem: *stem, To: to, SageReview: wsdoc.SageReviewConfig{Design: design.Value, Completeness: completeness.Value}, Env: c.env})
		}
		return c.output(command, "", nil, mcp.FormatTicketMutate(verb, result), withPartialMutationNotice(err, result.PartialMutationNotice))
	case "create-empty":
		stem := fs.String("stem", "", "semantic ticket stem without date prefix")
		state := fs.String("initial-state", "", "ticket status")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		rest := fs.Args()
		if *stem == "" && len(rest) > 0 {
			*stem, rest = rest[0], rest[1:]
		}
		if *state == "" && len(rest) > 0 {
			*state = rest[0]
		}
		result, err := wsdoc.TicketCreate(c.root(*root), wsdoc.TicketCreateOptions{Stem: *stem, InitialState: *state, Env: c.env})
		return c.output(command, "", nil, mcp.FormatTicketCreate(result), err)
	case "verify":
		format := fs.String("format", "", "output format: text or json")
		var paths multiFlag
		fs.Var(&paths, "path", "ticket path; may be repeated")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		paths = append(paths, fs.Args()...)
		result, err := wsdoc.TicketVerify(c.root(*root), paths)
		return c.output(command, *format, result, mcp.FormatTicketVerify(result), err)
	}
	return c.usage()
}

func (c *hostedCLI) mcpLine(server *mcp.Server, method string, params any) (mcpLineResponse, error) {
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		return mcpLineResponse{}, err
	}
	var out bytes.Buffer
	if err := server.ServeStdio(c.ctx, bytes.NewReader(append(raw, '\n')), &out); err != nil {
		return mcpLineResponse{}, err
	}
	var resp mcpLineResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		return resp, err
	}
	if resp.Error != nil {
		return resp, fmt.Errorf("%s", resp.Error.Message)
	}
	return resp, nil
}

func (c *hostedCLI) mcpCommand(command string, args []string) int {
	if (command == "call" && len(args) != 2) || (command == "tools" && len(args) > 1) {
		return c.usage()
	}
	server := c.server(c.root("."))
	defer server.Close()
	if command == "call" {
		var probe any
		if err := json.Unmarshal([]byte(args[1]), &probe); err != nil {
			return c.failed("call", fmt.Errorf("malformed JSON arguments: %w", err))
		}
		resp, err := c.mcpLine(server, "tools/call", map[string]any{"name": args[0], "arguments": json.RawMessage(args[1])})
		if err != nil {
			return c.failed("call "+args[0], err)
		}
		var result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return c.failed("call", err)
		}
		text := ""
		if len(result.Content) > 0 {
			text = result.Content[0].Text
		}
		if result.IsError {
			fmt.Fprint(c.stderr, text)
			if !strings.HasSuffix(text, "\n") {
				fmt.Fprintln(c.stderr)
			}
			return 1
		}
		fmt.Fprint(c.stdout, text)
		return 0
	}
	resp, err := c.mcpLine(server, "tools/list", map[string]any{})
	if err != nil {
		return c.failed("tools", err)
	}
	var result struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return c.failed("tools", err)
	}
	if len(args) == 0 {
		fmt.Fprintln(c.stdout, toolsMappingRule)
	}
	for _, tool := range result.Tools {
		name, _ := tool["name"].(string)
		if len(args) == 0 {
			fmt.Fprintf(c.stdout, "%s: %s\n", name, tool["description"])
		} else if name == args[0] {
			return c.output("tools", "json", tool["inputSchema"], "", nil)
		}
	}
	if len(args) != 0 {
		return c.failed("tools", fmt.Errorf("tool not found: %s", args[0]))
	}
	return 0
}
