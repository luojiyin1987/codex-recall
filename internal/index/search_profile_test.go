package index

import (
	"context"
	"testing"
	"time"
)

func TestSQLiteSearchWithProfileReportsFTSPhases(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	session := testSearchSession("profile-fts", "/work/profile", "profile", "vscode", time.Now().UTC())
	if err := idx.ReplaceSession(ctx, session, []Message{
		{Ordinal: 0, Role: "user", Text: "profile WebRTC transport"},
		{Ordinal: 1, Role: "assistant", Text: "profile WebRTC transport again"},
	}); err != nil {
		t.Fatal(err)
	}

	matches, profile, err := idx.SearchWithProfile(ctx, SearchOptions{Query: "WebRTC transport", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
	if profile.Backend != "fts5" {
		t.Fatalf("backend = %q, want fts5", profile.Backend)
	}
	if profile.RowsScanned < 1 {
		t.Fatalf("rows scanned = %d, want >= 1", profile.RowsScanned)
	}
	if profile.SessionsReturned != 1 {
		t.Fatalf("sessions returned = %d, want 1", profile.SessionsReturned)
	}
	if profile.SearchTotal <= 0 {
		t.Fatalf("search total = %s, want > 0", profile.SearchTotal)
	}
}

func TestSQLiteSearchWithProfileReportsSubstringFallback(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	session := testSearchSession("profile-short", "/work/profile", "profile", "cli", time.Now().UTC())
	if err := idx.ReplaceSession(ctx, session, []Message{{Ordinal: 0, Role: "user", Text: "Go runtime profile"}}); err != nil {
		t.Fatal(err)
	}

	matches, profile, err := idx.SearchWithProfile(ctx, SearchOptions{Query: "go", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
	if profile.Backend != "substring" {
		t.Fatalf("backend = %q, want substring", profile.Backend)
	}
	if profile.RowsScanned < 1 {
		t.Fatalf("rows scanned = %d, want >= 1", profile.RowsScanned)
	}
	if profile.SessionsReturned != 1 {
		t.Fatalf("sessions returned = %d, want 1", profile.SessionsReturned)
	}
}
