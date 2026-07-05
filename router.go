package headroom

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"regexp"
)

// Router classifies a content block into a ContentType using cheap
// structural probes. The Router is approximate by design: each
// Compressor is the authoritative detector for its own type and returns
// a typed rejection error if the Router misroutes.
type Router struct{}

// NewRouter returns a Router.
func NewRouter() *Router { return &Router{} }

// diffPrefixRe matches the first lines of a unified diff: a "diff --git"
// header, an "Index:"/"index" line, or the "--- a/..." file header.
var diffPrefixRe = regexp.MustCompile(`(?m)^(?:diff --git |Index |\+\+\+ |--- |@@ )`)

// Classify returns the ContentType the block most likely belongs to.
// The probes are tried cheapest-first: JSON validity, Go parsability,
// diff line prefix, log signature, else text.
func (r *Router) Classify(in []byte) ContentType {
	if len(in) == 0 {
		return ContentTypeText
	}
	if json.Valid(in) {
		return ContentTypeJSON
	}
	// Go probe: parse as a file with PackageClauseOnly so we don't do
	// a full AST walk on prose that happens to start with "package".
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "router.go", in, parser.PackageClauseOnly); err == nil {
		// PackageClauseOnly alone is too lax ("package" + anything is
		// enough). Require a full parse to confirm.
		if _, err := parser.ParseFile(fset, "router.go", in, 0); err == nil {
			return ContentTypeGo
		}
	}
	if diffPrefixRe.Match(in) {
		return ContentTypeDiff
	}
	if logLineRe.Match(in) {
		return ContentTypeLog
	}
	return ContentTypeText
}
