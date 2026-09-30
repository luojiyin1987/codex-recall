package index

import (
	"context"
	"time"
)

// Session is the stable metadata stored in the derived local index.
type Session struct {
	ID             string
	Timestamp      time.Time
	CWD            string
	Project        string
	Source         string
	RolloutPath    string
	ContentHash    string
	RolloutSize    int64
	RolloutMTimeNS int64
}

// Message is one searchable conversation message in a session.
type Message struct {
	SessionID string
	Ordinal   int
	Role      string
	Text      string
	Timestamp *time.Time
}

// SessionReplacement is one atomic session metadata + message replacement.
type SessionReplacement struct {
	Session  Session
	Messages []Message
}

// WriteProfile records the SQLite work performed while publishing one batch
// of session replacements. It is diagnostic evidence only and does not change
// transaction or durability semantics.
type WriteProfile struct {
	Validation       time.Duration
	TransactionBegin time.Duration
	StatementPrepare time.Duration
	SessionUpsert    time.Duration
	FTSDelete        time.Duration
	MessageDelete    time.Duration
	MessageInsert    time.Duration
	FTSInsert        time.Duration
	Commit           time.Duration
}

type SearchOptions struct {
	Query   string
	Limit   int
	Project string
	Source  string
}

type SearchMatch struct {
	Session Session
	Ordinal int
	Role    string
	Snippet string
	Score   float64
	Why     string
}

// SearchProfile captures lightweight timing and cardinality evidence for one
// indexed search. QuerySetup covers the database QueryContext call; ResultScan
// covers row iteration and result assembly excluding snippet formatting.
type SearchProfile struct {
	Backend          string
	QuerySetup       time.Duration
	ResultScan       time.Duration
	Snippet          time.Duration
	SearchTotal      time.Duration
	RowsScanned      int
	SessionsReturned int
}

// Index stores disposable, derived data built from Codex rollout files.
// Codex rollout files remain the source of truth.
type Index interface {
	UpsertSession(ctx context.Context, session Session) error
	ReplaceMessages(ctx context.Context, sessionID string, messages []Message) error
	ReplaceSession(ctx context.Context, session Session, messages []Message) error
	ReplaceSessions(ctx context.Context, replacements []SessionReplacement) error
	Session(ctx context.Context, id string) (Session, bool, error)
	Sessions(ctx context.Context) ([]Session, error)
	DeleteSession(ctx context.Context, id string) error
	Search(ctx context.Context, options SearchOptions) ([]SearchMatch, error)
	Close() error
}
