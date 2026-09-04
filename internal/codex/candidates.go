package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type lookPathFunc func(string) (string, error)
type commandFunc func(string, ...string) ([]byte, error)
type discoverFilesFunc func(string) ([]string, error)
type contextCommandFunc func(context.Context, string, ...string) ([]byte, error)
type contextDiscoverFilesFunc func(context.Context, string) ([]string, error)

// SearchCandidate keeps a path with metadata parsed during candidate ranking.
type SearchCandidate struct {
	Path       string
	session    Session
	hasSession bool
}

func searchCandidateFiles(home, query string, lookPath lookPathFunc, run commandFunc) ([]string, error) {
	return searchCandidateFilesWithDiscovery(home, query, lookPath, run, DiscoverFiles)
}

func searchCandidateFilesContext(ctx context.Context, home, query string) ([]string, error) {
	return searchCandidateFilesWithContextDependencies(ctx, home, query, exec.LookPath, runCommandContext, DiscoverFilesContext)
}

func searchCandidateFilesWithContextDependencies(
	ctx context.Context,
	home, query string,
	lookPath lookPathFunc,
	run contextCommandFunc,
	discover contextDiscoverFilesFunc,
) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rgPath, err := lookPath("rg")
	roots := existingRolloutRoots(home)
	if err == nil && len(roots) > 0 {
		output, runErr := run(ctx, rgPath, ripgrepArgs(roots, query)...)
		if runErr == nil {
			return parseRipgrepPaths(output), nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
	}
	return discover(ctx, home)
}

func searchCandidateFilesWithDiscovery(home, query string, lookPath lookPathFunc, run commandFunc, discover discoverFilesFunc) ([]string, error) {
	rgPath, err := lookPath("rg")
	roots := existingRolloutRoots(home)
	if err == nil && len(roots) > 0 {
		output, runErr := run(rgPath, ripgrepArgs(roots, query)...)
		if runErr == nil {
			return parseRipgrepPaths(output), nil
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
	}

	return discover(home)
}

func runCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func runCommandContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
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
		"--pcre2",
		"--ignore-case",
		"--no-ignore",
		"--glob", "*.jsonl",
		"--",
		conversationCandidatePattern(query),
	}
	return append(args, roots...)
}

func conversationCandidatePattern(query string) string {
	term := regexp.QuoteMeta(jsonEscapedSearchTerm(query))
	response := `(?=.*"type"\s*:\s*"response_item")(?=.*"type"\s*:\s*"message")(?=.*"role"\s*:\s*"(?:user|assistant)")`
	event := `(?=.*"type"\s*:\s*"event_msg")(?=.*"type"\s*:\s*"(?:user_message|agent_message)")`
	return `^(?=.*` + term + `)(?:` + response + `|` + event + `).*$`
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
