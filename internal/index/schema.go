package index

const schemaVersion = 2

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

var schemaMigrations = map[int][]string{
	2: {
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    session_id UNINDEXED,
    ordinal UNINDEXED,
    role UNINDEXED,
    text,
    tokenize = 'unicode61'
)`,
		`INSERT INTO messages_fts (session_id, ordinal, role, text)
SELECT m.session_id, m.ordinal, m.role, m.text
FROM messages AS m
WHERE NOT EXISTS (
    SELECT 1
    FROM messages_fts AS f
    WHERE f.session_id = m.session_id
      AND f.ordinal = m.ordinal
)`,
	},
}
