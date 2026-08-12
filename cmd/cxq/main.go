package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
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

	home := *homeFlag
	if home == "" {
		var err error
		home, err = codex.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve Codex home: %w", err)
		}
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
		date := "-"
		if !session.Timestamp.IsZero() {
			date = session.Timestamp.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", date, session.Project(), session.Source, session.ID)
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "cxq: skipped %d unreadable session file(s)\n", skipped)
	}
	return nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `codex-recall (cxq)

Usage:
  cxq list [--home PATH]

Commands:
  list    Discover and list local Codex sessions
  help    Show this help`)
}
