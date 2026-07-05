// Package headroom is a tau plugin that realises the context-compression
// patterns documented in docs/input/context/plugins/headroom.md natively
// in Go via tau's extension interfaces (declared in pkg/tau).
//
// The plugin extends tau (the runtime) by providing implementations of
// tau's extension interfaces. The plugin never calls CreateAgentSession,
// agent.Run, or any orchestration API. The embedder wires the plugin's
// returned middleware and tools into a tau.Options value it owns.
package headroom

import (
	"context"
	"errors"
	"sync"
)

// ContentType labels the kind of a content block. The Router classifies
// each outgoing block into one of these types so the registry can
// dispatch to the matching Compressor.
type ContentType string

const (
	// ContentTypeJSON is the type for JSON-encoded content blocks.
	ContentTypeJSON ContentType = "json"
	// ContentTypeGo is the type for Go source code content blocks.
	ContentTypeGo ContentType = "go"
	// ContentTypeLog is the type for log-formatted content blocks.
	ContentTypeLog ContentType = "log"
	// ContentTypeDiff is the type for unified-diff content blocks.
	ContentTypeDiff ContentType = "diff"
	// ContentTypeText is the fallback type for prose and unstructured
	// content. The TextCompressor is registered for this type.
	ContentTypeText ContentType = "text"
	// ContentTypeUnknown marks a block the Router could not classify.
	// The CCR mutator treats unknown blocks as pass-through.
	ContentTypeUnknown ContentType = "unknown"
)

// ErrCompressorAlreadyRegistered is returned by Registry.Register when a
// Compressor is already registered for one of the content types the new
// Compressor claims.
var ErrCompressorAlreadyRegistered = errors.New("headroom: compressor already registered for content type")

// ErrUnknownCompressor is returned by Registry.Lookup when no Compressor
// is registered for the requested content type.
var ErrUnknownCompressor = errors.New("headroom: no compressor registered for content type")

// Compressor transforms a content block's bytes into a lossy summary.
// Implementations MUST be safe for concurrent use.
type Compressor interface {
	// ContentTypes returns the content types this Compressor handles.
	// A single Compressor may handle more than one type.
	ContentTypes() []ContentType

	// Compress transforms in into a lossy summary. If the input is not
	// of a shape the Compressor can handle, it MUST return a typed
	// rejection error (e.g. ErrNotJSON) so the caller can fall back to
	// pass-through.
	Compress(ctx context.Context, in []byte) (summary []byte, err error)
}

// Registry maps ContentType to its Compressor. The zero value is NOT
// valid; use NewRegistry.
type Registry struct {
	mu sync.RWMutex
	m  map[ContentType]Compressor
}

// NewRegistry returns an empty Registry.
//
// The Registry is safe for concurrent use: Register and Lookup may be
// called from multiple goroutines. Lookups do not block registrations
// and vice versa.
func NewRegistry() *Registry {
	return &Registry{m: make(map[ContentType]Compressor)}
}

// Register adds c to the registry for every content type c claims. If a
// Compressor is already registered for any of those types, Register
// returns ErrCompressorAlreadyRegistered and leaves the registry
// unchanged.
func (r *Registry) Register(c Compressor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	types := c.ContentTypes()
	// Pre-check: refuse to overwrite any existing registration.
	for _, t := range types {
		if _, ok := r.m[t]; ok {
			return ErrCompressorAlreadyRegistered
		}
	}
	for _, t := range types {
		r.m[t] = c
	}
	return nil
}

// Lookup returns the Compressor registered for t, or
// ErrUnknownCompressor if none is registered.
func (r *Registry) Lookup(t ContentType) (Compressor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.m[t]
	if !ok {
		return nil, ErrUnknownCompressor
	}
	return c, nil
}
