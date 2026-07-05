package headroom

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tau "github.com/coevin/tau/pkg/tau"
)

// postMutationCapture is a test-only ResponseObserver that captures
// the *Request the runtime hands to observers after every mutator in
// the Middleware slice has run. The runtime passes the same *Request
// pointer to mutators and observers (internal/agent/session.go:189
// mutates req in place; :208 and :236 pass &req to observeResponse).
// So the req this observer receives is the post-mutator Request —
// exactly what was sent to the provider.
//
// This lets a test prove the runtime invoked a RequestMutator during
// a real turn, without substituting a unit-test drive on a fresh
// Request. See docs/input/context/plugin-support/plugin-observability.md
// §1 for the pattern.
type postMutationCapture struct {
	mu       sync.Mutex
	captured *tau.Request
}

func (p *postMutationCapture) ObserveResponse(ctx context.Context, req *tau.Request, resp *tau.Response) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captured = req
	return nil
}

func (p *postMutationCapture) Request() *tau.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.captured
}

// TestEndToEnd_PluginWiredIntoAgentSession exercises the full plugin
// through tau.CreateAgentSession: the embedder wires the plugin's
// Middleware + Tools into Options, runs one turn against a faux
// provider, and asserts:
//
//   - (a) CreateAgentSession accepted the options.
//   - (b) The CCR mutator ran, verified via a sibling ResponseObserver
//     (postMutationCapture) that inspects the post-mutation *Request
//     the runtime hands observers and asserts the system prompt
//     carries the retrieval hint. This is a real integration
//     assertion: it observes the runtime's actual post-mutation state
//     rather than re-running the mutator on a synthetic Request.
//   - (c) ccr_retrieve is in the registered tool names.
func TestEndToEnd_PluginWiredIntoAgentSession(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })

	tools := append(tau.BuiltinTools(), Tools(store)...)
	mw := Middleware(RegisterOptions{
		CCR:              true,
		PrefixStabilizer: true,
		OutputObserver:   true,
		LearnObserver:    true,
	}, store)

	// Append the test-only capture observer alongside the plugin's
	// real middleware. The runtime will invoke it after every
	// completed turn with the post-mutation *Request.
	capture := &postMutationCapture{}
	mw = append(mw, capture)

	sess, err := tau.CreateAgentSession(ctx, tau.Options{
		Cwd:           t.TempDir(),
		Model:         "faux",
		LLMClient:     tau.NewFauxProvider("ok"),
		Tools:         tools,
		Settings:      tau.DefaultSettings(),
		StateManager:  tau.NewInMemoryManager(t.TempDir()),
		ContextWindow: 200000,
		Middleware:    mw,
		Store:         store,
	})
	if err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}
	t.Cleanup(func() { _ = sess.Shutdown(ctx) })

	// (c) ccr_retrieve is in the registered tool names.
	toolNames := sess.Tools()
	found := false
	for _, n := range toolNames {
		if n == "ccr_retrieve" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ccr_retrieve not in registered tools: %v", toolNames)
	}

	// Run one turn. The faux provider returns "ok"; the run should
	// complete cleanly. observeResponse is called synchronously at
	// internal/agent/session.go:236 before Run returns, so by the
	// time Run returns the capture observer has observed.
	if err := sess.Run(ctx, "say ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// (b) Inspect the post-mutation *Request the runtime handed the
	// capture observer. If the CCR mutator ran, the system prompt
	// must carry the retrieval hint.
	finalReq := capture.Request()
	if finalReq == nil {
		t.Fatalf("capture observer was not invoked; finalReq is nil")
	}
	var hintFound bool
	for _, b := range finalReq.System {
		if tc, ok := b.(tau.TextContent); ok && strings.Contains(tc.Text, "ccr_retrieve") {
			hintFound = true
			break
		}
	}
	if !hintFound {
		t.Errorf("CCR retrieval hint not present in post-mutation system prompt; the mutator did not run or did not augment as expected")
	}
}
