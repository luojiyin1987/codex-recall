package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type tokenizerExperimentStats struct {
	Tokenizer   string
	Hits        int
	Reciprocal  float64
	Misses      []string
	DatabaseBytes int64
	BuildTime   time.Duration
	SearchTime  time.Duration
}

func TestTrigramRetrievalExperiment(t *testing.T) {
	corpus := loadRetrievalEvaluationCorpus(t)

	unicode := runTokenizerExperiment(t, corpus, "unicode61")
	trigram := runTokenizerExperiment(t, corpus, "trigram")

	logTokenizerExperiment(t, unicode, len(corpus.Queries))
	logTokenizerExperiment(t, trigram, len(corpus.Queries))

	if unicode.Hits < corpus.MinimumIndexHitAt5 {
		t.Fatalf("unicode61 Hit@5 = %d/%d, baseline floor is %d",
			unicode.Hits, len(corpus.Queries), corpus.MinimumIndexHitAt5)
	}
	if trigram.Hits < unicode.Hits {
		t.Fatalf("trigram Hit@5 = %d/%d, below unicode61 %d/%d",
			trigram.Hits, len(corpus.Queries), unicode.Hits, len(corpus.Queries))
	}
}

func BenchmarkRetrievalTokenizerSearch(b *testing.B) {
	corpus := loadRetrievalEvaluationCorpus(b)
	for _, tokenizer := range []string{"unicode61", "trigram"} {
		b.Run(tokenizer, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), tokenizer+".db")
			db, _, err := buildTokenizerExperimentDB(path, corpus, tokenizer)
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()

			b.ReportAllocs()
			b.ReportMetric(float64(len(corpus.Queries)), "corpus-queries")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				query := corpus.Queries[i%len(corpus.Queries)]
				if _, err := searchTokenizerExperiment(
					context.Background(), db, query.Query, corpus.Limit,
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRetrievalTokenizerBuild(b *testing.B) {
	corpus := loadRetrievalEvaluationCorpus(b)
	for _, tokenizer := range []string{"unicode61", "trigram"} {
		b.Run(tokenizer, func(b *testing.B) {
			dir := b.TempDir()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				path := filepath.Join(dir, fmt.Sprintf("%s-%08d.db", tokenizer, i))
				db, _, err := buildTokenizerExperimentDB(path, corpus, tokenizer)
				if err != nil {
					b.Fatal(err)
				}
				if err := db.Close(); err != nil {
					b.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func runTokenizerExperiment(t *testing.T, corpus retrievalEvaluationCorpus, tokenizer string) tokenizerExperimentStats {
	t.Helper()

	path := filepath.Join(t.TempDir(), tokenizer+".db")
	db, buildTime, err := buildTokenizerExperimentDB(path, corpus, tokenizer)
	if err != nil {
		t.Fatalf("build %s experiment index: %v", tokenizer, err)
	}
	defer db.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s experiment index: %v", tokenizer, err)
	}

	stats := tokenizerExperimentStats{
		Tokenizer:     tokenizer,
		DatabaseBytes: info.Size(),
		BuildTime:     buildTime,
	}

	searchStarted := time.Now()
	for _, query := range corpus.Queries {
		sessionIDs, err := searchTokenizerExperiment(
			context.Background(), db, query.Query, corpus.Limit,
		)
		if err != nil {
			t.Fatalf("%s search %q: %v", tokenizer, query.Name, err)
		}
		rank := rankSessionID(sessionIDs, query.RelevantSessionID)
		if rank > 0 {
			stats.Hits++
			stats.Reciprocal += 1 / float64(rank)
			continue
		}
		stats.Misses = append(stats.Misses,
			fmt.Sprintf("%s[%s]: %q", query.Name, query.Category, query.Query))
	}
	stats.SearchTime = time.Since(searchStarted)
	return stats
}

func buildTokenizerExperimentDB(
	path string,
	corpus retrievalEvaluationCorpus,
	tokenizer string,
) (*sql.DB, time.Duration, error) {
	tokenizerSQL, err := tokenizerExperimentSQL(tokenizer)
	if err != nil {
		return nil, 0, err
	}

	started := time.Now()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, 0, fmt.Errorf("open experiment database: %w", err)
	}
	db.SetMaxOpenConns(1)

	closeOnError := func(err error) (*sql.DB, time.Duration, error) {
		_ = db.Close()
		return nil, 0, err
	}

	create := fmt.Sprintf(`CREATE VIRTUAL TABLE messages_fts USING fts5(
    session_id UNINDEXED,
    ordinal UNINDEXED,
    text,
    tokenize = '%s'
)`, tokenizerSQL)
	if _, err := db.Exec(create); err != nil {
		return closeOnError(fmt.Errorf("create %s FTS table: %w", tokenizer, err))
	}

	tx, err := db.Begin()
	if err != nil {
		return closeOnError(fmt.Errorf("begin %s experiment insert: %w", tokenizer, err))
	}
	stmt, err := tx.Prepare(`INSERT INTO messages_fts (session_id, ordinal, text) VALUES (?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return closeOnError(fmt.Errorf("prepare %s experiment insert: %w", tokenizer, err))
	}

	for _, session := range corpus.Sessions {
		for ordinal, text := range []string{session.User, session.Assistant} {
			if _, err := stmt.Exec(session.ID, ordinal, text); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return closeOnError(fmt.Errorf("insert %s experiment message: %w", tokenizer, err))
			}
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return closeOnError(fmt.Errorf("close %s experiment insert: %w", tokenizer, err))
	}
	if err := tx.Commit(); err != nil {
		return closeOnError(fmt.Errorf("commit %s experiment insert: %w", tokenizer, err))
	}
	return db, time.Since(started), nil
}

func tokenizerExperimentSQL(tokenizer string) (string, error) {
	switch tokenizer {
	case "unicode61", "trigram":
		return tokenizer, nil
	default:
		return "", fmt.Errorf("unsupported experiment tokenizer %q", tokenizer)
	}
}

func searchTokenizerExperiment(
	ctx context.Context,
	db *sql.DB,
	query string,
	limit int,
) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("experiment query must not be blank")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("experiment limit must be greater than zero")
	}

	rows, err := db.QueryContext(ctx, `
SELECT session_id
FROM messages_fts
WHERE messages_fts MATCH ?
ORDER BY bm25(messages_fts) ASC, CAST(ordinal AS INTEGER) ASC
`, quoteTokenizerExperimentLiteral(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessionIDs := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, err
		}
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		sessionIDs = append(sessionIDs, sessionID)
		if len(sessionIDs) == limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessionIDs, nil
}

func quoteTokenizerExperimentLiteral(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

func rankSessionID(sessionIDs []string, relevant string) int {
	for i, sessionID := range sessionIDs {
		if sessionID == relevant {
			return i + 1
		}
	}
	return 0
}

func logTokenizerExperiment(t *testing.T, stats tokenizerExperimentStats, cases int) {
	t.Helper()
	averageSearch := time.Duration(0)
	if cases > 0 {
		averageSearch = stats.SearchTime / time.Duration(cases)
	}
	t.Logf("%s: Hit@5=%d/%d MRR=%.3f db_bytes=%d build=%s search_total=%s search_avg=%s",
		stats.Tokenizer,
		stats.Hits,
		cases,
		stats.Reciprocal/float64(cases),
		stats.DatabaseBytes,
		stats.BuildTime,
		stats.SearchTime,
		averageSearch,
	)
	if len(stats.Misses) == 0 {
		t.Logf("%s misses: none", stats.Tokenizer)
		return
	}
	t.Logf("%s misses (%d): %v", stats.Tokenizer, len(stats.Misses), stats.Misses)
}

func loadRetrievalEvaluationCorpus(tb testing.TB) retrievalEvaluationCorpus {
	tb.Helper()
	var corpus retrievalEvaluationCorpus
	if err := json.Unmarshal(retrievalEvaluationData, &corpus); err != nil {
		tb.Fatalf("parse retrieval evaluation corpus: %v", err)
	}
	return corpus
}
