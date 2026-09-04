package textutil

import "testing"

func TestNormalizeWhitespace(t *testing.T) {
	got := NormalizeWhitespace("  alpha\n\tbeta\u3000gamma  ")
	want := "alpha beta gamma"
	if got != want {
		t.Fatalf("NormalizeWhitespace() = %q, want %q", got, want)
	}
}
