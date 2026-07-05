package headroom

import (
	"context"
	"strings"
)

// TextCompressor is the fallback compressor for prose and unstructured
// content. The zero value is NOT valid; use NewTextCompressor.
type TextCompressor struct {
	// MaxProseBytes caps the output size on a sentence boundary.
	// Zero disables pruning (output preserves all input content minus
	// whitespace normalisation).
	MaxProseBytes int
}

// NewTextCompressor returns a TextCompressor. If maxProseBytes > 0 the
// output is truncated at the last sentence boundary (`.` `!` or `?`
// followed by whitespace) on or before MaxProseBytes bytes.
func NewTextCompressor(maxProseBytes int) *TextCompressor {
	return &TextCompressor{MaxProseBytes: maxProseBytes}
}

// ContentTypes returns [ContentTypeText].
func (c *TextCompressor) ContentTypes() []ContentType {
	return []ContentType{ContentTypeText}
}

// Compress normalises whitespace (strips trailing whitespace per line,
// collapses 3+ consecutive blank lines to 2) and optionally prunes the
// output on a sentence boundary.
func (c *TextCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	// Per-line trailing-whitespace strip.
	lines := strings.Split(string(in), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	normalised := collapseBlankLines([]byte(strings.Join(lines, "\n")), 2)

	if c.MaxProseBytes <= 0 || len(normalised) <= c.MaxProseBytes {
		return normalised, nil
	}
	// Find the last sentence boundary on or before the cap.
	cut := c.MaxProseBytes
	if cut > len(normalised) {
		cut = len(normalised)
	}
	end := -1
	for i := 0; i < cut; i++ {
		ch := normalised[i]
		if (ch == '.' || ch == '!' || ch == '?') && i+1 < len(normalised) {
			next := normalised[i+1]
			if next == ' ' || next == '\n' || next == '\t' || next == '\r' {
				end = i + 1
			}
		}
	}
	if end < 0 {
		// No sentence boundary in range: hard cut at the cap.
		end = cut
	}
	return normalised[:end], nil
}
