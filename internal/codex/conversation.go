package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// ConversationMessage is a user or assistant message extracted from a rollout.
type ConversationMessage struct {
	Timestamp time.Time
	Role      string
	Text      string
}

// ReadConversation returns user and assistant conversation messages from a
// rollout in file order. Tool output, reasoning, metadata, and malformed
// unrelated records are ignored. Adjacent duplicate representations of the
// same logical message are collapsed only when they come from different
// rollout record types (for example event_msg followed by response_item).
func ReadConversation(path string) ([]ConversationMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	messages := make([]ConversationMessage, 0)
	reader := bufio.NewReader(file)
	lastRole := ""
	lastText := ""
	lastRecordType := ""

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			var rec record
			if json.Unmarshal(bytes.TrimSpace(line), &rec) == nil {
				role, text := conversationText(rec)
				text = strings.TrimSpace(text)
				if role != "" && text != "" {
					normalized := normalizePreviewText(text)
					duplicateRepresentation := role == lastRole && normalized == lastText && rec.Type != lastRecordType
					if !duplicateRepresentation {
						timestamp, _ := parseTimestamp(rec.Timestamp)
						messages = append(messages, ConversationMessage{
							Timestamp: timestamp,
							Role:      role,
							Text:      text,
						})
					}
					lastRole = role
					lastText = normalized
					lastRecordType = rec.Type
				}
			}
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return messages, nil
}
