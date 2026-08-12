package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
