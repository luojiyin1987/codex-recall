package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRunnerSearchIndexProfileWritesTimingToStderr(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ac"
	path := filepath.Join(home, "sessions", "2026", "09", "07", "rollout-2026-09-07T01-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := strings.Join([]string{
		`{"timestamp":"2026-09-07T01:00:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-09-07T01:00:00Z","cwd":"/tmp/profile-project","source":"vscode"}}`,
		`{"timestamp":"2026-09-07T01:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"profile WebRTC transport"}}`,
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
	if err := runner.run([]string{"search", "--index", "--profile", "--home", home, "WebRTC transport"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), sessionID) || strings.Contains(stdout.String(), "SEARCH_PROFILE") {
		t.Fatalf("profile polluted search stdout: %q", stdout.String())
	}
	for _, want := range []string{
		"SEARCH_PROFILE",
		"BACKEND",
		"fts5",
		"DATABASE_OPEN",
		"QUERY_SETUP",
		"RESULT_SCAN",
		"SNIPPET",
		"INDEX_SEARCH",
		"TOTAL",
		"ROWS_SCANNED",
		"SESSIONS_RETURNED",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("profile stderr missing %q: %q", want, stderr.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--index", "--json", "--profile", "--home", home, "WebRTC transport"}); err != nil {
		t.Fatal(err)
	}
	var got searchJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("profile corrupted JSON stdout %q: %v", stdout.String(), err)
	}
	if len(got.Results) != 1 || got.Results[0].SessionID != sessionID {
		t.Fatalf("JSON results = %#v", got.Results)
	}
	if !strings.Contains(stderr.String(), "SEARCH_PROFILE") {
		t.Fatalf("JSON profile stderr = %q", stderr.String())
	}
}

func TestCLIRunnerSearchIndexProfileReportsSubstringBackend(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ad"
	path := filepath.Join(home, "sessions", "2026", "09", "07", "rollout-2026-09-07T01-05-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := strings.Join([]string{
		`{"timestamp":"2026-09-07T01:05:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-09-07T01:05:00Z","cwd":"/tmp/profile-project","source":"cli"}}`,
		`{"timestamp":"2026-09-07T01:05:01Z","type":"event_msg","payload":{"type":"user_message","message":"Go runtime profile"}}`,
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
	if err := runner.run([]string{"search", "--index", "--profile", "--home", home, "go"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "substring") {
		t.Fatalf("substring profile stderr = %q", stderr.String())
	}
}

func TestCLIRunnerSearchProfileRequiresIndex(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)

	err := runner.run([]string{"search", "--profile", "needle"})
	if err == nil || !strings.Contains(err.Error(), "--profile requires --index") {
		t.Fatalf("search error = %v", err)
	}
}
