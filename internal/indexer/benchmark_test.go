package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkIndexInitialBuild(b *testing.B) {
	for _, sessions := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			home, _ := writeBenchmarkRollouts(b, sessions)
			databasePath := filepath.Join(b.TempDir(), "index.db")

			b.ReportAllocs()
			b.ResetTimer()
			var databaseBytes int64
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

				b.StopTimer()
				info, err := os.Stat(databasePath)
				if err != nil {
					b.Fatal(err)
				}
				databaseBytes = info.Size()
			}
			b.ReportMetric(float64(databaseBytes), "db-bytes")
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
			for i := 0; i < b.N; i++ {
				result, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: databasePath})
				if err != nil {
					b.Fatal(err)
				}
				if result.Indexed != 0 || result.Skipped != sessions {
					b.Fatalf("unchanged refresh = %#v", result)
				}
			}
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
			}
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
