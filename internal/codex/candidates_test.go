package codex

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestSearchCandidateFilesUsesRipgrepResults(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	matching := filepath.Join(root, "rollout-match.jsonl")
	other := filepath.Join(root, "rollout-other.jsonl")
	for _, path := range []string{matching, other} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var gotArgs []string
	got, err := searchCandidateFiles(home, `annotated "tag"`,
		func(name string) (string, error) { return "/fake/rg", nil },
		func(name string, args ...string) ([]byte, error) {
			if name != "/fake/rg" {
				t.Fatalf("name = %q", name)
			}
			gotArgs = append([]string(nil), args...)
			return []byte(matching + "\x00"), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{matching}) {
		t.Fatalf("candidates = %#v", got)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"-l", "-0", "--ignore-case", "--no-ignore", "*.jsonl", "response_item", "event_msg", "user_message", `annotated \\"tag\\"`, root} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "--fixed-strings") {
		t.Fatalf("args %q use fixed-string mode", joined)
	}
}

func TestSearchCandidateFilesFallsBackWhenRipgrepMissing(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := searchCandidateFiles(home, "needle",
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { t.Fatal("runner should not be called"); return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{path}) {
		t.Fatalf("candidates = %#v", got)
	}
}

func TestSearchCandidateFilesFallsBackWhenRipgrepFails(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := searchCandidateFiles(home, "needle",
		func(string) (string, error) { return "/fake/rg", nil },
		func(string, ...string) ([]byte, error) { return nil, errors.New("boom") },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{path}) {
		t.Fatalf("candidates = %#v", got)
	}
}

func TestParseRipgrepPathsSortsNullDelimitedOutput(t *testing.T) {
	got := parseRipgrepPaths([]byte("b.jsonl\x00a.jsonl\x00"))
	want := []string{"a.jsonl", "b.jsonl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestConversationSearchPatternMatchesOnlyConversationRecords(t *testing.T) {
	pattern := regexp.MustCompile("(?i)" + conversationSearchPattern(`annotated "tag"`))

	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "response user message",
			line: `{"type":"response_item","payload":{"type":"message","id":"message-1","role":"user","content":[{"type":"input_text","text":"Annotated \"tag\" release"}]}}`,
			want: true,
		},
		{
			name: "legacy assistant message",
			line: `{"type": "event_msg", "payload": {"type": "agent_message", "client_id": "client-1", "message": "Annotated \"tag\" release"}}`,
			want: true,
		},
		{
			name: "tool output",
			line: `{"type":"response_item","payload":{"type":"function_call_output","output":"Annotated \"tag\" release"}}`,
			want: false,
		},
		{
			name: "different message",
			line: `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"lightweight release"}]}}`,
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pattern.MatchString(test.line); got != test.want {
				t.Fatalf("pattern.MatchString() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRankCandidateFilesUsesMetadata(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	older := filepath.Join(root, "older.jsonl")
	newer := filepath.Join(root, "newer.jsonl")
	other := filepath.Join(root, "other.jsonl")
	broken := filepath.Join(root, "broken.jsonl")
	writeCandidateSession(t, older, "older", "2026-08-12T00:00:00Z", "/tmp/lint-md", "vscode")
	writeCandidateSession(t, newer, "newer", "2026-08-13T00:00:00Z", "/tmp/lint-md", "vscode")
	writeCandidateSession(t, other, "other", "2026-08-14T00:00:00Z", "/tmp/other", "vscode")
	if err := os.WriteFile(broken, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := RankCandidateFiles([]string{older, other, broken, newer}, func(session Session) bool {
		return session.Project() == "lint-md"
	})
	want := []string{newer, older, broken}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RankCandidateFiles() = %#v, want %#v", got, want)
	}
}

func writeCandidateSession(t *testing.T, path, id, timestamp, cwd, source string) {
	t.Helper()
	content := `{"timestamp":"` + timestamp + `","type":"session_meta","payload":{"id":"` + id + `","timestamp":"` + timestamp + `","cwd":"` + cwd + `","source":"` + source + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
