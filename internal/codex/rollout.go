package codex

import (
	"bufio"
	"context"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type recordVisitor func(record) (stop bool, err error)

func visitRolloutFile(path string, visit recordVisitor) error {
	return visitRolloutFileContext(context.Background(), path, visit)
}

func visitRolloutFileContext(ctx context.Context, path string, visit recordVisitor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return visitRolloutContext(ctx, file, visit)
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
