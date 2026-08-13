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

// SearchCandidateFiles returns rollout files that may contain query. When
// ripgrep is available it is used as a fast prefilter; otherwise the complete
// discovered file set is returned so SearchFile preserves the same semantics.
func SearchCandidateFiles(home, query string) ([]string, error) {
	return searchCandidateFiles(home, query, exec.LookPath, runCommand)
}

// RankCandidateFiles filters candidates and puts newer sessions first.
// Files with unreadable metadata remain last to preserve search errors.
func RankCandidateFiles(paths []string, include func(Session) bool) []string {
	type rankedCandidate struct {
		path    string
		session Session
	}

	ranked := make([]rankedCandidate, 0, len(paths))
	unreadable := make([]string, 0)
	for _, path := range paths {
		session, err := ParseFile(path)
		if err != nil {
			unreadable = append(unreadable, path)
			continue
		}
		if include == nil || include(session) {
			ranked = append(ranked, rankedCandidate{path: path, session: session})
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].session.Timestamp.After(ranked[j].session.Timestamp)
	})
	result := make([]string, 0, len(ranked)+len(unreadable))
	for _, candidate := range ranked {
		result = append(result, candidate.path)
	}
	return append(result, unreadable...)
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
		"--ignore-case",
		"--no-ignore",
		"--glob", "*.jsonl",
		"--",
		conversationSearchPattern(query),
	}
	return append(args, roots...)
}

func conversationSearchPattern(query string) string {
	term := regexp.QuoteMeta(jsonEscapedSearchTerm(query))
	response := `"type"\s*:\s*"response_item"\s*,\s*"payload"\s*:\s*\{\s*"type"\s*:\s*"message".*"role"\s*:\s*"(user|assistant)".*` + term
	event := `"type"\s*:\s*"event_msg"\s*,\s*"payload"\s*:\s*\{\s*"type"\s*:\s*"(user_message|agent_message)".*"message"\s*:.*` + term
	return "(" + response + "|" + event + ")"
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
