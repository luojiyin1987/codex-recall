package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type recordVisitor func(record) (stop bool, err error)

func visitRolloutFileContext(ctx context.Context, path string, visit recordVisitor) error {
	_, err := visitRolloutFileContextMeasured(ctx, path, visit)
	return err
}

func visitRolloutFileContextMeasured(ctx context.Context, path string, visit recordVisitor) (int64, error) {
	return visitRolloutFileContextFilteredMeasured(ctx, path, nil, visit)
}

// visitRolloutFileContextFilteredMeasured counts bytes read and decodes only
// the records that shouldDecode returns true for. A nil shouldDecode decodes
// every record.
func visitRolloutFileContextFilteredMeasured(ctx context.Context, path string, shouldDecode func([]byte) bool, visit recordVisitor) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	reader := &byteCountingReader{reader: file}
	err = visitRolloutContextFiltered(ctx, reader, shouldDecode, visit)
	return reader.bytesRead, err
}

type byteCountingReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *byteCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func visitRollout(input io.Reader, visit recordVisitor) error {
	return visitRolloutContext(context.Background(), input, visit)
}

func visitRolloutContext(ctx context.Context, input io.Reader, visit recordVisitor) error {
	return visitRolloutContextFiltered(ctx, input, nil, visit)
}

// visitRolloutContextFiltered reads JSONL records. It calls shouldDecode with
// each trimmed line before the envelope decode. It skips the line when
// shouldDecode returns false. A nil shouldDecode decodes every line. The filter
// lets the conversation decoder skip the payload copy for irrelevant records.
func visitRolloutContextFiltered(ctx context.Context, input io.Reader, shouldDecode func([]byte) bool, visit recordVisitor) error {
	reader := bufio.NewReader(input)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if shouldDecode == nil || shouldDecode(trimmed) {
				var rec record
				if json.Unmarshal(trimmed, &rec) == nil {
					stop, err := visit(rec)
					if err != nil {
						return err
					}
					if stop {
						return nil
					}
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
