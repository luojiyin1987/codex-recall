package codex

import (
	"context"
	"encoding/json"
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
	return ReadConversationContext(context.Background(), path)
}

// ReadConversationContext is ReadConversation with cooperative cancellation.
func ReadConversationContext(ctx context.Context, path string) ([]ConversationMessage, error) {
	messages, _, err := ReadConversationContextMeasured(ctx, path)
	return messages, err
}

// ReadConversationContextMeasured also returns bytes read by the decoder.
func ReadConversationContextMeasured(ctx context.Context, path string) ([]ConversationMessage, int64, error) {
	messages := make([]ConversationMessage, 0)
	lastRole := ""
	lastText := ""
	lastRecordType := ""

	bytesRead, err := visitRolloutFileContextFilteredMeasured(ctx, path, isConversationRecord, func(rec record) (bool, error) {
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
		return nil, bytesRead, err
	}
	return messages, bytesRead, nil
}

// conversationRecordProbe keeps only the type fields that select a
// conversation record. It keeps no payload bytes.
type conversationRecordProbe struct {
	Type    string `json:"type"`
	Payload struct {
		Type string `json:"type"`
	} `json:"payload"`
}

// conversationEventRole returns the conversation role for an event_msg payload
// type. It returns "" when the payload type is not a conversation record. It is
// the single source of truth for event message roles.
func conversationEventRole(payloadType string) string {
	switch payloadType {
	case "user_message":
		return "user"
	case "agent_message":
		return "assistant"
	default:
		return ""
	}
}

// isConversationPayloadType reports whether a record can contribute a
// conversation message. It is the single source of truth for conversation
// membership. The cheap probe in isConversationRecord and the full decode in
// conversationText both use it. A new record shape cannot drift between the
// filter and the parser, because the filter cannot drop a record that the
// parser supports.
func isConversationPayloadType(recordType, payloadType string) bool {
	switch recordType {
	case "response_item":
		return payloadType == "message"
	case "event_msg":
		return conversationEventRole(payloadType) != ""
	default:
		return false
	}
}

// isConversationRecord reports whether a trimmed rollout line can contribute a
// conversation message. It decodes only the record and payload type. It keeps
// no payload bytes, so the decoder does not copy large reasoning and tool
// output. The filter is conservative: it returns true for every record that
// isConversationPayloadType accepts.
func isConversationRecord(line []byte) bool {
	var probe conversationRecordProbe
	if err := json.Unmarshal(line, &probe); err != nil {
		return false
	}
	return isConversationPayloadType(probe.Type, probe.Payload.Type)
}
