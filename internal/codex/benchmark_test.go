package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkLiveSearch(b *testing.B) {
	for _, sessions := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("sessions=%d", sessions), func(b *testing.B) {
			home := writeLiveSearchBenchmarkRollouts(b, sessions)

			// Force the deterministic built-in scanner. Otherwise benchmark
			// results depend on whether the host machine happens to have rg.
			b.Setenv("PATH", "")

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err := Search(home, SearchOptions{
					Query: "benchmark needle",
					Limit: 20,
				})
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Matches) == 0 {
					b.Fatal("live search benchmark returned no matches")
				}
			}
		})
	}
}

func writeLiveSearchBenchmarkRollouts(b *testing.B, sessionCount int) string {
	b.Helper()

	home := b.TempDir()
	root := filepath.Join(home, "sessions", "2026", "09", "04")
	if err := os.MkdirAll(root, 0o755); err != nil {
		b.Fatal(err)
	}

	for i := 0; i < sessionCount; i++ {
		sessionID := fmt.Sprintf("bench-%06d", i)
		project := "project-b"
		if i%2 == 0 {
			project = "project-a"
		}
		userText := "ordinary benchmark conversation text"
		if i%10 == 0 {
			userText = "benchmark needle appears in this conversation"
		}
		path := filepath.Join(root, fmt.Sprintf("rollout-2026-09-04T08-%02d-%02d-%s.jsonl", (i/60)%60, i%60, sessionID))
		content := strings.Join([]string{
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:00Z","type":"session_meta","payload":{"id":"%s","timestamp":"2026-09-04T08:00:00Z","cwd":"/work/%s","source":"vscode"}}`, sessionID, project),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"%s"}}`, userText),
			`{"timestamp":"2026-09-04T08:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"assistant benchmark response"}}`,
		}, "\n") + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return home
}
