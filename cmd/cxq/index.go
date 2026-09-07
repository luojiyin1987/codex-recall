package main

import (
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/luojiyin1987/codex-recall/internal/indexer"
)

func (c cliRunner) runIndex(args []string) error {
	flags := flag.NewFlagSet("index", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	dbFlag := flags.String("db", "", "SQLite index path (default: CODEX_HOME/.codex-recall/index.db)")
	profileFlag := flags.Bool("profile", false, "show index refresh timing profile on stderr")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("index: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("index does not accept positional arguments; usage: cxq index [--profile] [--home PATH] [--db PATH]")
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
	if *profileFlag {
		return writeIndexRefreshProfile(c.stderr, result.Profile)
	}
	return nil
}

func writeIndexRefreshProfile(w io.Writer, profile indexer.RefreshProfile) error {
	writer := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "INDEX_PROFILE")
	fmt.Fprintf(writer, "PREPARE\t%s\n", profile.Prepare)
	fmt.Fprintf(writer, "DATABASE_OPEN\t%s\n", profile.DatabaseOpen)
	fmt.Fprintf(writer, "DISCOVERY\t%s\n", profile.Build.Discovery)
	fmt.Fprintf(writer, "METADATA_PARSE\t%s\n", profile.Build.MetadataParse)
	fmt.Fprintf(writer, "CATALOG_FINALIZE\t%s\n", profile.Build.CatalogFinalize)
	fmt.Fprintf(writer, "INDEX_STATE_READ\t%s\n", profile.Build.IndexStateRead)
	fmt.Fprintf(writer, "FINGERPRINT\t%s\n", profile.Build.Fingerprint)
	fmt.Fprintf(writer, "HASH\t%s\n", profile.Build.Hash)
	fmt.Fprintf(writer, "CONVERSATION_DECODE\t%s\n", profile.Build.ConversationDecode)
	fmt.Fprintf(writer, "DATABASE_WRITE\t%s\n", profile.Build.DatabaseWrite)
	fmt.Fprintf(writer, "STALE_CLEANUP\t%s\n", profile.Build.StaleCleanup)
	fmt.Fprintf(writer, "BUILD\t%s\n", profile.Build.Total)
	fmt.Fprintf(writer, "DATABASE_CLOSE\t%s\n", profile.DatabaseClose)
	fmt.Fprintf(writer, "TOTAL\t%s\n", profile.Total)
	fmt.Fprintf(writer, "ROLLOUT_FILES\t%d\n", profile.Build.RolloutFiles)
	fmt.Fprintf(writer, "METADATA_UNREADABLE_FILES\t%d\n", profile.Build.MetadataUnreadableFiles)
	fmt.Fprintf(writer, "FILES_FINGERPRINTED\t%d\n", profile.Build.FilesFingerprinted)
	fmt.Fprintf(writer, "FINGERPRINT_FAST_PATHS\t%d\n", profile.Build.FingerprintFastPaths)
	fmt.Fprintf(writer, "FINGERPRINT_UPDATES\t%d\n", profile.Build.FingerprintUpdates)
	fmt.Fprintf(writer, "FINGERPRINT_BATCHES_WRITTEN\t%d\n", profile.Build.FingerprintBatchesWritten)
	fmt.Fprintf(writer, "FILES_HASHED\t%d\n", profile.Build.FilesHashed)
	fmt.Fprintf(writer, "HASH_BYTES\t%d\n", profile.Build.HashBytes)
	fmt.Fprintf(writer, "FILES_DECODED\t%d\n", profile.Build.FilesDecoded)
	fmt.Fprintf(writer, "CONVERSATION_BYTES\t%d\n", profile.Build.ConversationBytes)
	fmt.Fprintf(writer, "MESSAGES_DECODED\t%d\n", profile.Build.MessagesDecoded)
	fmt.Fprintf(writer, "BATCHES_WRITTEN\t%d\n", profile.Build.BatchesWritten)
	return writer.Flush()
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
