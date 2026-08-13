package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFileMatchesResponseItemCaseInsensitive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-current.jsonl")
	input := strings.Join([]string{
		`{"timestamp":"2026-08-12T02:59:15Z","type":"session_meta","payload":{"id":"019abc","cwd":"/tmp/lint-md","source":"vscode"}}`,
		`{"timestamp":"2026-08-12T03:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Why does Promise need a controlled pause?"}]}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok, err := SearchFile(path, "promise")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("SearchFile() did not match")
	}
	if got.Session.ID != "019abc" || got.Session.Project() != "lint-md" {
		t.Fatalf("session = %#v", got.Session)
	}
	if got.Role != "user" {
		t.Fatalf("Role = %q", got.Role)
	}
	if !strings.Contains(got.Snippet, "Promise") {
		t.Fatalf("Snippet = %q", got.Snippet)
	}
}

func TestSearchFileMatchesLegacyEventMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-02-22T12-28-43-019c839b-5ed3-7603-bb29-446c47c1e18d.jsonl")
	input := strings.Join([]string{
		`{"type":"turn_context","payload":{"cwd":"/tmp/legacy-project"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"Annotated tag objects point at another object."}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok, err := SearchFile(path, "annotated tag")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("SearchFile() did not match legacy event")
	}
	if got.Session.ID != "019c839b-5ed3-7603-bb29-446c47c1e18d" {
		t.Fatalf("ID = %q", got.Session.ID)
	}
	if got.Session.Project() != "legacy-project" {
		t.Fatalf("Project = %q", got.Session.Project())
	}
	if got.Role != "assistant" {
		t.Fatalf("Role = %q", got.Role)
	}
}

func TestSearchFileIgnoresToolOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	input := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"019abc","cwd":"/tmp/project"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"x","output":"secret needle"}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok, err := SearchFile(path, "needle")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("SearchFile() matched tool output")
	}
}

func TestSearchFileRejectsEmptyQuery(t *testing.T) {
	_, _, err := SearchFile("unused", "   ")
	if err == nil {
		t.Fatal("SearchFile() accepted empty query")
	}
}

func TestSearchSnippetNormalizesWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	input := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"019abc","cwd":"/tmp/project"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"alpha\n\nPromise\tcontrol flow omega"}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok, err := SearchFile(path, "Promise")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("SearchFile() did not match")
	}
	if strings.ContainsAny(got.Snippet, "\n\t") {
		t.Fatalf("Snippet not normalized: %q", got.Snippet)
	}
}

func TestSearchFilesStopsAtLimit(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.jsonl")
	second := filepath.Join(dir, "second.jsonl")
	missing := filepath.Join(dir, "missing.jsonl")
	for index, path := range []string{first, second} {
		input := strings.Join([]string{
			`{"type":"session_meta","payload":{"id":"session-` + string(rune('1'+index)) + `","cwd":"/tmp/project"}}`,
			`{"type":"event_msg","payload":{"type":"user_message","message":"needle"}}`,
		}, "\n")
		if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	matches, errs := SearchFiles([]string{first, second, missing}, "needle", 2)
	if len(errs) != 0 {
		t.Fatalf("SearchFiles() errors = %v", errs)
	}
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
}

func TestSearchFilesBoundsCapacityByCandidateCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "match.jsonl")
	input := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"session-1","cwd":"/tmp/project"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"needle"}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	matches, errs := SearchFiles([]string{path}, "needle", 1024)
	if len(errs) != 0 {
		t.Fatalf("SearchFiles() errors = %v", errs)
	}
	if cap(matches) > 1 {
		t.Fatalf("cap(matches) = %d, want at most 1", cap(matches))
	}
}
