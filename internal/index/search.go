package index

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/luojiyin1987/codex-recall/internal/textutil"
)

const (
	lexicalWhy         = "lexical:fts5"
	substringWhy       = "lexical:substring"
	trigramMinimumRunes = 3
)

// Search returns at most one lexical match per session. Queries with at least
// three Unicode code points use the trigram FTS5 index. Shorter queries scan
// the derived messages table with a case-insensitive literal substring match,
// because FTS5 trigram MATCH cannot produce tokens shorter than three code
// points. Both paths remain derived-index only.
func (s *SQLiteIndex) Search(ctx context.Context, options SearchOptions) ([]SearchMatch, error) {
	matches, _, err := s.SearchWithProfile(ctx, options)
	return matches, err
}

// SearchWithProfile runs the same indexed retrieval path as Search while also
// returning lightweight phase timings and row/session counts.
func (s *SQLiteIndex) SearchWithProfile(ctx context.Context, options SearchOptions) ([]SearchMatch, SearchProfile, error) {
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return nil, SearchProfile{}, errors.New("search query must not be blank")
	}
	if options.Limit <= 0 {
		return nil, SearchProfile{}, errors.New("search limit must be greater than zero")
	}

	start := time.Now()
	var (
		matches []SearchMatch
		profile SearchProfile
		err     error
	)
	if utf8.RuneCountInString(query) < trigramMinimumRunes {
		matches, profile, err = s.searchShortLiteralProfiled(ctx, query, options)
	} else {
		matches, profile, err = s.searchFTSProfiled(ctx, query, options)
	}
	profile.SearchTotal = time.Since(start)
	profile.SessionsReturned = len(matches)
	return matches, profile, err
}

func (s *SQLiteIndex) searchFTSProfiled(ctx context.Context, query string, options SearchOptions) ([]SearchMatch, SearchProfile, error) {
	profile := SearchProfile{Backend: "fts5"}
	var filters []string
	var args []any
	args = append(args, quoteFTSLiteral(query))

	if project := strings.TrimSpace(options.Project); project != "" {
		filters = append(filters, "s.project = ? COLLATE NOCASE")
		args = append(args, project)
	}
	if source := strings.TrimSpace(options.Source); source != "" {
		filters = append(filters, "s.source = ? COLLATE NOCASE")
		args = append(args, source)
	}

	where := "messages_fts MATCH ?"
	if len(filters) > 0 {
		where += " AND " + strings.Join(filters, " AND ")
	}

	queryStart := time.Now()
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT
    s.session_id,
    s.timestamp,
    s.cwd,
    s.project,
    s.source,
    s.rollout_path,
    s.content_hash,
    CAST(f.ordinal AS INTEGER),
    m.role,
    m.text,
    bm25(messages_fts)
FROM messages_fts AS f
JOIN sessions AS s ON s.session_id = f.session_id
JOIN messages AS m
  ON m.session_id = f.session_id
 AND m.ordinal = CAST(f.ordinal AS INTEGER)
WHERE %s
ORDER BY bm25(messages_fts) ASC, s.timestamp DESC, CAST(f.ordinal AS INTEGER) ASC
`, where), args...)
	profile.QuerySetup = time.Since(queryStart)
	if err != nil {
		return nil, profile, fmt.Errorf("search sqlite lexical index: %w", err)
	}
	defer rows.Close()

	matches := make([]SearchMatch, 0, options.Limit)
	seenSessions := make(map[string]struct{}, options.Limit)
	scanStart := time.Now()
	var snippetDuration time.Duration
	for rows.Next() {
		profile.RowsScanned++

		var match SearchMatch
		var timestamp string
		var text string
		if err := rows.Scan(
			&match.Session.ID,
			&timestamp,
			&match.Session.CWD,
			&match.Session.Project,
			&match.Session.Source,
			&match.Session.RolloutPath,
			&match.Session.ContentHash,
			&match.Ordinal,
			&match.Role,
			&text,
			&match.Score,
		); err != nil {
			return nil, profile, fmt.Errorf("scan sqlite lexical match: %w", err)
		}
		if _, seen := seenSessions[match.Session.ID]; seen {
			continue
		}

		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, profile, fmt.Errorf("parse indexed session timestamp %q: %w", timestamp, err)
		}
		match.Session.Timestamp = parsed
		snippetStart := time.Now()
		match.Snippet = literalSnippet(text, query)
		snippetDuration += time.Since(snippetStart)
		match.Why = lexicalWhy
		seenSessions[match.Session.ID] = struct{}{}
		matches = append(matches, match)
		if len(matches) == options.Limit {
			break
		}
	}
	scanDuration := time.Since(scanStart)
	profile.Snippet = snippetDuration
	if scanDuration > snippetDuration {
		profile.ResultScan = scanDuration - snippetDuration
	}
	if err := rows.Err(); err != nil {
		return nil, profile, fmt.Errorf("iterate sqlite lexical matches: %w", err)
	}
	return matches, profile, nil
}

func (s *SQLiteIndex) searchShortLiteralProfiled(ctx context.Context, query string, options SearchOptions) ([]SearchMatch, SearchProfile, error) {
	profile := SearchProfile{Backend: "substring"}
	var filters []string
	var args []any
	if project := strings.TrimSpace(options.Project); project != "" {
		filters = append(filters, "s.project = ? COLLATE NOCASE")
		args = append(args, project)
	}
	if source := strings.TrimSpace(options.Source); source != "" {
		filters = append(filters, "s.source = ? COLLATE NOCASE")
		args = append(args, source)
	}

	where := ""
	if len(filters) > 0 {
		where = "WHERE " + strings.Join(filters, " AND ")
	}

	queryStart := time.Now()
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT
    s.session_id,
    s.timestamp,
    s.cwd,
    s.project,
    s.source,
    s.rollout_path,
    s.content_hash,
    m.ordinal,
    m.role,
    m.text
FROM messages AS m
JOIN sessions AS s ON s.session_id = m.session_id
%s
ORDER BY s.timestamp DESC, m.ordinal ASC
`, where), args...)
	profile.QuerySetup = time.Since(queryStart)
	if err != nil {
		return nil, profile, fmt.Errorf("search indexed messages for short literal: %w", err)
	}
	defer rows.Close()

	needle := strings.ToLower(query)
	matches := make([]SearchMatch, 0, options.Limit)
	seenSessions := make(map[string]struct{}, options.Limit)
	scanStart := time.Now()
	var snippetDuration time.Duration
	for rows.Next() {
		profile.RowsScanned++
		if err := ctx.Err(); err != nil {
			return nil, profile, err
		}

		var match SearchMatch
		var timestamp string
		var text string
		if err := rows.Scan(
			&match.Session.ID,
			&timestamp,
			&match.Session.CWD,
			&match.Session.Project,
			&match.Session.Source,
			&match.Session.RolloutPath,
			&match.Session.ContentHash,
			&match.Ordinal,
			&match.Role,
			&text,
		); err != nil {
			return nil, profile, fmt.Errorf("scan indexed short-literal candidate: %w", err)
		}
		if _, seen := seenSessions[match.Session.ID]; seen {
			continue
		}
		if !strings.Contains(strings.ToLower(text), needle) {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, profile, fmt.Errorf("parse indexed session timestamp %q: %w", timestamp, err)
		}
		match.Session.Timestamp = parsed
		snippetStart := time.Now()
		match.Snippet = literalSnippet(text, query)
		snippetDuration += time.Since(snippetStart)
		match.Score = 0
		match.Why = substringWhy
		seenSessions[match.Session.ID] = struct{}{}
		matches = append(matches, match)
		if len(matches) == options.Limit {
			break
		}
	}
	scanDuration := time.Since(scanStart)
	profile.Snippet = snippetDuration
	if scanDuration > snippetDuration {
		profile.ResultScan = scanDuration - snippetDuration
	}
	if err := rows.Err(); err != nil {
		return nil, profile, fmt.Errorf("iterate indexed short-literal candidates: %w", err)
	}
	return matches, profile, nil
}

func literalSnippet(text, query string) string {
	normalized := textutil.NormalizeWhitespace(text)
	lower := strings.ToLower(normalized)
	byteOffset := strings.Index(lower, strings.ToLower(query))
	if byteOffset < 0 {
		return normalized
	}

	runes := []rune(normalized)
	startRune := utf8.RuneCountInString(lower[:byteOffset])
	queryRunes := utf8.RuneCountInString(query)
	start := startRune - 24
	if start < 0 {
		start = 0
	}
	end := startRune + queryRunes + 24
	if end > len(runes) {
		end = len(runes)
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "… " + snippet
	}
	if end < len(runes) {
		snippet += " …"
	}
	return snippet
}

// quoteFTSLiteral treats the user's input as one FTS phrase instead of exposing
// FTS5 operators such as AND, OR, NOT, column filters, or prefix syntax.
func quoteFTSLiteral(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}
