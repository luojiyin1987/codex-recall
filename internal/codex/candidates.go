package codex

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type lookPathFunc func(string) (string, error)
type commandFunc func(string, ...string) ([]byte, error)

// SearchCandidateFiles returns rollout files that may contain query. When
// ripgrep is available it is used as a fast prefilter; otherwise the complete
// discovered file set is returned so SearchFile preserves the same semantics.
func SearchCandidateFiles(home, query string) ([]string, error) {
	return searchCandidateFiles(home, query, exec.LookPath, runCommand)
}

func searchCandidateFiles(home, query string, lookPath lookPathFunc, run commandFunc) ([]string, error) {
	allFiles, err := DiscoverFiles(home)
	if err != nil {
		return nil, err
	}
	if len(allFiles) == 0 {
		return allFiles, nil
	}

	rgPath, err := lookPath("rg")
	if err != nil {
		return allFiles, nil
	}

	roots := existingRolloutRoots(home)
	if len(roots) == 0 {
		return allFiles, nil
	}

	output, err := run(rgPath, ripgrepArgs(roots, query)...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		return allFiles, nil
	}

	return parseRipgrepPaths(output), nil
}

func runCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func existingRolloutRoots(home string) []string {
	roots := make([]string, 0, len(rolloutRoots))
	for _, name := range rolloutRoots {
		root := filepath.Join(home, name)
		info, err := os.Stat(root)
		if err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	return roots
}

func ripgrepArgs(roots []string, query string) []string {
	args := []string{
		"-l",
		"-0",
		"--fixed-strings",
		"--ignore-case",
		"--no-ignore",
		"--glob", "*.jsonl",
		"--",
		jsonEscapedSearchTerm(query),
	}
	return append(args, roots...)
}

func jsonEscapedSearchTerm(query string) string {
	encoded, _ := json.Marshal(query)
	if len(encoded) < 2 {
		return query
	}
	return string(encoded[1 : len(encoded)-1])
}

func parseRipgrepPaths(output []byte) []string {
	parts := strings.Split(string(output), "\x00")
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			paths = append(paths, filepath.Clean(part))
		}
	}
	sort.Strings(paths)
	return paths
}
