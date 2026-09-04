package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchRequiresExistingDerivedIndex(t *testing.T) {
	home := t.TempDir()
	path := DefaultIndexPath(home)

	_, err := Search(context.Background(), home, SearchOptions{Query: "needle", Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "run cxq index") {
		t.Fatalf("Search() error = %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("Search() created missing database: %v", statErr)
	}
}

func TestSearchReadsRefreshedIndexWithoutRefreshingRollouts(t *testing.T) {
	home := t.TempDir()
	path := writeRollout(t, home, "search-session", "indexed needle", "answer")
	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(`{"timestamp":"2026-09-04T07:00:03Z","type":"event_msg","payload":{"type":"user_message","message":"fresh rollout text"}}` + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	indexed, err := Search(context.Background(), home, SearchOptions{Query: "indexed needle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed.Matches) != 1 {
		t.Fatalf("indexed matches = %#v", indexed.Matches)
	}

	fresh, err := Search(context.Background(), home, SearchOptions{Query: "fresh rollout", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Matches) != 0 {
		t.Fatalf("Search() refreshed rollouts unexpectedly: %#v", fresh.Matches)
	}
}

func TestSearchUsesCustomDatabasePath(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "custom-search", "custom needle", "answer")
	custom := filepath.Join(t.TempDir(), "nested", "custom.db")
	if _, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: custom}); err != nil {
		t.Fatal(err)
	}

	result, err := Search(context.Background(), home, SearchOptions{
		DatabasePath: custom,
		Query:        "custom needle",
		Limit:        10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DatabasePath != custom || len(result.Matches) != 1 {
		t.Fatalf("Search() = %#v", result)
	}
}
