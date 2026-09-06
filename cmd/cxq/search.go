package main

import (
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/luojiyin1987/codex-recall/internal/codex"
	"github.com/luojiyin1987/codex-recall/internal/indexer"
)

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
		return fmt.Errorf("search: %w", err)
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
		result, err := indexer.Search(c.ctx, home, indexer.SearchOptions{
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

	result, err := codex.SearchContext(c.ctx, home, codex.SearchOptions{
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
		return fmt.Errorf("compare: %w", err)
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
	result, err := indexer.Compare(c.ctx, home, indexer.CompareOptions{
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

func (c cliRunner) runPack(args []string) error {
	flags := flag.NewFlagSet("pack", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	dbFlag := flags.String("db", "", "SQLite index path (default: CODEX_HOME/.codex-recall/index.db)")
	limitFlag := flags.Int("limit", 5, "maximum number of indexed sessions to include")
	projectFlag := flags.String("project", "", "only sessions whose project exactly matches this value")
	sourceFlag := flags.String("source", "", "only sessions whose source exactly matches this value")
	jsonFlag := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("pack: %w", err)
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq pack [--json] [--db PATH] [--home PATH] [--limit N] [--project PROJECT] [--source SOURCE] QUERY")
	}
	if *limitFlag <= 0 {
		return fmt.Errorf("limit must be greater than zero")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	result, err := indexer.Pack(c.ctx, home, indexer.PackOptions{
		DatabasePath: *dbFlag,
		Query:        flags.Arg(0),
		Limit:        *limitFlag,
		Project:      *projectFlag,
		Source:       *sourceFlag,
	})
	if err != nil {
		return err
	}
	if *jsonFlag {
		return writePackJSON(c.stdout, result)
	}

	fmt.Fprintln(c.stdout, "CONTEXT_PACK")
	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "QUERY\t%s\n", result.Query)
	fmt.Fprintf(writer, "PROJECT\t%s\n", emptyDash(result.Project))
	fmt.Fprintf(writer, "SOURCE\t%s\n", emptyDash(result.Source))
	fmt.Fprintf(writer, "DATABASE\t%s\n", result.DatabasePath)
	fmt.Fprintf(writer, "EVIDENCE\t%d\n", len(result.Evidence))
	if err := writer.Flush(); err != nil {
		return err
	}

	for rank, evidence := range result.Evidence {
		fmt.Fprintf(c.stdout, "\n[%d]\n", rank+1)
		writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(writer, "SESSION\t%s\n", evidence.SessionID)
		fmt.Fprintf(writer, "DATE\t%s\n", formatTimestamp(evidence.Timestamp))
		fmt.Fprintf(writer, "PROJECT\t%s\n", emptyDash(evidence.Project))
		fmt.Fprintf(writer, "SOURCE\t%s\n", emptyDash(evidence.Source))
		fmt.Fprintf(writer, "ROLE\t%s\n", evidence.Role)
		fmt.Fprintf(writer, "ORDINAL\t%d\n", evidence.Ordinal)
		fmt.Fprintf(writer, "SCORE\t%g\n", evidence.Score)
		fmt.Fprintf(writer, "WHY\t%s\n", evidence.Why)
		fmt.Fprintf(writer, "RESUME\t%s\n", evidence.ResumeCommand)
		fmt.Fprintf(writer, "MATCH\t%s\n", evidence.Snippet)
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	return nil
}
