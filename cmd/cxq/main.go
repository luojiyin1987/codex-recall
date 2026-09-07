package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := runContext(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cxq:", err)
		os.Exit(1)
	}
}

func runContext(ctx context.Context, args []string) error {
	return newCLIRunnerWithContext(ctx, os.Stdin, os.Stdout, os.Stderr).run(args)
}

type cliRunner struct {
	ctx    context.Context
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func newCLIRunner(stdin io.Reader, stdout, stderr io.Writer) cliRunner {
	return newCLIRunnerWithContext(context.Background(), stdin, stdout, stderr)
}

func newCLIRunnerWithContext(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) cliRunner {
	if ctx == nil {
		ctx = context.Background()
	}
	return cliRunner{ctx: ctx, stdin: stdin, stdout: stdout, stderr: stderr}
}

func (c cliRunner) run(args []string) error {
	if len(args) == 0 {
		c.printUsage()
		return nil
	}

	switch args[0] {
	case "index":
		return c.runIndex(args[1:])
	case "status":
		return c.runStatus(args[1:])
	case "list":
		return c.runList(args[1:])
	case "search":
		return c.runSearch(args[1:])
	case "compare":
		return c.runCompare(args[1:])
	case "pack":
		return c.runPack(args[1:])
	case "show":
		return c.runShow(args[1:])
	case "resume":
		return c.runResume(args[1:])
	case "open":
		return c.runOpen(args[1:])
	case "version", "-v", "--version":
		return c.runVersion(args[1:])
	case "help", "-h", "--help":
		c.printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (c cliRunner) printUsage() {
	fmt.Fprintln(c.stderr, `codex-recall (cxq)

Usage:
  cxq index [--profile] [--full-hash] [--home PATH] [--db PATH]
  cxq status [--json] [--home PATH] [--db PATH]
  cxq list [--json] [--home PATH] [--project PROJECT] [--source SOURCE]
  cxq search [--json] [--index] [--explain] [--profile] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY
  cxq compare [--json] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY
  cxq pack [--json] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY
  cxq show [--home PATH] SESSION
  cxq resume [--home PATH] SESSION
  cxq open [--home PATH] [--target TARGET] [--vscode-scheme SCHEME] SESSION
  cxq version

Commands:
  index   Build or refresh the local derived SQLite index
  status  Show basic facts about the existing derived index
  list    Discover and list local Codex sessions, optionally filtered
  search  Search live conversation text, or the derived FTS index with --index
  compare Compare live and indexed search result sets for the same query
  pack    Build a deterministic context pack from indexed search evidence
  show    Show user and assistant messages from a session
  resume  Resume a session with the official Codex CLI
  open    Open a session in its source client
  version Show the cxq version
  help    Show this help

Examples:
  cxq index
  cxq status
  cxq status --json
  cxq list --project deepseek-harness-remote
  cxq list --json --project deepseek-harness-remote
  cxq search --project deepseek-harness-remote "WebRTC"
  cxq search --json --project deepseek-harness-remote "WebRTC"
  cxq search --index --json --project deepseek-harness-remote "WebRTC"
  cxq search --index --explain --project deepseek-harness-remote "WebRTC"
  cxq search --index --profile --project deepseek-harness-remote "WebRTC"
  cxq compare --project deepseek-harness-remote "WebRTC"
  cxq compare --json --project deepseek-harness-remote "WebRTC"
  cxq pack --project deepseek-harness-remote "WebRTC"
  cxq pack --json --project deepseek-harness-remote "WebRTC"`)
}
