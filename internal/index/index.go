package index

import (
	"context"
	"time"
)

// Session is the stable metadata stored in the derived local index.
type Session struct {
	ID          string
	Timestamp   time.Time
	CWD         string
	Project     string
	Source      string
	RolloutPath string
	ContentHash string
}

// Message is one searchable conversation message in a session.
type Message struct {
	SessionID string
	Ordinal   int
	Role      string
	Text      string
	Timestamp *time.Time
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

// Index stores disposable, derived data built from Codex rollout files.
// Codex rollout files remain the source of truth.
type Index interface {
	UpsertSession(ctx context.Context, session Session) error
	ReplaceMessages(ctx context.Context, sessionID string, messages []Message) error
	ReplaceSession(ctx context.Context, session Session, messages []Message) error
	Session(ctx context.Context, id string) (Session, bool, error)
	Sessions(ctx context.Context) ([]Session, error)
	DeleteSession(ctx context.Context, id string) error
	Search(ctx context.Context, options SearchOptions) ([]SearchMatch, error)
	Close() error
}
