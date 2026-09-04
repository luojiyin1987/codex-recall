package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRunnerPackHumanAndJSON(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ab"
	path := filepath.Join(home, "sessions", "2026", "09", "04", "rollout-2026-09-04T09-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"timestamp":"2026-09-04T09:00:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-09-04T09:00:00Z","cwd":"/work/codex-recall","source":"vscode"}}`,
		`{"timestamp":"2026-09-04T09:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"deterministic context pack foundation"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
	if err := runner.run([]string{"pack", "--home", home, "--project", "codex-recall", "context pack"}); err != nil {
		t.Fatal(err)
	}
	human := stdout.String()
	for _, want := range []string{
		"CONTEXT_PACK",
		"QUERY",
		"context pack",
		"EVIDENCE",
		sessionID,
		"lexical:fts5",
		"cxq resume " + sessionID,
	} {
		if !strings.Contains(human, want) {
			t.Fatalf("human pack missing %q: %q", want, human)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("human stderr = %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"pack", "--json", "--home", home, "--project", "codex-recall", "context pack"}); err != nil {
		t.Fatal(err)
	}
	var got packJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	if got.SchemaVersion != 1 || got.Query != "context pack" || got.Project != "codex-recall" {
		t.Fatalf("pack JSON header = %#v", got)
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("evidence = %#v", got.Evidence)
	}
	evidence := got.Evidence[0]
	if evidence.SessionID != sessionID || evidence.Timestamp == nil || *evidence.Timestamp != "2026-09-04T09:00:00Z" {
		t.Fatalf("evidence provenance = %#v", evidence)
	}
	if evidence.Why != "lexical:fts5" || evidence.ResumeCommand != "cxq resume "+sessionID {
		t.Fatalf("evidence = %#v", evidence)
	}
	if stderr.Len() != 0 {
		t.Fatalf("json stderr = %q", stderr.String())
	}
}
