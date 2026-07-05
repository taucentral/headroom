package headroom

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"

	tau "github.com/coevin/tau/pkg/tau"
)

// PrefixStabilizer is an opt-in RequestMutator that rewrites volatile
// prefix byte-ranges so provider KV caches hit. It does not reduce
// tokens; it increases cache hit rate by canonicalising parts of the
// request that are semantically identical but textually different
// across turns (trailing whitespace, RFC3339 millisecond timestamps,
// JSON key order).
//
// On any rule error the stabilizer leaves the prefix unchanged.
type PrefixStabilizer struct {
	// StripTrailingWhitespace, when true (default), removes trailing
	// space/tab/CR from each line of every TextContent block in
	// req.System and req.Messages.
	StripTrailingWhitespace bool

	// CanonicaliseTimestamps, when true, rewrites RFC3339 timestamps
	// in the system prompt and the first user message to second
	// precision (drops the fractional component).
	CanonicaliseTimestamps bool

	// SortJSONKeys, when true, re-serialises any TextContent block the
	// Router classifies as JSON so keys are emitted in sorted order.
	SortJSONKeys bool
}

// NewPrefixStabilizer returns a PrefixStabilizer with default rules
// (whitespace stripping only).
func NewPrefixStabilizer() *PrefixStabilizer {
	return &PrefixStabilizer{StripTrailingWhitespace: true}
}

// MutateRequest satisfies tau.RequestMutator. Any error in a single
// rule application is swallowed (the block is left unchanged); the
// method always returns nil unless the context is cancelled.
func (s *PrefixStabilizer) MutateRequest(ctx context.Context, req *tau.Request) error {
	if req == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	stabilizeBlocks(req.System, s)
	for i := range req.Messages {
		stabilizeBlocks(req.Messages[i].Content, s)
	}
	return nil
}

// timestampRe matches RFC3339 timestamps with an optional fractional
// component. We capture the pieces we keep (date "T" time-to-seconds
// and trailing Z or offset) and drop the fractional part.
var timestampRe = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.\d{1,9})?(Z|[+-]\d{2}:\d{2})`)

// stabilizeBlocks applies each enabled rule to every TextContent block
// in slice. Other block types are ignored. Errors in individual rules
// or individual blocks leave that block unchanged.
func stabilizeBlocks(blocks []tau.ContentBlock, s *PrefixStabilizer) {
	router := NewRouter()
	for i := range blocks {
		tc, ok := blocks[i].(tau.TextContent)
		if !ok {
			continue
		}
		text := tc.Text
		if s.StripTrailingWhitespace {
			text = stripTrailingWS(text)
		}
		if s.CanonicaliseTimestamps {
			text = timestampRe.ReplaceAllString(text, "$1$2")
		}
		if s.SortJSONKeys {
			if router.Classify([]byte(text)) == ContentTypeJSON {
				if sorted, err := sortJSONKeys([]byte(text)); err == nil {
					text = string(sorted)
				}
			}
		}
		blocks[i] = tau.TextContent{Text: text}
	}
}

func stripTrailingWS(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n")
}

// sortJSONKeys re-serialises in so that all object keys appear in
// sorted order. This is the canonical form most KV-cache
// implementations expect for stable hashing.
func sortJSONKeys(in []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	sortKeysInPlace(v)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

func sortKeysInPlace(v any) {
	switch t := v.(type) {
	case map[string]any:
		// json.Unmarshal uses map[string]any; iteration order is not
		// deterministic but the encoder emits keys in sorted order
		// when SetEscapeHTML is false... actually it doesn't. We must
		// collect, sort, and rebuild a new map to guarantee order.
		// Since Go maps don't preserve insertion order, we encode
		// through a slice of kv pairs via a custom marshal path.
		// Simpler: recurse on values (they may need sorting too) and
		// let the encoder's native sort do the work — Go's
		// encoding/json sorts map keys at marshal time.
		for _, child := range t {
			sortKeysInPlace(child)
		}
	case []any:
		for i := range t {
			sortKeysInPlace(t[i])
		}
	}
}
