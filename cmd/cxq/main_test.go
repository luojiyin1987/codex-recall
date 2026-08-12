package main

import (
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

func TestVSCodeConversationURL(t *testing.T) {
	got, err := vscodeConversationURL("vscode", "019fe0cb-1234")
	if err != nil {
		t.Fatal(err)
	}
	want := "vscode://openai.chatgpt/local/019fe0cb-1234"
	if got != want {
		t.Fatalf("vscodeConversationURL() = %q, want %q", got, want)
	}

	got, err = vscodeConversationURL("vscode-insiders", "019fe0cb-1234")
	if err != nil {
		t.Fatal(err)
	}
	want = "vscode-insiders://openai.chatgpt/local/019fe0cb-1234"
	if got != want {
		t.Fatalf("vscodeConversationURL() = %q, want %q", got, want)
	}
}

func TestVSCodeConversationURLRejectsInvalidValues(t *testing.T) {
	if _, err := vscodeConversationURL("http", "019fe0cb"); err == nil {
		t.Fatal("vscodeConversationURL() accepted an invalid scheme")
	}
	if _, err := vscodeConversationURL("vscode", ""); err == nil {
		t.Fatal("vscodeConversationURL() accepted an empty session ID")
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
	if got := strings.TrimSpace(string(output)); got != realDir {
		t.Fatalf("probe cwd = %q, want %q", got, realDir)
	}

	other := exec.Command(shimPath, "/c", "echo ok")
	other.Dir = sessionDir
	other.Env = env
	output, err = other.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(output)); got != sessionDir {
		t.Fatalf("other command cwd = %q, want %q", got, sessionDir)
	}

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

func testEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
