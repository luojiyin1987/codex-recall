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

type SearchOptions struct {
	DatabasePath string
	Query        string
	Limit        int
	Project      string
	Source       string
}

type SearchProfile struct {
	DatabaseOpen time.Duration
	Index        index.SearchProfile
	Total        time.Duration
}

type SearchResult struct {
	DatabasePath string
	Matches      []index.SearchMatch
	Profile      SearchProfile
}

// Search queries an existing derived SQLite index without refreshing it.
// It refuses to create a missing database so indexed search cannot silently
// return an empty result set merely because cxq index has not been run.
func Search(ctx context.Context, home string, options SearchOptions) (SearchResult, error) {
	totalStart := time.Now()
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

	openStart := time.Now()
	store, err := index.OpenSQLite(databasePath)
	openDuration := time.Since(openStart)
	if err != nil {
		return SearchResult{
			DatabasePath: databasePath,
			Profile: SearchProfile{
				DatabaseOpen: openDuration,
				Total:        time.Since(totalStart),
			},
		}, fmt.Errorf("open derived index: %w", err)
	}

	matches, indexProfile, searchErr := store.SearchWithProfile(ctx, index.SearchOptions{
		Query:   options.Query,
		Limit:   options.Limit,
		Project: options.Project,
		Source:  options.Source,
	})
	closeErr := store.Close()
	result := SearchResult{
		DatabasePath: databasePath,
		Matches:      matches,
		Profile: SearchProfile{
			DatabaseOpen: openDuration,
			Index:        indexProfile,
			Total:        time.Since(totalStart),
		},
	}

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
