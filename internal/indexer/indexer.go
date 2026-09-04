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

const contentHashVersion = "v1"

type Store interface {
	Session(ctx context.Context, id string) (index.Session, bool, error)
	Sessions(ctx context.Context) ([]index.Session, error)
	ReplaceSession(ctx context.Context, session index.Session, messages []index.Message) error
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
	sessions, warnings, err := codex.NewCatalog(home).Sessions()
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

	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		contentHash, err := hashRollout(session.Path)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Errorf("%s: hash rollout: %w", session.Path, err))
			continue
		}

		current, found, err := store.Session(ctx, session.ID)
		if err != nil {
			return result, fmt.Errorf("read indexed session %q: %w", session.ID, err)
		}
		if found && current.ContentHash == contentHash && current.RolloutPath == session.Path {
			result.Skipped++
			continue
		}

		conversation, err := codex.ReadConversation(session.Path)
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
		if err := store.ReplaceSession(ctx, indexedSession, indexedMessages); err != nil {
			return result, fmt.Errorf("replace indexed session %q: %w", session.ID, err)
		}
		result.Indexed++
	}

	if len(warnings) == 0 {
		indexedSessions, err := store.Sessions(ctx)
		if err != nil {
			return result, fmt.Errorf("list indexed sessions for reconciliation: %w", err)
		}
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
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return contentHashVersion + ":sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
