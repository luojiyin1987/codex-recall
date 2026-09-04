package index

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSQLiteSearchFindsLexicalMessages(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	newer := testSearchSession("newer", "/work/alpha", "alpha", "vscode", time.Date(2026, 9, 4, 5, 0, 0, 0, time.UTC))
	older := testSearchSession("older", "/work/beta", "beta", "cli", time.Date(2026, 9, 3, 5, 0, 0, 0, time.UTC))

	if err := idx.ReplaceSession(ctx, older, []Message{{Ordinal: 0, Role: "assistant", Text: "WebRTC transport failed during reconnect"}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.ReplaceSession(ctx, newer, []Message{
		{Ordinal: 0, Role: "user", Text: "please inspect WebRTC transport"},
		{Ordinal: 1, Role: "assistant", Text: "unrelated answer"},
	}); err != nil {
		t.Fatal(err)
	}

	matches, err := idx.Search(ctx, SearchOptions{Query: "webrtc transport", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2: %#v", len(matches), matches)
	}
	if matches[0].Session.ID != "newer" || matches[0].Ordinal != 0 {
		t.Fatalf("matches[0] = %#v", matches[0])
	}
	if matches[0].Why != lexicalWhy {
		t.Fatalf("why = %q, want %q", matches[0].Why, lexicalWhy)
	}
	if !strings.Contains(strings.ToLower(matches[0].Snippet), "webrtc transport") {
		t.Fatalf("snippet = %q", matches[0].Snippet)
	}
}

func TestSQLiteSearchAppliesProjectAndSourceFilters(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	for _, session := range []Session{
		testSearchSession("alpha-vscode", "/work/alpha", "alpha", "vscode", time.Now().UTC()),
		testSearchSession("alpha-cli", "/work/alpha", "alpha", "cli", time.Now().UTC()),
		testSearchSession("beta-vscode", "/work/beta", "beta", "vscode", time.Now().UTC()),
	} {
		if err := idx.ReplaceSession(ctx, session, []Message{{Ordinal: 0, Role: "user", Text: "shared needle"}}); err != nil {
			t.Fatal(err)
		}
	}

	matches, err := idx.Search(ctx, SearchOptions{
		Query:   "shared needle",
		Limit:   10,
		Project: "ALPHA",
		Source:  "VSCODE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Session.ID != "alpha-vscode" {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestSQLiteSearchReplacementRemovesStaleFTSRows(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	session := testSearchSession("replace", "/work/demo", "demo", "cli", time.Now().UTC())

	if err := idx.ReplaceSession(ctx, session, []Message{{Ordinal: 0, Role: "assistant", Text: "old searchable token"}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.ReplaceMessages(ctx, session.ID, []Message{{Ordinal: 0, Role: "assistant", Text: "new searchable token"}}); err != nil {
		t.Fatal(err)
	}

	oldMatches, err := idx.Search(ctx, SearchOptions{Query: "old searchable", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(oldMatches) != 0 {
		t.Fatalf("old matches = %#v", oldMatches)
	}

	newMatches, err := idx.Search(ctx, SearchOptions{Query: "new searchable", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(newMatches) != 1 {
		t.Fatalf("new matches = %#v", newMatches)
	}
}

func TestSQLiteSearchTreatsFTSOperatorsAsLiteralInput(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	session := testSearchSession("literal", "/work/demo", "demo", "cli", time.Now().UTC())
	if err := idx.ReplaceSession(ctx, session, []Message{{Ordinal: 0, Role: "user", Text: "AND is written here"}}); err != nil {
		t.Fatal(err)
	}

	matches, err := idx.Search(ctx, SearchOptions{Query: "AND", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestSQLiteSearchValidatesOptions(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	if _, err := idx.Search(ctx, SearchOptions{Query: " ", Limit: 10}); err == nil {
		t.Fatal("Search() accepted blank query")
	}
	if _, err := idx.Search(ctx, SearchOptions{Query: "needle", Limit: 0}); err == nil {
		t.Fatal("Search() accepted non-positive limit")
	}
}

func testSearchSession(id, cwd, project, source string, timestamp time.Time) Session {
	return Session{
		ID:          id,
		Timestamp:   timestamp,
		CWD:         cwd,
		Project:     project,
		Source:      source,
		RolloutPath: "/codex/" + id + ".jsonl",
		ContentHash: "v1:sha256:" + id,
	}
}


func TestSQLiteSearchLimitCountsUniqueSessions(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	hot := testSearchSession("hot", "/work/hot", "hot", "vscode", time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC))
	other := testSearchSession("other", "/work/other", "other", "cli", time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC))

	if err := idx.ReplaceSession(ctx, hot, []Message{
		{Ordinal: 0, Role: "user", Text: "needle"},
		{Ordinal: 1, Role: "assistant", Text: "needle"},
		{Ordinal: 2, Role: "user", Text: "needle"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := idx.ReplaceSession(ctx, other, []Message{
		{Ordinal: 0, Role: "assistant", Text: "needle"},
	}); err != nil {
		t.Fatal(err)
	}

	matches, err := idx.Search(ctx, SearchOptions{Query: "needle", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2 unique sessions: %#v", len(matches), matches)
	}
	if matches[0].Session.ID == matches[1].Session.ID {
		t.Fatalf("search returned duplicate session IDs: %#v", matches)
	}
}

func TestSQLiteSearchKeepsBestRankedMessagePerSession(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	session := testSearchSession("ranked", "/work/ranked", "ranked", "vscode", time.Now().UTC())
	if err := idx.ReplaceSession(ctx, session, []Message{
		{Ordinal: 0, Role: "user", Text: "needle appears with several unrelated words around it"},
		{Ordinal: 1, Role: "assistant", Text: "needle"},
	}); err != nil {
		t.Fatal(err)
	}

	matches, err := idx.Search(ctx, SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
	if matches[0].Ordinal != 1 {
		t.Fatalf("representative ordinal = %d, want best-ranked ordinal 1: %#v", matches[0].Ordinal, matches[0])
	}
}

func TestSQLiteSearchNormalizesSnippetWhitespace(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	session := testSearchSession("whitespace", "/work/demo", "demo", "cli", time.Now().UTC())

	if err := idx.ReplaceSession(ctx, session, []Message{{
		Ordinal: 0,
		Role:    "user",
		Text:    "alpha\n\tneedle\u3000beta",
	}}); err != nil {
		t.Fatal(err)
	}

	matches, err := idx.Search(ctx, SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
	if strings.ContainsAny(matches[0].Snippet, "\n\r\t") {
		t.Fatalf("snippet contains line-breaking whitespace: %q", matches[0].Snippet)
	}
	if !strings.Contains(matches[0].Snippet, "alpha needle beta") {
		t.Fatalf("snippet = %q, want normalized preview", matches[0].Snippet)
	}
}


func TestSQLiteSearchTrigramFindsSubstringShapes(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		query string
	}{
		{name: "Chinese continuous substring", text: "我们使用批量事务写入索引", query: "批量事务"},
		{name: "camelCase prefix", text: "callbackInfo remains stable", query: "callback"},
		{name: "UUID prefix", text: "019fe0cb-9760-78b1-b545-b5e90d1dd0d7", query: "019fe0"},
		{name: "SHA prefix", text: "49bcc7e8ab386d700cb8ccfa5a5b72d97528898f", query: "49bcc7e8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := openTestIndex(t)
			ctx := context.Background()
			session := testSearchSession("substring", "/work/demo", "demo", "vscode", time.Now().UTC())
			if err := idx.ReplaceSession(ctx, session, []Message{{Ordinal: 0, Role: "user", Text: tc.text}}); err != nil {
				t.Fatal(err)
			}
			matches, err := idx.Search(ctx, SearchOptions{Query: tc.query, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 1 || matches[0].Session.ID != "substring" {
				t.Fatalf("matches = %#v", matches)
			}
			if matches[0].Why != lexicalWhy {
				t.Fatalf("why = %q, want %q", matches[0].Why, lexicalWhy)
			}
		})
	}
}

func TestSQLiteSearchShortLiteralFallback(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	newer := testSearchSession("newer-short", "/work/alpha", "alpha", "vscode", time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC))
	older := testSearchSession("older-short", "/work/beta", "beta", "cli", time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC))

	if err := idx.ReplaceSession(ctx, older, []Message{{Ordinal: 0, Role: "assistant", Text: "Go 语言错误排查"}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.ReplaceSession(ctx, newer, []Message{{Ordinal: 0, Role: "user", Text: "GO runtime 错误"}}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		query   string
		project string
		wantID  string
	}{
		{name: "two ASCII runes case insensitive", query: "go", wantID: "newer-short"},
		{name: "one Chinese rune", query: "错", wantID: "newer-short"},
		{name: "two Chinese runes with filter", query: "错误", project: "beta", wantID: "older-short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches, err := idx.Search(ctx, SearchOptions{
				Query: tc.query, Limit: 10, Project: tc.project,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) == 0 || matches[0].Session.ID != tc.wantID {
				t.Fatalf("matches = %#v, want first session %q", matches, tc.wantID)
			}
			if matches[0].Why != substringWhy {
				t.Fatalf("why = %q, want %q", matches[0].Why, substringWhy)
			}
			if matches[0].Score != 0 {
				t.Fatalf("score = %g, want 0 for unranked substring fallback", matches[0].Score)
			}
			if !strings.Contains(strings.ToLower(matches[0].Snippet), strings.ToLower(tc.query)) {
				t.Fatalf("snippet = %q, want query %q", matches[0].Snippet, tc.query)
			}
		})
	}
}
