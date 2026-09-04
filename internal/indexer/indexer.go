package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/luojiyin1987/codex-recall/internal/codex"
	"github.com/luojiyin1987/codex-recall/internal/index"
)

const (
	contentHashVersion = "v1"
	indexWriteBatchSize = 256
)

type Store interface {
	Sessions(ctx context.Context) ([]index.Session, error)
	ReplaceSessions(ctx context.Context, replacements []index.SessionReplacement) error
	DeleteSession(ctx context.Context, id string) error
}

type Result struct {
	Discovered int
	Indexed    int
	Skipped    int
	Deleted    int
	Warnings   []error
}

// Build incrementally refreshes a derived index from the logical Codex
// sessions under home. Rollout files remain the source of truth.
func Build(ctx context.Context, home string, store Store) (Result, error) {
	sessions, warnings, err := codex.NewCatalog(home).SessionsContext(ctx)
	result := Result{
		Discovered: len(sessions),
		Warnings:   append([]error(nil), warnings...),
	}
	if err != nil {
		return result, fmt.Errorf("discover Codex sessions: %w", err)
	}

	currentSessionIDs := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		currentSessionIDs[session.ID] = struct{}{}
	}

	indexedSessions, err := store.Sessions(ctx)
	if err != nil {
		return result, fmt.Errorf("list indexed sessions: %w", err)
	}
	indexedByID := make(map[string]index.Session, len(indexedSessions))
	for _, session := range indexedSessions {
		indexedByID[session.ID] = session
	}

	pending := make([]index.SessionReplacement, 0, indexWriteBatchSize)
	flushPending := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := store.ReplaceSessions(ctx, pending); err != nil {
			return err
		}
		result.Indexed += len(pending)
		pending = pending[:0]
		return nil
	}

	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		contentHash, err := hashRolloutContext(ctx, session.Path)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Errorf("%s: hash rollout: %w", session.Path, err))
			continue
		}

		current, found := indexedByID[session.ID]
		if found && current.ContentHash == contentHash && current.RolloutPath == session.Path {
			result.Skipped++
			continue
		}

		conversation, err := codex.ReadConversationContext(ctx, session.Path)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Errorf("%s: read conversation: %w", session.Path, err))
			continue
		}

		indexedMessages := make([]index.Message, 0, len(conversation))
		for ordinal, message := range conversation {
			indexed := index.Message{
				SessionID: session.ID,
				Ordinal:   ordinal,
				Role:      message.Role,
				Text:      message.Text,
			}
			if !message.Timestamp.IsZero() {
				timestamp := message.Timestamp
				indexed.Timestamp = &timestamp
			}
			indexedMessages = append(indexedMessages, indexed)
		}

		indexedSession := index.Session{
			ID:          session.ID,
			Timestamp:   session.Timestamp,
			CWD:         session.CWD,
			Project:     session.Project(),
			Source:      session.Source,
			RolloutPath: session.Path,
			ContentHash: contentHash,
		}
		pending = append(pending, index.SessionReplacement{
			Session:  indexedSession,
			Messages: indexedMessages,
		})
		if len(pending) == indexWriteBatchSize {
			if err := flushPending(); err != nil {
				return result, fmt.Errorf("replace indexed session batch: %w", err)
			}
		}
	}
	if err := flushPending(); err != nil {
		return result, fmt.Errorf("replace indexed session batch: %w", err)
	}

	if len(warnings) == 0 {
		for _, indexedSession := range indexedSessions {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if _, current := currentSessionIDs[indexedSession.ID]; current {
				continue
			}
			if err := store.DeleteSession(ctx, indexedSession.ID); err != nil {
				return result, fmt.Errorf("delete stale indexed session %q: %w", indexedSession.ID, err)
			}
			result.Deleted++
		}
	}

	return result, nil
}

func hashRollout(path string) (string, error) {
	return hashRolloutContext(context.Background(), path)
}

func hashRolloutContext(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx: ctx, reader: file}); err != nil {
		return "", err
	}
	return contentHashVersion + ":sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
