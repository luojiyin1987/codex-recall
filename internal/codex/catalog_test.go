package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogSessionsDeduplicatesAndOrdersSessions(t *testing.T) {
	home := t.TempDir()
	name := rolloutLookupName("2026-08-12T01-00-00", "019same")
	writeLookupSession(t, home, "archived_sessions", name, "019same", "2026-08-11T01:00:00Z", "/tmp/old")
	writeLookupSession(t, home, "sessions", name, "019same", "2026-08-12T01:00:00Z", "/tmp/new")
	writeLookupSession(t, home, "sessions", rolloutLookupName("2026-08-13T01-00-00", "019latest"), "019latest", "2026-08-13T01:00:00Z", "/tmp/latest")
	broken := filepath.Join(home, "sessions", "broken.jsonl")
	if err := os.WriteFile(broken, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, warnings, err := NewCatalog(home).Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].ID != "019latest" || sessions[1].Project() != "new" {
		t.Fatalf("sessions = %#v", sessions)
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1", len(warnings))
	}
}
