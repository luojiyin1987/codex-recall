package indexer

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/luojiyin1987/codex-recall/internal/codex"
	"github.com/luojiyin1987/codex-recall/internal/index"
)

//go:embed testdata/retrieval_eval.json
var retrievalEvaluationData []byte

type retrievalEvaluationCorpus struct {
	SchemaVersion      int                          `json:"schema_version"`
	Limit              int                          `json:"limit"`
	MinimumLiveHitAt5  int                          `json:"minimum_live_hit_at_5"`
	MinimumIndexHitAt5 int                          `json:"minimum_index_hit_at_5"`
	Sessions           []retrievalEvaluationSession `json:"sessions"`
	Queries            []retrievalEvaluationQuery   `json:"queries"`
}

type retrievalEvaluationSession struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Project   string `json:"project"`
	Source    string `json:"source"`
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

type retrievalEvaluationQuery struct {
	Name              string `json:"name"`
	Category          string `json:"category"`
	Query             string `json:"query"`
	RelevantSessionID string `json:"relevant_session_id"`
}

type retrievalEvaluationBackend struct {
	Name       string
	Hits       int
	Reciprocal float64
	Misses     []string
}

func TestRetrievalEvaluationCorpus(t *testing.T) {
	// Make live retrieval deterministic across developer machines and CI.
	// The evaluation measures the built-in scanner rather than optional rg
	// candidate filtering.
	t.Setenv("PATH", "")

	var corpus retrievalEvaluationCorpus
	if err := json.Unmarshal(retrievalEvaluationData, &corpus); err != nil {
		t.Fatalf("parse retrieval evaluation corpus: %v", err)
	}
	if corpus.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", corpus.SchemaVersion)
	}
	if corpus.Limit != 5 {
		t.Fatalf("limit = %d, want 5", corpus.Limit)
	}
	if len(corpus.Queries) == 0 || len(corpus.Sessions) == 0 {
		t.Fatal("retrieval evaluation corpus must contain sessions and queries")
	}

	knownSessions := make(map[string]struct{}, len(corpus.Sessions))
	home := t.TempDir()
	for _, session := range corpus.Sessions {
		if _, duplicate := knownSessions[session.ID]; duplicate {
			t.Fatalf("duplicate session id %q", session.ID)
		}
		knownSessions[session.ID] = struct{}{}
		writeRetrievalEvaluationRollout(t, home, session)
	}
	for _, query := range corpus.Queries {
		if _, ok := knownSessions[query.RelevantSessionID]; !ok {
			t.Fatalf("query %q references unknown session %q", query.Name, query.RelevantSessionID)
		}
	}

	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	live := retrievalEvaluationBackend{Name: "live"}
	indexed := retrievalEvaluationBackend{Name: "index"}

	for _, query := range corpus.Queries {
		liveResult, err := codex.SearchContext(context.Background(), home, codex.SearchOptions{
			Query: query.Query,
			Limit: corpus.Limit,
		})
		if err != nil {
			t.Fatalf("live search %q: %v", query.Name, err)
		}
		liveRank := rankLiveSession(liveResult.Matches, query.RelevantSessionID)
		recordRetrievalEvaluation(&live, query, liveRank)

		indexResult, err := Search(context.Background(), home, SearchOptions{
			Query: query.Query,
			Limit: corpus.Limit,
		})
		if err != nil {
			t.Fatalf("indexed search %q: %v", query.Name, err)
		}
		indexRank := rankIndexedSession(indexResult.Matches, query.RelevantSessionID)
		recordRetrievalEvaluation(&indexed, query, indexRank)

		t.Logf("%-30s category=%-18s live_rank=%d index_rank=%d query=%q",
			query.Name, query.Category, liveRank, indexRank, query.Query)
	}

	caseCount := float64(len(corpus.Queries))
	t.Logf("retrieval evaluation: cases=%d live_hit@5=%d index_hit@5=%d live_mrr=%.3f index_mrr=%.3f",
		len(corpus.Queries),
		live.Hits,
		indexed.Hits,
		live.Reciprocal/caseCount,
		indexed.Reciprocal/caseCount,
	)
	logRetrievalMisses(t, live)
	logRetrievalMisses(t, indexed)

	if live.Hits < corpus.MinimumLiveHitAt5 {
		t.Fatalf("live Hit@5 = %d/%d, baseline floor is %d; misses: %v",
			live.Hits, len(corpus.Queries), corpus.MinimumLiveHitAt5, live.Misses)
	}
	if indexed.Hits < corpus.MinimumIndexHitAt5 {
		t.Fatalf("indexed Hit@5 = %d/%d, baseline floor is %d; misses: %v",
			indexed.Hits, len(corpus.Queries), corpus.MinimumIndexHitAt5, indexed.Misses)
	}
}

func recordRetrievalEvaluation(backend *retrievalEvaluationBackend, query retrievalEvaluationQuery, rank int) {
	if rank > 0 {
		backend.Hits++
		backend.Reciprocal += 1 / float64(rank)
		return
	}
	backend.Misses = append(backend.Misses, fmt.Sprintf("%s[%s]: %q", query.Name, query.Category, query.Query))
}

func logRetrievalMisses(t *testing.T, backend retrievalEvaluationBackend) {
	t.Helper()
	if len(backend.Misses) == 0 {
		t.Logf("%s misses: none", backend.Name)
		return
	}
	sort.Strings(backend.Misses)
	t.Logf("%s misses (%d): %v", backend.Name, len(backend.Misses), backend.Misses)
}

func rankLiveSession(matches []codex.SearchMatch, sessionID string) int {
	for i, match := range matches {
		if match.Session.ID == sessionID {
			return i + 1
		}
	}
	return 0
}

func rankIndexedSession(matches []index.SearchMatch, sessionID string) int {
	for i, match := range matches {
		if match.Session.ID == sessionID {
			return i + 1
		}
	}
	return 0
}

func writeRetrievalEvaluationRollout(t *testing.T, home string, session retrievalEvaluationSession) {
	t.Helper()

	root := filepath.Join(home, "sessions", "2026", "09", "04")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "rollout-"+session.ID+".jsonl")

	records := []map[string]any{
		{
			"timestamp": session.Timestamp,
			"type":      "session_meta",
			"payload": map[string]any{
				"id":        session.ID,
				"timestamp": session.Timestamp,
				"cwd":       filepath.Join("/work", session.Project),
				"source":    session.Source,
			},
		},
		{
			"timestamp": session.Timestamp,
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": session.User,
			},
		},
		{
			"timestamp": session.Timestamp,
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "agent_message",
				"message": session.Assistant,
			},
		},
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
