package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luojiyin1987/codex-recall/internal/codex"
)

func (c cliRunner) runOpen(args []string) error {
	flags := flag.NewFlagSet("open", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	targetFlag := flags.String("target", "auto", "open target: auto, vscode, or cli")
	schemeFlag := flags.String("vscode-scheme", "vscode", "VS Code URI scheme: vscode or vscode-insiders")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("open: %w", err)
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq open [--home PATH] [--target TARGET] [--vscode-scheme SCHEME] SESSION")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	session, err := codex.NewCatalog(home).Resolve(flags.Arg(0))
	if err != nil {
		return err
	}
	target, err := resolveOpenTarget(*targetFlag, session.Source)
	if err != nil {
		return err
	}
	if target == "cli" {
		return c.resumeSession(home, session)
	}

	conversationURL, err := vscodeConversationURL(*schemeFlag, session.ID)
	if err != nil {
		return err
	}
	return openConversationURL(conversationURL)
}

func (c cliRunner) resumeSession(home string, session codex.Session) error {
	cmd := exec.Command("codex", "resume", session.ID)
	cmd.Stdin = c.stdin
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	cmdEnv := withCodexHome(os.Environ(), home)
	cmdEnv, cleanup, shimErr := withWSLTermProgramProbeShim(cmdEnv, exec.LookPath)
	if shimErr != nil {
		fmt.Fprintf(c.stderr, "cxq: warning: prepare WSL terminal probe: %v\n", shimErr)
	} else {
		defer cleanup()
	}
	cmd.Env = cmdEnv
	if dir, dirErr := resolveResumeDir(session.CWD); dirErr != nil {
		fmt.Fprintf(c.stderr, "cxq: warning: session cwd %q is unavailable (%v); resuming from current directory\n", session.CWD, dirErr)
	} else if dir != "" {
		cmd.Dir = dir
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex resume %s: %w", session.ID, err)
	}
	return nil
}

func openConversationURL(conversationURL string) error {
	spec, err := newOpenCommand(conversationURL, runtime.GOOS, os.Environ(), exec.LookPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(spec.Name, spec.Args...)
	cmd.Dir = spec.Dir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open VS Code conversation: %w", err)
	}
	return nil
}
