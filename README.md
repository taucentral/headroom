# tau-plugins/headroom

A tau plugin that realises the context-compression patterns documented
in `docs/input/context/plugins/headroom.md` natively in Go. The plugin
extends tau by providing implementations of tau's extension interfaces
(`RequestMutator`, `ResponseObserver`, `HeadlessTool`, declared in
`pkg/tau`). No Python sidecar, no CGO, no ML runtime.

The plugin never calls `tau.CreateAgentSession`, `agent.Run`,
`tau.RegisterProvider`, or any orchestration API. The embedder wires
the plugin's returned middleware and tools into a `tau.Options` value
the embedder owns.

The reference basis for this plugin's design is
`docs/input/context/plugins/headroom.md`. The OpenSpec change proposal
lives at `openspec/changes/add-headroom-plugin/`.

## Status

**v0.x — scaffold complete, source implemented.** The implementation
mirrors the task list at `openspec/changes/add-headroom-plugin/tasks.md`.

## What it does

The plugin ships six components, all composed from existing tau SDK
extension points (`pkg/tau/`):

1. **Deterministic compressors** for JSON, Go AST, log, diff, and text,
   plus a heuristic content router (no ML).
2. **CCR request mutator** — compresses outgoing content blocks, writes
   the verbatim original to an injected `tau.Store` under its SHA-256
   hash, and appends a retrieval hint to the system prompt.
3. **`ccr_retrieve` headless tool** — returns the original bytes by
   hash.
4. **Prefix-stabilizer request mutator** — rewrites volatile prefix
   byte-ranges so provider KV caches hit.
5. **Output-compression response observer** — drops ceremony from the
   persisted assistant response.
6. **Learn response observer** — extracts corrections from failed turns
   and writes them to the `Store`.

## Embedding

```go
package main

import (
	"context"
	"log"

	tau "github.com/coevin/tau/pkg/tau"
	headroom "github.com/coevin/tau-plugins/headroom"
)

func main() {
	ctx := context.Background()

	store, err := tau.NewFileStore(".headroom-cache")
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer store.Close()

	mw := headroom.Middleware(headroom.RegisterOptions{
		CCR:              true,
		PrefixStabilizer: true,
		OutputObserver:   true,
		LearnObserver:    true,
	}, store)
	tools := append(tau.BuiltinTools(), headroom.Tools(store)...)

	sess, err := tau.CreateAgentSession(ctx, tau.Options{
		Cwd:           ".",
		Model:         "claude-opus-4-5-20251101",
		LLMClient:     tau.NewFauxProvider("hello from the model"),
		Tools:         tools,
		Settings:      tau.DefaultSettings(),
		StateManager:  tau.NewInMemoryManager("."),
		ContextWindow: 200000,
		Middleware:    mw,
		Store:         store,
	})
	if err != nil {
		log.Fatalf("create session: %v", err)
	}
	defer sess.Shutdown(ctx)

	if err := sess.Run(ctx, "Say hello."); err != nil {
		log.Fatalf("run: %v", err)
	}
}
```

The snippet above is complete. Drop it into `main.go` in a module that
`require`s both `github.com/coevin/tau` and
`github.com/coevin/tau-plugins/headroom` and it will compile and run.
Replace `tau.NewFauxProvider` with a real LLM client for production
use. See `pkg/tau/doc.go` for the canonical embedding pattern.

## License

To be determined. The plugin is currently unlicensed pending a decision
on whether to match tau's license or ship under a separate one.
