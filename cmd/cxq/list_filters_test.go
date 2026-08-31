package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRunnerListFiltersByProjectAndSource(t *testing.T) {
	home := t.TempDir()
	writeListFilterSession(t, home, "019abc11-1234-7abc-8def-0123456789ab", "/tmp/deepseek-harness-remote", "vscode")
	writeListFilterSession(t, home, "019abc12-1234-7abc-8def-0123456789ab", "/tmp/deepseek-harness-remote", "cli")
	writeListFilterSession(t, home, "019abc13-1234-7abc-8def-0123456789ab", "/tmp/other-project", "vscode")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{
		"list",
		"--home", home,
		"--project", " DEEPSEEK-HARNESS-REMOTE ",
		"--source", "VSCODE",
	}); err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "019abc11-1234-7abc-8def-0123456789ab") {
		t.Fatalf("list output missing matching session: %q", output)
	}
	if strings.Contains(output, "019abc12-1234-7abc-8def-0123456789ab") {
		t.Fatalf("list output included wrong source: %q", output)
	}
	if strings.Contains(output, "019abc13-1234-7abc-8def-0123456789ab") {
		t.Fatalf("list output included wrong project: %q", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("list stderr = %q", stderr.String())
	}
}

func TestCLIRunnerSearchWithoutQueryPointsToList(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)

	err := runner.run([]string{"search", "--project", "deepseek-harness-remote"})
	if err == nil {
		t.Fatal("search without QUERY succeeded")
	}
	if !strings.Contains(err.Error(), "search requires QUERY") {
		t.Fatalf("search error = %q", err)
	}
	if !strings.Contains(err.Error(), "cxq list [--project PROJECT] [--source SOURCE]") {
		t.Fatalf("search error does not point to filtered list: %q", err)
	}
}

func TestCLIRunnerListPositionalPointsToSearch(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)

	err := runner.run([]string{"list", "search", "--project", "deepseek-harness-remote"})
	if err == nil {
		t.Fatal("list accepted positional arguments")
	}
	if !strings.Contains(err.Error(), "cxq search [OPTIONS] QUERY") {
		t.Fatalf("list error does not point to search: %q", err)
	}
}

func TestCLIUsageShowsFilteredListExample(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"help"}); err != nil {
		t.Fatal(err)
	}

	help := stderr.String()
	if !strings.Contains(help, "cxq list [--home PATH] [--project PROJECT] [--source SOURCE]") {
		t.Fatalf("help missing list filters: %q", help)
	}
	if !strings.Contains(help, "cxq list --project deepseek-harness-remote") {
		t.Fatalf("help missing filtered-list example: %q", help)
	}
}

func writeListFilterSession(t *testing.T, home, sessionID, cwd, source string) {
	t.Helper()
	path := filepath.Join(home, "sessions", "2026", "08", "31", "rollout-2026-08-31T01-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-08-31T01:00:00Z","cwd":"` + cwd + `","source":"` + source + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
