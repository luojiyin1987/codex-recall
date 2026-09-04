package index

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenSQLiteMigratesV1MessagesIntoFTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range schemaStatements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}

	timestamp := time.Date(2026, 9, 4, 5, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.Exec(`
INSERT INTO sessions (
    session_id, timestamp, cwd, project, source, rollout_path, content_hash, indexed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, "legacy", timestamp, "/work/legacy", "legacy", "cli", "/codex/legacy.jsonl", "hash", timestamp); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
INSERT INTO messages (session_id, ordinal, role, text, timestamp)
VALUES (?, ?, ?, ?, ?)
`, "legacy", 0, "assistant", "preexisting migration needle", timestamp); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	idx, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	assertSchemaVersion(t, idx.db, schemaVersion)
	assertTableExists(t, idx.db, "messages_fts")

	matches, err := idx.Search(context.Background(), SearchOptions{Query: "migration needle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Session.ID != "legacy" {
		t.Fatalf("matches = %#v", matches)
	}
}
