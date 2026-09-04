package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/luojiyin1987/codex-recall/internal/codex"
	"github.com/luojiyin1987/codex-recall/internal/index"
	"github.com/luojiyin1987/codex-recall/internal/indexer"
)

const jsonSchemaVersion = 1

type searchJSONOutput struct {
	SchemaVersion int                `json:"schema_version"`
	Backend       string             `json:"backend"`
	Query         string             `json:"query"`
	Results       []searchJSONResult `json:"results"`
}

type searchJSONResult struct {
	SessionID string   `json:"session_id"`
	Timestamp *string  `json:"timestamp"`
	Project   string   `json:"project"`
	Source    string   `json:"source"`
	Role      string   `json:"role"`
	Snippet   string   `json:"snippet"`
	Ordinal   *int     `json:"ordinal"`
	Score     *float64 `json:"score"`
	Why       *string  `json:"why"`
}

type statusJSONOutput struct {
	SchemaVersion int     `json:"schema_version"`
	Database      string  `json:"database"`
	Sessions      int     `json:"sessions"`
	LatestSession *string `json:"latest_session"`
	DatabaseBytes int64   `json:"database_bytes"`
}

type listJSONOutput struct {
	SchemaVersion int              `json:"schema_version"`
	Results       []listJSONResult `json:"results"`
}

type listJSONResult struct {
	SessionID string  `json:"session_id"`
	Timestamp *string `json:"timestamp"`
	Project   string  `json:"project"`
	Source    string  `json:"source"`
}

type compareJSONOutput struct {
	SchemaVersion int                `json:"schema_version"`
	Database      string             `json:"database"`
	Query         string             `json:"query"`
	LiveResults   int                `json:"live_results"`
	IndexResults  int                `json:"index_results"`
	LiveSessions  int                `json:"live_sessions"`
	IndexSessions int                `json:"index_sessions"`
	Overlap       int                `json:"overlap"`
	LiveOnly      int                `json:"live_only"`
	IndexOnly     int                `json:"index_only"`
	Entries       []compareJSONEntry `json:"entries"`
}

type compareJSONEntry struct {
	Status    string            `json:"status"`
	SessionID string            `json:"session_id"`
	Live      *searchJSONResult `json:"live"`
	Indexed   *searchJSONResult `json:"indexed"`
}

type packJSONOutput struct {
	SchemaVersion int                `json:"schema_version"`
	Database      string             `json:"database"`
	Query         string             `json:"query"`
	Project       string             `json:"project"`
	Source        string             `json:"source"`
	Evidence      []packJSONEvidence `json:"evidence"`
}

type packJSONEvidence struct {
	SessionID     string   `json:"session_id"`
	Timestamp     *string  `json:"timestamp"`
	Project       string   `json:"project"`
	Source        string   `json:"source"`
	Role          string   `json:"role"`
	Ordinal       int      `json:"ordinal"`
	Snippet       string   `json:"snippet"`
	Score         float64  `json:"score"`
	Why           string   `json:"why"`
	ResumeCommand string   `json:"resume_command"`
}

func liveSearchJSONResult(match codex.SearchMatch) searchJSONResult {
	return searchJSONResult{
		SessionID: match.Session.ID,
		Timestamp: jsonTimestamp(match.Session.Timestamp),
		Project:   match.Session.Project(),
		Source:    match.Session.Source,
		Role:      match.Role,
		Snippet:   match.Snippet,
	}
}

func writeLiveSearchJSON(w io.Writer, query string, matches []codex.SearchMatch) error {
	results := make([]searchJSONResult, 0, len(matches))
	for _, match := range matches {
		results = append(results, liveSearchJSONResult(match))
	}
	return writeJSON(w, searchJSONOutput{
		SchemaVersion: jsonSchemaVersion,
		Backend:       "live",
		Query:         query,
		Results:       results,
	})
}

func indexedSearchJSONResult(match index.SearchMatch) searchJSONResult {
	ordinal := match.Ordinal
	score := match.Score
	why := match.Why
	return searchJSONResult{
		SessionID: match.Session.ID,
		Timestamp: jsonTimestamp(match.Session.Timestamp),
		Project:   match.Session.Project,
		Source:    match.Session.Source,
		Role:      match.Role,
		Snippet:   match.Snippet,
		Ordinal:   &ordinal,
		Score:     &score,
		Why:       &why,
	}
}

func writeIndexedSearchJSON(w io.Writer, query string, matches []index.SearchMatch) error {
	results := make([]searchJSONResult, 0, len(matches))
	for _, match := range matches {
		results = append(results, indexedSearchJSONResult(match))
	}
	return writeJSON(w, searchJSONOutput{
		SchemaVersion: jsonSchemaVersion,
		Backend:       "index",
		Query:         query,
		Results:       results,
	})
}

func writeStatusJSON(w io.Writer, result indexer.StatusResult) error {
	return writeJSON(w, statusJSONOutput{
		SchemaVersion: jsonSchemaVersion,
		Database:      result.DatabasePath,
		Sessions:      result.Sessions,
		LatestSession: jsonTimestamp(result.LatestSession),
		DatabaseBytes: result.DatabaseBytes,
	})
}

func writeListJSON(w io.Writer, sessions []codex.Session) error {
	results := make([]listJSONResult, 0, len(sessions))
	for _, session := range sessions {
		results = append(results, listJSONResult{
			SessionID: session.ID,
			Timestamp: jsonTimestamp(session.Timestamp),
			Project:   session.Project(),
			Source:    session.Source,
		})
	}
	return writeJSON(w, listJSONOutput{
		SchemaVersion: jsonSchemaVersion,
		Results:       results,
	})
}

func writeCompareJSON(w io.Writer, query string, result indexer.CompareResult) error {
	entries := make([]compareJSONEntry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		jsonEntry := compareJSONEntry{
			Status:    entry.Status,
			SessionID: entry.SessionID,
		}
		if entry.Live != nil {
			live := liveSearchJSONResult(*entry.Live)
			jsonEntry.Live = &live
		}
		if entry.Indexed != nil {
			indexed := indexedSearchJSONResult(*entry.Indexed)
			jsonEntry.Indexed = &indexed
		}
		entries = append(entries, jsonEntry)
	}
	return writeJSON(w, compareJSONOutput{
		SchemaVersion: jsonSchemaVersion,
		Database:      result.DatabasePath,
		Query:         query,
		LiveResults:   result.LiveResults,
		IndexResults:  result.IndexResults,
		LiveSessions:  result.LiveSessions,
		IndexSessions: result.IndexSessions,
		Overlap:       result.Overlap,
		LiveOnly:      result.LiveOnly,
		IndexOnly:     result.IndexOnly,
		Entries:       entries,
	})
}

func writePackJSON(w io.Writer, result indexer.PackResult) error {
	evidence := make([]packJSONEvidence, 0, len(result.Evidence))
	for _, item := range result.Evidence {
		evidence = append(evidence, packJSONEvidence{
			SessionID:     item.SessionID,
			Timestamp:     jsonTimestamp(item.Timestamp),
			Project:       item.Project,
			Source:        item.Source,
			Role:          item.Role,
			Ordinal:       item.Ordinal,
			Snippet:       item.Snippet,
			Score:         item.Score,
			Why:           item.Why,
			ResumeCommand: item.ResumeCommand,
		})
	}
	return writeJSON(w, packJSONOutput{
		SchemaVersion: jsonSchemaVersion,
		Database:      result.DatabasePath,
		Query:         result.Query,
		Project:       result.Project,
		Source:        result.Source,
		Evidence:      evidence,
	})
}

func jsonTimestamp(timestamp time.Time) *string {
	if timestamp.IsZero() {
		return nil
	}
	value := timestamp.UTC().Format(time.RFC3339Nano)
	return &value
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
