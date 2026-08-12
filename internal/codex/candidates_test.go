package codex

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	for _, want := range []string{"-l", "-0", "--fixed-strings", "--ignore-case", "--no-ignore", "*.jsonl", `annotated \"tag\"`, root} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
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
