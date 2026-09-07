package indexer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDefaultIndexPathUsesCodexHome(t *testing.T) {
	home := filepath.Join("tmp", "codex-home")
	got := DefaultIndexPath(home)
	want := filepath.Join(home, ".codex-recall", "index.db")
	if got != want {
		t.Fatalf("DefaultIndexPath() = %q, want %q", got, want)
	}
}

func TestRefreshCreatesDefaultIndexAndSkipsUnchangedSession(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "session-refresh", "hello", "world")

	first, err := Refresh(context.Background(), home, RefreshOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.DatabasePath != DefaultIndexPath(home) {
		t.Fatalf("database path = %q, want %q", first.DatabasePath, DefaultIndexPath(home))
	}
	if first.Indexed != 1 || first.Skipped != 0 {
		t.Fatalf("first Refresh() = %#v", first)
	}
	if _, err := os.Stat(first.DatabasePath); err != nil {
		t.Fatalf("stat index database: %v", err)
	}

	second, err := Refresh(context.Background(), home, RefreshOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Indexed != 0 || second.Skipped != 1 {
		t.Fatalf("second Refresh() = %#v", second)
	}
	if second.Profile.Build.FilesHashed != 0 || second.Profile.Build.HashBytes != 0 {
		t.Fatalf("unchanged refresh hash profile = %#v", second.Profile.Build)
	}
	if second.Profile.Build.FingerprintFastPaths != 1 {
		t.Fatalf("unchanged refresh fingerprint profile = %#v", second.Profile.Build)
	}
}

func TestRefreshPersistsChangedFingerprintWhenHashIsUnchanged(t *testing.T) {
	home := t.TempDir()
	path := writeRollout(t, home, "session-refresh", "hello", "world")

	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changedTime := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}

	changed, err := Refresh(context.Background(), home, RefreshOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Indexed != 0 || changed.Skipped != 1 {
		t.Fatalf("changed fingerprint refresh = %#v", changed)
	}
	if changed.Profile.Build.FilesHashed != 1 || changed.Profile.Build.FilesDecoded != 0 {
		t.Fatalf("changed fingerprint profile = %#v", changed.Profile.Build)
	}
	if changed.Profile.Build.FingerprintUpdates != 1 {
		t.Fatalf("fingerprint updates = %d, want 1", changed.Profile.Build.FingerprintUpdates)
	}

	unchanged, err := Refresh(context.Background(), home, RefreshOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Profile.Build.FilesHashed != 0 || unchanged.Profile.Build.FingerprintFastPaths != 1 {
		t.Fatalf("persisted fingerprint profile = %#v", unchanged.Profile.Build)
	}
}

func TestRefreshCreatesCustomParentWithoutChangingItsMode(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(parent, "nested", "custom.db")

	result, err := Refresh(context.Background(), home, RefreshOptions{DatabasePath: custom})
	if err != nil {
		t.Fatal(err)
	}
	if result.DatabasePath != custom {
		t.Fatalf("database path = %q, want %q", result.DatabasePath, custom)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("stat custom database: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(parent)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("existing custom parent mode = %o, want 755", info.Mode().Perm())
		}
	}
}

func TestRefreshProtectsDefaultIndexDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix directory permission semantics")
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".codex-recall")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Refresh(context.Background(), home, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("default index directory mode = %o, want 700", info.Mode().Perm())
	}
}

func TestRefreshRejectsBlankHome(t *testing.T) {
	_, err := Refresh(context.Background(), "   ", RefreshOptions{})
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("Refresh() error = %v", err)
	}
}
