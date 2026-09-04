package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/luojiyin1987/codex-recall/internal/index"
)

type SearchOptions struct {
	DatabasePath string
	Query        string
	Limit        int
	Project      string
	Source       string
}

type SearchResult struct {
	DatabasePath string
	Matches      []index.SearchMatch
}

// Search queries an existing derived SQLite index without refreshing it.
// It refuses to create a missing database so indexed search cannot silently
// return an empty result set merely because cxq index has not been run.
func Search(ctx context.Context, home string, options SearchOptions) (SearchResult, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return SearchResult{}, errors.New("codex home must not be empty")
	}

	databasePath := strings.TrimSpace(options.DatabasePath)
	if databasePath == "" {
		databasePath = DefaultIndexPath(home)
	}

	info, err := os.Stat(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return SearchResult{DatabasePath: databasePath}, fmt.Errorf("derived index not found at %q; run cxq index first", databasePath)
	}
	if err != nil {
		return SearchResult{DatabasePath: databasePath}, fmt.Errorf("stat derived index %q: %w", databasePath, err)
	}
	if info.IsDir() {
		return SearchResult{DatabasePath: databasePath}, fmt.Errorf("derived index path %q is a directory", databasePath)
	}

	store, err := index.OpenSQLite(databasePath)
	if err != nil {
		return SearchResult{DatabasePath: databasePath}, fmt.Errorf("open derived index: %w", err)
	}

	matches, searchErr := store.Search(ctx, index.SearchOptions{
		Query:   options.Query,
		Limit:   options.Limit,
		Project: options.Project,
		Source:  options.Source,
	})
	closeErr := store.Close()
	result := SearchResult{DatabasePath: databasePath, Matches: matches}

	switch {
	case searchErr != nil && closeErr != nil:
		return result, errors.Join(searchErr, fmt.Errorf("close derived index: %w", closeErr))
	case searchErr != nil:
		return result, searchErr
	case closeErr != nil:
		return result, fmt.Errorf("close derived index: %w", closeErr)
	default:
		return result, nil
	}
}
