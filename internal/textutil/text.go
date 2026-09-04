package textutil

import "strings"

// NormalizeWhitespace collapses Unicode whitespace runs to single ASCII spaces
// and trims leading/trailing whitespace for stable one-line previews.
func NormalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
