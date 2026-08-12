package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

var ErrSessionMetaNotFound = errors.New("session_meta record not found")

type record struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	CWD       string          `json:"cwd"`
	Source    json.RawMessage `json:"source"`
}

// ParseFile extracts the first session_meta record from a Codex rollout file.
// Unknown record types are ignored so the parser remains tolerant of new
// event types added by Codex.
func ParseFile(path string) (Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer file.Close()

	session, err := Parse(file)
	if err != nil {
		return Session{}, fmt.Errorf("%s: %w", path, err)
	}
	session.Path = path
	return session, nil
}

// Parse extracts session metadata from a rollout JSONL stream.
func Parse(r io.Reader) (Session, error) {
	scanner := bufio.NewScanner(r)
	// Rollout lines can contain large prompts or tool results. The first
	// session_meta line is normally small, but use a generous limit so an
	// unexpected earlier record does not make discovery brittle.
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			// A damaged unrelated line should not hide a later valid
			// session_meta record.
			continue
		}
		if rec.Type != "session_meta" {
			continue
		}

		var meta sessionMeta
		if err := json.Unmarshal(rec.Payload, &meta); err != nil {
			return Session{}, fmt.Errorf("decode session_meta: %w", err)
		}
		if meta.ID == "" {
			return Session{}, errors.New("session_meta is missing id")
		}

		timestamp := meta.Timestamp
		if timestamp == "" {
			timestamp = rec.Timestamp
		}
		parsedTime, err := parseTimestamp(timestamp)
		if err != nil {
			return Session{}, err
		}

		return Session{
			ID:        meta.ID,
			Timestamp: parsedTime,
			CWD:       meta.CWD,
			Source:    parseSource(meta.Source),
		}, nil
	}
	if err := scanner.Err(); err != nil {
		return Session{}, err
	}
	return Session{}, ErrSessionMetaNotFound
}

func parseTimestamp(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
	}
	return parsed, nil
}

func parseSource(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "-"
	}
	var source string
	if err := json.Unmarshal(raw, &source); err == nil && source != "" {
		return source
	}
	// Source has changed shape across Codex versions. Keep the MVP parser
	// forward-compatible instead of rejecting the whole session.
	return "other"
}
