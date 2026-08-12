package codex

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const codexHomeEnv = "CODEX_HOME"

var rolloutRoots = []string{"sessions", "archived_sessions"}

// ResolveHome returns CODEX_HOME when set, otherwise ~/.codex.
func ResolveHome() (string, error) {
	if home := os.Getenv(codexHomeEnv); home != "" {
		return filepath.Clean(home), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// DiscoverFiles returns local Codex rollout JSONL files below the active and
// archived session roots. Results are sorted to make CLI output and tests
// deterministic.
func DiscoverFiles(home string) ([]string, error) {
	var files []string
	for _, name := range rolloutRoots {
		root := filepath.Join(home, name)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(entry.Name()) == ".jsonl" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}
