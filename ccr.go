package headroom

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	tau "github.com/coevin/tau/pkg/tau"
)

// ccrRetrieveHint is the sentence appended to req.System on every turn
// the CCR mutator processes. It teaches the model that compressed
// blocks can be retrieved verbatim by hash via the ccr_retrieve tool.
const ccrRetrieveHint = "Content blocks compressed by CCR may be retrieved verbatim via the `ccr_retrieve` tool using the listed hash."

// CCRRequestMutator implements tau.RequestMutator. For each outgoing
// Request, the mutator walks every TextContent block in every Message,
// classifies the block, dispatches to the matching Compressor, writes
// the original to the OriginalCache, and replaces the block with a
// retrieval marker followed by the compressed summary.
//
// Any error (classification race, compressor rejection, store failure,
// compressor panic) causes the block to be passed through unchanged.
// The mutator never aborts the turn unless the context itself is
// cancelled.
type CCRRequestMutator struct {
	reg    *Registry
	router *Router
	cache  *OriginalCache
}

// NewCCRRequestMutator returns a CCRRequestMutator wired to the given
// registry, router, and cache. The returned value is what the embedder
// (or a test) places in tau.Options.Middleware; the plugin itself never
// calls tau.CreateAgentSession.
func NewCCRRequestMutator(reg *Registry, router *Router, cache *OriginalCache) *CCRRequestMutator {
	return &CCRRequestMutator{reg: reg, router: router, cache: cache}
}

// MutateRequest satisfies tau.RequestMutator. It is invoked by tau's
// runtime on every outgoing turn.
func (m *CCRRequestMutator) MutateRequest(ctx context.Context, req *tau.Request) error {
	if req == nil {
		return nil
	}
	m.ensureHint(req)
	for i := range req.Messages {
		msg := &req.Messages[i]
		for j := range msg.Content {
			block := &msg.Content[j]
			m.mutateBlock(ctx, block)
		}
	}
	return nil
}

// ensureHint appends the CCR retrieval hint as a TextContent block at
// the end of req.System if (and only if) it is not already present.
// This makes the hint idempotent across turns within a session.
func (m *CCRRequestMutator) ensureHint(req *tau.Request) {
	for i := range req.System {
		switch b := (req.System[i]).(type) {
		case tau.TextContent:
			if strings.Contains(b.Text, "ccr_retrieve") {
				return
			}
		}
	}
	req.System = append(req.System, tau.TextContent{Text: ccrRetrieveHint})
}

// mutateBlock compresses a single content block in place if it is a
// TextContent whose classification matches a registered Compressor. Any
// error (including panics) leaves the block unchanged.
func (m *CCRRequestMutator) mutateBlock(ctx context.Context, block *tau.ContentBlock) {
	tc, ok := (*block).(tau.TextContent)
	if !ok {
		return
	}
	original := []byte(tc.Text)
	if len(original) == 0 {
		return
	}

	contentType := m.router.Classify(original)
	if contentType == ContentTypeUnknown || contentType == ContentTypeText {
		// Pass-through: text and unknown blocks are not compressed by
		// default (the TextCompressor is opt-in via RegisterOptions).
		// When the TextCompressor IS registered we still fall through
		// to Lookup below.
	}

	c, err := m.reg.Lookup(contentType)
	if err != nil {
		if errors.Is(err, ErrUnknownCompressor) {
			return
		}
		return
	}

	summary, err := m.safeCompress(ctx, c, original)
	if err != nil {
		return
	}
	hash, err := m.cache.Put(ctx, original, contentType)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<compressed via ccr; retrieve original with hash %s>\n", hash)
	buf.Write(summary)
	*block = tau.TextContent{Text: buf.String()}
}

// safeCompress wraps Compressor.Compress in a panic-recovering shim.
// If the compressor panics, the function returns a non-nil error so
// the caller falls back to pass-through.
func (m *CCRRequestMutator) safeCompress(ctx context.Context, c Compressor, in []byte) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("headroom: compressor panic: %v", r)
			out = nil
		}
	}()
	return c.Compress(ctx, in)
}
