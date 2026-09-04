package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luojiyin1987/codex-recall/internal/index"
)

type fakeStore struct {
	sessions     map[string]index.Session
	messages     map[string][]index.Message
	replacements int
	batchCalls   int
	batchSizes   []int
	deletions    int
	listCalls    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		sessions: make(map[string]index.Session),
		messages: make(map[string][]index.Message),
	}
}

func (f *fakeStore) Sessions(_ context.Context) ([]index.Session, error) {
	f.listCalls++
	sessions := make([]index.Session, 0, len(f.sessions))
	for _, session := range f.sessions {
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, id string) error {
	delete(f.sessions, id)
	delete(f.messages, id)
	f.deletions++
	return nil
}

func (f *fakeStore) ReplaceSessions(_ context.Context, replacements []index.SessionReplacement) error {
	f.batchCalls++
	f.batchSizes = append(f.batchSizes, len(replacements))
	for _, replacement := range replacements {
		session := replacement.Session
		f.sessions[session.ID] = session
		f.messages[session.ID] = append([]index.Message(nil), replacement.Messages...)
		f.replacements++
	}
	return nil
}

func TestBuildIndexesLogicalSession(t *testing.T) {
	home := t.TempDir()
	path := writeRollout(t, home, "session-1", "hello", "hello back")
	store := newFakeStore()

	result, err := Build(context.Background(), home, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Discovered != 1 || result.Indexed != 1 || result.Skipped != 0 || len(result.Warnings) != 0 {
		t.Fatalf("Build() result = %#v", result)
	}
	if store.replacements != 1 {
		t.Fatalf("replacements = %d, want 1", store.replacements)
	}

	session := store.sessions["session-1"]
	if session.Project != "demo" || session.Source != "vscode" || session.RolloutPath != path {
		t.Fatalf("indexed session = %#v", session)
	}
	if !strings.HasPrefix(session.ContentHash, "v1:sha256:") {
		t.Fatalf("content hash = %q", session.ContentHash)
	}

	messages := store.messages["session-1"]
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if messages[0].Ordinal != 0 || messages[0].Role != "user" || messages[0].Text != "hello" {
		t.Fatalf("messages[0] = %#v", messages[0])
	}
	if messages[1].Ordinal != 1 || messages[1].Role != "assistant" || messages[1].Text != "hello back" {
		t.Fatalf("messages[1] = %#v", messages[1])
	}
	if messages[0].Timestamp == nil || messages[1].Timestamp == nil {
		t.Fatal("message timestamps were not retained")
	}
}

func TestBuildSkipsUnchangedContent(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "session-1", "hello", "hello back")
	store := newFakeStore()

	first, err := Build(context.Background(), home, store)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(context.Background(), home, store)
	if err != nil {
		t.Fatal(err)
	}

	if first.Indexed != 1 {
		t.Fatalf("first Build() = %#v", first)
	}
	if second.Indexed != 0 || second.Skipped != 1 {
		t.Fatalf("second Build() = %#v", second)
	}
	if store.replacements != 1 {
		t.Fatalf("replacements = %d, want 1", store.replacements)
	}
}

func TestBuildReindexesChangedContent(t *testing.T) {
	home := t.TempDir()
	path := writeRollout(t, home, "session-1", "before", "answer")
	store := newFakeStore()

	if _, err := Build(context.Background(), home, store); err != nil {
		t.Fatal(err)
	}
	oldHash := store.sessions["session-1"].ContentHash

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(`{"timestamp":"2026-09-04T04:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"changed"}]}}` + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	result, err := Build(context.Background(), home, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Indexed != 1 || result.Skipped != 0 {
		t.Fatalf("Build() = %#v", result)
	}
	if store.replacements != 2 {
		t.Fatalf("replacements = %d, want 2", store.replacements)
	}
	if store.sessions["session-1"].ContentHash == oldHash {
		t.Fatal("content hash did not change")
	}
	messages := store.messages["session-1"]
	if len(messages) != 3 || messages[2].Text != "changed" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestBuildPreservesCatalogWarnings(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "broken.jsonl")
	if err := os.WriteFile(bad, []byte(`{"type":"session_meta","payload":{"id":""}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Build(context.Background(), home, newFakeStore())
	if err != nil {
		t.Fatal(err)
	}
	if result.Discovered != 0 || len(result.Warnings) != 1 {
		t.Fatalf("Build() = %#v", result)
	}
}

func TestBuildHonorsCanceledContext(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "session-1", "hello", "world")
	store := newFakeStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Build(ctx, home, store)
	if err == nil {
		t.Fatal("Build() ignored canceled context")
	}
	if store.replacements != 0 {
		t.Fatalf("replacements = %d, want 0", store.replacements)
	}
}

func writeRollout(t *testing.T, home, id, userText, assistantText string) string {
	t.Helper()
	root := filepath.Join(home, "sessions", "2026", "09", "04")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "rollout-2026-09-04T04-00-00-"+id+".jsonl")
	content := strings.Join([]string{
		`{"timestamp":"2026-09-04T04:00:00Z","type":"session_meta","payload":{"id":"` + id + `","timestamp":"2026-09-04T04:00:00Z","cwd":"/work/demo","source":"vscode"}}`,
		`{"timestamp":"2026-09-04T04:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + userText + `"}]}}`,
		`{"timestamp":"2026-09-04T04:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + assistantText + `"}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHashRolloutIsStableAndVersioned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte("same bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := hashRollout(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashRollout(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("hashes differ: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, contentHashVersion+":sha256:") {
		t.Fatalf("hash = %q", first)
	}
	if len(first) != len(contentHashVersion+":sha256:")+64 {
		t.Fatalf("hash length = %d", len(first))
	}
}


func TestBuildDeletesStaleIndexedSessionsWhenCatalogIsClean(t *testing.T) {
	home := t.TempDir()
	path := writeRollout(t, home, "current", "hello", "world")
	store := newFakeStore()
	store.sessions["stale"] = index.Session{
		ID:          "stale",
		RolloutPath: "/old/stale.jsonl",
		ContentHash: "v1:sha256:stale",
	}
	store.messages["stale"] = []index.Message{{SessionID: "stale", Ordinal: 0, Role: "user", Text: "old"}}

	result, err := Build(context.Background(), home, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("Deleted = %d, want 1", result.Deleted)
	}
	if _, ok := store.sessions["stale"]; ok {
		t.Fatal("stale indexed session was not deleted")
	}
	if _, ok := store.sessions["current"]; !ok {
		t.Fatal("current session was not indexed")
	}
	if store.deletions != 1 || store.listCalls != 1 {
		t.Fatalf("deletions = %d, listCalls = %d", store.deletions, store.listCalls)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("current rollout disappeared: %v", err)
	}
}

func TestBuildSkipsStaleDeletionWhenCatalogHasWarnings(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "current", "hello", "world")
	root := filepath.Join(home, "sessions")
	bad := filepath.Join(root, "broken.jsonl")
	if err := os.WriteFile(bad, []byte(`{"type":"session_meta","payload":{"id":""}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newFakeStore()
	store.sessions["possibly-stale"] = index.Session{
		ID:          "possibly-stale",
		RolloutPath: "/old/possibly-stale.jsonl",
		ContentHash: "v1:sha256:possibly-stale",
	}

	result, err := Build(context.Background(), home, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("Build() did not preserve catalog warning")
	}
	if result.Deleted != 0 || store.deletions != 0 {
		t.Fatalf("stale deletion ran despite catalog warning: %#v", result)
	}
	if store.listCalls != 1 {
		t.Fatalf("Sessions() called %d time(s), want 1 preload", store.listCalls)
	}
	if _, ok := store.sessions["possibly-stale"]; !ok {
		t.Fatal("possibly stale session was deleted despite catalog warning")
	}
}

func TestBuildWritesSessionsInBoundedBatches(t *testing.T) {
	home := t.TempDir()
	for i := 0; i < indexWriteBatchSize+1; i++ {
		id := fmt.Sprintf("session-%03d", i)
		writeRollout(t, home, id, "hello", "world")
	}
	store := newFakeStore()

	result, err := Build(context.Background(), home, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Indexed != indexWriteBatchSize+1 {
		t.Fatalf("Indexed = %d, want %d", result.Indexed, indexWriteBatchSize+1)
	}
	if store.batchCalls != 2 {
		t.Fatalf("batchCalls = %d, want 2", store.batchCalls)
	}
	if len(store.batchSizes) != 2 || store.batchSizes[0] != indexWriteBatchSize || store.batchSizes[1] != 1 {
		t.Fatalf("batchSizes = %#v, want [%d 1]", store.batchSizes, indexWriteBatchSize)
	}
	if store.listCalls != 1 {
		t.Fatalf("listCalls = %d, want 1", store.listCalls)
	}
}
