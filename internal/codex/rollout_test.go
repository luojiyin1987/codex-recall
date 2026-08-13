package codex

import (
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
