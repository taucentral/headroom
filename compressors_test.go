package headroom

import (
	"bytes"
	"context"
	"errors"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestGoASTCompressor_RoundTrip(t *testing.T) {
	t.Parallel()
	c := NewGoASTCompressor()
	in := []byte(`// Package foo does foo.
package foo

import "fmt"

// Hello prints hello.
func Hello() {
    fmt.Println("hello")


}
`)
	out, err := c.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	// Output must be re-parseable.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "out.go", out, 0); err != nil {
		t.Fatalf("output not re-parseable: %v\n%s", err, out)
	}
	if bytes.Contains(out, []byte("Package foo does foo")) {
		t.Errorf("doc comment not stripped:\n%s", out)
	}
	if bytes.Contains(out, []byte("Hello prints hello")) {
		t.Errorf("function doc comment not stripped:\n%s", out)
	}
	if bytes.Contains(out, []byte("\n\n\n")) {
		t.Errorf("3+ consecutive blank lines remain:\n%s", out)
	}
}

func TestGoASTCompressor_RejectsNonGo(t *testing.T) {
	t.Parallel()
	c := NewGoASTCompressor()
	cases := [][]byte{
		[]byte("not go code at all"),
		[]byte("func main() {"), // missing body
	}
	for _, in := range cases {
		_, err := c.Compress(context.Background(), in)
		if !errors.Is(err, ErrNotGoSource) {
			t.Errorf("Compress(%q): got err=%v, want ErrNotGoSource", in, err)
		}
	}
}

func TestLogCompressor_RepeatedLines(t *testing.T) {
	t.Parallel()
	c := NewLogCompressor()
	line := "2026-06-29T12:00:00Z INFO something happened"
	in := strings.Repeat(line+"\n", 50)
	out, err := c.Compress(context.Background(), []byte(in))
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if !strings.Contains(string(out), "[repeated 49 times]") {
		t.Errorf("expected [repeated 49 times] marker, got:\n%s", out)
	}
	if strings.Count(string(out), line) != 1 {
		t.Errorf("expected exactly one copy of the line, got:\n%s", out)
	}
}

func TestLogCompressor_LineTruncation(t *testing.T) {
	t.Parallel()
	c := &LogCompressor{MaxLineLen: 20}
	long := strings.Repeat("a", 200)
	in := []byte("2026-06-29T12:00:00Z INFO " + long + "\n")
	out, err := c.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) > 23 { // 20 cap + "..." suffix
			t.Errorf("line not truncated (%d bytes): %q", len(line), line)
		}
	}
}

func TestLogCompressor_RejectsNonLog(t *testing.T) {
	t.Parallel()
	c := NewLogCompressor()
	_, err := c.Compress(context.Background(), []byte("just prose, no timestamps"))
	if !errors.Is(err, ErrNotLogFormat) {
		t.Errorf("got err=%v, want ErrNotLogFormat", err)
	}
}

func TestDiffCompressor_RoundTrip(t *testing.T) {
	t.Parallel()
	c := NewDiffCompressor()
	in := []byte(`diff --git a/f b/f
index 0000000..1111111
--- a/f
+++ b/f
@@ -1,5 +1,5 @@
 context line 1
 context line 2
 context line 3
-old line
+new line
 context line 4
`)
	out, err := c.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	s := string(out)
	// -old line and +new line must be preserved verbatim.
	if !strings.Contains(s, "-old line") {
		t.Errorf("minus-line not preserved:\n%s", s)
	}
	if !strings.Contains(s, "+new line") {
		t.Errorf("plus-line not preserved:\n%s", s)
	}
	// Context-only run (4 context lines) must be consolidated.
	if !strings.Contains(s, "context lines>") {
		t.Errorf("context run not consolidated:\n%s", s)
	}
}

func TestDiffCompressor_RejectsNonDiff(t *testing.T) {
	t.Parallel()
	c := NewDiffCompressor()
	_, err := c.Compress(context.Background(), []byte("just some prose\nno diff here"))
	if !errors.Is(err, ErrNotDiff) {
		t.Errorf("got err=%v, want ErrNotDiff", err)
	}
}

func TestTextCompressor_WhitespaceNormalised(t *testing.T) {
	t.Parallel()
	c := NewTextCompressor(0) // no pruning
	in := []byte("line one   \n\n\n\n\nline two")
	out, err := c.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "line one   ") {
		t.Errorf("trailing whitespace not stripped: %q", s)
	}
	if strings.Contains(s, "\n\n\n\n") {
		t.Errorf("3+ blank lines not collapsed: %q", s)
	}
}

func TestTextCompressor_ProsePrunedOnSentenceBoundary(t *testing.T) {
	t.Parallel()
	c := NewTextCompressor(64)
	// Many short sentences; the cut must land on a sentence boundary
	// on or before 64 bytes.
	one := "One. "
	two := "Two. "
	three := "Three. "
	in := []byte(strings.Repeat(one, 4) + strings.Repeat(two, 4) + strings.Repeat(three, 20))
	out, err := c.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if len(out) > 64 {
		t.Errorf("output not pruned to <=64 bytes: %d %q", len(out), out)
	}
	// Must end on a sentence boundary.
	if len(out) == 0 || (out[len(out)-1] != '.' && out[len(out)-1] != ' ') {
		t.Errorf("output not cut on sentence boundary: %q", out)
	}
}
