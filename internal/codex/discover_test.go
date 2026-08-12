package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestResolveHomeUsesCodexHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "custom-codex")
	t.Setenv(codexHomeEnv, home)

	got, err := ResolveHome()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(home) {
		t.Fatalf("ResolveHome() = %q, want %q", got, filepath.Clean(home))
	}
}

func TestDiscoverFilesFindsJSONLRecursively(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, "sessions", "2026", "08", "12")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}

	first := filepath.Join(sessions, "rollout-b.jsonl")
	second := filepath.Join(home, "sessions", "rollout-a.jsonl")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sessions, "ignore.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{second, first}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverFiles() = %#v, want %#v", got, want)
	}
}

func TestDiscoverFilesIncludesArchivedSessions(t *testing.T) {
	home := t.TempDir()
	archived := filepath.Join(home, "archived_sessions", "rollout-old.jsonl")
	if err := os.MkdirAll(filepath.Dir(archived), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archived, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{archived}) {
		t.Fatalf("DiscoverFiles() = %#v, want archived rollout", got)
	}
}

func TestDiscoverFilesMissingSessionsDirectory(t *testing.T) {
	got, err := DiscoverFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("DiscoverFiles() returned %d files, want 0", len(got))
	}
}
