package main

import (
	"os"
	"path/filepath"
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
