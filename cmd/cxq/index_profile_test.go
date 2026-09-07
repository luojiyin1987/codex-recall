package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRunnerIndexProfileWritesRefreshEvidenceToStderr(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "2026", "09", "07", "rollout-2026-09-07T02-00-00-profile-index.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := strings.Join([]string{
		`{"timestamp":"2026-09-07T02:00:00Z","type":"session_meta","payload":{"id":"profile-index","timestamp":"2026-09-07T02:00:00Z","cwd":"/tmp/profile-project","source":"cli"}}`,
		`{"timestamp":"2026-09-07T02:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"refresh profile"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"index", "--profile", "--home", home}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "INDEX_PROFILE") {
		t.Fatalf("profile polluted index stdout: %q", stdout.String())
	}
	for _, want := range []string{
		"INDEX_PROFILE",
		"PREPARE",
		"DATABASE_OPEN",
		"DISCOVERY",
		"METADATA_PARSE",
		"CATALOG_FINALIZE",
		"INDEX_STATE_READ",
		"HASH",
		"CONVERSATION_DECODE",
		"DATABASE_WRITE",
		"STALE_CLEANUP",
		"BUILD",
		"DATABASE_CLOSE",
		"TOTAL",
		"ROLLOUT_FILES",
		"METADATA_UNREADABLE_FILES",
		"FILES_HASHED",
		"HASH_BYTES",
		"FILES_DECODED",
		"CONVERSATION_BYTES",
		"MESSAGES_DECODED",
		"BATCHES_WRITTEN",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("profile stderr missing %q: %q", want, stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), "INDEXED") || !strings.Contains(stdout.String(), "1") {
		t.Fatalf("index summary = %q", stdout.String())
	}
}
