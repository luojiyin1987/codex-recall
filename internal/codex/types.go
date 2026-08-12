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
