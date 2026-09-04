package codex

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestVisitRolloutSkipsMalformedRecordsAndStops(t *testing.T) {
	input := strings.Join([]string{
		"not json",
		`{"type":"first","payload":{}}`,
		`{"type":"second","payload":{}}`,
	}, "\n")
	var visited []string

	err := visitRollout(strings.NewReader(input), func(rec record) (bool, error) {
		visited = append(visited, rec.Type)
		return rec.Type == "first", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(visited) != 1 || visited[0] != "first" {
		t.Fatalf("visited = %v, want [first]", visited)
	}
}

func TestVisitRolloutContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := visitRolloutContext(ctx, strings.NewReader("{}\n"), func(record) (bool, error) {
		t.Fatal("visitor ran after cancellation")
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
