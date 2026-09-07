package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/luojiyin1987/codex-recall/internal/codex"
	"github.com/luojiyin1987/codex-recall/internal/index"
)

const (
	contentHashVersion  = "v1"
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
	Profile    BuildProfile
}

// BuildProfile records work performed during one derived-index build.
type BuildProfile struct {
	Discovery          time.Duration
	MetadataParse      time.Duration
	Catalog            time.Duration
	IndexStateRead     time.Duration
	Hash               time.Duration
	ConversationDecode time.Duration
	DatabaseWrite      time.Duration
	StaleCleanup       time.Duration
	Total              time.Duration
	FilesHashed        int
	HashBytes          int64
	FilesDecoded       int
	// ConversationSourceBytes is the hashed size of files sent to the decoder.
	ConversationSourceBytes int64
	MessagesDecoded         int
	BatchesWritten          int
	RolloutFiles            int
	UnreadableFiles         int
}

// Build incrementally refreshes a derived index from the logical Codex
// sessions under home. Rollout files remain the source of truth.
func Build(ctx context.Context, home string, store Store) (result Result, returnErr error) {
	buildStart := time.Now()
	defer func() {
		result.Profile.Total = time.Since(buildStart)
	}()

	sessions, warnings, catalogProfile, err := codex.NewCatalog(home).SessionsWithProfileContext(ctx)
	result.Profile.Discovery = catalogProfile.Discovery
	result.Profile.MetadataParse = catalogProfile.MetadataParse
	result.Profile.Catalog = catalogProfile.Total
	result.Profile.RolloutFiles = catalogProfile.FilesDiscovered
	result.Profile.UnreadableFiles = catalogProfile.FilesUnreadable
	result.Discovered = len(sessions)
	result.Warnings = append([]error(nil), warnings...)
	if err != nil {
		return result, fmt.Errorf("discover Codex sessions: %w", err)
	}

	currentSessionIDs := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		currentSessionIDs[session.ID] = struct{}{}
	}

	indexStateStart := time.Now()
	indexedSessions, err := store.Sessions(ctx)
	result.Profile.IndexStateRead = time.Since(indexStateStart)
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
		writeStart := time.Now()
		err := store.ReplaceSessions(ctx, pending)
		result.Profile.DatabaseWrite += time.Since(writeStart)
		if err != nil {
			return err
		}
		result.Indexed += len(pending)
		result.Profile.BatchesWritten++
		pending = pending[:0]
		return nil
	}

	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		hashStart := time.Now()
		contentHash, bytesRead, err := hashRolloutContextMeasured(ctx, session.Path)
		result.Profile.Hash += time.Since(hashStart)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Errorf("%s: hash rollout: %w", session.Path, err))
			continue
		}
		result.Profile.FilesHashed++
		result.Profile.HashBytes += bytesRead

		current, found := indexedByID[session.ID]
		if found && current.ContentHash == contentHash && current.RolloutPath == session.Path {
			result.Skipped++
			continue
		}

		conversationStart := time.Now()
		conversation, err := codex.ReadConversationContext(ctx, session.Path)
		result.Profile.ConversationDecode += time.Since(conversationStart)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Errorf("%s: read conversation: %w", session.Path, err))
			continue
		}
		result.Profile.FilesDecoded++
		result.Profile.ConversationSourceBytes += bytesRead
		result.Profile.MessagesDecoded += len(conversation)

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

	staleStart := time.Now()
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
	result.Profile.StaleCleanup = time.Since(staleStart)

	return result, nil
}

func hashRollout(path string) (string, error) {
	return hashRolloutContext(context.Background(), path)
}

func hashRolloutContext(ctx context.Context, path string) (string, error) {
	hash, _, err := hashRolloutContextMeasured(ctx, path)
	return hash, err
}

func hashRolloutContextMeasured(ctx context.Context, path string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hash := sha256.New()
	bytesRead, err := io.Copy(hash, contextReader{ctx: ctx, reader: file})
	if err != nil {
		return "", bytesRead, err
	}
	return contentHashVersion + ":sha256:" + hex.EncodeToString(hash.Sum(nil)), bytesRead, nil
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
