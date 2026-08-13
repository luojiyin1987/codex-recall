package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSessionExactAndUniquePrefix(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-12T01-00-00", "019abc111"), "019abc111", "2026-08-12T01:00:00Z", "/tmp/alpha")
	writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-12T02-00-00", "019def222"), "019def222", "2026-08-12T02:00:00Z", "/tmp/beta")

	exact, err := NewCatalog(home).Resolve("019abc111")
	if err != nil {
		t.Fatal(err)
	}
	if exact.ID != "019abc111" || exact.Project() != "alpha" {
		t.Fatalf("exact = %#v", exact)
	}

	prefix, err := NewCatalog(home).Resolve("019def")
	if err != nil {
		t.Fatal(err)
	}
	if prefix.ID != "019def222" || prefix.Project() != "beta" {
		t.Fatalf("prefix = %#v", prefix)
	}
}

func TestResolveSessionRejectsAmbiguousPrefix(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-12T01-00-00", "019abc111"), "019abc111", "2026-08-12T01:00:00Z", "/tmp/alpha")
	writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-12T02-00-00", "019abc222"), "019abc222", "2026-08-12T02:00:00Z", "/tmp/beta")

	_, err := NewCatalog(home).Resolve("019abc")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ResolveSession() error = %v", err)
	}
}

func TestResolveSessionNotFound(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-12T01-00-00", "019abc111"), "019abc111", "2026-08-12T01:00:00Z", "/tmp/alpha")

	_, err := NewCatalog(home).Resolve("deadbeef")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ResolveSession() error = %v", err)
	}
}

func TestResolveSessionDeduplicatesSameID(t *testing.T) {
	home := t.TempDir()
	name := rolloutLookupName("2026-08-12T01-00-00", "019same")
	writeLookupSession(t, home, "archived_sessions", name, "019same", "2026-08-11T01:00:00Z", "/tmp/old")
	writeLookupSession(t, home, "sessions", name, "019same", "2026-08-12T01:00:00Z", "/tmp/new")

	session, err := NewCatalog(home).Resolve("019same")
	if err != nil {
		t.Fatal(err)
	}
	if session.Project() != "new" {
		t.Fatalf("Project() = %q, want new", session.Project())
	}
}

func TestSessionCandidateFilesFiltersStandardRollouts(t *testing.T) {
	home := t.TempDir()
	wanted := writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-12T01-00-00", "019target111"), "019target111", "2026-08-12T01:00:00Z", "/tmp/target")
	writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-12T02-00-00", "019other222"), "019other222", "2026-08-12T02:00:00Z", "/tmp/other")
	legacyName := writeLookupSession(t, home, "sessions", "custom.jsonl", "019custom333", "2026-08-12T03:00:00Z", "/tmp/custom")

	files, err := sessionCandidateFiles(home, "019target")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2: %v", len(files), files)
	}
	if files[0] != legacyName || files[1] != wanted {
		t.Fatalf("files = %v, want [%s %s]", files, legacyName, wanted)
	}
}

func TestResolveSessionKeepsNonstandardFilenameCompatibility(t *testing.T) {
	home := t.TempDir()
	writeLookupSession(t, home, "sessions", "imported-history.jsonl", "019legacy444", "2026-08-12T01:00:00Z", "/tmp/legacy")

	session, err := NewCatalog(home).Resolve("019legacy")
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "019legacy444" || session.Project() != "legacy" {
		t.Fatalf("session = %#v", session)
	}
}

func rolloutLookupName(timestamp, id string) string {
	return "rollout-" + timestamp + "-" + id + ".jsonl"
}

func writeLookupSession(t *testing.T, home, root, name, id, timestamp, cwd string) string {
	t.Helper()
	dir := filepath.Join(home, root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	content := `{"timestamp":"` + timestamp + `","type":"session_meta","payload":{"id":"` + id + `","timestamp":"` + timestamp + `","cwd":"` + cwd + `","source":"vscode"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
