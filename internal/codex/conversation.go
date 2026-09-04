package codex

import (
	"strings"
	"time"

	"github.com/luojiyin1987/codex-recall/internal/textutil"
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
	messages := make([]ConversationMessage, 0)
	lastRole := ""
	lastText := ""
	lastRecordType := ""

	err := visitRolloutFile(path, func(rec record) (bool, error) {
		role, text := conversationText(rec)
		text = strings.TrimSpace(text)
		if role == "" || text == "" {
			return false, nil
		}
		normalized := textutil.NormalizeWhitespace(text)
		duplicateRepresentation := role == lastRole && normalized == lastText && rec.Type != lastRecordType
		if !duplicateRepresentation {
			timestamp, _ := parseTimestamp(rec.Timestamp)
			messages = append(messages, ConversationMessage{Timestamp: timestamp, Role: role, Text: text})
		}
		lastRole = role
		lastText = normalized
		lastRecordType = rec.Type
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return messages, nil
}
