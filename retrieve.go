package headroom

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"

	tau "github.com/coevin/tau/pkg/tau"
)

// retrieveArgs is the parameter schema for the ccr_retrieve tool. It is
// reflected into a JSON Schema for the model.
type retrieveArgs struct {
	Hash string `json:"hash" jsonschema:"description=Hex-encoded SHA-256 of the original content block to retrieve.,minLength=64,maxLength=64"`
}

// Retrieve is the ccr_retrieve tool. It returns the verbatim original
// bytes for a SHA-256 hash by looking the hash up in the
// embedder-supplied OriginalCache. The plugin registers the tool via
// the Tools() entry point; tau's runtime invokes Execute when the model
// calls ccr_retrieve.
type Retrieve struct {
	cache *OriginalCache
}

// NewRetrieve returns the ccr_retrieve tool backed by cache. The
// returned value is what the embedder (or a test) places in
// tau.Options.Tools.
func NewRetrieve(cache *OriginalCache) *Retrieve {
	return &Retrieve{cache: cache}
}

// Name returns the tool's namespaced identifier. The "ccr_" prefix
// leaves room for future plugins (memory_retrieve, redaction_retrieve,
// etc.) without collision.
func (r *Retrieve) Name() string { return "ccr_retrieve" }

// Description returns natural-language help shown to the model.
func (r *Retrieve) Description() string {
	return "Retrieve the verbatim original of a content block that was compressed by the CCR (Compress-Cache-Retrieve) middleware. Pass the hex-encoded SHA-256 hash exactly as it appears in the `<compressed via ccr; retrieve original with hash <h>>` marker."
}

// Parameters returns the JSON Schema for the tool's args, reflected
// from retrieveArgs. Marshals to valid JSON Schema draft 2020-12.
func (r *Retrieve) Parameters() jsonschema.Schema {
	return reflectSchema(&retrieveArgs{})
}

// Execute looks up call.Args["hash"] in the cache and returns the
// original bytes as text. On miss, malformed args, or a closed store,
// Execute returns an IsError tool result; it never panics.
func (r *Retrieve) Execute(ctx context.Context, call tau.ToolCall) (tau.ToolResult, error) {
	var args retrieveArgs
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return tau.NewErrorResult(fmt.Sprintf("ccr_retrieve: malformed args: %v", err)), nil
	}
	if args.Hash == "" {
		return tau.NewErrorResult("ccr_retrieve: missing required field \"hash\""), nil
	}
	original, err := r.cache.Get(ctx, args.Hash)
	if err != nil {
		return tau.NewErrorResult(fmt.Sprintf("ccr_retrieve: no original for hash %s", args.Hash)), nil
	}
	return tau.NewTextResult(string(original)), nil
}

// reflectSchema is the plugin-local equivalent of tau's internal
// tools.ReflectSchema. We reproduce the behaviour here because the
// internal helper is not exported (pkg/tau does not re-export it).
// Embedders implementing custom HeadlessTools must use this pattern.
func reflectSchema(sample any) jsonschema.Schema {
	r := new(jsonschema.Reflector)
	r.DoNotReference = true
	s := r.Reflect(sample)
	if s == nil {
		return jsonschema.Schema{Type: "object"}
	}
	s.Version = ""
	s.ID = ""
	s.Definitions = nil
	return *s
}
