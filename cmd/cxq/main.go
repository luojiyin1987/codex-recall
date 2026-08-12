package main

import (
	"flag"
	"fmt"
	"os"
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

func formatDate(session codex.Session) string {
	if session.Timestamp.IsZero() {
		return "-"
	}
	return session.Timestamp.Local().Format("2006-01-02 15:04")
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `codex-recall (cxq)

Usage:
  cxq list [--home PATH]
  cxq search [--home PATH] [--limit N] QUERY

Commands:
  list    Discover and list local Codex sessions
  search  Search user and assistant conversation text
  help    Show this help`)
}
