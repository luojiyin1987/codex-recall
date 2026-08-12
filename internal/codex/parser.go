package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

type turnContext struct {
	CWD string `json:"cwd"`
}

// ParseFile extracts session metadata from a Codex rollout file. Current
// rollouts use session_meta. Older rollouts without session_meta fall back to
// the timestamp/session ID encoded in the rollout filename and, when present,
// the cwd from a turn_context record.
func ParseFile(path string) (Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer file.Close()

	session, err := Parse(file)
	if errors.Is(err, ErrSessionMetaNotFound) {
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
			return Session{}, fmt.Errorf("%s: rewind for legacy parse: %w", path, seekErr)
		}
		session, err = parseLegacy(file, path)
	}
	if err != nil {
		return Session{}, fmt.Errorf("%s: %w", path, err)
	}
	session.Path = path
	return session, nil
}

// Parse extracts session metadata from a rollout JSONL stream.
func Parse(r io.Reader) (Session, error) {
	scanner := newRolloutScanner(r)

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

func parseLegacy(r io.Reader, path string) (Session, error) {
	id, timestamp, err := parseRolloutFilename(path)
	if err != nil {
		return Session{}, err
	}

	session := Session{
		ID:        id,
		Timestamp: timestamp,
		Source:    "other",
	}

	scanner := newRolloutScanner(r)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var rec record
		if err := json.Unmarshal(line, &rec); err != nil || rec.Type != "turn_context" {
			continue
		}

		var context turnContext
		if err := json.Unmarshal(rec.Payload, &context); err != nil {
			continue
		}
		if context.CWD != "" {
			session.CWD = context.CWD
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func parseRolloutFilename(path string) (string, time.Time, error) {
	name := filepath.Base(path)
	if filepath.Ext(name) != ".jsonl" {
		return "", time.Time{}, errors.New("legacy rollout filename is not .jsonl")
	}
	name = strings.TrimSuffix(name, ".jsonl")
	const prefix = "rollout-"
	if !strings.HasPrefix(name, prefix) {
		return "", time.Time{}, errors.New("legacy rollout filename is missing rollout prefix")
	}

	rest := strings.TrimPrefix(name, prefix)
	const timestampLength = len("2006-01-02T15-04-05")
	if len(rest) <= timestampLength || rest[timestampLength] != '-' {
		return "", time.Time{}, errors.New("legacy rollout filename is missing timestamp or session id")
	}

	timestamp, err := time.ParseInLocation("2006-01-02T15-04-05", rest[:timestampLength], time.UTC)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse legacy rollout timestamp: %w", err)
	}
	id := rest[timestampLength+1:]
	if id == "" {
		return "", time.Time{}, errors.New("legacy rollout filename is missing session id")
	}
	return id, timestamp, nil
}

func newRolloutScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	// Rollout lines can contain large prompts or tool results. Use a generous
	// limit so metadata discovery does not become brittle.
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return scanner
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
