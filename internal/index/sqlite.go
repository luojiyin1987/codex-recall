package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

type SQLiteIndex struct {
	db *sql.DB
}

var _ Index = (*SQLiteIndex)(nil)

func OpenSQLite(path string) (*SQLiteIndex, error) {
	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite index: %w", err)
	}
	// SQLite foreign-key pragmas are connection-local. Keep the small local
	// index on one database connection so the invariant is deterministic.
	db.SetMaxOpenConns(1)

	idx := &SQLiteIndex{db: db}
	if err := idx.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return idx, nil
}

func (s *SQLiteIndex) initialize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	var currentVersion int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&currentVersion); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if currentVersion > schemaVersion {
		return fmt.Errorf("sqlite index schema version %d is newer than supported version %d", currentVersion, schemaVersion)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite schema initialization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, statement := range schemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite schema: %w", err)
		}
	}
	if currentVersion < schemaVersion {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			return fmt.Errorf("set sqlite schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema initialization: %w", err)
	}
	return nil
}

func (s *SQLiteIndex) UpsertSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return errors.New("session id must not be empty")
	}
	if session.RolloutPath == "" {
		return errors.New("rollout path must not be empty")
	}
	if session.ContentHash == "" {
		return errors.New("content hash must not be empty")
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (
    session_id, timestamp, cwd, project, source, rollout_path, content_hash, indexed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    timestamp = excluded.timestamp,
    cwd = excluded.cwd,
    project = excluded.project,
    source = excluded.source,
    rollout_path = excluded.rollout_path,
    content_hash = excluded.content_hash,
    indexed_at = excluded.indexed_at
`, session.ID, formatTime(session.Timestamp), session.CWD, session.Project, session.Source,
		session.RolloutPath, session.ContentHash, formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("upsert indexed session %q: %w", session.ID, err)
	}
	return nil
}

func (s *SQLiteIndex) ReplaceMessages(ctx context.Context, sessionID string, messages []Message) error {
	if sessionID == "" {
		return errors.New("session id must not be empty")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin message replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM messages WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("clear indexed messages for session %q: %w", sessionID, err)
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO messages (session_id, ordinal, role, text, timestamp)
VALUES (?, ?, ?, ?, ?)
`)
	if err != nil {
		return fmt.Errorf("prepare indexed message insert: %w", err)
	}
	defer stmt.Close()

	for _, message := range messages {
		if message.SessionID != "" && message.SessionID != sessionID {
			return fmt.Errorf("message session id %q does not match replacement session %q", message.SessionID, sessionID)
		}
		if message.Ordinal < 0 {
			return fmt.Errorf("message ordinal must not be negative: %d", message.Ordinal)
		}
		if message.Role == "" {
			return fmt.Errorf("message role must not be empty at ordinal %d", message.Ordinal)
		}

		var timestamp any
		if message.Timestamp != nil {
			timestamp = formatTime(*message.Timestamp)
		}
		if _, err := stmt.ExecContext(ctx, sessionID, message.Ordinal, message.Role, message.Text, timestamp); err != nil {
			return fmt.Errorf("insert indexed message at ordinal %d: %w", message.Ordinal, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit message replacement: %w", err)
	}
	return nil
}

func (s *SQLiteIndex) Session(ctx context.Context, id string) (Session, bool, error) {
	var session Session
	var timestamp string

	err := s.db.QueryRowContext(ctx, `
SELECT session_id, timestamp, cwd, project, source, rollout_path, content_hash
FROM sessions
WHERE session_id = ?
`, id).Scan(
		&session.ID,
		&timestamp,
		&session.CWD,
		&session.Project,
		&session.Source,
		&session.RolloutPath,
		&session.ContentHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("read indexed session %q: %w", id, err)
	}

	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return Session{}, false, fmt.Errorf("parse indexed session timestamp %q: %w", timestamp, err)
	}
	session.Timestamp = parsed
	return session, true, nil
}

func (s *SQLiteIndex) Close() error {
	return s.db.Close()
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
