package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/luojiyin1987/codex-recall/internal/textutil"
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
	return SearchContext(context.Background(), home, options)
}

// SearchContext is Search with cooperative cancellation.
func SearchContext(ctx context.Context, home string, options SearchOptions) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if options.Limit <= 0 {
		return SearchResult{}, errors.New("search limit must be greater than zero")
	}
	if strings.TrimSpace(options.Query) == "" {
		return SearchResult{}, errors.New("search query must not be empty")
	}

	paths, err := searchCandidateFilesContext(ctx, home, options.Query)
	if err != nil {
		return SearchResult{}, fmt.Errorf("discover search candidates: %w", err)
	}
	var include func(Session) bool
	if strings.TrimSpace(options.Project) != "" || strings.TrimSpace(options.Source) != "" {
		include = func(session Session) bool {
			return matchesSessionFilters(session, options.Project, options.Source)
		}
	}
	candidates, err := NewCatalog(home).rankCandidatesContext(ctx, paths, include)
	if err != nil {
		return SearchResult{}, err
	}
	matches, warnings, err := searchFilesContext(ctx, candidates, options.Query, options.Limit)
	if err != nil {
		return SearchResult{Matches: matches, Warnings: warnings}, err
	}
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
	matches, warnings, err := searchFilesContext(context.Background(), candidates, query, limit)
	if err != nil {
		warnings = append(warnings, err)
	}
	return matches, warnings
}

func searchFilesContext(ctx context.Context, candidates []SearchCandidate, query string, limit int) ([]SearchMatch, []error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if limit <= 0 {
		return nil, nil, errors.New("search limit must be greater than zero")
	}
	matcher, err := compileSearchMatcher(query)
	if err != nil {
		return nil, nil, err
	}

	capacity := min(limit, len(candidates))
	matches := make([]SearchMatch, 0, capacity)
	var searchErrors []error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return matches, searchErrors, err
		}
		var session *Session
		if candidate.hasSession {
			session = &candidate.session
		}
		match, ok, err := searchFileContext(ctx, candidate.Path, matcher, session)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return matches, searchErrors, ctxErr
			}
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
	if err := ctx.Err(); err != nil {
		return matches, searchErrors, err
	}
	return matches, searchErrors, nil
}

// SearchFile returns the first user/assistant conversation match in a rollout.
// Tool output, reasoning, and metadata are intentionally excluded.
func SearchFile(path, query string) (SearchMatch, bool, error) {
	matcher, err := compileSearchMatcher(query)
	if err != nil {
		return SearchMatch{}, false, err
	}
	return searchFileContext(context.Background(), path, matcher, nil)
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
	return searchFileContext(context.Background(), path, matcher, session)
}

func searchFileContext(ctx context.Context, path string, matcher *regexp.Regexp, session *Session) (SearchMatch, bool, error) {
	var match SearchMatch
	found := false
	err := visitRolloutFileContext(ctx, path, func(rec record) (bool, error) {
		role, text := conversationText(rec)
		if text == "" {
			return false, nil
		}
		normalized := textutil.NormalizeWhitespace(text)
		loc := matcher.FindStringIndex(normalized)
		if loc == nil {
			return false, nil
		}
		if session == nil {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			parsed, metaErr := ParseFile(path)
			if metaErr != nil {
				return false, metaErr
			}
			session = &parsed
		}
		match = SearchMatch{Session: *session, Role: role, Snippet: excerptAroundMatch(normalized, loc[0], loc[1])}
		found = true
		return true, nil
	})
	if err != nil {
		return SearchMatch{}, false, err
	}
	return match, found, nil
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
