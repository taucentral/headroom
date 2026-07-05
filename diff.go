package headroom

import (
	"bytes"
	"context"
	"errors"
	"strings"
)

// ErrNotDiff is returned by DiffCompressor.Compress when the input is
// not a unified diff.
var ErrNotDiff = errors.New("headroom: not a unified diff")

// DiffCompressor consolidates runs of context-only lines in unified
// diffs while preserving every `-` and `+` line verbatim. The zero
// value is NOT valid; use NewDiffCompressor.
type DiffCompressor struct {
	// MinContextRun is the minimum run length of context-only lines
	// that triggers consolidation. Default 2: a single context line
	// between two changes is preserved as a structural anchor.
	MinContextRun int
}

// NewDiffCompressor returns a DiffCompressor with default settings
// (MinContextRun = 2).
func NewDiffCompressor() *DiffCompressor {
	return &DiffCompressor{MinContextRun: 2}
}

// ContentTypes returns [ContentTypeDiff].
func (c *DiffCompressor) ContentTypes() []ContentType {
	return []ContentType{ContentTypeDiff}
}

// Compress parses in as a unified diff. Every hunk header (`@@ ... @@`)
// and every `-`/`+` line is preserved verbatim. Runs of `MinContextRun`
// or more context-only lines (lines beginning with a single space, or
// empty lines inside a hunk) are replaced with `<N context lines>`.
// Non-diff input returns ErrNotDiff.
func (c *DiffCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	min := c.MinContextRun
	if min < 1 {
		min = 2
	}
	text := string(in)
	// Heuristic: a unified diff has at least one hunk header. Without
	// one, we refuse to compress.
	if !strings.Contains(text, "\n@@") && !strings.HasPrefix(text, "@@") {
		return nil, ErrNotDiff
	}
	lines := strings.Split(text, "\n")
	var out bytes.Buffer
	pendingContext := 0
	flushContext := func() {
		if pendingContext == 0 {
			return
		}
		if pendingContext >= min {
			out.WriteString("<")
			out.WriteString(itoa(pendingContext))
			out.WriteString(" context lines>\n")
		} else {
			for i := 0; i < pendingContext; i++ {
				out.WriteString("<context line>\n")
			}
		}
		pendingContext = 0
	}
	for _, line := range lines {
		if line == "" {
			// Blank line inside a diff is treated as a context line
			// with no leading space (common when copy-pasting).
			pendingContext++
			continue
		}
		switch line[0] {
		case '@':
			flushContext()
			out.WriteString(line)
			out.WriteByte('\n')
		case '+', '-':
			flushContext()
			out.WriteString(line)
			out.WriteByte('\n')
		case ' ':
			pendingContext++
		default:
			// Lines outside hunks (file headers like "diff --git",
			// "index abc..def", "--- a/", "+++ b/"): preserve verbatim.
			flushContext()
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	flushContext()
	res := out.Bytes()
	if n := len(res); n > 0 && res[n-1] == '\n' {
		res = res[:n-1]
	}
	return res, nil
}
