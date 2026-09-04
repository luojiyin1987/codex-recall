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
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return nil, errors.New("search query must not be blank")
	}
	if options.Limit <= 0 {
		return nil, errors.New("search limit must be greater than zero")
	}
	if utf8.RuneCountInString(query) < trigramMinimumRunes {
		return s.searchShortLiteral(ctx, query, options)
	}
	return s.searchFTS(ctx, query, options)
}

func (s *SQLiteIndex) searchFTS(ctx context.Context, query string, options SearchOptions) ([]SearchMatch, error) {
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
	if err != nil {
		return nil, fmt.Errorf("search sqlite lexical index: %w", err)
	}
	defer rows.Close()

	matches := make([]SearchMatch, 0, options.Limit)
	seenSessions := make(map[string]struct{}, options.Limit)
	for rows.Next() {
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
			return nil, fmt.Errorf("scan sqlite lexical match: %w", err)
		}
		if _, seen := seenSessions[match.Session.ID]; seen {
			continue
		}

		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse indexed session timestamp %q: %w", timestamp, err)
		}
		match.Session.Timestamp = parsed
		match.Snippet = literalSnippet(text, query)
		match.Why = lexicalWhy
		seenSessions[match.Session.ID] = struct{}{}
		matches = append(matches, match)
		if len(matches) == options.Limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite lexical matches: %w", err)
	}
	return matches, nil
}

func (s *SQLiteIndex) searchShortLiteral(ctx context.Context, query string, options SearchOptions) ([]SearchMatch, error) {
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
	if err != nil {
		return nil, fmt.Errorf("search indexed messages for short literal: %w", err)
	}
	defer rows.Close()

	needle := strings.ToLower(query)
	matches := make([]SearchMatch, 0, options.Limit)
	seenSessions := make(map[string]struct{}, options.Limit)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
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
			return nil, fmt.Errorf("scan indexed short-literal candidate: %w", err)
		}
		if _, seen := seenSessions[match.Session.ID]; seen {
			continue
		}
		if !strings.Contains(strings.ToLower(text), needle) {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse indexed session timestamp %q: %w", timestamp, err)
		}
		match.Session.Timestamp = parsed
		match.Snippet = literalSnippet(text, query)
		match.Score = 0
		match.Why = substringWhy
		seenSessions[match.Session.ID] = struct{}{}
		matches = append(matches, match)
		if len(matches) == options.Limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed short-literal candidates: %w", err)
	}
	return matches, nil
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
