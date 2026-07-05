package headroom

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// ErrNotGoSource is returned by GoASTCompressor.Compress when the input
// is not parseable as a Go source file.
var ErrNotGoSource = errors.New("headroom: not Go source")

// GoASTCompressor reformats Go source by parsing it, dropping comments,
// and re-emitting via go/format. The zero value is NOT valid; use
// NewGoASTCompressor.
type GoASTCompressor struct {
	// DropComments controls whether doc/line comments are stripped.
	// Default true.
	DropComments bool
}

// NewGoASTCompressor returns a GoASTCompressor with default options
// (comments dropped).
func NewGoASTCompressor() *GoASTCompressor {
	return &GoASTCompressor{DropComments: true}
}

// ContentTypes returns [ContentTypeGo].
func (c *GoASTCompressor) ContentTypes() []ContentType {
	return []ContentType{ContentTypeGo}
}

// Compress parses in as a Go source file, drops comments per config,
// and re-emits via go/format.Node. Non-Go input returns ErrNotGoSource.
func (c *GoASTCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "headroom.go", in, parser.ParseComments)
	if err != nil {
		return nil, ErrNotGoSource
	}
	if c.DropComments {
		// Clear both the file-level comment list and every Doc/Cmt
		// field on every declaration. go/format emits Doc fields from
		// per-node references, so nil-ing f.Comments alone is not
		// enough.
		f.Comments = nil
		ast.Inspect(f, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.GenDecl:
				d.Doc = nil
			case *ast.FuncDecl:
				d.Doc = nil
			case *ast.Field:
				d.Doc = nil
				d.Comment = nil
			case *ast.ValueSpec:
				d.Doc = nil
				d.Comment = nil
			case *ast.TypeSpec:
				d.Doc = nil
				d.Comment = nil
			case *ast.ImportSpec:
				d.Doc = nil
				d.Comment = nil
			case *ast.File:
				d.Doc = nil
			}
			return true
		})
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, ErrNotGoSource
	}
	return collapseBlankLines(buf.Bytes(), 2), nil
}

