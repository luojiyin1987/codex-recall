package indexer

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestStatusReportsExistingIndex(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "status-a", "hello", "world")
	writeRollout(t, home, "status-b", "hello again", "world again")

	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	result, err := Status(context.Background(), home, StatusOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.DatabasePath != DefaultIndexPath(home) {
		t.Fatalf("DatabasePath = %q, want %q", result.DatabasePath, DefaultIndexPath(home))
	}
	if result.Sessions != 2 {
		t.Fatalf("Sessions = %d, want 2", result.Sessions)
	}
	wantLatest := time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC)
	if !result.LatestSession.Equal(wantLatest) {
		t.Fatalf("LatestSession = %v, want %v", result.LatestSession, wantLatest)
	}
	if result.DatabaseBytes <= 0 {
		t.Fatalf("DatabaseBytes = %d, want > 0", result.DatabaseBytes)
	}
}

func TestStatusRequiresExistingIndex(t *testing.T) {
	home := t.TempDir()
	databasePath := DefaultIndexPath(home)

	_, err := Status(context.Background(), home, StatusOptions{})
	if err == nil {
		t.Fatal("Status() accepted missing index")
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("Status() created database unexpectedly: %v", statErr)
	}
}
