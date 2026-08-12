package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSessionExactAndUniquePrefix(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "sessions", "a.jsonl", "019abc111", "2026-08-12T01:00:00Z", "/tmp/alpha")
	writeLookupSession(t, home, "sessions", "b.jsonl", "019def222", "2026-08-12T02:00:00Z", "/tmp/beta")

	exact, err := ResolveSession(home, "019abc111")
	if err != nil {
		t.Fatal(err)
	}
	if exact.ID != "019abc111" || exact.Project() != "alpha" {
		t.Fatalf("exact = %#v", exact)
	}

	prefix, err := ResolveSession(home, "019def")
	if err != nil {
		t.Fatal(err)
	}
	if prefix.ID != "019def222" || prefix.Project() != "beta" {
		t.Fatalf("prefix = %#v", prefix)
	}
}

func TestResolveSessionRejectsAmbiguousPrefix(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "sessions", "a.jsonl", "019abc111", "2026-08-12T01:00:00Z", "/tmp/alpha")
	writeLookupSession(t, home, "sessions", "b.jsonl", "019abc222", "2026-08-12T02:00:00Z", "/tmp/beta")

	_, err := ResolveSession(home, "019abc")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ResolveSession() error = %v", err)
	}
}

func TestResolveSessionNotFound(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "sessions", "a.jsonl", "019abc111", "2026-08-12T01:00:00Z", "/tmp/alpha")

	_, err := ResolveSession(home, "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ResolveSession() error = %v", err)
	}
}

func TestResolveSessionDeduplicatesSameID(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "archived_sessions", "old.jsonl", "019same", "2026-08-11T01:00:00Z", "/tmp/old")
	writeLookupSession(t, home, "sessions", "new.jsonl", "019same", "2026-08-12T01:00:00Z", "/tmp/new")

	session, err := ResolveSession(home, "019same")
	if err != nil {
		t.Fatal(err)
	}
	if session.Project() != "new" {
		t.Fatalf("Project() = %q, want new", session.Project())
	}
}

func writeLookupSession(t *testing.T, home, root, name, id, timestamp, cwd string) {
	t.Helper()
	dir := filepath.Join(home, root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"timestamp":"` + timestamp + `","type":"session_meta","payload":{"id":"` + id + `","timestamp":"` + timestamp + `","cwd":"` + cwd + `","source":"vscode"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
