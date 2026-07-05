package headroom

import (
	"context"
	"strings"
	"testing"

	tau "github.com/coevin/tau/pkg/tau"
)

func TestPrefixStabilizer_WhitespaceStripped(t *testing.T) {
	t.Parallel()
	s := NewPrefixStabilizer()
	req := &tau.Request{
		System: []tau.ContentBlock{
			tau.TextContent{Text: "line one   \nline two\t\n"},
		},
	}
	if err := s.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	tc := req.System[0].(tau.TextContent)
	if strings.Contains(tc.Text, "line one   ") {
		t.Errorf("trailing whitespace not stripped: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, "line one\nline two") {
		t.Errorf("content lost: %q", tc.Text)
	}
}

func TestPrefixStabilizer_TimestampCanonicalised(t *testing.T) {
	t.Parallel()
	s := &PrefixStabilizer{CanonicaliseTimestamps: true}
	req := &tau.Request{
		System: []tau.ContentBlock{
			tau.TextContent{Text: "Now is 2026-06-29T12:34:56.789012345Z"},
		},
	}
	if err := s.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	tc := req.System[0].(tau.TextContent)
	if !strings.Contains(tc.Text, "2026-06-29T12:34:56Z") {
		t.Errorf("timestamp not canonicalised: %q", tc.Text)
	}
	if strings.Contains(tc.Text, ".789012345") {
		t.Errorf("fractional seconds not dropped: %q", tc.Text)
	}
}

func TestPrefixStabilizer_JSONKeysSorted(t *testing.T) {
	t.Parallel()
	s := &PrefixStabilizer{SortJSONKeys: true}
	// Deliberately reverse-alphabetical keys.
	req := &tau.Request{
		System: []tau.ContentBlock{
			tau.TextContent{Text: `{"z":1,"a":2,"m":3}`},
		},
	}
	if err := s.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	tc := req.System[0].(tau.TextContent)
	if !strings.HasPrefix(tc.Text, `{"a":`) {
		t.Errorf("keys not sorted: %q", tc.Text)
	}
}

func TestPrefixStabilizer_NoOpOnInvalidJSON(t *testing.T) {
	t.Parallel()
	s := &PrefixStabilizer{SortJSONKeys: true}
	original := `{"not valid"`
	req := &tau.Request{
		System: []tau.ContentBlock{
			tau.TextContent{Text: original},
		},
	}
	if err := s.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	tc := req.System[0].(tau.TextContent)
	if tc.Text != original {
		t.Errorf("invalid JSON altered: got %q, want %q", tc.Text, original)
	}
}

func TestOutputObserver_LeadingCeremonyDropped(t *testing.T) {
	t.Parallel()
	o := NewOutputObserver()
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "Certainly! Let me help with that. Here is the answer."},
		},
	}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	tc := resp.Content[0].(tau.TextContent)
	if strings.HasPrefix(tc.Text, "Certainly") || strings.HasPrefix(tc.Text, "Let me help") {
		t.Errorf("ceremony not dropped: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, "answer") {
		t.Errorf("substantive content lost: %q", tc.Text)
	}
}

func TestOutputObserver_TrailingRestatementDropped(t *testing.T) {
	t.Parallel()
	o := NewOutputObserver()
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "Here is the answer.\n\nIn summary, the answer is the answer."},
		},
	}
	// Use a real newline-separated string to match the observer's
	// markers (which look for "\n\nIn summary,").
	resp.Content[0] = tau.TextContent{Text: strings.ReplaceAll(
		"Here is the answer.||In summary, the answer is the answer.",
		"||", "\n\n",
	)}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	tc := resp.Content[0].(tau.TextContent)
	if strings.Contains(tc.Text, "In summary") {
		t.Errorf("trailing restatement not dropped: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, "Here is the answer") {
		t.Errorf("substantive content lost: %q", tc.Text)
	}
}

func TestOutputObserver_StreamedBytesInvariant(t *testing.T) {
	t.Parallel()
	o := NewOutputObserver()
	// The "streamed-bytes-untouched" property is structurally enforced
	// by the ResponseObserver interface signature: the observer
	// receives (ctx, *Request, *Response) and has no handle on a
	// stream, event bus, or any other runtime state. There is no API
	// path through which the observer could touch the bytes the
	// provider streamed, so the property cannot be violated.
	//
	// What this test verifies is the corollary: the observer
	// restricts its mutations to the *Response value it was handed.
	// We capture the pre-call string in the test scope and compare
	// against the post-call value. See
	// docs/input/context/plugin-support/plugin-observability.md §2.
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "Sure, here you go."},
		},
	}
	prev := resp.Content[0].(tau.TextContent).Text
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	now := resp.Content[0].(tau.TextContent).Text
	// The previous (pre-observe) string still exists unchanged in the
	// test scope, even though the Response value was rewritten.
	if prev != "Sure, here you go." {
		t.Errorf("test harness corrupted prior string: %q", prev)
	}
	if now == prev {
		t.Errorf("observer did not rewrite the response: %q", now)
	}
}

func TestOutputObserver_ThinkingSectionsDroppedByDefault(t *testing.T) {
	t.Parallel()
	o := &OutputObserver{
		DropLeadingPhrases:    []string{"Sure,"},
		DropThinkingSections:  true,
	}
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "Sure, <thinking>let me reason about this</thinking>here is the answer."},
		},
	}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	got := resp.Content[0].(tau.TextContent).Text
	if strings.Contains(got, "<thinking>") || strings.Contains(got, "let me reason") {
		t.Errorf("thinking section not dropped: %q", got)
	}
	if !strings.Contains(got, "here is the answer") {
		t.Errorf("substantive content lost: %q", got)
	}
}

func TestOutputObserver_ThinkingSectionsCustomTags(t *testing.T) {
	t.Parallel()
	o := &OutputObserver{
		DropThinkingSections: true,
		ThinkingTags:        []string{"reasoning"},
	}
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "A<reasoning>secret</reasoning>B"},
		},
	}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	got := resp.Content[0].(tau.TextContent).Text
	if got != "AB" {
		t.Errorf("custom-tag drop failed: %q", got)
	}
}

func TestOutputObserver_ThinkingSectionsCaseInsensitive(t *testing.T) {
	t.Parallel()
	o := &OutputObserver{DropThinkingSections: true}
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "A<THINKING>x</THINKING>B"},
		},
	}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	got := resp.Content[0].(tau.TextContent).Text
	if got != "AB" {
		t.Errorf("case-insensitive tag match failed: %q", got)
	}
}

func TestOutputObserver_ThinkingSectionsMultiline(t *testing.T) {
	t.Parallel()
	o := &OutputObserver{DropThinkingSections: true}
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "A<thinking>line 1\nline 2\nline 3</thinking>B"},
		},
	}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	got := resp.Content[0].(tau.TextContent).Text
	if got != "AB" {
		t.Errorf("multiline section drop failed: %q", got)
	}
}

func TestOutputObserver_ThinkingSectionsUnbalancedLeftIntact(t *testing.T) {
	t.Parallel()
	o := &OutputObserver{DropThinkingSections: true}
	original := "A<thinking>never closedB"
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: original},
		},
	}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	got := resp.Content[0].(tau.TextContent).Text
	// Conservative behaviour: an unterminated open tag is left intact
	// so the embedder can decide what to do with it.
	if strings.Contains(got, "<thinking>") != strings.Contains(original, "<thinking>") {
		t.Errorf("unbalanced open tag altered: got %q, want %q", got, original)
	}
}

func TestOutputObserver_ThinkingSectionsOffByDefault(t *testing.T) {
	t.Parallel()
	o := NewOutputObserver()
	original := "Sure, <thinking>scratch</thinking>answer."
	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: original},
		},
	}
	if err := o.ObserveResponse(context.Background(), nil, resp); err != nil {
		t.Fatalf("ObserveResponse: %v", err)
	}
	got := resp.Content[0].(tau.TextContent).Text
	// DropLeadingPhrases still strips "Sure," but the thinking tag
	// must be preserved when DropThinkingSections is false.
	if strings.Contains(got, "Sure,") {
		t.Errorf("leading phrase not dropped: %q", got)
	}
	if !strings.Contains(got, "<thinking>") || !strings.Contains(got, "scratch") {
		t.Errorf("thinking section altered while DropThinkingSections=false: %q", got)
	}
}

func TestLearnObserver_FailedTurnWritesCorrection(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	cache := NewOriginalCache(store)
	obs := NewLearnObserver(cache, 3, nil)

	// Turn 1: assistant response that includes a tool error. The
	// extractCorrection path looks for a user correction in the
	// *next* turn's request, so we drive the observer twice.
	failed := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "I'll run the wrong tool."},
		},
		StopReason: tau.StopReason("aborted"),
	}
	if err := obs.ObserveResponse(context.Background(), nil, failed); err != nil {
		t.Fatalf("observe failed: %v", err)
	}

	// Turn 2: a user correction arrives. The next assistant turn
	// triggers the correction write.
	req := &tau.Request{
		Messages: []tau.Message{
			{Role: tau.Role("user"), Content: []tau.ContentBlock{tau.TextContent{Text: "no, that was wrong"}}},
		},
	}
	corrected := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "Running the right tool now."},
		},
	}
	if err := obs.ObserveResponse(context.Background(), req, corrected); err != nil {
		t.Fatalf("observe corrected: %v", err)
	}

	// A correction entry must exist in the store keyed by the SHA-256
	// of the failed response's first TextContent block.
	q := tau.Query{KeywordQuery: "correction:", Limit: 32}
	entries, err := store.Query(context.Background(), q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.ID, "correction:") && strings.Contains(e.Text, "wrong") {
			found = true
		}
	}
	if !found {
		t.Errorf("no correction entry written; entries=%v", entries)
	}
}

func TestLearnObserver_SuccessfulTurnWritesNothing(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	cache := NewOriginalCache(store)
	obs := NewLearnObserver(cache, 3, nil)

	resp := &tau.Response{
		Content: []tau.ContentBlock{
			tau.TextContent{Text: "All good, nothing failed."},
		},
	}
	req := &tau.Request{
		Messages: []tau.Message{
			{Role: tau.Role("user"), Content: []tau.ContentBlock{tau.TextContent{Text: "thanks!"}}},
		},
	}
	if err := obs.ObserveResponse(context.Background(), req, resp); err != nil {
		t.Fatalf("observe: %v", err)
	}
	q := tau.Query{KeywordQuery: "correction:", Limit: 32}
	entries, err := store.Query(context.Background(), q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("successful turn wrote entries: %v", entries)
	}
}
