package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type refreshBenchmarkTotals struct {
	prepare            time.Duration
	databaseOpen       time.Duration
	discovery          time.Duration
	metadataParse      time.Duration
	catalogFinalize    time.Duration
	indexStateRead     time.Duration
	hash               time.Duration
	conversationDecode time.Duration
	databaseWrite      time.Duration
	staleCleanup       time.Duration
	databaseClose      time.Duration
	filesHashed        int
	hashBytes          int64
	filesDecoded       int
	messagesDecoded    int
}

func (t *refreshBenchmarkTotals) add(profile RefreshProfile) {
	t.prepare += profile.Prepare
	t.databaseOpen += profile.DatabaseOpen
	t.discovery += profile.Build.Discovery
	t.metadataParse += profile.Build.MetadataParse
	t.catalogFinalize += profile.Build.CatalogFinalize
	t.indexStateRead += profile.Build.IndexStateRead
	t.hash += profile.Build.Hash
	t.conversationDecode += profile.Build.ConversationDecode
	t.databaseWrite += profile.Build.DatabaseWrite
	t.staleCleanup += profile.Build.StaleCleanup
	t.databaseClose += profile.DatabaseClose
	t.filesHashed += profile.Build.FilesHashed
	t.hashBytes += profile.Build.HashBytes
	t.filesDecoded += profile.Build.FilesDecoded
	t.messagesDecoded += profile.Build.MessagesDecoded
}

func (t refreshBenchmarkTotals) report(b *testing.B) {
	b.Helper()
	iterations := float64(b.N)
	b.ReportMetric(float64(t.prepare.Nanoseconds())/iterations, "prepare-ns/op")
	b.ReportMetric(float64(t.databaseOpen.Nanoseconds())/iterations, "db-open-ns/op")
	b.ReportMetric(float64(t.discovery.Nanoseconds())/iterations, "discovery-ns/op")
	b.ReportMetric(float64(t.metadataParse.Nanoseconds())/iterations, "metadata-ns/op")
	b.ReportMetric(float64(t.catalogFinalize.Nanoseconds())/iterations, "catalog-finalize-ns/op")
	b.ReportMetric(float64(t.indexStateRead.Nanoseconds())/iterations, "index-read-ns/op")
	b.ReportMetric(float64(t.hash.Nanoseconds())/iterations, "hash-ns/op")
	b.ReportMetric(float64(t.conversationDecode.Nanoseconds())/iterations, "decode-ns/op")
	b.ReportMetric(float64(t.databaseWrite.Nanoseconds())/iterations, "db-write-ns/op")
	b.ReportMetric(float64(t.staleCleanup.Nanoseconds())/iterations, "stale-cleanup-ns/op")
	b.ReportMetric(float64(t.databaseClose.Nanoseconds())/iterations, "db-close-ns/op")
	b.ReportMetric(float64(t.filesHashed)/iterations, "files-hashed/op")
	b.ReportMetric(float64(t.hashBytes)/iterations, "hash-bytes/op")
	b.ReportMetric(float64(t.filesDecoded)/iterations, "files-decoded/op")
	b.ReportMetric(float64(t.messagesDecoded)/iterations, "messages-decoded/op")
}

func BenchmarkIndexInitialBuild(b *testing.B) {
	for _, sessions := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			home, _ := writeBenchmarkRollouts(b, sessions)
			databasePath := filepath.Join(b.TempDir(), "index.db")

			b.ReportAllocs()
			b.ResetTimer()
			var databaseBytes int64
			var totals refreshBenchmarkTotals
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := os.Remove(databasePath); err != nil && !os.IsNotExist(err) {
					b.Fatal(err)
				}
				b.StartTimer()

				result, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: databasePath})
				if err != nil {
					b.Fatal(err)
				}
				if result.Indexed != sessions || result.Skipped != 0 {
					b.Fatalf("initial refresh = %#v", result)
				}
				totals.add(result.Profile)

				b.StopTimer()
				info, err := os.Stat(databasePath)
				if err != nil {
					b.Fatal(err)
				}
				databaseBytes = info.Size()
			}
			b.ReportMetric(float64(databaseBytes), "db-bytes")
			totals.report(b)
		})
	}
}

func BenchmarkIndexUnchangedRefresh(b *testing.B) {
	for _, sessions := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			home, _ := writeBenchmarkRollouts(b, sessions)
			databasePath := filepath.Join(b.TempDir(), "index.db")
			if _, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: databasePath}); err != nil {
				b.Fatal(err)
			}
			reportIndexFileSize(b, databasePath)

			b.ReportAllocs()
			b.ResetTimer()
			var totals refreshBenchmarkTotals
			for i := 0; i < b.N; i++ {
				result, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: databasePath})
				if err != nil {
					b.Fatal(err)
				}
				if result.Indexed != 0 || result.Skipped != sessions {
					b.Fatalf("unchanged refresh = %#v", result)
				}
				totals.add(result.Profile)
			}
			totals.report(b)
		})
	}
}

func BenchmarkIndexSingleSessionUpdate(b *testing.B) {
	for _, sessions := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			home, paths := writeBenchmarkRollouts(b, sessions)
			databasePath := filepath.Join(b.TempDir(), "index.db")
			if _, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: databasePath}); err != nil {
				b.Fatal(err)
			}
			reportIndexFileSize(b, databasePath)

			target := paths[sessions/2]
			b.ReportAllocs()
			b.ResetTimer()
			var totals refreshBenchmarkTotals
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				writeBenchmarkRollout(b, target, sessions/2, fmt.Sprintf("generation-%08d", i+1))
				b.StartTimer()

				result, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: databasePath})
				if err != nil {
					b.Fatal(err)
				}
				if result.Indexed != 1 || result.Skipped != sessions-1 {
					b.Fatalf("single-session refresh = %#v", result)
				}
				totals.add(result.Profile)
			}
			totals.report(b)
		})
	}
}

func writeBenchmarkRollouts(b *testing.B, sessionCount int) (string, []string) {
	b.Helper()

	home := b.TempDir()
	root := filepath.Join(home, "sessions", "2026", "09", "04")
	if err := os.MkdirAll(root, 0o755); err != nil {
		b.Fatal(err)
	}

	paths := make([]string, sessionCount)
	for i := 0; i < sessionCount; i++ {
		path := filepath.Join(root, fmt.Sprintf("rollout-2026-09-04T08-%02d-%02d-bench-%06d.jsonl", (i/60)%60, i%60, i))
		writeBenchmarkRollout(b, path, i, "generation-00000000")
		paths[i] = path
	}
	return home, paths
}

func writeBenchmarkRollout(b *testing.B, path string, sessionNumber int, generation string) {
	b.Helper()

	project := "project-b"
	if sessionNumber%2 == 0 {
		project = "project-a"
	}
	sessionID := fmt.Sprintf("bench-%06d", sessionNumber)
	userText := "ordinary benchmark conversation text"
	if sessionNumber%10 == 0 {
		userText = "benchmark needle appears in this conversation"
	}

	content := strings.Join([]string{
		fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:00Z","type":"session_meta","payload":{"id":"%s","timestamp":"2026-09-04T08:00:00Z","cwd":"/work/%s","source":"vscode"}}`, sessionID, project),
		fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"%s %s"}}`, userText, generation),
		`{"timestamp":"2026-09-04T08:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"assistant benchmark response"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
}

func reportIndexFileSize(b *testing.B, path string) {
	b.Helper()

	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(info.Size()), "db-bytes")
}
