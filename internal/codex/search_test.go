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

	matches, errs := searchFiles(searchCandidates(first, second, missing), "needle", 2)
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

	matches, errs := searchFiles(searchCandidates(path), "needle", 1024)
	if len(errs) != 0 {
		t.Fatalf("SearchFiles() errors = %v", errs)
	}
	if cap(matches) > 1 {
		t.Fatalf("cap(matches) = %d, want at most 1", cap(matches))
	}
}

func TestSearchFilesReusesCandidateMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message-only.jsonl")
	input := `{"type":"event_msg","payload":{"type":"user_message","message":"needle"}}` + "\n"
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	session := Session{ID: "cached", Path: path}

	matches, errs := searchFiles([]SearchCandidate{{Path: path, session: session, hasSession: true}}, "needle", 1)
	if len(errs) != 0 {
		t.Fatalf("SearchFiles() errors = %v", errs)
	}
	if len(matches) != 1 || matches[0].Session.ID != "cached" {
		t.Fatalf("SearchFiles() matches = %#v", matches)
	}
}

func TestCompileSearchMatcherUsesLiteralCaseInsensitiveMatching(t *testing.T) {
	matcher, err := compileSearchMatcher("Tag(v2)")
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.MatchString("release tag(v2)") {
		t.Fatal("matcher did not match the literal query")
	}
	if matcher.MatchString("release tagv2") {
		t.Fatal("matcher treated query characters as regular expression syntax")
	}
}

func searchCandidates(paths ...string) []SearchCandidate {
	candidates := make([]SearchCandidate, 0, len(paths))
	for _, path := range paths {
		candidates = append(candidates, SearchCandidate{Path: path})
	}
	return candidates
}

func TestMatchesSessionFilters(t *testing.T) {
	session := Session{CWD: "/tmp/Lint-MD", Source: "VSCode"}
	tests := []struct {
		name    string
		project string
		source  string
		want    bool
	}{
		{name: "no filters", want: true},
		{name: "project", project: "lint-md", want: true},
		{name: "project mismatch", project: "other", want: false},
		{name: "source", source: "vscode", want: true},
		{name: "source mismatch", source: "cli", want: false},
		{name: "both", project: " LINT-MD ", source: " VSCODE ", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchesSessionFilters(session, test.project, test.source); got != test.want {
				t.Fatalf("matchesSessionFilters() = %v, want %v", got, test.want)
			}
		})
	}
}
