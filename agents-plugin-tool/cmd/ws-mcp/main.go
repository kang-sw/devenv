package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/kang-sw/devenv/internal/mcp"
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
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp <version|doctor|serve>")
}

func doctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	_ = fs.Parse(args)

	report := wsdoc.Doctor(*root)
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

	server := mcp.NewServer(*root, version)
	if err := server.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp serve: %v\n", err)
		os.Exit(1)
	}
}
