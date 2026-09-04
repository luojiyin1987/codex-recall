package indexer

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCompareClassifiesSessionOverlapAndBackendOnlyHits(t *testing.T) {
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
	if result.Overlap != 1 || result.LiveOnly != 1 || result.IndexOnly != 1 {
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
	if statuses["live-session"] != CompareLiveOnly {
		t.Fatalf("live-session status = %q", statuses["live-session"])
	}
	if statuses["index-session"] != CompareIndexOnly {
		t.Fatalf("index-session status = %q", statuses["index-session"])
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
