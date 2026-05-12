package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsprompt"
	"github.com/kang-sw/devenv/internal/wsstate"
)

var version = "0.23.3-dev"
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
	case "subquery":
		subquery(os.Args[2:])
	case "config":
		configCommand(os.Args[2:])
	case "path":
		path(os.Args[2:])
	case "agents":
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
	fmt.Fprintln(os.Stderr, "usage: ws-mcp <version|doctor|runtime|serve|subquery|config|path|agents|git|tickets|specs|mental-models|references>")
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

	bundle, err := wsprompt.Bundle(sourceCommit)
	if err != nil {
		fatal("runtime info", err)
	}
	fmt.Printf("{\"version\":%q,\"source_commit\":%q,\"prompt_bundle\":{\"source_commit\":%q,\"content_sha256\":%q,\"prompts\":[", version, sourceCommit, bundle.SourceCommit, bundle.ContentSHA256)
	for i, prompt := range bundle.Prompts {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf("%q", prompt)
	}
	fmt.Println("]}}")
}

type runtimeCapabilitiesPayload struct {
	Version      string              `json:"version"`
	SourceCommit string              `json:"source_commit"`
	MCPProtocol  string              `json:"mcp_protocol"`
	PromptBundle wsprompt.BundleInfo `json:"prompt_bundle"`
	Tools        []string            `json:"tools"`
	Commands     []string            `json:"commands"`
}

func runtimeCapabilities(args []string) {
	fs := flag.NewFlagSet("runtime capabilities", flag.ExitOnError)
	_ = fs.Parse(args)

	bundle, err := wsprompt.Bundle(sourceCommit)
	if err != nil {
		fatal("runtime capabilities", err)
	}
	payload := runtimeCapabilitiesPayload{
		Version:      version,
		SourceCommit: sourceCommit,
		MCPProtocol:  mcp.ProtocolVersion,
		PromptBundle: bundle,
		Tools:        mcp.LeadToolNames(),
		Commands:     runtimeCapabilityCommandNames(),
	}
	printJSONOrFatal("runtime capabilities", payload, nil)
}

func runtimeCapabilityCommandNames() []string {
	commands := []string{
		"agents.call",
		"agents.cancel",
		"agents.check-inbox",
		"agents.debug.events",
		"agents.debug.runtime-log",
		"agents.debug.stderr",
		"agents.debug.stdout",
		"agents.debug.tail",
		"agents.erase",
		"agents.interrupt",
		"agents.print",
		"agents.register",
		"agents.result",
		"agents.run-current",
		"agents.status",
		"agents.tail",
		"agents.wait",
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
		"specs.find",
		"specs.list",
		"specs.status",
		"subquery",
		"tickets.find",
		"tickets.list",
		"tickets.status",
	}
	sort.Strings(commands)
	return commands
}

func subquery(args []string) {
	fs := flag.NewFlagSet("subquery", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	deepResearch := fs.Bool("deep-research", false, "use deep model alias for broad tracing or research")
	harness := fs.String("harness", "", "MCP host harness for model alias resolution")
	promptFile := fs.String("prompt-file", "", "prompt file; use - for stdin")
	_ = fs.Parse(args)

	prompt, err := promptFromArgs(fs.Args(), *promptFile)
	if err != nil {
		fatal("subquery", err)
	}
	text, err := wsagent.NewManager(wsagent.Options{}).Subquery(wsagent.SubqueryOptions{
		Root:         defaultRoot(*root),
		Question:     prompt,
		DeepResearch: *deepResearch,
		Harness:      *harness,
	})
	if err != nil {
		fatal("subquery", err)
	}
	fmt.Print(text)
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
	_ = fs.Parse(args)

	view, err := wsconfig.Show(wsconfig.Options{})
	printJSONOrFatal("config show", view, err)
}

func configAgentsTier(args []string) {
	fs := flag.NewFlagSet("config agents-tier", flag.ExitOnError)
	tier := fs.String("tier", "", "model alias: light, core, or deep")
	backend := fs.String("backend", "", "backend name; inferred from model when omitted")
	model := fs.String("model", "", "concrete model for this alias")
	_ = fs.Parse(args)

	cfg, err := wsconfig.SetAgentsTier(wsconfig.Options{}, *tier, *backend, *model)
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
	_ = fs.Parse(args)

	result, err := wsgit.NewClient().Status(context.Background(), defaultRoot(*root))
	printJSONOrFatal("git status", result, err)
}

func gitDiff(args []string) {
	fs := flag.NewFlagSet("git diff", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	rangeValue := fs.String("range", "", "revision range")
	mode := fs.String("mode", wsgit.DiffModeStat, "diff mode: full, stat, or name_only; defaults to stat")
	var paths multiFlag
	fs.Var(&paths, "path", "path filter; may be repeated")
	_ = fs.Parse(args)
	paths = append(paths, fs.Args()...)

	result, err := wsgit.NewClient().Diff(context.Background(), defaultRoot(*root), wsgit.DiffOptions{Range: *rangeValue, Mode: *mode, Paths: paths})
	printJSONOrFatal("git diff", result, err)
}

func gitLog(args []string) {
	fs := flag.NewFlagSet("git log", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	rangeValue := fs.String("range", "", "revision range")
	limit := fs.Int("limit", 20, "maximum commits to return, capped at 100")
	includeBody := fs.Bool("include-body", false, "include commit body")
	_ = fs.Parse(args)

	result, err := wsgit.NewClient().Log(context.Background(), defaultRoot(*root), wsgit.LogOptions{Range: *rangeValue, Limit: *limit, IncludeBody: *includeBody})
	printJSONOrFatal("git log", result, err)
}

func gitMergeBase(args []string) {
	fs := flag.NewFlagSet("git merge-base", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	base := fs.String("base", "", "base revision")
	head := fs.String("head", "", "head revision")
	_ = fs.Parse(args)
	remaining := fs.Args()
	if *base == "" && len(remaining) > 0 {
		*base = remaining[0]
	}
	if *head == "" && len(remaining) > 1 {
		*head = remaining[1]
	}

	result, err := wsgit.NewClient().MergeBase(context.Background(), defaultRoot(*root), *base, *head)
	printJSONOrFatal("git merge-base", result, err)
}

func gitCommit(args []string) {
	fs := flag.NewFlagSet("git commit", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	title := fs.String("title", "", "single-line commit title")
	description := fs.String("description", "", "commit description body")
	descriptionFile := fs.String("description-file", "", "commit description file; use - for stdin")
	var paths multiFlag
	var aiContext multiFlag
	var updatedTickets multiFlag
	var updatedSpecs multiFlag
	var updatedMentalModels multiFlag
	fs.Var(&paths, "path", "path to stage and commit; may be repeated")
	fs.Var(&aiContext, "ai-context", "AI Context bullet; may be repeated")
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
		UpdatedTickets:      updatedTickets,
		UpdatedSpecs:        updatedSpecs,
		UpdatedMentalModels: updatedMentalModels,
	})
	printJSONOrFatal("git commit", result, err)
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
	default:
		ticketsUsage()
		os.Exit(2)
	}
}

func ticketsUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp tickets <list|find|status>")
}

func ticketsList(args []string) {
	fs := flag.NewFlagSet("tickets list", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	includeDone := fs.Bool("include-done", false, "include ai-docs/tickets/.done")
	includeDropped := fs.Bool("include-dropped", false, "include ai-docs/tickets/.dropped")
	var statuses multiFlag
	fs.Var(&statuses, "status", "ticket status to scan (ready, todo, idea; archives require include flags); may be repeated")
	_ = fs.Parse(args)

	result, err := wsdoc.TicketsList(defaultRoot(*root), wsdoc.TicketListOptions{
		Statuses:       statuses,
		IncludeDone:    *includeDone,
		IncludeDropped: *includeDropped,
	})
	printJSONOrFatal("tickets list", result, err)
}

func ticketsFind(args []string) {
	fs := flag.NewFlagSet("tickets find", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	query := fs.String("query", "", "case-insensitive text query")
	ticketStem := fs.String("ticket-stem", "", "exact ticket stem")
	mentionsTicketStem := fs.String("mentions-ticket-stem", "", "ticket stem that result tickets must mention")
	includeDone := fs.Bool("include-done", false, "include ai-docs/tickets/.done")
	includeDropped := fs.Bool("include-dropped", false, "include ai-docs/tickets/.dropped")
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
	printJSONOrFatal("tickets find", result, err)
}

func ticketsStatus(args []string) {
	fs := flag.NewFlagSet("tickets status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	ticketStem := fs.String("ticket-stem", "", "ticket stem to inspect")
	includeDone := fs.Bool("include-done", false, "allow lookup under ai-docs/tickets/.done")
	includeDropped := fs.Bool("include-dropped", false, "allow lookup under ai-docs/tickets/.dropped")
	_ = fs.Parse(args)
	if *ticketStem == "" && len(fs.Args()) > 0 {
		*ticketStem = fs.Args()[0]
	}

	result, err := wsdoc.TicketsStatus(defaultRoot(*root), wsdoc.TicketStatusOptions{
		TicketStem:     *ticketStem,
		IncludeDone:    *includeDone,
		IncludeDropped: *includeDropped,
	})
	printJSONOrFatal("tickets status", result, err)
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
	_ = fs.Parse(args)

	result, err := wsdoc.SpecsList(defaultRoot(*root))
	printJSONOrFatal("specs list", result, err)
}

func specsFind(args []string) {
	fs := flag.NewFlagSet("specs find", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	query := fs.String("query", "", "case-insensitive text query")
	specStem := fs.String("spec-stem", "", "exact spec anchor stem")
	ticketStem := fs.String("ticket-stem", "", "ticket stem referenced by specs")
	_ = fs.Parse(args)

	result, err := wsdoc.SpecsFind(defaultRoot(*root), wsdoc.SpecFindOptions{
		Query:      *query,
		SpecStem:   *specStem,
		TicketStem: *ticketStem,
	})
	printJSONOrFatal("specs find", result, err)
}

func specsStatus(args []string) {
	fs := flag.NewFlagSet("specs status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	specStem := fs.String("spec-stem", "", "spec anchor stem to inspect")
	_ = fs.Parse(args)
	if *specStem == "" && len(fs.Args()) > 0 {
		*specStem = fs.Args()[0]
	}

	result, err := wsdoc.SpecsStatus(defaultRoot(*root), wsdoc.SpecStatusOptions{SpecStem: *specStem})
	printJSONOrFatal("specs status", result, err)
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
	_ = fs.Parse(args)

	result, err := wsdoc.MentalModelsFind(defaultRoot(*root), wsdoc.MentalModelFindOptions{
		Query:    *query,
		SpecStem: *specStem,
		Domain:   *domain,
	})
	printJSONOrFatal("mental-models find", result, err)
}

func mentalModelsStatus(args []string) {
	fs := flag.NewFlagSet("mental-models status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	domain := fs.String("domain", "", "mental-model domain")
	path := fs.String("path", "", "relative path under ai-docs/mental-model")
	_ = fs.Parse(args)
	if *domain == "" && *path == "" && len(fs.Args()) > 0 {
		*domain = fs.Args()[0]
	}

	result, err := wsdoc.MentalModelsStatus(defaultRoot(*root), wsdoc.MentalModelStatusOptions{Domain: *domain, Path: *path})
	printJSONOrFatal("mental-models status", result, err)
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
	_ = fs.Parse(args)

	result, err := wsdoc.ReferencesTrace(defaultRoot(*root), wsdoc.ReferenceTraceOptions{
		TicketStem: *ticketStem,
		SpecStem:   *specStem,
	})
	printJSONOrFatal("references trace", result, err)
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
	fmt.Fprintln(os.Stderr, "usage: ws-mcp agents <register|call|run-current|wait|result|status|interrupt|check-inbox|tail|debug|cancel|recall|print|erase>")
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
	fs := flag.NewFlagSet("agents register", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	backend := fs.String("backend", "", "agent backend")
	harness := fs.String("harness", "", "MCP host harness for model alias resolution")
	tier := fs.String("tier", "", "deprecated model alias selector")
	model := fs.String("model", "", "model alias or concrete backend model")
	systemFile := fs.String("system-prompt-file", "", "system prompt file")
	var prompts multiFlag
	var promptRefs multiFlag
	fs.Var(&prompts, "prompt", "embedded prompt stem or absolute prompt path")
	fs.Var(&promptRefs, "prompt-ref", "logical prompt reference")
	_ = fs.Parse(args)

	systemText, err := readOptionalFile(*systemFile)
	if err != nil {
		fatal("agents register", err)
	}
	agent, _, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{
		Root:             defaultRoot(*root),
		Name:             *name,
		Backend:          *backend,
		Harness:          *harness,
		Tier:             *tier,
		Model:            *model,
		Prompts:          prompts,
		PromptRefs:       promptRefs,
		SystemPromptText: systemText,
	})
	if err != nil {
		fatal("agents register", err)
	}
	fmt.Printf("%s\n", agent.Name)
}

func agentsCall(args []string) {
	fs := flag.NewFlagSet("agents call", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	promptFile := fs.String("prompt-file", "", "prompt file; use - for stdin")
	_ = fs.Parse(args)

	prompt, err := promptFromArgs(fs.Args(), *promptFile)
	if err != nil {
		fatal("agents call", err)
	}
	result, err := wsagent.NewManager(wsagent.Options{}).Call(wsagent.CallOptions{
		Root:   defaultRoot(*root),
		Name:   *name,
		Prompt: prompt,
	})
	if err != nil {
		fatal("agents call", err)
	}
	fmt.Printf("%s\t%s\tpid=%d\nfollow_up: agents.result --timeout 10m | agents.wait --timeout 10m | agents.status | agents.tail | agents.cancel\n", result.AgentName, result.Status, result.PID)
}

func agentsRunCurrent(args []string) {
	fs := flag.NewFlagSet("agents run-current", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	if err := wsagent.NewManager(wsagent.Options{}).RunCurrent(defaultRoot(*root), *name); err != nil {
		fatal("agents run-current", err)
	}
}

func agentsWait(args []string) {
	fs := flag.NewFlagSet("agents wait", flag.ExitOnError)
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
		fatal("agents wait", err)
	}
	fmt.Print(text)
}

func agentsResult(args []string) {
	fs := flag.NewFlagSet("agents result", flag.ExitOnError)
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
		fatal("agents result", err)
	}
	fmt.Print(text)
}

func agentsStatus(args []string) {
	fs := flag.NewFlagSet("agents status", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Status(defaultRoot(*root), *name)
	if err != nil {
		fatal("agents status", err)
	}
	fmt.Print(text)
}

func agentsInterrupt(args []string) {
	fs := flag.NewFlagSet("agents interrupt", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	messageFile := fs.String("message-file", "", "message file; use - for stdin")
	_ = fs.Parse(args)

	message, err := promptFromArgs(fs.Args(), *messageFile)
	if err != nil {
		fatal("agents interrupt", err)
	}
	result, err := wsagent.NewManager(wsagent.Options{}).Interrupt(wsagent.InterruptOptions{
		Root:    defaultRoot(*root),
		Name:    *name,
		Message: message,
	})
	if err != nil {
		fatal("agents interrupt", err)
	}
	fmt.Printf("%s\tqueued\tmessage=%s\n", result.AgentName, result.MessageID)
}

func agentsCheckInbox(args []string) {
	fs := flag.NewFlagSet("agents check-inbox", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	messages, err := wsagent.NewManager(wsagent.Options{}).DeliverPendingInbox(defaultRoot(*root), *name, "hook")
	if err != nil {
		fatal("agents check-inbox", err)
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
	fs := flag.NewFlagSet("agents tail", flag.ExitOnError)
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
		fatal("agents tail", err)
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
	fmt.Fprintln(os.Stderr, "usage: ws-mcp agents debug <tail|stdout|stderr|runtime-log|events>")
}

func agentsDebugStream(stream string, args []string) {
	fs := flag.NewFlagSet("agents debug "+stream, flag.ExitOnError)
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
		fatal("agents debug "+stream, err)
	}
	fmt.Print(text)
}

func agentsCancel(args []string) {
	fs := flag.NewFlagSet("agents cancel", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Cancel(defaultRoot(*root), *name)
	if err != nil {
		fatal("agents cancel", err)
	}
	fmt.Print(text)
}

func agentsRecall(args []string) {
	fs := flag.NewFlagSet("agents recall", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	promptFile := fs.String("prompt-file", "", "recovery prompt file; use - for stdin")
	_ = fs.Parse(args)

	prompt, err := optionalPromptFromArgs(fs.Args(), *promptFile)
	if err != nil {
		fatal("agents recall", err)
	}
	text, err := wsagent.NewManager(wsagent.Options{}).Recall(wsagent.RecallOptions{
		Root:   defaultRoot(*root),
		Name:   *name,
		Prompt: prompt,
	})
	if err != nil {
		fatal("agents recall", err)
	}
	fmt.Print(text)
}

func agentsPrint(args []string) {
	fs := flag.NewFlagSet("agents print", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	text, err := wsagent.NewManager(wsagent.Options{}).Print(defaultRoot(*root), *name)
	if err != nil {
		fatal("agents print", err)
	}
	fmt.Print(text)
}

func agentsErase(args []string) {
	fs := flag.NewFlagSet("agents erase", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "agent name")
	_ = fs.Parse(args)

	if err := wsagent.NewManager(wsagent.Options{}).Erase(defaultRoot(*root), *name); err != nil {
		fatal("agents erase", err)
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
