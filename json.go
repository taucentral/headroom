package headroom

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrNotJSON is returned by JSONCompressor.Compress when the input is
// not valid JSON.
var ErrNotJSON = errors.New("headroom: not JSON")

// JSONCompressor minifies JSON content and optionally prunes keys whose
// values the model already knows from context. The zero value is NOT
// valid; use NewJSONCompressor.
type JSONCompressor struct {
	// PruneKeys are key names whose values are dropped from the
	// emitted JSON. Default empty (no pruning).
	PruneKeys []string
}

// NewJSONCompressor returns a JSONCompressor with the given prune keys.
// Passing no prune keys yields a pure minifier.
func NewJSONCompressor(pruneKeys ...string) *JSONCompressor {
	return &JSONCompressor{PruneKeys: pruneKeys}
}

// ContentTypes returns [ContentTypeJSON].
func (c *JSONCompressor) ContentTypes() []ContentType {
	return []ContentType{ContentTypeJSON}
}

// Compress parses in as arbitrary JSON, prunes any PruneKeys, and
// re-emits as minified JSON. Non-JSON input returns ErrNotJSON.
func (c *JSONCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, ErrNotJSON
	}
	pruneInPlace(v, c.PruneKeys)
	return json.Marshal(v)
}

// pruneInPlace walks the decoded JSON tree (map[string]any / []any /
// scalar) and deletes any key in prune from every map it visits. Maps
// and slices are mutated in place; scalars are returned unchanged.
func pruneInPlace(v any, prune []string) {
	if len(prune) == 0 {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		for _, k := range prune {
			delete(t, k)
		}
		for _, child := range t {
			pruneInPlace(child, prune)
		}
	case []any:
		for i := range t {
			pruneInPlace(t[i], prune)
		}
	}
}
