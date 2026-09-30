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
