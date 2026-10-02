package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luojiyin1987/codex-recall/internal/textutil"
)

// decodeBenchmarkShape controls the synthetic rollout mix. messageBytes sets
// the size of indexable chat text. toolBytes sets the size of tool and
// reasoning text that the decoder reads but never indexes.
type decodeBenchmarkShape struct {
	name         string
	turns        int
	messageBytes int
	toolBytes    int
}

// decodeBenchmarkShapes returns the shared synthetic files. message-heavy
// stresses indexable text. tool-heavy stresses payload bytes that the decoder
// reads but never indexes.
func decodeBenchmarkShapes() []decodeBenchmarkShape {
	return []decodeBenchmarkShape{
		{name: "mixed", turns: 120, messageBytes: 2048, toolBytes: 8192},
		{name: "message-heavy", turns: 120, messageBytes: 8192, toolBytes: 1024},
		{name: "tool-heavy", turns: 120, messageBytes: 512, toolBytes: 16384},
	}
}

// BenchmarkConversationDecodeLayers splits the decoder pipeline into layers.
// Subtract adjacent layers to get incremental phase cost:
//
//	CONVERSATION_READ = scan-only
//	RECORD_DECODE     = scan-envelope          - scan-only
//	PAYLOAD_DECODE    = scan-envelope-payload  - scan-envelope
//	TEXT_PROCESS      = full                   - scan-envelope-payload
//	TIMESTAMP_PARSE   = BenchmarkParseTimestamp x messages/op
//
// Every layer reads the same file from the page cache. The scan-only layer
// mirrors production: a bufio.Reader driven by ReadBytes('\n').
func BenchmarkConversationDecodeLayers(b *testing.B) {
	for _, shape := range decodeBenchmarkShapes() {
		path, size := writeDecodeBenchmarkRollout(b, shape)
		records := countDecodeBenchmarkRecords(b, path)
		messages := countDecodeBenchmarkMessages(b, path)

		b.Run(shape.name+"/scan-only", func(b *testing.B) {
			benchmarkDecodeLayer(b, size, func() (int, error) {
				return scanDecodeBenchmarkLines(path)
			})
			reportDecodeLayerMetrics(b, size, records, 0)
		})
		b.Run(shape.name+"/scan-envelope", func(b *testing.B) {
			benchmarkDecodeLayer(b, size, func() (int, error) {
				return decodeBenchmarkEnvelope(path)
			})
			reportDecodeLayerMetrics(b, size, records, 0)
		})
		b.Run(shape.name+"/scan-envelope-payload", func(b *testing.B) {
			benchmarkDecodeLayer(b, size, func() (int, error) {
				return decodeBenchmarkEnvelopePayload(path)
			})
			reportDecodeLayerMetrics(b, size, records, 0)
		})
		b.Run(shape.name+"/full", func(b *testing.B) {
			benchmarkDecodeLayer(b, size, func() (int, error) {
				decoded, _, err := ReadConversationContextMeasured(context.Background(), path)
				if err != nil {
					return 0, err
				}
				if len(decoded) != messages {
					return 0, fmt.Errorf("decoded %d messages, want %d", len(decoded), messages)
				}
				return len(decoded), nil
			})
			reportDecodeLayerMetrics(b, size, records, messages)
		})
	}
}

// BenchmarkConversationDecodeRealFile layers the same phases over one real
// rollout file. Set CODEX_RECALL_DECODE_BENCH_FILE to the file path. Use
// -benchtime=1x for large files.
func BenchmarkConversationDecodeRealFile(b *testing.B) {
	path := os.Getenv("CODEX_RECALL_DECODE_BENCH_FILE")
	if path == "" {
		b.Skip("set CODEX_RECALL_DECODE_BENCH_FILE to a rollout JSONL file")
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	size := info.Size()
	records := countDecodeBenchmarkRecords(b, path)
	messages := countDecodeBenchmarkMessages(b, path)

	b.Run("scan-only", func(b *testing.B) {
		benchmarkDecodeLayer(b, size, func() (int, error) {
			return scanDecodeBenchmarkLines(path)
		})
		reportDecodeLayerMetrics(b, size, records, 0)
	})
	b.Run("scan-envelope", func(b *testing.B) {
		benchmarkDecodeLayer(b, size, func() (int, error) {
			return decodeBenchmarkEnvelope(path)
		})
		reportDecodeLayerMetrics(b, size, records, 0)
	})
	b.Run("scan-envelope-payload", func(b *testing.B) {
		benchmarkDecodeLayer(b, size, func() (int, error) {
			return decodeBenchmarkEnvelopePayload(path)
		})
		reportDecodeLayerMetrics(b, size, records, 0)
	})
	b.Run("full", func(b *testing.B) {
		benchmarkDecodeLayer(b, size, func() (int, error) {
			decoded, _, err := ReadConversationContextMeasured(context.Background(), path)
			if err != nil {
				return 0, err
			}
			if len(decoded) != messages {
				return 0, fmt.Errorf("decoded %d messages, want %d", len(decoded), messages)
			}
			return len(decoded), nil
		})
		reportDecodeLayerMetrics(b, size, records, messages)
	})
}

// BenchmarkConversationDecodeStrategies compares two decode strategies.
//
//	current:   the production path. It decodes the full envelope with a
//	           json.RawMessage payload. It copies the payload for every record.
//	selective: it probes the record and payload type first. It decodes the
//	           payload only for message records.
func BenchmarkConversationDecodeStrategies(b *testing.B) {
	for _, shape := range decodeBenchmarkShapes() {
		path, size := writeDecodeBenchmarkRollout(b, shape)
		messages := countDecodeBenchmarkMessages(b, path)

		b.Run(shape.name+"/current", func(b *testing.B) {
			benchmarkDecodeStrategy(b, size, messages, func() (int, error) {
				decoded, _, err := ReadConversationContextMeasured(context.Background(), path)
				if err != nil {
					return 0, err
				}
				return len(decoded), nil
			})
		})
		b.Run(shape.name+"/selective", func(b *testing.B) {
			benchmarkDecodeStrategy(b, size, messages, func() (int, error) {
				decoded, _, err := decodeConversationSelective(context.Background(), path)
				if err != nil {
					return 0, err
				}
				return len(decoded), nil
			})
		})
	}
}

// BenchmarkConversationDecodeRealFileStrategies runs the same A/B over one
// real rollout file. Set CODEX_RECALL_DECODE_BENCH_FILE to the file path. Use
// -benchtime=1x for large files.
func BenchmarkConversationDecodeRealFileStrategies(b *testing.B) {
	path := os.Getenv("CODEX_RECALL_DECODE_BENCH_FILE")
	if path == "" {
		b.Skip("set CODEX_RECALL_DECODE_BENCH_FILE to a rollout JSONL file")
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	size := info.Size()
	messages := countDecodeBenchmarkMessages(b, path)

	b.Run("current", func(b *testing.B) {
		benchmarkDecodeStrategy(b, size, messages, func() (int, error) {
			decoded, _, err := ReadConversationContextMeasured(context.Background(), path)
			if err != nil {
				return 0, err
			}
			return len(decoded), nil
		})
	})
	b.Run("selective", func(b *testing.B) {
		benchmarkDecodeStrategy(b, size, messages, func() (int, error) {
			decoded, _, err := decodeConversationSelective(context.Background(), path)
			if err != nil {
				return 0, err
			}
			return len(decoded), nil
		})
	})
}

func benchmarkDecodeStrategy(b *testing.B, size int64, wantMessages int, run func() (int, error)) {
	b.Helper()
	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := run()
		if err != nil {
			b.Fatal(err)
		}
		if got != wantMessages {
			b.Fatalf("decoded %d messages, want %d", got, wantMessages)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(wantMessages), "messages/op")
}

// recordProbe is the minimal record discriminator. It decodes only the record
// type and the payload type. It keeps no payload bytes, so the decode does not
// copy large tool output. A message record gets its timestamp from the second
// decode of the line.
type recordProbe struct {
	Type    string `json:"type"`
	Payload struct {
		Type string `json:"type"`
	} `json:"payload"`
}

// conversationTextSelective probes the record kind first. It decodes the
// payload only for message records. The output matches conversationText. It
// costs a second parse of the line for message records.
func conversationTextSelective(line []byte) (role, text, timestamp, recordType string) {
	var probe recordProbe
	if err := json.Unmarshal(line, &probe); err != nil {
		return "", "", "", ""
	}
	switch probe.Type {
	case "response_item":
		if probe.Payload.Type != "message" {
			return "", "", "", ""
		}
	case "event_msg":
		if probe.Payload.Type != "user_message" && probe.Payload.Type != "agent_message" {
			return "", "", "", ""
		}
	default:
		return "", "", "", ""
	}

	var rec record
	if err := json.Unmarshal(line, &rec); err != nil {
		return "", "", "", ""
	}
	role, text = conversationText(rec)
	return role, text, rec.Timestamp, rec.Type
}

// decodeConversationSelective decodes a rollout with the selective strategy.
// It is benchmark-only. It must return the same messages as
// ReadConversationContextMeasured.
func decodeConversationSelective(ctx context.Context, path string) ([]ConversationMessage, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	counter := &byteCountingReader{reader: file}
	reader := bufio.NewReader(counter)
	messages := make([]ConversationMessage, 0)
	lastRole := ""
	lastText := ""
	lastRecordType := ""

	for {
		if err := ctx.Err(); err != nil {
			return nil, counter.bytesRead, err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			role, text, timestampValue, recordType := conversationTextSelective(bytes.TrimSpace(line))
			text = strings.TrimSpace(text)
			if role != "" && text != "" {
				normalized := textutil.NormalizeWhitespace(text)
				duplicateRepresentation := role == lastRole && normalized == lastText && recordType != lastRecordType
				if !duplicateRepresentation {
					timestamp, _ := parseTimestamp(timestampValue)
					messages = append(messages, ConversationMessage{Timestamp: timestamp, Role: role, Text: text})
				}
				lastRole = role
				lastText = normalized
				lastRecordType = recordType
			}
		}
		if errors.Is(readErr, io.EOF) {
			return messages, counter.bytesRead, nil
		}
		if readErr != nil {
			return nil, counter.bytesRead, readErr
		}
	}
}

func reportDecodeLayerMetrics(b *testing.B, size int64, records, messages int) {
	b.Helper()
	b.ReportMetric(float64(size), "file-bytes")
	b.ReportMetric(float64(records), "records/op")
	if messages > 0 {
		b.ReportMetric(float64(messages), "messages/op")
	}
}

// BenchmarkParseTimestamp measures the timestamp phase in isolation.
func BenchmarkParseTimestamp(b *testing.B) {
	values := []string{
		"2026-09-04T08:00:00Z",
		"2026-09-04T08:00:00.123456789Z",
	}
	for _, value := range values {
		b.Run(value, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := parseTimestamp(value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkNormalizeWhitespace measures the dedup normalization cost on
// representative message text sizes.
func BenchmarkNormalizeWhitespace(b *testing.B) {
	for _, size := range []int{64, 2048, 8192} {
		text := decodeBenchmarkText("message", size)
		b.Run(fmt.Sprintf("bytes=%d", len(text)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = textutil.NormalizeWhitespace(text)
			}
		})
	}
}

func benchmarkDecodeLayer(b *testing.B, size int64, run func() (int, error)) {
	b.Helper()
	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := run(); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

// scanDecodeBenchmarkLines reads raw lines only. It is the CONVERSATION_READ
// layer plus the byte scanning that ReadBytes performs.
func scanDecodeBenchmarkLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	lines := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lines++
		}
		if errors.Is(readErr, io.EOF) {
			return lines, nil
		}
		if readErr != nil {
			return lines, readErr
		}
	}
}

// decodeBenchmarkEnvelope decodes only the record envelope. It is the
// RECORD_DECODE layer on top of CONVERSATION_READ.
func decodeBenchmarkEnvelope(path string) (int, error) {
	records := 0
	err := visitRolloutFileContext(context.Background(), path, func(record) (bool, error) {
		records++
		return false, nil
	})
	return records, err
}

// decodeBenchmarkEnvelopePayload decodes the envelope and the relevant
// payload. It is the PAYLOAD_DECODE layer on top of RECORD_DECODE. It uses
// conversationText, so it also pays content assembly for message records.
func decodeBenchmarkEnvelopePayload(path string) (int, error) {
	records := 0
	err := visitRolloutFileContext(context.Background(), path, func(rec record) (bool, error) {
		if role, text := conversationText(rec); role != "" && text != "" {
			records++
		}
		return false, nil
	})
	return records, err
}

func countDecodeBenchmarkRecords(b *testing.B, path string) int {
	b.Helper()
	records, err := decodeBenchmarkEnvelope(path)
	if err != nil {
		b.Fatal(err)
	}
	return records
}

func countDecodeBenchmarkMessages(b *testing.B, path string) int {
	b.Helper()
	messages, _, err := ReadConversationContextMeasured(context.Background(), path)
	if err != nil {
		b.Fatal(err)
	}
	return len(messages)
}

// writeDecodeBenchmarkRollout writes one synthetic Codex rollout file. Each
// turn emits adjacent duplicate representations of the user and assistant
// messages, then tool, reasoning, and metadata noise. The noise records make
// the envelope-decode cost realistic.
func writeDecodeBenchmarkRollout(b *testing.B, shape decodeBenchmarkShape) (string, int64) {
	b.Helper()

	path := filepath.Join(b.TempDir(), shape.name+".jsonl")
	content := decodeBenchmarkRolloutContent(shape)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
	return path, int64(len(content))
}

// decodeBenchmarkRolloutContent builds one synthetic rollout as a string. The
// test for selective decode reuses it to stay in sync with the benchmark.
func decodeBenchmarkRolloutContent(shape decodeBenchmarkShape) string {
	messageText := decodeBenchmarkText("conversation message text", shape.messageBytes)
	toolText := decodeBenchmarkText("tool output payload", shape.toolBytes)

	var builder strings.Builder
	builder.WriteString(`{"timestamp":"2026-09-04T08:00:00Z","type":"session_meta","payload":{"id":"decode-bench","timestamp":"2026-09-04T08:00:00Z","cwd":"/work/project","source":"vscode"}}`)
	builder.WriteByte('\n')

	for turn := 0; turn < shape.turns; turn++ {
		userText := fmt.Sprintf("user turn %d %s", turn, messageText)
		assistantText := fmt.Sprintf("assistant turn %d %s", turn, messageText)
		records := []string{
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:01Z","type":"event_msg","payload":{"type":"user_message","message":%q}}`, userText),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}}`, userText),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":%q}}`, assistantText),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:04Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":%q}]}}`, assistantText),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:05Z","type":"response_item","payload":{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":%q}]}}`, toolText),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:06Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":%q,"call_id":"call-%d"}}`, toolText, turn),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:07Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-%d","output":%q}}`, turn, toolText),
			fmt.Sprintf(`{"timestamp":"2026-09-04T08:00:08Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":%d,"output_tokens":%d}}}}`, turn, turn),
			`{"timestamp":"2026-09-04T08:00:09Z","type":"turn_context","payload":{"cwd":"/work/project","approval_policy":"never","sandbox_policy":{"mode":"workspace-write"},"model":"gpt-5-codex"}}`,
		}
		for _, record := range records {
			builder.WriteString(record)
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}

func decodeBenchmarkText(prefix string, size int) string {
	if size <= len(prefix) {
		return prefix
	}
	const filler = "lorem ipsum dolor sit amet "
	repeats := (size - len(prefix)) / len(filler)
	if repeats < 1 {
		repeats = 1
	}
	return prefix + " " + strings.Repeat(filler, repeats)
}

// TestDecodeConversationSelectiveMatchesCurrent guards the benchmark-only
// selective decoder against the production decoder.
func TestDecodeConversationSelectiveMatchesCurrent(t *testing.T) {
	shape := decodeBenchmarkShape{name: "equivalence", turns: 8, messageBytes: 256, toolBytes: 1024}
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	content := decodeBenchmarkRolloutContent(shape) + "not-json\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	want, wantBytes, err := ReadConversationContextMeasured(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	got, gotBytes, err := decodeConversationSelective(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if gotBytes != wantBytes {
		t.Fatalf("bytes read = %d, want %d", gotBytes, wantBytes)
	}
	if len(got) != len(want) {
		t.Fatalf("messages = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("messages[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
