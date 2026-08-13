package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	matchContextBeforeRunes = 48
	matchContextAfterRunes  = 96
)

type SearchMatch struct {
	Session Session
	Role    string
	Snippet string
}

type SearchOptions struct {
	Query   string
	Limit   int
	Project string
	Source  string
}

type SearchResult struct {
	Matches  []SearchMatch
	Warnings []error
}

type responseMessage struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type eventMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Search finds the newest conversation matches that satisfy the options.
func Search(home string, options SearchOptions) (SearchResult, error) {
	if options.Limit <= 0 {
		return SearchResult{}, errors.New("search limit must be greater than zero")
	}
	if strings.TrimSpace(options.Query) == "" {
		return SearchResult{}, errors.New("search query must not be empty")
	}

	paths, err := searchCandidateFiles(home, options.Query, exec.LookPath, runCommand)
	if err != nil {
		return SearchResult{}, fmt.Errorf("discover search candidates: %w", err)
	}
	var include func(Session) bool
	if strings.TrimSpace(options.Project) != "" || strings.TrimSpace(options.Source) != "" {
		include = func(session Session) bool {
			return matchesSessionFilters(session, options.Project, options.Source)
		}
	}
	candidates := rankCandidateFiles(paths, include)
	matches, warnings := searchFiles(candidates, options.Query, options.Limit)
	return SearchResult{Matches: matches, Warnings: warnings}, nil
}

func matchesSessionFilters(session Session, project, source string) bool {
	project = strings.TrimSpace(project)
	source = strings.TrimSpace(source)
	if project != "" && !strings.EqualFold(session.Project(), project) {
		return false
	}
	if source != "" && !strings.EqualFold(session.Source, source) {
		return false
	}
	return true
}

// searchFiles scans ordered candidates until it reaches limit.
func searchFiles(candidates []SearchCandidate, query string, limit int) ([]SearchMatch, []error) {
	if limit <= 0 {
		return nil, []error{errors.New("search limit must be greater than zero")}
	}
	matcher, err := compileSearchMatcher(query)
	if err != nil {
		return nil, []error{err}
	}

	capacity := min(limit, len(candidates))
	matches := make([]SearchMatch, 0, capacity)
	var searchErrors []error
	for _, candidate := range candidates {
		var session *Session
		if candidate.hasSession {
			session = &candidate.session
		}
		match, ok, err := searchFile(candidate.Path, matcher, session)
		if err != nil {
			searchErrors = append(searchErrors, err)
			continue
		}
		if !ok {
			continue
		}
		matches = append(matches, match)
		if len(matches) == limit {
			break
		}
	}
	return matches, searchErrors
}

// SearchFile returns the first user/assistant conversation match in a rollout.
// Tool output, reasoning, and metadata are intentionally excluded.
func SearchFile(path, query string) (SearchMatch, bool, error) {
	matcher, err := compileSearchMatcher(query)
	if err != nil {
		return SearchMatch{}, false, err
	}
	return searchFile(path, matcher, nil)
}

func compileSearchMatcher(query string) (*regexp.Regexp, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("search query must not be empty")
	}
	matcher, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return nil, fmt.Errorf("compile search query: %w", err)
	}
	return matcher, nil
}

func searchFile(path string, matcher *regexp.Regexp, session *Session) (SearchMatch, bool, error) {

	file, err := os.Open(path)
	if err != nil {
		return SearchMatch{}, false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			var rec record
			if json.Unmarshal(bytes.TrimSpace(line), &rec) == nil {
				role, text := conversationText(rec)
				if text != "" {
					normalized := normalizePreviewText(text)
					if loc := matcher.FindStringIndex(normalized); loc != nil {
						if session == nil {
							parsed, metaErr := ParseFile(path)
							if metaErr != nil {
								return SearchMatch{}, false, metaErr
							}
							session = &parsed
						}
						return SearchMatch{
							Session: *session,
							Role:    role,
							Snippet: excerptAroundMatch(normalized, loc[0], loc[1]),
						}, true, nil
					}
				}
			}
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return SearchMatch{}, false, readErr
		}
	}

	return SearchMatch{}, false, nil
}

func conversationText(rec record) (string, string) {
	switch rec.Type {
	case "response_item":
		var message responseMessage
		if err := json.Unmarshal(rec.Payload, &message); err != nil || message.Type != "message" {
			return "", ""
		}
		if message.Role != "user" && message.Role != "assistant" {
			return "", ""
		}
		parts := make([]string, 0, len(message.Content))
		for _, item := range message.Content {
			if (item.Type == "input_text" || item.Type == "output_text") && strings.TrimSpace(item.Text) != "" {
				parts = append(parts, item.Text)
			}
		}
		return message.Role, strings.Join(parts, " ")

	case "event_msg":
		var event eventMessage
		if err := json.Unmarshal(rec.Payload, &event); err != nil || strings.TrimSpace(event.Message) == "" {
			return "", ""
		}
		switch event.Type {
		case "user_message":
			return "user", event.Message
		case "agent_message":
			return "assistant", event.Message
		default:
			return "", ""
		}
	default:
		return "", ""
	}
}

func normalizePreviewText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func excerptAroundMatch(text string, matchStart, matchEnd int) string {
	runes := []rune(text)
	startRune := utf8.RuneCountInString(text[:matchStart]) - matchContextBeforeRunes
	if startRune < 0 {
		startRune = 0
	}
	endRune := utf8.RuneCountInString(text[:matchEnd]) + matchContextAfterRunes
	if endRune > len(runes) {
		endRune = len(runes)
	}

	excerpt := strings.TrimSpace(string(runes[startRune:endRune]))
	if startRune > 0 {
		excerpt = "... " + excerpt
	}
	if endRune < len(runes) {
		excerpt += " ..."
	}
	return excerpt
}
