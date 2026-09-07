package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRunnerSearchIndexExplainShowsRetrievalMetadata(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ab"
	path := filepath.Join(home, "sessions", "2026", "09", "07", "rollout-2026-09-07T00-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := strings.Join([]string{
		`{"timestamp":"2026-09-07T00:00:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-09-07T00:00:00Z","cwd":"/tmp/explain-project","source":"vscode"}}`,
		`{"timestamp":"2026-09-07T00:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"indexed explain WebRTC transport"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"index", "--home", home}); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--index", "--explain", "--home", home, "WebRTC transport"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{
		"ORDINAL",
		"SCORE",
		"WHY",
		"lexical:fts5",
		sessionID,
		"indexed explain WebRTC transport",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("explain output missing %q: %q", want, output)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("explain stderr = %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--index", "--home", home, "WebRTC transport"}); err != nil {
		t.Fatal(err)
	}
	output = stdout.String()
	if strings.Contains(output, "ORDINAL") || strings.Contains(output, "lexical:fts5") {
		t.Fatalf("default indexed output exposed explain columns: %q", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("default indexed search stderr = %q", stderr.String())
	}
}

func TestCLIRunnerSearchExplainRequiresIndex(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)

	err := runner.run([]string{"search", "--explain", "needle"})
	if err == nil || !strings.Contains(err.Error(), "--explain requires --index") {
		t.Fatalf("search error = %v", err)
	}
}
