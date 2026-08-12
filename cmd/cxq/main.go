package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/luojiyin1987/codex-recall/internal/codex"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cxq:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "list":
		return runList(args[1:])
	case "search":
		return runSearch(args[1:])
	case "show":
		return runShow(args[1:])
	case "resume":
		return runResume(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("list does not accept positional arguments")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}

	files, err := codex.DiscoverFiles(home)
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}

	sessions := make([]codex.Session, 0, len(files))
	var skipped int
	for _, path := range files {
		session, err := codex.ParseFile(path)
		if err != nil {
			skipped++
			fmt.Fprintf(os.Stderr, "cxq: warning: %v\n", err)
			continue
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp.After(sessions[j].Timestamp)
	})

	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "DATE\tPROJECT\tSOURCE\tSESSION")
	for _, session := range sessions {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", formatDate(session), session.Project(), session.Source, session.ID)
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "cxq: skipped %d unreadable session file(s)\n", skipped)
	}
	return nil
}

func runSearch(args []string) error {
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	limitFlag := flags.Int("limit", 20, "maximum number of matching sessions to display")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq search [--home PATH] [--limit N] QUERY")
	}
	if *limitFlag <= 0 {
		return fmt.Errorf("limit must be greater than zero")
	}
	query := flags.Arg(0)

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	files, err := codex.SearchCandidateFiles(home, query)
	if err != nil {
		return fmt.Errorf("discover search candidates: %w", err)
	}

	matches := make([]codex.SearchMatch, 0)
	var skipped int
	for _, path := range files {
		match, ok, err := codex.SearchFile(path, query)
		if err != nil {
			skipped++
			fmt.Fprintf(os.Stderr, "cxq: warning: %v\n", err)
			continue
		}
		if ok {
			matches = append(matches, match)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Session.Timestamp.After(matches[j].Session.Timestamp)
	})
	if len(matches) > *limitFlag {
		matches = matches[:*limitFlag]
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "DATE\tPROJECT\tROLE\tSESSION\tMATCH")
	for _, match := range matches {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			formatDate(match.Session), match.Session.Project(), match.Role, match.Session.ID, match.Snippet)
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "cxq: skipped %d unreadable session file(s)\n", skipped)
	}
	return nil
}

func runShow(args []string) error {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq show [--home PATH] SESSION")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	session, err := codex.ResolveSession(home, flags.Arg(0))
	if err != nil {
		return err
	}
	messages, err := codex.ReadConversation(session.Path)
	if err != nil {
		return fmt.Errorf("read conversation: %w", err)
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "SESSION\t%s\n", session.ID)
	fmt.Fprintf(writer, "DATE\t%s\n", formatDate(session))
	fmt.Fprintf(writer, "PROJECT\t%s\n", session.Project())
	fmt.Fprintf(writer, "SOURCE\t%s\n", session.Source)
	fmt.Fprintf(writer, "CWD\t%s\n", emptyDash(session.CWD))
	fmt.Fprintf(writer, "PATH\t%s\n", session.Path)
	if err := writer.Flush(); err != nil {
		return err
	}

	if len(messages) == 0 {
		fmt.Fprintln(os.Stdout, "\n(no user/assistant messages)")
		return nil
	}
	for _, message := range messages {
		label := message.Role
		if !message.Timestamp.IsZero() {
			label += " " + message.Timestamp.Local().Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(os.Stdout, "\n[%s]\n%s\n", label, message.Text)
	}
	return nil
}

func runResume(args []string) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq resume [--home PATH] SESSION")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	session, err := codex.ResolveSession(home, flags.Arg(0))
	if err != nil {
		return err
	}

	cmd := exec.Command("codex", "resume", session.ID)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmdEnv := withCodexHome(os.Environ(), home)
	cmdEnv, cleanup, shimErr := withWSLTermProgramProbeShim(cmdEnv, exec.LookPath)
	if shimErr != nil {
		fmt.Fprintf(os.Stderr, "cxq: warning: prepare WSL terminal probe: %v\n", shimErr)
	} else {
		defer cleanup()
	}
	cmd.Env = cmdEnv
	if dir, dirErr := resolveResumeDir(session.CWD); dirErr != nil {
		fmt.Fprintf(os.Stderr, "cxq: warning: session cwd %q is unavailable (%v); resuming from current directory\n", session.CWD, dirErr)
	} else if dir != "" {
		cmd.Dir = dir
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex resume %s: %w", session.ID, err)
	}
	return nil
}

type lookPathFunc func(string) (string, error)

func withWSLTermProgramProbeShim(env []string, lookPath lookPathFunc) ([]string, func(), error) {
	cleanup := func() {}
	if !hasEnvValue(env, "WSL_DISTRO_NAME") && !hasEnvValue(env, "WSL_INTEROP") {
		return env, cleanup, nil
	}

	realCmd, err := lookPath("cmd.exe")
	if err != nil {
		return env, cleanup, nil
	}
	shimDir, err := os.MkdirTemp("", "cxq-wsl-cmd-")
	if err != nil {
		return env, cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(shimDir) }

	// Codex probes Windows TERM_PROGRAM during WSL startup.
	// cmd.exe can block when its current directory is a WSL UNC path.
	const shim = `#!/bin/sh
if [ "$#" -eq 4 ] && [ "$1" = "/d" ] && [ "$2" = "/s" ] && [ "$3" = "/c" ] && [ "$4" = "set TERM_PROGRAM" ]; then
    cd "$_CXQ_REAL_CMD_DIR" || exit 1
fi
exec "$_CXQ_REAL_CMD_EXE" "$@"
`
	if err := os.WriteFile(filepath.Join(shimDir, "cmd.exe"), []byte(shim), 0o700); err != nil {
		cleanup()
		return env, func() {}, err
	}

	env = withEnvValue(env, "_CXQ_REAL_CMD_EXE", realCmd)
	env = withEnvValue(env, "_CXQ_REAL_CMD_DIR", filepath.Dir(realCmd))
	path := envValue(env, "PATH")
	if path == "" {
		path = shimDir
	} else {
		path = shimDir + string(os.PathListSeparator) + path
	}
	env = withEnvValue(env, "PATH", path)
	return env, cleanup, nil
}

func hasEnvValue(env []string, key string) bool {
	return envValue(env, key) != ""
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func withEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				result = append(result, prefix+value)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, prefix+value)
	}
	return result
}

func resolveResumeDir(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return cwd, nil
}

func resolveHome(homeFlag string) (string, error) {
	if homeFlag != "" {
		return homeFlag, nil
	}
	home, err := codex.ResolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve Codex home: %w", err)
	}
	return home, nil
}

func withCodexHome(env []string, home string) []string {
	const prefix = "CODEX_HOME="
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				result = append(result, prefix+home)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, prefix+home)
	}
	return result
}

func formatDate(session codex.Session) string {
	if session.Timestamp.IsZero() {
		return "-"
	}
	return session.Timestamp.Local().Format("2006-01-02 15:04")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `codex-recall (cxq)

Usage:
  cxq list [--home PATH]
  cxq search [--home PATH] [--limit N] QUERY
  cxq show [--home PATH] SESSION
  cxq resume [--home PATH] SESSION

Commands:
  list    Discover and list local Codex sessions
  search  Search user and assistant conversation text
  show    Show user and assistant messages from a session
  resume  Resume a session with the official Codex CLI
  help    Show this help`)
}
