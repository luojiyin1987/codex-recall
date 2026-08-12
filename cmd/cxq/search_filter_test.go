package main

import (
	"testing"

	"github.com/luojiyin1987/codex-recall/internal/codex"
)

func TestMatchesSearchFilters(t *testing.T) {
	session := codex.Session{
		CWD:    "/tmp/Lint-MD",
		Source: "VSCode",
	}

	tests := []struct {
		name    string
		project string
		source  string
		want    bool
	}{
		{name: "no filters", want: true},
		{name: "project", project: "lint-md", want: true},
		{name: "project case insensitive", project: "LINT-MD", want: true},
		{name: "project mismatch", project: "cve-lite-cli", want: false},
		{name: "source", source: "vscode", want: true},
		{name: "source case insensitive", source: "VSCODE", want: true},
		{name: "source mismatch", source: "cli", want: false},
		{name: "both", project: "lint-md", source: "vscode", want: true},
		{name: "both with mismatch", project: "lint-md", source: "cli", want: false},
		{name: "trim filter values", project: " lint-md ", source: " vscode ", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchesSearchFilters(session, test.project, test.source); got != test.want {
				t.Fatalf("matchesSearchFilters() = %v, want %v", got, test.want)
			}
		})
	}
}
