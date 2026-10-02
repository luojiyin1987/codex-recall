package codex

import (
	"bufio"
	"context"
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
	shapes := []decodeBenchmarkShape{
		{name: "mixed", turns: 120, messageBytes: 2048, toolBytes: 8192},
		{name: "message-heavy", turns: 120, messageBytes: 8192, toolBytes: 1024},
		{name: "tool-heavy", turns: 120, messageBytes: 512, toolBytes: 16384},
	}

	for _, shape := range shapes {
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

	content := builder.String()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
	return path, int64(len(content))
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
