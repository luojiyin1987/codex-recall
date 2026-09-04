package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/luojiyin1987/codex-recall/internal/index"
)

type StatusOptions struct {
	DatabasePath string
}

type StatusResult struct {
	DatabasePath  string
	DatabaseBytes int64
	Sessions      int
	LatestSession time.Time
}

// Status reports basic facts about an existing derived index without refreshing
// it. A missing database is an error so this command cannot silently create an
// empty index.
func Status(ctx context.Context, home string, options StatusOptions) (StatusResult, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return StatusResult{}, errors.New("codex home must not be empty")
	}

	databasePath := strings.TrimSpace(options.DatabasePath)
	if databasePath == "" {
		databasePath = DefaultIndexPath(home)
	}
	result := StatusResult{DatabasePath: databasePath}

	info, err := os.Stat(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("derived index not found at %q; run cxq index first", databasePath)
	}
	if err != nil {
		return result, fmt.Errorf("stat derived index %q: %w", databasePath, err)
	}
	if info.IsDir() {
		return result, fmt.Errorf("derived index path %q is a directory", databasePath)
	}

	store, err := index.OpenSQLite(databasePath)
	if err != nil {
		return result, fmt.Errorf("open derived index: %w", err)
	}

	sessions, statusErr := store.Sessions(ctx)
	closeErr := store.Close()
	if statusErr != nil && closeErr != nil {
		return result, errors.Join(statusErr, fmt.Errorf("close derived index: %w", closeErr))
	}
	if statusErr != nil {
		return result, statusErr
	}
	if closeErr != nil {
		return result, fmt.Errorf("close derived index: %w", closeErr)
	}

	result.Sessions = len(sessions)
	for _, session := range sessions {
		if session.Timestamp.After(result.LatestSession) {
			result.LatestSession = session.Timestamp
		}
	}

	info, err = os.Stat(databasePath)
	if err != nil {
		return result, fmt.Errorf("stat derived index %q after read: %w", databasePath, err)
	}
	result.DatabaseBytes = info.Size()
	return result, nil
}
