package indexer

import (
	"context"

	"github.com/luojiyin1987/codex-recall/internal/codex"
	"github.com/luojiyin1987/codex-recall/internal/index"
)

const (
	CompareBoth      = "both"
	CompareLiveOnly  = "live-only"
	CompareIndexOnly = "index-only"
)

type CompareOptions struct {
	DatabasePath string
	Query        string
	Limit        int
	Project      string
	Source       string
}

type ComparisonEntry struct {
	Status    string
	SessionID string
	Live      *codex.SearchMatch
	Indexed   *index.SearchMatch
}

type CompareResult struct {
	DatabasePath  string
	LiveResults   int
	IndexResults  int
	LiveSessions  int
	IndexSessions int
	Overlap       int
	LiveOnly      int
	IndexOnly     int
	Entries       []ComparisonEntry
	Warnings      []error
}

// Compare runs both current search backends for the same options and compares
// the unique session IDs present in each backend's returned top-N result set.
// It does not refresh the derived index.
func Compare(ctx context.Context, home string, options CompareOptions) (CompareResult, error) {
	live, err := codex.Search(home, codex.SearchOptions{
		Query:   options.Query,
		Limit:   options.Limit,
		Project: options.Project,
		Source:  options.Source,
	})
	if err != nil {
		return CompareResult{}, err
	}

	indexed, err := Search(ctx, home, SearchOptions{
		DatabasePath: options.DatabasePath,
		Query:        options.Query,
		Limit:        options.Limit,
		Project:      options.Project,
		Source:       options.Source,
	})
	if err != nil {
		return CompareResult{Warnings: append([]error(nil), live.Warnings...)}, err
	}

	liveBySession := make(map[string]codex.SearchMatch, len(live.Matches))
	liveOrder := make([]string, 0, len(live.Matches))
	for _, match := range live.Matches {
		if _, exists := liveBySession[match.Session.ID]; exists {
			continue
		}
		liveBySession[match.Session.ID] = match
		liveOrder = append(liveOrder, match.Session.ID)
	}

	indexBySession := make(map[string]index.SearchMatch, len(indexed.Matches))
	indexOrder := make([]string, 0, len(indexed.Matches))
	for _, match := range indexed.Matches {
		if _, exists := indexBySession[match.Session.ID]; exists {
			continue
		}
		indexBySession[match.Session.ID] = match
		indexOrder = append(indexOrder, match.Session.ID)
	}

	result := CompareResult{
		DatabasePath:  indexed.DatabasePath,
		LiveResults:   len(live.Matches),
		IndexResults:  len(indexed.Matches),
		LiveSessions:  len(liveBySession),
		IndexSessions: len(indexBySession),
		Warnings:      append([]error(nil), live.Warnings...),
	}

	for _, sessionID := range liveOrder {
		liveMatch := liveBySession[sessionID]
		if indexedMatch, ok := indexBySession[sessionID]; ok {
			liveCopy := liveMatch
			indexCopy := indexedMatch
			result.Entries = append(result.Entries, ComparisonEntry{
				Status:    CompareBoth,
				SessionID: sessionID,
				Live:      &liveCopy,
				Indexed:   &indexCopy,
			})
			result.Overlap++
			continue
		}
		liveCopy := liveMatch
		result.Entries = append(result.Entries, ComparisonEntry{
			Status:    CompareLiveOnly,
			SessionID: sessionID,
			Live:      &liveCopy,
		})
		result.LiveOnly++
	}

	for _, sessionID := range indexOrder {
		if _, exists := liveBySession[sessionID]; exists {
			continue
		}
		indexedMatch := indexBySession[sessionID]
		indexCopy := indexedMatch
		result.Entries = append(result.Entries, ComparisonEntry{
			Status:    CompareIndexOnly,
			SessionID: sessionID,
			Indexed:   &indexCopy,
		})
		result.IndexOnly++
	}

	return result, nil
}
