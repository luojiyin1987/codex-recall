package index

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkFTSSearch(b *testing.B) {
	for _, sessions := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			idx := openBenchmarkIndex(b, sessions)
			defer idx.Close()

			reportBenchmarkDBSize(b, idx)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				matches, err := idx.Search(context.Background(), SearchOptions{
					Query: "benchmark needle",
					Limit: 20,
				})
				if err != nil {
					b.Fatal(err)
				}
				if len(matches) == 0 {
					b.Fatal("FTS benchmark returned no matches")
				}
			}
		})
	}
}

func BenchmarkFTSSearchWithProjectFilter(b *testing.B) {
	for _, sessions := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			idx := openBenchmarkIndex(b, sessions)
			defer idx.Close()

			reportBenchmarkDBSize(b, idx)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				matches, err := idx.Search(context.Background(), SearchOptions{
					Query:   "benchmark needle",
					Limit:   20,
					Project: "project-a",
				})
				if err != nil {
					b.Fatal(err)
				}
				if len(matches) == 0 {
					b.Fatal("filtered FTS benchmark returned no matches")
				}
			}
		})
	}
}

func openBenchmarkIndex(b *testing.B, sessionCount int) *SQLiteIndex {
	b.Helper()

	path := filepath.Join(b.TempDir(), "index.db")
	idx, err := OpenSQLite(path)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	baseTime := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	for i := 0; i < sessionCount; i++ {
		project := "project-b"
		if i%2 == 0 {
			project = "project-a"
		}
		sessionID := fmt.Sprintf("bench-%06d", i)
		session := Session{
			ID:          sessionID,
			Timestamp:   baseTime.Add(time.Duration(i) * time.Second),
			CWD:         "/work/" + project,
			Project:     project,
			Source:      "vscode",
			RolloutPath: "/codex/" + sessionID + ".jsonl",
			ContentHash: "v1:sha256:" + sessionID,
		}
		text := "ordinary benchmark conversation text"
		if i%10 == 0 {
			text = "benchmark needle appears in this conversation"
		}
		if err := idx.ReplaceSession(ctx, session, []Message{
			{Ordinal: 0, Role: "user", Text: text},
			{Ordinal: 1, Role: "assistant", Text: "assistant benchmark response"},
		}); err != nil {
			_ = idx.Close()
			b.Fatal(err)
		}
	}
	return idx
}

func reportBenchmarkDBSize(b *testing.B, idx *SQLiteIndex) {
	b.Helper()

	var pageCount, pageSize int64
	if err := idx.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		b.Fatal(err)
	}
	if err := idx.db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(pageCount*pageSize), "db-bytes")
}
