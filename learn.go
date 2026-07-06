package headroom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	tau "github.com/taucentral/tau/pkg/tau"
)

// FailureClassifier is an optional embedder-supplied callback that
// inspects an assistant Response and reports whether the turn should
// be treated as failed. The LearnObserver uses it to detect tool
// errors via the JSON-encoded form of the response (the SDK does not
// re-export the llm.ToolResult block type, so the plugin inspects the
// marshalled JSON for `"isError":true` on a tool_result block).
//
// Returning true marks the turn as failed.
type FailureClassifier func(resp *tau.Response) bool

// LearnObserver is a ResponseObserver that mines failed turns for
// corrections. A turn is classified as failed when any of:
//   - the response's StopReason is "aborted";
//   - the embedder's FailureClassifier (if set) returns true;
//   - the response contains a marshalled tool_result block whose
//     IsError field is true (detected via JSON inspection).
//
// For a failed turn followed by a user correction in a subsequent
// turn, the observer writes a correction entry to the OriginalCache
// keyed by the SHA-256 of the failed response's first TextContent
// block. The observer is opt-in via RegisterOptions.LearnObserver.
type LearnObserver struct {
	cache    *OriginalCache
	classify FailureClassifier

	mu      sync.Mutex
	buffer  []learnEntry
	history int
}

type learnEntry struct {
	respFirstBlock []byte
	wasError       bool
}

// NewLearnObserver returns a LearnObserver. history is the ring-buffer
// size (default 3); <=0 falls back to 3.
func NewLearnObserver(cache *OriginalCache, history int, classify FailureClassifier) *LearnObserver {
	if history <= 0 {
		history = 3
	}
	return &LearnObserver{cache: cache, history: history, classify: classify}
}

// ObserveResponse satisfies tau.ResponseObserver. The observer is
// non-aborting: any cache write failure is logged and dropped.
func (o *LearnObserver) ObserveResponse(ctx context.Context, req *tau.Request, resp *tau.Response, streamErr error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if resp == nil {
		return nil
	}
	firstBlock := firstText(resp.Content)
	wasError := o.isFailed(resp)

	o.mu.Lock()
	prev := o.peekPrevious()
	o.push(learnEntry{respFirstBlock: firstBlock, wasError: wasError})
	o.mu.Unlock()

	if prev != nil && prev.wasError {
		if correction, ok := extractCorrection(req, firstBlock); ok {
			symptom := sha256.Sum256(prev.respFirstBlock)
			symptomHash := hex.EncodeToString(symptom[:])
			o.cache.writeCorrection(ctx, symptomHash, correction)
		}
	}
	return nil
}

// isFailed reports whether the response represents a failed turn per
// the configured heuristics.
func (o *LearnObserver) isFailed(resp *tau.Response) bool {
	if resp.StopReason == tau.StopReason("aborted") {
		return true
	}
	if o.classify != nil {
		return o.classify(resp)
	}
	// SDK does not re-export llm.ToolResult as a public type, so we
	// inspect the marshalled JSON for an isError:true tool_result
	// block. This is a conservative check; embedders who want a
	// stronger signal pass FailureClassifier.
	return responseContainsToolError(resp)
}

// responseContainsToolError marshals resp and looks for the
// "isError":true marker on a tool_result block. The JSON form of an
// llm.ToolResult block uses the discriminator "tool_result"; we look
// for occurrences of that discriminator paired with isError:true.
func responseContainsToolError(resp *tau.Response) bool {
	for _, b := range resp.Content {
		bs, err := json.Marshal(b)
		if err != nil {
			continue
		}
		s := string(bs)
		if !strings.Contains(s, `"type":"tool_result"`) {
			continue
		}
		if strings.Contains(s, `"isError":true`) {
			return true
		}
	}
	return false
}

func (o *LearnObserver) peekPrevious() *learnEntry {
	if len(o.buffer) == 0 {
		return nil
	}
	e := o.buffer[len(o.buffer)-1]
	return &e
}

func (o *LearnObserver) push(e learnEntry) {
	o.buffer = append(o.buffer, e)
	if len(o.buffer) > o.history {
		o.buffer = o.buffer[len(o.buffer)-o.history:]
	}
}

// firstText returns the bytes of the first TextContent block in slice,
// or nil if there is none.
func firstText(blocks []tau.ContentBlock) []byte {
	for _, b := range blocks {
		if tc, ok := b.(tau.TextContent); ok {
			return []byte(tc.Text)
		}
	}
	return nil
}

// extractCorrection looks at the user's most recent message in req and
// reports whether it contains a correction marker. The returned string
// is the cache-persisted correction summary.
func extractCorrection(req *tau.Request, assistantFirstBlock []byte) (string, bool) {
	if req == nil || len(req.Messages) == 0 {
		return "", false
	}
	var lastUser string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if string(req.Messages[i].Role) == "user" {
			lastUser = string(firstText(req.Messages[i].Content))
			break
		}
	}
	if lastUser == "" {
		return "", false
	}
	lower := strings.ToLower(lastUser)
	markers := []string{"no", "wrong", "actually", "s/", "incorrect"}
	hit := ""
	for _, m := range markers {
		if strings.Contains(lower, m) {
			hit = m
			break
		}
	}
	if hit == "" {
		return "", false
	}
	var b strings.Builder
	b.WriteString("Corrected turn (symptom follows).\n\nUser correction:\n")
	b.WriteString(lastUser)
	b.WriteString("\n\nFailed assistant first block:\n")
	b.Write(assistantFirstBlock)
	return b.String(), true
}

// timeNowUTC is split out so tests can stub it via package-internal
// reassignment.
var timeNowUTC = func() time.Time { return time.Now().UTC() }
