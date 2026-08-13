package codex

import (
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

// SearchCandidate keeps a path with metadata parsed during candidate ranking.
type SearchCandidate struct {
	Path       string
	session    Session
	hasSession bool
}

// SearchCandidateFiles returns rollout files that may contain query. When
// ripgrep is available it is used as a fast prefilter; otherwise the complete
// discovered file set is returned so SearchFile preserves the same semantics.
func SearchCandidateFiles(home, query string) ([]string, error) {
	return searchCandidateFiles(home, query, exec.LookPath, runCommand)
}

// RankCandidateFiles filters candidates and puts newer sessions first.
// Files with unreadable metadata remain last and can be skipped by the limit.
func RankCandidateFiles(paths []string, include func(Session) bool) []SearchCandidate {
	ranked := make([]SearchCandidate, 0, len(paths))
	unreadable := make([]SearchCandidate, 0)
	for _, path := range paths {
		session, err := ParseFile(path)
		if err != nil {
			unreadable = append(unreadable, SearchCandidate{Path: path})
			continue
		}
		if include == nil || include(session) {
			ranked = append(ranked, SearchCandidate{Path: path, session: session, hasSession: true})
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].session.Timestamp.After(ranked[j].session.Timestamp)
	})
	return append(ranked, unreadable...)
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
