package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveOpenTarget(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		source    string
		want      string
		wantErr   bool
	}{
		{name: "auto vscode", requested: "auto", source: "vscode", want: "vscode"},
		{name: "auto cli", requested: "auto", source: "cli", want: "cli"},
		{name: "explicit vscode", requested: "vscode", source: "cli", want: "vscode"},
		{name: "explicit cli", requested: "cli", source: "vscode", want: "cli"},
		{name: "unknown source", requested: "auto", source: "exec", wantErr: true},
		{name: "invalid target", requested: "browser", source: "vscode", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveOpenTarget(test.requested, test.source)
			if (err != nil) != test.wantErr {
				t.Fatalf("resolveOpenTarget() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("resolveOpenTarget() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCLIRunnerListSearchAndShow(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ab"
	path := filepath.Join(home, "sessions", "2026", "08", "13", "rollout-2026-08-13T01-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-08-13T01:00:00Z","cwd":"/tmp/project","source":"vscode"}}`,
		`{"timestamp":"2026-08-13T01:01:00Z","type":"event_msg","payload":{"type":"user_message","message":"needle message"}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"list", "--home", home}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), sessionID) || stderr.Len() != 0 {
		t.Fatalf("list stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--home", home, "needle"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "needle message") || stderr.Len() != 0 {
		t.Fatalf("search stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"show", "--home", home, "019abc11"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "[user 2026-08-13") || !strings.Contains(stdout.String(), "needle message") {
		t.Fatalf("show stdout = %q", stdout.String())
	}
}

func TestCLIRunnerWritesUsageToInjectedStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run(nil); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestVSCodeConversationURL(t *testing.T) {
	const sessionID = "019fe0cb-1234-7abc-8def-0123456789ab"
	got, err := vscodeConversationURL("vscode", sessionID)
	if err != nil {
		t.Fatal(err)
	}
	want := "vscode://openai.chatgpt/local/" + sessionID
	if got != want {
		t.Fatalf("vscodeConversationURL() = %q, want %q", got, want)
	}

	got, err = vscodeConversationURL("vscode-insiders", sessionID)
	if err != nil {
		t.Fatal(err)
	}
	want = "vscode-insiders://openai.chatgpt/local/" + sessionID
	if got != want {
		t.Fatalf("vscodeConversationURL() = %q, want %q", got, want)
	}
}

func TestVSCodeConversationURLRejectsInvalidValues(t *testing.T) {
	const sessionID = "019fe0cb-1234-7abc-8def-0123456789ab"
	if _, err := vscodeConversationURL("http", sessionID); err == nil {
		t.Fatal("vscodeConversationURL() accepted an invalid scheme")
	}
	for _, id := range []string{
		"",
		"019fe0cb",
		"019fe0cb-1234-7abc-8def-0123456789ab&calc.exe",
		"019fe0cb-1234-7abc-8def-0123456789a&",
		"019fe0cb-1234-7abc-8def-0123456789a|",
		"019fe0cb-1234-7abc-8def-0123456789a<",
		"019fe0cb-1234-7abc-8def-0123456789a>",
		"019fe0cb-1234-7abc-8def-0123456789a^",
	} {
		if _, err := vscodeConversationURL("vscode", id); err == nil {
			t.Fatalf("vscodeConversationURL() accepted invalid session ID %q", id)
		}
	}
}

func TestValidCodexSessionID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{id: "019fe0cb-1234-7abc-8def-0123456789ab", want: true},
		{id: "019FE0CB-1234-7ABC-8DEF-0123456789AB", want: true},
		{id: "019fe0cb12347abc8def0123456789ab", want: false},
		{id: "019fe0cb-1234-7abc-8def-0123456789ag", want: false},
		{id: "019fe0cb-1234-7abc-8def-0123456789a&", want: false},
		{id: " 19fe0cb-1234-7abc-8def-0123456789ab", want: false},
	}
	for _, test := range tests {
		if got := validCodexSessionID(test.id); got != test.want {
			t.Errorf("validCodexSessionID(%q) = %v, want %v", test.id, got, test.want)
		}
	}
}

func TestNewOpenCommandWSL(t *testing.T) {
	cmdPath := "/mnt/c/Windows/System32/cmd.exe"
	got, err := newOpenCommand(
		"vscode://openai.chatgpt/local/019fe0cb",
		"linux",
		[]string{"WSL_DISTRO_NAME=Ubuntu"},
		func(name string) (string, error) {
			if name != "cmd.exe" {
				t.Fatalf("lookPath() name = %q, want cmd.exe", name)
			}
			return cmdPath, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != cmdPath {
		t.Fatalf("command name = %q, want %q", got.Name, cmdPath)
	}
	wantArgs := []string{"/d", "/s", "/c", "start", "", "vscode://openai.chatgpt/local/019fe0cb"}
	if strings.Join(got.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("command args = %q, want %q", got.Args, wantArgs)
	}
	if got.Dir != filepath.Dir(cmdPath) {
		t.Fatalf("command directory = %q, want %q", got.Dir, filepath.Dir(cmdPath))
	}
}

func TestNewOpenCommandVSCodeWSL(t *testing.T) {
	codePath := "/home/user/.vscode-server/bin/commit/bin/remote-cli/code"
	got, err := newOpenCommand(
		"vscode://openai.chatgpt/local/019fe0cb",
		"linux",
		[]string{"WSL_DISTRO_NAME=Ubuntu", "VSCODE_IPC_HOOK_CLI=/run/user/1000/vscode-ipc.sock"},
		func(name string) (string, error) {
			if name != "code" {
				t.Fatalf("lookPath() name = %q, want code", name)
			}
			return codePath, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != codePath {
		t.Fatalf("command name = %q, want %q", got.Name, codePath)
	}
	wantArgs := []string{"--openExternal", "vscode://openai.chatgpt/local/019fe0cb"}
	if strings.Join(got.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("command args = %q, want %q", got.Args, wantArgs)
	}
	if got.Dir != "" {
		t.Fatalf("command directory = %q, want empty", got.Dir)
	}
}

func TestNewOpenCommandVSCodeWSLFallsBackToWindows(t *testing.T) {
	cmdPath := "/mnt/c/Windows/System32/cmd.exe"
	var names []string
	got, err := newOpenCommand(
		"vscode://openai.chatgpt/local/019fe0cb",
		"linux",
		[]string{"WSL_DISTRO_NAME=Ubuntu", "VSCODE_IPC_HOOK_CLI=/run/user/1000/vscode-ipc.sock"},
		func(name string) (string, error) {
			names = append(names, name)
			if name == "code" {
				return "", errors.New("missing")
			}
			return cmdPath, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, "\x00") != "code\x00cmd.exe" {
		t.Fatalf("lookPath() names = %q, want code then cmd.exe", names)
	}
	if got.Name != cmdPath {
		t.Fatalf("command name = %q, want %q", got.Name, cmdPath)
	}
}

func TestNewOpenCommandNativePlatforms(t *testing.T) {
	conversationURL := "vscode://openai.chatgpt/local/019fe0cb"
	tests := []struct {
		goos     string
		opener   string
		wantPath string
		wantArgs []string
	}{
		{goos: "linux", opener: "xdg-open", wantPath: "/usr/bin/xdg-open", wantArgs: []string{conversationURL}},
		{goos: "darwin", opener: "open", wantPath: "/usr/bin/open", wantArgs: []string{conversationURL}},
		{goos: "windows", opener: "cmd.exe", wantPath: `C:\Windows\System32\cmd.exe`, wantArgs: []string{"/d", "/s", "/c", "start", "", conversationURL}},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			got, err := newOpenCommand(conversationURL, test.goos, nil, func(name string) (string, error) {
				if name != test.opener {
					t.Fatalf("lookPath() name = %q, want %q", name, test.opener)
				}
				return test.wantPath, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != test.wantPath {
				t.Fatalf("command name = %q, want %q", got.Name, test.wantPath)
			}
			if strings.Join(got.Args, "\x00") != strings.Join(test.wantArgs, "\x00") {
				t.Fatalf("command args = %q, want %q", got.Args, test.wantArgs)
			}
			if got.Dir != "" {
				t.Fatalf("command directory = %q, want empty", got.Dir)
			}
		})
	}
}

func TestNewOpenCommandReportsMissingOpener(t *testing.T) {
	_, err := newOpenCommand("vscode://openai.chatgpt/local/019fe0cb", "linux", nil, func(name string) (string, error) {
		return "", errors.New("missing")
	})
	if err == nil || !strings.Contains(err.Error(), "find xdg-open") {
		t.Fatalf("newOpenCommand() error = %v", err)
	}
}

func TestNewOpenCommandRejectsUnsupportedPlatform(t *testing.T) {
	_, err := newOpenCommand("vscode://openai.chatgpt/local/019fe0cb", "plan9", nil, func(name string) (string, error) {
		t.Fatal("newOpenCommand() searched for an opener on an unsupported platform")
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "cannot open VS Code on plan9") {
		t.Fatalf("newOpenCommand() error = %v", err)
	}
}

func TestResolveResumeDirExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveResumeDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("resolveResumeDir() = %q, want %q", got, dir)
	}
}

func TestResolveResumeDirEmpty(t *testing.T) {
	got, err := resolveResumeDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("resolveResumeDir() = %q, want empty", got)
	}
}

func TestResolveResumeDirMissing(t *testing.T) {
	_, err := resolveResumeDir(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("resolveResumeDir() accepted missing directory")
	}
}

func TestResolveResumeDirRejectsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := resolveResumeDir(path)
	if err == nil {
		t.Fatal("resolveResumeDir() accepted regular file")
	}
}

func TestWSLTermProgramProbeShimChangesOnlyProbeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test requires a POSIX shell")
	}

	realDir := filepath.Join(t.TempDir(), "Windows", "System32")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realCmd := filepath.Join(realDir, "cmd.exe")
	if err := os.WriteFile(realCmd, []byte("#!/bin/sh\npwd\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	env, cleanup, err := withWSLTermProgramProbeShim(
		[]string{"PATH=/usr/bin", "WSL_DISTRO_NAME=Ubuntu"},
		func(name string) (string, error) { return realCmd, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	shimDir := strings.Split(testEnvValue(env, "PATH"), string(os.PathListSeparator))[0]
	shimPath := filepath.Join(shimDir, "cmd.exe")

	sessionDir := t.TempDir()
	probe := exec.Command(shimPath, "/d", "/s", "/c", "set TERM_PROGRAM")
	probe.Dir = sessionDir
	probe.Env = env
	output, err := probe.Output()
	if err != nil {
		t.Fatal(err)
	}
	requireSameDirectory(t, strings.TrimSpace(string(output)), realDir, "probe")

	other := exec.Command(shimPath, "/c", "echo ok")
	other.Dir = sessionDir
	other.Env = env
	output, err = other.Output()
	if err != nil {
		t.Fatal(err)
	}
	requireSameDirectory(t, strings.TrimSpace(string(output)), sessionDir, "other command")

	cleanup()
	if _, err := os.Stat(shimDir); !os.IsNotExist(err) {
		t.Fatalf("shim directory still exists after cleanup: %v", err)
	}
}

func TestWSLTermProgramProbeShimSkipsNonWSL(t *testing.T) {
	wanted := []string{"PATH=/usr/bin"}
	called := false
	got, cleanup, err := withWSLTermProgramProbeShim(wanted, func(name string) (string, error) {
		called = true
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if called {
		t.Fatal("withWSLTermProgramProbeShim looked for cmd.exe outside WSL")
	}
	if strings.Join(got, "\x00") != strings.Join(wanted, "\x00") {
		t.Fatalf("environment changed: got %q, want %q", got, wanted)
	}
}

func requireSameDirectory(t *testing.T, got, want, label string) {
	t.Helper()
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat %s cwd %q: %v", label, got, err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat expected %s cwd %q: %v", label, want, err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("%s cwd = %q, want same directory as %q", label, got, want)
	}
}

func testEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}


func TestCLIRunnerIndexRefreshesDerivedSQLite(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ab"
	path := filepath.Join(home, "sessions", "2026", "09", "04", "rollout-2026-09-04T04-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"timestamp":"2026-09-04T04:00:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-09-04T04:00:00Z","cwd":"/tmp/project","source":"vscode"}}`,
		`{"timestamp":"2026-09-04T04:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"indexed message"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"index", "--home", home}); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(home, ".codex-recall", "index.db")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("index database was not created: %v", err)
	}
	if !strings.Contains(stdout.String(), "INDEXED") || !strings.Contains(stdout.String(), "1") || stderr.Len() != 0 {
		t.Fatalf("first index stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"index", "--home", home}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "SKIPPED") || !strings.Contains(stdout.String(), "1") || stderr.Len() != 0 {
		t.Fatalf("second index stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestCLIRunnerIndexSupportsCustomDatabasePath(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(t.TempDir(), "nested", "custom.db")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"index", "--home", home, "--db", custom}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("custom index database was not created: %v", err)
	}
	if !strings.Contains(stdout.String(), custom) || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}


func TestCLIRunnerSearchIndexUsesDerivedFTSWithoutRefreshing(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ab"
	path := filepath.Join(home, "sessions", "2026", "09", "04", "rollout-2026-09-04T07-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"timestamp":"2026-09-04T07:00:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-09-04T07:00:00Z","cwd":"/tmp/project","source":"vscode"}}`,
		`{"timestamp":"2026-09-04T07:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"indexed WebRTC transport"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"index", "--home", home}); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--index", "--home", home, "WebRTC transport"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), sessionID) || !strings.Contains(stdout.String(), "indexed WebRTC transport") {
		t.Fatalf("indexed search stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("indexed search stderr = %q", stderr.String())
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(`{"timestamp":"2026-09-04T07:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"fresh unindexed phrase"}}` + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--index", "--home", home, "fresh unindexed"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), sessionID) {
		t.Fatalf("indexed search refreshed implicitly: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--home", home, "fresh unindexed"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), sessionID) {
		t.Fatalf("live search did not see appended rollout content: %q", stdout.String())
	}
}

func TestCLIRunnerSearchIndexRequiresExistingDatabase(t *testing.T) {
	home := t.TempDir()
	indexPath := filepath.Join(home, ".codex-recall", "index.db")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	err := runner.run([]string{"search", "--index", "--home", home, "needle"})
	if err == nil || !strings.Contains(err.Error(), "run cxq index") {
		t.Fatalf("indexed search error = %v", err)
	}
	if _, statErr := os.Stat(indexPath); !os.IsNotExist(statErr) {
		t.Fatalf("indexed search created database unexpectedly: %v", statErr)
	}
}

func TestCLIRunnerSearchIndexSupportsCustomDatabase(t *testing.T) {
	home := t.TempDir()
	sessionID := "019abc11-1234-7abc-8def-0123456789ab"
	path := filepath.Join(home, "sessions", "2026", "09", "04", "rollout-2026-09-04T07-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"timestamp":"2026-09-04T07:00:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","timestamp":"2026-09-04T07:00:00Z","cwd":"/tmp/project","source":"cli"}}`,
		`{"timestamp":"2026-09-04T07:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"custom database needle"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(t.TempDir(), "nested", "custom.db")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"index", "--home", home, "--db", custom}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"search", "--index", "--db", custom, "--home", home, "custom database"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), sessionID) {
		t.Fatalf("custom indexed search stdout = %q", stdout.String())
	}
}

func TestCLIRunnerSearchRejectsDatabaseWithoutIndexMode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	err := runner.run([]string{"search", "--db", "index.db", "needle"})
	if err == nil || !strings.Contains(err.Error(), "--db requires --index") {
		t.Fatalf("search error = %v", err)
	}
}


func TestCLIRunnerCompareReportsBackendDifferences(t *testing.T) {
	home := t.TempDir()
	writeCompareRollout := func(id, text string) {
		t.Helper()
		path := filepath.Join(home, "sessions", "2026", "09", "04", "rollout-2026-09-04T08-00-00-"+id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := strings.Join([]string{
			`{"timestamp":"2026-09-04T08:00:00Z","type":"session_meta","payload":{"id":"` + id + `","timestamp":"2026-09-04T08:00:00Z","cwd":"/tmp/demo","source":"vscode"}}`,
			`{"timestamp":"2026-09-04T08:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"` + text + `"}}`,
		}, "\n") + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeCompareRollout("both-session", "plain foo bar phrase")
	writeCompareRollout("live-session", "prefix xfoo barz suffix")
	writeCompareRollout("index-session", "punctuated foo-bar phrase")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	if err := runner.run([]string{"index", "--home", home}); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.run([]string{"compare", "--home", home, "foo bar"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{
		"OVERLAP", "1",
		"LIVE_ONLY", "1",
		"INDEX_ONLY", "1",
		"both-session",
		"live-session",
		"index-session",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("compare output missing %q: %q", want, output)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("compare stderr = %q", stderr.String())
	}
}

func TestCLIRunnerCompareRequiresExistingIndex(t *testing.T) {
	home := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
	err := runner.run([]string{"compare", "--home", home, "needle"})
	if err == nil || !strings.Contains(err.Error(), "run cxq index") {
		t.Fatalf("compare error = %v", err)
	}
}
