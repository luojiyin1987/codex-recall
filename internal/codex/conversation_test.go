package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConversationReturnsUserAssistantAndIgnoresToolNoise(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "" +
		`{"timestamp":"2026-08-12T03:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello world"}]}}` + "\n" +
		`{"timestamp":"2026-08-12T03:00:01Z","type":"response_item","payload":{"type":"function_call_output","call_id":"x","output":"tool noise"}}` + "\n" +
		`{"timestamp":"2026-08-12T03:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello back"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Text != "hello world" {
		t.Fatalf("messages[0] = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Text != "hello back" {
		t.Fatalf("messages[1] = %#v", messages[1])
	}
	if messages[0].Timestamp.IsZero() || messages[1].Timestamp.IsZero() {
		t.Fatal("timestamps were not parsed")
	}
}

func TestReadConversationSupportsLegacyEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "" +
		`{"timestamp":"2026-02-22T12:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"legacy user"}}` + "\n" +
		`{"timestamp":"2026-02-22T12:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"legacy assistant"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestReadConversationCollapsesAdjacentDuplicateRepresentations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "" +
		`{"timestamp":"2026-08-12T03:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"same message"}}` + "\n" +
		`{"timestamp":"2026-08-12T03:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"same   message"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
}

func TestReadConversationKeepsRepeatedSameTypeMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "" +
		`{"timestamp":"2026-08-12T03:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"repeat me"}}` + "\n" +
		`{"timestamp":"2026-08-12T03:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"repeat me"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
}

func TestReadConversationIgnoresMalformedUnrelatedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := "not-json\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"still works"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Text != "still works" {
		t.Fatalf("messages = %#v", messages)
	}
}
