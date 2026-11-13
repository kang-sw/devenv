package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

var version = "0.50.0-dev"
var sourceCommit = "dev"

func main() {
	// Only process entry points own lifecycle, signals and process exit. All
	// one-shot product commands share the same core as daemon-hosted CLI.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			serve(os.Args[2:])
			return
		case "dashboard":
			dashboardCommand(os.Args[2:])
			return
		case "daemon-start":
			daemonStart(os.Args[2:])
			return
		case "mailbox":
			if len(os.Args) > 2 && os.Args[2] == "wait" {
				mailboxWait(os.Args[3:])
				return
			}
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		fatal("cwd", err)
	}
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	command := hostedCLI{
		hello: wsdashboard.ProxyHello{Argv: os.Args[1:], Cwd: cwd, Env: env, OwnerPID: os.Getpid()},
		ctx:   context.Background(), stdout: os.Stdout, stderr: os.Stderr, env: os.Environ(),
		stdin: os.Stdin,
	}
	os.Exit(command.run())
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
	startParentDeathWatch(func(ppid int) {
		mcp.RecordLifecycleEvent("process.parent_exited", map[string]any{"ppid": ppid, "action": "self_terminate"})
		server.DashboardEnd()
		os.Exit(0)
	})
	go serveDashboard(server)
	if err := server.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp serve: %v\n", err)
		os.Exit(1)
	}
}

type runtimeCapabilitiesPayload struct {
	Version      string   `json:"version"`
	SourceCommit string   `json:"source_commit"`
	MCPProtocol  string   `json:"mcp_protocol"`
	Tools        []string `json:"tools"`
	Commands     []string `json:"commands"`
}

func runtimeCapabilityCommandNames() []string {
	commands := []string{
		"config.list", "config.tune", "git.commit", "git.diff", "git.log", "git.merge-base", "git.status",
		"path.generate", "runtime.capabilities", "runtime.info", "smoke", "tickets.close", "tickets.create-empty",
		"tickets.find", "tickets.list", "tickets.move", "tickets.status", "tickets.verify",
	}
	sort.Strings(commands)
	return commands
}

const toolsMappingRule = `mapping: ws/x.y(a: b) -> ws-cli call x.y '{"a":"b"}'`

type mcpLineResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *mcpLineError   `json:"error"`
}
type mcpLineError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
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

// Ticket mutation errors must retain the notice that the file already changed.
func withPartialMutationNotice(err error, notice string) error {
	if err == nil || notice == "" {
		return err
	}
	return fmt.Errorf("%w\npartial-mutation: %s A retry will not find an unchanged file.", err, notice)
}

func outputJSON(format string) bool { return strings.EqualFold(strings.TrimSpace(format), "json") }

type multiFlag []string

func (m *multiFlag) String() string         { return strings.Join(*m, ",") }
func (m *multiFlag) Set(value string) error { *m = append(*m, value); return nil }

func fatal(prefix string, err error) {
	fmt.Fprintf(os.Stderr, "ws-mcp %s: %v\n", prefix, err)
	os.Exit(1)
}

func defaultRoot(root string) string { return defaultRootFrom(root, os.Getenv) }
func defaultRootFrom(root string, getenv func(string) string) string {
	if root != "" && root != "." {
		return root
	}
	if env := getenv("WS_MCP_PROJECT_ROOT"); env != "" {
		return env
	}
	return root
}
