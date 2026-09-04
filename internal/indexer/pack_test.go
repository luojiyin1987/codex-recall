package indexer

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPackBuildsIndexedEvidenceWithProvenance(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "pack-session", "sqlite index foundation", "keep provenance")

	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	result, err := Pack(context.Background(), home, PackOptions{
		Query:   "sqlite index",
		Limit:   5,
		Project: "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Query != "sqlite index" || result.Project != "demo" {
		t.Fatalf("pack header = %#v", result)
	}
	if result.DatabasePath != DefaultIndexPath(home) {
		t.Fatalf("database = %q, want %q", result.DatabasePath, DefaultIndexPath(home))
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("evidence = %#v", result.Evidence)
	}

	evidence := result.Evidence[0]
	if evidence.SessionID != "pack-session" || evidence.Project != "demo" || evidence.Source != "vscode" {
		t.Fatalf("evidence provenance = %#v", evidence)
	}
	if evidence.Role != "user" || evidence.Ordinal != 0 || !strings.Contains(evidence.Snippet, "sqlite index") {
		t.Fatalf("evidence match = %#v", evidence)
	}
	if evidence.Why != "lexical:fts5" {
		t.Fatalf("why = %q", evidence.Why)
	}
	if evidence.ResumeCommand != "cxq resume pack-session" {
		t.Fatalf("resume command = %q", evidence.ResumeCommand)
	}
}

func TestPackRequiresExistingIndexWithoutCreatingOne(t *testing.T) {
	home := t.TempDir()
	databasePath := DefaultIndexPath(home)

	_, err := Pack(context.Background(), home, PackOptions{Query: "needle", Limit: 5})
	if err == nil || !strings.Contains(err.Error(), "run cxq index first") {
		t.Fatalf("Pack() error = %v", err)
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("Pack() created database unexpectedly: %v", statErr)
	}
}
