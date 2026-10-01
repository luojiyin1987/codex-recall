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

const upsertSessionSQL = `
INSERT INTO sessions (
    session_id, timestamp, cwd, project, source, rollout_path, content_hash,
    rollout_size, rollout_mtime_ns, indexed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    timestamp = excluded.timestamp,
    cwd = excluded.cwd,
    project = excluded.project,
    source = excluded.source,
    rollout_path = excluded.rollout_path,
    content_hash = excluded.content_hash,
    rollout_size = excluded.rollout_size,
    rollout_mtime_ns = excluded.rollout_mtime_ns,
    indexed_at = excluded.indexed_at
`

const selectMessageRowIDsSQL = "SELECT rowid FROM messages WHERE session_id = ?"

const deleteFTSMessageSQL = "DELETE FROM messages_fts WHERE rowid = ?"

type SQLiteIndex struct {
	db *sql.DB
}

var _ Index = (*SQLiteIndex)(nil)

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

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

	for version := currentVersion + 1; version <= schemaVersion; version++ {
		for _, statement := range schemaMigrations[version] {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("migrate sqlite schema to version %d: %w", version, err)
			}
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			return fmt.Errorf("set sqlite schema version %d: %w", version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema initialization: %w", err)
	}
	return nil
}

func (s *SQLiteIndex) UpsertSession(ctx context.Context, session Session) error {
	if err := validateSession(session); err != nil {
		return err
	}
	if err := execUpsertSession(ctx, s.db, session); err != nil {
		return fmt.Errorf("upsert indexed session %q: %w", session.ID, err)
	}
	return nil
}

// UpsertSessions writes session metadata in one bounded transaction.
func (s *SQLiteIndex) UpsertSessions(ctx context.Context, sessions []Session) error {
	if len(sessions) == 0 {
		return nil
	}
	for _, session := range sessions {
		if err := validateSession(session); err != nil {
			return err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin indexed session batch upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, upsertSessionSQL)
	if err != nil {
		return fmt.Errorf("prepare indexed session batch upsert: %w", err)
	}
	defer stmt.Close()
	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := execPreparedUpsertSession(ctx, stmt, session); err != nil {
			return fmt.Errorf("upsert indexed session %q: %w", session.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit indexed session batch upsert: %w", err)
	}
	return nil
}

func (s *SQLiteIndex) ReplaceMessages(ctx context.Context, sessionID string, messages []Message) error {
	if err := validateMessages(sessionID, messages); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin message replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := replaceMessagesTx(ctx, tx, sessionID, messages); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit message replacement: %w", err)
	}
	return nil
}

// ReplaceSession atomically publishes session metadata and its extracted
// messages. The content hash is therefore never advanced unless the matching
// message set and lexical index are committed too.
func (s *SQLiteIndex) ReplaceSession(ctx context.Context, session Session, messages []Message) error {
	return s.ReplaceSessions(ctx, []SessionReplacement{{
		Session:  session,
		Messages: messages,
	}})
}

// ReplaceSessions publishes a bounded batch in one transaction. Statements are
// prepared once per batch so large refreshes avoid one commit and repeated
// prepares for every individual session.
func (s *SQLiteIndex) ReplaceSessions(ctx context.Context, replacements []SessionReplacement) error {
	_, err := s.ReplaceSessionsWithProfile(ctx, replacements)
	return err
}

// ReplaceSessionsWithProfile is ReplaceSessions with diagnostic timing for the
// individual SQLite write phases. The profile does not alter write semantics.
func (s *SQLiteIndex) ReplaceSessionsWithProfile(ctx context.Context, replacements []SessionReplacement) (WriteProfile, error) {
	var profile WriteProfile
	if len(replacements) == 0 {
		return profile, nil
	}

	validationStart := time.Now()
	for _, replacement := range replacements {
		if err := validateSession(replacement.Session); err != nil {
			profile.Validation += time.Since(validationStart)
			return profile, err
		}
		if err := validateMessages(replacement.Session.ID, replacement.Messages); err != nil {
			profile.Validation += time.Since(validationStart)
			return profile, err
		}
	}
	profile.Validation += time.Since(validationStart)

	beginStart := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	profile.TransactionBegin += time.Since(beginStart)
	if err != nil {
		return profile, fmt.Errorf("begin indexed session batch replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	prepareStart := time.Now()
	upsertStmt, err := tx.PrepareContext(ctx, upsertSessionSQL)
	profile.StatementPrepare += time.Since(prepareStart)
	if err != nil {
		return profile, fmt.Errorf("prepare indexed session upsert: %w", err)
	}
	defer upsertStmt.Close()

	prepareStart = time.Now()
	selectMessageRowIDsStmt, err := tx.PrepareContext(ctx, selectMessageRowIDsSQL)
	profile.StatementPrepare += time.Since(prepareStart)
	if err != nil {
		return profile, fmt.Errorf("prepare indexed message rowid select: %w", err)
	}
	defer selectMessageRowIDsStmt.Close()

	prepareStart = time.Now()
	deleteFTSStmt, err := tx.PrepareContext(ctx, deleteFTSMessageSQL)
	profile.StatementPrepare += time.Since(prepareStart)
	if err != nil {
		return profile, fmt.Errorf("prepare lexical message delete: %w", err)
	}
	defer deleteFTSStmt.Close()

	prepareStart = time.Now()
	deleteMessageStmt, err := tx.PrepareContext(ctx, "DELETE FROM messages WHERE session_id = ?")
	profile.StatementPrepare += time.Since(prepareStart)
	if err != nil {
		return profile, fmt.Errorf("prepare indexed message delete: %w", err)
	}
	defer deleteMessageStmt.Close()

	prepareStart = time.Now()
	messageStmt, err := tx.PrepareContext(ctx, `
INSERT INTO messages (session_id, ordinal, role, text, timestamp)
VALUES (?, ?, ?, ?, ?)
`)
	profile.StatementPrepare += time.Since(prepareStart)
	if err != nil {
		return profile, fmt.Errorf("prepare indexed message insert: %w", err)
	}
	defer messageStmt.Close()

	prepareStart = time.Now()
	ftsStmt, err := tx.PrepareContext(ctx, `
INSERT INTO messages_fts (rowid, session_id, ordinal, role, text)
VALUES (?, ?, ?, ?, ?)
`)
	profile.StatementPrepare += time.Since(prepareStart)
	if err != nil {
		return profile, fmt.Errorf("prepare lexical message insert: %w", err)
	}
	defer ftsStmt.Close()

	for _, replacement := range replacements {
		if err := ctx.Err(); err != nil {
			return profile, err
		}
		session := replacement.Session
		upsertStart := time.Now()
		err := execPreparedUpsertSession(ctx, upsertStmt, session)
		profile.SessionUpsert += time.Since(upsertStart)
		if err != nil {
			return profile, fmt.Errorf("upsert indexed session %q: %w", session.ID, err)
		}
		if err := replaceMessagesPrepared(
			ctx,
			selectMessageRowIDsStmt,
			deleteFTSStmt,
			deleteMessageStmt,
			messageStmt,
			ftsStmt,
			session.ID,
			replacement.Messages,
			&profile,
		); err != nil {
			return profile, err
		}
	}

	commitStart := time.Now()
	err = tx.Commit()
	profile.Commit += time.Since(commitStart)
	if err != nil {
		return profile, fmt.Errorf("commit indexed session batch replacement: %w", err)
	}
	return profile, nil
}

func execUpsertSession(ctx context.Context, execer sqlExecer, session Session) error {
	_, err := execer.ExecContext(ctx, upsertSessionSQL,
		session.ID,
		formatTime(session.Timestamp),
		session.CWD,
		session.Project,
		session.Source,
		session.RolloutPath,
		session.ContentHash,
		session.RolloutSize,
		session.RolloutMTimeNS,
		formatTime(time.Now().UTC()),
	)
	return err
}

func execPreparedUpsertSession(ctx context.Context, stmt *sql.Stmt, session Session) error {
	_, err := stmt.ExecContext(ctx,
		session.ID,
		formatTime(session.Timestamp),
		session.CWD,
		session.Project,
		session.Source,
		session.RolloutPath,
		session.ContentHash,
		session.RolloutSize,
		session.RolloutMTimeNS,
		formatTime(time.Now().UTC()),
	)
	return err
}

func replaceMessagesPrepared(
	ctx context.Context,
	selectMessageRowIDsStmt *sql.Stmt,
	deleteFTSStmt *sql.Stmt,
	deleteMessageStmt *sql.Stmt,
	messageStmt *sql.Stmt,
	ftsStmt *sql.Stmt,
	sessionID string,
	messages []Message,
	profile *WriteProfile,
) error {
	deleteStart := time.Now()
	err := deleteFTSMessagesPrepared(ctx, selectMessageRowIDsStmt, deleteFTSStmt, sessionID)
	profile.FTSDelete += time.Since(deleteStart)
	if err != nil {
		return err
	}

	deleteStart = time.Now()
	_, err = deleteMessageStmt.ExecContext(ctx, sessionID)
	profile.MessageDelete += time.Since(deleteStart)
	if err != nil {
		return fmt.Errorf("clear indexed messages for session %q: %w", sessionID, err)
	}

	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return err
		}
		var timestamp any
		if message.Timestamp != nil {
			timestamp = formatTime(*message.Timestamp)
		}
		insertStart := time.Now()
		insertResult, err := messageStmt.ExecContext(ctx, sessionID, message.Ordinal, message.Role, message.Text, timestamp)
		if err != nil {
			profile.MessageInsert += time.Since(insertStart)
			return fmt.Errorf("insert indexed message at ordinal %d for session %q: %w", message.Ordinal, sessionID, err)
		}
		messageRowID, err := insertResult.LastInsertId()
		profile.MessageInsert += time.Since(insertStart)
		if err != nil {
			return fmt.Errorf("read indexed message rowid at ordinal %d for session %q: %w", message.Ordinal, sessionID, err)
		}

		insertStart = time.Now()
		_, err = ftsStmt.ExecContext(ctx, messageRowID, sessionID, message.Ordinal, message.Role, message.Text)
		profile.FTSInsert += time.Since(insertStart)
		if err != nil {
			return fmt.Errorf("insert lexical message at ordinal %d for session %q: %w", message.Ordinal, sessionID, err)
		}
	}
	return nil
}

func replaceMessagesTx(ctx context.Context, tx *sql.Tx, sessionID string, messages []Message) error {
	selectMessageRowIDsStmt, err := tx.PrepareContext(ctx, selectMessageRowIDsSQL)
	if err != nil {
		return fmt.Errorf("prepare indexed message rowid select: %w", err)
	}
	defer selectMessageRowIDsStmt.Close()
	deleteFTSStmt, err := tx.PrepareContext(ctx, deleteFTSMessageSQL)
	if err != nil {
		return fmt.Errorf("prepare lexical message delete: %w", err)
	}
	defer deleteFTSStmt.Close()
	if err := deleteFTSMessagesPrepared(ctx, selectMessageRowIDsStmt, deleteFTSStmt, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM messages WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("clear indexed messages for session %q: %w", sessionID, err)
	}

	messageStmt, err := tx.PrepareContext(ctx, `
INSERT INTO messages (session_id, ordinal, role, text, timestamp)
VALUES (?, ?, ?, ?, ?)
`)
	if err != nil {
		return fmt.Errorf("prepare indexed message insert: %w", err)
	}
	defer messageStmt.Close()

	ftsStmt, err := tx.PrepareContext(ctx, `
INSERT INTO messages_fts (rowid, session_id, ordinal, role, text)
VALUES (?, ?, ?, ?, ?)
`)
	if err != nil {
		return fmt.Errorf("prepare lexical message insert: %w", err)
	}
	defer ftsStmt.Close()

	for _, message := range messages {
		var timestamp any
		if message.Timestamp != nil {
			timestamp = formatTime(*message.Timestamp)
		}
		insertResult, err := messageStmt.ExecContext(ctx, sessionID, message.Ordinal, message.Role, message.Text, timestamp)
		if err != nil {
			return fmt.Errorf("insert indexed message at ordinal %d: %w", message.Ordinal, err)
		}
		messageRowID, err := insertResult.LastInsertId()
		if err != nil {
			return fmt.Errorf("read indexed message rowid at ordinal %d: %w", message.Ordinal, err)
		}
		if _, err := ftsStmt.ExecContext(ctx, messageRowID, sessionID, message.Ordinal, message.Role, message.Text); err != nil {
			return fmt.Errorf("insert lexical message at ordinal %d: %w", message.Ordinal, err)
		}
	}
	return nil
}

func deleteFTSMessagesPrepared(ctx context.Context, selectRowIDsStmt, deleteFTSStmt *sql.Stmt, sessionID string) error {
	rows, err := selectRowIDsStmt.QueryContext(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list lexical message rowids for session %q: %w", sessionID, err)
	}
	var rowIDs []int64
	for rows.Next() {
		var rowID int64
		if err := rows.Scan(&rowID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan lexical message rowid for session %q: %w", sessionID, err)
		}
		rowIDs = append(rowIDs, rowID)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close lexical message rowids for session %q: %w", sessionID, err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list lexical message rowids for session %q: %w", sessionID, err)
	}
	for _, rowID := range rowIDs {
		if _, err := deleteFTSStmt.ExecContext(ctx, rowID); err != nil {
			return fmt.Errorf("clear lexical message rowid %d for session %q: %w", rowID, sessionID, err)
		}
	}
	return nil
}

func validateSession(session Session) error {
	if session.ID == "" {
		return errors.New("session id must not be empty")
	}
	if session.RolloutPath == "" {
		return errors.New("rollout path must not be empty")
	}
	if session.ContentHash == "" {
		return errors.New("content hash must not be empty")
	}
	return nil
}

func validateMessages(sessionID string, messages []Message) error {
	if sessionID == "" {
		return errors.New("session id must not be empty")
	}
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
	}
	return nil
}

func (s *SQLiteIndex) Sessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT session_id, timestamp, cwd, project, source, rollout_path, content_hash,
       rollout_size, rollout_mtime_ns
FROM sessions
ORDER BY session_id
`)
	if err != nil {
		return nil, fmt.Errorf("list indexed sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var timestamp string
		if err := rows.Scan(
			&session.ID,
			&timestamp,
			&session.CWD,
			&session.Project,
			&session.Source,
			&session.RolloutPath,
			&session.ContentHash,
			&session.RolloutSize,
			&session.RolloutMTimeNS,
		); err != nil {
			return nil, fmt.Errorf("scan indexed session: %w", err)
		}
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse indexed session timestamp %q: %w", timestamp, err)
		}
		session.Timestamp = parsed
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed sessions: %w", err)
	}
	return sessions, nil
}

// DeleteSession atomically removes a stale derived session and all searchable
// data associated with it.
func (s *SQLiteIndex) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("session id must not be empty")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin indexed session deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	selectMessageRowIDsStmt, err := tx.PrepareContext(ctx, selectMessageRowIDsSQL)
	if err != nil {
		return fmt.Errorf("prepare indexed message rowid select: %w", err)
	}
	defer selectMessageRowIDsStmt.Close()
	deleteFTSStmt, err := tx.PrepareContext(ctx, deleteFTSMessageSQL)
	if err != nil {
		return fmt.Errorf("prepare lexical message delete: %w", err)
	}
	defer deleteFTSStmt.Close()
	if err := deleteFTSMessagesPrepared(ctx, selectMessageRowIDsStmt, deleteFTSStmt, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE session_id = ?", id); err != nil {
		return fmt.Errorf("delete indexed session %q: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit indexed session deletion: %w", err)
	}
	return nil
}

func (s *SQLiteIndex) Session(ctx context.Context, id string) (Session, bool, error) {
	var session Session
	var timestamp string

	err := s.db.QueryRowContext(ctx, `
SELECT session_id, timestamp, cwd, project, source, rollout_path, content_hash,
       rollout_size, rollout_mtime_ns
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
		&session.RolloutSize,
		&session.RolloutMTimeNS,
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
