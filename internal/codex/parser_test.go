package codex

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSessionMeta(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-08-12T02:58:59.000Z","type":"response_item","payload":{}}`,
		`{"timestamp":"2026-08-12T02:59:15.123456Z","type":"session_meta","payload":{"id":"019abc","timestamp":"2026-08-12T02:59:15.123456Z","cwd":"/home/luo/dev/codex-recall","source":"vscode"}}`,
	}, "\n")

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "019abc" {
		t.Fatalf("ID = %q, want 019abc", got.ID)
	}
	if got.CWD != "/home/luo/dev/codex-recall" {
		t.Fatalf("CWD = %q", got.CWD)
	}
	if got.Source != "vscode" {
		t.Fatalf("Source = %q, want vscode", got.Source)
	}
	wantTime, _ := time.Parse(time.RFC3339Nano, "2026-08-12T02:59:15.123456Z")
	if !got.Timestamp.Equal(wantTime) {
		t.Fatalf("Timestamp = %s, want %s", got.Timestamp, wantTime)
	}
	if got.Project() != "codex-recall" {
		t.Fatalf("Project() = %q, want codex-recall", got.Project())
	}
}

func TestProjectHandlesWindowsPathOnAnyHost(t *testing.T) {
	session := Session{CWD: `C:\dev\codex-recall`}
	if got := session.Project(); got != "codex-recall" {
		t.Fatalf("Project() = %q, want codex-recall", got)
	}
}

func TestParseSkipsMalformedUnrelatedLine(t *testing.T) {
	input := "not-json\n" +
		`{"timestamp":"2026-08-12T02:59:15Z","type":"session_meta","payload":{"id":"019abc","cwd":"C:\\dev\\codex-recall"}}`

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "019abc" {
		t.Fatalf("ID = %q, want 019abc", got.ID)
	}
}

func TestParseSourceObjectDoesNotFailSession(t *testing.T) {
	input := `{"timestamp":"2026-08-12T02:59:15Z","type":"session_meta","payload":{"id":"019abc","cwd":"/tmp/project","source":{"subagent":"review"}}}`

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "other" {
		t.Fatalf("Source = %q, want other", got.Source)
	}
}

func TestParseMissingSessionMeta(t *testing.T) {
	_, err := Parse(strings.NewReader(`{"type":"response_item","payload":{}}`))
	if !errors.Is(err, ErrSessionMetaNotFound) {
		t.Fatalf("error = %v, want ErrSessionMetaNotFound", err)
	}
}

func TestParseFileFallsBackForLegacyRollout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-02-22T12-28-43-019c839b-5ed3-7603-bb29-446c47c1e18d.jsonl")
	input := strings.Join([]string{
		`{"timestamp":"2026-02-22T12:28:43Z","type":"response_item","payload":{}}`,
		`{"timestamp":"2026-02-22T12:28:44Z","type":"turn_context","payload":{"cwd":"/home/luo/dev/legacy-project"}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "019c839b-5ed3-7603-bb29-446c47c1e18d" {
		t.Fatalf("ID = %q", got.ID)
	}
	if got.Source != "other" {
		t.Fatalf("Source = %q, want other", got.Source)
	}
	if got.Project() != "legacy-project" {
		t.Fatalf("Project() = %q, want legacy-project", got.Project())
	}
	wantTime := time.Date(2026, 2, 22, 12, 28, 43, 0, time.UTC)
	if !got.Timestamp.Equal(wantTime) {
		t.Fatalf("Timestamp = %s, want %s", got.Timestamp, wantTime)
	}
}

func TestParseFileLegacyRolloutWithoutCWDStillLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-02-22T12-28-43-019c839b-5ed3-7603-bb29-446c47c1e18d.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"response_item","payload":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project() != "-" {
		t.Fatalf("Project() = %q, want -", got.Project())
	}
}
