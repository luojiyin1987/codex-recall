package main

import (
	"flag"
	"fmt"
	"text/tabwriter"

	"github.com/luojiyin1987/codex-recall/internal/codex"
)

func (c cliRunner) runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	projectFlag := flags.String("project", "", "only sessions whose project exactly matches this value")
	sourceFlag := flags.String("source", "", "only sessions whose source exactly matches this value")
	jsonFlag := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("list: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("list does not accept positional arguments; to search conversation text, use cxq search [OPTIONS] QUERY")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}

	sessions, warnings, err := codex.NewCatalog(home).SessionsContext(c.ctx)
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}
	for _, warning := range warnings {
		fmt.Fprintf(c.stderr, "cxq: warning: %v\n", warning)
	}

	filtered := make([]codex.Session, 0, len(sessions))
	for _, session := range sessions {
		if session.MatchesFilters(*projectFlag, *sourceFlag) {
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
