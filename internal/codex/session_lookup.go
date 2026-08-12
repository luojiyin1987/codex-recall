package codex

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveSession resolves a full session ID or a unique ID prefix to a local
// Codex session. Duplicate copies of the same session ID are treated as one
// logical session, preferring the newest metadata record.
func ResolveSession(home, selector string) (Session, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Session{}, fmt.Errorf("session selector must not be empty")
	}

	files, err := DiscoverFiles(home)
	if err != nil {
		return Session{}, err
	}

	byID := make(map[string]Session)
	for _, path := range files {
		session, err := ParseFile(path)
		if err != nil {
			continue
		}
		current, ok := byID[session.ID]
		if !ok || session.Timestamp.After(current.Timestamp) {
			byID[session.ID] = session
		}
	}

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
