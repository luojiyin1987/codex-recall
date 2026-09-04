package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luojiyin1987/codex-recall/internal/index"
)

const (
	defaultIndexDir  = ".codex-recall"
	defaultIndexFile = "index.db"
)

type RefreshOptions struct {
	DatabasePath string
}

type RefreshResult struct {
	DatabasePath string
	Result
}

// DefaultIndexPath returns the derived SQLite index path for a Codex home.
// Keeping the index under home naturally isolates custom CODEX_HOME trees.
func DefaultIndexPath(home string) string {
	return filepath.Join(home, defaultIndexDir, defaultIndexFile)
}

// Refresh opens the derived SQLite index, incrementally rebuilds it from Codex
// rollouts, and closes it before returning.
func Refresh(ctx context.Context, home string, options RefreshOptions) (RefreshResult, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return RefreshResult{}, errors.New("Codex home must not be empty")
	}

	databasePath := strings.TrimSpace(options.DatabasePath)
	usingDefaultPath := databasePath == ""
	if usingDefaultPath {
		databasePath = DefaultIndexPath(home)
	}

	if err := prepareIndexParent(databasePath, usingDefaultPath); err != nil {
		return RefreshResult{DatabasePath: databasePath}, err
	}

	store, err := index.OpenSQLite(databasePath)
	if err != nil {
		return RefreshResult{DatabasePath: databasePath}, fmt.Errorf("open derived index: %w", err)
	}

	buildResult, buildErr := Build(ctx, home, store)
	closeErr := store.Close()
	result := RefreshResult{
		DatabasePath: databasePath,
		Result:       buildResult,
	}

	switch {
	case buildErr != nil && closeErr != nil:
		return result, errors.Join(buildErr, fmt.Errorf("close derived index: %w", closeErr))
	case buildErr != nil:
		return result, buildErr
	case closeErr != nil:
		return result, fmt.Errorf("close derived index: %w", closeErr)
	default:
		return result, nil
	}
}

func prepareIndexParent(databasePath string, privateDefault bool) error {
	parent := filepath.Dir(databasePath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create index directory %q: %w", parent, err)
	}
	if privateDefault {
		if err := os.Chmod(parent, 0o700); err != nil {
			return fmt.Errorf("protect index directory %q: %w", parent, err)
		}
	}
	return nil
}
