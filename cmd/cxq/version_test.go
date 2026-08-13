package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSelectVersion(t *testing.T) {
	tests := []struct {
		name          string
		injected      string
		moduleVersion string
		want          string
	}{
		{name: "injected", injected: "v1.2.3", moduleVersion: "v1.2.2", want: "v1.2.3"},
		{name: "module", moduleVersion: "v1.2.3", want: "v1.2.3"},
		{name: "development", moduleVersion: "(devel)", want: "devel"},
		{name: "missing", want: "devel"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := selectVersion(test.injected, test.moduleVersion); got != test.want {
				t.Fatalf("selectVersion(%q, %q) = %q, want %q", test.injected, test.moduleVersion, got, test.want)
			}
		})
	}
}

func TestCLIRunnerVersionAliases(t *testing.T) {
	original := buildVersion
	buildVersion = "v1.2.3"
	t.Cleanup(func() { buildVersion = original })

	for _, command := range []string{"version", "--version", "-v"} {
		t.Run(command, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			runner := newCLIRunner(strings.NewReader(""), &stdout, &stderr)
			if err := runner.run([]string{command}); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != "cxq v1.2.3\n" || stderr.Len() != 0 {
				t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestCLIRunnerVersionRejectsArguments(t *testing.T) {
	runner := newCLIRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err := runner.run([]string{"version", "extra"}); err == nil {
		t.Fatal("version accepted an argument")
	}
}
