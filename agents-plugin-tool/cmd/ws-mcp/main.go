package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

var version = "0.1.0-dev"

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
	case "serve":
		serve(os.Args[2:])
	case "agents":
		agents(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp <version|doctor|serve|agents>")
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

	server := mcp.NewServer(defaultRoot(*root), version)
	if err := server.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp serve: %v\n", err)
		os.Exit(1)
	}
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
	case "oneshot":
		agentsOneShot(args[1:])
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
	fmt.Fprintln(os.Stderr, "usage: ws-mcp agents <register|call|oneshot|print|erase>")
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
	backend := fs.String("backend", "codex", "agent backend")
	tier := fs.String("tier", "core", "workload tier")
	model := fs.String("model", "", "backend model override")
	systemFile := fs.String("system-prompt-file", "", "system prompt file")
	var promptRefs multiFlag
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
		Tier:             *tier,
		Model:            *model,
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
	_, text, err := wsagent.NewManager(wsagent.Options{}).Call(wsagent.CallOptions{
		Root:   defaultRoot(*root),
		Name:   *name,
		Prompt: prompt,
	})
	if err != nil {
		fatal("agents call", err)
	}
	fmt.Print(text)
}

func agentsOneShot(args []string) {
	fs := flag.NewFlagSet("agents oneshot", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	name := fs.String("name", "", "temporary agent name")
	backend := fs.String("backend", "codex", "agent backend")
	tier := fs.String("tier", "core", "workload tier")
	model := fs.String("model", "", "backend model override")
	systemFile := fs.String("system-prompt-file", "", "system prompt file")
	promptFile := fs.String("prompt-file", "", "prompt file; use - for stdin")
	var promptRefs multiFlag
	fs.Var(&promptRefs, "prompt-ref", "logical prompt reference")
	_ = fs.Parse(args)

	systemText, err := readOptionalFile(*systemFile)
	if err != nil {
		fatal("agents oneshot", err)
	}
	prompt, err := promptFromArgs(fs.Args(), *promptFile)
	if err != nil {
		fatal("agents oneshot", err)
	}
	text, err := wsagent.NewManager(wsagent.Options{}).OneShot(wsagent.OneShotOptions{
		Root:             defaultRoot(*root),
		Name:             *name,
		Backend:          *backend,
		Tier:             *tier,
		Model:            *model,
		PromptRefs:       promptRefs,
		SystemPromptText: systemText,
		Prompt:           prompt,
	})
	if err != nil {
		fatal("agents oneshot", err)
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
		if promptFile == "-" {
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", fmt.Errorf("read stdin prompt: %w", err)
			}
			return string(raw), nil
		}
		raw, err := os.ReadFile(promptFile)
		if err != nil {
			return "", fmt.Errorf("read prompt file: %w", err)
		}
		return string(raw), nil
	}
	if len(args) == 0 {
		return "", fmt.Errorf("prompt is required")
	}
	return strings.Join(args, " "), nil
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
