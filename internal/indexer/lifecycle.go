package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luojiyin1987/codex-recall/internal/index"
)

const (
	defaultIndexDir  = ".codex-recall"
	defaultIndexFile = "index.db"
)

type RefreshOptions struct {
	DatabasePath string
	FullHash     bool
}

type RefreshResult struct {
	DatabasePath string
	Result
	Profile RefreshProfile
}

// RefreshProfile records lifecycle work around the index build.
type RefreshProfile struct {
	Prepare       time.Duration
	DatabaseOpen  time.Duration
	Build         BuildProfile
	DatabaseClose time.Duration
	Total         time.Duration
}

// DefaultIndexPath returns the derived SQLite index path for a Codex home.
// Keeping the index under home naturally isolates custom CODEX_HOME trees.
func DefaultIndexPath(home string) string {
	return filepath.Join(home, defaultIndexDir, defaultIndexFile)
}

// Refresh opens the derived SQLite index, incrementally rebuilds it from Codex
// rollouts, and closes it before returning.
func Refresh(ctx context.Context, home string, options RefreshOptions) (RefreshResult, error) {
	totalStart := time.Now()
	home = strings.TrimSpace(home)
	if home == "" {
		return RefreshResult{}, errors.New("codex home must not be empty")
	}

	databasePath := strings.TrimSpace(options.DatabasePath)
	usingDefaultPath := databasePath == ""
	if usingDefaultPath {
		databasePath = DefaultIndexPath(home)
	}

	prepareStart := time.Now()
	prepareErr := prepareIndexParent(databasePath, usingDefaultPath)
	prepareDuration := time.Since(prepareStart)
	if prepareErr != nil {
		return RefreshResult{DatabasePath: databasePath, Profile: RefreshProfile{Prepare: prepareDuration, Total: time.Since(totalStart)}}, prepareErr
	}

	openStart := time.Now()
	store, err := index.OpenSQLite(databasePath)
	openDuration := time.Since(openStart)
	if err != nil {
		return RefreshResult{DatabasePath: databasePath, Profile: RefreshProfile{Prepare: prepareDuration, DatabaseOpen: openDuration, Total: time.Since(totalStart)}}, fmt.Errorf("open derived index: %w", err)
	}

	buildResult, buildErr := BuildWithOptions(ctx, home, store, BuildOptions{FullHash: options.FullHash})
	closeStart := time.Now()
	closeErr := store.Close()
	closeDuration := time.Since(closeStart)
	result := RefreshResult{
		DatabasePath: databasePath,
		Result:       buildResult,
		Profile: RefreshProfile{
			Prepare:       prepareDuration,
			DatabaseOpen:  openDuration,
			Build:         buildResult.Profile,
			DatabaseClose: closeDuration,
			Total:         time.Since(totalStart),
		},
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
