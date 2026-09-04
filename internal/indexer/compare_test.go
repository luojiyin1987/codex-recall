package indexer

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCompareTrigramAlignsLiteralSubstringOverlap(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "both-session", "plain foo bar phrase", "answer")
	writeRollout(t, home, "live-session", "prefix xfoo barz suffix", "answer")
	writeRollout(t, home, "index-session", "punctuated foo-bar phrase", "answer")

	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	result, err := Compare(context.Background(), home, CompareOptions{
		Query: "foo bar",
		Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.LiveResults != 2 {
		t.Fatalf("LiveResults = %d, want 2", result.LiveResults)
	}
	if result.IndexResults != 2 {
		t.Fatalf("IndexResults = %d, want 2", result.IndexResults)
	}
	if result.LiveSessions != 2 || result.IndexSessions != 2 {
		t.Fatalf("session counts = live %d index %d", result.LiveSessions, result.IndexSessions)
	}
	if result.Overlap != 2 || result.LiveOnly != 0 || result.IndexOnly != 0 {
		t.Fatalf("comparison counts = overlap %d live-only %d index-only %d", result.Overlap, result.LiveOnly, result.IndexOnly)
	}

	statuses := make(map[string]string)
	for _, entry := range result.Entries {
		statuses[entry.SessionID] = entry.Status
		switch entry.Status {
		case CompareBoth:
			if entry.Live == nil || entry.Indexed == nil {
				t.Fatalf("both entry missing evidence: %#v", entry)
			}
		case CompareLiveOnly:
			if entry.Live == nil || entry.Indexed != nil {
				t.Fatalf("live-only entry evidence = %#v", entry)
			}
		case CompareIndexOnly:
			if entry.Live != nil || entry.Indexed == nil {
				t.Fatalf("index-only entry evidence = %#v", entry)
			}
		}
	}
	if statuses["both-session"] != CompareBoth {
		t.Fatalf("both-session status = %q", statuses["both-session"])
	}
	if statuses["live-session"] != CompareBoth {
		t.Fatalf("live-session status = %q", statuses["live-session"])
	}
	if _, ok := statuses["index-session"]; ok {
		t.Fatalf("index-session unexpectedly matched query: %q", statuses["index-session"])
	}
}

func TestCompareDoesNotRefreshDerivedIndex(t *testing.T) {
	home := t.TempDir()
	path := writeRollout(t, home, "stale-session", "baseline text", "answer")
	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(`{"timestamp":"2026-09-04T08:00:03Z","type":"event_msg","payload":{"type":"user_message","message":"fresh comparison needle"}}` + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	result, err := Compare(context.Background(), home, CompareOptions{
		Query: "fresh comparison needle",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.LiveSessions != 1 || result.IndexSessions != 0 || result.LiveOnly != 1 {
		t.Fatalf("Compare() = %#v", result)
	}
}

func TestCompareRequiresExistingDerivedIndex(t *testing.T) {
	home := t.TempDir()
	_, err := Compare(context.Background(), home, CompareOptions{Query: "needle", Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "run cxq index") {
		t.Fatalf("Compare() error = %v", err)
	}
}


func TestCompareAppliesLimitToSessionsOnBothBackends(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "hot-session", "needle", "needle")
	writeRollout(t, home, "other-session", "needle", "answer")

	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	result, err := Compare(context.Background(), home, CompareOptions{
		Query: "needle",
		Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.LiveResults != 2 || result.LiveSessions != 2 {
		t.Fatalf("live counts = results %d sessions %d", result.LiveResults, result.LiveSessions)
	}
	if result.IndexResults != 2 || result.IndexSessions != 2 {
		t.Fatalf("index counts = results %d sessions %d", result.IndexResults, result.IndexSessions)
	}
}

func TestCompareCoversChineseAndCodeQueryShapes(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		name  string
		id    string
		text  string
		query string
	}{
		{name: "chinese exact token", id: "shape-chinese", text: "批量事务", query: "批量事务"},
		{name: "camel case", id: "shape-camel", text: "callbackInfo remains stable", query: "callbackInfo"},
		{name: "snake case", id: "shape-snake", text: "callback_info remains stable", query: "callback_info"},
		{name: "path", id: "shape-path", text: "inspect internal/index/sqlite.go", query: "internal/index/sqlite.go"},
		{name: "punctuated error", id: "shape-error", text: "dyld: missing LC_UUID load command", query: "missing LC_UUID load command"},
		{name: "uuid", id: "shape-uuid", text: "session 019fe0cb-9760-78b1-b545-b5e90d1dd0d7", query: "019fe0cb-9760-78b1-b545-b5e90d1dd0d7"},
		{name: "commit sha", id: "shape-sha", text: "commit 49bcc7e8ab386d700cb8ccfa5a5b72d97528898f", query: "49bcc7e8ab386d700cb8ccfa5a5b72d97528898f"},
		{name: "method call", id: "shape-method", text: "transport.send() failed", query: "transport.send()"},
		{name: "hyphenated text", id: "shape-hyphen", text: "foo-bar phrase", query: "foo-bar"},
	}
	for _, tc := range cases {
		writeRollout(t, home, tc.id, tc.text, "answer")
	}
	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Compare(context.Background(), home, CompareOptions{
				Query: tc.query,
				Limit: 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.LiveSessions != 1 || result.IndexSessions != 1 {
				t.Fatalf("session counts = live %d index %d for query %q", result.LiveSessions, result.IndexSessions, tc.query)
			}
			if result.Overlap != 1 || result.LiveOnly != 0 || result.IndexOnly != 0 {
				t.Fatalf("comparison = overlap %d live-only %d index-only %d for query %q",
					result.Overlap, result.LiveOnly, result.IndexOnly, tc.query)
			}
			if len(result.Entries) != 1 || result.Entries[0].Status != CompareBoth || result.Entries[0].SessionID != tc.id {
				t.Fatalf("entries = %#v for query %q", result.Entries, tc.query)
			}
		})
	}
}

func TestCompareTrigramCoversSubstringBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		query string
	}{
		{
			name:  "chinese substring inside continuous text",
			text:  "我们使用批量事务写入索引",
			query: "批量事务",
		},
		{
			name:  "camel case prefix",
			text:  "callbackInfo remains stable",
			query: "callback",
		},
		{
			name:  "uuid partial token",
			text:  "019fe0cb-9760-78b1-b545-b5e90d1dd0d7",
			query: "019fe0",
		},
		{
			name:  "commit sha prefix",
			text:  "49bcc7e8ab386d700cb8ccfa5a5b72d97528898f",
			query: "49bcc7e8",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, home, "boundary-session", tc.text, "answer")
			if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
				t.Fatal(err)
			}

			result, err := Compare(context.Background(), home, CompareOptions{
				Query: tc.query,
				Limit: 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.LiveSessions != 1 || result.IndexSessions != 1 {
				t.Fatalf("session counts = live %d index %d for query %q", result.LiveSessions, result.IndexSessions, tc.query)
			}
			if result.Overlap != 1 || result.LiveOnly != 0 || result.IndexOnly != 0 {
				t.Fatalf("comparison = overlap %d live-only %d index-only %d for query %q",
					result.Overlap, result.LiveOnly, result.IndexOnly, tc.query)
			}
			if len(result.Entries) != 1 || result.Entries[0].Status != CompareBoth {
				t.Fatalf("entries = %#v for query %q", result.Entries, tc.query)
			}
		})
	}
}
