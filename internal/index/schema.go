package index

const schemaVersion = 1

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
    session_id   TEXT PRIMARY KEY,
    timestamp    TEXT NOT NULL,
    cwd          TEXT NOT NULL DEFAULT '',
    project      TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT '',
    rollout_path TEXT NOT NULL UNIQUE,
    content_hash TEXT NOT NULL,
    indexed_at   TEXT NOT NULL
)`,
	`CREATE TABLE IF NOT EXISTS messages (
    session_id TEXT NOT NULL,
    ordinal    INTEGER NOT NULL,
    role       TEXT NOT NULL,
    text       TEXT NOT NULL,
    timestamp  TEXT,

    PRIMARY KEY (session_id, ordinal),
    FOREIGN KEY (session_id)
        REFERENCES sessions(session_id)
        ON DELETE CASCADE
)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_timestamp
    ON sessions(timestamp)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_project
    ON sessions(project)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_source
    ON sessions(source)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_session
    ON messages(session_id)`,
}
