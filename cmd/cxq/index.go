package main

import (
	"flag"
	"fmt"
	"text/tabwriter"

	"github.com/luojiyin1987/codex-recall/internal/indexer"
)

func (c cliRunner) runIndex(args []string) error {
	flags := flag.NewFlagSet("index", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	dbFlag := flags.String("db", "", "SQLite index path (default: CODEX_HOME/.codex-recall/index.db)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("index: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("index does not accept positional arguments; usage: cxq index [--home PATH] [--db PATH]")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	result, err := indexer.Refresh(c.ctx, home, indexer.RefreshOptions{DatabasePath: *dbFlag})
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
		return fmt.Errorf("status: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("status does not accept positional arguments; usage: cxq status [--json] [--home PATH] [--db PATH]")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	result, err := indexer.Status(c.ctx, home, indexer.StatusOptions{DatabasePath: *dbFlag})
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
