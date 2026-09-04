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

func writeLiveSearchJSON(w io.Writer, query string, matches []codex.SearchMatch) error {
	results := make([]searchJSONResult, 0, len(matches))
	for _, match := range matches {
		results = append(results, searchJSONResult{
			SessionID: match.Session.ID,
			Timestamp: jsonTimestamp(match.Session.Timestamp),
			Project:   match.Session.Project(),
			Source:    match.Session.Source,
			Role:      match.Role,
			Snippet:   match.Snippet,
		})
	}
	return writeJSON(w, searchJSONOutput{
		SchemaVersion: jsonSchemaVersion,
		Backend:       "live",
		Query:         query,
		Results:       results,
	})
}

func writeIndexedSearchJSON(w io.Writer, query string, matches []index.SearchMatch) error {
	results := make([]searchJSONResult, 0, len(matches))
	for _, match := range matches {
		ordinal := match.Ordinal
		score := match.Score
		why := match.Why
		results = append(results, searchJSONResult{
			SessionID: match.Session.ID,
			Timestamp: jsonTimestamp(match.Session.Timestamp),
			Project:   match.Session.Project,
			Source:    match.Session.Source,
			Role:      match.Role,
			Snippet:   match.Snippet,
			Ordinal:   &ordinal,
			Score:     &score,
			Why:       &why,
		})
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
