package headroom

import (
	tau "github.com/taucentral/tau/pkg/tau"
)

// RegisterOptions selects which plugin components the embedder wants
// wired into tau.Options. Every component is opt-in; a zero-value
// RegisterOptions produces an empty Middleware slice and the Tools
// function still returns the ccr_retrieve tool (it has no dependencies
// beyond a Store).
type RegisterOptions struct {
	// CCR enables the Compress-Cache-Retrieve request mutator. The
	// mutator compresses outgoing TextContent blocks, caches the
	// originals, and appends a retrieval hint to the system prompt.
	CCR bool

	// PrefixStabilizer enables the prefix-stabilizer request mutator.
	// It canonicalises volatile byte-ranges so provider KV caches
	// hit. Independent of CCR; can be enabled alongside it.
	PrefixStabilizer bool

	// OutputObserver enables the output-compression response observer
	// that drops ceremony from persisted responses.
	OutputObserver bool

	// LearnObserver enables the learn response observer that mines
	// failed turns for corrections.
	LearnObserver bool

	// PruneKeys configures the JSONCompressor's key-pruning list.
	// Default empty (no pruning). Applied to the CCR mutator's
	// JSONCompressor only.
	PruneKeys []string

	// MaxProseBytes configures the TextCompressor's prose-pruning
	// threshold. Zero disables prose pruning. Applied to the CCR
	// mutator's TextCompressor only when > 0.
	MaxProseBytes int
}

// Middleware returns the requested middleware components as a slice
// of `any`, each element of which satisfies at least one of tau's
// extension interfaces (RequestMutator or ResponseObserver). The
// embedder passes the returned slice directly into tau.Options.Middleware.
//
// Each call returns fresh, independent values; calling Middleware
// twice with the same opts yields two independent sets of middleware.
//
// store is the embedder-supplied tau.Store the CCR mutator and Learn
// observer use to persist originals and corrections. Pass nil to
// disable persistence (the CCR mutator becomes a pass-through and the
// Learn observer is a no-op); for real use, supply tau.NewFileStore
// or an equivalent implementation.
//
// The plugin never calls tau.CreateAgentSession, agent.Run, or any
// orchestration API; the embedder wires the returned slice into
// Options.Middleware.
func Middleware(opts RegisterOptions, store tau.Store) []any {
	var out []any
	if opts.CCR {
		reg := buildRegistry(opts)
		cache := NewOriginalCache(store)
		out = append(out, NewCCRRequestMutator(reg, NewRouter(), cache))
	}
	if opts.PrefixStabilizer {
		out = append(out, NewPrefixStabilizer())
	}
	if opts.OutputObserver {
		out = append(out, NewOutputObserver())
	}
	if opts.LearnObserver {
		cache := NewOriginalCache(store)
		out = append(out, NewLearnObserver(cache, 3, nil))
	}
	return out
}

// Tools returns the plugin's headless tools. The slice always contains
// exactly one element: the ccr_retrieve tool wired to the given Store.
// The embedder passes the returned slice (typically appended to
// tau.BuiltinTools()) into tau.Options.Tools.
func Tools(store tau.Store) []tau.HeadlessTool {
	cache := NewOriginalCache(store)
	return []tau.HeadlessTool{NewRetrieve(cache)}
}

// buildRegistry constructs a fresh Compressor registry populated with
// the four deterministic compressors the plugin ships. The
// TextCompressor is only included when MaxProseBytes > 0 (otherwise
// prose falls through as pass-through, matching the spec's "prose
// compression deferred indefinitely" non-goal).
func buildRegistry(opts RegisterOptions) *Registry {
	reg := NewRegistry()
	_ = reg.Register(NewJSONCompressor(opts.PruneKeys...))
	_ = reg.Register(NewGoASTCompressor())
	_ = reg.Register(NewLogCompressor())
	_ = reg.Register(NewDiffCompressor())
	if opts.MaxProseBytes > 0 {
		_ = reg.Register(NewTextCompressor(opts.MaxProseBytes))
	}
	return reg
}
