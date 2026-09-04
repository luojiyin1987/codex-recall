package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"time"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/luojiyin1987/codex-recall/internal/codex"
	"github.com/luojiyin1987/codex-recall/internal/indexer"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cxq:", err)
		os.Exit(1)
	}
}

type cliRunner struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func run(args []string) error {
	return newCLIRunner(os.Stdin, os.Stdout, os.Stderr).run(args)
}

func newCLIRunner(stdin io.Reader, stdout, stderr io.Writer) cliRunner {
	return cliRunner{stdin: stdin, stdout: stdout, stderr: stderr}
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

func (c cliRunner) runIndex(args []string) error {
	flags := flag.NewFlagSet("index", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	dbFlag := flags.String("db", "", "SQLite index path (default: CODEX_HOME/.codex-recall/index.db)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("index does not accept positional arguments; usage: cxq index [--home PATH] [--db PATH]")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	result, err := indexer.Refresh(context.Background(), home, indexer.RefreshOptions{DatabasePath: *dbFlag})
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(c.stderr, "cxq: warning: %v\n", warning)
	}

	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "DATABASE\t%s\n", result.DatabasePath)
	fmt.Fprintf(writer, "DISCOVERED\t%d\n", result.Discovered)
	fmt.Fprintf(writer, "INDEXED\t%d\n", result.Indexed)
	fmt.Fprintf(writer, "SKIPPED\t%d\n", result.Skipped)
	fmt.Fprintf(writer, "DELETED\t%d\n", result.Deleted)
	if err := writer.Flush(); err != nil {
		return err
	}
	if len(result.Warnings) > 0 {
		fmt.Fprintf(c.stderr, "cxq: index refresh completed with %d warning(s)\n", len(result.Warnings))
	}
	return nil
}

func (c cliRunner) runStatus(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	dbFlag := flags.String("db", "", "SQLite index path (default: CODEX_HOME/.codex-recall/index.db)")
	jsonFlag := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("status does not accept positional arguments; usage: cxq status [--json] [--home PATH] [--db PATH]")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	result, err := indexer.Status(context.Background(), home, indexer.StatusOptions{DatabasePath: *dbFlag})
	if err != nil {
		return err
	}
	if *jsonFlag {
		return writeStatusJSON(c.stdout, result)
	}

	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "DATABASE\t%s\n", result.DatabasePath)
	fmt.Fprintf(writer, "SESSIONS\t%d\n", result.Sessions)
	fmt.Fprintf(writer, "LATEST_SESSION\t%s\n", formatTimestamp(result.LatestSession))
	fmt.Fprintf(writer, "DATABASE_BYTES\t%d\n", result.DatabaseBytes)
	return writer.Flush()
}

func (c cliRunner) runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	projectFlag := flags.String("project", "", "only sessions whose project exactly matches this value")
	sourceFlag := flags.String("source", "", "only sessions whose source exactly matches this value")
	jsonFlag := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("list does not accept positional arguments; to search conversation text, use cxq search [OPTIONS] QUERY")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}

	sessions, warnings, err := codex.NewCatalog(home).Sessions()
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}
	for _, warning := range warnings {
		fmt.Fprintf(c.stderr, "cxq: warning: %v\n", warning)
	}

	filtered := make([]codex.Session, 0, len(sessions))
	for _, session := range sessions {
		if matchesListFilters(session, *projectFlag, *sourceFlag) {
			filtered = append(filtered, session)
		}
	}
	if *jsonFlag {
		if err := writeListJSON(c.stdout, filtered); err != nil {
			return err
		}
		if len(warnings) > 0 {
			fmt.Fprintf(c.stderr, "cxq: skipped %d unreadable session file(s)\n", len(warnings))
		}
		return nil
	}

	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "DATE\tPROJECT\tSOURCE\tSESSION")
	for _, session := range filtered {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", formatDate(session), session.Project(), session.Source, session.ID)
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	if len(warnings) > 0 {
		fmt.Fprintf(c.stderr, "cxq: skipped %d unreadable session file(s)\n", len(warnings))
	}
	return nil
}

func matchesListFilters(session codex.Session, project, source string) bool {
	project = strings.TrimSpace(project)
	source = strings.TrimSpace(source)
	if project != "" && !strings.EqualFold(session.Project(), project) {
		return false
	}
	if source != "" && !strings.EqualFold(session.Source, source) {
		return false
	}
	return true
}

func (c cliRunner) runSearch(args []string) error {
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	limitFlag := flags.Int("limit", 20, "maximum number of matching sessions to display")
	projectFlag := flags.String("project", "", "only sessions whose project exactly matches this value")
	sourceFlag := flags.String("source", "", "only sessions whose source exactly matches this value")
	indexFlag := flags.Bool("index", false, "search the derived SQLite FTS index instead of live rollout files")
	dbFlag := flags.String("db", "", "SQLite index path for --index (default: CODEX_HOME/.codex-recall/index.db)")
	jsonFlag := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("search requires QUERY; to list sessions without a text query, use cxq list [--project PROJECT] [--source SOURCE]")
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("search accepts exactly one QUERY; usage: cxq search [--json] [--index] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY")
	}
	if strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("search requires a non-blank QUERY")
	}
	if *limitFlag <= 0 {
		return fmt.Errorf("limit must be greater than zero")
	}
	query := flags.Arg(0)
	if !*indexFlag && strings.TrimSpace(*dbFlag) != "" {
		return fmt.Errorf("--db requires --index")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	if *indexFlag {
		result, err := indexer.Search(context.Background(), home, indexer.SearchOptions{
			DatabasePath: *dbFlag,
			Query:        query,
			Limit:        *limitFlag,
			Project:      *projectFlag,
			Source:       *sourceFlag,
		})
		if err != nil {
			return err
		}
		if *jsonFlag {
			return writeIndexedSearchJSON(c.stdout, query, result.Matches)
		}

		writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "DATE\tPROJECT\tSOURCE\tROLE\tSESSION\tMATCH")
		for _, match := range result.Matches {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
				formatTimestamp(match.Session.Timestamp), match.Session.Project, match.Session.Source, match.Role, match.Session.ID, match.Snippet)
		}
		return writer.Flush()
	}

	result, err := codex.Search(home, codex.SearchOptions{
		Query:   query,
		Limit:   *limitFlag,
		Project: *projectFlag,
		Source:  *sourceFlag,
	})
	if err != nil {
		return err
	}
	for _, searchErr := range result.Warnings {
		fmt.Fprintf(c.stderr, "cxq: warning: %v\n", searchErr)
	}
	if *jsonFlag {
		if err := writeLiveSearchJSON(c.stdout, query, result.Matches); err != nil {
			return err
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(c.stderr, "cxq: skipped %d unreadable session file(s)\n", len(result.Warnings))
		}
		return nil
	}

	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "DATE\tPROJECT\tSOURCE\tROLE\tSESSION\tMATCH")
	for _, match := range result.Matches {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			formatDate(match.Session), match.Session.Project(), match.Session.Source, match.Role, match.Session.ID, match.Snippet)
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	if len(result.Warnings) > 0 {
		fmt.Fprintf(c.stderr, "cxq: skipped %d unreadable session file(s)\n", len(result.Warnings))
	}
	return nil
}

func (c cliRunner) runCompare(args []string) error {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	dbFlag := flags.String("db", "", "SQLite index path (default: CODEX_HOME/.codex-recall/index.db)")
	limitFlag := flags.Int("limit", 20, "maximum results requested from each search backend")
	projectFlag := flags.String("project", "", "only sessions whose project exactly matches this value")
	sourceFlag := flags.String("source", "", "only sessions whose source exactly matches this value")
	jsonFlag := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq compare [--json] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY")
	}
	if *limitFlag <= 0 {
		return fmt.Errorf("limit must be greater than zero")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	result, err := indexer.Compare(context.Background(), home, indexer.CompareOptions{
		DatabasePath: *dbFlag,
		Query:        flags.Arg(0),
		Limit:        *limitFlag,
		Project:      *projectFlag,
		Source:       *sourceFlag,
	})
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(c.stderr, "cxq: warning: %v\n", warning)
	}
	if *jsonFlag {
		if err := writeCompareJSON(c.stdout, flags.Arg(0), result); err != nil {
			return err
		}
		if len(result.Warnings) > 0 {
			fmt.Fprintf(c.stderr, "cxq: live search completed with %d warning(s)\n", len(result.Warnings))
		}
		return nil
	}

	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "DATABASE\t%s\n", result.DatabasePath)
	fmt.Fprintf(writer, "LIVE_RESULTS\t%d\n", result.LiveResults)
	fmt.Fprintf(writer, "INDEX_RESULTS\t%d\n", result.IndexResults)
	fmt.Fprintf(writer, "LIVE_SESSIONS\t%d\n", result.LiveSessions)
	fmt.Fprintf(writer, "INDEX_SESSIONS\t%d\n", result.IndexSessions)
	fmt.Fprintf(writer, "OVERLAP\t%d\n", result.Overlap)
	fmt.Fprintf(writer, "LIVE_ONLY\t%d\n", result.LiveOnly)
	fmt.Fprintf(writer, "INDEX_ONLY\t%d\n", result.IndexOnly)
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "STATUS\tSESSION\tLIVE_ROLE\tLIVE_MATCH\tINDEX_ROLE\tINDEX_MATCH")
	for _, entry := range result.Entries {
		liveRole, liveMatch := "-", "-"
		if entry.Live != nil {
			liveRole = entry.Live.Role
			liveMatch = entry.Live.Snippet
		}
		indexRole, indexMatch := "-", "-"
		if entry.Indexed != nil {
			indexRole = entry.Indexed.Role
			indexMatch = entry.Indexed.Snippet
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			entry.Status, entry.SessionID, liveRole, liveMatch, indexRole, indexMatch)
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if len(result.Warnings) > 0 {
		fmt.Fprintf(c.stderr, "cxq: live search completed with %d warning(s)\n", len(result.Warnings))
	}
	return nil
}

func (c cliRunner) runShow(args []string) error {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
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
	session, err := codex.NewCatalog(home).Resolve(flags.Arg(0))
	if err != nil {
		return err
	}
	messages, err := codex.ReadConversation(session.Path)
	if err != nil {
		return fmt.Errorf("read conversation: %w", err)
	}

	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
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
		fmt.Fprintln(c.stdout, "\n(no user/assistant messages)")
		return nil
	}
	for _, message := range messages {
		label := message.Role
		if !message.Timestamp.IsZero() {
			label += " " + message.Timestamp.Local().Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(c.stdout, "\n[%s]\n%s\n", label, message.Text)
	}
	return nil
}

func (c cliRunner) runResume(args []string) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
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
	session, err := codex.NewCatalog(home).Resolve(flags.Arg(0))
	if err != nil {
		return err
	}
	return c.resumeSession(home, session)
}

func (c cliRunner) runOpen(args []string) error {
	flags := flag.NewFlagSet("open", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	targetFlag := flags.String("target", "auto", "open target: auto, vscode, or cli")
	schemeFlag := flags.String("vscode-scheme", "vscode", "VS Code URI scheme: vscode or vscode-insiders")
	if err := flags.Parse(args); err != nil {
		return err
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

type lookPathFunc func(string) (string, error)

type openCommand struct {
	Name string
	Args []string
	Dir  string
}

func resolveOpenTarget(requested, source string) (string, error) {
	switch requested {
	case "vscode", "cli":
		return requested, nil
	case "auto":
		switch strings.ToLower(source) {
		case "vscode":
			return "vscode", nil
		case "cli":
			return "cli", nil
		default:
			return "", fmt.Errorf("session source %q has no automatic open target; use --target vscode or --target cli", emptyDash(source))
		}
	default:
		return "", fmt.Errorf("invalid open target %q; use auto, vscode, or cli", requested)
	}
}

func vscodeConversationURL(scheme, sessionID string) (string, error) {
	if scheme != "vscode" && scheme != "vscode-insiders" {
		return "", fmt.Errorf("invalid VS Code URI scheme %q; use vscode or vscode-insiders", scheme)
	}
	if !validCodexSessionID(sessionID) {
		return "", fmt.Errorf("invalid Codex session ID %q; expected a UUID", sessionID)
	}
	return (&url.URL{
		Scheme: scheme,
		Host:   "openai.chatgpt",
		Path:   "/local/" + sessionID,
	}).String(), nil
}

func validCodexSessionID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for index := range id {
		switch index {
		case 8, 13, 18, 23:
			if id[index] != '-' {
				return false
			}
		default:
			character := id[index]
			if !((character >= '0' && character <= '9') ||
				(character >= 'a' && character <= 'f') ||
				(character >= 'A' && character <= 'F')) {
				return false
			}
		}
	}
	return true
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

func newOpenCommand(conversationURL, goos string, env []string, lookPath lookPathFunc) (openCommand, error) {
	if goos == "linux" && (hasEnvValue(env, "WSL_DISTRO_NAME") || hasEnvValue(env, "WSL_INTEROP")) {
		if hasEnvValue(env, "VSCODE_IPC_HOOK_CLI") {
			if name, err := lookPath("code"); err == nil {
				return openCommand{
					Name: name,
					Args: []string{"--openExternal", conversationURL},
				}, nil
			}
		}

		name, err := lookPath("cmd.exe")
		if err != nil {
			return openCommand{}, fmt.Errorf("find cmd.exe: %w", err)
		}
		return openCommand{
			Name: name,
			Args: []string{"/d", "/s", "/c", "start", "", conversationURL},
			Dir:  filepath.Dir(name),
		}, nil
	}

	var opener string
	var args []string
	switch goos {
	case "windows":
		opener = "cmd.exe"
		args = []string{"/d", "/s", "/c", "start", "", conversationURL}
	case "darwin":
		opener = "open"
		args = []string{conversationURL}
	case "linux":
		opener = "xdg-open"
		args = []string{conversationURL}
	default:
		return openCommand{}, fmt.Errorf("cannot open VS Code on %s", goos)
	}

	name, err := lookPath(opener)
	if err != nil {
		return openCommand{}, fmt.Errorf("find %s: %w", opener, err)
	}
	return openCommand{Name: name, Args: args}, nil
}

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
	return formatTimestamp(session.Timestamp)
}

func formatTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "-"
	}
	return timestamp.Local().Format("2006-01-02 15:04")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func (c cliRunner) printUsage() {
	fmt.Fprintln(c.stderr, `codex-recall (cxq)

Usage:
  cxq index [--home PATH] [--db PATH]
  cxq status [--json] [--home PATH] [--db PATH]
  cxq list [--json] [--home PATH] [--project PROJECT] [--source SOURCE]
  cxq search [--json] [--index] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY
  cxq compare [--json] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY
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
  cxq compare --project deepseek-harness-remote "WebRTC"
  cxq compare --json --project deepseek-harness-remote "WebRTC"`)
}
