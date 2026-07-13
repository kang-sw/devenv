package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsstate"
)

var version = "0.33.9-dev"
var sourceCommit = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version)
	case "doctor":
		doctor(os.Args[2:])
	case "runtime":
		runtime(os.Args[2:])
	case "serve":
		serve(os.Args[2:])
	case "smoke":
		smoke(os.Args[2:])
	case "config":
		configCommand(os.Args[2:])
	case "path":
		path(os.Args[2:])
	case "mercenary":
		fatalIfNoAgentCommand("mercenary")
		agents(os.Args[2:])
	case "git":
		gitCommand(os.Args[2:])
	case "tickets":
		ticketsCommand(os.Args[2:])
	case "specs":
		specsCommand(os.Args[2:])
	case "mental-models":
		mentalModelsCommand(os.Args[2:])
	case "references":
		referencesCommand(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	if mcp.NoAgentMode() {
		fmt.Fprintln(os.Stderr, "usage: ws-mcp <version|doctor|runtime|serve|smoke|config|path|git|tickets|specs|mental-models|references>")
		return
	}
	fmt.Fprintln(os.Stderr, "usage: ws-mcp <version|doctor|runtime|serve|smoke|config|path|mercenary|git|tickets|specs|mental-models|references>")
}

func doctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	_ = fs.Parse(args)

	report := wsdoc.Doctor(defaultRoot(*root))
	for _, line := range report.Lines {
		fmt.Println(line)
	}
	if !report.OK {
		os.Exit(1)
	}
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	stdio := fs.Bool("stdio", false, "serve MCP over stdin/stdout")
	root := fs.String("root", ".", "repository root")
	_ = fs.Parse(args)

	if !*stdio {
		fmt.Fprintln(os.Stderr, "ws-mcp serve: only --stdio is supported")
		os.Exit(2)
	}

	server := mcp.NewServer(defaultRoot(*root), version, sourceCommit)
	if err := server.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp serve: %v\n", err)
		os.Exit(1)
	}
}

func smoke(args []string) {
	fs := flag.NewFlagSet("smoke", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	_ = fs.Parse(args)

	if err := runSmoke(defaultRoot(*root), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp smoke: %v\n", err)
		os.Exit(1)
	}
}

func runSmoke(root string, out io.Writer) error {
	fmt.Fprintf(out, "version: %s\n", version)

	report := wsdoc.Doctor(root)
	for _, line := range report.Lines {
		fmt.Fprintln(out, line)
	}
	if !report.OK {
		return fmt.Errorf("doctor failed")
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"project_tree","arguments":{}}}`,
	}, "\n") + "\n"
	var responses bytes.Buffer
	server := mcp.NewServer(root, version, sourceCommit)
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &responses); err != nil {
		return fmt.Errorf("stdio smoke failed: %w", err)
	}
	text := responses.String()
	for _, want := range []string{"runtime.info", "project_tree", "ai-docs/"} {
		if !strings.Contains(text, want) {
			return fmt.Errorf("stdio smoke response missing %q", want)
		}
	}
	fmt.Fprintf(out, "ok stdio smoke: %d bytes\n", responses.Len())
	return nil
}

func runtime(args []string) {
	if len(args) < 1 {
		runtimeUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "info":
		runtimeInfo(args[1:])
	case "capabilities":
		runtimeCapabilities(args[1:])
	default:
		runtimeUsage()
		os.Exit(2)
	}
}

func runtimeUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp runtime <info|capabilities>")
}

func runtimeInfo(args []string) {
	fs := flag.NewFlagSet("runtime info", flag.ExitOnError)
	_ = fs.Parse(args)

	fmt.Printf("{\"version\":%q,\"source_commit\":%q}\n", version, sourceCommit)
}

type runtimeCapabilitiesPayload struct {
	Version      string   `json:"version"`
	SourceCommit string   `json:"source_commit"`
	MCPProtocol  string   `json:"mcp_protocol"`
	Tools        []string `json:"tools"`
	Commands     []string `json:"commands"`
}

func runtimeCapabilities(args []string) {
	fs := flag.NewFlagSet("runtime capabilities", flag.ExitOnError)
	_ = fs.Parse(args)

	payload := runtimeCapabilitiesPayload{
		Version:      version,
		SourceCommit: sourceCommit,
		MCPProtocol:  mcp.ProtocolVersion,
		Tools:        mcp.LeadToolNames(),
		Commands:     runtimeCapabilityCommandNames(),
	}
	printJSONOrFatal("runtime capabilities", payload, nil)
}

func runtimeCapabilityCommandNames() []string {
	commands := []string{
		"mercenary.call",
		"mercenary.cancel",
		"mercenary.check-inbox",
		"mercenary.debug.events",
		"mercenary.debug.runtime-log",
		"mercenary.debug.stderr",
		"mercenary.debug.stdout",
		"mercenary.debug.tail",
		"mercenary.erase",
		"mercenary.interrupt",
		"mercenary.print",
		"mercenary.register",
		"mercenary.result",
		"mercenary.run-current",
		"mercenary.status",
		"mercenary.tail",
		"mercenary.wait",
		"config.agents-tier",
		"config.show",
		"git.commit",
		"git.diff",
		"git.log",
		"git.merge-base",
		"git.status",
		"mental-models.find",
		"mental-models.status",
		"path.generate",
		"references.trace",
		"runtime.capabilities",
		"runtime.info",
		"smoke",
		"specs.find",
		"specs.list",
		"specs.status",
		"tickets.close",
		"tickets.create",
		"tickets.find",
		"tickets.list",
		"tickets.move",
		"tickets.status",
	}
	if mcp.NoAgentMode() {
		commands = filterNoAgentCommands(commands)
	}
	sort.Strings(commands)
	return commands
}

func filterNoAgentCommands(commands []string) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		if strings.HasPrefix(command, "mercenary.") || command == "config.agents-tier" {
			continue
		}
		out = append(out, command)
	}
	return out
}

func fatalIfNoAgentCommand(command string) {
	if !mcp.NoAgentMode() {
		return
	}
	fatal(command, fmt.Errorf("%s agentless mode disables agent-backed command: %s", mcp.RuntimeNamespace(), command))
}

func configCommand(args []string) {
	if len(args) < 1 {
		configUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "show":
		configShow(args[1:])
	case "agents-tier":
		fatalIfNoAgentCommand("config agents-tier")
		configAgentsTier(args[1:])
	default:
		configUsage()
		os.Exit(2)
	}
}

func configUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp config <show|agents-tier>")
}

func configShow(args []string) {
	fs := flag.NewFlagSet("config show", flag.ExitOnError)
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	view, err := wsconfig.Show(wsconfig.Options{})
	if outputJSON(*format) {
		printJSONOrFatal("config show", view, err)
		return
	}
	printTextOrFatal("config show", mcp.FormatConfigView(view), err)
}

func configAgentsTier(args []string) {
	fs := flag.NewFlagSet("config agents-tier", flag.ExitOnError)
	tier := fs.String("tier", "", "capability tier: small, medium, large, or xlarge")
	backend := fs.String("backend", "", "backend name; inferred from model when omitted")
	model := fs.String("model", "", "concrete model for this alias")
	effort := fs.String("effort", "", "portable reasoning effort for this alias: none, low, medium, high, or xhigh")
	harness := fs.String("harness", "", "harness alias key to configure: codex, claude, or default")
	_ = fs.Parse(args)

	var effortProvided bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "effort" {
			effortProvided = true
		}
	})
	var cfg wsconfig.Config
	var err error
	if effortProvided {
		cfg, err = wsconfig.SetAgentsTierForHarness(wsconfig.Options{}, *tier, *backend, *model, *harness, *effort)
	} else {
		cfg, err = wsconfig.SetAgentsTierForHarness(wsconfig.Options{}, *tier, *backend, *model, *harness)
	}
	printJSONOrFatal("config agents-tier", cfg, err)
}

func path(args []string) {
	if len(args) < 1 {
		pathUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "generate":
		pathGenerate(args[1:])
	default:
		pathUsage()
		os.Exit(2)
	}
}

func pathUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp path <generate>")
}

func pathGenerate(args []string) {
	fs := flag.NewFlagSet("path generate", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	kind := fs.String("kind", "review", "generated path kind")
	_ = fs.Parse(args)

	paths, err := wsstate.NewManager(wsstate.Options{}).GeneratePaths(defaultRoot(*root), *kind, fs.Args())
	if err != nil {
		fatal("path generate", err)
	}
	for _, path := range paths {
		fmt.Println(path.Path)
	}
}

func gitCommand(args []string) {
	if len(args) < 1 {
		gitUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "status":
		gitStatus(args[1:])
	case "diff":
		gitDiff(args[1:])
	case "log":
		gitLog(args[1:])
	case "merge-base":
		gitMergeBase(args[1:])
	case "commit":
		gitCommit(args[1:])
	default:
		gitUsage()
		os.Exit(2)
	}
}

func gitUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp git <status|diff|log|merge-base|commit>")
}

func gitStatus(args []string) {
	fs := flag.NewFlagSet("git status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	result, err := wsgit.NewClient().Status(context.Background(), defaultRoot(*root))
	if outputJSON(*format) {
		printJSONOrFatal("git status", result, err)
		return
	}
	text, err := runNativeGitText(defaultRoot(*root), "status", "--short", "--branch")
	printTextOrFatal("git status", text, err)
}

func gitDiff(args []string) {
	fs := flag.NewFlagSet("git diff", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	rangeValue := fs.String("range", "", "revision range")
	mode := fs.String("mode", wsgit.DiffModeStat, "diff mode: full, stat, or name_only; defaults to stat")
	format := fs.String("format", "", `output format: text or json`)
	var paths multiFlag
	fs.Var(&paths, "path", "path filter; may be repeated")
	_ = fs.Parse(args)
	paths = append(paths, fs.Args()...)

	result, err := wsgit.NewClient().Diff(context.Background(), defaultRoot(*root), wsgit.DiffOptions{Range: *rangeValue, Mode: *mode, Paths: paths})
	if outputJSON(*format) {
		printJSONOrFatal("git diff", result, err)
		return
	}
	printTextOrFatal("git diff", result.Output, err)
}

func gitLog(args []string) {
	fs := flag.NewFlagSet("git log", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	rangeValue := fs.String("range", "", "revision range")
	limit := fs.Int("limit", 20, "maximum commits to return, capped at 100")
	includeBody := fs.Bool("include-body", false, "include commit body")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	result, err := wsgit.NewClient().Log(context.Background(), defaultRoot(*root), wsgit.LogOptions{Range: *rangeValue, Limit: *limit, IncludeBody: *includeBody})
	if outputJSON(*format) {
		printJSONOrFatal("git log", result, err)
		return
	}
	text, err := runNativeGitLogText(defaultRoot(*root), *rangeValue, *limit)
	printTextOrFatal("git log", text, err)
}

func gitMergeBase(args []string) {
	fs := flag.NewFlagSet("git merge-base", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	base := fs.String("base", "", "base revision")
	head := fs.String("head", "", "head revision")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)
	remaining := fs.Args()
	if *base == "" && len(remaining) > 0 {
		*base = remaining[0]
	}
	if *head == "" && len(remaining) > 1 {
		*head = remaining[1]
	}

	result, err := wsgit.NewClient().MergeBase(context.Background(), defaultRoot(*root), *base, *head)
	if outputJSON(*format) {
		printJSONOrFatal("git merge-base", result, err)
		return
	}
	text, err := runNativeGitText(defaultRoot(*root), "merge-base", *base, *head)
	printTextOrFatal("git merge-base", text, err)
}

func gitCommit(args []string) {
	fs := flag.NewFlagSet("git commit", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	title := fs.String("title", "", "single-line commit title")
	description := fs.String("description", "", "commit description body")
	descriptionFile := fs.String("description-file", "", "commit description file; use - for stdin")
	format := fs.String("format", "", `output format: text or json`)
	var paths multiFlag
	var aiContext multiFlag
	var mentalModelNotes multiFlag
	var updatedTickets multiFlag
	var updatedSpecs multiFlag
	var updatedMentalModels multiFlag
	fs.Var(&paths, "path", "path to stage and commit; may be repeated")
	fs.Var(&aiContext, "ai-context", "AI Context bullet; may be repeated")
	fs.Var(&mentalModelNotes, "mental-model-note", "Mental Model Notes bullet under AI Context; may be repeated")
	fs.Var(&updatedTickets, "updated-ticket", "ticket update summary; may be repeated")
	fs.Var(&updatedSpecs, "updated-spec", "spec update summary; may be repeated")
	fs.Var(&updatedMentalModels, "updated-mental-model", "mental-model update summary; may be repeated")
	_ = fs.Parse(args)
	paths = append(paths, fs.Args()...)

	body := *description
	if *descriptionFile != "" {
		text, err := readInputFile(*descriptionFile, "description")
		if err != nil {
			fatal("git commit", err)
		}
		body = text
	}
	result, err := wsgit.NewClient().Commit(context.Background(), defaultRoot(*root), wsgit.CommitOptions{
		Paths:               paths,
		Title:               *title,
		Description:         body,
		AIContext:           aiContext,
		MentalModelNotes:    mentalModelNotes,
		UpdatedTickets:      updatedTickets,
		UpdatedSpecs:        updatedSpecs,
		UpdatedMentalModels: updatedMentalModels,
	})
	if outputJSON(*format) {
		printJSONOrFatal("git commit", result, err)
		return
	}
	printTextOrFatal("git commit", mcp.FormatGitCommit(result), err)
}

func ticketsCommand(args []string) {
	if len(args) < 1 {
		ticketsUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		ticketsList(args[1:])
	case "find":
		ticketsFind(args[1:])
	case "status":
		ticketsStatus(args[1:])
	case "close":
		ticketsClose(args[1:])
	case "move":
		ticketsMove(args[1:])
	case "create":
		ticketsCreate(args[1:])
	default:
		ticketsUsage()
		os.Exit(2)
	}
}

func ticketsUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp tickets <list|find|status|close|move|create>")
}

func ticketsList(args []string) {
	fs := flag.NewFlagSet("tickets list", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	includeDone := fs.Bool("include-done", false, "include ai-docs/tickets/.done")
	includeDropped := fs.Bool("include-dropped", false, "include ai-docs/tickets/.dropped")
	format := fs.String("format", "", `output format: text or json`)
	var statuses multiFlag
	fs.Var(&statuses, "status", "ticket status to scan (ready, todo, idea; archives require include flags); may be repeated")
	_ = fs.Parse(args)

	result, err := wsdoc.TicketsList(defaultRoot(*root), wsdoc.TicketListOptions{
		Statuses:       statuses,
		IncludeDone:    *includeDone,
		IncludeDropped: *includeDropped,
	})
	if outputJSON(*format) {
		printJSONOrFatal("tickets list", result, err)
		return
	}
	printTextOrFatal("tickets list", mcp.FormatTickets(result), err)
}

func ticketsFind(args []string) {
	fs := flag.NewFlagSet("tickets find", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	query := fs.String("query", "", "case-insensitive text query")
	ticketStem := fs.String("ticket-stem", "", "exact ticket stem")
	mentionsTicketStem := fs.String("mentions-ticket-stem", "", "ticket stem that result tickets must mention")
	includeDone := fs.Bool("include-done", false, "include ai-docs/tickets/.done")
	includeDropped := fs.Bool("include-dropped", false, "include ai-docs/tickets/.dropped")
	format := fs.String("format", "", `output format: text or json`)
	var statuses multiFlag
	fs.Var(&statuses, "status", "ticket status to scan (ready, todo, idea; archives require include flags); may be repeated")
	_ = fs.Parse(args)

	result, err := wsdoc.TicketsFind(defaultRoot(*root), wsdoc.TicketFindOptions{
		Statuses:           statuses,
		IncludeDone:        *includeDone,
		IncludeDropped:     *includeDropped,
		Query:              *query,
		TicketStem:         *ticketStem,
		MentionsTicketStem: *mentionsTicketStem,
	})
	if outputJSON(*format) {
		printJSONOrFatal("tickets find", result, err)
		return
	}
	printTextOrFatal("tickets find", mcp.FormatTickets(result), err)
}

func ticketsStatus(args []string) {
	fs := flag.NewFlagSet("tickets status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	ticketStem := fs.String("ticket-stem", "", "ticket stem to inspect")
	includeDone := fs.Bool("include-done", false, "allow lookup under ai-docs/tickets/.done")
	includeDropped := fs.Bool("include-dropped", false, "allow lookup under ai-docs/tickets/.dropped")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)
	if *ticketStem == "" && len(fs.Args()) > 0 {
		*ticketStem = fs.Args()[0]
	}

	result, err := wsdoc.TicketsStatus(defaultRoot(*root), wsdoc.TicketStatusOptions{
		TicketStem:     *ticketStem,
		IncludeDone:    *includeDone,
		IncludeDropped: *includeDropped,
	})
	if outputJSON(*format) {
		printJSONOrFatal("tickets status", result, err)
		return
	}
	var tickets []wsdoc.TicketInfo
	if result != nil {
		tickets = append(tickets, *result)
	}
	printTextOrFatal("tickets status", mcp.FormatTickets(tickets), err)
}

func ticketsClose(args []string) {
	fs := flag.NewFlagSet("tickets close", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	stem := fs.String("stem", "", "ticket stem to close")
	status := fs.String("status", "", "close target: done or dropped")
	resolution := fs.String("resolution", "", "optional resolution text appended as a ## Resolution section")
	_ = fs.Parse(args)
	if *stem == "" && len(fs.Args()) > 0 {
		*stem = fs.Args()[0]
	}

	result, err := wsdoc.TicketsClose(defaultRoot(*root), wsgit.ExecRunner{}, wsdoc.TicketCloseOptions{
		TicketStem: *stem,
		Status:     *status,
		Resolution: *resolution,
		Today:      time.Now().Format("2006-01-02"),
	})
	printTextOrFatal("tickets close", mcp.FormatTicketMutate("closed", result), err)
}

func ticketsMove(args []string) {
	fs := flag.NewFlagSet("tickets move", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	stem := fs.String("stem", "", "ticket stem to move")
	to := fs.String("to", "", "target status: idea, todo, or ready")
	_ = fs.Parse(args)
	if *stem == "" && len(fs.Args()) > 0 {
		*stem = fs.Args()[0]
	}

	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	resolved, _ := resolver.Get("", "sage_review")
	result, err := wsdoc.TicketsMove(defaultRoot(*root), wsgit.ExecRunner{}, wsdoc.TicketMoveOptions{
		TicketStem: *stem,
		To:         *to,
		SageReview: resolved.Value,
	})
	printTextOrFatal("tickets move", mcp.FormatTicketMutate("moved", result), err)
}

func ticketsCreate(args []string) {
	fs := flag.NewFlagSet("tickets create", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	stem := fs.String("stem", "", "semantic ticket stem without date prefix")
	initialState := fs.String("initial-state", "", "ticket status: idea, todo, or ready")
	_ = fs.Parse(args)
	rest := fs.Args()
	if *stem == "" && len(rest) > 0 {
		*stem = rest[0]
		rest = rest[1:]
	}
	if *initialState == "" && len(rest) > 0 {
		*initialState = rest[0]
	}

	result, err := wsdoc.TicketCreate(defaultRoot(*root), wsdoc.TicketCreateOptions{
		Stem:         *stem,
		InitialState: *initialState,
	})
	printTextOrFatal("tickets create", mcp.FormatTicketCreate(result), err)
}

func specsCommand(args []string) {
	if len(args) < 1 {
		specsUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		specsList(args[1:])
	case "find":
		specsFind(args[1:])
	case "status":
		specsStatus(args[1:])
	default:
		specsUsage()
		os.Exit(2)
	}
}

func specsUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp specs <list|find|status>")
}

func specsList(args []string) {
	fs := flag.NewFlagSet("specs list", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	result, err := wsdoc.SpecsList(defaultRoot(*root))
	if outputJSON(*format) {
		printJSONOrFatal("specs list", result, err)
		return
	}
	printTextOrFatal("specs list", mcp.FormatSpecs(result), err)
}

func specsFind(args []string) {
	fs := flag.NewFlagSet("specs find", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	query := fs.String("query", "", "case-insensitive text query")
	specStem := fs.String("spec-stem", "", "exact spec anchor stem")
	ticketStem := fs.String("ticket-stem", "", "ticket stem referenced by specs")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	result, err := wsdoc.SpecsFind(defaultRoot(*root), wsdoc.SpecFindOptions{
		Query:      *query,
		SpecStem:   *specStem,
		TicketStem: *ticketStem,
	})
	if outputJSON(*format) {
		printJSONOrFatal("specs find", result, err)
		return
	}
	if strings.TrimSpace(*query) != "" {
		printTextOrFatal("specs find", mcp.FormatSpecFind(*query, result), err)
		return
	}
	printTextOrFatal("specs find", mcp.FormatSpecs(result), err)
}

func specsStatus(args []string) {
	fs := flag.NewFlagSet("specs status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	specStem := fs.String("spec-stem", "", "spec anchor stem to inspect")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)
	if *specStem == "" && len(fs.Args()) > 0 {
		*specStem = fs.Args()[0]
	}

	result, err := wsdoc.SpecsStatus(defaultRoot(*root), wsdoc.SpecStatusOptions{SpecStem: *specStem})
	if outputJSON(*format) {
		printJSONOrFatal("specs status", result, err)
		return
	}
	printTextOrFatal("specs status", mcp.FormatSpecStatus(result), err)
}

func mentalModelsCommand(args []string) {
	if len(args) < 1 {
		mentalModelsUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "find":
		mentalModelsFind(args[1:])
	case "status":
		mentalModelsStatus(args[1:])
	default:
		mentalModelsUsage()
		os.Exit(2)
	}
}

func mentalModelsUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp mental-models <find|status>")
}

func mentalModelsFind(args []string) {
	fs := flag.NewFlagSet("mental-models find", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	query := fs.String("query", "", "case-insensitive text query")
	specStem := fs.String("spec-stem", "", "spec anchor stem referenced by mental models")
	domain := fs.String("domain", "", "mental-model domain")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	result, err := wsdoc.MentalModelsFind(defaultRoot(*root), wsdoc.MentalModelFindOptions{
		Query:    *query,
		SpecStem: *specStem,
		Domain:   *domain,
	})
	if outputJSON(*format) {
		printJSONOrFatal("mental-models find", result, err)
		return
	}
	if strings.TrimSpace(*query) != "" {
		printTextOrFatal("mental-models find", mcp.FormatMentalModelFind(*query, result), err)
		return
	}
	printTextOrFatal("mental-models find", mcp.FormatMentalModels(result), err)
}

func mentalModelsStatus(args []string) {
	fs := flag.NewFlagSet("mental-models status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	domain := fs.String("domain", "", "mental-model domain")
	path := fs.String("path", "", "relative path under ai-docs/mental-model")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)
	if *domain == "" && *path == "" && len(fs.Args()) > 0 {
		*domain = fs.Args()[0]
	}

	result, err := wsdoc.MentalModelsStatus(defaultRoot(*root), wsdoc.MentalModelStatusOptions{Domain: *domain, Path: *path})
	if outputJSON(*format) {
		printJSONOrFatal("mental-models status", result, err)
		return
	}
	printTextOrFatal("mental-models status", mcp.FormatMentalModels(result), err)
}

func referencesCommand(args []string) {
	if len(args) < 1 {
		referencesUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "trace":
		referencesTrace(args[1:])
	default:
		referencesUsage()
		os.Exit(2)
	}
}

func referencesUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp references <trace>")
}

func referencesTrace(args []string) {
	fs := flag.NewFlagSet("references trace", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	ticketStem := fs.String("ticket-stem", "", "ticket stem to trace")
	specStem := fs.String("spec-stem", "", "spec anchor stem to trace")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	result, err := wsdoc.ReferencesTrace(defaultRoot(*root), wsdoc.ReferenceTraceOptions{
		TicketStem: *ticketStem,
		SpecStem:   *specStem,
	})
	if outputJSON(*format) {
		printJSONOrFatal("references trace", result, err)
		return
	}
	printTextOrFatal("references trace", mcp.FormatReferenceTrace(result), err)
}

func printJSONOrFatal(prefix string, value any, err error) {
	if err != nil {
		fatal(prefix, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		fatal(prefix, err)
	}
	fmt.Println(string(encoded))
}

func printTextOrFatal(prefix, text string, err error) {
	if err != nil {
		fatal(prefix, err)
	}
	fmt.Print(text)
}

func outputJSON(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), "json")
}

func runNativeGitText(root string, args ...string) (string, error) {
	out, err := wsgit.ExecRunner{}.RunGit(context.Background(), root, args...)
	return string(out), err
}

func runNativeGitLogText(root, rangeValue string, limit int) (string, error) {
	if strings.HasPrefix(rangeValue, "-") {
		return "", fmt.Errorf("range must be a revision or range, not a git option")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	args := []string{"log", "-n", fmt.Sprint(limit), "--date=iso-strict"}
	if rangeValue != "" {
		args = append(args, rangeValue)
	}
	return runNativeGitText(root, args...)
}

func agents(args []string) {
	if len(args) < 1 {
		agentsUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "register":
		agentsRegister(args[1:])
	case "call":
		agentsCall(args[1:])
	case "run-current":
		agentsRunCurrent(args[1:])
	case "wait":
		agentsWait(args[1:])
	case "result":
		agentsResult(args[1:])
	case "status":
		agentsStatus(args[1:])
	case "interrupt":
		agentsInterrupt(args[1:])
	case "check-inbox":
		agentsCheckInbox(args[1:])
	case "tail":
		agentsTail(args[1:])
	case "debug":
		agentsDebug(args[1:])
	case "cancel":
		agentsCancel(args[1:])
	case "recall":
		agentsRecall(args[1:])
	case "print":
		agentsPrint(args[1:])
	case "erase":
		agentsErase(args[1:])
	default:
		agentsUsage()
		os.Exit(2)
	}
}

func agentsUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp mercenary <register|call|run-current|wait|result|status|interrupt|check-inbox|tail|debug|cancel|recall|print|erase>")
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func agentsRegister(args []string) {
	fs := flag.NewFlagSet("mercenary register", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	backend := fs.String("backend", "", "agent backend")
	harness := fs.String("harness", "", "MCP host harness for model alias resolution")
	tier := fs.String("tier", "", "deprecated model alias selector")
	model := fs.String("model", "", "model alias or concrete backend model")
	systemFile := fs.String("system-prompt-file", "", "system prompt file")
	_ = fs.Parse(args)

	systemText, err := readOptionalFile(*systemFile)
	if err != nil {
		fatal("mercenary register", err)
	}
	agent, _, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{
		Root:             defaultRoot(*root),
		Name:             *name,
		Backend:          *backend,
		Harness:          *harness,
		Tier:             *tier,
		Model:            *model,
		SystemPromptText: systemText,
	})
	if err != nil {
		fatal("mercenary register", err)
	}
	fmt.Printf("%s\n", agent.Name)
}

func agentsCall(args []string) {
	fs := flag.NewFlagSet("mercenary call", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	promptFile := fs.String("prompt-file", "", "prompt file; use - for stdin")
	_ = fs.Parse(args)

	prompt, err := promptFromArgs(fs.Args(), *promptFile)
	if err != nil {
		fatal("mercenary call", err)
	}
	result, err := wsagent.NewManager(wsagent.Options{}).Call(wsagent.CallOptions{
		Root:   defaultRoot(*root),
		Name:   *name,
		Prompt: prompt,
	})
	if err != nil {
		fatal("mercenary call", err)
	}
	fmt.Printf("%s\t%s\tpid=%d\nfollow_up: ws.mercenary.result --timeout 10m | ws.mercenary.wait --timeout 10m | ws.mercenary.status | ws.mercenary.tail | ws.mercenary.cancel\n", result.AgentName, result.Status, result.PID)
}

func agentsRunCurrent(args []string) {
	fs := flag.NewFlagSet("mercenary run-current", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	if err := wsagent.NewManager(wsagent.Options{}).RunCurrent(defaultRoot(*root), *name); err != nil {
		fatal("mercenary run-current", err)
	}
}

func agentsWait(args []string) {
	fs := flag.NewFlagSet("mercenary wait", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	var names multiFlag
	fs.Var(&names, "name", "agent name; may be repeated")
	timeout := fs.Duration("timeout", 0, "maximum wait duration; defaults to 10m")
	_ = fs.Parse(args)
	names = append(names, fs.Args()...)

	text, err := wsagent.NewManager(wsagent.Options{}).Wait(wsagent.WaitOptions{
		Root:    defaultRoot(*root),
		Names:   names,
		Timeout: *timeout,
	})
	if err != nil {
		fatal("mercenary wait", err)
	}
	fmt.Print(text)
}

func agentsResult(args []string) {
	fs := flag.NewFlagSet("mercenary result", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	timeout := fs.Duration("timeout", 0, "maximum wait duration; defaults to non-blocking")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Result(wsagent.ResultOptions{
		Root:    defaultRoot(*root),
		Name:    *name,
		Timeout: *timeout,
	})
	if err != nil {
		fatal("mercenary result", err)
	}
	fmt.Print(text)
}

func agentsStatus(args []string) {
	fs := flag.NewFlagSet("mercenary status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Status(defaultRoot(*root), *name)
	if err != nil {
		fatal("mercenary status", err)
	}
	fmt.Print(text)
}

func agentsInterrupt(args []string) {
	fs := flag.NewFlagSet("mercenary interrupt", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	messageFile := fs.String("message-file", "", "message file; use - for stdin")
	_ = fs.Parse(args)

	message, err := promptFromArgs(fs.Args(), *messageFile)
	if err != nil {
		fatal("mercenary interrupt", err)
	}
	result, err := wsagent.NewManager(wsagent.Options{}).Interrupt(wsagent.InterruptOptions{
		Root:    defaultRoot(*root),
		Name:    *name,
		Message: message,
	})
	if err != nil {
		fatal("mercenary interrupt", err)
	}
	fmt.Printf("%s\tqueued\tmessage=%s\n", result.AgentName, result.MessageID)
}

func agentsCheckInbox(args []string) {
	fs := flag.NewFlagSet("mercenary check-inbox", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	messages, err := wsagent.NewManager(wsagent.Options{}).DeliverPendingInbox(defaultRoot(*root), *name, "hook")
	if err != nil {
		fatal("mercenary check-inbox", err)
	}
	if len(messages) > 0 {
		fmt.Fprint(os.Stderr, wsagent.ComposeLeadMessageFeedback(messages))
		os.Exit(2)
	}
}

func agentsTail(args []string) {
	agentsTailRaw(args, false)
}

func agentsTailRaw(args []string, raw bool) {
	fs := flag.NewFlagSet("mercenary tail", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	lines := fs.Int("lines", 40, "number of lines per section")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Tail(wsagent.TailOptions{
		Root:  defaultRoot(*root),
		Name:  *name,
		Lines: *lines,
		Raw:   raw,
	})
	if err != nil {
		fatal("mercenary tail", err)
	}
	fmt.Print(text)
}

func agentsDebug(args []string) {
	if len(args) < 1 {
		agentsDebugUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "tail":
		agentsTailRaw(args[1:], true)
	case "stdout":
		agentsDebugStream("stdout", args[1:])
	case "stderr":
		agentsDebugStream("stderr", args[1:])
	case "runtime-log":
		agentsDebugStream("runtime_log", args[1:])
	case "events":
		agentsDebugStream("events", args[1:])
	default:
		agentsDebugUsage()
		os.Exit(2)
	}
}

func agentsDebugUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp mercenary debug <tail|stdout|stderr|runtime-log|events>")
}

func agentsDebugStream(stream string, args []string) {
	fs := flag.NewFlagSet("mercenary debug "+stream, flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	lines := fs.Int("lines", 40, "number of lines")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).DiagnosticStream(wsagent.DiagnosticStreamOptions{
		Root:   defaultRoot(*root),
		Name:   *name,
		Stream: stream,
		Lines:  *lines,
	})
	if err != nil {
		fatal("mercenary debug "+stream, err)
	}
	fmt.Print(text)
}

func agentsCancel(args []string) {
	fs := flag.NewFlagSet("mercenary cancel", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Cancel(defaultRoot(*root), *name)
	if err != nil {
		fatal("mercenary cancel", err)
	}
	fmt.Print(text)
}

func agentsRecall(args []string) {
	fs := flag.NewFlagSet("mercenary recall", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	promptFile := fs.String("prompt-file", "", "recovery prompt file; use - for stdin")
	_ = fs.Parse(args)

	prompt, err := optionalPromptFromArgs(fs.Args(), *promptFile)
	if err != nil {
		fatal("mercenary recall", err)
	}
	text, err := wsagent.NewManager(wsagent.Options{}).Recall(wsagent.RecallOptions{
		Root:   defaultRoot(*root),
		Name:   *name,
		Prompt: prompt,
	})
	if err != nil {
		fatal("mercenary recall", err)
	}
	fmt.Print(text)
}

func agentsPrint(args []string) {
	fs := flag.NewFlagSet("mercenary print", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Print(defaultRoot(*root), *name)
	if err != nil {
		fatal("mercenary print", err)
	}
	fmt.Print(text)
}

func agentsErase(args []string) {
	fs := flag.NewFlagSet("mercenary erase", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	if err := wsagent.NewManager(wsagent.Options{}).Erase(defaultRoot(*root), *name); err != nil {
		fatal("mercenary erase", err)
	}
}

func promptFromArgs(args []string, promptFile string) (string, error) {
	if promptFile != "" {
		return readInputFile(promptFile, "prompt")
	}
	if len(args) == 0 {
		return "", fmt.Errorf("prompt is required")
	}
	return strings.Join(args, " "), nil
}

func optionalPromptFromArgs(args []string, promptFile string) (string, error) {
	if promptFile != "" {
		return readInputFile(promptFile, "prompt")
	}
	if len(args) == 0 {
		return "", nil
	}
	return strings.Join(args, " "), nil
}

func readInputFile(path, label string) (string, error) {
	if path == "-" {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin %s: %w", label, err)
		}
		return string(raw), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s file: %w", label, err)
	}
	return string(raw), nil
}

func readOptionalFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(raw), nil
}

func fatal(prefix string, err error) {
	fmt.Fprintf(os.Stderr, "ws-mcp %s: %v\n", prefix, err)
	os.Exit(1)
}

func defaultRoot(root string) string {
	if root != "" && root != "." {
		return root
	}
	if env := os.Getenv("WS_MCP_PROJECT_ROOT"); env != "" {
		return env
	}
	return root
}
