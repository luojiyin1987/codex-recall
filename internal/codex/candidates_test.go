package codex

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSearchCandidateFilesUsesRipgrepResults(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	matching := filepath.Join(root, "rollout-match.jsonl")
	other := filepath.Join(root, "rollout-other.jsonl")
	for _, path := range []string{matching, other} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var gotArgs []string
	got, err := searchCandidateFiles(home, `annotated "tag"`,
		func(name string) (string, error) { return "/fake/rg", nil },
		func(name string, args ...string) ([]byte, error) {
			if name != "/fake/rg" {
				t.Fatalf("name = %q", name)
			}
			gotArgs = append([]string(nil), args...)
			return []byte(matching + "\x00"), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{matching}) {
		t.Fatalf("candidates = %#v", got)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"-l", "-0", "--pcre2", "--ignore-case", "--no-ignore", "*.jsonl", "response_item", "event_msg", `annotated \\"tag\\"`, root} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestSearchCandidateFilesFallsBackWhenRipgrepMissing(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := searchCandidateFiles(home, "needle",
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { t.Fatal("runner should not be called"); return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{path}) {
		t.Fatalf("candidates = %#v", got)
	}
}

func TestSearchCandidateFilesFallsBackWhenRipgrepFails(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := searchCandidateFiles(home, "needle",
		func(string) (string, error) { return "/fake/rg", nil },
		func(string, ...string) ([]byte, error) { return nil, errors.New("boom") },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{path}) {
		t.Fatalf("candidates = %#v", got)
	}
}

func TestSearchCandidateFilesSkipsDiscoveryWhenRipgrepSucceeds(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	matching := filepath.Join(root, "matching.jsonl")
	discovered := false

	got, err := searchCandidateFilesWithDiscovery(
		home,
		"needle",
		func(string) (string, error) { return "/fake/rg", nil },
		func(string, ...string) ([]byte, error) { return []byte(matching + "\x00"), nil },
		func(string) ([]string, error) {
			discovered = true
			return nil, errors.New("discovery should not run")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if discovered {
		t.Fatal("SearchCandidateFiles() ran discovery after ripgrep succeeded")
	}
	if !reflect.DeepEqual(got, []string{matching}) {
		t.Fatalf("SearchCandidateFiles() = %#v, want [%s]", got, matching)
	}
}

func TestSearchCandidateFilesExcludesToolOnlyMatches(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
	if err := exec.Command("rg", "--pcre2-version").Run(); err != nil {
		t.Skip("rg does not support PCRE2")
	}

	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	conversation := filepath.Join(root, "conversation.jsonl")
	tool := filepath.Join(root, "tool.jsonl")
	writeSearchFixture(t, root, filepath.Base(conversation), []string{
		`{"payload":{"role":"user","content":[{"text":"release \"tag\"","type":"input_text"}],"type":"message"},"type":"response_item"}`,
	})
	writeSearchFixture(t, root, filepath.Base(tool), []string{
		`{"payload":{"type":"function_call_output","output":"release \"tag\""},"type":"response_item"}`,
	})

	got, err := SearchCandidateFiles(home, `release "tag"`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{conversation}) {
		t.Fatalf("SearchCandidateFiles() = %#v, want [%s]", got, conversation)
	}
}

func TestParseRipgrepPathsSortsNullDelimitedOutput(t *testing.T) {
	got := parseRipgrepPaths([]byte("b.jsonl\x00a.jsonl\x00"))
	want := []string{"a.jsonl", "b.jsonl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestRankCandidateFilesUsesMetadata(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	older := filepath.Join(root, "older.jsonl")
	newer := filepath.Join(root, "newer.jsonl")
	other := filepath.Join(root, "other.jsonl")
	broken := filepath.Join(root, "broken.jsonl")
	writeCandidateSession(t, older, "older", "2026-08-12T00:00:00Z", "/tmp/lint-md", "vscode")
	writeCandidateSession(t, newer, "newer", "2026-08-13T00:00:00Z", "/tmp/lint-md", "vscode")
	writeCandidateSession(t, other, "other", "2026-08-14T00:00:00Z", "/tmp/other", "vscode")
	if err := os.WriteFile(broken, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := RankCandidateFiles([]string{older, other, broken, newer}, func(session Session) bool {
		return session.Project() == "lint-md"
	})
	want := []string{newer, older, broken}
	if gotPaths := candidatePaths(got); !reflect.DeepEqual(gotPaths, want) {
		t.Fatalf("RankCandidateFiles() paths = %#v, want %#v", gotPaths, want)
	}
	if !got[0].hasSession || !got[1].hasSession || got[2].hasSession {
		t.Fatalf("RankCandidateFiles() metadata = %#v", got)
	}
}

func TestOptimizedSearchMatchesBaseline(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}

	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSearchFixture(t, root, "reordered.jsonl", []string{
		`{"type":"session_meta","payload":{"source":"vscode","cwd":"/tmp/lint-md","timestamp":"2026-08-13T03:00:00Z","id":"reordered"}}`,
		`{"type":"response_item","payload":{"role":"user","content":[{"text":"release tag","type":"input_text"}],"type":"message"}}`,
	})
	writeSearchFixture(t, root, "event.jsonl", []string{
		`{"type":"session_meta","payload":{"id":"event","timestamp":"2026-08-13T02:00:00Z","cwd":"/tmp/lint-md","source":"vscode"}}`,
		`{"type":"event_msg","payload":{"message":"release tag","type":"agent_message"}}`,
	})
	writeSearchFixture(t, root, "older.jsonl", []string{
		`{"type":"session_meta","payload":{"id":"older","timestamp":"2026-08-13T01:00:00Z","cwd":"/tmp/lint-md","source":"vscode"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"release tag"}]}}`,
	})
	writeSearchFixture(t, root, "other-project.jsonl", []string{
		`{"type":"session_meta","payload":{"id":"other-project","timestamp":"2026-08-13T04:00:00Z","cwd":"/tmp/other","source":"vscode"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"release tag"}}`,
	})
	writeSearchFixture(t, root, "other-source.jsonl", []string{
		`{"type":"session_meta","payload":{"id":"other-source","timestamp":"2026-08-13T05:00:00Z","cwd":"/tmp/lint-md","source":"cli"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"release tag"}}`,
	})
	writeSearchFixture(t, root, "tool.jsonl", []string{
		`{"type":"session_meta","payload":{"id":"tool","timestamp":"2026-08-13T06:00:00Z","cwd":"/tmp/lint-md","source":"vscode"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","output":"release tag"}}`,
	})
	writeSearchFixture(t, root, "broken.jsonl", []string{`broken release tag`})

	files, err := DiscoverFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	include := func(session Session) bool {
		return session.Project() == "lint-md" && session.Source == "vscode"
	}
	baseline := make([]SearchMatch, 0)
	for _, path := range files {
		match, ok, err := SearchFile(path, "tag")
		if err == nil && ok && include(match.Session) {
			baseline = append(baseline, match)
		}
	}
	sort.Slice(baseline, func(i, j int) bool {
		return baseline[i].Session.Timestamp.After(baseline[j].Session.Timestamp)
	})
	baseline = baseline[:2]

	candidates, err := SearchCandidateFiles(home, "tag")
	if err != nil {
		t.Fatal(err)
	}
	optimized, searchErrors := SearchFiles(RankCandidateFiles(candidates, include), "tag", 2)
	if len(searchErrors) != 0 {
		t.Fatalf("SearchFiles() errors = %v", searchErrors)
	}
	if !reflect.DeepEqual(optimized, baseline) {
		t.Fatalf("optimized = %#v, baseline = %#v", optimized, baseline)
	}
}

func writeSearchFixture(t *testing.T, root, name string, lines []string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func candidatePaths(candidates []SearchCandidate) []string {
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.Path)
	}
	return paths
}

func writeCandidateSession(t *testing.T, path, id, timestamp, cwd, source string) {
	t.Helper()
	content := `{"timestamp":"` + timestamp + `","type":"session_meta","payload":{"id":"` + id + `","timestamp":"` + timestamp + `","cwd":"` + cwd + `","source":"` + source + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
