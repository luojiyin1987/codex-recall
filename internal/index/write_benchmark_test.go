package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type writeBenchmarkTotals struct {
	validation       time.Duration
	transactionBegin time.Duration
	statementPrepare time.Duration
	sessionUpsert    time.Duration
	ftsDelete        time.Duration
	messageDelete    time.Duration
	messageInsert    time.Duration
	ftsInsert        time.Duration
	commit           time.Duration
}

func (t *writeBenchmarkTotals) add(profile WriteProfile) {
	t.validation += profile.Validation
	t.transactionBegin += profile.TransactionBegin
	t.statementPrepare += profile.StatementPrepare
	t.sessionUpsert += profile.SessionUpsert
	t.ftsDelete += profile.FTSDelete
	t.messageDelete += profile.MessageDelete
	t.messageInsert += profile.MessageInsert
	t.ftsInsert += profile.FTSInsert
	t.commit += profile.Commit
}

func (t writeBenchmarkTotals) report(b *testing.B) {
	b.Helper()
	iterations := float64(b.N)
	b.ReportMetric(float64(t.validation.Nanoseconds())/iterations, "validation-ns/op")
	b.ReportMetric(float64(t.transactionBegin.Nanoseconds())/iterations, "tx-begin-ns/op")
	b.ReportMetric(float64(t.statementPrepare.Nanoseconds())/iterations, "prepare-ns/op")
	b.ReportMetric(float64(t.sessionUpsert.Nanoseconds())/iterations, "session-upsert-ns/op")
	b.ReportMetric(float64(t.ftsDelete.Nanoseconds())/iterations, "fts-delete-ns/op")
	b.ReportMetric(float64(t.messageDelete.Nanoseconds())/iterations, "message-delete-ns/op")
	b.ReportMetric(float64(t.messageInsert.Nanoseconds())/iterations, "message-insert-ns/op")
	b.ReportMetric(float64(t.ftsInsert.Nanoseconds())/iterations, "fts-insert-ns/op")
	b.ReportMetric(float64(t.commit.Nanoseconds())/iterations, "commit-ns/op")
}

func BenchmarkSQLiteWritePhases(b *testing.B) {
	cases := []struct {
		name         string
		sessions     int
		messages     int
		messageBytes int
	}{
		{name: "short", sessions: 32, messages: 20, messageBytes: 1024},
		{name: "long", sessions: 32, messages: 100, messageBytes: 4096},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			replacements := benchmarkSessionReplacements(tc.sessions, tc.messages, tc.messageBytes)
			databaseDir := b.TempDir()
			var totals writeBenchmarkTotals
			var databaseBytes int64
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				databasePath := filepath.Join(databaseDir, fmt.Sprintf("write-%d.db", i))
				idx, err := OpenSQLite(databasePath)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()

				profile, err := idx.ReplaceSessionsWithProfile(context.Background(), replacements)
				if err != nil {
					b.Fatal(err)
				}
				totals.add(profile)

				b.StopTimer()
				var pageCount, pageSize int64
				if err := idx.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
					b.Fatal(err)
				}
				if err := idx.db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
					b.Fatal(err)
				}
				databaseBytes = pageCount * pageSize
				if err := idx.Close(); err != nil {
					b.Fatal(err)
				}
				_ = os.Remove(databasePath)
				b.StartTimer()
			}

			b.StopTimer()
			b.ReportMetric(float64(tc.sessions), "sessions/op")
			b.ReportMetric(float64(tc.sessions*tc.messages), "messages/op")
			b.ReportMetric(float64(tc.sessions*tc.messages*tc.messageBytes), "message-bytes/op")
			b.ReportMetric(float64(databaseBytes), "db-bytes")
			totals.report(b)
		})
	}
}

func BenchmarkSQLiteExistingSessionReplacement(b *testing.B) {
	for _, messageCount := range []int{20, 100, 500, 2000} {
		b.Run(fmt.Sprintf("messages-%d", messageCount), func(b *testing.B) {
			replacements := benchmarkSessionReplacements(1, messageCount, 1024)
			idx, err := OpenSQLite(filepath.Join(b.TempDir(), "replacement.db"))
			if err != nil {
				b.Fatal(err)
			}
			defer idx.Close()
			if _, err := idx.ReplaceSessionsWithProfile(context.Background(), replacements); err != nil {
				b.Fatal(err)
			}

			var totals writeBenchmarkTotals
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				profile, err := idx.ReplaceSessionsWithProfile(context.Background(), replacements)
				if err != nil {
					b.Fatal(err)
				}
				totals.add(profile)
			}
			b.StopTimer()

			b.ReportMetric(float64(messageCount), "messages/op")
			b.ReportMetric(float64(messageCount*1024), "message-bytes/op")
			totals.report(b)
		})
	}
}

type coldBuildBenchmarkProfile struct {
	relationalWrite time.Duration
	ftsBuild        time.Duration
	commit          time.Duration
}

func BenchmarkSQLiteColdBuildFTSStrategies(b *testing.B) {
	cases := []struct {
		name         string
		sessions     int
		messages     int
		messageBytes int
	}{
		{name: "short", sessions: 32, messages: 20, messageBytes: 1024},
		{name: "long", sessions: 32, messages: 100, messageBytes: 4096},
		{name: "history-shape", sessions: 128, messages: 35, messageBytes: 4096},
	}

	for _, tc := range cases {
		replacements := benchmarkSessionReplacements(tc.sessions, tc.messages, tc.messageBytes)

		b.Run(tc.name+"/interleaved", func(b *testing.B) {
			databaseDir := b.TempDir()
			var relationalWrite time.Duration
			var ftsBuild time.Duration
			var commit time.Duration
			var databaseBytes int64
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				databasePath := filepath.Join(databaseDir, fmt.Sprintf("interleaved-%d.db", i))
				idx, err := OpenSQLite(databasePath)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()

				profile, err := idx.ReplaceSessionsWithProfile(context.Background(), replacements)
				if err != nil {
					b.Fatal(err)
				}
				relationalWrite += profile.SessionUpsert + profile.MessageInsert
				ftsBuild += profile.FTSInsert
				commit += profile.Commit

				b.StopTimer()
				databaseBytes = benchmarkDatabaseBytes(b, idx)
				if err := idx.Close(); err != nil {
					b.Fatal(err)
				}
				_ = os.Remove(databasePath)
				b.StartTimer()
			}
			b.StopTimer()

			b.ReportMetric(float64(tc.sessions), "sessions/op")
			b.ReportMetric(float64(tc.sessions*tc.messages), "messages/op")
			b.ReportMetric(float64(tc.sessions*tc.messages*tc.messageBytes), "message-bytes/op")
			b.ReportMetric(float64(databaseBytes), "db-bytes")
			b.ReportMetric(float64(relationalWrite.Nanoseconds())/float64(b.N), "relational-write-ns/op")
			b.ReportMetric(float64(ftsBuild.Nanoseconds())/float64(b.N), "fts-build-ns/op")
			b.ReportMetric(float64(commit.Nanoseconds())/float64(b.N), "commit-ns/op")
		})

		b.Run(tc.name+"/bulk", func(b *testing.B) {
			databaseDir := b.TempDir()
			var totals coldBuildBenchmarkProfile
			var databaseBytes int64
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				databasePath := filepath.Join(databaseDir, fmt.Sprintf("bulk-%d.db", i))
				idx, err := OpenSQLite(databasePath)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()

				profile, err := benchmarkBulkColdBuild(context.Background(), idx, replacements)
				if err != nil {
					b.Fatal(err)
				}
				totals.relationalWrite += profile.relationalWrite
				totals.ftsBuild += profile.ftsBuild
				totals.commit += profile.commit

				b.StopTimer()
				databaseBytes = benchmarkDatabaseBytes(b, idx)
				if err := idx.Close(); err != nil {
					b.Fatal(err)
				}
				_ = os.Remove(databasePath)
				b.StartTimer()
			}
			b.StopTimer()

			b.ReportMetric(float64(tc.sessions), "sessions/op")
			b.ReportMetric(float64(tc.sessions*tc.messages), "messages/op")
			b.ReportMetric(float64(tc.sessions*tc.messages*tc.messageBytes), "message-bytes/op")
			b.ReportMetric(float64(databaseBytes), "db-bytes")
			b.ReportMetric(float64(totals.relationalWrite.Nanoseconds())/float64(b.N), "relational-write-ns/op")
			b.ReportMetric(float64(totals.ftsBuild.Nanoseconds())/float64(b.N), "fts-build-ns/op")
			b.ReportMetric(float64(totals.commit.Nanoseconds())/float64(b.N), "commit-ns/op")
		})
	}
}

func benchmarkBulkColdBuild(ctx context.Context, idx *SQLiteIndex, replacements []SessionReplacement) (coldBuildBenchmarkProfile, error) {
	var profile coldBuildBenchmarkProfile

	tx, err := idx.db.BeginTx(ctx, nil)
	if err != nil {
		return profile, err
	}
	defer func() { _ = tx.Rollback() }()

	upsertStmt, err := tx.PrepareContext(ctx, upsertSessionSQL)
	if err != nil {
		return profile, err
	}
	defer upsertStmt.Close()

	messageStmt, err := tx.PrepareContext(ctx, `
INSERT INTO messages (session_id, ordinal, role, text, timestamp)
VALUES (?, ?, ?, ?, ?)
`)
	if err != nil {
		return profile, err
	}
	defer messageStmt.Close()

	relationalStart := time.Now()
	for _, replacement := range replacements {
		if err := execPreparedUpsertSession(ctx, upsertStmt, replacement.Session); err != nil {
			return profile, err
		}
		for _, message := range replacement.Messages {
			var timestamp any
			if message.Timestamp != nil {
				timestamp = formatTime(*message.Timestamp)
			}
			if _, err := messageStmt.ExecContext(
				ctx,
				replacement.Session.ID,
				message.Ordinal,
				message.Role,
				message.Text,
				timestamp,
			); err != nil {
				return profile, err
			}
		}
	}
	profile.relationalWrite = time.Since(relationalStart)

	ftsStart := time.Now()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages_fts (rowid, session_id, ordinal, role, text)
SELECT message_id, session_id, ordinal, role, text
FROM messages
`); err != nil {
		return profile, err
	}
	profile.ftsBuild = time.Since(ftsStart)

	commitStart := time.Now()
	if err := tx.Commit(); err != nil {
		return profile, err
	}
	profile.commit = time.Since(commitStart)
	return profile, nil
}

func benchmarkDatabaseBytes(b *testing.B, idx *SQLiteIndex) int64 {
	b.Helper()
	var pageCount, pageSize int64
	if err := idx.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		b.Fatal(err)
	}
	if err := idx.db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		b.Fatal(err)
	}
	return pageCount * pageSize
}

func benchmarkSessionReplacements(sessionCount, messageCount, messageBytes int) []SessionReplacement {
	baseTime := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	baseText := "codex recall sqlite trigram indexing benchmark conversation with source maps agent runtime errors commands paths and identifiers "
	replacements := make([]SessionReplacement, 0, sessionCount)
	for i := 0; i < sessionCount; i++ {
		sessionID := fmt.Sprintf("write-bench-%06d", i)
		messages := make([]Message, 0, messageCount)
		for ordinal := 0; ordinal < messageCount; ordinal++ {
			prefix := fmt.Sprintf("session %d message %d ", i, ordinal)
			text := prefix + strings.Repeat(baseText, messageBytes/len(baseText)+2)
			if len(text) > messageBytes {
				text = text[:messageBytes]
			}
			role := "assistant"
			if ordinal%2 == 0 {
				role = "user"
			}
			messages = append(messages, Message{Ordinal: ordinal, Role: role, Text: text})
		}
		replacements = append(replacements, SessionReplacement{
			Session: Session{
				ID:          sessionID,
				Timestamp:   baseTime.Add(time.Duration(i) * time.Second),
				CWD:         "/work/write-benchmark",
				Project:     "write-benchmark",
				Source:      "vscode",
				RolloutPath: "/codex/" + sessionID + ".jsonl",
				ContentHash: "v1:sha256:" + sessionID,
			},
			Messages: messages,
		})
	}
	return replacements
}
