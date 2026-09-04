package codex

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Catalog applies shared discovery, parsing, deduplication, and ordering rules.
type Catalog struct {
	home string
}

// NewCatalog returns a read-only catalog for a Codex home directory.
func NewCatalog(home string) Catalog {
	return Catalog{home: home}
}

// Sessions returns logical sessions ordered from newest to oldest.
func (c Catalog) Sessions() ([]Session, []error, error) {
	return c.SessionsContext(context.Background())
}

// SessionsContext is Sessions with cooperative cancellation.
func (c Catalog) SessionsContext(ctx context.Context) ([]Session, []error, error) {
	paths, err := DiscoverFilesContext(ctx, c.home)
	if err != nil {
		return nil, nil, err
	}
	byID, unreadable, err := parseSessionsContext(ctx, paths)
	if err != nil {
		return nil, nil, err
	}
	sessions := make([]Session, 0, len(byID))
	for _, session := range byID {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp.After(sessions[j].Timestamp)
	})
	warnings := make([]error, 0, len(unreadable))
	for _, item := range unreadable {
		warnings = append(warnings, item.err)
	}
	return sessions, warnings, nil
}

// Resolve returns the session for a full ID or unique ID prefix.
func (c Catalog) Resolve(selector string) (Session, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Session{}, fmt.Errorf("session selector must not be empty")
	}
	paths, err := sessionCandidateFiles(c.home, selector)
	if err != nil {
		return Session{}, err
	}
	byID, _ := parseSessions(paths)
	if session, ok := byID[selector]; ok {
		return session, nil
	}
	matches := make([]Session, 0)
	for id, session := range byID {
		if strings.HasPrefix(id, selector) {
			matches = append(matches, session)
		}
	}
	if len(matches) == 0 {
		return Session{}, fmt.Errorf("session %q not found", selector)
	}
	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
		return Session{}, fmt.Errorf("session prefix %q is ambiguous (%d matches); provide more characters", selector, len(matches))
	}
	return matches[0], nil
}

func (c Catalog) rankCandidates(paths []string, include func(Session) bool) []SearchCandidate {
	ranked, _ := c.rankCandidatesContext(context.Background(), paths, include)
	return ranked
}

func (c Catalog) rankCandidatesContext(ctx context.Context, paths []string, include func(Session) bool) ([]SearchCandidate, error) {
	byID, unreadable, err := parseSessionsContext(ctx, paths)
	if err != nil {
		return nil, err
	}
	ranked := make([]SearchCandidate, 0, len(byID)+len(unreadable))
	for _, session := range byID {
		if include == nil || include(session) {
			ranked = append(ranked, SearchCandidate{Path: session.Path, session: session, hasSession: true})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].session.Timestamp.After(ranked[j].session.Timestamp)
	})
	for _, item := range unreadable {
		ranked = append(ranked, SearchCandidate{Path: item.path})
	}
	return ranked, nil
}

type sessionParseError struct {
	path string
	err  error
}

func parseSessions(paths []string) (map[string]Session, []sessionParseError) {
	byID, unreadable, _ := parseSessionsContext(context.Background(), paths)
	return byID, unreadable
}

func parseSessionsContext(ctx context.Context, paths []string) (map[string]Session, []sessionParseError, error) {
	byID := make(map[string]Session)
	var unreadable []sessionParseError
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, unreadable, err
		}
		session, err := ParseFile(path)
		if err != nil {
			unreadable = append(unreadable, sessionParseError{path: path, err: err})
			continue
		}
		current, ok := byID[session.ID]
		if !ok || session.Timestamp.After(current.Timestamp) {
			byID[session.ID] = session
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, unreadable, err
	}
	return byID, unreadable, nil
}
