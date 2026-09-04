package indexer

import (
	"context"
	"strings"
	"time"
)

type PackOptions struct {
	DatabasePath string
	Query        string
	Limit        int
	Project      string
	Source       string
}

type PackEvidence struct {
	SessionID     string
	Timestamp     time.Time
	Project       string
	Source        string
	Role          string
	Ordinal       int
	Snippet       string
	Score         float64
	Why           string
	ResumeCommand string
}

type PackResult struct {
	DatabasePath string
	Query        string
	Project      string
	Source       string
	Evidence     []PackEvidence
}

// Pack builds a deterministic context pack from the existing derived index.
// It does not refresh the index or derive semantic memories; every evidence
// item retains direct provenance to one indexed Codex session/message.
func Pack(ctx context.Context, home string, options PackOptions) (PackResult, error) {
	result := PackResult{
		Query:   strings.TrimSpace(options.Query),
		Project: strings.TrimSpace(options.Project),
		Source:  strings.TrimSpace(options.Source),
	}

	searchResult, err := Search(ctx, home, SearchOptions{
		DatabasePath: options.DatabasePath,
		Query:        options.Query,
		Limit:        options.Limit,
		Project:      options.Project,
		Source:       options.Source,
	})
	result.DatabasePath = searchResult.DatabasePath
	if err != nil {
		return result, err
	}

	result.Evidence = make([]PackEvidence, 0, len(searchResult.Matches))
	for _, match := range searchResult.Matches {
		result.Evidence = append(result.Evidence, PackEvidence{
			SessionID:     match.Session.ID,
			Timestamp:     match.Session.Timestamp,
			Project:       match.Session.Project,
			Source:        match.Session.Source,
			Role:          match.Role,
			Ordinal:       match.Ordinal,
			Snippet:       match.Snippet,
			Score:         match.Score,
			Why:           match.Why,
			ResumeCommand: "cxq resume " + match.Session.ID,
		})
	}
	return result, nil
}
