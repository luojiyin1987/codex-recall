package index

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const lexicalWhy = "lexical:fts5"

// Search returns at most one lexical match per session. FTS5 rows remain
// globally ordered by relevance, so the first row seen for a session is its
// highest-ranked representative message and Limit counts unique sessions.
func (s *SQLiteIndex) Search(ctx context.Context, options SearchOptions) ([]SearchMatch, error) {
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return nil, errors.New("search query must not be blank")
	}
	if options.Limit <= 0 {
		return nil, errors.New("search limit must be greater than zero")
	}

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
    f.role,
    snippet(messages_fts, 3, '', '', ' … ', 24),
    bm25(messages_fts)
FROM messages_fts AS f
JOIN sessions AS s ON s.session_id = f.session_id
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
			&match.Snippet,
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

// quoteFTSLiteral treats the user's input as one FTS phrase instead of exposing
// FTS5 operators such as AND, OR, NOT, column filters, or prefix syntax.
func quoteFTSLiteral(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}
