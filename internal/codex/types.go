package codex

import (
	"path"
	"strings"
	"time"
)

// Session is the small, stable subset of Codex session metadata that
// codex-recall needs for discovery and listing.
type Session struct {
	ID        string
	Timestamp time.Time
	CWD       string
	Source    string
	Path      string
}

// MatchesFilters reports whether the session matches the given project and
// source filters. Both filters are case-insensitive exact-match after
// trimming whitespace. An empty filter matches everything.
func (s Session) MatchesFilters(project, source string) bool {
	project = strings.TrimSpace(project)
	source = strings.TrimSpace(source)
	if project != "" && !strings.EqualFold(s.Project(), project) {
		return false
	}
	if source != "" && !strings.EqualFold(s.Source, source) {
		return false
	}
	return true
}

// Project returns a compact project name derived from the session working
// directory. Normalize separators first so a Linux/WSL build can also display
// project names from Windows-authored sessions.
func (s Session) Project() string {
	if s.CWD == "" {
		return "-"
	}
	normalized := strings.ReplaceAll(s.CWD, `\`, "/")
	normalized = strings.TrimRight(normalized, "/")
	if normalized == "" {
		return "/"
	}
	project := path.Base(normalized)
	if project == "." || project == "/" || project == "" {
		return normalized
	}
	return project
}
