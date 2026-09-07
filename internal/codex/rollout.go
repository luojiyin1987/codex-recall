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
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	reader := &byteCountingReader{reader: file}
	err = visitRolloutContext(ctx, reader, visit)
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
	reader := bufio.NewReader(input)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			var rec record
			if json.Unmarshal(bytes.TrimSpace(line), &rec) == nil {
				stop, err := visit(rec)
				if err != nil {
					return err
				}
				if stop {
					return nil
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
