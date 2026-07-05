package headroom

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tau "github.com/coevin/tau/pkg/tau"
)

func newRetrieveWithStore(t *testing.T) (*Retrieve, *memStore) {
	t.Helper()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	cache := NewOriginalCache(store)
	return NewRetrieve(cache), store
}

func TestRetrieve_NameAndDescription(t *testing.T) {
	t.Parallel()
	r, _ := newRetrieveWithStore(t)
	if r.Name() != "ccr_retrieve" {
		t.Errorf("Name = %q, want \"ccr_retrieve\"", r.Name())
	}
	if r.Description() == "" {
		t.Errorf("Description is empty")
	}
}

func TestRetrieve_ParametersIsValidJSONSchema(t *testing.T) {
	t.Parallel()
	r, _ := newRetrieveWithStore(t)
	s := r.Parameters()
	bs, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(bs, &round); err != nil {
		t.Fatalf("schema does not round-trip: %v\n%s", err, bs)
	}
	if round["type"] != "object" {
		t.Errorf("schema type = %v, want \"object\"", round["type"])
		props, _ := round["properties"].(map[string]any)
		if _, ok := props["hash"]; !ok {
			t.Errorf("schema has no \"hash\" property: %s", bs)
		}
		required, _ := round["required"].([]any)
		found := false
		for _, r := range required {
			if r == "hash" {
				found = true
			}
		}
		if !found {
			t.Errorf("schema does not mark \"hash\" required: %s", bs)
		}
	}
}

func TestRetrieve_ExecuteHit(t *testing.T) {
	t.Parallel()
	r, store := newRetrieveWithStore(t)
	cache := NewOriginalCache(store)
	original := `{"x":1,"y":2}`
	hash, err := cache.Put(context.Background(), []byte(original), ContentTypeJSON)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	call := tau.ToolCall{Args: mustMarshal(t, map[string]string{"hash": hash})}
	res, err := r.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError=true on hit; content=%v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatalf("empty content")
	}
	tc, ok := res.Content[0].(tau.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T", res.Content[0])
	}
	if tc.Text != original {
		t.Errorf("got %q, want %q", tc.Text, original)
	}
}

func TestRetrieve_ExecuteMiss(t *testing.T) {
	t.Parallel()
	r, _ := newRetrieveWithStore(t)
	call := tau.ToolCall{Args: mustMarshal(t, map[string]string{"hash": strings.Repeat("0", 64)})}
	res, err := r.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned unexpected err: %v", err)
	}
	if !res.IsError {
		t.Errorf("IsError=false on miss")
	}
}

func TestRetrieve_ExecuteMalformedArgs(t *testing.T) {
	t.Parallel()
	r, _ := newRetrieveWithStore(t)
	cases := []tau.ToolCall{
		{Args: nil},
		{Args: []byte(`not json`)},
		{Args: []byte(`{"wrong":"field"}`)},
		{Args: []byte(`{"hash":""}`)},
	}
	for i, call := range cases {
		res, err := r.Execute(context.Background(), call)
		if err != nil {
			t.Errorf("case %d: Execute returned go err: %v", i, err)
			continue
		}
		if !res.IsError {
			t.Errorf("case %d: IsError=false, want true", i)
		}
	}
}

func TestRetrieve_ExecuteClosedStore(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	cache := NewOriginalCache(store)
	r := NewRetrieve(cache)
	// Close the store first.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	call := tau.ToolCall{Args: mustMarshal(t, map[string]string{"hash": strings.Repeat("0", 64)})}
	// Must not panic.
	res, err := r.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute panicked or returned err: %v", err)
	}
	if !res.IsError {
		t.Errorf("IsError=false on closed store")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	bs, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bs
}
