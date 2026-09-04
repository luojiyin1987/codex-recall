package index

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenSQLiteInitializesVersionedSchemaIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	idx, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}

	assertSchemaVersion(t, idx.db, schemaVersion)
	assertTableExists(t, idx.db, "sessions")
	assertTableExists(t, idx.db, "messages")

	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	idx, err = OpenSQLite(path)
	if err != nil {
		t.Fatalf("second OpenSQLite() failed: %v", err)
	}
	defer idx.Close()
	assertSchemaVersion(t, idx.db, schemaVersion)
}

func TestSQLiteIndexUpsertSession(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	when := time.Date(2026, 9, 4, 3, 0, 0, 123, time.UTC)

	session := Session{
		ID:          "session-1",
		Timestamp:   when,
		CWD:         "/work/旧项目",
		Project:     "旧项目",
		Source:      "vscode",
		RolloutPath: "/codex/session-1.jsonl",
		ContentHash: "hash-1",
	}
	if err := idx.UpsertSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	session.Project = "新项目"
	session.Source = "cli"
	session.ContentHash = "hash-2"
	if err := idx.UpsertSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	got, ok, err := idx.Session(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Session() did not find upserted session")
	}
	if got.Project != "新项目" || got.Source != "cli" || got.ContentHash != "hash-2" {
		t.Fatalf("Session() = %#v", got)
	}
	if !got.Timestamp.Equal(when) {
		t.Fatalf("Timestamp = %v, want %v", got.Timestamp, when)
	}

	var count int
	if err := idx.db.QueryRow("SELECT count(*) FROM sessions WHERE session_id = ?", session.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("session row count = %d, want 1", count)
	}
}

func TestSQLiteIndexReplaceMessagesIsAtomicReplacement(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	insertTestSession(t, idx, "session-1")

	if err := idx.ReplaceMessages(ctx, "session-1", []Message{
		{Ordinal: 0, Role: "user", Text: "第一条消息"},
		{Ordinal: 1, Role: "assistant", Text: "old message"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := idx.ReplaceMessages(ctx, "session-1", []Message{
		{Ordinal: 0, Role: "assistant", Text: "replacement ✓"},
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := idx.db.Query("SELECT ordinal, role, text FROM messages WHERE session_id = ? ORDER BY ordinal", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("replacement left no message")
	}
	var ordinal int
	var role, text string
	if err := rows.Scan(&ordinal, &role, &text); err != nil {
		t.Fatal(err)
	}
	if ordinal != 0 || role != "assistant" || text != "replacement ✓" {
		t.Fatalf("message = (%d, %q, %q)", ordinal, role, text)
	}
	if rows.Next() {
		t.Fatal("stale message remained after replacement")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteIndexReplaceMessagesRollsBackOnInvalidBatch(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	insertTestSession(t, idx, "session-1")

	if err := idx.ReplaceMessages(ctx, "session-1", []Message{{Ordinal: 0, Role: "user", Text: "keep me"}}); err != nil {
		t.Fatal(err)
	}

	err := idx.ReplaceMessages(ctx, "session-1", []Message{
		{Ordinal: 0, Role: "assistant", Text: "new"},
		{Ordinal: 1, Role: "", Text: "invalid"},
	})
	if err == nil {
		t.Fatal("ReplaceMessages() accepted invalid role")
	}

	var text string
	if err := idx.db.QueryRow("SELECT text FROM messages WHERE session_id = ? AND ordinal = 0", "session-1").Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "keep me" {
		t.Fatalf("message after rollback = %q, want %q", text, "keep me")
	}
}

func TestSQLiteForeignKeysAreEnforced(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	err := idx.ReplaceMessages(ctx, "missing-session", []Message{{Ordinal: 0, Role: "user", Text: "orphan"}})
	if err == nil {
		t.Fatal("ReplaceMessages() created orphan message")
	}
}

func openTestIndex(t *testing.T) *SQLiteIndex {
	t.Helper()
	idx, err := OpenSQLite(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := idx.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return idx
}

func insertTestSession(t *testing.T, idx *SQLiteIndex, id string) {
	t.Helper()
	err := idx.UpsertSession(context.Background(), Session{
		ID:          id,
		Timestamp:   time.Date(2026, 9, 4, 3, 0, 0, 0, time.UTC),
		RolloutPath: "/codex/" + id + ".jsonl",
		ContentHash: "hash-" + id,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertSchemaVersion(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("table %q count = %d, want 1", table, count)
	}
}


func TestSQLiteIndexReplaceSessionCommitsMetadataAndMessagesTogether(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	when := time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC)

	session := Session{
		ID:          "session-atomic",
		Timestamp:   when,
		CWD:         "/work/demo",
		Project:     "demo",
		Source:      "vscode",
		RolloutPath: "/codex/session-atomic.jsonl",
		ContentHash: "v1:sha256:first",
	}
	if err := idx.ReplaceSession(ctx, session, []Message{
		{Ordinal: 0, Role: "user", Text: "hello"},
		{Ordinal: 1, Role: "assistant", Text: "world"},
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := idx.Session(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.ContentHash != session.ContentHash {
		t.Fatalf("Session() = %#v, found = %v", got, ok)
	}

	var count int
	if err := idx.db.QueryRow("SELECT count(*) FROM messages WHERE session_id = ?", session.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("message count = %d, want 2", count)
	}
}

func TestSQLiteIndexReplaceSessionRollsBackHashWhenMessageInsertFails(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	original := Session{
		ID:          "session-rollback",
		Timestamp:   time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC),
		RolloutPath: "/codex/session-rollback.jsonl",
		ContentHash: "v1:sha256:old",
	}
	if err := idx.ReplaceSession(ctx, original, []Message{
		{Ordinal: 0, Role: "user", Text: "keep me"},
	}); err != nil {
		t.Fatal(err)
	}

	updated := original
	updated.ContentHash = "v1:sha256:new"
	err := idx.ReplaceSession(ctx, updated, []Message{
		{Ordinal: 0, Role: "user", Text: "new message"},
		{Ordinal: 0, Role: "assistant", Text: "duplicate ordinal forces sqlite failure"},
	})
	if err == nil {
		t.Fatal("ReplaceSession() unexpectedly accepted duplicate ordinals")
	}

	got, ok, readErr := idx.Session(ctx, original.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !ok {
		t.Fatal("original session disappeared after rollback")
	}
	if got.ContentHash != original.ContentHash {
		t.Fatalf("content hash = %q, want rollback to %q", got.ContentHash, original.ContentHash)
	}

	var text string
	if err := idx.db.QueryRow("SELECT text FROM messages WHERE session_id = ? AND ordinal = 0", original.ID).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "keep me" {
		t.Fatalf("message after rollback = %q, want %q", text, "keep me")
	}
}


func TestSQLiteIndexSessionsListsStoredSessions(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	insertTestSession(t, idx, "session-b")
	insertTestSession(t, idx, "session-a")

	sessions, err := idx.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].ID != "session-a" || sessions[1].ID != "session-b" {
		t.Fatalf("Sessions() order = %#v", sessions)
	}
}

func TestSQLiteIndexDeleteSessionRemovesRelationalAndLexicalData(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	session := Session{
		ID:          "stale-session",
		Timestamp:   time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC),
		CWD:         "/work/demo",
		Project:     "demo",
		Source:      "vscode",
		RolloutPath: "/codex/stale-session.jsonl",
		ContentHash: "v1:sha256:stale",
	}
	if err := idx.ReplaceSession(ctx, session, []Message{
		{Ordinal: 0, Role: "user", Text: "stale searchable needle"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := idx.DeleteSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}

	if _, found, err := idx.Session(ctx, session.ID); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("deleted session still exists")
	}

	var messageCount int
	if err := idx.db.QueryRow("SELECT count(*) FROM messages WHERE session_id = ?", session.ID).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 0 {
		t.Fatalf("relational message count = %d, want 0", messageCount)
	}

	var ftsCount int
	if err := idx.db.QueryRow("SELECT count(*) FROM messages_fts WHERE session_id = ?", session.ID).Scan(&ftsCount); err != nil {
		t.Fatal(err)
	}
	if ftsCount != 0 {
		t.Fatalf("FTS message count = %d, want 0", ftsCount)
	}

	matches, err := idx.Search(ctx, SearchOptions{Query: "stale searchable", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("deleted session remained searchable: %#v", matches)
	}
}

func TestSQLiteIndexReplaceSessionsRollsBackWholeBatchOnWriteFailure(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	first := Session{
		ID:          "batch-first",
		Timestamp:   time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC),
		RolloutPath: "/codex/batch-first.jsonl",
		ContentHash: "v1:sha256:first-old",
	}
	second := Session{
		ID:          "batch-second",
		Timestamp:   time.Date(2026, 9, 4, 10, 0, 1, 0, time.UTC),
		RolloutPath: "/codex/batch-second.jsonl",
		ContentHash: "v1:sha256:second-old",
	}
	if err := idx.ReplaceSession(ctx, first, []Message{{Ordinal: 0, Role: "user", Text: "first old"}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.ReplaceSession(ctx, second, []Message{{Ordinal: 0, Role: "user", Text: "second old"}}); err != nil {
		t.Fatal(err)
	}

	firstUpdated := first
	firstUpdated.ContentHash = "v1:sha256:first-new"
	secondUpdated := second
	secondUpdated.ContentHash = "v1:sha256:second-new"

	err := idx.ReplaceSessions(ctx, []SessionReplacement{
		{
			Session:  firstUpdated,
			Messages: []Message{{Ordinal: 0, Role: "assistant", Text: "first new"}},
		},
		{
			Session: secondUpdated,
			Messages: []Message{
				{Ordinal: 0, Role: "user", Text: "duplicate one"},
				{Ordinal: 0, Role: "assistant", Text: "duplicate two"},
			},
		},
	})
	if err == nil {
		t.Fatal("ReplaceSessions() unexpectedly accepted failing batch")
	}

	gotFirst, ok, readErr := idx.Session(ctx, first.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !ok || gotFirst.ContentHash != first.ContentHash {
		t.Fatalf("first session after rollback = %#v, found = %v", gotFirst, ok)
	}
	gotSecond, ok, readErr := idx.Session(ctx, second.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !ok || gotSecond.ContentHash != second.ContentHash {
		t.Fatalf("second session after rollback = %#v, found = %v", gotSecond, ok)
	}

	for _, tc := range []struct {
		id   string
		want string
	}{
		{id: first.ID, want: "first old"},
		{id: second.ID, want: "second old"},
	} {
		var text string
		if err := idx.db.QueryRow("SELECT text FROM messages WHERE session_id = ? AND ordinal = 0", tc.id).Scan(&text); err != nil {
			t.Fatal(err)
		}
		if text != tc.want {
			t.Fatalf("message for %s after rollback = %q, want %q", tc.id, text, tc.want)
		}
	}
}
